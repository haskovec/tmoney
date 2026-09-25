package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// newLedgerMoveApp wires the account and transfer services over a fresh file
// with one invested HSA holding the given investment rows, and returns the
// app plus the account.
func newLedgerMoveApp(t *testing.T, rows ...*investment.Transaction) (*App, *account.Account, *investment.Repository, *transaction.Repository) {
	t.Helper()
	database := dbtest.New(t)
	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	invRepo := investment.NewRepository(database)

	acct := account.NewAccount("Cedar Bank HSA", account.TypeHSAInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := accountRepo.Create(acct); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		r.AccountID = acct.ID
		if err := invRepo.Create(r); err != nil {
			t.Fatal(err)
		}
	}

	transferSvc := transfer.NewService(txnRepo, invRepo, transaction.NewSplitRepository(database),
		accountRepo, category.NewRepository(database), database)
	a := &App{
		db: database,
		services: app.Services{
			Account:  account.NewService(accountRepo, database),
			Transfer: transferSvc,
		},
		undoManager: undo.NewManager(),
		acct: acctSurface{
			modalSurface: modalSurface{dlg: func() *dialog.Dialog {
				d := buildEditAccountDialog(acct)
				d.Fields()[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeHSA)
				return d
			}()},
			data: &accountDialogData{mode: accountDialogModeEdit, account: acct},
		},
	}
	return a, acct, invRepo, txnRepo
}

func invRow(kind investment.TransactionType, amount string) *investment.Transaction {
	return investment.NewTransaction(types.NilID, types.MustParseDate("2024-03-01"), kind, types.MustNewMoney(amount))
}

func TestSubmitAccountDialog_LedgerMove_OpensConfirmAndMovesOnYes(t *testing.T) {
	a, acct, invRepo, txnRepo := newLedgerMoveApp(t,
		invRow(investment.TransactionTypeDeposit, "250.00"),
		invRow(investment.TransactionTypeWithdrawal, "-180.25"),
	)
	// Something to prove the undo stack is cleared.
	a.undoManager.Push(undo.NewEditAccountCommand(a.services.Account, acct))

	model, cmd := a.submitAccountDialog()
	a = model.(*App)
	if cmd != nil {
		t.Fatal("a ledger move must wait for confirmation, not save at once")
	}
	if a.acct.dlg != nil {
		t.Error("edit dialog should be closed while the confirm dialog is up")
	}
	if !a.confirm.IsVisible() {
		t.Fatal("confirm dialog not shown")
	}

	// Yes.
	_, cmd = a.confirmDialogAction(dialog.DialogActionSubmit)
	if cmd == nil {
		t.Fatal("confirm should return the save command")
	}
	saved, ok := cmd().(ledgerMoveSavedMsg)
	if !ok {
		t.Fatalf("expected ledgerMoveSavedMsg")
	}

	// The pre-move backup holds the state before the move.
	assertPreMoveBackup(t, saved.backupPath, acct.ID, 2)

	got, err := a.services.Account.GetByID(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != account.TypeHSA {
		t.Errorf("type = %s, want hsa", got.Type)
	}
	rows, _ := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
	reg, _ := txnRepo.CountByAccount(acct.ID)
	if len(rows) != 0 || reg != 2 {
		t.Errorf("rows after move: inv %d reg %d", len(rows), reg)
	}
	if a.undoManager.CanUndo() {
		t.Error("undo stack should be cleared after a one-way ledger move")
	}
}

func TestSubmitAccountDialog_LedgerMove_NoOnConfirmWritesNothing(t *testing.T) {
	a, acct, invRepo, _ := newLedgerMoveApp(t, invRow(investment.TransactionTypeDeposit, "250.00"))

	model, _ := a.submitAccountDialog()
	a = model.(*App)
	if !a.confirm.IsVisible() {
		t.Fatal("confirm dialog not shown")
	}
	_, cmd := a.confirmDialogAction(dialog.DialogActionCancel)
	if cmd != nil {
		t.Error("cancel should not return a command")
	}
	got, _ := a.services.Account.GetByID(acct.ID)
	rows, _ := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
	if got.Type != account.TypeHSAInvestment || len(rows) != 1 {
		t.Errorf("cancel changed data: type %s rows %d", got.Type, len(rows))
	}
}

func TestSubmitAccountDialog_LedgerMove_RefusalLandsOnTypeField(t *testing.T) {
	buy := invRow(investment.TransactionTypeBuy, "-250.00")
	buy.SecurityID = types.NullableID{ID: types.NewID(), Valid: true}
	buy.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
	buy.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("25.00"), Valid: true}
	a, _, _, _ := newLedgerMoveApp(t, buy)

	model, cmd := a.submitAccountDialog()
	a = model.(*App)
	if cmd != nil {
		t.Error("a refused move must not save")
	}
	if a.acct.dlg == nil {
		t.Fatal("dialog should stay open on refusal")
	}
	if a.acct.dlg.Fields()[acctFieldType].Error == "" {
		t.Error("refusal reason should land on the Type field")
	}
	if a.confirm.IsVisible() {
		t.Error("no confirm dialog on refusal")
	}
}

// assertPreMoveBackup opens the backup file and checks that the account is
// still an investment account with its rows in the investment ledger.
func assertPreMoveBackup(t *testing.T, path string, acctID types.ID, invRows int) {
	t.Helper()
	if path == "" || !strings.Contains(path, ".manual-backup.") {
		t.Fatalf("backup path = %q, want a manual backup", path)
	}
	bk, err := db.Open(path)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = bk.Close() }()
	got, err := account.NewRepository(bk).GetByID(acctID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := investment.NewRepository(bk).ListByAccount(acctID, investment.TransactionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != account.TypeHSAInvestment || len(rows) != invRows {
		t.Errorf("backup holds type %s with %d investment rows, want hsa_investment with %d", got.Type, len(rows), invRows)
	}
}

// TestSubmitAccountDialog_EmptyLotTrackedAccountChangesType pins that an
// empty lot-tracked investment account can change to a register type from
// the dialog, which has no Track Lots control.
func TestSubmitAccountDialog_EmptyLotTrackedAccountChangesType(t *testing.T) {
	a, acct, _, _ := newLedgerMoveApp(t)
	acct.TrackLots = true
	if err := a.services.Account.Update(acct); err != nil {
		t.Fatal(err)
	}

	_, cmd := a.submitAccountDialog()
	if cmd == nil {
		t.Fatal("an empty account needs no confirmation; expected the save command")
	}
	if msg, ok := cmd().(errMsg); ok {
		t.Fatalf("save failed: %v", msg.err)
	}
	got, err := a.services.Account.GetByID(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != account.TypeHSA || got.TrackLots {
		t.Errorf("after save: type %s trackLots %v, want hsa and false", got.Type, got.TrackLots)
	}
}

// TestSubmitAccountDialog_LedgerMove_DuplicateNameStopsBeforeConfirm pins
// that a name clash is reported in the dialog, before any backup or move.
func TestSubmitAccountDialog_LedgerMove_DuplicateNameStopsBeforeConfirm(t *testing.T) {
	a, acct, invRepo, _ := newLedgerMoveApp(t, invRow(investment.TransactionTypeDeposit, "250.00"))
	other := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := a.services.Account.Create(other); err != nil {
		t.Fatal(err)
	}
	a.acct.dlg.Fields()[acctFieldName].Value = "Checking"

	model, cmd := a.submitAccountDialog()
	a = model.(*App)
	if cmd != nil || a.confirm.IsVisible() {
		t.Fatal("a duplicate name must stop before the confirm dialog")
	}
	if a.acct.dlg == nil || a.acct.dlg.Fields()[acctFieldName].Error == "" {
		t.Error("the name field should carry the error")
	}
	rows, _ := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
	if len(rows) != 1 {
		t.Errorf("rows moved: %d left", len(rows))
	}
}

// TestSubmitAccountDialog_LedgerMove_InvalidFieldStopsBeforeConfirm pins
// that a field the account cannot store is reported before the confirm.
func TestSubmitAccountDialog_LedgerMove_InvalidFieldStopsBeforeConfirm(t *testing.T) {
	a, _, _, _ := newLedgerMoveApp(t, invRow(investment.TransactionTypeDeposit, "250.00"))
	a.acct.dlg.Fields()[acctFieldNotes].Value = strings.Repeat("x", 2001)

	model, cmd := a.submitAccountDialog()
	a = model.(*App)
	if cmd != nil || a.confirm.IsVisible() {
		t.Fatal("an invalid field must stop before the confirm dialog")
	}
	if a.acct.dlg == nil || a.acct.dlg.ErrorMsg() == "" {
		t.Error("the dialog should show the validation error")
	}
}

// TestSubmitAccountDialog_LedgerMove_FailedCommitChangesNothing pins that a
// move refused at commit time leaves the rows, the type and the undo history
// as they were.
func TestSubmitAccountDialog_LedgerMove_FailedCommitChangesNothing(t *testing.T) {
	a, acct, invRepo, txnRepo := newLedgerMoveApp(t, invRow(investment.TransactionTypeDeposit, "250.00"))
	a.undoManager.Push(undo.NewEditAccountCommand(a.services.Account, acct))

	model, _ := a.submitAccountDialog()
	a = model.(*App)
	// A row added between the plan and the confirm makes the plan stale.
	late := invRow(investment.TransactionTypeDeposit, "5.00")
	late.AccountID = acct.ID
	if err := invRepo.Create(late); err != nil {
		t.Fatal(err)
	}
	_, cmd := a.confirmDialogAction(dialog.DialogActionSubmit)
	if _, ok := cmd().(errMsg); !ok {
		t.Fatal("expected an error for the stale plan")
	}
	got, _ := a.services.Account.GetByID(acct.ID)
	rows, _ := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
	reg, _ := txnRepo.CountByAccount(acct.ID)
	if got.Type != account.TypeHSAInvestment || len(rows) != 2 || reg != 0 {
		t.Errorf("a failed commit changed data: type %s inv %d reg %d", got.Type, len(rows), reg)
	}
	if !a.undoManager.CanUndo() {
		t.Error("undo history should stay when nothing moved")
	}
}
