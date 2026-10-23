package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// TestRestoreWithAFailedReopen: Restore replaced the file, but the reopen of
// the handle failed (a share that dropped for a moment). The file is restored
// all the same, so the session must drop what it derived from the old file:
// the undo history above all, whose commands would write the old rows into
// the restored data once the handle opens again.
func TestRestoreWithAFailedReopen(t *testing.T) {
	database, path := dbtest.NewFile(t, "fictional-restore.tdb")
	t.Cleanup(func() { _ = database.Close() })
	a := NewApp(database, &config.Config{})

	acct := account.NewAccount("Alpha Fictional", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := database.WithFileClosed(func(p string) error {
		var err error
		saved, err = backup.CreateManualBackup(p)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	edit, err := a.services.Account.GetByID(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit.Name = "Beta Fictional"
	if err := a.undoManager.Execute(undo.NewEditAccountCommand(a.services.Account, edit)); err != nil {
		t.Fatal(err)
	}

	// The restore succeeds; then the file cannot be opened for a moment.
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	msg := restoreFile(database, saved, func(dbPath, backupPath string) (string, error) {
		safety, err := backup.Restore(dbPath, backupPath)
		if err == nil {
			err = os.Chmod(dbPath, 0)
		}
		return safety, err
	})
	confirmed, ok := msg.(restoreConfirmedMsg)
	if !ok {
		t.Fatalf("restoreFile() = %#v, want restoreConfirmedMsg: the file was restored", msg)
	}
	if confirmed.reopenErr == nil {
		t.Skip("the reopen of a file with mode 000 succeeded (running as root?)")
	}
	a.Update(msg)

	if n := a.undoManager.UndoLen(); n != 0 {
		t.Errorf("the undo history of the old file has %d commands after the restore, want 0", n)
	}
	if a.err == nil || !strings.Contains(a.err.Error(), "restored from backup") {
		t.Errorf("a.err = %v, want a message that the restore was made but the reopen failed", a.err)
	}
}
