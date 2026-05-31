package db_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	dbpkg "github.com/haskovec/tmoney/internal/db"
)

func TestDBCreate_CreatesNewDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb.tdb")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "create", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db create): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Created database:") {
		t.Errorf("output should confirm creation, got: %s", out)
	}
	if !strings.Contains(out, dbPath) {
		t.Errorf("output should contain path, got: %s", out)
	}

	database, err := dbpkg.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open created database: %v", err)
	}
	database.Close()
}

func TestDBCreate_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "existing.tdb")

	database, err := dbpkg.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: failed to create initial database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"db", "create", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("cli.ExecuteWith(db create) on existing file should return error")
	}
}

func TestDBCreate_AddsExtension(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb") // no .tdb extension

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "create", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db create): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, ".tdb") {
		t.Errorf("output should show .tdb extension was added, got: %s", out)
	}

	database, err := dbpkg.Open(dbPath + ".tdb")
	if err != nil {
		t.Fatalf("failed to open created database with .tdb extension: %v", err)
	}
	database.Close()
}

// TestDBCreate_ThenListAccounts verifies a db created via the new Cobra
// command is functional — the legacy --list-accounts dispatcher can read
// it. (When --list-accounts migrates, this test will be updated to use
// the new account list verb.)
func TestDBCreate_ThenListAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "create", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db create): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := cli.ExecuteWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list) after db create: %v", err)
	}

	if !strings.Contains(stdout.String(), "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", stdout.String())
	}
}

func TestDBCreate_MissingPath(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"db", "create"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(db create) without path should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestDBCmd_HelpListsCreate(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "create") {
		t.Errorf("expected `db --help` to list `create`; got:\n%s", stdout.String())
	}
}

func TestDBCreate_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"db", "create", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(db create --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "Create") {
		t.Errorf("expected `db create --help` to describe the command; got:\n%s", stdout.String())
	}
}
