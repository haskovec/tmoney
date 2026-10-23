package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAutoBackupRemovesStalePartials: a killed copy leaves a partial file that
// no listing shows. The next auto-backup deletes it once it is old enough, and
// keeps every file that is not a partial of this database.
func TestAutoBackupRemovesStalePartials(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	old := time.Now().Add(-2 * stalePartialAge)

	files := map[string]struct {
		age  time.Time
		gone bool
	}{
		"test.tdb.partial-111":                                 {old, true},         // a killed Restore
		"test.tdb.backup.2024-03-15T14-30-00.partial-222":      {old, true},         // a killed auto-backup
		"test.tdb.manual-backup.2024-03-15T14-30-00.partial-3": {old, true},         // a killed manual backup
		"tmoney-restore-444.tmp":                               {old, true},         // a killed Restore of an older version
		"test.tdb.partial-555":                                 {time.Now(), false}, // a copy that runs now
		"other.tdb.partial-666":                                {old, false},        // another database's copy
		"test.tdb.notes.partial-777":                           {old, false},        // not a backup name
		"test.tdb.manual-backup.2024-03-15T14-30-00":           {old, false},        // a backup
	}
	for name, f := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("fictional partial copy"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, f.age, f.age); err != nil {
			t.Fatal(err)
		}
	}
	// A directory with a partial name is not a copy.
	dirName := filepath.Join(dir, "test.tdb.partial-888")
	if err := os.Mkdir(dirName, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dirName, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateAutoBackup(dbPath); err != nil {
		t.Fatalf("CreateAutoBackup() error = %v", err)
	}

	for name, f := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if gone := os.IsNotExist(err); gone != f.gone {
			t.Errorf("%s: deleted = %v, want %v", name, gone, f.gone)
		}
	}
	if _, err := os.Stat(dirName); err != nil {
		t.Errorf("the directory with a partial name was deleted: %v", err)
	}
}

// TestBackupKeepsSourcePermissions: the copy goes through a temp file, which
// os.CreateTemp makes 0600; the backup must keep the mode of the database.
func TestBackupKeepsSourcePermissions(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	if err := os.Chmod(dbPath, 0o640); err != nil {
		t.Fatal(err)
	}
	bk, err := CreateManualBackup(dbPath)
	if err != nil {
		t.Fatalf("CreateManualBackup() error = %v", err)
	}
	info, err := os.Stat(bk)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("backup mode = %v, want %v", got, os.FileMode(0o640))
	}
}
