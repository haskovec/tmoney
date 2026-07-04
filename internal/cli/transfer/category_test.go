package transfer

import (
	"errors"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/transaction"
)

func TestResolveTransferCategory_Empty(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	for _, in := range []string{"", "   "} {
		got, err := resolveTransferCategory(svc, in)
		if err != nil {
			t.Fatalf("resolveTransferCategory(%q): %v", in, err)
		}
		if got.Valid {
			t.Errorf("resolveTransferCategory(%q) = valid %v, want cleared", in, got)
		}
	}
}

func TestResolveTransferCategory_ParentPath(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	bills := category.NewCategory("Bills", category.TypeExpense)
	if err := svc.Category.Create(bills); err != nil {
		t.Fatalf("create category: %v", err)
	}

	got, err := resolveTransferCategory(svc, "Bills")
	if err != nil {
		t.Fatalf("resolveTransferCategory: %v", err)
	}
	if !got.Valid || got.ID != bills.ID {
		t.Errorf("resolveTransferCategory(Bills) = %+v, want id %s", got, bills.ID)
	}
}

func TestResolveTransferCategory_SubcategoryPath(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	parent := category.NewCategory("Bills", category.TypeExpense)
	if err := svc.Category.Create(parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child := category.NewSubcategory("Credit Card", parent.ID, category.TypeExpense)
	if err := svc.Category.Create(child); err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := resolveTransferCategory(svc, "Bills:Credit Card")
	if err != nil {
		t.Fatalf("resolveTransferCategory: %v", err)
	}
	if !got.Valid || got.ID != child.ID {
		t.Errorf("resolveTransferCategory(Bills:Credit Card) = %+v, want child id %s", got, child.ID)
	}
}

func TestResolveTransferCategory_Unknown(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	_, err := resolveTransferCategory(svc, "Nonexistent")
	if err == nil {
		t.Fatal("resolveTransferCategory(Nonexistent) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err.Error())
	}
}

func TestResolveTransferCategory_UnknownSubcategory(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	parent := category.NewCategory("Bills", category.TypeExpense)
	if err := svc.Category.Create(parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	_, err := resolveTransferCategory(svc, "Bills:Nope")
	if err == nil {
		t.Fatal("resolveTransferCategory(Bills:Nope) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err.Error())
	}
}

func TestResolveTransferCategory_SystemCategoryRejected(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	// OpenSvc → app.NewServices seeds the system "Value Adjustment" category.
	svc := clitest.OpenSvc(t, dbPath)

	_, err := resolveTransferCategory(svc, category.ValueAdjustmentCategoryName)
	if err == nil {
		t.Fatal("resolveTransferCategory(Value Adjustment) = nil error, want system-category rejection")
	}
	var sysErr *transaction.SystemCategoryTransferError
	if !errors.As(err, &sysErr) {
		t.Errorf("error = %v (%T), want *transaction.SystemCategoryTransferError", err, err)
	}
}
