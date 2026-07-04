package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// newInvCatEnv builds an investment Service plus the repositories needed to
// seed a category and read back the regular-side transfer leg.
func newInvCatEnv(t *testing.T) (svc *Service, accountRepo *account.Repository, catRepo *category.Repository, regRepo *transaction.Repository) {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo = account.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	priceRepo := price.NewRepository(database)
	regRepo = transaction.NewRepository(database)
	caRepo := NewCorporateActionRepository(database)
	svc = NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, regRepo, caRepo, database)
	catRepo = category.NewRepository(database)
	return svc, accountRepo, catRepo, regRepo
}

func newInvCategory(t *testing.T, catRepo *category.Repository, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	return cat
}

// TestDepositFromAccount_CategoryOnRegularLeg checks a reg→inv transfer labels
// the regular-side (bank) leg with the category. The investment-side leg has no
// category column, so there is nothing to label there.
func TestDepositFromAccount_CategoryOnRegularLeg(t *testing.T) {
	svc, accountRepo, catRepo, regRepo := newInvCatEnv(t)
	inv := createInvAccount(t, accountRepo, "Brokerage")
	check := createCheckAccount(t, accountRepo, "Checking")
	bills := newInvCategory(t, catRepo, "Bills")
	date := types.NewDate(2024, time.March, 15)

	result, err := svc.DepositFromAccount(inv.ID, check.ID, date, types.MustNewMoney("500.00"),
		"contribution", types.NullableID{ID: bills.ID, Valid: true})
	if err != nil {
		t.Fatalf("DepositFromAccount: %v", err)
	}

	// In-memory regular leg carries the category.
	if !result.RegularTransaction.CategoryID.Valid || result.RegularTransaction.CategoryID.ID != bills.ID {
		t.Fatalf("regular leg should carry category %s, got valid=%v id=%v",
			bills.ID, result.RegularTransaction.CategoryID.Valid, result.RegularTransaction.CategoryID.ID)
	}

	// Persisted regular leg carries the category.
	reg, err := regRepo.GetByID(result.RegularTransaction.ID)
	if err != nil {
		t.Fatalf("reload regular leg: %v", err)
	}
	if !reg.CategoryID.Valid || reg.CategoryID.ID != bills.ID {
		t.Fatalf("persisted regular leg should carry category %s, got valid=%v id=%v",
			bills.ID, reg.CategoryID.Valid, reg.CategoryID.ID)
	}
}

// TestTransferCash_CategoryOnRegularLeg checks the inv→reg direction labels the
// regular-side leg with the category.
func TestTransferCash_CategoryOnRegularLeg(t *testing.T) {
	svc, accountRepo, catRepo, regRepo := newInvCatEnv(t)
	inv := createInvAccount(t, accountRepo, "Brokerage")
	check := createCheckAccount(t, accountRepo, "Checking")
	bills := newInvCategory(t, catRepo, "Bills")
	date := types.NewDate(2024, time.March, 15)

	result, err := svc.TransferCash(inv.ID, check.ID, date, types.MustNewMoney("250.00"),
		"draw", types.NullableID{ID: bills.ID, Valid: true})
	if err != nil {
		t.Fatalf("TransferCash: %v", err)
	}

	reg, err := regRepo.GetByID(result.RegularTransaction.ID)
	if err != nil {
		t.Fatalf("reload regular leg: %v", err)
	}
	if !reg.CategoryID.Valid || reg.CategoryID.ID != bills.ID {
		t.Fatalf("persisted regular leg should carry category %s, got valid=%v id=%v",
			bills.ID, reg.CategoryID.Valid, reg.CategoryID.ID)
	}
}

// TestTransferCash_NoCategoryLeavesBankLegBare checks the default (no category)
// path is unchanged.
func TestTransferCash_NoCategoryLeavesBankLegBare(t *testing.T) {
	svc, accountRepo, _, regRepo := newInvCatEnv(t)
	inv := createInvAccount(t, accountRepo, "Brokerage")
	check := createCheckAccount(t, accountRepo, "Checking")
	date := types.NewDate(2024, time.March, 15)

	result, err := svc.TransferCash(inv.ID, check.ID, date, types.MustNewMoney("250.00"), "", types.NullableID{})
	if err != nil {
		t.Fatalf("TransferCash: %v", err)
	}
	reg, err := regRepo.GetByID(result.RegularTransaction.ID)
	if err != nil {
		t.Fatalf("reload regular leg: %v", err)
	}
	if reg.CategoryID.Valid {
		t.Fatalf("regular leg should have no category, got %s", reg.CategoryID.ID)
	}
}
