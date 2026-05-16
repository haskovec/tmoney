package transaction

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Split CRUD Tests
// =============================================================================

func TestSplitRepository_Create(t *testing.T) {
	t.Run("creates valid split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create account
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create category
		cat := category.NewCategory("Groceries", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create transaction
		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split
		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-100.00"))
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
		if retrieved.CategoryID != cat.ID {
			t.Errorf("Expected category ID %v, got %v", cat.ID, retrieved.CategoryID)
		}
		if !retrieved.Amount.Equal(types.MustNewMoney("-100.00")) {
			t.Errorf("Expected amount -100.00, got %s", retrieved.Amount.String())
		}
	})

	t.Run("creates split with memo", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split with memo
		split := NewSplitWithMemo(txn.ID, cat.ID, types.MustNewMoney("-50.00"), "Lunch items")
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
		categoryRepo := category.NewRepository(database)

		// Create category
		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		fakeTransactionID := types.NewID()
		split := NewSplit(fakeTransactionID, cat.ID, types.MustNewMoney("-50.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Error("Create() expected error for non-existent transaction")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects split for non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		// Create account and transaction
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		fakeCategoryID := types.NewID()
		split := NewSplit(txn.ID, fakeCategoryID, types.MustNewMoney("-50.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Error("Create() expected error for non-existent category")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-75.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-75.00"))
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

		fakeID := types.NewID()
		_, err := splitRepo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent split")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_ListByTransaction(t *testing.T) {
	t.Run("returns empty list for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		// Create account and transaction
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := category.NewCategory("Groceries", category.TypeExpense)
		category2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := NewSplit(txn.ID, category1.ID, types.MustNewMoney("-120.00"))
		split2 := NewSplit(txn.ID, category2.ID, types.MustNewMoney("-30.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn1 := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
		txn2 := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-200.00"))
		if err := txnRepo.Create(txn1); err != nil {
			t.Fatalf("Create txn1 error = %v", err)
		}
		if err := txnRepo.Create(txn2); err != nil {
			t.Fatalf("Create txn2 error = %v", err)
		}

		// Create splits for both transactions
		split1 := NewSplit(txn1.ID, cat.ID, types.MustNewMoney("-100.00"))
		split2 := NewSplit(txn2.ID, cat.ID, types.MustNewMoney("-200.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := category.NewCategory("Original", category.TypeExpense)
		category2 := category.NewCategory("Updated", category.TypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := NewSplit(txn.ID, category1.ID, types.MustNewMoney("-100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		// Update split
		split.CategoryID = category2.ID
		split.Amount = types.MustNewMoney("-80.00")
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
		if !retrieved.Amount.Equal(types.MustNewMoney("-80.00")) {
			t.Errorf("Expected amount -80.00, got %s", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Updated memo" {
			t.Error("Expected memo to be updated")
		}
	})

	t.Run("returns NotFoundError for non-existent split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies for valid transaction and category references
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-50.00"))
		// Don't create the split, just try to update it
		err := splitRepo.Update(split)
		if err == nil {
			t.Error("Update() expected error for non-existent split")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects update with non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-50.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create split error = %v", err)
		}

		// Try to update with non-existent category
		split.CategoryID = types.NewID()
		err := splitRepo.Update(split)
		if err == nil {
			t.Error("Update() expected error for non-existent category")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_Delete(t *testing.T) {
	t.Run("deletes existing split", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-100.00"))
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

		fakeID := types.NewID()
		err := splitRepo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent split")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSplitRepository_DeleteByTransaction(t *testing.T) {
	t.Run("deletes all splits for transaction", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := category.NewCategory("Cat1", category.TypeExpense)
		category2 := category.NewCategory("Cat2", category.TypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create multiple splits
		split1 := NewSplit(txn.ID, category1.ID, types.MustNewMoney("-100.00"))
		split2 := NewSplit(txn.ID, category2.ID, types.MustNewMoney("-50.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		// Create account and transaction
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		for range 3 {
			split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-33.33"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create multiple transactions with splits in this category
		for range 2 {
			txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-50.00"))
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
			split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-50.00"))
			if err := splitRepo.Create(split); err != nil {
				t.Fatalf("Create split error = %v", err)
			}
		}

		count, err := splitRepo.CountByCategory(cat.ID)
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := category.NewCategory("Cat1", category.TypeExpense)
		category2 := category.NewCategory("Cat2", category.TypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := NewSplit(txn.ID, category1.ID, types.MustNewMoney("-120.00"))
		split2 := NewSplit(txn.ID, category2.ID, types.MustNewMoney("-30.00"))
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
		if !total.Equal(types.MustNewMoney("-150.00")) {
			t.Errorf("Expected -150.00, got %s", total.String())
		}
	})

	t.Run("returns zero for transaction without splits", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		// Create account and transaction
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category1 := category.NewCategory("Cat1", category.TypeExpense)
		category2 := category.NewCategory("Cat2", category.TypeExpense)
		if err := categoryRepo.Create(category1); err != nil {
			t.Fatalf("Create category1 error = %v", err)
		}
		if err := categoryRepo.Create(category2); err != nil {
			t.Fatalf("Create category2 error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits that sum to transaction amount
		split1 := NewSplit(txn.ID, category1.ID, types.MustNewMoney("-120.00"))
		split2 := NewSplit(txn.ID, category2.ID, types.MustNewMoney("-30.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		cat := category.NewCategory("Test", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create split that doesn't match transaction amount
		split := NewSplit(txn.ID, cat.ID, types.MustNewMoney("-100.00"))
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
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		// Create account and transaction
		now := time.Now()
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-100.00"))
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

		fakeID := types.NewID()
		_, err := splitRepo.ValidateSplitsAgainstTransaction(fakeID)
		if err == nil {
			t.Error("Expected error for non-existent transaction")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Transfer-Line Round-Trip (MS-003)
// =============================================================================

func TestSplitItemRepo_TransferLine_RoundTrip(t *testing.T) {
	database := createTestDB(t)
	splitRepo := NewSplitRepository(database)
	accountRepo := account.NewRepository(database)
	txnRepo := NewRepository(database)
	categoryRepo := category.NewRepository(database)

	now := time.Now()
	today := types.NewDate(now.Year(), now.Month(), now.Day())

	sourceAcct := account.NewAccount("Source", account.TypeChecking, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(sourceAcct); err != nil {
		t.Fatalf("Create source account: %v", err)
	}
	destAcct := account.NewAccount("Dest", account.TypeSavings, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(destAcct); err != nil {
		t.Fatalf("Create dest account: %v", err)
	}

	cat := category.NewCategory("Income", category.TypeIncome)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	// Parent net amount is 700 = 1000 (income) + -300 (transfer-line).
	parent := NewTransaction(sourceAcct.ID, today, types.MustNewMoney("700.00"))
	if err := txnRepo.Create(parent); err != nil {
		t.Fatalf("Create parent transaction: %v", err)
	}

	categorized := NewSplit(parent.ID, cat.ID, types.MustNewMoney("1000.00"))
	if err := splitRepo.Create(categorized); err != nil {
		t.Fatalf("Create categorized split: %v", err)
	}

	transferID := types.NewID()
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: destAcct.ID, Valid: true},
		TransferID:        types.NullableID{ID: transferID, Valid: true},
	}
	if err := splitRepo.Create(transferLine); err != nil {
		t.Fatalf("Create transfer-line split: %v", err)
	}

	gotCat, err := splitRepo.GetByID(categorized.ID)
	if err != nil {
		t.Fatalf("GetByID categorized: %v", err)
	}
	if gotCat.CategoryID != cat.ID {
		t.Errorf("categorized.CategoryID = %v, want %v", gotCat.CategoryID, cat.ID)
	}
	if gotCat.TransferAccountID.Valid {
		t.Errorf("categorized.TransferAccountID should be NULL, got %v", gotCat.TransferAccountID.ID)
	}
	if gotCat.TransferID.Valid {
		t.Errorf("categorized.TransferID should be NULL, got %v", gotCat.TransferID.ID)
	}

	gotXfer, err := splitRepo.GetByID(transferLine.ID)
	if err != nil {
		t.Fatalf("GetByID transfer-line: %v", err)
	}
	if !gotXfer.CategoryID.IsNil() {
		t.Errorf("transfer.CategoryID = %v, want NilID", gotXfer.CategoryID)
	}
	if !gotXfer.TransferAccountID.Valid || gotXfer.TransferAccountID.ID != destAcct.ID {
		t.Errorf("transfer.TransferAccountID = %v, want {%v,true}",
			gotXfer.TransferAccountID, destAcct.ID)
	}
	if !gotXfer.TransferID.Valid || gotXfer.TransferID.ID != transferID {
		t.Errorf("transfer.TransferID = %v, want {%v,true}", gotXfer.TransferID, transferID)
	}
	if !gotXfer.Amount.Equal(types.MustNewMoney("-300.00")) {
		t.Errorf("transfer.Amount = %s, want -300.00", gotXfer.Amount.String())
	}

	list, err := splitRepo.ListByTransaction(parent.ID)
	if err != nil {
		t.Fatalf("ListByTransaction: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByTransaction returned %d rows, want 2", len(list))
	}
	var foundCat, foundXfer bool
	for _, s := range list {
		switch s.ID {
		case categorized.ID:
			foundCat = true
			if s.CategoryID != cat.ID {
				t.Errorf("list categorized.CategoryID mismatch")
			}
			if s.TransferAccountID.Valid || s.TransferID.Valid {
				t.Errorf("list categorized transfer fields should be NULL")
			}
		case transferLine.ID:
			foundXfer = true
			if !s.CategoryID.IsNil() {
				t.Errorf("list transfer.CategoryID should be NilID, got %v", s.CategoryID)
			}
			if !s.TransferAccountID.Valid || s.TransferAccountID.ID != destAcct.ID {
				t.Errorf("list transfer.TransferAccountID mismatch")
			}
			if !s.TransferID.Valid || s.TransferID.ID != transferID {
				t.Errorf("list transfer.TransferID mismatch")
			}
		}
	}
	if !foundCat || !foundXfer {
		t.Errorf("missing rows in list: foundCategorized=%v foundTransfer=%v",
			foundCat, foundXfer)
	}
}

func TestSplitItemRepo_TransferLine_Update(t *testing.T) {
	database := createTestDB(t)
	splitRepo := NewSplitRepository(database)
	accountRepo := account.NewRepository(database)
	txnRepo := NewRepository(database)

	now := time.Now()
	today := types.NewDate(now.Year(), now.Month(), now.Day())

	sourceAcct := account.NewAccount("Source", account.TypeChecking, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(sourceAcct); err != nil {
		t.Fatalf("Create source account: %v", err)
	}
	destAcct := account.NewAccount("Dest", account.TypeSavings, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(destAcct); err != nil {
		t.Fatalf("Create dest account: %v", err)
	}
	dest2Acct := account.NewAccount("Dest2", account.TypeSavings, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(dest2Acct); err != nil {
		t.Fatalf("Create dest2 account: %v", err)
	}

	parent := NewTransaction(sourceAcct.ID, today, types.MustNewMoney("-500.00"))
	if err := txnRepo.Create(parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	transferID := types.NewID()
	split := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-500.00"),
		TransferAccountID: types.NullableID{ID: destAcct.ID, Valid: true},
		TransferID:        types.NullableID{ID: transferID, Valid: true},
	}
	if err := splitRepo.Create(split); err != nil {
		t.Fatalf("Create transfer-line: %v", err)
	}

	// Update: change target account and amount.
	split.TransferAccountID = types.NullableID{ID: dest2Acct.ID, Valid: true}
	split.Amount = types.MustNewMoney("-250.00")
	if err := splitRepo.Update(split); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := splitRepo.GetByID(split.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.TransferAccountID.ID != dest2Acct.ID {
		t.Errorf("TransferAccountID = %v, want %v",
			got.TransferAccountID.ID, dest2Acct.ID)
	}
	if got.TransferID.ID != transferID {
		t.Errorf("TransferID = %v, want %v", got.TransferID.ID, transferID)
	}
	if !got.Amount.Equal(types.MustNewMoney("-250.00")) {
		t.Errorf("Amount = %s, want -250.00", got.Amount.String())
	}
	if !got.CategoryID.IsNil() {
		t.Errorf("CategoryID should be NilID, got %v", got.CategoryID)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestSplitRepository_SplitTransactionWorkflow(t *testing.T) {
	t.Run("full split transaction lifecycle", func(t *testing.T) {
		database := createTestDB(t)
		splitRepo := NewSplitRepository(database)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)
		categoryRepo := category.NewRepository(database)

		// Create account
		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create categories
		groceries := category.NewCategory("Groceries", category.TypeExpense)
		household := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(groceries); err != nil {
			t.Fatalf("Create groceries category error = %v", err)
		}
		if err := categoryRepo.Create(household); err != nil {
			t.Fatalf("Create household category error = %v", err)
		}

		// Create transaction
		txn := NewTransaction(acct.ID, types.NewDate(now.Year(), now.Month(), now.Day()), types.MustNewMoney("-150.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		// Create splits
		split1 := NewSplitWithMemo(txn.ID, groceries.ID, types.MustNewMoney("-120.00"), "Food items")
		split2 := NewSplitWithMemo(txn.ID, household.ID, types.MustNewMoney("-30.00"), "Cleaning supplies")
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
		splits[0].Amount = types.MustNewMoney("-110.00")
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
		split3 := NewSplit(txn.ID, groceries.ID, types.MustNewMoney("-10.00"))
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
