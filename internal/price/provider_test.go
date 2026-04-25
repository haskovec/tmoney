package price

import (
	"testing"
)

// =============================================================================
// SM-028: Provider interface and ManualProvider
// =============================================================================

func TestManualProvider_ImplementsInterface(t *testing.T) {
	var _ Provider = &ManualProvider{}
}

func TestManualProvider_FetchQuote(t *testing.T) {
	t.Run("returns error requiring manual entry", func(t *testing.T) {
		p := &ManualProvider{}
		quote, err := p.FetchQuote("AAPL")
		if err == nil {
			t.Fatal("FetchQuote() expected error for manual provider")
		}
		if quote != nil {
			t.Errorf("FetchQuote() expected nil quote, got %+v", quote)
		}
		if err.Error() != "manual entry required" {
			t.Errorf("error = %q, want %q", err.Error(), "manual entry required")
		}
	})
}

func TestManualProvider_Name(t *testing.T) {
	if (&ManualProvider{}).Name() != "manual" {
		t.Errorf("Name() = %q, want %q", (&ManualProvider{}).Name(), "manual")
	}
}

// =============================================================================
// SM-029: ProviderRegistry
// =============================================================================

func TestProviderRegistry_New(t *testing.T) {
	t.Run("has manual provider pre-registered", func(t *testing.T) {
		registry := NewProviderRegistry()

		provider, err := registry.Get("manual")
		if err != nil {
			t.Fatalf("Get('manual') error = %v", err)
		}
		if provider.Name() != "manual" {
			t.Errorf("Expected name 'manual', got %q", provider.Name())
		}
	})
}

func TestProviderRegistry_Register(t *testing.T) {
	t.Run("registers and retrieves provider by name", func(t *testing.T) {
		registry := NewProviderRegistry()
		mock := &mockProvider{name: "mock-api"}

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
		registry := NewProviderRegistry()

		_, err := registry.Get("nonexistent")
		if err == nil {
			t.Error("Get('nonexistent') expected error")
		}
	})
}

func TestProviderRegistry_List(t *testing.T) {
	t.Run("lists all registered providers", func(t *testing.T) {
		registry := NewProviderRegistry()
		registry.Register(&mockProvider{name: "alpha"})
		registry.Register(&mockProvider{name: "beta"})

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

// mockProvider is a test helper implementing Provider.
type mockProvider struct {
	name  string
	quote *Quote
	err   error
}

func (m *mockProvider) FetchQuote(_ string) (*Quote, error) {
	return m.quote, m.err
}

func (m *mockProvider) Name() string {
	return m.name
}
