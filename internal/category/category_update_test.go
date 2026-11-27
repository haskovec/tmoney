package category

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

func TestService_Update_ReferencedCategory(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}

	parent := NewCategory("Food", TypeExpense)
	otherParent := NewCategory("Living", TypeExpense)
	if err := svc.Create(parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := svc.Create(otherParent); err != nil {
		t.Fatalf("create otherParent: %v", err)
	}
	child := NewSubcategory("Groceries", parent.ID, TypeExpense)
	if err := svc.Create(child); err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Reference child across every table with an FK or reference to categories.
	payeeID := types.NewID()
	txnID := types.NewID()
	schedID := types.NewID()
	stmts := []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO payees (id, name, default_category_id) VALUES (CAST(? AS UUID), 'Market', CAST(? AS UUID))`,
			[]any{payeeID.String(), child.ID.String()},
		},
		{
			`INSERT INTO transactions (id, account_id, date, amount, payee_id, category_id, status) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, -25.00, CAST(? AS UUID), CAST(? AS UUID), 'uncleared')`,
			[]any{txnID.String(), acct.ID.String(), types.Today().String(), payeeID.String(), child.ID.String()},
		},
		{
			`INSERT INTO transaction_splits (id, transaction_id, category_id, amount) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), -25.00)`,
			[]any{types.NewID().String(), txnID.String(), child.ID.String()},
		},
		{
			`INSERT INTO scheduled_transactions (id, account_id, payee_id, category_id, amount, frequency, start_date, next_date) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), -25.00, 'monthly', ?, ?)`,
			[]any{schedID.String(), acct.ID.String(), payeeID.String(), child.ID.String(), types.Today().String(), types.Today().String()},
		},
		{
			`INSERT INTO scheduled_split_items (id, scheduled_transaction_id, category_id, amount) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), -25.00)`,
			[]any{types.NewID().String(), schedID.String(), child.ID.String()},
		},
	}
	for _, s := range stmts {
		if _, err := database.Conn().Exec(s.sql, s.args...); err != nil {
			t.Fatalf("seed ref: %v", err)
		}
	}

	t.Run("rename referenced parent category", func(t *testing.T) {
		parent.Name = "Food & Dining"
		if err := svc.Update(parent); err != nil {
			t.Fatalf("Update(parent) error = %v", err)
		}
		got, err := svc.GetByID(parent.ID)
		if err != nil {
			t.Fatalf("GetByID(parent) error = %v", err)
		}
		if got.Name != "Food & Dining" {
			t.Errorf("parent.Name = %q, want %q", got.Name, "Food & Dining")
		}
	})

	t.Run("rename and reparent referenced child category", func(t *testing.T) {
		child.Name = "Supermarket"
		child.SetParent(otherParent.ID)
		if err := svc.Update(child); err != nil {
			t.Fatalf("Update(child) error = %v", err)
		}
		got, err := svc.GetByID(child.ID)
		if err != nil {
			t.Fatalf("GetByID(child) error = %v", err)
		}
		if got.Name != "Supermarket" {
			t.Errorf("child.Name = %q, want %q", got.Name, "Supermarket")
		}
		if !got.ParentID.Valid || got.ParentID.ID != otherParent.ID {
			t.Errorf("child.ParentID = %+v, want %s", got.ParentID, otherParent.ID)
		}
	})

	t.Run("promote referenced child to top-level", func(t *testing.T) {
		child.ClearParent()
		if err := svc.Update(child); err != nil {
			t.Fatalf("Update(child clear parent) error = %v", err)
		}
		got, err := svc.GetByID(child.ID)
		if err != nil {
			t.Fatalf("GetByID(child) error = %v", err)
		}
		if got.ParentID.Valid {
			t.Errorf("expected top-level category, got ParentID=%+v", got.ParentID)
		}
	})
}

func TestRepository_Update_FailedWritePreservesRow(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)

	cat := NewCategory("Utilities", TypeExpense)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Trigger a DB CHECK violation (invalid type) directly on repo.Update.
	// With DELETE+INSERT outside a transaction this permanently deleted the row.
	bad := *cat
	bad.Type = Type("bogus")
	if err := repo.Update(&bad); err == nil {
		t.Fatal("expected error updating category with invalid type")
	}

	got, err := repo.GetByID(cat.ID)
	if err != nil {
		t.Fatalf("category was lost after failed Update(): %v", err)
	}
	if got.Name != "Utilities" || got.Type != TypeExpense {
		t.Errorf("category mutated after failed Update(): got %+v", got)
	}
}

func TestService_MergeCategories_WithReferencedChild(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}

	source := NewCategory("OldFood", TypeExpense)
	target := NewCategory("NewFood", TypeExpense)
	if err := svc.Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := svc.Create(target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	child := NewSubcategory("Coffee", source.ID, TypeExpense)
	if err := svc.Create(child); err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Reference the child subcategory from a transaction; MergeCategories
	// reparents child from source -> target via b.repo.Update(child).
	if _, err := database.Conn().Exec(
		`INSERT INTO transactions (id, account_id, date, amount, category_id, status) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, -5.00, CAST(? AS UUID), 'uncleared')`,
		types.NewID().String(), acct.ID.String(), types.Today().String(), child.ID.String(),
	); err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	if err := svc.MergeCategories(source.ID, target.ID); err != nil {
		t.Fatalf("MergeCategories() error = %v", err)
	}

	got, err := svc.GetByID(child.ID)
	if err != nil {
		t.Fatalf("GetByID(child) error = %v", err)
	}
	if !got.ParentID.Valid || got.ParentID.ID != target.ID {
		t.Errorf("child.ParentID = %+v, want %s", got.ParentID, target.ID)
	}
}

func TestRepository_Delete_PayeeDefaultAndScheduledSplitGuards(t *testing.T) {
	t.Run("blocks delete when referenced by payee default_category_id", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		cat := NewCategory("PayeeDefaultCat", TypeExpense)
		if err := repo.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if _, err := database.Conn().Exec(
			`INSERT INTO payees (id, name, default_category_id) VALUES (CAST(? AS UUID), 'Coffee Shop', CAST(? AS UUID))`,
			types.NewID().String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("insert payee: %v", err)
		}

		err := repo.Delete(cat.ID)
		if err == nil {
			t.Fatal("Delete() expected error when referenced by payee default_category_id")
		}
		var dep *dberrors.HasDependentsError
		if !errors.As(err, &dep) {
			t.Fatalf("expected HasDependentsError, got %T: %v", err, err)
		}
		if dep.Dependents != "payees" || dep.Count != 1 {
			t.Errorf("got Dependents=%q Count=%d, want payees / 1", dep.Dependents, dep.Count)
		}
	})

	t.Run("blocks delete when referenced by scheduled_split_items", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		cat := NewCategory("SchedSplitCat", TypeExpense)
		if err := repo.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if _, err := database.Conn().Exec(
			`INSERT INTO scheduled_split_items (id, scheduled_transaction_id, category_id, amount) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), -10.00)`,
			types.NewID().String(), types.NewID().String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("insert scheduled_split_item: %v", err)
		}

		err := repo.Delete(cat.ID)
		if err == nil {
			t.Fatal("Delete() expected error when referenced by scheduled_split_items")
		}
		var dep *dberrors.HasDependentsError
		if !errors.As(err, &dep) {
			t.Fatalf("expected HasDependentsError, got %T: %v", err, err)
		}
		if dep.Dependents != "scheduled split lines" || dep.Count != 1 {
			t.Errorf("got Dependents=%q Count=%d, want scheduled split lines / 1", dep.Dependents, dep.Count)
		}
	})
}

func TestCategory_Update_HierarchyAndSystemGuards(t *testing.T) {
	setup := func(t *testing.T) (*Service, *Category, *Category, *Category) {
		t.Helper()
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		parent := NewCategory("Food", TypeExpense)
		otherParent := NewCategory("Living", TypeExpense)
		if err := svc.Create(parent); err != nil {
			t.Fatalf("create parent: %v", err)
		}
		if err := svc.Create(otherParent); err != nil {
			t.Fatalf("create otherParent: %v", err)
		}
		child := NewSubcategory("Groceries", parent.ID, TypeExpense)
		if err := svc.Create(child); err != nil {
			t.Fatalf("create child: %v", err)
		}
		return svc, parent, otherParent, child
	}

	t.Run("rejects self-parenting", func(t *testing.T) {
		svc, _, otherParent, _ := setup(t)
		attempt := *otherParent
		attempt.SetParent(otherParent.ID)
		if err := svc.Update(&attempt); err == nil {
			t.Fatal("Update() expected error when setting category as its own parent")
		}
		got, err := svc.GetByID(otherParent.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.ParentID.Valid {
			t.Errorf("ParentID mutated to %+v, want invalid", got.ParentID)
		}
	})

	t.Run("rejects making a category with subcategories into a subcategory", func(t *testing.T) {
		svc, parent, otherParent, _ := setup(t)
		attempt := *parent
		attempt.SetParent(otherParent.ID)
		if err := svc.Update(&attempt); err == nil {
			t.Fatal("Update() expected error when reparenting a category that has children")
		}
		got, err := svc.GetByID(parent.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.ParentID.Valid {
			t.Errorf("ParentID mutated to %+v, want invalid", got.ParentID)
		}
	})

	t.Run("rejects changing type of a category with subcategories", func(t *testing.T) {
		svc, parent, _, _ := setup(t)
		attempt := *parent
		attempt.Type = TypeIncome
		if err := svc.Update(&attempt); err == nil {
			t.Fatal("Update() expected error when changing type of category with children")
		}
		got, err := svc.GetByID(parent.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.Type != TypeExpense {
			t.Errorf("Type mutated to %q, want %q", got.Type, TypeExpense)
		}
	})

	t.Run("rejects escalating a user category to system category", func(t *testing.T) {
		svc, _, otherParent, _ := setup(t)
		attempt := *otherParent
		attempt.IsSystem = true
		if err := svc.Update(&attempt); err == nil {
			t.Fatal("Update() expected error when setting IsSystem = true on user category")
		}
		got, err := svc.GetByID(otherParent.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.IsSystem {
			t.Error("IsSystem mutated to true, want false")
		}
	})
}
