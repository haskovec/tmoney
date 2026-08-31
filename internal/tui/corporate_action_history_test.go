package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func newTestCorporateActionViewData(t *testing.T) (*App, *investment.CorporateAction, *investment.CorporateAction) {
	t.Helper()

	aaplID := types.NewID()
	msftID := types.NewID()
	googID := types.NewID()
	secMap := map[types.ID]*security.Security{
		aaplID: {Ticker: "AAPL", Name: "Apple Inc."},
		msftID: {Ticker: "MSFT", Name: "Microsoft Corp."},
		googID: {Ticker: "GOOG", Name: "Alphabet Inc."},
	}

	splitParams := investment.SplitParams{Numerator: 2, Denominator: 1}
	splitJSON, _ := splitParams.ToJSON()
	splitAction := investment.NewCorporateAction(
		investment.ActionTypeSplit,
		aaplID,
		types.NewDate(2024, time.June, 1),
		splitJSON,
	)

	mergerParams := investment.MergerParams{ExchangeRatio: 0.5}
	mergerJSON, _ := mergerParams.ToJSON()
	mergerAction := investment.NewCorporateAction(
		investment.ActionTypeMerger,
		msftID,
		types.NewDate(2024, time.April, 15),
		mergerJSON,
	)
	mergerAction.SetTargetSecurity(googID)

	app := &App{
		corporateActionView: &corporateActionViewData{
			actions: []*investment.CorporateAction{splitAction, mergerAction},
			secMap:  secMap,
		},
	}
	return app, splitAction, mergerAction
}

func TestFilteredCorporateActions_NoFilterReturnsAll(t *testing.T) {
	app, _, _ := newTestCorporateActionViewData(t)
	got := app.filteredCorporateActions()
	if len(got) != 2 {
		t.Errorf("filtered count = %d, want 2", len(got))
	}
}

func TestFilteredCorporateActions_TickerMatch(t *testing.T) {
	app, split, _ := newTestCorporateActionViewData(t)
	app.corporateActionViewFilter = "aapl"
	got := app.filteredCorporateActions()
	if len(got) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(got))
	}
	if got[0].ID != split.ID {
		t.Errorf("filtered[0] = %s, want split action", got[0].ID.String())
	}
}

func TestFilteredCorporateActions_TargetTickerMatch(t *testing.T) {
	// "GOOG" only appears as the merger's target — confirming we search
	// against the resolved target ticker too.
	app, _, merger := newTestCorporateActionViewData(t)
	app.corporateActionViewFilter = "GOOG"
	got := app.filteredCorporateActions()
	if len(got) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(got))
	}
	if got[0].ID != merger.ID {
		t.Errorf("filtered[0] = %s, want merger action", got[0].ID.String())
	}
}

func TestFilteredCorporateActions_TypeMatch(t *testing.T) {
	app, _, merger := newTestCorporateActionViewData(t)
	app.corporateActionViewFilter = "merger"
	got := app.filteredCorporateActions()
	if len(got) != 1 || got[0].ID != merger.ID {
		t.Errorf("expected merger action only, got %v", got)
	}
}

func TestFormatGlobalCorporateActionRow_IncludesTicker(t *testing.T) {
	app, split, _ := newTestCorporateActionViewData(t)
	row := formatGlobalCorporateActionRow(split, app.corporateActionView.secMap)
	if len(row) != 4 {
		t.Fatalf("row length = %d, want 4", len(row))
	}
	if row[0] != "2024-06-01" {
		t.Errorf("date = %q, want 2024-06-01", row[0])
	}
	if row[1] != "AAPL" {
		t.Errorf("ticker = %q, want AAPL", row[1])
	}
	if !strings.Contains(row[2], "Stock Split") {
		t.Errorf("type = %q, want it to contain 'Stock Split'", row[2])
	}
	if !strings.Contains(row[3], "Ratio 2:1") {
		t.Errorf("details = %q, want 'Ratio 2:1'", row[3])
	}
}

// ===========================================================================
// Details overlay: rendering and mouse support
// ===========================================================================

// screenCellOf returns the visible column and row of needle's first cell in a
// rendered screen. strings.Index gives a BYTE offset and rendered rows carry
// multi-byte box-drawing runes, so the column has to be measured with
// ansi.StringWidth over the prefix — a byte offset reads two columns right of
// the truth, which is survivable inside a wide button and fatal for a
// three-column [x] flush against the right edge.
func screenCellOf(t *testing.T, screen, needle string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(screen, "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, needle); i >= 0 {
			return ansi.StringWidth(plain[:i]), row
		}
	}
	t.Fatalf("%q not found on screen", needle)
	return 0, 0
}

// corporateActionDetailEnv builds an app on the corporate-action register with
// the details overlay open, sized to a real screen.
func corporateActionDetailEnv(t *testing.T, w, h int) (*App, *investment.CorporateAction) {
	t.Helper()
	app, split, _ := newTestCorporateActionViewData(t)
	styles := widget.NewStyles()
	styles.Resize(w, h)
	app.ready = true
	app.width, app.height = w, h
	app.currentView = ViewCorporateActions
	app.keys = defaultKeyMap()
	app.menubar = widget.NewMenuBar()
	app.statusbar = widget.NewStatusBar()
	app.sidebar = NewSidebar()
	app.styles = styles
	app.buildCorporateActionViewTable()
	app.corporateActionDetail = split
	return app, split
}

// TestCorporateActionDetail_OverlaySitsWhereOverlayTopLeftSaysItDoes is the
// offset guard, and the reason the overlay moved into the app-level cascade.
// Rendered inside the view body it sat one row below where the standard
// transform predicts, because of the 1-row header — so a hit test built on that
// transform missed every target.
func TestCorporateActionDetail_OverlaySitsWhereOverlayTopLeftSaysItDoes(t *testing.T) {
	for _, tc := range []struct{ w, h int }{
		{120, 40}, {100, 24}, {80, 20}, {200, 60}, {120, 14}, {38, 40},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.w, tc.h), func(t *testing.T) {
			app, _ := corporateActionDetailEnv(t, tc.w, tc.h)
			overlay := app.renderCorporateActionDetails()
			startCol, startRow := widget.OverlayTopLeft(overlay, app.width, app.height)
			x, y := screenCellOf(t, app.renderLayout(), "Action Details")
			if x != startCol+3 || y != startRow+2 {
				t.Errorf("title at (%d,%d), want (%d,%d) — offset dX=%d dY=%d",
					x, y, startCol+3, startRow+2, x-(startCol+3), y-(startRow+2))
			}
		})
	}
}

// TestCorporateActionDetails_ContentFitsTheBox is the separator-wrap
// regression: the inner width was computed as overlayWidth-4 where the content
// band is overlayWidth-6, so the separator spilled onto a stub row and made the
// panel one line taller than it looks.
func TestCorporateActionDetails_ContentFitsTheBox(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	lines := strings.Split(app.renderCorporateActionDetails(), "\n")
	if len(lines) != 12 {
		t.Errorf("panel is %d lines, want 12 (a spill row means the separator wrapped)", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 70 {
			t.Errorf("line %d is %d columns wide, want 70", i, w)
		}
	}
}

// TestCorporateActionDetails_ShortTerminalKeepsHint covers the clipping the move
// fixed: composited inside the view body, the panel lost its bottom border below
// about 18 rows and "esc close" fell off the screen entirely.
func TestCorporateActionDetails_ShortTerminalKeepsHint(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 12)
	if !strings.Contains(ansi.Strip(app.renderLayout()), "esc close") {
		t.Error("esc close is off screen on a 12-row terminal")
	}
}

// TestCorporateActionDetails_NilViewRendersNothing pins the guard the renderer
// gained when it moved out of renderCorporateActionView, whose nil check used to
// cover it. The ticker lookups dereference corporateActionView.secMap.
func TestCorporateActionDetails_NilViewRendersNothing(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	app.corporateActionView = nil
	if got := app.renderCorporateActionDetails(); got != "" {
		t.Errorf("render returned %d bytes, want empty", len(got))
	}
}

// TestCorporateActionDetail_CloseButtonClosesOverlay is the mouse wiring.
func TestCorporateActionDetail_CloseButtonClosesOverlay(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	x, y := screenCellOf(t, app.renderLayout(), "[x]")

	model, _ := app.handleMouseEvent(tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
	app = model.(*App)
	if app.corporateActionDetail != nil {
		t.Error("clicking [x] did not close the details overlay")
	}
}

// TestCorporateActionDetail_MissesAreInert checks that nothing but the [x]
// closes the overlay, and that the register underneath cannot move.
func TestCorporateActionDetail_MissesAreInert(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	x, y := screenCellOf(t, app.renderLayout(), "[x]")

	for _, tc := range []struct {
		name string
		x, y int
	}{
		{"one column left of the close button", x - 1, y},
		{"one row below", x + 1, y + 1},
		{"one row above", x + 1, y - 1},
		{"the body", x - 20, y + 3},
		{"outside the panel", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.corporateActionDetailMouseAction(tc.x, tc.y); got != dialog.DialogActionNone {
				t.Errorf("action = %v, want none", got)
			}
		})
	}
}

// TestCorporateActionDetail_WheelIsSwallowed checks the register does not scroll
// behind the modal.
func TestCorporateActionDetail_WheelIsSwallowed(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	if app.corporateActionViewTable == nil {
		t.Skip("no table built")
	}
	app.corporateActionViewTable.SetCursor(1)
	before := app.corporateActionViewTable.Cursor()

	model, _ := app.handleMouseEvent(tea.MouseWheelMsg{X: 60, Y: 20, Button: tea.MouseWheelDown})
	app = model.(*App)

	if app.corporateActionDetail == nil {
		t.Error("the wheel closed the overlay")
	}
	if got := app.corporateActionViewTable.Cursor(); got != before {
		t.Errorf("register cursor moved to %d from %d behind the modal", got, before)
	}
}

// TestCorporateActionDetail_ViewSwitchClearsOverlay covers a live bug the move
// exposed: switchView left the overlay set, so isDialogVisible stayed true and
// every click on the next view was routed into the dialog cascade and dropped —
// a dead mouse with no modal on screen.
func TestCorporateActionDetail_ViewSwitchClearsOverlay(t *testing.T) {
	app, _ := corporateActionDetailEnv(t, 120, 40)
	app.switchView(ViewDashboard)

	if app.corporateActionDetail != nil {
		t.Error("switching view left the details overlay set")
	}
	if app.isDialogVisible() {
		t.Error("isDialogVisible is still true, so the mouse stays dead on the new view")
	}
	if strings.Contains(ansi.Strip(app.renderLayout()), "Action Details") {
		t.Error("the details panel is painted over the dashboard")
	}
}
