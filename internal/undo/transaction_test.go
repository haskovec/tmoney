package undo_test

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

type testEnv struct {
	txnSvc       *transaction.Service
	accountRepo  *account.Repository
	categoryRepo *category.Repository
}

func createTestEnv(t *testing.T) *testEnv {
	t.Helper()
	database := createTestDB(t)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	return &testEnv{
		txnSvc:       svc,
		accountRepo:  accountRepo,
		categoryRepo: categoryRepo,
	}
}

func createTestAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

func createTestCategory(t *testing.T, repo *category.Repository, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Failed to create test category: %v", err)
	}
	return cat
}

// =============================================================================
// CreateTransactionCommand Tests
// =============================================================================

func TestCreateTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes transaction", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)

		cmd := undo.NewCreateTransactionCommand(env.txnSvc, txn)

		// Execute: transaction should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !retrieved.Amount.Equal(amount) {
			t.Errorf("amount = %s, want %s", retrieved.Amount.String(), amount.String())
		}

		// Undo: transaction should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("expected error after Undo (transaction should be deleted)")
		}
	})
}

func TestCreateTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewCreateTransactionCommand(nil, nil)
	if cmd.Description() != "Create transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transaction")
	}
}

func TestCreateTransactionCommand_WithSplits(t *testing.T) {
	t.Run("creates transaction with splits and undoes both", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")
		cat1 := createTestCategory(t, env.categoryRepo, "Food")
		cat2 := createTestCategory(t, env.categoryRepo, "Drink")

		amount := types.MustNewMoney("-100.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)

		split1 := transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-60.00"))
		split2 := transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-40.00"))

		cmd := undo.NewCreateTransactionWithSplitsCommand(env.txnSvc, txn, []*transaction.Split{split1, split2})

		// Execute
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		splits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 2 {
			t.Errorf("expected 2 splits, got %d", len(splits))
		}

		// Undo: transaction and splits should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("expected error after Undo (transaction should be deleted)")
		}
	})
}

func TestCreateTransactionCommand_WithManager(t *testing.T) {
	t.Run("works with undo manager execute and undo", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-25.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)

		mgr := undo.NewManager()
		cmd := undo.NewCreateTransactionCommand(env.txnSvc, txn)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		if !mgr.CanUndo() {
			t.Error("should be able to undo after execute")
		}

		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Create transaction" {
			t.Errorf("undo desc = %q, want %q", desc, "Create transaction")
		}

		_, err = env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("transaction should not exist after undo")
		}

		// Redo should recreate
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Create transaction" {
			t.Errorf("redo desc = %q, want %q", desc, "Create transaction")
		}

		retrieved, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if !retrieved.Amount.Equal(amount) {
			t.Errorf("amount after redo = %s, want %s", retrieved.Amount.String(), amount.String())
		}
	})
}

// =============================================================================
// EditTransactionCommand Tests
// =============================================================================

func TestEditTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		originalAmount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), originalAmount)
		txn.SetMemo("Original memo")
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		newAmount := types.MustNewMoney("-75.00")
		edited.Amount = newAmount
		edited.SetMemo("Edited memo")

		cmd := undo.NewEditTransactionCommand(env.txnSvc, edited)

		// Execute: should be edited
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !retrieved.Amount.Equal(newAmount) {
			t.Errorf("amount after edit = %s, want %s", retrieved.Amount.String(), newAmount.String())
		}
		if retrieved.Memo.String != "Edited memo" {
			t.Errorf("memo after edit = %q, want %q", retrieved.Memo.String, "Edited memo")
		}

		// Undo: should restore original
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Equal(originalAmount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.String(), originalAmount.String())
		}
		if restored.Memo.String != "Original memo" {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, "Original memo")
		}
	})
}

func TestEditTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewEditTransactionCommand(nil, nil)
	if cmd.Description() != "Edit transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit transaction")
	}
}

// =============================================================================
// DeleteTransactionCommand Tests
// =============================================================================

func TestDeleteTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes and then recreates transaction", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)
		txn.SetMemo("Test memo")
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewDeleteTransactionCommand(env.txnSvc, txn.ID)

		// Execute: transaction should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("expected error after Execute (transaction should be deleted)")
		}

		// Undo: transaction should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Equal(amount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.String(), amount.String())
		}
		if restored.Memo.String != "Test memo" {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, "Test memo")
		}
	})
}

func TestDeleteTransactionCommand_WithSplits(t *testing.T) {
	t.Run("deletes transaction with splits and restores both", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")
		cat1 := createTestCategory(t, env.categoryRepo, "Groceries")
		cat2 := createTestCategory(t, env.categoryRepo, "Dining")

		amount := types.MustNewMoney("-100.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)

		split1 := transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-60.00"))
		split2 := transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-40.00"))

		if err := env.txnSvc.CreateWithSplits(txn, []*transaction.Split{split1, split2}); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		cmd := undo.NewDeleteTransactionCommand(env.txnSvc, txn.ID)

		// Execute: transaction and splits gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("expected error after Execute (transaction should be deleted)")
		}

		// Undo: transaction and splits restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Equal(amount) {
			t.Errorf("amount = %s, want %s", restored.Amount.String(), amount.String())
		}

		splits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after Undo error = %v", err)
		}
		if len(splits) != 2 {
			t.Errorf("expected 2 splits after undo, got %d", len(splits))
		}
	})
}

func TestDeleteTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteTransactionCommand(nil, types.NewID())
	if cmd.Description() != "Delete transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete transaction")
	}
}

// =============================================================================
// VoidTransactionCommand Tests
// =============================================================================

func TestVoidTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("voids and then restores transaction", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		originalAmount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), originalAmount)
		txn.SetMemo("Original memo")
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewVoidTransactionCommand(env.txnSvc, txn.ID)

		// Execute: transaction should be void
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		voided, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after void error = %v", err)
		}
		if !voided.IsVoid() {
			t.Error("transaction should be void after Execute")
		}
		if !voided.Amount.IsZero() {
			t.Errorf("amount should be zero after void, got %s", voided.Amount.String())
		}
		if voided.Memo.String != "**VOID**" {
			t.Errorf("memo should be **VOID** after void, got %q", voided.Memo.String)
		}

		// Undo: transaction should be restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.IsVoid() {
			t.Error("transaction should not be void after undo")
		}
		if !restored.Amount.Equal(originalAmount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.String(), originalAmount.String())
		}
		if restored.Memo.String != "Original memo" {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, "Original memo")
		}
		if restored.Status != transaction.StatusUncleared {
			t.Errorf("status after undo = %q, want %q", restored.Status, transaction.StatusUncleared)
		}
	})
}

func TestVoidTransactionCommand_ClearedTransaction(t *testing.T) {
	t.Run("voids cleared transaction and restores cleared status", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-30.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := env.txnSvc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		cmd := undo.NewVoidTransactionCommand(env.txnSvc, txn.ID)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Status != transaction.StatusCleared {
			t.Errorf("status after undo = %q, want %q", restored.Status, transaction.StatusCleared)
		}
	})
}

func TestVoidTransactionCommand_WithSplits(t *testing.T) {
	t.Run("voids split transaction and restores splits on undo", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")
		cat1 := createTestCategory(t, env.categoryRepo, "Rent")
		cat2 := createTestCategory(t, env.categoryRepo, "Utilities")

		amount := types.MustNewMoney("-100.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)

		split1 := transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-60.00"))
		split2 := transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-40.00"))

		if err := env.txnSvc.CreateWithSplits(txn, []*transaction.Split{split1, split2}); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		cmd := undo.NewVoidTransactionCommand(env.txnSvc, txn.ID)

		// Execute: void removes splits
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		splits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after void error = %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("expected 0 splits after void, got %d", len(splits))
		}

		// Undo: splits should be restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredSplits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after undo error = %v", err)
		}
		if len(restoredSplits) != 2 {
			t.Errorf("expected 2 splits after undo, got %d", len(restoredSplits))
		}
	})
}

func TestVoidTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewVoidTransactionCommand(nil, types.NewID())
	if cmd.Description() != "Void transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Void transaction")
	}
}

// =============================================================================
// CreateTransferCommand Tests
// =============================================================================

func TestCreateTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates transfer and deletes both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("500.00")
		cmd := undo.NewCreateTransferCommand(env.txnSvc, from.ID, to.ID, types.Today(), amount)

		// Execute: transfer should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		pair := cmd.Pair()
		if pair == nil {
			t.Fatal("Pair() should not be nil after Execute")
		}

		fromTxn, err := env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(from) error = %v", err)
		}
		if !fromTxn.Amount.Equal(amount.Neg()) {
			t.Errorf("from amount = %s, want %s", fromTxn.Amount.String(), amount.Neg().String())
		}

		toTxn, err := env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(to) error = %v", err)
		}
		if !toTxn.Amount.Equal(amount) {
			t.Errorf("to amount = %s, want %s", toTxn.Amount.String(), amount.String())
		}

		// Undo: both sides should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("from transaction should not exist after undo")
		}
		_, err = env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("to transaction should not exist after undo")
		}
	})
}

func TestCreateTransferCommand_Description(t *testing.T) {
	cmd := undo.NewCreateTransferCommand(nil, types.NewID(), types.NewID(), types.Today(), types.ZeroMoney)
	if cmd.Description() != "Create transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transfer")
	}
}

func TestCreateTransferCommand_PairNilBeforeExecute(t *testing.T) {
	cmd := undo.NewCreateTransferCommand(nil, types.NewID(), types.NewID(), types.Today(), types.ZeroMoney)
	if cmd.Pair() != nil {
		t.Error("Pair() should be nil before Execute")
	}
}

// =============================================================================
// DeleteTransferCommand Tests
// =============================================================================

func TestDeleteTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes transfer and recreates both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("200.00")
		pair, err := env.txnSvc.CreateTransfer(from.ID, to.ID, types.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		transferID := pair.FromTransaction.TransferID.ID
		cmd := undo.NewDeleteTransferCommand(env.txnSvc, transferID)

		// Execute: both sides should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("from transaction should not exist after delete")
		}
		_, err = env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("to transaction should not exist after delete")
		}

		// Undo: both sides should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredFrom, err := env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(from) after undo error = %v", err)
		}
		if !restoredFrom.Amount.Equal(amount.Neg()) {
			t.Errorf("from amount after undo = %s, want %s", restoredFrom.Amount.String(), amount.Neg().String())
		}

		restoredTo, err := env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(to) after undo error = %v", err)
		}
		if !restoredTo.Amount.Equal(amount) {
			t.Errorf("to amount after undo = %s, want %s", restoredTo.Amount.String(), amount.String())
		}
	})
}

func TestDeleteTransferCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteTransferCommand(nil, types.NewID())
	if cmd.Description() != "Delete transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete transfer")
	}
}

// =============================================================================
// VoidTransferCommand Tests
// =============================================================================

func TestVoidTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("voids transfer and restores both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("300.00")
		pair, err := env.txnSvc.CreateTransfer(from.ID, to.ID, types.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		cmd := undo.NewVoidTransferCommand(env.txnSvc, pair.FromTransaction.ID)

		// Execute: both sides should be void
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		voidedFrom, err := env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(from) after void error = %v", err)
		}
		if !voidedFrom.IsVoid() {
			t.Error("from transaction should be void")
		}
		if !voidedFrom.Amount.IsZero() {
			t.Errorf("from amount should be zero, got %s", voidedFrom.Amount.String())
		}

		voidedTo, err := env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(to) after void error = %v", err)
		}
		if !voidedTo.IsVoid() {
			t.Error("to transaction should be void")
		}

		// Undo: both sides should be restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredFrom, err := env.txnSvc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(from) after undo error = %v", err)
		}
		if restoredFrom.IsVoid() {
			t.Error("from transaction should not be void after undo")
		}
		if !restoredFrom.Amount.Equal(amount.Neg()) {
			t.Errorf("from amount after undo = %s, want %s", restoredFrom.Amount.String(), amount.Neg().String())
		}

		restoredTo, err := env.txnSvc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID(to) after undo error = %v", err)
		}
		if restoredTo.IsVoid() {
			t.Error("to transaction should not be void after undo")
		}
		if !restoredTo.Amount.Equal(amount) {
			t.Errorf("to amount after undo = %s, want %s", restoredTo.Amount.String(), amount.String())
		}
	})
}

func TestVoidTransferCommand_NotATransfer(t *testing.T) {
	t.Run("returns error when transaction is not a transfer", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewVoidTransferCommand(env.txnSvc, txn.ID)
		err := cmd.Execute()
		if err == nil {
			t.Error("Execute() should return error for non-transfer transaction")
		}
	})
}

func TestVoidTransferCommand_Description(t *testing.T) {
	cmd := undo.NewVoidTransferCommand(nil, types.NewID())
	if cmd.Description() != "Void transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Void transfer")
	}
}

// =============================================================================
// Integration: compound commands with manager
// =============================================================================

func TestCompoundCommand_VoidTransferWithManager(t *testing.T) {
	t.Run("void transfer via manager undo/redo cycle", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("150.00")
		pair, err := env.txnSvc.CreateTransfer(from.ID, to.ID, types.Today(), amount)
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		mgr := undo.NewManager()
		cmd := undo.NewVoidTransferCommand(env.txnSvc, pair.FromTransaction.ID)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		// Verify void
		fromTxn, _ := env.txnSvc.GetByID(pair.FromTransaction.ID)
		if !fromTxn.IsVoid() {
			t.Error("from should be void after execute")
		}

		// Undo
		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Void transfer" {
			t.Errorf("undo desc = %q, want %q", desc, "Void transfer")
		}

		fromTxn, _ = env.txnSvc.GetByID(pair.FromTransaction.ID)
		if fromTxn.IsVoid() {
			t.Error("from should not be void after undo")
		}

		// Redo
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Void transfer" {
			t.Errorf("redo desc = %q, want %q", desc, "Void transfer")
		}

		fromTxn, _ = env.txnSvc.GetByID(pair.FromTransaction.ID)
		if !fromTxn.IsVoid() {
			t.Error("from should be void after redo")
		}
	})
}
