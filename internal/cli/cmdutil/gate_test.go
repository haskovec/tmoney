package cmdutil

import (
	"errors"
	"os"
	"testing"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// TestMain also runs the child process of dbtest.OpenElsewhere.
func TestMain(m *testing.M) {
	dbtest.RunOpenChild()
	os.Exit(m.Run())
}

// stubCreateAutoBackup replaces createAutoBackup for one test.
func stubCreateAutoBackup(t *testing.T, f func(path string) (string, error)) {
	t.Helper()
	saved := createAutoBackup
	createAutoBackup = f
	t.Cleanup(func() { createAutoBackup = saved })
}

// The backup after a CLI command holds the gate of the file from before the
// close to the end of the copy. Another tmoney, such as a TUI that a
// terminal resumes, does not open the file in between: it waits for the gate.
// After the backup, it opens the file.
func TestAutoBackupAfterModificationKeepsOtherTmoneyOutUntilTheCopyEnds(t *testing.T) {
	database, path := dbtest.NewFile(t, "fin.tdb")
	var during, made string
	stubCreateAutoBackup(t, func(p string) (string, error) {
		during = dbtest.OpenElsewhere(t, p)
		var err error
		made, err = backup.CreateAutoBackup(p)
		return made, err
	})

	stderr := captureStderr(t, func() { AutoBackupAfterModification(database) })

	if during != "busy" {
		t.Errorf("an open in another process during the backup: %s, want busy", during)
	}
	if _, err := os.Stat(made); made == "" || err != nil {
		t.Errorf("the backup %q is not there: %v", made, err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no warning", stderr)
	}
	if after := dbtest.OpenElsewhere(t, path); after != "opened" {
		t.Errorf("an open in another process after the backup: %s, want opened", after)
	}
}

// A backup that ran and failed is a warning; the command still succeeds.
func TestAutoBackupAfterModificationWarnsOfAFailedBackup(t *testing.T) {
	database, _ := dbtest.NewFile(t, "fin.tdb")
	stubCreateAutoBackup(t, func(string) (string, error) {
		return "", errors.New("fictional: the disk is full")
	})

	stderr := captureStderr(t, func() { AutoBackupAfterModification(database) })

	if want := "warning: auto-backup failed: fictional: the disk is full\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}
