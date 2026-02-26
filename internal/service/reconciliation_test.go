package service

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func createTestReconciliationService(t *testing.T) (*ReconciliationService, *TransactionService, *repository.AccountRepository) {
	t.Helper()
	database := createTestDB(t)
	reconRepo := repository.NewReconciliationRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	accountRepo := repository.NewAccountRepository(database)

	reconSvc := NewReconciliationService(reconRepo, txnRepo, accountRepo, database)
	txnSvc := NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	return reconSvc, txnSvc, accountRepo
}

func createTestCheckingAccount(t *testing.T, repo *repository.AccountRepository, name string, openingBalance string) *models.Account {
	t.Helper()
	balance := models.MustNewMoney(openingBalance)
	account := models.NewAccount(name, models.AccountTypeChecking, "USD", balance, models.NewDate(2024, 1, 1))
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return account
}

// =============================================================================
// StartReconciliation Tests
// =============================================================================

func TestReconciliationService_StartReconciliation(t *testing.T) {
	t.Run("starts reconciliation for valid account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		session, err := svc.StartReconciliation(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("950.00"),
		)
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		if session == nil {
			t.Fatal("StartReconciliation() returned nil session")
		}
		if session.AccountID != account.ID {
			t.Errorf("AccountID = %s, want %s", session.AccountID, account.ID)
		}
		if !session.StatementBalance.Equal(models.MustNewMoney("950.00")) {
			t.Errorf("StatementBalance = %s, want 950.00", session.StatementBalance.String())
		}
		if !session.IsInProgress() {
			t.Error("Session should be in progress")
		}
	})

	t.Run("replaces existing in-progress session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Start first session
		session1, err := svc.StartReconciliation(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("950.00"),
		)
		if err != nil {
			t.Fatalf("First StartReconciliation() error = %v", err)
		}

		// Start second session (replaces first)
		session2, err := svc.StartReconciliation(
			account.ID,
			models.NewDate(2024, 2, 28),
			models.MustNewMoney("1100.00"),
		)
		if err != nil {
			t.Fatalf("Second StartReconciliation() error = %v", err)
		}

		// Verify first session is gone
		if session1.ID == session2.ID {
			t.Error("New session should have a different ID")
		}

		// Verify only one active session
		active, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active.ID != session2.ID {
			t.Error("Active session should be the second session")
		}
	})

	t.Run("rejects closed account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "0.00")

		// Close the account
		account.Close()
		if err := accountRepo.Update(account); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		_, err := svc.StartReconciliation(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("0.00"),
		)
		if err == nil {
			t.Fatal("StartReconciliation() should return error for closed account")
		}
		if _, ok := err.(*AccountIsClosedError); !ok {
			t.Errorf("Expected AccountIsClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects future statement date", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		futureDate := models.Today().AddDays(30)
		_, err := svc.StartReconciliation(
			account.ID,
			futureDate,
			models.MustNewMoney("1000.00"),
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
			models.NewID(),
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000.00"),
		)
		if err == nil {
			t.Fatal("StartReconciliation() should return error for non-existent account")
		}
	})
}

// =============================================================================
// GetCandidateTransactions Tests
// =============================================================================

func TestReconciliationService_GetCandidateTransactions(t *testing.T) {
	t.Run("returns uncleared and cleared transactions on or before statement date", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create transactions with various dates and statuses
		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 20), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}
		if err := txnSvc.ClearTransaction(txn2.ID); err != nil {
			t.Fatalf("Clear txn2: %v", err)
		}

		// Transaction after statement date (should be excluded)
		txn3 := models.NewTransaction(account.ID, models.NewDate(2024, 2, 5), models.MustNewMoney("-75.00"))
		if err := txnSvc.Create(txn3); err != nil {
			t.Fatalf("Create txn3: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 2 {
			t.Fatalf("Expected 2 candidates, got %d", len(candidates))
		}
	})

	t.Run("excludes reconciled transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		// Reconcile txn1
		if err := txnSvc.ReconcileTransaction(txn1.ID); err != nil {
			t.Fatalf("Reconcile txn1: %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 1 {
			t.Fatalf("Expected 1 candidate (reconciled excluded), got %d", len(candidates))
		}
		if candidates[0].ID != txn2.ID {
			t.Error("Candidate should be the uncleared transaction")
		}
	})

	t.Run("excludes void transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		// Void txn1
		if err := txnSvc.VoidTransaction(txn1.ID); err != nil {
			t.Fatalf("Void txn1: %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 1 {
			t.Fatalf("Expected 1 candidate (void excluded), got %d", len(candidates))
		}
	})

	t.Run("returns empty list when no candidates", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 0 {
			t.Errorf("Expected 0 candidates, got %d", len(candidates))
		}
	})

	t.Run("orders by date ascending", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txnLater := models.NewTransaction(account.ID, models.NewDate(2024, 1, 20), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txnLater); err != nil {
			t.Fatalf("Create txnLater: %v", err)
		}

		txnEarlier := models.NewTransaction(account.ID, models.NewDate(2024, 1, 5), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txnEarlier); err != nil {
			t.Fatalf("Create txnEarlier: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 2 {
			t.Fatalf("Expected 2 candidates, got %d", len(candidates))
		}
		if candidates[0].ID != txnEarlier.ID {
			t.Error("First candidate should be the earlier transaction")
		}
		if candidates[1].ID != txnLater.ID {
			t.Error("Second candidate should be the later transaction")
		}
	})
}

// =============================================================================
// CalculateClearedTotal Tests
// =============================================================================

func TestReconciliationService_CalculateClearedTotal(t *testing.T) {
	t.Run("opening balance only when no transactions", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		total, err := svc.CalculateClearedTotal(account.ID, nil)
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		if !total.Equal(models.MustNewMoney("1000.00")) {
			t.Errorf("Cleared total = %s, want 1000.00", total.String())
		}
	})

	t.Run("includes reconciled transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := txnSvc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		total, err := svc.CalculateClearedTotal(account.ID, nil)
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		// 1000 + (-200) = 800
		if !total.Equal(models.MustNewMoney("800.00")) {
			t.Errorf("Cleared total = %s, want 800.00", total.String())
		}
	})

	t.Run("includes checked transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-150.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		total, err := svc.CalculateClearedTotal(account.ID, []models.ID{txn.ID})
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		// 1000 + (-150) = 850
		if !total.Equal(models.MustNewMoney("850.00")) {
			t.Errorf("Cleared total = %s, want 850.00", total.String())
		}
	})

	t.Run("combines opening balance, reconciled, and checked", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Reconciled: -200
		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 5), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}
		if err := txnSvc.ReconcileTransaction(txn1.ID); err != nil {
			t.Fatalf("Reconcile txn1: %v", err)
		}

		// Checked: -150
		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-150.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		// Unchecked (not passed in): -100
		txn3 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn3); err != nil {
			t.Fatalf("Create txn3: %v", err)
		}
		_ = txn3 // not checked

		total, err := svc.CalculateClearedTotal(account.ID, []models.ID{txn2.ID})
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		// 1000 + (-200) + (-150) = 650
		if !total.Equal(models.MustNewMoney("650.00")) {
			t.Errorf("Cleared total = %s, want 650.00", total.String())
		}
	})

	t.Run("does not include uncleared transactions not checked", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-300.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		total, err := svc.CalculateClearedTotal(account.ID, nil)
		if err != nil {
			t.Fatalf("CalculateClearedTotal() error = %v", err)
		}

		// Only opening balance since no reconciled and no checked
		if !total.Equal(models.MustNewMoney("1000.00")) {
			t.Errorf("Cleared total = %s, want 1000.00", total.String())
		}
	})
}

// =============================================================================
// FinishReconciliation Tests
// =============================================================================

func TestReconciliationService_FinishReconciliation(t *testing.T) {
	t.Run("completes when difference is zero", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create a transaction: -200
		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Start reconciliation: statement balance = 800 (1000 - 200)
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Finish with the transaction checked
		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false)
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
		active, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active != nil {
			t.Error("No active session should remain after finish")
		}
	})

	t.Run("rejects non-zero difference without force", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Start reconciliation: statement balance = 900 (but cleared total would be 800)
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("900.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Finish should fail because 900 - 800 = 100 difference
		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false)
		if err == nil {
			t.Fatal("FinishReconciliation() should fail with non-zero difference")
		}
		if _, ok := err.(*ReconciliationDifferenceError); !ok {
			t.Errorf("Expected ReconciliationDifferenceError, got %T: %v", err, err)
		}
	})

	t.Run("allows force with non-zero difference", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Start reconciliation: statement balance = 900 (difference = 100)
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("900.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Force finish
		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, true)
		if err != nil {
			t.Fatalf("FinishReconciliation(force=true) error = %v", err)
		}

		// Transaction should still be reconciled
		updated, err := txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !updated.IsReconciled() {
			t.Error("Transaction should be reconciled after forced finish")
		}
	})

	t.Run("fails with no active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		err := svc.FinishReconciliation(account.ID, nil, false)
		if err == nil {
			t.Fatal("FinishReconciliation() should fail with no active session")
		}
		if _, ok := err.(*NoActiveReconciliationError); !ok {
			t.Errorf("Expected NoActiveReconciliationError, got %T: %v", err, err)
		}
	})

	t.Run("skips already reconciled transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create and pre-reconcile a transaction
		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 5), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}
		if err := txnSvc.ReconcileTransaction(txn1.ID); err != nil {
			t.Fatalf("Reconcile txn1: %v", err)
		}

		// Create an uncleared transaction
		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		// Statement balance = 1000 - 100 - 200 = 700
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		// Pass both (including already reconciled) - should not error
		err = svc.FinishReconciliation(account.ID, []models.ID{txn1.ID, txn2.ID}, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}
	})

	t.Run("rejects void transactions in checked list", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := txnSvc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("Void: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, true)
		if err == nil {
			t.Fatal("FinishReconciliation() should reject void transactions")
		}
		if _, ok := err.(*TransactionIsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("reconciles multiple transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 5), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		txn3 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 15), models.MustNewMoney("500.00"))
		if err := txnSvc.Create(txn3); err != nil {
			t.Fatalf("Create txn3: %v", err)
		}

		// Statement = 1000 - 100 - 200 + 500 = 1200
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1200.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(account.ID, []models.ID{txn1.ID, txn2.ID, txn3.ID}, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}

		// All three should be reconciled
		for _, id := range []models.ID{txn1.ID, txn2.ID, txn3.ID} {
			updated, err := txnSvc.GetByID(id)
			if err != nil {
				t.Fatalf("GetByID(%s) error = %v", id, err)
			}
			if !updated.IsReconciled() {
				t.Errorf("Transaction %s should be reconciled", id)
			}
		}
	})
}

// =============================================================================
// CancelReconciliation Tests
// =============================================================================

func TestReconciliationService_CancelReconciliation(t *testing.T) {
	t.Run("cancels active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.CancelReconciliation(account.ID)
		if err != nil {
			t.Fatalf("CancelReconciliation() error = %v", err)
		}

		active, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active != nil {
			t.Error("No active session should remain after cancel")
		}
	})

	t.Run("does not modify transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("950.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.CancelReconciliation(account.ID)
		if err != nil {
			t.Fatalf("CancelReconciliation() error = %v", err)
		}

		// Transaction should still be uncleared
		updated, err := txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if updated.IsReconciled() {
			t.Error("Transaction should not be reconciled after cancel")
		}
	})

	t.Run("fails with no active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		err := svc.CancelReconciliation(account.ID)
		if err == nil {
			t.Fatal("CancelReconciliation() should fail with no active session")
		}
		if _, ok := err.(*NoActiveReconciliationError); !ok {
			t.Errorf("Expected NoActiveReconciliationError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// GetReconciliationStatus Tests
// =============================================================================

func TestReconciliationService_GetReconciliationStatus(t *testing.T) {
	t.Run("returns empty status for new account", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		status, err := svc.GetReconciliationStatus(account.ID)
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

	t.Run("returns active session with candidate count", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create some transactions
		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}
		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 20), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("850.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		status, err := svc.GetReconciliationStatus(account.ID)
		if err != nil {
			t.Fatalf("GetReconciliationStatus() error = %v", err)
		}

		if status.ActiveSession == nil {
			t.Fatal("ActiveSession should not be nil")
		}
		if status.CandidateCount != 2 {
			t.Errorf("CandidateCount = %d, want 2", status.CandidateCount)
		}
	})

	t.Run("returns last completed session", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Complete a reconciliation
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}
		if err := svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false); err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}

		status, err := svc.GetReconciliationStatus(account.ID)
		if err != nil {
			t.Fatalf("GetReconciliationStatus() error = %v", err)
		}

		if status.ActiveSession != nil {
			t.Error("ActiveSession should be nil after completion")
		}
		if status.LastCompletedSession == nil {
			t.Fatal("LastCompletedSession should not be nil")
		}
		if !status.LastCompletedSession.IsCompleted() {
			t.Error("Last completed session should have completed status")
		}
	})
}

// =============================================================================
// GetActiveSession / GetLastCompletedSession Tests
// =============================================================================

func TestReconciliationService_GetActiveSession(t *testing.T) {
	t.Run("returns nil when no active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		session, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if session != nil {
			t.Error("Expected nil session")
		}
	})

	t.Run("returns active session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		created, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		session, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if session == nil {
			t.Fatal("Expected non-nil session")
		}
		if session.ID != created.ID {
			t.Error("Session ID mismatch")
		}
	})
}

func TestReconciliationService_GetLastCompletedSession(t *testing.T) {
	t.Run("returns nil when no completed sessions", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		session, err := svc.GetLastCompletedSession(account.ID)
		if err != nil {
			t.Fatalf("GetLastCompletedSession() error = %v", err)
		}
		if session != nil {
			t.Error("Expected nil session")
		}
	})

	t.Run("returns most recent completed session", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// First reconciliation
		txn1 := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("900.00"))
		if err != nil {
			t.Fatalf("StartReconciliation #1 error = %v", err)
		}
		if err := svc.FinishReconciliation(account.ID, []models.ID{txn1.ID}, false); err != nil {
			t.Fatalf("FinishReconciliation #1 error = %v", err)
		}

		// Second reconciliation
		txn2 := models.NewTransaction(account.ID, models.NewDate(2024, 2, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2: %v", err)
		}

		_, err = svc.StartReconciliation(account.ID, models.NewDate(2024, 2, 28), models.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation #2 error = %v", err)
		}
		if err := svc.FinishReconciliation(account.ID, []models.ID{txn2.ID}, false); err != nil {
			t.Fatalf("FinishReconciliation #2 error = %v", err)
		}

		session, err := svc.GetLastCompletedSession(account.ID)
		if err != nil {
			t.Fatalf("GetLastCompletedSession() error = %v", err)
		}
		if session == nil {
			t.Fatal("Expected non-nil session")
		}
		// Should be the second reconciliation (most recent)
		if !session.StatementBalance.Equal(models.MustNewMoney("700.00")) {
			t.Errorf("StatementBalance = %s, want 700.00", session.StatementBalance.String())
		}
	})
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestReconciliationService_EdgeCases(t *testing.T) {
	t.Run("reconciliation with zero opening balance", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "0.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("500.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("500.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}
	})

	t.Run("reconciliation with negative balance (credit card)", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)

		balance := models.MustNewMoney("0.00")
		account := models.NewAccount("Visa", models.AccountTypeCreditCard, "USD", balance, models.NewDate(2024, 1, 1))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account: %v", err)
		}

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-500.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("-500.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}
	})

	t.Run("finish with empty transaction list", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Statement balance equals opening balance (no transactions to check)
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.FinishReconciliation(account.ID, nil, false)
		if err != nil {
			t.Fatalf("FinishReconciliation() with empty list error = %v", err)
		}
	})

	t.Run("statement date on same day as transaction is included", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		candidates, err := svc.GetCandidateTransactions(account.ID, models.NewDate(2024, 1, 31))
		if err != nil {
			t.Fatalf("GetCandidateTransactions() error = %v", err)
		}

		if len(candidates) != 1 {
			t.Errorf("Expected 1 candidate (same date included), got %d", len(candidates))
		}
	})
}

// =============================================================================
// ReopenSession Tests
// =============================================================================

func TestReconciliationService_ReopenSession(t *testing.T) {
	t.Run("reopens completed session", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		session, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("800.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		if err := svc.FinishReconciliation(account.ID, []models.ID{txn.ID}, false); err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}

		// Reopen the session
		if err := svc.ReopenSession(session.ID); err != nil {
			t.Fatalf("ReopenSession() error = %v", err)
		}

		// Session should now be active again
		active, err := svc.GetActiveSession(account.ID)
		if err != nil {
			t.Fatalf("GetActiveSession() error = %v", err)
		}
		if active == nil {
			t.Fatal("Expected active session after reopen")
		}
		if active.ID != session.ID {
			t.Error("Active session should be the reopened session")
		}
		if !active.IsInProgress() {
			t.Error("Session should be in_progress after reopen")
		}
		if active.CompletedAt.Valid {
			t.Error("CompletedAt should be cleared after reopen")
		}
	})

	t.Run("rejects non-completed session", func(t *testing.T) {
		svc, _, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		session, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("1000.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}

		err = svc.ReopenSession(session.ID)
		if err == nil {
			t.Fatal("ReopenSession() should fail for in_progress session")
		}
	})

	t.Run("rejects non-existent session", func(t *testing.T) {
		svc, _, _ := createTestReconciliationService(t)

		err := svc.ReopenSession(models.NewID())
		if err == nil {
			t.Fatal("ReopenSession() should fail for non-existent session")
		}
	})
}

// =============================================================================
// RestoreTransactionStatuses Tests
// =============================================================================

func TestReconciliationService_RestoreTransactionStatuses(t *testing.T) {
	t.Run("restores reconciled transactions to previous status", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		// Create an uncleared transaction and a cleared transaction
		txnUncleared := models.NewTransaction(account.ID, models.NewDate(2024, 1, 5), models.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txnUncleared); err != nil {
			t.Fatalf("Create txnUncleared: %v", err)
		}

		txnCleared := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txnCleared); err != nil {
			t.Fatalf("Create txnCleared: %v", err)
		}
		if err := txnSvc.ClearTransaction(txnCleared.ID); err != nil {
			t.Fatalf("Clear txnCleared: %v", err)
		}

		// Reconcile both
		_, err := svc.StartReconciliation(account.ID, models.NewDate(2024, 1, 31), models.MustNewMoney("700.00"))
		if err != nil {
			t.Fatalf("StartReconciliation() error = %v", err)
		}
		if err := svc.FinishReconciliation(account.ID, []models.ID{txnUncleared.ID, txnCleared.ID}, false); err != nil {
			t.Fatalf("FinishReconciliation() error = %v", err)
		}

		// Restore to previous statuses
		statuses := map[models.ID]models.TransactionStatus{
			txnUncleared.ID: models.TransactionStatusUncleared,
			txnCleared.ID:   models.TransactionStatusCleared,
		}
		if err := svc.RestoreTransactionStatuses(statuses); err != nil {
			t.Fatalf("RestoreTransactionStatuses() error = %v", err)
		}

		// Verify uncleared is restored
		updated1, err := txnSvc.GetByID(txnUncleared.ID)
		if err != nil {
			t.Fatalf("GetByID txnUncleared: %v", err)
		}
		if updated1.Status != models.TransactionStatusUncleared {
			t.Errorf("txnUncleared status = %s, want uncleared", updated1.Status)
		}

		// Verify cleared is restored
		updated2, err := txnSvc.GetByID(txnCleared.ID)
		if err != nil {
			t.Fatalf("GetByID txnCleared: %v", err)
		}
		if updated2.Status != models.TransactionStatusCleared {
			t.Errorf("txnCleared status = %s, want cleared", updated2.Status)
		}
	})

	t.Run("skips non-reconciled transactions", func(t *testing.T) {
		svc, txnSvc, accountRepo := createTestReconciliationService(t)
		account := createTestCheckingAccount(t, accountRepo, "Checking", "1000.00")

		txn := models.NewTransaction(account.ID, models.NewDate(2024, 1, 10), models.MustNewMoney("-50.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Try to restore an already-uncleared transaction (should be a no-op)
		statuses := map[models.ID]models.TransactionStatus{
			txn.ID: models.TransactionStatusUncleared,
		}
		if err := svc.RestoreTransactionStatuses(statuses); err != nil {
			t.Fatalf("RestoreTransactionStatuses() error = %v", err)
		}

		// Transaction should still be uncleared
		updated, err := txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if updated.Status != models.TransactionStatusUncleared {
			t.Errorf("Status = %s, want uncleared", updated.Status)
		}
	})

	t.Run("handles empty status map", func(t *testing.T) {
		svc, _, _ := createTestReconciliationService(t)

		err := svc.RestoreTransactionStatuses(map[models.ID]models.TransactionStatus{})
		if err != nil {
			t.Fatalf("RestoreTransactionStatuses() with empty map error = %v", err)
		}
	})
}
