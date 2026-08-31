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
		sourceTicker:  "AAPL",
		targetTicker:  "MSFT",
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
		sourceTicker:  "AAPL",
		targetTicker:  "MSFT",
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

	// dialog.Dialog should be closed
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

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
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

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
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

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		statusbar: widget.NewStatusBar(),
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

// ===========================================================================
// Merger confirmation: mouse support
// ===========================================================================

// mergerConfirmMouseEnv builds an app with the merger confirmation open at a
// real screen size, and returns the rendered overlay plus its geometry.
func mergerConfirmMouseEnv(t *testing.T, w, h, accounts int) (*App, string, int, int, int) {
	t.Helper()
	affected := make([]mergerAffectedAccount, 0, accounts)
	for i := range accounts {
		affected = append(affected, mergerAffectedAccount{
			accountName: fmt.Sprintf("Brokerage %d", i+1),
		})
	}
	app := &App{
		ready:  true,
		width:  w,
		height: h,
		styles: widget.NewStyles(),
		mergerConfirmData: &mergerConfirmData{
			sourceTicker:  "AAPL",
			targetTicker:  "MSFT",
			exchangeRatio: 2.0,
			date:          "06/10/2024",
			accounts:      affected,
		},
		mergerConfirmParams: &mergerConfirmParams{
			sourceSecurityID: types.NewID(),
			targetSecurityID: types.NewID(),
			mergerDate:       types.NewDate(2024, time.June, 10),
			exchangeRatio:    2.0,
		},
	}
	overlay := app.renderMergerConfirmation()
	startCol, startRow := widget.OverlayTopLeft(overlay, w, h)
	overlayWidth := 0
	for _, ln := range strings.Split(overlay, "\n") {
		if lw := lipgloss.Width(ln); lw > overlayWidth {
			overlayWidth = lw
		}
	}
	return app, overlay, startCol, startRow, overlayWidth - dialog.DialogHorizontalOverhead
}

// TestRenderMergerConfirmation_SeparatorsFitContentWidth is the wrap
// regression: the inner width was two columns wider than the content band, so
// every separator spilled onto a stub row.
func TestRenderMergerConfirmation_SeparatorsFitContentWidth(t *testing.T) {
	_, overlay, _, _, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
	lines := strings.Split(overlay, "\n")
	runs := 0
	for i, ln := range lines {
		// Skip the panel's own top and bottom border, which OverlayBox draws
		// with the same box-drawing rune the separators use.
		if i > 0 && i < len(lines)-1 && strings.Contains(ansi.Strip(ln), "───") {
			runs++
		}
		if lw := lipgloss.Width(ln); lw != 70 {
			t.Errorf("line %d is %d columns wide, want 70", i, lw)
		}
	}
	// renderMergerConfirmation draws exactly three separators. A fourth or
	// fifth row means one of them wrapped onto a stub line.
	if runs != 3 {
		t.Errorf("%d content rows carry a separator run, want 3 (more means they wrapped)", runs)
	}
}

// TestRenderMergerConfirmation_HasCloseButtonAndActionRow pins the affordances
// and, crucially, that the button row is the LAST content line — the hit test
// locates it from the rendered height, never by counting sections.
func TestRenderMergerConfirmation_HasCloseButtonAndActionRow(t *testing.T) {
	_, overlay, _, _, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
	plain := ansi.Strip(overlay)
	for _, want := range []string{"[x]", "[ Cancel ]", "[ Merge ]", "enter/y confirm", "esc cancel"} {
		if !strings.Contains(plain, want) {
			t.Errorf("render is missing %q", want)
		}
	}
	lines := strings.Split(overlay, "\n")
	mergeRow := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "[ Merge ]") {
			mergeRow = i
		}
	}
	if want := len(lines) - 3; mergeRow != want {
		t.Errorf("[ Merge ] on row %d, want %d (the last content line)", mergeRow, want)
	}
}

// TestRenderMergerConfirmation_ButtonRowAtNarrowWidth pins the same invariant
// where the hint itself wraps to two rows, which is exactly what would break a
// section-counting hit test.
func TestRenderMergerConfirmation_ButtonRowAtNarrowWidth(t *testing.T) {
	_, overlay, _, _, _ := mergerConfirmMouseEnv(t, 38, 40, 1)
	lines := strings.Split(overlay, "\n")
	plain := ansi.Strip(overlay)
	if !strings.Contains(plain, "[ Cancel ]") || !strings.Contains(plain, "[ Merge ]") {
		t.Fatal("both buttons should fit at the narrow floor width")
	}
	mergeRow := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "[ Merge ]") {
			mergeRow = i
		}
	}
	if want := len(lines) - 3; mergeRow != want {
		t.Errorf("[ Merge ] on row %d, want %d", mergeRow, want)
	}
}

// TestMergerConfirmMouseAction_Targets checks each live target and confirms the
// [x] and Cancel both mean cancel while only Merge submits.
func TestMergerConfirmMouseAction_Targets(t *testing.T) {
	for _, w := range []int{38, 80, 120, 200} {
		t.Run(fmt.Sprintf("width%d", w), func(t *testing.T) {
			app, overlay, startCol, startRow, contentWidth := mergerConfirmMouseEnv(t, w, 40, 1)

			// The [x] sits in the last three content columns of the title row.
			if got := app.mergerConfirmMouseAction(startCol+3+contentWidth-2, startRow+2); got != dialog.DialogActionCancel {
				t.Errorf("[x] gave %v, want cancel", got)
			}
			for _, tc := range []struct {
				label string
				want  dialog.DialogAction
			}{
				{"[ Cancel ]", dialog.DialogActionCancel},
				{"[ Merge ]", dialog.DialogActionSubmit},
			} {
				x, y := screenCellOf(t, overlay, tc.label)
				if got := app.mergerConfirmMouseAction(startCol+x+2, startRow+y); got != tc.want {
					t.Errorf("%s gave %v, want %v", tc.label, got, tc.want)
				}
			}
		})
	}
}

// TestMergerConfirmMouseAction_InertRegions is the safety net: no stray click
// anywhere else may merge, and none may dismiss the modal either.
func TestMergerConfirmMouseAction_InertRegions(t *testing.T) {
	app, overlay, startCol, startRow, contentWidth := mergerConfirmMouseEnv(t, 80, 40, 2)
	_, hintRow := screenCellOf(t, overlay, "enter/y confirm")
	_, srcRow := screenCellOf(t, overlay, "Source:")
	_, acctRow := screenCellOf(t, overlay, "Brokerage 1")
	_, mergeRow := screenCellOf(t, overlay, "[ Merge ]")

	for _, tc := range []struct {
		name string
		x, y int
	}{
		{"the keyboard hint row", startCol + 10, startRow + hintRow},
		{"the summary body", startCol + 10, startRow + srcRow},
		{"an affected account row", startCol + 10, startRow + acctRow},
		{"the button row's leading gap", startCol + 3, startRow + mergeRow},
		{"the top-left border cell", startCol, startRow},
		{"one column left of the panel", startCol - 1, startRow + 2},
		{"one column right of the panel", startCol + 3 + contentWidth, startRow + 2},
		{"one row above the panel", startCol + 10, startRow - 1},
		{"far outside", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.mergerConfirmMouseAction(tc.x, tc.y); got != dialog.DialogActionNone {
				t.Errorf("action = %v, want none", got)
			}
		})
	}
}

// TestMergerConfirmMouseAction_NilData guards the nil path.
func TestMergerConfirmMouseAction_NilData(t *testing.T) {
	app := &App{styles: widget.NewStyles(), width: 80, height: 40}
	if got := app.mergerConfirmMouseAction(40, 20); got != dialog.DialogActionNone {
		t.Errorf("action = %v, want none", got)
	}
}

// TestMergerConfirm_MouseClickOnMerge_Executes is the wiring. A non-nil command
// is the proof the merger ran: both exits clear the state, and only
// executeMerger returns a command.
func TestMergerConfirm_MouseClickOnMerge_Executes(t *testing.T) {
	app, overlay, startCol, startRow, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
	x, y := screenCellOf(t, overlay, "[ Merge ]")

	model, cmd := app.handleMouseEvent(tea.MouseClickMsg{X: startCol + x + 2, Y: startRow + y, Button: tea.MouseLeft})
	app = model.(*App)

	if app.mergerConfirmData != nil || app.mergerConfirmParams != nil {
		t.Error("confirmation state should be cleared after Merge")
	}
	if cmd == nil {
		t.Error("clicking Merge produced no command, so the merger did not run")
	}
}

// TestMergerConfirm_MouseCancelPathsDoNotMerge is the destructive-safety test.
// cmd == nil is load-bearing: closeMergerConfirmation and executeMerger both
// clear the state, so state alone cannot tell them apart.
func TestMergerConfirm_MouseCancelPathsDoNotMerge(t *testing.T) {
	for _, label := range []string{"[ Cancel ]", "[x]"} {
		t.Run(label, func(t *testing.T) {
			app, overlay, startCol, startRow, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
			x, y := screenCellOf(t, overlay, label)

			model, cmd := app.handleMouseEvent(tea.MouseClickMsg{X: startCol + x + 1, Y: startRow + y, Button: tea.MouseLeft})
			app = model.(*App)

			if app.mergerConfirmData != nil {
				t.Error("the overlay should have closed")
			}
			if cmd != nil {
				t.Error("a cancel path returned a command — it executed the merger")
			}
		})
	}
}

// TestMergerConfirm_StrayClickNeitherMergesNorDismisses pins that the modal
// stays put and nothing runs.
func TestMergerConfirm_StrayClickNeitherMergesNorDismisses(t *testing.T) {
	app, overlay, startCol, startRow, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
	_, hintRow := screenCellOf(t, overlay, "enter/y confirm")

	for _, tc := range []struct {
		name string
		x, y int
	}{
		{"the hint row", startCol + 10, startRow + hintRow},
		{"outside the panel", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, cmd := app.handleMouseEvent(tea.MouseClickMsg{X: tc.x, Y: tc.y, Button: tea.MouseLeft})
			got := model.(*App)
			if got.mergerConfirmData == nil || got.mergerConfirmParams == nil {
				t.Error("a stray click dismissed the confirmation")
			}
			if cmd != nil {
				t.Error("a stray click executed the merger")
			}
		})
	}
}

// TestMergerConfirm_WheelIsInert checks the wheel cannot merge.
func TestMergerConfirm_WheelIsInert(t *testing.T) {
	app, overlay, startCol, startRow, _ := mergerConfirmMouseEnv(t, 80, 40, 1)
	_, mergeRow := screenCellOf(t, overlay, "[ Merge ]")

	model, cmd := app.handleMouseEvent(tea.MouseWheelMsg{X: startCol + 20, Y: startRow + mergeRow, Button: tea.MouseWheelDown})
	app = model.(*App)

	if app.mergerConfirmData == nil {
		t.Error("the wheel dismissed the confirmation")
	}
	if cmd != nil {
		t.Error("the wheel executed the merger")
	}
}

// TestMergerConfirm_ClippedOverlayHasNoReachableMerge is the fail-safe: when the
// panel outgrows the terminal the Merge button is off screen, and no click on
// any visible cell may reach it.
func TestMergerConfirm_ClippedOverlayHasNoReachableMerge(t *testing.T) {
	app, _, _, _, _ := mergerConfirmMouseEnv(t, 120, 12, 20)
	for y := 0; y < app.height; y++ {
		for x := 0; x < app.width; x++ {
			if got := app.mergerConfirmMouseAction(x, y); got == dialog.DialogActionSubmit {
				t.Fatalf("a click at (%d,%d) can merge on a clipped overlay", x, y)
			}
		}
	}
}
