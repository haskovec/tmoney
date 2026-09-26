package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// restoreDefaultTheme reapplies the embedded default theme. Use as a
// t.Cleanup so a test that calls ApplyTheme doesn't leak palette state
// into the widget package-level Color* vars and confuse later tests.
func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("restoreDefaultTheme: load default: %v", err)
	}
	s := widget.NewStyles()
	s.ApplyTheme(def)
}

// runCmd runs cmd and feeds each message it yields back into the app, a few
// levels deep, the way the Bubble Tea loop would. An errMsg fails the test.
func runCmd(t *testing.T, a *App, cmd tea.Cmd, depth int) {
	t.Helper()
	if cmd == nil || depth == 0 {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			runCmd(t, a, c, depth)
		}
	case errMsg:
		t.Fatalf("command failed: %v", msg.err)
	default:
		_, next := a.Update(msg)
		runCmd(t, a, next, depth-1)
	}
}
