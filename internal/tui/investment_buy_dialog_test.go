package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildSecurityOptions(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)

	options, ids := buildSecurityOptions([]*security.Security{sec1, sec2})

	if len(options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(options))
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	// Should be sorted by ticker
	if !strings.HasPrefix(options[0], "AAPL") {
		t.Errorf("first option = %q, want AAPL prefix", options[0])
	}
	if !strings.HasPrefix(options[1], "MSFT") {
		t.Errorf("second option = %q, want MSFT prefix", options[1])
	}
}

func TestBuildSecurityOptions_ExcludesHidden(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)
	sec2.Hidden = true

	options, ids := buildSecurityOptions([]*security.Security{sec1, sec2})

	if len(options) != 1 {
		t.Fatalf("expected 1 option (hidden excluded), got %d", len(options))
	}
	if !strings.HasPrefix(options[0], "AAPL") {
		t.Errorf("option = %q, want AAPL prefix", options[0])
	}
	if ids[0] != sec1.ID {
		t.Errorf("id mismatch: got %v, want %v", ids[0], sec1.ID)
	}
}

func TestBuildSecurityOptions_Empty(t *testing.T) {
	options, ids := buildSecurityOptions([]*security.Security{})

	if len(options) != 0 {
		t.Errorf("expected 0 options, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids, got %d", len(ids))
	}
}

func TestBuildSecurityOptions_SortedByTicker(t *testing.T) {
	sec1 := security.NewSecurity("TSLA", "Tesla Inc.", security.TypeStock)
	sec2 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec3 := security.NewSecurity("GOOG", "Alphabet Inc.", security.TypeStock)

	options, _ := buildSecurityOptions([]*security.Security{sec1, sec2, sec3})

	if len(options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(options))
	}
	if !strings.HasPrefix(options[0], "AAPL") {
		t.Errorf("first = %q, want AAPL", options[0])
	}
	if !strings.HasPrefix(options[1], "GOOG") {
		t.Errorf("second = %q, want GOOG", options[1])
	}
	if !strings.HasPrefix(options[2], "TSLA") {
		t.Errorf("third = %q, want TSLA", options[2])
	}
}

func TestBuildSecurityOptions_Format(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	options, _ := buildSecurityOptions([]*security.Security{sec})

	if options[0] != "AAPL - Apple Inc." {
		t.Errorf("option format = %q, want %q", options[0], "AAPL - Apple Inc.")
	}
}

func TestBuildBuyDialog_NewTransaction(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildBuyDialog(options, nil, ids)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	if len(fields) != 7 {
		t.Fatalf("expected 7 fields, got %d", len(fields))
	}

	// Field 0: Security (select)
	if fields[0].Type != FieldSelect {
		t.Errorf("field 0 type = %d, want FieldSelect (%d)", fields[0].Type, FieldSelect)
	}
	if fields[0].Label != "Security" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Security")
	}
	if len(fields[0].Options) != 2 {
		t.Errorf("expected 2 security options, got %d", len(fields[0].Options))
	}

	// Field 1: Date (masked, required, default today)
	if fields[1].Type != FieldDate {
		t.Errorf("field 1 type = %d, want FieldDate (%d)", fields[1].Type, FieldDate)
	}
	if fields[1].Label != "Date" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Date")
	}
	if !fields[1].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[1].Value != today {
		t.Errorf("date default = %q, want %q", fields[1].Value, today)
	}

	// Field 2: Shares (text, required)
	if fields[2].Type != FieldText {
		t.Errorf("field 2 type = %d, want FieldText", fields[2].Type)
	}
	if fields[2].Label != "Shares" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Shares")
	}
	if !fields[2].Required {
		t.Error("shares field should be required")
	}
	if fields[2].Value != "" {
		t.Errorf("shares default = %q, want empty for new", fields[2].Value)
	}

	// Field 3: Price/Share (text)
	if fields[3].Label != "Price/Share" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Price/Share")
	}

	// Field 4: Total (text)
	if fields[4].Label != "Total" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Total")
	}

	// Field 5: Commission (text)
	if fields[5].Label != "Commission" {
		t.Errorf("field 5 label = %q, want %q", fields[5].Label, "Commission")
	}

	// Field 6: Memo (text)
	if fields[6].Label != "Memo" {
		t.Errorf("field 6 label = %q, want %q", fields[6].Label, "Memo")
	}
}

// TestBuildBuyDialog_DateFieldMaskedOverwrite verifies the buy dialog's
// Date field uses overwrite-style masked input — typing a digit replaces
// the digit at the cursor and auto-advances over the slash.
func TestBuildBuyDialog_DateFieldMaskedOverwrite(t *testing.T) {
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)
	txn := investment.NewTransactionWithSecurity(
		types.NewID(), date, investment.TransactionTypeBuy,
		types.MustNewMoney("-1850.00"), secID, types.MustNewQuantity("10"),
	)
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{secID}

	d := buildBuyDialog(options, txn, ids)
	d.SetFocusIndex(1) // focus the Date field

	// Type "0" then "5" — overwrites "03" with "05", cursor advances skipping the slash.
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})

	if d.Fields()[1].Value != "05/15/2024" {
		t.Errorf("Value = %q, want %q (overwrite + skip slash)", d.Fields()[1].Value, "05/15/2024")
	}
}

func TestBuildBuyDialog_EditTransaction(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy,
		types.MustNewMoney("-1850.00"), secID, types.MustNewQuantity("10"),
	)
	txn.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("185.00"), Valid: true}
	txn.Commission = types.NullableMoney{Money: types.MustNewMoney("4.95"), Valid: true}
	txn.Memo = types.NullableString{String: "Buy Apple", Valid: true}

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID, types.NewID()}

	d := buildBuyDialog(options, txn, ids)
	fields := d.Fields()

	// Security should be pre-selected to the matching security
	if fields[0].SelectedIndex != 0 {
		t.Errorf("security selected index = %d, want 0", fields[0].SelectedIndex)
	}

	// Date should match the transaction date
	if fields[1].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[1].Value, "03/15/2024")
	}

	// Shares
	if fields[2].Value != "10" {
		t.Errorf("shares = %q, want %q", fields[2].Value, "10")
	}

	// Price/Share
	if fields[3].Value != "185.00" {
		t.Errorf("price = %q, want %q", fields[3].Value, "185.00")
	}

	// Total (negative stored, displayed as positive)
	if fields[4].Value != "1850.00" {
		t.Errorf("total = %q, want %q", fields[4].Value, "1850.00")
	}

	// Commission
	if fields[5].Value != "4.95" {
		t.Errorf("commission = %q, want %q", fields[5].Value, "4.95")
	}

	// Memo
	if fields[6].Value != "Buy Apple" {
		t.Errorf("memo = %q, want %q", fields[6].Value, "Buy Apple")
	}
}

func TestBuildBuyDialog_EditPreSelectsCorrectSecurity(t *testing.T) {
	secID1 := types.NewID()
	secID2 := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	// Transaction is for secID2
	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy,
		types.MustNewMoney("-500.00"), secID2, types.MustNewQuantity("5"),
	)

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID1, secID2}

	d := buildBuyDialog(options, txn, ids)
	fields := d.Fields()

	if fields[0].SelectedIndex != 1 {
		t.Errorf("security selected index = %d, want 1 (MSFT)", fields[0].SelectedIndex)
	}
}

func TestParseSharesInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid integer", "10", "10", false},
		{"valid decimal", "10.5", "10.5", false},
		{"valid small", "0.001", "0.001", false},
		{"zero", "0", "", true},
		{"negative", "-5", "", true},
		{"empty", "", "", true},
		{"spaces", "  10  ", "10", false},
		{"invalid", "abc", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := parseSharesInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.String() != tt.want {
				t.Errorf("quantity = %q, want %q", q.String(), tt.want)
			}
		})
	}
}

func TestParseOptionalMoneyInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		want    string
		wantErr bool
	}{
		{"empty", "", true, "", false},
		{"spaces", "   ", true, "", false},
		{"valid", "185.00", false, "185", false},
		{"with dollar", "$185.00", false, "185", false},
		{"invalid", "abc", false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseOptionalMoneyInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if m != nil {
					t.Errorf("expected nil, got %v", m)
				}
				return
			}
			if m == nil {
				t.Fatal("expected non-nil money")
			}
			if m.String() != tt.want {
				t.Errorf("money = %q, want %q", m.String(), tt.want)
			}
		})
	}
}

func TestSubmitBuyDialog_ValidationErrors(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		buyDialog: buildBuyDialog([]string{"AAPL - Apple Inc."}, nil, secIDs),
		buyDialogData: &buyDialogData{
			securities: []*security.Security{},
		},
		buyDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.buyDialog.Fields()
	fields[1].Value = "not-a-date" // invalid date
	fields[2].Value = ""           // empty shares
	fields[3].Value = ""           // no price
	fields[4].Value = ""           // no total

	model, cmd := app.submitBuyDialog()
	updatedApp := model.(*App)

	// Should not close dialog when errors exist
	if updatedApp.buyDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	// Check field errors
	fields = updatedApp.buyDialog.Fields()
	if fields[1].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("shares field should have error")
	}
	if fields[3].Error == "" {
		t.Error("price field should have error when both price and total are empty")
	}
	if fields[4].Error == "" {
		t.Error("total field should have error when both price and total are empty")
	}
}

func TestSubmitBuyDialog_ValidWithPricePerShare(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData: &buyDialogData{
			securities: []*security.Security{},
		},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024" // date
	fields[2].Value = "10"         // shares
	fields[3].Value = "185.00"     // price per share

	model, cmd := app.submitBuyDialog()
	updatedApp := model.(*App)

	// Should close dialog on valid submit
	if updatedApp.buyDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitBuyDialog_ValidWithTotal(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData: &buyDialogData{
			securities: []*security.Security{},
		},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "06/01/2024" // date
	fields[2].Value = "5"          // shares
	fields[4].Value = "925.00"     // total amount

	model, cmd := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitBuyDialog_NoSecurities(t *testing.T) {
	app := &App{
		buyDialog: buildBuyDialog([]string{}, nil, []types.ID{}),
		buyDialogData: &buyDialogData{
			securities: []*security.Security{},
		},
		buyDialogSecurityIDs: []types.ID{},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "185.00"

	model, _ := app.submitBuyDialog()
	updatedApp := model.(*App)

	// Should stay open with security error
	if updatedApp.buyDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.buyDialog.Fields()
	if fields[0].Error == "" {
		t.Error("security field should have error when no securities available")
	}
}

func TestSubmitBuyDialog_InvalidCommission(t *testing.T) {
	secID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "185.00"
	fields[5].Value = "not-a-number" // invalid commission

	model, _ := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog == nil {
		t.Error("dialog should remain open on commission error")
	}
	fields = updatedApp.buyDialog.Fields()
	if fields[5].Error == "" {
		t.Error("commission field should have error")
	}
}

func TestHandleBuyDialogKey_Cancel(t *testing.T) {
	secID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleBuyDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.buyDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.buyDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.buyDialogSecurityIDs != nil {
		t.Error("dialog security IDs should be cleared after cancel")
	}
}

func TestHandleBuyDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleBuyDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitBuyDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitBuyDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseBuyDialog(t *testing.T) {
	secID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
	}

	app.closeBuyDialog()

	if app.buyDialog != nil {
		t.Error("buyDialog should be nil after close")
	}
	if app.buyDialogData != nil {
		t.Error("buyDialogData should be nil after close")
	}
	if app.buyDialogSecurityIDs != nil {
		t.Error("buyDialogSecurityIDs should be nil after close")
	}
}

func TestBuyDialogDataMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &buyDialogData{
		securities: []*security.Security{sec},
	}

	msg := buyDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
}

func TestBuyDialogSavedMsg(t *testing.T) {
	// Ensure the message type exists and is usable
	msg := buyDialogSavedMsg{}
	_ = msg
}

func TestSubmitBuyDialog_InvalidPrice(t *testing.T) {
	secID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "not-valid" // invalid price
	fields[4].Value = ""

	model, _ := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog == nil {
		t.Error("dialog should remain open on price error")
	}
	fields = updatedApp.buyDialog.Fields()
	if fields[3].Error == "" {
		t.Error("price field should have error")
	}
}

func TestSubmitBuyDialog_InvalidTotal(t *testing.T) {
	secID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = ""
	fields[4].Value = "not-valid" // invalid total

	model, _ := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog == nil {
		t.Error("dialog should remain open on total error")
	}
	fields = updatedApp.buyDialog.Fields()
	if fields[4].Error == "" {
		t.Error("total field should have error")
	}
}

func TestSubmitBuyDialog_WithCommissionAndMemo(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "185.00"
	fields[5].Value = "4.95"      // commission
	fields[6].Value = "Buy Apple" // memo

	model, cmd := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog != nil {
		t.Error("dialog should close on valid submit with commission and memo")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitBuyDialog_DollarSignInCommission(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		buyDialog: buildBuyDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		buyDialogData:        &buyDialogData{},
		buyDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.buyDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "$185.00" // dollar sign in price
	fields[5].Value = "$4.95"   // dollar sign in commission

	model, cmd := app.submitBuyDialog()
	updatedApp := model.(*App)

	if updatedApp.buyDialog != nil {
		t.Error("dialog should close (dollar signs stripped)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}
