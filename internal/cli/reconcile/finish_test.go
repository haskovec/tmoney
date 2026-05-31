package reconcile_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReconcileFinish_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "finish", "--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile finish) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestReconcileFinish_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"reconcile", "finish", "--file", "test.tdb",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile finish) without --account should return error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "account" not set`) {
		t.Errorf("expected Cobra required-flag error mentioning \"account\", got: %v", err)
	}
}

func TestReconcileFinish_NoActiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"reconcile", "finish",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile finish) with no active session should return error")
	}
	if !strings.Contains(err.Error(), "no active reconciliation") {
		t.Errorf("expected error to mention no active session, got: %v", err)
	}
}

func TestReconcileFinish_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

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
	err = cli.ExecuteWith([]string{
		"reconcile", "finish",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile finish): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"Reconciliation completed for Checking",
		"2024-01-31",
		"$850.00",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo2 := transaction.NewRepository(database2)
	for _, want := range []*transaction.Transaction{txn1, txn2} {
		got, err := txnRepo2.GetByID(want.ID)
		if err != nil {
			t.Fatalf("failed to get transaction %s: %v", want.ID, err)
		}
		if got.Status != transaction.StatusReconciled {
			t.Errorf("transaction %s should be reconciled, got %q", want.ID, got.Status)
		}
	}
}

func TestReconcileFinish_WithDifference(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

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

	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"reconcile", "finish",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(reconcile finish) with non-zero difference should return error")
	}
	if !strings.Contains(err.Error(), "Difference") {
		t.Errorf("expected error to mention Difference, got: %v", err)
	}
}

func TestReconcileFinish_WithForce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

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

	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"reconcile", "finish",
		"--file", dbPath,
		"--account", "Checking",
		"--force",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile finish --force): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation completed for Checking") {
		t.Errorf("output should confirm completion, got:\n%s", output)
	}
}

func TestReconcileFinish_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "finish", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile finish --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "finish") {
		t.Errorf("expected `reconcile finish --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestReconcileCmd_HelpListsFinish(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"reconcile", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(reconcile --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "finish") {
		t.Errorf("expected `reconcile --help` to list `finish`; got:\n%s", stdout.String())
	}
}
