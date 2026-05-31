package account_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

func TestAccountBalance_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "balance"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account balance) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestAccountBalance_NoAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "balance", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account balance): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "No accounts found") {
		t.Errorf("expected 'No accounts found' in output, got: %s", stdout.String())
	}
}

func TestAccountBalance_WithAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := accountdom.NewRepository(database)
	for _, a := range []*accountdom.Account{
		accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today()),
		accountdom.NewAccount("Savings", accountdom.TypeSavings, "USD", types.MustNewMoney("5000.00"), types.Today()),
		accountdom.NewAccount("Visa", accountdom.TypeCreditCard, "USD", types.MustNewMoney("-500.00"), types.Today()),
	} {
		if err := repo.Create(a); err != nil {
			t.Fatalf("setup: create account %s: %v", a.Name, err)
		}
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "balance", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account balance): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"BALANCES", "Checking", "Savings", "Visa", "Net Worth"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestAccountBalance_ShortFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Quick", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "balance", "-f", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account balance -f): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Quick") {
		t.Errorf("expected 'Quick' in output, got: %s", stdout.String())
	}
}

func TestAccountBalance_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "balance", "extra", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account balance extra) should return error")
	}
}

func TestAccountCmd_HelpListsBalance(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "balance") {
		t.Errorf("expected `account --help` to list `balance`; got:\n%s", stdout.String())
	}
}

func TestAccountBalance_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "balance", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account balance --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "balance") {
		t.Errorf("expected `account balance --help` to describe the command; got:\n%s", stdout.String())
	}
}
