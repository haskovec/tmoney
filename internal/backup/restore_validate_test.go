package backup_test

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2" // registers the "duckdb" driver for the foreign file
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// These tests use real TMoney databases with fictional data. They use only the
// exported API, so they run unchanged against the code before the fix.

const marker = "fictional_marker"

// newMarkedDB writes a closed TMoney database at dir/name whose _metadata holds
// marker=value, and returns its path.
func newMarkedDB(t *testing.T, dir, name, value string) string {
	t.Helper()
	database, path := dbtest.NewFileIn(t, dir, name)
	if err := database.SetMetadata(marker, value); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

func readMarker(t *testing.T, path string) string {
	t.Helper()
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open(%s) after the restore: %v", filepath.Base(path), err)
	}
	defer func() { _ = database.Close() }()
	v, err := database.GetMetadata(marker)
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	return v
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// badBackups are files that must never replace a database. Each one writes
// its file at path, given the bytes of a good database.
var badBackups = map[string]func(t *testing.T, path string, good []byte){
	"empty (killed right after create)": func(t *testing.T, path string, _ []byte) {
		writeFile(t, path, nil)
	},
	"first half (killed halfway)": func(t *testing.T, path string, good []byte) {
		writeFile(t, path, good[:len(good)/2])
	},
	"all but the last byte": func(t *testing.T, path string, good []byte) {
		writeFile(t, path, good[:len(good)-1])
	},
	"not a DuckDB file": func(t *testing.T, path string, _ []byte) {
		writeFile(t, path, bytes.Repeat([]byte("fictional notes\n"), 1000))
	},
	"a DuckDB file of another app": func(t *testing.T, path string, _ []byte) {
		conn, err := sql.Open("duckdb", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`CREATE TABLE notes AS SELECT 'fictional' AS s`); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	},
	"a TMoney file with no schema version": func(t *testing.T, path string, _ []byte) {
		conn, err := sql.Open("duckdb", path)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`CREATE TABLE _metadata (key TEXT, value TEXT)`,
			`INSERT INTO _metadata VALUES ('app_identifier', 'tmoney'), ('schema_version', NULL)`,
		} {
			if _, err := conn.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	},
	"a TMoney file from a newer app version": func(t *testing.T, path string, _ []byte) {
		database, _ := dbtest.NewFileIn(t, filepath.Dir(path), filepath.Base(path))
		if err := database.SetMetadata("schema_version", "999"); err != nil {
			t.Fatal(err)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	},
}

// checkRefusal checks the text of Restore's refusal of the bad backup at path:
// it names that backup, and never Restore's copy of it, which is gone by the
// time the user reads the message. A backup of a newer app version is not
// called invalid: the user only needs a newer TMoney.
func checkRefusal(t *testing.T, kind string, err error, path string) {
	t.Helper()
	msg := err.Error()
	want := "not a valid TMoney database"
	if strings.Contains(kind, "newer app version") {
		want = "newer version of TMoney"
	}
	if !strings.Contains(msg, want) {
		t.Errorf("refusal = %q, want it to say %q", msg, want)
	}
	if !strings.Contains(msg, path) {
		t.Errorf("refusal = %q, want it to name the backup %s", msg, path)
	}
	if strings.Contains(msg, "partial-") {
		t.Errorf("refusal = %q names Restore's deleted copy, not the backup", msg)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreRefusesBadBackup: Restore must not put a truncated or foreign file
// in place of a good database. Such files exist: a backup that a crash cut
// short before backups were atomic, or a wrong file passed to `db restore`.
func TestRestoreRefusesBadBackup(t *testing.T) {
	for name, write := range badBackups {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := newMarkedDB(t, dir, "fin.tdb", "current")
			good, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			bad := dbPath + backup.ManualBackupInfix + "2026-01-01T00-00-00"
			write(t, bad, good)
			before := names(t, dir)

			safety, err := backup.Restore(dbPath, bad)
			t.Logf("Restore: safety=%q err=%v", filepath.Base(safety), err)
			if err == nil {
				t.Errorf("Restore took the bad backup without an error")
			} else {
				checkRefusal(t, name, err, bad)
			}

			now, _ := os.ReadFile(dbPath)
			if !bytes.Equal(now, good) {
				t.Errorf("the database changed: %d bytes, was %d", len(now), len(good))
			}
			if after := names(t, dir); strings.Join(after, ",") != strings.Join(before, ",") {
				t.Errorf("the folder changed: %v, was %v", after, before)
			}
			if reopened, err := db.Open(dbPath); err != nil {
				t.Errorf("db.Open after the refused restore: %v", err)
			} else {
				_ = reopened.Close()
			}
		})
	}
}

// TestRestoreRefusesBadBackupWhileOpen is the TUI path: Restore runs inside
// WithFileClosed, which reopens the same handle afterwards without the checks
// of db.Open. A refused backup must leave the handle open on the old data.
func TestRestoreRefusesBadBackupWhileOpen(t *testing.T) {
	for name, write := range badBackups {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := newMarkedDB(t, dir, "fin.tdb", "current")
			good, _ := os.ReadFile(dbPath)
			bad := dbPath + backup.ManualBackupInfix + "2026-01-01T00-00-00"
			write(t, bad, good)

			database, err := db.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()

			err = database.WithFileClosed(func(path string) error {
				_, err := backup.Restore(path, bad)
				return err
			})
			t.Logf("WithFileClosed(Restore): %v", err)
			if err == nil {
				t.Errorf("WithFileClosed(Restore) took the bad backup without an error")
			} else {
				checkRefusal(t, name, err, bad)
			}
			v, err := database.GetMetadata(marker)
			t.Logf("then the open handle reads %s: %q, %v", marker, v, err)
			if err != nil || v != "current" {
				t.Errorf("after the refused restore the open handle reads %q, %v; want %q", v, err, "current")
			}
		})
	}
}

// TestRestoreGoodBackup: a complete backup still restores, and the safety
// backup holds the state from before.
func TestRestoreGoodBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := newMarkedDB(t, dir, "fin.tdb", "old")
	saved, err := backup.CreateManualBackup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetMetadata(marker, "new"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	safety, err := backup.Restore(dbPath, saved)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := readMarker(t, dbPath); got != "old" {
		t.Errorf("restored database has marker %q, want %q", got, "old")
	}
	if got := readMarker(t, safety); got != "new" {
		t.Errorf("safety backup has marker %q, want %q", got, "new")
	}
	for _, n := range names(t, dir) {
		if strings.Contains(n, "partial-") || strings.HasSuffix(n, ".restoring") || strings.HasPrefix(n, "tmoney-restore-") {
			t.Errorf("Restore left %s", n)
		}
	}
}

// TestRestoreUnreadableBackup: a good backup that this user cannot read (one
// that another user made of a database with mode 0600, or one in a folder
// without search permission) is not a bad backup. The refusal must say that the file
// cannot be read, not that it is not a TMoney database, and change nothing.
func TestRestoreUnreadableBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of any mode")
	}
	for name, lock := range map[string]func(t *testing.T, saved string){
		"file mode 000":                    func(t *testing.T, saved string) { chmodUntilCleanup(t, saved, 0, 0o600) },
		"folder without search permission": func(t *testing.T, saved string) { chmodUntilCleanup(t, filepath.Dir(saved), 0o600, 0o700) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := newMarkedDB(t, dir, "fin.tdb", "current")
			good, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "saved"), 0o700); err != nil {
				t.Fatal(err)
			}
			saved := filepath.Join(dir, "saved", "fin.tdb"+backup.ManualBackupInfix+"2026-01-01T00-00-00")
			writeFile(t, saved, good)
			lock(t, saved)
			before := names(t, dir)

			_, err = backup.Restore(dbPath, saved)
			if err == nil {
				t.Fatal("Restore took a backup that it cannot read")
			}
			msg := err.Error()
			if !strings.Contains(msg, "cannot read backup file "+saved) {
				t.Errorf("refusal = %q, want it to say that it cannot read the backup %s", msg, saved)
			}
			if strings.Contains(msg, "not a valid TMoney database") {
				t.Errorf("refusal = %q calls a good backup invalid", msg)
			}
			if now, _ := os.ReadFile(dbPath); !bytes.Equal(now, good) {
				t.Errorf("the database changed: %d bytes, was %d", len(now), len(good))
			}
			if after := names(t, dir); strings.Join(after, ",") != strings.Join(before, ",") {
				t.Errorf("the folder changed: %v, was %v", after, before)
			}
		})
	}
}

// chmodUntilCleanup gives path the mode locked, and the mode unlocked when the
// test ends, so that t.TempDir can delete it.
func chmodUntilCleanup(t *testing.T, path string, locked, unlocked os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, locked); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, unlocked) })
}

// TestRestoreBackupOpenElsewhere: a good backup that another program has open
// (a second TMoney that opened the backup file to look at it) fails the
// attach on DuckDB's lock. The refusal must say so, not that the backup is not
// a TMoney database, and change nothing.
func TestRestoreBackupOpenElsewhere(t *testing.T) {
	dir := t.TempDir()
	dbPath := newMarkedDB(t, dir, "fin.tdb", "current")
	saved, err := backup.CreateManualBackup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	holdOpenElsewhere(t, saved)
	before := names(t, dir)

	_, err = backup.Restore(dbPath, saved)
	if err == nil {
		t.Fatal("Restore took a backup that another program has open")
	}
	msg := err.Error()
	if !strings.Contains(msg, "backup file "+saved+" is open in another program") {
		t.Errorf("refusal = %q, want it to say that another program has the backup %s open", msg, saved)
	}
	if strings.Contains(msg, "not a valid TMoney database") {
		t.Errorf("refusal = %q calls a good backup invalid", msg)
	}
	if now, _ := os.ReadFile(dbPath); !bytes.Equal(now, good) {
		t.Errorf("the database changed: %d bytes, was %d", len(now), len(good))
	}
	if after := names(t, dir); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("the folder changed: %v, was %v", after, before)
	}
}

// holdOpenEnv names the database that TestHoldOpenChild keeps open.
const holdOpenEnv = "TMONEY_HOLD_OPEN_DB"

// TestHoldOpenChild is the child process of holdOpenElsewhere: another TMoney
// process with a database open. It is skipped in a normal run.
func TestHoldOpenChild(t *testing.T) {
	path := os.Getenv(holdOpenEnv)
	if path == "" {
		t.Skip("helper process")
	}
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	fmt.Println("open")
	time.Sleep(time.Minute) // the parent kills this process
}

// holdOpenElsewhere starts a child process that opens the database at path,
// as another TMoney would, and returns when it has the file open. The child
// ends with the test.
func holdOpenElsewhere(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHoldOpenChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), holdOpenEnv+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	opened := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			if scanner.Text() == "open" {
				opened <- true
				return
			}
		}
		opened <- false
	}()
	select {
	case ok := <-opened:
		if !ok {
			t.Fatal("the child did not open the database")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the child did not open the database in 30 s")
	}
}
