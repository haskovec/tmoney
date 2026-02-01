package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// TestTransactionLifecycle tests the complete transaction lifecycle:
// create database -> create account -> create transaction -> list -> update -> delete -> cleanup
func TestTransactionLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)

	// Create an account first (required for transactions)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.NewDate(2024, 1, 1),
	)
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	var transactionID models.ID

	// Step 1: Create a transaction
	t.Run("Create transaction", func(t *testing.T) {
		txn := models.NewTransaction(
			account.ID,
			models.NewDate(2024, 1, 15),
			models.MustNewMoney("-50.00"),
		)
		txn.SetMemo("Coffee shop")

		err := txnRepo.Create(txn)
		if err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
		transactionID = txn.ID
	})

	// Step 2: List transactions
	t.Run("List transactions", func(t *testing.T) {
		transactions, err := txnRepo.List()
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}

		if len(transactions) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(transactions))
		}

		retrieved := transactions[0]
		if !retrieved.Amount.Equal(models.MustNewMoney("-50.00")) {
			t.Errorf("Expected amount '-50.00', got %q", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Coffee shop" {
			t.Errorf("Expected memo 'Coffee shop', got %v", retrieved.Memo)
		}
		if retrieved.Status != models.TransactionStatusPending {
			t.Errorf("Expected status 'pending', got %q", retrieved.Status)
		}
	})

	// Step 3: Get by ID
	t.Run("Get transaction by ID", func(t *testing.T) {
		txn, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}
		if txn.ID != transactionID {
			t.Errorf("Expected ID %s, got %s", transactionID.String(), txn.ID.String())
		}
	})

	// Step 4: Update transaction
	t.Run("Update transaction", func(t *testing.T) {
		txn, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}

		txn.Clear() // Mark as cleared
		txn.SetMemo("Coffee shop - updated")

		err = txnRepo.Update(txn)
		if err != nil {
			t.Fatalf("Failed to update transaction: %v", err)
		}

		// Verify update
		updated, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get updated transaction: %v", err)
		}
		if updated.Status != models.TransactionStatusCleared {
			t.Errorf("Expected status 'cleared', got %q", updated.Status)
		}
		if !updated.Memo.Valid || updated.Memo.String != "Coffee shop - updated" {
			t.Errorf("Expected memo 'Coffee shop - updated', got %v", updated.Memo)
		}
	})

	// Step 5: Delete transaction
	t.Run("Delete transaction", func(t *testing.T) {
		err := txnRepo.Delete(transactionID)
		if err != nil {
			t.Fatalf("Failed to delete transaction: %v", err)
		}

		// Verify deletion
		transactions, err := txnRepo.List()
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}
		if len(transactions) != 0 {
			t.Errorf("Expected 0 transactions after deletion, got %d", len(transactions))
		}
	})
}

// TestTransactionListByAccount tests listing transactions by account.
func TestTransactionListByAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)

	// Create two accounts
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today())

	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	if err := accountRepo.Create(savings); err != nil {
		t.Fatalf("Failed to create savings account: %v", err)
	}

	// Create transactions for each account
	txn1 := models.NewTransaction(checking.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-100.00"))
	txn2 := models.NewTransaction(checking.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("-50.00"))
	txn3 := models.NewTransaction(savings.ID, models.NewDate(2024, 1, 12), models.MustNewMoney("500.00"))

	for _, txn := range []*models.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	// List transactions by checking account
	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("Failed to list checking transactions: %v", err)
	}
	if len(checkingTxns) != 2 {
		t.Errorf("Expected 2 checking transactions, got %d", len(checkingTxns))
	}

	// List transactions by savings account
	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("Failed to list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Errorf("Expected 1 savings transaction, got %d", len(savingsTxns))
	}

	// Verify ordering (most recent first)
	if checkingTxns[0].Date.Before(checkingTxns[1].Date) {
		t.Error("Transactions should be ordered by date descending")
	}
}

// TestTransactionListByDateRange tests listing transactions by date range.
func TestTransactionListByDateRange(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)

	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create transactions on different dates
	dates := []models.Date{
		models.NewDate(2024, 1, 5),
		models.NewDate(2024, 1, 15),
		models.NewDate(2024, 1, 25),
		models.NewDate(2024, 2, 5),
	}

	for i, date := range dates {
		txn := models.NewTransaction(account.ID, date, models.MustNewMoney("-10.00"))
		txn.SetMemo("Transaction " + string(rune('A'+i)))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	// Query for January only
	janTxns, err := txnRepo.ListByDateRange(
		models.NewDate(2024, 1, 1),
		models.NewDate(2024, 1, 31),
	)
	if err != nil {
		t.Fatalf("Failed to list January transactions: %v", err)
	}
	if len(janTxns) != 3 {
		t.Errorf("Expected 3 January transactions, got %d", len(janTxns))
	}

	// Query for mid-January
	midJanTxns, err := txnRepo.ListByDateRange(
		models.NewDate(2024, 1, 10),
		models.NewDate(2024, 1, 20),
	)
	if err != nil {
		t.Fatalf("Failed to list mid-January transactions: %v", err)
	}
	if len(midJanTxns) != 1 {
		t.Errorf("Expected 1 mid-January transaction, got %d", len(midJanTxns))
	}
}

// TestTransactionNotFound tests error handling for non-existent transactions.
func TestTransactionNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)

	// Try to get non-existent transaction
	_, err = txnRepo.GetByID(models.NewID())
	if err == nil {
		t.Error("Expected error for non-existent transaction")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to delete non-existent transaction
	err = txnRepo.Delete(models.NewID())
	if err == nil {
		t.Error("Expected error for deleting non-existent transaction")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

// TestTransactionRequiresValidAccount tests that transactions require a valid account.
func TestTransactionRequiresValidAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)

	// Try to create transaction with non-existent account
	txn := models.NewTransaction(
		models.NewID(), // Random account ID that doesn't exist
		models.Today(),
		models.MustNewMoney("-50.00"),
	)

	err = txnRepo.Create(txn)
	if err == nil {
		t.Error("Expected error when creating transaction with non-existent account")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

// TestTransactionCountByAccount tests the CountByAccount method.
func TestTransactionCountByAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)

	// Create account
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Initially should be zero
	count, err := txnRepo.CountByAccount(account.ID)
	if err != nil {
		t.Fatalf("Failed to count transactions: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 transactions, got %d", count)
	}

	// Create some transactions
	for i := 0; i < 3; i++ {
		txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	// Should now be 3
	count, err = txnRepo.CountByAccount(account.ID)
	if err != nil {
		t.Fatalf("Failed to count transactions: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 transactions, got %d", count)
	}
}

// TestTransactionWithAllFields tests creating a transaction with all optional fields.
func TestTransactionWithAllFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)

	// Create account
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create category
	category := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := categoryRepo.Create(category); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	// Create payee
	payee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	// Create transaction with all fields
	txn := models.NewTransactionFull(
		account.ID,
		models.NewDate(2024, 1, 15),
		models.MustNewMoney("-25.50"),
		payee.ID,
		category.ID,
		"Morning coffee",
	)
	txn.SetCheckNumber("1234")
	txn.Clear()

	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Retrieve and verify all fields
	retrieved, err := txnRepo.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("Failed to get transaction: %v", err)
	}

	if !retrieved.PayeeID.Valid || retrieved.PayeeID.ID != payee.ID {
		t.Errorf("Expected payee ID %s, got %v", payee.ID.String(), retrieved.PayeeID)
	}
	if !retrieved.CategoryID.Valid || retrieved.CategoryID.ID != category.ID {
		t.Errorf("Expected category ID %s, got %v", category.ID.String(), retrieved.CategoryID)
	}
	if !retrieved.Memo.Valid || retrieved.Memo.String != "Morning coffee" {
		t.Errorf("Expected memo 'Morning coffee', got %v", retrieved.Memo)
	}
	if !retrieved.CheckNumber.Valid || retrieved.CheckNumber.String != "1234" {
		t.Errorf("Expected check number '1234', got %v", retrieved.CheckNumber)
	}
	if retrieved.Status != models.TransactionStatusCleared {
		t.Errorf("Expected status 'cleared', got %q", retrieved.Status)
	}
}

// TestAccountWithTransactionCannotBeDeleted tests that accounts with transactions cannot be deleted.
func TestAccountWithTransactionCannotBeDeleted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-transaction-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)

	// Create account
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create transaction
	txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Try to delete account
	err = accountRepo.Delete(account.ID)
	if err == nil {
		t.Error("Expected error when deleting account with transactions")
	}
	if _, ok := err.(*repository.HasDependentsError); !ok {
		t.Errorf("Expected HasDependentsError, got %T: %v", err, err)
	}
}
