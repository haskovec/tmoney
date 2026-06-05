package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func TestAccountList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "list"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestAccountList_FileNotFound(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "list", "--file", "/nonexistent/path.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account list) with nonexistent file should return error")
	}
}

func TestAccountList_NoAccounts(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "empty.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "No accounts found") {
		t.Errorf("expected 'No accounts found', got: %s", stdout.String())
	}
}

func TestAccountList_WithAccounts(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Test Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"ACCOUNTS", "Test Checking", "Checking", "USD"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestAccountList_ShortFileFlag(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "list", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list -f): %v\nstderr=%s", err, stderr)
	}
}

func TestAccountList_IncludeClosed(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)

	active := accountdom.NewAccount("Active Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(active); err != nil {
		t.Fatalf("setup: create active: %v", err)
	}
	closed := accountdom.NewAccount("Closed Savings", accountdom.TypeSavings, "USD", types.MustNewMoney("0"), types.Today())
	closed.Close()
	if err := repo.Create(closed); err != nil {
		t.Fatalf("setup: create closed: %v", err)
	}
	database.Close()

	// Without --include-closed: only active
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list): %v\nstderr=%s", err, stderr)
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
	if err := cli.ExecuteWith([]string{"account", "list", "--file", dbPath, "--include-closed"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list --include-closed): %v\nstderr=%s", err, stderr)
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
	err := cli.ExecuteWith([]string{"account", "list", "--file", "x.tdb", "extra"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account list ... extra) should return error")
	}
}

func TestAccountCmd_HelpListsList(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `account --help` to list `list`; got:\n%s", stdout.String())
	}
}

func TestAccountList_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `account list --help` to describe the command; got:\n%s", stdout.String())
	}
}
