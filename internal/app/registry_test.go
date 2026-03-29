package app

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
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

func TestNewServices(t *testing.T) {
	database := createTestDB(t)
	svc := NewServices(database)

	t.Run("all services are initialized", func(t *testing.T) {
		if svc.Account == nil {
			t.Error("Account service should not be nil")
		}
		if svc.Transaction == nil {
			t.Error("Transaction service should not be nil")
		}
		if svc.Category == nil {
			t.Error("Category service should not be nil")
		}
		if svc.Payee == nil {
			t.Error("Payee service should not be nil")
		}
		if svc.Scheduled == nil {
			t.Error("Scheduled service should not be nil")
		}
		if svc.Report == nil {
			t.Error("Report service should not be nil")
		}
		if svc.Reconciliation == nil {
			t.Error("Reconciliation service should not be nil")
		}
		if svc.Security == nil {
			t.Error("Security service should not be nil")
		}
		if svc.Price == nil {
			t.Error("Price service should not be nil")
		}
		if svc.Investment == nil {
			t.Error("Investment service should not be nil")
		}
	})

	t.Run("all repositories are initialized", func(t *testing.T) {
		if svc.AccountRepo == nil {
			t.Error("AccountRepo should not be nil")
		}
		if svc.TransactionRepo == nil {
			t.Error("TransactionRepo should not be nil")
		}
		if svc.SplitRepo == nil {
			t.Error("SplitRepo should not be nil")
		}
		if svc.TransferRepo == nil {
			t.Error("TransferRepo should not be nil")
		}
		if svc.CategoryRepo == nil {
			t.Error("CategoryRepo should not be nil")
		}
		if svc.PayeeRepo == nil {
			t.Error("PayeeRepo should not be nil")
		}
		if svc.ScheduledTxnRepo == nil {
			t.Error("ScheduledTxnRepo should not be nil")
		}
		if svc.ReconciliationRepo == nil {
			t.Error("ReconciliationRepo should not be nil")
		}
		if svc.SecurityRepo == nil {
			t.Error("SecurityRepo should not be nil")
		}
		if svc.PriceRepo == nil {
			t.Error("PriceRepo should not be nil")
		}
		if svc.InvestmentRepo == nil {
			t.Error("InvestmentRepo should not be nil")
		}
		if svc.LotRepo == nil {
			t.Error("LotRepo should not be nil")
		}
		if svc.PositionRepo == nil {
			t.Error("PositionRepo should not be nil")
		}
		if svc.TransactionLotRepo == nil {
			t.Error("TransactionLotRepo should not be nil")
		}
	})

	t.Run("services are functional", func(t *testing.T) {
		_, err := svc.Account.List(true)
		if err != nil {
			t.Errorf("Account.List() error = %v", err)
		}

		_, err = svc.CategoryRepo.List()
		if err != nil {
			t.Errorf("CategoryRepo.List() error = %v", err)
		}

		_, err = svc.PayeeRepo.List()
		if err != nil {
			t.Errorf("PayeeRepo.List() error = %v", err)
		}
	})
}
