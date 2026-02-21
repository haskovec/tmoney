package service

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func createTestTransactionService(t *testing.T) (*TransactionService, *repository.AccountRepository) {
	t.Helper()
	database := createTestDB(t)
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	accountRepo := repository.NewAccountRepository(database)

	svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	return svc, accountRepo
}

func createTestAccount(t *testing.T, repo *repository.AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(name, models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return account
}

func TestNewTransactionService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _ := createTestTransactionService(t)
		if svc == nil {
			t.Error("NewTransactionService should not return nil")
		}
	})
}

func TestTransactionService_Create(t *testing.T) {
	t.Run("creates valid transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		err := svc.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := svc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), retrieved.Amount.String())
		}
	})

	t.Run("validates transaction before creating", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		// Zero amount is invalid
		txn := models.NewTransaction(account.ID, models.Today(), models.ZeroMoney)
		err := svc.Create(txn)
		if err == nil {
			t.Error("Create() expected error for zero amount")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("auto-populates category from payee default", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		// Create account, category, and payee with default category
		account := createTestAccount(t, accountRepo, "Checking")

		category := models.NewCategory("Groceries", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		payee := models.NewPayeeWithCategory("Kroger", category.ID)
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		// Create transaction with payee but no category
		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransactionWithPayee(account.ID, models.Today(), amount, payee.ID)

		err := svc.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify category was auto-populated
		retrieved, _ := svc.GetByID(txn.ID)
		if !retrieved.HasCategory() {
			t.Error("Expected category to be auto-populated from payee")
		}
		if retrieved.CategoryID.ID != category.ID {
			t.Errorf("Expected category %s, got %s", category.ID.String(), retrieved.CategoryID.ID.String())
		}
	})
}

func TestTransactionService_Update(t *testing.T) {
	t.Run("updates transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the amount
		newAmount, _ := models.NewMoney("-75.00")
		txn.Amount = newAmount
		if err := svc.Update(txn); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if !retrieved.Amount.Equal(newAmount) {
			t.Errorf("Expected amount %s, got %s", newAmount.String(), retrieved.Amount.String())
		}
	})

	t.Run("validates transaction before updating", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Invalid update: zero amount
		txn.Amount = models.ZeroMoney
		err := svc.Update(txn)
		if err == nil {
			t.Error("Update() expected error for zero amount")
		}
	})
}

func TestTransactionService_Delete(t *testing.T) {
	t.Run("deletes transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(txn.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(txn.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("deletes transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		category := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		split := models.NewSplit(txn.ID, category.ID, splitAmount)

		if err := svc.CreateWithSplits(txn, []*models.Split{split}); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Delete should succeed and remove splits too
		if err := svc.Delete(txn.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify splits are gone
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("Expected 0 splits after delete, got %d", len(splits))
		}
	})
}

func TestTransactionService_List(t *testing.T) {
	t.Run("lists all transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount1, _ := models.NewMoney("-50.00")
		amount2, _ := models.NewMoney("-75.00")

		if err := svc.Create(models.NewTransaction(account.ID, models.Today(), amount1)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account.ID, models.Today(), amount2)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions, got %d", len(txns))
		}
	})

	t.Run("lists transactions by account", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account1 := createTestAccount(t, accountRepo, "Checking")
		account2 := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("-50.00")

		if err := svc.Create(models.NewTransaction(account1.ID, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account2.ID, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.ListByAccount(account1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction for account1, got %d", len(txns))
		}
	})
}

func TestTransactionService_CreateWithSplits(t *testing.T) {
	t.Run("creates transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		split1Amount, _ := models.NewMoney("-70.00")
		split2Amount, _ := models.NewMoney("-30.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, split1Amount),
			models.NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Verify splits were created
		retrievedSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(retrievedSplits) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(retrievedSplits))
		}
	})

	t.Run("rejects splits that don't sum to transaction amount", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		// Splits don't sum to -100
		splitAmount, _ := models.NewMoney("-50.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat.ID, splitAmount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("CreateWithSplits() expected error for mismatched split totals")
		}
	})

	t.Run("rejects transaction with both category and splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		txn.SetCategory(cat.ID) // Has category

		splitAmount, _ := models.NewMoney("-100.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat.ID, splitAmount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("CreateWithSplits() expected error when transaction has category and splits")
		}
		if _, ok := err.(*TransactionHasSplitsError); !ok {
			t.Errorf("Expected TransactionHasSplitsError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_ValidateSplitTotals(t *testing.T) {
	t.Run("returns true when splits match", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		valid, err := svc.ValidateSplitTotals(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitTotals() error = %v", err)
		}
		if !valid {
			t.Error("ValidateSplitTotals() should return true when splits match")
		}
	})
}

func TestTransactionService_CreateTransfer(t *testing.T) {
	t.Run("creates transfer between accounts", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if pair.FromTransaction == nil || pair.ToTransaction == nil {
			t.Fatal("CreateTransfer() should return both transactions")
		}

		// Verify from transaction
		if !pair.FromTransaction.Amount.IsNegative() {
			t.Error("From transaction should have negative amount")
		}
		if pair.FromTransaction.AccountID != checking.ID {
			t.Error("From transaction should be in checking account")
		}

		// Verify to transaction
		if !pair.ToTransaction.Amount.IsPositive() {
			t.Error("To transaction should have positive amount")
		}
		if pair.ToTransaction.AccountID != savings.ID {
			t.Error("To transaction should be in savings account")
		}

		// Verify transfer link
		if !pair.FromTransaction.IsTransfer() {
			t.Error("From transaction should be marked as transfer")
		}
		if !pair.ToTransaction.IsTransfer() {
			t.Error("To transaction should be marked as transfer")
		}
	})

	t.Run("rejects negative transfer amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("-500.00")
		_, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err == nil {
			t.Error("CreateTransfer() expected error for negative amount")
		}
		if _, ok := err.(*InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T: %v", err, err)
		}
	})

	t.Run("rejects transfer to same account", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("500.00")
		_, err := svc.CreateTransfer(checking.ID, checking.ID, models.Today(), amount)
		if err == nil {
			t.Error("CreateTransfer() expected error for same account")
		}
	})
}

func TestTransactionService_DeleteTransfer(t *testing.T) {
	t.Run("deletes both sides of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Delete the transfer
		if err := svc.DeleteTransfer(pair.FromTransaction.TransferID.ID); err != nil {
			t.Fatalf("DeleteTransfer() error = %v", err)
		}

		// Both sides should be gone
		_, err = svc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("From transaction should be deleted")
		}
		_, err = svc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("To transaction should be deleted")
		}
	})

	t.Run("delete via Delete also deletes both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Delete via regular Delete (should detect transfer and delete both)
		if err := svc.Delete(pair.FromTransaction.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Both sides should be gone
		_, err = svc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("From transaction should be deleted")
		}
		_, err = svc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("To transaction should be deleted")
		}
	})
}

func TestTransactionService_UpdateTransfer(t *testing.T) {
	t.Run("updates both sides of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Update the transfer
		newAmount, _ := models.NewMoney("750.00")
		newDate, _ := models.ParseDate("2024-06-15")
		err = svc.UpdateTransfer(pair.FromTransaction.TransferID.ID, newDate, newAmount, "Savings transfer", models.TransactionStatusCleared)
		if err != nil {
			t.Fatalf("UpdateTransfer() error = %v", err)
		}

		// Verify both sides updated
		updatedPair, _ := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if !updatedPair.FromTransaction.Amount.Neg().Equal(newAmount) {
			t.Errorf("From amount should be -%s, got %s", newAmount.String(), updatedPair.FromTransaction.Amount.String())
		}
		if !updatedPair.ToTransaction.Amount.Equal(newAmount) {
			t.Errorf("To amount should be %s, got %s", newAmount.String(), updatedPair.ToTransaction.Amount.String())
		}
		if updatedPair.FromTransaction.Status != models.TransactionStatusCleared {
			t.Error("From status should be cleared")
		}
		if updatedPair.ToTransaction.Status != models.TransactionStatusCleared {
			t.Error("To status should be cleared")
		}
	})
}

func TestTransactionService_GetTransferCounterpart(t *testing.T) {
	t.Run("returns other side of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Get counterpart from from-side
		other, err := svc.GetTransferCounterpart(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetTransferCounterpart() error = %v", err)
		}
		if other.ID != pair.ToTransaction.ID {
			t.Error("GetTransferCounterpart() should return to-transaction")
		}

		// Get counterpart from to-side
		other, err = svc.GetTransferCounterpart(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetTransferCounterpart() error = %v", err)
		}
		if other.ID != pair.FromTransaction.ID {
			t.Error("GetTransferCounterpart() should return from-transaction")
		}
	})
}

func TestTransactionService_IsTransfer(t *testing.T) {
	t.Run("returns true for transfer transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		isTransfer, err := svc.IsTransfer(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if !isTransfer {
			t.Error("IsTransfer() should return true for transfer")
		}
	})

	t.Run("returns false for regular transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isTransfer, err := svc.IsTransfer(txn.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if isTransfer {
			t.Error("IsTransfer() should return false for regular transaction")
		}
	})
}

func TestTransactionService_StatusOperations(t *testing.T) {
	t.Run("ClearTransaction sets status to cleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusCleared {
			t.Errorf("Expected status cleared, got %s", retrieved.Status)
		}
	})

	t.Run("ReconcileTransaction sets status to reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusReconciled {
			t.Errorf("Expected status reconciled, got %s", retrieved.Status)
		}
	})

	t.Run("MarkTransactionUncleared sets status to uncleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Clear first
		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		// Then mark uncleared
		if err := svc.MarkTransactionUncleared(txn.ID); err != nil {
			t.Fatalf("MarkTransactionUncleared() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusUncleared {
			t.Errorf("Expected status uncleared, got %s", retrieved.Status)
		}
	})
}

func TestTransactionService_Duplicate(t *testing.T) {
	t.Run("duplicates transaction with today's date", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		oldDate, _ := models.ParseDate("2024-01-15")
		amount, _ := models.NewMoney("-50.00")
		original := models.NewTransaction(account.ID, oldDate, amount)
		original.SetMemo("Original memo")
		if err := svc.Create(original); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Clear the original
		if err := svc.ClearTransaction(original.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		duplicate, err := svc.Duplicate(original.ID)
		if err != nil {
			t.Fatalf("Duplicate() error = %v", err)
		}

		if duplicate.ID == original.ID {
			t.Error("Duplicate should have new ID")
		}
		if !duplicate.Amount.Equal(original.Amount) {
			t.Error("Duplicate should have same amount")
		}
		if duplicate.Date == oldDate {
			t.Error("Duplicate should have today's date, not original date")
		}
		if duplicate.Status != models.TransactionStatusUncleared {
			t.Error("Duplicate should have uncleared status")
		}
	})

	t.Run("duplicates transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		original := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		splits := []*models.Split{
			models.NewSplit(original.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(original, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		duplicate, err := svc.Duplicate(original.ID)
		if err != nil {
			t.Fatalf("Duplicate() error = %v", err)
		}

		// Verify splits were duplicated
		dupSplits, err := svc.GetSplits(duplicate.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(dupSplits) != 1 {
			t.Errorf("Expected 1 split in duplicate, got %d", len(dupSplits))
		}
	})

	t.Run("rejects duplicating transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		_, err = svc.Duplicate(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Duplicate() expected error for transfer")
		}
		if _, ok := err.(*CannotDuplicateTransferError); !ok {
			t.Errorf("Expected CannotDuplicateTransferError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_GetBalanceImpact(t *testing.T) {
	t.Run("returns single impact for regular transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		impacts, err := svc.GetBalanceImpact(txn.ID)
		if err != nil {
			t.Fatalf("GetBalanceImpact() error = %v", err)
		}
		if len(impacts) != 1 {
			t.Errorf("Expected 1 impact, got %d", len(impacts))
		}
		if !impacts[0].Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), impacts[0].Amount.String())
		}
	})

	t.Run("returns two impacts for transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		impacts, err := svc.GetBalanceImpact(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetBalanceImpact() error = %v", err)
		}
		if len(impacts) != 2 {
			t.Errorf("Expected 2 impacts for transfer, got %d", len(impacts))
		}

		// One should be from (negative), one should be to (positive)
		hasFrom := false
		hasTo := false
		for _, impact := range impacts {
			if impact.IsTransferFrom {
				hasFrom = true
			}
			if impact.IsTransferTo {
				hasTo = true
			}
		}
		if !hasFrom || !hasTo {
			t.Error("Transfer should have both from and to impacts")
		}
	})
}

func TestTransactionService_AddSplit_TransferError(t *testing.T) {
	t.Run("rejects adding split to transfer", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		cat := models.NewCategory("Transfer", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Try to add a split to the transfer
		splitAmount, _ := models.NewMoney("-500.00")
		split := models.NewSplit(pair.FromTransaction.ID, cat.ID, splitAmount)

		err = svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected error for transfer")
		}
		if _, ok := err.(*TransferCannotHaveSplitsError); !ok {
			t.Errorf("Expected TransferCannotHaveSplitsError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Search Tests
// =============================================================================

// createTestTransactionServiceWithCategories creates a test service with access
// to category and payee repos for search tests.
func createTestTransactionServiceWithCategories(t *testing.T) (
	*TransactionService,
	*repository.AccountRepository,
	*repository.CategoryRepository,
	*repository.PayeeRepository,
) {
	t.Helper()
	database := createTestDB(t)
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	accountRepo := repository.NewAccountRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)

	svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	return svc, accountRepo, categoryRepo, payeeRepo
}

func TestTransactionService_ListByDateRange(t *testing.T) {
	t.Run("returns transactions within date range", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		date1, _ := models.ParseDate("2024-01-15")
		date2, _ := models.ParseDate("2024-02-15")
		date3, _ := models.ParseDate("2024-03-15")

		if err := svc.Create(models.NewTransaction(account.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account.ID, date3, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := models.ParseDate("2024-01-01")
		endDate, _ := models.ParseDate("2024-02-28")

		txns, err := svc.ListByDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("ListByDateRange() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions in range, got %d", len(txns))
		}
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		date1, _ := models.ParseDate("2024-06-15")
		if err := svc.Create(models.NewTransaction(account.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := models.ParseDate("2024-01-01")
		endDate, _ := models.ParseDate("2024-02-28")

		txns, err := svc.ListByDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("ListByDateRange() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions, got %d", len(txns))
		}
	})
}

func TestTransactionService_ListByAccountAndDateRange(t *testing.T) {
	t.Run("filters by both account and date range", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account1 := createTestAccount(t, accountRepo, "Checking")
		account2 := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("-50.00")
		date1, _ := models.ParseDate("2024-01-15")
		date2, _ := models.ParseDate("2024-02-15")
		date3, _ := models.ParseDate("2024-03-15")

		// Account 1: Jan, Feb, Mar
		if err := svc.Create(models.NewTransaction(account1.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account1.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account1.ID, date3, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Account 2: Feb
		if err := svc.Create(models.NewTransaction(account2.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := models.ParseDate("2024-01-01")
		endDate, _ := models.ParseDate("2024-02-28")

		txns, err := svc.ListByAccountAndDateRange(account1.ID, startDate, endDate)
		if err != nil {
			t.Fatalf("ListByAccountAndDateRange() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions for account1 in range, got %d", len(txns))
		}

		txns2, err := svc.ListByAccountAndDateRange(account2.ID, startDate, endDate)
		if err != nil {
			t.Fatalf("ListByAccountAndDateRange() error = %v", err)
		}
		if len(txns2) != 1 {
			t.Errorf("Expected 1 transaction for account2 in range, got %d", len(txns2))
		}
	})
}

func TestTransactionService_SearchByPayee(t *testing.T) {
	t.Run("finds transactions by payee name", func(t *testing.T) {
		svc, accountRepo, _, payeeRepo := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		payee1 := models.NewPayee("Kroger Grocery")
		if err := payeeRepo.Create(payee1); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}
		payee2 := models.NewPayee("Target")
		if err := payeeRepo.Create(payee2); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := models.NewMoney("-50.00")

		txn1 := models.NewTransactionWithPayee(account.ID, models.Today(), amount, payee1.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		txn2 := models.NewTransactionWithPayee(account.ID, models.Today(), amount, payee2.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Search for "Kroger" (partial, case-insensitive)
		txns, err := svc.SearchByPayee("kroger")
		if err != nil {
			t.Fatalf("SearchByPayee() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'kroger', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching payee", func(t *testing.T) {
		svc, accountRepo, _, payeeRepo := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		payee := models.NewPayee("Kroger")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransactionWithPayee(account.ID, models.Today(), amount, payee.ID)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByPayee("walmart")
		if err != nil {
			t.Fatalf("SearchByPayee() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'walmart', got %d", len(txns))
		}
	})
}

func TestTransactionService_SearchByMemo(t *testing.T) {
	t.Run("finds transactions by memo", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")

		txn1 := models.NewTransaction(account.ID, models.Today(), amount)
		txn1.SetMemo("Grocery shopping at store")
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.Today(), amount)
		txn2.SetMemo("Gas station fill up")
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByMemo("grocery")
		if err != nil {
			t.Fatalf("SearchByMemo() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'grocery', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching memo", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		txn.SetMemo("Grocery shopping")
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByMemo("electric")
		if err != nil {
			t.Fatalf("SearchByMemo() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'electric', got %d", len(txns))
		}
	})
}

func TestTransactionService_SearchByCategory(t *testing.T) {
	t.Run("finds transactions by category name", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Groceries", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		cat2 := models.NewCategory("Entertainment", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-50.00")

		txn1 := models.NewTransaction(account.ID, models.Today(), amount)
		txn1.SetCategory(cat1.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.Today(), amount)
		txn2.SetCategory(cat2.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByCategory("groceries")
		if err != nil {
			t.Fatalf("SearchByCategory() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'groceries', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching category", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Groceries", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		txn.SetCategory(cat.ID)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByCategory("utilities")
		if err != nil {
			t.Fatalf("SearchByCategory() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'utilities', got %d", len(txns))
		}
	})
}

func TestTransactionService_Search(t *testing.T) {
	t.Run("searches with combined criteria", func(t *testing.T) {
		svc, accountRepo, categoryRepo, payeeRepo := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Groceries", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		payee := models.NewPayee("Kroger")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := models.NewMoney("-50.00")
		date1, _ := models.ParseDate("2024-01-15")
		date2, _ := models.ParseDate("2024-03-15")

		// Txn 1: Kroger, Groceries, Jan
		txn1 := models.NewTransactionWithPayee(account.ID, date1, amount, payee.ID)
		txn1.SetCategory(cat.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Txn 2: Kroger, Groceries, Mar
		txn2 := models.NewTransactionWithPayee(account.ID, date2, amount, payee.ID)
		txn2.SetCategory(cat.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Search with date range that only includes Jan
		startDate, _ := models.ParseDate("2024-01-01")
		endDate, _ := models.ParseDate("2024-02-01")
		criteria := repository.TransactionSearchCriteria{
			PayeeName: "kroger",
			StartDate: &startDate,
			EndDate:   &endDate,
		}

		txns, err := svc.Search(criteria)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching combined criteria, got %d", len(txns))
		}
	})

	t.Run("searches with memo criteria", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")

		txn1 := models.NewTransaction(account.ID, models.Today(), amount)
		txn1.SetMemo("Weekly groceries")
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.Today(), amount)
		txn2.SetMemo("Gas station")
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		criteria := repository.TransactionSearchCriteria{
			Memo: "groceries",
		}

		txns, err := svc.Search(criteria)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching memo, got %d", len(txns))
		}
	})

	t.Run("empty criteria returns all", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		if err := svc.Create(models.NewTransaction(account.ID, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewTransaction(account.ID, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.Search(repository.TransactionSearchCriteria{})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions with empty criteria, got %d", len(txns))
		}
	})
}

// =============================================================================
// Split Update Tests
// =============================================================================

func TestTransactionService_AddSplit(t *testing.T) {
	t.Run("adds split to existing transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Add a split that matches the total
		splitAmount, _ := models.NewMoney("-100.00")
		split := models.NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err != nil {
			t.Fatalf("AddSplit() error = %v", err)
		}

		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 1 {
			t.Errorf("Expected 1 split, got %d", len(splits))
		}
	})

	t.Run("returns mismatch error when split total does not match", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Add a split with partial amount (doesn't match total)
		splitAmount, _ := models.NewMoney("-30.00")
		split := models.NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected mismatch error")
		}
		if _, ok := err.(*SplitTotalMismatchError); !ok {
			t.Errorf("Expected SplitTotalMismatchError, got %T: %v", err, err)
		}

		// Split should still be created despite mismatch
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 1 {
			t.Errorf("Expected 1 split (still created), got %d", len(splits))
		}
	})
}

func TestTransactionService_UpdateSplit(t *testing.T) {
	t.Run("updates split amount", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		split1Amount, _ := models.NewMoney("-60.00")
		split2Amount, _ := models.NewMoney("-40.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, split1Amount),
			models.NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Update first split
		retrievedSplits, _ := svc.GetSplits(txn.ID)
		newAmount, _ := models.NewMoney("-80.00")
		retrievedSplits[0].Amount = newAmount

		err := svc.UpdateSplit(retrievedSplits[0])
		if err != nil {
			t.Fatalf("UpdateSplit() error = %v", err)
		}

		// Verify the update
		updatedSplits, _ := svc.GetSplits(txn.ID)
		found := false
		for _, s := range updatedSplits {
			if s.ID == retrievedSplits[0].ID {
				if !s.Amount.Equal(newAmount) {
					t.Errorf("Expected updated amount %s, got %s", newAmount.String(), s.Amount.String())
				}
				found = true
			}
		}
		if !found {
			t.Error("Updated split not found")
		}
	})

	t.Run("validates split before updating", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		// Set invalid zero amount
		retrievedSplits[0].Amount = models.ZeroMoney

		err := svc.UpdateSplit(retrievedSplits[0])
		if err == nil {
			t.Error("UpdateSplit() expected error for zero amount")
		}
	})
}

func TestTransactionService_DeleteSplit(t *testing.T) {
	t.Run("deletes a split", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		split1Amount, _ := models.NewMoney("-60.00")
		split2Amount, _ := models.NewMoney("-40.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, split1Amount),
			models.NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) != 2 {
			t.Fatalf("Expected 2 splits, got %d", len(retrievedSplits))
		}

		// Delete the first split
		if err := svc.DeleteSplit(retrievedSplits[0].ID); err != nil {
			t.Fatalf("DeleteSplit() error = %v", err)
		}

		remaining, _ := svc.GetSplits(txn.ID)
		if len(remaining) != 1 {
			t.Errorf("Expected 1 remaining split, got %d", len(remaining))
		}
	})
}

func TestTransactionService_ReplaceSplits(t *testing.T) {
	t.Run("replaces all splits", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		cat3 := models.NewCategory("Entertainment", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat3); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		origSplits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Replace with two new splits
		newSplit1, _ := models.NewMoney("-70.00")
		newSplit2, _ := models.NewMoney("-30.00")
		newSplits := []*models.Split{
			models.NewSplit(txn.ID, cat2.ID, newSplit1),
			models.NewSplit(txn.ID, cat3.ID, newSplit2),
		}

		if err := svc.ReplaceSplits(txn.ID, newSplits); err != nil {
			t.Fatalf("ReplaceSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) != 2 {
			t.Errorf("Expected 2 splits after replace, got %d", len(retrievedSplits))
		}
	})

	t.Run("rejects replacement splits that do not sum to transaction amount", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		splitAmount, _ := models.NewMoney("-100.00")
		origSplits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Try to replace with splits that don't sum correctly
		badAmount, _ := models.NewMoney("-50.00")
		badSplits := []*models.Split{
			models.NewSplit(txn.ID, cat2.ID, badAmount),
		}

		err := svc.ReplaceSplits(txn.ID, badSplits)
		if err == nil {
			t.Error("ReplaceSplits() expected error for mismatched totals")
		}
	})
}

// =============================================================================
// Transfer Update Tests
// =============================================================================

func TestTransactionService_UpdateTransferAmount(t *testing.T) {
	t.Run("updates amount on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		newAmount, _ := models.NewMoney("750.00")
		if err := svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, newAmount); err != nil {
			t.Fatalf("UpdateTransferAmount() error = %v", err)
		}

		// Verify both sides updated
		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if !updatedPair.FromTransaction.Amount.Neg().Equal(newAmount) {
			t.Errorf("From amount should be -%s, got %s", newAmount.String(), updatedPair.FromTransaction.Amount.String())
		}
		if !updatedPair.ToTransaction.Amount.Equal(newAmount) {
			t.Errorf("To amount should be %s, got %s", newAmount.String(), updatedPair.ToTransaction.Amount.String())
		}
	})

	t.Run("rejects negative amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		negAmount, _ := models.NewMoney("-100.00")
		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, negAmount)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for negative amount")
		}
		if _, ok := err.(*InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T: %v", err, err)
		}
	})

	t.Run("rejects zero amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, models.ZeroMoney)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for zero amount")
		}
	})
}

func TestTransactionService_UpdateTransferDate(t *testing.T) {
	t.Run("updates date on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		newDate, _ := models.ParseDate("2024-06-15")
		if err := svc.UpdateTransferDate(pair.FromTransaction.TransferID.ID, newDate); err != nil {
			t.Fatalf("UpdateTransferDate() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Date != newDate {
			t.Errorf("From date should be %s, got %s", newDate, updatedPair.FromTransaction.Date)
		}
		if updatedPair.ToTransaction.Date != newDate {
			t.Errorf("To date should be %s, got %s", newDate, updatedPair.ToTransaction.Date)
		}
	})
}

func TestTransactionService_UpdateTransferStatus(t *testing.T) {
	t.Run("updates status on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusCleared); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Status != models.TransactionStatusCleared {
			t.Errorf("From status should be cleared, got %s", updatedPair.FromTransaction.Status)
		}
		if updatedPair.ToTransaction.Status != models.TransactionStatusCleared {
			t.Errorf("To status should be cleared, got %s", updatedPair.ToTransaction.Status)
		}
	})

	t.Run("updates to reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Status != models.TransactionStatusReconciled {
			t.Errorf("From status should be reconciled, got %s", updatedPair.FromTransaction.Status)
		}
		if updatedPair.ToTransaction.Status != models.TransactionStatusReconciled {
			t.Errorf("To status should be reconciled, got %s", updatedPair.ToTransaction.Status)
		}
	})
}

// =============================================================================
// Void Transaction Tests
// =============================================================================

func TestTransactionService_VoidTransaction(t *testing.T) {
	t.Run("voids a regular transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		txn.SetMemo("Original memo")
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		retrieved, err := svc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.Status != models.TransactionStatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
		if !retrieved.Amount.IsZero() {
			t.Errorf("Expected amount 0, got %s", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "**VOID**" {
			t.Errorf("Expected memo '**VOID**', got %q", retrieved.Memo.String)
		}
	})

	t.Run("voids a cleared transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-75.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
	})

	t.Run("rejects voiding a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Voiding again should fail
		err := svc.VoidTransaction(txn.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for already void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects voiding a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.VoidTransaction(txn.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("voids a split transaction and removes splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)

		split1Amount, _ := models.NewMoney("-60.00")
		split2Amount, _ := models.NewMoney("-40.00")
		splits := []*models.Split{
			models.NewSplit(txn.ID, cat1.ID, split1Amount),
			models.NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Void the split transaction
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Verify transaction is voided
		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
		if !retrieved.Amount.IsZero() {
			t.Errorf("Expected amount 0, got %s", retrieved.Amount.String())
		}

		// Verify splits are removed
		remainingSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(remainingSplits) != 0 {
			t.Errorf("Expected 0 splits after void, got %d", len(remainingSplits))
		}
	})

	t.Run("voids a transfer (both sides)", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Void via the from-side
		if err := svc.VoidTransaction(pair.FromTransaction.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Both sides should be void
		fromTxn, err := svc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if fromTxn.Status != models.TransactionStatusVoid {
			t.Errorf("From transaction should be void, got %s", fromTxn.Status)
		}
		if !fromTxn.Amount.IsZero() {
			t.Errorf("From amount should be 0, got %s", fromTxn.Amount.String())
		}
		if !fromTxn.Memo.Valid || fromTxn.Memo.String != "**VOID**" {
			t.Errorf("From memo should be '**VOID**', got %q", fromTxn.Memo.String)
		}

		toTxn, err := svc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if toTxn.Status != models.TransactionStatusVoid {
			t.Errorf("To transaction should be void, got %s", toTxn.Status)
		}
		if !toTxn.Amount.IsZero() {
			t.Errorf("To amount should be 0, got %s", toTxn.Amount.String())
		}
		if !toTxn.Memo.Valid || toTxn.Memo.String != "**VOID**" {
			t.Errorf("To memo should be '**VOID**', got %q", toTxn.Memo.String)
		}
	})

	t.Run("voids transfer via to-side", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("300.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Void via the to-side
		if err := svc.VoidTransaction(pair.ToTransaction.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Both sides should be void
		fromTxn, _ := svc.GetByID(pair.FromTransaction.ID)
		if fromTxn.Status != models.TransactionStatusVoid {
			t.Errorf("From transaction should be void, got %s", fromTxn.Status)
		}

		toTxn, _ := svc.GetByID(pair.ToTransaction.ID)
		if toTxn.Status != models.TransactionStatusVoid {
			t.Errorf("To transaction should be void, got %s", toTxn.Status)
		}
	})

	t.Run("rejects voiding transfer when one side is reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Reconcile both sides (transfer status updates both)
		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		// Voiding should fail
		err = svc.VoidTransaction(pair.FromTransaction.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_Update_VoidGuard(t *testing.T) {
	t.Run("rejects editing a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Try to update the void transaction
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := models.NewMoney("-100.00")
		retrieved.Amount = newAmount
		retrieved.Status = models.TransactionStatusUncleared

		err := svc.Update(retrieved)
		if err == nil {
			t.Error("Update() expected error for void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects editing a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Try to update the reconciled transaction
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := models.NewMoney("-100.00")
		retrieved.Amount = newAmount

		err := svc.Update(retrieved)
		if err == nil {
			t.Error("Update() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Reconciled Transaction Locking Tests
// =============================================================================

func TestTransactionService_Delete_ReconciledGuard(t *testing.T) {
	t.Run("rejects deleting a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.Delete(txn.ID)
		if err == nil {
			t.Error("Delete() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects deleting a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.Delete(txn.ID)
		if err == nil {
			t.Error("Delete() expected error for void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects deleting a reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		err = svc.Delete(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Delete() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_UnReconcileTransaction(t *testing.T) {
	t.Run("un-reconciles a reconciled transaction to cleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		if err := svc.UnReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("UnReconcileTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusCleared {
			t.Errorf("Expected status cleared after un-reconcile, got %s", retrieved.Status)
		}
	})

	t.Run("rejects un-reconciling a non-reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for uncleared transaction")
		}
		if _, ok := err.(*TransactionNotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects un-reconciling a cleared transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for cleared transaction")
		}
		if _, ok := err.(*TransactionNotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects un-reconciling a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for void transaction")
		}
		if _, ok := err.(*TransactionNotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("allows editing after un-reconcile", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Reconcile then un-reconcile
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}
		if err := svc.UnReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("UnReconcileTransaction() error = %v", err)
		}

		// Should now be editable
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := models.NewMoney("-100.00")
		retrieved.Amount = newAmount
		if err := svc.Update(retrieved); err != nil {
			t.Fatalf("Update() should succeed after un-reconcile, got error: %v", err)
		}

		final, _ := svc.GetByID(txn.ID)
		if !final.Amount.Equal(newAmount) {
			t.Errorf("Expected amount %s after edit, got %s", newAmount.String(), final.Amount.String())
		}
	})
}

func TestTransactionService_StatusOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects ClearTransaction on reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.ClearTransaction(txn.ID)
		if err == nil {
			t.Error("ClearTransaction() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects MarkTransactionUncleared on reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.MarkTransactionUncleared(txn.ID)
		if err == nil {
			t.Error("MarkTransactionUncleared() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ClearTransaction on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.ClearTransaction(txn.ID)
		if err == nil {
			t.Error("ClearTransaction() expected error for void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects MarkTransactionUncleared on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.MarkTransactionUncleared(txn.ID)
		if err == nil {
			t.Error("MarkTransactionUncleared() expected error for void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ReconcileTransaction on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.ReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("ReconcileTransaction() expected error for void transaction")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_SplitOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects AddSplit on reconciled transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		splitAmount, _ := models.NewMoney("-100.00")
		split := models.NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateSplit on reconciled transaction", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it first (no splits yet)
		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Insert a split directly via SQL to create a reconciled transaction with splits
		splitID := models.NewID()
		if _, err := database.Conn().Exec(
			`INSERT INTO transaction_splits (id, transaction_id, category_id, amount, created_at)
			 VALUES (?, ?, ?, -100.0000, CURRENT_TIMESTAMP)`,
			splitID.String(), txn.ID.String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("Failed to insert split: %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) == 0 {
			t.Fatal("Expected splits to exist")
		}
		newAmount, _ := models.NewMoney("-80.00")
		retrievedSplits[0].Amount = newAmount

		err := svc.UpdateSplit(retrievedSplits[0])
		if err == nil {
			t.Error("UpdateSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects DeleteSplit on reconciled transaction", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := repository.NewTransactionRepository(database)
		splitRepo := repository.NewSplitRepository(database)
		transferRepo := repository.NewTransferRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		categoryRepo := repository.NewCategoryRepository(database)

		svc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		account := createTestAccount(t, accountRepo, "Checking")

		cat := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it first (no splits yet)
		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Insert a split directly via SQL
		splitID := models.NewID()
		if _, err := database.Conn().Exec(
			`INSERT INTO transaction_splits (id, transaction_id, category_id, amount, created_at)
			 VALUES (?, ?, ?, -100.0000, CURRENT_TIMESTAMP)`,
			splitID.String(), txn.ID.String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("Failed to insert split: %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) == 0 {
			t.Fatal("Expected splits to exist")
		}

		err := svc.DeleteSplit(retrievedSplits[0].ID)
		if err == nil {
			t.Error("DeleteSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ReplaceSplits on reconciled transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		account := createTestAccount(t, accountRepo, "Checking")

		cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
		cat2 := models.NewCategory("Household", models.CategoryTypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it (no splits needed for ReplaceSplits guard test)
		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(account.ID, models.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		splitAmount, _ := models.NewMoney("-100.00")
		newSplits := []*models.Split{
			models.NewSplit(txn.ID, cat2.ID, splitAmount),
		}

		err := svc.ReplaceSplits(txn.ID, newSplits)
		if err == nil {
			t.Error("ReplaceSplits() expected error for reconciled transaction")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_TransferOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects UpdateTransfer on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newAmount, _ := models.NewMoney("750.00")
		newDate, _ := models.ParseDate("2024-06-15")
		err = svc.UpdateTransfer(pair.FromTransaction.TransferID.ID, newDate, newAmount, "Updated", models.TransactionStatusCleared)
		if err == nil {
			t.Error("UpdateTransfer() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateTransferAmount on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newAmount, _ := models.NewMoney("750.00")
		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, newAmount)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateTransferDate on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newDate, _ := models.ParseDate("2024-06-15")
		err = svc.UpdateTransferDate(pair.FromTransaction.TransferID.ID, newDate)
		if err == nil {
			t.Error("UpdateTransferDate() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects DeleteTransfer on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		err = svc.DeleteTransfer(pair.FromTransaction.TransferID.ID)
		if err == nil {
			t.Error("DeleteTransfer() expected error for reconciled transfer")
		}
		if _, ok := err.(*TransactionIsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("allows UpdateTransferStatus on non-reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, models.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// UpdateTransferStatus should still work (it's how reconciliation happens)
		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, _ := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if updatedPair.FromTransaction.Status != models.TransactionStatusReconciled {
			t.Errorf("Expected reconciled status, got %s", updatedPair.FromTransaction.Status)
		}
	})
}
