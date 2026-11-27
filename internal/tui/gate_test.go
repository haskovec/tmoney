package tui

import (
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

// The quit backup holds the gate of the file from before the close to the
// end of the copy. Another tmoney, such as the one that a terminal resume
// starts, does not open the file in between: it waits for the gate. After the
// quit, it opens the file.
func TestQuitBackupKeepsOtherTmoneyOutUntilTheCopyEnds(t *testing.T) {
	database, path := dbtest.NewFile(t, "fin.tdb")
	var during, made string
	saved := createAutoBackup
	createAutoBackup = func(p string) (string, error) {
		during = dbtest.OpenElsewhere(t, p)
		var err error
		made, err = backup.CreateAutoBackup(p)
		return made, err
	}
	t.Cleanup(func() { createAutoBackup = saved })

	(&App{db: database}).shutdown()

	if during != "busy" {
		t.Errorf("an open in another process during the quit backup: %s, want busy", during)
	}
	if _, err := os.Stat(made); made == "" || err != nil {
		t.Errorf("the quit backup %q is not there: %v", made, err)
	}
	if after := dbtest.OpenElsewhere(t, path); after != "opened" {
		t.Errorf("an open in another process after the quit: %s, want opened", after)
	}
}
