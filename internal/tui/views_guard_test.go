package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
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
