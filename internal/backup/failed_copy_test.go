package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFailedCopyKeepsEarlierBackupOfSameName: backup names have one-second
// resolution, so a second backup in the same second has the name of the first.
// If the second copy fails partway, the first backup must still be whole.
//
// The second copy fails on its first read: its source is a directory, which
// os.Open accepts and read(2) refuses with EISDIR. Only the internal API that
// both the old and the new code have is used, so the test runs on both.
func TestFailedCopyKeepsEarlierBackupOfSameName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fin.tdb")
	content := bytes.Repeat([]byte("fictional ledger row\n"), 500)
	if err := os.WriteFile(dbPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	first, err := createManualBackupAt(dbPath, now)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// The source now fails to read.
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = createManualBackupAt(dbPath, now)
	t.Logf("second backup in the same second: %v", err)
	if err == nil {
		t.Fatal("a copy from a directory succeeded")
	}

	got, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("the first backup is gone: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("the failed copy destroyed the first backup of the same name: it has %d bytes, want %d", len(got), len(content))
	}
}
