package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/price"
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
	transactionLotRepo := NewTransactionLotRepository(database)
	priceRepo := price.NewRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, database)
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

// testServiceEnv holds all repos/services needed for buy/sell tests.
type testServiceEnv struct {
	svc                *Service
	accountRepo        *account.Repository
	secRepo            *security.Repository
	priceRepo          *price.Repository
	positionRepo       *PositionRepository
	lotRepo            *LotRepository
	transactionLotRepo *TransactionLotRepository
}

func createFullTestService(t *testing.T) *testServiceEnv {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo := account.NewRepository(database)
	secRepo := security.NewRepository(database)
	priceRepo := price.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, database)
	return &testServiceEnv{
		svc:                svc,
		accountRepo:        accountRepo,
		secRepo:            secRepo,
		priceRepo:          priceRepo,
		positionRepo:       positionRepo,
		lotRepo:            lotRepo,
		transactionLotRepo: transactionLotRepo,
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

// =============================================================================
// SM-078: Sell transaction (non-lot-tracking account)
// =============================================================================

func TestService_Sell_NonLotTracking(t *testing.T) {
	t.Run("sell reduces position shares and adds cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash and buy shares
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Sell 5 shares at $200 each = $1000
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify transaction
		if txn.Type != TransactionTypeSell {
			t.Errorf("Expected type sell, got %s", txn.Type)
		}
		if !txn.HasSecurity() || txn.SecurityID.ID != sec.ID {
			t.Error("Expected transaction to reference security")
		}
		if !txn.HasShares() || txn.Shares.Quantity.String() != "5" {
			t.Errorf("Expected shares 5, got %s", txn.Shares.Quantity.String())
		}
		// Sell TotalAmount stored as positive (adding to cash)
		if txn.TotalAmount.String() != "1000" {
			t.Errorf("Expected total amount '1000', got %q", txn.TotalAmount.String())
		}
		// Price per share: 1000/5 = 200
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "200" {
			t.Errorf("Expected price_per_share '200', got %q", txn.PricePerShare.Money.String())
		}

		// Verify position reduced
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "5" {
			t.Errorf("Expected position shares '5', got %q", pos.Shares.String())
		}
		// Average cost unchanged
		if pos.AverageCostPerShare.String() != "185" {
			t.Errorf("Expected avg cost '185', got %q", pos.AverageCostPerShare.String())
		}

		// Verify cash increased: 10000 - 1850 (buy) + 1000 (sell) = 9150
		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "9150" {
			t.Errorf("Expected cash balance '9150', got %q", balance.String())
		}
	})

	t.Run("selling more than held returns error", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Try to sell 20 when only 10 held
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("20")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Sell() expected error for insufficient shares")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected InsufficientSharesError, got %T: %v", err, err)
		}
	})

	t.Run("position removed when shares reach 0", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Sell all 10 shares
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("10")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Position should have zero shares (deleted)
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !pos.Shares.IsZero() {
			t.Errorf("Expected zero shares after full sell, got %q", pos.Shares.String())
		}
	})

	t.Run("sell with commission", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Sell 5 shares, total proceeds 1004.95 with 4.95 commission
		sellTotal := types.MustNewMoney("1004.95")
		sellShares := types.MustNewQuantity("5")
		commission := types.MustNewMoney("4.95")
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, commission, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Price per share = (1004.95 - 4.95) / 5 = 200
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "200" {
			t.Errorf("Expected price_per_share '200', got %q", txn.PricePerShare.Money.String())
		}
		if !txn.Commission.Valid || txn.Commission.Money.String() != "4.95" {
			t.Errorf("Expected commission '4.95', got %q", txn.Commission.Money.String())
		}
	})

	t.Run("sell with shares and price_per_share auto-fills total", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		price := types.MustNewMoney("200.00")
		sellShares := types.MustNewQuantity("5")
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &price, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Total should be 5 * 200 = 1000
		if txn.TotalAmount.String() != "1000" {
			t.Errorf("Expected total amount '1000', got %q", txn.TotalAmount.String())
		}
	})

	t.Run("sell creates transaction record with memo", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "Taking profits", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status pending, got %s", txn.Status)
		}
		if txn.Memo.String != "Taking profits" {
			t.Errorf("Expected memo 'Taking profits', got %q", txn.Memo.String)
		}
	})

	t.Run("sell rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		sellTotal := types.MustNewMoney("100.00")
		sellShares := types.MustNewQuantity("1")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Sell() expected error for non-investment account")
		}
		if _, ok := err.(*account.NotInvestmentError); !ok {
			t.Errorf("Expected NotInvestmentError, got %T: %v", err, err)
		}
	})

	t.Run("sell does not create lots for non-lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// No lots should exist
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
// SM-079: Sell transaction (lot-tracking account)
// =============================================================================

func TestService_Sell_LotTracking(t *testing.T) {
	t.Run("sell reduces lot shares and adds cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Get the lot
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}

		// Sell 5 shares from the lot
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		if txn.Type != TransactionTypeSell {
			t.Errorf("Expected type sell, got %s", txn.Type)
		}
		if txn.TotalAmount.String() != "1000" {
			t.Errorf("Expected total '1000', got %q", txn.TotalAmount.String())
		}

		// Verify lot reduced
		updatedLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(updatedLots) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(updatedLots))
		}
		if updatedLots[0].Shares.String() != "5" {
			t.Errorf("Expected lot shares '5', got %q", updatedLots[0].Shares.String())
		}

		// Verify cash: 10000 - 1850 + 1000 = 9150
		balance, _ := env.svc.GetCashBalance(acct.ID)
		if balance.String() != "9150" {
			t.Errorf("Expected cash balance '9150', got %q", balance.String())
		}
	})

	t.Run("lot closed when all shares sold", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		// Sell all 10 shares
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("10")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// No open lots
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(openLots) != 0 {
			t.Errorf("Expected 0 open lots, got %d", len(openLots))
		}

		// Closed lot should exist
		allLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if len(allLots) != 1 {
			t.Fatalf("Expected 1 total lot, got %d", len(allLots))
		}
		if !allLots[0].Closed {
			t.Error("Expected lot to be closed")
		}
	})

	t.Run("junction record created linking transaction to lot", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify junction records
		tls, err := env.transactionLotRepo.GetByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("Expected 1 transaction lot, got %d", len(tls))
		}
		if tls[0].LotID != lots[0].ID {
			t.Errorf("Expected lot ID %s, got %s", lots[0].ID, tls[0].LotID)
		}
		if tls[0].Shares.String() != "5" {
			t.Errorf("Expected junction shares '5', got %q", tls[0].Shares.String())
		}
	})

	t.Run("sell from multiple lots", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date1 := types.NewDate(2024, time.March, 1)
		date2 := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date1, types.MustNewMoney("20000.00"), "")

		// Buy lot 1: 10 shares at $100
		total1 := types.MustNewMoney("1000.00")
		shares1 := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date1, shares1, &total1, nil, types.ZeroMoney, "")

		// Buy lot 2: 5 shares at $200
		total2 := types.MustNewMoney("1000.00")
		shares2 := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date2, shares2, &total2, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// Sell 8 shares: 5 from lot 1, 3 from lot 2
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("8")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("3")},
		}
		txn, err := env.svc.Sell(acct.ID, sec.ID, date2, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify lots updated
		updatedLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(updatedLots) != 2 {
			t.Fatalf("Expected 2 open lots, got %d", len(updatedLots))
		}
		// Lot 1: 10 - 5 = 5
		if updatedLots[0].Shares.String() != "5" {
			t.Errorf("Expected lot 1 shares '5', got %q", updatedLots[0].Shares.String())
		}
		// Lot 2: 5 - 3 = 2
		if updatedLots[1].Shares.String() != "2" {
			t.Errorf("Expected lot 2 shares '2', got %q", updatedLots[1].Shares.String())
		}

		// Verify junction records
		tls, _ := env.transactionLotRepo.GetByTransaction(txn.ID)
		if len(tls) != 2 {
			t.Fatalf("Expected 2 transaction lots, got %d", len(tls))
		}
	})

	t.Run("sell requires lot allocations for lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Sell without lot allocations
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Sell() expected error when no lot allocations provided")
		}
	})

	t.Run("total shares across lots must equal sell shares", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		// Sell 5 shares but allocate only 3
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("3")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err == nil {
			t.Fatal("Sell() expected error for mismatched lot allocation")
		}
		if _, ok := err.(*LotAllocationMismatchError); !ok {
			t.Errorf("Expected LotAllocationMismatchError, got %T: %v", err, err)
		}
	})

	t.Run("proceeds added to cash", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		sellTotal := types.MustNewMoney("1500.00")
		sellShares := types.MustNewQuantity("10")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Cash: 5000 - 1000 (buy) + 1500 (sell) = 5500
		balance, _ := env.svc.GetCashBalance(acct.ID)
		if balance.String() != "5500" {
			t.Errorf("Expected cash balance '5500', got %q", balance.String())
		}
	})
}

// =============================================================================
// SM-080: Sell validation: lot selection
// =============================================================================

func TestService_Sell_LotValidation(t *testing.T) {
	t.Run("selling from non-existent lot returns error", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Use a fake lot ID
		fakeLotID := types.NewID()
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: fakeLotID, Shares: types.MustNewQuantity("5")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err == nil {
			t.Fatal("Sell() expected error for non-existent lot")
		}
		if _, ok := err.(*LotNotFoundError); !ok {
			t.Errorf("Expected LotNotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("selling more shares than lot holds returns error", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		// Try to sell 20 from a lot that only has 10
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("20")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("20")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err == nil {
			t.Fatal("Sell() expected error for lot insufficient shares")
		}
		if _, ok := err.(*LotInsufficientSharesError); !ok {
			t.Errorf("Expected LotInsufficientSharesError, got %T: %v", err, err)
		}
	})

	t.Run("selling from lot in different account returns error", func(t *testing.T) {
		env := createFullTestService(t)
		acct1 := createLotTrackingAccount(t, env.accountRepo, "Brokerage 1")
		acct2 := createLotTrackingAccount(t, env.accountRepo, "Brokerage 2")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Buy in account 1
		_, _ = env.svc.Deposit(acct1.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct1.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct1.ID, sec.ID, false)

		// Try to sell from account 2 using account 1's lot
		_, _ = env.svc.Deposit(acct2.ID, date, types.MustNewMoney("10000.00"), "")
		sellTotal := types.MustNewMoney("1000.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
		_, err := env.svc.Sell(acct2.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err == nil {
			t.Fatal("Sell() expected error for lot in wrong account")
		}
		if _, ok := err.(*LotWrongAccountError); !ok {
			t.Errorf("Expected LotWrongAccountError, got %T: %v", err, err)
		}
	})

	t.Run("partial lot sell works", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		// Sell only 3 of 10 shares
		sellTotal := types.MustNewMoney("600.00")
		sellShares := types.MustNewQuantity("3")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("3")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Lot should have 7 remaining
		updatedLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(updatedLots) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(updatedLots))
		}
		if updatedLots[0].Shares.String() != "7" {
			t.Errorf("Expected lot shares '7', got %q", updatedLots[0].Shares.String())
		}
		if updatedLots[0].Closed {
			t.Error("Expected lot to remain open")
		}
	})
}

// =============================================================================
// SM-081: Auto-create price on buy
// =============================================================================

func TestService_Buy_AutoCreatesPrice(t *testing.T) {
	t.Run("buy creates price record with source=transaction", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		totalAmount := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &totalAmount, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify price was created
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v, expected price to be auto-created", err)
		}
		if p.Price.String() != "185" {
			t.Errorf("Expected price '185', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})

	t.Run("buy does not overwrite existing manual price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Create a manual price first
		manualPrice := price.NewPrice(sec.ID, date, types.MustNewMoney("190.00"), price.SourceManual)
		if err := env.priceRepo.Create(manualPrice); err != nil {
			t.Fatalf("Create manual price error = %v", err)
		}

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		totalAmount := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &totalAmount, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify existing manual price was preserved
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p.Price.String() != "190" {
			t.Errorf("Expected manual price '190' preserved, got %q", p.Price.String())
		}
		if p.Source != price.SourceManual {
			t.Errorf("Expected source 'manual' preserved, got %q", p.Source.String())
		}
	})

	t.Run("buy does not overwrite existing import price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Create an import price first
		importPrice := price.NewPrice(sec.ID, date, types.MustNewMoney("192.00"), price.SourceImport)
		if err := env.priceRepo.Create(importPrice); err != nil {
			t.Fatalf("Create import price error = %v", err)
		}

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		totalAmount := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &totalAmount, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify existing import price was preserved
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p.Price.String() != "192" {
			t.Errorf("Expected import price '192' preserved, got %q", p.Price.String())
		}
		if p.Source != price.SourceImport {
			t.Errorf("Expected source 'import' preserved, got %q", p.Source.String())
		}
	})

	t.Run("buy with lot tracking also creates price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		totalAmount := types.MustNewMoney("3000.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &totalAmount, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v, expected price to be auto-created", err)
		}
		if p.Price.String() != "300" {
			t.Errorf("Expected price '300', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})
}

// =============================================================================
// SM-082: Auto-create price on sell
// =============================================================================

func TestService_Sell_AutoCreatesPrice(t *testing.T) {
	t.Run("sell creates price record with source=transaction", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		buyDate := types.NewDate(2024, time.March, 15)
		sellDate := types.NewDate(2024, time.April, 15)

		_, _ = env.svc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), "")

		// Buy shares first
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, buyDate, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Sell some shares at a different date
		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("5")
		_, err := env.svc.Sell(acct.ID, sec.ID, sellDate, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify price was created for the sell date
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, sellDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v, expected price to be auto-created", err)
		}
		// 2000/5 = 400
		if p.Price.String() != "400" {
			t.Errorf("Expected price '400', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})

	t.Run("sell does not overwrite existing manual price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		buyDate := types.NewDate(2024, time.March, 15)
		sellDate := types.NewDate(2024, time.April, 15)

		_, _ = env.svc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), "")

		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, buyDate, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Create a manual price for the sell date
		manualPrice := price.NewPrice(sec.ID, sellDate, types.MustNewMoney("410.00"), price.SourceManual)
		if err := env.priceRepo.Create(manualPrice); err != nil {
			t.Fatalf("Create manual price error = %v", err)
		}

		sellTotal := types.MustNewMoney("2000.00")
		sellShares := types.MustNewQuantity("5")
		_, err := env.svc.Sell(acct.ID, sec.ID, sellDate, sellShares, &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify existing manual price was preserved
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, sellDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p.Price.String() != "410" {
			t.Errorf("Expected manual price '410' preserved, got %q", p.Price.String())
		}
		if p.Source != price.SourceManual {
			t.Errorf("Expected source 'manual' preserved, got %q", p.Source.String())
		}
	})

	t.Run("sell with lot tracking also creates price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		buyDate := types.NewDate(2024, time.March, 15)
		sellDate := types.NewDate(2024, time.April, 15)

		_, _ = env.svc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), "")

		buyTotal := types.MustNewMoney("3000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, buyDate, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Get the lot for the sell allocation
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) == 0 {
			t.Fatal("Expected at least one lot after buy")
		}

		sellTotal := types.MustNewMoney("2500.00")
		sellShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: sellShares}}
		_, err := env.svc.Sell(acct.ID, sec.ID, sellDate, sellShares, &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, sellDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v, expected price to be auto-created", err)
		}
		// 2500/5 = 500
		if p.Price.String() != "500" {
			t.Errorf("Expected price '500', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})
}
