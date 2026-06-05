package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransactionList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransactionList_MissingAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestTransactionList_AccountNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Nonexistent"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestTransactionList_NoTransactions(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "TRANSACTIONS: Checking") {
		t.Errorf("expected TRANSACTIONS header, got: %s", out)
	}
	if !strings.Contains(out, "No transactions found") {
		t.Errorf("expected 'No transactions found', got: %s", out)
	}
}

func TestTransactionList_WithTransactions(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("setup: create payee: %v", err)
	}

	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Food", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn1 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-5.50"))
	txn1.SetPayee(py.ID)
	txn1.SetCategory(cat.ID)
	txn1.Clear()
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("setup: create transaction 1: %v", err)
	}

	txn2 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-25.00"))
	txn2.SetPayee(py.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("setup: create transaction 2: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"TRANSACTIONS: Checking", "Coffee Shop", "Food", "-$5.50", "Showing 2 transaction(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "  C\n") {
		t.Errorf("expected status code C for cleared transaction, got:\n%s", out)
	}
}

func TestTransactionList_WithLimit(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	for i := range 5 {
		txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-10.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("setup: create transaction %d: %v", i, err)
		}
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--limit", "2"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list --limit): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "Showing 2 transaction(s)") {
		t.Errorf("expected 2 transactions with --limit 2, got: %s", stdout.String())
	}
}

func TestTransactionList_WithDateFilter(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	dates := []string{"2024-01-15", "2024-02-15", "2024-03-15"}
	amts := []string{"-10.00", "-20.00", "-30.00"}
	for i, ds := range dates {
		d, _ := types.ParseDate(ds)
		txn := transactiondom.NewTransaction(acct.ID, d, types.MustNewMoney(amts[i]))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("setup: create transaction %d: %v", i, err)
		}
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "list",
		"--file", dbPath,
		"--account", "Checking",
		"--from", "2024-02-01",
		"--to", "2024-02-28",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list with date filter): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Showing 1 transaction(s)") {
		t.Errorf("expected 1 transaction in date range, got: %s", out)
	}
	if !strings.Contains(out, "2024-02-15") {
		t.Errorf("expected February transaction, got: %s", out)
	}
}

func TestTransactionList_InvalidFromDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--from", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list) with invalid --from should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from date") {
		t.Errorf("expected 'invalid --from date', got: %v", err)
	}
}

func TestTransactionList_InvalidToDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--to", "not-a-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list) with invalid --to should return error")
	}
	if !strings.Contains(err.Error(), "invalid --to date") {
		t.Errorf("expected 'invalid --to date', got: %v", err)
	}
}

func TestTransactionList_StatusFilter(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn1 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("setup: txn1: %v", err)
	}
	txn2 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-20.00"))
	txn2.Clear()
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("setup: txn2: %v", err)
	}
	txn3 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-30.00"))
	txn3.Clear()
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("setup: txn3: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--status", "cleared"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list --status cleared): %v", err)
	}
	if !strings.Contains(stdout.String(), "Showing 2 transaction(s)") {
		t.Errorf("expected 2 cleared transactions, got: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--status", "uncleared"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list --status uncleared): %v", err)
	}
	if !strings.Contains(stdout.String(), "Showing 1 transaction(s)") {
		t.Errorf("expected 1 uncleared transaction, got: %s", stdout.String())
	}
}

func TestTransactionList_InvalidStatus(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--status", "bogus"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list --status bogus) should error")
	}
	if !strings.Contains(err.Error(), "invalid --status") {
		t.Errorf("expected 'invalid --status', got: %v", err)
	}
}

func TestTransactionList_NegativeLimit(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--limit", "-1"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction list --limit -1) should error")
	}
	if !strings.Contains(err.Error(), "--limit") {
		t.Errorf("expected error about --limit, got: %v", err)
	}
}

func TestTransactionList_ShowIDs_AddsIDColumn(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-12.34"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("setup: create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking", "--show-ids"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list --show-ids): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, txn.ID.String()) {
		t.Errorf("expected transaction UUID %q in --show-ids output, got:\n%s", txn.ID.String(), out)
	}
	if !strings.Contains(out, "ID\t") && !strings.Contains(out, "ID ") {
		t.Errorf("expected ID column header in --show-ids output, got:\n%s", out)
	}
}

func TestTransactionList_DefaultOmitsIDColumn(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-12.34"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("setup: create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if strings.Contains(out, txn.ID.String()) {
		t.Errorf("expected transaction UUID NOT in default output, got:\n%s", out)
	}
}

func TestTransactionCmd_HelpListsList(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `transaction --help` to list `list`; got:\n%s", stdout.String())
	}
}

func TestTransactionList_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `transaction list --help` to describe the command; got:\n%s", stdout.String())
	}
}
