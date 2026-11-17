package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin Edit Schedule for a schedule whose account was closed after
// the schedule was made. The account pickers offer open accounts only, so the
// form used to open on the first open account and Save moved the schedule
// there without a word. The form must show where the schedule is, keep it
// there unless the user picks another account, and never move it onto a
// closed account.

// closedSchedEnv is the paycheck edit harness (a real DB behind the App) plus
// a closed checking account, "Old Bank".
type closedSchedEnv struct {
	*paycheckEditEnv
	oldBank *account.Account
}

func newClosedSchedEnv(t *testing.T) *closedSchedEnv {
	t.Helper()
	env := &closedSchedEnv{paycheckEditEnv: newPaycheckEditEnv(t)}
	env.oldBank = account.NewAccount("Old Bank", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := env.accountSvc.Create(env.oldBank); err != nil {
		t.Fatalf("create Old Bank: %v", err)
	}
	return env
}

// save creates st while every account is still open, then closes Old Bank: a
// schedule cannot be created against a closed account.
func (env *closedSchedEnv) save(t *testing.T, st *scheduled.Transaction) {
	t.Helper()
	if err := env.schedSvc.Create(st); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if err := env.accountSvc.Close(env.oldBank.ID, types.Today()); err != nil {
		t.Fatalf("close Old Bank: %v", err)
	}
}

// openEdit opens Edit Schedule on the only schedule, the way the Scheduled
// view does: load the view, then run the edit loader on the cursor row.
func (env *closedSchedEnv) openEdit(t *testing.T) {
	t.Helper()
	a := env.app
	a.Update(a.loadScheduledViewData()())
	cmd := a.loadEditScheduledDialogData()
	if cmd == nil {
		t.Fatal("edit loader returned no command: the schedule is not on the Scheduled view")
	}
	a.Update(cmd())
	if !a.sched.IsVisible() {
		t.Fatal("Edit Schedule did not open")
	}
}

// submit presses Save and runs the save command, if Save gave one.
func (env *closedSchedEnv) submit(t *testing.T) {
	t.Helper()
	_, cmd := env.app.scheduledDialogAction(dialog.DialogActionSubmit)
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("save failed: %v", e.err)
		}
	}
}

func (env *closedSchedEnv) reload(t *testing.T, id types.ID) *scheduled.Transaction {
	t.Helper()
	st, err := env.schedSvc.GetByID(id)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	return st
}

// pickSchedAccount points a combo at the account id, as a committed pick does.
func pickSchedAccount(t *testing.T, f *dialog.Field, ids []types.ID, id types.ID) {
	t.Helper()
	i := indexOfID(ids, id)
	if i < 0 {
		t.Fatalf("account %s is not in the picker", id)
	}
	f.SelectedIndex = i
	f.ComboHighlight = i
}

func selectedAccount(f *dialog.Field, ids []types.ID) types.ID {
	if f.SelectedIndex < 0 || f.SelectedIndex >= len(ids) {
		return types.NilID
	}
	return ids[f.SelectedIndex]
}

func TestEditSchedule_ClosedAccount_OpensOnItsAccount(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := scheduled.NewTransactionWithAmount(env.oldBank.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-42.00"))
	env.save(t, st)

	env.openEdit(t)
	acct := env.app.sched.dlg.Fields()[schedFieldAccount]
	if got := selectedAccount(acct, env.app.sched.accountIDs); got != env.oldBank.ID {
		t.Fatalf("Account opens on %q, want Old Bank: Save would move the schedule", acct.SelectedOption())
	}
	if got, want := acct.SelectedOption(), "Old Bank (closed)"; got != want {
		t.Errorf("Account shows %q, want %q", got, want)
	}
}

func TestEditSchedule_ClosedAccount_SaveKeepsAccount(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := scheduled.NewTransactionWithAmount(env.oldBank.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-42.00"))
	env.save(t, st)

	env.openEdit(t)
	// A Tab over the focused Account combo commits its highlight; it must
	// not move the selection off the closed account.
	env.app.handleScheduledDialogKey(tea.KeyPressMsg{Code: tea.KeyTab})
	env.app.sched.dlg.Fields()[schedFieldMemo].Value = "still here"
	env.submit(t)

	got := env.reload(t, st.ID)
	if got.AccountID != env.oldBank.ID {
		t.Fatalf("Save moved the schedule to %s, want it kept on Old Bank", got.AccountID)
	}
	if !got.Memo.Valid || got.Memo.String != "still here" {
		t.Errorf("memo = %v, want the edit saved", got.Memo)
	}
}

func TestEditSchedule_ClosedAccount_RedirectToOpenAccount(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := scheduled.NewTransactionWithAmount(env.oldBank.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-42.00"))
	env.save(t, st)

	env.openEdit(t)
	pickSchedAccount(t, env.app.sched.dlg.Fields()[schedFieldAccount], env.app.sched.accountIDs, env.checking.ID)
	env.submit(t)

	if got := env.reload(t, st.ID); got.AccountID != env.checking.ID {
		t.Fatalf("schedule is on %s, want it redirected to Checking", got.AccountID)
	}
}

func TestEditSchedule_OpenAccounts_NoClosedEntry(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := scheduled.NewTransactionWithAmount(env.checking.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-42.00"))
	env.save(t, st)

	env.openEdit(t)
	if i := indexOfID(env.app.sched.accountIDs, env.oldBank.ID); i >= 0 {
		t.Errorf("the picker offers closed Old Bank to a schedule that does not use it")
	}
}

// --- Scheduled transfer ------------------------------------------------------

func newClosedTransfer(from, to types.ID) *scheduled.Transaction {
	st := scheduled.NewTransactionWithAmount(from, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-100.00"))
	st.SetTransfer(to)
	return st
}

func TestEditScheduledTransfer_ClosedTo_OpensAndSavesOnIt(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := newClosedTransfer(env.checking.ID, env.oldBank.ID)
	env.save(t, st)

	env.openEdit(t)
	fields := env.app.sched.dlg.Fields()
	ids := env.app.sched.accountIDs
	if got := selectedAccount(fields[schedXferFieldFrom], ids); got != env.checking.ID {
		t.Errorf("From opens on %q, want Checking", fields[schedXferFieldFrom].SelectedOption())
	}
	if got := selectedAccount(fields[schedXferFieldTo], ids); got != env.oldBank.ID {
		t.Fatalf("To opens on %q, want Old Bank: Save would re-point the transfer",
			fields[schedXferFieldTo].SelectedOption())
	}

	fields[schedXferFieldMemo].Value = "kept"
	env.submit(t)
	got := env.reload(t, st.ID)
	if got.AccountID != env.checking.ID || got.TransferAccountID.ID != env.oldBank.ID {
		t.Fatalf("Save re-pointed the transfer to %s -> %s", got.AccountID, got.TransferAccountID.ID)
	}
	if !got.Memo.Valid || got.Memo.String != "kept" {
		t.Errorf("memo = %v, want the edit saved", got.Memo)
	}
}

func TestEditScheduledTransfer_RefusesMoveOntoClosedAccount(t *testing.T) {
	env := newClosedSchedEnv(t)
	st := newClosedTransfer(env.checking.ID, env.oldBank.ID)
	env.save(t, st)

	env.openEdit(t)
	fields := env.app.sched.dlg.Fields()
	ids := env.app.sched.accountIDs
	// Swap the legs: Old Bank becomes the source, which it was not before.
	pickSchedAccount(t, fields[schedXferFieldFrom], ids, env.oldBank.ID)
	pickSchedAccount(t, fields[schedXferFieldTo], ids, env.checking.ID)
	env.submit(t)

	if fields[schedXferFieldFrom].Error == "" {
		t.Error("From shows no error for a closed account the transfer did not use as its source")
	}
	if !env.app.sched.IsVisible() {
		t.Error("the dialog closed: the refused save should keep it open")
	}
	got := env.reload(t, st.ID)
	if got.AccountID != env.checking.ID || got.TransferAccountID.ID != env.oldBank.ID {
		t.Fatalf("the transfer moved to %s -> %s; a closed account takes no new schedule",
			got.AccountID, got.TransferAccountID.ID)
	}
}
