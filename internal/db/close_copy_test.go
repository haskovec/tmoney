package db

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// copyFileTo copies ONLY the main database file, exactly as backup.copyFile does.
func copyFileTo(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create copy: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy: %v", err)
	}
}

func insertProbe(t *testing.T, database *DB, key string) {
	t.Helper()
	if err := database.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(`INSERT INTO _metadata (key, value) VALUES (?, 'present')`, key)
		return err
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}

func assertProbe(t *testing.T, path, key string) {
	t.Helper()
	copied, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", path, err)
	}
	defer copied.Close()
	got, err := copied.GetMetadata(key)
	if err != nil {
		t.Fatalf("%s is missing the committed row %q: %v", path, key, err)
	}
	if got != "present" {
		t.Errorf("%s = %q, want %q", key, got, "present")
	}
}

// TestClose_FileCopyIncludesCommittedWrites guards the contract every backup in
// the app relies on: after Close, a plain copy of the database FILE — nothing
// else, no .wal — opens as a database that contains every committed write.
// On Windows the copy cannot even be opened before Close.
func TestClose_FileCopyIncludesCommittedWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tdb")

	database, err := Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	insertProbe(t, database, "close_probe")

	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	copyPath := filepath.Join(dir, "copy.tdb")
	copyFileTo(t, path, copyPath)
	assertProbe(t, copyPath, "close_probe")
}

// TestClose_IsTerminal: a write after an intentional Close must fail, and must
// NOT reopen the file through healAfterFatal. Reopening would race the backup
// or restore that closed the handle.
func TestClose_IsTerminal(t *testing.T) {
	dir := t.TempDir()
	database, err := Create(filepath.Join(dir, "test.tdb"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closedPool := database.Conn()

	err = database.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(`UPDATE _metadata SET value = 'x' WHERE key = 'default_currency'`)
		return err
	})
	if err == nil {
		t.Fatal("WithTx() on a closed DB should fail")
	}
	if database.Conn() != closedPool {
		t.Fatal("WithTx() on a closed DB reopened the file; Close must be terminal")
	}
	if !database.isClosed() {
		t.Fatal("closed flag was cleared without a reopen")
	}
	if err := database.reconnect(); !errors.Is(err, ErrClosed) {
		t.Fatalf("reconnect() on a closed DB = %v, want ErrClosed", err)
	}
}

// TestWithFileClosed: the file is copyable inside fn, the same handle works
// again afterwards, and fn's error survives the reopen.
func TestWithFileClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tdb")
	database, err := Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer database.Close()
	insertProbe(t, database, "before")

	copyPath := filepath.Join(dir, "copy.tdb")
	err = database.WithFileClosed(func(p string) error {
		if p != path {
			t.Errorf("fn path = %s, want %s", p, path)
		}
		if !database.isClosed() {
			t.Error("database should be closed inside fn")
		}
		copyFileTo(t, p, copyPath)
		return nil
	})
	if err != nil {
		t.Fatalf("WithFileClosed() error = %v", err)
	}
	assertProbe(t, copyPath, "before")

	// The SAME handle writes again after the reopen.
	if database.isClosed() {
		t.Fatal("database should be open again after WithFileClosed")
	}
	insertProbe(t, database, "after")

	// fn's error is returned, and the handle is still reopened.
	sentinel := errors.New("copy failed")
	if err := database.WithFileClosed(func(string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("WithFileClosed() = %v, want %v", err, sentinel)
	}
	insertProbe(t, database, "after_failure")
}
