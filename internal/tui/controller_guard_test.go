package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/undo"
)

// A controller surface owns open, submit and close, and nothing about it needs
// *App. These guards pin that claim from both sides: no method on the surface
// names App, no production code reads a surface field through App, and every
// dependency the surface is handed is a live func rather than a captured
// pointer. All three run over controllerSurfaces, which is the only list of
// controller surfaces; TestGuard_ControllerTableMatchesApp keeps it complete.
//
// They do NOT claim a surface could compile in its own package: each still
// names package-level helpers such as errMsg and buildCategoryOptions. The
// guards are about the *App boundary only.

// controllerSurface is one row of the table: the App field that holds the
// surface, its struct type, its deps type, and the App method that binds them.
type controllerSurface struct {
	field   string
	surface reflect.Type
	// deps and bind are nil for a surface that needs no service; the two deps
	// guards skip such a row and the two state guards still run over it.
	deps reflect.Type
	bind func(*App) any
	// probes calls each dep accessor in turn. It is a hand list because reflect
	// cannot call an unexported func field; TestControllerDeps_FollowADatabaseSwitch
	// requires exactly one probe per dep so the list cannot fall behind the struct.
	probes func(*App) []func() any
}

// controllerSurfaces is every surface that has been through the 4c motion.
var controllerSurfaces = []controllerSurface{
	{
		field:   "transfer",
		surface: reflect.TypeFor[transferSurface](),
		deps:    reflect.TypeFor[transferDeps](),
		bind:    func(a *App) any { return a.transferDeps() },
		probes: func(a *App) []func() any {
			d := a.transferDeps()
			return []func() any{
				func() any { return d.accounts() },
				func() any { return d.categories() },
				func() any { return d.transfers() },
				func() any { return d.undo() },
			}
		},
	},
	{
		field:   "paycheck",
		surface: reflect.TypeFor[paycheckSurface](),
		deps:    reflect.TypeFor[paycheckDeps](),
		bind:    func(a *App) any { return a.paycheckDeps() },
		probes: func(a *App) []func() any {
			d := a.paycheckDeps()
			return []func() any{
				func() any { return d.accounts() },
				func() any { return d.categories() },
				func() any { return d.payees() },
				func() any { return d.scheduled() },
				func() any { return d.undo() },
			}
		},
	},
	{
		field:   "split",
		surface: reflect.TypeFor[splitSurface](),
		deps:    reflect.TypeFor[splitDeps](),
		bind:    func(a *App) any { return a.splitDeps() },
		probes: func(a *App) []func() any {
			d := a.splitDeps()
			return []func() any{
				func() any { return d.payees() },
				func() any { return d.transactions() },
				func() any { return d.undo() },
			}
		},
	},
	{
		field:   "loan",
		surface: reflect.TypeFor[loanSurface](),
		deps:    reflect.TypeFor[loanDeps](),
		bind:    func(a *App) any { return a.loanDeps() },
		probes: func(a *App) []func() any {
			d := a.loanDeps()
			return []func() any{
				func() any { return d.accounts() },
				func() any { return d.categories() },
				func() any { return d.payees() },
				func() any { return d.scheduled() },
				func() any { return d.undo() },
			}
		},
	},
	{
		// The sub-dialog every other surface diverts into. It emits a message
		// and touches no service, so it has no deps; the router that persists
		// the category and re-shows the originator is App's, because it
		// coordinates eight sibling surfaces.
		field:   "createCat",
		surface: reflect.TypeFor[createCatSurface](),
	},
}

// TestGuard_ControllerTableMatchesApp keeps the table honest from the App side:
// every row names a real App field of the row's surface type, and every
// controller surface is in the table. A surface is a controller when it has a
// <name>Deps struct OR declares a no-arg close() — the second detector exists
// because a surface that needs no service (createCatSurface) has no deps
// struct and would otherwise never trip the check. Without this, a surface
// could be added and never guarded.
func TestGuard_ControllerTableMatchesApp(t *testing.T) {
	appT := reflect.TypeFor[App]()
	for _, row := range controllerSurfaces {
		f, ok := appT.FieldByName(row.field)
		if !ok {
			t.Errorf("controllerSurfaces names App.%s, which does not exist", row.field)
			continue
		}
		if f.Type != row.surface {
			t.Errorf("App.%s is a %s, but the table says %s", row.field, f.Type, row.surface)
		}
	}
	// The reverse direction: a surface type with a sibling deps type is a
	// controller, whether or not someone remembered the table.
	inTable := map[string]bool{}
	for _, row := range controllerSurfaces {
		inTable[row.surface.Name()] = true
	}
	for _, path := range productionGoFiles(t) {
		for _, name := range depsStructNames(t, readSourceFile(t, path)) {
			surface := strings.TrimSuffix(name, "Deps") + "Surface"
			if !inTable[surface] {
				t.Errorf("%s declares %s, so %s is a controller surface, but it is not in "+
					"controllerSurfaces. Add a row; the guards run over the table only.", path, name, surface)
			}
		}
		// A surface that owns close() has been through the controller motion,
		// whether or not it needed deps — createCatSurface has none. This is
		// the detector that catches a deps-free surface left out of the table.
		for _, surface := range receiversDeclaringClose(t, readSourceFile(t, path)) {
			if !inTable[surface] {
				t.Errorf("%s declares (*%s).close, so it is a controller surface, but it is not in "+
					"controllerSurfaces. Add a row; the guards run over the table only.", path, surface)
			}
		}
	}
}

// TestGuard_ControllerSurfacesNameNoApp: no method on a controller surface may
// mention App. A method that takes *App is a controller in name only — it can
// reach the status bar, a sibling surface or a service field, and the guard
// would prove nothing.
func TestGuard_ControllerSurfacesNameNoApp(t *testing.T) {
	for _, row := range controllerSurfaces {
		t.Run(row.field, func(t *testing.T) {
			// Every production file, not just the surface's own: Go lets a method
			// live in any file of the package, so a file-scoped guard would
			// advertise a package-wide invariant it cannot see.
			seen := 0
			for _, path := range productionGoFiles(t) {
				methods, offenders := methodsNamingApp(t, readSourceFile(t, path), row.surface.Name())
				seen += methods
				for _, name := range offenders {
					t.Errorf("%s: (*%s).%s names App. A surface method takes what it "+
						"needs as a value or through its deps; App state reaches it as a "+
						"parameter, never as a receiver.", path, row.surface.Name(), name)
				}
			}
			if seen == 0 {
				t.Fatalf("no method on *%s found in any production file, so this "+
					"guard would pass vacuously. Either the surface was renamed or it is gone.", row.surface.Name())
			}
		})
	}
}

// TestGuard_ControllerStateIsReachedOnlyByMethod: no production code may read a
// controller surface's field through App. Every a.<surface>.X in package tui
// must be a method call, which is what makes the surface's state its own.
//
// Test files are excluded, and that exclusion is the honest measure rather than
// a loophole: section 3 of the design counts 2,084 app.<unexported> references
// across the 66 test files as the reason a package split is expensive. This
// guard says the production code no longer reaches in. The tests still do.
func TestGuard_ControllerStateIsReachedOnlyByMethod(t *testing.T) {
	for _, row := range controllerSurfaces {
		t.Run(row.field, func(t *testing.T) {
			fields := structFieldNames(row.surface)
			if len(fields) == 0 {
				t.Fatalf("%s has no fields; this guard would pass vacuously", row.surface.Name())
			}
			for _, path := range productionGoFiles(t) {
				src := readSourceFile(t, path)
				for _, reach := range appFieldReaches(t, src, row.field, fields) {
					t.Errorf("%s reads a.%s.%s. The surface's state is private to the "+
						"surface: add a method for what the caller needs.", path, row.field, reach)
				}
			}
		})
	}
}

// TestGuard_ControllerDepsAreLiveIndirections: every dep is a function, and the
// App binding supplies all of them.
//
// A service pointer field here would compile and would be a use-after-close the
// first time the user opened another file: switchDatabase replaces the services
// and closes the previous *db.DB. A closure re-reads the field at call time, so
// it survives the switch by construction.
func TestGuard_ControllerDepsAreLiveIndirections(t *testing.T) {
	for _, row := range controllerSurfaces {
		t.Run(row.field, func(t *testing.T) {
			if row.deps == nil {
				t.Skip("surface has no deps")
			}
			if row.deps.NumField() == 0 {
				t.Fatalf("%s has no fields; this guard would pass vacuously", row.deps.Name())
			}
			for i := range row.deps.NumField() {
				f := row.deps.Field(i)
				if f.Type.Kind() != reflect.Func {
					t.Errorf("%s.%s is a %s, not a func. Deps are live indirections: "+
						"switchDatabase replaces App's services and closes the previous database, "+
						"so a captured pointer becomes a use-after-close.", row.deps.Name(), f.Name, f.Type)
				}
			}

			// A zero App still yields a fully populated deps struct — the services
			// it reaches are nil, which every caller already guards, but the funcs
			// are not.
			deps := reflect.ValueOf(row.bind(&App{}))
			if deps.Type() != row.deps {
				t.Fatalf("the binding returned a %s, the table says %s", deps.Type(), row.deps)
			}
			for i := range row.deps.NumField() {
				if deps.Field(i).IsNil() {
					t.Errorf("the binding left %s.%s nil; every dep must be callable",
						row.deps.Name(), row.deps.Field(i).Name)
				}
			}
		})
	}
}

// TestControllerDeps_FollowADatabaseSwitch is the behaviour behind the
// structural guard above. switchDatabase replaces App's services when the user
// opens another file and closes the previous *db.DB, and a deps struct taken
// before that must see the new services — a captured pointer would still name
// the ones built against the closed database, which is a use-after-close, not
// stale data.
//
// The test is generic over the table: take deps from an App with nothing, so
// every accessor returns nil; then give the App a service of every kind and an
// undo manager; every accessor must now return something. A closure captured
// at bind time could not.
func TestControllerDeps_FollowADatabaseSwitch(t *testing.T) {
	for _, row := range controllerSurfaces {
		t.Run(row.field, func(t *testing.T) {
			if row.deps == nil {
				t.Skip("surface has no deps")
			}
			app := &App{}
			probes := row.probes(app)
			if len(probes) != row.deps.NumField() {
				t.Fatalf("%d probes for %d deps in %s; the probe list must name every dep",
					len(probes), row.deps.NumField(), row.deps.Name())
			}
			for i, probe := range probes {
				if !isNilPointer(probe()) {
					t.Fatalf("an App with no services handed out a non-nil %s.%s", row.deps.Name(), row.deps.Field(i).Name)
				}
			}

			// The switch. switchDatabase assigns a.services whole and clears the
			// undo manager rather than replacing it (file_dialog.go), so the
			// manager swap here is synthetic: it proves the closure re-reads the
			// field, which is the property the deps rely on.
			svcV := reflect.ValueOf(&app.services).Elem()
			for i := range svcV.NumField() {
				if f := svcV.Field(i); f.Kind() == reflect.Pointer {
					f.Set(reflect.New(f.Type().Elem()))
				}
			}
			app.undoManager = undo.NewManager()

			for i, probe := range probes {
				if isNilPointer(probe()) {
					t.Errorf("%s.%s did not follow the switch; it is a captured pointer, not an indirection",
						row.deps.Name(), row.deps.Field(i).Name)
				}
			}
		})
	}
}

// receiversDeclaringClose returns the receiver type of every `func (s *T) close()`
// in src — the method every controller surface declares and no other does.
func receiversDeclaringClose(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, d := range parseSource(t, src).Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != "close" {
			continue
		}
		if fn.Type.Params.NumFields() != 0 || fn.Type.Results.NumFields() != 0 {
			continue
		}
		if name := receiverTypeName(fn.Recv.List[0].Type); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// depsStructNames returns the names of every struct type in src whose name ends
// in Deps and whose every field is a func — the deps-bag shape.
func depsStructNames(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, d := range parseSource(t, src).Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			ts := s.(*ast.TypeSpec)
			st, ok := ts.Type.(*ast.StructType)
			if !ok || !strings.HasSuffix(ts.Name.Name, "Deps") || len(st.Fields.List) == 0 {
				continue
			}
			allFuncs := true
			for _, f := range st.Fields.List {
				if _, ok := f.Type.(*ast.FuncType); !ok {
					allFuncs = false
				}
			}
			if allFuncs {
				out = append(out, ts.Name.Name)
			}
		}
	}
	return out
}

// methodsNamingApp is the guard above as a pure function over source text, so
// the self-test can run the same code against fabricated input rather than
// against a copy of the rule that can drift. It returns how many methods on the
// named receiver it saw and which of them mention App.
func methodsNamingApp(t *testing.T, src, receiver string) (seen int, offenders []string) {
	t.Helper()
	file := parseSource(t, src)
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) != receiver {
			continue
		}
		seen++
		ast.Inspect(fn, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == "App" {
				offenders = append(offenders, fn.Name.Name)
				return false
			}
			return true
		})
	}
	sort.Strings(offenders)
	return seen, slices.Compact(offenders)
}

// appFieldReaches returns every `<x>.<surface>.<field>` selector in src whose
// field is one of the surface's own. It matches on the syntax tree rather than
// on text, so a field name appearing in a comment or a string is not a hit.
func appFieldReaches(t *testing.T, src, surface string, fields map[string]bool) []string {
	t.Helper()
	file := parseSource(t, src)
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !fields[sel.Sel.Name] {
			return true
		}
		inner, ok := sel.X.(*ast.SelectorExpr)
		if !ok || inner.Sel.Name != surface {
			return true
		}
		out = append(out, sel.Sel.Name)
		return true
	})
	sort.Strings(out)
	return slices.Compact(out)
}

// structFieldNames returns a struct's field names, following embedded structs so
// a promoted field counts as the outer type's own.
func structFieldNames(st reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := range st.NumField() {
		f := st.Field(i)
		out[f.Name] = true
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for name := range structFieldNames(f.Type) {
				out[name] = true
			}
		}
	}
	return out
}

// productionGoFiles lists the package's non-test sources, relative to the
// package directory.
func productionGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		t.Fatal("no production .go files found; the walk is broken")
	}
	return out
}

func readSourceFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func parseSource(t *testing.T, src string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	return file
}

// receiverTypeName returns the bare type name of a method receiver, with any
// pointer stripped.
func receiverTypeName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// TestGuard_ControllerSelfTest proves the source guards fire, by running the
// same functions the guards run over fabricated input.
func TestGuard_ControllerSelfTest(t *testing.T) {
	t.Run("a surface method taking *App is detected", func(t *testing.T) {
		seen, offenders := methodsNamingApp(t, `package tui

func (s *transferSurface) clean() int { return 1 }
func (s *transferSurface) dirty(a *App) {}
func (a *App) unrelated() {}
`, "transferSurface")
		if seen != 2 {
			t.Errorf("saw %d methods on the receiver, want 2", seen)
		}
		if !slices.Equal(offenders, []string{"dirty"}) {
			t.Errorf("offenders = %v, want [dirty]", offenders)
		}
	})

	t.Run("a surface method building an App in its body is detected", func(t *testing.T) {
		_, offenders := methodsNamingApp(t, `package tui

func (s *transferSurface) sneaky() any { return &App{} }
`, "transferSurface")
		if !slices.Equal(offenders, []string{"sneaky"}) {
			t.Errorf("offenders = %v, want [sneaky] (an App in the body counts)", offenders)
		}
	})

	t.Run("a field read through App is detected", func(t *testing.T) {
		fields := map[string]bool{"dlg": true, "data": true}
		got := appFieldReaches(t, `package tui

func (a *App) reachIn() {
	if a.transfer.dlg != nil {
		_ = a.transfer.data
	}
	a.transfer.close()
	_ = a.txn.dlg
}
`, "transfer", fields)
		if !slices.Equal(got, []string{"data", "dlg"}) {
			t.Errorf("reaches = %v, want [data dlg]; a method call and another "+
				"surface's field must not count", got)
		}
	})

	t.Run("reflection finds the surface's own and promoted fields", func(t *testing.T) {
		fields := structFieldNames(reflect.TypeFor[transferSurface]())
		for _, want := range []string{"dlg", "data", "accountIDs", "categoryIDs"} {
			if !fields[want] {
				t.Errorf("structFieldNames missed %q, so the guard would not see a read of it", want)
			}
		}
	})
}

// TestGuard_ControllerSelfTest_DepsFinder proves the table's reverse check sees
// a deps struct and ignores a struct that merely ends in Deps.
func TestGuard_ControllerSelfTest_DepsFinder(t *testing.T) {
	got := depsStructNames(t, `package tui

type fooDeps struct {
	a func() int
	b func() string
}
type notReallyDeps struct {
	a func() int
	n int
}
type emptyDeps struct{}
`)
	if !slices.Equal(got, []string{"fooDeps"}) {
		t.Errorf("depsStructNames = %v, want [fooDeps]", got)
	}
}

// isNilPointer reports whether v holds a nil pointer. A nil *T stored in an any
// is not a nil interface, so a plain == nil check would read every dep as set.
func isNilPointer(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// TestGuard_ControllerSelfTest_CloseFinder proves the close() detector sees a
// controller surface and ignores a close with a different shape.
func TestGuard_ControllerSelfTest_CloseFinder(t *testing.T) {
	got := receiversDeclaringClose(t, `package tui

func (s *fooSurface) close() { *s = fooSurface{} }
func (s *barSurface) close(force bool) {}
func (s *bazSurface) closeAll() {}
func close() {}
`)
	if !slices.Equal(got, []string{"fooSurface"}) {
		t.Errorf("receiversDeclaringClose = %v, want [fooSurface]", got)
	}
}
