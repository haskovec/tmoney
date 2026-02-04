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

// Helper to create test categories
func createTestCategory(t *testing.T, repo *repository.CategoryRepository, name string, categoryType models.CategoryType) *models.Category {
	t.Helper()
	cat := models.NewCategory(name, categoryType)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Failed to create category %s: %v", name, err)
	}
	return cat
}

func createTestSubcategory(t *testing.T, repo *repository.CategoryRepository, name string, parentID models.ID, categoryType models.CategoryType) *models.Category {
	t.Helper()
	cat := models.NewSubcategory(name, parentID, categoryType)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Failed to create subcategory %s: %v", name, err)
	}
	return cat
}

func createTestReportServiceWithCategories(t *testing.T) (*ReportService, *repository.AccountRepository, *repository.TransactionRepository, *repository.CategoryRepository, *repository.SplitRepository) {
	t.Helper()
	database := createTestDB(t)
	accountRepo := repository.NewAccountRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	svc := NewReportService(accountRepo, database)
	return svc, accountRepo, txnRepo, categoryRepo, splitRepo
}

func TestReportService_SpendingByCategoryMonth_EmptyDatabase(t *testing.T) {
	t.Run("returns zero spending with no transactions", func(t *testing.T) {
		svc, _, _, _, _ := createTestReportServiceWithCategories(t)

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

func TestReportService_SpendingByCategoryMonth_DirectTransactions(t *testing.T) {
	t.Run("calculates spending from direct transactions", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)

		// Create expense transaction (negative amount)
		txnDate := models.NewDate(2024, 1, 15)
		amount, _ := models.NewMoney("-100.00")
		txn := models.NewTransaction(checking.ID, txnDate, amount)
		txn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		if len(report.Categories) != 1 {
			t.Fatalf("Expected 1 category, got %d", len(report.Categories))
		}

		expectedSpending, _ := models.NewMoney("100.00")
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

	t.Run("aggregates multiple transactions in same category", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		food := createTestCategory(t, categoryRepo, "Food", models.CategoryTypeExpense)

		// Create multiple transactions
		txnDate := models.NewDate(2024, 1, 15)
		amounts := []string{"-50.00", "-30.00", "-20.00"}
		for _, amtStr := range amounts {
			amount, _ := models.NewMoney(amtStr)
			txn := models.NewTransaction(checking.ID, txnDate, amount)
			txn.CategoryID = models.NullableID{ID: food.ID, Valid: true}
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Failed to create transaction: %v", err)
			}
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		expectedSpending, _ := models.NewMoney("100.00")
		if !report.TotalSpending.Equal(expectedSpending) {
			t.Errorf("Expected total spending %s, got %s", expectedSpending.String(), report.TotalSpending.String())
		}
	})
}

func TestReportService_SpendingByCategoryMonth_ExcludesIncome(t *testing.T) {
	t.Run("does not include income categories in spending", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create income category
		salary := createTestCategory(t, categoryRepo, "Salary", models.CategoryTypeIncome)

		// Create income transaction (positive amount with income category)
		txnDate := models.NewDate(2024, 1, 15)
		amount, _ := models.NewMoney("5000.00")
		txn := models.NewTransaction(checking.ID, txnDate, amount)
		txn.CategoryID = models.NullableID{ID: salary.ID, Valid: true}
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		if len(report.Categories) != 0 {
			t.Errorf("Expected 0 categories (income excluded), got %d", len(report.Categories))
		}
		if !report.TotalSpending.IsZero() {
			t.Errorf("Expected zero spending, got %s", report.TotalSpending.String())
		}
	})
}

func TestReportService_SpendingByCategoryMonth_ExcludesPositiveAmounts(t *testing.T) {
	t.Run("does not include positive amounts (refunds)", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)

		// Create a refund (positive amount with expense category)
		txnDate := models.NewDate(2024, 1, 15)
		amount, _ := models.NewMoney("50.00") // Positive = refund
		txn := models.NewTransaction(checking.ID, txnDate, amount)
		txn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		// Refunds should not count as spending
		if len(report.Categories) != 0 {
			t.Errorf("Expected 0 categories (refunds excluded), got %d", len(report.Categories))
		}
	})
}

func TestReportService_SpendingByCategoryMonth_DateFiltering(t *testing.T) {
	t.Run("only includes transactions in the specified month", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("1000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)

		// Create transaction in January
		janDate := models.NewDate(2024, 1, 15)
		janAmount, _ := models.NewMoney("-100.00")
		janTxn := models.NewTransaction(checking.ID, janDate, janAmount)
		janTxn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(janTxn); err != nil {
			t.Fatalf("Failed to create January transaction: %v", err)
		}

		// Create transaction in February
		febDate := models.NewDate(2024, 2, 15)
		febAmount, _ := models.NewMoney("-200.00")
		febTxn := models.NewTransaction(checking.ID, febDate, febAmount)
		febTxn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(febTxn); err != nil {
			t.Fatalf("Failed to create February transaction: %v", err)
		}

		// Request January report
		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		expectedSpending, _ := models.NewMoney("100.00")
		if !report.TotalSpending.Equal(expectedSpending) {
			t.Errorf("Expected total spending %s (January only), got %s", expectedSpending.String(), report.TotalSpending.String())
		}
	})
}

func TestReportService_SpendingByCategoryMonth_MultipleCategories(t *testing.T) {
	t.Run("calculates percentages across multiple categories", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense categories
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)
		utilities := createTestCategory(t, categoryRepo, "Utilities", models.CategoryTypeExpense)
		entertainment := createTestCategory(t, categoryRepo, "Entertainment", models.CategoryTypeExpense)

		txnDate := models.NewDate(2024, 1, 15)

		// 50% groceries
		groceryAmt, _ := models.NewMoney("-500.00")
		groceryTxn := models.NewTransaction(checking.ID, txnDate, groceryAmt)
		groceryTxn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(groceryTxn); err != nil {
			t.Fatalf("Failed to create grocery transaction: %v", err)
		}

		// 30% utilities
		utilityAmt, _ := models.NewMoney("-300.00")
		utilityTxn := models.NewTransaction(checking.ID, txnDate, utilityAmt)
		utilityTxn.CategoryID = models.NullableID{ID: utilities.ID, Valid: true}
		if err := txnRepo.Create(utilityTxn); err != nil {
			t.Fatalf("Failed to create utility transaction: %v", err)
		}

		// 20% entertainment
		entertainmentAmt, _ := models.NewMoney("-200.00")
		entertainmentTxn := models.NewTransaction(checking.ID, txnDate, entertainmentAmt)
		entertainmentTxn.CategoryID = models.NullableID{ID: entertainment.ID, Valid: true}
		if err := txnRepo.Create(entertainmentTxn); err != nil {
			t.Fatalf("Failed to create entertainment transaction: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		if len(report.Categories) != 3 {
			t.Fatalf("Expected 3 categories, got %d", len(report.Categories))
		}

		expectedTotal, _ := models.NewMoney("1000.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}

		// Check that categories are sorted by amount descending
		if report.Categories[0].Name != "Groceries" {
			t.Errorf("Expected first category to be 'Groceries', got %q", report.Categories[0].Name)
		}
		if report.Categories[1].Name != "Utilities" {
			t.Errorf("Expected second category to be 'Utilities', got %q", report.Categories[1].Name)
		}
		if report.Categories[2].Name != "Entertainment" {
			t.Errorf("Expected third category to be 'Entertainment', got %q", report.Categories[2].Name)
		}

		// Check percentages (with tolerance for floating point)
		if report.Categories[0].Percentage < 49.9 || report.Categories[0].Percentage > 50.1 {
			t.Errorf("Expected Groceries percentage ~50%%, got %.1f%%", report.Categories[0].Percentage)
		}
		if report.Categories[1].Percentage < 29.9 || report.Categories[1].Percentage > 30.1 {
			t.Errorf("Expected Utilities percentage ~30%%, got %.1f%%", report.Categories[1].Percentage)
		}
		if report.Categories[2].Percentage < 19.9 || report.Categories[2].Percentage > 20.1 {
			t.Errorf("Expected Entertainment percentage ~20%%, got %.1f%%", report.Categories[2].Percentage)
		}
	})
}

func TestReportService_SpendingByCategoryMonth_Subcategories(t *testing.T) {
	t.Run("groups subcategory spending under parent", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create parent category and subcategories
		food := createTestCategory(t, categoryRepo, "Food", models.CategoryTypeExpense)
		groceries := createTestSubcategory(t, categoryRepo, "Groceries", food.ID, models.CategoryTypeExpense)
		restaurants := createTestSubcategory(t, categoryRepo, "Restaurants", food.ID, models.CategoryTypeExpense)

		txnDate := models.NewDate(2024, 1, 15)

		// Transaction in Groceries subcategory
		groceryAmt, _ := models.NewMoney("-200.00")
		groceryTxn := models.NewTransaction(checking.ID, txnDate, groceryAmt)
		groceryTxn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
		if err := txnRepo.Create(groceryTxn); err != nil {
			t.Fatalf("Failed to create grocery transaction: %v", err)
		}

		// Transaction in Restaurants subcategory
		restaurantAmt, _ := models.NewMoney("-100.00")
		restaurantTxn := models.NewTransaction(checking.ID, txnDate, restaurantAmt)
		restaurantTxn.CategoryID = models.NullableID{ID: restaurants.ID, Valid: true}
		if err := txnRepo.Create(restaurantTxn); err != nil {
			t.Fatalf("Failed to create restaurant transaction: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		// Should have one top-level category (Food) with aggregated spending
		if len(report.Categories) != 1 {
			t.Fatalf("Expected 1 top-level category, got %d", len(report.Categories))
		}

		foodCat := report.Categories[0]
		if foodCat.Name != "Food" {
			t.Errorf("Expected category 'Food', got %q", foodCat.Name)
		}

		expectedAmount, _ := models.NewMoney("300.00")
		if !foodCat.Amount.Equal(expectedAmount) {
			t.Errorf("Expected Food amount %s, got %s", expectedAmount.String(), foodCat.Amount.String())
		}

		// Should have subcategories
		if len(foodCat.Subcategories) != 2 {
			t.Fatalf("Expected 2 subcategories, got %d", len(foodCat.Subcategories))
		}
	})
}

func TestReportService_SpendingByCategoryYear(t *testing.T) {
	t.Run("calculates spending for entire year", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("10000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)

		// Create transactions across the year
		months := []time.Month{1, 3, 6, 9, 12}
		for _, month := range months {
			txnDate := models.NewDate(2024, month, 15)
			amount, _ := models.NewMoney("-100.00")
			txn := models.NewTransaction(checking.ID, txnDate, amount)
			txn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Failed to create transaction: %v", err)
			}
		}

		report, err := svc.SpendingByCategoryYear(2024)
		if err != nil {
			t.Fatalf("SpendingByCategoryYear() error = %v", err)
		}

		if report.Period != "2024" {
			t.Errorf("Expected period '2024', got %q", report.Period)
		}

		expectedTotal, _ := models.NewMoney("500.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total spending %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}
	})
}

func TestReportService_SpendingByCategoryDateRange(t *testing.T) {
	t.Run("calculates spending for custom date range", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, _ := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense category
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)

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
			txnDate := models.NewDate(2024, d.month, d.day)
			amount, _ := models.NewMoney("-100.00")
			txn := models.NewTransaction(checking.ID, txnDate, amount)
			txn.CategoryID = models.NullableID{ID: groceries.ID, Valid: true}
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Failed to create transaction: %v", err)
			}
		}

		// Query Feb 1 to Mar 31 (should include 2 transactions)
		startDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(2024, 3, 31, 23, 59, 59, 999999999, time.UTC)

		report, err := svc.SpendingByCategoryDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("SpendingByCategoryDateRange() error = %v", err)
		}

		expectedTotal, _ := models.NewMoney("200.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total spending %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}

		expectedPeriod := "2024-02-01 to 2024-03-31"
		if report.Period != expectedPeriod {
			t.Errorf("Expected period %q, got %q", expectedPeriod, report.Period)
		}
	})
}

func TestReportService_SpendingByCategoryMonth_SplitTransactions(t *testing.T) {
	t.Run("includes split transaction amounts by category", func(t *testing.T) {
		svc, accountRepo, txnRepo, categoryRepo, splitRepo := createTestReportServiceWithCategories(t)

		// Create account
		balance, _ := models.NewMoney("5000.00")
		checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", balance, models.Today())
		if err := accountRepo.Create(checking); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Create expense categories
		groceries := createTestCategory(t, categoryRepo, "Groceries", models.CategoryTypeExpense)
		household := createTestCategory(t, categoryRepo, "Household", models.CategoryTypeExpense)

		// Create a split transaction (e.g., Walmart purchase with groceries and household items)
		txnDate := models.NewDate(2024, 1, 15)
		totalAmount, _ := models.NewMoney("-150.00")
		txn := models.NewTransaction(checking.ID, txnDate, totalAmount)
		// Split transaction has no category_id
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		// Create splits
		groceryAmount, _ := models.NewMoney("-100.00")
		grocerySplit := models.NewSplit(txn.ID, groceries.ID, groceryAmount)
		if err := splitRepo.Create(grocerySplit); err != nil {
			t.Fatalf("Failed to create grocery split: %v", err)
		}

		householdAmount, _ := models.NewMoney("-50.00")
		householdSplit := models.NewSplit(txn.ID, household.ID, householdAmount)
		if err := splitRepo.Create(householdSplit); err != nil {
			t.Fatalf("Failed to create household split: %v", err)
		}

		report, err := svc.SpendingByCategoryMonth(2024, 1)
		if err != nil {
			t.Fatalf("SpendingByCategoryMonth() error = %v", err)
		}

		expectedTotal, _ := models.NewMoney("150.00")
		if !report.TotalSpending.Equal(expectedTotal) {
			t.Errorf("Expected total spending %s, got %s", expectedTotal.String(), report.TotalSpending.String())
		}

		if len(report.Categories) != 2 {
			t.Fatalf("Expected 2 categories, got %d", len(report.Categories))
		}

		// Check Groceries (should be first as it has higher amount)
		expectedGrocery, _ := models.NewMoney("100.00")
		if report.Categories[0].Name != "Groceries" {
			t.Errorf("Expected first category 'Groceries', got %q", report.Categories[0].Name)
		}
		if !report.Categories[0].Amount.Equal(expectedGrocery) {
			t.Errorf("Expected Groceries amount %s, got %s", expectedGrocery.String(), report.Categories[0].Amount.String())
		}

		// Check Household
		expectedHousehold, _ := models.NewMoney("50.00")
		if report.Categories[1].Name != "Household" {
			t.Errorf("Expected second category 'Household', got %q", report.Categories[1].Name)
		}
		if !report.Categories[1].Amount.Equal(expectedHousehold) {
			t.Errorf("Expected Household amount %s, got %s", expectedHousehold.String(), report.Categories[1].Amount.String())
		}
	})
}
