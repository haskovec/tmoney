package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransactionVoid_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "void", "abc123"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction void) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransactionVoid_MissingID(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "void", "--file", "irrelevant.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction void) without positional ID should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestTransactionVoid_InvalidID(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "void", "not-a-valid-id", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction void) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid transaction ID") {
		t.Errorf("error should mention invalid transaction ID, got: %v", err)
	}
}

func TestTransactionVoid_Voids(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

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

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "void", txnID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction void) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"Transaction voided successfully", "Checking", "Void"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo2 := transactiondom.NewRepository(database2)
	voidedTxn, err := txnRepo2.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("failed to get voided transaction: %v", err)
	}
	if voidedTxn.Status != transactiondom.StatusVoid {
		t.Errorf("transaction status should be void, got %q", voidedTxn.Status)
	}
	if !voidedTxn.Amount.IsZero() {
		t.Errorf("voided transaction amount should be zero, got %s", voidedTxn.Amount.String())
	}
}

func TestTransactionVoid_AlreadyVoid(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

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

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn.Void()
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "void", txnID, "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("voiding an already void transaction should return error")
	}
}

func TestTransactionVoid_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "void", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction void --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "void") {
		t.Errorf("expected `transaction void --help` output to mention void; got:\n%s", stdout.String())
	}
}

func TestTransactionCmd_HelpListsVoid(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "void") {
		t.Errorf("expected `transaction --help` to list `void`; got:\n%s", stdout.String())
	}
}
