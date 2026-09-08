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

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/undo"
)

// The phase 5 pilot's claim is narrow and checkable: the transfer surface owns
// open, submit and close, and nothing about it needs *App. These guards pin the
// two halves of that claim from opposite sides — the surface does not reach out,
// and App does not reach in — because both are one careless line from being
// false again, and neither is a compile error while the two live in one package.
//
// What they deliberately do NOT claim is that transferSurface could compile in
// its own package today. It still names errMsg, parseAmountInput, parseDateInput,
// buildCategoryOptions and topLevelParentNames, all declared in package tui.
// Section 3 of the design measures that cost and defers the decision; these
// guards are about the *App boundary, which is the one phase 5 moved.

// TestGuard_TransferSurfaceNamesNoApp: no method on *transferSurface may
// mention App. A method that takes *App is a controller in name only — it can
// reach the status bar, a sibling surface or a service field, and the pilot
// would prove nothing.
func TestGuard_TransferSurfaceNamesNoApp(t *testing.T) {
	src := readSourceFile(t, "transfer_dialog.go")
	methods, offenders := methodsNamingApp(t, src, "transferSurface")
	if methods == 0 {
		t.Fatal("no method on *transferSurface found in transfer_dialog.go, so this " +
			"guard would pass vacuously. If the surface moved file, point the guard at it.")
	}
	for _, name := range offenders {
		t.Errorf("(*transferSurface).%s names App. A surface method takes what it needs "+
			"as a value or through transferDeps; App state reaches it as a parameter, "+
			"never as a receiver.", name)
	}
}

// TestGuard_TransferStateIsReachedOnlyByMethod: no production code may read a
// transferSurface field through App. Every a.transfer.X in package tui must be a
// method call, which is what makes the surface's state its own.
//
// Test files are excluded, and that exclusion is the honest measure rather than
// a loophole: section 3 of the design counts 2,084 app.<unexported> references
// across the 66 test files as the reason a package split is expensive. This
// guard says the production code no longer reaches in. The tests still do.
func TestGuard_TransferStateIsReachedOnlyByMethod(t *testing.T) {
	fields := structFieldNames(reflect.TypeFor[transferSurface]())
	if len(fields) == 0 {
		t.Fatal("transferSurface has no fields; this guard would pass vacuously")
	}

	for _, path := range productionGoFiles(t) {
		src := readSourceFile(t, path)
		for _, reach := range appFieldReaches(t, src, "transfer", fields) {
			t.Errorf("%s reads a.transfer.%s. The transfer surface's state is private to "+
				"the surface: add a method for what the caller needs.", path, reach)
		}
	}
}

// TestGuard_TransferDepsAreLiveIndirections: every dep is a function, and
// App.transferDeps supplies all of them.
//
// A *transfer.Service field here would compile and would be a use-after-close
// the first time the user opened another file: switchDatabase re-points the
// service fields and closes the previous *db.DB. A closure re-reads the field at
// call time, so it survives the switch by construction.
func TestGuard_TransferDepsAreLiveIndirections(t *testing.T) {
	depsT := reflect.TypeFor[transferDeps]()
	if depsT.NumField() == 0 {
		t.Fatal("transferDeps has no fields; this guard would pass vacuously")
	}
	for i := range depsT.NumField() {
		f := depsT.Field(i)
		if f.Type.Kind() != reflect.Func {
			t.Errorf("transferDeps.%s is a %s, not a func. Deps are live indirections: "+
				"switchDatabase re-points App's services and closes the previous database, "+
				"so a captured pointer becomes a use-after-close.", f.Name, f.Type)
		}
	}

	// A zero App still yields a fully populated deps struct — the services it
	// reaches are nil, which every caller already guards, but the funcs are not.
	deps := reflect.ValueOf((&App{}).transferDeps())
	for i := range depsT.NumField() {
		if deps.Field(i).IsNil() {
			t.Errorf("App.transferDeps left %s nil; every dep must be callable",
				depsT.Field(i).Name)
		}
	}
}

// TestTransferDeps_FollowADatabaseSwitch is the behaviour behind the structural
// guard above. switchDatabase re-points App's service fields when the user opens
// another file and closes the previous *db.DB, and a deps struct taken before
// that must see the new services — a captured pointer would still name the ones
// built against the closed database, which is a use-after-close, not stale data.
func TestTransferDeps_FollowADatabaseSwitch(t *testing.T) {
	app := &App{}
	deps := app.transferDeps()

	if deps.transfers() != nil || deps.accounts() != nil || deps.categories() != nil || deps.undo() != nil {
		t.Fatal("an App with no services must hand out nil services, not a panic")
	}

	// What switchDatabase does, in one line each.
	transfers := &transfer.Service{}
	accounts := &account.Service{}
	categories := &category.Service{}
	manager := undo.NewManager()
	app.transferSvc = transfers
	app.accountSvc = accounts
	app.categorySvc = categories
	app.undoManager = manager

	if deps.transfers() != transfers {
		t.Error("deps.transfers did not follow the switch; it is a captured pointer, not an indirection")
	}
	if deps.accounts() != accounts {
		t.Error("deps.accounts did not follow the switch")
	}
	if deps.categories() != categories {
		t.Error("deps.categories did not follow the switch")
	}
	if deps.undo() != manager {
		t.Error("deps.undo did not follow the switch")
	}
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

// TestGuard_TransferControllerSelfTest proves both source guards fire, by
// running the same functions the guards run over fabricated input.
func TestGuard_TransferControllerSelfTest(t *testing.T) {
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
