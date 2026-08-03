package transaction

import (
	"testing"

	accountpkg "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// TestRepositoryUpdate_SplitTransactionStatusChange is a regression test for
// the reconcile-finish failure on a split (multi-category) transaction.
//
// FinishReconciliation flips a transaction's status to "reconciled" and
// persists it via Repository.Update. When the transaction has child
// transaction_splits rows, the old DELETE+INSERT implementation could not
// delete the parent row — the transaction_splits.transaction_id foreign key
// blocked it — and the operation failed with
// "failed to delete for update: ... Violates foreign key constraint".
// Migration 026 (drop the blocking secondary indexes) plus the in-place
// UPDATE in Repository.Update fix it. This test reproduces the exact call
// FinishReconciliation makes.
func TestRepositoryUpdate_SplitTransactionStatusChange(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	payeeRepo := payee.NewRepository(database)
	accountRepo := accountpkg.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
	accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

	account := accountpkg.NewAccount("Wealthfront Checking", accountpkg.TypeChecking, "USD",
		types.MustNewMoney("0.00"), types.NewDate(2024, 1, 1))
	if err := accountSvc.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	cat1 := category.NewCategory("Salary", category.TypeIncome)
	cat2 := category.NewCategory("Bonus", category.TypeIncome)
	if err := categoryRepo.Create(cat1); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := categoryRepo.Create(cat2); err != nil {
		t.Fatalf("create category: %v", err)
	}

	// A split deposit, like the paycheck that surfaced the bug.
	txn := NewTransaction(account.ID, types.NewDate(2026, 5, 29), types.MustNewMoney("150.00"))
	splits := []*Split{
		NewSplit(txn.ID, cat1.ID, types.MustNewMoney("100.00")),
		NewSplit(txn.ID, cat2.ID, types.MustNewMoney("50.00")),
	}
	if err := txnSvc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Mimic reconcile-finish: load, flip status to reconciled, persist via the
	// repository (the exact sequence FinishReconciliation performs).
	loaded, err := txnRepo.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	loaded.Reconcile()
	if err := txnRepo.Update(loaded); err != nil {
		t.Fatalf("Update() on a split transaction must not trip the splits FK: %v", err)
	}

	// Status change persisted.
	reloaded, err := txnRepo.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("GetByID() after update error = %v", err)
	}
	if !reloaded.IsReconciled() {
		t.Errorf("status = %q, want reconciled", reloaded.Status.String())
	}

	// Splits are intact and unchanged.
	remaining, err := splitRepo.ListByTransaction(txn.ID)
	if err != nil {
		t.Fatalf("ListByTransaction() error = %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("splits after update = %d, want 2", len(remaining))
	}
}
