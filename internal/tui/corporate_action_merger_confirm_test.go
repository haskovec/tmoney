package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// SM-170: Merge security dialog with confirmation step

func TestMergerConfirmData_Empty(t *testing.T) {
	data := &mergerConfirmData{
		sourceTicker: "AAPL",
		targetTicker: "MSFT",
		accounts:     nil,
	}

	if len(data.accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(data.accounts))
	}
}

func TestMergerConfirmData_WithAccountsAndLots(t *testing.T) {
	acctID := types.NewID()
	data := &mergerConfirmData{
		sourceTicker: "AAPL",
		targetTicker: "MSFT",
		exchangeRatio: 2.0,
		cashPerShare:  5.00,
		accounts: []mergerAffectedAccount{
			{
				accountID:   acctID,
				accountName: "Brokerage",
				trackLots:   true,
				lots: []*investment.Lot{
					{
						BaseModel:    types.NewBaseModel(),
						AccountID:    acctID,
						Shares:       types.NewQuantityFromFloat(10),
						CostPerShare: types.NewMoneyFromFloat(150.00),
						PurchaseDate: types.NewDate(2024, 1, 15),
					},
				},
			},
		},
	}

	if len(data.accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(data.accounts))
	}
	if data.accounts[0].accountName != "Brokerage" {
		t.Errorf("account name = %q, want %q", data.accounts[0].accountName, "Brokerage")
	}
	if !data.accounts[0].trackLots {
		t.Error("account should be lot-tracking")
	}
	if len(data.accounts[0].lots) != 1 {
		t.Errorf("expected 1 lot, got %d", len(data.accounts[0].lots))
	}
}

func TestMergerConfirmData_WithPositions(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	data := &mergerConfirmData{
		sourceTicker: "AAPL",
		targetTicker: "MSFT",
		exchangeRatio: 2.0,
		accounts: []mergerAffectedAccount{
			{
				accountID:   acctID,
				accountName: "Simple Portfolio",
				trackLots:   false,
				position: &investment.Position{
					BaseModel:           types.NewBaseModel(),
					AccountID:           acctID,
					SecurityID:          secID,
					Shares:              types.NewQuantityFromFloat(50),
					AverageCostPerShare: types.NewMoneyFromFloat(100.00),
				},
			},
		},
	}

	if len(data.accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(data.accounts))
	}
	if data.accounts[0].trackLots {
		t.Error("account should NOT be lot-tracking")
	}
	if data.accounts[0].position == nil {
		t.Error("position should not be nil")
	}
}

func TestMergerConfirmDataMsg(t *testing.T) {
	data := &mergerConfirmData{
		sourceTicker: "AAPL",
		targetTicker: "MSFT",
	}
	msg := mergerConfirmDataMsg{data: data}

	if msg.data.sourceTicker != "AAPL" {
		t.Errorf("source ticker = %q, want AAPL", msg.data.sourceTicker)
	}
}

func TestBuildMergerConfirmParams(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()
	d := types.NewDate(2024, 6, 10)

	params := mergerConfirmParams{
		sourceSecurityID: sourceID,
		targetSecurityID: targetID,
		mergerDate:       d,
		exchangeRatio:    2.5,
		cashPerShare:     10.00,
	}

	if params.sourceSecurityID != sourceID {
		t.Error("source security ID mismatch")
	}
	if params.targetSecurityID != targetID {
		t.Error("target security ID mismatch")
	}
	if params.exchangeRatio != 2.5 {
		t.Errorf("exchange ratio = %f, want 2.5", params.exchangeRatio)
	}
	if params.cashPerShare != 10.00 {
		t.Errorf("cash per share = %f, want 10.00", params.cashPerShare)
	}
}

func TestSubmitMergerDialog_TransitionsToConfirmation(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0 // source = AAPL
	fields[1].SelectedIndex = 1 // target = MSFT
	fields[2].Value = "06/10/2024"
	fields[3].Value = "2.5"
	fields[4].Value = "10.00"

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	// Dialog should be closed
	if updatedApp.mergerDialog != nil {
		t.Error("merger dialog should be closed after submit")
	}

	// Should have stored confirm params
	if updatedApp.mergerConfirmParams == nil {
		t.Fatal("confirm params should be stored")
	}
	if updatedApp.mergerConfirmParams.sourceSecurityID != sourceID {
		t.Error("source security ID mismatch")
	}
	if updatedApp.mergerConfirmParams.targetSecurityID != targetID {
		t.Error("target security ID mismatch")
	}
	if updatedApp.mergerConfirmParams.exchangeRatio != 2.5 {
		t.Errorf("exchange ratio = %f, want 2.5", updatedApp.mergerConfirmParams.exchangeRatio)
	}
	if updatedApp.mergerConfirmParams.cashPerShare != 10.00 {
		t.Errorf("cash per share = %f, want 10.00", updatedApp.mergerConfirmParams.cashPerShare)
	}

	// Should return a command to load confirmation data
	if cmd == nil {
		t.Error("should return command to load confirmation data")
	}
}

func TestSubmitMergerDialog_TransitionsWithoutCash(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0
	fields[1].SelectedIndex = 1
	fields[2].Value = "06/10/2024"
	fields[3].Value = "1.5"
	fields[4].Value = "" // no cash

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed")
	}
	if updatedApp.mergerConfirmParams == nil {
		t.Fatal("confirm params should be stored")
	}
	if updatedApp.mergerConfirmParams.cashPerShare != 0 {
		t.Errorf("cash per share = %f, want 0", updatedApp.mergerConfirmParams.cashPerShare)
	}
	if cmd == nil {
		t.Error("should return command to load confirmation data")
	}
}

func TestCloseMergerConfirmation(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker: "AAPL",
			targetTicker: "MSFT",
		},
		mergerConfirmParams: &mergerConfirmParams{
			sourceSecurityID: types.NewID(),
		},
	}

	app.closeMergerConfirmation()

	if app.mergerConfirmData != nil {
		t.Error("confirm data should be nil after close")
	}
	if app.mergerConfirmParams != nil {
		t.Error("confirm params should be nil after close")
	}
}

func TestHandleMergerConfirmKey_Escape(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker: "AAPL",
			targetTicker: "MSFT",
		},
		mergerConfirmParams: &mergerConfirmParams{
			sourceSecurityID: types.NewID(),
		},
		keys: defaultKeyMap(),
	}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, _ := app.handleMergerConfirmKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.mergerConfirmData != nil {
		t.Error("confirm data should be cleared after Escape")
	}
	if updatedApp.mergerConfirmParams != nil {
		t.Error("confirm params should be cleared after Escape")
	}
}

func TestHandleMergerConfirmKey_NilData(t *testing.T) {
	app := &App{
		keys: defaultKeyMap(),
	}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, cmd := app.handleMergerConfirmKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when data is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when data is nil")
	}
}

func TestHandleMergerConfirmKey_Enter(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()
	d := types.NewDate(2024, 6, 10)

	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker: "AAPL",
			targetTicker: "MSFT",
			accounts: []mergerAffectedAccount{
				{accountName: "Brokerage"},
			},
		},
		mergerConfirmParams: &mergerConfirmParams{
			sourceSecurityID: sourceID,
			targetSecurityID: targetID,
			mergerDate:       d,
			exchangeRatio:    2.0,
			cashPerShare:     0,
		},
		keys: defaultKeyMap(),
	}

	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	model, cmd := app.handleMergerConfirmKey(enterKey)
	updatedApp := model.(*App)

	// Confirmation should be closed
	if updatedApp.mergerConfirmData != nil {
		t.Error("confirm data should be cleared after Enter")
	}
	if updatedApp.mergerConfirmParams != nil {
		t.Error("confirm params should be cleared after Enter")
	}
	// Should return an execution command
	if cmd == nil {
		t.Error("should return a command for async merger execution")
	}
}

func TestRenderMergerConfirmation_NotNil(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			cashPerShare:  5.00,
			date:          "06/10/2024",
			accounts: []mergerAffectedAccount{
				{
					accountName: "Brokerage",
					trackLots:   true,
					lots: []*investment.Lot{
						{
							Shares:       types.NewQuantityFromFloat(10),
							CostPerShare: types.NewMoneyFromFloat(150.00),
							PurchaseDate: types.NewDate(2024, 1, 15),
						},
					},
				},
			},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if result == "" {
		t.Error("render should not return empty string")
	}
}

func TestRenderMergerConfirmation_NilData(t *testing.T) {
	app := &App{}
	result := app.renderMergerConfirmation()
	if result != "" {
		t.Error("should return empty string when data is nil")
	}
}

func TestRenderMergerConfirmation_ContainsSourceAndTarget(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			date:          "06/10/2024",
			accounts:      []mergerAffectedAccount{},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if !strings.Contains(result, "AAPL") {
		t.Error("render should contain source ticker AAPL")
	}
	if !strings.Contains(result, "MSFT") {
		t.Error("render should contain target ticker MSFT")
	}
}

func TestRenderMergerConfirmation_ContainsCashInfo(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			cashPerShare:  5.00,
			date:          "06/10/2024",
			accounts:      []mergerAffectedAccount{},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if !strings.Contains(result, "5.00") {
		t.Error("render should contain cash per share amount")
	}
}

func TestRenderMergerConfirmation_NoAccounts(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			date:          "06/10/2024",
			accounts:      []mergerAffectedAccount{},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if !strings.Contains(result, "No accounts") {
		t.Error("should show 'No accounts' when no accounts affected")
	}
}

func TestRenderMergerConfirmation_WithLotTrackingAccount(t *testing.T) {
	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			date:          "06/10/2024",
			accounts: []mergerAffectedAccount{
				{
					accountName: "Brokerage",
					trackLots:   true,
					lots: []*investment.Lot{
						{
							Shares:       types.NewQuantityFromFloat(10),
							CostPerShare: types.NewMoneyFromFloat(150.00),
							PurchaseDate: types.NewDate(2024, 1, 15),
						},
						{
							Shares:       types.NewQuantityFromFloat(20),
							CostPerShare: types.NewMoneyFromFloat(160.00),
							PurchaseDate: types.NewDate(2024, 3, 1),
						},
					},
				},
			},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if !strings.Contains(result, "Brokerage") {
		t.Error("should contain account name")
	}
}

func TestRenderMergerConfirmation_WithNonLotAccount(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			date:          "06/10/2024",
			accounts: []mergerAffectedAccount{
				{
					accountName: "Simple",
					trackLots:   false,
					position: &investment.Position{
						AccountID:           acctID,
						SecurityID:          secID,
						Shares:              types.NewQuantityFromFloat(50),
						AverageCostPerShare: types.NewMoneyFromFloat(100.00),
					},
				},
			},
		},
		styles: NewStyles(),
		width:  80,
		height: 40,
	}

	result := app.renderMergerConfirmation()
	if !strings.Contains(result, "Simple") {
		t.Error("should contain account name")
	}
}

func TestApp_Update_MergerConfirmDataMsg(t *testing.T) {
	app := &App{
		statusbar: NewStatusBar(),
		mergerConfirmParams: &mergerConfirmParams{
			sourceSecurityID: types.NewID(),
		},
	}

	data := &mergerConfirmData{
		sourceTicker: "AAPL",
		targetTicker: "MSFT",
		accounts: []mergerAffectedAccount{
			{accountName: "Brokerage"},
		},
	}

	msg := mergerConfirmDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.mergerConfirmData == nil {
		t.Error("confirm data should be set after receiving message")
	}
	if updatedApp.mergerConfirmData.sourceTicker != "AAPL" {
		t.Errorf("source ticker = %q, want AAPL", updatedApp.mergerConfirmData.sourceTicker)
	}
}

