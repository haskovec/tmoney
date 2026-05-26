package tui

import (
	"testing"

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
