package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/config"
)

// TR-024: when the View → Show closed positions toggle is off the item
// must use the 2-space prefix so it lines up with theme rows that lack
// the ✓ marker, and carry the MenuActionToggleClosedPositions action.
func TestBuildShowClosedPositionsItem_Off(t *testing.T) {
	item := buildShowClosedPositionsItem(false)

	if item.action != MenuActionToggleClosedPositions {
		t.Errorf("action = %d, want MenuActionToggleClosedPositions", item.action)
	}
	if !strings.Contains(item.label, "Show closed positions") {
		t.Errorf("label = %q, want it to contain 'Show closed positions'", item.label)
	}
	if strings.HasPrefix(item.label, "✓") {
		t.Errorf("label = %q, should NOT start with '✓' when toggle is off", item.label)
	}
	if !strings.HasPrefix(item.label, "  ") {
		t.Errorf("label = %q, want a 2-space prefix when toggle is off", item.label)
	}
}

// TR-024: when the toggle is on the label gains a "✓ " prefix so the
// user can see at a glance that closed positions are currently
// included in valuation output.
func TestBuildShowClosedPositionsItem_On(t *testing.T) {
	item := buildShowClosedPositionsItem(true)

	if item.action != MenuActionToggleClosedPositions {
		t.Errorf("action = %d, want MenuActionToggleClosedPositions", item.action)
	}
	if !strings.HasPrefix(item.label, "✓ ") {
		t.Errorf("label = %q, want a '✓ ' prefix when toggle is on", item.label)
	}
	if !strings.Contains(item.label, "Show closed positions") {
		t.Errorf("label = %q, want it to contain 'Show closed positions'", item.label)
	}
}

// TR-024: buildViewMenuItems is the closure body wired into the View
// menu in app.go. It puts the toggle first (a structural setting) and
// then every theme entry. The toggle's ✓ prefix tracks the showClosed
// arg; the themes' ✓ prefix tracks the activeTheme arg — verifying
// both arguments are routed correctly.
func TestBuildViewMenuItems_OrdersToggleFirstAndThemes(t *testing.T) {
	got := buildViewMenuItems("turbo-vision", true)

	if len(got) < 2 {
		t.Fatalf("expected at least 2 items (toggle + themes), got %d", len(got))
	}

	if got[0].action != MenuActionToggleClosedPositions {
		t.Errorf("items[0].action = %d, want MenuActionToggleClosedPositions", got[0].action)
	}
	if !strings.HasPrefix(got[0].label, "✓ ") {
		t.Errorf("items[0].label = %q, want '✓ ' prefix (toggle is on)", got[0].label)
	}

	// Themes follow the toggle; the active one carries a ✓ marker.
	var foundActiveTheme bool
	for _, item := range got[1:] {
		if item.action != MenuActionLoadTheme {
			t.Errorf("post-toggle item %q has action %d, want MenuActionLoadTheme", item.label, item.action)
		}
		if item.data == "turbo-vision" && strings.HasPrefix(item.label, "✓ ") {
			foundActiveTheme = true
		}
	}
	if !foundActiveTheme {
		t.Error("active theme 'turbo-vision' should appear with '✓ ' prefix")
	}
}

func TestBuildViewMenuItems_ToggleOffNoCheckmark(t *testing.T) {
	got := buildViewMenuItems("", false)

	if len(got) < 1 {
		t.Fatal("expected at least the toggle item")
	}
	if got[0].action != MenuActionToggleClosedPositions {
		t.Fatalf("items[0].action = %d, want MenuActionToggleClosedPositions", got[0].action)
	}
	if strings.HasPrefix(got[0].label, "✓") {
		t.Errorf("items[0].label = %q, should NOT have '✓' prefix when toggle is off", got[0].label)
	}
}

// TR-024: selecting the toggle from the menu must flip the persisted
// state on a.cfg so the next valuation request picks up the new
// IncludeClosed value. The handler should also save (best-effort) so
// the choice survives a restart — Save() is a no-op under `go test`,
// so we only assert the in-memory flip.
func TestApp_HandleMenuAction_ToggleClosedPositions_FlipsState(t *testing.T) {
	cfg := &config.Config{ShowClosedPositions: false}
	app := &App{
		cfg:         cfg,
		menubar:     NewMenuBar(),
		currentView: ViewDashboard,
	}

	app.handleMenuAction(MenuActionToggleClosedPositions, "")

	if !cfg.ShowClosedPositions {
		t.Error("ShowClosedPositions should be true after first toggle")
	}

	app.handleMenuAction(MenuActionToggleClosedPositions, "")
	if cfg.ShowClosedPositions {
		t.Error("ShowClosedPositions should be false after second toggle")
	}
}

// TR-024: ValuationOptions used by dashboard / register / portfolio
// must carry IncludeClosed sourced from cfg.ShowClosedPositions so the
// View toggle actually plumbs through to the valuation service.
func TestApp_ValuationOptions_ReflectsConfigToggle(t *testing.T) {
	t.Run("nil cfg -> IncludeClosed false", func(t *testing.T) {
		app := &App{cfg: nil}
		opts := app.valuationOptions()
		if opts.IncludeClosed {
			t.Errorf("nil cfg should produce IncludeClosed=false, got %+v", opts)
		}
	})

	t.Run("cfg.ShowClosedPositions=false -> IncludeClosed false", func(t *testing.T) {
		app := &App{cfg: &config.Config{ShowClosedPositions: false}}
		opts := app.valuationOptions()
		if opts.IncludeClosed {
			t.Errorf("ShowClosedPositions=false should produce IncludeClosed=false, got %+v", opts)
		}
	})

	t.Run("cfg.ShowClosedPositions=true -> IncludeClosed true", func(t *testing.T) {
		app := &App{cfg: &config.Config{ShowClosedPositions: true}}
		opts := app.valuationOptions()
		if !opts.IncludeClosed {
			t.Errorf("ShowClosedPositions=true should produce IncludeClosed=true, got %+v", opts)
		}
	})
}
