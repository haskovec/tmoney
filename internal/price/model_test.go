package price

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// --- SM-005: Source enum tests ---

func TestSource(t *testing.T) {
	t.Run("AllSources returns all sources", func(t *testing.T) {
		sources := AllSources()
		expected := 4
		if len(sources) != expected {
			t.Errorf("Expected %d price sources, got %d", expected, len(sources))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if SourceManual.String() != "manual" {
			t.Errorf("Expected 'manual', got %q", SourceManual.String())
		}
		if SourceTransaction.String() != "transaction" {
			t.Errorf("Expected 'transaction', got %q", SourceTransaction.String())
		}
	})

	t.Run("IsValid returns true for valid sources", func(t *testing.T) {
		validSources := []Source{
			SourceManual,
			SourceTransaction,
			SourceImport,
			SourceAPI,
		}
		for _, ps := range validSources {
			if !ps.IsValid() {
				t.Errorf("IsValid should return true for %q", ps)
			}
		}
	})

	t.Run("IsValid returns false for invalid source", func(t *testing.T) {
		invalid := Source("scrape")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'scrape'")
		}

		empty := Source("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			source   Source
			expected string
		}{
			{SourceManual, "Manual"},
			{SourceTransaction, "Transaction"},
			{SourceImport, "Import"},
			{SourceAPI, "API"},
		}
		for _, tc := range tests {
			if tc.source.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.source, tc.expected, tc.source.DisplayName())
			}
		}
	})
}

func TestParseSource(t *testing.T) {
	t.Run("Parses valid sources", func(t *testing.T) {
		tests := []struct {
			input    string
			expected Source
		}{
			{"manual", SourceManual},
			{"transaction", SourceTransaction},
			{"import", SourceImport},
			{"api", SourceAPI},
			{"Manual", SourceManual},
			{"API", SourceAPI},
		}
		for _, tc := range tests {
			ps, err := ParseSource(tc.input)
			if err != nil {
				t.Errorf("ParseSource(%q) returned error: %v", tc.input, err)
			}
			if ps != tc.expected {
				t.Errorf("ParseSource(%q) = %q, expected %q", tc.input, ps, tc.expected)
			}
		}
	})

	t.Run("Rejects invalid sources", func(t *testing.T) {
		_, err := ParseSource("scrape")
		if err == nil {
			t.Error("ParseSource('scrape') should return error")
		}
	})
}

func TestSourceScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := SourceManual.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "manual" {
			t.Errorf("Expected 'manual', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var ps Source
		err := ps.Scan("import")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != SourceImport {
			t.Errorf("Expected SourceImport, got %q", ps)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var ps Source
		err := ps.Scan([]byte("api"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != SourceAPI {
			t.Errorf("Expected SourceAPI, got %q", ps)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var ps Source
		err := ps.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ps != "" {
			t.Errorf("Expected empty string, got %q", ps)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var ps Source
		err := ps.Scan(123)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}

// --- SM-006: Price model and validation tests ---

func TestNewPrice(t *testing.T) {
	t.Run("Creates price with required fields", func(t *testing.T) {
		secID := types.NewID()
		date := types.NewDate(2024, 6, 15)
		p := types.MustNewMoney("150.25")

		sp := NewPrice(secID, date, p, SourceManual)

		if sp.ID.IsNil() {
			t.Error("NewPrice should create non-nil ID")
		}
		if sp.SecurityID != secID {
			t.Error("SecurityID should match")
		}
		if !sp.Date.Equal(date) {
			t.Errorf("Expected date %v, got %v", date, sp.Date)
		}
		if sp.Price.Cmp(p) != 0 {
			t.Errorf("Expected price %v, got %v", p, sp.Price)
		}
		if sp.Source != SourceManual {
			t.Errorf("Expected source manual, got %q", sp.Source)
		}
		if sp.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
	})
}

func TestPriceValidation(t *testing.T) {
	validPrice := func() *Price {
		return NewPrice(
			types.NewID(),
			types.NewDate(2024, 6, 15),
			types.MustNewMoney("150.25"),
			SourceManual,
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
		sp.SecurityID = types.ID{}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Nil security ID should fail validation")
		}
	})

	t.Run("Zero date fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Date = types.ZeroDate
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Zero date should fail validation")
		}
	})

	t.Run("Future date fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Date = types.NewDate(2099, 12, 31)
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Future date should fail validation")
		}
	})

	t.Run("Zero price fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Price = types.ZeroMoney
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Zero price should fail validation")
		}
	})

	t.Run("Negative price fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Price = types.MustNewMoney("-10.00")
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Negative price should fail validation")
		}
	})

	t.Run("Invalid source fails validation", func(t *testing.T) {
		sp := validPrice()
		sp.Source = Source("invalid")
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid source should fail validation")
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		sp := validPrice()
		sp.SecurityID = types.ID{}
		sp.Date = types.ZeroDate
		sp.Price = types.ZeroMoney
		sp.Source = Source("invalid")
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

		sp.Price = types.ZeroMoney
		if sp.IsValid() {
			t.Error("Invalid price should return false from IsValid")
		}
	})
}
