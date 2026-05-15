package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
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
