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

func TestAccountReopen_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "reopen", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("reopen without --file should return an error")
	}
}

func TestAccountReopen_NotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "reopen", "Nope", "--file", dbPath}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found', got %v", err)
	}
}

func TestAccountReopen_HappyPath(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Old Savings", accountdom.TypeSavings, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	acct.Close(types.MustParseDate("2024-03-14"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "reopen", "Old Savings", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("reopen: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Account reopened.") {
		t.Errorf("expected reopen confirmation, got: %s", stdout.String())
	}

	// The account is active again and the close date is cleared.
	stdout.Reset()
	stderr.Reset()
	if err := cli.ExecuteWith([]string{"account", "show", "Old Savings", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("show after reopen: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "Active") {
		t.Errorf("expected reopened account to be Active, got: %s", out)
	}
	if strings.Contains(out, "Closed") {
		t.Errorf("expected close date cleared after reopen, got: %s", out)
	}
}

func TestAccountReopen_NotClosedRejected(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Active Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "reopen", "Active Checking", "--file", dbPath}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "not closed") {
		t.Fatalf("expected 'not closed' error, got %v", err)
	}
}

func TestAccountReopen_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "reopen", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account reopen --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "reopen") {
		t.Errorf("expected `account reopen --help` to describe the command; got:\n%s", stdout.String())
	}
}
