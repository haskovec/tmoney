package price

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

// helper to create a security for price tests
func createTestSecurity(t *testing.T, repo *security.Repository) *security.Security {
	t.Helper()
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	return sec
}

func mustMoney(t *testing.T, s string) types.Money {
	t.Helper()
	m, err := types.NewMoney(s)
	if err != nil {
		t.Fatalf("invalid money %q: %v", s, err)
	}
	return m
}

// =============================================================================
// Repository.Create Tests (SM-021)
// =============================================================================

func TestRepository_Create(t *testing.T) {
	t.Run("creates valid price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		p := NewPrice(
			sec.ID,
			types.NewDate(2024, time.January, 15),
			mustMoney(t, "185.50"),
			SourceManual,
		)

		err := priceRepo.Create(p)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was persisted
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 15))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
		if retrieved.Source != SourceManual {
			t.Errorf("Expected source manual, got %q", retrieved.Source)
		}
	})

	t.Run("rejects duplicate security_id and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := types.NewDate(2024, time.January, 15)
		p1 := NewPrice(sec.ID, date, mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create first price error = %v", err)
		}

		p2 := NewPrice(sec.ID, date, mustMoney(t, "186.00"), SourceManual)
		err := priceRepo.Create(p2)
		if err == nil {
			t.Error("Create() expected error for duplicate security_id+date")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})

	t.Run("allows same date for different securities", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)

		sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
		sec2 := security.NewSecurity("MSFT", "Microsoft", security.TypeStock)
		if err := secRepo.Create(sec1); err != nil {
			t.Fatalf("Create sec1 error = %v", err)
		}
		if err := secRepo.Create(sec2); err != nil {
			t.Fatalf("Create sec2 error = %v", err)
		}

		date := types.NewDate(2024, time.January, 15)
		p1 := NewPrice(sec1.ID, date, mustMoney(t, "185.50"), SourceManual)
		p2 := NewPrice(sec2.ID, date, mustMoney(t, "370.00"), SourceManual)

		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create price1 error = %v", err)
		}
		if err := priceRepo.Create(p2); err != nil {
			t.Fatalf("Create price2 error = %v", err)
		}
	})
}

// =============================================================================
// Repository.CreateOrUpdate Tests (SM-022)
// =============================================================================

func TestRepository_CreateOrUpdate(t *testing.T) {
	t.Run("inserts new price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		p := NewPrice(
			sec.ID,
			types.NewDate(2024, time.January, 15),
			mustMoney(t, "185.50"),
			SourceManual,
		)

		err := priceRepo.CreateOrUpdate(p)
		if err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 15))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("updates existing price for same security and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := types.NewDate(2024, time.January, 15)
		p1 := NewPrice(sec.ID, date, mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p1); err != nil {
			t.Fatalf("Create first price error = %v", err)
		}

		p2 := NewPrice(sec.ID, date, mustMoney(t, "190.00"), SourceAPI)
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
		if retrieved.Source != SourceAPI {
			t.Errorf("Expected updated source api, got %q", retrieved.Source)
		}
	})
}

// =============================================================================
// Repository.GetBySecurityAndDate Tests (SM-023)
// =============================================================================

func TestRepository_GetBySecurityAndDate(t *testing.T) {
	t.Run("returns exact match", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		date := types.NewDate(2024, time.January, 15)
		p := NewPrice(sec.ID, date, mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p); err != nil {
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
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		_, err := priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 15))
		if err == nil {
			t.Error("GetBySecurityAndDate() expected error for no match")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Repository.GetCurrentPrice Tests (SM-024)
// =============================================================================

func TestRepository_GetCurrentPrice(t *testing.T) {
	t.Run("returns most recent price on or before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
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
			p := NewPrice(sec.ID, types.NewDate(2024, time.January, d.day), mustMoney(t, d.price), SourceManual)
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		// Query for Jan 17 should return Jan 15 price
		retrieved, err := priceRepo.GetCurrentPrice(sec.ID, types.NewDate(2024, time.January, 17))
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.Float64() != 185.50 {
			t.Errorf("Expected price 185.50, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns NotFoundError when no price before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		p := NewPrice(sec.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		_, err := priceRepo.GetCurrentPrice(sec.ID, types.NewDate(2024, time.January, 10))
		if err == nil {
			t.Error("GetCurrentPrice() expected error when no price before date")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Repository.GetPriceHistory Tests (SM-025)
// =============================================================================

func TestRepository_GetPriceHistory(t *testing.T) {
	t.Run("returns all prices ordered by date desc", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		dates := []int{10, 15, 20}
		for _, day := range dates {
			p := NewPrice(sec.ID, types.NewDate(2024, time.January, day), mustMoney(t, "185.50"), SourceManual)
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
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		for _, day := range []int{10, 15, 20, 25} {
			p := NewPrice(sec.ID, types.NewDate(2024, time.January, day), mustMoney(t, "185.50"), SourceManual)
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		from := types.NewDate(2024, time.January, 12)
		to := types.NewDate(2024, time.January, 22)
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
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
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
// Repository.Delete Tests (SM-026)
// =============================================================================

func TestRepository_Delete(t *testing.T) {
	t.Run("deletes existing price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		p := NewPrice(sec.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := priceRepo.Delete(p.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify it's gone
		_, err = priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 15))
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError after delete, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent price", func(t *testing.T) {
		database := createTestDB(t)
		priceRepo := NewRepository(database)

		err := priceRepo.Delete(types.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent ID")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Repository.BulkCreate Tests (SM-027)
// =============================================================================

func TestRepository_BulkCreate(t *testing.T) {
	t.Run("inserts multiple prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.January, 12), mustMoney(t, "182.00"), SourceImport),
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
	})

	t.Run("skips duplicates when overwrite is false", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		// Create an existing price
		existing := NewPrice(sec.ID, types.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), SourceManual)
		if err := priceRepo.Create(existing); err != nil {
			t.Fatalf("Create existing error = %v", err)
		}

		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.January, 10), mustMoney(t, "999.00"), SourceImport), // duplicate
			NewPrice(sec.ID, types.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), SourceImport), // new
		}

		result, err := priceRepo.BulkCreate(prices, false)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Imported != 1 {
			t.Errorf("Expected imported 1, got %d", result.Imported)
		}
		if result.Skipped != 1 {
			t.Errorf("Expected skipped 1, got %d", result.Skipped)
		}

		// Verify original price was not overwritten
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 10))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 180.00 {
			t.Errorf("Expected original price 180.00 preserved, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("overwrites duplicates when overwrite is true", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		existing := NewPrice(sec.ID, types.NewDate(2024, time.January, 10), mustMoney(t, "180.00"), SourceManual)
		if err := priceRepo.Create(existing); err != nil {
			t.Fatalf("Create existing error = %v", err)
		}

		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.January, 10), mustMoney(t, "999.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.January, 11), mustMoney(t, "181.00"), SourceImport),
		}

		result, err := priceRepo.BulkCreate(prices, true)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Imported != 2 {
			t.Errorf("Expected imported 2, got %d", result.Imported)
		}

		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 10))
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.Float64() != 999.00 {
			t.Errorf("Expected overwritten price 999.00, got %v", retrieved.Price.Float64())
		}
	})

	t.Run("returns zero counts for empty input", func(t *testing.T) {
		database := createTestDB(t)
		priceRepo := NewRepository(database)

		result, err := priceRepo.BulkCreate(nil, false)
		if err != nil {
			t.Fatalf("BulkCreate() error = %v", err)
		}
		if result.Total != 0 {
			t.Errorf("Expected total 0, got %d", result.Total)
		}
	})
}

// =============================================================================
// Repository.GetLatestPrices Tests
// =============================================================================

func TestRepository_GetLatestPrices(t *testing.T) {
	t.Run("returns empty slice when no securities exist", func(t *testing.T) {
		database := createTestDB(t)
		priceRepo := NewRepository(database)

		got, err := priceRepo.GetLatestPrices()
		if err != nil {
			t.Fatalf("GetLatestPrices() error = %v", err)
		}
		if got == nil {
			t.Fatal("expected empty slice, got nil")
		}
		if len(got) != 0 {
			t.Errorf("expected 0 rows, got %d", len(got))
		}
	})

	t.Run("excludes securities that have no prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)

		// AAPL has prices, MSFT does not.
		aapl := createTestSecurity(t, secRepo)
		msft := security.NewSecurity("MSFT", "Microsoft", security.TypeStock)
		if err := secRepo.Create(msft); err != nil {
			t.Fatalf("create MSFT: %v", err)
		}
		p := NewPrice(aapl.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "185.50"), SourceManual)
		if err := priceRepo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		got, err := priceRepo.GetLatestPrices()
		if err != nil {
			t.Fatalf("GetLatestPrices() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 row, got %d", len(got))
		}
		if got[0].Ticker != "AAPL" {
			t.Errorf("ticker = %q, want AAPL", got[0].Ticker)
		}
	})

	t.Run("excludes hidden securities", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)

		visible := createTestSecurity(t, secRepo)
		hidden := security.NewSecurity("HIDE", "Hidden Inc", security.TypeStock)
		hidden.Hide()
		if err := secRepo.Create(hidden); err != nil {
			t.Fatalf("create hidden: %v", err)
		}
		if err := priceRepo.Create(NewPrice(visible.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "100.00"), SourceManual)); err != nil {
			t.Fatalf("create visible price: %v", err)
		}
		if err := priceRepo.Create(NewPrice(hidden.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "50.00"), SourceManual)); err != nil {
			t.Fatalf("create hidden price: %v", err)
		}

		got, err := priceRepo.GetLatestPrices()
		if err != nil {
			t.Fatalf("GetLatestPrices() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 row, got %d", len(got))
		}
		if got[0].Ticker == "HIDE" {
			t.Error("hidden security should be excluded")
		}
	})

	t.Run("returns latest date when a security has multiple prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		sec := createTestSecurity(t, secRepo)

		for _, day := range []int{10, 15, 20} {
			p := NewPrice(sec.ID, types.NewDate(2024, time.January, day), mustMoney(t, "100.00"), SourceManual)
			// Differentiate the price so we can verify the right row was picked.
			p.Price = mustMoney(t, "10"+itoaPad(day))
			if err := priceRepo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		got, err := priceRepo.GetLatestPrices()
		if err != nil {
			t.Fatalf("GetLatestPrices() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 row, got %d", len(got))
		}
		if got[0].Date.Time().Day() != 20 {
			t.Errorf("date day = %d, want 20", got[0].Date.Time().Day())
		}
		if got[0].Price.String() != "1020" {
			t.Errorf("price = %q, want 1020", got[0].Price.String())
		}
	})

	t.Run("sorts rows by ticker", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)

		zeb := security.NewSecurity("ZEB", "Zebra", security.TypeStock)
		if err := secRepo.Create(zeb); err != nil {
			t.Fatalf("create zeb: %v", err)
		}
		ant := security.NewSecurity("ANT", "Anteater", security.TypeStock)
		if err := secRepo.Create(ant); err != nil {
			t.Fatalf("create ant: %v", err)
		}
		mid := security.NewSecurity("MID", "Middle", security.TypeStock)
		if err := secRepo.Create(mid); err != nil {
			t.Fatalf("create mid: %v", err)
		}
		for _, sec := range []*security.Security{zeb, ant, mid} {
			if err := priceRepo.Create(NewPrice(sec.ID, types.NewDate(2024, time.January, 15), mustMoney(t, "100.00"), SourceManual)); err != nil {
				t.Fatalf("create price: %v", err)
			}
		}

		got, err := priceRepo.GetLatestPrices()
		if err != nil {
			t.Fatalf("GetLatestPrices() error = %v", err)
		}
		want := []string{"ANT", "MID", "ZEB"}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			if got[i].Ticker != w {
				t.Errorf("got[%d].Ticker = %q, want %q", i, got[i].Ticker, w)
			}
		}
	})
}

func itoaPad(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
