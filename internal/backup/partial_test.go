package backup

import (
	"os"
	"path/filepath"
	"runtime"
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
		"test.tdb.partial-111":                                        {old, true},         // a killed Restore
		"test.tdb.backup.2024-03-15T14-30-00.partial-222":             {old, true},         // a killed auto-backup
		"test.tdb.manual-backup.2024-03-15T14-30-00.partial-3":        {old, true},         // a killed manual backup
		"tmoney-restore-444.tmp":                                      {old, true},         // a killed Restore of an older version
		"test.tdb.partial-555":                                        {time.Now(), false}, // a copy that runs now
		"other.tdb.partial-666":                                       {old, false},        // another database's copy
		"test.tdb.notes.partial-777":                                  {old, false},        // not a backup name
		"test.tdb.partial-2025.tdb":                                   {old, false},        // another database
		"test.tdb.partial-2025.tdb.manual-backup.2024-03-15T14-30-00": {old, false},        // its backup
		"test.tdb.partial-notes.txt":                                  {old, false},        // a user's file
		"test.tdb.partial-":                                           {old, false},        // no digits
		"test.tdb.manual-backup.2024-03-15T14-30-00":                  {old, false},        // a backup
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
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

// TestRestoreKeepsTheDatabaseMode: the restored database keeps the mode of the
// database it replaces, not the mode of the backup. A read-only backup (the
// user protected it, or it came from read-only media) must not make the
// database read-only, which TMoney cannot open.
func TestRestoreKeepsTheDatabaseMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	skipValidation(t)
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	if err := os.Chmod(dbPath, 0o640); err != nil {
		t.Fatal(err)
	}
	bk, err := CreateManualBackup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bk, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(dbPath, bk); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("database mode after the restore = %v, want %v", got, os.FileMode(0o640))
	}
}

// TestSameSecondBackupsKeepBoth: backup names have one-second resolution. A
// second backup in the same second must not replace the first: the first can
// be the safety backup of a restore, which the CLI's auto-backup after the
// restore would otherwise replace with the restored state.
func TestSameSecondBackupsKeepBoth(t *testing.T) {
	for name, create := range map[string]func(string, time.Time) (string, error){
		"auto":   func(p string, at time.Time) (string, error) { return createAutoBackupAt(p, at, "") },
		"manual": createManualBackupAt,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := createTestDB(t, dir)
			at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
			first, err := create(dbPath, at)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dbPath, []byte("fictional later state"), 0o644); err != nil {
				t.Fatal(err)
			}
			second, err := create(dbPath, at)
			if err != nil {
				t.Fatal(err)
			}
			if second == first {
				t.Fatalf("both backups are at %s", filepath.Base(first))
			}
			for path, want := range map[string]string{first: "test database content", second: "fictional later state"} {
				if got, _ := os.ReadFile(path); string(got) != want {
					t.Errorf("%s holds %q, want %q", filepath.Base(path), got, want)
				}
			}
			backups, err := ListBackups(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(backups) != 2 || backups[0].Path != second {
				t.Errorf("ListBackups() = %v, want the second backup first, then the first", backups)
			}
		})
	}
}

// TestRestoreKeepsItsOwnCopyFromTheSweep: the safety backup's retention pass
// deletes old partial files by their modification time. A file system whose
// clock is behind (a network share) can give Restore's fresh copy an old
// time; the pass must not delete that copy.
func TestRestoreKeepsItsOwnCopyFromTheSweep(t *testing.T) {
	saved := validateDatabase
	validateDatabase = func(path string) error {
		old := time.Now().Add(-2 * stalePartialAge)
		return os.Chtimes(path, old, old)
	}
	t.Cleanup(func() { validateDatabase = saved })

	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	bk, err := CreateManualBackup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("fictional later state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(dbPath, bk); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if got, _ := os.ReadFile(dbPath); string(got) != "test database content" {
		t.Errorf("database = %q after the restore, want the backup", got)
	}
}
