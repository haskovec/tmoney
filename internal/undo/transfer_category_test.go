package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

func wantLegCategory(t *testing.T, txn *transaction.Transaction, want types.NullableID, label string) {
	t.Helper()
	if want.Valid {
		if !txn.CategoryID.Valid || txn.CategoryID.ID != want.ID {
			t.Fatalf("%s: want category %s, got valid=%v id=%v", label, want.ID, txn.CategoryID.Valid, txn.CategoryID.ID)
		}
		return
	}
	if txn.CategoryID.Valid {
		t.Fatalf("%s: want no category, got %s", label, txn.CategoryID.ID)
	}
}

func newUndoCategory(t *testing.T, env *testEnv, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := env.categoryRepo.Create(cat); err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	return cat
}

func TestCreateTransferCommand_RoundTripsCategory(t *testing.T) {
	env := createTestEnv(t)
	from := createTestAccount(t, env.accountRepo, "Checking")
	to := createTestAccount(t, env.accountRepo, "Savings")
	bills := newUndoCategory(t, env, "Bills")

	cmd := undo.NewCreateTransferCommand(env.txnSvc, from.ID, to.ID, types.Today(),
		types.MustNewMoney("100.00"), "rent", types.NullableID{ID: bills.ID, Valid: true})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	pair := cmd.Pair()
	if pair == nil {
		t.Fatalf("Pair() nil after Execute")
	}
	want := types.NullableID{ID: bills.ID, Valid: true}
	reloaded, err := env.txnSvc.GetTransferPair(pair.FromTransaction.TransferID.ID)
	if err != nil {
		t.Fatalf("GetTransferPair: %v", err)
	}
	wantLegCategory(t, reloaded.FromTransaction, want, "from")
	wantLegCategory(t, reloaded.ToTransaction, want, "to")

	// Undo deletes both legs.
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := env.txnSvc.GetByID(pair.FromTransaction.ID); err == nil {
		t.Error("from leg should be gone after undo")
	}
	if _, err := env.txnSvc.GetByID(pair.ToTransaction.ID); err == nil {
		t.Error("to leg should be gone after undo")
	}
}

func TestEditTransferCommand_RoundTripsCategory(t *testing.T) {
	env := createTestEnv(t)
	from := createTestAccount(t, env.accountRepo, "Checking")
	to := createTestAccount(t, env.accountRepo, "Savings")
	catA := newUndoCategory(t, env, "Bills")
	catB := newUndoCategory(t, env, "Groceries")

	// Create with category A.
	pair, err := env.txnSvc.CreateTransfer(from.ID, to.ID, types.Today(),
		types.MustNewMoney("100.00"), "", types.NullableID{ID: catA.ID, Valid: true})
	if err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	transferID := pair.FromTransaction.TransferID.ID

	// Edit to category B; command captures beforeCategory = A.
	cmd := undo.NewEditTransferCommand(env.txnSvc, transferID, types.Today(),
		types.MustNewMoney("100.00"), "", transaction.StatusUncleared, types.NullableID{ID: catB.ID, Valid: true})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	afterEdit, err := env.txnSvc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("GetTransferPair after edit: %v", err)
	}
	wantB := types.NullableID{ID: catB.ID, Valid: true}
	wantLegCategory(t, afterEdit.FromTransaction, wantB, "from after edit")
	wantLegCategory(t, afterEdit.ToTransaction, wantB, "to after edit")

	// Undo restores category A on both legs.
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	afterUndo, err := env.txnSvc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("GetTransferPair after undo: %v", err)
	}
	wantA := types.NullableID{ID: catA.ID, Valid: true}
	wantLegCategory(t, afterUndo.FromTransaction, wantA, "from after undo")
	wantLegCategory(t, afterUndo.ToTransaction, wantA, "to after undo")
}
