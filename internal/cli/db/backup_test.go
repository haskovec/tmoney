package db_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	dbpkg "github.com/haskovec/tmoney/internal/db"
)

func TestDBBackup_CreatesManualBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "backup", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db backup): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Backup created:") {
		t.Errorf("expected 'Backup created:' in output, got: %s", out)
	}

	// The reported backup path should exist on disk and contain the manual infix.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-1]
	backupPath := strings.TrimPrefix(last, "Backup created: ")
	if _, statErr := os.Stat(backupPath); statErr != nil {
		t.Errorf("reported backup path does not exist: %v", statErr)
	}
	if !strings.Contains(backupPath, ".manual-backup.") {
		t.Errorf("expected manual-backup infix in path, got: %s", backupPath)
	}
}

func TestDBBackup_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "backup", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db backup -f): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Backup created:") {
		t.Errorf("expected 'Backup created:' in output, got: %s", stdout.String())
	}
}

func TestDBBackup_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "backup"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db backup) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestDBBackup_NonexistentFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "backup", "--file", "/no/such/file.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db backup) on missing source file should return error")
	}
	if !strings.Contains(err.Error(), "failed to create backup") {
		t.Errorf("expected wrapped backup error, got: %v", err)
	}
}

func TestDBBackup_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "backup", "--file", "x.tdb", "extra"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db backup ... extra) should return error")
	}
}

func TestDBCmd_HelpListsBackup(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "backup") {
		t.Errorf("expected `db --help` to list `backup`; got:\n%s", stdout.String())
	}
}

func TestDBBackup_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "backup", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db backup --help): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "backup") {
		t.Errorf("expected `db backup --help` to describe the command; got:\n%s", out)
	}
}
