package db

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestClose_FileCopyIncludesCommittedWrites guards the contract every backup in
// the app relies on: after Close, a plain copy of the database FILE — nothing
// else, no .wal — opens as a database that contains every committed write.
//
// It is also the only order that works at all on Windows, where DuckDB's open
// handle makes os.Open on the file fail; that is why the copy here runs after
// Close rather than after a CHECKPOINT on the open handle.
func TestClose_FileCopyIncludesCommittedWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tdb")

	database, err := Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := database.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(`INSERT INTO _metadata (key, value) VALUES ('close_probe', 'present')`)
		return err
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Copy ONLY the main file, exactly as backup.copyFile does.
	copyPath := filepath.Join(dir, "copy.tdb")
	src, err := os.Open(path)
	if err != nil {
		t.Fatalf("open source after Close: %v", err)
	}
	dst, err := os.Create(copyPath)
	if err != nil {
		t.Fatalf("create copy: %v", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("copy: %v", err)
	}
	_ = src.Close()
	_ = dst.Close()

	copied, err := Open(copyPath)
	if err != nil {
		t.Fatalf("Open(copy) error = %v", err)
	}
	defer copied.Close()

	got, err := copied.GetMetadata("close_probe")
	if err != nil {
		t.Fatalf("the copy is missing the committed row: %v", err)
	}
	if got != "present" {
		t.Errorf("close_probe = %q, want %q", got, "present")
	}
}
