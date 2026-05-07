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

func TestAccountShow_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "show", "Test Account"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account show) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestAccountShow_MissingNameArg(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "show", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account show) without positional name should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestAccountShow_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"account", "show", "Nonexistent Account", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account show) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestAccountShow_ValidAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)
	acct := account.NewAccount("Test Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	acct.SetInstitution("Chase Bank")
	acct.SetAccountNumber("1234567890")
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "show", "Test Checking", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account show): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"ACCOUNT: Test Checking", "Checking", "USD", "Chase Bank", "****7890", "Current Balance", "Active"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestAccountShow_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)
	acct := account.NewAccount("Quick", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "show", "Quick", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account show -f): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "ACCOUNT: Quick") {
		t.Errorf("expected account header, got: %s", stdout.String())
	}
}

func TestAccountShow_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"account", "show", "A", "B", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(account show ... extra) should return error")
	}
}

func TestAccountCmd_HelpListsShow(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "show") {
		t.Errorf("expected `account --help` to list `show`; got:\n%s", stdout.String())
	}
}

func TestAccountShow_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"account", "show", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(account show --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "show") {
		t.Errorf("expected `account show --help` to describe the command; got:\n%s", stdout.String())
	}
}
