package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
)

// A per-surface state struct holds form state, never a service. switchDatabase
// re-points every service field and closes the previous *db.DB, so a service
// pointer captured on a surface would be a use-after-close. switch_database_test
// walks App's top-level fields only and cannot see one that moved inside a
// surface, hence this guard.
func TestGuard_NoSurfaceStructHoldsAService(t *testing.T) {
	serviceTypes := servicePointerTypes()
	if len(serviceTypes) == 0 {
		t.Fatal("app.Services exposes no pointer fields; this guard would pass vacuously")
	}

	surfaces := surfaceStructTypes(t)
	if len(surfaces) == 0 {
		t.Fatal("no surface struct found on App, so this guard would pass vacuously. " +
			"Surfaces embed modalSurface; if that changed, update this filter.")
	}

	for _, st := range surfaces {
		for i := range st.NumField() {
			f := st.Field(i)
			if serviceTypes[f.Type] {
				t.Errorf("%s.%s is a %s — a surface struct must not hold a service. "+
					"switchDatabase re-points services and closes the previous database, "+
					"so a captured pointer becomes a use-after-close. Keep the service on "+
					"App and pass it in at call time.", st.Name(), f.Name, f.Type)
			}
		}
	}
}

// TestGuard_NoSurfaceStructHoldsItsDeps: a surface must not store its
// dependency bag as a field.
//
// close() is `*s = surface{}` for every surface, because a surface's zero value
// IS its closed state. A stored deps struct would be zeroed by that reset, and
// the next call through it would panic on a nil func — the trap the transferDeps
// doc comment describes and that nothing else here would catch. The service
// guard above cannot see it either: a struct of closures holds no service
// pointer.
//
// The rule is narrow on purpose. A single func field is legitimate surface
// state — confirmSurface.action is the pending continuation, and close() SHOULD
// clear it. What is forbidden is a bag whose every field is a func, which is
// what a deps struct is. Deps travel as a call parameter.
func TestGuard_NoSurfaceStructHoldsItsDeps(t *testing.T) {
	surfaces := surfaceStructTypes(t)
	if len(surfaces) == 0 {
		t.Fatal("no surface struct found on App; this guard would pass vacuously")
	}
	for _, st := range surfaces {
		for i := range st.NumField() {
			f := st.Field(i)
			if isDependencyBag(f.Type) {
				t.Errorf("%s.%s is a %s — every field of it is a func, so it is a "+
					"dependency bag, and close() would zero it. Pass deps in as a "+
					"parameter to the methods that need them.", st.Name(), f.Name, f.Type)
			}
		}
	}
}

// isDependencyBag reports whether t is a non-empty struct whose every field is a
// func — the shape of transferDeps and of any deps struct that follows it.
func isDependencyBag(t reflect.Type) bool {
	if t.Kind() != reflect.Struct || t.NumField() == 0 {
		return false
	}
	for i := range t.NumField() {
		if t.Field(i).Type.Kind() != reflect.Func {
			return false
		}
	}
	return true
}

// TestGuard_EverySurfaceIsVisibleIsNilSafe is the phase 3 shape of the section
// 5.0 typed-nil trap. The registry holds surfaces now, and App builds them
// lazily, so a nil surface in a Modal interface value is the common case rather
// than the edge one.
//
// PRESENCE is already enforced by the compiler: modalSurface deliberately does
// not provide IsVisible, so a surface that forgets to declare one does not
// implement Modal and cannot go in the registry. What the compiler cannot check
// is the nil guard inside it — `return s.dlg.IsVisible()` compiles and panics.
// That is what this calls, on a nil pointer of every surface type.
func TestGuard_EverySurfaceIsVisibleIsNilSafe(t *testing.T) {
	surfaces := surfaceStructTypes(t)
	if len(surfaces) == 0 {
		t.Fatal("no surface struct found on App; this guard would pass vacuously")
	}
	for _, st := range surfaces {
		t.Run(st.Name(), func(t *testing.T) {
			if visible, panicked := callIsVisibleOnNil(reflect.PointerTo(st)); panicked {
				t.Errorf("(*%s)(nil).IsVisible() panicked. Declare it nil-safe: "+
					"func (s *%s) IsVisible() bool { return s != nil && s.dlg.IsVisible() }",
					st.Name(), st.Name())
			} else if visible {
				t.Errorf("(*%s)(nil).IsVisible() reported true", st.Name())
			}
		})
	}
}

// callIsVisibleOnNil calls IsVisible on a nil pointer of the given type,
// reporting the result and whether it panicked.
func callIsVisibleOnNil(ptrType reflect.Type) (visible, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	m := reflect.Zero(ptrType).MethodByName("IsVisible")
	if !m.IsValid() {
		return false, true
	}
	return m.Call(nil)[0].Bool(), false
}

// surfaceStructTypes returns every struct type reachable from an App field that
// embeds modalSurface — the phase 3 per-surface state structs.
func surfaceStructTypes(t *testing.T) []reflect.Type {
	t.Helper()
	base := reflect.TypeFor[modalSurface]()
	appT := reflect.TypeFor[App]()
	var out []reflect.Type
	for i := range appT.NumField() {
		ft := appT.Field(i).Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			continue
		}
		for j := range ft.NumField() {
			if f := ft.Field(j); f.Anonymous && f.Type == base {
				out = append(out, ft)
				break
			}
		}
	}
	return out
}

// servicePointerTypes is every pointer type app.Services hands out — the same
// rule switch_database_test.go uses, so the two guards agree on what a service
// is.
func servicePointerTypes() map[reflect.Type]bool {
	out := map[reflect.Type]bool{}
	svcT := reflect.TypeOf(app.Services{})
	for i := range svcT.NumField() {
		if f := svcT.Field(i); f.Type.Kind() == reflect.Ptr {
			out[f.Type] = true
		}
	}
	return out
}

// TestGuard_SurfaceGuardSelfTest proves both guards above fire, by running the
// same predicates over fabricated types rather than over a copy of the rule.
func TestGuard_SurfaceGuardSelfTest(t *testing.T) {
	t.Run("a service-typed field is detected", func(t *testing.T) {
		serviceTypes := servicePointerTypes()
		bad := reflect.TypeFor[surfaceWithAService]()
		found := false
		for i := range bad.NumField() {
			if serviceTypes[bad.Field(i).Type] {
				found = true
			}
		}
		if !found {
			t.Error("the service-type predicate missed a service-typed field")
		}
	})

	t.Run("a surface whose IsVisible omits the nil check is detected", func(t *testing.T) {
		_, panicked := callIsVisibleOnNil(reflect.TypeFor[*surfaceWithUnsafeIsVisible]())
		if !panicked {
			t.Error("the nil-safety predicate missed an IsVisible without its nil check")
		}
		if _, panicked := callIsVisibleOnNil(reflect.TypeFor[*securitySurface]()); panicked {
			t.Error("the nil-safety predicate reported a panic on a correct surface")
		}
	})

	t.Run("a stored dependency bag is detected", func(t *testing.T) {
		if !isDependencyBag(reflect.TypeFor[transferDeps]()) {
			t.Error("isDependencyBag missed transferDeps, the very shape it exists to reject")
		}
		bad := reflect.TypeFor[surfaceWithStoredDeps]()
		found := false
		for i := range bad.NumField() {
			if isDependencyBag(bad.Field(i).Type) {
				found = true
			}
		}
		if !found {
			t.Error("the deps predicate missed a surface holding its deps")
		}
	})

	t.Run("a single continuation field is not a dependency bag", func(t *testing.T) {
		// confirmSurface.action is per-open state that close() must clear, not a
		// dep. A blanket "no func fields" rule would fire on it, and would be
		// wrong.
		st := reflect.TypeFor[confirmSurface]()
		for i := range st.NumField() {
			if f := st.Field(i); isDependencyBag(f.Type) {
				t.Errorf("the deps predicate flagged confirmSurface.%s, which is state", f.Name)
			}
		}
		if isDependencyBag(reflect.TypeFor[func() int]()) {
			t.Error("a bare func is not a struct and must not read as a bag")
		}
	})

	t.Run("surfaceStructTypes finds the real surfaces", func(t *testing.T) {
		var names []string
		for _, st := range surfaceStructTypes(t) {
			names = append(names, st.Name())
		}
		joined := strings.Join(names, ",")
		// All six current surfaces, so the nil-safety guard cannot go quiet on
		// one the finder stopped seeing.
		for _, want := range []string{
			"closeAcctSurface", "securitySurface", "priceSurface",
			"loanSurface", "importSurface", "linkTransfersSurface",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("surfaceStructTypes missed %s (found %v)", want, names)
			}
		}
	})
}

// surfaceWithAService and surfaceWithoutIsVisible exist only as fixtures for
// the self-test above. They are never held by App.
type surfaceWithAService struct {
	modalSurface
	accounts *account.Service
}

func (s *surfaceWithAService) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// surfaceWithStoredDeps is the mistake the deps guard exists to reject: close()
// would zero the bag and the next call through it would panic on a nil func.
type surfaceWithStoredDeps struct {
	modalSurface
	deps transferDeps
}

func (s *surfaceWithStoredDeps) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// surfaceWithUnsafeIsVisible declares IsVisible the wrong way: it compiles, so
// only a call can catch it.
type surfaceWithUnsafeIsVisible struct {
	modalSurface
	data int
}

func (s *surfaceWithUnsafeIsVisible) IsVisible() bool { return s.dlg.IsVisible() }
