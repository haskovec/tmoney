package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// The view table in views.go is the one list of views. These guards keep it
// the only list: every View constant has exactly one entry, and no other code
// spells the views out again.

// TestGuard_EveryViewHasOneEntry is guard 1: each View constant in app.go has
// exactly one entry in the table, and the table has no entry for a value that
// is not a constant.
func TestGuard_EveryViewHasOneEntry(t *testing.T) {
	consts := viewConstants(t)

	entries := map[View]int{}
	for _, e := range views() {
		entries[e.id]++
	}
	for _, vc := range consts {
		switch n := entries[vc.Value]; n {
		case 1:
		case 0:
			t.Errorf("%s has no entry in allViews (views.go); add one", vc.Name)
		default:
			t.Errorf("%s has %d entries in allViews (views.go); want exactly one", vc.Name, n)
		}
		delete(entries, vc.Value)
	}
	for id := range entries {
		t.Errorf("allViews has an entry for View(%d), which is not a View constant", id)
	}
}

// maxViewSwitchCases is the most View constants a switch outside views.go may
// name. Four is the largest feature switch (refreshAfterCorporateAction): a
// list of which views have a feature, not a list of the views. Raise it only
// as a visible decision.
const maxViewSwitchCases = 4

// TestGuard_NoViewSwitchOutsideTheTable is guard 2: outside views.go, no
// switch names more than maxViewSwitchCases View constants in its cases. A
// larger one is a second list of the views, and its arms belong in allViews.
func TestGuard_NoViewSwitchOutsideTheTable(t *testing.T) {
	names := viewConstantNames(t)
	for _, path := range productionGoFiles(t) {
		if path == "views.go" {
			continue
		}
		for _, sw := range viewSwitches(t, readSourceFile(t, path), names) {
			if len(sw.views) > maxViewSwitchCases {
				t.Errorf("%s:%s has a switch that names %d View constants, more than %d; "+
					"a per-view arm belongs in allViews (views.go)", path, sw.pos, len(sw.views), maxViewSwitchCases)
			}
		}
	}
}

// viewSwitches returns each switch in src with the distinct View constants
// its case expressions name, in either form: `case ViewA, ViewB:` and
// `case a.currentView == ViewA:`. names is the set of constants.
func viewSwitches(t *testing.T, src string, names map[string]bool) []viewChain {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	var out []viewChain
	ast.Inspect(file, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		seen := map[string]bool{}
		var views []string
		for _, stmt := range sw.Body.List {
			for _, expr := range stmt.(*ast.CaseClause).List {
				ast.Inspect(expr, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && names[id.Name] && !seen[id.Name] {
						seen[id.Name] = true
						views = append(views, id.Name)
					}
					return true
				})
			}
		}
		if len(views) > 0 {
			out = append(out, viewChain{pos: strconv.Itoa(fset.Position(sw.Pos()).Line), views: views})
		}
		return true
	})
	return out
}

// TestGuard_ViewsSelfTest_Switches proves guard 2's finder sees both forms of
// case, counts a constant once, and ignores what is not a View constant.
func TestGuard_ViewsSelfTest_Switches(t *testing.T) {
	names := map[string]bool{"ViewA": true, "ViewB": true, "ViewC": true, "ViewD": true, "ViewE": true}
	got := viewSwitches(t, `package tui

func (a *App) f(v View, n int) {
	switch v {
	case ViewA, ViewB:
	case ViewC:
	case ViewA:
	default:
	}
	switch {
	case a.currentView == ViewD || a.currentView == ViewE:
	case n > 1:
	}
	switch n {
	case 1, 2:
	}
}
`, names)
	var lines []string
	for _, sw := range got {
		lines = append(lines, sw.pos+":"+strings.Join(sw.views, ","))
	}
	want := []string{"4:ViewA,ViewB,ViewC", "10:ViewD,ViewE"}
	if !slices.Equal(lines, want) {
		t.Errorf("switches = %v, want %v", lines, want)
	}
}

// TestGuard_NoFullScreenListOutsideTheTable is guard 3: outside views.go, no
// chain of || and && compares currentView against three or more View
// constants. Such a chain is a view list, like the full-screen predicate that
// renderContent and handleMouseContent each spelled out before the table's
// fullScreen flag replaced both.
func TestGuard_NoFullScreenListOutsideTheTable(t *testing.T) {
	names := viewConstantNames(t)
	for _, path := range productionGoFiles(t) {
		if path == "views.go" {
			continue
		}
		for _, c := range currentViewChains(t, readSourceFile(t, path), names) {
			t.Errorf("%s:%s compares currentView against %d View constants; "+
				"a per-view fact belongs in allViews (views.go)", path, c.pos, len(c.views))
		}
	}
}

// viewConstantNames returns the names of the View constants as a set.
func viewConstantNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, vc := range viewConstants(t) {
		names[vc.Name] = true
	}
	return names
}

// viewChain is one ||/&& chain that compares currentView against View
// constants: where it starts, and the distinct constants it names.
type viewChain struct {
	pos   string
	views []string
}

// currentViewChains returns each maximal ||/&& chain in src that compares
// currentView (a field or a variable of that name) with == or != against
// three or more distinct View constants. names is the set of constants.
func currentViewChains(t *testing.T, src string, names map[string]bool) []viewChain {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	inChain := map[ast.Node]bool{}
	var out []viewChain
	ast.Inspect(file, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || inChain[be] || (be.Op != token.LOR && be.Op != token.LAND) {
			return true
		}
		seen := map[string]bool{}
		var views []string
		for _, leaf := range chainLeaves(be, inChain) {
			cmp, ok := leaf.(*ast.BinaryExpr)
			if !ok || (cmp.Op != token.EQL && cmp.Op != token.NEQ) {
				continue
			}
			name := comparedViewConstant(cmp, names)
			if name != "" && !seen[name] {
				seen[name] = true
				views = append(views, name)
			}
		}
		if len(views) >= 3 {
			line := fset.Position(be.Pos()).Line
			out = append(out, viewChain{pos: strconv.Itoa(line), views: views})
		}
		return true
	})
	return out
}

// chainLeaves flattens a chain of || and && (through parentheses) into its
// operands, and marks each inner link so the walk does not count it again.
func chainLeaves(e ast.Expr, inChain map[ast.Node]bool) []ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	be, ok := e.(*ast.BinaryExpr)
	if !ok || (be.Op != token.LOR && be.Op != token.LAND) {
		return []ast.Expr{e}
	}
	inChain[be] = true
	return append(chainLeaves(be.X, inChain), chainLeaves(be.Y, inChain)...)
}

// comparedViewConstant returns the View constant that cmp compares
// currentView against, or "" when cmp is not such a comparison.
func comparedViewConstant(cmp *ast.BinaryExpr, names map[string]bool) string {
	for _, pair := range [][2]ast.Expr{{cmp.X, cmp.Y}, {cmp.Y, cmp.X}} {
		if !isCurrentView(pair[0]) {
			continue
		}
		if id, ok := pair[1].(*ast.Ident); ok && names[id.Name] {
			return id.Name
		}
	}
	return ""
}

// isCurrentView reports whether e is x.currentView or a bare currentView.
func isCurrentView(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name == "currentView"
	case *ast.Ident:
		return v.Name == "currentView"
	}
	return false
}

// TestGuard_ViewsSelfTest_Chains proves guard 3 fires, by running its finder
// over fabricated source.
func TestGuard_ViewsSelfTest_Chains(t *testing.T) {
	names := map[string]bool{"ViewA": true, "ViewB": true, "ViewC": true}
	got := currentViewChains(t, `package tui

func (a *App) f(other View) bool {
	if a.currentView == ViewA || a.currentView == ViewB {
		return true
	}
	if a.currentView == ViewA || a.currentView == ViewA || a.currentView == ViewB {
		return true
	}
	if other == ViewA || other == ViewB || other == ViewC {
		return true
	}
	full := a.currentView == ViewA ||
		(ViewB == a.currentView && a.x) ||
		a.currentView != ViewC
	return full && (currentView == ViewA || currentView == ViewB || currentView == ViewC)
}
`, names)
	var lines []string
	for _, c := range got {
		lines = append(lines, c.pos+":"+strings.Join(c.views, ","))
	}
	want := []string{"13:ViewA,ViewB,ViewC", "16:ViewA,ViewB,ViewC"}
	if !slices.Equal(lines, want) {
		t.Errorf("chains = %v, want %v; two constants, a repeated constant, and "+
			"another variable must not count, and parentheses must not split a chain", lines, want)
	}
}

// TestGuard_NoViewStateHoldsAService: a view's state struct holds loaded data
// and widgets, never a service. It is the surface guard's rule
// (TestGuard_NoSurfaceStructHoldsAService) for the other half of App: a
// service pointer captured in view state would survive switchDatabase and be a
// use-after-close.
//
// The structs are found by rule, not by a hand list that a twelfth view would
// leave stale: every struct type declared in this package that App holds and
// whose pointer does not implement Modal (the surface guard takes those). That
// also takes in App's other non-modal structs, such as keyMap and Sidebar; none
// of them may hold a service either. The walk goes down through pointers and
// through this package's struct types, because the view's data struct sits one
// level below App once it moves into the view's state.
func TestGuard_NoViewStateHoldsAService(t *testing.T) {
	serviceTypes := servicePointerTypes()
	if len(serviceTypes) == 0 {
		t.Fatal("app.Services exposes no pointer fields; this guard would pass vacuously")
	}

	structs := nonModalStateStructs(reflect.TypeFor[App]())
	if len(structs) == 0 {
		t.Fatal("no non-modal state struct found on App, so this guard would pass vacuously. " +
			"View state structs are declared in this package and do not implement Modal; " +
			"if that changed, update nonModalStateStructs.")
	}

	for _, st := range structs {
		for _, path := range servicesReachableFrom(st, serviceTypes) {
			t.Errorf("%s is a service — a view state struct must not hold one. "+
				"switchDatabase re-points services and closes the previous database, "+
				"so a captured pointer becomes a use-after-close. Keep the service on "+
				"App and pass it in at call time.", path)
		}
	}
}

// TestGuard_NoViewStateHoldsItsDeps: a view's deps are passed to each call,
// never stored in its state, as TestGuard_NoSurfaceStructHoldsItsDeps requires
// of a surface. The hazard is the same one: the natural way to clear a view is
// to assign its zero value, and a stored deps struct would come back with nil
// funcs, so the next call through it would panic.
func TestGuard_NoViewStateHoldsItsDeps(t *testing.T) {
	structs := nonModalStateStructs(reflect.TypeFor[App]())
	if len(structs) == 0 {
		t.Fatal("no non-modal state struct found on App; this guard would pass vacuously")
	}
	for _, st := range structs {
		for i := range st.NumField() {
			f := st.Field(i)
			if isDependencyBag(f.Type) {
				t.Errorf("%s.%s is a %s: every field of it is a func, so it is a "+
					"dependency bag. Pass deps in as a parameter to the methods that "+
					"need them.", st.Name(), f.Name, f.Type)
			}
		}
	}
}

// nonModalStateStructs returns every struct type declared in root's package
// that a field of root holds, by value or by pointer, and whose pointer does
// not implement Modal.
func nonModalStateStructs(root reflect.Type) []reflect.Type {
	modalT := reflect.TypeFor[Modal]()
	var out []reflect.Type
	for i := range root.NumField() {
		ft := root.Field(i).Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct || ft.PkgPath() != root.PkgPath() {
			continue
		}
		if !reflect.PointerTo(ft).Implements(modalT) {
			out = append(out, ft)
		}
	}
	return out
}

// servicesReachableFrom returns the path, as Type.field.field, of every field
// reachable from st whose type is a service pointer. It goes down through
// pointers, slices, arrays and maps, and into struct types declared in st's
// package; a struct from another package (a widget, say) is not its concern.
// Each struct type is walked once, so a service is reported at the first path
// that reaches it; one report is enough to fail the guard.
func servicesReachableFrom(st reflect.Type, serviceTypes map[reflect.Type]bool) []string {
	var out []string
	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type, path string)
	walk = func(t reflect.Type, path string) {
		if seen[t] {
			return
		}
		seen[t] = true
		for i := range t.NumField() {
			f := t.Field(i)
			p := path + "." + f.Name
			ft := f.Type
		unwrap:
			for {
				if serviceTypes[ft] {
					out = append(out, p+" ("+ft.String()+")")
					break
				}
				switch ft.Kind() {
				case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
					ft = ft.Elem()
				case reflect.Struct:
					if ft.PkgPath() == st.PkgPath() {
						walk(ft, p)
					}
					break unwrap
				default:
					break unwrap
				}
			}
		}
	}
	walk(st, st.Name())
	return out
}

// TestGuard_ViewsSelfTest_ViewStateServices proves the finder and the walk
// fire, over fabricated types rather than over a copy of the rule.
func TestGuard_ViewsSelfTest_ViewStateServices(t *testing.T) {
	t.Run("the finder takes view state and skips surfaces and foreign structs", func(t *testing.T) {
		var names []string
		for _, st := range nonModalStateStructs(reflect.TypeFor[appWithViewState]()) {
			names = append(names, st.Name())
		}
		want := []string{"viewStateWithAService", "viewDataWithAService"}
		if !slices.Equal(names, want) {
			t.Errorf("nonModalStateStructs = %v, want %v", names, want)
		}
	})

	t.Run("a service one level down is found", func(t *testing.T) {
		got := servicesReachableFrom(reflect.TypeFor[viewStateWithAService](), servicePointerTypes())
		want := []string{
			"viewStateWithAService.data.accounts (*account.Service)",
			"viewStateWithAService.byID (*account.Service)",
		}
		if !slices.Equal(got, want) {
			t.Errorf("servicesReachableFrom = %v, want %v", got, want)
		}
	})

	t.Run("a clean view state reports nothing", func(t *testing.T) {
		if got := servicesReachableFrom(reflect.TypeFor[priceViewState](), servicePointerTypes()); len(got) != 0 {
			t.Errorf("servicesReachableFrom(priceViewState) = %v, want none", got)
		}
	})
}

// appWithViewState, viewStateWithAService and viewDataWithAService exist only
// as fixtures for the self-test above. They are never held by App. The fixture
// literal keeps the linter from reporting their fields as unused.
type appWithViewState struct {
	view    viewStateWithAService
	data    *viewDataWithAService
	surface surfaceWithAService
	table   *widget.Table
}

type viewStateWithAService struct {
	data *viewDataWithAService
	byID map[types.ID]*account.Service
}

type viewDataWithAService struct {
	accounts *account.Service
}

var _ = appWithViewState{
	view:    viewStateWithAService{data: &viewDataWithAService{accounts: nil}, byID: nil},
	data:    nil,
	surface: surfaceWithAService{},
	table:   nil,
}
