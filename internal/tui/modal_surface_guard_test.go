package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
)

// Guard for section 5.5: a per-surface state struct holds form state, never a
// service. App keeps the services and passes them in at call time.
//
// The rule is not stylistic. switchDatabase re-points 18 service fields when
// the user opens another file and closes the previous *db.DB, so a surface that
// captured a service pointer at open time would be a use-after-close — the
// exact class commit 6dede4d fixed.
//
// AND THE EXISTING REGRESSION TEST CANNOT SEE IT. switch_database_test.go
// reflects over App's TOP-LEVEL fields only; it does not recurse. The moment a
// service pointer moves inside a surface struct it silently leaves that guard's
// coverage and the test still passes. Its anti-vacuity check does not help
// either: it proves *some* fields matched, not that the ones just moved are
// still among them.
//
// The design offered two fixes — recurse one level in switch_database_test, or
// forbid the field outright. This is the second, chosen because it is the
// invariant the design actually wants and it cannot rot into a
// partially-covering walk.
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

	t.Run("surfaceStructTypes finds the real surfaces", func(t *testing.T) {
		var names []string
		for _, st := range surfaceStructTypes(t) {
			names = append(names, st.Name())
		}
		joined := strings.Join(names, ",")
		for _, want := range []string{"securitySurface", "closeAcctSurface"} {
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

// surfaceWithUnsafeIsVisible declares IsVisible the wrong way: it compiles, so
// only a call can catch it.
type surfaceWithUnsafeIsVisible struct {
	modalSurface
	data int
}

func (s *surfaceWithUnsafeIsVisible) IsVisible() bool { return s.dlg.IsVisible() }
