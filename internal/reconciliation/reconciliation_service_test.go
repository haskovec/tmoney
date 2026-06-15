package reconciliation

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestReconciliationService(t *testing.T) (*Service, *transaction.Service, *account.Repository) {
	t.Helper()
	database := createTestDB(t)
	reconRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)

	reconSvc := NewService(reconRepo, txnRepo, accountRepo, database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	return reconSvc, txnSvc, accountRepo
}

func createTestCheckingAccount(t *testing.T, repo *account.Repository, name string, openingBalance string) *account.Account {
	t.Helper()
	balance := types.MustNewMoney(openingBalance)
	acct := account.NewAccount(name, account.TypeChecking, "USD", balance, types.NewDate(2024, 1, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

// =============================================================================
// StartReconciliation Tests
// =============================================================================

func TestService_StartReconciliation(t *testing.T) {
	t.Run("starts reconciliation for valid account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		session, err := svc.StartReconciliation(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("950.00"),
		)
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		if session == nil {
			t.Fatal("StartReconciliation() returned nil session")
		}
		if session.AccountID != acct.ID {
			t.Errorf("AccountID = %s, want %s", session.AccountID, acct.ID)
		}
		if !session.StatementBalance.Equal(types.MustNewMoney("950.00")) {
			t.Errorf("StatementBalance = %s, want 950.00", session.StatementBalance.String())
		}
		if !session.IsInProgress() {
			t.Error("Session should be in progress")
		}
	})

	t.Run("replaces existing in-progress session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Start first session
		session1, err := svc.StartReconciliation(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("950.00"),
		)
		if err != nil {
			t.Fatalf("First StartReconciliation() error = %v", err)
		}

		// Start second session (replaces first)
		session2, err := svc.StartReconciliation(
			acct.ID,
			types.NewDate(2024, 2, 28),
			types.MustNewMoney("1100.00"),
		)
		if err != nil {
			t.Fatalf("Second StartReconciliation() error = %v", err)
		}

		// Verify first session is gone
		if session1.ID == session2.ID {
			t.Error("New session should have a different ID")
		}

		// Verify only one active session
		active, err := svc.GetActiveSession(acct.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active.ID != session2.ID {
			t.Error("Active session should be the second session")
		}
	})

	t.Run("rejects closed account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "0.00")

		// Close the account
		acct.Close(types.Today())
		if err := accountRepo.Update(acct); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		_, err := svc.StartReconciliation(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("0.00"),
		)
		if err == nil {
			t.Fatal("StartReconciliation() should return error for closed account")
		}
		if _, ok := err.(*account.IsClosedError); !ok {
			t.Errorf("Expected IsClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects future statement date", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		futureDate := types.Today().AddDays(30)
		_, err := svc.StartReconciliation(
			acct.ID,
			futureDate,
			types.MustNewMoney("1000.00"),
		)
		if err == nil {
			t.Fatal("StartReconciliation() should return error for future date")
		}
		if _, ok := err.(*StatementDateFutureError); !ok {
			t.Errorf("Expected StatementDateFutureError, got %T: %v", err, err)
		}
	})

	t.Run("rejects non-existent account", func(t *testing.T) {
		svc, _, _ := createTestReconciliationService(t)

		_, err := svc.StartReconciliation(
			types.NewID(),
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("1000.00"),
		)
		if err == nil {
			t.Fatal("StartReconciliation() should return error for non-existent account")
		}
	})
}

// =============================================================================
// GetCandidateTransactions Tests
// =============================================================================

func TestService_GetCandidateTransactions(t *testing.T) {
	t.Run("returns uncleared and cleared transactions on or before statement date", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create transactions with various dates and statuses
		txn1 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		txn2 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 20), types.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}
		if err := txnSvc.ClearTransaction(txn2.ID); err != nil {
			t.Fatalf("Clear txn2: %v", err)
		}

		// Transaction after statement date (should be excluded)
		txn3 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 2, 5), types.MustNewMoney("-75.00"))
		if err := txnSvc.Create(txn3); err != nil {
			t.Fatalf("Create txn3: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(acct.ID, types.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 2 {
			t.Fatalf("Expected 2 candidates, got %d", len(candidates))
		}
	})

	t.Run("returns empty list when no candidates", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		candidates, err := svc.GetCandidateTransactions(acct.ID, types.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 0 {
			t.Errorf("Expected 0 candidates, got %d", len(candidates))
		}
	})
}

// =============================================================================
// CalculateClearedTotal Tests
// =============================================================================

func TestService_CalculateClearedTotal(t *testing.T) {
	t.Run("opening balance only when no transactions", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		total, err := svc.CalculateClearedTotal(acct.ID, nil)
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		if !total.Equal(types.MustNewMoney("1000.00")) {
			t.Errorf("Cleared total = %s, want 1000.00", total.String())
		}
	})

	t.Run("includes checked transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-150.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		total, err := svc.CalculateClearedTotal(acct.ID, []types.ID{txn.ID})
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		// 1000 + (-150) = 850
		if !total.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Cleared total = %s, want 850.00", total.String())
		}
	})
}

// =============================================================================
// FinishReconciliation Tests
// =============================================================================

func TestService_FinishReconciliation(t *testing.T) {
	t.Run("completes when difference is zero", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create a transaction: -200
		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Start reconciliation: statement balance = 800 (1000 - 200)
		_, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Finish with the transaction checked
		err = svc.FinishReconciliation(acct.ID, []types.ID{txn.ID}, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}

		// Verify transaction is now reconciled
		updated, err := txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !updated.IsReconciled() {
			t.Error("Transaction should be reconciled after finish")
		}

		// Verify session is completed
		active, err := svc.GetActiveSession(acct.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active != nil {
			t.Error("No active session should remain after finish")
		}
	})

	t.Run("rejects non-zero difference without force", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Start reconciliation: statement balance = 900 (but cleared total would be 800)
		_, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("900.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Finish should fail because 900 - 800 = 100 difference
		err = svc.FinishReconciliation(acct.ID, []types.ID{txn.ID}, false)
		if err == nil {
			t.Fatal("FinishReconciliation() should fail with non-zero difference")
		}
		if _, ok := err.(*DifferenceError); !ok {
			t.Errorf("Expected DifferenceError, got %T: %v", err, err)
		}
	})

	t.Run("fails with no active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		err := svc.FinishReconciliation(acct.ID, nil, false)
		if err == nil {
			t.Fatal("FinishReconciliation() should fail with no active session")
		}
		if _, ok := err.(*NoActiveError); !ok {
			t.Errorf("Expected NoActiveError, got %T: %v", err, err)
		}
	})

	t.Run("rejects void transactions in checked list", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := txnSvc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("Void: %v", err)
		}

		_, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(acct.ID, []types.ID{txn.ID}, true)
		if err == nil {
			t.Fatal("FinishReconciliation() should reject void transactions")
		}
		if _, ok := err.(*transaction.IsVoidError); !ok {
			t.Errorf("Expected IsVoidError, got %T: %v", err, err)
		}
	})
}

// TestService_FinishReconciliation_SplitTransaction is a regression test for
// the production failure where finishing a reconciliation that includes a
// split (multi-category) transaction — e.g. a split paycheck deposit — failed
// with "failed to reconcile transaction <id>: failed to delete for update:
// Constraint Error: Violates foreign key constraint". The transaction had
// child transaction_splits rows whose inbound FK blocked the parent's status
// update. Migration 026 (drop that FK) plus the in-place UPDATE in
// transaction.Repository.Update fix it.
func TestService_FinishReconciliation_SplitTransaction(t *testing.T) {
	database := createTestDB(t)
	reconRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(reconRepo, txnRepo, accountRepo, database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	acct := createTestCheckingAccount(t, accountRepo, "Wealthfront Checking", "1000.00")

	cat1 := category.NewCategory("Salary", category.TypeIncome)
	cat2 := category.NewCategory("Bonus", category.TypeIncome)
	if err := categoryRepo.Create(cat1); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := categoryRepo.Create(cat2); err != nil {
		t.Fatalf("create category: %v", err)
	}

	// A split deposit of +150 (100 + 50), like the paycheck that surfaced the bug.
	txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("150.00"))
	splits := []*transaction.Split{
		transaction.NewSplit(txn.ID, cat1.ID, types.MustNewMoney("100.00")),
		transaction.NewSplit(txn.ID, cat2.ID, types.MustNewMoney("50.00")),
	}
	if err := txnSvc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Statement balance = 1000 + 150 = 1150, so the difference is zero.
	if _, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("1150.00")); err != nil {
		t.Fatalf("StartReconciliation() error = %v", err)
	}

	if err := svc.FinishReconciliation(acct.ID, []types.ID{txn.ID}, false); err != nil {
		t.Fatalf("FinishReconciliation() must not fail on a split transaction: %v", err)
	}

	updated, err := txnSvc.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.IsReconciled() {
		t.Error("split transaction should be reconciled after finish")
	}

	remaining, err := txnSvc.GetSplits(txn.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("splits after reconcile = %d, want 2", len(remaining))
	}

	active, err := svc.GetActiveSession(acct.ID)
	if err != nil {
		t.Fatalf("GetActiveSession() error = %v", err)
	}
	if active != nil {
		t.Error("no active session should remain after finish")
	}
}

// =============================================================================
// CancelReconciliation Tests
// =============================================================================

func TestService_CancelReconciliation(t *testing.T) {
	t.Run("cancels active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		_, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.CancelReconciliation(acct.ID)
		if err != nil {
			t.Fatalf("CancelReconciliation() error = %v", err)
		}

		active, err := svc.GetActiveSession(acct.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active != nil {
			t.Error("No active session should remain after cancel")
		}
	})

	t.Run("fails with no active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		err := svc.CancelReconciliation(acct.ID)
		if err == nil {
			t.Fatal("CancelReconciliation() should fail with no active session")
		}
		if _, ok := err.(*NoActiveError); !ok {
			t.Errorf("Expected NoActiveError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// GetReconciliationStatus Tests
// =============================================================================

func TestService_GetReconciliationStatus(t *testing.T) {
	t.Run("returns empty status for new account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		acct := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		status, err := svc.GetReconciliationStatus(acct.ID)
		if err != nil {
			t.Fatalf("GetReconciliationStatus() error = %v", err)
		}

		if status.ActiveSession != nil {
			t.Error("ActiveSession should be nil for new account")
		}
		if status.LastCompletedSession != nil {
			t.Error("LastCompletedSession should be nil for new account")
		}
		if status.CandidateCount != 0 {
			t.Errorf("CandidateCount = %d, want 0", status.CandidateCount)
		}
	})
}

// =============================================================================
// RestoreTransactionStatuses Tests
// =============================================================================

func TestService_RestoreTransactionStatuses(t *testing.T) {
	t.Run("handles empty status map", func(t *testing.T) {
		svc, _, _ := createTestReconciliationService(t)

		err := svc.RestoreTransactionStatuses(map[types.ID]transaction.Status{})
		if err != nil {
			t.Fatalf("RestoreTransactionStatuses() with empty map error = %v", err)
		}
	})
}
