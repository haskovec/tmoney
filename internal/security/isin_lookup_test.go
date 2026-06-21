package security

import (
	"testing"

	"github.com/haskovec/tmoney/internal/dberrors"
)

func TestGetByISIN(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	sec := NewSecurity("", "MFS Mid Cap Value CT", TypeOther)
	sec.SetISIN("US0378331005")
	if err := svc.Create(sec); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("finds by exact ISIN", func(t *testing.T) {
		got, err := svc.GetByISIN("US0378331005")
		if err != nil {
			t.Fatalf("GetByISIN() error = %v", err)
		}
		if got.ID != sec.ID {
			t.Errorf("GetByISIN returned wrong security")
		}
	})

	t.Run("is case-insensitive", func(t *testing.T) {
		got, err := svc.GetByISIN("us0378331005")
		if err != nil {
			t.Fatalf("GetByISIN() lower-case error = %v", err)
		}
		if got.ID != sec.ID {
			t.Errorf("GetByISIN lower-case returned wrong security")
		}
	})

	t.Run("empty ISIN never matches", func(t *testing.T) {
		_, err := svc.GetByISIN("")
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError for empty ISIN, got %v", err)
		}
	})

	t.Run("unknown ISIN returns not found", func(t *testing.T) {
		_, err := svc.GetByISIN("US4592001014")
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %v", err)
		}
	})
}

func TestISINUniqueness(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	a := NewSecurity("AAPL", "Apple Inc.", TypeStock)
	a.SetISIN("US0378331005")
	if err := svc.Create(a); err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	// A different security may not reuse the same ISIN (even normalized/cased).
	b := NewSecurity("AAPL2", "Apple Dup", TypeStock)
	b.SetISIN("us0378331005")
	if err := svc.Create(b); err == nil {
		t.Error("Create() expected duplicate-ISIN error")
	}
}

func TestResolve(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	tickered := NewSecurity("AAPL", "Apple Inc.", TypeStock)
	if err := svc.Create(tickered); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	cit := NewSecurity("", "MFS Mid Cap Value CT", TypeOther)
	cit.SetISIN("US0378331005")
	if err := svc.Create(cit); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("by ticker", func(t *testing.T) {
		got, err := svc.Resolve("AAPL", "", "")
		if err != nil || got.ID != tickered.ID {
			t.Fatalf("Resolve by ticker = %v, %v", got, err)
		}
	})

	t.Run("by ISIN", func(t *testing.T) {
		got, err := svc.Resolve("", "US0378331005", "")
		if err != nil || got.ID != cit.ID {
			t.Fatalf("Resolve by ISIN = %v, %v", got, err)
		}
	})

	t.Run("by exact name", func(t *testing.T) {
		got, err := svc.Resolve("", "", "MFS Mid Cap Value CT")
		if err != nil || got.ID != cit.ID {
			t.Fatalf("Resolve by name = %v, %v", got, err)
		}
	})

	t.Run("no selector errors", func(t *testing.T) {
		if _, err := svc.Resolve("", "", ""); err == nil {
			t.Error("expected error when no selector given")
		}
	})

	t.Run("multiple selectors error", func(t *testing.T) {
		if _, err := svc.Resolve("AAPL", "US0378331005", ""); err == nil {
			t.Error("expected error when more than one selector given")
		}
	})

	t.Run("ambiguous name errors", func(t *testing.T) {
		// Two tickered securities can share a name (allowed); --name must then
		// refuse rather than guess.
		d1 := NewSecurity("DUP1", "Shared Name Fund", TypeStock)
		d2 := NewSecurity("DUP2", "Shared Name Fund", TypeStock)
		if err := svc.Create(d1); err != nil {
			t.Fatalf("Create() d1 = %v", err)
		}
		if err := svc.Create(d2); err != nil {
			t.Fatalf("Create() d2 = %v", err)
		}
		if _, err := svc.Resolve("", "", "Shared Name Fund"); err == nil {
			t.Error("expected ambiguous-name error")
		}
	})
}
