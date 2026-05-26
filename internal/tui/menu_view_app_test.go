package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

func TestApp_HandleMenuAction_ToggleClosedPositions_FlipsState(t *testing.T) {
	cfg := &config.Config{ShowClosedPositions: false}
	app := &App{
		cfg:         cfg,
		menubar:     widget.NewMenuBar(),
		currentView: ViewDashboard,
	}

	app.handleMenuAction(widget.MenuActionToggleClosedPositions, "")

	if !cfg.ShowClosedPositions {
		t.Error("ShowClosedPositions should be true after first toggle")
	}

	app.handleMenuAction(widget.MenuActionToggleClosedPositions, "")
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
