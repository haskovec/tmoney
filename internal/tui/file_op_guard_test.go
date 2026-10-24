package tui

import (
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
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))

	if a.confirm.action != nil {
		t.Error("the confirm dialog of the restore of A is still up after File > Open of other files")
	}
	if action != nil {
		action() // a Yes that came before the dialog closed
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
		a.Update(a.confirm.action()) // Yes
	}
	if got, err := second.GetMetadata("fictional_state"); err != nil || got != "b-own" {
		t.Errorf("B shows fictional_state = %q (%v) after a restore picked from the backups of A, want its own %q", got, err, "b-own")
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
// a backup of A runs. File > Open of A waits for it, and the backup reports
// its file, not "database is closed".
func TestOpenThePreviousFileSettlesItsBackup(t *testing.T) {
	first, pathA := dbtest.NewFile(t, "first.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	inFn, proceed := holdBackups(t)

	backupDone := make(chan tea.Msg, 1)
	backupCmd := a.createManualBackupCmd()
	go func() { backupDone <- backupCmd() }()
	<-inFn
	switchTo(t, a, closedFile(t, "second.tdb"))

	openDone := make(chan tea.Msg, 1)
	openCmd := a.submitOpenFile(pathA)
	go func() { openDone <- openCmd() }()
	time.Sleep(100 * time.Millisecond)
	close(proceed)
	if res := <-backupDone; func() bool { _, ok := res.(backupCreatedMsg); return !ok }() {
		t.Errorf("backup result = %#v, want backupCreatedMsg", res)
	}
	if msg := <-openDone; func() bool { _, ok := msg.(fileDialogSavedMsg); return !ok }() {
		t.Errorf("File > Open of A after the backup = %#v, want fileDialogSavedMsg", msg)
	} else {
		a.Update(msg)
	}
}

// TestQueuedWorkOnARetiredHandleDoesNotStart: a backup or restore of A was
// asked for, and its command runs only after two switches dropped A. It must
// not run, must not reopen A, and must say why.
func TestQueuedWorkOnARetiredHandleDoesNotStart(t *testing.T) {
	// Each one returns the command as the App builds it for A: before the
	// switches.
	for name, ask := range map[string]func(a *App, first *db.DB, bk string) tea.Cmd{
		"backup": func(a *App, _ *db.DB, _ string) tea.Cmd { return a.createManualBackupCmd() },
		"restore": func(_ *App, first *db.DB, bk string) tea.Cmd {
			return func() tea.Msg { return restoreFile(first, bk, backup.Restore) }
		},
	} {
		t.Run(name, func(t *testing.T) {
			first, bk := restoreFixture(t, "fictional-a.tdb")
			a := NewApp(first, &config.Config{})
			closeAll(t, a)
			cmd := ask(a, first, bk)
			switchTo(t, a, closedFile(t, "fictional-b.tdb"))
			switchTo(t, a, closedFile(t, "fictional-c.tdb"))

			msg := cmd()
			failed, ok := msg.(errMsg)
			if !ok || !strings.Contains(failed.err.Error(), "did not start") {
				t.Errorf("%s on a dropped handle = %#v, want an errMsg that says it did not start", name, msg)
			}
			if err := first.Conn().Ping(); err == nil {
				t.Errorf("the %s reopened a handle that the App dropped", name)
			}
			if got := backupFiles(t, first.Path()); got != 1 {
				t.Errorf("A has %d backups, want only the 1 of the fixture", got)
			}
		})
	}
}

// TestLedgerMoveAfterASwitchStaysOnItsFile: Yes on a ledger move of A, and
// File > Open of B lands before the command runs. The move must run on A, the
// file of the edit, and must not clear the undo history of B.
func TestLedgerMoveAfterASwitchStaysOnItsFile(t *testing.T) {
	first, _ := dbtest.NewFile(t, "first.tdb")
	accountRepo := account.NewRepository(first)
	invRepo := investment.NewRepository(first)
	acct := account.NewAccount("Cedar Bank HSA", account.TypeHSAInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := accountRepo.Create(acct); err != nil {
		t.Fatal(err)
	}
	r := invRow(investment.TransactionTypeDeposit, "250.00")
	r.AccountID = acct.ID
	if err := invRepo.Create(r); err != nil {
		t.Fatal(err)
	}

	a := NewApp(first, &config.Config{})
	closeAll(t, a)
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

// TestRetiredHandleLeavesTheListWhenSettled: after two switches a.retired
// holds the first handle until its settle reports; a handle that did not
// settle stays, and no command asks again.
func TestRetiredHandleLeavesTheListWhenSettled(t *testing.T) {
	first := dbtest.New(t)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))
	switchTo(t, a, closedFile(t, "fictional-c.tdb"))
	if len(a.retired) != 1 || a.retired[0] != first {
		t.Fatalf("a.retired = %v, want the first handle", a.retired)
	}
	if _, cmd := a.Update(retiredSettledMsg{db: first, settled: false}); cmd != nil || len(a.retired) != 1 {
		t.Errorf("a handle that did not settle: a.retired = %v, cmd = %v; want it kept and no command", a.retired, cmd)
	}
	a.Update(settleRetired(first)())
	if len(a.retired) != 0 {
		t.Errorf("a.retired = %v after the handle settled, want empty", a.retired)
	}
}
