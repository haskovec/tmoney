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
