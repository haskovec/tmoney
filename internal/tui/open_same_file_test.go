package tui

import (
	"os"
	"path/filepath"
	"testing"

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
