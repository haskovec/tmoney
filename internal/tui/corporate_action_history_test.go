package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestFormatCorporateActionDetails_Split(t *testing.T) {
	params := investment.SplitParams{Numerator: 4, Denominator: 1}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	ca := &investment.CorporateAction{
		ActionType: investment.ActionTypeSplit,
		Parameters: jsonStr,
	}

	details := formatCorporateActionDetails(ca, nil)
	if details != "Ratio 4:1" {
		t.Errorf("split details = %q, want %q", details, "Ratio 4:1")
	}
}

func TestFormatCorporateActionDetails_ReverseSplit(t *testing.T) {
	params := investment.SplitParams{Numerator: 1, Denominator: 10}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	ca := &investment.CorporateAction{
		ActionType: investment.ActionTypeReverseSplit,
		Parameters: jsonStr,
	}

	details := formatCorporateActionDetails(ca, nil)
	if details != "Ratio 1:10" {
		t.Errorf("reverse split details = %q, want %q", details, "Ratio 1:10")
	}
}

func TestFormatCorporateActionDetails_Merger(t *testing.T) {
	params := investment.MergerParams{ExchangeRatio: 0.5, CashPerShare: 10.00}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	targetID := types.NewID()
	targetSec := security.NewSecurity("NEWCO", "New Company", security.TypeStock)
	targetSec.ID = targetID
	secMap := map[types.ID]*security.Security{targetID: targetSec}

	ca := &investment.CorporateAction{
		ActionType:       investment.ActionTypeMerger,
		Parameters:       jsonStr,
		TargetSecurityID: types.NullableID{ID: targetID, Valid: true},
	}

	details := formatCorporateActionDetails(ca, secMap)
	expected := "→ NEWCO, ratio 0.50, cash $10.00/sh"
	if details != expected {
		t.Errorf("merger details = %q, want %q", details, expected)
	}
}

func TestFormatCorporateActionDetails_MergerNoCash(t *testing.T) {
	params := investment.MergerParams{ExchangeRatio: 1.5}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	targetID := types.NewID()
	targetSec := security.NewSecurity("BIGCO", "Big Company", security.TypeStock)
	targetSec.ID = targetID
	secMap := map[types.ID]*security.Security{targetID: targetSec}

	ca := &investment.CorporateAction{
		ActionType:       investment.ActionTypeMerger,
		Parameters:       jsonStr,
		TargetSecurityID: types.NullableID{ID: targetID, Valid: true},
	}

	details := formatCorporateActionDetails(ca, secMap)
	expected := "→ BIGCO, ratio 1.50"
	if details != expected {
		t.Errorf("merger no cash details = %q, want %q", details, expected)
	}
}

func TestFormatCorporateActionDetails_SpinOff(t *testing.T) {
	params := investment.SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	targetID := types.NewID()
	targetSec := security.NewSecurity("SPUN", "Spun Off Inc.", security.TypeStock)
	targetSec.ID = targetID
	secMap := map[types.ID]*security.Security{targetID: targetSec}

	ca := &investment.CorporateAction{
		ActionType:       investment.ActionTypeSpinOff,
		Parameters:       jsonStr,
		TargetSecurityID: types.NullableID{ID: targetID, Valid: true},
	}

	details := formatCorporateActionDetails(ca, secMap)
	expected := "→ SPUN, ratio 0.25, parent 80.0%"
	if details != expected {
		t.Errorf("spin-off details = %q, want %q", details, expected)
	}
}

func TestFormatCorporateActionDetails_UnknownTarget(t *testing.T) {
	params := investment.MergerParams{ExchangeRatio: 1.0}
	jsonStr, err := params.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	unknownID := types.NewID()
	ca := &investment.CorporateAction{
		ActionType:       investment.ActionTypeMerger,
		Parameters:       jsonStr,
		TargetSecurityID: types.NullableID{ID: unknownID, Valid: true},
	}

	details := formatCorporateActionDetails(ca, nil)
	expected := "→ ???, ratio 1.00"
	if details != expected {
		t.Errorf("unknown target details = %q, want %q", details, expected)
	}
}

func TestFormatCorporateActionDetails_InvalidJSON(t *testing.T) {
	ca := &investment.CorporateAction{
		ActionType: investment.ActionTypeSplit,
		Parameters: "not json",
	}

	details := formatCorporateActionDetails(ca, nil)
	if details != "not json" {
		t.Errorf("invalid json details = %q, want raw parameters", details)
	}
}

func TestBuildCorporateActionHistoryTable(t *testing.T) {
	secID := types.NewID()
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.ID = secID

	splitParams := investment.SplitParams{Numerator: 4, Denominator: 1}
	splitJSON, _ := splitParams.ToJSON()

	date := types.NewDate(2024, 6, 10)
	ca := &investment.CorporateAction{
		ID:         types.NewID(),
		ActionType: investment.ActionTypeSplit,
		SecurityID: secID,
		ActionDate: date,
		Parameters: splitJSON,
	}

	app := &App{}
	app.corporateActionHistory = &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{ca},
		secMap:   map[types.ID]*security.Security{secID: sec},
	}
	app.buildCorporateActionHistoryTable()

	if app.corporateActionHistoryTable == nil {
		t.Fatal("table should not be nil")
	}
}

func TestBuildCorporateActionHistoryTable_Empty(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{}
	app.corporateActionHistory = &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{},
		secMap:   map[types.ID]*security.Security{},
	}
	app.buildCorporateActionHistoryTable()

	if app.corporateActionHistoryTable == nil {
		t.Fatal("table should not be nil even with no actions")
	}
}

func TestBuildCorporateActionHistoryTable_NilData(t *testing.T) {
	app := &App{}
	app.buildCorporateActionHistoryTable()
	// Should not panic
}

func TestHandleCorporateActionHistoryKeys_Escape(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		keys: defaultKeyMap(),
		corporateActionHistory: &corporateActionHistoryData{
			security: sec,
			actions:  []*investment.CorporateAction{},
			secMap:   map[types.ID]*security.Security{},
		},
		corporateActionHistoryTable: NewTable([]Column{{Header: "Test"}}),
	}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, _ := app.handleCorporateActionHistoryKeys(escKey)
	updatedApp := model.(*App)

	if updatedApp.corporateActionHistory != nil {
		t.Error("history data should be cleared after Escape")
	}
	if updatedApp.corporateActionHistoryTable != nil {
		t.Error("history table should be cleared after Escape")
	}
}

func TestHandleCorporateActionHistoryKeys_NilData(t *testing.T) {
	app := &App{}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, cmd := app.handleCorporateActionHistoryKeys(escKey)

	if model.(*App) != app {
		t.Error("should return same app when data is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when data is nil")
	}
}

func TestCorporateActionHistoryDataLoadedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	data := &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{},
		secMap:   map[types.ID]*security.Security{},
	}

	msg := corporateActionHistoryDataLoadedMsg{data: data}
	if msg.data.security.Ticker != "AAPL" {
		t.Errorf("expected AAPL, got %s", msg.data.security.Ticker)
	}
}

func TestCloseCorporateActionHistory(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		corporateActionHistory: &corporateActionHistoryData{
			security: sec,
			actions:  []*investment.CorporateAction{},
			secMap:   map[types.ID]*security.Security{},
		},
		corporateActionHistoryTable: NewTable([]Column{{Header: "Test"}}),
	}

	app.closeCorporateActionHistory()

	if app.corporateActionHistory != nil {
		t.Error("corporateActionHistory should be nil after close")
	}
	if app.corporateActionHistoryTable != nil {
		t.Error("corporateActionHistoryTable should be nil after close")
	}
}

func TestRenderCorporateActionHistory_NilData(t *testing.T) {
	app := &App{
		styles: NewStyles(),
		width:  80,
		height: 24,
	}

	// Should not panic with nil data
	result := app.renderCorporateActionHistory()
	if result != "" {
		t.Error("should return empty string when data is nil")
	}
}

func TestRenderCorporateActionHistory_WithData(t *testing.T) {
	secID := types.NewID()
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.ID = secID

	splitParams := investment.SplitParams{Numerator: 4, Denominator: 1}
	splitJSON, _ := splitParams.ToJSON()

	ca := &investment.CorporateAction{
		ID:         types.NewID(),
		ActionType: investment.ActionTypeSplit,
		SecurityID: secID,
		ActionDate: types.NewDate(2024, 6, 10),
		Parameters: splitJSON,
	}

	app := &App{
		styles: NewStyles(),
		width:  80,
		height: 24,
	}
	app.corporateActionHistory = &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{ca},
		secMap:   map[types.ID]*security.Security{secID: sec},
	}
	app.buildCorporateActionHistoryTable()

	result := app.renderCorporateActionHistory()
	if result == "" {
		t.Error("should return non-empty string with data")
	}
}

func TestRenderCorporateActionHistory_EmptyActions(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		styles: NewStyles(),
		width:  80,
		height: 24,
	}
	app.corporateActionHistory = &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{},
		secMap:   map[types.ID]*security.Security{},
	}
	app.buildCorporateActionHistoryTable()

	result := app.renderCorporateActionHistory()
	if result == "" {
		t.Error("should return non-empty string even with no actions")
	}
}

func TestFormatCorporateActionRow(t *testing.T) {
	secID := types.NewID()
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.ID = secID

	params := investment.SplitParams{Numerator: 4, Denominator: 1}
	jsonStr, _ := params.ToJSON()

	ca := &investment.CorporateAction{
		ActionType: investment.ActionTypeSplit,
		SecurityID: secID,
		ActionDate: types.NewDate(2024, 6, 10),
		Parameters: jsonStr,
	}

	secMap := map[types.ID]*security.Security{secID: sec}
	row := formatCorporateActionRow(ca, secMap)

	if len(row) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(row))
	}
	if row[0] != "2024-06-10" {
		t.Errorf("date = %q, want %q", row[0], "2024-06-10")
	}
	if row[1] != "Stock Split" {
		t.Errorf("type = %q, want %q", row[1], "Stock Split")
	}
	if row[2] != "Ratio 4:1" {
		t.Errorf("details = %q, want %q", row[2], "Ratio 4:1")
	}
}

func TestApp_Update_CorporateActionHistoryDataLoadedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	data := &corporateActionHistoryData{
		security: sec,
		actions:  []*investment.CorporateAction{},
		secMap:   map[types.ID]*security.Security{},
	}

	app := &App{
		statusbar: NewStatusBar(),
	}

	msg := corporateActionHistoryDataLoadedMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.corporateActionHistory == nil {
		t.Error("corporateActionHistory should be set after data loaded")
	}
	if updatedApp.corporateActionHistoryTable == nil {
		t.Error("corporateActionHistoryTable should be built after data loaded")
	}
}
