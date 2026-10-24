package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// A backup or a restore runs as a command, so the TUI takes input while it
// runs: the user can open other files. These tests switch files while such
// work runs on the handle of an earlier file, and check that no second handle
// opens that file, and that no work runs on a handle that the App dropped.

// restoreFixture returns an open file whose fictional_state is "before", and a
// manual backup of it whose fictional_state is "restored".
func restoreFixture(t *testing.T, name string) (*db.DB, string) {
	t.Helper()
	d, _ := dbtest.NewFile(t, name)
	if err := d.SetMetadata("fictional_state", "restored"); err != nil {
		t.Fatal(err)
	}
	var bk string
	if err := d.WithFileClosed(func(p string) error {
		var err error
		bk, err = backup.CreateManualBackup(p)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetMetadata("fictional_state", "before"); err != nil {
		t.Fatal(err)
	}
	return d, bk
}

// closedFile makes a database file and closes its handle, so that only File >
// Open opens it.
func closedFile(t *testing.T, name string) string {
	t.Helper()
	d, p := dbtest.NewFile(t, name)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// switchTo runs File > Open of path to its end, as the program does.
func switchTo(t *testing.T, a *App, path string) {
	t.Helper()
	cmd := a.submitOpenFile(path)
	if cmd == nil {
		t.Fatalf("File > Open of %s did not start", filepath.Base(path))
	}
	msg := cmd()
	if _, ok := msg.(fileDialogSavedMsg); !ok {
		t.Fatalf("File > Open of %s = %#v, want fileDialogSavedMsg", filepath.Base(path), msg)
	}
	a.Update(msg)
}

// closeAll closes every handle that the App knows, at the end of a test.
func closeAll(t *testing.T, a *App) {
	t.Cleanup(func() {
		for _, h := range a.keptHandles() {
			_ = h.Close()
		}
		_ = a.db.Close()
	})
}

// holdBackups makes createManualBackup wait inside its file operation until
// proceed closes. inFn closes when it waits.
func holdBackups(t *testing.T) (inFn, proceed chan struct{}) {
	t.Helper()
	inFn, proceed = make(chan struct{}), make(chan struct{})
	saved := createManualBackup
	createManualBackup = func(path string) (string, error) {
		close(inFn)
		<-proceed
		return saved(path)
	}
	t.Cleanup(func() { createManualBackup = saved })
	return inFn, proceed
}

// TestOpenAnOlderFileWaitsForItsRestore: a restore of A runs; the user opens
// B, then C, then A. The second switch dropped the handle of A, and File >
// Open of A made a second handle on the file that the restore was about to
// replace: it showed the old data, and its writes were lost.
func TestOpenAnOlderFileWaitsForItsRestore(t *testing.T) {
	saved := prevDrainTimeout
	prevDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { prevDrainTimeout = saved })

	first, bk := restoreFixture(t, "fictional-a.tdb")
	path := first.Path()
	a := NewApp(first, &config.Config{})
	closeAll(t, a)

	inRestore, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan tea.Msg, 1)
	go func() {
		result <- restoreFile(first, bk, func(dbPath, backupPath string) (string, error) {
			close(inRestore)
			<-proceed
			return backup.Restore(dbPath, backupPath)
		})
	}()
	<-inRestore
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))

	var msg tea.Msg
	if cmd := a.submitOpenFile(path); cmd != nil {
		msg = cmd()
	}
	if opened, ok := msg.(fileDialogSavedMsg); ok {
		close(proceed)
		<-result
		got, _ := opened.db.GetMetadata("fictional_state")
		_ = opened.db.Close()
		t.Fatalf("File > Open of an older file opened it while its restore ran; the new handle shows fictional_state = %q after the restore, want %q", got, "restored")
	}
	if failed, ok := msg.(fileOpFailedMsg); !ok || !strings.Contains(failed.err.Error(), "try again") {
		t.Errorf("File > Open of A during its restore = %#v, want a fileOpFailedMsg that says try again", msg)
	}
	a.Update(msg)
	close(proceed)
	res := <-result
	if confirmed, ok := res.(restoreConfirmedMsg); !ok || confirmed.reopenErr != nil {
		t.Errorf("restore result = %#v, want restoreConfirmedMsg with no reopen error", res)
	}
	a.Update(res)

	switchTo(t, a, path)
	if got, err := a.db.GetMetadata("fictional_state"); err != nil || got != "restored" {
		t.Errorf("after the restore, File > Open of A shows fictional_state = %q (%v), want %q", got, err, "restored")
	}
}

// TestRestoreOfARetiredHandleDoesNotReopenIt: the restore confirm of A is up,
// and two File > Open land before the Yes. The restore must not run on the
// handle that the App dropped, and must not reopen it.
func TestRestoreOfARetiredHandleDoesNotReopenIt(t *testing.T) {
	first, _ := restoreFixture(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)

	d, backups, err := buildRestoreBackupDialog(first.Path())
	if err != nil || len(backups) == 0 {
		t.Fatalf("buildRestoreBackupDialog() = %d backups, %v", len(backups), err)
	}
	a.backupDialog = &backupDialogState{dialog: d, backups: backups, db: first}
	a.submitBackupDialog() // the confirm dialog is up
	action := a.confirm.action
	if action == nil {
		t.Fatal("submitBackupDialog showed no restore confirm")
	}
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))

	if a.confirm.action != nil {
		t.Error("the confirm dialog of the restore of A is still up after File > Open of other files")
	}
	// A Yes that came before the dialog closed.
	if failed, ok := action().(errMsg); !ok || !strings.Contains(failed.err.Error(), "did not start") {
		t.Errorf("the restore of a dropped handle = %#v, want an errMsg that says it did not start", failed)
	}
	if err := first.Conn().Ping(); err == nil {
		t.Error("the restore reopened a handle that the App dropped; nothing will close it")
	}
	if got := backupFiles(t, first.Path()); got != 1 {
		t.Errorf("A has %d backups, want only the 1 of the fixture: the restore must not run", got)
	}
}

// backupFiles counts the backups of the database at path.
func backupFiles(t *testing.T, path string) int {
	t.Helper()
	backups, err := backup.ListBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	return len(backups)
}

// TestRestoreDialogKeepsItsFile: the Restore list of A is up when File > Open
// of B lands. A pick and Yes then restored a backup of A over B.
func TestRestoreDialogKeepsItsFile(t *testing.T) {
	first, _ := restoreFixture(t, "fictional-a.tdb")
	second, _ := dbtest.NewFile(t, "fictional-b.tdb")
	if err := second.SetMetadata("fictional_state", "b-own"); err != nil {
		t.Fatal(err)
	}
	a := NewApp(first, &config.Config{})
	closeAll(t, a)

	d, backups, err := buildRestoreBackupDialog(first.Path())
	if err != nil || len(backups) == 0 {
		t.Fatalf("buildRestoreBackupDialog() = %d backups, %v", len(backups), err)
	}
	a.backupDialog = &backupDialogState{dialog: d, backups: backups, db: first}
	a.Update(fileDialogSavedMsg{db: second, path: second.Path()})

	if a.backupDialog != nil {
		t.Error("the Restore list of A is still up after File > Open of B")
	}
	notes := a.statusbar.Notifications()
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Text, "Another file opened") {
		t.Errorf("notifications = %v, want one that says the dialog of the last file closed", notes)
	}
	a.submitBackupDialog()
	if a.confirm.action != nil {
		t.Fatal("a restore could be confirmed from the closed Restore list of A")
	}
}

// TestRestoreListRestoresItsOwnFile: the Restore list keeps the handle of the
// file whose backups it shows. If it outlives a file switch (switchDatabase
// closes it; this is the second guard), a pick must restore that file, not
// the one that is open now.
func TestRestoreListRestoresItsOwnFile(t *testing.T) {
	first, _ := restoreFixture(t, "fictional-a.tdb")
	second, _ := dbtest.NewFile(t, "fictional-b.tdb")
	if err := second.SetMetadata("fictional_state", "b-own"); err != nil {
		t.Fatal(err)
	}
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	d, backups, err := buildRestoreBackupDialog(first.Path())
	if err != nil || len(backups) == 0 {
		t.Fatalf("buildRestoreBackupDialog() = %d backups, %v", len(backups), err)
	}
	a.Update(fileDialogSavedMsg{db: second, path: second.Path()})
	a.backupDialog = &backupDialogState{dialog: d, backups: backups, db: first} // the list of A is still up

	a.submitBackupDialog()
	if a.confirm.action == nil {
		t.Fatal("submitBackupDialog showed no restore confirm")
	}
	a.Update(a.confirm.action()) // Yes
	if got, err := second.GetMetadata("fictional_state"); err != nil || got != "b-own" {
		t.Errorf("B shows fictional_state = %q (%v) after a restore picked from the backups of A, want its own %q", got, err, "b-own")
	}
	if got, err := first.GetMetadata("fictional_state"); err != nil || got != "restored" {
		t.Errorf("A shows fictional_state = %q (%v), want %q from its backup", got, err, "restored")
	}
}

// TestOpenAnOlderFileWaitsForItsBackup: a manual backup of A runs; the user
// opens B, then C, then A. File > Open of A must wait until the backup has
// copied A, and the backup must report its file: it reported "database is
// closed" although it was written.
func TestOpenAnOlderFileWaitsForItsBackup(t *testing.T) {
	first, pathA := dbtest.NewFile(t, "first.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	inFn, proceed := holdBackups(t)

	backupDone := make(chan tea.Msg, 1)
	backupCmd := a.createManualBackupCmd()
	go func() { backupDone <- backupCmd() }()
	<-inFn
	switchTo(t, a, closedFile(t, "second.tdb"))
	switchTo(t, a, closedFile(t, "third.tdb"))

	openCmd := a.submitOpenFile(pathA)
	if openCmd == nil {
		t.Fatal("File > Open of A did not start")
	}
	openDone := make(chan tea.Msg, 1)
	go func() { openDone <- openCmd() }()
	select {
	case msg := <-openDone:
		if saved, ok := msg.(fileDialogSavedMsg); ok {
			_ = saved.db.Close()
		}
		t.Fatalf("File > Open of A = %#v while the backup of A ran, want it to wait", msg)
	case <-time.After(300 * time.Millisecond):
	}
	close(proceed)

	if res := <-backupDone; func() bool { _, ok := res.(backupCreatedMsg); return !ok }() {
		t.Errorf("backup result = %#v, want backupCreatedMsg: the backup file is written", res)
	}
	msg := <-openDone
	saved, ok := msg.(fileDialogSavedMsg)
	if !ok {
		t.Fatalf("File > Open of A after the backup = %#v, want fileDialogSavedMsg", msg)
	}
	a.Update(saved)
}

// TestOpenThePreviousFileSettlesItsBackup: one switch only (A is prevDB), and
// a backup of A runs longer than the open waits. File > Open of A must not
// open A, and says "try again"; its settle closes A for good during the
// backup. The backup must still report its file: it reported "database is
// closed", because its reopen was dropped.
func TestOpenThePreviousFileSettlesItsBackup(t *testing.T) {
	saved := prevDrainTimeout
	prevDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { prevDrainTimeout = saved })
	first, pathA := dbtest.NewFile(t, "first.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	inFn, proceed := holdBackups(t)

	backupDone := make(chan tea.Msg, 1)
	backupCmd := a.createManualBackupCmd()
	go func() { backupDone <- backupCmd() }()
	<-inFn
	switchTo(t, a, closedFile(t, "second.tdb"))

	msg := a.submitOpenFile(pathA)()
	if failed, ok := msg.(fileOpFailedMsg); !ok || !strings.Contains(failed.err.Error(), "try again") {
		t.Errorf("File > Open of A while its backup runs = %#v, want a fileOpFailedMsg that says try again", msg)
	}
	a.Update(msg)
	close(proceed)
	if res := <-backupDone; func() bool { _, ok := res.(backupCreatedMsg); return !ok }() {
		t.Errorf("backup result = %#v, want backupCreatedMsg: the backup file is written", res)
	}
	switchTo(t, a, pathA)
}

// TestNewFileOverAKeptFileSaysItExists: File > New with the path of the
// previous file, which exists. db.Create refuses it, so the handle of that
// file must stay as it is: the settle closed it for good, and waited for its
// backup, only to say "try again".
func TestNewFileOverAKeptFileSaysItExists(t *testing.T) {
	saved := prevDrainTimeout
	prevDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { prevDrainTimeout = saved })
	first, pathA := dbtest.NewFile(t, "first.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	inFn, proceed := holdBackups(t)

	backupDone := make(chan tea.Msg, 1)
	backupCmd := a.createManualBackupCmd()
	go func() { backupDone <- backupCmd() }()
	<-inFn
	switchTo(t, a, closedFile(t, "second.tdb"))

	msg := a.submitNewFile(pathA)()
	if failed, ok := msg.(fileOpFailedMsg); !ok || !strings.Contains(failed.err.Error(), "already exists") {
		t.Errorf("File > New over the previous file = %#v, want a fileOpFailedMsg that says it exists", msg)
	}
	a.Update(msg)
	close(proceed)
	<-backupDone
	if err := first.Conn().Ping(); err != nil {
		t.Errorf("File > New over the previous file closed its handle: %v", err)
	}
}

// TestQueuedWorkOnARetiredHandleDoesNotStart: a backup or restore of A was
// asked for, and its command runs only after the switches dropped its handle:
// to two other files, or to B and back to A. It must not run, must not reopen
// that handle, and must say why. After the switch back, A is open again on a
// new handle: the text said "so it is closed".
func TestQueuedWorkOnARetiredHandleDoesNotStart(t *testing.T) {
	// Each one returns the command as the App builds it for A: before the
	// switches.
	for name, ask := range map[string]func(t *testing.T, a *App, first *db.DB, bk string) tea.Cmd{
		"backup": func(_ *testing.T, a *App, _ *db.DB, _ string) tea.Cmd { return a.createManualBackupCmd() },
		"restore": func(_ *testing.T, _ *App, first *db.DB, bk string) tea.Cmd {
			return func() tea.Msg { return restoreFile(first, bk, backup.Restore) }
		},
		"ledger move": func(t *testing.T, a *App, _ *db.DB, _ string) tea.Cmd {
			cmd, _ := askLedgerMove(t, a)
			return cmd
		},
	} {
		for _, back := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, back to A %v", name, back), func(t *testing.T) {
				first, bk := restoreFixture(t, "fictional-a.tdb")
				a := NewApp(first, &config.Config{})
				closeAll(t, a)
				cmd := ask(t, a, first, bk)
				switchTo(t, a, closedFile(t, "fictional-b.tdb"))
				if back {
					switchTo(t, a, first.Path())
				} else {
					switchTo(t, a, closedFile(t, "fictional-c.tdb"))
				}

				msg := cmd()
				failed, ok := msg.(errMsg)
				const why = "did not start: fictional-a.tdb was closed or opened again after you asked for it."
				if !ok || !strings.Contains(failed.err.Error(), why) {
					t.Errorf("%s on a dropped handle = %#v (%v), want an errMsg that says %q", name, msg, failed.err, why)
				}
				if err := first.Conn().Ping(); err == nil {
					t.Errorf("the %s reopened a handle that the App dropped", name)
				}
				if got := backupFiles(t, first.Path()); got != 1 {
					t.Errorf("A has %d backups, want only the 1 of the fixture", got)
				}
				if back {
					if err := a.db.Conn().Ping(); a.db == first || err != nil {
						t.Errorf("after the switch back, the open handle of A is the dropped one, or does not work: %v", err)
					}
				}
			})
		}
	}
}

// TestErrClosedOfTheWorkIsAFailure: a backup or restore whose own work fails
// with ErrClosed did run. It must report that failure, not "did not start",
// which is only for a handle closed for good before the work.
func TestErrClosedOfTheWorkIsAFailure(t *testing.T) {
	failure := fmt.Errorf("fictional copy failure: %w", db.ErrClosed)
	saved := createManualBackup
	createManualBackup = func(string) (string, error) { return "", failure }
	t.Cleanup(func() { createManualBackup = saved })
	for name, run := range map[string]func(a *App) error{
		"backup": func(a *App) error {
			msg, _ := a.createManualBackupCmd()().(errMsg)
			return msg.err
		},
		"restore": func(a *App) error {
			msg, _ := restoreFile(a.db, "fictional.backup", func(string, string) (string, error) { return "", failure }).(errMsg)
			return msg.err
		},
		"ledger move": func(a *App) error {
			_, err := backupBeforeLedgerMove(a.db)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := dbtest.NewFile(t, "fictional-a.tdb")
			a := NewApp(d, &config.Config{})
			closeAll(t, a)
			err := run(a)
			if err == nil || !strings.Contains(err.Error(), "fictional copy failure") || strings.Contains(err.Error(), "did not start") {
				t.Errorf("%s = %v; want its own failure, not \"did not start\"", name, err)
			}
		})
	}
}

// askLedgerMove adds an HSA investment account with one row to the open file
// of a, and returns the command of Yes on its move to the register.
func askLedgerMove(t *testing.T, a *App) (tea.Cmd, *account.Account) {
	t.Helper()
	accountRepo := account.NewRepository(a.db)
	invRepo := investment.NewRepository(a.db)
	acct := account.NewAccount("Cedar Bank HSA", account.TypeHSAInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := accountRepo.Create(acct); err != nil {
		t.Fatal(err)
	}
	r := invRow(investment.TransactionTypeDeposit, "250.00")
	r.AccountID = acct.ID
	if err := invRepo.Create(r); err != nil {
		t.Fatal(err)
	}
	a.acct = acctSurface{
		modalSurface: modalSurface{dlg: func() *dialog.Dialog {
			d := buildEditAccountDialog(acct)
			d.Fields()[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeHSA)
			return d
		}()},
		data: &accountDialogData{mode: accountDialogModeEdit, account: acct},
	}
	a.submitAccountDialog()
	_, cmd := a.confirmDialogAction(dialog.DialogActionSubmit)
	if cmd == nil {
		t.Fatal("Yes on the ledger move gave no command")
	}
	return cmd, acct
}

// TestLedgerMoveAfterASwitchStaysOnItsFile: Yes on a ledger move of A, and
// File > Open of B lands before the command runs. The move must run on A, the
// file of the edit, and must not clear the undo history of B.
func TestLedgerMoveAfterASwitchStaysOnItsFile(t *testing.T) {
	first, _ := dbtest.NewFile(t, "first.tdb")
	accountRepo := account.NewRepository(first)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	cmd, acct := askLedgerMove(t, a)
	switchTo(t, a, closedFile(t, "second.tdb"))
	other := account.NewAccount("Juniper Fictional", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := a.undoManager.Execute(undo.NewCreateAccountCommand(a.services.Account, other)); err != nil {
		t.Fatal(err)
	}

	msg := cmd()
	if _, ok := msg.(ledgerMoveSavedMsg); !ok {
		t.Fatalf("ledger move after a switch = %#v, want ledgerMoveSavedMsg", msg)
	}
	a.Update(msg)
	if got, err := accountRepo.GetByID(acct.ID); err != nil || got.Type != account.TypeHSA {
		t.Errorf("account in A after the move: %v, %v; want type hsa", got, err)
	}
	if !a.undoManager.CanUndo() {
		t.Error("the ledger move of A cleared the undo history of B")
	}
}

// TestShutdownWaitsForARestoreOfAnOlderFile: quit must let a restore that
// runs on the handle of an earlier file end; it cut the restore off.
func TestShutdownWaitsForARestoreOfAnOlderFile(t *testing.T) {
	first, bk := restoreFixture(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})

	inRestore, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan tea.Msg, 1)
	go func() {
		result <- restoreFile(first, bk, func(dbPath, backupPath string) (string, error) {
			close(inRestore)
			<-proceed
			return backup.Restore(dbPath, backupPath)
		})
	}()
	<-inRestore
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))

	done := make(chan struct{})
	go func() { a.shutdown(); close(done) }()
	select {
	case <-done:
		t.Fatal("shutdown returned while the restore of an older file still ran")
	case <-time.After(300 * time.Millisecond):
	}
	close(proceed)
	<-done
	if confirmed, ok := (<-result).(restoreConfirmedMsg); !ok || confirmed.reopenErr != nil {
		t.Errorf("restore result = %#v, want restoreConfirmedMsg with no reopen error", confirmed)
	}
}

// shortenRetry makes the pause before a retired handle is asked again short.
func shortenRetry(t *testing.T) {
	t.Helper()
	saved := retrySettleDelay
	retrySettleDelay = 100 * time.Millisecond
	t.Cleanup(func() { retrySettleDelay = saved })
}

// TestRetiredHandleLeavesTheListWhenSettled: after two switches a.retired
// holds the first handle until its settle reports. A handle that did not
// settle is asked again after a pause: nothing asked again, and it stayed in
// the list for the session. A report on a handle that is not in the list asks
// nothing.
func TestRetiredHandleLeavesTheListWhenSettled(t *testing.T) {
	shortenRetry(t)
	first := dbtest.New(t)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))
	if len(a.retired) != 1 || a.retired[0] != first {
		t.Fatalf("a.retired = %v, want the first handle", a.retired)
	}

	asked := time.Now()
	_, cmd := a.Update(retiredSettledMsg{db: first, settled: false})
	if cmd == nil || len(a.retired) != 1 {
		t.Fatalf("a handle that did not settle: a.retired = %v, cmd = %v; want it kept and asked again", a.retired, cmd)
	}
	msg := cmd()
	if waited := time.Since(asked); waited < retrySettleDelay {
		t.Errorf("the handle was asked again after %v, want a pause of %v first", waited, retrySettleDelay)
	}
	if _, next := a.Update(msg); next != nil || len(a.retired) != 0 {
		t.Errorf("after the handle settled: a.retired = %v, cmd = %v; want empty and no command", a.retired, next)
	}
	if _, cmd := a.Update(retiredSettledMsg{db: first, settled: false}); cmd != nil {
		t.Error("a report on a handle that is not in the list asked again")
	}
}

// TestRetiredHandleIsAskedAgainAfterItsWorkEnds: a backup of A runs past the
// first settle after two switches. When the backup ends, the next ask settles
// the handle of A, and it leaves the list.
func TestRetiredHandleIsAskedAgainAfterItsWorkEnds(t *testing.T) {
	shortenRetry(t)
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	inFn, proceed := holdBackups(t)
	backupDone := make(chan tea.Msg, 1)
	backupCmd := a.createManualBackupCmd()
	go func() { backupDone <- backupCmd() }()
	<-inFn
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))

	// The first settle ran out of time while the backup ran.
	_, cmd := a.Update(retiredSettledMsg{db: first, settled: false})
	if cmd == nil {
		t.Fatal("a retired handle that did not settle is not asked again")
	}
	close(proceed)
	if res := <-backupDone; func() bool { _, ok := res.(backupCreatedMsg); return !ok }() {
		t.Errorf("backup result = %#v, want backupCreatedMsg", res)
	}
	msg := cmd()
	if settled, ok := msg.(retiredSettledMsg); !ok || settled.db != first || !settled.settled {
		t.Fatalf("the second ask = %#v, want the handle of A settled", msg)
	}
	a.Update(msg)
	if len(a.retired) != 0 {
		t.Errorf("a.retired = %v after the backup ended, want empty", a.retired)
	}
}
