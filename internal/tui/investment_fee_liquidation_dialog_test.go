package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildFeeLiquidationDialog_NewTransaction(t *testing.T) {
	options := []string{"FXAIX - Fidelity 500 Index", "FCNTX - Fidelity Contrafund"}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildFeeLiquidationDialog(options, nil, ids)
	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	// Date, Security, Shares, Total, Price/Share, Commission, Memo = 7 fields (no lot fields).
	if len(fields) != 7 {
		t.Fatalf("expected 7 fields, got %d", len(fields))
	}
	if fields[0].Type != dialog.FieldDate || fields[0].Label != "Date" || !fields[0].Required {
		t.Errorf("field 0 should be a required Date field, got label=%q type=%d required=%v", fields[0].Label, fields[0].Type, fields[0].Required)
	}
	if fields[0].Value != time.Now().Format("01/02/2006") {
		t.Errorf("date default = %q, want today", fields[0].Value)
	}
	if fields[1].Type != dialog.FieldCombo || fields[1].Label != "Security" {
		t.Errorf("field 1 should be Security combo, got label=%q type=%d", fields[1].Label, fields[1].Type)
	}
	if len(fields[1].Options) != 2 {
		t.Errorf("expected 2 security options, got %d", len(fields[1].Options))
	}
	if fields[2].Label != "Shares" || !fields[2].Required {
		t.Errorf("field 2 should be required Shares, got label=%q required=%v", fields[2].Label, fields[2].Required)
	}
	for i, want := range []string{"Date", "Security", "Shares", "Total", "Price/Share", "Commission", "Memo"} {
		if fields[i].Label != want {
			t.Errorf("field %d label = %q, want %q", i, fields[i].Label, want)
		}
	}
}

func TestBuildFeeLiquidationDialog_NumericFields(t *testing.T) {
	d := buildFeeLiquidationDialog([]string{"FXAIX - Fidelity 500 Index"}, nil, []types.ID{types.NewID()})
	assertNumericFields(t, d,
		[]string{"Shares", "Total", "Price/Share", "Commission"},
		[]string{"Memo"},
	)
}

func TestBuildFeeLiquidationDialog_EditTransaction(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	options := []string{"FXAIX - Fidelity 500 Index", "FCNTX - Fidelity Contrafund"}
	ids := []types.ID{types.NewID(), secID} // editTxn's security is the 2nd option

	editTxn := investment.NewTransactionWithSecurity(acctID, types.NewDate(2026, time.June, 15),
		investment.TransactionTypeFeeLiquidation, types.MustNewMoney("5.00"), secID, types.MustNewQuantity("0.123"))
	editTxn.SetPricePerShare(types.MustNewMoney("40.65"))
	editTxn.SetMemo("Q2 recordkeeping fee")

	d := buildFeeLiquidationDialog(options, editTxn, ids)
	fields := d.Fields()
	if fields[0].Value != "06/15/2026" {
		t.Errorf("date = %q, want 06/15/2026", fields[0].Value)
	}
	if fields[1].SelectedIndex != 1 {
		t.Errorf("security SelectedIndex = %d, want 1 (pre-selected)", fields[1].SelectedIndex)
	}
	if fields[2].Value != "0.123" {
		t.Errorf("shares = %q, want 0.123", fields[2].Value)
	}
	if fields[3].Value != "5.00" {
		t.Errorf("total (fee) = %q, want 5.00", fields[3].Value)
	}
	if fields[4].Value != "40.65" {
		t.Errorf("price = %q, want 40.65", fields[4].Value)
	}
	if fields[6].Value != "Q2 recordkeeping fee" {
		t.Errorf("memo = %q, want %q", fields[6].Value, "Q2 recordkeeping fee")
	}
}

func feeLiqApp(t *testing.T, secID, acctID types.ID) *App {
	t.Helper()
	return &App{
		feeLiquidationDialog:            buildFeeLiquidationDialog([]string{"FXAIX - Fidelity 500 Index"}, nil, []types.ID{secID}),
		feeLiquidationDialogData:        &feeLiquidationDialogData{securities: []*security.Security{}},
		feeLiquidationDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Fidelity 401k",
				Type:      account.TypeInvestment,
				// TrackLots false → non-lot path, nil allocations, no lotRepo needed.
			},
		},
	}
}

func TestSubmitFeeLiquidationDialog_ValidationErrors(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "not-a-date"
	fields[2].Value = "" // empty shares
	fields[3].Value = "" // no total
	fields[4].Value = "" // no price

	model, cmd := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog == nil {
		t.Error("dialog should stay open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return a command on validation errors")
	}
	fields = updated.feeLiquidationDialog.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("shares field should have error")
	}
	if fields[3].Error == "" || fields[4].Error == "" {
		t.Error("total and price fields should error when both empty")
	}
}

func TestSubmitFeeLiquidationDialog_NoSecurities(t *testing.T) {
	app := &App{
		feeLiquidationDialog:            buildFeeLiquidationDialog([]string{}, nil, []types.ID{}),
		feeLiquidationDialogData:        &feeLiquidationDialogData{securities: []*security.Security{}},
		feeLiquidationDialogSecurityIDs: []types.ID{},
	}
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "06/15/2026"
	fields[2].Value = "0.123"
	fields[4].Value = "40.65"

	model, _ := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog == nil {
		t.Error("dialog should stay open when no securities")
	}
	if updated.feeLiquidationDialog.Fields()[1].Error == "" {
		t.Error("security field should error when no securities available")
	}
}

func TestSubmitFeeLiquidationDialog_ValidWithPricePerShare(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "06/15/2026"
	fields[2].Value = "0.123"
	fields[4].Value = "40.65"

	model, cmd := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog != nil {
		t.Error("dialog should close after a valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for the async save")
	}
}

func TestSubmitFeeLiquidationDialog_ValidWithTotal(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "06/15/2026"
	fields[2].Value = "0.123"
	fields[3].Value = "5.00" // total (fee), no price

	model, cmd := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog != nil {
		t.Error("dialog should close after a valid submit with total")
	}
	if cmd == nil {
		t.Error("should return a command for the async save")
	}
}

func TestSubmitFeeLiquidationDialog_DollarSignInCommission(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "06/15/2026"
	fields[2].Value = "0.123"
	fields[4].Value = "40.65"
	fields[5].Value = "$1.00" // $-prefixed commission should parse

	model, cmd := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog != nil {
		t.Error("dialog should close with a $-prefixed commission")
	}
	if cmd == nil {
		t.Error("should return a command for the async save")
	}
}

func TestSubmitFeeLiquidationDialog_InvalidCommission(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	fields := app.feeLiquidationDialog.Fields()
	fields[0].Value = "06/15/2026"
	fields[2].Value = "0.123"
	fields[4].Value = "40.65"
	fields[5].Value = "not-a-number"

	model, cmd := app.submitFeeLiquidationDialog()
	updated := model.(*App)
	if updated.feeLiquidationDialog == nil {
		t.Error("dialog should stay open on invalid commission")
	}
	if cmd != nil {
		t.Error("should not return a command on invalid commission")
	}
	if updated.feeLiquidationDialog.Fields()[5].Error == "" {
		t.Error("commission field should have error")
	}
}

func TestHandleFeeLiquidationDialogKey_Cancel(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	model, _ := app.handleFeeLiquidationDialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := model.(*App)
	if updated.feeLiquidationDialog != nil {
		t.Error("dialog should be nil after Escape")
	}
	if updated.feeLiquidationDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updated.feeLiquidationDialogSecurityIDs != nil {
		t.Error("security IDs should be cleared after cancel")
	}
}

func TestHandleFeeLiquidationDialogKey_NilDialog(t *testing.T) {
	app := &App{}
	model, cmd := app.handleFeeLiquidationDialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.(*App) != app || cmd != nil {
		t.Error("should no-op when dialog is nil")
	}
}

func TestSubmitFeeLiquidationDialog_NilDialog(t *testing.T) {
	app := &App{}
	model, cmd := app.submitFeeLiquidationDialog()
	if model.(*App) != app || cmd != nil {
		t.Error("should no-op when dialog is nil")
	}
}

func TestCloseFeeLiquidationDialog(t *testing.T) {
	app := feeLiqApp(t, types.NewID(), types.NewID())
	app.closeFeeLiquidationDialog()
	if app.feeLiquidationDialog != nil || app.feeLiquidationDialogData != nil || app.feeLiquidationDialogSecurityIDs != nil {
		t.Error("all fee-liquidation dialog state should be nil after close")
	}
}

func TestFeeLiquidationDialog_RendersInView(t *testing.T) {
	// Regression: the dialog must be wired into the View overlay (app_view.go)
	// and the modal-visibility check (app_helpers.go), not just the key router —
	// otherwise selecting "Fee via Liquidation" builds the dialog in state but
	// nothing ever appears on screen.
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
	app.feeLiquidationDialog = buildFeeLiquidationDialog([]string{"FXAIX - Fidelity 500 Index"}, nil, []types.ID{types.NewID()})

	if !app.isDialogVisible() {
		t.Error("isDialogVisible() must report the fee-liquidation dialog as open")
	}
	view := app.View()
	if !strings.Contains(view.Content, "Fee via Liquidation") {
		t.Errorf("rendered view should overlay the fee-liquidation dialog (title 'Fee via Liquidation'); got:\n%s", view.Content)
	}
}

func TestInvestmentTypeSelector_IncludesFeeLiquidation(t *testing.T) {
	options := investmentTransactionTypeOptions()
	idx := -1
	for i, o := range options {
		if o == investment.TransactionTypeFeeLiquidation.DisplayName() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("type selector options should include %q; got %v", investment.TransactionTypeFeeLiquidation.DisplayName(), options)
	}
	// The three selector helpers must agree on the ordinal so edit-mode
	// pre-selection lines up with the new-mode list.
	if got := investmentTransactionTypeFromIndex(idx); got != investment.TransactionTypeFeeLiquidation {
		t.Errorf("fromIndex(%d) = %v, want FeeLiquidation", idx, got)
	}
	if got := investmentTransactionTypeIndex(investment.TransactionTypeFeeLiquidation); got != idx {
		t.Errorf("typeIndex(FeeLiquidation) = %d, want %d", got, idx)
	}
}
