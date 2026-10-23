package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// DuckDB caches one instance per file path, so a second *db.DB on the open file
// shares the instance of the first. A backup or restore through one handle then
// closes only that handle's pool: the instance lives on, its last writes stay in
// <path>.wal, and the backup misses them. File > Open of the file that is open
// must therefore not open it again.

// TestOpenTheOpenFile_KeepsOneHandle covers File > Open and Open Recent with the
// path of the open file, also when the path is spelled another way.
func TestOpenTheOpenFile_KeepsOneHandle(t *testing.T) {
	database, path := dbtest.NewFile(t, "fin.tdb")
	t.Cleanup(func() { _ = database.Close() })
	link := filepath.Join(t.TempDir(), "link.tdb")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}

	for name, open := range map[string]string{"same path": path, "symlink": link} {
		t.Run(name, func(t *testing.T) {
			a := NewApp(database, &config.Config{})
			msg := a.submitOpenFile(open)()
			if saved, ok := msg.(fileDialogSavedMsg); ok {
				_ = saved.db.Close()
				t.Fatalf("opening the open file made a second handle on it")
			}
			if _, ok := msg.(fileAlreadyOpenMsg); !ok {
				t.Fatalf("submitOpenFile(%s)() = %T, want fileAlreadyOpenMsg", name, msg)
			}
			a.Update(msg)
			if a.db != database {
				t.Error("the open database changed")
			}
		})
	}
}

// TestOpenThePreviousFile_ClosesItsKeptHandleFirst: after a switch, the handle
// of the previous file stays open (prevDB) until the next switch. Opening that
// file again must close it first, or the new handle shares its instance and
// its backups miss the rows in the WAL.
func TestOpenThePreviousFile_ClosesItsKeptHandleFirst(t *testing.T) {
	first, path := dbtest.NewFile(t, "first.tdb")
	t.Cleanup(func() { _ = first.Close() })
	a := NewApp(first, &config.Config{})
	a.switchDatabase(dbtest.New(t))

	msg := a.submitOpenFile(path)()
	saved, ok := msg.(fileDialogSavedMsg)
	if !ok {
		t.Fatalf("submitOpenFile() = %#v, want fileDialogSavedMsg", msg)
	}
	reopened := saved.db
	t.Cleanup(func() { _ = reopened.Close() })

	if err := reopened.SetMetadata("fictional_probe", "present"); err != nil {
		t.Fatal(err)
	}
	err := reopened.WithFileClosed(func(p string) error {
		if _, err := os.Stat(p + ".wal"); err == nil {
			t.Error("the backup would copy the file while <path>.wal still holds a committed row: " +
				"the kept handle of the previous file shares the instance")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithFileClosed() error = %v", err)
	}
}

// TestOneOperationPerFile: a second File > Open or New of a file while the
// first one is still opening (a slow share) must not run. Two opens of one
// file gave two handles on it, and two creates of one path let the loser
// delete the file that the winner uses. Another file still opens, so an open
// that never returns blocks only its own file.
func TestOneOperationPerFile(t *testing.T) {
	first := dbtest.New(t)
	_, other := dbtest.NewFile(t, "other.tdb")
	_, third := dbtest.NewFile(t, "third.tdb")
	a := NewApp(first, &config.Config{})

	cmd := a.submitOpenFile(other)
	if cmd == nil {
		t.Fatal("the first open did not start")
	}
	if a.submitOpenFile(other) != nil {
		t.Error("a second open of the file started while the first one was in flight")
	}
	if a.submitNewFile(other) != nil {
		t.Error("a create of the path started while an open of it was in flight")
	}
	if a.submitOpenFile(third) == nil {
		t.Error("an open of another file was refused while one was in flight")
	}

	a.Update(cmd())
	t.Cleanup(func() { _ = a.db.Close() })
	if a.submitOpenFile(other) == nil {
		t.Error("no operation on the file can start after the first one ended")
	}
}

// TestAlreadyOpenAfterASwitchOpensAgain: the open of file A started while A was
// open, so its command reported "already open". But an open of B that was in
// flight switched to B first. A is not open any more, so the request must not
// be dropped: the open runs again.
func TestAlreadyOpenAfterASwitchOpensAgain(t *testing.T) {
	firstDB, firstPath := dbtest.NewFile(t, "first.tdb")
	t.Cleanup(func() { _ = firstDB.Close() })
	_, secondPath := dbtest.NewFile(t, "second.tdb")
	a := NewApp(firstDB, &config.Config{})

	toSecond := a.submitOpenFile(secondPath)
	stale := a.submitOpenFile(firstPath)()
	if _, ok := stale.(fileAlreadyOpenMsg); !ok {
		t.Fatalf("submitOpenFile(first)() = %T, want fileAlreadyOpenMsg", stale)
	}
	a.Update(toSecond())
	t.Cleanup(func() { _ = a.db.Close() })

	_, again := a.Update(stale)
	if again == nil {
		t.Fatal("the stale report dropped the open of the first file")
	}
	saved, ok := again().(fileDialogSavedMsg)
	if !ok {
		t.Fatal("the open of the first file ran again but did not open it")
	}
	_ = saved.db.Close()
}

// TestFailedOpenEndsTheFileOperation: an open that fails reports its error and
// does not block the next File > Open.
func TestFailedOpenEndsTheFileOperation(t *testing.T) {
	a := NewApp(dbtest.New(t), &config.Config{})
	missing := filepath.Join(t.TempDir(), "missing.tdb")
	a.Update(a.submitOpenFile(missing)())
	if a.err == nil {
		t.Error("the failed open reported no error")
	}
	if a.submitOpenFile(missing) == nil {
		t.Error("the failed open blocked the next file operation")
	}
}

// TestOpenThePreviousFileWaitsForItsReads: Close ends only the idle
// connections of the kept handle. A load of the old session that still holds
// one keeps the DuckDB instance, and a new handle would share it. The open
// must wait for that read, and say "try again" if it holds on.
func TestOpenThePreviousFileWaitsForItsReads(t *testing.T) {
	saved := prevDrainTimeout
	prevDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { prevDrainTimeout = saved })

	first, path := dbtest.NewFile(t, "first.tdb")
	t.Cleanup(func() { _ = first.Close() })
	rows, err := first.Conn().Query(`SELECT key FROM _metadata`)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("no row")
	}
	t.Cleanup(func() { _ = rows.Close() })
	a := NewApp(first, &config.Config{})
	a.switchDatabase(dbtest.New(t))

	msg := a.submitOpenFile(path)()
	if saved, ok := msg.(fileDialogSavedMsg); ok {
		_ = saved.db.Close()
		t.Fatal("the open ran while a read held the kept handle of the file")
	}
	failed, ok := msg.(fileOpFailedMsg)
	if !ok || !strings.Contains(failed.err.Error(), "try again") {
		t.Fatalf("submitOpenFile() = %#v, want a fileOpFailedMsg that says try again", msg)
	}
	a.Update(msg)

	_ = rows.Close()
	saved2, ok := a.submitOpenFile(path)().(fileDialogSavedMsg)
	if !ok {
		t.Fatal("the open after the read let go did not open the file")
	}
	_ = saved2.db.Close()
}

// TestOpenWithATildeIsTheOpenFile: the dialog passes the text as typed. "~/x"
// names the open file as much as its full path does.
func TestOpenWithATildeIsTheOpenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	database, _ := dbtest.NewFileIn(t, home, "fin.tdb")
	t.Cleanup(func() { _ = database.Close() })
	a := NewApp(database, &config.Config{})

	msg := a.submitOpenFile("~/fin.tdb")()
	if saved, ok := msg.(fileDialogSavedMsg); ok {
		_ = saved.db.Close()
		t.Fatal("File > Open of ~/fin.tdb made a second handle on the open file")
	}
	if _, ok := msg.(fileAlreadyOpenMsg); !ok {
		t.Errorf("submitOpenFile(~/fin.tdb)() = %T, want fileAlreadyOpenMsg", msg)
	}
}

// TestNewFileUnderTheHomeFolder: File > New of "~/x" creates x.tdb in the home
// folder, and no folder named "~" in the working folder.
func TestNewFileUnderTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	t.Chdir(work)
	a := NewApp(dbtest.New(t), &config.Config{})

	saved, ok := a.submitNewFile("~/fictional-new")().(fileDialogSavedMsg)
	if !ok {
		t.Fatal("File > New of ~/fictional-new did not create the file")
	}
	t.Cleanup(func() { _ = saved.db.Close() })
	if want := filepath.Join(home, "fictional-new.tdb"); saved.db.Path() != want {
		t.Errorf("the new database is at %s, want %s", saved.db.Path(), want)
	}
	if _, err := os.Stat(filepath.Join(work, "~")); err == nil {
		t.Error("File > New made a folder named ~ in the working folder")
	}
}

// TestNewFileWithoutTheExtensionIsTheSameFile: Create adds ".tdb", so File >
// New of "x" while an open of "x.tdb" is in flight is an operation on the same
// file.
func TestNewFileWithoutTheExtensionIsTheSameFile(t *testing.T) {
	_, other := dbtest.NewFile(t, "other.tdb")
	a := NewApp(dbtest.New(t), &config.Config{})
	if a.submitOpenFile(other) == nil {
		t.Fatal("the open did not start")
	}
	if a.submitNewFile(strings.TrimSuffix(other, ".tdb")) != nil {
		t.Error("File > New of the path without .tdb started while an open of the file was in flight")
	}
}

// TestOpenThePreviousFileWaitsForItsRestore: a restore of file A still runs
// when the user switches to B and then opens A again. The new handle must not
// open A before the restore has replaced it: it would show the old data, and
// its writes would go to the replaced file and be lost.
func TestOpenThePreviousFileWaitsForItsRestore(t *testing.T) {
	saved := prevDrainTimeout
	prevDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { prevDrainTimeout = saved })

	first, path := dbtest.NewFile(t, "first.tdb")
	t.Cleanup(func() { _ = first.Close() })
	if err := first.SetMetadata("fictional_state", "before"); err != nil {
		t.Fatal(err)
	}
	bk, bkPath := dbtest.NewFile(t, "fictional-backup.tdb")
	if err := bk.SetMetadata("fictional_state", "restored"); err != nil {
		t.Fatal(err)
	}
	if err := bk.Close(); err != nil {
		t.Fatal(err)
	}

	a := NewApp(first, &config.Config{})
	inRestore, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan tea.Msg, 1)
	go func() {
		result <- restoreFile(first, bkPath, func(dbPath, backupPath string) (string, error) {
			close(inRestore)
			<-proceed
			return backup.Restore(dbPath, backupPath)
		})
	}()
	<-inRestore
	a.switchDatabase(dbtest.New(t)) // File > Open B: A is the kept handle now

	msg := a.submitOpenFile(path)() // File > Open A while its restore runs
	if opened, ok := msg.(fileDialogSavedMsg); ok {
		_ = opened.db.Close()
		close(proceed)
		<-result
		t.Fatal("File > Open of the previous file opened it while its restore ran")
	}
	if failed, ok := msg.(fileOpFailedMsg); !ok || !strings.Contains(failed.err.Error(), "try again") {
		t.Errorf("submitOpenFile() = %#v, want a fileOpFailedMsg that says try again", msg)
	}
	a.Update(msg)
	close(proceed)
	<-result

	opened, ok := a.submitOpenFile(path)().(fileDialogSavedMsg)
	if !ok {
		t.Fatal("the open after the restore did not open the file")
	}
	defer func() { _ = opened.db.Close() }()
	if got, err := opened.db.GetMetadata("fictional_state"); err != nil || got != "restored" {
		t.Errorf("the new handle shows fictional_state = %q (%v), want the restored %q", got, err, "restored")
	}
}
