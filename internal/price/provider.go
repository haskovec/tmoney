package price

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Quote is a single price observation returned by a Provider.
// Currency is the currency the provider reports the price in; the service
// layer is responsible for verifying it matches the security's currency
// before persisting.
type Quote struct {
	Date     types.Date
	Price    types.Money
	Currency string
}

// Provider fetches the most recent closed-session price for a ticker.
type Provider interface {
	// FetchQuote returns the latest closed-session quote for ticker.
	// Implementations must never return an in-progress (intraday) price;
	// they should fall back to the previous closed session if the current
	// one has not yet closed.
	FetchQuote(ticker string) (*Quote, error)

	// FetchQuoteOn returns the closing quote for ticker on or before the given
	// date, resolving a weekend/holiday date to the prior trading day.
	FetchQuoteOn(ticker string, date types.Date) (*Quote, error)

	// Name returns the provider's identifier (e.g., "yahoo", "manual").
	Name() string
}

// ManualProvider is a no-op provider used as a sentinel when prices must
// be entered by hand. FetchQuote always returns an error.
type ManualProvider struct{}

// FetchQuote returns an error indicating manual entry is required.
func (m *ManualProvider) FetchQuote(_ string) (*Quote, error) {
	return nil, fmt.Errorf("manual entry required")
}

// FetchQuoteOn returns an error indicating manual entry is required.
func (m *ManualProvider) FetchQuoteOn(_ string, _ types.Date) (*Quote, error) {
	return nil, fmt.Errorf("manual entry required")
}

// Name returns "manual".
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

// UnsupportedTickerError is returned by a Provider when the upstream
// service has no data for the requested ticker (e.g., Yahoo 404 or an
// empty result set). The orchestrator treats this as a per-ticker
// failure and continues with the rest of the run.
type UnsupportedTickerError struct {
	Ticker string
	Detail string
}

func (e *UnsupportedTickerError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("ticker %q not supported by provider: %s", e.Ticker, e.Detail)
	}
	return fmt.Sprintf("ticker %q not supported by provider", e.Ticker)
}
