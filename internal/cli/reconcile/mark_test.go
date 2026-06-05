package reconcile_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReconcileMark_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", "00000000-0000-0000-0000-000000000000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile mark) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestReconcileMark_MissingIDs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", "--file", "test.tdb",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile mark) without positional IDs should return error")
	}
	if !strings.Contains(err.Error(), "requires at least") {
		t.Errorf("expected Cobra minimum-args error, got: %v", err)
	}
}

func TestReconcileMark_InvalidID(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", "not-a-valid-id",
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile mark) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid transaction ID") {
		t.Errorf("expected error to mention invalid transaction ID, got: %v", err)
	}
}

func TestReconcileMark_TransactionNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", "00000000-0000-0000-0000-000000000000",
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile mark) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "transaction not found") {
		t.Errorf("expected error to mention transaction not found, got: %v", err)
	}
}

func TestReconcileMark_NoActiveSession(t *testing.T) {
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
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", txnID,
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile mark) without active session should return error")
	}
	if !strings.Contains(err.Error(), "no active reconciliation session") {
		t.Errorf("expected error to mention no active session, got: %v", err)
	}
}

func TestReconcileMark_Success(t *testing.T) {
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

	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("850.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "mark", txn1.ID.String(), txn2.ID.String(),
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile mark): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"Marked 2 transaction(s) for reconciliation",
		"Current difference",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestReconcileMark_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "mark", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile mark --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "mark") {
		t.Errorf("expected `reconcile mark --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestReconcileCmd_HelpListsMark(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "mark") {
		t.Errorf("expected `reconcile --help` to list `mark`; got:\n%s", stdout.String())
	}
}
