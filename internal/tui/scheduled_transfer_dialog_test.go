package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildNonInvestmentAccountOptions_ExcludesInvestment(t *testing.T) {
	accts := []*account.Account{
		account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("Visa", account.TypeCreditCard, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("HSA", account.TypeHSA, "USD", types.ZeroMoney, types.Today()),
		account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today()),
	}

	options, ids := buildNonInvestmentAccountOptions(accts)
	if len(options) != 3 || len(ids) != 3 {
		t.Fatalf("expected 3 non-investment accounts, got options=%v", options)
	}
	for _, name := range options {
		if name == "401k" || name == "HSA" {
			t.Errorf("investment account %q should be excluded", name)
		}
	}
}

func TestBuildNewScheduledTransferDialog_FieldLayout(t *testing.T) {
	d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"})
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
	// To should default to a different index than From.
	if fields[schedXferFieldTo].SelectedIndex == fields[schedXferFieldFrom].SelectedIndex {
		t.Error("To should default to a different account than From")
	}
}

func TestBuildEditScheduledTransferDialog_Prefills(t *testing.T) {
	from := types.NewID()
	to := types.NewID()
	accountIDs := []types.ID{from, to}
	options := []string{"Checking", "Visa"}

	st := scheduled.NewTransactionWithAmount(from, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(to)

	d := buildEditScheduledTransferDialog(st, options, accountIDs)
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
			d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"})
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
			d := buildNewScheduledTransferDialog([]string{"Checking", "Visa"})
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
