package report

import (
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

func createTestReportService(t *testing.T) (*Service, *account.Repository, *transactionRepo) {
	t.Helper()
	database := createTestDB(t)
	accountRepo := account.NewRepository(database)
	txnRepo := &transactionRepo{db: database}
	svc := NewService(accountRepo, database)
	return svc, accountRepo, txnRepo
}

// transactionRepo is a minimal helper for creating transactions in tests.
// We import from the repository package indirectly to avoid circular deps.
type transactionRepo struct {
	db *db.DB
}

func (r *transactionRepo) createTransaction(t *testing.T, accountID types.ID, date types.Date, amount types.Money) {
	t.Helper()
	id := types.NewID()
	now := types.Now()
	query := `INSERT INTO transactions (id, account_id, date, amount, status, created_at, updated_at) VALUES (?, CAST(? AS UUID), ?, ?, 'uncleared', ?, ?)`
	_, err := r.db.Conn().Exec(query, id, accountID.String(), date.Time(), amount.String(), now.Time(), now.Time())
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}
}

func (r *transactionRepo) createTransactionWithCategory(t *testing.T, accountID types.ID, date types.Date, amount types.Money, categoryID types.ID) {
	t.Helper()
	id := types.NewID()
	now := types.Now()
	query := `INSERT INTO transactions (id, account_id, date, amount, category_id, status, created_at, updated_at) VALUES (?, CAST(? AS UUID), ?, ?, CAST(? AS UUID), 'uncleared', ?, ?)`
	_, err := r.db.Conn().Exec(query, id, accountID.String(), date.Time(), amount.String(), categoryID.String(), now.Time(), now.Time())
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}
}

type categoryRepo struct {
	db *db.DB
}

func (r *categoryRepo) createCategory(t *testing.T, name string, catType string) types.ID {
	t.Helper()
	id := types.NewID()
	now := types.Now()
	query := `INSERT INTO categories (id, name, type, system_category, created_at, updated_at) VALUES (?, ?, ?, false, ?, ?)`
	_, err := r.db.Conn().Exec(query, id, name, catType, now.Time(), now.Time())
	if err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}
	return id
}

func TestNewService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)
		if svc == nil {
			t.Error("NewService should not return nil")
		}
	})
}

func TestService_NetWorth_EmptyDatabase(t *testing.T) {
	t.Run("returns zero net worth with no accounts", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Assets) != 0 {
			t.Errorf("Expected 0 assets, got %d", len(report.Assets))
		}
		if len(report.Liabilities) != 0 {
			t.Errorf("Expected 0 liabilities, got %d", len(report.Liabilities))
		}
		if !report.TotalAssets.IsZero() {
			t.Errorf("Expected zero total assets, got %s", report.TotalAssets.String())
		}
		if !report.TotalLiabilities.IsZero() {
			t.Errorf("Expected zero total liabilities, got %s", report.TotalLiabilities.String())
		}
		if !report.NetWorth.IsZero() {
			t.Errorf("Expected zero net worth, got %s", report.NetWorth.String())
		}
	})
}

func TestService_NetWorth_AssetAccounts(t *testing.T) {
	t.Run("calculates net worth from asset accounts with opening balances", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create checking account with opening balance
		checkingBalance, _ := types.NewMoney("1000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", checkingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking account: %v", err)
		}

		// Create savings account with opening balance
		savingsBalance, _ := types.NewMoney("5000.00")
		savings := account.NewAccount("Savings", account.TypeSavings, "USD", savingsBalance, types.Today())
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Failed to create savings account: %v", err)
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Assets) != 2 {
			t.Errorf("Expected 2 assets, got %d", len(report.Assets))
		}

		expectedTotal, _ := types.NewMoney("6000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
		if !report.NetWorth.Equal(expectedTotal) {
			t.Errorf("Expected net worth %s, got %s", expectedTotal.String(), report.NetWorth.String())
		}
	})

	t.Run("includes all asset account types", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		balance, _ := types.NewMoney("1000.00")

		// Create one of each asset type
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", balance, types.Today())
		savings := account.NewAccount("Savings", account.TypeSavings, "USD", balance, types.Today())
		investment := account.NewAccount("Investment", account.TypeInvestment, "USD", balance, types.Today())
		cash := account.NewAccount("Cash", account.TypeCash, "USD", balance, types.Today())
		asset := account.NewAccount("House", account.TypeAsset, "USD", balance, types.Today())

		for _, acct := range []*account.Account{checking, savings, investment, cash, asset} {
			if err := accountRepo.Create(acct); err != nil {
				t.Fatalf("Failed to create account %s: %v", acct.Name, err)
			}
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Assets) != 5 {
			t.Errorf("Expected 5 assets, got %d", len(report.Assets))
		}

		expectedTotal, _ := types.NewMoney("5000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
	})
}

func TestService_NetWorth_LiabilityAccounts(t *testing.T) {
	t.Run("calculates liabilities from credit card and loan accounts", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create credit card with balance (owed)
		ccBalance, _ := types.NewMoney("500.00")
		creditCard := account.NewAccount("Visa", account.TypeCreditCard, "USD", ccBalance, types.Today())
		if err := accountRepo.Create(creditCard); err != nil {
			t.Fatalf("Failed to create credit card: %v", err)
		}

		// Create loan with balance (owed)
		loanBalance, _ := types.NewMoney("10000.00")
		loan := account.NewAccount("Car Loan", account.TypeLoan, "USD", loanBalance, types.Today())
		if err := accountRepo.Create(loan); err != nil {
			t.Fatalf("Failed to create loan: %v", err)
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Liabilities) != 2 {
			t.Errorf("Expected 2 liabilities, got %d", len(report.Liabilities))
		}

		expectedLiabilities, _ := types.NewMoney("10500.00")
		if !report.TotalLiabilities.Equal(expectedLiabilities) {
			t.Errorf("Expected total liabilities %s, got %s", expectedLiabilities.String(), report.TotalLiabilities.String())
		}

		// Net worth should be negative when only liabilities exist
		expectedNetWorth, _ := types.NewMoney("-10500.00")
		if !report.NetWorth.Equal(expectedNetWorth) {
			t.Errorf("Expected net worth %s, got %s", expectedNetWorth.String(), report.NetWorth.String())
		}
	})
}

func TestService_NetWorth_MixedAccounts(t *testing.T) {
	t.Run("calculates net worth with assets and liabilities", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Assets
		checkingBalance, _ := types.NewMoney("5000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", checkingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		savingsBalance, _ := types.NewMoney("10000.00")
		savings := account.NewAccount("Savings", account.TypeSavings, "USD", savingsBalance, types.Today())
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Failed to create savings: %v", err)
		}

		// Liabilities
		ccBalance, _ := types.NewMoney("2000.00")
		creditCard := account.NewAccount("Visa", account.TypeCreditCard, "USD", ccBalance, types.Today())
		if err := accountRepo.Create(creditCard); err != nil {
			t.Fatalf("Failed to create credit card: %v", err)
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		// Total assets: 5000 + 10000 = 15000
		expectedAssets, _ := types.NewMoney("15000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}

		// Total liabilities: 2000
		expectedLiabilities, _ := types.NewMoney("2000.00")
		if !report.TotalLiabilities.Equal(expectedLiabilities) {
			t.Errorf("Expected total liabilities %s, got %s", expectedLiabilities.String(), report.TotalLiabilities.String())
		}

		// Net worth: 15000 - 2000 = 13000
		expectedNetWorth, _ := types.NewMoney("13000.00")
		if !report.NetWorth.Equal(expectedNetWorth) {
			t.Errorf("Expected net worth %s, got %s", expectedNetWorth.String(), report.NetWorth.String())
		}
	})
}

func TestService_NetWorth_WithTransactions(t *testing.T) {
	t.Run("includes transactions in balance calculation", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		// Create checking account with opening balance of 1000
		openingBalance, _ := types.NewMoney("1000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", openingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add some transactions
		deposit, _ := types.NewMoney("500.00")
		txnRepo.createTransaction(t, checking.ID, types.Today(), deposit)

		withdrawal, _ := types.NewMoney("-200.00")
		txnRepo.createTransaction(t, checking.ID, types.Today(), withdrawal)

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		// Balance: 1000 + 500 - 200 = 1300
		expectedBalance, _ := types.NewMoney("1300.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})
}

func TestService_NetWorthAsOf(t *testing.T) {
	t.Run("calculates net worth as of a specific date", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		// Create checking account with opening balance
		openingBalance, _ := types.NewMoney("1000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", openingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add transaction in the past
		pastDate := types.Today().AddDays(-10)
		pastTxnAmount, _ := types.NewMoney("500.00")
		txnRepo.createTransaction(t, checking.ID, pastDate, pastTxnAmount)

		// Add transaction in the future
		futureDate := types.Today().AddDays(10)
		futureTxnAmount, _ := types.NewMoney("1000.00")
		txnRepo.createTransaction(t, checking.ID, futureDate, futureTxnAmount)

		// Net worth as of today should only include past transaction
		asOfDate := time.Now()
		report, err := svc.NetWorthAsOf(asOfDate)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		// Balance: 1000 + 500 = 1500 (future transaction not included)
		expectedBalance, _ := types.NewMoney("1500.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})

	t.Run("calculates net worth as of a past date", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		openingBalance, _ := types.NewMoney("1000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", openingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add transaction 5 days ago
		txnDate := types.Today().AddDays(-5)
		txnAmount, _ := types.NewMoney("500.00")
		txnRepo.createTransaction(t, checking.ID, txnDate, txnAmount)

		// Net worth as of 10 days ago should not include the transaction
		pastDate := types.Today().AddDays(-10).Time()
		report, err := svc.NetWorthAsOf(pastDate)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		// Balance: 1000 (transaction not yet happened)
		expectedBalance, _ := types.NewMoney("1000.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})
}

func TestService_NetWorth_ClosedAccounts(t *testing.T) {
	t.Run("excludes closed accounts by default", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create active account
		activeBalance, _ := types.NewMoney("5000.00")
		active := account.NewAccount("Active Checking", account.TypeChecking, "USD", activeBalance, types.Today())
		if err := accountRepo.Create(active); err != nil {
			t.Fatalf("Failed to create active account: %v", err)
		}

		// Create closed account
		closedBalance := types.ZeroMoney
		closed := account.NewAccount("Closed Checking", account.TypeChecking, "USD", closedBalance, types.Today())
		closed.Close(types.Today())
		if err := accountRepo.Create(closed); err != nil {
			t.Fatalf("Failed to create closed account: %v", err)
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Assets) != 1 {
			t.Errorf("Expected 1 asset (closed account excluded), got %d", len(report.Assets))
		}
	})

	t.Run("includes closed accounts when requested", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create active account
		activeBalance, _ := types.NewMoney("5000.00")
		active := account.NewAccount("Active Checking", account.TypeChecking, "USD", activeBalance, types.Today())
		if err := accountRepo.Create(active); err != nil {
			t.Fatalf("Failed to create active account: %v", err)
		}

		// Create closed account with balance
		closedBalance, _ := types.NewMoney("1000.00")
		closed := account.NewAccount("Closed Checking", account.TypeChecking, "USD", closedBalance, types.Today())
		closed.Close(types.Today())
		if err := accountRepo.Create(closed); err != nil {
			t.Fatalf("Failed to create closed account: %v", err)
		}

		report, err := svc.NetWorthAsOfIncludingClosed(time.Now())
		if err != nil {
			t.Fatalf("NetWorthAsOfIncludingClosed() error = %v", err)
		}

		if len(report.Assets) != 2 {
			t.Errorf("Expected 2 assets (including closed), got %d", len(report.Assets))
		}

		expectedTotal, _ := types.NewMoney("6000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
	})
}

func TestService_NetWorth_ReportContainsAsOfDate(t *testing.T) {
	t.Run("report includes the as-of date", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)

		asOf := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		report, err := svc.NetWorthAsOf(asOf)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		if !report.AsOfDate.Equal(asOf) {
			t.Errorf("Expected AsOfDate %v, got %v", asOf, report.AsOfDate)
		}
	})
}

func TestService_NetWorth_AccountBalanceDetails(t *testing.T) {
	t.Run("account balances include correct details", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		balance, _ := types.NewMoney("5000.00")
		checking := account.NewAccount("My Checking", account.TypeChecking, "USD", balance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		if len(report.Assets) != 1 {
			t.Fatalf("Expected 1 asset, got %d", len(report.Assets))
		}

		asset := report.Assets[0]
		if asset.AccountID != checking.ID {
			t.Errorf("Expected account ID %s, got %s", checking.ID.String(), asset.AccountID.String())
		}
		if asset.Name != "My Checking" {
			t.Errorf("Expected name 'My Checking', got %q", asset.Name)
		}
		if asset.Type != string(account.TypeChecking) {
			t.Errorf("Expected type %s, got %s", account.TypeChecking, asset.Type)
		}
		if !asset.Balance.Equal(balance) {
			t.Errorf("Expected balance %s, got %s", balance.String(), asset.Balance.String())
		}
	})
}

// mockInvestmentValuer is a test double for InvestmentValuer.
type mockInvestmentValuer struct {
	valuations    map[string]types.Money // accountID string -> total value
	missingPrices map[string]bool        // accountID string -> has missing prices
	err           error
}

func (m *mockInvestmentValuer) GetAccountValuation(accountID types.ID, _ types.Date) (*ValuationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	value := types.ZeroMoney
	if v, ok := m.valuations[accountID.String()]; ok {
		value = v
	}
	hasMissing := false
	if m.missingPrices != nil {
		hasMissing = m.missingPrices[accountID.String()]
	}
	return &ValuationResult{
		TotalValue:       value,
		HasMissingPrices: hasMissing,
	}, nil
}

func TestService_NetWorth_InvestmentAccountValuation(t *testing.T) {
	t.Run("uses investment valuer for investment account total value", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		// Create a checking account with $5000
		checkingBalance, _ := types.NewMoney("5000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", checkingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Create an investment account with $1000 opening balance (cash)
		investBalance, _ := types.NewMoney("1000.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		// Mock valuer returns $15000 for the investment account (cash + market value)
		investTotal, _ := types.NewMoney("15000.00")
		valuer := &mockInvestmentValuer{
			valuations: map[string]types.Money{
				invest.ID.String(): investTotal,
			},
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		// Total assets: checking $5000 + investment $15000 = $20000
		expectedAssets, _ := types.NewMoney("20000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}
		if !report.NetWorth.Equal(expectedAssets) {
			t.Errorf("Expected net worth %s, got %s", expectedAssets.String(), report.NetWorth.String())
		}

		// Verify the investment account balance shows valuation, not opening balance
		for _, a := range report.Assets {
			if a.AccountID == invest.ID {
				if !a.Balance.Equal(investTotal) {
					t.Errorf("Expected investment balance %s, got %s", investTotal.String(), a.Balance.String())
				}
			}
		}
	})

	t.Run("falls back to transaction balance when valuer returns error", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		investBalance, _ := types.NewMoney("1000.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		valuer := &mockInvestmentValuer{
			err: fmt.Errorf("valuation failed"),
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		// Should fall back to the transaction-based balance of $1000
		expectedAssets, _ := types.NewMoney("1000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s (fallback), got %s", expectedAssets.String(), report.TotalAssets.String())
		}
	})

	t.Run("without valuer uses transaction-based balance for investment accounts", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		investBalance, _ := types.NewMoney("1000.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		// No valuer provided — should use transaction-based balance
		svc := NewService(accountRepo, database)

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		expectedAssets, _ := types.NewMoney("1000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}
	})

	t.Run("investment valuation with liabilities computes correct net worth", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		// Investment account
		investBalance, _ := types.NewMoney("500.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		// Credit card liability
		ccBalance, _ := types.NewMoney("3000.00")
		cc := account.NewAccount("Visa", account.TypeCreditCard, "USD", ccBalance, types.Today())
		if err := accountRepo.Create(cc); err != nil {
			t.Fatalf("Failed to create credit card: %v", err)
		}

		// Mock valuer: investment worth $25000 (cash + holdings)
		investTotal, _ := types.NewMoney("25000.00")
		valuer := &mockInvestmentValuer{
			valuations: map[string]types.Money{
				invest.ID.String(): investTotal,
			},
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		expectedAssets, _ := types.NewMoney("25000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}

		expectedLiabilities, _ := types.NewMoney("3000.00")
		if !report.TotalLiabilities.Equal(expectedLiabilities) {
			t.Errorf("Expected total liabilities %s, got %s", expectedLiabilities.String(), report.TotalLiabilities.String())
		}

		// Net worth: 25000 - 3000 = 22000
		expectedNetWorth, _ := types.NewMoney("22000.00")
		if !report.NetWorth.Equal(expectedNetWorth) {
			t.Errorf("Expected net worth %s, got %s", expectedNetWorth.String(), report.NetWorth.String())
		}
	})

	t.Run("as-of date is passed to investment valuer", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		investBalance, _ := types.NewMoney("0.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		investTotal, _ := types.NewMoney("10000.00")
		var capturedDate types.Date
		valuer := &dateCapturingValuer{
			value:        investTotal,
			capturedDate: &capturedDate,
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		asOf := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		_, err := svc.NetWorthAsOf(asOf)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		expectedDate := types.NewDate(2024, 6, 15)
		if capturedDate != expectedDate {
			t.Errorf("Expected valuer to receive date %s, got %s", expectedDate.String(), capturedDate.String())
		}
	})
}

// dateCapturingValuer captures the date passed to GetAccountValuation.
type dateCapturingValuer struct {
	value        types.Money
	capturedDate *types.Date
}

func (v *dateCapturingValuer) GetAccountValuation(_ types.ID, asOf types.Date) (*ValuationResult, error) {
	*v.capturedDate = asOf
	return &ValuationResult{TotalValue: v.value}, nil
}

func TestService_NetWorth_MissingPriceFallback(t *testing.T) {
	t.Run("flags account as estimated when holdings have missing prices", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		// Create an investment account
		investBalance, _ := types.NewMoney("500.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		// Mock valuer returns a value but flags missing prices (cost basis used)
		investTotal, _ := types.NewMoney("10000.00")
		valuer := &mockInvestmentValuer{
			valuations: map[string]types.Money{
				invest.ID.String(): investTotal,
			},
			missingPrices: map[string]bool{
				invest.ID.String(): true,
			},
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		// Find the investment account in assets
		var found bool
		for _, a := range report.Assets {
			if a.AccountID == invest.ID {
				found = true
				if !a.EstimatedValue {
					t.Error("Expected EstimatedValue=true for account with missing prices")
				}
				if !a.Balance.Equal(investTotal) {
					t.Errorf("Expected balance %s, got %s", investTotal.String(), a.Balance.String())
				}
			}
		}
		if !found {
			t.Error("Investment account not found in assets")
		}
	})

	t.Run("account not flagged as estimated when all holdings have prices", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		investBalance, _ := types.NewMoney("500.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		// Mock valuer returns a value with no missing prices
		investTotal, _ := types.NewMoney("15000.00")
		valuer := &mockInvestmentValuer{
			valuations: map[string]types.Money{
				invest.ID.String(): investTotal,
			},
			missingPrices: map[string]bool{
				invest.ID.String(): false,
			},
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		for _, a := range report.Assets {
			if a.AccountID == invest.ID {
				if a.EstimatedValue {
					t.Error("Expected EstimatedValue=false when all holdings have pricing")
				}
			}
		}
	})

	t.Run("non-investment accounts never flagged as estimated", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		checkingBalance, _ := types.NewMoney("5000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", checkingBalance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		svc := NewService(accountRepo, database)

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		for _, a := range report.Assets {
			if a.AccountID == checking.ID {
				if a.EstimatedValue {
					t.Error("Non-investment account should not have EstimatedValue=true")
				}
			}
		}
	})

	t.Run("valuer error falls back to transaction balance with no estimated flag", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		investBalance, _ := types.NewMoney("1000.00")
		invest := account.NewAccount("Brokerage", account.TypeInvestment, "USD", investBalance, types.Today())
		if err := accountRepo.Create(invest); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		valuer := &mockInvestmentValuer{
			err: fmt.Errorf("valuation failed"),
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		for _, a := range report.Assets {
			if a.AccountID == invest.ID {
				if a.EstimatedValue {
					t.Error("On valuer error, should not flag EstimatedValue (using transaction balance instead)")
				}
				expectedBalance, _ := types.NewMoney("1000.00")
				if !a.Balance.Equal(expectedBalance) {
					t.Errorf("Expected fallback balance %s, got %s", expectedBalance.String(), a.Balance.String())
				}
			}
		}
	})

	t.Run("mixed accounts with one estimated and one fully priced", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)

		// Investment account 1: has all prices
		inv1Balance, _ := types.NewMoney("0.00")
		inv1 := account.NewAccount("Brokerage A", account.TypeInvestment, "USD", inv1Balance, types.Today())
		if err := accountRepo.Create(inv1); err != nil {
			t.Fatalf("Failed to create investment account 1: %v", err)
		}

		// Investment account 2: has missing prices
		inv2Balance, _ := types.NewMoney("0.00")
		inv2 := account.NewAccount("Brokerage B", account.TypeInvestment, "USD", inv2Balance, types.Today())
		if err := accountRepo.Create(inv2); err != nil {
			t.Fatalf("Failed to create investment account 2: %v", err)
		}

		inv1Total, _ := types.NewMoney("20000.00")
		inv2Total, _ := types.NewMoney("5000.00")
		valuer := &mockInvestmentValuer{
			valuations: map[string]types.Money{
				inv1.ID.String(): inv1Total,
				inv2.ID.String(): inv2Total,
			},
			missingPrices: map[string]bool{
				inv1.ID.String(): false, // all priced
				inv2.ID.String(): true,  // missing prices
			},
		}

		svc := NewService(accountRepo, database, WithInvestmentValuer(valuer))

		report, err := svc.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}

		for _, a := range report.Assets {
			if a.AccountID == inv1.ID {
				if a.EstimatedValue {
					t.Error("Brokerage A should not have EstimatedValue=true (all prices available)")
				}
			}
			if a.AccountID == inv2.ID {
				if !a.EstimatedValue {
					t.Error("Brokerage B should have EstimatedValue=true (missing prices)")
				}
			}
		}

		// Total assets still correct: 20000 + 5000 = 25000
		expectedAssets, _ := types.NewMoney("25000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}
	})
}

func TestService_SpendingByCategoryMonth_EmptyDatabase(t *testing.T) {
	t.Run("returns zero spending with no transactions", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		if len(report.Categories) != 0 {
			t.Errorf("Expected 0 categories, got %d", len(report.Categories))
		}
		if !report.TotalSpending.IsZero() {
			t.Errorf("Expected zero total spending, got %s", report.TotalSpending.String())
		}
		if report.Period != "January 2024" {
			t.Errorf("Expected period 'January 2024', got %q", report.Period)
		}
	})
}

func TestService_SpendingByCategoryMonth_DirectTransactions(t *testing.T) {
	t.Run("calculates spending from direct transactions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		txnRepo := &transactionRepo{db: database}
		catRepo := &categoryRepo{db: database}
		svc := NewService(accountRepo, database)

		// Create account
		balance, _ := types.NewMoney("1000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", balance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceriesID := catRepo.createCategory(t, "Groceries", "expense")

		// Create expense transaction (negative amount)
		txnDate := types.NewDate(2024, 1, 15)
		amount, _ := types.NewMoney("-100.00")
		txnRepo.createTransactionWithCategory(t, checking.ID, txnDate, amount, groceriesID)

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		if len(report.Categories) != 1 {
			t.Fatalf("Expected 1 category, got %d", len(report.Categories))
		}

		expectedSpending, _ := types.NewMoney("100.00")
		if !report.TotalSpending.Equal(expectedSpending) {
			t.Errorf("Expected total spending %s, got %s", expectedSpending.String(), report.TotalSpending.String())
		}

		cat := report.Categories[0]
		if cat.Name != "Groceries" {
			t.Errorf("Expected category 'Groceries', got %q", cat.Name)
		}
		if !cat.Amount.Equal(expectedSpending) {
			t.Errorf("Expected amount %s, got %s", expectedSpending.String(), cat.Amount.String())
		}
		if cat.Percentage != 100.0 {
			t.Errorf("Expected 100%% percentage, got %.1f%%", cat.Percentage)
		}
	})
}

func TestService_SpendingByCategoryYear(t *testing.T) {
	t.Run("calculates spending for entire year", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		txnRepo := &transactionRepo{db: database}
		catRepo := &categoryRepo{db: database}
		svc := NewService(accountRepo, database)

		// Create account
		balance, _ := types.NewMoney("10000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", balance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceriesID := catRepo.createCategory(t, "Groceries", "expense")

		// Create transactions across the year
		months := []time.Month{1, 3, 6, 9, 12}
		for _, month := range months {
			txnDate := types.NewDate(2024, month, 15)
			amount, _ := types.NewMoney("-100.00")
			txnRepo.createTransactionWithCategory(t, checking.ID, txnDate, amount, groceriesID)
		}

		report, err := svc.SpendingByCategoryYear(2024)
		if err != nil {
			t.Fatalf("SpendingByCategoryYear() error = %v", err)
		}

		if report.Period != "2024" {
			t.Errorf("Expected period '2024', got %q", report.Period)
		}

		expectedTotal, _ := types.NewMoney("500.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total spending %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}
	})
}

func TestService_SpendingByCategoryDateRange(t *testing.T) {
	t.Run("calculates spending for custom date range", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		txnRepo := &transactionRepo{db: database}
		catRepo := &categoryRepo{db: database}
		svc := NewService(accountRepo, database)

		// Create account
		balance, _ := types.NewMoney("5000.00")
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", balance, types.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceriesID := catRepo.createCategory(t, "Groceries", "expense")

		// Create transactions
		dates := []struct {
			month time.Month
			day   int
		}{
			{1, 15},
			{2, 15},
			{3, 15},
			{4, 15},
		}
		for _, d := range dates {
			txnDate := types.NewDate(2024, d.month, d.day)
			amount, _ := types.NewMoney("-100.00")
			txnRepo.createTransactionWithCategory(t, checking.ID, txnDate, amount, groceriesID)
		}

		// Query Feb 1 to Mar 31 (should include 2 transactions)
		startDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 3, 31, 23, 59, 59, 999999999, time.UTC)

		report, err := svc.SpendingByCategoryDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("SpendingByCategoryDateRange() error = %v", err)
		}

		expectedTotal, _ := types.NewMoney("200.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total spending %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}

		expectedPeriod := "2024-02-01 to 2024-03-31"
		if report.Period != expectedPeriod {
			t.Errorf("Expected period %q, got %q", expectedPeriod, report.Period)
		}
	})
}
