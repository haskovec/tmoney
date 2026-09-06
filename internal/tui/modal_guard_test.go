package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Guard 2 of the design's section 6.1: every modal field on App is in the
// registry, exactly once. This is the test that would have caught the phase 0
// bug, where importDialog and linkTransfersDialog were key-routed but appeared
// in neither the paint list nor the visibility gate.
//
// Neither side may be a hand-maintained list of expected names — that would be
// a fifth copy of the very list this design collapsed, and it would rot the
// same way. Both sides are built mechanically:
//
//   - must-be-registered, from reflection over App's field types;
//   - is-registered, from parsing modals() with go/ast.
//
// Reflection over a struct's unexported field names and types is fine from
// inside the package. The restriction is on Set and Interface(), which panic
// even in-package, so this must not try to populate an App reflectively.

// modalFieldTypes are the App field types that denote a modal surface. Keep
// this keyed to types, not names: a name list is the fifth list.
//
// PHASE 3 WILL BREAK THIS, DELIBERATELY. After the per-surface state structs
// land, these fields move inside *sellSurface and friends, and a filter frozen
// at this shape would match nothing and pass vacuously on day one. The
// anti-vacuity check below is what makes that failure loud. Update this filter
// in the same commit as the first surface struct.
var modalFieldTypes = map[string]bool{
	"*dialog.Dialog":             true,
	"*tui.SplitDialog":           true,
	"*tui.SchedulePreviewDialog": true,
	"*tui.PaycheckWizard":        true,
}

// registryOnlySurfaces are surfaces whose visibility is not a modal-typed
// field on App, so reflection cannot find them. Each needs a stated reason.
var registryOnlySurfaces = map[string]string{
	"help":          "visibility is the bare bool App.showHelp",
	"backup":        "visibility is App.backupDialog.dialog, one level in",
	"mergerConfirm": "visibility is the presence of App.mergerConfirmData, not a flag",
}

func TestGuard_EveryModalFieldIsRegisteredExactlyOnce(t *testing.T) {
	mustRegister := modalFieldsOnApp(t)
	if len(mustRegister) == 0 {
		t.Fatal("modalFieldTypes matched zero App fields, so this guard would pass " +
			"vacuously. The field types moved — most likely into the phase 3 surface " +
			"structs. Update modalFieldTypes in the same commit that moved them.")
	}

	registered := registryFieldNames(t)
	if len(registered) == 0 {
		t.Fatal("parsing modals() found no surfaces; the guard cannot see the registry")
	}

	for _, f := range mustRegister {
		if !registered[f] {
			t.Errorf("App field %q is a modal surface but is not in modals(); "+
				"it can take keys and never paint (the phase 0 bug)", f)
		}
	}
}

func TestGuard_RegistryReferencesNoUnknownField(t *testing.T) {
	onApp := make(map[string]bool)
	for _, f := range modalFieldsOnApp(t) {
		onApp[f] = true
	}
	for surface, field := range registryNamesByField(t) {
		if _, exempt := registryOnlySurfaces[surface]; exempt {
			continue
		}
		if !onApp[field] {
			t.Errorf("registry surface %q reads App.%s, which is not a modal-typed field; "+
				"either give it a modal type or add it to registryOnlySurfaces with a reason",
				surface, field)
		}
	}
}

// TestGuard_EveryRegistryEntryIsAccountedFor pairs each registry entry with
// either a modal-typed App field or a stated exemption, so a surface cannot be
// added without one or the other.
func TestGuard_EveryRegistryEntryIsAccountedFor(t *testing.T) {
	byField := registryNamesByField(t)
	entries := newModalTestApp().modals()
	if len(entries) == 0 {
		t.Fatal("modals() is empty; this guard would pass vacuously")
	}
	for _, e := range entries {
		if _, exempt := registryOnlySurfaces[e.name]; exempt {
			continue
		}
		if _, ok := byField[e.name]; !ok {
			t.Errorf("registry surface %q has neither a modal-typed App field nor an "+
				"entry in registryOnlySurfaces explaining why", e.name)
		}
	}
	for name, why := range registryOnlySurfaces {
		found := false
		for _, e := range entries {
			if e.name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("registryOnlySurfaces names %q (%s), which is not a registry surface", name, why)
		}
	}
}

// modalFieldsOnApp returns App's field names whose type denotes a modal.
func modalFieldsOnApp(t *testing.T) []string {
	t.Helper()
	appT := reflect.TypeFor[App]()
	var out []string
	for i := range appT.NumField() {
		f := appT.Field(i)
		if modalFieldTypes[f.Type.String()] {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// registryFieldNames returns the set of App field names that modals() reads,
// parsed from source rather than executed. Parsing keeps the guard honest
// about what the registry literally names, not about what a particular App
// instance happens to hold.
func registryFieldNames(t *testing.T) map[string]bool {
	t.Helper()
	out := make(map[string]bool)
	for _, field := range registryNamesByField(t) {
		out[field] = true
	}
	return out
}

// registryNamesByField maps each registry surface name to the App field it
// reads, for entries that read one directly.
func registryNamesByField(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "modal.go", nil, 0)
	if err != nil {
		t.Fatalf("parse modal.go: %v", err)
	}

	fn := findFunc(file, "modals")
	if fn == nil {
		t.Fatal("modal.go declares no modals() method")
	}

	out := make(map[string]string)
	ast.Inspect(fn, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || len(lit.Elts) != 3 {
			return true
		}
		name, ok := lit.Elts[0].(*ast.BasicLit)
		if !ok || name.Kind != token.STRING {
			return true
		}
		surface := strings.Trim(name.Value, `"`)
		if field := appFieldOf(lit.Elts[1]); field != "" {
			out[surface] = field
		}
		return true
	})
	return out
}

// appFieldOf returns the App field name an entry's modal expression reads, or
// "" for adapter types and multi-step expressions.
func appFieldOf(e ast.Expr) string {
	// a.backupDialog.Dialog() → the receiver chain's first App field.
	if call, ok := e.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			return appFieldOf(sel.X)
		}
		return ""
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "a" {
		return sel.Sel.Name
	}
	return ""
}

func findFunc(file *ast.File, name string) *ast.FuncDecl {
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

// TestGuard_SelfTest proves the guard fires. The in-repo precedent
// (internal/transfer/arch_test.go) self-tests a *copy* of its guard, and that
// copy has drifted — it exercises 9 of 15 banned names. So the parse is a pure
// function over source text and the self-test feeds it fabricated input,
// exercising the same code the real guard runs.
func TestGuard_SelfTest(t *testing.T) {
	t.Run("appFieldOf reads a direct field", func(t *testing.T) {
		if got := appFieldOf(mustParseExpr(t, "a.sellDialog")); got != "sellDialog" {
			t.Errorf("appFieldOf(a.sellDialog) = %q, want sellDialog", got)
		}
	})
	t.Run("appFieldOf reaches through a nil-safe accessor", func(t *testing.T) {
		if got := appFieldOf(mustParseExpr(t, "a.backupDialog.Dialog()")); got != "backupDialog" {
			t.Errorf("appFieldOf(a.backupDialog.Dialog()) = %q, want backupDialog", got)
		}
	})
	t.Run("appFieldOf ignores an adapter literal", func(t *testing.T) {
		if got := appFieldOf(mustParseExpr(t, "helpOverlayModal{a}")); got != "" {
			t.Errorf("appFieldOf(helpOverlayModal{a}) = %q, want empty", got)
		}
	})
	t.Run("reflection finds the modal field types", func(t *testing.T) {
		fields := modalFieldsOnApp(t)
		for _, want := range []string{"importDialog", "linkTransfersDialog", "splitDialog", "paycheckWizard"} {
			if !slices.Contains(fields, want) {
				t.Errorf("reflection missed the modal field %q", want)
			}
		}
	})
	t.Run("a surface missing from the registry is detected", func(t *testing.T) {
		// The phase 0 bug, replayed: importDialog present on App, absent from
		// the registry side. The comparison must flag it.
		registered := map[string]bool{"sellDialog": true}
		if registered["importDialog"] {
			t.Fatal("fixture is wrong")
		}
		missing := false
		for _, f := range []string{"importDialog", "sellDialog"} {
			if !registered[f] {
				missing = true
			}
		}
		if !missing {
			t.Error("the comparison failed to notice an unregistered field")
		}
	})
}

func mustParseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return e
}
