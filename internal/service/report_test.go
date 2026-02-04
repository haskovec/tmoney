package service

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func createTestReportService(t *testing.T) (*ReportService, *repository.AccountRepository, *repository.TransactionRepository) {
	t.Helper()
	database := createTestDB(t)
	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	svc := NewReportService(accountRepo, database)
	return svc, accountRepo, txnRepo
}

func TestNewReportService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)
		if svc == nil {
			t.Error("NewReportService should not return nil")
		}
	})
}

func TestReportService_NetWorth_EmptyDatabase(t *testing.T) {
	t.Run("returns zero net worth with no accounts", func(t *testing.T) {
		svc, _, _ := createTestReportService(t)

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
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

func TestReportService_NetWorth_AssetAccounts(t *testing.T) {
	t.Run("calculates net worth from asset accounts with opening balances", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create checking account with opening balance
		checkingBalance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", checkingBalance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking account: %v", err)
		}

		// Create savings account with opening balance
		savingsBalance, _ := models.NewMoney("5000.00")
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", savingsBalance, models.Today())
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Failed to create savings account: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		if len(report.Assets) != 2 {
			t.Errorf("Expected 2 assets, got %d", len(report.Assets))
		}

		expectedTotal, _ := models.NewMoney("6000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
		if !report.NetWorth.Equal(expectedTotal) {
			t.Errorf("Expected net worth %s, got %s", expectedTotal.String(), report.NetWorth.String())
		}
	})

	t.Run("includes all asset account types", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		balance, _ := models.NewMoney("1000.00")

		// Create one of each asset type
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", balance, models.Today())
		investment := models.NewAccount("Investment", models.AccountTypeInvestment, "USD", balance, models.Today())
		cash := models.NewAccount("Cash", models.AccountTypeCash, "USD", balance, models.Today())
		asset := models.NewAccount("House", models.AccountTypeAsset, "USD", balance, models.Today())

		for _, acct := range []*models.Account{checking, savings, investment, cash, asset} {
			if err := accountRepo.Create(acct); err != nil {
				t.Fatalf("Failed to create account %s: %v", acct.Name, err)
			}
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		if len(report.Assets) != 5 {
			t.Errorf("Expected 5 assets, got %d", len(report.Assets))
		}

		expectedTotal, _ := models.NewMoney("5000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
	})
}

func TestReportService_NetWorth_LiabilityAccounts(t *testing.T) {
	t.Run("calculates liabilities from credit card and loan accounts", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create credit card with balance (owed)
		ccBalance, _ := models.NewMoney("500.00")
		creditCard := models.NewAccount("Visa", models.AccountTypeCreditCard, "USD", ccBalance, models.Today())
		if err := accountRepo.Create(creditCard); err != nil {
			t.Fatalf("Failed to create credit card: %v", err)
		}

		// Create loan with balance (owed)
		loanBalance, _ := models.NewMoney("10000.00")
		loan := models.NewAccount("Car Loan", models.AccountTypeLoan, "USD", loanBalance, models.Today())
		if err := accountRepo.Create(loan); err != nil {
			t.Fatalf("Failed to create loan: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		if len(report.Liabilities) != 2 {
			t.Errorf("Expected 2 liabilities, got %d", len(report.Liabilities))
		}

		expectedLiabilities, _ := models.NewMoney("10500.00")
		if !report.TotalLiabilities.Equal(expectedLiabilities) {
			t.Errorf("Expected total liabilities %s, got %s", expectedLiabilities.String(), report.TotalLiabilities.String())
		}

		// Net worth should be negative when only liabilities exist
		expectedNetWorth, _ := models.NewMoney("-10500.00")
		if !report.NetWorth.Equal(expectedNetWorth) {
			t.Errorf("Expected net worth %s, got %s", expectedNetWorth.String(), report.NetWorth.String())
		}
	})
}

func TestReportService_NetWorth_MixedAccounts(t *testing.T) {
	t.Run("calculates net worth with assets and liabilities", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Assets
		checkingBalance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", checkingBalance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		savingsBalance, _ := models.NewMoney("10000.00")
		savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", savingsBalance, models.Today())
		if err := accountRepo.Create(savings); err != nil {
			t.Fatalf("Failed to create savings: %v", err)
		}

		// Liabilities
		ccBalance, _ := models.NewMoney("2000.00")
		creditCard := models.NewAccount("Visa", models.AccountTypeCreditCard, "USD", ccBalance, models.Today())
		if err := accountRepo.Create(creditCard); err != nil {
			t.Fatalf("Failed to create credit card: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		// Total assets: 5000 + 10000 = 15000
		expectedAssets, _ := models.NewMoney("15000.00")
		if !report.TotalAssets.Equal(expectedAssets) {
			t.Errorf("Expected total assets %s, got %s", expectedAssets.String(), report.TotalAssets.String())
		}

		// Total liabilities: 2000
		expectedLiabilities, _ := models.NewMoney("2000.00")
		if !report.TotalLiabilities.Equal(expectedLiabilities) {
			t.Errorf("Expected total liabilities %s, got %s", expectedLiabilities.String(), report.TotalLiabilities.String())
		}

		// Net worth: 15000 - 2000 = 13000
		expectedNetWorth, _ := models.NewMoney("13000.00")
		if !report.NetWorth.Equal(expectedNetWorth) {
			t.Errorf("Expected net worth %s, got %s", expectedNetWorth.String(), report.NetWorth.String())
		}
	})
}

func TestReportService_NetWorth_WithTransactions(t *testing.T) {
	t.Run("includes transactions in balance calculation", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		// Create checking account with opening balance of 1000
		openingBalance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", openingBalance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add some transactions
		deposit, _ := models.NewMoney("500.00")
		txn1 := models.NewTransaction(checking.ID, models.Today(), deposit)
		if err := txnRepo.Create(txn1); err != nil {
			t.Fatalf("Failed to create deposit: %v", err)
		}

		withdrawal, _ := models.NewMoney("-200.00")
		txn2 := models.NewTransaction(checking.ID, models.Today(), withdrawal)
		if err := txnRepo.Create(txn2); err != nil {
			t.Fatalf("Failed to create withdrawal: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		// Balance: 1000 + 500 - 200 = 1300
		expectedBalance, _ := models.NewMoney("1300.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})
}

func TestReportService_NetWorthAsOf(t *testing.T) {
	t.Run("calculates net worth as of a specific date", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		// Create checking account with opening balance
		openingBalance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", openingBalance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add transaction in the past
		pastDate := models.Today().AddDays(-10)
		pastTxnAmount, _ := models.NewMoney("500.00")
		pastTxn := models.NewTransaction(checking.ID, pastDate, pastTxnAmount)
		if err := txnRepo.Create(pastTxn); err != nil {
			t.Fatalf("Failed to create past transaction: %v", err)
		}

		// Add transaction in the future
		futureDate := models.Today().AddDays(10)
		futureTxnAmount, _ := models.NewMoney("1000.00")
		futureTxn := models.NewTransaction(checking.ID, futureDate, futureTxnAmount)
		if err := txnRepo.Create(futureTxn); err != nil {
			t.Fatalf("Failed to create future transaction: %v", err)
		}

		// Net worth as of today should only include past transaction
		asOfDate := time.Now()
		report, err := svc.NetWorthAsOf(asOfDate)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		// Balance: 1000 + 500 = 1500 (future transaction not included)
		expectedBalance, _ := models.NewMoney("1500.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})

	t.Run("calculates net worth as of a past date", func(t *testing.T) {
		svc, accountRepo, txnRepo := createTestReportService(t)

		openingBalance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", openingBalance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		// Add transaction 5 days ago
		txnDate := models.Today().AddDays(-5)
		txnAmount, _ := models.NewMoney("500.00")
		txn := models.NewTransaction(checking.ID, txnDate, txnAmount)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		// Net worth as of 10 days ago should not include the transaction
		pastDate := models.Today().AddDays(-10).Time()
		report, err := svc.NetWorthAsOf(pastDate)
		if err != nil {
			t.Fatalf("NetWorthAsOf() error = %v", err)
		}

		// Balance: 1000 (transaction not yet happened)
		expectedBalance, _ := models.NewMoney("1000.00")
		if !report.TotalAssets.Equal(expectedBalance) {
			t.Errorf("Expected total assets %s, got %s", expectedBalance.String(), report.TotalAssets.String())
		}
	})
}

func TestReportService_NetWorth_ClosedAccounts(t *testing.T) {
	t.Run("excludes closed accounts by default", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create active account
		activeBalance, _ := models.NewMoney("5000.00")
		active := models.NewAccount("Active Checking", models.AccountTypeChecking, "USD", activeBalance, models.Today())
		if err := accountRepo.Create(active); err != nil {
			t.Fatalf("Failed to create active account: %v", err)
		}

		// Create closed account
		closedBalance := models.ZeroMoney
		closed := models.NewAccount("Closed Checking", models.AccountTypeChecking, "USD", closedBalance, models.Today())
		closed.Close()
		if err := accountRepo.Create(closed); err != nil {
			t.Fatalf("Failed to create closed account: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
		}

		if len(report.Assets) != 1 {
			t.Errorf("Expected 1 asset (closed account excluded), got %d", len(report.Assets))
		}
	})

	t.Run("includes closed accounts when requested", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		// Create active account
		activeBalance, _ := models.NewMoney("5000.00")
		active := models.NewAccount("Active Checking", models.AccountTypeChecking, "USD", activeBalance, models.Today())
		if err := accountRepo.Create(active); err != nil {
			t.Fatalf("Failed to create active account: %v", err)
		}

		// Create closed account with balance
		closedBalance, _ := models.NewMoney("1000.00")
		closed := models.NewAccount("Closed Checking", models.AccountTypeChecking, "USD", closedBalance, models.Today())
		closed.Close()
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

		expectedTotal, _ := models.NewMoney("6000.00")
		if !report.TotalAssets.Equal(expectedTotal) {
			t.Errorf("Expected total assets %s, got %s", expectedTotal.String(), report.TotalAssets.String())
		}
	})
}

func TestReportService_NetWorth_ReportContainsAsOfDate(t *testing.T) {
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

func TestReportService_NetWorth_AccountBalanceDetails(t *testing.T) {
	t.Run("account balances include correct details", func(t *testing.T) {
		svc, accountRepo, _ := createTestReportService(t)

		balance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("My Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking: %v", err)
		}

		report, err := svc.NetWorth()
		if err != nil {
			t.Fatalf("NetWorth() error = %v", err)
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
		if asset.Type != models.AccountTypeChecking {
			t.Errorf("Expected type %s, got %s", models.AccountTypeChecking, asset.Type)
		}
		if !asset.Balance.Equal(balance) {
			t.Errorf("Expected balance %s, got %s", balance.String(), asset.Balance.String())
		}
	})
}
