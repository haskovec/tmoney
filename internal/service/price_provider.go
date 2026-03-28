package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
)

// PriceProvider defines the interface for fetching security prices.
type PriceProvider interface {
	// FetchPrice fetches a single price for a ticker on a given date.
	FetchPrice(ticker string, date models.Date) (*models.SecurityPrice, error)

	// FetchPriceHistory fetches prices for a ticker within a date range.
	FetchPriceHistory(ticker string, from, to models.Date) ([]*models.SecurityPrice, error)

	// Name returns the name of the price provider.
	Name() string
}

// ManualPriceProvider is a no-op provider that requires manual entry.
type ManualPriceProvider struct{}

// FetchPrice returns an error indicating manual entry is required.
func (m *ManualPriceProvider) FetchPrice(ticker string, date models.Date) (*models.SecurityPrice, error) {
	return nil, fmt.Errorf("manual entry required")
}

// FetchPriceHistory returns an error indicating manual entry is required.
func (m *ManualPriceProvider) FetchPriceHistory(ticker string, from, to models.Date) ([]*models.SecurityPrice, error) {
	return nil, fmt.Errorf("manual entry required")
}

// Name returns the name of the manual price provider.
func (m *ManualPriceProvider) Name() string {
	return "manual"
}

// PriceProviderRegistry holds named price providers and allows selection.
type PriceProviderRegistry struct {
	providers map[string]PriceProvider
}

// NewPriceProviderRegistry creates a new PriceProviderRegistry with the
// manual provider pre-registered.
func NewPriceProviderRegistry() *PriceProviderRegistry {
	r := &PriceProviderRegistry{
		providers: make(map[string]PriceProvider),
	}
	r.Register(&ManualPriceProvider{})
	return r
}

// Register adds a provider to the registry using its Name() as the key.
func (r *PriceProviderRegistry) Register(provider PriceProvider) {
	r.providers[provider.Name()] = provider
}

// Get retrieves a provider by name.
func (r *PriceProviderRegistry) Get(name string) (PriceProvider, error) {
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("price provider %q not found", name)
	}
	return provider, nil
}

// List returns the names of all registered providers.
func (r *PriceProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
