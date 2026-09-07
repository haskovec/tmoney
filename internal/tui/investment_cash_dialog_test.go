package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// --- Build dialog tests ---

func TestBuildCashOperationDialog_NumericFields(t *testing.T) {
	d := buildCashOperationDialog("Deposit", nil)
	assertNumericFields(t, d, []string{"Amount"}, []string{"Memo"})
}

func TestBuildCashOperationDialog_Deposit(t *testing.T) {
	d := buildCashOperationDialog("Deposit", nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// dialog.Field 0: Date (masked, required, default today)
	if fields[0].Type != dialog.FieldDate {
		t.Errorf("field 0 type = %d, want dialog.FieldDate (%d)", fields[0].Type, dialog.FieldDate)
	}
	if fields[0].Label != "Date" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Date")
	}
	if !fields[0].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[0].Value != today {
		t.Errorf("date default = %q, want %q", fields[0].Value, today)
	}

	// dialog.Field 1: Amount (text, required)
	if fields[1].Type != dialog.FieldText {
		t.Errorf("field 1 type = %d, want dialog.FieldText", fields[1].Type)
	}
	if fields[1].Label != "Amount" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Amount")
	}
	if !fields[1].Required {
		t.Error("amount field should be required")
	}
	if fields[1].Value != "" {
		t.Errorf("amount default = %q, want empty for new", fields[1].Value)
	}

	// dialog.Field 2: Memo (text)
	if fields[2].Label != "Memo" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Memo")
	}
}

func TestBuildCashOperationDialog_Withdrawal(t *testing.T) {
	d := buildCashOperationDialog("Withdrawal", nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
}

func TestBuildCashOperationDialog_Fee(t *testing.T) {
	d := buildCashOperationDialog("Fee", nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
}

func TestBuildCashOperationDialog_Interest(t *testing.T) {
	d := buildCashOperationDialog("Interest", nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
}

func TestBuildCashOperationDialog_EditTransaction(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeDeposit,
		types.MustNewMoney("500.00"),
	)
	txn.Memo = types.NullableString{String: "Monthly deposit", Valid: true}

	d := buildCashOperationDialog("Deposit", txn)
	fields := d.Fields()

	// Date should match
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}

	// Amount
	if fields[1].Value != "500.00" {
		t.Errorf("amount = %q, want %q", fields[1].Value, "500.00")
	}

	// Memo
	if fields[2].Value != "Monthly deposit" {
		t.Errorf("memo = %q, want %q", fields[2].Value, "Monthly deposit")
	}
}

func TestBuildCashOperationDialog_EditNegativeAmount(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeWithdrawal,
		types.MustNewMoney("-250.00"),
	)

	d := buildCashOperationDialog("Withdrawal", txn)
	fields := d.Fields()

	// Negative amount should be displayed as positive
	if fields[1].Value != "250.00" {
		t.Errorf("amount = %q, want %q (negated)", fields[1].Value, "250.00")
	}
}

// --- Submit validation tests ---

func TestSubmitCashOperationDialog_ValidationErrors(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Deposit", nil)},
			opType: investment.TransactionTypeDeposit},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "not-a-date" // invalid date
	fields[1].Value = ""           // empty amount

	model, cmd := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.cashOperation.dlg.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[1].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitCashOperationDialog_InvalidAmount(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Deposit", nil)},
			opType: investment.TransactionTypeDeposit},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "03/15/2024"
	fields[1].Value = "not-valid" // invalid amount

	model, _ := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg == nil {
		t.Error("dialog should remain open on amount error")
	}
	fields = updatedApp.cashOperation.dlg.Fields()
	if fields[1].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitCashOperationDialog_ValidDeposit(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Deposit", nil)},
			opType: investment.TransactionTypeDeposit},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[1].Value = "500.00"     // amount

	model, cmd := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitCashOperationDialog_ValidWithMemo(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Withdrawal", nil)},
			opType: investment.TransactionTypeWithdrawal},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "03/15/2024"
	fields[1].Value = "250.00"
	fields[2].Value = "ATM withdrawal" // memo

	model, cmd := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg != nil {
		t.Error("dialog should close on valid submit with memo")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitCashOperationDialog_DollarSignInAmount(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Fee", nil)},
			opType: investment.TransactionTypeFee},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "06/01/2024"
	fields[1].Value = "$25.00" // dollar sign in amount

	model, cmd := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg != nil {
		t.Error("dialog should close (dollar sign stripped)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitCashOperationDialog_ValidInterest(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Interest", nil)},
			opType: investment.TransactionTypeInterest},

		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.cashOperation.dlg.Fields()
	fields[0].Value = "12/31/2024"
	fields[1].Value = "15.75"
	fields[2].Value = "Monthly interest"

	model, cmd := app.submitCashOperationDialog()
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg != nil {
		t.Error("dialog should close after valid interest submit")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

// --- Handle key tests ---

func TestHandleCashOperationDialogKey_Cancel(t *testing.T) {
	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Deposit", nil)},
			opType: investment.TransactionTypeDeposit},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleCashOperationDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.cashOperation.dlg != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.cashOperation.opType != "" {
		t.Error("cash operation type should be cleared after cancel")
	}
}

func TestHandleCashOperationDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleCashOperationDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Submit nil dialog tests ---

func TestSubmitCashOperationDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitCashOperationDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Close dialog tests ---

func TestCloseCashOperationDialog(t *testing.T) {
	app := &App{
		cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog("Deposit", nil)},
			opType: investment.TransactionTypeDeposit},
	}

	app.closeCashOperationDialog()

	if app.cashOperation.dlg != nil {
		t.Error("cashOperationDialog should be nil after close")
	}
	if app.cashOperation.opType != "" {
		t.Error("cashOperationType should be empty after close")
	}
}

// --- Message type tests ---

func TestCashOperationDialogSavedMsg(t *testing.T) {
	msg := cashOperationDialogSavedMsg{}
	_ = msg
}

// --- All four types produce correct title ---

func TestBuildCashOperationDialog_Titles(t *testing.T) {
	tests := []struct {
		title string
	}{
		{"Deposit"},
		{"Withdrawal"},
		{"Fee"},
		{"Interest"},
	}

	for _, tt := range tests {
		d := buildCashOperationDialog(tt.title, nil)
		if d == nil {
			t.Errorf("dialog for %s should not be nil", tt.title)
		}
		if !d.IsVisible() {
			t.Errorf("dialog for %s should be visible", tt.title)
		}
	}
}

// --- Each transaction type maps to correct service call ---

func TestCashOperationType_AllTypes(t *testing.T) {
	cashTypes := []investment.TransactionType{
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeFee,
		investment.TransactionTypeInterest,
	}

	for _, txnType := range cashTypes {
		acctID := types.NewID()
		app := &App{
			cashOperation: cashOperationSurface{modalSurface: modalSurface{dlg: buildCashOperationDialog(txnType.DisplayName(), nil)},
				opType: txnType},

			investmentRegister: &investmentRegisterData{
				account: &account.Account{
					BaseModel: types.BaseModel{ID: acctID},
					Type:      account.TypeInvestment,
				},
			},
		}

		fields := app.cashOperation.dlg.Fields()
		fields[0].Value = "03/15/2024"
		fields[1].Value = "100.00"

		model, cmd := app.submitCashOperationDialog()
		updatedApp := model.(*App)

		if updatedApp.cashOperation.dlg != nil {
			t.Errorf("dialog should be closed for type %s", txnType)
		}
		if cmd == nil {
			t.Errorf("should return command for type %s", txnType)
		}
	}
}

// The dialog closes before the async save runs, so the status-bar note must
// travel on the message. Reading opType in the saved arm named every deposit,
// withdrawal, fee and interest "Cash operation transaction saved".
func TestCashOperationSavedMsg_NoteNamesTheOperation(t *testing.T) {
	tests := []struct {
		opType investment.TransactionType
		want   string
	}{
		{investment.TransactionTypeDeposit, "Deposit transaction saved"},
		{investment.TransactionTypeWithdrawal, "Withdrawal transaction saved"},
		{investment.TransactionTypeFee, "Fee transaction saved"},
		{investment.TransactionTypeInterest, "Interest transaction saved"},
		{"", "Cash operation transaction saved"},
	}
	for _, tt := range tests {
		if got := cashOperationSavedNote(tt.opType); got != tt.want {
			t.Errorf("cashOperationSavedNote(%q) = %q, want %q", tt.opType, got, tt.want)
		}
	}

	// The arm must take the note off the message, not off the closed surface.
	app := &App{statusbar: widget.NewStatusBar()}
	app.Update(cashOperationDialogSavedMsg{note: "Deposit transaction saved"})
	notes := app.statusbar.Notifications()
	if len(notes) != 1 || notes[0].Text != "Deposit transaction saved" {
		t.Errorf("status bar = %+v, want one \"Deposit transaction saved\"", notes)
	}
}
