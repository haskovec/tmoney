package transaction_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransactionSearch_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "search", "amazon"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransactionSearch_MissingTerm(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "search", "--file", "irrelevant.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) without positional term should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestTransactionSearch_ByPayee(t *testing.T) {
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

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Amazon")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn1 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn1.SetPayee(py.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	txn2 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-25.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "Amazon", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "SEARCH RESULTS") {
		t.Error("output should contain SEARCH RESULTS header")
	}
	if !strings.Contains(output, "Amazon") {
		t.Error("output should contain Amazon payee")
	}
	if !strings.Contains(output, "-$50.00") {
		t.Error("output should contain the amount")
	}
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result, got: %s", output)
	}
}

func TestTransactionSearch_ByMemo(t *testing.T) {
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

	txnRepo := transactiondom.NewRepository(database)
	txn1 := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-75.00"))
	txn1.SetMemo("Office supplies from Staples")
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "office", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result for memo search, got: %s", output)
	}
}

func TestTransactionSearch_NoResults(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "nonexistent", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "No transactions found") {
		t.Errorf("output should say no transactions found, got: %s", output)
	}
}

func TestTransactionSearch_WithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("5000.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Target")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn1 := transactiondom.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn1.SetPayee(py.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create checking transaction: %v", err)
	}

	txn2 := transactiondom.NewTransaction(savings.ID, types.Today(), types.MustNewMoney("-30.00"))
	txn2.SetPayee(py.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create savings transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "Target", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with account filter, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
}

func TestTransactionSearch_AccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "anything", "--account", "Nonexistent", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) with nonexistent account should error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestTransactionSearch_WithMinMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Store")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	amounts := []string{"-10.00", "-50.00", "-100.00", "-200.00"}
	for _, amt := range amounts {
		txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney(amt))
		txn.SetPayee(py.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "Store", "--min", "-100.00", "--max", "-50.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 2 transaction(s)") {
		t.Errorf("output should show 2 results with amount filter, got: %s", output)
	}
}

func TestTransactionSearch_InvalidMinAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "test", "--min", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) with invalid --min should return error")
	}
	if !strings.Contains(err.Error(), "invalid --min") {
		t.Errorf("error should mention invalid --min, got: %v", err)
	}
}

func TestTransactionSearch_InvalidMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "test", "--max", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) with invalid --max should return error")
	}
	if !strings.Contains(err.Error(), "invalid --max") {
		t.Errorf("error should mention invalid --max, got: %v", err)
	}
}

func TestTransactionSearch_WithDateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	dates := []string{"2024-01-15", "2024-02-15", "2024-03-15"}
	for _, ds := range dates {
		d, _ := types.ParseDate(ds)
		txn := transactiondom.NewTransaction(acct.ID, d, types.MustNewMoney("-5.00"))
		txn.SetPayee(py.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"transaction", "search", "Coffee",
		"--from", "2024-02-01",
		"--to", "2024-02-28",
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search) returned error: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with date filter, got: %s", output)
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain February transaction date")
	}
}

func TestTransactionSearch_InvalidFromDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "anything", "--from", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) with invalid --from should error")
	}
	if !strings.Contains(err.Error(), "invalid --from") {
		t.Errorf("expected 'invalid --from' error, got: %v", err)
	}
}

func TestTransactionSearch_InvalidToDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "anything", "--to", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction search) with invalid --to should error")
	}
	if !strings.Contains(err.Error(), "invalid --to") {
		t.Errorf("expected 'invalid --to' error, got: %v", err)
	}
}

func TestTransactionSearch_ShowIDs_AddsIDColumn(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Amazon")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn.SetPayee(py.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "Amazon", "--file", dbPath, "--show-ids"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search --show-ids): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, txn.ID.String()) {
		t.Errorf("expected transaction UUID %q in --show-ids output, got:\n%s", txn.ID.String(), out)
	}
}

func TestTransactionSearch_DefaultOmitsIDColumn(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Amazon")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn.SetPayee(py.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transaction", "search", "Amazon", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if strings.Contains(out, txn.ID.String()) {
		t.Errorf("expected transaction UUID NOT in default output, got:\n%s", out)
	}
}

func TestTransactionSearch_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "search", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction search --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "search") {
		t.Errorf("expected `transaction search --help` output to mention search; got:\n%s", stdout.String())
	}
}

func TestTransactionCmd_HelpListsSearch(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "search") {
		t.Errorf("expected `transaction --help` to list `search`; got:\n%s", stdout.String())
	}
}
