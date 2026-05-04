package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/tui/theme"
)

// TestBuildThemeMenuItems_PopulatesFromBuiltins covers TH-022: opening
// View → Theme should show one menu entry per built-in theme, in the
// order BuiltinIDs() returns them. Each item carries the theme ID as
// its data payload and a MenuActionLoadTheme action so TH-023's
// dispatch can reach reloadTheme without re-deriving the ID from the
// label.
func TestBuildThemeMenuItems_PopulatesFromBuiltins(t *testing.T) {
	got := buildThemeMenuItems("")

	ids := theme.BuiltinIDs()
	if len(got) != len(ids) {
		t.Fatalf("len(items) = %d, want %d (BuiltinIDs)", len(got), len(ids))
	}

	for i, id := range ids {
		if got[i].action != MenuActionLoadTheme {
			t.Errorf("items[%d].action = %d, want MenuActionLoadTheme", i, got[i].action)
		}
		if got[i].data != id {
			t.Errorf("items[%d].data = %q, want %q", i, got[i].data, id)
		}
		if !strings.Contains(got[i].label, id) {
			t.Errorf("items[%d].label = %q, want it to contain %q", i, got[i].label, id)
		}
	}
}

// TestBuildThemeMenuItems_ActiveHasCheckmark — the active theme's label
// is prefixed with "✓ " so the dropdown shows which one is currently
// applied. Other items use a 2-column non-checkmark prefix so the IDs
// stay column-aligned in the dropdown.
func TestBuildThemeMenuItems_ActiveHasCheckmark(t *testing.T) {
	got := buildThemeMenuItems("turbo-vision")

	var active *menuItem
	for i := range got {
		if got[i].data == "turbo-vision" {
			active = &got[i]
			break
		}
	}
	if active == nil {
		t.Fatal("no item with data=turbo-vision")
	}
	if !strings.HasPrefix(active.label, "✓ ") {
		t.Errorf("active label = %q, want a %q prefix", active.label, "✓ ")
	}

	// All other items must NOT have the checkmark prefix.
	for _, item := range got {
		if item.data == "turbo-vision" {
			continue
		}
		if strings.HasPrefix(item.label, "✓") {
			t.Errorf("inactive item %q should not start with ✓", item.label)
		}
	}
}

// TestBuildThemeMenuItems_NoActive — when the active ID is empty (no
// theme persisted yet) no item should carry the checkmark.
func TestBuildThemeMenuItems_NoActive(t *testing.T) {
	got := buildThemeMenuItems("")
	for _, item := range got {
		if strings.HasPrefix(item.label, "✓") {
			t.Errorf("item %q should not start with ✓ when activeID is empty", item.label)
		}
	}
}

// TestBuildThemeMenuItems_UnknownActive — an active ID that doesn't
// match any built-in (e.g., a removed user theme persisted in config)
// must not crash and must not mark any item as active.
func TestBuildThemeMenuItems_UnknownActive(t *testing.T) {
	got := buildThemeMenuItems("nonexistent")
	for _, item := range got {
		if strings.HasPrefix(item.label, "✓") {
			t.Errorf("item %q should not start with ✓ when activeID is unknown", item.label)
		}
	}
}
