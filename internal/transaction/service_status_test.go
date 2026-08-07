package transaction

import (
	"testing"

	accountpkg "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Balance Calculation Tests (void excluded, cleared_balance = cleared+reconciled)
//
// These tests verify balance calculations through the AccountService layer
// with transactions in all four statuses: uncleared, cleared, reconciled, void.
// =============================================================================

func TestBalanceCalculation_VoidExcludedFromCurrentBalance(t *testing.T) {
	t.Run("void transaction excluded from current balance", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Add a -100 transaction (will stay)
		txn1 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Add a -200 transaction (will be voided)
		txn2 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify balance before void: 1000 - 100 - 200 = 700
		balBefore, err := accountSvc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}
		if !balBefore.CurrentBalance.Equal(types.MustNewMoney("700.00")) {
			t.Errorf("Expected balance before void = 700, got %s", balBefore.CurrentBalance.String())
		}

		// Void txn2
		if err := txnSvc.VoidTransaction(txn2.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Verify balance after void: 1000 - 100 = 900 (void excluded)
		balAfter, err := accountSvc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}
		if !balAfter.CurrentBalance.Equal(types.MustNewMoney("900.00")) {
			t.Errorf("Expected balance after void = 900, got %s", balAfter.CurrentBalance.String())
		}
	})
}

func TestBalanceCalculation_ClearedBalanceIncludesOnlyClearedAndReconciled(t *testing.T) {
	t.Run("cleared balance includes only cleared and reconciled transactions", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// txn1: -100, uncleared (NOT in cleared balance)
		txn1 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn1); err != nil {
			t.Fatalf("Create txn1 error = %v", err)
		}

		// txn2: -200, cleared (IN cleared balance)
		txn2 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-200.00"))
		if err := txnSvc.Create(txn2); err != nil {
			t.Fatalf("Create txn2 error = %v", err)
		}
		if err := txnSvc.ClearTransaction(txn2.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		// txn3: -300, reconciled (IN cleared balance)
		txn3 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-300.00"))
		if err := txnSvc.Create(txn3); err != nil {
			t.Fatalf("Create txn3 error = %v", err)
		}
		if err := txnRepo.UpdateStatus(txn3.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
		}

		// txn4: -400, void (NOT in any balance)
		txn4 := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-400.00"))
		if err := txnSvc.Create(txn4); err != nil {
			t.Fatalf("Create txn4 error = %v", err)
		}
		if err := txnSvc.VoidTransaction(txn4.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		balance, err := accountSvc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}

		// current_balance = 1000 - 100 - 200 - 300 = 400 (void excluded)
		expectedCurrent := types.MustNewMoney("400.00")
		if !balance.CurrentBalance.Equal(expectedCurrent) {
			t.Errorf("Expected current_balance = %s, got %s",
				expectedCurrent.String(), balance.CurrentBalance.String())
		}

		// cleared_balance = 1000 - 200 - 300 = 500 (only cleared + reconciled)
		expectedCleared := types.MustNewMoney("500.00")
		if !balance.ClearedBalance.Equal(expectedCleared) {
			t.Errorf("Expected cleared_balance = %s, got %s",
				expectedCleared.String(), balance.ClearedBalance.String())
		}
	})
}

func TestBalanceCalculation_GetAllBalancesExcludesVoid(t *testing.T) {
	t.Run("GetAllBalances excludes void transactions across all accounts", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("500.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Add and void a transaction
		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-100.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := txnSvc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		balances, err := accountSvc.GetAllBalances()
		if err != nil {
			t.Fatalf("GetAllBalances() error = %v", err)
		}

		bal, ok := balances[account.ID]
		if !ok {
			t.Fatal("Account not found in balances")
		}

		// Should be opening balance since void is excluded
		if !bal.CurrentBalance.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected balance 500, got %s", bal.CurrentBalance.String())
		}
	})
}

func TestBalanceCalculation_ReconciledInClearedBalance(t *testing.T) {
	t.Run("reconciled transactions count toward cleared balance", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Only reconciled transaction: -150
		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-150.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
		}

		balance, _ := accountSvc.GetBalance(account.ID)

		// Both balances should include the reconciled transaction
		// current = 1000 - 150 = 850, cleared = 1000 - 150 = 850
		if !balance.CurrentBalance.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Expected current_balance 850, got %s", balance.CurrentBalance.String())
		}
		if !balance.ClearedBalance.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Expected cleared_balance 850, got %s", balance.ClearedBalance.String())
		}
	})
}

func TestBalanceCalculation_UnReconciledRemainsInClearedBalance(t *testing.T) {
	t.Run("un-reconciled transaction stays in cleared balance as cleared", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-250.00"))
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Reconcile then un-reconcile -> cleared, written the way production writes
		// it: reconciliation.Service marks reconciled (reconciliation_service.go:282)
		// and UndoFinish restores the prior status (:391), both through UpdateStatus.
		if err := txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
		}
		if err := txnRepo.UpdateStatus(txn.ID, StatusCleared); err != nil {
			t.Fatalf("UpdateStatus(cleared) error = %v", err)
		}

		balance, _ := accountSvc.GetBalance(account.ID)

		// current = 1000 - 250 = 750
		if !balance.CurrentBalance.Equal(types.MustNewMoney("750.00")) {
			t.Errorf("Expected current_balance 750, got %s", balance.CurrentBalance.String())
		}

		// cleared_balance should still include this (now cleared) transaction
		// cleared = 1000 - 250 = 750
		if !balance.ClearedBalance.Equal(types.MustNewMoney("750.00")) {
			t.Errorf("Expected cleared_balance 750, got %s", balance.ClearedBalance.String())
		}
	})
}

// =============================================================================
// Status Lifecycle Integration Tests
//
// These test complete status transition flows through the service layer,
// verifying the full state machine works end-to-end.
// =============================================================================

func TestStatusLifecycle_UnclearedToClearedToVoid(t *testing.T) {
	t.Run("uncleared -> cleared -> void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-50.00"))
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// 1. Starts uncleared
		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusUncleared {
			t.Errorf("Step 1: expected uncleared, got %s", retrieved.Status)
		}

		// 2. Clear
		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}
		retrieved, _ = svc.GetByID(txn.ID)
		if retrieved.Status != StatusCleared {
			t.Errorf("Step 2: expected cleared, got %s", retrieved.Status)
		}

		// 3. Void from cleared state
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}
		retrieved, _ = svc.GetByID(txn.ID)
		if retrieved.Status != StatusVoid {
			t.Errorf("Step 3: expected void, got %s", retrieved.Status)
		}
		if !retrieved.Amount.IsZero() {
			t.Errorf("Step 3: expected amount 0, got %s", retrieved.Amount.String())
		}
	})
}

func TestStatusLifecycle_VoidIsTerminal(t *testing.T) {
	t.Run("void is a terminal state - all status operations fail", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-50.00"))
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// All status transitions should fail
		if err := svc.ClearTransaction(txn.ID); err == nil {
			t.Error("ClearTransaction on void should fail")
		}
		if err := svc.MarkTransactionUncleared(txn.ID); err == nil {
			t.Error("MarkTransactionUncleared on void should fail")
		}
		if err := svc.VoidTransaction(txn.ID); err == nil {
			t.Error("VoidTransaction on void should fail")
		}

		// Edit and delete should also fail
		retrieved, _ := svc.GetByID(txn.ID)
		retrieved.Amount = types.MustNewMoney("-99.00")
		if err := svc.Update(retrieved); err == nil {
			t.Error("Update on void should fail")
		}
		if err := svc.Delete(txn.ID); err == nil {
			t.Error("Delete on void should fail")
		}
	})
}

func TestStatusLifecycle_ReconciledIsLocked(t *testing.T) {
	t.Run("reconciled is locked - all modification operations fail", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account := createTestAccount(t, accountRepo, "Checking")

		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-50.00"))
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		// Reconciled is reached the way production reaches it: reconciliation.Service
		// writes the status column directly (reconciliation_service.go:282).
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
		}

		// Status transitions should fail
		if err := svc.ClearTransaction(txn.ID); err == nil {
			t.Error("ClearTransaction on reconciled should fail")
		}
		if err := svc.MarkTransactionUncleared(txn.ID); err == nil {
			t.Error("MarkTransactionUncleared on reconciled should fail")
		}
		if err := svc.VoidTransaction(txn.ID); err == nil {
			t.Error("VoidTransaction on reconciled should fail")
		}

		// Edit and delete should fail
		retrieved, _ := svc.GetByID(txn.ID)
		retrieved.Amount = types.MustNewMoney("-99.00")
		if err := svc.Update(retrieved); err == nil {
			t.Error("Update on reconciled should fail")
		}
		if err := svc.Delete(txn.ID); err == nil {
			t.Error("Delete on reconciled should fail")
		}
	})
}

func TestStatusLifecycle_SplitVoidWithBalanceImpact(t *testing.T) {
	t.Run("voiding a split transaction correctly affects balance", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Transport", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create split transaction: -150 total = -100 food + -50 transport
		txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney("-150.00"))
		splits := []*Split{
			NewSplit(txn.ID, cat1.ID, types.MustNewMoney("-100.00")),
			NewSplit(txn.ID, cat2.ID, types.MustNewMoney("-50.00")),
		}
		if err := txnSvc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Balance before: 1000 - 150 = 850
		balBefore, _ := accountSvc.GetBalance(account.ID)
		if !balBefore.CurrentBalance.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Expected balance 850 before void, got %s", balBefore.CurrentBalance.String())
		}

		// Void the split transaction
		if err := txnSvc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Balance after: 1000 (void excluded, splits removed)
		balAfter, _ := accountSvc.GetBalance(account.ID)
		if !balAfter.CurrentBalance.Equal(types.MustNewMoney("1000.00")) {
			t.Errorf("Expected balance 1000 after void, got %s", balAfter.CurrentBalance.String())
		}

		// Verify splits are removed
		remainingSplits, _ := txnSvc.GetSplits(txn.ID)
		if len(remainingSplits) != 0 {
			t.Errorf("Expected 0 splits after void, got %d", len(remainingSplits))
		}
	})
}

func TestStatusLifecycle_MultipleVoidsWithBalance(t *testing.T) {
	t.Run("multiple void transactions all excluded from balance", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		payeeRepo := payee.NewRepository(database)
		accountRepo := accountpkg.NewRepository(database)
		txnSvc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
		accountSvc := accountpkg.NewService(accountpkg.NewRepository(database), database)

		account := accountpkg.NewAccount("Checking", accountpkg.TypeChecking, "USD",
			types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
		if err := accountSvc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create 3 transactions and void them all
		amounts := []string{"-100.00", "-200.00", "-300.00"}
		var txnIDs []types.ID
		for _, amt := range amounts {
			txn := NewTransaction(account.ID, types.Today(), types.MustNewMoney(amt))
			if err := txnSvc.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			txnIDs = append(txnIDs, txn.ID)
		}

		// Balance before any voids: 1000 - 100 - 200 - 300 = 400
		bal, _ := accountSvc.GetBalance(account.ID)
		if !bal.CurrentBalance.Equal(types.MustNewMoney("400.00")) {
			t.Errorf("Expected 400 before voids, got %s", bal.CurrentBalance.String())
		}

		// Void each one and verify balance
		expectedBalances := []string{"500.00", "700.00", "1000.00"}
		for i, id := range txnIDs {
			if err := txnSvc.VoidTransaction(id); err != nil {
				t.Fatalf("VoidTransaction(%d) error = %v", i, err)
			}
			bal, _ := accountSvc.GetBalance(account.ID)
			expected := types.MustNewMoney(expectedBalances[i])
			if !bal.CurrentBalance.Equal(expected) {
				t.Errorf("After voiding %d txns: expected %s, got %s",
					i+1, expected.String(), bal.CurrentBalance.String())
			}
		}
	})
}
