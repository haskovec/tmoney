package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

type reconTestEnv struct {
	reconSvc    *reconciliation.Service
	txnSvc      *transaction.Service
	accountRepo *account.Repository
}

func createReconTestEnv(t *testing.T) *reconTestEnv {
	t.Helper()
	database := dbtest.New(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	reconRepo := reconciliation.NewRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	reconSvc := reconciliation.NewService(reconRepo, txnRepo, accountRepo, database)

	return &reconTestEnv{
		reconSvc:    reconSvc,
		txnSvc:      txnSvc,
		accountRepo: accountRepo,
	}
}

func createReconTestAccount(t *testing.T, repo *account.Repository, name string, balance string) *account.Account {
	t.Helper()
	bal := types.MustNewMoney(balance)
	acct := account.NewAccount(name, account.TypeChecking, "USD", bal, types.NewDate(2024, 1, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

// =============================================================================
// FinishReconciliationCommand Tests
// =============================================================================

func TestFinishReconciliationCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("reconciles transactions and undoes them", func(t *testing.T) {
		env := createReconTestEnv(t)
		acct := createReconTestAccount(t, env.accountRepo, "Checking", "1000.00")

		// Create two transactions
		txn1 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 5), types.MustNewMoney("-100.00"))
		if err := env.txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		txn2 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := env.txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		// Start reconciliation: 1000 - 100 - 200 = 700
		_, err := env.reconSvc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		cmd := undo.NewFinishReconciliationCommand(
			env.reconSvc, env.txnSvc, acct.ID, []types.ID{txn1.ID, txn2.ID},
		)

		// Execute: both transactions should be reconciled
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		updated1, _ := env.txnSvc.GetByID(txn1.ID)
		updated2, _ := env.txnSvc.GetByID(txn2.ID)
		if !updated1.IsReconciled() {
			t.Error("txn1 should be reconciled after Execute")
		}
		if !updated2.IsReconciled() {
			t.Error("txn2 should be reconciled after Execute")
		}

		// Verify session is completed
		active, _ := env.reconSvc.GetActiveSession(acct.ID)
		if active != nil {
			t.Error("Session should be completed after Execute")
		}

		// Undo: both transactions should revert to uncleared
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored1, _ := env.txnSvc.GetByID(txn1.ID)
		restored2, _ := env.txnSvc.GetByID(txn2.ID)
		if restored1.Status != transaction.StatusUncleared {
			t.Errorf("txn1 status = %s, want uncleared after Undo", restored1.Status)
		}
		if restored2.Status != transaction.StatusUncleared {
			t.Errorf("txn2 status = %s, want uncleared after Undo", restored2.Status)
		}

		// Session should be active again
		active, _ = env.reconSvc.GetActiveSession(acct.ID)
		if active == nil {
			t.Fatal("Session should be in_progress after Undo")
		}
		if !active.IsInProgress() {
			t.Error("Session should be in_progress after Undo")
		}
	})

	t.Run("preserves original cleared status on undo", func(t *testing.T) {
		env := createReconTestEnv(t)
		acct := createReconTestAccount(t, env.accountRepo, "Checking", "1000.00")

		// Create one uncleared and one cleared transaction
		txnUncleared := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 5), types.MustNewMoney("-100.00"))
		if err := env.txnSvc.Create(txnUncleared); err != nil {
			t.Fatalf("Create txnUncleared: %v", err)
		}

		txnCleared := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := env.txnSvc.Create(txnCleared); err != nil {
			t.Fatalf("Create txnCleared: %v", err)
		}
		if err := env.txnSvc.ClearTransaction(txnCleared.ID); err != nil {
			t.Fatalf("Clear txnCleared: %v", err)
		}

		// Start reconciliation: 1000 - 100 - 200 = 700
		_, err := env.reconSvc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		cmd := undo.NewFinishReconciliationCommand(
			env.reconSvc, env.txnSvc, acct.ID, []types.ID{txnUncleared.ID, txnCleared.ID},
		)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		// Both should now be reconciled
		r1, _ := env.txnSvc.GetByID(txnUncleared.ID)
		r2, _ := env.txnSvc.GetByID(txnCleared.ID)
		if !r1.IsReconciled() || !r2.IsReconciled() {
			t.Fatal("Both transactions should be reconciled after Execute")
		}

		// Undo
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// Verify original statuses are preserved
		restored1, _ := env.txnSvc.GetByID(txnUncleared.ID)
		restored2, _ := env.txnSvc.GetByID(txnCleared.ID)

		if restored1.Status != transaction.StatusUncleared {
			t.Errorf("txnUncleared status = %s, want uncleared", restored1.Status)
		}
		if restored2.Status != transaction.StatusCleared {
			t.Errorf("txnCleared status = %s, want cleared", restored2.Status)
		}
	})

	t.Run("description includes transaction count", func(t *testing.T) {
		env := createReconTestEnv(t)
		acct := createReconTestAccount(t, env.accountRepo, "Checking", "1000.00")

		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := env.reconSvc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		cmd := undo.NewFinishReconciliationCommand(
			env.reconSvc, env.txnSvc, acct.ID, []types.ID{txn.ID},
		)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		desc := cmd.Description()
		expected := "Reconcile 1 transactions"
		if desc != expected {
			t.Errorf("Description() = %q, want %q", desc, expected)
		}
	})

	t.Run("redo works after undo", func(t *testing.T) {
		env := createReconTestEnv(t)
		acct := createReconTestAccount(t, env.accountRepo, "Checking", "1000.00")

		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := env.txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := env.reconSvc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		manager := undo.NewManager()
		cmd := undo.NewFinishReconciliationCommand(
			env.reconSvc, env.txnSvc, acct.ID, []types.ID{txn.ID},
		)

		// Execute via manager
		if err := manager.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		// Transaction should be reconciled
		updated, _ := env.txnSvc.GetByID(txn.ID)
		if !updated.IsReconciled() {
			t.Error("Transaction should be reconciled")
		}

		// Undo via manager
		_, err = manager.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}

		// Transaction should be uncleared
		restored, _ := env.txnSvc.GetByID(txn.ID)
		if restored.Status != transaction.StatusUncleared {
			t.Errorf("After Undo: status = %s, want uncleared", restored.Status)
		}

		// Redo via manager
		_, err = manager.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}

		// Transaction should be reconciled again
		redone, _ := env.txnSvc.GetByID(txn.ID)
		if !redone.IsReconciled() {
			t.Error("Transaction should be reconciled after Redo")
		}

		// Session should be completed again
		active, _ := env.reconSvc.GetActiveSession(acct.ID)
		if active != nil {
			t.Error("Session should be completed after Redo")
		}
	})

	t.Run("handles already reconciled transactions in list", func(t *testing.T) {
		env := createReconTestEnv(t)
		acct := createReconTestAccount(t, env.accountRepo, "Checking", "1000.00")

		// Pre-reconcile a transaction
		txnRecon := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 5), types.MustNewMoney("-100.00"))
		if err := env.txnSvc.Create(txnRecon); err != nil {
			t.Fatalf("Create txnRecon: %v", err)
		}
		if err := env.txnSvc.ReconcileTransaction(txnRecon.ID); err != nil {
			t.Fatalf("Reconcile txnRecon: %v", err)
		}

		// New transaction to reconcile
		txnNew := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := env.txnSvc.Create(txnNew); err != nil {
			t.Fatalf("Create txnNew: %v", err)
		}

		// Statement: 1000 - 100 - 200 = 700
		_, err := env.reconSvc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		cmd := undo.NewFinishReconciliationCommand(
			env.reconSvc, env.txnSvc, acct.ID, []types.ID{txnRecon.ID, txnNew.ID},
		)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		// Undo - should only affect the newly reconciled transaction
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// txnRecon should still be reconciled (was already reconciled before)
		r1, _ := env.txnSvc.GetByID(txnRecon.ID)
		if !r1.IsReconciled() {
			t.Error("Pre-reconciled transaction should remain reconciled after Undo")
		}

		// txnNew should be uncleared
		r2, _ := env.txnSvc.GetByID(txnNew.ID)
		if r2.Status != transaction.StatusUncleared {
			t.Errorf("New transaction status = %s, want uncleared after Undo", r2.Status)
		}
	})
}
