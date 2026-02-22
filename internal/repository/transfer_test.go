package repository

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// Transfer Create Tests
// =============================================================================

func TestTransferRepository_Create(t *testing.T) {
	t.Run("creates valid transfer pair", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create two accounts
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		// Create transfer pair
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		err := transferRepo.Create(pair)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify both transactions were created
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		// From transaction should have negative amount
		if !retrieved.FromTransaction.Amount.Equal(models.MustNewMoney("-500.00")) {
			t.Errorf("Expected from amount -500.00, got %s", retrieved.FromTransaction.Amount.String())
		}

		// To transaction should have positive amount
		if !retrieved.ToTransaction.Amount.Equal(models.MustNewMoney("500.00")) {
			t.Errorf("Expected to amount 500.00, got %s", retrieved.ToTransaction.Amount.String())
		}

		// Verify account IDs
		if retrieved.FromTransaction.AccountID != checking.ID {
			t.Errorf("Expected from account %v, got %v", checking.ID, retrieved.FromTransaction.AccountID)
		}
		if retrieved.ToTransaction.AccountID != savings.ID {
			t.Errorf("Expected to account %v, got %v", savings.ID, retrieved.ToTransaction.AccountID)
		}

		// Verify transfer links
		if retrieved.FromTransaction.TransferAccountID.ID != savings.ID {
			t.Errorf("Expected from transfer_account_id %v, got %v", savings.ID, retrieved.FromTransaction.TransferAccountID.ID)
		}
		if retrieved.ToTransaction.TransferAccountID.ID != checking.ID {
			t.Errorf("Expected to transfer_account_id %v, got %v", checking.ID, retrieved.ToTransaction.TransferAccountID.ID)
		}
	})

	t.Run("rejects transfer to same account", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create one account
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		// Try to create transfer to same account
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, checking.ID, date, amount)

		err := transferRepo.Create(pair)
		if err == nil {
			t.Error("Create() expected error for transfer to same account")
		}
	})

	t.Run("rejects transfer with non-existent from account", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create only one account
		now := time.Now()
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		// Try to create transfer from non-existent account
		fakeAccountID := models.NewID()
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(fakeAccountID, savings.ID, date, amount)

		err := transferRepo.Create(pair)
		if err == nil {
			t.Error("Create() expected error for non-existent from account")
		}
	})

	t.Run("rejects transfer with non-existent to account", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create only one account
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		// Try to create transfer to non-existent account
		fakeAccountID := models.NewID()
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, fakeAccountID, date, amount)

		err := transferRepo.Create(pair)
		if err == nil {
			t.Error("Create() expected error for non-existent to account")
		}
	})
}

// =============================================================================
// Transfer Get Tests
// =============================================================================

func TestTransferRepository_GetByTransferID(t *testing.T) {
	t.Run("retrieves existing transfer pair", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("250.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if retrieved.FromTransaction.TransferID != retrieved.ToTransaction.TransferID {
			t.Error("Transfer IDs don't match between pair transactions")
		}
	})

	t.Run("returns NotFoundError for non-existent transfer", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)

		fakeID := models.NewID()
		_, err := transferRepo.GetByTransferID(fakeID)
		if err == nil {
			t.Error("GetByTransferID() expected error for non-existent transfer")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestTransferRepository_GetOtherSide(t *testing.T) {
	t.Run("retrieves other side of transfer", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("300.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Get other side from "from" transaction
		other, err := transferRepo.GetOtherSide(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetOtherSide() error = %v", err)
		}

		if other.ID != pair.ToTransaction.ID {
			t.Errorf("Expected to transaction ID %v, got %v", pair.ToTransaction.ID, other.ID)
		}

		// Get other side from "to" transaction
		other, err = transferRepo.GetOtherSide(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetOtherSide() error = %v", err)
		}

		if other.ID != pair.FromTransaction.ID {
			t.Errorf("Expected from transaction ID %v, got %v", pair.FromTransaction.ID, other.ID)
		}
	})

	t.Run("returns error for non-transfer transaction", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and regular transaction
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		txn := models.NewTransaction(checking.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		_, err := transferRepo.GetOtherSide(txn.ID)
		if err == nil {
			t.Error("GetOtherSide() expected error for non-transfer transaction")
		}
	})
}

// =============================================================================
// Transfer Update Tests
// =============================================================================

func TestTransferRepository_Update(t *testing.T) {
	t.Run("updates both sides of transfer", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the pair
		pair.FromTransaction.SetMemo("Updated memo")
		pair.ToTransaction.SetMemo("Updated memo")
		pair.FromTransaction.Amount = models.MustNewMoney("-750.00")
		pair.ToTransaction.Amount = models.MustNewMoney("750.00")

		if err := transferRepo.Update(pair); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		// Verify updates
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if !retrieved.FromTransaction.Amount.Equal(models.MustNewMoney("-750.00")) {
			t.Errorf("Expected from amount -750.00, got %s", retrieved.FromTransaction.Amount.String())
		}
		if !retrieved.ToTransaction.Amount.Equal(models.MustNewMoney("750.00")) {
			t.Errorf("Expected to amount 750.00, got %s", retrieved.ToTransaction.Amount.String())
		}
		if !retrieved.FromTransaction.Memo.Valid || retrieved.FromTransaction.Memo.String != "Updated memo" {
			t.Error("Expected memo to be updated on from transaction")
		}
		if !retrieved.ToTransaction.Memo.Valid || retrieved.ToTransaction.Memo.String != "Updated memo" {
			t.Error("Expected memo to be updated on to transaction")
		}
	})
}

func TestTransferRepository_UpdateAmount(t *testing.T) {
	t.Run("updates amount on both sides", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update amount
		newAmount := models.MustNewMoney("1000.00")
		if err := transferRepo.UpdateAmount(pair.FromTransaction.TransferID.ID, newAmount); err != nil {
			t.Fatalf("UpdateAmount() error = %v", err)
		}

		// Verify updates
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if !retrieved.FromTransaction.Amount.Equal(models.MustNewMoney("-1000.00")) {
			t.Errorf("Expected from amount -1000.00, got %s", retrieved.FromTransaction.Amount.String())
		}
		if !retrieved.ToTransaction.Amount.Equal(models.MustNewMoney("1000.00")) {
			t.Errorf("Expected to amount 1000.00, got %s", retrieved.ToTransaction.Amount.String())
		}
	})
}

func TestTransferRepository_UpdateDate(t *testing.T) {
	t.Run("updates date on both sides", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update date
		newDate := models.NewDate(2024, 6, 15)
		if err := transferRepo.UpdateDate(pair.FromTransaction.TransferID.ID, newDate); err != nil {
			t.Fatalf("UpdateDate() error = %v", err)
		}

		// Verify updates
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if retrieved.FromTransaction.Date != newDate {
			t.Errorf("Expected from date %v, got %v", newDate, retrieved.FromTransaction.Date)
		}
		if retrieved.ToTransaction.Date != newDate {
			t.Errorf("Expected to date %v, got %v", newDate, retrieved.ToTransaction.Date)
		}
	})
}

func TestTransferRepository_UpdateMemo(t *testing.T) {
	t.Run("updates memo on both sides", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update memo
		if err := transferRepo.UpdateMemo(pair.FromTransaction.TransferID.ID, "Monthly savings"); err != nil {
			t.Fatalf("UpdateMemo() error = %v", err)
		}

		// Verify updates
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if !retrieved.FromTransaction.Memo.Valid || retrieved.FromTransaction.Memo.String != "Monthly savings" {
			t.Error("Expected memo to be updated on from transaction")
		}
		if !retrieved.ToTransaction.Memo.Valid || retrieved.ToTransaction.Memo.String != "Monthly savings" {
			t.Error("Expected memo to be updated on to transaction")
		}
	})
}

func TestTransferRepository_UpdateStatus(t *testing.T) {
	t.Run("updates status on both sides", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update status
		if err := transferRepo.UpdateStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusCleared); err != nil {
			t.Fatalf("UpdateStatus() error = %v", err)
		}

		// Verify updates
		retrieved, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID() error = %v", err)
		}

		if retrieved.FromTransaction.Status != models.TransactionStatusCleared {
			t.Errorf("Expected from status cleared, got %s", retrieved.FromTransaction.Status)
		}
		if retrieved.ToTransaction.Status != models.TransactionStatusCleared {
			t.Errorf("Expected to status cleared, got %s", retrieved.ToTransaction.Status)
		}
	})
}

// =============================================================================
// Transfer Delete Tests
// =============================================================================

func TestTransferRepository_Delete(t *testing.T) {
	t.Run("deletes both sides of transfer", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Delete the transfer
		if err := transferRepo.Delete(pair.FromTransaction.TransferID.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify both transactions are deleted
		_, err := txnRepo.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Expected from transaction to be deleted")
		}
		_, err = txnRepo.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("Expected to transaction to be deleted")
		}
	})

	t.Run("returns NotFoundError for non-existent transfer", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)

		fakeID := models.NewID()
		err := transferRepo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent transfer")
		}
	})
}

func TestTransferRepository_DeleteByTransactionID(t *testing.T) {
	t.Run("deletes both sides given one transaction ID", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Delete using the from transaction ID
		if err := transferRepo.DeleteByTransactionID(pair.FromTransaction.ID); err != nil {
			t.Fatalf("DeleteByTransactionID() error = %v", err)
		}

		// Verify both transactions are deleted
		_, err := txnRepo.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Expected from transaction to be deleted")
		}
		_, err = txnRepo.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("Expected to transaction to be deleted")
		}
	})

	t.Run("returns error for non-transfer transaction", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and regular transaction
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		txn := models.NewTransaction(checking.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		err := transferRepo.DeleteByTransactionID(txn.ID)
		if err == nil {
			t.Error("DeleteByTransactionID() expected error for non-transfer transaction")
		}
	})
}

// =============================================================================
// Transfer Query Tests
// =============================================================================

func TestTransferRepository_IsTransfer(t *testing.T) {
	t.Run("returns true for transfer transaction", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts and transfer
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isTransfer, err := transferRepo.IsTransfer(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if !isTransfer {
			t.Error("Expected IsTransfer to return true")
		}
	})

	t.Run("returns false for non-transfer transaction", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account and regular transaction
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		txn := models.NewTransaction(checking.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		isTransfer, err := transferRepo.IsTransfer(txn.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if isTransfer {
			t.Error("Expected IsTransfer to return false")
		}
	})
}

func TestTransferRepository_ListByAccount(t *testing.T) {
	t.Run("lists transfer transactions for account", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create accounts
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		// Create a transfer
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)
		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create transfer error = %v", err)
		}

		// Create a regular transaction
		regularTxn := models.NewTransaction(checking.ID, date, models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(regularTxn); err != nil {
			t.Fatalf("Create regular transaction error = %v", err)
		}

		// List transfers for checking account
		transfers, err := transferRepo.ListByAccount(checking.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(transfers) != 1 {
			t.Errorf("Expected 1 transfer, got %d", len(transfers))
		}
	})

	t.Run("returns empty list for account without transfers", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create account with only regular transactions
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}

		regularTxn := models.NewTransaction(checking.ID, models.NewDate(now.Year(), now.Month(), now.Day()), models.MustNewMoney("-50.00"))
		if err := txnRepo.Create(regularTxn); err != nil {
			t.Fatalf("Create regular transaction error = %v", err)
		}

		transfers, err := transferRepo.ListByAccount(checking.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(transfers) != 0 {
			t.Errorf("Expected 0 transfers, got %d", len(transfers))
		}
	})
}

func TestTransferRepository_CountByAccount(t *testing.T) {
	t.Run("counts transfer transactions for account", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)

		// Create accounts
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		// Create multiple transfers
		for range 3 {
			amount := models.MustNewMoney("100.00")
			date := models.NewDate(now.Year(), now.Month(), now.Day())
			pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)
			if err := transferRepo.Create(pair); err != nil {
				t.Fatalf("Create transfer error = %v", err)
			}
		}

		// Count for checking (should have 3 outgoing transfers)
		count, err := transferRepo.CountByAccount(checking.ID)
		if err != nil {
			t.Fatalf("CountByAccount() error = %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3, got %d", count)
		}

		// Count for savings (should have 3 incoming transfers)
		count, err = transferRepo.CountByAccount(savings.ID)
		if err != nil {
			t.Fatalf("CountByAccount() error = %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3, got %d", count)
		}
	})
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestTransferRepository_TransferWorkflow(t *testing.T) {
	t.Run("full transfer lifecycle", func(t *testing.T) {
		database := createTestDB(t)
		transferRepo := NewTransferRepository(database)
		accountRepo := NewAccountRepository(database)
		txnRepo := NewTransactionRepository(database)

		// Create accounts
		now := time.Now()
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.MustNewMoney("5000.00"), models.NewDate(now.Year(), now.Month(), now.Day()))
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.MustNewMoney("1000.00"), models.NewDate(now.Year(), now.Month(), now.Day()))

		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Create checking error = %v", err)
		}
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Create savings error = %v", err)
		}

		// Create transfer: $500 from checking to savings
		amount := models.MustNewMoney("500.00")
		date := models.NewDate(now.Year(), now.Month(), now.Day())
		pair := models.NewTransferPair(checking.ID, savings.ID, date, amount)

		if err := transferRepo.Create(pair); err != nil {
			t.Fatalf("Create transfer error = %v", err)
		}

		// Verify transaction count in each account
		checkingTxns, err := txnRepo.ListByAccount(checking.ID)
		if err != nil {
			t.Fatalf("ListByAccount checking error = %v", err)
		}
		if len(checkingTxns) != 1 {
			t.Errorf("Expected 1 transaction in checking, got %d", len(checkingTxns))
		}

		savingsTxns, err := txnRepo.ListByAccount(savings.ID)
		if err != nil {
			t.Fatalf("ListByAccount savings error = %v", err)
		}
		if len(savingsTxns) != 1 {
			t.Errorf("Expected 1 transaction in savings, got %d", len(savingsTxns))
		}

		// Update transfer amount
		if err := transferRepo.UpdateAmount(pair.FromTransaction.TransferID.ID, models.MustNewMoney("750.00")); err != nil {
			t.Fatalf("UpdateAmount error = %v", err)
		}

		// Verify updated amounts
		updatedPair, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID error = %v", err)
		}
		if !updatedPair.FromTransaction.Amount.Equal(models.MustNewMoney("-750.00")) {
			t.Errorf("Expected from amount -750.00, got %s", updatedPair.FromTransaction.Amount.String())
		}

		// Clear the transfer
		if err := transferRepo.UpdateStatus(pair.FromTransaction.TransferID.ID, models.TransactionStatusCleared); err != nil {
			t.Fatalf("UpdateStatus error = %v", err)
		}

		// Verify status
		clearedPair, err := transferRepo.GetByTransferID(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetByTransferID error = %v", err)
		}
		if clearedPair.FromTransaction.Status != models.TransactionStatusCleared {
			t.Error("Expected from transaction to be cleared")
		}
		if clearedPair.ToTransaction.Status != models.TransactionStatusCleared {
			t.Error("Expected to transaction to be cleared")
		}

		// Delete the transfer
		if err := transferRepo.Delete(pair.FromTransaction.TransferID.ID); err != nil {
			t.Fatalf("Delete error = %v", err)
		}

		// Verify transactions are deleted
		checkingTxns, err = txnRepo.ListByAccount(checking.ID)
		if err != nil {
			t.Fatalf("ListByAccount checking error = %v", err)
		}
		if len(checkingTxns) != 0 {
			t.Errorf("Expected 0 transactions in checking after delete, got %d", len(checkingTxns))
		}

		savingsTxns, err = txnRepo.ListByAccount(savings.ID)
		if err != nil {
			t.Fatalf("ListByAccount savings error = %v", err)
		}
		if len(savingsTxns) != 0 {
			t.Errorf("Expected 0 transactions in savings after delete, got %d", len(savingsTxns))
		}
	})
}
