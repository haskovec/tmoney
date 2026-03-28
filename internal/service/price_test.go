package service

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// =============================================================================
// Helper: create a security in the DB for price tests
// =============================================================================

func createTestSecurity(t *testing.T, svc *SecurityService) *models.Security {
	t.Helper()
	sec := models.NewSecurity("AAPL", "Apple Inc.", models.SecurityTypeStock)
	if err := svc.Create(sec); err != nil {
		t.Fatalf("Failed to create test security: %v", err)
	}
	return sec
}

// =============================================================================
// SM-030: PriceService.AddPrice
// =============================================================================

func TestPriceService_AddPrice(t *testing.T) {
	t.Run("adds price for valid security and date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)
		price := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("150.00"), models.PriceSourceManual)

		err := svc.AddPrice(price)
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
		if retrieved.Source != models.PriceSourceManual {
			t.Errorf("Expected source 'manual', got %q", retrieved.Source)
		}
	})

	t.Run("rejects future date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		futureDate := models.NewDate(2099, time.December, 31)
		price := models.NewSecurityPrice(sec.ID, futureDate, models.MustNewMoney("150.00"), models.PriceSourceManual)

		err := svc.AddPrice(price)
		if err == nil {
			t.Error("AddPrice() expected error for future date")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects non-positive price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)
		price := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("0"), models.PriceSourceManual)

		err := svc.AddPrice(price)
		if err == nil {
			t.Error("AddPrice() expected error for zero price")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects duplicate security+date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)

		price1 := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("150.00"), models.PriceSourceManual)
		if err := svc.AddPrice(price1); err != nil {
			t.Fatalf("AddPrice() first error = %v", err)
		}

		price2 := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("155.00"), models.PriceSourceManual)
		err := svc.AddPrice(price2)
		if err == nil {
			t.Error("AddPrice() expected error for duplicate")
		}
		if _, ok := err.(*PriceAlreadyExistsError); !ok {
			t.Errorf("Expected PriceAlreadyExistsError, got %T: %v", err, err)
		}
	})

	t.Run("rejects price for non-existent security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		date := models.NewDate(2024, time.March, 15)
		price := models.NewSecurityPrice(models.NewID(), date, models.MustNewMoney("150.00"), models.PriceSourceManual)

		err := svc.AddPrice(price)
		if err == nil {
			t.Error("AddPrice() expected error for non-existent security")
		}
	})
}

// =============================================================================
// SM-031: PriceService.UpdatePrice
// =============================================================================

func TestPriceService_UpdatePrice(t *testing.T) {
	t.Run("updates existing price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)

		price1 := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("150.00"), models.PriceSourceManual)
		if err := svc.AddPrice(price1); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		// Update with new price for same security+date
		price2 := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("155.00"), models.PriceSourceManual)
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
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)

		price := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("0"), models.PriceSourceManual)
		err := svc.UpdatePrice(price)
		if err == nil {
			t.Error("UpdatePrice() expected error for zero price")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})
}

// =============================================================================
// SM-032: PriceService.GetCurrentPrice
// =============================================================================

func TestPriceService_GetCurrentPrice(t *testing.T) {
	t.Run("returns most recent price on or before date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)

		// Add prices on different dates
		for _, entry := range []struct {
			date  models.Date
			price string
		}{
			{models.NewDate(2024, time.March, 10), "145.00"},
			{models.NewDate(2024, time.March, 15), "150.00"},
			{models.NewDate(2024, time.March, 20), "155.00"},
		} {
			p := models.NewSecurityPrice(sec.ID, entry.date, models.MustNewMoney(entry.price), models.PriceSourceManual)
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		// Query as of March 17 — should get March 15 price
		asOf := models.NewDate(2024, time.March, 17)
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
		secRepo := repository.NewSecurityRepository(database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		date := models.NewDate(2024, time.March, 15)
		_, err := svc.GetCurrentPrice(models.NewID(), date)
		if err == nil {
			t.Error("GetCurrentPrice() expected error when no price exists")
		}
	})
}

// =============================================================================
// SM-033: PriceService.GetPriceHistory
// =============================================================================

func TestPriceService_GetPriceHistory(t *testing.T) {
	t.Run("returns prices in date range", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)

		dates := []models.Date{
			models.NewDate(2024, time.January, 15),
			models.NewDate(2024, time.February, 15),
			models.NewDate(2024, time.March, 15),
			models.NewDate(2024, time.April, 15),
		}
		for i, d := range dates {
			p := models.NewSecurityPrice(sec.ID, d, models.MustNewMoney("100.00"), models.PriceSourceManual)
			p.Price = models.NewMoneyFromInt(int64(100 + i*10))
			if err := svc.AddPrice(p); err != nil {
				t.Fatalf("AddPrice() error = %v", err)
			}
		}

		from := models.NewDate(2024, time.February, 1)
		to := models.NewDate(2024, time.March, 31)
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
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)

		for _, d := range []models.Date{
			models.NewDate(2024, time.January, 15),
			models.NewDate(2024, time.February, 15),
			models.NewDate(2024, time.March, 15),
		} {
			p := models.NewSecurityPrice(sec.ID, d, models.MustNewMoney("100.00"), models.PriceSourceManual)
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

// =============================================================================
// SM-034: PriceService.DeletePrice
// =============================================================================

func TestPriceService_DeletePrice(t *testing.T) {
	t.Run("deletes price", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)
		price := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("150.00"), models.PriceSourceManual)

		if err := svc.AddPrice(price); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}

		if err := svc.DeletePrice(price.ID); err != nil {
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
		secRepo := repository.NewSecurityRepository(database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		err := svc.DeletePrice(models.NewID())
		if err == nil {
			t.Error("DeletePrice() expected error for non-existent price")
		}
	})

	t.Run("delete does not cascade to security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := repository.NewSecurityRepository(database)
		secSvc := NewSecurityService(secRepo, database)
		priceRepo := repository.NewPriceRepository(database)
		svc := NewPriceService(priceRepo, secRepo, database)

		sec := createTestSecurity(t, secSvc)
		date := models.NewDate(2024, time.March, 15)
		price := models.NewSecurityPrice(sec.ID, date, models.MustNewMoney("150.00"), models.PriceSourceManual)

		if err := svc.AddPrice(price); err != nil {
			t.Fatalf("AddPrice() error = %v", err)
		}
		if err := svc.DeletePrice(price.ID); err != nil {
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
// SM-035: PriceService in registry
// =============================================================================

func TestNewServices_PriceService(t *testing.T) {
	t.Run("returns non-nil PriceService", func(t *testing.T) {
		database := createTestDB(t)
		services := NewServices(database)

		if services.Price == nil {
			t.Error("NewServices() should return non-nil PriceService")
		}
		if services.PriceRepo == nil {
			t.Error("NewServices() should return non-nil PriceRepo")
		}
	})

	t.Run("PriceService has provider registry", func(t *testing.T) {
		database := createTestDB(t)
		services := NewServices(database)

		registry := services.Price.ProviderRegistry()
		if registry == nil {
			t.Error("PriceService should have a non-nil provider registry")
		}

		// Manual provider should be pre-registered
		provider, err := registry.Get("manual")
		if err != nil {
			t.Fatalf("Get('manual') error = %v", err)
		}
		if provider.Name() != "manual" {
			t.Errorf("Expected provider name 'manual', got %q", provider.Name())
		}
	})
}
