package service

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// SM-028: PriceProvider interface and ManualPriceProvider
// =============================================================================

func TestManualPriceProvider_ImplementsInterface(t *testing.T) {
	// Compile-time check that ManualPriceProvider implements PriceProvider.
	var _ PriceProvider = &ManualPriceProvider{}
}

func TestManualPriceProvider_FetchPrice(t *testing.T) {
	t.Run("returns error requiring manual entry", func(t *testing.T) {
		provider := &ManualPriceProvider{}
		date := models.NewDate(2024, time.March, 15)

		price, err := provider.FetchPrice("AAPL", date)
		if err == nil {
			t.Error("FetchPrice() expected error for manual provider")
		}
		if price != nil {
			t.Error("FetchPrice() expected nil price for manual provider")
		}
		if err.Error() != "manual entry required" {
			t.Errorf("Expected 'manual entry required', got %q", err.Error())
		}
	})
}

func TestManualPriceProvider_FetchPriceHistory(t *testing.T) {
	t.Run("returns error requiring manual entry", func(t *testing.T) {
		provider := &ManualPriceProvider{}
		from := models.NewDate(2024, time.January, 1)
		to := models.NewDate(2024, time.March, 31)

		prices, err := provider.FetchPriceHistory("AAPL", from, to)
		if err == nil {
			t.Error("FetchPriceHistory() expected error for manual provider")
		}
		if prices != nil {
			t.Error("FetchPriceHistory() expected nil prices for manual provider")
		}
	})
}

func TestManualPriceProvider_Name(t *testing.T) {
	t.Run("returns 'manual'", func(t *testing.T) {
		provider := &ManualPriceProvider{}
		if provider.Name() != "manual" {
			t.Errorf("Expected name 'manual', got %q", provider.Name())
		}
	})
}

// =============================================================================
// SM-029: PriceProviderRegistry
// =============================================================================

func TestPriceProviderRegistry_New(t *testing.T) {
	t.Run("has manual provider pre-registered", func(t *testing.T) {
		registry := NewPriceProviderRegistry()

		provider, err := registry.Get("manual")
		if err != nil {
			t.Fatalf("Get('manual') error = %v", err)
		}
		if provider.Name() != "manual" {
			t.Errorf("Expected name 'manual', got %q", provider.Name())
		}
	})
}

func TestPriceProviderRegistry_Register(t *testing.T) {
	t.Run("registers and retrieves provider by name", func(t *testing.T) {
		registry := NewPriceProviderRegistry()
		mock := &mockPriceProvider{name: "mock-api"}

		registry.Register(mock)

		provider, err := registry.Get("mock-api")
		if err != nil {
			t.Fatalf("Get('mock-api') error = %v", err)
		}
		if provider.Name() != "mock-api" {
			t.Errorf("Expected name 'mock-api', got %q", provider.Name())
		}
	})

	t.Run("returns error for unknown provider", func(t *testing.T) {
		registry := NewPriceProviderRegistry()

		_, err := registry.Get("nonexistent")
		if err == nil {
			t.Error("Get('nonexistent') expected error")
		}
	})
}

func TestPriceProviderRegistry_List(t *testing.T) {
	t.Run("lists all registered providers", func(t *testing.T) {
		registry := NewPriceProviderRegistry()
		registry.Register(&mockPriceProvider{name: "alpha"})
		registry.Register(&mockPriceProvider{name: "beta"})

		names := registry.List()
		if len(names) != 3 { // manual + alpha + beta
			t.Fatalf("Expected 3 providers, got %d", len(names))
		}

		nameMap := make(map[string]bool)
		for _, n := range names {
			nameMap[n] = true
		}
		for _, expected := range []string{"manual", "alpha", "beta"} {
			if !nameMap[expected] {
				t.Errorf("Expected provider %q in list", expected)
			}
		}
	})
}

// mockPriceProvider is a test helper implementing PriceProvider.
type mockPriceProvider struct {
	name string
}

func (m *mockPriceProvider) FetchPrice(ticker string, date models.Date) (*models.SecurityPrice, error) {
	return nil, nil
}

func (m *mockPriceProvider) FetchPriceHistory(ticker string, from, to models.Date) ([]*models.SecurityPrice, error) {
	return nil, nil
}

func (m *mockPriceProvider) Name() string {
	return m.name
}
