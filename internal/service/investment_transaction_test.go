package service

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// =============================================================================
// Helpers
// =============================================================================

func createInvestmentTestService(t *testing.T) (*InvestmentTransactionService, *repository.AccountRepository) {
	t.Helper()
	database := createTestDB(t)
	invRepo := repository.NewInvestmentTransactionRepository(database)
	accountRepo := repository.NewAccountRepository(database)
	svc := NewInvestmentTransactionService(invRepo, accountRepo, database)
	return svc, accountRepo
}

func createInvestmentAccount(t *testing.T, repo *repository.AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(name, models.AccountTypeInvestment, "USD", models.ZeroMoney, models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create investment account: %v", err)
	}
	return account
}

func createCheckingAccount(t *testing.T, repo *repository.AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(name, models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	return account
}

// =============================================================================
// SM-070: Registry
// =============================================================================

func TestNewServices_InvestmentTransactionService(t *testing.T) {
	t.Run("returns non-nil InvestmentTransactionService", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewServices(database)

		if svc.InvestmentTransaction == nil {
			t.Error("InvestmentTransaction service should not be nil")
		}
	})

	t.Run("returns non-nil InvestmentTransactionRepo", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewServices(database)

		if svc.InvestmentTransactionRepo == nil {
			t.Error("InvestmentTransactionRepo should not be nil")
		}
	})
}

// =============================================================================
// SM-066: Deposit
// =============================================================================

func TestInvestmentTransactionService_Deposit(t *testing.T) {
	t.Run("deposit increases cash position", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		txn, err := svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		if txn == nil {
			t.Fatal("Deposit() returned nil transaction")
		}
		if txn.Type != models.InvestmentTransactionTypeDeposit {
			t.Errorf("Expected type deposit, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "1000" {
			t.Errorf("Expected total amount '1000', got %q", txn.TotalAmount.String())
		}

		// Verify cash balance increased
		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "1000" {
			t.Errorf("Expected cash balance '1000', got %q", balance.String())
		}
	})

	t.Run("deposit creates transaction record", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		txn, err := svc.Deposit(account.ID, date, models.MustNewMoney("500.00"), "Initial deposit")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		if txn.Memo.String != "Initial deposit" {
			t.Errorf("Expected memo 'Initial deposit', got %q", txn.Memo.String)
		}
		if txn.Status != models.InvestmentTransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
	})

	t.Run("deposit rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createCheckingAccount(t, accountRepo, "Checking")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for non-investment account")
		}
		if _, ok := err.(*AccountNotInvestmentError); !ok {
			t.Errorf("Expected AccountNotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("deposit rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(account.ID, date, models.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for zero amount")
		}

		_, err = svc.Deposit(account.ID, date, models.MustNewMoney("-100.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for negative amount")
		}
	})

	t.Run("multiple deposits accumulate", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("First Deposit() error = %v", err)
		}

		_, err = svc.Deposit(account.ID, date, models.MustNewMoney("500.00"), "")
		if err != nil {
			t.Fatalf("Second Deposit() error = %v", err)
		}

		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "1500" {
			t.Errorf("Expected cash balance '1500', got %q", balance.String())
		}
	})
}

// =============================================================================
// SM-067: Withdrawal
// =============================================================================

func TestInvestmentTransactionService_Withdrawal(t *testing.T) {
	t.Run("withdrawal decreases cash position", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit first
		_, err := svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Withdraw
		txn, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("400.00"), "")
		if err != nil {
			t.Fatalf("Withdrawal() error = %v", err)
		}

		if txn.Type != models.InvestmentTransactionTypeWithdrawal {
			t.Errorf("Expected type withdrawal, got %s", txn.Type)
		}

		// Verify balance
		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "600" {
			t.Errorf("Expected cash balance '600', got %q", balance.String())
		}
	})

	t.Run("withdrawal creates transaction record", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")

		txn, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("200.00"), "Withdraw for bills")
		if err != nil {
			t.Fatalf("Withdrawal() error = %v", err)
		}

		if txn.Memo.String != "Withdraw for bills" {
			t.Errorf("Expected memo 'Withdraw for bills', got %q", txn.Memo.String)
		}
		// Withdrawal is stored as negative
		if !txn.TotalAmount.IsNegative() {
			t.Errorf("Expected negative total amount, got %s", txn.TotalAmount.String())
		}
	})

	t.Run("withdrawal rejects insufficient cash", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit only 100
		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("100.00"), "")

		// Try to withdraw 200
		_, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("200.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects zero cash balance", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("1.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for zero balance")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createCheckingAccount(t, accountRepo, "Checking")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for non-investment account")
		}
		if _, ok := err.(*AccountNotInvestmentError); !ok {
			t.Errorf("Expected AccountNotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(account.ID, date, models.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for zero amount")
		}
	})
}

// =============================================================================
// SM-068: Interest
// =============================================================================

func TestInvestmentTransactionService_Interest(t *testing.T) {
	t.Run("interest increases cash position", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit first
		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")

		txn, err := svc.Interest(account.ID, date, models.MustNewMoney("25.50"), "")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}

		if txn.Type != models.InvestmentTransactionTypeInterest {
			t.Errorf("Expected type interest, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "25.5" {
			t.Errorf("Expected total amount '25.5', got %q", txn.TotalAmount.String())
		}

		// Verify balance
		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "1025.5" {
			t.Errorf("Expected cash balance '1025.5', got %q", balance.String())
		}
	})

	t.Run("interest creates transaction with correct type", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.June, 30)

		txn, err := svc.Interest(account.ID, date, models.MustNewMoney("12.34"), "Q2 interest")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}

		if txn.Type != models.InvestmentTransactionTypeInterest {
			t.Errorf("Expected type interest, got %s", txn.Type)
		}
		if txn.Memo.String != "Q2 interest" {
			t.Errorf("Expected memo 'Q2 interest', got %q", txn.Memo.String)
		}
		if txn.Status != models.InvestmentTransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
	})

	t.Run("interest rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createCheckingAccount(t, accountRepo, "Checking")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Interest(account.ID, date, models.MustNewMoney("10.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for non-investment account")
		}
		if _, ok := err.(*AccountNotInvestmentError); !ok {
			t.Errorf("Expected AccountNotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("interest rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Interest(account.ID, date, models.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for zero amount")
		}

		_, err = svc.Interest(account.ID, date, models.MustNewMoney("-5.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for negative amount")
		}
	})

	t.Run("interest works on zero balance account", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Interest on zero balance should work (e.g., margin interest credit)
		txn, err := svc.Interest(account.ID, date, models.MustNewMoney("5.00"), "")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}
		if txn == nil {
			t.Fatal("Interest() returned nil transaction")
		}

		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "5" {
			t.Errorf("Expected cash balance '5', got %q", balance.String())
		}
	})
}

// =============================================================================
// SM-069: Fee
// =============================================================================

func TestInvestmentTransactionService_Fee(t *testing.T) {
	t.Run("fee decreases cash position", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit first
		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")

		txn, err := svc.Fee(account.ID, date, models.MustNewMoney("9.95"), "")
		if err != nil {
			t.Fatalf("Fee() error = %v", err)
		}

		if txn.Type != models.InvestmentTransactionTypeFee {
			t.Errorf("Expected type fee, got %s", txn.Type)
		}

		// Verify balance
		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "990.05" {
			t.Errorf("Expected cash balance '990.05', got %q", balance.String())
		}
	})

	t.Run("fee stores memo", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")

		txn, err := svc.Fee(account.ID, date, models.MustNewMoney("25.00"), "Annual account fee")
		if err != nil {
			t.Fatalf("Fee() error = %v", err)
		}

		if txn.Memo.String != "Annual account fee" {
			t.Errorf("Expected memo 'Annual account fee', got %q", txn.Memo.String)
		}
		// Fee is stored as negative
		if !txn.TotalAmount.IsNegative() {
			t.Errorf("Expected negative total amount, got %s", txn.TotalAmount.String())
		}
	})

	t.Run("fee rejects insufficient cash", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit only 10
		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("10.00"), "")

		// Try fee of 50
		_, err := svc.Fee(account.ID, date, models.MustNewMoney("50.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects zero cash balance", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Fee(account.ID, date, models.MustNewMoney("1.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for zero balance")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createCheckingAccount(t, accountRepo, "Checking")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Fee(account.ID, date, models.MustNewMoney("10.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for non-investment account")
		}
		if _, ok := err.(*AccountNotInvestmentError); !ok {
			t.Errorf("Expected AccountNotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		_, err := svc.Fee(account.ID, date, models.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for zero amount")
		}
	})
}

// =============================================================================
// SM-070: GetCashBalance
// =============================================================================

func TestInvestmentTransactionService_GetCashBalance(t *testing.T) {
	t.Run("empty account has zero balance", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")

		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero balance, got %q", balance.String())
		}
	})

	t.Run("computes balance from mixed transactions", func(t *testing.T) {
		svc, accountRepo := createInvestmentTestService(t)
		account := createInvestmentAccount(t, accountRepo, "Brokerage")
		date := models.NewDate(2024, time.March, 15)

		// Deposit 1000
		_, _ = svc.Deposit(account.ID, date, models.MustNewMoney("1000.00"), "")
		// Interest 25
		_, _ = svc.Interest(account.ID, date, models.MustNewMoney("25.00"), "")
		// Fee 10
		_, _ = svc.Fee(account.ID, date, models.MustNewMoney("10.00"), "")
		// Withdrawal 200
		_, _ = svc.Withdrawal(account.ID, date, models.MustNewMoney("200.00"), "")

		// Expected: 1000 + 25 - 10 - 200 = 815
		balance, err := svc.GetCashBalance(account.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "815" {
			t.Errorf("Expected cash balance '815', got %q", balance.String())
		}
	})
}
