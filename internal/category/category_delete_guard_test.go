package category

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// TestRepository_Delete_SplitLinesGuard verifies that Delete refuses to
// remove a category still referenced by a transaction split line, returning
// a HasDependentsError whose Dependents reads "split lines".
func TestRepository_Delete_SplitLinesGuard(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)

	cat := NewCategory("SplitTarget", TypeExpense)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Insert a split line referencing the category. transaction_splits has
	// no FK on transaction_id (dropped in migration 026), so a synthetic
	// transaction_id is fine for exercising the guard.
	_, err := database.Conn().Exec(`
		INSERT INTO transaction_splits (id, transaction_id, category_id, amount)
		VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?)
	`, types.NewID().String(), types.NewID().String(), cat.ID.String(), "-10.00")
	if err != nil {
		t.Fatalf("insert split: %v", err)
	}

	err = repo.Delete(cat.ID)
	if err == nil {
		t.Fatal("Delete() expected error for category with split lines")
	}
	dep, ok := err.(*dberrors.HasDependentsError)
	if !ok {
		t.Fatalf("expected HasDependentsError, got %T: %v", err, err)
	}
	if dep.Dependents != "split lines" {
		t.Errorf("Dependents = %q, want %q", dep.Dependents, "split lines")
	}
	if dep.Count != 1 {
		t.Errorf("Count = %d, want 1", dep.Count)
	}
}

// TestRepository_Delete_ScheduledGuard verifies that Delete refuses to remove
// a category still referenced by a scheduled transaction, returning a
// HasDependentsError whose Dependents reads "scheduled transactions".
func TestRepository_Delete_ScheduledGuard(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)

	cat := NewCategory("ScheduledTarget", TypeExpense)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// A scheduled transaction requires a real account (account_id FK).
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, err := database.Conn().Exec(`
		INSERT INTO scheduled_transactions
			(id, account_id, category_id, amount, frequency, start_date, next_date)
		VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?, 'monthly', ?, ?)
	`, types.NewID().String(), acct.ID.String(), cat.ID.String(), "-25.00",
		types.Today().String(), types.Today().String())
	if err != nil {
		t.Fatalf("insert scheduled: %v", err)
	}

	err = repo.Delete(cat.ID)
	if err == nil {
		t.Fatal("Delete() expected error for category with scheduled transactions")
	}
	dep, ok := err.(*dberrors.HasDependentsError)
	if !ok {
		t.Fatalf("expected HasDependentsError, got %T: %v", err, err)
	}
	if dep.Dependents != "scheduled transactions" {
		t.Errorf("Dependents = %q, want %q", dep.Dependents, "scheduled transactions")
	}
	if dep.Count != 1 {
		t.Errorf("Count = %d, want 1", dep.Count)
	}
}

// TestService_Delete_SplitLinesGuard verifies the guard surfaces through the
// service layer as well.
func TestService_Delete_SplitLinesGuard(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	cat := NewCategory("SvcSplitTarget", TypeExpense)
	if err := svc.Create(cat); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := database.Conn().Exec(`
		INSERT INTO transaction_splits (id, transaction_id, category_id, amount)
		VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?)
	`, types.NewID().String(), types.NewID().String(), cat.ID.String(), "-10.00")
	if err != nil {
		t.Fatalf("insert split: %v", err)
	}

	err = svc.Delete(cat.ID)
	if err == nil {
		t.Fatal("Delete() expected error for category with split lines")
	}
	if !strings.Contains(err.Error(), "split lines") {
		t.Errorf("expected split-lines dependents error, got: %v", err)
	}
}
