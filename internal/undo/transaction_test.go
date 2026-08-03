package undo_test

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

type testEnv struct {
	txnSvc       *transaction.Service
	transferSvc  *transfer.Service
	accountRepo  *account.Repository
	categoryRepo *category.Repository
}

func createTestEnv(t *testing.T) *testEnv {
	t.Helper()
	database := createTestDB(t)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo, database)
	return &testEnv{
		txnSvc: svc,
		transferSvc: transfer.NewService(txnRepo, investment.NewRepository(database),
			splitRepo, accountRepo, categoryRepo, database),
		accountRepo:  accountRepo,
		categoryRepo: categoryRepo,
	}
}

// seedTransfer creates a bank↔bank transfer through the transfer service and
// returns its transfer_id plus both leg row IDs.
func seedTransfer(t *testing.T, env *testEnv, from, to *account.Account, amount types.Money) (types.ID, types.ID, types.ID) {
	t.Helper()
	res, err := env.transferSvc.Create(transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.Today(),
		Amount:        amount,
	})
	if err != nil {
		t.Fatalf("seed transfer: %v", err)
	}
	return res.TransferID, res.From.RowID, res.To.RowID
}

func createTestAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

// createTestAccountOfType creates an account of an arbitrary type, so the
// transfer command tests can drive all four (From, To) ledger combinations.
func createTestAccountOfType(t *testing.T, repo *account.Repository, name string, at account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, at, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
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
// Transfer command tests
// =============================================================================
//
// One set of tests for one set of commands. Before, the same four scenarios were
// covered separately for bank↔bank here and for the three investment shapes in
// investment_transfer_test.go, because there were seven commands for one concept.

func TestCreateTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates transfer and deletes both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("500.00")
		cmd := undo.NewCreateTransferCommand(env.transferSvc, transfer.Spec{
			FromAccountID: from.ID,
			ToAccountID:   to.ID,
			Date:          types.Today(),
			Amount:        amount,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		res := cmd.Result()
		if res == nil {
			t.Fatal("Result() should not be nil after Execute")
		}

		fromTxn, err := env.txnSvc.GetByID(res.From.RowID)
		if err != nil {
			t.Fatalf("GetByID(from) error = %v", err)
		}
		if !fromTxn.Amount.Equal(amount.Neg()) {
			t.Errorf("from amount = %s, want %s", fromTxn.Amount.String(), amount.Neg().String())
		}

		toTxn, err := env.txnSvc.GetByID(res.To.RowID)
		if err != nil {
			t.Fatalf("GetByID(to) error = %v", err)
		}
		if !toTxn.Amount.Equal(amount) {
			t.Errorf("to amount = %s, want %s", toTxn.Amount.String(), amount.String())
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		if _, err := env.txnSvc.GetByID(res.From.RowID); err == nil {
			t.Error("from transaction should not exist after undo")
		}
		if _, err := env.txnSvc.GetByID(res.To.RowID); err == nil {
			t.Error("to transaction should not exist after undo")
		}
	})
}

func TestCreateTransferCommand_Description(t *testing.T) {
	cmd := undo.NewCreateTransferCommand(nil, transfer.Spec{})
	if cmd.Description() != "Create transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transfer")
	}
}

func TestCreateTransferCommand_ResultNilBeforeExecute(t *testing.T) {
	cmd := undo.NewCreateTransferCommand(nil, transfer.Spec{})
	if cmd.Result() != nil {
		t.Error("Result() should be nil before Execute")
	}
	if err := cmd.Undo(); err == nil {
		t.Error("Undo() before Execute should error rather than panic")
	}
}

func TestDeleteTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes transfer and recreates both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("200.00")
		transferID, fromLeg, toLeg := seedTransfer(t, env, from, to, amount)

		cmd := undo.NewDeleteTransferCommand(env.transferSvc, transferID)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if _, err := env.txnSvc.GetByID(fromLeg); err == nil {
			t.Error("from transaction should not exist after delete")
		}
		if _, err := env.txnSvc.GetByID(toLeg); err == nil {
			t.Error("to transaction should not exist after delete")
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// The recreated pair reuses the ORIGINAL transfer_id, so a second undo
		// step still addresses the same transfer. The rows themselves are new.
		restored, err := env.transferSvc.Get(transferID)
		if err != nil {
			t.Fatalf("Get after undo error = %v", err)
		}
		if !restored.Amount.Equal(amount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount, amount)
		}
		if restored.From.AccountID != from.ID || restored.To.AccountID != to.ID {
			t.Errorf("direction after undo = %s→%s, want %s→%s",
				restored.From.AccountID, restored.To.AccountID, from.ID, to.ID)
		}
	})
}

func TestDeleteTransferCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteTransferCommand(nil, types.NewID())
	if cmd.Description() != "Delete transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete transfer")
	}
}

func TestVoidTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("voids transfer and restores both sides on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		amount := types.MustNewMoney("300.00")
		transferID, fromLeg, toLeg := seedTransfer(t, env, from, to, amount)

		cmd := undo.NewVoidTransferCommand(env.transferSvc, transferID)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		voidedFrom, err := env.txnSvc.GetByID(fromLeg)
		if err != nil {
			t.Fatalf("GetByID(from) after void error = %v", err)
		}
		if !voidedFrom.IsVoid() {
			t.Error("from transaction should be void")
		}
		if !voidedFrom.Amount.IsZero() {
			t.Errorf("from amount should be zero, got %s", voidedFrom.Amount.String())
		}

		voidedTo, err := env.txnSvc.GetByID(toLeg)
		if err != nil {
			t.Fatalf("GetByID(to) after void error = %v", err)
		}
		if !voidedTo.IsVoid() {
			t.Error("to transaction should be void")
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredFrom, err := env.txnSvc.GetByID(fromLeg)
		if err != nil {
			t.Fatalf("GetByID(from) after undo error = %v", err)
		}
		if restoredFrom.IsVoid() {
			t.Error("from transaction should not be void after undo")
		}
		if !restoredFrom.Amount.Equal(amount.Neg()) {
			t.Errorf("from amount after undo = %s, want %s", restoredFrom.Amount.String(), amount.Neg().String())
		}

		restoredTo, err := env.txnSvc.GetByID(toLeg)
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
	t.Run("returns error when the id is not a transfer", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")

		txn := transaction.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewVoidTransferCommand(env.transferSvc, txn.ID)
		if err := cmd.Execute(); err == nil {
			t.Error("Execute() should return error for a non-transfer id")
		}
	})
}

func TestVoidTransferCommand_Description(t *testing.T) {
	cmd := undo.NewVoidTransferCommand(nil, types.NewID())
	if cmd.Description() != "Void transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Void transfer")
	}
}

func TestEditTransferCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits both sides and restores prior values on undo", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		original := types.MustNewMoney("100.00")
		transferID, fromLeg, toLeg := seedTransfer(t, env, from, to, original)

		newAmount := types.MustNewMoney("250.00")
		newDate := types.NewDate(2024, time.March, 15)
		cmd := undo.NewEditTransferCommand(env.transferSvc, transferID, transfer.Edit{
			Date:   newDate,
			Amount: newAmount,
			Memo:   "updated",
			Status: transaction.StatusCleared,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		edited, err := env.transferSvc.Get(transferID)
		if err != nil {
			t.Fatalf("Get after edit error = %v", err)
		}
		if !edited.Amount.Equal(newAmount) {
			t.Errorf("amount = %s, want %s", edited.Amount, newAmount)
		}
		if edited.Date != newDate {
			t.Errorf("date = %v, want %v", edited.Date, newDate)
		}
		if edited.Memo != "updated" {
			t.Errorf("memo = %q, want %q", edited.Memo, "updated")
		}
		if edited.Status != transaction.StatusCleared {
			t.Errorf("status = %q, want cleared", edited.Status)
		}
		// The edit is in place: the same two rows still hold the transfer.
		if edited.From.RowID != fromLeg || edited.To.RowID != toLeg {
			t.Errorf("edit replaced rows: (%s,%s) -> (%s,%s)",
				fromLeg, toLeg, edited.From.RowID, edited.To.RowID)
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		reverted, err := env.transferSvc.Get(transferID)
		if err != nil {
			t.Fatalf("Get after undo error = %v", err)
		}
		if !reverted.Amount.Equal(original) {
			t.Errorf("amount after undo = %s, want %s", reverted.Amount, original)
		}
		if reverted.Memo != "" {
			t.Errorf("memo after undo = %q, want empty", reverted.Memo)
		}
		if reverted.Status != transaction.StatusUncleared {
			t.Errorf("status after undo = %q, want uncleared", reverted.Status)
		}
	})
}

func TestEditTransferCommand_UndoBeforeExecute(t *testing.T) {
	cmd := undo.NewEditTransferCommand(nil, types.NewID(), transfer.Edit{})
	if err := cmd.Undo(); err == nil {
		t.Error("Undo() before Execute should error rather than panic")
	}
}

func TestEditTransferCommand_Description(t *testing.T) {
	cmd := undo.NewEditTransferCommand(nil, types.NewID(), transfer.Edit{})
	if cmd.Description() != "Edit transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit transfer")
	}
}

// =============================================================================
// Integration: transfer commands through the manager
// =============================================================================

func TestCompoundCommand_VoidTransferWithManager(t *testing.T) {
	t.Run("void transfer via manager undo/redo cycle", func(t *testing.T) {
		env := createTestEnv(t)
		from := createTestAccount(t, env.accountRepo, "Checking")
		to := createTestAccount(t, env.accountRepo, "Savings")

		transferID, fromLeg, _ := seedTransfer(t, env, from, to, types.MustNewMoney("150.00"))

		mgr := undo.NewManager()
		cmd := undo.NewVoidTransferCommand(env.transferSvc, transferID)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}
		fromTxn, _ := env.txnSvc.GetByID(fromLeg)
		if !fromTxn.IsVoid() {
			t.Error("from should be void after execute")
		}

		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Void transfer" {
			t.Errorf("undo desc = %q, want %q", desc, "Void transfer")
		}
		fromTxn, _ = env.txnSvc.GetByID(fromLeg)
		if fromTxn.IsVoid() {
			t.Error("from should not be void after undo")
		}

		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Void transfer" {
			t.Errorf("redo desc = %q, want %q", desc, "Void transfer")
		}
		fromTxn, _ = env.txnSvc.GetByID(fromLeg)
		if !fromTxn.IsVoid() {
			t.Error("from should be void after redo")
		}
	})
}

// TestTransferCommands_AllFourShapes is what collapsing seven commands to four
// buys: the same undo/redo cycle, asserted once, across every shape — including
// the three investment shapes that used to need their own commands and their own
// test file.
func TestTransferCommands_AllFourShapes(t *testing.T) {
	shapes := []struct {
		name     string
		fromType account.Type
		toType   account.Type
	}{
		{"reg→reg", account.TypeChecking, account.TypeSavings},
		{"inv→reg", account.TypeInvestment, account.TypeChecking},
		{"reg→inv", account.TypeChecking, account.TypeInvestment},
		{"inv→inv", account.TypeInvestment, account.TypeInvestment},
	}

	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			env := createTestEnv(t)
			from := createTestAccountOfType(t, env.accountRepo, "From "+sh.name, sh.fromType)
			to := createTestAccountOfType(t, env.accountRepo, "To "+sh.name, sh.toType)

			amount := types.MustNewMoney("175.00")
			mgr := undo.NewManager()

			create := undo.NewCreateTransferCommand(env.transferSvc, transfer.Spec{
				FromAccountID: from.ID, ToAccountID: to.ID,
				Date: types.Today(), Amount: amount,
			})
			if err := mgr.Execute(create); err != nil {
				t.Fatalf("create: %v", err)
			}
			transferID := create.Result().TransferID

			if _, err := env.transferSvc.Get(transferID); err != nil {
				t.Fatalf("transfer missing after create: %v", err)
			}

			// Edit, then undo the edit.
			edit := undo.NewEditTransferCommand(env.transferSvc, transferID, transfer.Edit{
				Date: types.Today(), Amount: types.MustNewMoney("225.00"), Memo: "edited",
			})
			if err := mgr.Execute(edit); err != nil {
				t.Fatalf("edit: %v", err)
			}
			if _, err := mgr.Undo(); err != nil {
				t.Fatalf("undo edit: %v", err)
			}
			afterUndo, err := env.transferSvc.Get(transferID)
			if err != nil {
				t.Fatalf("get after undo edit: %v", err)
			}
			if !afterUndo.Amount.Equal(amount) {
				t.Errorf("amount after undoing edit = %s, want %s", afterUndo.Amount, amount)
			}

			// Delete, then undo the delete.
			del := undo.NewDeleteTransferCommand(env.transferSvc, transferID)
			if err := mgr.Execute(del); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if _, err := env.transferSvc.Get(transferID); err == nil {
				t.Error("transfer should be gone after delete")
			}
			if _, err := mgr.Undo(); err != nil {
				t.Fatalf("undo delete: %v", err)
			}
			if _, err := env.transferSvc.Get(transferID); err != nil {
				t.Errorf("transfer should be back after undoing delete: %v", err)
			}
		})
	}
}

// =============================================================================
// EditTransactionWithSplitsCommand Tests
// =============================================================================

func TestEditTransactionWithSplitsCommand_ExecuteAndUndo_SplitToSplit(t *testing.T) {
	t.Run("replaces splits and parent fields, restores both on undo", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")
		cat1 := createTestCategory(t, env.categoryRepo, "Food")
		cat2 := createTestCategory(t, env.categoryRepo, "Drink")
		cat3 := createTestCategory(t, env.categoryRepo, "Snack")

		origAmount := types.MustNewMoney("-100.00")
		origMemo := "lunch"
		txn := transaction.NewTransaction(acct.ID, types.Today(), origAmount)
		txn.SetMemo(origMemo)
		origSplits := []*transaction.Split{
			transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-60.00")),
			transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-40.00")),
		}
		if err := env.txnSvc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		newAmount := types.MustNewMoney("-150.00")
		updated, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		updated.Amount = newAmount
		updated.SetMemo("dinner")
		updated.ClearCategory()

		newSplits := []*transaction.Split{
			transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-90.00")),
			transaction.NewSplit(txn.ID, cat3.ID, types.MustNewMoney("-60.00")),
		}

		cmd := undo.NewEditTransactionWithSplitsCommand(env.txnSvc, updated, newSplits)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		got, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !got.Amount.Equal(newAmount) {
			t.Errorf("amount after edit = %s, want %s", got.Amount.String(), newAmount.String())
		}
		if got.Memo.String != "dinner" {
			t.Errorf("memo after edit = %q, want %q", got.Memo.String, "dinner")
		}

		gotSplits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after Execute error = %v", err)
		}
		if len(gotSplits) != 2 {
			t.Fatalf("expected 2 splits after edit, got %d", len(gotSplits))
		}
		// Splits are unordered by default; sum check the new amounts.
		var sum types.Money
		for _, s := range gotSplits {
			sum = sum.Add(s.Amount)
		}
		if !sum.Equal(newAmount) {
			t.Errorf("split sum after edit = %s, want %s", sum.String(), newAmount.String())
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Equal(origAmount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.String(), origAmount.String())
		}
		if restored.Memo.String != origMemo {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, origMemo)
		}

		restoredSplits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after Undo error = %v", err)
		}
		if len(restoredSplits) != 2 {
			t.Fatalf("expected 2 splits after undo, got %d", len(restoredSplits))
		}
		var origSum types.Money
		for _, s := range restoredSplits {
			origSum = origSum.Add(s.Amount)
		}
		if !origSum.Equal(origAmount) {
			t.Errorf("restored split sum = %s, want %s", origSum.String(), origAmount.String())
		}
	})
}

func TestEditTransactionWithSplitsCommand_SplitToPlain(t *testing.T) {
	t.Run("removing all splits converts back to plain transaction", func(t *testing.T) {
		env := createTestEnv(t)
		acct := createTestAccount(t, env.accountRepo, "Checking")
		cat1 := createTestCategory(t, env.categoryRepo, "Food")
		cat2 := createTestCategory(t, env.categoryRepo, "Drink")
		newCat := createTestCategory(t, env.categoryRepo, "Misc")

		amount := types.MustNewMoney("-50.00")
		txn := transaction.NewTransaction(acct.ID, types.Today(), amount)
		origSplits := []*transaction.Split{
			transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-30.00")),
			transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-20.00")),
		}
		if err := env.txnSvc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		updated, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		updated.SetCategory(newCat.ID)

		cmd := undo.NewEditTransactionWithSplitsCommand(env.txnSvc, updated, nil)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		gotSplits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after Execute error = %v", err)
		}
		if len(gotSplits) != 0 {
			t.Errorf("expected 0 splits after split->plain, got %d", len(gotSplits))
		}

		got, err := env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !got.CategoryID.Valid || got.CategoryID.ID != newCat.ID {
			t.Errorf("category after split->plain = %v, want %v", got.CategoryID, newCat.ID)
		}

		// Undo restores splits.
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}
		restoredSplits, err := env.txnSvc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() after Undo error = %v", err)
		}
		if len(restoredSplits) != 2 {
			t.Errorf("expected 2 splits after undo, got %d", len(restoredSplits))
		}
	})
}

func TestEditTransactionWithSplitsCommand_Description(t *testing.T) {
	cmd := undo.NewEditTransactionWithSplitsCommand(nil, nil, nil)
	if cmd.Description() != "Edit transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit transaction")
	}
}
