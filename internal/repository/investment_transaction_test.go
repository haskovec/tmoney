package repository

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// createInvestmentAccount creates a test investment account.
func createInvestmentAccount(t *testing.T, repo *AccountRepository) *models.Account {
	t.Helper()
	account := models.NewAccount(
		"Brokerage",
		models.AccountTypeInvestment,
		"USD",
		models.MustNewMoney("0.00"),
		models.NewDate(2024, 1, 1),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create investment account: %v", err)
	}
	return account
}

// createInvestmentSecurity creates a test security for investment transaction tests.
func createInvestmentSecurity(t *testing.T, repo *SecurityRepository, ticker, name string) *models.Security {
	t.Helper()
	sec := models.NewSecurity(ticker, name, models.SecurityTypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

// =============================================================================
// SM-050: InvestmentTransactionRepository.Create
// =============================================================================

func TestInvestmentTransactionRepository_Create(t *testing.T) {
	t.Run("creates a deposit transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeDeposit,
			models.MustNewMoney("10000.00"),
		)
		txn.SetMemo("Initial deposit")

		err := repo.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify by reading back
		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != account.ID {
			t.Errorf("Expected account_id %v, got %v", account.ID, retrieved.AccountID)
		}
		if retrieved.Type != models.InvestmentTransactionTypeDeposit {
			t.Errorf("Expected type deposit, got %q", retrieved.Type)
		}
		if retrieved.TotalAmount.String() != "10000" {
			t.Errorf("Expected total_amount 10000, got %q", retrieved.TotalAmount.String())
		}
		if retrieved.Status != models.InvestmentTransactionStatusPending {
			t.Errorf("Expected status pending, got %q", retrieved.Status)
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Initial deposit" {
			t.Errorf("Expected memo 'Initial deposit', got %v", retrieved.Memo)
		}
	})

	t.Run("creates a buy transaction with security and shares", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		txn := models.NewInvestmentTransactionWithSecurity(
			account.ID,
			models.NewDate(2024, 3, 20),
			models.InvestmentTransactionTypeBuy,
			models.MustNewMoney("1850.00"),
			sec.ID,
			models.MustNewQuantity("10"),
		)
		txn.SetPricePerShare(models.MustNewMoney("185.00"))
		txn.SetCommission(models.MustNewMoney("4.95"))

		err := repo.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.SecurityID.Valid || retrieved.SecurityID.ID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if !retrieved.Shares.Valid || retrieved.Shares.Quantity.String() != "10" {
			t.Errorf("Expected shares 10, got %v", retrieved.Shares)
		}
		if !retrieved.PricePerShare.Valid || retrieved.PricePerShare.Money.String() != "185" {
			t.Errorf("Expected price_per_share 185, got %v", retrieved.PricePerShare)
		}
		if !retrieved.Commission.Valid || retrieved.Commission.Money.String() != "4.95" {
			t.Errorf("Expected commission 4.95, got %v", retrieved.Commission)
		}
	})

	t.Run("creates transaction without optional fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 4, 1),
			models.InvestmentTransactionTypeFee,
			models.MustNewMoney("9.99"),
		)

		err := repo.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.SecurityID.Valid {
			t.Error("Expected security_id to be null")
		}
		if retrieved.Shares.Valid {
			t.Error("Expected shares to be null")
		}
		if retrieved.PricePerShare.Valid {
			t.Error("Expected price_per_share to be null")
		}
		if retrieved.Memo.Valid {
			t.Error("Expected memo to be null")
		}
	})
}

// =============================================================================
// SM-051: InvestmentTransactionRepository.GetByID
// =============================================================================

func TestInvestmentTransactionRepository_GetByID(t *testing.T) {
	t.Run("returns existing transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeDeposit,
			models.MustNewMoney("5000.00"),
		)
		if err := repo.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != txn.ID {
			t.Errorf("Expected ID %v, got %v", txn.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewInvestmentTransactionRepository(database)

		fakeID := models.NewID()
		_, err := repo.GetByID(fakeID)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		nfErr, ok := err.(*NotFoundError)
		if !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
		if nfErr.Entity != "investment_transaction" {
			t.Errorf("Expected entity 'investment_transaction', got %q", nfErr.Entity)
		}
	})
}

// =============================================================================
// SM-052: InvestmentTransactionRepository.ListByAccount
// =============================================================================

func TestInvestmentTransactionRepository_ListByAccount(t *testing.T) {
	t.Run("lists all transactions for account ordered by date desc", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		// Create transactions on different dates
		txn1 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("1000.00"))
		txn2 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("2000.00"))
		txn3 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeWithdrawal, models.MustNewMoney("500.00"))

		for _, txn := range []*models.InvestmentTransaction{txn1, txn2, txn3} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := repo.ListByAccount(account.ID, InvestmentTransactionFilter{})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("Expected 3 transactions, got %d", len(results))
		}
		// Should be ordered by date DESC
		if results[0].ID != txn2.ID {
			t.Error("Expected most recent transaction first")
		}
		if results[1].ID != txn3.ID {
			t.Error("Expected second most recent transaction second")
		}
		if results[2].ID != txn1.ID {
			t.Error("Expected oldest transaction last")
		}
	})

	t.Run("filters by type", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		txn1 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("1000.00"))
		txn2 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeWithdrawal, models.MustNewMoney("500.00"))
		txn3 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("2000.00"))

		for _, txn := range []*models.InvestmentTransaction{txn1, txn2, txn3} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		depositType := models.InvestmentTransactionTypeDeposit
		results, err := repo.ListByAccount(account.ID, InvestmentTransactionFilter{Type: &depositType})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 deposit transactions, got %d", len(results))
		}
		for _, r := range results {
			if r.Type != models.InvestmentTransactionTypeDeposit {
				t.Errorf("Expected type deposit, got %q", r.Type)
			}
		}
	})

	t.Run("filters by date range", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		txn1 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("1000.00"))
		txn2 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("2000.00"))
		txn3 := models.NewInvestmentTransaction(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeDeposit, models.MustNewMoney("3000.00"))

		for _, txn := range []*models.InvestmentTransaction{txn1, txn2, txn3} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		from := models.NewDate(2024, 2, 1)
		to := models.NewDate(2024, 2, 28)
		results, err := repo.ListByAccount(account.ID, InvestmentTransactionFilter{FromDate: &from, ToDate: &to})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 transaction in Feb, got %d", len(results))
		}
		if results[0].ID != txn2.ID {
			t.Error("Expected Feb transaction")
		}
	})

	t.Run("filters by security_id", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		aapl := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurity(t, secRepo, "MSFT", "Microsoft Corp.")

		txn1 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 1, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("1000.00"), aapl.ID, models.MustNewQuantity("5"))
		txn2 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 2, 15), models.InvestmentTransactionTypeBuy, models.MustNewMoney("2000.00"), msft.ID, models.MustNewQuantity("10"))
		txn3 := models.NewInvestmentTransactionWithSecurity(account.ID, models.NewDate(2024, 3, 15), models.InvestmentTransactionTypeSell, models.MustNewMoney("500.00"), aapl.ID, models.MustNewQuantity("2"))

		for _, txn := range []*models.InvestmentTransaction{txn1, txn2, txn3} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		aaplID := aapl.ID
		results, err := repo.ListByAccount(account.ID, InvestmentTransactionFilter{SecurityID: &aaplID})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 AAPL transactions, got %d", len(results))
		}
	})

	t.Run("returns empty slice for account with no transactions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)

		results, err := repo.ListByAccount(account.ID, InvestmentTransactionFilter{})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 transactions, got %d", len(results))
		}
	})
}

// =============================================================================
// SM-053: InvestmentTransactionRepository.Update
// =============================================================================

func TestInvestmentTransactionRepository_Update(t *testing.T) {
	t.Run("updates mutable fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeDeposit,
			models.MustNewMoney("5000.00"),
		)
		if err := repo.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		originalUpdatedAt := txn.UpdatedAt

		// Update fields
		txn.SetMemo("Updated memo")
		txn.Clear()

		err := repo.Update(txn)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Updated memo" {
			t.Errorf("Expected memo 'Updated memo', got %v", retrieved.Memo)
		}
		if retrieved.Status != models.InvestmentTransactionStatusCleared {
			t.Errorf("Expected status cleared, got %q", retrieved.Status)
		}
		if !retrieved.UpdatedAt.Time().After(originalUpdatedAt.Time()) {
			t.Error("Expected updated_at to change after update")
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeDeposit,
			models.MustNewMoney("5000.00"),
		)
		// Don't create — just try to update
		err := repo.Update(txn)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-054: InvestmentTransactionRepository.Delete
// =============================================================================

func TestInvestmentTransactionRepository_Delete(t *testing.T) {
	t.Run("deletes existing transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		txn := models.NewInvestmentTransaction(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeDeposit,
			models.MustNewMoney("5000.00"),
		)
		if err := repo.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := repo.Delete(txn.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify it's gone
		_, err = repo.GetByID(txn.ID)
		if err == nil {
			t.Fatal("Expected error after delete, got nil")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("deletes transaction with junction records", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		secRepo := NewSecurityRepository(database)
		repo := NewInvestmentTransactionRepository(database)

		account := createInvestmentAccount(t, accountRepo)
		sec := createInvestmentSecurity(t, secRepo, "AAPL", "Apple Inc.")

		// Create a buy transaction
		txn := models.NewInvestmentTransactionWithSecurity(
			account.ID,
			models.NewDate(2024, 3, 15),
			models.InvestmentTransactionTypeBuy,
			models.MustNewMoney("1850.00"),
			sec.ID,
			models.MustNewQuantity("10"),
		)
		if err := repo.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Create a lot and link it via junction table
		lotID := models.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO investment_lots (id, account_id, security_id, shares, original_shares, cost_per_share, purchase_date, source_transaction_id)
			VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, CAST(? AS UUID))
		`, lotID, account.ID.String(), sec.ID.String(), "10", "10", "185.00", "2024-03-15", txn.ID.String())
		if err != nil {
			t.Fatalf("Failed to create test lot: %v", err)
		}

		junctionID := models.NewID()
		_, err = database.Conn().Exec(`
			INSERT INTO investment_transaction_lots (id, transaction_id, lot_id, shares)
			VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?)
		`, junctionID, txn.ID.String(), lotID.String(), "10")
		if err != nil {
			t.Fatalf("Failed to create junction record: %v", err)
		}

		// Delete should remove junction records first, then the transaction
		err = repo.Delete(txn.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify transaction is gone
		_, err = repo.GetByID(txn.ID)
		if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError after delete, got %T: %v", err, err)
		}

		// Verify junction records are gone
		var count int
		err = database.Conn().QueryRow(`SELECT COUNT(*) FROM investment_transaction_lots WHERE CAST(transaction_id AS VARCHAR) = ?`, txn.ID.String()).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count junction records: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 junction records, got %d", count)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewInvestmentTransactionRepository(database)

		fakeID := models.NewID()
		err := repo.Delete(fakeID)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}
