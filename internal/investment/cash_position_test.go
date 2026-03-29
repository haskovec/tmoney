package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestServiceWithSecurity creates a test service along with a security repository
// for tests that need to create security-referencing transactions.
func createTestServiceWithSecurity(t *testing.T) (*Service, *account.Repository, *security.Repository) {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo := account.NewRepository(database)
	secRepo := security.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	svc := NewService(invRepo, accountRepo, positionRepo, lotRepo, database)
	return svc, accountRepo, secRepo
}

func createTestSecurity(t *testing.T, repo *security.Repository, ticker string) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, ticker+" Corp", security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

// =============================================================================
// SM-071: CashPosition model
// =============================================================================

func TestCashPosition_NewCashPosition(t *testing.T) {
	accountID := types.NewID()
	cp := NewCashPosition(accountID)

	if cp.AccountID != accountID {
		t.Errorf("Expected account_id %s, got %s", accountID, cp.AccountID)
	}
	if !cp.Balance.IsZero() {
		t.Errorf("Expected zero balance, got %s", cp.Balance.String())
	}
}

func TestCashPosition_Deposit(t *testing.T) {
	t.Run("deposit increases balance", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())

		err := cp.Deposit(types.MustNewMoney("500.00"))
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		if cp.Balance.String() != "500" {
			t.Errorf("Expected balance '500', got %q", cp.Balance.String())
		}
	})

	t.Run("multiple deposits accumulate", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())

		_ = cp.Deposit(types.MustNewMoney("300.00"))
		_ = cp.Deposit(types.MustNewMoney("200.00"))

		if cp.Balance.String() != "500" {
			t.Errorf("Expected balance '500', got %q", cp.Balance.String())
		}
	})

	t.Run("deposit rejects zero amount", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())

		err := cp.Deposit(types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error for zero deposit")
		}
		if _, ok := err.(*InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T", err)
		}
	})

	t.Run("deposit rejects negative amount", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())

		err := cp.Deposit(types.MustNewMoney("-100.00"))
		if err == nil {
			t.Fatal("Expected error for negative deposit")
		}
	})
}

func TestCashPosition_Withdraw(t *testing.T) {
	t.Run("withdraw decreases balance", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())
		_ = cp.Deposit(types.MustNewMoney("1000.00"))

		err := cp.Withdraw(types.MustNewMoney("400.00"))
		if err != nil {
			t.Fatalf("Withdraw() error = %v", err)
		}
		if cp.Balance.String() != "600" {
			t.Errorf("Expected balance '600', got %q", cp.Balance.String())
		}
	})

	t.Run("withdraw exact balance leaves zero", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())
		_ = cp.Deposit(types.MustNewMoney("250.00"))

		err := cp.Withdraw(types.MustNewMoney("250.00"))
		if err != nil {
			t.Fatalf("Withdraw() error = %v", err)
		}
		if !cp.Balance.IsZero() {
			t.Errorf("Expected zero balance, got %s", cp.Balance.String())
		}
	})

	t.Run("withdraw more than balance returns error", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())
		_ = cp.Deposit(types.MustNewMoney("100.00"))

		err := cp.Withdraw(types.MustNewMoney("200.00"))
		if err == nil {
			t.Fatal("Expected error for insufficient cash")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdraw from zero balance returns error", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())

		err := cp.Withdraw(types.MustNewMoney("1.00"))
		if err == nil {
			t.Fatal("Expected error for zero balance withdrawal")
		}
		if _, ok := err.(*InsufficientCashError); !ok {
			t.Errorf("Expected InsufficientCashError, got %T: %v", err, err)
		}
	})

	t.Run("withdraw rejects zero amount", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())
		_ = cp.Deposit(types.MustNewMoney("100.00"))

		err := cp.Withdraw(types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error for zero withdrawal")
		}
	})

	t.Run("withdraw rejects negative amount", func(t *testing.T) {
		cp := NewCashPosition(types.NewID())
		_ = cp.Deposit(types.MustNewMoney("100.00"))

		err := cp.Withdraw(types.MustNewMoney("-50.00"))
		if err == nil {
			t.Fatal("Expected error for negative withdrawal")
		}
	})
}

// =============================================================================
// SM-072: CashPosition computation from transactions
// =============================================================================

func TestGetCashBalance_TransactionTypeCashEffects(t *testing.T) {
	// Tests that GetCashBalance correctly sums all cash-affecting transaction types
	// and ignores non-cash-affecting types.

	t.Run("buy decreases cash (negative total)", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit some cash first
		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("5000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Manually create a buy transaction (negative total = cash outflow)
		buyTxn := NewTransaction(acct.ID, date, TransactionTypeBuy, types.MustNewMoney("-1850.00"))
		buyTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		buyTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
		if err := svc.repo.Create(buyTxn); err != nil {
			t.Fatalf("Create buy txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		// 5000 - 1850 = 3150
		if balance.String() != "3150" {
			t.Errorf("Expected cash balance '3150', got %q", balance.String())
		}
	})

	t.Run("sell increases cash (positive total)", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		// Manually create a sell transaction (positive total = cash inflow)
		sellTxn := NewTransaction(acct.ID, date, TransactionTypeSell, types.MustNewMoney("2000.00"))
		sellTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		sellTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
		if err := svc.repo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "2000" {
			t.Errorf("Expected cash balance '2000', got %q", balance.String())
		}
	})

	t.Run("dividend increases cash", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "GOOG")
		date := types.NewDate(2024, time.June, 15)

		// Dividend is cash-affecting and positive
		divTxn := NewTransaction(acct.ID, date, TransactionTypeDividend, types.MustNewMoney("125.00"))
		divTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		if err := svc.repo.Create(divTxn); err != nil {
			t.Fatalf("Create dividend txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "125" {
			t.Errorf("Expected cash balance '125', got %q", balance.String())
		}
	})

	t.Run("reinvest_dividend does NOT affect cash", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "VTI")
		date := types.NewDate(2024, time.June, 15)

		// Reinvested dividend should not affect cash
		reinvestTxn := NewTransaction(acct.ID, date, TransactionTypeReinvestDividend, types.MustNewMoney("125.00"))
		reinvestTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		reinvestTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("0.5"), Valid: true}
		if err := svc.repo.Create(reinvestTxn); err != nil {
			t.Fatalf("Create reinvest txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero cash balance for reinvest_dividend, got %q", balance.String())
		}
	})

	t.Run("fee_liquidation does NOT affect cash", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "SPY")
		date := types.NewDate(2024, time.June, 15)

		feeLiqTxn := NewTransaction(acct.ID, date, TransactionTypeFeeLiquidation, types.MustNewMoney("-50.00"))
		feeLiqTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		feeLiqTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("0.25"), Valid: true}
		if err := svc.repo.Create(feeLiqTxn); err != nil {
			t.Fatalf("Create fee_liquidation txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero cash balance for fee_liquidation, got %q", balance.String())
		}
	})

	t.Run("transfer_shares does NOT affect cash", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "AMZN")
		date := types.NewDate(2024, time.June, 15)

		transferTxn := NewTransaction(acct.ID, date, TransactionTypeTransferShares, types.ZeroMoney)
		transferTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		transferTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
		if err := svc.repo.Create(transferTxn); err != nil {
			t.Fatalf("Create transfer_shares txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero cash balance for transfer_shares, got %q", balance.String())
		}
	})

	t.Run("transfer_cash affects cash", func(t *testing.T) {
		svc, accountRepo, _ := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.June, 15)

		// Outgoing cash transfer (negative)
		transferTxn := NewTransaction(acct.ID, date, TransactionTypeTransferCash, types.MustNewMoney("-500.00"))
		if err := svc.repo.Create(transferTxn); err != nil {
			t.Fatalf("Create transfer_cash txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if balance.String() != "-500" {
			t.Errorf("Expected cash balance '-500', got %q", balance.String())
		}
	})

	t.Run("exchange does NOT affect cash", func(t *testing.T) {
		svc, accountRepo, secRepo := createTestServiceWithSecurity(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		sec := createTestSecurity(t, secRepo, "NVDA")
		date := types.NewDate(2024, time.June, 15)

		exchangeTxn := NewTransaction(acct.ID, date, TransactionTypeExchange, types.ZeroMoney)
		exchangeTxn.SecurityID = types.NullableID{ID: sec.ID, Valid: true}
		exchangeTxn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
		if err := svc.repo.Create(exchangeTxn); err != nil {
			t.Fatalf("Create exchange txn error = %v", err)
		}

		balance, err := svc.GetCashBalance(acct.ID)
		if err != nil {
			t.Fatalf("GetCashBalance() error = %v", err)
		}
		if !balance.IsZero() {
			t.Errorf("Expected zero cash balance for exchange, got %q", balance.String())
		}
	})

	t.Run("deposit and withdrawal already tested via service methods", func(t *testing.T) {
		// deposit, withdrawal, interest, fee are already thoroughly tested
		// in investment_service_test.go (SM-066 through SM-070).
		// This test verifies they are correctly classified as cash-affecting.
		if !TransactionTypeDeposit.AffectsCash() {
			t.Error("deposit should affect cash")
		}
		if !TransactionTypeWithdrawal.AffectsCash() {
			t.Error("withdrawal should affect cash")
		}
		if !TransactionTypeInterest.AffectsCash() {
			t.Error("interest should affect cash")
		}
		if !TransactionTypeFee.AffectsCash() {
			t.Error("fee should affect cash")
		}
	})

	t.Run("comprehensive cash effect classification", func(t *testing.T) {
		// Verify the complete mapping of transaction types to their cash effect.
		cashAffecting := map[TransactionType]bool{
			TransactionTypeBuy:              true,
			TransactionTypeSell:             true,
			TransactionTypeDividend:         true,
			TransactionTypeReinvestDividend: false,
			TransactionTypeFee:              true,
			TransactionTypeFeeLiquidation:   false,
			TransactionTypeDeposit:          true,
			TransactionTypeWithdrawal:       true,
			TransactionTypeInterest:         true,
			TransactionTypeTransferShares:   false,
			TransactionTypeTransferCash:     true,
			TransactionTypeExchange:         false,
		}

		for txnType, expected := range cashAffecting {
			if txnType.AffectsCash() != expected {
				t.Errorf("TransactionType %s: AffectsCash() = %v, want %v",
					txnType, txnType.AffectsCash(), expected)
			}
		}
	})
}
