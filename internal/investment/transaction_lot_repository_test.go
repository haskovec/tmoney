package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestTransactionLotData creates all supporting data needed for transaction lot tests.
func createTestTransactionLotData(t *testing.T, accountRepo *account.Repository, secRepo *security.Repository, txnRepo *Repository, lotRepo *LotRepository,
) (*account.Account, *security.Security, *Transaction, *Lot) {
	t.Helper()
	return createTestLotData(t, accountRepo, secRepo, txnRepo, lotRepo)
}

// =============================================================================
// SM-064: TransactionLotRepository.Create
// =============================================================================

func TestTransactionLotRepository_Create(t *testing.T) {
	t.Run("links transaction to lot with share count", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct, sec, buyTxn, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		// Create a sell transaction that references this lot
		sellTxn := NewTransactionWithSecurity(
			acct.ID,
			types.NewDate(2024, 6, 15),
			TransactionTypeSell,
			types.MustNewMoney("925.00"),
			sec.ID,
			types.MustNewQuantity("5"),
		)
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}
		_ = buyTxn // used indirectly via lot creation

		tl := NewTransactionLot(sellTxn.ID, lot.ID, types.MustNewQuantity("5"))
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

	t.Run("accepts orphan transaction_id (FK was dropped in migration 013)", func(t *testing.T) {
		// Migration 013 dropped the transaction_id / lot_id foreign keys on
		// investment_transaction_lots so that lot rows can be updated even
		// while junctions reference them. Referential integrity is now
		// enforced by the service layer (Repository.Delete cascades
		// junctions). The DB itself accepts orphan rows.
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		_, _, _, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		fakeTxnID := types.NewID()
		tl := NewTransactionLot(fakeTxnID, lot.ID, types.MustNewQuantity("5"))

		if err := tlRepo.Create(&tl); err != nil {
			t.Fatalf("Expected Create with orphan transaction_id to succeed, got: %v", err)
		}
	})

	t.Run("accepts orphan lot_id (FK was dropped in migration 013)", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct, sec, _, _ := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		sellTxn := NewTransactionWithSecurity(
			acct.ID,
			types.NewDate(2024, 6, 15),
			TransactionTypeSell,
			types.MustNewMoney("925.00"),
			sec.ID,
			types.MustNewQuantity("5"),
		)
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		fakeLotID := types.NewID()
		tl := NewTransactionLot(sellTxn.ID, fakeLotID, types.MustNewQuantity("5"))

		if err := tlRepo.Create(&tl); err != nil {
			t.Fatalf("Expected Create with orphan lot_id to succeed, got: %v", err)
		}
	})

	t.Run("links multiple lots to one transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create two buy transactions and lots
		buyTxn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		buyTxn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{buyTxn1, buyTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create buy transaction error = %v", err)
			}
		}

		lot1 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), buyTxn1.ID)
		lot2 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), buyTxn2.ID)
		for _, lot := range []*Lot{&lot1, &lot2} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		// Create a sell transaction that pulls from both lots
		sellTxn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 6, 15), TransactionTypeSell, types.MustNewMoney("1400.00"), sec.ID, types.MustNewQuantity("7"))
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		tl1 := NewTransactionLot(sellTxn.ID, lot1.ID, types.MustNewQuantity("5"))
		tl2 := NewTransactionLot(sellTxn.ID, lot2.ID, types.MustNewQuantity("2"))
		for _, tl := range []*TransactionLot{&tl1, &tl2} {
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create buy transactions and lots
		buyTxn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeBuy, types.MustNewMoney("1000.00"), sec.ID, types.MustNewQuantity("5"))
		buyTxn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeBuy, types.MustNewMoney("2000.00"), sec.ID, types.MustNewQuantity("10"))
		for _, txn := range []*Transaction{buyTxn1, buyTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create buy transaction error = %v", err)
			}
		}

		lot1 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("5"), types.MustNewMoney("200.00"), types.NewDate(2024, 1, 15), buyTxn1.ID)
		lot2 := NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("200.00"), types.NewDate(2024, 2, 15), buyTxn2.ID)
		for _, lot := range []*Lot{&lot1, &lot2} {
			if err := lotRepo.Create(lot); err != nil {
				t.Fatalf("Create lot error = %v", err)
			}
		}

		// Create sell transaction with two lot allocations
		sellTxn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 6, 15), TransactionTypeSell, types.MustNewMoney("1400.00"), sec.ID, types.MustNewQuantity("7"))
		if err := txnRepo.Create(sellTxn); err != nil {
			t.Fatalf("Create sell transaction error = %v", err)
		}

		tl1 := NewTransactionLot(sellTxn.ID, lot1.ID, types.MustNewQuantity("5"))
		tl2 := NewTransactionLot(sellTxn.ID, lot2.ID, types.MustNewQuantity("2"))
		for _, tl := range []*TransactionLot{&tl1, &tl2} {
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
		lotIDs := map[types.ID]bool{}
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create a transaction with no lot allocations
		txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 6, 15), TransactionTypeSell, types.MustNewMoney("925.00"), sec.ID, types.MustNewQuantity("5"))
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
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		txnRepo := NewRepository(database)
		lotRepo := NewLotRepository(database)
		tlRepo := NewTransactionLotRepository(database)

		acct, sec, _, lot := createTestTransactionLotData(t, accountRepo, secRepo, txnRepo, lotRepo)

		// Create two sell transactions
		sellTxn1 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 6, 15), TransactionTypeSell, types.MustNewMoney("500.00"), sec.ID, types.MustNewQuantity("3"))
		sellTxn2 := NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 7, 15), TransactionTypeSell, types.MustNewMoney("500.00"), sec.ID, types.MustNewQuantity("3"))
		for _, txn := range []*Transaction{sellTxn1, sellTxn2} {
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Create sell transaction error = %v", err)
			}
		}

		// Link only sellTxn1 to the lot
		tl := NewTransactionLot(sellTxn1.ID, lot.ID, types.MustNewQuantity("3"))
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
