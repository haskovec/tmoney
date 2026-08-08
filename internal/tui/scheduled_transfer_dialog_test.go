package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// TestBuildTransferAccountOptions_IncludesInvestment replaces a test that
// asserted the opposite. Excluding investment accounts made sense only while
// scheduled transfers were regular-to-regular; posting now routes through the
// transfer owner, which writes all four kinds. Worse, the exclusion corrupted
// data: a CLI-created schedule targeting an investment account found neither
// endpoint in the picker, both combos fell back to the first entry, and saving
// silently re-pointed the transfer.
func TestBuildTransferAccountOptions_IncludesInvestment(t *testing.T) {
	accts := []*account.Account{
		account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("Visa", account.TypeCreditCard, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("HSA", account.TypeHSA, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today()),
	}

	options, ids := buildTransferAccountOptions(accts)
	if len(options) != len(accts) || len(ids) != len(accts) {
		t.Fatalf("expected all %d accounts, got options=%v", len(accts), options)
	}
	for _, want := range []string{"401k", "HSA"} {
		found := false
		for _, name := range options {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("investment account %q is missing; a schedule that uses it cannot be edited safely", want)
		}
	}
}

// TestIndexOfID_ReportsNotFound pins the other half. Returning 0 for an ID the
// list does not hold is indistinguishable from the user picking the first entry,
// which is exactly how the endpoints used to be re-pointed on save.
func TestIndexOfID_ReportsNotFound(t *testing.T) {
	ids := []types.ID{types.NewID(), types.NewID()}
	if got := indexOfID(ids, ids[1]); got != 1 {
		t.Errorf("indexOfID(present) = %d, want 1", got)
	}
	if got := indexOfID(ids, types.NewID()); got != -1 {
		t.Errorf("indexOfID(absent) = %d, want -1 — 0 would silently select the first account", got)
	}
}

// TestBuildEditScheduledTransferDialog_PreservesInvestmentEndpoints is the
// regression itself: open a schedule whose endpoints are both investment
// accounts and the pickers must point at THOSE accounts.
func TestBuildEditScheduledTransferDialog_PreservesInvestmentEndpoints(t *testing.T) {
	rollover := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	roth := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	accts := []*account.Account{checking, rollover, roth}
	options, ids := buildTransferAccountOptions(accts)

	st := scheduled.NewTransactionWithAmount(rollover.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-500.00"))
	st.SetTransfer(roth.ID)

	d := buildEditScheduledTransferDialog(st, options, []string{"(None)"}, ids, []types.ID{types.NilID})
	fields := d.Fields()
	if got := fields[schedXferFieldFrom].SelectedIndex; ids[got] != rollover.ID {
		t.Errorf("From points at %q, want Rollover IRA — saving would re-point the transfer", options[got])
	}
	if got := fields[schedXferFieldTo].SelectedIndex; ids[got] != roth.ID {
		t.Errorf("To points at %q, want Roth IRA — saving would re-point the transfer", options[got])
	}
}

func TestBuildNewScheduledTransferDialog_FieldLayout(t *testing.T) {
	d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"}, []string{"(None)", "Bills"})
	fields := d.Fields()
	if len(fields) != schedXferFieldCount {
		t.Fatalf("expected %d fields, got %d", schedXferFieldCount, len(fields))
	}
	if fields[schedXferFieldFrom].Label != "From" {
		t.Errorf("field 0 label = %q, want From", fields[schedXferFieldFrom].Label)
	}
	if fields[schedXferFieldTo].Label != "To" {
		t.Errorf("field 1 label = %q, want To", fields[schedXferFieldTo].Label)
	}
	if !fields[schedXferFieldAmount].Required {
		t.Error("Amount should be required")
	}
	if fields[schedXferFieldCategory].Label != "Category" {
		t.Errorf("category field label = %q, want Category", fields[schedXferFieldCategory].Label)
	}
	if fields[schedXferFieldCategory].AddNewLabel == "" {
		t.Error("Category combo should support inline creation ([+ Add new category…])")
	}
	if fields[schedXferFieldMemo].Label != "Memo" {
		t.Errorf("memo field label = %q, want Memo", fields[schedXferFieldMemo].Label)
	}
	// To should default to a different index than From.
	if fields[schedXferFieldTo].SelectedIndex == fields[schedXferFieldFrom].SelectedIndex {
		t.Error("To should default to a different account than From")
	}
}

func TestBuildEditScheduledTransferDialog_Prefills(t *testing.T) {
	from := types.NewID()
	to := types.NewID()
	catID := types.NewID()
	accountIDs := []types.ID{from, to}
	options := []string{"Checking", "Visa"}
	categoryOptions := []string{"(None)", "Bills"}
	categoryIDs := []types.ID{types.NilID, catID}

	st := scheduled.NewTransactionWithAmount(from, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(to)
	st.SetCategory(catID)

	d := buildEditScheduledTransferDialog(st, options, categoryOptions, accountIDs, categoryIDs)
	fields := d.Fields()
	if fields[schedXferFieldFrom].SelectedIndex != 0 {
		t.Errorf("From index = %d, want 0", fields[schedXferFieldFrom].SelectedIndex)
	}
	if fields[schedXferFieldTo].SelectedIndex != 1 {
		t.Errorf("To index = %d, want 1", fields[schedXferFieldTo].SelectedIndex)
	}
	// Amount renders as a positive magnitude (Money.String drops trailing zeros).
	if got := fields[schedXferFieldAmount].Value; got != "200" {
		t.Errorf("Amount = %q, want 200", got)
	}
	// Category combo seeds from the schedule's existing category.
	if got := fields[schedXferFieldCategory].SelectedIndex; got != 1 {
		t.Errorf("Category index = %d, want 1 (Bills)", got)
	}
}

func TestSubmitScheduledTransferDialog_SelfTransfer(t *testing.T) {
	id := types.NewID()
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"}, []string{"(None)"})
			d.Fields()[schedXferFieldFrom].SelectedIndex = 0
			d.Fields()[schedXferFieldTo].SelectedIndex = 0 // same as From
			d.Fields()[schedXferFieldAmount].Value = "200.00"
			return d
		}(),
		schedDialogData:       &scheduledDialogData{mode: scheduledDialogModeNew, isTransfer: true},
		schedDialogAccountIDs: []types.ID{id, types.NewID()},
	}

	_, cmd := app.submitScheduledTransferDialog()
	if cmd != nil {
		t.Error("self-transfer should not return a save cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
}

func TestSubmitScheduledTransferDialog_MissingAmount(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"}, []string{"(None)"})
			d.Fields()[schedXferFieldAmount].Value = ""
			return d
		}(),
		schedDialogData:       &scheduledDialogData{mode: scheduledDialogModeNew, isTransfer: true},
		schedDialogAccountIDs: []types.ID{types.NewID(), types.NewID()},
	}

	_, cmd := app.submitScheduledTransferDialog()
	if cmd != nil {
		t.Error("missing amount should not return a save cmd")
	}
	if app.schedDialog.Fields()[schedXferFieldAmount].Error == "" {
		t.Error("missing amount should set a field-level error")
	}
}

func TestSchedulePreviewDialog_TransferShape(t *testing.T) {
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	visa := account.NewAccount("Visa", account.TypeCreditCard, "USD", types.ZeroMoney, types.Today())
	accts := []*account.Account{checking, visa}

	st := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(visa.ID)

	p := NewSchedulePreviewDialog(st, accts, nil, nil, nil, nil)
	if p == nil {
		t.Fatal("preview should not be nil")
	}
	if !p.IsTransfer() {
		t.Fatal("preview should report IsTransfer for a transfer schedule")
	}
	if p.IsMultiLine() {
		t.Error("transfer preview should not be multi-line")
	}
	fields := p.HeaderDialog().Fields()
	if fields[previewXferFieldAmount].Value != "200" {
		t.Errorf("preview amount = %q, want 200", fields[previewXferFieldAmount].Value)
	}
}

func TestFormatScheduledRow_Transfer(t *testing.T) {
	checking := types.NewID()
	visa := types.NewID()
	st := scheduled.NewTransactionWithAmount(checking, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(visa)

	app := &App{
		scheduled: &scheduledViewData{
			allTxns:      []*scheduled.Transaction{st},
			payeeNames:   map[types.ID]string{},
			accountNames: map[types.ID]string{checking: "Checking", visa: "Visa"},
		},
	}

	row := app.formatScheduledRow(st, false)
	// Columns: status, date, payee, amount, freq, account, auto.
	if row[2] != "→ Visa" {
		t.Errorf("payee column = %q, want \"→ Visa\"", row[2])
	}
	if row[5] != "Checking" {
		t.Errorf("account column = %q, want Checking", row[5])
	}
}
