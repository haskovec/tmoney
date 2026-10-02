package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// corporateActionsApp opens the Corporate Actions view from the dashboard,
// so Esc on the list has somewhere to go back to.
func corporateActionsApp(t *testing.T) *App {
	t.Helper()
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40
	a.switchView(ViewCorporateActions)
	runCmd(t, a, a.corporateActions.load(a.corporateActionDeps()), 1)
	return a
}

func press(a *App, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		a.Update(k)
	}
}

func typed(s string) []tea.KeyPressMsg {
	var out []tea.KeyPressMsg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

var escKey = tea.KeyPressMsg{Code: tea.KeyEscape}

// While the filter is being typed, digits and "?" are filter text, not the
// global view-switch and help keys.
func TestCorporateActions_FilterTakesGlobalKeys(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, typed("/12?")...)

	if a.currentView != ViewCorporateActions {
		t.Errorf("view = %v, want Corporate Actions: a digit switched the view", a.currentView)
	}
	if a.showHelp {
		t.Error("? opened the help overlay while typing the filter")
	}
	if a.corporateActions.filter != "12?" {
		t.Errorf("filter = %q, want %q", a.corporateActions.filter, "12?")
	}
}

// Esc while typing ends the entry and stays in the view.
func TestCorporateActions_EscEndsFilterEntry(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, typed("/ab")...)
	press(a, escKey)

	if a.currentView != ViewCorporateActions {
		t.Errorf("view = %v, want Corporate Actions", a.currentView)
	}
	if a.corporateActions.filterEditing {
		t.Error("Esc did not end filter entry")
	}
}

// Esc with the details panel open closes the panel and stays in the view.
func TestCorporateActions_EscClosesDetails(t *testing.T) {
	a := corporateActionsApp(t)
	a.corporateActions.detail = &investment.CorporateAction{}
	press(a, escKey)

	if a.corporateActions.detail != nil {
		t.Error("Esc did not close the details panel")
	}
	if a.currentView != ViewCorporateActions {
		t.Errorf("view = %v, want Corporate Actions", a.currentView)
	}
}

// Esc on the list goes back, as before.
func TestCorporateActions_EscOnListGoesBack(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, escKey)

	if a.currentView != ViewDashboard {
		t.Errorf("view = %v, want the dashboard it came from", a.currentView)
	}
}

// Leaving the view ends a filter entry, so the view does not return still
// capturing every key.
func TestCorporateActions_LeavingEndsFilterEntry(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, typed("/ab")...)
	a.switchView(ViewDashboard)

	if a.corporateActions.filterEditing {
		t.Error("the filter entry outlived the view")
	}
}

// A round trip through another view keeps the ticker filter, but closes the
// details panel and ends a filter entry: the view's leave hook drops only
// those two.
func TestCorporateActions_RoundTripKeepsTheFilter(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, typed("/ab")...)
	a.corporateActions.detail = &investment.CorporateAction{}
	if !a.corporateActions.filterEditing || a.corporateActions.filter != "ab" {
		t.Fatalf("setup: filterEditing=%v filter=%q, want true and %q",
			a.corporateActions.filterEditing, a.corporateActions.filter, "ab")
	}

	a.switchView(ViewDashboard)
	a.switchView(ViewCorporateActions)

	if a.corporateActions.filter != "ab" {
		t.Errorf("the round trip changed the filter to %q, want %q", a.corporateActions.filter, "ab")
	}
	if a.corporateActions.detail != nil {
		t.Error("the details panel survived the round trip")
	}
	if a.corporateActions.filterEditing {
		t.Error("the filter entry survived the round trip")
	}
}

// The rows under the ticker filter, through the app: the load arm, the render,
// the table builds while the filter is typed, and the selection on Enter and d.
// The fixture (corporateActionsEnv) lists an AAPL split, then an MSFT merger;
// under the filter "MSFT" only the merger shows.

// The load arm builds the table under the filter a drill-in from Securities
// set before the load.
func TestCorporateActions_LoadBuildsTheTableUnderTheFilter(t *testing.T) {
	app, _, _ := corporateActionsEnv(t, 120, 40, "MSFT")
	data := app.corporateActions.data
	app.corporateActions.data, app.corporateActions.table = nil, nil
	app.Update(corporateActionViewLoadedMsg{data: data})

	if got := app.corporateActions.table.RowCount(); got != 1 {
		t.Errorf("table has %d rows under the filter MSFT, want 1", got)
	}
}

// The render names the filter in its header, or says how to set one.
func TestCorporateActions_RenderNamesTheFilter(t *testing.T) {
	e, ok := viewFor(ViewCorporateActions)
	if !ok {
		t.Fatal("no view table entry for ViewCorporateActions")
	}
	app, _, _ := corporateActionsEnv(t, 120, 40, "MSFT")
	if out := e.render(app); !strings.Contains(out, "Filter: MSFT") {
		t.Errorf("the render does not name the filter:\n%s", out)
	}
	app, _, _ = corporateActionsEnv(t, 120, 40, "")
	if out := e.render(app); !strings.Contains(out, "Press / to filter by ticker or type") {
		t.Errorf("with no filter, the render does not say how to set one:\n%s", out)
	}
}

// Typing the filter rebuilds the table under the new text, on each character
// and on each backspace.
func TestCorporateActions_TypingRebuildsTheTableUnderTheFilter(t *testing.T) {
	app, _, _ := corporateActionsEnv(t, 120, 40, "")

	press(app, typed("/MSFTX")...)
	if got := app.corporateActions.table.RowCount(); got != 0 {
		t.Errorf("table has %d rows under the filter MSFTX, want 0", got)
	}
	press(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := app.corporateActions.table.RowCount(); got != 1 {
		t.Errorf("table has %d rows under the filter MSFT after a backspace, want 1", got)
	}
}

// Enter and d act on the row under the cursor among the filtered rows, not on
// the row at the same place in the full list.
func TestCorporateActions_SelectionFollowsTheFilter(t *testing.T) {
	t.Run("enter opens the details", func(t *testing.T) {
		app, _, merger := corporateActionsEnv(t, 120, 40, "MSFT")
		press(app, tea.KeyPressMsg{Code: tea.KeyEnter})
		if app.corporateActions.detail != merger {
			t.Error("enter opened the details of an action the filter hides")
		}
	})
	t.Run("d asks to reverse it", func(t *testing.T) {
		app, _, _ := corporateActionsEnv(t, 120, 40, "MSFT")
		press(app, typed("d")...)
		if !app.confirm.IsVisible() {
			t.Fatal("d did not ask to reverse the action")
		}
		if got := widget.StripAnsi(app.confirm.Render(app.styles)); !strings.Contains(got, "MSFT") {
			t.Errorf("d asked to reverse an action the filter hides:\n%s", got)
		}
	})
}
