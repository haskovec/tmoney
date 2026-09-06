package tui

import (
	"os"
	"path/filepath"
	"reflect"
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

// TestMouseCancel_LeavesTheSameStateAsEsc is the phase 2 fix, stated as a
// property: for every surface, cancelling by click must leave the App in the
// same shape as cancelling by Esc.
//
// Both paths are driven end to end — Esc through handleKeyPress, the click
// through handleMouseEvent onto the real Cancel button's coordinates. Calling
// the shared dispatcher twice would compare a function against itself and pass
// no matter what; the point is to exercise the two routes a user has.
func TestMouseCancel_LeavesTheSameStateAsEsc(t *testing.T) {
	setters := surfaceSetters()
	for _, e := range newModalTestApp().modals() {
		name := e.name
		show := setters[name]
		if _, ok := e.modal.(*dialog.Dialog); !ok {
			// The click needs DialogBounds to place it. The bespoke surfaces
			// (help, merger confirmation, split, paycheck, preview) are covered
			// by TestMouseCancel_RoutesThroughTheSameDispatcher instead.
			continue
		}
		t.Run(name, func(t *testing.T) {
			byKey := newModalTestApp()
			show(byKey)
			base := modalStateSnapshot(byKey)
			_, _ = byKey.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
			keyDiff := diffSnapshots(base, modalStateSnapshot(byKey))

			byMouse := newModalTestApp()
			show(byMouse)
			clickCancelOn(t, byMouse, name)
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
		})
	}
}

// clickCancelOn drives a real left click onto the named surface's Cancel
// button, through the same handleMouseEvent entry point a user's click takes.
func clickCancelOn(t *testing.T, a *App, name string) {
	t.Helper()
	d, ok := entryNamed(t, a, name).modal.(*dialog.Dialog)
	if !ok {
		t.Fatalf("surface %q is not a base dialog", name)
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

// TestMouseCancel_RoutesThroughTheSameDispatcher states the structural fact the
// test above depends on: there is exactly one action dispatcher per surface,
// and both input paths reach it. Without this, the test above could pass while
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
	src := dispatcherSources(t)
	for surface, tail := range tails {
		e := entryNamed(t, newModalTestApp(), surface)
		if e.onAction == nil {
			t.Errorf("surface %q has no shared dispatcher, so its %s tail is keyboard-only", surface, tail)
			continue
		}
		if e.onMouse != nil {
			t.Errorf("surface %q overrides the mouse path, so it can skip its %s tail", surface, tail)
		}
		body, ok := src[surface]
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

// dispatcherSources maps a surface name to the source text of its onAction
// dispatcher, read from disk. Parsing rather than executing keeps the test
// about where the tail lives, which is the property that broke.
func dispatcherSources(t *testing.T) map[string]string {
	t.Helper()
	want := map[string]string{
		"transaction": "transactionDialogAction",
		"scheduled":   "scheduledDialogAction",
		"account":     "accountDialogAction",
		"stockSplit":  "stockSplitDialogAction",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	out := make(map[string]string)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		for surface, fn := range want {
			head := "func (a *App) " + fn + "(action dialog.DialogAction)"
			i := strings.Index(s, head)
			if i < 0 {
				continue
			}
			j := strings.Index(s[i:], "\n}\n")
			if j < 0 {
				continue
			}
			out[surface] = s[i : i+j]
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
