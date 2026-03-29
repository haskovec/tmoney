package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestLotData creates a lot with required supporting data (account, security, transaction).
func createTestLotData(t *testing.T, accountRepo *account.Repository, secRepo *security.Repository, txnRepo *Repository, lotRepo *LotRepository,
) (*account.Account, *security.Security, *Transaction, *Lot) {
	t.Helper()

	acct := createInvestmentAccountForTest(t, accountRepo)
	sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

	txn := NewTransactionWithSecurity(
		acct.ID,
		types.NewDate(2024, 3, 15),
		TransactionTypeBuy,
		types.MustNewMoney("1850.00"),
		sec.ID,
		types.MustNewQuantity("10"),
	)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create investment transaction: %v", err)
	}

	lot := NewLot(
		acct.ID,
		sec.ID,
		types.MustNewQuantity("10"),
		types.MustNewMoney("185.00"),
		types.NewDate(2024, 3, 15),
		txn.ID,
	)
	if err := lotRepo.Create(&lot); err != nil {
		t.Fatalf("Failed to create lot: %v", err)
	}

	return acct, sec, txn, &lot
}

// =============================================================================
// SM-055: LotRepository.Create
// =============================================================================

func TestLotRepository_Create(t *testing.T) {
	t.Run("creates a lot and verifies all fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn := NewTransactionWithSecurity(
			acct.ID,
			types.NewDate(2024, 3, 15),
			TransactionTypeBuy,
			types.MustNewMoney("1850.00"),
			sec.ID,
			types.MustNewQuantity("10"),
		)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := NewLot(
			acct.ID,
			sec.ID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
			types.NewDate(2024, 3, 15),
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
		if retrieved.AccountID != acct.ID {
			t.Errorf("Expected account_id %v, got %v", acct.ID, retrieved.AccountID)
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
		secRepo := security.NewRepository(database)
		lotRepo := NewLotRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		fakeAccountID := types.NewID()
		fakeTxnID := types.NewID()

		lot := NewLot(
			fakeAccountID,
			sec.ID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
			types.NewDate(2024, 3, 15),
			fakeTxnID,
		)

		err := lotRepo.Create(&lot)
		if err == nil {
			t.Fatal("Expected error for invalid account_id foreign key, got nil")
		}
	})

	t.Run("verifies security_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		fakeSecurityID := types.NewID()
		fakeTxnID := types.NewID()

		lot := NewLot(
			acct.ID,
			fakeSecurityID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
			types.NewDate(2024, 3, 15),
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
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

		fakeID := types.NewID()
		_, err := lotRepo.GetByID(fakeID)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		nfErr, ok := err.(*dberrors.NotFoundError)
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create transactions for each lot
		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		txn3 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("1500.00"), sec.ID, types.MustNewQuantity("7"))
		for _, txn := range []*Transaction{txn1, txn2, txn3} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		lot1 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		lot2 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 3, 15), txn2.ID)
		lot3 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("7"), types.MustNewMoney("214.29"), types.NewDate(2024, 2, 15), txn3.ID)

		for _, lot := range []*Lot{&lot1, &lot2, &lot3} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		// Create an open lot and a closed lot
		openLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		closedLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)

		// Reduce closed lot to zero so it's closed
		if err := closedLot.Reduce(types.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*Lot{&openLot, &closedLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		openLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		closedLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)
		if err := closedLot.Reduce(types.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*Lot{&openLot, &closedLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 lots (including closed), got %d", len(results))
		}
	})

	t.Run("returns empty slice when no lots exist", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		results, err := lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 lots, got %d", len(results))
		}
	})

	t.Run("filters by security — does not return lots for other securities", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), aapl.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), msft.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		aaplLot := NewLot(acct.ID, aapl.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		msftLot := NewLot(acct.ID, msft.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*Lot{&aaplLot, &msftLot} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		results, err := lotRepo.ListByAccountAndSecurity(acct.ID, aapl.ID, false)
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeBuy, types.MustNewMoney("1850.00"), sec.ID, types.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"), types.NewDate(2024, 3, 15), txn.ID)
		if err := lotRepo.Create(&lot); err != nil {
			t.Fatalf("Create lot error = %v", err)
		}
		originalUpdatedAt := lot.UpdatedAt

		// Reduce shares
		if err := lot.Reduce(types.MustNewQuantity("3")); err != nil {
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeBuy, types.MustNewMoney("1850.00"), sec.ID, types.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"), types.NewDate(2024, 3, 15), txn.ID)
		if err := lotRepo.Create(&lot); err != nil {
			t.Fatalf("Create lot error = %v", err)
		}

		// Reduce all shares
		if err := lot.Reduce(types.MustNewQuantity("10")); err != nil {
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeBuy, types.MustNewMoney("1850.00"), sec.ID, types.MustNewQuantity("10"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		lot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"), types.NewDate(2024, 3, 15), txn.ID)
		// Don't create — just try to update
		err := lotRepo.Update(&lot)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		// Create two accounts
		account1 := createInvestmentAccountForTest(t, accountRepo)

		account2 := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.MustNewMoney("0.00"), types.NewDate(2024, 1, 1))
		if err := accountRepo.Create(account2); err != nil {
			t.Fatalf("Failed to create account2: %v", err)
		}

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create transactions and lots in both accounts
		txn1 := NewTransactionWithSecurity(account1.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(account2.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		lot1 := NewLot(account1.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		lot2 := NewLot(account2.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*Lot{&lot1, &lot2} {
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		openLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		closedLot := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)
		if err := closedLot.Reduce(types.MustNewQuantity("10")); err != nil {
			t.Fatalf("Reduce() error = %v", err)
		}

		for _, lot := range []*Lot{&openLot, &closedLot} {
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
		secRepo := security.NewRepository(database)
		lotRepo := NewLotRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		txn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), aapl.ID, types.MustNewQuantity("5"))
		txn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), msft.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{txn1, txn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create transaction error = %v", err)
			}
		}

		aaplLot := NewLot(acct.ID, aapl.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), txn1.ID)
		msftLot := NewLot(acct.ID, msft.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), txn2.ID)

		for _, lot := range []*Lot{&aaplLot, &msftLot} {
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
