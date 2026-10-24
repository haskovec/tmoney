package backup_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
)

// A database can be a symlink to the real file, such as a file in a synced
// folder. DuckDB opens the file at the end of the link and keeps its logs
// beside that file. The backups go beside the link, where the restore list
// looks for them.

// linkedDB writes a TMoney database at <tmp>/real/fin.tdb and a symlink to it
// at <tmp>/link/fin.tdb, and returns the link and the real file.
func linkedDB(t *testing.T) (link, real string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"link", "real"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real = filepath.Join(root, "real", "fin.tdb")
	database, err := db.Create(real)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(root, "link", "fin.tdb")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	return link, real
}

// checkLink fails the test unless link is still a symlink.
func checkLink(t *testing.T, link string) {
	t.Helper()
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is no longer a link (%v)", link, err)
	}
}

// checkNoLeftovers fails the test if a folder has a partial file or a moved log.
func checkNoLeftovers(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		for _, n := range dirNames(t, dir) {
			if strings.Contains(n, "partial-") || strings.HasSuffix(n, "-old") {
				t.Errorf("Restore left %s in %s", n, dir)
			}
		}
	}
}

// TestRestoreThroughALinkKeepsTheLink: the rename of the restore replaced the
// link with a file, and the real file kept the old data.
func TestRestoreThroughALinkKeepsTheLink(t *testing.T) {
	link, real := linkedDB(t)
	addRows(t, real, 1, false)
	good, err := backup.CreateManualBackup(link)
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, real, 200, false)

	safety, err := backup.Restore(link, good)
	if err != nil {
		t.Fatalf("Restore through a link: %v", err)
	}
	checkLink(t, link)
	if n := rowsInFileAlone(t, real); n != 1 {
		t.Errorf("the real file holds %d rows, want the 1 row of the backup", n)
	}
	if filepath.Dir(safety) != filepath.Dir(link) {
		t.Errorf("the safety backup is %s, want it beside the link, in the restore list", safety)
	}
	if n := rowsInFileAlone(t, safety); n != 201 {
		t.Errorf("the safety backup holds %d rows, want 201", n)
	}
	checkNoLeftovers(t, filepath.Dir(link), filepath.Dir(real))
}

// TestBackupThroughALinkTakesTheWAL: a backup through a link looked for the
// log beside the link and missed every row that was only in the log.
func TestBackupThroughALinkTakesTheWAL(t *testing.T) {
	for name, create := range map[string]func(string) (string, error){
		"manual": backup.CreateManualBackup,
		"auto":   backup.CreateAutoBackup,
	} {
		t.Run(name, func(t *testing.T) {
			link, real := linkedDB(t)
			addRows(t, real, 200, true)

			bk, err := create(link)
			if err != nil {
				t.Fatalf("backup through a link: %v", err)
			}
			if n := rowsInFileAlone(t, bk); n != 200 {
				t.Errorf("the backup holds %d rows, want the 200 committed rows", n)
			}
			if filepath.Dir(bk) != filepath.Dir(link) {
				t.Errorf("the backup is %s, want it beside the link", bk)
			}
		})
	}
}

// TestRestoreThroughALinkWithADamagedWAL: the raw safety backup must copy the
// log beside the real file, and the log must move away from it, or the next
// open replays it onto the restored file.
func TestRestoreThroughALinkWithADamagedWAL(t *testing.T) {
	link, real := linkedDB(t)
	addRows(t, real, 1, false)
	good, err := backup.CreateManualBackup(link)
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, real, 200, true)
	file, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	wal := damageWAL(t, real)

	safety, err := backup.Restore(link, good)
	if err != nil {
		t.Fatalf("Restore through a link: %v", err)
	}
	checkLink(t, link)
	if db.HasWAL(real) {
		t.Error("a log of the old database is beside the restored file")
	}
	if n := rowsInFileAlone(t, real); n != 1 {
		t.Errorf("the restored database holds %d rows, want the 1 row of the backup", n)
	}
	if got, _ := os.ReadFile(safety); !bytes.Equal(got, file) {
		t.Error("the safety backup is not the old database file")
	}
	if got, _ := os.ReadFile(safety + ".wal"); !bytes.Equal(got, wal) {
		t.Error("the safety backup has no raw copy of the old log")
	}
	checkNoLeftovers(t, filepath.Dir(link), filepath.Dir(real))
}

// TestRestoreUnderALinkedFolderKeepsTheSource: a link to a folder is not a
// link to the file. Retention compares the paths in such a folder as text, so
// they must stay as given, or the safety backup's retention pass deletes the
// source of the restore.
func TestRestoreUnderALinkedFolderKeepsTheSource(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	path := filepath.Join(root, "link", "fin.tdb")
	database, err := db.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 1, false)
	var oldest string
	for i := range backup.MaxAutoBackups {
		bk := path + backup.AutoBackupInfix + "2020-01-01T00-0" + string(rune('0'+i)) + "-00"
		copyFile(t, path, bk)
		if i == 0 {
			oldest = bk
		}
	}

	safety, err := backup.Restore(path, oldest)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(oldest); err != nil {
		t.Errorf("the retention pass of the safety backup deleted the source of the restore: %v", err)
	}
	if filepath.Dir(safety) != filepath.Join(root, "link") {
		t.Errorf("the safety backup is %s, want it in the folder as given", safety)
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		t.Errorf("the database is no longer a plain file (%v)", err)
	}
}

// TestRestoreThroughALinkToAFolderWithAPercent: through a link, the copy of
// the restore goes beside the real file, and its fold must work in any folder
// name. duckdb-go parses a DSN as a URL, so a "%" in the folder stopped the
// fold of a backup with a log beside it.
func TestRestoreThroughALinkToAFolderWithAPercent(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"link", "100% Budget"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(root, "100% Budget", "fin.tdb")
	if err := os.Rename(newDBFile(t), real); err != nil { // db.Create opens a DSN too
		t.Fatal(err)
	}
	link := filepath.Join(root, "link", "fin.tdb")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	src := newDBFile(t)
	addRows(t, src, 1, false)
	addRows(t, src, 200, true)
	bk := link + backup.ManualBackupInfix + "2026-01-01T00-00-00" // with its log, as a raw safety backup
	copyFile(t, src, bk)
	copyFile(t, src+".wal", bk+".wal")

	if _, err := backup.Restore(link, bk); err != nil {
		t.Fatalf("Restore through a link to a folder with a %% in its name: %v", err)
	}
	checkLink(t, link)
	if n := rowsInFileAlone(t, real); n != 201 {
		t.Errorf("the restored database holds %d rows, want all 201 of the backup and its log", n)
	}
	checkNoLeftovers(t, filepath.Dir(link), filepath.Dir(real))
}
