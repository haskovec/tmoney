package db_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	dbpkg "github.com/haskovec/tmoney/internal/db"
)

func TestDBReindex_RebuildsIndexes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "reindex", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db reindex): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "Rebuilt") || !strings.Contains(stdout.String(), "database indexes") {
		t.Errorf("expected success message, got: %s", stdout.String())
	}
}

func TestDBReindex_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "reindex", "--file", ""}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error when --file is empty")
	}
}
