package app

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
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

// TestFileInit_PaycheckCategoriesExist asserts that opening or creating
// a database via NewServices ensures the paycheck-wizard categories
// (Income:Salary, Tax:Federal, Tax:State, Tax:Social Security,
// Tax:Medicare, Insurance:Health) exist — both for fresh files and for
// existing files that previously had them removed.
func TestFileInit_PaycheckCategoriesExist(t *testing.T) {
	required := []struct{ parent, child string }{
		{"Income", "Salary"},
		{"Tax", "Federal"},
		{"Tax", "State"},
		{"Tax", "Social Security"},
		{"Tax", "Medicare"},
		{"Insurance", "Health"},
	}

	assertPresent := func(t *testing.T, svc *category.Service) {
		t.Helper()
		for _, r := range required {
			parent, err := svc.GetByName(r.parent, nil)
			if err != nil {
				t.Fatalf("parent %q missing: %v", r.parent, err)
			}
			if _, err := svc.GetByName(r.child, &parent.ID); err != nil {
				t.Fatalf("child %q under parent %q missing: %v", r.child, r.parent, err)
			}
		}
	}

	t.Run("fresh file gets paycheck categories", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewServices(database)
		assertPresent(t, svc.Category)
	})

	t.Run("existing file gains missing paycheck categories on reopen", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewServices(database)

		// Simulate an existing database that pre-dates the paycheck-
		// category seed: delete one of the children and its (also
		// previously-unseeded) parent.
		taxParent, err := svc.Category.GetByName("Tax", nil)
		if err != nil {
			t.Fatalf("initial Tax parent lookup: %v", err)
		}
		fedChild, err := svc.Category.GetByName("Federal", &taxParent.ID)
		if err != nil {
			t.Fatalf("initial Federal child lookup: %v", err)
		}
		if err := svc.CategoryRepo.Delete(fedChild.ID); err != nil {
			t.Fatalf("delete Federal child: %v", err)
		}

		// Re-run service construction; this should re-create the child.
		svc2 := NewServices(database)
		assertPresent(t, svc2.Category)
	})
}
