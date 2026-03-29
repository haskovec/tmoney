package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Helpers
// =============================================================================

func createTestService(t *testing.T) (*Service, *account.Repository) {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo := account.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, database)
	return svc, accountRepo
}

func createInvAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create investment account: %v", err)
	}
	return acct
}

func createCheckAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	return acct
}

// testServiceEnv holds all repos/services needed for buy tests.
type testServiceEnv struct {
	svc          *Service
	accountRepo  *account.Repository
	secRepo      *security.Repository
	positionRepo *PositionRepository
	lotRepo      *LotRepository
}

func createFullTestService(t *testing.T) *testServiceEnv {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo := account.NewRepository(database)
	secRepo := security.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, database)
	return &testServiceEnv{
		svc:          svc,
		accountRepo:  accountRepo,
		secRepo:      secRepo,
		positionRepo: positionRepo,
		lotRepo:      lotRepo,
	}
}

func createLotTrackingAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = true
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create lot-tracking account: %v", err)
	}
	return acct
}

func createSec(t *testing.T, repo *security.Repository, ticker string) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, ticker+" Corp", security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("Failed to create security: %v", err)
	}
	return sec
}

// =============================================================================
// SM-066: Deposit
// =============================================================================

func TestService_Deposit(t *testing.T) {
	t.Run("deposit increases cash position", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		txn, err := svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		if txn == nil {
			t.Fatal("Deposit() returned nil transaction")
		}
		if txn.Type != TransactionTypeDeposit {
			t.Errorf("Expected type deposit, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "1000" {
			t.Errorf("Expected total amount '1000', got %q", txn.TotalAmount.String())
		}

		// Verify cash balance increased
		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "1000" {
			t.Errorf("Expected cash balance '1000', got %q", balance.String())
		}
	})

	t.Run("deposit creates transaction record", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		txn, err := svc.Deposit(acct.ID, date, types.MustNewMoney("500.00"), "Initial deposit")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		if txn.Memo.String != "Initial deposit" {
			t.Errorf("Expected memo 'Initial deposit', got %q", txn.Memo.String)
		}
		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
	})

	t.Run("deposit rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("deposit rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for zero amount")
		}

		_, err = svc.Deposit(acct.ID, date, types.MustNewMoney("-100.00"), "")
		if err == nil {
			t.Fatal("Deposit() expected error for negative amount")
		}
	})

	t.Run("multiple deposits accumulate", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("First Deposit() error = %v", err)
		}

		_, err = svc.Deposit(acct.ID, date, types.MustNewMoney("500.00"), "")
		if err != nil {
			t.Fatalf("Second Deposit() error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
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

func TestService_Withdrawal(t *testing.T) {
	t.Run("withdrawal decreases cash position", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit first
		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Withdraw
		txn, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("400.00"), "")
		if err != nil {
			t.Fatalf("Withdrawal() error = %v", err)
		}

		if txn.Type != TransactionTypeWithdrawal {
			t.Errorf("Expected type withdrawal, got %s", txn.Type)
		}

		// Verify balance
		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "600" {
			t.Errorf("Expected cash balance '600', got %q", balance.String())
		}
	})

	t.Run("withdrawal creates transaction record", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")

		txn, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("200.00"), "Withdraw for bills")
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
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit only 100
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		// Try to withdraw 200
		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("200.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects zero cash balance", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("1.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for zero balance")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Withdrawal() expected error for zero amount")
		}
	})
}

// =============================================================================
// SM-068: Interest
// =============================================================================

func TestService_Interest(t *testing.T) {
	t.Run("interest increases cash position", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit first
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")

		txn, err := svc.Interest(acct.ID, date, types.MustNewMoney("25.50"), "")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}

		if txn.Type != TransactionTypeInterest {
			t.Errorf("Expected type interest, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "25.5" {
			t.Errorf("Expected total amount '25.5', got %q", txn.TotalAmount.String())
		}

		// Verify balance
		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "1025.5" {
			t.Errorf("Expected cash balance '1025.5', got %q", balance.String())
		}
	})

	t.Run("interest creates transaction with correct type", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.June, 30)

		txn, err := svc.Interest(acct.ID, date, types.MustNewMoney("12.34"), "Q2 interest")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}

		if txn.Type != TransactionTypeInterest {
			t.Errorf("Expected type interest, got %s", txn.Type)
		}
		if txn.Memo.String != "Q2 interest" {
			t.Errorf("Expected memo 'Q2 interest', got %q", txn.Memo.String)
		}
		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
	})

	t.Run("interest rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Interest(acct.ID, date, types.MustNewMoney("10.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("interest rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Interest(acct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for zero amount")
		}

		_, err = svc.Interest(acct.ID, date, types.MustNewMoney("-5.00"), "")
		if err == nil {
			t.Fatal("Interest() expected error for negative amount")
		}
	})

	t.Run("interest works on zero balance account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Interest on zero balance should work (e.g., margin interest credit)
		txn, err := svc.Interest(acct.ID, date, types.MustNewMoney("5.00"), "")
		if err != nil {
			t.Fatalf("Interest() error = %v", err)
		}
		if txn == nil {
			t.Fatal("Interest() returned nil transaction")
		}

		balance, err := svc.GetCashBalance(acct.ID)
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

func TestService_Fee(t *testing.T) {
	t.Run("fee decreases cash position", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit first
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")

		txn, err := svc.Fee(acct.ID, date, types.MustNewMoney("9.95"), "")
		if err != nil {
			t.Fatalf("Fee() error = %v", err)
		}

		if txn.Type != TransactionTypeFee {
			t.Errorf("Expected type fee, got %s", txn.Type)
		}

		// Verify balance
		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "990.05" {
			t.Errorf("Expected cash balance '990.05', got %q", balance.String())
		}
	})

	t.Run("fee stores memo", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")

		txn, err := svc.Fee(acct.ID, date, types.MustNewMoney("25.00"), "Annual account fee")
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
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit only 10
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("10.00"), "")

		// Try fee of 50
		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("50.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects zero cash balance", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("1.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for zero balance")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("10.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("fee rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Fee() expected error for zero amount")
		}
	})
}

// =============================================================================
// SM-070: GetCashBalance
// =============================================================================

func TestService_GetCashBalance(t *testing.T) {
	t.Run("empty account has zero balance", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero balance, got %q", balance.String())
		}
	})

	t.Run("computes balance from mixed transactions", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit 1000
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		// Interest 25
		_, _ = svc.Interest(acct.ID, date, types.MustNewMoney("25.00"), "")
		// Fee 10
		_, _ = svc.Fee(acct.ID, date, types.MustNewMoney("10.00"), "")
		// Withdrawal 200
		_, _ = svc.Withdrawal(acct.ID, date, types.MustNewMoney("200.00"), "")

		// Expected: 1000 + 25 - 10 - 200 = 815
		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "815" {
			t.Errorf("Expected cash balance '815', got %q", balance.String())
		}
	})
}

// =============================================================================
// SM-076: Buy transaction (non-lot-tracking account)
// =============================================================================

func TestService_Buy_NonLotTracking(t *testing.T) {
	t.Run("buy adds shares to position and deducts cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash first
		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		totalAmount := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &totalAmount, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify transaction created
		if txn.Type != TransactionTypeBuy {
			t.Errorf("Expected type buy, got %s", txn.Type)
		}
		if !txn.HasSecurity() || txn.SecurityID.ID != sec.ID {
			t.Error("Expected transaction to reference security")
		}
		if !txn.HasShares() || txn.Shares.Quantity.String() != "10" {
			t.Errorf("Expected shares 10, got %s", txn.Shares.Quantity.String())
		}
		// Buy TotalAmount stored as negative (deducting from cash)
		if txn.TotalAmount.String() != "-1850" {
			t.Errorf("Expected total amount '-1850', got %q", txn.TotalAmount.String())
		}
		// Price per share should be computed: 1850/10 = 185
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "185" {
			t.Errorf("Expected price_per_share '185', got %q", txn.PricePerShare.Money.String())
		}

		// Verify position updated
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "10" {
			t.Errorf("Expected position shares '10', got %q", pos.Shares.String())
		}
		if pos.AverageCostPerShare.String() != "185" {
			t.Errorf("Expected avg cost '185', got %q", pos.AverageCostPerShare.String())
		}

		// Verify cash deducted
		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "8150" {
			t.Errorf("Expected cash balance '8150', got %q", balance.String())
		}
	})

	t.Run("buy updates weighted average cost on second purchase", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")

		// First buy: 10 shares at $100
		total1 := types.MustNewMoney("1000.00")
		shares1 := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares1, &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("First Buy() error = %v", err)
		}

		// Second buy: 10 shares at $200
		total2 := types.MustNewMoney("2000.00")
		shares2 := types.MustNewQuantity("10")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, shares2, &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Second Buy() error = %v", err)
		}

		// Weighted avg: (1000 + 2000) / 20 = 150
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "20" {
			t.Errorf("Expected position shares '20', got %q", pos.Shares.String())
		}
		if pos.AverageCostPerShare.String() != "150" {
			t.Errorf("Expected avg cost '150', got %q", pos.AverageCostPerShare.String())
		}
	})

	t.Run("buy with shares and price_per_share auto-fills total", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		price := types.MustNewMoney("185.00")
		shares := types.MustNewQuantity("10")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Total should be 10 * 185 = 1850
		if txn.TotalAmount.String() != "-1850" {
			t.Errorf("Expected total amount '-1850', got %q", txn.TotalAmount.String())
		}
	})

	t.Run("buy with commission", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("1854.95")
		shares := types.MustNewQuantity("10")
		commission := types.MustNewMoney("4.95")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, commission, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Price per share = (1854.95 - 4.95) / 10 = 185
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "185" {
			t.Errorf("Expected price_per_share '185', got %q", txn.PricePerShare.Money.String())
		}
		if !txn.Commission.Valid || txn.Commission.Money.String() != "4.95" {
			t.Errorf("Expected commission '4.95', got %q", txn.Commission.Money.String())
		}

		// Cash deducted by full total amount (including commission)
		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "8145.05" {
			t.Errorf("Expected cash balance '8145.05', got %q", balance.String())
		}
	})

	t.Run("buy requires sufficient cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Only deposit 100
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err == nil {
			t.Fatal("Buy() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("buy rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		total := types.MustNewMoney("100.00")
		shares := types.MustNewQuantity("1")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err == nil {
			t.Fatal("Buy() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("buy creates transaction record with memo", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "Long term hold")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
		if txn.Memo.String != "Long term hold" {
			t.Errorf("Expected memo 'Long term hold', got %q", txn.Memo.String)
		}
	})

	t.Run("buy does not create lot for non-lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify no lots created
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 0 {
			t.Errorf("Expected 0 lots for non-lot-tracking account, got %d", len(lots))
		}
	})
}

// =============================================================================
// SM-077: Buy transaction (lot-tracking account)
// =============================================================================

func TestService_Buy_LotTracking(t *testing.T) {
	t.Run("buy creates new lot with correct fields", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify lot created
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}

		lot := lots[0]
		if lot.Shares.String() != "10" {
			t.Errorf("Expected lot shares '10', got %q", lot.Shares.String())
		}
		if lot.OriginalShares.String() != "10" {
			t.Errorf("Expected original shares '10', got %q", lot.OriginalShares.String())
		}
		if lot.CostPerShare.String() != "185" {
			t.Errorf("Expected cost_per_share '185', got %q", lot.CostPerShare.String())
		}
		if lot.PurchaseDate != date {
			t.Errorf("Expected purchase_date %v, got %v", date, lot.PurchaseDate)
		}
		if lot.SourceTransactionID != txn.ID {
			t.Errorf("Expected source_transaction_id %s, got %s", txn.ID, lot.SourceTransactionID)
		}
		if lot.Closed {
			t.Error("Expected lot to not be closed")
		}
	})

	t.Run("buy deducts total from cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "3150" {
			t.Errorf("Expected cash balance '3150', got %q", balance.String())
		}
	})

	t.Run("buy creates transaction record", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.June, 1)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("2000.00")
		shares := types.MustNewQuantity("5")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "Initial position")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		if txn.Type != TransactionTypeBuy {
			t.Errorf("Expected type buy, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "-2000" {
			t.Errorf("Expected total '-2000', got %q", txn.TotalAmount.String())
		}
		if txn.Memo.String != "Initial position" {
			t.Errorf("Expected memo 'Initial position', got %q", txn.Memo.String)
		}
	})

	t.Run("multiple buys create separate lots", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date1 := types.NewDate(2024, time.March, 1)
		date2 := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date1, types.MustNewMoney("20000.00"), "")

		// First buy: 10 shares at $100
		total1 := types.MustNewMoney("1000.00")
		shares1 := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date1, shares1, &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("First Buy() error = %v", err)
		}

		// Second buy: 5 shares at $200
		total2 := types.MustNewMoney("1000.00")
		shares2 := types.MustNewQuantity("5")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, shares2, &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Second Buy() error = %v", err)
		}

		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// First lot (ordered by purchase_date ASC)
		if lots[0].Shares.String() != "10" {
			t.Errorf("Expected first lot shares '10', got %q", lots[0].Shares.String())
		}
		if lots[0].CostPerShare.String() != "100" {
			t.Errorf("Expected first lot cost '100', got %q", lots[0].CostPerShare.String())
		}

		// Second lot
		if lots[1].Shares.String() != "5" {
			t.Errorf("Expected second lot shares '5', got %q", lots[1].Shares.String())
		}
		if lots[1].CostPerShare.String() != "200" {
			t.Errorf("Expected second lot cost '200', got %q", lots[1].CostPerShare.String())
		}
	})

	t.Run("buy requires sufficient cash for lot-tracking", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err == nil {
			t.Fatal("Buy() expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("buy with commission in lot-tracking creates lot with net cost", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		total := types.MustNewMoney("1854.95")
		shares := types.MustNewQuantity("10")
		commission := types.MustNewMoney("4.95")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, commission, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}

		// Cost per share = (1854.95 - 4.95) / 10 = 185
		if lots[0].CostPerShare.String() != "185" {
			t.Errorf("Expected lot cost_per_share '185', got %q", lots[0].CostPerShare.String())
		}
	})
}
