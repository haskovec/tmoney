package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransactionVoid_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"transaction", "void", "abc123"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transaction void) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransactionVoid_MissingID(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"transaction", "void", "--file", "irrelevant.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transaction void) without positional ID should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestTransactionVoid_InvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transaction", "void", "not-a-valid-id", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transaction void) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid transaction ID") {
		t.Errorf("error should mention invalid transaction ID, got: %v", err)
	}
}

func TestTransactionVoid_Voids(t *testing.T) {
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

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transaction", "void", txnID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transaction void) returned error: %v\nstderr=%s", err, stderr)
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

	txnRepo2 := transaction.NewRepository(database2)
	voidedTxn, err := txnRepo2.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("failed to get voided transaction: %v", err)
	}
	if voidedTxn.Status != transaction.StatusVoid {
		t.Errorf("transaction status should be void, got %q", voidedTxn.Status)
	}
	if !voidedTxn.Amount.IsZero() {
		t.Errorf("voided transaction amount should be zero, got %s", voidedTxn.Amount.String())
	}
}

func TestTransactionVoid_AlreadyVoid(t *testing.T) {
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

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn.Void()
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transaction", "void", txnID, "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("voiding an already void transaction should return error")
	}
}

func TestTransactionVoid_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"transaction", "void", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transaction void --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "void") {
		t.Errorf("expected `transaction void --help` output to mention void; got:\n%s", stdout.String())
	}
}

func TestTransactionCmd_HelpListsVoid(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transaction --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "void") {
		t.Errorf("expected `transaction --help` to list `void`; got:\n%s", stdout.String())
	}
}
