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

	t.Run("MarkTransactionPending sets status to pending", func(t *testing.T) {
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

		// Then mark pending
		if err := svc.MarkTransactionPending(txn.ID); err != nil {
			t.Fatalf("MarkTransactionPending() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != models.TransactionStatusPending {
			t.Errorf("Expected status pending, got %s", retrieved.Status)
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
		if duplicate.Status != models.TransactionStatusPending {
			t.Error("Duplicate should have pending status")
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
