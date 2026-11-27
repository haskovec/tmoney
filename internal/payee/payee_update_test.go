package payee

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

func TestService_Update_ReferencedPayee(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	acctID := types.NewID()
	cat1ID := types.NewID()
	cat2ID := types.NewID()

	if _, err := database.Conn().Exec(
		`INSERT INTO accounts (id, name, type, opening_balance, opening_date) VALUES (CAST(? AS UUID), 'Checking', 'checking', 0, ?)`,
		acctID.String(), types.Today().String(),
	); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	for _, c := range []struct {
		id   types.ID
		name string
	}{{cat1ID, "Groceries"}, {cat2ID, "Household"}} {
		if _, err := database.Conn().Exec(
			`INSERT INTO categories (id, name, type) VALUES (CAST(? AS UUID), ?, 'expense')`,
			c.id.String(), c.name,
		); err != nil {
			t.Fatalf("insert category %s: %v", c.name, err)
		}
	}

	p := NewPayeeWithCategory("Trader Joes", cat1ID)
	if err := svc.Create(p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Reference payee from payee_aliases, transactions, and scheduled_transactions.
	alias := NewAlias(p.ID, "TRADER", MatchTypeContains)
	if err := svc.CreateAlias(alias); err != nil {
		t.Fatalf("CreateAlias() error = %v", err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO transactions (id, account_id, date, amount, payee_id, status) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, -42.00, CAST(? AS UUID), 'uncleared')`,
		types.NewID().String(), acctID.String(), types.Today().String(), p.ID.String(),
	); err != nil {
		t.Fatalf("insert transaction: %v", err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO scheduled_transactions (id, account_id, payee_id, amount, frequency, start_date, next_date) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), -42.00, 'monthly', ?, ?)`,
		types.NewID().String(), acctID.String(), p.ID.String(), types.Today().String(), types.Today().String(),
	); err != nil {
		t.Fatalf("insert scheduled: %v", err)
	}

	t.Run("rename referenced payee", func(t *testing.T) {
		p.Name = "Trader Joe's"
		p.SetNotes("neighborhood market")
		if err := svc.Update(p); err != nil {
			t.Fatalf("Update(rename) error = %v", err)
		}
		got, err := svc.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.Name != "Trader Joe's" || got.Notes.String != "neighborhood market" {
			t.Errorf("got Name=%q Notes=%q", got.Name, got.Notes.String)
		}
	})

	t.Run("change default category on referenced payee", func(t *testing.T) {
		if err := svc.SetDefaultCategory(p.ID, cat2ID); err != nil {
			t.Fatalf("SetDefaultCategory() error = %v", err)
		}
		got, err := svc.GetDefaultCategory(p.ID)
		if err != nil {
			t.Fatalf("GetDefaultCategory() error = %v", err)
		}
		if got == nil || *got != cat2ID {
			t.Errorf("DefaultCategory = %v, want %s", got, cat2ID)
		}
	})

	t.Run("clear default category on referenced payee", func(t *testing.T) {
		if err := svc.ClearDefaultCategory(p.ID); err != nil {
			t.Fatalf("ClearDefaultCategory() error = %v", err)
		}
		got, err := svc.GetDefaultCategory(p.ID)
		if err != nil {
			t.Fatalf("GetDefaultCategory() error = %v", err)
		}
		if got != nil {
			t.Errorf("DefaultCategory = %v, want nil", got)
		}
	})
}

func TestService_SetDefaultCategory_InvalidCategoryPreservesPayee(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	p := NewPayee("Do Not Delete Me")
	if err := svc.Create(p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Previously, DELETE+INSERT outside a tx deleted the payee before failing on
	// the default_category_id FK, permanently losing the payee row.
	err := svc.SetDefaultCategory(p.ID, types.NewID())
	if err == nil {
		t.Fatal("SetDefaultCategory() expected error for nonexistent category")
	}
	var nf *dberrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}

	got, getErr := svc.GetByID(p.ID)
	if getErr != nil {
		t.Fatalf("payee was lost after failed SetDefaultCategory(): %v", getErr)
	}
	if got.Name != "Do Not Delete Me" || got.DefaultCategoryID.Valid {
		t.Errorf("payee corrupted after failed update: %+v", got)
	}
}

func TestRepository_Create_RejectsNonexistentDefaultCategory(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)

	p := NewPayeeWithCategory("Bad Default", types.NewID())
	err := repo.Create(p)
	if err == nil {
		t.Fatal("Create() expected error for nonexistent default_category_id")
	}
	var nf *dberrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}
