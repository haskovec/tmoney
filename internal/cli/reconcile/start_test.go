package reconcile_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReconcileStart_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile start) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestReconcileStart_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", "test.tdb",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile start) without --account should return error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "account" not set`) {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestReconcileStart_MissingDate(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", "test.tdb",
		"--account", "Checking",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile start) without --statement-date should return error")
	}
	if !strings.Contains(err.Error(), "statement-date") {
		t.Errorf("expected error to mention statement-date, got: %v", err)
	}
}

func TestReconcileStart_MissingBalance(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", "test.tdb",
		"--account", "Checking",
		"--statement-date", "2024-01-31",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile start) without --statement-balance should return error")
	}
	if !strings.Contains(err.Error(), "statement-balance") {
		t.Errorf("expected error to mention statement-balance, got: %v", err)
	}
}

func TestReconcileStart_InvalidDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "not-a-date",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected invalid --statement-date to error")
	}
	if !strings.Contains(err.Error(), "invalid --statement-date") {
		t.Errorf("expected error to mention invalid --statement-date, got: %v", err)
	}
}

func TestReconcileStart_InvalidBalance(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "not-a-number",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected invalid --statement-balance to error")
	}
	if !strings.Contains(err.Error(), "invalid --statement-balance") {
		t.Errorf("expected error to mention invalid --statement-balance, got: %v", err)
	}
}

func TestReconcileStart_AccountNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", dbPath,
		"--account", "NonExistent",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected unknown account to error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestReconcileStart_Success(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-10"), types.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "start",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "850.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile start): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"Reconciliation started for Checking",
		"2024-01-31",
		"$850.00",
		"Unreconciled transactions: 2",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestReconcileStart_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "start", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile start --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "start") {
		t.Errorf("expected `reconcile start --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestReconcileCmd_HelpListsStart(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "start") {
		t.Errorf("expected `reconcile --help` to list `start`; got:\n%s", stdout.String())
	}
}
