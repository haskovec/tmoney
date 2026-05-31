package db_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/cli"
	dbpkg "github.com/haskovec/tmoney/internal/db"
)

func TestDBRestore_RestoresFromBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	backupPath, err := backup.CreateManualBackup(dbPath)
	if err != nil {
		t.Fatalf("setup: CreateManualBackup: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(backupPath) })

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "restore", "--file", dbPath, backupPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db restore): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Restoring from:") {
		t.Errorf("expected 'Restoring from:' in output, got: %s", out)
	}
	if !strings.Contains(out, backupPath) {
		t.Errorf("expected backup path in output, got: %s", out)
	}
	if !strings.Contains(out, "Restore complete.") {
		t.Errorf("expected 'Restore complete.' in output, got: %s", out)
	}
	if !strings.Contains(out, "Backup created:") {
		t.Errorf("expected safety backup mention in output, got: %s", out)
	}
}

func TestDBRestore_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	backupPath, err := backup.CreateManualBackup(dbPath)
	if err != nil {
		t.Fatalf("setup: CreateManualBackup: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(backupPath) })

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "restore", "-f", dbPath, backupPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db restore -f): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Restore complete.") {
		t.Errorf("expected 'Restore complete.' in output, got: %s", stdout.String())
	}
}

func TestDBRestore_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "restore", "/some/backup.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db restore) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestDBRestore_MissingBackupArg(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "restore", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db restore) without backup path should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestDBRestore_NonexistentBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "finances.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"db", "restore", "--file", dbPath, "/no/such/backup.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db restore) on missing backup should return error")
	}
	if !strings.Contains(err.Error(), "failed to restore") {
		t.Errorf("expected wrapped restore error, got: %v", err)
	}
}

func TestDBRestore_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "restore", "--file", "x.tdb", "a.tdb", "b.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db restore ... extra) should return error")
	}
}

func TestDBCmd_HelpListsRestore(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "restore") {
		t.Errorf("expected `db --help` to list `restore`; got:\n%s", stdout.String())
	}
}

func TestDBRestore_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "restore", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db restore --help): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "restore") {
		t.Errorf("expected `db restore --help` to describe the command; got:\n%s", out)
	}
}
