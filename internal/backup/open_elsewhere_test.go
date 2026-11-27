package backup_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/backup"
)

// heldByFake makes the check whether another program has the database open
// say yes, for one test.
func heldByFake(t *testing.T) {
	t.Helper()
	backup.SetHeldElsewhere(t, func(string) (bool, error) { return true, nil })
}

// touch gives the file at path a new modification time, as a write does.
func touch(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

// backupFiles returns the names in dir of backups and of partial copies.
func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	for _, name := range dirNames(t, dir) {
		if strings.Contains(name, backup.AutoBackupInfix) || strings.Contains(name, backup.ManualBackupInfix) ||
			strings.Contains(name, ".tmoney-partial-") {
			found = append(found, name)
		}
	}
	return found
}

// A program that has the database open can write to it during the copy, and
// the copy can then mix old and new blocks. A backup refuses such a file
// before the copy, and leaves no file.
func TestABackupOfADatabaseOpenElsewhereIsRefused(t *testing.T) {
	for name, create := range map[string]func(string) (string, error){
		"manual": backup.CreateManualBackup,
		"auto":   backup.CreateAutoBackup,
	} {
		for _, holder := range []string{"fake check", "another process"} {
			t.Run(name+", "+holder, func(t *testing.T) {
				path := newDBFile(t)
				if holder == "fake check" {
					heldByFake(t)
				} else {
					holdOpenElsewhere(t, path)
				}
				before := dirNames(t, filepath.Dir(path))

				bk, err := create(path)
				if !errors.Is(err, backup.ErrOpenElsewhere) {
					t.Errorf("a backup of a database open elsewhere = %q, %v; want an error that wraps ErrOpenElsewhere", bk, err)
				}
				if after := dirNames(t, filepath.Dir(path)); strings.Join(after, ",") != strings.Join(before, ",") {
					t.Errorf("the refused backup changed the folder: %v, was %v", after, before)
				}
			})
		}
	}
}

// A program that does not take the gate (an older tmoney, another DuckDB
// client) can open the database during the copy and write to it. Then the
// copy can mix old and new blocks: the backup drops it.
func TestABackupOfADatabaseThatChangesDuringTheCopyIsNotKept(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, file string){
		"a write to the file": func(t *testing.T, file string) { touch(t, file) },
		"a write to its log":  func(t *testing.T, file string) { touch(t, file+".wal") },
		"an open":             func(t *testing.T, file string) { holdOpenElsewhere(t, file) },
	} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 200, true) // a log, which the copy folds in
			backup.SetAfterCopy(t, func(file string) { change(t, file) })

			bk, err := backup.CreateManualBackup(path)
			if !errors.Is(err, backup.ErrOpenElsewhere) || !strings.Contains(err.Error(), "changed during the backup") {
				t.Errorf("a backup of a database that changed during the copy = %q, %v; want a refusal that wraps ErrOpenElsewhere", bk, err)
			}
			if left := backupFiles(t, filepath.Dir(path)); len(left) != 0 {
				t.Errorf("the dropped copy left files: %v", left)
			}
		})
	}
}

// The caller of Restore holds the gate, and a tmoney that closes the file
// waits for it. So Restore refuses a database that another program has open
// at once, before it reads the backup: no copy and no safety backup.
func TestRestoreRefusesADatabaseOpenElsewhereAtOnce(t *testing.T) {
	path := newDBFile(t)
	// Not a database: a refusal of it would show that Restore read it.
	bk := filepath.Join(filepath.Dir(path), "fin.tdb"+backup.ManualBackupInfix+"2026-01-02T03-04-05")
	if err := os.WriteFile(bk, []byte("fictional text, not a database\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	heldByFake(t)
	before := dirNames(t, filepath.Dir(path))

	safety, err := backup.Restore(path, bk)
	if want := "the database is open in another program; close it and try again"; !errors.Is(err, backup.ErrOpenElsewhere) || err.Error() != want {
		t.Errorf("Restore of a database open elsewhere: %v; want %q", err, want)
	}
	if safety != "" {
		t.Errorf("the refused restore names a safety backup: %q", safety)
	}
	if after := dirNames(t, filepath.Dir(path)); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the refused restore changed the folder: %v, was %v", after, before)
	}
}
