package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// specs/tui.md:687-695: "Clicking a dialog button is exactly equivalent to the
// keyboard action (Enter on Save, Esc on Cancel): both input paths route
// through the same submit/cancel handler."
//
// They did not. The mouse cascade kept its own copy of every surface's action
// switch, and 11 Cancel arms had drifted: they inlined
// `X.SetVisible(false); X = nil` where the keyboard called the surface's close
// helper, leaking every other field that surface owned. The registry now sends
// both paths through one onAction per surface, so the tests below compare the
// two paths' *whole observable result* rather than one field at a time.

// modalStateSnapshot captures every App field a surface could leak: each
// pointer/slice/interface field's nil-ness, by name. Comparing snapshots is
// what makes this a leak test rather than a "the dialog closed" test — a
// hand-listed set of fields per surface would miss exactly the fields the
// inlined arms forgot.
func modalStateSnapshot(a *App) map[string]bool {
	v := reflect.ValueOf(a).Elem()
	t := v.Type()
	out := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
			out[t.Field(i).Name] = f.IsNil()
		}
	}
	return out
}

func diffSnapshots(before, after map[string]bool) []string {
	var out []string
	for k, b := range before {
		if after[k] != b {
			out = append(out, k)
		}
	}
	return out
}

// cancelDialogOf returns the base dialog whose Cancel button closes the surface,
// or nil for surfaces that are not built on one. The schedule preview is a
// composite whose header dialog carries the buttons.
func cancelDialogOf(m Modal) *dialog.Dialog {
	switch s := m.(type) {
	case *dialog.Dialog:
		return s
	case *SchedulePreviewDialog:
		return s.HeaderDialog()
	case interface{ Dialog() *dialog.Dialog }:
		// A per-surface state struct (modalSurface embedder).
		return s.Dialog()
	}
	return nil
}

// assertEscAndClickCancelAgree drives both real routes on a fresh App each —
// Esc through handleKeyPress, a left click through handleMouseEvent — and
// requires them to clear the same set of fields. Calling the shared dispatcher
// twice would compare a function against itself and pass no matter what; the
// point is to exercise the two routes a user has.
func assertEscAndClickCancelAgree(t *testing.T, show func(*App), click func(t *testing.T, a *App)) {
	t.Helper()
	byKey := newModalTestApp()
	show(byKey)
	base := modalStateSnapshot(byKey)
	_, _ = byKey.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	keyDiff := diffSnapshots(base, modalStateSnapshot(byKey))

	byMouse := newModalTestApp()
	show(byMouse)
	click(t, byMouse)
	mouseDiff := diffSnapshots(base, modalStateSnapshot(byMouse))

	if len(keyDiff) == 0 {
		t.Fatalf("Esc cleared nothing, so this fixture cannot detect a leak")
	}
	if len(mouseDiff) == 0 {
		t.Fatalf("the click never reached the Cancel button; Esc cleared %v", keyDiff)
	}
	if !equalSets(keyDiff, mouseDiff) {
		t.Errorf("Esc cleared %v, clicking Cancel cleared %v", keyDiff, mouseDiff)
	}
}

// TestMouseCancel_LeavesTheSameStateAsEsc is the phase 2 fix, stated as a
// property: for every surface built on a base dialog, cancelling by click must
// leave the App in the same shape as cancelling by Esc.
func TestMouseCancel_LeavesTheSameStateAsEsc(t *testing.T) {
	setters := surfaceSetters()
	for _, e := range newModalTestApp().modals() {
		name := e.name
		show := setters[name]
		probe := newModalTestApp()
		show(probe)
		if cancelDialogOf(entryNamed(t, probe, name).modal) == nil {
			// No base dialog to place the click against. The split editor and
			// the paycheck wizard are covered by the label-driven tests below;
			// help and the merger confirmation have their own hit tests.
			continue
		}
		t.Run(name, func(t *testing.T) {
			assertEscAndClickCancelAgree(t, show, func(t *testing.T, a *App) { clickCancelOn(t, a, name) })
		})
	}
}

// clickCancelOn drives a real left click onto the named surface's Cancel
// button, through the same handleMouseEvent entry point a user's click takes.
func clickCancelOn(t *testing.T, a *App, name string) {
	t.Helper()
	d := cancelDialogOf(entryNamed(t, a, name).modal)
	if d == nil {
		t.Fatalf("surface %q has no base dialog to click", name)
	}
	// The paint pass sets each dialog's height bound and DialogBounds reads it
	// back, so render before computing coordinates.
	_ = a.viewContent()

	startCol, startRow, _, _ := d.DialogBounds(a.width, a.height)
	contentWidth := d.Width() - dialog.DialogHorizontalOverhead
	btnY := startRow + 2 + d.ContentHeight() - 1
	btnX := startCol + 3 + contentWidth*3/4

	_, _ = a.handleMouseEvent(tea.MouseClickMsg{X: btnX, Y: btnY, Button: tea.MouseLeft})
}

// clickLabel drives a left click onto the first on-screen occurrence of label,
// for surfaces that are not base dialogs and so have no DialogBounds. It reads
// the rendered screen rather than recomputing the layout, so it cannot share a
// wrong assumption with the hit test it exercises.
func clickLabel(t *testing.T, a *App, label string) {
	t.Helper()
	x, y := screenCellOf(t, a.viewContent(), label)
	_, _ = a.handleMouseEvent(tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
}

// The split editor and the paycheck wizard are the two surfaces with their own
// hit tests AND companion state on App, so they are exactly where a mouse
// Cancel could quietly leak again. Both are driven end to end here.
func TestMouseCancel_SplitEditorAgreesWithEsc(t *testing.T) {
	show := func(a *App) {
		surfaceSetters()["split"](a)
		a.pendingSplitTxn = &pendingSplitTransaction{}
	}
	assertEscAndClickCancelAgree(t, show, func(t *testing.T, a *App) { clickLabel(t, a, "Cancel") })
}

func TestMouseCancel_PaycheckWizardAgreesWithEsc(t *testing.T) {
	show := surfaceSetters()["paycheckWizard"]
	assertEscAndClickCancelAgree(t, show, func(t *testing.T, a *App) { clickLabel(t, a, "Cancel") })
}

// TestMouseCancel_RoutesThroughTheSameDispatcher states the structural fact the
// tests above depend on: there is exactly one action dispatcher per surface,
// and both input paths reach it. Without this, the tests above could pass while
// the mouse walk kept a second copy.
func TestMouseCancel_RoutesThroughTheSameDispatcher(t *testing.T) {
	for _, e := range newModalTestApp().modals() {
		switch {
		case e.onAction != nil:
			// The default walk dispatches through it; nothing else to prove.
		case e.onMouse != nil:
			// Declares its own handling, and says why at its registry entry.
		default:
			t.Errorf("surface %q has no onAction and no onMouse, "+
				"so a click on it silently does nothing", e.name)
		}
	}
}

// TestMouseOverride_CallsTheSharedDispatcher closes the gap the structural test
// above leaves: handleDialogMouse prefers onMouse, so a surface that sets both
// never takes the default HandleMouse-then-onAction walk, and `onAction != nil`
// proves nothing about its mouse path. For those surfaces the override's source
// must call the named dispatcher. The schedule preview's override must also run
// the reseed tail its keyboard path runs after every header key.
func TestMouseOverride_CallsTheSharedDispatcher(t *testing.T) {
	src := appMethodSources(t)
	for _, e := range newModalTestApp().modals() {
		if e.onMouse == nil || e.onAction == nil {
			continue
		}
		override := methodName(e.onMouse)
		dispatcher := methodName(e.onAction)
		body, ok := src[override]
		if !ok {
			t.Errorf("surface %q: could not read the source of %s", e.name, override)
			continue
		}
		if !strings.Contains(body, dispatcher+"(") {
			t.Errorf("surface %q: mouse override %s does not call its dispatcher %s, "+
				"so a click and a keypress can drift apart", e.name, override, dispatcher)
		}
		if e.name == "schedulePreview" && !strings.Contains(body, "maybeReseedLoanPreview(") {
			t.Errorf("surface %q: the mouse path no longer runs maybeReseedLoanPreview, "+
				"so a Date change by click does not reseed a loan preview as typing does", e.name)
		}
	}
}

// The corporate-action detail panel is not a registry surface, but it holds
// isDialogVisible open. handleDialogMouse must still route to its [x] hit test
// and otherwise swallow the click rather than fall through to the table.
func TestMouseGate_CorporateActionDetailSwallowsClicks(t *testing.T) {
	a, _ := corporateActionDetailEnv(t, 120, 40)
	if !a.isDialogVisible() {
		t.Fatal("the detail panel must hold the mouse gate open")
	}
	if got := a.frontmostModal(); got != nil {
		t.Fatalf("the detail panel must not be a registry surface, got %q", got.name)
	}
	_, _ = a.handleDialogMouse(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	if a.corporateActionDetail == nil {
		t.Error("a click must not dismiss the detail panel")
	}
}

// Cancel was not the only asymmetry. Four surfaces ran a recompute AFTER their
// action switch — payee auto-fill, the scheduled dialog's category options, the
// account dialog's field visibility, the stock-split preview message — and the
// mouse cascade had none of them. So changing a select field by clicking left
// the derived state stale where changing it by keyboard did not.
//
// Extracting the whole handler tail into the shared dispatcher fixes all four
// by construction. This test states the structural fact rather than each
// symptom: the dispatcher a surface's key path uses is the one its mouse path
// uses, tails included.
func TestActionDispatcher_IsSharedIncludingTheHandlerTail(t *testing.T) {
	tails := map[string]string{
		"transaction": "checkPayeeAutoFill",
		"scheduled":   "refreshSchedCategoryOptionsForAccount",
		"account":     "updateAccountFieldVisibility",
		"stockSplit":  "refreshStockSplitDialogMessage",
	}
	src := appMethodSources(t)
	for surface, tail := range tails {
		e := entryNamed(t, newModalTestApp(), surface)
		if e.onAction == nil {
			t.Errorf("surface %q has no shared dispatcher, so its %s tail is keyboard-only", surface, tail)
			continue
		}
		if e.onMouse != nil {
			t.Errorf("surface %q overrides the mouse path, so it can skip its %s tail", surface, tail)
		}
		body, ok := src[methodName(e.onAction)]
		if !ok {
			t.Errorf("could not read %q's dispatcher source", surface)
			continue
		}
		if !strings.Contains(body, tail) {
			t.Errorf("%q's dispatcher no longer runs %s; if that tail moved back into "+
				"the key handler, the mouse path silently lost it", surface, tail)
		}
	}
}

// methodName returns the bare method name of an (*App) method expression, e.g.
// "sellDialogAction" for (*App).sellDialogAction.
func methodName(fn any) string {
	full := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	name := full[strings.LastIndex(full, ".")+1:]
	return strings.TrimSuffix(name, "-fm")
}

// appMethodSources maps every `func (a *App) name(` in this package's
// non-test files to its body text. Reading source rather than executing keeps
// these tests about where code lives, which is the property that broke.
func appMethodSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	out := make(map[string]string)
	const head = "func (a *App) "
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		for i := strings.Index(s, head); i >= 0; {
			rest := s[i+len(head):]
			paren := strings.Index(rest, "(")
			end := strings.Index(rest, "\n}\n")
			if paren < 0 || end < 0 {
				break
			}
			out[rest[:paren]] = rest[:end]
			next := strings.Index(rest[end:], head)
			if next < 0 {
				break
			}
			i = i + len(head) + end + next
		}
	}
	return out
}

func entryNamed(t *testing.T, a *App, name string) modalEntry {
	t.Helper()
	for _, e := range a.modals() {
		if e.name == name {
			return e
		}
	}
	t.Fatalf("no registry surface named %q", name)
	return modalEntry{}
}

func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
