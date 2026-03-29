package price

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Provider defines the interface for fetching security prices.
type Provider interface {
	// FetchPrice fetches a single price for a ticker on a given date.
	FetchPrice(ticker string, date types.Date) (*Price, error)

	// FetchPriceHistory fetches prices for a ticker within a date range.
	FetchPriceHistory(ticker string, from, to types.Date) ([]*Price, error)

	// Name returns the name of the price provider.
	Name() string
}

// ManualProvider is a no-op provider that requires manual entry.
type ManualProvider struct{}

// FetchPrice returns an error indicating manual entry is required.
func (m *ManualProvider) FetchPrice(ticker string, date types.Date) (*Price, error) {
	return nil, fmt.Errorf("manual entry required")
}

// FetchPriceHistory returns an error indicating manual entry is required.
func (m *ManualProvider) FetchPriceHistory(ticker string, from, to types.Date) ([]*Price, error) {
	return nil, fmt.Errorf("manual entry required")
}

// Name returns the name of the manual price provider.
func (m *ManualProvider) Name() string {
	return "manual"
}

// ProviderRegistry holds named price providers and allows selection.
type ProviderRegistry struct {
	providers map[string]Provider
}

// NewProviderRegistry creates a new ProviderRegistry with the
// manual provider pre-registered.
func NewProviderRegistry() *ProviderRegistry {
	r := &ProviderRegistry{
		providers: make(map[string]Provider),
	}
	r.Register(&ManualProvider{})
	return r
}

// Register adds a provider to the registry using its Name() as the key.
func (r *ProviderRegistry) Register(provider Provider) {
	r.providers[provider.Name()] = provider
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (Provider, error) {
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("price provider %q not found", name)
	}
	return provider, nil
}

// List returns the names of all registered providers.
func (r *ProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
