package service

import (
	"testing"
)

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
		if svc.ScheduledTxn == nil {
			t.Error("ScheduledTxn service should not be nil")
		}
		if svc.Report == nil {
			t.Error("Report service should not be nil")
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
	})

	t.Run("services are functional", func(t *testing.T) {
		// Verify services can actually perform operations without errors
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
