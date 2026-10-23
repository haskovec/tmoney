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
