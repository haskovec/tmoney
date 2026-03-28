package repository

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// Security Repository CRUD Tests
// =============================================================================

func TestSecurityRepository_Create(t *testing.T) {
	t.Run("creates valid security", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec.AssetClass = models.AssetClassLargeCapStock
		sec.SetExchange("NASDAQ")

		err := repo.Create(sec)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was persisted by reading it back
		retrieved, err := repo.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", retrieved.Ticker)
		}
		if retrieved.Name != "Apple Inc." {
			t.Errorf("Expected name 'Apple Inc.', got %q", retrieved.Name)
		}
		if retrieved.SecurityType != models.SecurityTypeStock {
			t.Errorf("Expected type stock, got %q", retrieved.SecurityType)
		}
		if retrieved.AssetClass != models.AssetClassLargeCapStock {
			t.Errorf("Expected asset class large_cap_stock, got %q", retrieved.AssetClass)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency USD, got %q", retrieved.Currency)
		}
		if !retrieved.Exchange.Valid || retrieved.Exchange.String != "NASDAQ" {
			t.Errorf("Expected exchange NASDAQ, got %v", retrieved.Exchange)
		}
		if retrieved.Hidden {
			t.Error("Expected hidden to be false")
		}
	})

	t.Run("creates security without exchange", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("BTC", "Bitcoin", models.SecurityTypeOther)
		sec.AssetClass = models.AssetClassCrypto

		err := repo.Create(sec)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Exchange.Valid {
			t.Error("Expected exchange to be null")
		}
	})

	t.Run("rejects duplicate ticker and currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		if err := repo.Create(sec1); err != nil {
			t.Fatalf("Create first security error = %v", err)
		}

		sec2 := models.NewSecurity("AAPL", "Apple Different", models.SecurityTypeETF)
		err := repo.Create(sec2)
		if err == nil {
			t.Error("Create() expected error for duplicate ticker+currency")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})

	t.Run("allows same ticker with different currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("VOD", "Vodafone USD", models.SecurityTypeStock)
		sec1.Currency = "USD"
		if err := repo.Create(sec1); err != nil {
			t.Fatalf("Create first security error = %v", err)
		}

		sec2 := models.NewSecurity("VOD", "Vodafone GBP", models.SecurityTypeStock)
		sec2.Currency = "GBP"
		err := repo.Create(sec2)
		if err != nil {
			t.Fatalf("Create second security with different currency error = %v", err)
		}
	})
}

func TestSecurityRepository_GetByID(t *testing.T) {
	t.Run("returns security by ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("MSFT", "Microsoft Corp.", models.SecurityTypeStock)
		if err := repo.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != sec.ID {
			t.Errorf("Expected ID %v, got %v", sec.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		_, err := repo.GetByID(models.NewID())
		if err == nil {
			t.Error("GetByID() expected error for non-existent ID")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSecurityRepository_GetByTicker(t *testing.T) {
	t.Run("returns security by ticker", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("GOOG", "Alphabet Inc.", models.SecurityTypeStock)
		if err := repo.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByTicker("GOOG", "")
		if err != nil {
			t.Fatalf("GetByTicker() error = %v", err)
		}
		if retrieved.Ticker != "GOOG" {
			t.Errorf("Expected ticker GOOG, got %q", retrieved.Ticker)
		}
	})

	t.Run("returns security by ticker and currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("VOD", "Vodafone USD", models.SecurityTypeStock)
		sec1.Currency = "USD"
		if err := repo.Create(sec1); err != nil {
			t.Fatalf("Create first error = %v", err)
		}

		sec2 := models.NewSecurity("VOD", "Vodafone GBP", models.SecurityTypeStock)
		sec2.Currency = "GBP"
		if err := repo.Create(sec2); err != nil {
			t.Fatalf("Create second error = %v", err)
		}

		retrieved, err := repo.GetByTicker("VOD", "GBP")
		if err != nil {
			t.Fatalf("GetByTicker() error = %v", err)
		}
		if retrieved.Currency != "GBP" {
			t.Errorf("Expected currency GBP, got %q", retrieved.Currency)
		}
	})

	t.Run("returns NotFoundError for non-existent ticker", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		_, err := repo.GetByTicker("ZZZZ", "")
		if err == nil {
			t.Error("GetByTicker() expected error for non-existent ticker")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestSecurityRepository_List(t *testing.T) {
	t.Run("lists all securities", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("VXUS", "Vanguard Intl", models.SecurityTypeETF)
		sec3 := models.NewSecurity("BTC", "Bitcoin", models.SecurityTypeOther)
		sec3.Hidden = true

		for _, s := range []*models.Security{sec1, sec2, sec3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		results, err := repo.List(SecurityFilter{})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 securities, got %d", len(results))
		}
	})

	t.Run("lists excluding hidden", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("BTC", "Bitcoin", models.SecurityTypeOther)
		sec2.Hidden = true

		for _, s := range []*models.Security{sec1, sec2} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		excludeHidden := true
		results, err := repo.List(SecurityFilter{ExcludeHidden: &excludeHidden})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 security, got %d", len(results))
		}
		if results[0].Ticker != "AAPL" {
			t.Errorf("Expected AAPL, got %q", results[0].Ticker)
		}
	})

	t.Run("filters by security type", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("VXUS", "Vanguard Intl", models.SecurityTypeETF)
		sec3 := models.NewSecurity("MSFT", "Microsoft", models.SecurityTypeStock)

		for _, s := range []*models.Security{sec1, sec2, sec3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		st := models.SecurityTypeStock
		results, err := repo.List(SecurityFilter{SecurityType: &st})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 stocks, got %d", len(results))
		}
	})

	t.Run("filters by asset class", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec1.AssetClass = models.AssetClassLargeCapStock
		sec2 := models.NewSecurity("BTC", "Bitcoin", models.SecurityTypeOther)
		sec2.AssetClass = models.AssetClassCrypto

		for _, s := range []*models.Security{sec1, sec2} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		ac := models.AssetClassCrypto
		results, err := repo.List(SecurityFilter{AssetClass: &ac})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 crypto, got %d", len(results))
		}
		if results[0].Ticker != "BTC" {
			t.Errorf("Expected BTC, got %q", results[0].Ticker)
		}
	})

	t.Run("combines filters", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec1.AssetClass = models.AssetClassLargeCapStock
		sec2 := models.NewSecurity("MSFT", "Microsoft", models.SecurityTypeStock)
		sec2.AssetClass = models.AssetClassLargeCapStock
		sec2.Hidden = true
		sec3 := models.NewSecurity("VXUS", "Vanguard Intl", models.SecurityTypeETF)
		sec3.AssetClass = models.AssetClassInternationalStock

		for _, s := range []*models.Security{sec1, sec2, sec3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		st := models.SecurityTypeStock
		excludeHidden := true
		results, err := repo.List(SecurityFilter{SecurityType: &st, ExcludeHidden: &excludeHidden})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 visible stock, got %d", len(results))
		}
	})

	t.Run("returns empty slice for no matches", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		results, err := repo.List(SecurityFilter{})
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if results == nil {
			t.Error("Expected empty slice, got nil")
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 securities, got %d", len(results))
		}
	})
}

func TestSecurityRepository_Update(t *testing.T) {
	t.Run("updates all mutable fields", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		if err := repo.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalUpdatedAt := sec.UpdatedAt

		sec.Ticker = "AAPL2"
		sec.Name = "Apple Inc. Updated"
		sec.SecurityType = models.SecurityTypeETF
		sec.AssetClass = models.AssetClassIndex
		sec.Currency = "EUR"
		sec.SetExchange("NYSE")
		sec.Hidden = true

		err := repo.Update(sec)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "AAPL2" {
			t.Errorf("Expected ticker AAPL2, got %q", retrieved.Ticker)
		}
		if retrieved.Name != "Apple Inc. Updated" {
			t.Errorf("Expected updated name, got %q", retrieved.Name)
		}
		if retrieved.SecurityType != models.SecurityTypeETF {
			t.Errorf("Expected type ETF, got %q", retrieved.SecurityType)
		}
		if retrieved.AssetClass != models.AssetClassIndex {
			t.Errorf("Expected asset class index, got %q", retrieved.AssetClass)
		}
		if retrieved.Currency != "EUR" {
			t.Errorf("Expected currency EUR, got %q", retrieved.Currency)
		}
		if !retrieved.Exchange.Valid || retrieved.Exchange.String != "NYSE" {
			t.Errorf("Expected exchange NYSE, got %v", retrieved.Exchange)
		}
		if !retrieved.Hidden {
			t.Error("Expected hidden to be true")
		}
		if !retrieved.UpdatedAt.Time().After(originalUpdatedAt.Time()) {
			t.Error("Expected updated_at to be later than original")
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		err := repo.Update(sec)
		if err == nil {
			t.Error("Update() expected error for non-existent ID")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate ticker+currency on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("MSFT", "Microsoft", models.SecurityTypeStock)
		if err := repo.Create(sec1); err != nil {
			t.Fatalf("Create first error = %v", err)
		}
		if err := repo.Create(sec2); err != nil {
			t.Fatalf("Create second error = %v", err)
		}

		sec2.Ticker = "AAPL" // conflict with sec1
		err := repo.Update(sec2)
		if err == nil {
			t.Error("Update() expected error for duplicate ticker+currency")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestSecurityRepository_Delete(t *testing.T) {
	t.Run("deletes security with no dependents", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		if err := repo.Create(sec); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := repo.Delete(sec.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err = repo.GetByID(sec.ID)
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError after delete, got %T: %v", err, err)
		}
	})

	t.Run("rejects delete when security has prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)

		sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		if err := secRepo.Create(sec); err != nil {
			t.Fatalf("Create security error = %v", err)
		}

		// Insert a price directly
		_, err := database.Conn().Exec(`
			INSERT INTO security_prices (id, security_id, date, price, source, created_at)
			VALUES (gen_random_uuid(), CAST(? AS UUID), '2024-01-15', 185.50, 'manual', CURRENT_TIMESTAMP)
		`, sec.ID.String())
		if err != nil {
			t.Fatalf("Insert price error = %v", err)
		}

		err = secRepo.Delete(sec.ID)
		if err == nil {
			t.Error("Delete() expected error when security has prices")
		}
		if _, ok := err.(*HasDependentsError); !ok {
			t.Errorf("Expected HasDependentsError, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewSecurityRepository(database)

		err := repo.Delete(models.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent ID")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}
