package tui

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// TestGuard_OnlyNewTUIServicesBuildsAServiceRegistry: app.NewServices runs
// write side effects as a constructor — it ensures the paycheck categories,
// heals investment accounts and heals scheduled next-dates. Inside the TUI it
// may run once per open database, in newTUIServices. Four tea.Cmd goroutines
// used to call it again (import preview and execute, link-transfers scan and
// execute), so opening the import preview silently healed data, and each read
// a.db at goroutine time rather than when the command was built. Every command
// uses the services App already holds, captured on the main goroutine.
func TestGuard_OnlyNewTUIServicesBuildsAServiceRegistry(t *testing.T) {
	seen := 0
	for _, path := range productionGoFiles(t) {
		for _, fn := range functionsCallingNewServices(t, readSourceFile(t, path)) {
			seen++
			if path == "app.go" && fn == "newTUIServices" {
				continue
			}
			t.Errorf("%s: %s calls app.NewServices. Use a.services, captured before the "+
				"command closure: a second registry re-runs the constructor's heal side "+
				"effects against the user's data.", path, fn)
		}
	}
	if seen == 0 {
		t.Fatal("no call to app.NewServices found anywhere, not even newTUIServices; the scan is broken")
	}
}

// functionsCallingNewServices returns the enclosing function name of every
// app.NewServices call in src. It matches on the syntax tree, so the name in a
// comment or string is not a hit.
func functionsCallingNewServices(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, d := range parseSource(t, src).Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewServices" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "app" {
				out = append(out, fn.Name.Name)
				return false
			}
			return true
		})
	}
	return out
}

func TestGuard_OnlyNewTUIServicesBuildsAServiceRegistry_SelfTest(t *testing.T) {
	got := functionsCallingNewServices(t, `package tui

// app.NewServices in a comment is not a call.
func clean() { s := "app.NewServices"; _ = s }
func dirty() tea.Cmd { return func() tea.Msg { svc := app.NewServices(a.db); _ = svc; return nil } }
`)
	if !slices.Equal(got, []string{"dirty"}) {
		t.Errorf("functionsCallingNewServices = %v, want [dirty]", got)
	}
}

// TestLinkTransfers_UsesTheServicesAppHolds is the behaviour behind the guard
// for one of the four commands: the scan runs through the App's own registry,
// and an App with no services reports that instead of building one or panicking.
func TestLinkTransfers_UsesTheServicesAppHolds(t *testing.T) {
	a := NewApp(dbtest.New(t), &config.Config{})
	switch msg := a.startLinkTransfers()().(type) {
	case linkTransfersPreviewedMsg:
		if msg.result == nil {
			t.Fatal("scan returned no result")
		}
	default:
		t.Fatalf("scan returned %T, want linkTransfersPreviewedMsg", msg)
	}

	bare := &App{}
	for name, cmd := range map[string]func() tea.Cmd{
		"startLinkTransfers":      bare.startLinkTransfers,
		"runLinkTransfersExecute": bare.runLinkTransfersExecute,
	} {
		if _, ok := cmd()().(errMsg); !ok {
			t.Errorf("%s on an App with no services must return errMsg, not build a registry", name)
		}
	}
}

// TestImport_UsesTheServicesAppHolds covers the other two commands the same
// way: a bare App gets an error, and a real one gets as far as the file.
func TestImport_UsesTheServicesAppHolds(t *testing.T) {
	// The file must exist: the preview opens it before it assembles the
	// services, and a missing file would fail there without proving anything
	// about the services path.
	path := filepath.Join(t.TempDir(), "empty.csv")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bare := &App{}
	for name, cmd := range map[string]tea.Cmd{
		"runImportPreview": bare.runImportPreview(&importDialogState{filePath: path, format: "csv"}),
		"runImportExecute": bare.runImportExecute(&importDialogState{}),
	} {
		msg, ok := cmd().(errMsg)
		if !ok {
			t.Errorf("%s on an App with no services returned %T, want errMsg", name, cmd())
			continue
		}
		if msg.err == nil || msg.err.Error() != "services not available" {
			t.Errorf("%s error = %v, want \"services not available\"", name, msg.err)
		}
	}
	if _, err := newImportService(NewApp(dbtest.New(t), &config.Config{}).services); err != nil {
		t.Errorf("a real App's services must assemble the import pipeline: %v", err)
	}
}
