package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
)

func TestDBListBackups_NoBackups(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"db", "list-backups", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(db list-backups): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "BACKUPS:") {
		t.Errorf("expected 'BACKUPS:' header in output, got: %s", out)
	}
	if !strings.Contains(out, "No backups found.") {
		t.Errorf("expected 'No backups found.' in output, got: %s", out)
	}
}

func TestDBListBackups_WithBackups(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	if _, err := backup.CreateManualBackup(dbPath); err != nil {
		t.Fatalf("setup: CreateManualBackup: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"db", "list-backups", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(db list-backups): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "BACKUPS:") {
		t.Errorf("expected 'BACKUPS:' header in output, got: %s", out)
	}
	if !strings.Contains(out, "Date") || !strings.Contains(out, "Size") || !strings.Contains(out, "Type") {
		t.Errorf("expected table headers Date/Size/Type, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "manual") {
		t.Errorf("expected manual backup type listed, got: %s", out)
	}
	if !strings.Contains(out, "1 backup(s) found") {
		t.Errorf("expected backup count summary, got: %s", out)
	}
}

func TestDBListBackups_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"db", "list-backups", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(db list-backups -f): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "BACKUPS:") {
		t.Errorf("expected 'BACKUPS:' in output, got: %s", stdout.String())
	}
}

func TestDBListBackups_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"db", "list-backups"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(db list-backups) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestDBListBackups_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"db", "list-backups", "--file", "x.tdb", "extra"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(db list-backups ... extra) should return error")
	}
}

func TestDBCmd_HelpListsListBackups(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"db", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(db --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list-backups") {
		t.Errorf("expected `db --help` to list `list-backups`; got:\n%s", stdout.String())
	}
}

func TestDBListBackups_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"db", "list-backups", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(db list-backups --help): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "list-backups") {
		t.Errorf("expected `db list-backups --help` to describe the command; got:\n%s", out)
	}
}
