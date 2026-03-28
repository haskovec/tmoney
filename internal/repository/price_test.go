package repository

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

// helper to create a security for price tests
func createTestSecurity(t *testing.T, repo *SecurityRepository) *models.Security {
	t.Helper()
	sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

func mustMoney(t *testing.T, s string) models.Money {
	t.Helper()
	m, err := models.NewMoney(s)
	if err != nil {
		t.Fatalf("invalid money %q: %v", s, err)
	}
	return m
}

// =============================================================================
// PriceRepository.Create Tests (SM-021)
// =============================================================================

func TestPriceRepository_Create(t *testing.T) {
	t.Run("creates valid price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		price := models.NewSecurityPrice(
			sec.ID,
			models.NewDate(2024, time.January, 15),
			mustMoney(t, "185.50"),
			models.PriceSourceManual,
		)

		err := priceRepo.Create(price)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was persisted
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 15))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
		if retrieved.Source != models.PriceSourceManual {
			t.Errorf("Expected source manual, got %q", retrieved.Source)
		}
	})

	t.Run("rejects duplicate security_id and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := models.NewDate(2024, time.January, 15)
		p1 := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create first price error = %v", err)
		}

		p2 := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "186.00"), models.PriceSourceManual)
		err := priceRepo.Create(p2)
		if err == nil {
			t.Error("Create() expected error for duplicate security_id+date")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})

	t.Run("allows same date for different securities", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("MSFT", "Microsoft", models.SecurityTypeStock)
		if err := secRepo.Create(sec1); err != nil {
			t.Fatalf("Create sec1 error = %v", err)
		}
		if err := secRepo.Create(sec2); err != nil {
			t.Fatalf("Create sec2 error = %v", err)
		}

		date := models.NewDate(2024, time.January, 15)
		p1 := models.NewSecurityPrice(sec1.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		p2 := models.NewSecurityPrice(sec2.ID, date, mustMoney(t, "370.00"), models.PriceSourceManual)

		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create price1 error = %v", err)
		}
		if err := priceRepo.Create(p2); err != nil {
			t.Fatalf("Create price2 error = %v", err)
		}
	})
}

// =============================================================================
// PriceRepository.CreateOrUpdate Tests (SM-022)
// =============================================================================

func TestPriceRepository_CreateOrUpdate(t *testing.T) {
	t.Run("inserts new price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		price := models.NewSecurityPrice(
			sec.ID,
			models.NewDate(2024, time.January, 15),
			mustMoney(t, "185.50"),
			models.PriceSourceManual,
		)

		err := priceRepo.CreateOrUpdate(price)
		if err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 15))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("updates existing price for same security and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := models.NewDate(2024, time.January, 15)
		p1 := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create first price error = %v", err)
		}

		p2 := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "190.00"), models.PriceSourceAPI)
		err := priceRepo.CreateOrUpdate(p2)
		if err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 190.00 {
			t.Errorf("Expected updated price 190.00, got %v", retrieved.Price.Float64())
		}
		if retrieved.Source != models.PriceSourceAPI {
			t.Errorf("Expected updated source api, got %q", retrieved.Source)
		}
	})
}

// =============================================================================
// PriceRepository.GetBySecurityAndDate Tests (SM-023)
// =============================================================================

func TestPriceRepository_GetBySecurityAndDate(t *testing.T) {
	t.Run("returns exact match", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := models.NewDate(2024, time.January, 15)
		price := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(price); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns NotFoundError when no match", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		_, err := priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 15))
		if err == nil {
			t.Error("GetBySecurityAndDate() expected error for no match")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// PriceRepository.GetCurrentPrice Tests (SM-024)
// =============================================================================

func TestPriceRepository_GetCurrentPrice(t *testing.T) {
	t.Run("returns most recent price on or before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		// Insert prices on Jan 10, Jan 15, Jan 20
		for _, d := range []struct {
			day   int
			price string
		}{
			{10, "180.00"},
			{15, "185.50"},
			{20, "190.00"},
		} {
			p := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, d.day), mustMoney(t, d.price), models.PriceSourceManual)
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		// Query for Jan 17 should return Jan 15 price
		retrieved, err := priceRepo.GetCurrentPrice(sec.ID, models.NewDate(2024, time.January, 17))
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns exact date match", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := models.NewDate(2024, time.January, 15)
		p := models.NewSecurityPrice(sec.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := priceRepo.GetCurrentPrice(sec.ID, date)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns NotFoundError when no price before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		p := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 15), mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		_, err := priceRepo.GetCurrentPrice(sec.ID, models.NewDate(2024, time.January, 10))
		if err == nil {
			t.Error("GetCurrentPrice() expected error when no price before date")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("respects security_id filter", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)

		sec1 := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
		sec2 := models.NewSecurity("MSFT", "Microsoft", models.SecurityTypeStock)
		if err := secRepo.Create(sec1); err != nil {
			t.Fatalf("Create sec1 error = %v", err)
		}
		if err := secRepo.Create(sec2); err != nil {
			t.Fatalf("Create sec2 error = %v", err)
		}

		date := models.NewDate(2024, time.January, 15)
		p1 := models.NewSecurityPrice(sec1.ID, date, mustMoney(t, "185.50"), models.PriceSourceManual)
		p2 := models.NewSecurityPrice(sec2.ID, date, mustMoney(t, "370.00"), models.PriceSourceManual)
		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create p1 error = %v", err)
		}
		if err := priceRepo.Create(p2); err != nil {
			t.Fatalf("Create p2 error = %v", err)
		}

		retrieved, err := priceRepo.GetCurrentPrice(sec1.ID, date)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected AAPL price 185.50, got %v", retrieved.Price.Float64())
		}
	})
}

// =============================================================================
// PriceRepository.GetPriceHistory Tests (SM-025)
// =============================================================================

func TestPriceRepository_GetPriceHistory(t *testing.T) {
	t.Run("returns all prices ordered by date desc", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		dates := []int{10, 15, 20}
		for _, day := range dates {
			p := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, day), mustMoney(t, "185.50"), models.PriceSourceManual)
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		prices, err := priceRepo.GetPriceHistory(sec.ID, nil, nil)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if len(prices) != 3 {
			t.Fatalf("Expected 3 prices, got %d", len(prices))
		}
		// Should be descending by date
		if prices[0].Date.Time().Day() != 20 {
			t.Errorf("Expected first price on day 20, got day %d", prices[0].Date.Time().Day())
		}
		if prices[2].Date.Time().Day() != 10 {
			t.Errorf("Expected last price on day 10, got day %d", prices[2].Date.Time().Day())
		}
	})

	t.Run("filters by date range", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		for _, day := range []int{10, 15, 20, 25} {
			p := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, day), mustMoney(t, "185.50"), models.PriceSourceManual)
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		from := models.NewDate(2024, time.January, 12)
		to := models.NewDate(2024, time.January, 22)
		prices, err := priceRepo.GetPriceHistory(sec.ID, &from, &to)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if len(prices) != 2 {
			t.Errorf("Expected 2 prices in range, got %d", len(prices))
		}
	})

	t.Run("returns empty slice for no results", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		prices, err := priceRepo.GetPriceHistory(sec.ID, nil, nil)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if prices == nil {
			t.Error("Expected empty slice, got nil")
		}
		if len(prices) != 0 {
			t.Errorf("Expected 0 prices, got %d", len(prices))
		}
	})
}

// =============================================================================
// PriceRepository.Delete Tests (SM-026)
// =============================================================================

func TestPriceRepository_Delete(t *testing.T) {
	t.Run("deletes existing price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		price := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 15), mustMoney(t, "185.50"), models.PriceSourceManual)
		if err := priceRepo.Create(price); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := priceRepo.Delete(price.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify it's gone
		_, err = priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 15))
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError after delete, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent price", func(t *testing.T) {
		database := createTestDB(t)
		priceRepo := NewPriceRepository(database)

		err := priceRepo.Delete(models.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent ID")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// PriceRepository.BulkCreate Tests (SM-027)
// =============================================================================

func TestPriceRepository_BulkCreate(t *testing.T) {
	t.Run("inserts multiple prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		prices := []*models.SecurityPrice{
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), models.PriceSourceImport),
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), models.PriceSourceImport),
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 12), mustMoney(t, "182.00"), models.PriceSourceImport),
		}

		result, err := priceRepo.BulkCreate(prices, false)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Total != 3 {
			t.Errorf("Expected total 3, got %d", result.Total)
		}
		if result.Imported != 3 {
			t.Errorf("Expected imported 3, got %d", result.Imported)
		}
		if result.Skipped != 0 {
			t.Errorf("Expected skipped 0, got %d", result.Skipped)
		}

		// Verify all were persisted
		history, err := priceRepo.GetPriceHistory(sec.ID, nil, nil)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if len(history) != 3 {
			t.Errorf("Expected 3 prices in history, got %d", len(history))
		}
	})

	t.Run("skips duplicates when overwrite is false", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		// Create an existing price
		existing := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), models.PriceSourceManual)
		if err := priceRepo.Create(existing); err != nil {
			t.Fatalf("Create existing error = %v", err)
		}

		prices := []*models.SecurityPrice{
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 10), mustMoney(t, "999.00"), models.PriceSourceImport), // duplicate
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), models.PriceSourceImport), // new
		}

		result, err := priceRepo.BulkCreate(prices, false)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Expected total 2, got %d", result.Total)
		}
		if result.Imported != 1 {
			t.Errorf("Expected imported 1, got %d", result.Imported)
		}
		if result.Skipped != 1 {
			t.Errorf("Expected skipped 1, got %d", result.Skipped)
		}

		// Verify original price was not overwritten
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 10))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 180.00 {
			t.Errorf("Expected original price 180.00 preserved, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("overwrites duplicates when overwrite is true", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := NewSecurityRepository(database)
		priceRepo := NewPriceRepository(database)
		sec := createTestSecurity(t, secRepo)

		// Create an existing price
		existing := models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), models.PriceSourceManual)
		if err := priceRepo.Create(existing); err != nil {
			t.Fatalf("Create existing error = %v", err)
		}

		prices := []*models.SecurityPrice{
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 10), mustMoney(t, "999.00"), models.PriceSourceImport), // overwrite
			models.NewSecurityPrice(sec.ID, models.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), models.PriceSourceImport), // new
		}

		result, err := priceRepo.BulkCreate(prices, true)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Expected total 2, got %d", result.Total)
		}
		if result.Imported != 2 {
			t.Errorf("Expected imported 2, got %d", result.Imported)
		}
		if result.Skipped != 0 {
			t.Errorf("Expected skipped 0, got %d", result.Skipped)
		}

		// Verify price was overwritten
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, models.NewDate(2024, time.January, 10))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 999.00 {
			t.Errorf("Expected overwritten price 999.00, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns zero counts for empty input", func(t *testing.T) {
		database := createTestDB(t)
		priceRepo := NewPriceRepository(database)

		result, err := priceRepo.BulkCreate(nil, false)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Total != 0 {
			t.Errorf("Expected total 0, got %d", result.Total)
		}
	})
}
