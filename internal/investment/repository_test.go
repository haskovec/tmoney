package investment

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")
	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createInvestmentAccountForTest(t *testing.T, repo *account.Repository) *account.Account {
	t.Helper()
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.MustNewMoney("0.00"), types.NewDate(2024, 1, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create investment account: %v", err)
	}
	return acct
}

func createInvestmentSecurityForTest(t *testing.T, repo *security.Repository, ticker, name string) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, name, security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

func TestRepository_Create(t *testing.T) {
	t.Run("creates a deposit transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)

		txn := NewTransaction(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeDeposit, types.MustNewMoney("10000.00"))
		txn.SetMemo("Initial deposit")

		err := repo.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Type != TransactionTypeDeposit {
			t.Errorf("Expected type deposit, got %q", retrieved.Type)
		}
		if retrieved.TotalAmount.String() != "10000" {
			t.Errorf("Expected total_amount 10000, got %q", retrieved.TotalAmount.String())
		}
	})

	t.Run("creates a buy transaction with security and shares", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		repo := NewRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		txn := NewTransactionWithSecurity(
			acct.ID, types.NewDate(2024, 3, 20), TransactionTypeBuy,
			types.MustNewMoney("1850.00"), sec.ID, types.MustNewQuantity("10"),
		)
		txn.SetPricePerShare(types.MustNewMoney("185.00"))
		txn.SetCommission(types.MustNewMoney("4.95"))

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
	})
}

func TestRepository_GetByID(t *testing.T) {
	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		_, err := repo.GetByID(types.NewID())
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_ListByAccount(t *testing.T) {
	t.Run("lists all transactions for account ordered by date desc", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)

		txn1 := NewTransaction(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		txn2 := NewTransaction(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeDeposit, types.MustNewMoney("2000.00"))
		txn3 := NewTransaction(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeWithdrawal, types.MustNewMoney("500.00"))

		for _, txn := range []*Transaction{txn1, txn2, txn3} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := repo.ListByAccount(acct.ID, TransactionFilter{})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("Expected 3 transactions, got %d", len(results))
		}
		if results[0].ID != txn2.ID {
			t.Error("Expected most recent transaction first")
		}
	})

	t.Run("filters by type", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)

		txn1 := NewTransaction(acct.ID, types.NewDate(2024, 1, 15), TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		txn2 := NewTransaction(acct.ID, types.NewDate(2024, 2, 15), TransactionTypeWithdrawal, types.MustNewMoney("500.00"))

		for _, txn := range []*Transaction{txn1, txn2} {
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		depositType := TransactionTypeDeposit
		results, err := repo.ListByAccount(acct.ID, TransactionFilter{Type: &depositType})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 deposit transaction, got %d", len(results))
		}
	})
}

func TestRepository_Delete(t *testing.T) {
	t.Run("deletes existing transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		txn := NewTransaction(acct.ID, types.NewDate(2024, 3, 15), TransactionTypeDeposit, types.MustNewMoney("5000.00"))
		if err := repo.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := repo.Delete(txn.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err = repo.GetByID(txn.ID)
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError after delete, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		err := repo.Delete(types.NewID())
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}
