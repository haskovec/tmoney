package models

import (
	"testing"
)

// --- SM-005: PriceSource enum tests ---

func TestPriceSource(t *testing.T) {
	t.Run("AllPriceSources returns all sources", func(t *testing.T) {
		sources := AllPriceSources()
		expected := 4
		if len(sources) != expected {
			t.Errorf("Expected %d price sources, got %d", expected, len(sources))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if PriceSourceManual.String() != "manual" {
			t.Errorf("Expected 'manual', got %q", PriceSourceManual.String())
		}
		if PriceSourceTransaction.String() != "transaction" {
			t.Errorf("Expected 'transaction', got %q", PriceSourceTransaction.String())
		}
	})

	t.Run("IsValid returns true for valid sources", func(t *testing.T) {
		validSources := []PriceSource{
			PriceSourceManual,
			PriceSourceTransaction,
			PriceSourceImport,
			PriceSourceAPI,
		}
		for _, ps := range validSources {
			if !ps.IsValid() {
				t.Errorf("IsValid should return true for %q", ps)
			}
		}
	})

	t.Run("IsValid returns false for invalid source", func(t *testing.T) {
		invalid := PriceSource("scrape")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'scrape'")
		}

		empty := PriceSource("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			source   PriceSource
			expected string
		}{
			{PriceSourceManual, "Manual"},
			{PriceSourceTransaction, "Transaction"},
			{PriceSourceImport, "Import"},
			{PriceSourceAPI, "API"},
		}
		for _, tc := range tests {
			if tc.source.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.source, tc.expected, tc.source.DisplayName())
			}
		}
	})
}

func TestParsePriceSource(t *testing.T) {
	t.Run("Parses valid sources", func(t *testing.T) {
		tests := []struct {
			input    string
			expected PriceSource
		}{
			{"manual", PriceSourceManual},
			{"transaction", PriceSourceTransaction},
			{"import", PriceSourceImport},
			{"api", PriceSourceAPI},
			{"Manual", PriceSourceManual},
			{"API", PriceSourceAPI},
		}
		for _, tc := range tests {
			ps, err := ParsePriceSource(tc.input)
			if err != nil {
				t.Errorf("ParsePriceSource(%q) returned error: %v", tc.input, err)
			}
			if ps != tc.expected {
				t.Errorf("ParsePriceSource(%q) = %q, expected %q", tc.input, ps, tc.expected)
			}
		}
	})

	t.Run("Rejects invalid sources", func(t *testing.T) {
		_, err := ParsePriceSource("scrape")
		if err == nil {
			t.Error("ParsePriceSource('scrape') should return error")
		}
	})
}

func TestPriceSourceScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := PriceSourceManual.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "manual" {
			t.Errorf("Expected 'manual', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var ps PriceSource
		err := ps.Scan("import")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != PriceSourceImport {
			t.Errorf("Expected PriceSourceImport, got %q", ps)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var ps PriceSource
		err := ps.Scan([]byte("api"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != PriceSourceAPI {
			t.Errorf("Expected PriceSourceAPI, got %q", ps)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var ps PriceSource
		err := ps.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != "" {
			t.Errorf("Expected empty string, got %q", ps)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var ps PriceSource
		err := ps.Scan(123)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}

// --- SM-006: SecurityPrice model and validation tests ---

func TestNewSecurityPrice(t *testing.T) {
	t.Run("Creates price with required fields", func(t *testing.T) {
		secID := NewID()
		date := NewDate(2024, 6, 15)
		price := MustNewMoney("150.25")

		sp := NewSecurityPrice(secID, date, price, PriceSourceManual)

		if sp.ID.IsNil() {
			t.Error("NewSecurityPrice should create non-nil ID")
		}
		if sp.SecurityID != secID {
			t.Error("SecurityID should match")
		}
		if !sp.Date.Equal(date) {
			t.Errorf("Expected date %v, got %v", date, sp.Date)
		}
		if sp.Price.Cmp(price) != 0 {
			t.Errorf("Expected price %v, got %v", price, sp.Price)
		}
		if sp.Source != PriceSourceManual {
			t.Errorf("Expected source manual, got %q", sp.Source)
		}
		if sp.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
	})
}

func TestSecurityPriceValidation(t *testing.T) {
	validPrice := func() *SecurityPrice {
		return NewSecurityPrice(
			NewID(),
			NewDate(2024, 6, 15),
			MustNewMoney("150.25"),
			PriceSourceManual,
		)
	}

	t.Run("Valid price passes validation", func(t *testing.T) {
		sp := validPrice()
		errs := sp.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid price should pass validation: %v", errs)
		}
	})

	t.Run("Nil security ID fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.SecurityID = ID{}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Nil security ID should fail validation")
		}
	})

	t.Run("Zero date fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Date = ZeroDate
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Zero date should fail validation")
		}
	})

	t.Run("Future date fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Date = NewDate(2099, 12, 31)
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Future date should fail validation")
		}
	})

	t.Run("Zero price fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Price = ZeroMoney
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Zero price should fail validation")
		}
	})

	t.Run("Negative price fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Price = MustNewMoney("-10.00")
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Negative price should fail validation")
		}
	})

	t.Run("Invalid source fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Source = PriceSource("invalid")
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid source should fail validation")
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		sp := validPrice()
		sp.SecurityID = ID{}
		sp.Date = ZeroDate
		sp.Price = ZeroMoney
		sp.Source = PriceSource("invalid")
		errs := sp.Validate()
		if len(errs) < 3 {
			t.Errorf("Expected at least 3 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("IsValid returns correct boolean", func(t *testing.T) {
		sp := validPrice()
		if !sp.IsValid() {
			t.Error("Valid price should return true from IsValid")
		}

		sp.Price = ZeroMoney
		if sp.IsValid() {
			t.Error("Invalid price should return false from IsValid")
		}
	})
}
