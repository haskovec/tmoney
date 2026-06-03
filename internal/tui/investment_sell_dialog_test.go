package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildSellDialog_NumericFields(t *testing.T) {
	lot := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}
	d := buildSellDialog([]string{"AAPL - Apple Inc."}, nil, []types.ID{types.NewID()}, []*investment.Lot{lot})
	assertNumericFields(t, d,
		[]string{"Shares", "Total", "Price/Share", "Commission"},
		[]string{"Memo"},
	)
}

func TestBuildSellDialog_NewTransaction_NonLotTracking(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildSellDialog(options, nil, ids, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	// Non-lot-tracking: Date, Security, Shares, Total, Price/Share, Commission, Memo = 7 fields
	if len(fields) != 7 {
		t.Fatalf("expected 7 fields for non-lot-tracking, got %d", len(fields))
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

	// dialog.Field 1: Security (typeahead combo)
	if fields[1].Type != dialog.FieldCombo {
		t.Errorf("field 1 type = %d, want dialog.FieldCombo (%d)", fields[1].Type, dialog.FieldCombo)
	}
	if fields[1].Label != "Security" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Security")
	}
	if len(fields[1].Options) != 2 {
		t.Errorf("expected 2 security options, got %d", len(fields[1].Options))
	}

	// dialog.Field 2: Shares (text, required)
	if fields[2].Label != "Shares" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Shares")
	}
	if !fields[2].Required {
		t.Error("shares field should be required")
	}

	// dialog.Field 3: Total
	if fields[3].Label != "Total" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Total")
	}

	// dialog.Field 4: Price/Share
	if fields[4].Label != "Price/Share" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Price/Share")
	}

	// dialog.Field 5: Commission
	if fields[5].Label != "Commission" {
		t.Errorf("field 5 label = %q, want %q", fields[5].Label, "Commission")
	}

	// dialog.Field 6: Memo
	if fields[6].Label != "Memo" {
		t.Errorf("field 6 label = %q, want %q", fields[6].Label, "Memo")
	}
}

func TestBuildSellDialog_WithLots(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{secID}

	lot1 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}
	lot2 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("50"),
		CostPerShare: types.MustNewMoney("175.00"),
		PurchaseDate: types.NewDate(2024, time.January, 10),
	}

	d := buildSellDialog(options, nil, ids, []*investment.Lot{lot1, lot2})

	fields := d.Fields()
	// With lots: Security, Date, Shares, Total, Price/Share, Commission, Memo + 2 lot fields = 9 fields
	if len(fields) != 9 {
		t.Fatalf("expected 9 fields with 2 lots, got %d", len(fields))
	}

	// Lot allocation fields should appear after Shares (index 3 and 4)
	if fields[3].Label == "" {
		t.Error("lot field 3 should have a label")
	}
	if fields[4].Label == "" {
		t.Error("lot field 4 should have a label")
	}

	// Verify lot labels contain purchase date and available shares info
	// Format: "Lot: MM/DD/YYYY - 100 shares @ $150.00"
	expectedLabel1 := "Lot: 06/15/2023 - 100 shares @ $150.00"
	if fields[3].Label != expectedLabel1 {
		t.Errorf("lot 1 label = %q, want %q", fields[3].Label, expectedLabel1)
	}
	expectedLabel2 := "Lot: 01/10/2024 - 50 shares @ $175.00"
	if fields[4].Label != expectedLabel2 {
		t.Errorf("lot 2 label = %q, want %q", fields[4].Label, expectedLabel2)
	}

	// Lot fields should default to empty
	if fields[3].Value != "" {
		t.Errorf("lot 1 value = %q, want empty", fields[3].Value)
	}
	if fields[4].Value != "" {
		t.Errorf("lot 2 value = %q, want empty", fields[4].Value)
	}
}

func TestBuildSellDialog_EditTransaction(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeSell,
		types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)
	txn.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("185.00"), Valid: true}
	txn.Commission = types.NullableMoney{Money: types.MustNewMoney("4.95"), Valid: true}
	txn.Memo = types.NullableString{String: "Sell Apple", Valid: true}

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID, types.NewID()}

	d := buildSellDialog(options, txn, ids, nil)
	fields := d.Fields()

	// Date
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}

	// Security should be pre-selected
	if fields[1].SelectedIndex != 0 {
		t.Errorf("security selected index = %d, want 0", fields[1].SelectedIndex)
	}

	// Shares
	if fields[2].Value != "10" {
		t.Errorf("shares = %q, want %q", fields[2].Value, "10")
	}

	// Total (sell total is positive, display as positive)
	if fields[3].Value != "1850.00" {
		t.Errorf("total = %q, want %q", fields[3].Value, "1850.00")
	}

	// Price/Share
	if fields[4].Value != "185.00" {
		t.Errorf("price = %q, want %q", fields[4].Value, "185.00")
	}

	// Commission
	if fields[5].Value != "4.95" {
		t.Errorf("commission = %q, want %q", fields[5].Value, "4.95")
	}

	// Memo
	if fields[6].Value != "Sell Apple" {
		t.Errorf("memo = %q, want %q", fields[6].Value, "Sell Apple")
	}
}

func TestBuildSellDialog_EditPreSelectsCorrectSecurity(t *testing.T) {
	secID1 := types.NewID()
	secID2 := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeSell,
		types.MustNewMoney("500.00"), secID2, types.MustNewQuantity("5"),
	)

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID1, secID2}

	d := buildSellDialog(options, txn, ids, nil)
	fields := d.Fields()

	if fields[1].SelectedIndex != 1 {
		t.Errorf("security selected index = %d, want 1 (MSFT)", fields[1].SelectedIndex)
	}
}

func TestSellDialogData_WithLots(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	lot := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}

	data := &sellDialogData{
		securities: []*security.Security{sec},
		lots:       []*investment.Lot{lot},
	}

	msg := sellDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
	if len(msg.data.lots) != 1 {
		t.Errorf("expected 1 lot, got %d", len(msg.data.lots))
	}
}

func TestSellDialogData_WithoutLots(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &sellDialogData{
		securities: []*security.Security{sec},
		lots:       nil,
	}

	if data.lots != nil {
		t.Error("lots should be nil for non-lot-tracking accounts")
	}
}

func TestSellDialogSavedMsg(t *testing.T) {
	msg := sellDialogSavedMsg{}
	_ = msg
}

func TestSubmitSellDialog_ValidationErrors(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		sellDialog: buildSellDialog([]string{"AAPL - Apple Inc."}, nil, secIDs, nil),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
		},
		sellDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.sellDialog.Fields()
	fields[0].Value = "not-a-date" // invalid date
	fields[2].Value = ""           // empty shares
	fields[3].Value = ""           // no total
	fields[4].Value = ""           // no price

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.sellDialog.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("shares field should have error")
	}
	if fields[3].Error == "" {
		t.Error("total field should have error when both price and total are empty")
	}
	if fields[4].Error == "" {
		t.Error("price field should have error when both price and total are empty")
	}
}

func TestSubmitSellDialog_NoSecurities(t *testing.T) {
	app := &App{
		sellDialog: buildSellDialog([]string{}, nil, []types.ID{}, nil),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
		},
		sellDialogSecurityIDs: []types.ID{},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[4].Value = "185.00"

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.sellDialog.Fields()
	if fields[1].Error == "" {
		t.Error("security field should have error when no securities available")
	}
}

func TestSubmitSellDialog_ValidWithPricePerShare(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
		},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[2].Value = "10"         // shares
	fields[4].Value = "185.00"     // price per share

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitSellDialog_ValidWithTotal(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
		},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "06/01/2024"
	fields[2].Value = "5"
	fields[3].Value = "925.00"

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitSellDialog_InvalidCommission(t *testing.T) {
	secID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[4].Value = "185.00"
	fields[5].Value = "not-a-number"

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open on commission error")
	}
	fields = updatedApp.sellDialog.Fields()
	if fields[5].Error == "" {
		t.Error("commission field should have error")
	}
}

func TestSubmitSellDialog_InvalidPrice(t *testing.T) {
	secID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = ""
	fields[4].Value = "not-valid"

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open on price error")
	}
	fields = updatedApp.sellDialog.Fields()
	if fields[4].Error == "" {
		t.Error("price field should have error")
	}
}

func TestSubmitSellDialog_InvalidTotal(t *testing.T) {
	secID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "not-valid"
	fields[4].Value = ""

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open on total error")
	}
	fields = updatedApp.sellDialog.Fields()
	if fields[3].Error == "" {
		t.Error("total field should have error")
	}
}

func TestSubmitSellDialog_WithCommissionAndMemo(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[4].Value = "185.00"
	fields[5].Value = "4.95"
	fields[6].Value = "Sell Apple"

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should close on valid submit with commission and memo")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitSellDialog_DollarSignInCommission(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[4].Value = "$185.00"
	fields[5].Value = "$4.95"

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should close (dollar signs stripped)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitSellDialog_WithLotAllocations(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	lot1 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}
	lot2 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("50"),
		CostPerShare: types.MustNewMoney("175.00"),
		PurchaseDate: types.NewDate(2024, time.January, 10),
	}

	lots := []*investment.Lot{lot1, lot2}

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			lots,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
			lots:       lots,
		},
		sellDialogSecurityIDs: []types.ID{secID},
		sellDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[2].Value = "30"         // total shares to sell
	fields[3].Value = "20"         // lot 1: sell 20 shares
	fields[4].Value = "10"         // lot 2: sell 10 shares
	fields[6].Value = "200.00"     // price per share
	// fields[5] = Total, fields[7] = Commission, fields[8] = Memo

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should be closed after valid submit with lot allocations")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitSellDialog_LotAllocationMismatch(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	lot1 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}

	lots := []*investment.Lot{lot1}

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			lots,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
			lots:       lots,
		},
		sellDialogSecurityIDs: []types.ID{secID},
		sellDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "30"     // selling 30 shares
	fields[3].Value = "20"     // lot 1 only 20 (mismatch: 20 != 30)
	fields[5].Value = "185.00" // price per share
	// fields[4] = Total, fields[6] = Commission, fields[7] = Memo

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open when lot allocations don't match total shares")
	}
}

func TestSubmitSellDialog_LotAllocationExceedsAvailable(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	lot1 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("10"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}

	lots := []*investment.Lot{lot1}

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			lots,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
			lots:       lots,
		},
		sellDialogSecurityIDs: []types.ID{secID},
		sellDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "20"     // selling 20 shares
	fields[3].Value = "20"     // lot 1 only has 10 available
	fields[5].Value = "185.00" // price per share
	// fields[4] = Total, fields[6] = Commission, fields[7] = Memo

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open when lot allocation exceeds available shares")
	}
}

func TestSubmitSellDialog_InvalidLotAllocation(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	lot1 := &investment.Lot{
		BaseModel:    types.NewBaseModel(),
		AccountID:    acctID,
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}

	lots := []*investment.Lot{lot1}

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			lots,
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
			lots:       lots,
		},
		sellDialogSecurityIDs: []types.ID{secID},
		sellDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "abc"    // invalid lot allocation
	fields[5].Value = "185.00" // price per share

	model, _ := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog == nil {
		t.Error("dialog should remain open on invalid lot allocation")
	}
	fields = updatedApp.sellDialog.Fields()
	if fields[3].Error == "" {
		t.Error("lot allocation field should have error for invalid input")
	}
}

func TestHandleSellDialogKey_Cancel(t *testing.T) {
	secID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleSellDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.sellDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.sellDialogSecurityIDs != nil {
		t.Error("dialog security IDs should be cleared after cancel")
	}
	if updatedApp.sellDialogLots != nil {
		t.Error("dialog lots should be cleared after cancel")
	}
}

func TestHandleSellDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleSellDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitSellDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitSellDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseSellDialog(t *testing.T) {
	secID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
			nil,
		),
		sellDialogData:        &sellDialogData{},
		sellDialogSecurityIDs: []types.ID{secID},
		sellDialogLots:        []*investment.Lot{},
	}

	app.closeSellDialog()

	if app.sellDialog != nil {
		t.Error("sellDialog should be nil after close")
	}
	if app.sellDialogData != nil {
		t.Error("sellDialogData should be nil after close")
	}
	if app.sellDialogSecurityIDs != nil {
		t.Error("sellDialogSecurityIDs should be nil after close")
	}
	if app.sellDialogLots != nil {
		t.Error("sellDialogLots should be nil after close")
	}
}

func TestBuildLotLabel(t *testing.T) {
	lot := &investment.Lot{
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}

	label := buildLotLabel(lot)
	expected := "Lot: 06/15/2023 - 100 shares @ $150.00"
	if label != expected {
		t.Errorf("lot label = %q, want %q", label, expected)
	}
}

func TestBuildLotLabel_SmallShares(t *testing.T) {
	lot := &investment.Lot{
		Shares:       types.MustNewQuantity("0.5"),
		CostPerShare: types.MustNewMoney("50000.00"),
		PurchaseDate: types.NewDate(2024, time.February, 29),
	}

	label := buildLotLabel(lot)
	expected := "Lot: 02/29/2024 - 0.5 shares @ $50000.00"
	if label != expected {
		t.Errorf("lot label = %q, want %q", label, expected)
	}
}

// fifoAllocate is the auto-allocation used for a new sell on a lot-tracked
// account (no per-lot fields are rendered). It must consume the oldest lots
// first, split the boundary lot, and error when the open lots fall short.
func TestFifoAllocate_ConsumesOldestFirst(t *testing.T) {
	lot1 := &investment.Lot{BaseModel: types.NewBaseModel(), Shares: types.MustNewQuantity("100")}
	lot2 := &investment.Lot{BaseModel: types.NewBaseModel(), Shares: types.MustNewQuantity("50")}
	// Caller passes lots in purchase-date order (oldest first = FIFO).
	openLots := []*investment.Lot{lot1, lot2}

	allocs, err := fifoAllocate(openLots, types.MustNewQuantity("120"))
	if err != nil {
		t.Fatalf("fifoAllocate: %v", err)
	}
	if len(allocs) != 2 {
		t.Fatalf("allocations = %d, want 2", len(allocs))
	}
	if allocs[0].LotID != lot1.ID || allocs[0].Shares.String() != "100" {
		t.Errorf("first allocation = %v %s, want lot1 100", allocs[0].LotID, allocs[0].Shares.String())
	}
	if allocs[1].LotID != lot2.ID || allocs[1].Shares.String() != "20" {
		t.Errorf("second allocation = %v %s, want lot2 20", allocs[1].LotID, allocs[1].Shares.String())
	}
}

func TestFifoAllocate_ExactSingleLot(t *testing.T) {
	lot1 := &investment.Lot{BaseModel: types.NewBaseModel(), Shares: types.MustNewQuantity("138")}
	allocs, err := fifoAllocate([]*investment.Lot{lot1}, types.MustNewQuantity("138"))
	if err != nil {
		t.Fatalf("fifoAllocate: %v", err)
	}
	if len(allocs) != 1 || allocs[0].Shares.String() != "138" {
		t.Fatalf("allocations = %v, want one allocation of 138", allocs)
	}
}

func TestFifoAllocate_Shortfall(t *testing.T) {
	lot1 := &investment.Lot{BaseModel: types.NewBaseModel(), Shares: types.MustNewQuantity("100")}
	_, err := fifoAllocate([]*investment.Lot{lot1}, types.MustNewQuantity("150"))
	if err == nil {
		t.Fatal("expected error when open lots don't cover requested shares")
	}
}

// Regression: a NEW sell on a lot-tracked account renders no per-lot fields
// (numLots == 0). submitSellDialog must not panic when no lot repo is wired —
// the FIFO auto-allocation branch is guarded and skipped — and the sale still
// submits. (The FIFO allocation math itself is covered by TestFifoAllocate_*.)
func TestSubmitSellDialog_NewSell_LotTracked_NoRepo_DoesNotPanic(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		sellDialog: buildSellDialog(
			[]string{"ETH - Ethereum"},
			nil,
			[]types.ID{secID},
			nil, // new sell: no lots -> no per-lot fields
		),
		sellDialogData: &sellDialogData{
			securities: []*security.Security{},
		},
		sellDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Wealthfront IRA",
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
		lotRepo: nil, // guarded: with no repo wired, the FIFO branch is skipped
	}

	fields := app.sellDialog.Fields()
	fields[0].Value = "07/23/2024" // date
	fields[2].Value = "138"        // shares
	fields[4].Value = "2500.00"    // price per share (no per-lot fields, so price is index 4)

	model, cmd := app.submitSellDialog()
	updatedApp := model.(*App)

	if updatedApp.sellDialog != nil {
		t.Error("dialog should close on a valid new lot-tracked sell")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}
