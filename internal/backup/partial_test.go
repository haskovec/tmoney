package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
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
		"test.tdb.tmoney-partial-111":                                        {old, true},         // a killed Restore
		"test.tdb.backup.2024-03-15T14-30-00.tmoney-partial-222":             {old, true},         // a killed auto-backup
		"test.tdb.manual-backup.2024-03-15T14-30-00.tmoney-partial-3":        {old, true},         // a killed manual backup
		"tmoney-restore-444.tmp":                                             {old, true},         // a killed Restore of an older version
		"test.tdb.tmoney-partial-555":                                        {time.Now(), false}, // a copy that runs now
		"other.tdb.tmoney-partial-666":                                       {old, false},        // another database's copy
		"test.tdb.notes.tmoney-partial-777":                                  {old, false},        // not a backup name
		"test.tdb.tmoney-partial-2025.tdb":                                   {old, false},        // another database
		"test.tdb.tmoney-partial-2025.tdb.manual-backup.2024-03-15T14-30-00": {old, false},        // its backup
		"test.tdb.tmoney-partial-notes.txt":                                  {old, false},        // a user's file
		"test.tdb.tmoney-partial-":                                           {old, false},        // no digits
		"test.tdb.partial-2025":                                              {old, false},        // a user's file, named by year
		"test.tdb.partial-20251023":                                          {old, false},        // a user's file, named by date
		"test.tdb.tmoney-partial-112.wal":                                    {old, true},         // the log of a killed copy
		"test.tdb.tmoney-partial-113.wal-old":                                {old, true},         // a killed Restore's moved log
		"test.tdb.tmoney-partial-114.wal.checkpoint":                         {old, true},         // the checkpoint log of a killed copy
		"test.tdb.tmoney-partial-115.wal.recovery-old":                       {old, true},         // a killed Restore's moved log
		"test.tdb.tmoney-partial-116.walrus":                                 {old, false},        // not a log
		"test.tdb.tmoney-partial-2025-old":                                   {old, false},        // a user's file, no log suffix
		"test.tdb.partial-2025.wal":                                          {old, false},        // the log of a user's file
		"test.tdb.partial-2025.wal-old":                                      {old, false},        // a user's file
		"test.tdb.manual-backup.2024-03-15T14-30-00":                         {old, false},        // a backup
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
	dirName := filepath.Join(dir, "test.tdb.tmoney-partial-888")
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

// TestPartialNamesMatchTheSweep: the sweep finds the leftover of a killed
// copy by its name only, so each name that a copy writes must be a name that
// isPartialOf knows: the partial file, the raw copies of the logs beside it,
// and the names to which Restore moves the logs of the database.
func TestPartialNamesMatchTheSweep(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	for _, suffix := range db.WALSuffixes {
		if err := os.WriteFile(dbPath+suffix, []byte("fictional log"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, dst := range []string{
		dbPath, // Restore
		dbPath + AutoBackupInfix + "2026-01-01T09-00-00",
		dbPath + ManualBackupInfix + "2026-01-01T09-00-00",
	} {
		temp, err := copyToTemp(dbPath, dst, dbPath, false)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{temp}
		for _, suffix := range db.WALSuffixes {
			if _, err := os.Stat(temp + suffix); err != nil {
				t.Errorf("no raw copy of the log %s: %v", suffix, err)
			}
			names = append(names, temp+suffix, temp+suffix+"-old")
		}
		for _, name := range names {
			if !isPartialOf(filepath.Base(dbPath), filepath.Base(name)) {
				t.Errorf("a copy to %s writes %s, which the sweep does not know", filepath.Base(dst), filepath.Base(name))
			}
		}
		removeCopy(temp)
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
		"auto":   func(p string, at time.Time) (string, error) { return createAutoBackupAt(p, at) },
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

// TestBurstOfAutoBackupsKeepsTheNewest: more than one auto-backup per second
// (a CLI script) pushes the names ahead of the clock. Each new backup must
// still be the newest one: a name in a second that retention freed would sort
// as the oldest, and its own retention pass would delete it at once. A
// restore's safety backup was lost this way.
func TestBurstOfAutoBackupsKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := range 2*MaxAutoBackups + 2 {
		state := fmt.Sprintf("fictional state %d", i)
		if err := os.WriteFile(dbPath, []byte(state), 0o644); err != nil {
			t.Fatal(err)
		}
		path, err := createAutoBackupAt(dbPath, at)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != state {
			t.Fatalf("backup %d: %s holds %q (%v), want %q", i, filepath.Base(path), got, err, state)
		}
		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if backups[0].Path != path {
			t.Fatalf("backup %d: the newest backup is %s, want the one just made, %s",
				i, filepath.Base(backups[0].Path), filepath.Base(path))
		}
	}
}

// TestPublishNeverReplaces: two processes can pick the same free name at the
// same time (the quit backup of the TUI and the auto-backup of a CLI command).
// The second one must not replace the first one's backup.
func TestPublishNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "test.tdb.manual-backup.2026-01-01T09-00-00")
	for i, content := range []string{"first", "second"} {
		temp := filepath.Join(dir, fmt.Sprintf("temp-%d", i))
		if err := os.WriteFile(temp, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		err := publish(temp, dst)
		if i == 0 && err != nil {
			t.Fatalf("publish() error = %v", err)
		}
		if i == 1 && !errors.Is(err, os.ErrExist) {
			t.Errorf("publish() over an existing backup = %v, want os.ErrExist", err)
		}
	}
	if got, _ := os.ReadFile(dst); string(got) != "first" {
		t.Errorf("the backup holds %q, want %q", got, "first")
	}
}

// TestRestoreAfterABurstKeepsTheSafetyBackup is the case that the burst lost:
// a restore right after many auto-backups, whose names are ahead of the clock.
func TestRestoreAfterABurstKeepsTheSafetyBackup(t *testing.T) {
	skipValidation(t)
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	saved, err := CreateManualBackup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for range MaxAutoBackups + 2 {
		if _, err := createAutoBackupAt(dbPath, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dbPath, []byte("fictional state before the restore"), 0o644); err != nil {
		t.Fatal(err)
	}
	safety, err := Restore(dbPath, saved)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if got, err := os.ReadFile(safety); err != nil || string(got) != "fictional state before the restore" {
		t.Errorf("the safety backup %s holds %q (%v), want the state before the restore", filepath.Base(safety), got, err)
	}
}

// TestBackupNamesInTheRepeatedHour: at the end of daylight saving time the
// wall clock repeats an hour. The names hold the wall clock and sort as text,
// so a new backup must still sort after the ones of the first pass of that
// hour, and a backup in a repeated second must still get a free name.
func TestBackupNamesInTheRepeatedHour(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	firstPass := func(minute int) time.Time { return time.Date(2026, 11, 1, 1, minute, 0, 0, chicago) }
	secondPass := func(minute int) time.Time { return firstPass(minute).Add(time.Hour) }

	t.Run("a later backup sorts last", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)
		for _, minute := range []int{10, 20, 30, 40, 50} {
			if _, err := createAutoBackupAt(dbPath, firstPass(minute)); err != nil {
				t.Fatal(err)
			}
		}
		path, err := createAutoBackupAt(dbPath, secondPass(5))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the backup of the repeated hour is gone: %v", err)
		}
		backups, _ := ListBackups(dbPath)
		if backups[0].Path != path {
			t.Errorf("the newest backup is %s, want the one just made, %s", filepath.Base(backups[0].Path), filepath.Base(path))
		}
	})

	t.Run("a repeated second", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)
		first, err := createManualBackupAt(dbPath, firstPass(10))
		if err != nil {
			t.Fatal(err)
		}
		second, err := createManualBackupAt(dbPath, secondPass(10))
		if err != nil {
			t.Fatalf("a backup in the repeated second: %v", err)
		}
		if second == first {
			t.Errorf("both backups are at %s", filepath.Base(first))
		}
	})
}

// TestNamesAheadOfTheClockKeepTheNewBackup: names can be ahead of the clock
// (a clock that was set back, a move to a time zone to the west, a backup
// copied in from another machine). A new backup must still be the newest one,
// also when MaxAutoBackups names are ahead: a name at the time now would sort
// after them, and its own retention pass would delete it, also the safety
// backup of a restore. So a new name follows the newest one, not the clock.
func TestNamesAheadOfTheClockKeepTheNewBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := range MaxAutoBackups {
		ahead := dbPath + AutoBackupInfix + at.AddDate(1, 0, i).Format(TimestampFormat)
		if err := os.WriteFile(ahead, []byte("fictional backup from a clock that was ahead"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path, err := createAutoBackupAt(dbPath, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the new backup is gone: %v", err)
	}
	if backups, _ := ListBackups(dbPath); backups[0].Path != path {
		t.Errorf("the newest backup is %s, want the one just made, %s", filepath.Base(backups[0].Path), filepath.Base(path))
	}
}

// TestRetentionDeletesTheRawLogsOfABackup: a raw safety backup (see
// createRawSafetyBackup) has copies of the old logs beside it. They go with it.
func TestRetentionDeletesTheRawLogsOfABackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	oldest, err := createAutoBackupAt(dbPath, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldest+".wal", []byte("fictional raw log"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range MaxAutoBackups {
		if _, err := createAutoBackupAt(dbPath, at.Add(time.Duration(i+1)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{oldest, oldest + ".wal"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there after retention deleted its backup", filepath.Base(path))
		}
	}
}

// TestRawLogRenameFailureStopsTheBackup: the raw copies of the logs must reach
// their names beside the backup. If one cannot, the backup is not complete:
// it fails, it leaves nothing under a backup name, and the source keeps its
// logs.
func TestRawLogRenameFailureStopsTheBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := createTestDB(t, dir)
	if err := os.WriteFile(dbPath+".wal", []byte("fictional raw log"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := dbPath + AutoBackupInfix + "2026-01-01T09-00-00"
	if err := os.Mkdir(dst+".wal", 0o755); err != nil { // the rename of the log fails
		t.Fatal(err)
	}
	if _, err := copyDatabase(dbPath, func() string { return dst }, false); err == nil {
		t.Error("copyDatabase() = nil, but the raw log did not reach its name")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("a backup without its log is under the name %s", filepath.Base(dst))
	}
	if got, _ := os.ReadFile(dbPath + ".wal"); string(got) != "fictional raw log" {
		t.Errorf("the log of the source changed: %q", got)
	}
}
