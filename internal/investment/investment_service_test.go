package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
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
	txnRepo := transaction.NewRepository(database)
	caRepo := NewCorporateActionRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, txnRepo, caRepo, database)
	return svc, accountRepo
}

func createInvAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create investment account: %v", err)
	}
	return acct
}

func createCheckAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	return acct
}

// testServiceEnv holds all repos/services needed for buy/sell tests.
type testServiceEnv struct {
	svc                *Service
	invRepo            *Repository
	accountRepo        *account.Repository
	secRepo            *security.Repository
	priceRepo          *price.Repository
	positionRepo       *PositionRepository
	lotRepo            *LotRepository
	transactionLotRepo *TransactionLotRepository
	caRepo             *CorporateActionRepository
	db                 *db.DB
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
	txnRepo := transaction.NewRepository(database)
	caRepo := NewCorporateActionRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, txnRepo, caRepo, database)
	return &testServiceEnv{
		svc:                svc,
		invRepo:            invRepo,
		accountRepo:        accountRepo,
		secRepo:            secRepo,
		priceRepo:          priceRepo,
		positionRepo:       positionRepo,
		lotRepo:            lotRepo,
		transactionLotRepo: transactionLotRepo,
		caRepo:             caRepo,
		db:                 database,
	}
}

func createLotTrackingAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
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

	t.Run("withdrawal allows insufficient cash (negative balance)", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit only 100
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		// Withdraw 200 — cash goes negative; no error
		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("200.00"), "")
		if err != nil {
			t.Fatalf("Withdrawal() unexpected error: %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-100" {
			t.Errorf("Expected cash balance '-100', got %q", balance.String())
		}
	})

	t.Run("withdrawal allows zero cash balance (negative balance)", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("1.00"), "")
		if err != nil {
			t.Fatalf("Withdrawal() unexpected error: %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-1" {
			t.Errorf("Expected cash balance '-1', got %q", balance.String())
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

	t.Run("fee allows insufficient cash (negative balance)", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		// Deposit only 10
		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("10.00"), "")

		// Fee of 50 — cash goes negative; no error
		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("50.00"), "")
		if err != nil {
			t.Fatalf("Fee() unexpected error: %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-40" {
			t.Errorf("Expected cash balance '-40', got %q", balance.String())
		}
	})

	t.Run("fee allows zero cash balance (negative balance)", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("1.00"), "")
		if err != nil {
			t.Fatalf("Fee() unexpected error: %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-1" {
			t.Errorf("Expected cash balance '-1', got %q", balance.String())
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

	t.Run("buy allows insufficient cash (negative balance)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Only deposit 100
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		// Buy 1850 — cash goes negative; no error (same-day data-entry use case)
		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() unexpected error: %v", err)
		}

		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-1750" {
			t.Errorf("Expected cash balance '-1750', got %q", balance.String())
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

	t.Run("buy allows insufficient cash for lot-tracking (negative balance)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")

		total := types.MustNewMoney("1850.00")
		shares := types.MustNewQuantity("10")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, shares, &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() unexpected error: %v", err)
		}

		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-1750" {
			t.Errorf("Expected cash balance '-1750', got %q", balance.String())
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

// =============================================================================
// SM-084: Cash Dividend
// =============================================================================

func TestService_Dividend(t *testing.T) {
	t.Run("dividend increases cash by amount", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		txn, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("47.50"), "Q1 dividend")
		if err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}

		if txn.Type != TransactionTypeDividend {
			t.Errorf("Expected type %q, got %q", TransactionTypeDividend, txn.Type)
		}
		if txn.TotalAmount.String() != "47.5" {
			t.Errorf("Expected total amount '47.5', got %q", txn.TotalAmount.String())
		}
		if !txn.SecurityID.Valid || txn.SecurityID.ID != sec.ID {
			t.Errorf("Expected security ID %s, got %v", sec.ID, txn.SecurityID)
		}
		if !txn.Memo.Valid || txn.Memo.String != "Q1 dividend" {
			t.Errorf("Expected memo 'Q1 dividend', got %v", txn.Memo)
		}

		// Cash balance should increase
		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "47.5" {
			t.Errorf("Expected cash balance '47.5', got %q", balance.String())
		}
	})

	t.Run("dividend with existing cash adds to balance", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		// Deposit some cash first
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000.00"), "")

		_, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("25.00"), "")
		if err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}

		balance, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "5025" {
			t.Errorf("Expected cash balance '5025', got %q", balance.String())
		}
	})

	t.Run("dividend does not change share count", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Buy some shares first
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Record dividend
		divDate := types.NewDate(2024, time.June, 15)
		_, err := env.svc.Dividend(acct.ID, sec.ID, divDate, types.MustNewMoney("47.50"), "")
		if err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}

		// Shares should be unchanged
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "10" {
			t.Errorf("Expected shares '10' unchanged, got %q", pos.Shares.String())
		}
	})

	t.Run("dividend rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("47.50"), "")
		if err == nil {
			t.Fatal("Expected error for non-investment account, got nil")
		}
	})

	t.Run("dividend rejects non-positive amount", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Expected error for zero amount, got nil")
		}

		_, err = env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("-10.00"), "")
		if err == nil {
			t.Fatal("Expected error for negative amount, got nil")
		}
	})

	t.Run("transaction has no shares set", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		txn, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("47.50"), "")
		if err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}

		if txn.HasShares() {
			t.Errorf("Expected dividend transaction to have no shares, but shares were set")
		}
	})
}

// =============================================================================
// SM-085: Reinvest Dividend (non-lot-tracking)
// =============================================================================

func TestService_ReinvestDividend_NonLotTracking(t *testing.T) {
	t.Run("reinvest adds shares to position", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Buy initial shares
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Reinvest dividend
		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("370.00")
		reinvestShares := types.MustNewQuantity("2")
		txn, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "DRIP")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		if txn.Type != TransactionTypeReinvestDividend {
			t.Errorf("Expected type %q, got %q", TransactionTypeReinvestDividend, txn.Type)
		}
		if txn.TotalAmount.String() != "370" {
			t.Errorf("Expected total amount '370', got %q", txn.TotalAmount.String())
		}
		if !txn.HasShares() || txn.Shares.Quantity.String() != "2" {
			t.Errorf("Expected shares '2', got %v", txn.Shares)
		}
		// price_per_share = 370/2 = 185
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "185" {
			t.Errorf("Expected price_per_share '185', got %v", txn.PricePerShare)
		}

		// Position should have 12 shares (10 + 2)
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "12" {
			t.Errorf("Expected shares '12', got %q", pos.Shares.String())
		}
	})

	t.Run("reinvest recalculates average cost", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Buy 10 shares at $100
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Reinvest 2 shares at $120 each
		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("240.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		// Weighted average: (10*100 + 2*120) / 12 = 1240/12 ≈ 103.333...
		// Check it's approximately right (exact value depends on decimal precision)
		avgCost := pos.AverageCostPerShare
		if avgCost.String() == "100" || avgCost.String() == "120" {
			t.Errorf("Expected weighted average between 100 and 120, got %q", avgCost.String())
		}
	})

	t.Run("reinvest has no cash movement", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Cash balance after buy: 10000 - 1850 = 8150
		balanceBefore, _ := env.svc.GetCashBalance(acct.ID)

		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("370.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		balanceAfter, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balanceBefore.String() != balanceAfter.String() {
			t.Errorf("Expected cash unchanged at %q, got %q", balanceBefore.String(), balanceAfter.String())
		}
	})

	t.Run("reinvest with price_per_share provided", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestShares := types.MustNewQuantity("2")
		pps := types.MustNewMoney("185.00")
		txn, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, nil, &pps, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}
		// total = 2 * 185 = 370
		if txn.TotalAmount.String() != "370" {
			t.Errorf("Expected total '370', got %q", txn.TotalAmount.String())
		}
	})

	t.Run("reinvest rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		total := types.MustNewMoney("370.00")
		shares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date, shares, &total, nil, "")
		if err == nil {
			t.Fatal("Expected error for non-investment account, got nil")
		}
	})

	t.Run("reinvest creates position from zero", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.June, 15)

		total := types.MustNewMoney("370.00")
		shares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date, shares, &total, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "2" {
			t.Errorf("Expected shares '2', got %q", pos.Shares.String())
		}
		if pos.AverageCostPerShare.String() != "185" {
			t.Errorf("Expected avg cost '185', got %q", pos.AverageCostPerShare.String())
		}
	})
}

// =============================================================================
// SM-086: Reinvest Dividend (lot-tracking)
// =============================================================================

func TestService_ReinvestDividend_LotTracking(t *testing.T) {
	t.Run("reinvest creates new lot", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Buy initial shares
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Verify 1 lot exists
		lotsBefore, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lotsBefore) != 1 {
			t.Fatalf("Expected 1 lot before reinvest, got %d", len(lotsBefore))
		}

		// Reinvest dividend
		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("370.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		// Should have 2 lots now
		lotsAfter, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lotsAfter) != 2 {
			t.Fatalf("Expected 2 lots after reinvest, got %d", len(lotsAfter))
		}
	})

	t.Run("reinvest lot has correct properties", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("3000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("370.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		// Find the reinvest lot (the newer one)
		var reinvestLot *Lot
		for _, l := range lots {
			if l.PurchaseDate.Time().Equal(reinvestDate.Time()) {
				reinvestLot = l
				break
			}
		}
		if reinvestLot == nil {
			t.Fatal("Could not find reinvest lot by purchase date")
		}

		if reinvestLot.Shares.String() != "2" {
			t.Errorf("Expected lot shares '2', got %q", reinvestLot.Shares.String())
		}
		// 370/2 = 185
		if reinvestLot.CostPerShare.String() != "185" {
			t.Errorf("Expected cost_per_share '185', got %q", reinvestLot.CostPerShare.String())
		}
		if reinvestLot.Closed {
			t.Error("Expected lot to not be closed")
		}
	})

	t.Run("reinvest with lot tracking has no cash movement", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1500.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		balanceBefore, _ := env.svc.GetCashBalance(acct.ID)

		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("300.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		balanceAfter, _ := env.svc.GetCashBalance(acct.ID)
		if balanceBefore.String() != balanceAfter.String() {
			t.Errorf("Expected cash unchanged at %q, got %q", balanceBefore.String(), balanceAfter.String())
		}
	})
}

// =============================================================================
// SM-083: Auto-create price on reinvest dividend
// =============================================================================

func TestService_ReinvestDividend_AutoCreatesPrice(t *testing.T) {
	t.Run("reinvest creates price record", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		reinvestDate := types.NewDate(2024, time.June, 15)
		reinvestTotal := types.MustNewMoney("400.00")
		reinvestShares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, reinvestShares, &reinvestTotal, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		// Price should be auto-created for reinvest date
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, reinvestDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v, expected price to be auto-created", err)
		}
		// 400/2 = 200
		if p.Price.String() != "200" {
			t.Errorf("Expected price '200', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})

	t.Run("reinvest does not overwrite existing manual price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		reinvestDate := types.NewDate(2024, time.June, 15)

		// Create manual price first
		manualPrice := price.NewPrice(sec.ID, reinvestDate, types.MustNewMoney("210.00"), price.SourceManual)
		if err := env.priceRepo.Create(manualPrice); err != nil {
			t.Fatalf("Create manual price error = %v", err)
		}

		total := types.MustNewMoney("400.00")
		shares := types.MustNewQuantity("2")
		_, err := env.svc.ReinvestDividend(acct.ID, sec.ID, reinvestDate, shares, &total, nil, "")
		if err != nil {
			t.Fatalf("ReinvestDividend() error = %v", err)
		}

		// Manual price should be preserved
		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, reinvestDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p.Price.String() != "210" {
			t.Errorf("Expected manual price '210' preserved, got %q", p.Price.String())
		}
		if p.Source != price.SourceManual {
			t.Errorf("Expected source 'manual' preserved, got %q", p.Source.String())
		}
	})
}

// =============================================================================
// SM-087: Fee via Liquidation (non-lot-tracking)
// =============================================================================

func TestService_FeeLiquidation_NonLotTracking(t *testing.T) {
	t.Run("fee liquidation reduces shares", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit and buy 10 shares at $100 each
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Fee liquidation: sell 1 share at $110 to cover a $110 fee
		feeDate := types.NewDate(2024, time.June, 15)
		feeAmount := types.MustNewMoney("110.00")
		feeShares := types.MustNewQuantity("1")
		pps := types.MustNewMoney("110.00")
		txn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeAmount, &pps, types.ZeroMoney, "Annual fee", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		// Verify transaction fields
		if txn.Type != TransactionTypeFeeLiquidation {
			t.Errorf("Expected type %q, got %q", TransactionTypeFeeLiquidation, txn.Type)
		}
		if txn.TotalAmount.String() != "110" {
			t.Errorf("Expected total amount '110', got %q", txn.TotalAmount.String())
		}
		if !txn.HasShares() || txn.Shares.Quantity.String() != "1" {
			t.Errorf("Expected shares '1', got %v", txn.Shares)
		}
		if !txn.HasPricePerShare() || txn.PricePerShare.Money.String() != "110" {
			t.Errorf("Expected price_per_share '110', got %v", txn.PricePerShare)
		}
		if !txn.HasSecurity() {
			t.Error("Expected transaction to have security set")
		}

		// Position should now have 9 shares
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "9" {
			t.Errorf("Expected shares '9', got %q", pos.Shares.String())
		}
	})

	t.Run("fee liquidation has no net cash effect", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Cash after buy: 10000 - 1000 = 9000
		balanceBefore, _ := env.svc.GetCashBalance(acct.ID)

		feeDate := types.NewDate(2024, time.June, 15)
		feeAmount := types.MustNewMoney("110.00")
		feeShares := types.MustNewQuantity("1")
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeAmount, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		balanceAfter, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balanceBefore.String() != balanceAfter.String() {
			t.Errorf("Expected cash unchanged at %q, got %q", balanceBefore.String(), balanceAfter.String())
		}
	})

	t.Run("fee liquidation with price_per_share computes total", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.June, 15)
		feeShares := types.MustNewQuantity("2")
		pps := types.MustNewMoney("50.00")
		txn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, nil, &pps, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}
		// total = 2 * 50 = 100
		if txn.TotalAmount.String() != "100" {
			t.Errorf("Expected total '100', got %q", txn.TotalAmount.String())
		}
	})

	t.Run("fee liquidation with commission", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.June, 15)
		feeShares := types.MustNewQuantity("2")
		feeTotal := types.MustNewMoney("210.00")
		commission := types.MustNewMoney("10.00")
		txn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, commission, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}
		// price_per_share = (210 - 10) / 2 = 100
		if txn.PricePerShare.Money.String() != "100" {
			t.Errorf("Expected price_per_share '100', got %q", txn.PricePerShare.Money.String())
		}
		if txn.Commission.Money.String() != "10" {
			t.Errorf("Expected commission '10', got %q", txn.Commission.Money.String())
		}
	})

	t.Run("fee liquidation fails with insufficient shares", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("500.00")
		buyShares := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("1000.00")
		feeShares := types.MustNewQuantity("10") // Only have 5
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Expected error for insufficient shares, got nil")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected InsufficientSharesError, got %T: %v", err, err)
		}
	})

	t.Run("fee liquidation deletes position when all shares sold", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("500.00")
		buyShares := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("550.00")
		feeShares := types.MustNewQuantity("5")
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !pos.Shares.IsZero() {
			t.Errorf("Expected zero shares, got %q", pos.Shares.String())
		}
	})

	t.Run("fee liquidation rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		feeTotal := types.MustNewMoney("100.00")
		feeShares := types.MustNewQuantity("1")
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, feeShares, &feeTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Expected error for non-investment account, got nil")
		}
	})

	t.Run("fee liquidation auto-creates price record", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.July, 1)
		feeTotal := types.MustNewMoney("120.00")
		feeShares := types.MustNewQuantity("1")
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, feeDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p.Price.String() != "120" {
			t.Errorf("Expected auto-created price '120', got %q", p.Price.String())
		}
		if p.Source != price.SourceTransaction {
			t.Errorf("Expected source 'transaction', got %q", p.Source.String())
		}
	})
}

// =============================================================================
// SM-088: Fee via Liquidation (lot-tracking)
// =============================================================================

func TestService_FeeLiquidation_LotTracking(t *testing.T) {
	t.Run("fee liquidation reduces lot shares", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit and buy 10 shares
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		// Get the lot
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}

		// Fee liquidation: sell 3 shares from the lot
		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("330.00")
		feeShares := types.MustNewQuantity("3")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("3")},
		}
		txn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "Fee", allocs)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		if txn.Type != TransactionTypeFeeLiquidation {
			t.Errorf("Expected type %q, got %q", TransactionTypeFeeLiquidation, txn.Type)
		}

		// Lot should have 7 shares remaining
		updatedLot, err := env.lotRepo.GetByID(lots[0].ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if updatedLot.Shares.String() != "7" {
			t.Errorf("Expected lot shares '7', got %q", updatedLot.Shares.String())
		}
		if updatedLot.Closed {
			t.Error("Expected lot to remain open")
		}
	})

	t.Run("fee liquidation creates junction records", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Buy two lots
		buy1Total := types.MustNewMoney("500.00")
		buy1Shares := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buy1Shares, &buy1Total, nil, types.ZeroMoney, "")

		date2 := types.NewDate(2024, time.April, 15)
		buy2Total := types.MustNewMoney("600.00")
		buy2Shares := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date2, buy2Shares, &buy2Total, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// Fee liquidation from both lots
		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("440.00")
		feeShares := types.MustNewQuantity("4")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("2")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("2")},
		}
		txn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		// Verify junction records
		junctions, err := env.transactionLotRepo.GetByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(junctions) != 2 {
			t.Errorf("Expected 2 junction records, got %d", len(junctions))
		}
	})

	t.Run("fee liquidation has no net cash effect (lot-tracking)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		balanceBefore, _ := env.svc.GetCashBalance(acct.ID)

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("110.00")
		feeShares := types.MustNewQuantity("1")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("1")},
		}
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		balanceAfter, err := env.svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balanceBefore.String() != balanceAfter.String() {
			t.Errorf("Expected cash unchanged at %q, got %q", balanceBefore.String(), balanceAfter.String())
		}
	})

	t.Run("fee liquidation closes lot when all shares used", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("500.00")
		buyShares := types.MustNewQuantity("5")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("550.00")
		feeShares := types.MustNewQuantity("5")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")},
		}
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		updatedLot, err := env.lotRepo.GetByID(lots[0].ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !updatedLot.Closed {
			t.Error("Expected lot to be closed")
		}
		if !updatedLot.Shares.IsZero() {
			t.Errorf("Expected lot shares '0', got %q", updatedLot.Shares.String())
		}
	})

	t.Run("fee liquidation requires lot allocations for lot-tracking", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("110.00")
		feeShares := types.MustNewQuantity("1")
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("Expected error for missing lot allocations, got nil")
		}
	})

	t.Run("fee liquidation rejects allocation mismatch", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		buyShares := types.MustNewQuantity("10")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		feeDate := types.NewDate(2024, time.June, 15)
		feeTotal := types.MustNewMoney("330.00")
		feeShares := types.MustNewQuantity("3")
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("2")}, // Only 2, need 3
		}
		_, err := env.svc.FeeLiquidation(acct.ID, sec.ID, feeDate, feeShares, &feeTotal, nil, types.ZeroMoney, "", allocs)
		if err == nil {
			t.Fatal("Expected error for allocation mismatch, got nil")
		}
		if _, ok := err.(*LotAllocationMismatchError); !ok {
			t.Errorf("Expected LotAllocationMismatchError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-089: Cash transfer between investment and regular account
// =============================================================================

func TestService_TransferCash(t *testing.T) {
	t.Run("withdrawal from investment creates paired transactions", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash to investment account first
		_, err := svc.Deposit(invAcct.ID, date, types.MustNewMoney("5000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Transfer cash out of investment into checking
		result, err := svc.TransferCash(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("1000.00"), "Transfer to checking")
		if err != nil {
			t.Fatalf("TransferCash() error = %v", err)
		}

		// Verify investment transaction
		if result.InvestmentTransaction == nil {
			t.Fatal("Expected non-nil investment transaction")
		}
		if result.InvestmentTransaction.Type != TransactionTypeTransferCash {
			t.Errorf("Expected type transfer_cash, got %s", result.InvestmentTransaction.Type)
		}
		if result.InvestmentTransaction.TotalAmount.String() != "-1000" {
			t.Errorf("Expected investment amount '-1000', got %q", result.InvestmentTransaction.TotalAmount.String())
		}
		if result.InvestmentTransaction.AccountID != invAcct.ID {
			t.Errorf("Expected investment account ID %s, got %s", invAcct.ID, result.InvestmentTransaction.AccountID)
		}
		if result.InvestmentTransaction.Memo.String != "Transfer to checking" {
			t.Errorf("Expected memo 'Transfer to checking', got %q", result.InvestmentTransaction.Memo.String)
		}

		// Verify regular transaction
		if result.RegularTransaction == nil {
			t.Fatal("Expected non-nil regular transaction")
		}
		if result.RegularTransaction.Amount.String() != "1000" {
			t.Errorf("Expected regular amount '1000', got %q", result.RegularTransaction.Amount.String())
		}
		if result.RegularTransaction.AccountID != checkAcct.ID {
			t.Errorf("Expected regular account ID %s, got %s", checkAcct.ID, result.RegularTransaction.AccountID)
		}
		if result.RegularTransaction.Memo.String != "Transfer to checking" {
			t.Errorf("Expected memo 'Transfer to checking', got %q", result.RegularTransaction.Memo.String)
		}

		// Verify linked by transfer_id
		if !result.InvestmentTransaction.IsTransfer() {
			t.Error("Investment transaction should be a transfer")
		}
		if !result.RegularTransaction.IsTransfer() {
			t.Error("Regular transaction should be a transfer")
		}
		if result.InvestmentTransaction.TransferID.ID != result.RegularTransaction.TransferID.ID {
			t.Error("Transfer IDs should match")
		}
		if result.InvestmentTransaction.TransferAccountID.ID != checkAcct.ID {
			t.Error("Investment transfer_account_id should point to checking account")
		}
		if result.RegularTransaction.TransferAccountID.ID != invAcct.ID {
			t.Error("Regular transfer_account_id should point to investment account")
		}

		// Verify cash balance decreased
		balance, err := svc.GetCashBalance(invAcct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "4000" {
			t.Errorf("Expected cash balance '4000', got %q", balance.String())
		}
	})

	t.Run("withdrawal allows insufficient cash (negative balance)", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		// Deposit only 500
		_, _ = svc.Deposit(invAcct.ID, date, types.MustNewMoney("500.00"), "")

		// Transfer 1000 — cash goes negative; no error
		_, err := svc.TransferCash(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("TransferCash() unexpected error: %v", err)
		}

		balance, err := svc.GetCashBalance(invAcct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-500" {
			t.Errorf("Expected cash balance '-500', got %q", balance.String())
		}
	})

	t.Run("withdrawal rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.TransferCash(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Expected error for zero amount")
		}

		_, err = svc.TransferCash(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("-100.00"), "")
		if err == nil {
			t.Fatal("Expected error for negative amount")
		}
	})

	t.Run("withdrawal rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		checkAcct1 := createCheckAccount(t, accountRepo, "Checking1")
		checkAcct2 := createCheckAccount(t, accountRepo, "Checking2")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.TransferCash(checkAcct1.ID, checkAcct2.ID, date, types.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Expected error for non-investment account")
		}
	})

	t.Run("withdrawal rejects investment-to-investment transfer", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct1 := createInvAccount(t, accountRepo, "Brokerage1")
		invAcct2 := createInvAccount(t, accountRepo, "Brokerage2")
		date := types.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(invAcct1.ID, date, types.MustNewMoney("5000.00"), "")

		_, err := svc.TransferCash(invAcct1.ID, invAcct2.ID, date, types.MustNewMoney("1000.00"), "")
		if err == nil {
			t.Fatal("Expected error for investment-to-investment transfer")
		}
		if _, ok := err.(*NotRegularAccountError); !ok {
			t.Errorf("Expected NotRegularAccountError, got %T: %v", err, err)
		}
	})

	t.Run("withdrawal rejects same account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(invAcct.ID, date, types.MustNewMoney("5000.00"), "")

		// This should fail because investment account can't be the regular account
		_, err := svc.TransferCash(invAcct.ID, invAcct.ID, date, types.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Expected error for same account transfer")
		}
	})
}

func TestService_DepositFromAccount(t *testing.T) {
	t.Run("deposit from checking creates paired transactions", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		result, err := svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("2000.00"), "Fund account")
		if err != nil {
			t.Fatalf("DepositFromAccount() error = %v", err)
		}

		// Verify investment transaction (deposit — positive amount)
		if result.InvestmentTransaction.Type != TransactionTypeTransferCash {
			t.Errorf("Expected type transfer_cash, got %s", result.InvestmentTransaction.Type)
		}
		if result.InvestmentTransaction.TotalAmount.String() != "2000" {
			t.Errorf("Expected investment amount '2000', got %q", result.InvestmentTransaction.TotalAmount.String())
		}
		if result.InvestmentTransaction.AccountID != invAcct.ID {
			t.Errorf("Expected investment account ID %s, got %s", invAcct.ID, result.InvestmentTransaction.AccountID)
		}

		// Verify regular transaction (withdrawal — negative amount)
		if result.RegularTransaction.Amount.String() != "-2000" {
			t.Errorf("Expected regular amount '-2000', got %q", result.RegularTransaction.Amount.String())
		}
		if result.RegularTransaction.AccountID != checkAcct.ID {
			t.Errorf("Expected regular account ID %s, got %s", checkAcct.ID, result.RegularTransaction.AccountID)
		}

		// Verify linked by transfer_id
		if result.InvestmentTransaction.TransferID.ID != result.RegularTransaction.TransferID.ID {
			t.Error("Transfer IDs should match")
		}
		if result.InvestmentTransaction.TransferAccountID.ID != checkAcct.ID {
			t.Error("Investment transfer_account_id should point to checking account")
		}
		if result.RegularTransaction.TransferAccountID.ID != invAcct.ID {
			t.Error("Regular transfer_account_id should point to investment account")
		}

		// Verify cash balance increased in investment account
		balance, err := svc.GetCashBalance(invAcct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "2000" {
			t.Errorf("Expected cash balance '2000', got %q", balance.String())
		}
	})

	t.Run("deposit creates transaction with memo", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		result, err := svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("500.00"), "Monthly contribution")
		if err != nil {
			t.Fatalf("DepositFromAccount() error = %v", err)
		}

		if result.InvestmentTransaction.Memo.String != "Monthly contribution" {
			t.Errorf("Expected memo 'Monthly contribution', got %q", result.InvestmentTransaction.Memo.String)
		}
		if result.RegularTransaction.Memo.String != "Monthly contribution" {
			t.Errorf("Expected memo 'Monthly contribution', got %q", result.RegularTransaction.Memo.String)
		}
	})

	t.Run("deposit rejects non-positive amount", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("0.00"), "")
		if err == nil {
			t.Fatal("Expected error for zero amount")
		}

		_, err = svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("-100.00"), "")
		if err == nil {
			t.Fatal("Expected error for negative amount")
		}
	})

	t.Run("deposit rejects non-investment account", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		checkAcct1 := createCheckAccount(t, accountRepo, "Checking1")
		checkAcct2 := createCheckAccount(t, accountRepo, "Checking2")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.DepositFromAccount(checkAcct1.ID, checkAcct2.ID, date, types.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Expected error for non-investment account")
		}
	})

	t.Run("deposit rejects investment-to-investment", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct1 := createInvAccount(t, accountRepo, "Brokerage1")
		invAcct2 := createInvAccount(t, accountRepo, "Brokerage2")
		date := types.NewDate(2024, time.March, 15)

		_, err := svc.DepositFromAccount(invAcct1.ID, invAcct2.ID, date, types.MustNewMoney("100.00"), "")
		if err == nil {
			t.Fatal("Expected error for investment-to-investment transfer")
		}
		if _, ok := err.(*NotRegularAccountError); !ok {
			t.Errorf("Expected NotRegularAccountError, got %T: %v", err, err)
		}
	})

	t.Run("deposit and withdrawal round trip", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		// Deposit from checking into investment
		_, err := svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("3000.00"), "")
		if err != nil {
			t.Fatalf("DepositFromAccount() error = %v", err)
		}

		balance, _ := svc.GetCashBalance(invAcct.ID)
		if balance.String() != "3000" {
			t.Errorf("Expected cash balance '3000' after deposit, got %q", balance.String())
		}

		// Transfer cash back from investment to checking
		_, err = svc.TransferCash(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("1000.00"), "")
		if err != nil {
			t.Fatalf("TransferCash() error = %v", err)
		}

		balance, _ = svc.GetCashBalance(invAcct.ID)
		if balance.String() != "2000" {
			t.Errorf("Expected cash balance '2000' after withdrawal, got %q", balance.String())
		}
	})

	t.Run("investment transaction persisted and readable", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, accountRepo, "Checking")
		date := types.NewDate(2024, time.March, 15)

		result, err := svc.DepositFromAccount(invAcct.ID, checkAcct.ID, date, types.MustNewMoney("1500.00"), "Deposit")
		if err != nil {
			t.Fatalf("DepositFromAccount() error = %v", err)
		}

		// Read back the investment transaction from the repo
		readBack, err := svc.repo.GetByID(result.InvestmentTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if readBack.Type != TransactionTypeTransferCash {
			t.Errorf("Expected type transfer_cash, got %s", readBack.Type)
		}
		if !readBack.IsTransfer() {
			t.Error("Read-back transaction should be a transfer")
		}
		if readBack.TransferID.ID != result.TransferID {
			t.Errorf("Expected transfer_id %s, got %s", result.TransferID, readBack.TransferID.ID)
		}
		if readBack.TransferAccountID.ID != checkAcct.ID {
			t.Errorf("Expected transfer_account_id %s, got %s", checkAcct.ID, readBack.TransferAccountID.ID)
		}
	})
}

// =============================================================================
// TransferCashBetweenInvestments — cash transfer between two investment
// accounts (e.g. IRA → IRA rollovers). Parallel in shape to TransferShares.
// =============================================================================

func TestTransferCashBetweenInvestments_HappyPath(t *testing.T) {
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	dst := createInvAccount(t, accountRepo, "Dest IRA")
	date := types.NewDate(2024, time.March, 15)

	if _, err := svc.Deposit(src.ID, date, types.MustNewMoney("2000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	result, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("500.00"), "rollover")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	if result.SourceTransaction == nil || result.DestinationTransaction == nil {
		t.Fatal("expected both source and destination transactions to be non-nil")
	}

	if result.SourceTransaction.Type != TransactionTypeTransferCash {
		t.Errorf("source type = %s, want transfer_cash", result.SourceTransaction.Type)
	}
	if result.DestinationTransaction.Type != TransactionTypeTransferCash {
		t.Errorf("destination type = %s, want transfer_cash", result.DestinationTransaction.Type)
	}

	if result.SourceTransaction.TotalAmount.String() != "-500" {
		t.Errorf("source amount = %q, want -500", result.SourceTransaction.TotalAmount.String())
	}
	if result.DestinationTransaction.TotalAmount.String() != "500" {
		t.Errorf("destination amount = %q, want 500", result.DestinationTransaction.TotalAmount.String())
	}

	if result.SourceTransaction.AccountID != src.ID {
		t.Errorf("source AccountID = %s, want %s", result.SourceTransaction.AccountID, src.ID)
	}
	if result.DestinationTransaction.AccountID != dst.ID {
		t.Errorf("destination AccountID = %s, want %s", result.DestinationTransaction.AccountID, dst.ID)
	}

	if !result.SourceTransaction.IsTransfer() || !result.DestinationTransaction.IsTransfer() {
		t.Error("both transactions should be transfers")
	}
	if result.SourceTransaction.TransferID.ID != result.DestinationTransaction.TransferID.ID {
		t.Errorf("transfer_id mismatch: src=%s dst=%s",
			result.SourceTransaction.TransferID.ID, result.DestinationTransaction.TransferID.ID)
	}
	if result.SourceTransaction.TransferID.ID != result.TransferID {
		t.Errorf("source transfer_id = %s, want %s", result.SourceTransaction.TransferID.ID, result.TransferID)
	}
	if result.SourceTransaction.TransferAccountID.ID != dst.ID {
		t.Errorf("source transfer_account_id = %s, want %s", result.SourceTransaction.TransferAccountID.ID, dst.ID)
	}
	if result.DestinationTransaction.TransferAccountID.ID != src.ID {
		t.Errorf("destination transfer_account_id = %s, want %s", result.DestinationTransaction.TransferAccountID.ID, src.ID)
	}

	if result.SourceTransaction.Memo.String != "rollover" {
		t.Errorf("source memo = %q, want %q", result.SourceTransaction.Memo.String, "rollover")
	}
	if result.DestinationTransaction.Memo.String != "rollover" {
		t.Errorf("destination memo = %q, want %q", result.DestinationTransaction.Memo.String, "rollover")
	}

	srcBalance, err := svc.GetCashBalance(src.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(src) error = %v", err)
	}
	if srcBalance.String() != "1500" {
		t.Errorf("source cash balance = %q, want 1500", srcBalance.String())
	}
	dstBalance, err := svc.GetCashBalance(dst.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(dst) error = %v", err)
	}
	if dstBalance.String() != "500" {
		t.Errorf("destination cash balance = %q, want 500", dstBalance.String())
	}
}

func TestTransferCashBetweenInvestments_AllowsNegativeSourceBalance(t *testing.T) {
	// Matches the broader investment-cash invariant: cash may go negative.
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	dst := createInvAccount(t, accountRepo, "Dest IRA")
	date := types.NewDate(2024, time.March, 15)

	if _, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("250.00"), ""); err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	bal, err := svc.GetCashBalance(src.ID)
	if err != nil {
		t.Fatalf("GetCashBalance() error = %v", err)
	}
	if bal.String() != "-250" {
		t.Errorf("source cash balance = %q, want -250", bal.String())
	}
}

func TestTransferCashBetweenInvestments_RejectsNonInvestmentSource(t *testing.T) {
	svc, accountRepo := createTestService(t)
	check := createCheckAccount(t, accountRepo, "Checking")
	inv := createInvAccount(t, accountRepo, "IRA")
	date := types.NewDate(2024, time.March, 15)

	_, err := svc.TransferCashBetweenInvestments(check.ID, inv.ID, date, types.MustNewMoney("100.00"), "")
	if err == nil {
		t.Fatal("expected error when source is non-investment")
	}
}

func TestTransferCashBetweenInvestments_RejectsNonInvestmentDestination(t *testing.T) {
	svc, accountRepo := createTestService(t)
	inv := createInvAccount(t, accountRepo, "IRA")
	check := createCheckAccount(t, accountRepo, "Checking")
	date := types.NewDate(2024, time.March, 15)

	_, err := svc.TransferCashBetweenInvestments(inv.ID, check.ID, date, types.MustNewMoney("100.00"), "")
	if err == nil {
		t.Fatal("expected error when destination is non-investment")
	}
}

func TestTransferCashBetweenInvestments_RejectsSameAccount(t *testing.T) {
	svc, accountRepo := createTestService(t)
	inv := createInvAccount(t, accountRepo, "IRA")
	date := types.NewDate(2024, time.March, 15)

	_, err := svc.TransferCashBetweenInvestments(inv.ID, inv.ID, date, types.MustNewMoney("100.00"), "")
	if err == nil {
		t.Fatal("expected error when source and destination are the same account")
	}
}

func TestTransferCashBetweenInvestments_RejectsNonPositiveAmount(t *testing.T) {
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	dst := createInvAccount(t, accountRepo, "Dest IRA")
	date := types.NewDate(2024, time.March, 15)

	_, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("0.00"), "")
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, ok := err.(*InvalidTransferAmountError); !ok {
		t.Errorf("expected *InvalidTransferAmountError for zero amount, got %T: %v", err, err)
	}

	_, err = svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("-50.00"), "")
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
	if _, ok := err.(*InvalidTransferAmountError); !ok {
		t.Errorf("expected *InvalidTransferAmountError for negative amount, got %T: %v", err, err)
	}
}

func TestTransferCashBetweenInvestments_NoLeakOnDestinationFailure(t *testing.T) {
	// If creating the destination row fails, the source row must be rolled
	// back so we don't leave an orphan transfer leg. We can't easily inject
	// a repo-level failure without mocking, so simulate the failure mode by
	// pointing destination at a non-existent investment account ID.
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	date := types.NewDate(2024, time.March, 15)

	bogusDst := types.NewID()

	_, err := svc.TransferCashBetweenInvestments(src.ID, bogusDst, date, types.MustNewMoney("100.00"), "")
	if err == nil {
		t.Fatal("expected error for non-existent destination account")
	}

	// No source row should have been left behind.
	bal, err := svc.GetCashBalance(src.ID)
	if err != nil {
		t.Fatalf("GetCashBalance() error = %v", err)
	}
	if !bal.IsZero() {
		t.Errorf("source balance = %q, want zero (no leftover transfer leg)", bal.String())
	}
}

// =============================================================================
// UpdateTransferCash — reproduces the scenario from the in-TUI bug report:
// user accidentally created a deposit-from-savings (cash flowing the wrong
// way), then tried to edit the transfer to flip the direction.
// =============================================================================

func TestService_UpdateTransferCash(t *testing.T) {
	t.Run("flip deposit-in to withdraw-out keeps amount, swaps signs", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		savings := createCheckAccount(t, accountRepo, "Savings")
		date := types.NewDate(2024, time.March, 15)

		// Original wrong-direction transfer: cash flows savings → brokerage.
		orig, err := svc.DepositFromAccount(invAcct.ID, savings.ID, date, types.MustNewMoney("1170.33"), "to savings")
		if err != nil {
			t.Fatalf("DepositFromAccount() error = %v", err)
		}
		if orig.InvestmentTransaction.TotalAmount.String() != "1170.33" {
			t.Fatalf("setup precondition: expected investment +1170.33, got %s", orig.InvestmentTransaction.TotalAmount.String())
		}
		if orig.RegularTransaction.Amount.String() != "-1170.33" {
			t.Fatalf("setup precondition: expected savings -1170.33, got %s", orig.RegularTransaction.Amount.String())
		}

		// User flips direction: now cash flows brokerage → savings.
		fixed, err := svc.UpdateTransferCash(
			orig.InvestmentTransaction.ID,
			invAcct.ID, savings.ID, date,
			types.MustNewMoney("1170.33"),
			"to savings",
			"out",
			transaction.StatusUncleared,
		)
		if err != nil {
			t.Fatalf("UpdateTransferCash() error = %v", err)
		}
		if fixed.InvestmentTransaction.TotalAmount.String() != "-1170.33" {
			t.Errorf("after flip: expected investment -1170.33, got %s", fixed.InvestmentTransaction.TotalAmount.String())
		}
		if fixed.RegularTransaction.Amount.String() != "1170.33" {
			t.Errorf("after flip: expected savings +1170.33, got %s", fixed.RegularTransaction.Amount.String())
		}

		// The original investment-side row must be gone (replaced).
		if _, err := svc.repo.GetByID(orig.InvestmentTransaction.ID); err == nil {
			t.Error("original investment txn should have been deleted by UpdateTransferCash")
		}

		// Only the new transfer pair should remain — no orphan savings rows.
		txnRepo := transaction.NewRepository(svc.db)
		savingsTxns, err := txnRepo.ListByAccount(savings.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(savingsTxns) != 1 {
			t.Fatalf("expected exactly 1 savings txn after edit, got %d", len(savingsTxns))
		}
		if savingsTxns[0].Amount.String() != "1170.33" {
			t.Errorf("savings txn amount = %s, want 1170.33", savingsTxns[0].Amount.String())
		}
		if !savingsTxns[0].IsTransfer() {
			t.Error("remaining savings txn should still be linked as a transfer")
		}
		if savingsTxns[0].TransferID.ID != fixed.TransferID {
			t.Errorf("savings txn transfer_id = %s, want %s", savingsTxns[0].TransferID.ID, fixed.TransferID)
		}
	})

	t.Run("edit-in-place same direction: amount and memo change cleanly", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		savings := createCheckAccount(t, accountRepo, "Savings")
		date := types.NewDate(2024, time.March, 15)

		// Real withdrawal: brokerage → savings.
		orig, err := svc.TransferCash(invAcct.ID, savings.ID, date, types.MustNewMoney("500.00"), "initial")
		if err != nil {
			t.Fatalf("TransferCash() error = %v", err)
		}

		// Edit: same direction, new amount and memo.
		fixed, err := svc.UpdateTransferCash(
			orig.InvestmentTransaction.ID,
			invAcct.ID, savings.ID, date,
			types.MustNewMoney("750.00"),
			"updated",
			"out",
			transaction.StatusUncleared,
		)
		if err != nil {
			t.Fatalf("UpdateTransferCash() error = %v", err)
		}
		if fixed.InvestmentTransaction.TotalAmount.String() != "-750" {
			t.Errorf("investment amount after edit = %s, want -750", fixed.InvestmentTransaction.TotalAmount.String())
		}
		if fixed.RegularTransaction.Amount.String() != "750" {
			t.Errorf("savings amount after edit = %s, want 750", fixed.RegularTransaction.Amount.String())
		}
		if fixed.InvestmentTransaction.Memo.String != "updated" {
			t.Errorf("memo after edit = %q, want %q", fixed.InvestmentTransaction.Memo.String, "updated")
		}

		// Original rows must be gone.
		if _, err := svc.repo.GetByID(orig.InvestmentTransaction.ID); err == nil {
			t.Error("original investment txn should have been deleted")
		}
		txnRepo := transaction.NewRepository(svc.db)
		if _, err := txnRepo.GetByID(orig.RegularTransaction.ID); err == nil {
			t.Error("original savings-side txn should have been deleted")
		}
	})

	t.Run("invalid direction returns error and original transfer is gone", func(t *testing.T) {
		// Documents current behavior: UpdateTransferCash deletes the old
		// pair before validating direction, so a bad direction string
		// leaves the database with no transfer at all. The TUI never
		// passes an invalid direction (it's read from a 2-option select),
		// so this can only happen via direct service misuse, but it's
		// worth pinning down so a future refactor that swaps validation
		// order doesn't silently change behavior.
		svc, accountRepo := createTestService(t)
		invAcct := createInvAccount(t, accountRepo, "Brokerage")
		savings := createCheckAccount(t, accountRepo, "Savings")
		date := types.NewDate(2024, time.March, 15)

		orig, err := svc.TransferCash(invAcct.ID, savings.ID, date, types.MustNewMoney("100.00"), "")
		if err != nil {
			t.Fatalf("TransferCash() error = %v", err)
		}

		_, err = svc.UpdateTransferCash(
			orig.InvestmentTransaction.ID,
			invAcct.ID, savings.ID, date,
			types.MustNewMoney("100.00"),
			"",
			"sideways",
			transaction.StatusUncleared,
		)
		if err == nil {
			t.Fatal("UpdateTransferCash with invalid direction should return error")
		}
	})
}

// =============================================================================
// UpdateTransferCash — inv↔inv flavor. When both legs are investment accounts
// (e.g., an IRA→IRA rollover originally created via
// TransferCashBetweenInvestments), the edit path must clean up the
// other-investment-side counterpart (not a txnRepo row) and recreate the pair
// via TransferCashBetweenInvestments.
// =============================================================================

func TestUpdateTransferCash_InvToInv_HappyPath(t *testing.T) {
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	dst := createInvAccount(t, accountRepo, "Dest IRA")
	date := types.NewDate(2024, time.March, 15)

	if _, err := svc.Deposit(src.ID, date, types.MustNewMoney("2000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	orig, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("500.00"), "rollover")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	// Edit amount and memo, keep direction (source → destination).
	fixed, err := svc.UpdateTransferCash(
		orig.SourceTransaction.ID,
		src.ID, dst.ID, date,
		types.MustNewMoney("750.00"),
		"rollover (updated)",
		"out",
		transaction.StatusUncleared,
	)
	if err != nil {
		t.Fatalf("UpdateTransferCash() error = %v", err)
	}

	if fixed.InvestmentTransaction == nil {
		t.Fatal("expected primary investment transaction in result")
	}
	if fixed.CounterpartInvestmentTransaction == nil {
		t.Fatal("expected counterpart investment transaction in result for inv↔inv edit")
	}
	if fixed.RegularTransaction != nil {
		t.Errorf("expected RegularTransaction nil for inv↔inv edit, got %+v", fixed.RegularTransaction)
	}

	if fixed.InvestmentTransaction.AccountID != src.ID {
		t.Errorf("primary AccountID = %s, want source %s", fixed.InvestmentTransaction.AccountID, src.ID)
	}
	if fixed.CounterpartInvestmentTransaction.AccountID != dst.ID {
		t.Errorf("counterpart AccountID = %s, want dest %s", fixed.CounterpartInvestmentTransaction.AccountID, dst.ID)
	}
	if fixed.InvestmentTransaction.TotalAmount.String() != "-750" {
		t.Errorf("primary amount = %q, want -750", fixed.InvestmentTransaction.TotalAmount.String())
	}
	if fixed.CounterpartInvestmentTransaction.TotalAmount.String() != "750" {
		t.Errorf("counterpart amount = %q, want 750", fixed.CounterpartInvestmentTransaction.TotalAmount.String())
	}
	if fixed.InvestmentTransaction.Memo.String != "rollover (updated)" {
		t.Errorf("primary memo = %q, want %q", fixed.InvestmentTransaction.Memo.String, "rollover (updated)")
	}
	if fixed.CounterpartInvestmentTransaction.Memo.String != "rollover (updated)" {
		t.Errorf("counterpart memo = %q, want %q", fixed.CounterpartInvestmentTransaction.Memo.String, "rollover (updated)")
	}
	if !fixed.InvestmentTransaction.TransferID.Valid || !fixed.CounterpartInvestmentTransaction.TransferID.Valid {
		t.Error("both legs should be linked as transfers")
	}
	if fixed.InvestmentTransaction.TransferID.ID != fixed.CounterpartInvestmentTransaction.TransferID.ID {
		t.Errorf("transfer_id mismatch between legs: primary=%s counterpart=%s",
			fixed.InvestmentTransaction.TransferID.ID, fixed.CounterpartInvestmentTransaction.TransferID.ID)
	}

	// Original investment rows must be gone (replaced).
	if _, err := svc.repo.GetByID(orig.SourceTransaction.ID); err == nil {
		t.Error("original source transaction should have been deleted by UpdateTransferCash")
	}
	if _, err := svc.repo.GetByID(orig.DestinationTransaction.ID); err == nil {
		t.Error("original destination transaction should have been deleted by UpdateTransferCash")
	}

	// Cash balances reflect the new amount only — no orphan +500 on the destination
	// (the failure mode if we forgot to clean up the old destination leg).
	dstBalance, err := svc.GetCashBalance(dst.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(dst) error = %v", err)
	}
	if dstBalance.String() != "750" {
		t.Errorf("destination balance = %q, want 750 (no leftover from original 500 leg)", dstBalance.String())
	}
	srcBalance, err := svc.GetCashBalance(src.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(src) error = %v", err)
	}
	if srcBalance.String() != "1250" {
		t.Errorf("source balance = %q, want 1250 (2000 deposit - 750 transfer)", srcBalance.String())
	}
}

func TestUpdateTransferCash_InvToInv_FlipDirection(t *testing.T) {
	// User originally moved cash A→B but meant B→A. Edit flips the
	// direction by passing direction="in" (which means "cash arrives at
	// investmentAccountID"), swapping source/destination.
	svc, accountRepo := createTestService(t)
	a := createInvAccount(t, accountRepo, "IRA A")
	b := createInvAccount(t, accountRepo, "IRA B")
	date := types.NewDate(2024, time.March, 15)

	if _, err := svc.Deposit(b.ID, date, types.MustNewMoney("2000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// Wrong-way transfer: A → B (but A has no cash, ends up negative).
	orig, err := svc.TransferCashBetweenInvestments(a.ID, b.ID, date, types.MustNewMoney("400.00"), "")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	// Flip: investmentAccountID=A, regularAccountID=B, direction="in" means
	// cash arrives at A. So new orientation is B → A.
	fixed, err := svc.UpdateTransferCash(
		orig.SourceTransaction.ID,
		a.ID, b.ID, date,
		types.MustNewMoney("400.00"),
		"",
		"in",
		transaction.StatusUncleared,
	)
	if err != nil {
		t.Fatalf("UpdateTransferCash() error = %v", err)
	}

	// After flip: A should be the destination (positive), B the source (negative).
	if fixed.InvestmentTransaction.AccountID != b.ID {
		t.Errorf("primary AccountID = %s, want B %s (source after flip)", fixed.InvestmentTransaction.AccountID, b.ID)
	}
	if fixed.CounterpartInvestmentTransaction.AccountID != a.ID {
		t.Errorf("counterpart AccountID = %s, want A %s (destination after flip)", fixed.CounterpartInvestmentTransaction.AccountID, a.ID)
	}
	if fixed.InvestmentTransaction.TotalAmount.String() != "-400" {
		t.Errorf("source amount = %q, want -400", fixed.InvestmentTransaction.TotalAmount.String())
	}
	if fixed.CounterpartInvestmentTransaction.TotalAmount.String() != "400" {
		t.Errorf("destination amount = %q, want 400", fixed.CounterpartInvestmentTransaction.TotalAmount.String())
	}

	// Final balances: A=+400 (received), B=1600 (2000 deposit - 400 sent).
	aBalance, _ := svc.GetCashBalance(a.ID)
	if aBalance.String() != "400" {
		t.Errorf("A balance = %q, want 400", aBalance.String())
	}
	bBalance, _ := svc.GetCashBalance(b.ID)
	if bBalance.String() != "1600" {
		t.Errorf("B balance = %q, want 1600", bBalance.String())
	}
}

// =============================================================================
// UpdateTransferCash — status threading. The unified Transfer dialog's
// Status-on-Edit radio (P1-008) flows a transaction.Status value through to
// UpdateTransferCash, which must apply the corresponding status to both
// freshly-created legs of the pair: investment-side rows get the matching
// investment.TransactionStatus, regular-side rows get the same transaction.Status.
// Status defaults map: Uncleared↔Pending; Cleared↔Cleared; Reconciled↔Reconciled.
// =============================================================================

func TestUpdateTransferCash_InvToReg_AppliesStatusToBothLegs(t *testing.T) {
	svc, accountRepo := createTestService(t)
	invAcct := createInvAccount(t, accountRepo, "Brokerage")
	savings := createCheckAccount(t, accountRepo, "Savings")
	date := types.NewDate(2024, time.March, 15)

	orig, err := svc.TransferCash(invAcct.ID, savings.ID, date, types.MustNewMoney("500.00"), "")
	if err != nil {
		t.Fatalf("TransferCash() error = %v", err)
	}

	// Edit the same pair, but flip status to Cleared.
	fixed, err := svc.UpdateTransferCash(
		orig.InvestmentTransaction.ID,
		invAcct.ID, savings.ID, date,
		types.MustNewMoney("500.00"),
		"",
		"out",
		transaction.StatusCleared,
	)
	if err != nil {
		t.Fatalf("UpdateTransferCash() error = %v", err)
	}

	if fixed.InvestmentTransaction.Status != TransactionStatusCleared {
		t.Errorf("investment leg status = %q, want %q",
			fixed.InvestmentTransaction.Status, TransactionStatusCleared)
	}
	if fixed.RegularTransaction == nil {
		t.Fatal("expected regular leg in result for inv↔reg edit")
	}
	if fixed.RegularTransaction.Status != transaction.StatusCleared {
		t.Errorf("regular leg status = %q, want %q",
			fixed.RegularTransaction.Status, transaction.StatusCleared)
	}

	// Persisted state matches in-memory result.
	persistedInv, err := svc.repo.GetByID(fixed.InvestmentTransaction.ID)
	if err != nil {
		t.Fatalf("GetByID(investment) error = %v", err)
	}
	if persistedInv.Status != TransactionStatusCleared {
		t.Errorf("persisted investment status = %q, want %q",
			persistedInv.Status, TransactionStatusCleared)
	}
	txnRepo := transaction.NewRepository(svc.db)
	persistedReg, err := txnRepo.GetByID(fixed.RegularTransaction.ID)
	if err != nil {
		t.Fatalf("GetByID(regular) error = %v", err)
	}
	if persistedReg.Status != transaction.StatusCleared {
		t.Errorf("persisted regular status = %q, want %q",
			persistedReg.Status, transaction.StatusCleared)
	}
}

func TestUpdateTransferCash_RegToInv_AppliesStatusToBothLegs(t *testing.T) {
	svc, accountRepo := createTestService(t)
	invAcct := createInvAccount(t, accountRepo, "Brokerage")
	savings := createCheckAccount(t, accountRepo, "Savings")
	date := types.NewDate(2024, time.March, 15)

	orig, err := svc.DepositFromAccount(invAcct.ID, savings.ID, date, types.MustNewMoney("600.00"), "")
	if err != nil {
		t.Fatalf("DepositFromAccount() error = %v", err)
	}

	fixed, err := svc.UpdateTransferCash(
		orig.InvestmentTransaction.ID,
		invAcct.ID, savings.ID, date,
		types.MustNewMoney("600.00"),
		"",
		"in",
		transaction.StatusCleared,
	)
	if err != nil {
		t.Fatalf("UpdateTransferCash() error = %v", err)
	}

	if fixed.InvestmentTransaction.Status != TransactionStatusCleared {
		t.Errorf("investment leg status = %q, want %q",
			fixed.InvestmentTransaction.Status, TransactionStatusCleared)
	}
	if fixed.RegularTransaction == nil {
		t.Fatal("expected regular leg in result for reg→inv edit")
	}
	if fixed.RegularTransaction.Status != transaction.StatusCleared {
		t.Errorf("regular leg status = %q, want %q",
			fixed.RegularTransaction.Status, transaction.StatusCleared)
	}
}

func TestUpdateTransferCash_InvToInv_AppliesStatusToBothLegs(t *testing.T) {
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "Source IRA")
	dst := createInvAccount(t, accountRepo, "Dest IRA")
	date := types.NewDate(2024, time.March, 15)

	if _, err := svc.Deposit(src.ID, date, types.MustNewMoney("2000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	orig, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("500.00"), "rollover")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	fixed, err := svc.UpdateTransferCash(
		orig.SourceTransaction.ID,
		src.ID, dst.ID, date,
		types.MustNewMoney("500.00"),
		"rollover",
		"out",
		transaction.StatusCleared,
	)
	if err != nil {
		t.Fatalf("UpdateTransferCash() error = %v", err)
	}

	if fixed.InvestmentTransaction == nil || fixed.CounterpartInvestmentTransaction == nil {
		t.Fatal("expected both inv legs in result for inv↔inv edit")
	}
	if fixed.InvestmentTransaction.Status != TransactionStatusCleared {
		t.Errorf("primary inv leg status = %q, want %q",
			fixed.InvestmentTransaction.Status, TransactionStatusCleared)
	}
	if fixed.CounterpartInvestmentTransaction.Status != TransactionStatusCleared {
		t.Errorf("counterpart inv leg status = %q, want %q",
			fixed.CounterpartInvestmentTransaction.Status, TransactionStatusCleared)
	}

	persistedSrc, err := svc.repo.GetByID(fixed.InvestmentTransaction.ID)
	if err != nil {
		t.Fatalf("GetByID(src) error = %v", err)
	}
	if persistedSrc.Status != TransactionStatusCleared {
		t.Errorf("persisted src status = %q, want %q",
			persistedSrc.Status, TransactionStatusCleared)
	}
	persistedDst, err := svc.repo.GetByID(fixed.CounterpartInvestmentTransaction.ID)
	if err != nil {
		t.Fatalf("GetByID(dst) error = %v", err)
	}
	if persistedDst.Status != TransactionStatusCleared {
		t.Errorf("persisted dst status = %q, want %q",
			persistedDst.Status, TransactionStatusCleared)
	}
}

// =============================================================================
// DeleteTransaction — exercises the cascade for paired transfer rows.
// The plain investmentRepo.Delete is a low-level primitive that leaves the
// regular-side (for transfer_cash) or other-investment-side (for
// transfer_shares) counterpart orphaned, which is what the user hit when
// pressing `d` on a wrong-direction cash transfer in the investment register.
// =============================================================================

func TestService_DeleteTransaction_CashTransferCascadesToRegularSide(t *testing.T) {
	svc, accountRepo := createTestService(t)
	invAcct := createInvAccount(t, accountRepo, "Brokerage")
	savings := createCheckAccount(t, accountRepo, "Savings")
	date := types.NewDate(2024, time.March, 15)

	result, err := svc.TransferCash(invAcct.ID, savings.ID, date, types.MustNewMoney("1170.33"), "to savings")
	if err != nil {
		t.Fatalf("TransferCash() error = %v", err)
	}

	if err := svc.DeleteTransaction(result.InvestmentTransaction.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	// Investment side gone.
	if _, err := svc.repo.GetByID(result.InvestmentTransaction.ID); err == nil {
		t.Error("investment-side transaction should have been deleted")
	}

	// Savings side gone too — no orphan with a dangling transfer_id.
	txnRepo := transaction.NewRepository(svc.db)
	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(savingsTxns) != 0 {
		t.Errorf("expected savings account to be empty after delete, got %d txns (orphaned counterparts)", len(savingsTxns))
	}
}

func TestService_DeleteTransaction_ShareTransferCascadesToOtherInvestmentSide(t *testing.T) {
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source Brokerage")
	dst := createInvAccount(t, env.accountRepo, "Dest Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(src.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	totalAmt := types.MustNewMoney("5000.00")
	if _, err := env.svc.Buy(src.ID, sec.ID, date, types.MustNewQuantity("50"), &totalAmt, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	xfer, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, date, types.MustNewQuantity("20"), "rollover", nil)
	if err != nil {
		t.Fatalf("TransferShares() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(xfer.SourceTransaction.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	if _, err := env.svc.repo.GetByID(xfer.SourceTransaction.ID); err == nil {
		t.Error("source-side share-transfer txn should have been deleted")
	}
	if _, err := env.svc.repo.GetByID(xfer.DestinationTransaction.ID); err == nil {
		t.Error("destination-side share-transfer txn should have been deleted (counterpart cascade)")
	}
}

func TestService_DeleteTransaction_InvToInvCashTransferCascadesToOtherInvestmentSide(t *testing.T) {
	// Inv↔inv cash transfers (e.g. IRA→IRA rollovers) store both legs in the
	// investment repo. Deleting either leg must cascade to remove its
	// counterpart — otherwise the user is left with a phantom credit on the
	// destination account.
	svc, accountRepo := createTestService(t)
	src := createInvAccount(t, accountRepo, "E*Trade IRA")
	dst := createInvAccount(t, accountRepo, "Wealthfront IRA")
	date := types.NewDate(2024, time.March, 15)

	xfer, err := svc.TransferCashBetweenInvestments(src.ID, dst.ID, date, types.MustNewMoney("2000.00"), "rollover")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments() error = %v", err)
	}

	if err := svc.DeleteTransaction(xfer.SourceTransaction.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	if _, err := svc.repo.GetByID(xfer.SourceTransaction.ID); err == nil {
		t.Error("source-side cash-transfer txn should have been deleted")
	}
	if _, err := svc.repo.GetByID(xfer.DestinationTransaction.ID); err == nil {
		t.Error("destination-side cash-transfer txn should have been deleted (counterpart cascade)")
	}
}

func TestService_DeleteTransaction_NonTransfer_DeletesSingleRow(t *testing.T) {
	// Sanity check: for non-transfer types, DeleteTransaction behaves
	// like the underlying repo delete — one row out, no cascade lookups.
	svc, accountRepo := createTestService(t)
	invAcct := createInvAccount(t, accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	dep, err := svc.Deposit(invAcct.ID, date, types.MustNewMoney("1000.00"), "")
	if err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	if err := svc.DeleteTransaction(dep.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}
	if _, err := svc.repo.GetByID(dep.ID); err == nil {
		t.Error("deposit transaction should have been deleted")
	}
}

// =============================================================================
// DeleteTransaction — reverses position/lot side-effects, not just the row.
// Regression for the reported bug: deleting a (wrong-security) sell in an
// investment account left the holding short by the sold quantity, because the
// delete path removed the row + junctions but never reversed the forward
// position/lot mutation. DeleteTransaction must mirror the Update* path's
// reverse-then-delete contract.
// =============================================================================

func TestService_DeleteTransaction_LotTracked_SellRestoresLotShares(t *testing.T) {
	// The exact reported scenario: a lot-tracked account, a sell entered against
	// the wrong security, then deleted. The drained lot must be restored to its
	// full original quantity and reopened.
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VNQ")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("2000.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("20"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected 1 open lot after buy, got %d", len(lots))
	}
	lotID := lots[0].ID

	sellPPS := types.MustNewMoney("58.16")
	sell, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("11.08311"), nil, &sellPPS, types.ZeroMoney, "",
		[]SellLotAllocation{{LotID: lotID, Shares: types.MustNewQuantity("11.08311")}})
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(sell.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	lot, err := env.lotRepo.GetByID(lotID)
	if err != nil {
		t.Fatalf("GetByID(lot) error = %v", err)
	}
	if lot.Shares.String() != "20" {
		t.Errorf("lot shares after sell-delete = %q, want %q (sell effect not reversed)", lot.Shares.String(), "20")
	}
	if lot.Closed {
		t.Error("lot should be reopened (closed=false) after its only sell was deleted")
	}

	total, err := env.svc.TotalSharesForSecurity(sec.ID)
	if err != nil {
		t.Fatalf("TotalSharesForSecurity() error = %v", err)
	}
	if total.String() != "20" {
		t.Errorf("total shares after sell-delete = %q, want %q", total.String(), "20")
	}
}

func TestService_DeleteTransaction_NonLot_SellRestoresPosition(t *testing.T) {
	// Same bug on an average-cost (non-lot) account: deleting a sell must add
	// the shares back to the aggregate position.
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "401k") // non-lot
	sec := createSec(t, env.secRepo, "VWO")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("2000.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("20"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	sellPPS := types.MustNewMoney("58.16")
	sell, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"), nil, &sellPPS, types.ZeroMoney, "", nil)
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(sell.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if pos.Shares.String() != "20" {
		t.Errorf("position shares after sell-delete = %q, want %q (sell effect not reversed)", pos.Shares.String(), "20")
	}
}

func TestService_DeleteTransaction_LotTracked_BuyRemovesOrphanLot(t *testing.T) {
	// Deleting a buy in a lot-tracked account must remove the lot it opened —
	// otherwise the lot is orphaned (its source txn is gone) but still open and
	// still counted in every open-lot sum.
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("2000.00")
	buy, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("20"), &buyTotal, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(buy.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	if _, err := env.lotRepo.GetBySourceTransaction(buy.ID); err == nil {
		t.Error("lot opened by the deleted buy should have been removed (orphan lot left behind)")
	}
	total, err := env.svc.TotalSharesForSecurity(sec.ID)
	if err != nil {
		t.Fatalf("TotalSharesForSecurity() error = %v", err)
	}
	if !total.IsZero() {
		t.Errorf("total shares after buy-delete = %q, want 0 (orphan lot still counted)", total.String())
	}
}

func TestService_DeleteTransaction_LotTracked_BuySoldAgainstRefused(t *testing.T) {
	// Deleting a buy whose lot was later sold against must be refused: removing
	// the lot would leave the surviving sell's junction pointing at a missing
	// lot and corrupt cost basis. (Same guard the Update path applies.)
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("2000.00")
	buy, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("20"), &buyTotal, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	lotID := lots[0].ID
	sellPPS := types.MustNewMoney("110.00")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"), nil, &sellPPS, types.ZeroMoney, "",
		[]SellLotAllocation{{LotID: lotID, Shares: types.MustNewQuantity("5")}}); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(buy.ID); err == nil {
		t.Error("deleting a buy whose lot was sold against should be refused")
	}
	// The refused delete must be atomic: the buy and its lot still exist.
	if _, err := env.svc.repo.GetByID(buy.ID); err != nil {
		t.Errorf("buy txn should still exist after a refused delete: %v", err)
	}
	if _, err := env.lotRepo.GetBySourceTransaction(buy.ID); err != nil {
		t.Errorf("buy lot should still exist after a refused delete: %v", err)
	}
}

func TestService_DeleteTransaction_NonLot_ShareTransferRestoresBothPositions(t *testing.T) {
	// A share transfer touches two accounts; deleting either leg must reverse
	// both sides — restore the source position and remove the destination's.
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source Brokerage")
	dst := createInvAccount(t, env.accountRepo, "Dest Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(src.ID, date, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("5000.00")
	if _, err := env.svc.Buy(src.ID, sec.ID, date, types.MustNewQuantity("50"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	xfer, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, date, types.MustNewQuantity("20"), "rollover", nil)
	if err != nil {
		t.Fatalf("TransferShares() error = %v", err)
	}

	if err := env.svc.DeleteTransaction(xfer.SourceTransaction.ID); err != nil {
		t.Fatalf("DeleteTransaction() error = %v", err)
	}

	srcPos, err := env.positionRepo.GetByAccountAndSecurity(src.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity(src) error = %v", err)
	}
	if srcPos.Shares.String() != "50" {
		t.Errorf("source shares after transfer-delete = %q, want %q (transfer not reversed)", srcPos.Shares.String(), "50")
	}
	dstPos, err := env.positionRepo.GetByAccountAndSecurity(dst.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity(dst) error = %v", err)
	}
	if !dstPos.Shares.IsZero() {
		t.Errorf("dest shares after transfer-delete = %q, want 0 (received shares not removed)", dstPos.Shares.String())
	}
}

// =============================================================================
// SM-090: Share transfer between investment accounts (non-lot)
// =============================================================================

func TestService_TransferShares_NonLot(t *testing.T) {
	t.Run("transfer reduces source position and increases destination position", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source Brokerage")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash and buy shares in source account
		_, err := env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		totalAmt := types.MustNewMoney("5000.00")
		_, err = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("50"), &totalAmt, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Transfer 20 shares from source to destination
		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("20"), "Move shares", nil)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify result has both transactions
		if result.SourceTransaction == nil {
			t.Fatal("Expected non-nil source transaction")
		}
		if result.DestinationTransaction == nil {
			t.Fatal("Expected non-nil destination transaction")
		}

		// Verify source transaction
		if result.SourceTransaction.Type != TransactionTypeTransferShares {
			t.Errorf("Expected type transfer_shares, got %s", result.SourceTransaction.Type)
		}
		if result.SourceTransaction.AccountID != srcAcct.ID {
			t.Errorf("Expected source account ID %s, got %s", srcAcct.ID, result.SourceTransaction.AccountID)
		}
		if result.SourceTransaction.Shares.Quantity.String() != "20" {
			t.Errorf("Expected shares '20', got %q", result.SourceTransaction.Shares.Quantity.String())
		}
		if result.SourceTransaction.SecurityID.ID != sec.ID {
			t.Errorf("Expected security ID %s, got %s", sec.ID, result.SourceTransaction.SecurityID.ID)
		}

		// Verify destination transaction
		if result.DestinationTransaction.Type != TransactionTypeTransferShares {
			t.Errorf("Expected type transfer_shares, got %s", result.DestinationTransaction.Type)
		}
		if result.DestinationTransaction.AccountID != dstAcct.ID {
			t.Errorf("Expected dest account ID %s, got %s", dstAcct.ID, result.DestinationTransaction.AccountID)
		}
		if result.DestinationTransaction.Shares.Quantity.String() != "20" {
			t.Errorf("Expected shares '20', got %q", result.DestinationTransaction.Shares.Quantity.String())
		}

		// Verify linked by transfer_id
		if !result.SourceTransaction.IsTransfer() {
			t.Error("Source transaction should be a transfer")
		}
		if !result.DestinationTransaction.IsTransfer() {
			t.Error("Destination transaction should be a transfer")
		}
		if result.SourceTransaction.TransferID.ID != result.DestinationTransaction.TransferID.ID {
			t.Error("Transfer IDs should match")
		}
		if result.SourceTransaction.TransferAccountID.ID != dstAcct.ID {
			t.Error("Source transfer_account_id should point to destination account")
		}
		if result.DestinationTransaction.TransferAccountID.ID != srcAcct.ID {
			t.Error("Destination transfer_account_id should point to source account")
		}

		// Verify source position reduced to 30 shares
		srcPos, err := env.positionRepo.GetByAccountAndSecurity(srcAcct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if srcPos.Shares.String() != "30" {
			t.Errorf("Expected source shares '30', got %q", srcPos.Shares.String())
		}
		// Average cost should remain at 100
		if srcPos.AverageCostPerShare.String() != "100" {
			t.Errorf("Expected source avg cost '100', got %q", srcPos.AverageCostPerShare.String())
		}

		// Verify destination position has 20 shares with cost basis from source
		dstPos, err := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if dstPos.Shares.String() != "20" {
			t.Errorf("Expected dest shares '20', got %q", dstPos.Shares.String())
		}
		// Average cost per share should carry over from source (100)
		if dstPos.AverageCostPerShare.String() != "100" {
			t.Errorf("Expected dest avg cost '100', got %q", dstPos.AverageCostPerShare.String())
		}

		// Verify no cash movement in either account
		srcCash, _ := env.svc.GetCashBalance(srcAcct.ID)
		if srcCash.String() != "5000" {
			t.Errorf("Expected source cash '5000', got %q", srcCash.String())
		}
		dstCash, _ := env.svc.GetCashBalance(dstAcct.ID)
		if dstCash.String() != "0" {
			t.Errorf("Expected dest cash '0', got %q", dstCash.String())
		}
	})

	t.Run("transfer all shares removes source position", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash and buy shares
		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("5000.00"), "")
		totalAmt := types.MustNewMoney("5000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &totalAmt, nil, types.ZeroMoney, "")

		// Transfer all 10 shares
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Source position should be gone (zero shares)
		srcPos, err := env.positionRepo.GetByAccountAndSecurity(srcAcct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !srcPos.Shares.IsZero() {
			t.Errorf("Expected source shares '0', got %q", srcPos.Shares.String())
		}

		// Destination should have all 10 shares
		dstPos, err := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if dstPos.Shares.String() != "10" {
			t.Errorf("Expected dest shares '10', got %q", dstPos.Shares.String())
		}
		if dstPos.AverageCostPerShare.String() != "500" {
			t.Errorf("Expected dest avg cost '500', got %q", dstPos.AverageCostPerShare.String())
		}
	})

	t.Run("transfer to existing position merges cost basis", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		// Buy 10 shares at $200 in source
		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		srcTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &srcTotal, nil, types.ZeroMoney, "")

		// Buy 10 shares at $100 in destination
		_, _ = env.svc.Deposit(dstAcct.ID, date, types.MustNewMoney("10000.00"), "")
		dstTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &dstTotal, nil, types.ZeroMoney, "")

		// Transfer 5 shares from source to destination (5 shares at $200 avg cost)
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", nil)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Source should have 5 shares at $200
		srcPos, _ := env.positionRepo.GetByAccountAndSecurity(srcAcct.ID, sec.ID)
		if srcPos.Shares.String() != "5" {
			t.Errorf("Expected source shares '5', got %q", srcPos.Shares.String())
		}

		// Destination should have 15 shares with merged cost basis
		// Existing: 10 shares × $100 = $1000
		// Transferred: 5 shares × $200 = $1000
		// Total: 15 shares, $2000 total cost → $133.333... avg
		dstPos, _ := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if dstPos.Shares.String() != "15" {
			t.Errorf("Expected dest shares '15', got %q", dstPos.Shares.String())
		}
		// Cost basis should be preserved: (10*100 + 5*200) / 15 ≈ 133.33 avg
		// Due to decimal division precision, check within tolerance
		actualCostBasis := dstPos.CostBasis()
		expectedCostBasis := types.MustNewMoney("2000.00")
		diff := actualCostBasis.Sub(expectedCostBasis).Abs()
		tolerance := types.MustNewMoney("0.01")
		if diff.Cmp(tolerance) > 0 {
			t.Errorf("Expected dest cost basis ~'%s', got '%s' (diff: %s)", expectedCostBasis.String(), actualCostBasis.String(), diff.String())
		}
	})

	t.Run("transfer rejects insufficient shares", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "TSLA")
		date := types.NewDate(2024, time.March, 15)

		// Buy only 5 shares
		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("5000.00"), "")
		totalAmt := types.MustNewMoney("500.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("5"), &totalAmt, nil, types.ZeroMoney, "")

		// Try to transfer 10
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err == nil {
			t.Fatal("Expected error for insufficient shares")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected InsufficientSharesError, got %T: %v", err, err)
		}
	})

	t.Run("transfer rejects non-positive shares", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "NFLX")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("0"), "", nil)
		if err == nil {
			t.Fatal("Expected error for zero shares")
		}

		_, err = env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("-5"), "", nil)
		if err == nil {
			t.Fatal("Expected error for negative shares")
		}
	})

	t.Run("transfer rejects non-investment source account", func(t *testing.T) {
		env := createFullTestService(t)
		checkAcct := createCheckAccount(t, env.accountRepo, "Checking")
		invAcct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AMD")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.TransferShares(checkAcct.ID, invAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err == nil {
			t.Fatal("Expected error for non-investment source account")
		}
	})

	t.Run("transfer rejects non-investment destination account", func(t *testing.T) {
		env := createFullTestService(t)
		invAcct := createInvAccount(t, env.accountRepo, "Brokerage")
		checkAcct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "NVDA")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.TransferShares(invAcct.ID, checkAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err == nil {
			t.Fatal("Expected error for non-investment destination account")
		}
	})

	t.Run("transfer rejects same account", func(t *testing.T) {
		env := createFullTestService(t)
		invAcct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "META")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.TransferShares(invAcct.ID, invAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err == nil {
			t.Fatal("Expected error for same account transfer")
		}
	})

	t.Run("transfer with memo sets memo on both transactions", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "AMZN")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		totalAmt := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &totalAmt, nil, types.ZeroMoney, "")

		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "Account consolidation", nil)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		if result.SourceTransaction.Memo.String != "Account consolidation" {
			t.Errorf("Expected source memo 'Account consolidation', got %q", result.SourceTransaction.Memo.String)
		}
		if result.DestinationTransaction.Memo.String != "Account consolidation" {
			t.Errorf("Expected dest memo 'Account consolidation', got %q", result.DestinationTransaction.Memo.String)
		}
	})

	t.Run("transfer with no position in source returns insufficient shares", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createInvAccount(t, env.accountRepo, "Source")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "INTC")
		date := types.NewDate(2024, time.March, 15)

		// No shares bought in source, try to transfer
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", nil)
		if err == nil {
			t.Fatal("Expected error for no position")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected InsufficientSharesError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-091: Share transfer between investment accounts (lot-tracking)
// =============================================================================

func TestService_TransferShares_LotTracking(t *testing.T) {
	t.Run("transfer reduces source lots and creates destination lots", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "AAPL")
		buyDate := types.NewDate(2024, time.March, 15)

		// Deposit cash and buy shares in source account
		_, err := env.svc.Deposit(srcAcct.ID, buyDate, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		buyTotal := types.MustNewMoney("1850.00")
		_, err = env.svc.Buy(srcAcct.ID, sec.ID, buyDate, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Get the source lot
		lots, err := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}

		// Transfer 5 shares from source to destination
		transferDate := types.NewDate(2024, time.June, 1)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, transferDate, types.MustNewQuantity("5"), "Consolidate", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify result
		if result.SourceTransaction == nil || result.DestinationTransaction == nil {
			t.Fatal("Expected non-nil transactions")
		}
		if result.SourceTransaction.Type != TransactionTypeTransferShares {
			t.Errorf("Expected type transfer_shares, got %s", result.SourceTransaction.Type)
		}

		// Verify source lot reduced to 5 shares
		updatedLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(updatedLots) != 1 {
			t.Fatalf("Expected 1 open source lot, got %d", len(updatedLots))
		}
		if updatedLots[0].Shares.String() != "5" {
			t.Errorf("Expected source lot shares '5', got %q", updatedLots[0].Shares.String())
		}

		// Verify destination lot created with original purchase_date and cost_per_share
		dstLots, _ := env.lotRepo.ListByAccountAndSecurity(dstAcct.ID, sec.ID, false)
		if len(dstLots) != 1 {
			t.Fatalf("Expected 1 destination lot, got %d", len(dstLots))
		}
		if dstLots[0].Shares.String() != "5" {
			t.Errorf("Expected dest lot shares '5', got %q", dstLots[0].Shares.String())
		}
		if dstLots[0].CostPerShare.String() != "185" {
			t.Errorf("Expected dest lot cost_per_share '185', got %q", dstLots[0].CostPerShare.String())
		}
		// Purchase date should be preserved from the original buy
		if dstLots[0].PurchaseDate.Time().Format("2006-01-02") != "2024-03-15" {
			t.Errorf("Expected dest lot purchase_date '2024-03-15', got %q", dstLots[0].PurchaseDate.Time().Format("2006-01-02"))
		}

		// Verify no cash movement
		srcCash, _ := env.svc.GetCashBalance(srcAcct.ID)
		if srcCash.String() != "8150" {
			t.Errorf("Expected source cash '8150', got %q", srcCash.String())
		}
		dstCash, _ := env.svc.GetCashBalance(dstAcct.ID)
		if dstCash.String() != "0" {
			t.Errorf("Expected dest cash '0', got %q", dstCash.String())
		}
	})

	t.Run("source lot closed when all shares transferred", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("5000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Source lot should be closed
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(openLots) != 0 {
			t.Errorf("Expected 0 open source lots, got %d", len(openLots))
		}
		allLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, true)
		if len(allLots) != 1 {
			t.Fatalf("Expected 1 total source lot (closed), got %d", len(allLots))
		}
		if !allLots[0].Closed {
			t.Error("Expected source lot to be closed")
		}

		// Destination lot should have all 10 shares
		dstLots, _ := env.lotRepo.ListByAccountAndSecurity(dstAcct.ID, sec.ID, false)
		if len(dstLots) != 1 {
			t.Fatalf("Expected 1 dest lot, got %d", len(dstLots))
		}
		if dstLots[0].Shares.String() != "10" {
			t.Errorf("Expected dest lot shares '10', got %q", dstLots[0].Shares.String())
		}
	})

	t.Run("transfer from multiple lots creates multiple destination lots", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "GOOG")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date1, types.MustNewMoney("20000.00"), "")

		// Buy two lots at different prices
		buy1Total := types.MustNewMoney("1000.00") // 10 shares @ $100
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date1, types.MustNewQuantity("10"), &buy1Total, nil, types.ZeroMoney, "")
		buy2Total := types.MustNewMoney("3000.00") // 10 shares @ $300
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date2, types.MustNewQuantity("10"), &buy2Total, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// Transfer 5 from each lot
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("5")},
		}
		transferDate := types.NewDate(2024, time.June, 1)
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, transferDate, types.MustNewQuantity("10"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify source lots each have 5 remaining
		srcLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(srcLots) != 2 {
			t.Fatalf("Expected 2 open source lots, got %d", len(srcLots))
		}
		for _, l := range srcLots {
			if l.Shares.String() != "5" {
				t.Errorf("Expected source lot shares '5', got %q", l.Shares.String())
			}
		}

		// Verify 2 destination lots created with original cost basis
		dstLots, _ := env.lotRepo.ListByAccountAndSecurity(dstAcct.ID, sec.ID, false)
		if len(dstLots) != 2 {
			t.Fatalf("Expected 2 dest lots, got %d", len(dstLots))
		}

		// Verify each dest lot preserves original purchase_date and cost_per_share
		for _, dl := range dstLots {
			if dl.Shares.String() != "5" {
				t.Errorf("Expected dest lot shares '5', got %q", dl.Shares.String())
			}
		}
		// Find which dest lot matches which source lot by cost
		costMap := make(map[string]string)
		for _, dl := range dstLots {
			costMap[dl.CostPerShare.String()] = dl.PurchaseDate.Time().Format("2006-01-02")
		}
		if costMap["100"] != "2024-01-15" {
			t.Errorf("Expected dest lot with cost '100' to have purchase_date '2024-01-15', got %q", costMap["100"])
		}
		if costMap["300"] != "2024-03-15" {
			t.Errorf("Expected dest lot with cost '300' to have purchase_date '2024-03-15', got %q", costMap["300"])
		}
	})

	t.Run("junction records created for source transaction", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "TSLA")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}

		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify junction records for source transaction
		tls, err := env.transactionLotRepo.GetByTransaction(result.SourceTransaction.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("Expected 1 junction record, got %d", len(tls))
		}
		if tls[0].LotID != lots[0].ID {
			t.Errorf("Expected lot ID %s, got %s", lots[0].ID, tls[0].LotID)
		}
		if tls[0].Shares.String() != "5" {
			t.Errorf("Expected junction shares '5', got %q", tls[0].Shares.String())
		}
	})

	t.Run("lot allocation mismatch returns error", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "NFLX")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		// Allocate 3 shares from lot but request transfer of 5
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("3")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", allocs)
		if err == nil {
			t.Fatal("Expected error for allocation mismatch")
		}
		if _, ok := err.(*LotAllocationMismatchError); !ok {
			t.Errorf("Expected LotAllocationMismatchError, got %T: %v", err, err)
		}
	})

	t.Run("lot allocations required for lot-tracking source", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "AMD")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		// Pass nil lot allocations for lot-tracking source
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", nil)
		if err == nil {
			t.Fatal("Expected error for missing lot allocations")
		}
	})

	t.Run("lot not found returns error", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "INTC")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		fakeID := types.NewID()
		allocs := []SellLotAllocation{{LotID: fakeID, Shares: types.MustNewQuantity("5")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", allocs)
		if err == nil {
			t.Fatal("Expected error for non-existent lot")
		}
		if _, ok := err.(*LotNotFoundError); !ok {
			t.Errorf("Expected LotNotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("lot insufficient shares returns error", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createLotTrackingAccount(t, env.accountRepo, "Dest Tax")
		sec := createSec(t, env.secRepo, "META")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("5"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		// Try to transfer 10 from a lot that only has 5
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", allocs)
		if err == nil {
			t.Fatal("Expected error for insufficient lot shares")
		}
		if _, ok := err.(*LotInsufficientSharesError); !ok {
			t.Errorf("Expected LotInsufficientSharesError, got %T: %v", err, err)
		}
	})

	t.Run("lot-tracking source to non-lot destination updates position", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source Tax")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest NonLot")
		sec := createSec(t, env.secRepo, "NVDA")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1850.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("5"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify destination gets a position (not lots)
		dstPos, _ := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if dstPos.Shares.String() != "5" {
			t.Errorf("Expected dest position shares '5', got %q", dstPos.Shares.String())
		}
		if dstPos.AverageCostPerShare.String() != "185" {
			t.Errorf("Expected dest avg cost '185', got %q", dstPos.AverageCostPerShare.String())
		}

		// Verify no lots created in destination
		dstLots, _ := env.lotRepo.ListByAccountAndSecurity(dstAcct.ID, sec.ID, false)
		if len(dstLots) != 0 {
			t.Errorf("Expected 0 dest lots for non-lot account, got %d", len(dstLots))
		}
	})
}

// SM-092: Share transfer: lot-tracking to non-lot-tracking
// =============================================================================

func TestService_TransferShares_LotToNonLot(t *testing.T) {
	t.Run("multiple lots closed and destination position gets aggregated cost basis", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "AAPL")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date1, types.MustNewMoney("20000.00"), "")

		// Buy two lots at different prices
		buy1Total := types.MustNewMoney("1000.00") // 10 shares @ $100
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date1, types.MustNewQuantity("10"), &buy1Total, nil, types.ZeroMoney, "")
		buy2Total := types.MustNewMoney("3000.00") // 10 shares @ $300
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date2, types.MustNewQuantity("10"), &buy2Total, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// Transfer ALL shares from both lots to non-lot destination
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("10")},
		}
		transferDate := types.NewDate(2024, time.June, 1)
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, transferDate, types.MustNewQuantity("20"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify both source lots are closed
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(openLots) != 0 {
			t.Errorf("Expected 0 open source lots, got %d", len(openLots))
		}
		allLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, true)
		if len(allLots) != 2 {
			t.Fatalf("Expected 2 total source lots (both closed), got %d", len(allLots))
		}
		for _, l := range allLots {
			if !l.Closed {
				t.Errorf("Expected source lot %s to be closed", l.ID)
			}
		}

		// Verify destination position has aggregated cost basis
		// Total cost = (10 × $100) + (10 × $300) = $4000
		// Weighted average cost per share = $4000 / 20 = $200
		dstPos, _ := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if dstPos.Shares.String() != "20" {
			t.Errorf("Expected dest position shares '20', got %q", dstPos.Shares.String())
		}
		if dstPos.AverageCostPerShare.String() != "200" {
			t.Errorf("Expected dest avg cost '200', got %q", dstPos.AverageCostPerShare.String())
		}

		// Verify no lots created in destination
		dstLots, _ := env.lotRepo.ListByAccountAndSecurity(dstAcct.ID, sec.ID, false)
		if len(dstLots) != 0 {
			t.Errorf("Expected 0 dest lots for non-lot account, got %d", len(dstLots))
		}
	})

	t.Run("partial transfer from multiple lots preserves remaining source lots", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "MSFT")
		date1 := types.NewDate(2024, time.January, 10)
		date2 := types.NewDate(2024, time.February, 20)

		_, _ = env.svc.Deposit(srcAcct.ID, date1, types.MustNewMoney("20000.00"), "")

		// Lot 1: 10 shares @ $200
		buy1Total := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date1, types.MustNewQuantity("10"), &buy1Total, nil, types.ZeroMoney, "")
		// Lot 2: 10 shares @ $400
		buy2Total := types.MustNewMoney("4000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date2, types.MustNewQuantity("10"), &buy2Total, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		// Transfer 3 from each lot (partial)
		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("3")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("3")},
		}
		transferDate := types.NewDate(2024, time.June, 1)
		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, transferDate, types.MustNewQuantity("6"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify source lots are NOT closed — still have 7 shares each
		srcLots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(srcLots) != 2 {
			t.Fatalf("Expected 2 open source lots, got %d", len(srcLots))
		}
		for _, l := range srcLots {
			if l.Shares.String() != "7" {
				t.Errorf("Expected source lot shares '7', got %q", l.Shares.String())
			}
			if l.Closed {
				t.Errorf("Source lot should not be closed")
			}
		}

		// Verify destination position
		// Total cost = (3 × $200) + (3 × $400) = $600 + $1200 = $1800
		// Average cost per share = $1800 / 6 = $300
		dstPos, _ := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if dstPos.Shares.String() != "6" {
			t.Errorf("Expected dest position shares '6', got %q", dstPos.Shares.String())
		}
		if dstPos.AverageCostPerShare.String() != "300" {
			t.Errorf("Expected dest avg cost '300', got %q", dstPos.AverageCostPerShare.String())
		}
	})

	t.Run("transfer to existing destination position merges cost basis", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("20000.00"), "")
		_, _ = env.svc.Deposit(dstAcct.ID, date, types.MustNewMoney("20000.00"), "")

		// Destination already holds 10 shares @ $100 average
		dstBuyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &dstBuyTotal, nil, types.ZeroMoney, "")

		// Source lot: 10 shares @ $300
		srcBuyTotal := types.MustNewMoney("3000.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &srcBuyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "Consolidate", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify destination position merges correctly
		// Existing: 10 shares × $100 = $1000
		// Incoming: 10 shares × $300 = $3000
		// Total: 20 shares, $4000 cost → $200 avg
		dstPos, _ := env.positionRepo.GetByAccountAndSecurity(dstAcct.ID, sec.ID)
		if dstPos.Shares.String() != "20" {
			t.Errorf("Expected dest position shares '20', got %q", dstPos.Shares.String())
		}
		if dstPos.AverageCostPerShare.String() != "200" {
			t.Errorf("Expected dest avg cost '200', got %q", dstPos.AverageCostPerShare.String())
		}
	})

	t.Run("junction records created for source transaction in mixed transfer", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "TSLA")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date1, types.MustNewMoney("20000.00"), "")

		// Buy two lots
		buy1Total := types.MustNewMoney("500.00") // 5 shares @ $100
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date1, types.MustNewQuantity("5"), &buy1Total, nil, types.ZeroMoney, "")
		buy2Total := types.MustNewMoney("1000.00") // 5 shares @ $200
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date2, types.MustNewQuantity("5"), &buy2Total, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		if len(lots) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(lots))
		}

		allocs := []SellLotAllocation{
			{LotID: lots[0].ID, Shares: types.MustNewQuantity("2")},
			{LotID: lots[1].ID, Shares: types.MustNewQuantity("3")},
		}

		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date2, types.MustNewQuantity("5"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Verify junction records exist for the source transaction
		tls, err := env.transactionLotRepo.GetByTransaction(result.SourceTransaction.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(tls) != 2 {
			t.Fatalf("Expected 2 junction records, got %d", len(tls))
		}

		// Build map of lot ID to shares
		junctionMap := make(map[types.ID]string)
		for _, tl := range tls {
			junctionMap[tl.LotID] = tl.Shares.String()
		}
		if junctionMap[lots[0].ID] != "2" {
			t.Errorf("Expected junction shares '2' for lot 0, got %q", junctionMap[lots[0].ID])
		}
		if junctionMap[lots[1].ID] != "3" {
			t.Errorf("Expected junction shares '3' for lot 1, got %q", junctionMap[lots[1].ID])
		}
	})

	t.Run("no cash movement in mixed transfer", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "META")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")
		_, _ = env.svc.Deposit(dstAcct.ID, date, types.MustNewMoney("5000.00"), "")

		buyTotal := types.MustNewMoney("2000.00") // 10 shares @ $200
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}

		_, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Source cash should be 10000 - 2000 (buy) = 8000, unchanged by transfer
		srcCash, _ := env.svc.GetCashBalance(srcAcct.ID)
		if srcCash.String() != "8000" {
			t.Errorf("Expected source cash '8000', got %q", srcCash.String())
		}

		// Dest cash should be 5000, unchanged by transfer
		dstCash, _ := env.svc.GetCashBalance(dstAcct.ID)
		if dstCash.String() != "5000" {
			t.Errorf("Expected dest cash '5000', got %q", dstCash.String())
		}
	})

	t.Run("transaction records have correct amounts and types", func(t *testing.T) {
		env := createFullTestService(t)
		srcAcct := createLotTrackingAccount(t, env.accountRepo, "Source LotTrack")
		dstAcct := createInvAccount(t, env.accountRepo, "Dest Average")
		sec := createSec(t, env.secRepo, "AMD")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(srcAcct.ID, date, types.MustNewMoney("10000.00"), "")

		// 10 shares @ $150
		buyTotal := types.MustNewMoney("1500.00")
		_, _ = env.svc.Buy(srcAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(srcAcct.ID, sec.ID, false)
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}

		result, err := env.svc.TransferShares(srcAcct.ID, dstAcct.ID, sec.ID, date, types.MustNewQuantity("10"), "Move shares", allocs)
		if err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		// Source transaction: negative cost basis
		if result.SourceTransaction.Type != TransactionTypeTransferShares {
			t.Errorf("Expected source type transfer_shares, got %s", result.SourceTransaction.Type)
		}
		if result.SourceTransaction.TotalAmount.String() != "-1500" {
			t.Errorf("Expected source amount '-1500', got %q", result.SourceTransaction.TotalAmount.String())
		}
		if result.SourceTransaction.Shares.Quantity.String() != "10" {
			t.Errorf("Expected source shares '10', got %q", result.SourceTransaction.Shares.Quantity.String())
		}

		// Destination transaction: positive cost basis
		if result.DestinationTransaction.Type != TransactionTypeTransferShares {
			t.Errorf("Expected dest type transfer_shares, got %s", result.DestinationTransaction.Type)
		}
		if result.DestinationTransaction.TotalAmount.String() != "1500" {
			t.Errorf("Expected dest amount '1500', got %q", result.DestinationTransaction.TotalAmount.String())
		}
		if result.DestinationTransaction.Shares.Quantity.String() != "10" {
			t.Errorf("Expected dest shares '10', got %q", result.DestinationTransaction.Shares.Quantity.String())
		}

		// Both linked via same transfer ID
		if !result.SourceTransaction.TransferID.Valid {
			t.Error("Expected source transaction to have transfer ID")
		}
		if result.SourceTransaction.TransferID.ID != result.DestinationTransaction.TransferID.ID {
			t.Error("Expected matching transfer IDs")
		}
	})
}

// =============================================================================
// SharesBySecurity (used by split-dialog preview)
// =============================================================================

func TestService_SharesBySecurity(t *testing.T) {
	t.Run("aggregates lot-tracking and non-lot-tracking accounts", func(t *testing.T) {
		env := createFullTestService(t)
		nonLot := createInvAccount(t, env.accountRepo, "Brokerage Avg")
		lotAcct := createLotTrackingAccount(t, env.accountRepo, "IRA Lots")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(nonLot.ID, date, types.MustNewMoney("10000.00"), "")
		_, _ = env.svc.Deposit(lotAcct.ID, date, types.MustNewMoney("10000.00"), "")

		nonLotBuy := types.MustNewMoney("750.00")
		if _, err := env.svc.Buy(nonLot.ID, sec.ID, date, types.MustNewQuantity("5"), &nonLotBuy, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(nonLot) error = %v", err)
		}

		lotBuy := types.MustNewMoney("1500.00")
		if _, err := env.svc.Buy(lotAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &lotBuy, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(lotAcct) error = %v", err)
		}

		results, err := env.svc.SharesBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("SharesBySecurity() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 accounts, got %d", len(results))
		}

		// Sorted alphabetically: "Brokerage Avg" before "IRA Lots".
		if results[0].AccountName != "Brokerage Avg" {
			t.Errorf("results[0].AccountName = %q, want %q", results[0].AccountName, "Brokerage Avg")
		}
		if results[0].Shares.String() != "5" {
			t.Errorf("results[0].Shares = %q, want %q", results[0].Shares.String(), "5")
		}
		if results[1].AccountName != "IRA Lots" {
			t.Errorf("results[1].AccountName = %q, want %q", results[1].AccountName, "IRA Lots")
		}
		if results[1].Shares.String() != "10" {
			t.Errorf("results[1].Shares = %q, want %q", results[1].Shares.String(), "10")
		}
	})

	t.Run("does not double-count a lot-tracking account that also has a position row", func(t *testing.T) {
		// In production heal maintains an aggregate position row alongside the
		// lots for lot-tracking accounts. SharesBySecurity must report the lot
		// sum once, not lots + position.
		env := createFullTestService(t)
		lotAcct := createLotTrackingAccount(t, env.accountRepo, "IRA Lots")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(lotAcct.ID, date, types.MustNewMoney("10000.00"), "")
		buy := types.MustNewMoney("1500.00")
		if _, err := env.svc.Buy(lotAcct.ID, sec.ID, date, types.MustNewQuantity("10"), &buy, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy() error = %v", err)
		}
		// Simulate the heal-maintained aggregate position row.
		pos := NewPositionWithShares(lotAcct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("150"))
		if err := env.positionRepo.CreateOrUpdate(&pos); err != nil {
			t.Fatalf("CreateOrUpdate(position) error = %v", err)
		}

		results, err := env.svc.SharesBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("SharesBySecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 account, got %d", len(results))
		}
		if results[0].Shares.String() != "10" {
			t.Errorf("shares = %q, want %q (lot sum, not lot+position)", results[0].Shares.String(), "10")
		}
	})

	t.Run("returns empty slice when no accounts hold the security", func(t *testing.T) {
		env := createFullTestService(t)
		sec := createSec(t, env.secRepo, "NONE")

		results, err := env.svc.SharesBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("SharesBySecurity() error = %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected empty slice, got %d entries", len(results))
		}
	})
}
