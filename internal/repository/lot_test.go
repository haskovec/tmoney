package repository

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// createTestLotData creates a lot with required supporting data (account, security, transaction).
func createTestLotData(t *testing.T, accountRepo *AccountRepository, secRepo *SecurityRepository, txnRepo *InvestmentTransactionRepository, lotRepo *LotRepository,
) (*models.Account, *models.Security, *models.InvestmentTransaction, *models.Lot) {
	t.Helper()

	account := createInvestmentAccount(t, accountRepo)
	sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

	txn := models.NewInvestmentTransactionWithSecurity(
		account.ID,
		models.NewDate(2024, 3, 15),
		models.InvestmentTransactionTypeBuy,
		models.MustNewMoney("1850.00"),
		sec.ID,
		models.MustNewQuantity("10"),
	)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create investment transaction: %v", err)
	}

	lot := models.NewLot(
		account.ID,
		sec.ID,
		models.MustNewQuantity("10"),
		models.MustNewMoney("185.00"),
		models.NewDate(2024, 3, 15),
		txn.ID,
	)
	if err := lotRepo.Create(&lot); err != nil {
		t.Fatalf("Failed to create lot: %v", err)
	}

	return account, sec, txn, &lot
}

// =============================================================================
// SM-055: LotRepository.Create
// =============================================================================

func TestLotRepository_Create(t *testing.T) {
	t.Run("creates a lot and verifies all fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn := models.NewInvestmentTransactionWithSecurity(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeBuy,
			models.MustNewMoney("1850.00"),
			sec.ID,
			models.MustNewQuantity("10"),
		)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := models.NewLot(
			account.ID,
			sec.ID,
			models.MustNewQuantity("10"),
			models.MustNewMoney("185.00"),
			models.NewDate(2024, 3, 15),
			txn.ID,
		)

		err := lotRepo.Create(&lot)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify by reading back
		retrieved, err := lotRepo.GetByID(lot.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != account.ID {
			t.Errorf("Expected account_id %v, got %v", account.ID, retrieved.AccountID)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.Shares.String() != "10" {
			t.Errorf("Expected shares 10, got %q", retrieved.Shares.String())
		}
		if retrieved.OriginalShares.String() != "10" {
			t.Errorf("Expected original_shares 10, got %q", retrieved.OriginalShares.String())
		}
		if retrieved.CostPerShare.String() != "185" {
			t.Errorf("Expected cost_per_share 185, got %q", retrieved.CostPerShare.String())
		}
		if retrieved.PurchaseDate.Time().Year() != 2024 || retrieved.PurchaseDate.Time().Month() != 3 || retrieved.PurchaseDate.Time().Day() != 15 {
			t.Errorf("Expected purchase_date 2024-03-15, got %v", retrieved.PurchaseDate)
		}
		if retrieved.SourceTransactionID != txn.ID {
			t.Errorf("Expected source_transaction_id %v, got %v", txn.ID, retrieved.SourceTransactionID)
		}
		if retrieved.Closed {
			t.Error("Expected closed to be false")
		}
	})

	t.Run("verifies account_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		lotRepo := NewLotRepository(database)

		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")
		fakeAccountID := models.NewID()
		fakeTxnID := models.NewID()

		lot := models.NewLot(
			fakeAccountID,
			sec.ID,
			models.MustNewQuantity("10"),
			models.MustNewMoney("185.00"),
			models.NewDate(2024, 3, 15),
			fakeTxnID,
		)

		err := lotRepo.Create(&lot)
		if err == nil {
			t.Fatal("Expected error for invalid account_id foreign key, got nil")
		}
	})

	t.Run("verifies security_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		fakeSecurityID := models.NewID()
		fakeTxnID := models.NewID()

		lot := models.NewLot(
			account.ID,
			fakeSecurityID,
			models.MustNewQuantity("10"),
			models.MustNewMoney("185.00"),
			models.NewDate(2024, 3, 15),
			fakeTxnID,
		)

		err := lotRepo.Create(&lot)
		if err == nil {
			t.Fatal("Expected error for invalid security_id foreign key, got nil")
		}
	})
}

// =============================================================================
// SM-056: LotRepository.GetByID
// =============================================================================

func TestLotRepository_GetByID(t *testing.T) {
	t.Run("returns existing lot", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		_, _, _, lot := createTestLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		retrieved, err := lotRepo.GetByID(lot.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != lot.ID {
			t.Errorf("Expected ID %v, got %v", lot.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		lotRepo := NewLotRepository(database)

		fakeID := models.NewID()
		_, err := lotRepo.GetByID(fakeID)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		nfErr, ok := err.(*NotFoundError)
		if !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
		if nfErr.Entity != "lot" {
			t.Errorf("Expected entity 'lot', got %q", nfErr.Entity)
		}
	})
}

// =============================================================================
// SM-057: LotRepository.ListByAccountAndSecurity
// =============================================================================

func TestLotRepository_ListByAccountAndSecurity(t *testing.T) {
	t.Run("lists open lots ordered by purchase_date", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create transactions for each lot
		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		txn3 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1500.00"), sec.ID, models.MustNewQuantity("7"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2, txn3} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		lot1 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		lot2 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 3, 15), txn2.ID)
		lot3 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("7"), models.MustNewMoney("214.29"), models.NewDate(2024, 2, 15), txn3.ID)

		for _, lot := range []*models.Lot{&lot1, &lot2, &lot3} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(account.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("Expected 3 lots, got %d", len(results))
		}
		// Ordered by purchase_date ASC
		if results[0].ID != lot1.ID {
			t.Error("Expected oldest lot first (Jan)")
		}
		if results[1].ID != lot3.ID {
			t.Error("Expected second lot second (Feb)")
		}
		if results[2].ID != lot2.ID {
			t.Error("Expected newest lot last (Mar)")
		}
	})

	t.Run("excludes closed lots by default", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		// Create an open lot and a closed lot
		openLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		closedLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)

		// Reduce closed lot to zero so it's closed
		if err := closedLot.Reduce(models.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*models.Lot{&openLot, &closedLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(account.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(results))
		}
		if results[0].ID != openLot.ID {
			t.Error("Expected the open lot")
		}
	})

	t.Run("includes closed lots when requested", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		openLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		closedLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)
		if err := closedLot.Reduce(models.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*models.Lot{&openLot, &closedLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(account.ID, sec.ID, true)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 lots (including closed), got %d", len(results))
		}
	})

	t.Run("returns empty slice when no lots exist", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		results, err := lotRepo.ListByAccountAndSecurity(account.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 lots, got %d", len(results))
		}
	})

	t.Run("filters by security — does not return lots for other securities", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		aapl := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurity(t, secRepo, "MSFT", "Microsoft Corp.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), aapl.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), msft.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		aaplLot := models.NewLot(account.ID, aapl.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		msftLot := models.NewLot(account.ID, msft.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*models.Lot{&aaplLot, &msftLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(account.ID, aapl.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 AAPL lot, got %d", len(results))
		}
		if results[0].ID != aaplLot.ID {
			t.Error("Expected AAPL lot")
		}
	})
}

// =============================================================================
// SM-058: LotRepository.Update (reduce shares)
// =============================================================================

func TestLotRepository_Update(t *testing.T) {
	t.Run("updates shares count and updated_at changes", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1850.00"), sec.ID, models.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("185.00"), models.NewDate(2024, 3, 15), txn.ID)
		if err := lotRepo.Create(&lot); err != nil {
			t.Fatalf("Create lot error = %v", err)
		}
		originalUpdatedAt := lot.UpdatedAt

		// Reduce shares
		if err := lot.Reduce(models.MustNewQuantity("3")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		err := lotRepo.Update(&lot)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := lotRepo.GetByID(lot.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Shares.String() != "7" {
			t.Errorf("Expected shares 7, got %q", retrieved.Shares.String())
		}
		if retrieved.OriginalShares.String() != "10" {
			t.Errorf("Expected original_shares 10, got %q", retrieved.OriginalShares.String())
		}
		if retrieved.Closed {
			t.Error("Expected closed to be false")
		}
		if !retrieved.UpdatedAt.Time().After(originalUpdatedAt.Time()) {
			t.Error("Expected updated_at to change after update")
		}
	})

	t.Run("sets closed=true when shares reach zero", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1850.00"), sec.ID, models.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("185.00"), models.NewDate(2024, 3, 15), txn.ID)
		if err := lotRepo.Create(&lot); err != nil {
			t.Fatalf("Create lot error = %v", err)
		}

		// Reduce all shares
		if err := lot.Reduce(models.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		err := lotRepo.Update(&lot)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := lotRepo.GetByID(lot.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Shares.String() != "0" {
			t.Errorf("Expected shares 0, got %q", retrieved.Shares.String())
		}
		if !retrieved.Closed {
			t.Error("Expected closed to be true when shares reach zero")
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1850.00"), sec.ID, models.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("185.00"), models.NewDate(2024, 3, 15), txn.ID)
		// Don't create — just try to update
		err := lotRepo.Update(&lot)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-059: LotRepository.GetOpenLotsBySecurity
// =============================================================================

func TestLotRepository_GetOpenLotsBySecurity(t *testing.T) {
	t.Run("returns open lots across all accounts", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		// Create two accounts
		account1 := createInvestmentAccount(t, accountRepo)

		account2 := models.NewAccount("Roth IRA", models.AccountTypeInvestment, "USD", models.MustNewMoney("0.00"), models.NewDate(2024, 1, 1))
		if err := accountRepo.Create(account2); err != nil {
			t.Fatalf("Failed to create account2: %v", err)
		}

		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create transactions and lots in both accounts
		txn1 := models.NewInvestmentTransactionWithSecurity(account1.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account2.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		lot1 := models.NewLot(account1.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		lot2 := models.NewLot(account2.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*models.Lot{&lot1, &lot2} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.GetOpenLotsBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("GetOpenLotsBySecurity() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 open lots across accounts, got %d", len(results))
		}
	})

	t.Run("excludes closed lots", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		openLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		closedLot := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)
		if err := closedLot.Reduce(models.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*models.Lot{&openLot, &closedLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.GetOpenLotsBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("GetOpenLotsBySecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(results))
		}
		if results[0].ID != openLot.ID {
			t.Error("Expected the open lot")
		}
	})

	t.Run("returns empty slice when no open lots exist", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		lotRepo := NewLotRepository(database)

		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		results, err := lotRepo.GetOpenLotsBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("GetOpenLotsBySecurity() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 lots, got %d", len(results))
		}
	})

	t.Run("does not return lots for other securities", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		aapl := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurity(t, secRepo, "MSFT", "Microsoft Corp.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), aapl.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), msft.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		aaplLot := models.NewLot(account.ID, aapl.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), txn1.ID)
		msftLot := models.NewLot(account.ID, msft.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*models.Lot{&aaplLot, &msftLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.GetOpenLotsBySecurity(aapl.ID)
		if err != nil {
			t.Fatalf("GetOpenLotsBySecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 AAPL lot, got %d", len(results))
		}
		if results[0].ID != aaplLot.ID {
			t.Error("Expected AAPL lot")
		}
	})
}
