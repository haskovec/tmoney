package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
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

	cmd := undo.NewCreateTransferCommand(env.transferSvc, transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("100.00"),
		Memo:          "rent",
		CategoryID:    types.NullableID{ID: bills.ID, Valid: true},
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	res := cmd.Result()
	if res == nil {
		t.Fatalf("Result() nil after Execute")
	}
	want := types.NullableID{ID: bills.ID, Valid: true}

	// Both legs of a bank↔bank pair carry the category.
	for label, rowID := range map[string]types.ID{"from": res.From.RowID, "to": res.To.RowID} {
		leg, err := env.txnSvc.GetByID(rowID)
		if err != nil {
			t.Fatalf("GetByID(%s leg): %v", label, err)
		}
		wantLegCategory(t, leg, want, label)
	}

	// Undo deletes both legs.
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := env.txnSvc.GetByID(res.From.RowID); err == nil {
		t.Error("from leg should be gone after undo")
	}
	if _, err := env.txnSvc.GetByID(res.To.RowID); err == nil {
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
	created, err := env.transferSvc.Create(transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("100.00"),
		CategoryID:    types.NullableID{ID: catA.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	transferID := created.TransferID

	// Edit to category B. The command captures the pre-edit state from the
	// service's own Result.Before, inside the same transaction as the write.
	cmd := undo.NewEditTransferCommand(env.transferSvc, transferID, transfer.Edit{
		Date:       types.Today(),
		Amount:     types.MustNewMoney("100.00"),
		Status:     transaction.StatusUncleared,
		CategoryID: types.NullableID{ID: catB.ID, Valid: true},
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wantB := types.NullableID{ID: catB.ID, Valid: true}
	afterEdit, err := env.transferSvc.Get(transferID)
	if err != nil {
		t.Fatalf("Get after edit: %v", err)
	}
	if afterEdit.CategoryID != wantB {
		t.Errorf("transfer category after edit = %+v, want %+v", afterEdit.CategoryID, wantB)
	}
	for label, rowID := range map[string]types.ID{"from": created.From.RowID, "to": created.To.RowID} {
		leg, err := env.txnSvc.GetByID(rowID)
		if err != nil {
			t.Fatalf("GetByID(%s leg) after edit: %v", label, err)
		}
		wantLegCategory(t, leg, wantB, label+" after edit")
	}

	// Undo restores category A on both legs.
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	wantA := types.NullableID{ID: catA.ID, Valid: true}
	for label, rowID := range map[string]types.ID{"from": created.From.RowID, "to": created.To.RowID} {
		leg, err := env.txnSvc.GetByID(rowID)
		if err != nil {
			t.Fatalf("GetByID(%s leg) after undo: %v", label, err)
		}
		wantLegCategory(t, leg, wantA, label+" after undo")
	}
}

// TestEditTransferCommand_UndoRestoresClearedCategory pins the direction the old
// command could get wrong: clearing a category and then undoing must put it
// back, not leave it cleared.
func TestEditTransferCommand_UndoRestoresClearedCategory(t *testing.T) {
	env := createTestEnv(t)
	from := createTestAccount(t, env.accountRepo, "Checking")
	to := createTestAccount(t, env.accountRepo, "Savings")
	bills := newUndoCategory(t, env, "Bills")

	created, err := env.transferSvc.Create(transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("40.00"),
		CategoryID:    types.NullableID{ID: bills.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cmd := undo.NewEditTransferCommand(env.transferSvc, created.TransferID, transfer.Edit{
		Date:   types.Today(),
		Amount: types.MustNewMoney("40.00"),
		// CategoryID left zero: clear the label.
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	cleared, err := env.transferSvc.Get(created.TransferID)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if cleared.CategoryID.Valid {
		t.Fatalf("category should be cleared, got %s", cleared.CategoryID.ID)
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	restored, err := env.transferSvc.Get(created.TransferID)
	if err != nil {
		t.Fatalf("Get after undo: %v", err)
	}
	if !restored.CategoryID.Valid || restored.CategoryID.ID != bills.ID {
		t.Errorf("category after undo = %+v, want %s", restored.CategoryID, bills.ID)
	}
}
