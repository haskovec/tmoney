package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
)

// A database whose close skipped the checkpoint has committed rows only in
// <path>.wal: after a crash or a kill, and after a DuckDB fatal error
// invalidated the instance. PRAGMA disable_checkpoint_on_shutdown leaves the
// same files behind.

// addRows commits n rows to the table "rows" of the closed database at path
// and closes it. With leaveWAL the close skips the checkpoint, so the rows are
// only in <path>.wal.
func addRows(t *testing.T, path string, n int, leaveWAL bool) {
	t.Helper()
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{`CREATE TABLE IF NOT EXISTS rows (id INTEGER)`, `CHECKPOINT`}
	if leaveWAL {
		stmts = append(stmts, `PRAGMA disable_checkpoint_on_shutdown`)
	}
	for _, q := range stmts {
		if _, err := database.Conn().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := database.Conn().Exec(`INSERT INTO rows SELECT range FROM range(?)`, n); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".wal"); (err == nil) != leaveWAL {
		t.Fatalf("WAL left = %v, want %v", err == nil, leaveWAL)
	}
}

// rowsInFileAlone copies the single file at path, without any WAL, and counts
// the rows of the table "rows" in the copy.
func rowsInFileAlone(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	alone := filepath.Join(t.TempDir(), "alone.tdb")
	if err := os.WriteFile(alone, data, 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("duckdb", alone+"?access_mode=read_only")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM rows`).Scan(&n); err != nil {
		t.Fatalf("count in %s: %v", filepath.Base(path), err)
	}
	return n
}

func newDBFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fin.tdb")
	database, err := db.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestBackupTakesTheRowsInTheWAL: a backup is one file, so it must hold the
// rows that are still only in the WAL. The database itself must not change.
func TestBackupTakesTheRowsInTheWAL(t *testing.T) {
	for name, create := range map[string]func(string) (string, error){
		"manual": backup.CreateManualBackup,
		"auto":   backup.CreateAutoBackup,
	} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 200, true)
			before, _ := os.ReadFile(path + ".wal")

			bk, err := create(path)
			if err != nil {
				t.Fatalf("backup: %v", err)
			}
			if n := rowsInFileAlone(t, bk); n != 200 {
				t.Errorf("the backup holds %d rows, want the 200 committed rows", n)
			}
			if _, err := os.Stat(bk + ".wal"); err == nil {
				t.Errorf("the backup has a WAL beside it")
			}
			if after, _ := os.ReadFile(path + ".wal"); string(after) != string(before) {
				t.Errorf("the backup changed the WAL of the database")
			}
		})
	}
}

// TestRestoreMovesTheWALAway: the WAL of the current database must not stay
// beside the restored file, or the next open replays it onto that file. The
// safety backup must keep its rows.
func TestRestoreMovesTheWALAway(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	old, err := backup.CreateManualBackup(path) // 1 row
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 200, true) // 201 rows, 200 of them only in the WAL

	safety, err := backup.Restore(path, old)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(path + ".wal"); err == nil {
		t.Errorf("the WAL of the old database is still beside the restored file")
	}
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	err = database.Conn().QueryRow(`SELECT count(*) FROM rows`).Scan(&n)
	_ = database.Close()
	if err != nil || n != 1 {
		t.Errorf("the restored database has %d rows (%v), want the 1 row of the backup", n, err)
	}
	if n := rowsInFileAlone(t, safety); n != 201 {
		t.Errorf("the safety backup holds %d rows, want all 201", n)
	}
}

// TestBackupOfAnIgnoredWAL: DuckDB ignores a WAL that does not read as one
// (all zeros after a crash, or garbage) and leaves it in place for good. Such
// a WAL must not block backups or restores, and must not reach the copy.
func TestBackupOfAnIgnoredWAL(t *testing.T) {
	for name, create := range map[string]func(string) (string, error){
		"manual": backup.CreateManualBackup,
		"auto":   backup.CreateAutoBackup,
	} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 1, false)
			if err := os.WriteFile(path+".wal", make([]byte, 4096), 0o600); err != nil {
				t.Fatal(err)
			}
			bk, err := create(path)
			if err != nil {
				t.Fatalf("backup: %v", err)
			}
			if n := rowsInFileAlone(t, bk); n != 1 {
				t.Errorf("the backup holds %d rows, want 1", n)
			}
			if db.HasWAL(bk) {
				t.Error("the ignored WAL reached the backup")
			}
			if _, err := backup.Restore(path, bk); err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if db.HasWAL(path) {
				t.Error("the ignored WAL is still beside the restored file")
			}
			checkNoLeftovers(t, filepath.Dir(path))
		})
	}
}

// TestRestoreFoldsTheCurrentWALFirst: right before the rename that puts the
// restored copy in place, the old database file alone must hold every
// committed row. A kill at that moment once left the database without the rows
// of its WAL, which Restore had moved aside, and no message said so.
func TestRestoreFoldsTheCurrentWALFirst(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	old, err := backup.CreateManualBackup(path) // 1 row
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 200, true) // 201 rows, 200 of them only in the WAL

	checked := false
	backup.SetBeforeSwap(t, func(dbPath string) {
		checked = true
		if n := rowsInFileAlone(t, dbPath); n != 201 {
			t.Errorf("right before the rename the database file alone holds %d rows, want all 201", n)
		}
		for _, suffix := range db.WALSuffixes {
			if _, err := os.Stat(dbPath + suffix); err == nil {
				t.Errorf("right before the rename %s is beside the database", filepath.Base(dbPath+suffix))
			}
		}
	})
	if _, err := backup.Restore(path, old); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !checked {
		t.Fatal("Restore did not run the hook")
	}
}

// damageWAL flips 64 bytes in the middle of <path>.wal: DuckDB then fails the
// open of path on a checksum.
func damageWAL(t *testing.T, path string) []byte {
	t.Helper()
	wal, err := os.ReadFile(path + ".wal")
	if err != nil {
		t.Fatal(err)
	}
	for i := len(wal) / 2; i < len(wal)/2+64 && i < len(wal); i++ {
		wal[i] ^= 0xff
	}
	if err := os.WriteFile(path+".wal", wal, 0o600); err != nil {
		t.Fatal(err)
	}
	if database, err := db.Open(path); err == nil {
		_ = database.Close()
		t.Skip("DuckDB opened the database with the damaged WAL")
	}
	return wal
}

// TestRestoreWithAnUnreadableWAL: DuckDB cannot apply a damaged WAL, so it
// cannot open the database. A restore from a good backup is then the way back,
// and it must work. The safety backup keeps the file and the raw WAL, so no
// byte of the old state is lost.
func TestRestoreWithAnUnreadableWAL(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path) // 1 row
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 200, true)
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wal := damageWAL(t, path)

	if _, err := backup.CreateManualBackup(path); err == nil || !strings.Contains(err.Error(), "write-ahead log") {
		t.Errorf("a backup of the database with the damaged WAL: %v, want an error about the write-ahead log", err)
	}
	safety, err := backup.Restore(path, good)
	if err != nil {
		t.Fatalf("Restore from a good backup: %v", err)
	}
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open after the restore: %v", err)
	}
	var n int
	err = database.Conn().QueryRow(`SELECT count(*) FROM rows`).Scan(&n)
	_ = database.Close()
	if err != nil || n != 1 {
		t.Errorf("the restored database has %d rows (%v), want the 1 row of the backup", n, err)
	}
	if got, _ := os.ReadFile(safety); !bytes.Equal(got, file) {
		t.Errorf("the safety backup is not the old database file")
	}
	if got, _ := os.ReadFile(safety + ".wal"); !bytes.Equal(got, wal) {
		t.Errorf("the safety backup has no raw copy of the old WAL beside it")
	}

	// The safety backup is listed like any other. A restore of it refuses it
	// while the damaged log is beside it, and says what to do.
	_, err = backup.Restore(path, safety)
	if err == nil || !strings.Contains(err.Error(), "move "+safety+".wal away") {
		t.Errorf("Restore of the raw safety backup: %v, want a refusal that says to move %s.wal away", err, filepath.Base(safety))
	}
}

// TestBackupOfAReadOnlyDatabaseWithAWAL: the copy takes the mode of the
// database only after the fold has written it.
func TestBackupOfAReadOnlyDatabaseWithAWAL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes a file of any mode")
	}
	path := newDBFile(t)
	addRows(t, path, 200, true)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	bk, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatalf("backup of a read-only database with a WAL: %v", err)
	}
	if n := rowsInFileAlone(t, bk); n != 200 {
		t.Errorf("the backup holds %d rows, want 200", n)
	}
	if info, err := os.Stat(bk); err != nil {
		t.Error(err)
	} else if info.Mode().Perm() != 0o444 {
		t.Errorf("backup mode = %v, want -r--r--r--", info.Mode().Perm())
	}
}

// leaveCheckpointWAL leaves <path>.wal.checkpoint beside the closed database
// at path, as a crash or a fatal error during a checkpoint does. DuckDB 1.5
// writes that file only for a commit that lands while a checkpoint runs; the
// end of the checkpoint moves the rows to <path>.wal and removes the file. So
// a second connection commits rows to the table "rows" until the file is
// there. The checkpoint sleeps at its start, so it cannot end before a commit
// lands in it, and then it stops before it writes the header.
func leaveCheckpointWAL(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx := context.Background()
	checkpointer, err := conn.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := conn.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`PRAGMA disable_checkpoint_on_shutdown`,
		// A row in <path>.wal: with an empty WAL, CHECKPOINT has no work.
		`INSERT INTO rows VALUES (2)`,
		`SET debug_checkpoint_sleep_ms = 200`,
		`SET debug_checkpoint_abort = 'before_header'`,
	} {
		if _, err := checkpointer.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	stopped := make(chan error, 1)
	go func() {
		_, err := checkpointer.ExecContext(ctx, `CHECKPOINT`)
		stopped <- err
	}()
	err = commitUntilCheckpointWAL(t, writer, path, stopped)
	if err == nil || !strings.Contains(err.Error(), "Checkpoint aborted before header write") {
		t.Fatalf("CHECKPOINT = %v, want the debug abort before the header", err)
	}
	_ = writer.Close()
	_ = checkpointer.Close()
	_ = conn.Close()
	if _, err := os.Stat(path + ".wal.checkpoint"); err != nil {
		t.Fatalf("DuckDB left no .wal.checkpoint: %v", err)
	}
}

// commitUntilCheckpointWAL commits rows on writer until <path>.wal.checkpoint
// is there, and returns the error of the checkpoint that stopped sends. A
// commit that starts while the checkpoint sleeps waits for it and then goes
// to the checkpoint WAL; a commit before the checkpoint starts goes to
// <path>.wal, so the loop tries again.
func commitUntilCheckpointWAL(t *testing.T, writer *sql.Conn, path string, stopped <-chan error) error {
	t.Helper()
	for i := 0; ; i++ {
		if _, err := os.Stat(path + ".wal.checkpoint"); err == nil {
			return <-stopped
		}
		select {
		case err := <-stopped:
			return err // the checkpoint ended first; the caller reports it
		default:
		}
		if _, err := writer.ExecContext(context.Background(), `INSERT INTO rows VALUES (?)`, 1000+i); err != nil {
			// The stopped checkpoint invalidates the instance, so a commit
			// after it fails. Any other failure shows up as a missing file.
			t.Logf("commit %d: %v", i, err)
			return <-stopped
		}
	}
}

// TestBackupTakesTheCheckpointWAL: a backup must hold every row that an open
// of the database shows, also the rows in <path>.wal.checkpoint, and Restore
// must not leave that file beside the restored database.
func TestBackupTakesTheCheckpointWAL(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path) // 1 row
	if err != nil {
		t.Fatal(err)
	}
	leaveCheckpointWAL(t, path)

	// What an open of the database shows, on a copy of all its files. The
	// list is spelled out, not db.WALSuffixes, so that the test fails when
	// that list misses the checkpoint WAL.
	logs := []string{".wal", ".wal.checkpoint", ".wal.recovery"}
	probe := filepath.Join(t.TempDir(), "probe.tdb")
	for _, suffix := range append([]string{""}, logs...) {
		if _, err := os.Stat(path + suffix); err == nil {
			copyFile(t, path+suffix, probe+suffix)
		}
	}
	conn, err := sql.Open("duckdb", probe)
	if err != nil {
		t.Fatal(err)
	}
	var want int
	err = conn.QueryRow(`SELECT count(*) FROM rows`).Scan(&want)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}

	bk, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if n := rowsInFileAlone(t, bk); n != want {
		t.Errorf("the backup holds %d rows; an open of the database shows %d", n, want)
	}
	if _, err := backup.Restore(path, good); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for _, suffix := range logs {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Errorf("%s of the old database is beside the restored file", filepath.Base(path+suffix))
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreRefusesADatabaseOpenElsewhere: `tmoney db restore` while another
// TMoney process has the database open (a TUI in another terminal) swapped the
// file under that process: its later writes went to the old file and were
// lost. DuckDB's lock on the file shows the other process, so Restore must stop
// before it changes anything.
func TestRestoreRefusesADatabaseOpenElsewhere(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatal(err)
	}

	holdOpenElsewhere(t, path)
	if db.HasWAL(path) {
		t.Fatal("setup: the held database has a WAL; without one, only the lock check can refuse")
	}

	before := dirNames(t, filepath.Dir(path))
	_, err = backup.Restore(path, good)
	if err == nil || !strings.Contains(err.Error(), "open in another program") {
		t.Errorf("Restore while another process has the database open: %v, want a refusal", err)
	}
	if after := dirNames(t, filepath.Dir(path)); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the refused restore changed the folder: %v, was %v", after, before)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestRestoreRefusesAMissingDatabase: Restore opens the database read-write
// to fold its logs, and DuckDB creates a missing file there (and deletes a
// .wal beside it). A restore onto a path with no database (a wrong --file, a
// deleted file, a dangling link) must stop before it writes anything: no new
// file, no empty safety backup that pushes a real one out of retention, and a
// leftover log stays as it was.
func TestRestoreRefusesAMissingDatabase(t *testing.T) {
	for _, leftover := range []string{"no log", ".wal", "a dangling link"} {
		t.Run(leftover, func(t *testing.T) {
			src := newDBFile(t)
			bk, err := backup.CreateManualBackup(src)
			if err != nil {
				t.Fatal(err)
			}
			for range backup.MaxAutoBackups {
				if _, err := backup.CreateAutoBackup(src); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(src); err != nil {
				t.Fatal(err)
			}
			dbPath := src
			log := []byte("fictional log bytes")
			switch leftover {
			case ".wal":
				if err := os.WriteFile(dbPath+".wal", log, 0o600); err != nil {
					t.Fatal(err)
				}
			case "a dangling link":
				dbPath = filepath.Join(filepath.Dir(src), "link.tdb")
				if err := os.Symlink(filepath.Join(filepath.Dir(src), "target.tdb"), dbPath); err != nil {
					t.Skip("no symlinks here:", err)
				}
			}
			before := dirNames(t, filepath.Dir(src))

			safety, err := backup.Restore(dbPath, bk)
			if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "cannot read database file "+dbPath) {
				t.Fatalf("Restore onto a missing database: safety %q, err %v; want a refusal that the database is not there", safety, err)
			}
			if after := dirNames(t, filepath.Dir(src)); strings.Join(after, ",") != strings.Join(before, ",") {
				t.Errorf("the refused restore changed the folder: %v, was %v", after, before)
			}
			if leftover == ".wal" {
				if got, _ := os.ReadFile(dbPath + ".wal"); !bytes.Equal(got, log) {
					t.Errorf("the leftover log changed: %q", got)
				}
			}
		})
	}
}

// TestRestoreRefusesAWriterThatTheFileModeHides: a file made read-only while
// another TMoney has it open fails the fold with "Permission denied", not with
// DuckDB's lock error. Restore swapped the file under that program, and what
// it wrote later went to the old file.
func TestRestoreRefusesAWriterThatTheFileModeHides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes a file of any mode")
	}
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	holdOpenElsewhere(t, path)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	file, _ := os.ReadFile(path)
	before := dirNames(t, filepath.Dir(path))

	_, err = backup.Restore(path, good)
	if err == nil || !strings.Contains(err.Error(), "open in another program") {
		t.Errorf("Restore under a writer behind a read-only mode: %v, want a refusal", err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, file) {
		t.Error("the refused restore changed the database")
	}
	if after := dirNames(t, filepath.Dir(path)); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the refused restore changed the folder: %v, was %v", after, before)
	}
}

// TestRestoreOverADamagedDatabase: a restore is the way back from a database
// that DuckDB cannot open. The fold fails on it, and no other program has it
// open, so Restore must go on with the raw safety backup.
func TestRestoreOverADamagedDatabase(t *testing.T) {
	for name, damage := range map[string]func(t *testing.T, path string){
		"not a DuckDB file": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(strings.Repeat("fictional text, not a database\n", 100)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"bad header checksum": func(t *testing.T, path string) {
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			if _, err := f.WriteAt(bytes.Repeat([]byte{0xa5}, 32), 4096+40); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 1, false)
			good, err := backup.CreateManualBackup(path)
			if err != nil {
				t.Fatal(err)
			}
			addRows(t, path, 200, true)
			damage(t, path)
			damaged, _ := os.ReadFile(path)

			safety, err := backup.Restore(path, good)
			if err != nil {
				t.Fatalf("Restore over a damaged database: %v", err)
			}
			if got, _ := os.ReadFile(safety); !bytes.Equal(got, damaged) {
				t.Error("the safety backup is not the damaged file")
			}
		})
	}
}

// TestHeldElsewhere: Restore asks the operating system whether another
// program has the database open, not DuckDB's error text.
func TestHeldElsewhere(t *testing.T) {
	path := newDBFile(t)
	if held, err := db.HeldElsewhere(path); err != nil || held {
		t.Errorf("a closed database: held = %v, %v; want false", held, err)
	}
	holdOpenElsewhere(t, path)
	if held, err := db.HeldElsewhere(path); err != nil || !held {
		t.Errorf("a database that another process has open: held = %v, %v; want true", held, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatal(err)
		}
		if held, err := db.HeldElsewhere(path); err != nil || !held {
			t.Errorf("the same, with the file made read-only: held = %v, %v; want true", held, err)
		}
	}
	if _, err := db.HeldElsewhere(filepath.Join(t.TempDir(), "gone.tdb")); err == nil {
		t.Error("a missing file: no error")
	}
}

// rowsWithLogs counts the rows of the table "rows" that an open of the
// database at path shows, with its logs, on a read-only copy.
func rowsWithLogs(t *testing.T, path string) int {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe.tdb")
	for _, suffix := range append([]string{""}, db.WALSuffixes...) {
		if _, err := os.Stat(path + suffix); err == nil {
			copyFile(t, path+suffix, probe+suffix)
		}
	}
	conn, err := sql.Open("duckdb", probe+"?access_mode=read_only")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM rows`).Scan(&n); err != nil {
		t.Fatalf("count in %s: %v", filepath.Base(path), err)
	}
	return n
}

// TestRestoreKeepsAnIgnoredWAL: DuckDB ignores a log that it reads as torn,
// such as one whose first entry has a size that points past the end, but its
// bytes still hold the committed rows. The fold of the current database must
// not delete it: the safety backup keeps a raw copy, so a repair of the log
// can bring the rows back.
func TestRestoreKeepsAnIgnoredWAL(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 200, true)
	wal, err := os.ReadFile(path + ".wal")
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(wal[8:], 0xffffffff) // the size of the first entry
	if err := os.WriteFile(path+".wal", wal, 0o600); err != nil {
		t.Fatal(err)
	}
	if n := rowsWithLogs(t, path); n != 1 {
		t.Fatalf("setup: an open shows %d rows with the changed log, want 1 (the log ignored)", n)
	}

	safety, err := backup.Restore(path, good)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, err := os.ReadFile(safety + ".wal"); err != nil || !bytes.Equal(got, wal) {
		t.Errorf("the safety backup has no raw copy of the ignored log (%v)", err)
	}
	if db.HasWAL(path) {
		t.Error("a log of the old database is beside the restored file")
	}
	if _, err := backup.Restore(path, safety); err != nil {
		t.Fatalf("Restore of the safety backup: %v", err)
	}
	if n := rowsInFileAlone(t, path); n != 1 {
		t.Errorf("after a restore of the safety backup the database holds %d rows, want 1", n)
	}
	if _, err := os.Stat(safety + ".wal"); err != nil {
		t.Errorf("the restore of the safety backup removed its log: %v", err)
	}
}

// TestRestoreOverAReadOnlyDatabase: DuckDB cannot open a read-only database
// file for writing, so Restore cannot fold the WAL files into it in place,
// with or without a WAL. The restore must still work, no log of the old
// database may stay beside the restored file (the next open would replay it),
// and the safety backup must hold every committed row: a restore of it is the
// way back.
func TestRestoreOverAReadOnlyDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes a file of any mode")
	}
	for name, walRows := range map[string]int{"with a WAL": 200, "without a WAL": 0} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 1, false)
			good, err := backup.CreateManualBackup(path) // 1 row
			if err != nil {
				t.Fatal(err)
			}
			if walRows > 0 {
				addRows(t, path, walRows, true) // only in the WAL
			}
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
			safety, err := backup.Restore(path, good)
			if err != nil {
				t.Fatalf("Restore over a read-only database: %v", err)
			}
			if n := rowsInFileAlone(t, path); n != 1 {
				t.Errorf("the restored database holds %d rows, want the 1 row of the backup", n)
			}
			for _, suffix := range db.WALSuffixes {
				if _, err := os.Stat(path + suffix); err == nil {
					t.Errorf("%s of the old database is beside the restored file", filepath.Base(path+suffix))
				}
			}
			if _, err := backup.Restore(path, safety); err != nil {
				t.Fatalf("Restore of the safety backup: %v", err)
			}
			if n := rowsInFileAlone(t, path); n != 1+walRows {
				t.Errorf("after a restore of the safety backup the database holds %d rows, want all %d", n, 1+walRows)
			}
		})
	}
}

// TestRestoreHintNamesEveryLog: DuckDB reads <backup>.wal.checkpoint too, and
// its error can name that file. The hint of a refusal names every log beside
// the backup: moving them all away leaves the file alone, which restores.
func TestRestoreHintNamesEveryLog(t *testing.T) {
	src := newDBFile(t)
	addRows(t, src, 1, false)
	addRows(t, src, 200, true)
	damageWAL(t, src)
	dbPath := newDBFile(t)
	bk := filepath.Join(filepath.Dir(dbPath), "fin.tdb"+backup.ManualBackupInfix+"2026-01-02T03-04-05")
	copyFile(t, src, bk)
	copyFile(t, src+".wal", bk+".wal")
	if err := os.WriteFile(bk+".wal.checkpoint", []byte("fictional log"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := backup.Restore(dbPath, bk)
	if want := "move " + bk + ".wal and " + bk + ".wal.checkpoint away"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Restore of a backup with a damaged log: %v, want the hint %q", err, want)
	}
}

// TestRestoreStopsWhenItCannotCheckTheLock: after a failed fold, Restore asks
// the operating system whether another program has the database open. When
// that check fails (a file system with no locks), it cannot tell, and a swap
// under such a program loses its later writes: it must stop.
func TestRestoreStopsWhenItCannotCheckTheLock(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 200, true)
	damageWAL(t, path) // the fold fails
	backup.SetHeldElsewhere(t, func(string) (bool, error) {
		return false, errors.New("fictional: this file system has no locks")
	})
	file, _ := os.ReadFile(path)
	before := dirNames(t, filepath.Dir(path))

	_, err = backup.Restore(path, good)
	if err == nil || !strings.Contains(err.Error(), "cannot check whether another program has the database open") {
		t.Errorf("Restore when the lock check fails: %v, want a refusal", err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, file) {
		t.Error("the refused restore changed the database")
	}
	if after := dirNames(t, filepath.Dir(path)); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the refused restore changed the folder: %v, was %v", after, before)
	}
}

// TestRestoreStopsWhenTheDatabaseChanges: a program that opens the database
// after the lock check and writes to it during the safety backup changes the
// file or its log. The safety backup does not have that write, and the swap
// would lose it (the moved log was deleted), so Restore must stop.
func TestRestoreStopsWhenTheDatabaseChanges(t *testing.T) {
	for name, leaveWAL := range map[string]bool{"a write to the file": false, "a write to its log": true} {
		t.Run(name, func(t *testing.T) {
			path := newDBFile(t)
			addRows(t, path, 1, false)
			good, err := backup.CreateManualBackup(path)
			if err != nil {
				t.Fatal(err)
			}
			backup.SetAfterSafetyBackup(t, func(file string) { addRows(t, file, 1, leaveWAL) })

			safety, err := backup.Restore(path, good)
			if err == nil || !strings.Contains(err.Error(), "changed during the restore") {
				t.Fatalf("Restore while another program writes: %v, want a refusal", err)
			}
			if n := rowsWithLogs(t, path); n != 2 {
				t.Errorf("after the refused restore the database shows %d rows, want the 2 with the new one", n)
			}
			if _, err := os.Stat(safety); err != nil {
				t.Errorf("the refusal names no safety backup that exists: %q (%v)", safety, err)
			}
		})
	}
}

// TestRestoreStopsWhenAProgramOpensTheDatabase: a program that opens the
// database after the lock check, and does not write yet, leaves the file and
// its logs as they were. After the swap it writes to the old file, and its log
// goes beside the restored file, where the next open replays it. Restore must
// check the lock again right before the swap.
func TestRestoreStopsWhenAProgramOpensTheDatabase(t *testing.T) {
	path := newDBFile(t)
	addRows(t, path, 1, false)
	good, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	addRows(t, path, 5, false)
	backup.SetAfterSafetyBackup(t, func(file string) { holdOpenElsewhere(t, file) })

	_, err = backup.Restore(path, good)
	if err == nil || !strings.Contains(err.Error(), "open in another program") {
		t.Errorf("Restore while another program opened the database: %v, want a refusal", err)
	}
	if n := rowsInFileAlone(t, path); n != 6 {
		t.Errorf("after the refused restore the database holds %d rows, want its 6", n)
	}
}
