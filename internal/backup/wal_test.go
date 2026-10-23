package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	addRows(t, path, 200, true)         // 201 rows, 200 of them only in the WAL
	time.Sleep(1100 * time.Millisecond) // a new second, so a new backup name

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
	path := newDBFile(t)
	addRows(t, path, 1, false)
	if err := os.WriteFile(path+".wal", make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	bk, err := backup.CreateManualBackup(path)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if n := rowsInFileAlone(t, bk); n != 1 {
		t.Errorf("the backup holds %d rows, want 1", n)
	}
	if _, err := backup.Restore(path, bk); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(path + ".wal"); err == nil {
		t.Error("the ignored WAL is still beside the restored file")
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
	if info, err := os.Stat(bk); err != nil || info.Mode().Perm() != 0o444 {
		t.Errorf("backup mode = %v (%v), want -r--r--r--", info.Mode().Perm(), err)
	}
}

// leaveCheckpointWAL stops a checkpoint of the closed database at path while a
// second connection commits rows, the way a crash or a fatal error during a
// checkpoint does. DuckDB then keeps committed rows in <path>.wal.checkpoint
// as well as in <path>.wal. It reports whether that file is there.
func leaveCheckpointWAL(t *testing.T, path string) bool {
	t.Helper()
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, q := range []string{
		`PRAGMA disable_checkpoint_on_shutdown`,
		`CREATE TABLE fill (id BIGINT, s VARCHAR)`,
		`INSERT INTO fill SELECT range, md5(range::VARCHAR) FROM range(30000)`,
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ctx := context.Background()
	checkpointer, err := conn.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := conn.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkpointer.ExecContext(ctx, `SET debug_checkpoint_abort = 'before_header'`); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := writer.ExecContext(ctx, `INSERT INTO rows VALUES (?)`, 1000+i); err != nil {
				return
			}
		}
	}()
	time.Sleep(20 * time.Millisecond)
	_, err = checkpointer.ExecContext(ctx, `CHECKPOINT`)
	t.Logf("the stopped checkpoint: %v", err)
	close(stop)
	<-done
	_ = checkpointer.Close()
	_ = writer.Close()
	_ = conn.Close()
	_, err = os.Stat(path + ".wal.checkpoint")
	return err == nil
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
	if !leaveCheckpointWAL(t, path) {
		t.Skip("DuckDB left no .wal.checkpoint this run")
	}

	// What an open of the database shows, on a copy of all its files.
	probe := filepath.Join(t.TempDir(), "probe.tdb")
	for _, suffix := range append([]string{""}, db.WALSuffixes...) {
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
	for _, suffix := range db.WALSuffixes {
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
