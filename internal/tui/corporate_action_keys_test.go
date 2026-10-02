package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
)

// corporateActionsApp opens the Corporate Actions view from the dashboard,
// so Esc on the list has somewhere to go back to.
func corporateActionsApp(t *testing.T) *App {
	t.Helper()
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40
	a.switchView(ViewCorporateActions)
	runCmd(t, a, a.loadCorporateActionViewData(), 1)
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
	if a.corporateActionViewFilter != "12?" {
		t.Errorf("filter = %q, want %q", a.corporateActionViewFilter, "12?")
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
	if a.corporateActionView.filterEditing {
		t.Error("Esc did not end filter entry")
	}
}

// Esc with the details panel open closes the panel and stays in the view.
func TestCorporateActions_EscClosesDetails(t *testing.T) {
	a := corporateActionsApp(t)
	a.corporateActionView.detail = &investment.CorporateAction{}
	press(a, escKey)

	if a.corporateActionView.detail != nil {
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

	if a.corporateActionView.filterEditing {
		t.Error("the filter entry outlived the view")
	}
}

// A round trip through another view keeps the ticker filter, but closes the
// details panel and ends a filter entry: the view's leave hook drops only
// those two.
func TestCorporateActions_RoundTripKeepsTheFilter(t *testing.T) {
	a := corporateActionsApp(t)
	press(a, typed("/ab")...)
	a.corporateActionView.detail = &investment.CorporateAction{}
	if !a.corporateActionView.filterEditing || a.corporateActionViewFilter != "ab" {
		t.Fatalf("setup: filterEditing=%v filter=%q, want true and %q",
			a.corporateActionView.filterEditing, a.corporateActionViewFilter, "ab")
	}

	a.switchView(ViewDashboard)
	a.switchView(ViewCorporateActions)

	if a.corporateActionViewFilter != "ab" {
		t.Errorf("the round trip changed the filter to %q, want %q", a.corporateActionViewFilter, "ab")
	}
	if a.corporateActionView.detail != nil {
		t.Error("the details panel survived the round trip")
	}
	if a.corporateActionView.filterEditing {
		t.Error("the filter entry survived the round trip")
	}
}
