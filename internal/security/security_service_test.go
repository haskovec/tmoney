package security

import (
	"fmt"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// mockLotChecker implements OpenLotChecker for testing.
type mockLotChecker struct {
	hasLots bool
	err     error
}

func (m *mockLotChecker) HasOpenLots(_ types.ID) (bool, error) {
	return m.hasLots, m.err
}

// mockPositionChecker implements OpenPositionChecker for testing.
type mockPositionChecker struct {
	hasPositions bool
	err          error
}

func (m *mockPositionChecker) HasOpenPositions(_ types.ID) (bool, error) {
	return m.hasPositions, m.err
}

// =============================================================================
// SM-015: Service.Create
// =============================================================================

func TestService_Create(t *testing.T) {
	t.Run("creates valid security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)

		err := svc.Create(sec)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", retrieved.Ticker)
		}
		if retrieved.Name != "Apple Inc." {
			t.Errorf("Expected name 'Apple Inc.', got %q", retrieved.Name)
		}
		if retrieved.SecurityType != TypeStock {
			t.Errorf("Expected type stock, got %q", retrieved.SecurityType)
		}
		if retrieved.AssetClass != AssetClassUnclassified {
			t.Errorf("Expected asset class unclassified, got %q", retrieved.AssetClass)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", retrieved.Currency)
		}
		if retrieved.Hidden {
			t.Error("Expected hidden to be false")
		}
	})

	t.Run("accepts tickerless security with a name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("", "MFS Mid Cap Value CT", TypeOther)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() unexpected error for tickerless security: %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "" {
			t.Errorf("Expected empty ticker, got %q", retrieved.Ticker)
		}
		if retrieved.Name != "MFS Mid Cap Value CT" {
			t.Errorf("Expected name preserved, got %q", retrieved.Name)
		}
	})

	t.Run("rejects a second tickerless security with the same name+currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		if err := svc.Create(NewSecurity("", "MFS Mid Cap Value CT", TypeOther)); err != nil {
			t.Fatalf("Create() first error = %v", err)
		}
		err := svc.Create(NewSecurity("", "MFS Mid Cap Value CT", TypeOther))
		if err == nil {
			t.Error("Create() expected duplicate error for second tickerless name+currency")
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "", TypeStock)
		err := svc.Create(sec)
		if err == nil {
			t.Error("Create() expected error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects invalid security type", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", Type("invalid"))
		err := svc.Create(sec)
		if err == nil {
			t.Error("Create() expected error for invalid security type")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects duplicate ticker and currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec1 := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec1); err != nil {
			t.Fatalf("Create() first error = %v", err)
		}

		sec2 := NewSecurity("AAPL", "Apple Inc. Duplicate", TypeStock)
		err := svc.Create(sec2)
		if err == nil {
			t.Error("Create() expected error for duplicate ticker+currency")
		}
	})

	t.Run("allows same ticker with different currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec1 := NewSecurity("AAPL", "Apple Inc. USD", TypeStock)
		sec1.Currency = "USD"
		if err := svc.Create(sec1); err != nil {
			t.Fatalf("Create() first error = %v", err)
		}

		sec2 := NewSecurity("AAPL", "Apple Inc. EUR", TypeStock)
		sec2.Currency = "EUR"
		err := svc.Create(sec2)
		if err != nil {
			t.Fatalf("Create() second error = %v (should allow same ticker with different currency)", err)
		}
	})
}

// =============================================================================
// SM-016: Service.Update
// =============================================================================

func TestService_Update(t *testing.T) {
	t.Run("updates valid security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sec.Name = "Apple Corporation"
		sec.AssetClass = AssetClassLargeCapStock
		sec.SetExchange("NASDAQ")
		if err := svc.Update(sec); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Apple Corporation" {
			t.Errorf("Expected name 'Apple Corporation', got %q", retrieved.Name)
		}
		if retrieved.AssetClass != AssetClassLargeCapStock {
			t.Errorf("Expected asset class large_cap_stock, got %q", retrieved.AssetClass)
		}
		if !retrieved.Exchange.Valid || retrieved.Exchange.String != "NASDAQ" {
			t.Errorf("Expected exchange 'NASDAQ', got %v", retrieved.Exchange)
		}
	})

	t.Run("allows ticker change", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sec.Ticker = "AAPL2"
		if err := svc.Update(sec); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "AAPL2" {
			t.Errorf("Expected ticker 'AAPL2', got %q", retrieved.Ticker)
		}
	})

	t.Run("rejects invalid update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sec.ISIN = "US0378331004" // Invalid: wrong check digit
		err := svc.Update(sec)
		if err == nil {
			t.Error("Update() expected error for invalid ISIN")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects duplicate ticker+currency on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec1 := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec2 := NewSecurity("GOOG", "Alphabet Inc.", TypeStock)
		for _, s := range []*Security{sec1, sec2} {
			if err := svc.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		sec2.Ticker = "AAPL" // Conflict with sec1
		err := svc.Update(sec2)
		if err == nil {
			t.Error("Update() expected error for duplicate ticker+currency")
		}
	})
}

// =============================================================================
// SM-017: Service.Hide and Unhide
// =============================================================================

func TestService_Hide(t *testing.T) {
	t.Run("hides security with no positions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Hidden {
			t.Error("Security should be hidden after Hide()")
		}
	})

	t.Run("rejects hiding already hidden security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		err := svc.Hide(sec.ID)
		if err == nil {
			t.Error("Hide() expected error for already hidden security")
		}
		if _, ok := err.(*AlreadyHiddenError); !ok {
			t.Errorf("Expected AlreadyHiddenError, got %T", err)
		}
	})

	t.Run("rejects hiding non-existent security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.Hide(types.NewID())
		if err == nil {
			t.Error("Hide() expected error for non-existent security")
		}
	})

	// SM-096: Wire SecurityService.Hide to real position check

	t.Run("rejects hiding security with open lots", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database,
			WithLotChecker(&mockLotChecker{hasLots: true}),
			WithPositionChecker(&mockPositionChecker{hasPositions: false}),
		)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Hide(sec.ID)
		if err == nil {
			t.Fatal("Hide() expected error when security has open lots")
		}
		if _, ok := err.(*HasOpenPositionsError); !ok {
			t.Errorf("Expected HasOpenPositionsError, got %T: %v", err, err)
		}
	})

	t.Run("rejects hiding security with non-zero positions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database,
			WithLotChecker(&mockLotChecker{hasLots: false}),
			WithPositionChecker(&mockPositionChecker{hasPositions: true}),
		)

		sec := NewSecurity("MSFT", "Microsoft Corp.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Hide(sec.ID)
		if err == nil {
			t.Fatal("Hide() expected error when security has open positions")
		}
		if _, ok := err.(*HasOpenPositionsError); !ok {
			t.Errorf("Expected HasOpenPositionsError, got %T: %v", err, err)
		}
	})

	t.Run("allows hiding when all positions are zero and lots are closed", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database,
			WithLotChecker(&mockLotChecker{hasLots: false}),
			WithPositionChecker(&mockPositionChecker{hasPositions: false}),
		)

		sec := NewSecurity("GOOG", "Alphabet Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Hidden {
			t.Error("Security should be hidden")
		}
	})

	t.Run("returns error when lot checker fails", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database,
			WithLotChecker(&mockLotChecker{err: fmt.Errorf("db connection lost")}),
			WithPositionChecker(&mockPositionChecker{hasPositions: false}),
		)

		sec := NewSecurity("TSLA", "Tesla Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Hide(sec.ID)
		if err == nil {
			t.Fatal("Hide() expected error when lot checker fails")
		}
		if _, ok := err.(*HasOpenPositionsError); ok {
			t.Error("Should be a wrapped error, not HasOpenPositionsError")
		}
	})

	t.Run("returns error when position checker fails", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database,
			WithLotChecker(&mockLotChecker{hasLots: false}),
			WithPositionChecker(&mockPositionChecker{err: fmt.Errorf("db connection lost")}),
		)

		sec := NewSecurity("AMZN", "Amazon.com Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Hide(sec.ID)
		if err == nil {
			t.Fatal("Hide() expected error when position checker fails")
		}
		if _, ok := err.(*HasOpenPositionsError); ok {
			t.Error("Should be a wrapped error, not HasOpenPositionsError")
		}
	})
}

func TestService_Unhide(t *testing.T) {
	t.Run("unhides hidden security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		if err := svc.Unhide(sec.ID); err != nil {
			t.Fatalf("Unhide() error = %v", err)
		}

		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Hidden {
			t.Error("Security should be visible after Unhide()")
		}
	})

	t.Run("rejects unhiding visible security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Unhide(sec.ID)
		if err == nil {
			t.Error("Unhide() expected error for visible security")
		}
		if _, ok := err.(*NotHiddenError); !ok {
			t.Errorf("Expected NotHiddenError, got %T", err)
		}
	})

	t.Run("rejects unhiding non-existent security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.Unhide(types.NewID())
		if err == nil {
			t.Error("Unhide() expected error for non-existent security")
		}
	})
}

// =============================================================================
// SM-018: Service.Delete
// =============================================================================

func TestService_Delete(t *testing.T) {
	t.Run("deletes security with no history", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(sec.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(sec.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("rejects deleting security with prices", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Insert a price directly
		_, err := database.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES (?, CURRENT_DATE, 150.00, 'manual')
		`, sec.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert price: %v", err)
		}

		err = svc.Delete(sec.ID)
		if err == nil {
			t.Error("Delete() expected error for security with prices")
		}
		if _, ok := err.(*HasDependentsError); !ok {
			t.Errorf("Expected HasDependentsError, got %T: %v", err, err)
		}
	})

	t.Run("suggests hiding when delete fails due to dependents", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Insert a price directly
		_, err := database.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES (?, CURRENT_DATE, 150.00, 'manual')
		`, sec.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert price: %v", err)
		}

		err = svc.Delete(sec.ID)
		if err == nil {
			t.Fatal("Delete() expected error")
		}

		depErr, ok := err.(*HasDependentsError)
		if !ok {
			t.Fatalf("Expected HasDependentsError, got %T", err)
		}
		// Error message should suggest hiding
		msg := depErr.Error()
		if !containsStr(msg, "hiding") {
			t.Errorf("Error message should suggest hiding, got: %s", msg)
		}
	})

	t.Run("rejects deleting non-existent security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.Delete(types.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent security")
		}
	})
}

// =============================================================================
// SM-019: Service.Search
// =============================================================================

func TestService_Search(t *testing.T) {
	t.Run("searches by partial ticker match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		for _, s := range []*Security{
			NewSecurity("AAPL", "Apple Inc.", TypeStock),
			NewSecurity("GOOG", "Alphabet Inc.", TypeStock),
			NewSecurity("MSFT", "Microsoft Corp.", TypeStock),
		} {
			if err := svc.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := svc.Search("AAP")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", results[0].Ticker)
		}
	})

	t.Run("searches by partial name match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		for _, s := range []*Security{
			NewSecurity("AAPL", "Apple Inc.", TypeStock),
			NewSecurity("GOOG", "Alphabet Inc.", TypeStock),
		} {
			if err := svc.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := svc.Search("alpha")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Ticker != "GOOG" {
			t.Errorf("Expected ticker 'GOOG', got %q", results[0].Ticker)
		}
	})

	t.Run("search is case-insensitive", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		for _, query := range []string{"aapl", "AAPL", "Aapl", "apple", "APPLE", "Apple"} {
			results, err := svc.Search(query)
			if err != nil {
				t.Fatalf("Search(%q) error = %v", query, err)
			}
			if len(results) != 1 {
				t.Errorf("Search(%q) expected 1 result, got %d", query, len(results))
			}
		}
	})

	t.Run("empty query returns all securities", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		for _, s := range []*Security{
			NewSecurity("AAPL", "Apple Inc.", TypeStock),
			NewSecurity("GOOG", "Alphabet Inc.", TypeStock),
		} {
			if err := svc.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := svc.Search("")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		results, err := svc.Search("xyz")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})
}

// =============================================================================
// SM-177: Hidden security enforcement
// =============================================================================

func TestService_SearchExcludesHidden(t *testing.T) {
	t.Run("search excludes hidden securities", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		visible := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		hidden := NewSecurity("GOOG", "Alphabet Inc.", TypeStock)
		if err := svc.Create(visible); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(hidden); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Hide(hidden.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		// Search by partial name should not return hidden security
		results, err := svc.Search("alpha")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results (hidden excluded), got %d", len(results))
		}

		// Search that matches visible security should return it
		results, err = svc.Search("apple")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", results[0].Ticker)
		}
	})

	t.Run("empty search query excludes hidden securities", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec1 := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec2 := NewSecurity("GOOG", "Alphabet Inc.", TypeStock)
		sec3 := NewSecurity("MSFT", "Microsoft Corp.", TypeStock)
		for _, s := range []*Security{sec1, sec2, sec3} {
			if err := svc.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}
		// Hide GOOG
		if err := svc.Hide(sec2.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		results, err := svc.Search("")
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results (hidden excluded), got %d", len(results))
		}
		for _, r := range results {
			if r.Ticker == "GOOG" {
				t.Error("Hidden security GOOG should not appear in search results")
			}
		}
	})

	t.Run("hidden security still retrievable by GetByID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		sec := NewSecurity("GOOG", "Alphabet Inc.", TypeStock)
		if err := svc.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		// GetByID should still return hidden securities (for portfolio/historical)
		retrieved, err := svc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "GOOG" {
			t.Errorf("Expected ticker 'GOOG', got %q", retrieved.Ticker)
		}
		if !retrieved.Hidden {
			t.Error("Expected Hidden to be true")
		}
	})
}

// =============================================================================
// Helpers
// =============================================================================

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
