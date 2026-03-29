package repository

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// createTestTransactionLotData creates all supporting data needed for transaction lot tests.
func createTestTransactionLotData(t *testing.T, accountRepo *AccountRepository, secRepo *SecurityRepository, txnRepo *InvestmentTransactionRepository, lotRepo *LotRepository,
) (*models.Account, *models.Security, *models.InvestmentTransaction, *models.Lot) {
	t.Helper()
	return createTestLotData(t, accountRepo, secRepo, txnRepo, lotRepo)
}

// =============================================================================
// SM-064: TransactionLotRepository.Create
// =============================================================================

func TestTransactionLotRepository_Create(t *testing.T) {
	t.Run("links transaction to lot with share count", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account, sec, buyTxn, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		// Create a sell transaction that references this lot
		sellTxn := models.NewInvestmentTransactionWithSecurity(
			account.ID,
			models.NewDate(2024, 6, 15),
			models.InvestmentTransactionTypeSell,
			models.MustNewMoney("925.00"),
			sec.ID,
			models.MustNewQuantity("5"),
		)
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}
		_ = buyTxn // used indirectly via lot creation

		tl := models.NewTransactionLot(sellTxn.ID, lot.ID, models.MustNewQuantity("5"))
		err := tlRepo.Create(&tl)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify by reading back
		results, err := tlRepo.GetByTransaction(sellTxn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 transaction lot, got %d", len(results))
		}
		if results[0].ID != tl.ID {
			t.Errorf("Expected ID %v, got %v", tl.ID, results[0].ID)
		}
		if results[0].TransactionID != sellTxn.ID {
			t.Errorf("Expected transaction_id %v, got %v", sellTxn.ID, results[0].TransactionID)
		}
		if results[0].LotID != lot.ID {
			t.Errorf("Expected lot_id %v, got %v", lot.ID, results[0].LotID)
		}
		if results[0].Shares.String() != "5" {
			t.Errorf("Expected shares 5, got %q", results[0].Shares.String())
		}
	})

	t.Run("verifies transaction_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		_, _, _, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		fakeTxnID := models.NewID()
		tl := models.NewTransactionLot(fakeTxnID, lot.ID, models.MustNewQuantity("5"))

		err := tlRepo.Create(&tl)
		if err == nil {
			t.Fatal("Expected error for invalid transaction_id foreign key, got nil")
		}
	})

	t.Run("verifies lot_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account, sec, _, _ := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		sellTxn := models.NewInvestmentTransactionWithSecurity(
			account.ID,
			models.NewDate(2024, 6, 15),
			models.InvestmentTransactionTypeSell,
			models.MustNewMoney("925.00"),
			sec.ID,
			models.MustNewQuantity("5"),
		)
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		fakeLotID := models.NewID()
		tl := models.NewTransactionLot(sellTxn.ID, fakeLotID, models.MustNewQuantity("5"))

		err := tlRepo.Create(&tl)
		if err == nil {
			t.Fatal("Expected error for invalid lot_id foreign key, got nil")
		}
	})

	t.Run("links multiple lots to one transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create two buy transactions and lots
		buyTxn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		buyTxn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{buyTxn1, buyTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create buy transaction error = %v", err)
			}
		}

		lot1 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), buyTxn1.ID)
		lot2 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), buyTxn2.ID)
		for _, lot := range []*models.Lot{&lot1, &lot2} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		// Create a sell transaction that pulls from both lots
		sellTxn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 6, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("1400.00"), sec.ID, models.MustNewQuantity("7"))
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		tl1 := models.NewTransactionLot(sellTxn.ID, lot1.ID, models.MustNewQuantity("5"))
		tl2 := models.NewTransactionLot(sellTxn.ID, lot2.ID, models.MustNewQuantity("2"))
		for _, tl := range []*models.TransactionLot{&tl1, &tl2} {
			if err := tlRepo.Create(tl); err != nil {
				t.Fatalf("Create transaction lot error = %v", err)
			}
		}

		results, err := tlRepo.GetByTransaction(sellTxn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 transaction lots, got %d", len(results))
		}
	})
}

// =============================================================================
// SM-065: TransactionLotRepository.GetByTransaction
// =============================================================================

func TestTransactionLotRepository_GetByTransaction(t *testing.T) {
	t.Run("returns all lot allocations for a transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create buy transactions and lots
		buyTxn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), sec.ID, models.MustNewQuantity("5"))
		buyTxn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), sec.ID, models.MustNewQuantity("10"))
		for _, txn := range []*models.InvestmentTransaction{buyTxn1, buyTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create buy transaction error = %v", err)
			}
		}

		lot1 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("5"), models.MustNewMoney("200.00"), models.NewDate(2024, 1, 15), buyTxn1.ID)
		lot2 := models.NewLot(account.ID, sec.ID, models.MustNewQuantity("10"), models.MustNewMoney("200.00"), models.NewDate(2024, 2, 15), buyTxn2.ID)
		for _, lot := range []*models.Lot{&lot1, &lot2} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		// Create sell transaction with two lot allocations
		sellTxn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 6, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("1400.00"), sec.ID, models.MustNewQuantity("7"))
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		tl1 := models.NewTransactionLot(sellTxn.ID, lot1.ID, models.MustNewQuantity("5"))
		tl2 := models.NewTransactionLot(sellTxn.ID, lot2.ID, models.MustNewQuantity("2"))
		for _, tl := range []*models.TransactionLot{&tl1, &tl2} {
			if err := tlRepo.Create(tl); err != nil {
				t.Fatalf("Create transaction lot error = %v", err)
			}
		}

		results, err := tlRepo.GetByTransaction(sellTxn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 allocations, got %d", len(results))
		}

		// Verify lot IDs are present
		lotIDs := map[models.ID]bool{}
		for _, r := range results {
			lotIDs[r.LotID] = true
		}
		if !lotIDs[lot1.ID] {
			t.Error("Expected lot1 in results")
		}
		if !lotIDs[lot2.ID] {
			t.Error("Expected lot2 in results")
		}
	})

	t.Run("returns empty slice when no allocations exist", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create a transaction with no lot allocations
		txn := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 6, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("925.00"), sec.ID, models.MustNewQuantity("5"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}

		results, err := tlRepo.GetByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 allocations, got %d", len(results))
		}
	})

	t.Run("does not return allocations for other transactions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		txnRepo := NewInvestmentTransactionRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		account, sec, _, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		// Create two sell transactions
		sellTxn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 6, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("500.00"), sec.ID, models.MustNewQuantity("3"))
		sellTxn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 7, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("500.00"), sec.ID, models.MustNewQuantity("3"))
		for _, txn := range []*models.InvestmentTransaction{sellTxn1, sellTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create sell transaction error = %v", err)
			}
		}

		// Link only sellTxn1 to the lot
		tl := models.NewTransactionLot(sellTxn1.ID, lot.ID, models.MustNewQuantity("3"))
		if err := tlRepo.Create(&tl); err != nil {
			t.Fatalf("Create transaction lot error = %v", err)
		}

		// sellTxn2 should have no allocations
		results, err := tlRepo.GetByTransaction(sellTxn2.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 allocations for sellTxn2, got %d", len(results))
		}

		// sellTxn1 should have 1 allocation
		results, err = tlRepo.GetByTransaction(sellTxn1.ID)
		if err != nil {
			t.Fatalf("GetByTransaction() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 allocation for sellTxn1, got %d", len(results))
		}
	})
}
