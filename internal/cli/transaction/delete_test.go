package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// reloadOptional is reload's forgiving sibling: a missing transaction
// returns nil instead of failing the test, so delete tests can assert
// the row is gone while still using the services handle.
func reloadOptional(t *testing.T, dbPath string, txnID types.ID) (*transactiondom.Transaction, *app.Services, func()) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	svc := app.NewServices(database)
	txn, err := svc.TransactionRepo.GetByID(txnID)
	if err != nil {
		txn = nil
	}
	return txn, svc, func() { database.Close() }
}

func runDelete(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"transaction", "delete"}, args...), stdout, stderr)
	return stdout.String(), err
}

func TestTransactionDelete_MissingFile(t *testing.T) {
	_, err := runDelete(t, "abc123")
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestTransactionDelete_MissingID(t *testing.T) {
	_, err := runDelete(t, "--file", "irrelevant.tdb")
	if err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Fatalf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestTransactionDelete_InvalidID(t *testing.T) {
	dbPath, _, _ := editFixture(t)
	_, err := runDelete(t, "not-a-uuid", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "invalid transaction ID") {
		t.Fatalf("expected invalid-ID error, got: %v", err)
	}
}

func TestTransactionDelete_Deletes_BalanceReflects(t *testing.T) {
	dbPath, acctID, txnID := editFixture(t)

	out, err := runDelete(t, txnID.String(), "--file", dbPath)
	if err != nil {
		t.Fatalf("transaction delete: %v", err)
	}
	for _, want := range []string{"Transaction deleted successfully", "Checking", "Coffee Shop"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}

	// Row is gone and the balance is back to the opening balance.
	dbPathCopy := dbPath
	txn, svc, closeDB := reloadOptional(t, dbPathCopy, txnID)
	defer closeDB()
	if txn != nil {
		t.Error("transaction should be deleted")
	}
	bal, err := svc.Account.GetBalance(acctID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !bal.CurrentBalance.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("balance = %s, want 1000.00", bal.CurrentBalance.String())
	}
}

func TestTransactionDelete_RefusesTransferLeg(t *testing.T) {
	dbPath, txnID := transferLegFixture(t)
	_, err := runDelete(t, txnID.String(), "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "transfer delete") {
		t.Fatalf("expected pointer to transfer delete, got: %v", err)
	}
}

func TestTransactionDelete_RefusesSplitParent(t *testing.T) {
	dbPath, txnID := splitParentFixture(t)
	_, err := runDelete(t, txnID.String(), "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("expected split-parent refusal, got: %v", err)
	}
}

func TestTransactionDelete_RefusesReconciled(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	markStatus(t, dbPath, txnID, transactiondom.StatusReconciled)
	_, err := runDelete(t, txnID.String(), "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "reconciled") {
		t.Fatalf("expected reconciled refusal, got: %v", err)
	}
}

func TestTransactionDelete_RefusesVoid(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	markStatus(t, dbPath, txnID, transactiondom.StatusVoid)
	_, err := runDelete(t, txnID.String(), "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "void") {
		t.Fatalf("expected void refusal, got: %v", err)
	}
}

func TestTransactionDelete_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "delete", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction delete --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "delete") {
		t.Errorf("expected help output to mention delete; got:\n%s", stdout.String())
	}
}

func TestTransactionCmd_HelpListsEditAndDelete(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction --help): %v", err)
	}
	for _, want := range []string{"edit", "delete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected `transaction --help` to list %q; got:\n%s", want, stdout.String())
		}
	}
}
