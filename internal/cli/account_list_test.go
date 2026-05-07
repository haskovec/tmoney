package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

func TestAccountList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "list"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestAccountList_FileNotFound(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "list", "--file", "/nonexistent/path.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account list) with nonexistent file should return error")
	}
}

func TestAccountList_NoAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "No accounts found") {
		t.Errorf("expected 'No accounts found', got: %s", stdout.String())
	}
}

func TestAccountList_WithAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)
	acct := account.NewAccount("Test Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"ACCOUNTS", "Test Checking", "Checking", "USD"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestAccountList_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "list", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list -f): %v\nstderr=%s", err, stderr)
	}
}

func TestAccountList_IncludeClosed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)

	active := account.NewAccount("Active Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(active); err != nil {
		t.Fatalf("setup: create active: %v", err)
	}
	closed := account.NewAccount("Closed Savings", account.TypeSavings, "USD", types.MustNewMoney("0"), types.Today())
	closed.Close()
	if err := repo.Create(closed); err != nil {
		t.Fatalf("setup: create closed: %v", err)
	}
	database.Close()

	// Without --include-closed: only active
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "Active Checking") {
		t.Errorf("expected active account in output, got: %s", out)
	}
	if strings.Contains(out, "Closed Savings") {
		t.Errorf("expected closed account hidden without --include-closed, got: %s", out)
	}

	// With --include-closed: both shown
	stdout.Reset()
	stderr.Reset()
	if err := executeWith([]string{"account", "list", "--file", dbPath, "--include-closed"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list --include-closed): %v\nstderr=%s", err, stderr)
	}
	out = stdout.String()
	if !strings.Contains(out, "Active Checking") {
		t.Errorf("expected active account in output, got: %s", out)
	}
	if !strings.Contains(out, "Closed Savings") {
		t.Errorf("expected closed account with --include-closed, got: %s", out)
	}
}

func TestAccountList_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "list", "--file", "x.tdb", "extra"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account list ... extra) should return error")
	}
}

func TestAccountCmd_HelpListsList(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `account --help` to list `list`; got:\n%s", stdout.String())
	}
}

func TestAccountList_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `account list --help` to describe the command; got:\n%s", stdout.String())
	}
}
