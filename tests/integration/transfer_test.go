package integration

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// createTransferTestService creates a test database with a TransactionService for transfer testing.
func createTransferTestService(t *testing.T) (*transaction.Service, *db.DB, func()) {
	t.Helper()

	database := dbtest.New(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	svc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	cleanup := func() {}

	return svc, database, cleanup
}

// createTwoAccounts is a helper to create a checking and savings account.
func createTwoAccounts(t *testing.T, database *db.DB) (*account.Account, *account.Account) {
	t.Helper()

	accountRepo := account.NewRepository(database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
	savings := account.NewAccount("Savings", account.TypeSavings, "USD",
		types.MustNewMoney("5000.00"), types.NewDate(2024, 1, 1))

	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	if err := accountRepo.Create(savings); err != nil {
		t.Fatalf("Failed to create savings account: %v", err)
	}

	return checking, savings
}

func TestTransferCreate(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	t.Run("creates linked transfer between accounts", func(t *testing.T) {
		pair, err := svc.CreateTransfer(
			checking.ID, savings.ID,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("500.00"),
		)
		if err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		// Verify from side (negative amount)
		if !pair.FromTransaction.Amount.Equal(types.MustNewMoney("-500.00")) {
			t.Errorf("Expected from amount -500.00, got %s", pair.FromTransaction.Amount.String())
		}
		if pair.FromTransaction.AccountID != checking.ID {
			t.Errorf("Expected from account to be checking")
		}

		// Verify to side (positive amount)
		if !pair.ToTransaction.Amount.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected to amount 500.00, got %s", pair.ToTransaction.Amount.String())
		}
		if pair.ToTransaction.AccountID != savings.ID {
			t.Errorf("Expected to account to be savings")
		}

		// Both should be transfers
		if !pair.FromTransaction.IsTransfer() {
			t.Error("Expected from transaction to be a transfer")
		}
		if !pair.ToTransaction.IsTransfer() {
			t.Error("Expected to transaction to be a transfer")
		}

		// Transfer IDs should match
		if pair.FromTransaction.TransferID != pair.ToTransaction.TransferID {
			t.Error("Transfer IDs should match between both sides")
		}
	})

	t.Run("rejects negative transfer amount", func(t *testing.T) {
		_, err := svc.CreateTransfer(
			checking.ID, savings.ID,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-100.00"),
		)
		if err == nil {
			t.Error("Expected error for negative transfer amount")
		}
		if _, ok := err.(*transaction.InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T: %v", err, err)
		}
	})

	t.Run("rejects zero transfer amount", func(t *testing.T) {
		_, err := svc.CreateTransfer(
			checking.ID, savings.ID,
			types.NewDate(2024, 1, 15),
			types.ZeroMoney,
		)
		if err == nil {
			t.Error("Expected error for zero transfer amount")
		}
	})
}

func TestTransferGetPair(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	pair, err := svc.CreateTransfer(
		checking.ID, savings.ID,
		types.NewDate(2024, 2, 1),
		types.MustNewMoney("250.00"),
	)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	t.Run("retrieves transfer pair by transfer ID", func(t *testing.T) {
		transferID := pair.FromTransaction.TransferID.ID
		retrieved, err := svc.GetTransferPair(transferID)
		if err != nil {
			t.Fatalf("Failed to get transfer pair: %v", err)
		}

		if !retrieved.FromTransaction.Amount.Equal(types.MustNewMoney("-250.00")) {
			t.Errorf("Expected from amount -250.00, got %s", retrieved.FromTransaction.Amount.String())
		}
		if !retrieved.ToTransaction.Amount.Equal(types.MustNewMoney("250.00")) {
			t.Errorf("Expected to amount 250.00, got %s", retrieved.ToTransaction.Amount.String())
		}
	})

	t.Run("returns error for non-existent transfer ID", func(t *testing.T) {
		_, err := svc.GetTransferPair(types.NewID())
		if err == nil {
			t.Error("Expected error for non-existent transfer")
		}
	})
}

func TestTransferGetCounterpart(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	pair, err := svc.CreateTransfer(
		checking.ID, savings.ID,
		types.NewDate(2024, 2, 1),
		types.MustNewMoney("300.00"),
	)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	t.Run("gets counterpart from the from side", func(t *testing.T) {
		other, err := svc.GetTransferCounterpart(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("Failed to get counterpart: %v", err)
		}
		if other.ID != pair.ToTransaction.ID {
			t.Errorf("Expected counterpart to be to-transaction")
		}
		if !other.Amount.Equal(types.MustNewMoney("300.00")) {
			t.Errorf("Expected counterpart amount 300.00, got %s", other.Amount.String())
		}
	})

	t.Run("gets counterpart from the to side", func(t *testing.T) {
		other, err := svc.GetTransferCounterpart(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("Failed to get counterpart: %v", err)
		}
		if other.ID != pair.FromTransaction.ID {
			t.Errorf("Expected counterpart to be from-transaction")
		}
		if !other.Amount.Equal(types.MustNewMoney("-300.00")) {
			t.Errorf("Expected counterpart amount -300.00, got %s", other.Amount.String())
		}
	})
}

func TestTransferUpdate(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	pair, err := svc.CreateTransfer(
		checking.ID, savings.ID,
		types.NewDate(2024, 3, 1),
		types.MustNewMoney("200.00"),
	)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	transferID := pair.FromTransaction.TransferID.ID

	t.Run("updates transfer amount on both sides", func(t *testing.T) {
		err := svc.UpdateTransferAmount(transferID, types.MustNewMoney("350.00"))
		if err != nil {
			t.Fatalf("Failed to update amount: %v", err)
		}

		updated, err := svc.GetTransferPair(transferID)
		if err != nil {
			t.Fatalf("Failed to get updated pair: %v", err)
		}

		if !updated.FromTransaction.Amount.Equal(types.MustNewMoney("-350.00")) {
			t.Errorf("Expected from amount -350.00, got %s", updated.FromTransaction.Amount.String())
		}
		if !updated.ToTransaction.Amount.Equal(types.MustNewMoney("350.00")) {
			t.Errorf("Expected to amount 350.00, got %s", updated.ToTransaction.Amount.String())
		}
	})

	t.Run("updates transfer date on both sides", func(t *testing.T) {
		newDate := types.NewDate(2024, 3, 15)
		err := svc.UpdateTransferDate(transferID, newDate)
		if err != nil {
			t.Fatalf("Failed to update date: %v", err)
		}

		updated, err := svc.GetTransferPair(transferID)
		if err != nil {
			t.Fatalf("Failed to get updated pair: %v", err)
		}

		if !updated.FromTransaction.Date.Equal(newDate) {
			t.Errorf("Expected from date %v, got %v", newDate, updated.FromTransaction.Date)
		}
		if !updated.ToTransaction.Date.Equal(newDate) {
			t.Errorf("Expected to date %v, got %v", newDate, updated.ToTransaction.Date)
		}
	})

	t.Run("updates transfer status on both sides", func(t *testing.T) {
		err := svc.UpdateTransferStatus(transferID, transaction.StatusCleared)
		if err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		updated, err := svc.GetTransferPair(transferID)
		if err != nil {
			t.Fatalf("Failed to get updated pair: %v", err)
		}

		if updated.FromTransaction.Status != transaction.StatusCleared {
			t.Errorf("Expected from status 'cleared', got %q", updated.FromTransaction.Status)
		}
		if updated.ToTransaction.Status != transaction.StatusCleared {
			t.Errorf("Expected to status 'cleared', got %q", updated.ToTransaction.Status)
		}
	})

	t.Run("UpdateTransfer updates all fields", func(t *testing.T) {
		err := svc.UpdateTransfer(
			transferID,
			types.NewDate(2024, 4, 1),
			types.MustNewMoney("999.00"),
			"Monthly transfer",
			transaction.StatusReconciled,
		)
		if err != nil {
			t.Fatalf("Failed to update transfer: %v", err)
		}

		updated, err := svc.GetTransferPair(transferID)
		if err != nil {
			t.Fatalf("Failed to get updated pair: %v", err)
		}

		if !updated.FromTransaction.Amount.Equal(types.MustNewMoney("-999.00")) {
			t.Errorf("Expected from amount -999.00, got %s", updated.FromTransaction.Amount.String())
		}
		if !updated.ToTransaction.Amount.Equal(types.MustNewMoney("999.00")) {
			t.Errorf("Expected to amount 999.00, got %s", updated.ToTransaction.Amount.String())
		}
		if !updated.FromTransaction.Date.Equal(types.NewDate(2024, 4, 1)) {
			t.Errorf("Expected date 2024-04-01")
		}
		if !updated.FromTransaction.Memo.Valid || updated.FromTransaction.Memo.String != "Monthly transfer" {
			t.Errorf("Expected memo 'Monthly transfer', got %v", updated.FromTransaction.Memo)
		}
		if updated.FromTransaction.Status != transaction.StatusReconciled {
			t.Errorf("Expected status 'reconciled', got %q", updated.FromTransaction.Status)
		}
	})
}

func TestTransferDelete(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	t.Run("deletes both sides of transfer", func(t *testing.T) {
		pair, err := svc.CreateTransfer(
			checking.ID, savings.ID,
			types.NewDate(2024, 5, 1),
			types.MustNewMoney("100.00"),
		)
		if err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		transferID := pair.FromTransaction.TransferID.ID
		fromID := pair.FromTransaction.ID
		toID := pair.ToTransaction.ID

		err = svc.DeleteTransfer(transferID)
		if err != nil {
			t.Fatalf("Failed to delete transfer: %v", err)
		}

		// Both transactions should be gone
		_, err = svc.GetByID(fromID)
		if err == nil {
			t.Error("Expected from-transaction to be deleted")
		}

		_, err = svc.GetByID(toID)
		if err == nil {
			t.Error("Expected to-transaction to be deleted")
		}
	})

	t.Run("deleting one side via Delete removes both", func(t *testing.T) {
		pair, err := svc.CreateTransfer(
			checking.ID, savings.ID,
			types.NewDate(2024, 5, 15),
			types.MustNewMoney("75.00"),
		)
		if err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		fromID := pair.FromTransaction.ID
		toID := pair.ToTransaction.ID

		// Delete via the generic Delete method using one side's ID
		err = svc.Delete(fromID)
		if err != nil {
			t.Fatalf("Failed to delete transfer via Delete: %v", err)
		}

		// Both should be gone
		_, err = svc.GetByID(fromID)
		if err == nil {
			t.Error("Expected from-transaction to be deleted")
		}

		_, err = svc.GetByID(toID)
		if err == nil {
			t.Error("Expected to-transaction to be deleted")
		}
	})
}

func TestTransferIsTransfer(t *testing.T) {
	svc, database, cleanup := createTransferTestService(t)
	defer cleanup()

	checking, savings := createTwoAccounts(t, database)

	// Create a transfer
	pair, err := svc.CreateTransfer(
		checking.ID, savings.ID,
		types.NewDate(2024, 6, 1),
		types.MustNewMoney("50.00"),
	)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	// Create a regular transaction
	txnRepo := transaction.NewRepository(database)
	regularTxn := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-20.00"))
	if err := txnRepo.Create(regularTxn); err != nil {
		t.Fatalf("Failed to create regular transaction: %v", err)
	}

	t.Run("identifies transfer transaction", func(t *testing.T) {
		isTransfer, err := svc.IsTransfer(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("Failed to check IsTransfer: %v", err)
		}
		if !isTransfer {
			t.Error("Expected transaction to be a transfer")
		}
	})

	t.Run("identifies non-transfer transaction", func(t *testing.T) {
		isTransfer, err := svc.IsTransfer(regularTxn.ID)
		if err != nil {
			t.Fatalf("Failed to check IsTransfer: %v", err)
		}
		if isTransfer {
			t.Error("Expected regular transaction not to be a transfer")
		}
	})
}
