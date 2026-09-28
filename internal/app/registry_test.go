package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

// Services exports services only. A repository field would let a caller skip
// the rules of the service built on it, which W9 removed.
func TestServices_ExportsNoRepository(t *testing.T) {
	st := reflect.TypeFor[Services]()
	if st.NumField() == 0 {
		t.Fatal("Services has no fields, so this check proves nothing")
	}
	for i := range st.NumField() {
		f := st.Field(i)
		typ := f.Type
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if strings.HasSuffix(typ.Name(), "Repository") {
			t.Errorf("Services.%s is a %s; give the reader a service method instead", f.Name, f.Type)
		}
	}
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
		if svc.InvestmentValuation == nil {
			t.Error("InvestmentValuation service should not be nil")
		}
		if svc.InvestmentEdit == nil {
			t.Error("InvestmentEdit service should not be nil")
		}
		if svc.CorporateAction == nil {
			t.Error("CorporateAction service should not be nil")
		}
		if svc.TransferLink == nil {
			t.Error("TransferLink service should not be nil")
		}
		if svc.Transfer == nil {
			t.Error("Transfer service should not be nil")
		}
	})

	t.Run("services are functional", func(t *testing.T) {
		_, err := svc.Account.List(true)
		if err != nil {
			t.Errorf("Account.List() error = %v", err)
		}

		_, err = svc.Category.List()
		if err != nil {
			t.Errorf("Category.List() error = %v", err)
		}

		_, err = svc.Payee.List()
		if err != nil {
			t.Errorf("Payee.List() error = %v", err)
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
		if err := svc.Prepare(); err != nil {
			t.Fatal(err)
		}
		assertPresent(t, svc.Category)
	})

	t.Run("existing file gains missing paycheck categories on reopen", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewServices(database)
		if err := svc.Prepare(); err != nil {
			t.Fatal(err)
		}

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
		if err := category.NewRepository(database).Delete(fedChild.ID); err != nil {
			t.Fatalf("delete Federal child: %v", err)
		}

		// Reopen; Prepare should re-create the child.
		svc2 := NewServices(database)
		if err := svc2.Prepare(); err != nil {
			t.Fatal(err)
		}
		assertPresent(t, svc2.Category)
	})
}
