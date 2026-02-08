package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
)

// createSplitTestService creates a test database with a TransactionService for split testing.
func createSplitTestService(t *testing.T) (*service.TransactionService, *db.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "tmoney-split-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	svc := service.NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

	cleanup := func() {
		database.Close()
		os.RemoveAll(tempDir)
	}

	return svc, database, cleanup
}

// createSplitTestData creates an account and categories for split testing.
func createSplitTestData(t *testing.T, database *db.DB) (*models.Account, *models.Category, *models.Category) {
	t.Helper()

	accountRepo := repository.NewAccountRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)

	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
		models.MustNewMoney("2000.00"), models.NewDate(2024, 1, 1))
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	food := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := categoryRepo.Create(food); err != nil {
		t.Fatalf("Failed to create food category: %v", err)
	}

	household := models.NewCategory("Household", models.CategoryTypeExpense)
	if err := categoryRepo.Create(household); err != nil {
		t.Fatalf("Failed to create household category: %v", err)
	}

	return account, food, household
}

func TestSplitCreateWithSplits(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	t.Run("creates transaction with valid splits", func(t *testing.T) {
		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("-100.00"))

		splits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-60.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-40.00")),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err != nil {
			t.Fatalf("Failed to create transaction with splits: %v", err)
		}

		// Verify transaction was created
		retrieved, err := svc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}
		if !retrieved.Amount.Equal(models.MustNewMoney("-100.00")) {
			t.Errorf("Expected amount -100.00, got %s", retrieved.Amount.String())
		}

		// Verify splits were created
		retrievedSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(retrievedSplits) != 2 {
			t.Fatalf("Expected 2 splits, got %d", len(retrievedSplits))
		}
	})

	t.Run("rejects splits that do not sum to transaction amount", func(t *testing.T) {
		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 16), models.MustNewMoney("-100.00"))

		splits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-60.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-30.00")), // Only -90, not -100
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("Expected error when splits don't sum to transaction amount")
		}
	})

	t.Run("rejects transaction with category and splits", func(t *testing.T) {
		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 17), models.MustNewMoney("-50.00"))
		txn.SetCategory(food.ID) // Category set - not allowed with splits

		splits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-30.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-20.00")),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("Expected error when transaction has both category and splits")
		}
		if _, ok := err.(*service.TransactionHasSplitsError); !ok {
			t.Errorf("Expected TransactionHasSplitsError, got %T: %v", err, err)
		}
	})
}

func TestSplitGetSplits(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	txn := models.NewTransaction(account.ID, models.NewDate(2024, 2, 1), models.MustNewMoney("-75.00"))
	splits := []*models.Split{
		models.NewSplitWithMemo(txn.ID, food.ID, models.MustNewMoney("-50.00"), "Groceries"),
		models.NewSplitWithMemo(txn.ID, household.ID, models.MustNewMoney("-25.00"), "Cleaning supplies"),
	}

	if err := svc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("Failed to create transaction with splits: %v", err)
	}

	t.Run("returns all splits for a transaction", func(t *testing.T) {
		retrievedSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(retrievedSplits) != 2 {
			t.Fatalf("Expected 2 splits, got %d", len(retrievedSplits))
		}

		// Verify splits have correct data
		for _, s := range retrievedSplits {
			if s.TransactionID != txn.ID {
				t.Errorf("Expected transaction ID %s, got %s", txn.ID.String(), s.TransactionID.String())
			}
		}
	})

	t.Run("returns empty for transaction without splits", func(t *testing.T) {
		noSplitTxn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
		txnRepo := repository.NewTransactionRepository(database)
		if err := txnRepo.Create(noSplitTxn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		splits, err := svc.GetSplits(noSplitTxn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("Expected 0 splits, got %d", len(splits))
		}
	})
}

func TestSplitAddSplit(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	// Create a transaction without splits first
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.NewDate(2024, 3, 1), models.MustNewMoney("-80.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	t.Run("adds split and validates total", func(t *testing.T) {
		split1 := models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-50.00"))
		err := svc.AddSplit(split1)
		// Should return a mismatch error since -50 != -80
		if err == nil {
			t.Error("Expected mismatch error after adding partial split")
		}
		if _, ok := err.(*service.SplitTotalMismatchError); !ok {
			t.Errorf("Expected SplitTotalMismatchError, got %T: %v", err, err)
		}

		// Add the remaining split to make totals match
		split2 := models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-30.00"))
		err = svc.AddSplit(split2)
		if err != nil {
			t.Fatalf("Failed to add second split: %v", err)
		}

		// Verify we now have 2 splits
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(splits) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(splits))
		}
	})
}

func TestSplitReplaceSplits(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	// Create a third category
	categoryRepo := repository.NewCategoryRepository(database)
	transport := models.NewCategory("Transport", models.CategoryTypeExpense)
	if err := categoryRepo.Create(transport); err != nil {
		t.Fatalf("Failed to create transport category: %v", err)
	}

	// Create transaction with initial splits
	txn := models.NewTransaction(account.ID, models.NewDate(2024, 4, 1), models.MustNewMoney("-120.00"))
	initialSplits := []*models.Split{
		models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-60.00")),
		models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-60.00")),
	}
	if err := svc.CreateWithSplits(txn, initialSplits); err != nil {
		t.Fatalf("Failed to create transaction with splits: %v", err)
	}

	t.Run("replaces all splits", func(t *testing.T) {
		newSplits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-40.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-30.00")),
			models.NewSplit(txn.ID, transport.ID, models.MustNewMoney("-50.00")),
		}

		err := svc.ReplaceSplits(txn.ID, newSplits)
		if err != nil {
			t.Fatalf("Failed to replace splits: %v", err)
		}

		// Verify new splits
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(splits) != 3 {
			t.Errorf("Expected 3 splits after replacement, got %d", len(splits))
		}
	})

	t.Run("rejects replacement splits that do not sum correctly", func(t *testing.T) {
		badSplits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-40.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-40.00")),
			// Total: -80, transaction is -120 - mismatch
		}

		err := svc.ReplaceSplits(txn.ID, badSplits)
		if err == nil {
			t.Error("Expected error when replacement splits don't sum correctly")
		}
	})
}

func TestSplitValidateSplitTotals(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	t.Run("returns true when splits match transaction amount", func(t *testing.T) {
		txn := models.NewTransaction(account.ID, models.NewDate(2024, 5, 1), models.MustNewMoney("-100.00"))
		splits := []*models.Split{
			models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-70.00")),
			models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-30.00")),
		}
		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		valid, err := svc.ValidateSplitTotals(txn.ID)
		if err != nil {
			t.Fatalf("Failed to validate: %v", err)
		}
		if !valid {
			t.Error("Expected splits to be valid")
		}
	})
}

func TestSplitDeleteSplit(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	txn := models.NewTransaction(account.ID, models.NewDate(2024, 6, 1), models.MustNewMoney("-90.00"))
	split1 := models.NewSplit(txn.ID, food.ID, models.MustNewMoney("-50.00"))
	split2 := models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(txn, []*models.Split{split1, split2}); err != nil {
		t.Fatalf("Failed to create: %v", err)
	}

	t.Run("deletes a single split", func(t *testing.T) {
		// Get the actual split IDs from the database (CreateWithSplits assigns IDs)
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}
		if len(splits) != 2 {
			t.Fatalf("Expected 2 splits, got %d", len(splits))
		}

		err = svc.DeleteSplit(splits[0].ID)
		if err != nil {
			t.Fatalf("Failed to delete split: %v", err)
		}

		remaining, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get remaining splits: %v", err)
		}
		if len(remaining) != 1 {
			t.Errorf("Expected 1 remaining split, got %d", len(remaining))
		}
	})
}

func TestSplitUpdateSplit(t *testing.T) {
	svc, database, cleanup := createSplitTestService(t)
	defer cleanup()

	account, food, household := createSplitTestData(t, database)

	txn := models.NewTransaction(account.ID, models.NewDate(2024, 7, 1), models.MustNewMoney("-100.00"))
	splits := []*models.Split{
		models.NewSplitWithMemo(txn.ID, food.ID, models.MustNewMoney("-60.00"), "Original memo"),
		models.NewSplit(txn.ID, household.ID, models.MustNewMoney("-40.00")),
	}
	if err := svc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("Failed to create: %v", err)
	}

	t.Run("updates split amount and memo", func(t *testing.T) {
		existingSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get splits: %v", err)
		}

		// Find the food split
		var foodSplit *models.Split
		for _, s := range existingSplits {
			if s.CategoryID == food.ID {
				foodSplit = s
				break
			}
		}
		if foodSplit == nil {
			t.Fatal("Could not find food split")
		}

		foodSplit.Amount = models.MustNewMoney("-65.00")
		foodSplit.SetMemo("Updated memo")

		err = svc.UpdateSplit(foodSplit)
		if err != nil {
			t.Fatalf("Failed to update split: %v", err)
		}

		// Verify the update
		updatedSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get updated splits: %v", err)
		}

		for _, s := range updatedSplits {
			if s.CategoryID == food.ID {
				if !s.Amount.Equal(models.MustNewMoney("-65.00")) {
					t.Errorf("Expected updated amount -65.00, got %s", s.Amount.String())
				}
				if !s.Memo.Valid || s.Memo.String != "Updated memo" {
					t.Errorf("Expected memo 'Updated memo', got %v", s.Memo)
				}
			}
		}
	})
}
