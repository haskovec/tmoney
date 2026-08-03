package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
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

// TestTransactionVoid_TransferLeg drives the real CLI command against a transfer
// leg.
//
// This is the test that was missing. Phase 5a made
// transaction.Service.VoidTransaction refuse a whole-transfer leg — correctly,
// since it writes one row and a transfer is two — and the TUI was re-pointed at
// transfer.Void while this command was not. Nothing caught it because no test
// ran `transaction void` against a transfer.
func TestTransactionVoid_TransferLeg(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.Today())
	savings := account.NewAccount("Savings", account.TypeSavings, "USD",
		types.MustNewMoney("500.00"), types.Today())
	for _, a := range []*account.Account{checking, savings} {
		if err := acctRepo.Create(a); err != nil {
			t.Fatalf("create account: %v", err)
		}
	}

	svc := app.NewServices(database)
	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   savings.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("125.00"),
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	fromLeg, toLeg, transferID := res.From.RowID, res.To.RowID, res.TransferID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith(
		[]string{"transaction", "void", fromLeg.String(), "--file", dbPath}, stdout, stderr,
	); err != nil {
		t.Fatalf("cli transaction void on a transfer leg: %v\nstderr=%s", err, stderr)
	}
	if out := stdout.String(); !strings.Contains(out, "counterpart was also voided") {
		t.Errorf("expected the counterpart note in output, got:\n%s", out)
	}

	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database2.Close()

	// BOTH legs are voided and zeroed, not just the named one.
	repo := transactiondom.NewRepository(database2)
	for label, id := range map[string]types.ID{"from": fromLeg, "to": toLeg} {
		leg, err := repo.GetByID(id)
		if err != nil {
			t.Fatalf("load %s leg: %v", label, err)
		}
		if leg.Status != transactiondom.StatusVoid {
			t.Errorf("%s leg status = %q, want void", label, leg.Status)
		}
		if !leg.Amount.IsZero() {
			t.Errorf("%s leg amount = %s, want 0", label, leg.Amount)
		}
	}
	_ = transferID
}

// TestTransactionVoid_InvestmentInvolvingTransferIsNamed pins that the CLI
// reports the SPECIFIC reason an investment-involving transfer cannot be voided
// (investment_transactions has no void status), rather than a generic failure.
func TestTransactionVoid_InvestmentInvolvingTransferIsNamed(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.Today())
	brokerage := account.NewAccount("Brokerage", account.TypeInvestment, "USD",
		types.ZeroMoney, types.Today())
	for _, a := range []*account.Account{checking, brokerage} {
		if err := acctRepo.Create(a); err != nil {
			t.Fatalf("create account: %v", err)
		}
	}

	svc := app.NewServices(database)
	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   brokerage.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("200.00"),
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	bankLeg := res.From.RowID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith(
		[]string{"transaction", "void", bankLeg.String(), "--file", dbPath}, stdout, stderr,
	)
	if err == nil {
		t.Fatal("expected voiding an investment-involving transfer to fail")
	}
	// The old failure was "expected 2 transactions for transfer, found 1".
	if !strings.Contains(err.Error(), "cannot be voided") ||
		!strings.Contains(err.Error(), "no void status") {
		t.Errorf("error should name the real limitation, got: %v", err)
	}
}
