package price

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Helper: create a security in the DB for price tests
// =============================================================================

func createTestSecurityForService(t *testing.T, secRepo *security.Repository) *security.Security {
	t.Helper()
	svc := security.NewService(secRepo, nil)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := svc.Create(sec); err != nil {
		t.Fatalf("Failed to create test security: %v", err)
	}
	return sec
}

// =============================================================================
// SM-030: Service.AddPrice
// =============================================================================

func TestService_AddPrice(t *testing.T) {
	t.Run("adds price for valid security and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)

		err := svc.AddPrice(p)
		if err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		retrieved, err := svc.GetCurrentPrice(sec.ID, date)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.String() != "150" {
			t.Errorf("Expected price '150', got %q", retrieved.Price.String())
		}
		if retrieved.Source != SourceManual {
			t.Errorf("Expected source 'manual', got %q", retrieved.Source)
		}
	})

	t.Run("rejects future date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		futureDate := types.NewDate(2099, time.December, 31)
		p := NewPrice(sec.ID, futureDate, types.MustNewMoney("150.00"), SourceManual)

		err := svc.AddPrice(p)
		if err == nil {
			t.Error("AddPrice() expected error for future date")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects non-positive price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("0"), SourceManual)

		err := svc.AddPrice(p)
		if err == nil {
			t.Error("AddPrice() expected error for zero price")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects duplicate security+date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)

		price1 := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(price1); err != nil {
			t.Fatalf("AddPrice() first error = %v", err)
		}

		price2 := NewPrice(sec.ID, date, types.MustNewMoney("155.00"), SourceManual)
		err := svc.AddPrice(price2)
		if err == nil {
			t.Error("AddPrice() expected error for duplicate")
		}
		if _, ok := err.(*AlreadyExistsError); !ok {
			t.Errorf("Expected AlreadyExistsError, got %T: %v", err, err)
		}
	})

	t.Run("rejects price for non-existent security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(types.NewID(), date, types.MustNewMoney("150.00"), SourceManual)

		err := svc.AddPrice(p)
		if err == nil {
			t.Error("AddPrice() expected error for non-existent security")
		}
	})

	t.Run("rejects price for hidden security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Hide the security
		secSvc := security.NewService(secRepo, database)
		if err := secSvc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)

		err := svc.AddPrice(p)
		if err == nil {
			t.Error("AddPrice() expected error for hidden security")
		}
		if _, ok := err.(*HiddenSecurityError); !ok {
			t.Errorf("Expected HiddenSecurityError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-177: UpdatePrice rejects hidden security
// =============================================================================

func TestService_UpdatePrice_HiddenSecurity(t *testing.T) {
	t.Run("rejects update for hidden security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Add a price while visible
		date := types.NewDate(2024, time.March, 15)
		price1 := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(price1); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Now hide the security
		secSvc := security.NewService(secRepo, database)
		if err := secSvc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		// Updating price should fail
		price2 := NewPrice(sec.ID, date, types.MustNewMoney("155.00"), SourceManual)
		err := svc.UpdatePrice(price2)
		if err == nil {
			t.Error("UpdatePrice() expected error for hidden security")
		}
		if _, ok := err.(*HiddenSecurityError); !ok {
			t.Errorf("Expected HiddenSecurityError, got %T: %v", err, err)
		}
	})

	t.Run("existing price history preserved for hidden security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Add prices while visible
		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(p); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Hide the security
		secSvc := security.NewService(secRepo, database)
		if err := secSvc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		// Price history should still be readable
		retrieved, err := svc.GetCurrentPrice(sec.ID, date)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.String() != "150" {
			t.Errorf("Expected price '150' preserved, got %q", retrieved.Price.String())
		}
	})
}

// =============================================================================
// SM-031: Service.UpdatePrice
// =============================================================================

func TestService_UpdatePrice(t *testing.T) {
	t.Run("updates existing price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)

		price1 := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(price1); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Update with new price for same security+date
		price2 := NewPrice(sec.ID, date, types.MustNewMoney("155.00"), SourceManual)
		if err := svc.UpdatePrice(price2); err != nil {
			t.Fatalf("UpdatePrice() error = %v", err)
		}

		retrieved, err := svc.GetCurrentPrice(sec.ID, date)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.String() != "155" {
			t.Errorf("Expected price '155', got %q", retrieved.Price.String())
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)

		p := NewPrice(sec.ID, date, types.MustNewMoney("0"), SourceManual)
		err := svc.UpdatePrice(p)
		if err == nil {
			t.Error("UpdatePrice() expected error for zero price")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})
}

// =============================================================================
// SM-032: Service.GetCurrentPrice
// =============================================================================

func TestService_GetCurrentPrice(t *testing.T) {
	t.Run("returns most recent price on or before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Add prices on different dates
		for _, entry := range []struct {
			date  types.Date
			price string
		}{
			{types.NewDate(2024, time.March, 10), "145.00"},
			{types.NewDate(2024, time.March, 15), "150.00"},
			{types.NewDate(2024, time.March, 20), "155.00"},
		} {
			p := NewPrice(sec.ID, entry.date, types.MustNewMoney(entry.price), SourceManual)
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		// Query as of March 17 -- should get March 15 price
		asOf := types.NewDate(2024, time.March, 17)
		retrieved, err := svc.GetCurrentPrice(sec.ID, asOf)
		if err != nil {
			t.Fatalf("GetCurrentPrice() error = %v", err)
		}
		if retrieved.Price.String() != "150" {
			t.Errorf("Expected price '150', got %q", retrieved.Price.String())
		}
	})

	t.Run("returns error when no price exists", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		_, err := svc.GetCurrentPrice(types.NewID(), date)
		if err == nil {
			t.Error("GetCurrentPrice() expected error when no price exists")
		}
	})
}

// =============================================================================
// SM-033: Service.GetPriceHistory
// =============================================================================

func TestService_GetPriceHistory(t *testing.T) {
	t.Run("returns prices in date range", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		dates := []types.Date{
			types.NewDate(2024, time.January, 15),
			types.NewDate(2024, time.February, 15),
			types.NewDate(2024, time.March, 15),
			types.NewDate(2024, time.April, 15),
		}
		for i, d := range dates {
			p := NewPrice(sec.ID, d, types.MustNewMoney("100.00"), SourceManual)
			p.Price = types.NewMoneyFromInt(int64(100 + i*10))
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		from := types.NewDate(2024, time.February, 1)
		to := types.NewDate(2024, time.March, 31)
		prices, err := svc.GetPriceHistory(sec.ID, &from, &to)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if len(prices) != 2 {
			t.Fatalf("Expected 2 prices, got %d", len(prices))
		}
	})

	t.Run("nil range returns all prices", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		for _, d := range []types.Date{
			types.NewDate(2024, time.January, 15),
			types.NewDate(2024, time.February, 15),
			types.NewDate(2024, time.March, 15),
		} {
			p := NewPrice(sec.ID, d, types.MustNewMoney("100.00"), SourceManual)
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		prices, err := svc.GetPriceHistory(sec.ID, nil, nil)
		if err != nil {
			t.Fatalf("GetPriceHistory() error = %v", err)
		}
		if len(prices) != 3 {
			t.Fatalf("Expected 3 prices, got %d", len(prices))
		}
	})
}

func TestService_GetLatestPrices(t *testing.T) {
	database := createTestDB(t)
	secRepo := security.NewRepository(database)
	priceRepo := NewRepository(database)
	svc := NewService(priceRepo, secRepo, database)

	sec := createTestSecurityForService(t, secRepo)
	for _, d := range []types.Date{
		types.NewDate(2024, time.January, 10),
		types.NewDate(2024, time.January, 20),
	} {
		p := NewPrice(sec.ID, d, types.MustNewMoney("100.00"), SourceManual)
		if err := svc.AddPrice(p); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}
	}

	got, err := svc.GetLatestPrices()
	if err != nil {
		t.Fatalf("GetLatestPrices() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].SecurityID != sec.ID {
		t.Errorf("SecurityID mismatch")
	}
	if got[0].Date.Time().Day() != 20 {
		t.Errorf("date day = %d, want 20", got[0].Date.Time().Day())
	}
}

// =============================================================================
// SM-034: Service.DeletePrice
// =============================================================================

func TestService_DeletePrice(t *testing.T) {
	t.Run("deletes price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)

		if err := svc.AddPrice(p); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		if err := svc.DeletePrice(p.ID); err != nil {
			t.Fatalf("DeletePrice() error = %v", err)
		}

		// Verify it's gone
		_, err := svc.GetCurrentPrice(sec.ID, date)
		if err == nil {
			t.Error("GetCurrentPrice() expected error after delete")
		}
	})

	t.Run("returns error for non-existent price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		err := svc.DeletePrice(types.NewID())
		if err == nil {
			t.Error("DeletePrice() expected error for non-existent price")
		}
	})

	t.Run("delete does not cascade to security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)
		secSvc := security.NewService(secRepo, database)

		date := types.NewDate(2024, time.March, 15)
		p := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)

		if err := svc.AddPrice(p); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}
		if err := svc.DeletePrice(p.ID); err != nil {
			t.Fatalf("DeletePrice() error = %v", err)
		}

		// Security should still exist
		retrieved, err := secSvc.GetByID(sec.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", retrieved.Ticker)
		}
	})
}

// =============================================================================
// SM-116: Service.BulkImport
// =============================================================================

func TestService_BulkImport(t *testing.T) {
	t.Run("imports valid prices with source=import", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.March, 10), types.MustNewMoney("145.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 11), types.MustNewMoney("147.50"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 12), types.MustNewMoney("150.00"), SourceImport),
		}

		result, err := svc.BulkImport(prices, false)
		if err != nil {
			t.Fatalf("BulkImport() error = %v", err)
		}
		if result.Total != 3 {
			t.Errorf("Expected Total=3, got %d", result.Total)
		}
		if result.Imported != 3 {
			t.Errorf("Expected Imported=3, got %d", result.Imported)
		}
		if result.Skipped != 0 {
			t.Errorf("Expected Skipped=0, got %d", result.Skipped)
		}

		// Verify prices were stored with source=import
		for _, p := range prices {
			retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, p.Date)
			if err != nil {
				t.Fatalf("GetBySecurityAndDate() error = %v", err)
			}
			if retrieved.Source != SourceImport {
				t.Errorf("Expected source 'import', got %q", retrieved.Source)
			}
		}
	})

	t.Run("skips existing prices by default", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)

		// Add an existing price
		existing := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(existing); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Bulk import with a duplicate and a new one
		prices := []*Price{
			NewPrice(sec.ID, date, types.MustNewMoney("155.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 16), types.MustNewMoney("160.00"), SourceImport),
		}

		result, err := svc.BulkImport(prices, false)
		if err != nil {
			t.Fatalf("BulkImport() error = %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Expected Total=2, got %d", result.Total)
		}
		if result.Imported != 1 {
			t.Errorf("Expected Imported=1, got %d", result.Imported)
		}
		if result.Skipped != 1 {
			t.Errorf("Expected Skipped=1, got %d", result.Skipped)
		}

		// Verify original price was not overwritten
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.String() != "150" {
			t.Errorf("Expected original price '150' preserved, got %q", retrieved.Price.String())
		}
		if retrieved.Source != SourceManual {
			t.Errorf("Expected source 'manual' preserved, got %q", retrieved.Source)
		}
	})

	t.Run("overwrites existing when flag set", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		date := types.NewDate(2024, time.March, 15)

		// Add an existing price
		existing := NewPrice(sec.ID, date, types.MustNewMoney("150.00"), SourceManual)
		if err := svc.AddPrice(existing); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Bulk import with overwrite
		prices := []*Price{
			NewPrice(sec.ID, date, types.MustNewMoney("155.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 16), types.MustNewMoney("160.00"), SourceImport),
		}

		result, err := svc.BulkImport(prices, true)
		if err != nil {
			t.Fatalf("BulkImport() error = %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Expected Total=2, got %d", result.Total)
		}
		if result.Imported != 2 {
			t.Errorf("Expected Imported=2, got %d", result.Imported)
		}
		if result.Skipped != 0 {
			t.Errorf("Expected Skipped=0, got %d", result.Skipped)
		}

		// Verify price was overwritten
		retrieved, err := priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if retrieved.Price.String() != "155" {
			t.Errorf("Expected overwritten price '155', got %q", retrieved.Price.String())
		}
		if retrieved.Source != SourceImport {
			t.Errorf("Expected source 'import', got %q", retrieved.Source)
		}
	})

	t.Run("returns summary counts", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Pre-populate some prices
		for _, d := range []types.Date{
			types.NewDate(2024, time.March, 10),
			types.NewDate(2024, time.March, 11),
		} {
			p := NewPrice(sec.ID, d, types.MustNewMoney("100.00"), SourceManual)
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		// Import 5 prices: 2 duplicates + 3 new
		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.March, 10), types.MustNewMoney("101.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 11), types.MustNewMoney("102.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 12), types.MustNewMoney("103.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 13), types.MustNewMoney("104.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 14), types.MustNewMoney("105.00"), SourceImport),
		}

		result, err := svc.BulkImport(prices, false)
		if err != nil {
			t.Fatalf("BulkImport() error = %v", err)
		}
		if result.Total != 5 {
			t.Errorf("Expected Total=5, got %d", result.Total)
		}
		if result.Imported != 3 {
			t.Errorf("Expected Imported=3, got %d", result.Imported)
		}
		if result.Skipped != 2 {
			t.Errorf("Expected Skipped=2, got %d", result.Skipped)
		}
	})

	t.Run("rejects batch with hidden security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Hide the security
		secSvc := security.NewService(secRepo, database)
		if err := secSvc.Hide(sec.ID); err != nil {
			t.Fatalf("Hide() error = %v", err)
		}

		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.March, 10), types.MustNewMoney("145.00"), SourceImport),
		}

		_, err := svc.BulkImport(prices, false)
		if err == nil {
			t.Error("BulkImport() expected error for hidden security")
		}
		if _, ok := err.(*HiddenSecurityError); !ok {
			t.Errorf("Expected HiddenSecurityError, got %T: %v", err, err)
		}
	})

	t.Run("rejects invalid prices in batch", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		sec := createTestSecurityForService(t, secRepo)
		priceRepo := NewRepository(database)
		svc := NewService(priceRepo, secRepo, database)

		// Include an invalid price (zero amount) in the batch
		prices := []*Price{
			NewPrice(sec.ID, types.NewDate(2024, time.March, 10), types.MustNewMoney("145.00"), SourceImport),
			NewPrice(sec.ID, types.NewDate(2024, time.March, 11), types.MustNewMoney("0"), SourceImport),
		}

		_, err := svc.BulkImport(prices, false)
		if err == nil {
			t.Error("BulkImport() expected error for invalid price")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})
}
