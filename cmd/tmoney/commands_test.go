package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func TestRun_ListAccountsMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-accounts"}, stdout, stderr)
	if err == nil {
		t.Error("run(--list-accounts) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ListAccountsFileNotFound(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-accounts", "--file", "/nonexistent/path.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run with nonexistent file should return error")
	}
}

func TestRun_ListAccountsWithValidFile(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Test Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the list-accounts command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "ACCOUNTS") {
		t.Error("output should contain ACCOUNTS header")
	}
	if !strings.Contains(output, "Test Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account type")
	}
	if !strings.Contains(output, "USD") {
		t.Error("output should contain currency")
	}
}

func TestRun_ListAccountsNoAccounts(t *testing.T) {
	// Create a temporary database with no accounts
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Run the list-accounts command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestRun_CreateDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb.tdb")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Created database:") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, dbPath) {
		t.Errorf("output should contain path, got: %s", output)
	}

	// Verify the file was created and can be opened
	database, err := db.Open(dbPath)
	if err != nil {
		t.Errorf("failed to open created database: %v", err)
		return
	}
	database.Close()
}

func TestRun_CreateDBWithEqualsFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb.tdb")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create=" + dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create=path) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Created database:") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
}

func TestRun_CreateDBAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "existing.tdb")

	// Create the database first
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create initial database: %v", err)
	}
	database.Close()

	// Try to create again
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--create", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--create) on existing file should return error")
	}
}

func TestRun_CreateDBAddsExtension(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb") // No .tdb extension

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, ".tdb") {
		t.Errorf("output should show .tdb extension was added, got: %s", output)
	}

	// Verify the file with .tdb extension was created
	database, err := db.Open(dbPath + ".tdb")
	if err != nil {
		t.Errorf("failed to open created database with .tdb extension: %v", err)
		return
	}
	database.Close()
}

func TestRun_CreateThenListAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	// Create the database
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--create) returned error: %v", err)
	}

	// List accounts (should be empty but work)
	stdout.Reset()
	stderr.Reset()
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) after create returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestRun_AccountMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--account", "Test Account"}, stdout, stderr)
	if err == nil {
		t.Error("run(--account) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AccountNotFound(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--account", "Nonexistent Account", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--account) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AccountWithValidAccount(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account with optional fields
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Test Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	account.SetInstitution("Chase Bank")
	account.SetAccountNumber("1234567890")
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the account command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--account", "Test Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "ACCOUNT: Test Checking") {
		t.Error("output should contain account header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account type")
	}
	if !strings.Contains(output, "USD") {
		t.Error("output should contain currency")
	}
	if !strings.Contains(output, "Chase Bank") {
		t.Error("output should contain institution")
	}
	if !strings.Contains(output, "****7890") {
		t.Error("output should contain masked account number")
	}
	if !strings.Contains(output, "Current Balance") {
		t.Error("output should contain current balance")
	}
	if !strings.Contains(output, "Active") {
		t.Error("output should show active status")
	}
}

func TestRun_BalanceMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--balance"}, stdout, stderr)
	if err == nil {
		t.Error("run(--balance) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_BalanceNoAccounts(t *testing.T) {
	// Create a temporary database with no accounts
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Run the balance command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--balance", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--balance) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestRun_BalanceWithAccounts(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create test accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("5000.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	creditCard := models.NewAccount(
		"Visa",
		models.AccountTypeCreditCard,
		"USD",
		models.MustNewMoney("-500.00"),
		models.Today(),
	)
	if err := repo.Create(creditCard); err != nil {
		t.Fatalf("failed to create credit card account: %v", err)
	}

	database.Close()

	// Run the balance command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--balance", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--balance) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "BALANCES") {
		t.Error("output should contain BALANCES header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
	if !strings.Contains(output, "Savings") {
		t.Error("output should contain Savings account")
	}
	if !strings.Contains(output, "Visa") {
		t.Error("output should contain Visa account")
	}
	if !strings.Contains(output, "Net Worth") {
		t.Error("output should contain Net Worth")
	}
}

func TestRun_ListAccountsWithClosedAccount(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create an active and a closed account
	repo := repository.NewAccountRepository(database)

	activeAccount := models.NewAccount(
		"Active Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(activeAccount); err != nil {
		t.Fatalf("failed to create active account: %v", err)
	}

	closedAccount := models.NewAccount(
		"Closed Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("0"),
		models.Today(),
	)
	closedAccount.Close()
	if err := repo.Create(closedAccount); err != nil {
		t.Fatalf("failed to create closed account: %v", err)
	}

	database.Close()

	// Test without --include-closed (should only show active)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Active Checking") {
		t.Error("output should contain active account")
	}
	if strings.Contains(output, "Closed Savings") {
		t.Error("output should NOT contain closed account without --include-closed")
	}

	// Test with --include-closed (should show both)
	stdout.Reset()
	stderr.Reset()
	err = run([]string{"--list-accounts", "--file", dbPath, "--include-closed"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts --include-closed) returned error: %v", err)
		return
	}

	output = stdout.String()
	if !strings.Contains(output, "Active Checking") {
		t.Error("output should contain active account")
	}
	if !strings.Contains(output, "Closed Savings") {
		t.Error("output should contain closed account with --include-closed")
	}
}

func TestRun_TransactionsMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--transactions", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_TransactionsMissingAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_TransactionsAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Nonexistent", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_TransactionsNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the transactions command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "TRANSACTIONS: Checking") {
		t.Error("output should contain TRANSACTIONS header")
	}
	if !strings.Contains(output, "No transactions found") {
		t.Errorf("output should say no transactions found, got: %s", output)
	}
}

func TestRun_TransactionsWithTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create transactions
	txnRepo := repository.NewTransactionRepository(database)

	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-5.50"))
	txn1.SetPayee(payee.ID)
	txn1.SetCategory(category.ID)
	txn1.Clear()
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-25.00"))
	txn2.SetPayee(payee.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	database.Close()

	// Run the transactions command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "TRANSACTIONS: Checking") {
		t.Error("output should contain TRANSACTIONS header")
	}
	if !strings.Contains(output, "Coffee Shop") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Food") {
		t.Error("output should contain category name")
	}
	if !strings.Contains(output, "-$5.50") {
		t.Error("output should contain transaction amount")
	}
	// Status column should show "C" code (tabwriter converts tabs to spaces)
	if !strings.Contains(output, "  C\n") {
		t.Error("output should contain status code C for cleared transaction")
	}
	if !strings.Contains(output, "Showing 2 transaction(s)") {
		t.Errorf("output should show transaction count, got: %s", output)
	}
}

func TestRun_TransactionsWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create multiple transactions
	txnRepo := repository.NewTransactionRepository(database)
	for i := range 5 {
		txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction %d: %v", i, err)
		}
	}

	database.Close()

	// Run with --limit 2
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath, "--limit", "2"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions --limit) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 2 transaction(s)") {
		t.Errorf("output should show 2 transactions with limit, got: %s", output)
	}
}

func TestRun_TransactionsWithDateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transactions with different dates
	txnRepo := repository.NewTransactionRepository(database)

	jan15, _ := models.ParseDate("2024-01-15")
	txn1 := models.NewTransaction(account.ID, jan15, models.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	feb15, _ := models.ParseDate("2024-02-15")
	txn2 := models.NewTransaction(account.ID, feb15, models.MustNewMoney("-20.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	mar15, _ := models.ParseDate("2024-03-15")
	txn3 := models.NewTransaction(account.ID, mar15, models.MustNewMoney("-30.00"))
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("failed to create transaction 3: %v", err)
	}

	database.Close()

	// Run with date filter for February only
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transactions", "--account", "Checking", "--file", dbPath,
		"--from", "2024-02-01", "--to", "2024-02-28",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions with date filter) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 1 transaction(s)") {
		t.Errorf("output should show 1 transaction in date range, got: %s", output)
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Errorf("output should show February transaction, got: %s", output)
	}
}

func TestRun_TransactionsInvalidDateFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath, "--from", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_AddTransactionMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-transaction", "--account", "Checking", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddTransactionMissingAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_AddTransactionMissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("error should mention --amount requirement, got: %v", err)
	}
}

func TestRun_AddTransactionInvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "invalid"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("error should mention invalid amount, got: %v", err)
	}
}

func TestRun_AddTransactionInvalidDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_AddTransactionAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Nonexistent", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AddTransactionCategoryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--category", "Nonexistent"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with nonexistent category should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AddTransactionBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-50.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transaction created successfully") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Errorf("output should show account name, got: %s", output)
	}
	if !strings.Contains(output, "-$50.00") {
		t.Errorf("output should show amount, got: %s", output)
	}

	// Verify transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Amount.String() != "-50" {
		t.Errorf("transaction amount = %s, want -50", transactions[0].Amount.String())
	}
}

func TestRun_AddTransactionWithPayeeAutoCreate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command with payee
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-5.50",
		"--payee", "Coffee Shop",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Coffee Shop") {
		t.Errorf("output should show payee name, got: %s", output)
	}
	if !strings.Contains(output, "(new)") {
		t.Errorf("output should indicate new payee was created, got: %s", output)
	}

	// Verify payee was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	payeeRepo := repository.NewPayeeRepository(database)
	payee, err := payeeRepo.GetByName("Coffee Shop")
	if err != nil {
		t.Errorf("payee should have been created: %v", err)
	}
	if payee.Name != "Coffee Shop" {
		t.Errorf("payee name = %q, want %q", payee.Name, "Coffee Shop")
	}
}

func TestRun_AddTransactionWithExistingPayee(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create existing payee
	payeeRepo := repository.NewPayeeRepository(database)
	existingPayee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(existingPayee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}
	database.Close()

	// Run the add-transaction command with existing payee
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-5.50",
		"--payee", "Coffee Shop",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Coffee Shop") {
		t.Errorf("output should show payee name, got: %s", output)
	}
	// Should NOT say (new) for existing payee
	if strings.Contains(output, "(new)") {
		t.Errorf("output should NOT indicate new payee for existing payee, got: %s", output)
	}
}

func TestRun_AddTransactionWithCategory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	database.Close()

	// Run the add-transaction command with category
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-25.00",
		"--category", "Food",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Food") {
		t.Errorf("output should show category name, got: %s", output)
	}

	// Verify transaction has category
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if !transactions[0].CategoryID.Valid {
		t.Error("transaction should have category set")
	}
}

func TestRun_AddTransactionWithDateAndMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command with date and memo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-15.00",
		"--date", "2024-01-15",
		"--memo", "Lunch with friend",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "2024-01-15") {
		t.Errorf("output should show date, got: %s", output)
	}
	if !strings.Contains(output, "Lunch with friend") {
		t.Errorf("output should show memo, got: %s", output)
	}

	// Verify transaction
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Date.String() != "2024-01-15" {
		t.Errorf("transaction date = %s, want 2024-01-15", transactions[0].Date.String())
	}
	if !transactions[0].Memo.Valid || transactions[0].Memo.String != "Lunch with friend" {
		t.Errorf("transaction memo = %v, want 'Lunch with friend'", transactions[0].Memo)
	}
}

func TestRun_AddTransactionFullExample(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Dining", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	database.Close()

	// Run with all options
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-45.50",
		"--payee", "Olive Garden",
		"--category", "Dining",
		"--date", "2024-06-15",
		"--memo", "Birthday dinner",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transaction created successfully") {
		t.Error("output should confirm creation")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should show account")
	}
	if !strings.Contains(output, "-$45.50") {
		t.Error("output should show amount")
	}
	if !strings.Contains(output, "Olive Garden") {
		t.Error("output should show payee")
	}
	if !strings.Contains(output, "Dining") {
		t.Error("output should show category")
	}
	if !strings.Contains(output, "2024-06-15") {
		t.Error("output should show date")
	}
	if !strings.Contains(output, "Birthday dinner") {
		t.Error("output should show memo")
	}
}

func TestRun_AddAccountMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-account", "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddAccountMissingName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --name should return error")
	}
	if !strings.Contains(err.Error(), "requires --name") {
		t.Errorf("error should mention --name requirement, got: %v", err)
	}
}

func TestRun_AddAccountMissingType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --type should return error")
	}
	if !strings.Contains(err.Error(), "requires --type") {
		t.Errorf("error should mention --type requirement, got: %v", err)
	}
}

func TestRun_AddAccountInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "invalid_type"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error should mention invalid type, got: %v", err)
	}
}

func TestRun_AddAccountInvalidOpeningBalance(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-balance", "invalid"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid opening-balance should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-balance") {
		t.Errorf("error should mention invalid opening-balance, got: %v", err)
	}
}

func TestRun_AddAccountInvalidOpeningDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid opening-date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-date") {
		t.Errorf("error should mention invalid opening-date, got: %v", err)
	}
}

func TestRun_AddAccountDuplicateName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create existing account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with duplicate name should return error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists, got: %v", err)
	}
}

func TestRun_AddAccountCreditLimitOnNonCreditCard(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--credit-limit", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with credit-limit on non-credit_card should return error")
	}
	if !strings.Contains(err.Error(), "only valid for credit_card") {
		t.Errorf("error should mention credit_card requirement, got: %v", err)
	}
}

func TestRun_AddAccountInterestRateOnNonLoan(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--interest-rate", "5.5"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with interest-rate on non-loan should return error")
	}
	if !strings.Contains(err.Error(), "only valid for loan") {
		t.Errorf("error should mention loan requirement, got: %v", err)
	}
}

func TestRun_AddAccountBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-account",
		"--file", dbPath,
		"--name", "My Checking",
		"--type", "checking",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Account created successfully") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, "My Checking") {
		t.Errorf("output should show account name, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Errorf("output should show account type, got: %s", output)
	}
	if !strings.Contains(output, "USD") {
		t.Errorf("output should show default currency, got: %s", output)
	}
	if !strings.Contains(output, "$0.00") {
		t.Errorf("output should show default opening balance, got: %s", output)
	}

	// Verify account was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("My Checking")
	if err != nil {
		t.Errorf("account should have been created: %v", err)
		return
	}
	if account.Name != "My Checking" {
		t.Errorf("account name = %q, want %q", account.Name, "My Checking")
	}
	if account.Type != models.AccountTypeChecking {
		t.Errorf("account type = %v, want %v", account.Type, models.AccountTypeChecking)
	}
}

func TestRun_AddAccountWithAllOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-account",
		"--file", dbPath,
		"--name", "Primary Checking",
		"--type", "checking",
		"--currency", "EUR",
		"--opening-balance", "1000.50",
		"--opening-date", "2024-01-15",
		"--institution", "Chase Bank",
		"--account-number", "1234567890",
		"--notes", "Primary account",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Account created successfully") {
		t.Error("output should confirm creation")
	}
	if !strings.Contains(output, "Primary Checking") {
		t.Error("output should show account name")
	}
	if !strings.Contains(output, "EUR") {
		t.Error("output should show currency")
	}
	if !strings.Contains(output, "1000.50") || !strings.Contains(output, "€") {
		t.Errorf("output should show opening balance with EUR symbol, got: %s", output)
	}
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should show opening date")
	}
	if !strings.Contains(output, "Chase Bank") {
		t.Error("output should show institution")
	}
	if !strings.Contains(output, "1234567890") {
		t.Error("output should show account number")
	}
	if !strings.Contains(output, "Primary account") {
		t.Error("output should show notes")
	}

	// Verify account fields
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Primary Checking")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if account.Currency != "EUR" {
		t.Errorf("account currency = %q, want %q", account.Currency, "EUR")
	}
	if account.OpeningDate.String() != "2024-01-15" {
		t.Errorf("account opening date = %s, want 2024-01-15", account.OpeningDate.String())
	}
	if !account.Institution.Valid || account.Institution.String != "Chase Bank" {
		t.Errorf("account institution = %v, want 'Chase Bank'", account.Institution)
	}
	if !account.AccountNumber.Valid || account.AccountNumber.String != "1234567890" {
		t.Errorf("account number = %v, want '1234567890'", account.AccountNumber)
	}
	if !account.Notes.Valid || account.Notes.String != "Primary account" {
		t.Errorf("account notes = %v, want 'Primary account'", account.Notes)
	}
}

func TestRun_AddAccountCreditCard(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-account",
		"--file", dbPath,
		"--name", "Visa Card",
		"--type", "credit_card",
		"--credit-limit", "5000.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Credit Limit") {
		t.Errorf("output should show credit limit, got: %s", output)
	}
	if !strings.Contains(output, "$5000.00") {
		t.Errorf("output should show credit limit value, got: %s", output)
	}

	// Verify credit limit
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Visa Card")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if !account.CreditLimit.Valid {
		t.Error("credit limit should be set")
	}
	if account.CreditLimit.Money.String() != "5000" {
		t.Errorf("credit limit = %s, want 5000", account.CreditLimit.Money.String())
	}
}

func TestRun_AddAccountLoan(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-account",
		"--file", dbPath,
		"--name", "Car Loan",
		"--type", "loan",
		"--opening-balance", "-15000.00",
		"--interest-rate", "5.5",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Interest Rate") {
		t.Errorf("output should show interest rate, got: %s", output)
	}
	if !strings.Contains(output, "5.5%") {
		t.Errorf("output should show interest rate value, got: %s", output)
	}

	// Verify interest rate
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Car Loan")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if !account.InterestRate.Valid {
		t.Error("interest rate should be set")
	}
	if account.InterestRate.Money.String() != "5.5" {
		t.Errorf("interest rate = %s, want 5.5", account.InterestRate.Money.String())
	}
}

func TestRun_AddAccountAllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	accountTypes := []string{"checking", "savings", "credit_card", "investment", "cash", "loan", "asset"}

	for _, acctType := range accountTypes {
		t.Run(acctType, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			err = run([]string{
				"--add-account",
				"--file", dbPath,
				"--name", "Test " + acctType,
				"--type", acctType,
			}, stdout, stderr)
			if err != nil {
				t.Errorf("run(--add-account --type %s) returned error: %v", acctType, err)
				return
			}

			output := stdout.String()
			if !strings.Contains(output, "Account created successfully") {
				t.Errorf("output should confirm creation for type %s, got: %s", acctType, output)
			}
		})
	}
}

func TestRun_TransferMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--transfer", "--from", "Checking", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_TransferMissingFrom(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --from should return error")
	}
	if !strings.Contains(err.Error(), "requires --from") {
		t.Errorf("error should mention --from requirement, got: %v", err)
	}
}

func TestRun_TransferMissingTo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --to should return error")
	}
	if !strings.Contains(err.Error(), "requires --to") {
		t.Errorf("error should mention --to requirement, got: %v", err)
	}
}

func TestRun_TransferMissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("error should mention --amount requirement, got: %v", err)
	}
}

func TestRun_TransferInvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "not-a-number"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("error should mention invalid amount, got: %v", err)
	}
}

func TestRun_TransferNegativeAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "-100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with negative amount should return error")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("error should mention positive amount, got: %v", err)
	}
}

func TestRun_TransferSourceAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Nonexistent", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with nonexistent source account should return error")
	}
	if !strings.Contains(err.Error(), "source account") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention source account not found, got: %v", err)
	}
}

func TestRun_TransferDestAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Nonexistent", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with nonexistent destination account should return error")
	}
	if !strings.Contains(err.Error(), "destination account") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention destination account not found, got: %v", err)
	}
}

func TestRun_TransferBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "100.00"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transfer) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "From:") && !strings.Contains(output, "Checking") {
		t.Error("output should show source account")
	}
	if !strings.Contains(output, "To:") && !strings.Contains(output, "Savings") {
		t.Error("output should show destination account")
	}
	if !strings.Contains(output, "$100.00") {
		t.Error("output should show transfer amount")
	}
}

func TestRun_TransferWithDateAndMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "250.50",
		"--date", "2024-06-15",
		"--memo", "Monthly savings",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transfer) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "2024-06-15") {
		t.Error("output should show transfer date")
	}
	if !strings.Contains(output, "Monthly savings") {
		t.Error("output should show memo")
	}
	if !strings.Contains(output, "$250.50") {
		t.Error("output should show transfer amount")
	}
}

func TestRun_TransferInvalidDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100",
		"--date", "not-a-date",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_TransferVerifyTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	acctRepo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	// Run transfer
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--transfer) returned error: %v", err)
	}

	// Verify transactions were created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)

	// Check checking account transactions
	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("failed to list checking transactions: %v", err)
	}
	if len(checkingTxns) != 1 {
		t.Fatalf("expected 1 checking transaction, got %d", len(checkingTxns))
	}
	if !checkingTxns[0].Amount.Equal(models.MustNewMoney("-100.00")) {
		t.Errorf("checking transaction amount = %s, want -$100.00", checkingTxns[0].Amount.String())
	}
	if !checkingTxns[0].IsTransfer() {
		t.Error("checking transaction should be a transfer")
	}

	// Check savings account transactions
	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("failed to list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Fatalf("expected 1 savings transaction, got %d", len(savingsTxns))
	}
	if !savingsTxns[0].Amount.Equal(models.MustNewMoney("100.00")) {
		t.Errorf("savings transaction amount = %s, want $100.00", savingsTxns[0].Amount.String())
	}
	if !savingsTxns[0].IsTransfer() {
		t.Error("savings transaction should be a transfer")
	}

	// Verify they have the same transfer ID
	if checkingTxns[0].TransferID.ID != savingsTxns[0].TransferID.ID {
		t.Error("both transactions should have the same transfer ID")
	}
}

func TestRun_SearchMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--search", "amazon"}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_SearchByPayee(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Amazon")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-50.00"))
	txn1.SetPayee(payee.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	// Create another transaction without the payee
	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-25.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	database.Close()

	// Run search
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Amazon", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
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

func TestRun_SearchByMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transaction with memo
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-75.00"))
	txn1.SetMemo("Office supplies from Staples")
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Run search for memo content
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "office", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result for memo search, got: %s", output)
	}
}

func TestRun_SearchNoResults(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "nonexistent", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No transactions found") {
		t.Errorf("output should say no transactions found, got: %s", output)
	}
}

func TestRun_SearchWithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create two accounts
	acctRepo := repository.NewAccountRepository(database)
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("5000.00"), models.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Target")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions in both accounts
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(checking.ID, models.Today(), models.MustNewMoney("-50.00"))
	txn1.SetPayee(payee.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create checking transaction: %v", err)
	}

	txn2 := models.NewTransaction(savings.ID, models.Today(), models.MustNewMoney("-30.00"))
	txn2.SetPayee(payee.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create savings transaction: %v", err)
	}

	database.Close()

	// Search with account filter
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Target", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with account filter, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
}

func TestRun_SearchWithMinMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Store")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions with different amounts
	txnRepo := repository.NewTransactionRepository(database)
	amounts := []string{"-10.00", "-50.00", "-100.00", "-200.00"}
	for _, amt := range amounts {
		txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney(amt))
		txn.SetPayee(payee.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	// Search with min/max filters (for negative amounts, min means more negative)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Store", "--min", "-100.00", "--max", "-50.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 2 transaction(s)") {
		t.Errorf("output should show 2 results with amount filter, got: %s", output)
	}
}

func TestRun_SearchInvalidMinAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "test", "--min", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) with invalid --min should return error")
	}
	if !strings.Contains(err.Error(), "invalid --min") {
		t.Errorf("error should mention invalid --min, got: %v", err)
	}
}

func TestRun_SearchInvalidMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "test", "--max", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) with invalid --max should return error")
	}
	if !strings.Contains(err.Error(), "invalid --max") {
		t.Errorf("error should mention invalid --max, got: %v", err)
	}
}

func TestRun_SearchWithDateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions with different dates
	txnRepo := repository.NewTransactionRepository(database)
	jan15, _ := models.ParseDate("2024-01-15")
	feb15, _ := models.ParseDate("2024-02-15")
	mar15, _ := models.ParseDate("2024-03-15")

	dates := []models.Date{jan15, feb15, mar15}
	for _, d := range dates {
		txn := models.NewTransaction(account.ID, d, models.MustNewMoney("-5.00"))
		txn.SetPayee(payee.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	// Search with date filter
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Coffee", "--from", "2024-02-01", "--to", "2024-02-28", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with date filter, got: %s", output)
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain February transaction date")
	}
}

func TestRun_ScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--scheduled"}, stdout, stderr)
	if err == nil {
		t.Error("run(--scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_PostScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--post-scheduled", "abc123"}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_SkipScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--skip-scheduled", "abc123"}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ScheduledNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SCHEDULED TRANSACTIONS") {
		t.Error("output should contain SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "No scheduled transactions found") {
		t.Error("output should indicate no scheduled transactions found")
	}
}

func TestRun_ScheduledWithTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(),
		models.MustNewMoney("-15.99"),
	)
	st.SetPayee(payee.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SCHEDULED TRANSACTIONS") {
		t.Error("output should contain SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "-$15.99") {
		t.Error("output should contain amount")
	}
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Error("output should show count of scheduled transactions")
	}
}

func TestRun_ScheduledDueOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a due scheduled transaction (today)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(), // Due today
		models.MustNewMoney("-10.00"),
	)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled --due command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--due", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled --due) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "DUE SCHEDULED TRANSACTIONS") {
		t.Error("output should contain DUE SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Error("output should show count of due transactions")
	}
}

func TestRun_ScheduledFilterByAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create two test accounts
	acctRepo := repository.NewAccountRepository(database)
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("500.00"), models.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	// Create scheduled transactions for each account
	stRepo := repository.NewScheduledTransactionRepository(database)
	st1 := models.NewScheduledTransactionWithAmount(checking.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-10.00"))
	if err := stRepo.Create(st1); err != nil {
		t.Fatalf("failed to create scheduled transaction 1: %v", err)
	}
	st2 := models.NewScheduledTransactionWithAmount(savings.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-20.00"))
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction 2: %v", err)
	}

	database.Close()

	// Run the scheduled command filtered by account
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled --account Checking) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Errorf("output should show 1 scheduled transaction, got: %s", output)
	}
	if !strings.Contains(output, "-$10.00") {
		t.Error("output should contain the checking account scheduled transaction")
	}
}

func TestRun_PostScheduledInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestRun_PostScheduledNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Use a valid UUID format that doesn't exist
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_PostScheduledSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-15.99"))
	st.SetPayee(payee.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	// Post the scheduled transaction
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--post-scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "posted successfully") {
		t.Error("output should confirm posting")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "-$15.99") {
		t.Error("output should contain amount")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "Next:") {
		t.Error("output should show next date")
	}

	// Verify the transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	txns, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(txns))
	}
}

func TestRun_PostScheduledWithCustomAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction (variable amount)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	// Post with custom amount
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", stID, "--amount", "-25.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--post-scheduled) with --amount returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "-$25.00") {
		t.Error("output should contain custom amount")
	}
}

func TestRun_SkipScheduledInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestRun_SkipScheduledNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_SkipScheduledSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-15.99"))
	st.SetPayee(payee.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()
	originalNextDate := st.NextDate.String()

	database.Close()

	// Skip the scheduled transaction
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--skip-scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "skipped") {
		t.Error("output should confirm skipping")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "Skipped:") {
		t.Error("output should show skipped date")
	}
	if !strings.Contains(output, originalNextDate) {
		t.Error("output should show original date in Skipped field")
	}
	if !strings.Contains(output, "Next:") {
		t.Error("output should show next date")
	}

	// Verify no transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	txns, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("expected 0 transactions after skip, got %d", len(txns))
	}
}

func TestRun_ScheduledVariableAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with variable amount (no amount set)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	// Variable amount should show as "~"
	if !strings.Contains(output, "~") {
		t.Error("output should show ~ for variable amount")
	}
}

func TestRun_ScheduledWithOccurrences(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with limited occurrences
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-50.00"))
	st.SetOccurrences(5)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	// Should show occurrences remaining
	if !strings.Contains(output, "(5 left)") {
		t.Error("output should show occurrences remaining")
	}
}

func TestRun_ReportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--report", "net-worth"}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ReportMissingType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) without report type should return error")
	}
	if !strings.Contains(err.Error(), "requires a report type") {
		t.Errorf("error should mention report type requirement, got: %v", err)
	}
}

func TestRun_ReportInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "invalid-type", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "unknown report type") {
		t.Errorf("error should mention unknown report type, got: %v", err)
	}
}

func TestRun_ReportNetWorthEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "NET WORTH REPORT") {
		t.Error("output should contain NET WORTH REPORT header")
	}
	if !strings.Contains(output, "ASSETS") {
		t.Error("output should contain ASSETS section")
	}
	if !strings.Contains(output, "LIABILITIES") {
		t.Error("output should contain LIABILITIES section")
	}
	if !strings.Contains(output, "NET WORTH:") {
		t.Error("output should contain NET WORTH summary")
	}
}

func TestRun_ReportNetWorthWithAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create asset and liability accounts
	acctRepo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("5000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("10000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	creditCard := models.NewAccount(
		"Credit Card",
		models.AccountTypeCreditCard,
		"USD",
		models.MustNewMoney("0"),
		models.Today(),
	)
	if err := acctRepo.Create(creditCard); err != nil {
		t.Fatalf("failed to create credit card account: %v", err)
	}

	// Add a credit card transaction (liability)
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(creditCard.ID, models.Today(), models.MustNewMoney("-500.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
	if !strings.Contains(output, "Savings") {
		t.Error("output should contain Savings account")
	}
	if !strings.Contains(output, "Credit Card") {
		t.Error("output should contain Credit Card account")
	}
}

func TestRun_ReportNetWorthWithAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--as-of", "2024-01-15", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth --as-of) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "January 15, 2024") {
		t.Error("output should show the as-of date")
	}
}

func TestRun_ReportNetWorthInvalidAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--as-of", "invalid-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report net-worth) with invalid as-of date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --as-of date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_ReportSpendingMissingPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) without period should return error")
	}
	if !strings.Contains(err.Error(), "requires --month") {
		t.Errorf("error should mention period requirement, got: %v", err)
	}
}

func TestRun_ReportSpendingByMonth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --month) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "January 2024") {
		t.Error("output should show the period")
	}
}

func TestRun_ReportSpendingByYear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--year", "2024", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --year) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "2024") {
		t.Error("output should show the year")
	}
}

func TestRun_ReportSpendingByDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--from", "2024-01-01", "--to", "2024-06-30", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --from --to) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "2024-01-01 to 2024-06-30") {
		t.Error("output should show the date range")
	}
}

func TestRun_ReportSpendingWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("5000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create expense category
	catRepo := repository.NewCategoryRepository(database)
	groceries := models.NewCategory("Groceries", models.CategoryTypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create expense transaction
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-01-15"), models.MustNewMoney("-150.00"))
	txn.SetCategory(groceries.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Groceries") {
		t.Error("output should contain Groceries category")
	}
	if !strings.Contains(output, "$150.00") {
		t.Error("output should show spending amount")
	}
	if !strings.Contains(output, "100.0%") {
		t.Error("output should show percentage")
	}
}

func TestRun_ReportSpendingInvalidMonth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) with invalid month should return error")
	}
	if !strings.Contains(err.Error(), "invalid --month format") {
		t.Errorf("error should mention invalid month format, got: %v", err)
	}
}

func TestRun_ReportSpendingInvalidMonthValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-13", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) with month > 12 should return error")
	}
	if !strings.Contains(err.Error(), "month must be between 1 and 12") {
		t.Errorf("error should mention month range, got: %v", err)
	}
}

func TestRun_VoidMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--void", "abc123"}, stdout, stderr)
	if err == nil {
		t.Error("run(--void) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_VoidInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--void", "not-a-valid-id", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--void) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestRun_VoidTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a transaction
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	// Void the transaction
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--void", txnID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--void) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transaction voided successfully") {
		t.Error("output should confirm void")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Void") {
		t.Error("output should show void status")
	}

	// Verify the transaction is now void
	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo2 := repository.NewTransactionRepository(database2)
	voidedTxn, err := txnRepo2.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("failed to get voided transaction: %v", err)
	}
	if voidedTxn.Status != models.TransactionStatusVoid {
		t.Errorf("transaction status should be void, got %q", voidedTxn.Status)
	}
	if !voidedTxn.Amount.IsZero() {
		t.Errorf("voided transaction amount should be zero, got %s", voidedTxn.Amount.String())
	}
}

func TestRun_VoidAlreadyVoidTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create account and void transaction
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-50.00"))
	txn.Void()
	txn.Amount = models.ZeroMoney
	txn.SetMemo("**VOID**")
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txnID := txn.ID.String()
	database.Close()

	// Try to void again
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--void", txnID, "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("voiding an already void transaction should return error")
	}
}

func TestRun_TransactionsWithStatusFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transactions with different statuses
	txnRepo := repository.NewTransactionRepository(database)

	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
	// Default status is uncleared
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-20.00"))
	txn2.Clear()
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	txn3 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-30.00"))
	txn3.Clear()
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("failed to create transaction 3: %v", err)
	}

	database.Close()

	// Filter by cleared status
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--status", "cleared", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions --status cleared) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 2 transaction(s)") {
		t.Errorf("should show 2 cleared transactions, got: %s", output)
	}

	// Filter by uncleared status
	stdout.Reset()
	stderr.Reset()
	err = run([]string{"--transactions", "--account", "Checking", "--status", "uncleared", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions --status uncleared) returned error: %v", err)
		return
	}

	output = stdout.String()
	if !strings.Contains(output, "Showing 1 transaction(s)") {
		t.Errorf("should show 1 uncleared transaction, got: %s", output)
	}
}

func TestRun_TransactionsWithInvalidStatusFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--status", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions --status invalid) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --status") {
		t.Errorf("error should mention invalid status, got: %v", err)
	}
}

func TestRun_TransactionsStatusCodeDisplay(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)

	// Uncleared transaction
	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Cleared transaction
	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-20.00"))
	txn2.Clear()
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions) returned error: %v", err)
		return
	}

	output := stdout.String()
	// Status column should show single-letter codes (tabwriter converts tabs to spaces)
	if !strings.Contains(output, "  U\n") {
		t.Errorf("output should contain status code U for uncleared transaction, got:\n%s", output)
	}
	if !strings.Contains(output, "  C\n") {
		t.Errorf("output should contain status code C for cleared transaction, got:\n%s", output)
	}
	// Should NOT contain full status words in the table
	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		if strings.Contains(line, "Uncleared") && !strings.Contains(line, "Status") {
			t.Error("output should use status codes, not full words like 'Uncleared'")
		}
	}
}

// --- Reconciliation CLI Tests ---

func TestRun_StartReconcileMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--account", "Checking", "--statement-date", "2024-01-31", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_StartReconcileMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--statement-date", "2024-01-31", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_StartReconcileMissingDate(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--account", "Checking", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --statement-date")
	}
	if !strings.Contains(err.Error(), "requires --statement-date") {
		t.Errorf("error should mention --statement-date, got: %v", err)
	}
}

func TestRun_StartReconcileMissingBalance(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--account", "Checking", "--statement-date", "2024-01-31"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --statement-balance")
	}
	if !strings.Contains(err.Error(), "requires --statement-balance") {
		t.Errorf("error should mention --statement-balance, got: %v", err)
	}
}

func TestRun_StartReconcileSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create some transactions
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-05"), models.MustNewMoney("-50.00"))
	txn2 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-10"), models.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--start-reconcile",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "850.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--start-reconcile) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation started for Checking") {
		t.Error("output should confirm reconciliation started")
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$850.00") {
		t.Error("output should contain statement balance")
	}
	if !strings.Contains(output, "Unreconciled transactions: 2") {
		t.Errorf("output should show 2 unreconciled transactions, got:\n%s", output)
	}
}

func TestRun_StartReconcileAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--start-reconcile",
		"--file", dbPath,
		"--account", "NonExistent",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Error("should fail with nonexistent account")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestRun_MarkReconciledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--mark-reconciled", "some-id", "--file", ""}, stdout, stderr)
	// --mark-reconciled without --file should fail
	if err == nil {
		t.Error("should fail without proper --file")
	}
}

func TestRun_MarkReconciledNoIDs(t *testing.T) {
	_, _, err := parseArgs([]string{"--mark-reconciled"})
	if err == nil {
		t.Error("--mark-reconciled without IDs should return parse error")
	}
	if !strings.Contains(err.Error(), "requires at least one") {
		t.Errorf("error should mention requiring IDs, got: %v", err)
	}
}

func TestRun_FinishReconcileMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--finish-reconcile", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_FinishReconcileMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--finish-reconcile", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_FinishReconcileNoActiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail with no active session")
	}
	if !strings.Contains(err.Error(), "no active reconciliation") {
		t.Errorf("error should mention no active session, got: %v", err)
	}
}

func TestRun_FinishReconcileSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account with opening balance 1000
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transactions: -50 and -100 = 850 total balance
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-05"), models.MustNewMoney("-50.00"))
	txn2 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-10"), models.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Start a reconciliation session
	reconRepo := repository.NewReconciliationRepository(database)
	session := models.NewReconciliationSession(
		account.ID,
		models.MustParseDate("2024-01-31"),
		models.MustNewMoney("850.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish reconciliation
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--finish-reconcile) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation completed for Checking") {
		t.Errorf("output should confirm completion, got:\n%s", output)
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$850.00") {
		t.Error("output should contain balance")
	}

	// Verify transactions are now reconciled
	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo2 := repository.NewTransactionRepository(database2)
	reconciledTxn1, err := txnRepo2.GetByID(txn1.ID)
	if err != nil {
		t.Fatalf("failed to get transaction: %v", err)
	}
	if reconciledTxn1.Status != models.TransactionStatusReconciled {
		t.Errorf("transaction 1 should be reconciled, got %q", reconciledTxn1.Status)
	}

	reconciledTxn2, err := txnRepo2.GetByID(txn2.ID)
	if err != nil {
		t.Fatalf("failed to get transaction: %v", err)
	}
	if reconciledTxn2.Status != models.TransactionStatusReconciled {
		t.Errorf("transaction 2 should be reconciled, got %q", reconciledTxn2.Status)
	}
}

func TestRun_FinishReconcileWithDifference(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a transaction: balance is 1000 - 50 = 950
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-01-05"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Create session with wrong balance (should cause difference)
	reconRepo := repository.NewReconciliationRepository(database)
	session := models.NewReconciliationSession(
		account.ID,
		models.MustParseDate("2024-01-31"),
		models.MustNewMoney("5000.00"), // Wrong balance
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish should fail due to difference
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail with non-zero difference")
	}
	if !strings.Contains(err.Error(), "Difference") {
		t.Errorf("error should mention difference, got: %v", err)
	}
}

func TestRun_FinishReconcileWithForce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-01-05"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	reconRepo := repository.NewReconciliationRepository(database)
	session := models.NewReconciliationSession(
		account.ID,
		models.MustParseDate("2024-01-31"),
		models.MustNewMoney("5000.00"), // Wrong balance
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish with --force should succeed
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking", "--force"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--finish-reconcile --force) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation completed for Checking") {
		t.Errorf("output should confirm completion, got:\n%s", output)
	}
}

func TestRun_ReconcileStatusMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--reconcile-status", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_ReconcileStatusMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--reconcile-status", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_ReconcileStatusNoSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--reconcile-status", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--reconcile-status) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Error("output should contain status header")
	}
	if !strings.Contains(output, "Last reconciled:  Never") {
		t.Errorf("output should show 'Never' for last reconciled, got:\n%s", output)
	}
	if !strings.Contains(output, "Current session:  None") {
		t.Errorf("output should show 'None' for current session, got:\n%s", output)
	}
}

func TestRun_ReconcileStatusWithActiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create an active reconciliation session
	reconRepo := repository.NewReconciliationRepository(database)
	session := models.NewReconciliationSession(
		account.ID,
		models.MustParseDate("2024-01-31"),
		models.MustNewMoney("5000.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--reconcile-status", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--reconcile-status) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Error("output should contain status header")
	}
	if !strings.Contains(output, "In progress") {
		t.Errorf("output should show 'In progress', got:\n%s", output)
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$5000.00") {
		t.Errorf("output should contain statement balance, got:\n%s", output)
	}
}

func TestRun_FullReconciliationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create account with opening balance 1000
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transactions: -200 and +500 = 1300 total
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-05"), models.MustNewMoney("-200.00"))
	txn2 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-15"), models.MustNewMoney("500.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Step 1: Start reconciliation
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--start-reconcile",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "1300.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("start reconcile failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Reconciliation started") {
		t.Error("should confirm reconciliation started")
	}

	// Step 2: Check status
	stdout.Reset()
	err = run([]string{
		"--reconcile-status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("reconcile status failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "In progress") {
		t.Error("status should show in progress")
	}

	// Step 3: Finish reconciliation
	stdout.Reset()
	err = run([]string{
		"--finish-reconcile",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("finish reconcile failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Reconciliation completed") {
		t.Error("should confirm reconciliation completed")
	}

	// Step 4: Verify status shows completed
	stdout.Reset()
	err = run([]string{
		"--reconcile-status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("reconcile status after completion failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Last reconciled:  2024-01-31") {
		t.Errorf("status should show last reconciled date, got:\n%s", output)
	}
	if !strings.Contains(output, "Current session:  None") {
		t.Errorf("status should show no current session, got:\n%s", output)
	}
}

func TestParseArgs_ReconciliationFlags(t *testing.T) {
	// Test --start-reconcile flag parsing
	opts, _, err := parseArgs([]string{
		"--start-reconcile",
		"--file", "test.tdb",
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000.00",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.startReconcile {
		t.Error("startReconcile should be true")
	}
	if opts.statementDate != "2024-01-31" {
		t.Errorf("statementDate should be 2024-01-31, got %q", opts.statementDate)
	}
	if opts.statementBalance != "5000.00" {
		t.Errorf("statementBalance should be 5000.00, got %q", opts.statementBalance)
	}

	// Test --finish-reconcile with --force
	opts, _, err = parseArgs([]string{
		"--finish-reconcile",
		"--force",
		"--file", "test.tdb",
		"--account", "Checking",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.finishReconcile {
		t.Error("finishReconcile should be true")
	}
	if !opts.reconcileForce {
		t.Error("reconcileForce should be true")
	}

	// Test --mark-reconciled with multiple IDs
	opts, _, err = parseArgs([]string{
		"--mark-reconciled", "id1", "id2", "id3",
		"--file", "test.tdb",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if len(opts.markReconciled) != 3 {
		t.Errorf("markReconciled should have 3 IDs, got %d", len(opts.markReconciled))
	}

	// Test --reconcile-status
	opts, _, err = parseArgs([]string{
		"--reconcile-status",
		"--file", "test.tdb",
		"--account", "Checking",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.reconcileStatus {
		t.Error("reconcileStatus should be true")
	}

	// Test = form for statement-date and statement-balance
	opts, _, err = parseArgs([]string{
		"--start-reconcile",
		"--statement-date=2024-02-28",
		"--statement-balance=1234.56",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if opts.statementDate != "2024-02-28" {
		t.Errorf("statementDate should be 2024-02-28, got %q", opts.statementDate)
	}
	if opts.statementBalance != "1234.56" {
		t.Errorf("statementBalance should be 1234.56, got %q", opts.statementBalance)
	}
}

// =============================================================================
// Add Scheduled Transaction Tests
// =============================================================================

func TestRun_AddScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-scheduled", "--account", "Checking", "--frequency", "monthly"}, stdout, stderr)
	if err == nil {
		t.Error("--add-scheduled without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddScheduledMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-scheduled", "--file", "test.tdb", "--frequency", "monthly"}, stdout, stderr)
	if err == nil {
		t.Error("--add-scheduled without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_AddScheduledMissingFrequency(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-scheduled", "--file", "test.tdb", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("--add-scheduled without --frequency should return error")
	}
	if !strings.Contains(err.Error(), "requires --frequency") {
		t.Errorf("error should mention --frequency requirement, got: %v", err)
	}
}

func TestRun_AddScheduledInvalidFrequency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-scheduled", "--file", dbPath, "--account", "Checking", "--frequency", "invalid"}, stdout, stderr)
	if err == nil {
		t.Error("--add-scheduled with invalid frequency should return error")
	}
	if !strings.Contains(err.Error(), "invalid --frequency") {
		t.Errorf("error should mention invalid frequency, got: %v", err)
	}
}

func TestRun_AddScheduledSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-1500",
		"--payee", "Landlord",
		"--memo", "Monthly rent",
		"--day", "1",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--add-scheduled) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Scheduled transaction created successfully!") {
		t.Error("output should contain success message")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "-$1500.00") {
		t.Error("output should contain amount")
	}
	if !strings.Contains(output, "Landlord") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Monthly rent") {
		t.Error("output should contain memo")
	}
}

func TestRun_AddScheduledWithAutoPost(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-150",
		"--payee", "Insurance",
		"--auto-post",
		"--lead-days", "3",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--add-scheduled --auto-post) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Auto-post: Yes (3 days early)") {
		t.Errorf("output should contain auto-post indicator, got: %s", output)
	}
}

func TestRun_AddScheduledLeadDaysWithoutAutoPost(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--lead-days", "3",
	}, stdout, stderr)
	if err == nil {
		t.Error("--lead-days without --auto-post should return error")
	}
	if !strings.Contains(err.Error(), "requires --auto-post") {
		t.Errorf("error should mention --auto-post requirement, got: %v", err)
	}
}

func TestRun_AddScheduledInvalidLeadDays(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--auto-post",
		"--lead-days", "5",
	}, stdout, stderr)
	if err == nil {
		t.Error("--lead-days 5 should return error")
	}
	if !strings.Contains(err.Error(), "must be 0, 3, or 7") {
		t.Errorf("error should mention valid lead-days values, got: %v", err)
	}
}

func TestRun_AddScheduledWithOccurrences(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--occurrences", "12",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--add-scheduled --occurrences) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Scheduled transaction created successfully!") {
		t.Error("output should contain success message")
	}
}

func TestRun_AddScheduledVariableAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create without --amount to make it variable
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-scheduled", "--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--payee", "Electric Co",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--add-scheduled variable amount) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Variable") {
		t.Error("output should show variable amount")
	}
}

// =============================================================================
// Scheduled Transactions Auto-Post Indicator Tests
// =============================================================================

func TestRun_ScheduledShowsAutoPostIndicator(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with auto-post
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(),
		models.MustNewMoney("-1500.00"),
	)
	st.SetAutoPost(true)
	st.SetPostLeadDays(3)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	// Create another without auto-post
	st2 := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(),
		models.MustNewMoney("-50.00"),
	)
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--scheduled) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto 3d]") {
		t.Errorf("output should contain [Auto 3d] indicator, got: %s", output)
	}
	if !strings.Contains(output, "Auto") {
		t.Error("output should contain Auto header column")
	}
}

func TestRun_ScheduledAutoPostZeroLeadDays(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(),
		models.MustNewMoney("-100.00"),
	)
	st.SetAutoPost(true)
	// PostLeadDays defaults to 0
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--scheduled) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto]") {
		t.Errorf("output should contain [Auto] indicator, got: %s", output)
	}
}

// =============================================================================
// Parse Args Tests for New Flags
// =============================================================================

func TestParseArgs_AddScheduledFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--add-scheduled",
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--auto-post",
		"--lead-days", "3",
		"--day", "15",
		"--occurrences", "12",
		"--end-date", "2025-12-31",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}

	if !opts.addScheduled {
		t.Error("addScheduled should be true")
	}
	if opts.stFrequency != "monthly" {
		t.Errorf("stFrequency should be monthly, got %q", opts.stFrequency)
	}
	if !opts.autoPost {
		t.Error("autoPost should be true")
	}
	if opts.leadDays != "3" {
		t.Errorf("leadDays should be 3, got %q", opts.leadDays)
	}
	if opts.stDay != "15" {
		t.Errorf("stDay should be 15, got %q", opts.stDay)
	}
	if opts.stOccurrences != "12" {
		t.Errorf("stOccurrences should be 12, got %q", opts.stOccurrences)
	}
	if opts.stEndDate != "2025-12-31" {
		t.Errorf("stEndDate should be 2025-12-31, got %q", opts.stEndDate)
	}
}

func TestParseArgs_AddScheduledEqualsFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--add-scheduled",
		"--frequency=weekly",
		"--day=5",
		"--occurrences=4",
		"--end-date=2025-06-30",
		"--lead-days=7",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}

	if opts.stFrequency != "weekly" {
		t.Errorf("stFrequency should be weekly, got %q", opts.stFrequency)
	}
	if opts.stDay != "5" {
		t.Errorf("stDay should be 5, got %q", opts.stDay)
	}
	if opts.stOccurrences != "4" {
		t.Errorf("stOccurrences should be 4, got %q", opts.stOccurrences)
	}
	if opts.stEndDate != "2025-06-30" {
		t.Errorf("stEndDate should be 2025-06-30, got %q", opts.stEndDate)
	}
	if opts.leadDays != "7" {
		t.Errorf("leadDays should be 7, got %q", opts.leadDays)
	}
}

// --- Import CLI tests ---

func TestParseArgs_ImportFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--import", "bank.csv",
		"--account", "Checking",
		"--file", "test.tdb",
		"--confirm",
		"--skip-duplicates",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.importFile != "bank.csv" {
		t.Errorf("importFile should be bank.csv, got %q", opts.importFile)
	}
	if opts.accountName != "Checking" {
		t.Errorf("accountName should be Checking, got %q", opts.accountName)
	}
	if !opts.confirm {
		t.Error("confirm should be true")
	}
	if !opts.skipDuplicates {
		t.Error("skipDuplicates should be true")
	}
}

func TestParseArgs_ImportFlagsEqualFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--import=bank.txt",
		"--format=csv",
		"--account", "Checking",
		"--file", "test.tdb",
		"--update-duplicates",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.importFile != "bank.txt" {
		t.Errorf("importFile should be bank.txt, got %q", opts.importFile)
	}
	if opts.formatOverride != "csv" {
		t.Errorf("formatOverride should be csv, got %q", opts.formatOverride)
	}
	if !opts.updateDuplicates {
		t.Error("updateDuplicates should be true")
	}
}

func TestParseArgs_ImportMissingFile(t *testing.T) {
	_, _, err := parseArgs([]string{"--import"})
	if err == nil {
		t.Error("parseArgs should return error for --import without argument")
	}
}

func TestParseArgs_FormatMissing(t *testing.T) {
	_, _, err := parseArgs([]string{"--format"})
	if err == nil {
		t.Error("parseArgs should return error for --format without argument")
	}
}

func TestRun_ImportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import", "bank.csv", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ImportMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import", "bank.csv", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_ImportMutuallyExclusiveDuplicateFlags(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--skip-duplicates",
		"--update-duplicates",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with both --skip-duplicates and --update-duplicates should return error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

func TestRun_ImportInvalidFormat(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--format", "xml",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with invalid --format should return error")
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestRun_ImportNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", "/nonexistent/bank.csv",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with nonexistent import file should return error")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error should mention failed to open, got: %v", err)
	}
}

func TestRun_ImportCSVDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file for import
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Category,Memo\n2024-03-01,-50.00,Coffee Shop,Food:Coffee,Morning coffee\n2024-03-02,-120.00,Electric Co,Bills:Utilities,March electric\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import dry-run) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "2 transactions") {
		t.Error("output should show 2 parsed transactions")
	}
	if !strings.Contains(output, "Run with --confirm") {
		t.Error("output should prompt to run with --confirm")
	}
}

func TestRun_ImportCSVConfirm(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file for import
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Memo\n2024-03-01,-50.00,Coffee Shop,Morning coffee\n2024-03-02,-120.00,Electric Co,March electric\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--confirm",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --confirm) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE") {
		t.Error("output should contain IMPORT COMPLETE header")
	}
	if !strings.Contains(output, "Created:  2") {
		t.Errorf("output should show 2 created transactions, got: %s", output)
	}

	// Verify transactions were actually created
	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo := repository.NewTransactionRepository(database2)
	txns, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txns))
	}
}

func TestRun_ImportClosedAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	account := models.NewAccount("Closed Account", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	account.Active = false
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Closed Account",
	}, stdout, stderr)
	if err == nil {
		t.Error("import into closed account should return error")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should mention account is closed, got: %v", err)
	}
}

func TestRun_ImportFormatOverride(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file with a .txt extension
	csvPath := filepath.Join(tmpDir, "import.txt")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --format csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestRun_ImportSkipDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	accountRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create an existing transaction that should match an import row
	txnRepo := repository.NewTransactionRepository(database)
	existingTxn := models.NewTransaction(account.ID, models.MustParseDate("2024-03-01"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(existingTxn); err != nil {
		t.Fatalf("failed to create existing transaction: %v", err)
	}

	database.Close()

	// Create a CSV file with a matching transaction
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n2024-03-02,-75.00,Gas Station\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--skip-duplicates",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --skip-duplicates) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestRun_ImportAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Error("import with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

// writeTestFile is a test helper that writes content to a file.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// --- Export CLI tests ---

func TestParseArgs_ExportFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--export", "out.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--format", "csv",
		"--from", "2024-01-01",
		"--to", "2024-12-31",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.exportFile != "out.csv" {
		t.Errorf("exportFile should be out.csv, got %q", opts.exportFile)
	}
	if opts.accountName != "Checking" {
		t.Errorf("accountName should be Checking, got %q", opts.accountName)
	}
	if opts.formatOverride != "csv" {
		t.Errorf("formatOverride should be csv, got %q", opts.formatOverride)
	}
	if opts.fromDate != "2024-01-01" {
		t.Errorf("fromDate should be 2024-01-01, got %q", opts.fromDate)
	}
	if opts.toDate != "2024-12-31" {
		t.Errorf("toDate should be 2024-12-31, got %q", opts.toDate)
	}
}

func TestParseArgs_ExportFlagsEqualForm(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--export=out.qif",
		"--file", "test.tdb",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.exportFile != "out.qif" {
		t.Errorf("exportFile should be out.qif, got %q", opts.exportFile)
	}
}

func TestParseArgs_ExportMissingFile(t *testing.T) {
	_, _, err := parseArgs([]string{"--export"})
	if err == nil {
		t.Error("parseArgs should return error for --export without argument")
	}
}

func TestRun_ExportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--export", "out.csv"}, stdout, stderr)
	if err == nil {
		t.Error("run(--export) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ExportUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", filepath.Join(tmpDir, "out.csv"),
		"--file", dbPath,
		"--format", "ofx",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with OFX format should return error")
	}
	if !strings.Contains(err.Error(), "must be csv or qif") {
		t.Errorf("error should mention valid formats, got: %v", err)
	}
}

func TestRun_ExportCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	// Create database with account and transactions
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.MustParseDate("2024-03-01"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := models.NewTransaction(account.ID, models.MustParseDate("2024-03-15"), models.MustNewMoney("-120.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export to CSV
	exportPath := filepath.Join(tmpDir, "export.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "EXPORT COMPLETE") {
		t.Error("output should contain EXPORT COMPLETE header")
	}
	if !strings.Contains(output, "Transactions: 2") {
		t.Errorf("output should show 2 transactions, got: %s", output)
	}
	if !strings.Contains(output, "CSV") {
		t.Error("output should show CSV format")
	}

	// Verify the file was created and has content
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	csvStr := string(content)
	if !strings.Contains(csvStr, "Date,Account,Payee,Category,Amount") {
		t.Error("CSV should contain header row")
	}
	if !strings.Contains(csvStr, "2024-03-01") {
		t.Error("CSV should contain first transaction date")
	}
	if !strings.Contains(csvStr, "2024-03-15") {
		t.Error("CSV should contain second transaction date")
	}
}

func TestRun_ExportQIF(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-06-01"), models.MustNewMoney("-75.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	exportPath := filepath.Join(tmpDir, "export.qif")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export QIF) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "QIF") {
		t.Error("output should show QIF format")
	}

	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	qifStr := string(content)
	if !strings.Contains(qifStr, "!Type:") {
		t.Error("QIF should contain type header")
	}
	if !strings.Contains(qifStr, "T-75.00") {
		t.Error("QIF should contain transaction amount")
	}
}

func TestRun_ExportWithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("5000"), models.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(checking.ID, models.MustParseDate("2024-03-01"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := models.NewTransaction(savings.ID, models.MustParseDate("2024-03-01"), models.MustNewMoney("-25.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export only Checking account
	exportPath := filepath.Join(tmpDir, "checking.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --account) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Accounts:     1") {
		t.Errorf("output should show 1 account, got: %s", output)
	}
	if !strings.Contains(output, "Transactions: 1") {
		t.Errorf("output should show 1 transaction, got: %s", output)
	}
}

func TestRun_ExportWithDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.MustParseDate("2024-01-15"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := models.NewTransaction(account.ID, models.MustParseDate("2024-03-15"), models.MustNewMoney("-75.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn3 := models.NewTransaction(account.ID, models.MustParseDate("2024-06-15"), models.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export only Q1 2024
	exportPath := filepath.Join(tmpDir, "q1.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--from", "2024-01-01",
		"--to", "2024-03-31",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --from --to) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Transactions: 2") {
		t.Errorf("output should show 2 transactions for Q1, got: %s", output)
	}
}

func TestRun_ExportFormatOverrideCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-03-01"), models.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export with .txt extension but force CSV format
	exportPath := filepath.Join(tmpDir, "export.txt")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --format csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "CSV") {
		t.Error("output should show CSV format")
	}
}

func TestRun_ExportAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestRun_ExportNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Empty", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Empty",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with no transactions should return error")
	}
	if !strings.Contains(err.Error(), "no transactions") {
		t.Errorf("error should mention no transactions, got: %v", err)
	}
}

func TestRun_ExportUndetectableFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.xyz")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Error("export with undetectable format should return error")
	}
	if !strings.Contains(err.Error(), "cannot detect format") {
		t.Errorf("error should mention format detection failure, got: %v", err)
	}
}
