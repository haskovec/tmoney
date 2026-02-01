package repository

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// Split CRUD Tests
// =============================================================================

func TestSplitRepository_Create(t *testing.T) {
	t.Run("creates valid split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create account
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create category
		category := models.NewCategory("Groceries", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create transaction
		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split
		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-100.00"))
		err := splitRepo.Create(split)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.TransactionID != txn.ID {
			t.Errorf("Expected transaction ID %v, got %v", txn.ID, retrieved.TransactionID)
		}
		if retrieved.CategoryID != category.ID {
			t.Errorf("Expected category ID %v, got %v", category.ID, retrieved.CategoryID)
		}
		if !retrieved.Amount.Equal(models.MustNewMoney("-100.00")) {
			t.Errorf("Expected amount -100.00, got %s", retrieved.Amount.String())
		}
	})

	t.Run("creates split with memo", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split with memo
		split := models.NewSplitWithMemo(txn.ID, category.ID, models.MustNewMoney("-50.00"), "Lunch items")
		err := splitRepo.Create(split)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Lunch items" {
			t.Error("Expected memo to be set")
		}
	})

	t.Run("rejects split for non-existent transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create category
		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		fakeTransactionID := models.NewID()
		split := models.NewSplit(fakeTransactionID, category.ID, models.MustNewMoney("-50.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Error("Create() expected error for non-existent transaction")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects split for non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and transaction
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		fakeCategoryID := models.NewID()
		split := models.NewSplit(txn.ID, fakeCategoryID, models.MustNewMoney("-50.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Error("Create() expected error for non-existent category")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-75.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-75.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		retrieved, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != split.ID {
			t.Errorf("Expected ID %v, got %v", split.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)

		fakeID := models.NewID()
		_, err := splitRepo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent split")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_ListByTransaction(t *testing.T) {
	t.Run("returns empty list for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and transaction
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		splits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction() error = %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("Expected 0 splits, got %d", len(splits))
		}
	})

	t.Run("returns all splits for transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := models.NewCategory("Groceries", models.CategoryTypeExpense)
		category2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := models.NewSplit(txn.ID, category1.ID, models.MustNewMoney("-120.00"))
		split2 := models.NewSplit(txn.ID, category2.ID, models.MustNewMoney("-30.00"))
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		splits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction() error = %v", err)
		}
		if len(splits) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(splits))
		}
	})

	t.Run("does not return splits from other transactions", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn1 := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		txn2 := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-200.00"))
		if err := txnRepo.Create(txn1); err != nil {
			t.Fatalf("Create txn1 error = %v", err)
		}
		if err := txnRepo.Create(txn2); err != nil {
			t.Fatalf("Create txn2 error = %v", err)
		}

		// Create splits for both transactions
		split1 := models.NewSplit(txn1.ID, category.ID, models.MustNewMoney("-100.00"))
		split2 := models.NewSplit(txn2.ID, category.ID, models.MustNewMoney("-200.00"))
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		// List splits for txn1 only
		splits, err := splitRepo.ListByTransaction(txn1.ID)
		if err != nil {
			t.Fatalf("ListByTransaction() error = %v", err)
		}
		if len(splits) != 1 {
			t.Errorf("Expected 1 split, got %d", len(splits))
		}
		if splits[0].TransactionID != txn1.ID {
			t.Errorf("Expected transaction ID %v, got %v", txn1.ID, splits[0].TransactionID)
		}
	})
}

func TestSplitRepository_Update(t *testing.T) {
	t.Run("updates existing split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := models.NewCategory("Original", models.CategoryTypeExpense)
		category2 := models.NewCategory("Updated", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := models.NewSplit(txn.ID, category1.ID, models.MustNewMoney("-100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		// Update split
		split.CategoryID = category2.ID
		split.Amount = models.MustNewMoney("-80.00")
		split.SetMemo("Updated memo")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.CategoryID != category2.ID {
			t.Errorf("Expected category ID %v, got %v", category2.ID, retrieved.CategoryID)
		}
		if !retrieved.Amount.Equal(models.MustNewMoney("-80.00")) {
			t.Errorf("Expected amount -80.00, got %s", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Updated memo" {
			t.Error("Expected memo to be updated")
		}
	})

	t.Run("returns NotFoundError for non-existent split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies for valid transaction and category references
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-50.00"))
		// Don't create the split, just try to update it
		err := splitRepo.Update(split)
		if err == nil {
			t.Error("Update() expected error for non-existent split")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects update with non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-50.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		// Try to update with non-existent category
		split.CategoryID = models.NewID()
		err := splitRepo.Update(split)
		if err == nil {
			t.Error("Update() expected error for non-existent category")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_Delete(t *testing.T) {
	t.Run("deletes existing split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		if err := splitRepo.Delete(split.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := splitRepo.GetByID(split.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("returns NotFoundError for non-existent split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)

		fakeID := models.NewID()
		err := splitRepo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent split")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_DeleteByTransaction(t *testing.T) {
	t.Run("deletes all splits for transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := models.NewCategory("Cat1", models.CategoryTypeExpense)
		category2 := models.NewCategory("Cat2", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create multiple splits
		split1 := models.NewSplit(txn.ID, category1.ID, models.MustNewMoney("-100.00"))
		split2 := models.NewSplit(txn.ID, category2.ID, models.MustNewMoney("-50.00"))
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		count, err := splitRepo.DeleteByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("DeleteByTransaction() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 deleted, got %d", count)
		}

		// Verify splits are deleted
		splits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction() error = %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("Expected 0 splits after delete, got %d", len(splits))
		}
	})

	t.Run("returns zero for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and transaction
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		count, err := splitRepo.DeleteByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("DeleteByTransaction() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 deleted, got %d", count)
		}
	})
}

// =============================================================================
// Count and Total Tests
// =============================================================================

func TestSplitRepository_CountByTransaction(t *testing.T) {
	t.Run("counts splits for transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		for i := 0; i < 3; i++ {
			split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-33.33"))
			if err := splitRepo.Create(split); err != nil {
				t.Fatalf("Create split error = %v", err)
			}
		}

		count, err := splitRepo.CountByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("CountByTransaction() error = %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3, got %d", count)
		}
	})
}

func TestSplitRepository_CountByCategory(t *testing.T) {
	t.Run("counts splits for category", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create multiple transactions with splits in this category
		for i := 0; i < 2; i++ {
			txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
			split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-50.00"))
			if err := splitRepo.Create(split); err != nil {
				t.Fatalf("Create split error = %v", err)
			}
		}

		count, err := splitRepo.CountByCategory(category.ID)
		if err != nil {
			t.Fatalf("CountByCategory() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2, got %d", count)
		}
	})
}

func TestSplitRepository_GetTotalByTransaction(t *testing.T) {
	t.Run("returns total of all splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := models.NewCategory("Cat1", models.CategoryTypeExpense)
		category2 := models.NewCategory("Cat2", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := models.NewSplit(txn.ID, category1.ID, models.MustNewMoney("-120.00"))
		split2 := models.NewSplit(txn.ID, category2.ID, models.MustNewMoney("-30.00"))
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		total, err := splitRepo.GetTotalByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("GetTotalByTransaction() error = %v", err)
		}
		if !total.Equal(models.MustNewMoney("-150.00")) {
			t.Errorf("Expected -150.00, got %s", total.String())
		}
	})

	t.Run("returns zero for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and transaction
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		total, err := splitRepo.GetTotalByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("GetTotalByTransaction() error = %v", err)
		}
		if !total.IsZero() {
			t.Errorf("Expected 0, got %s", total.String())
		}
	})
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestSplitRepository_ValidateSplitsAgainstTransaction(t *testing.T) {
	t.Run("returns true when splits match transaction amount", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := models.NewCategory("Cat1", models.CategoryTypeExpense)
		category2 := models.NewCategory("Cat2", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits that sum to transaction amount
		split1 := models.NewSplit(txn.ID, category1.ID, models.MustNewMoney("-120.00"))
		split2 := models.NewSplit(txn.ID, category2.ID, models.MustNewMoney("-30.00"))
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		valid, err := splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if !valid {
			t.Error("Expected validation to pass")
		}
	})

	t.Run("returns false when splits don't match transaction amount", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Test", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split that doesn't match transaction amount
		split := models.NewSplit(txn.ID, category.ID, models.MustNewMoney("-100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		valid, err := splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if valid {
			t.Error("Expected validation to fail")
		}
	})

	t.Run("returns false for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and transaction
		now := time.Now()
		account := models.NewAccount("Test Account", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		valid, err := splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if valid {
			t.Error("Expected validation to fail for transaction without splits")
		}
	})

	t.Run("returns error for non-existent transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)

		fakeID := models.NewID()
		_, err := splitRepo.ValidateSplitsAgainstTransaction(fakeID)
		if err == nil {
			t.Error("Expected error for non-existent transaction")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestSplitRepository_SplitTransactionWorkflow(t *testing.T) {
	t.Run("full split transaction lifecycle", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create account
		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create categories
		groceries := models.NewCategory("Groceries", models.CategoryTypeExpense)
		household := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(groceries); err != nil {
			t.Fatalf("Create groceries category error = %v", err)
		}
		if err := categoryRepo.Create(household); err != nil {
			t.Fatalf("Create household category error = %v", err)
		}

		// Create transaction
		txn := models.NewTransaction(account.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := models.NewSplitWithMemo(txn.ID, groceries.ID, models.MustNewMoney("-120.00"), "Food items")
		split2 := models.NewSplitWithMemo(txn.ID, household.ID, models.MustNewMoney("-30.00"), "Cleaning supplies")
		if err := splitRepo.Create(split1); err != nil {
			t.Fatalf("Create split1 error = %v", err)
		}
		if err := splitRepo.Create(split2); err != nil {
			t.Fatalf("Create split2 error = %v", err)
		}

		// Validate splits match transaction
		valid, err := splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if !valid {
			t.Error("Expected splits to match transaction amount")
		}

		// List splits
		splits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction() error = %v", err)
		}
		if len(splits) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(splits))
		}

		// Update a split
		splits[0].Amount = models.MustNewMoney("-110.00")
		if err := splitRepo.Update(splits[0]); err != nil {
			t.Fatalf("Update split error = %v", err)
		}

		// Now validation should fail
		valid, err = splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if valid {
			t.Error("Expected validation to fail after updating split amount")
		}

		// Fix by adding another split
		split3 := models.NewSplit(txn.ID, groceries.ID, models.MustNewMoney("-10.00"))
		if err := splitRepo.Create(split3); err != nil {
			t.Fatalf("Create split3 error = %v", err)
		}

		// Validation should pass again
		valid, err = splitRepo.ValidateSplitsAgainstTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitsAgainstTransaction() error = %v", err)
		}
		if !valid {
			t.Error("Expected validation to pass after adding corrective split")
		}

		// Delete all splits
		deleted, err := splitRepo.DeleteByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("DeleteByTransaction() error = %v", err)
		}
		if deleted != 3 {
			t.Errorf("Expected 3 deleted, got %d", deleted)
		}

		// Verify splits are gone
		count, err := splitRepo.CountByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("CountByTransaction() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 splits after delete, got %d", count)
		}
	})
}
