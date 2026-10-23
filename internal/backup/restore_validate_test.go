package backup_test

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
	if strings.Contains(msg, ".partial-") {
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
		if strings.Contains(n, ".partial-") || strings.HasSuffix(n, ".restoring") || strings.HasPrefix(n, "tmoney-restore-") {
			t.Errorf("Restore left %s", n)
		}
	}
}
