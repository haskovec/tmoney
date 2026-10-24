package tui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// A command runs in its own goroutine, and Update runs on another. A File >
// Open can switch the file between the two, and switchDatabase writes the
// App's fields while the command runs. So a command reads nothing from the App
// when it runs: it copies what it needs (a.services, a.db, a.undoManager, the
// state of a view) when it is made, in Update. Then a load reads only the file
// it was made for, a write goes to that file, and no command reads a field
// that Update writes. An object that a command shares with Update (a dialog
// state, the undo manager of the file) is not a field read, and this guard
// does not see it.
// The deps builders (a.dashboardDeps() and the others) bind the deps the same
// way, when Update makes them.
//
// TestGuard_CommandsTouchNoAppWhileTheyRun pins that rule.

// TestGuard_CommandsTouchNoAppWhileTheyRun: no func literal that runs as a
// command, and no dep that a deps builder binds, names the App.
func TestGuard_CommandsTouchNoAppWhileTheyRun(t *testing.T) {
	for _, path := range productionGoFiles(t) {
		for _, use := range appUsesInCommands(t, readSourceFile(t, path)) {
			t.Errorf("%s:%s. A command must not read the App when it runs: copy what it "+
				"needs when Update makes it (svc := a.services, file := a.fileOfOpen(), ...).", path, use)
		}
	}
}

// appUsesInCommands returns each use of the App inside a command, as
// "line: a.name in fn". A command is a func literal of type func(...)
// tea.Msg, or any func literal inside a function that returns a ...Deps
// struct (a deps builder). The App is the *App receiver or an *App parameter
// of the enclosing function.
func appUsesInCommands(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	var uses []string
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		apps := appVarsOf(fd)
		if len(apps) == 0 {
			continue
		}
		builder := returnsDeps(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			fl, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			if !builder && !returnsMsg(fl) {
				return true // a callback that runs in Update; look inside it
			}
			ast.Inspect(fl.Body, func(m ast.Node) bool {
				use := ""
				switch x := m.(type) {
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && apps[id.Name] {
						use = id.Name + "." + x.Sel.Name
					}
				case *ast.Ident:
					if apps[x.Name] {
						use = x.Name
					}
				}
				if use == "" {
					return true
				}
				uses = append(uses, fmt.Sprintf("%d: %s in %s", fset.Position(m.Pos()).Line, use, fd.Name.Name))
				return false
			})
			return false
		})
	}
	return uses
}

// appVarsOf returns the names of the *App receiver and *App parameters of fd.
func appVarsOf(fd *ast.FuncDecl) map[string]bool {
	apps := map[string]bool{}
	var fields []*ast.Field
	if fd.Recv != nil {
		fields = append(fields, fd.Recv.List...)
	}
	fields = append(fields, fd.Type.Params.List...)
	for _, f := range fields {
		star, ok := f.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); ok && id.Name == "App" {
			for _, n := range f.Names {
				apps[n.Name] = true
			}
		}
	}
	return apps
}

// returnsDeps reports whether fd returns one value of a type named ...Deps.
func returnsDeps(fd *ast.FuncDecl) bool {
	res := fd.Type.Results
	if res == nil || len(res.List) != 1 {
		return false
	}
	id, ok := res.List[0].Type.(*ast.Ident)
	return ok && strings.HasSuffix(id.Name, "Deps")
}

// returnsMsg reports whether fl returns one tea.Msg: a command, or the
// callback of tea.Tick.
func returnsMsg(fl *ast.FuncLit) bool {
	res := fl.Type.Results
	if res == nil || len(res.List) != 1 {
		return false
	}
	se, ok := res.List[0].Type.(*ast.SelectorExpr)
	return ok && se.Sel.Name == "Msg"
}

// TestGuard_CommandGuardSelfTest proves that the finder sees each kind of use,
// and ignores a copy made in Update and a callback that runs in Update.
func TestGuard_CommandGuardSelfTest(t *testing.T) {
	got := appUsesInCommands(t, `package tui

func (a *App) live() tea.Cmd {
	return func() tea.Msg { return a.services.Account.List() }
}
func (a *App) copied() tea.Cmd {
	svc := a.services
	return func() tea.Msg { return svc.Account.List() }
}
func (a *App) passed() tea.Cmd {
	return func() tea.Msg { return helper(a) }
}
func (a *App) ticked() tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return a.reload() })
}
func (a *App) inUpdate() {
	sort.Slice(xs, func(i, j int) bool { return a.less(i, j) })
}
func (a *App) nested() tea.Cmd {
	return func() tea.Msg {
		return run(func() error { return a.undoManager.Execute(c) })
	}
}
func helper(a *App) tea.Cmd {
	return func() tea.Msg { return a.db }
}
func (a *App) fooDeps() fooDeps {
	svc := a.services
	return fooDeps{
		ok:   func() *account.Service { return svc.Account },
		live: func() *account.Service { return a.services.Account },
	}
}
func (s *fooSurface) load() tea.Cmd {
	return func() tea.Msg { return s.x }
}
`)
	want := []string{
		"4: a.services in live",
		"11: a in passed",
		"14: a.reload in ticked",
		"21: a.undoManager in nested",
		"25: a.db in helper",
		"31: a.services in fooDeps",
	}
	if !slices.Equal(got, want) {
		t.Errorf("appUsesInCommands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
