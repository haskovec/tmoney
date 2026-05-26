package widget

import (
	"strings"
	"testing"
)

// TR-024: when the View → Show closed positions toggle is off the item
// must use the 2-space prefix so it lines up with theme rows that lack
// the ✓ marker, and carry the MenuActionToggleClosedPositions action.
func TestBuildShowClosedPositionsItem_Off(t *testing.T) {
	item := BuildShowClosedPositionsItem(false)

	if item.Action != MenuActionToggleClosedPositions {
		t.Errorf("action = %d, want MenuActionToggleClosedPositions", item.Action)
	}
	if !strings.Contains(item.Label, "Show closed positions") {
		t.Errorf("label = %q, want it to contain 'Show closed positions'", item.Label)
	}
	if strings.HasPrefix(item.Label, "✓") {
		t.Errorf("label = %q, should NOT start with '✓' when toggle is off", item.Label)
	}
	if !strings.HasPrefix(item.Label, "  ") {
		t.Errorf("label = %q, want a 2-space prefix when toggle is off", item.Label)
	}
}

// TR-024: when the toggle is on the label gains a "✓ " prefix so the
// user can see at a glance that closed positions are currently
// included in valuation output.
func TestBuildShowClosedPositionsItem_On(t *testing.T) {
	item := BuildShowClosedPositionsItem(true)

	if item.Action != MenuActionToggleClosedPositions {
		t.Errorf("action = %d, want MenuActionToggleClosedPositions", item.Action)
	}
	if !strings.HasPrefix(item.Label, "✓ ") {
		t.Errorf("label = %q, want a '✓ ' prefix when toggle is on", item.Label)
	}
	if !strings.Contains(item.Label, "Show closed positions") {
		t.Errorf("label = %q, want it to contain 'Show closed positions'", item.Label)
	}
}

// TR-024: buildViewMenuItems is the closure body wired into the View
// menu in app.go. It puts the toggle first (a structural setting) and
// then every theme entry. The toggle's ✓ prefix tracks the showClosed
// arg; the themes' ✓ prefix tracks the activeTheme arg — verifying
// both arguments are routed correctly.
func TestBuildViewMenuItems_OrdersToggleFirstAndThemes(t *testing.T) {
	got := BuildViewMenuItems("turbo-vision", true)

	if len(got) < 2 {
		t.Fatalf("expected at least 2 items (toggle + themes), got %d", len(got))
	}

	if got[0].Action != MenuActionToggleClosedPositions {
		t.Errorf("items[0].action = %d, want MenuActionToggleClosedPositions", got[0].Action)
	}
	if !strings.HasPrefix(got[0].Label, "✓ ") {
		t.Errorf("items[0].label = %q, want '✓ ' prefix (toggle is on)", got[0].Label)
	}

	// Themes follow the toggle; the active one carries a ✓ marker.
	var foundActiveTheme bool
	for _, item := range got[1:] {
		if item.Action != MenuActionLoadTheme {
			t.Errorf("post-toggle item %q has action %d, want MenuActionLoadTheme", item.Label, item.Action)
		}
		if item.Data == "turbo-vision" && strings.HasPrefix(item.Label, "✓ ") {
			foundActiveTheme = true
		}
	}
	if !foundActiveTheme {
		t.Error("active theme 'turbo-vision' should appear with '✓ ' prefix")
	}
}

func TestBuildViewMenuItems_ToggleOffNoCheckmark(t *testing.T) {
	got := BuildViewMenuItems("", false)

	if len(got) < 1 {
		t.Fatal("expected at least the toggle item")
	}
	if got[0].Action != MenuActionToggleClosedPositions {
		t.Fatalf("items[0].action = %d, want MenuActionToggleClosedPositions", got[0].Action)
	}
	if strings.HasPrefix(got[0].Label, "✓") {
		t.Errorf("items[0].label = %q, should NOT have '✓' prefix when toggle is off", got[0].Label)
	}
}

// TR-024: selecting the toggle from the menu must flip the persisted
// state on a.cfg so the next valuation request picks up the new
// IncludeClosed value. The handler should also save (best-effort) so
// the choice survives a restart — Save() is a no-op under `go test`,
// so we only assert the in-memory flip.
