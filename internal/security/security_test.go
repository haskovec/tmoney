package security

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// --- SM-001: Type enum tests ---

func TestType(t *testing.T) {
	t.Run("AllTypes returns all types", func(t *testing.T) {
		allTypes := AllTypes()
		expected := 4
		if len(allTypes) != expected {
			t.Errorf("Expected %d security types, got %d", expected, len(allTypes))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if TypeStock.String() != "stock" {
			t.Errorf("Expected 'stock', got %q", TypeStock.String())
		}
		if TypeMutualFund.String() != "mutual_fund" {
			t.Errorf("Expected 'mutual_fund', got %q", TypeMutualFund.String())
		}
	})

	t.Run("IsValid returns true for valid types", func(t *testing.T) {
		validTypes := []Type{
			TypeStock,
			TypeETF,
			TypeMutualFund,
			TypeOther,
		}
		for _, st := range validTypes {
			if !st.IsValid() {
				t.Errorf("IsValid should return true for %q", st)
			}
		}
	})

	t.Run("IsValid returns false for invalid type", func(t *testing.T) {
		invalid := Type("bond")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'bond'")
		}

		empty := Type("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			securityType Type
			expected     string
		}{
			{TypeStock, "Stock"},
			{TypeETF, "ETF"},
			{TypeMutualFund, "Mutual Fund"},
			{TypeOther, "Other"},
		}
		for _, tc := range tests {
			if tc.securityType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.securityType, tc.expected, tc.securityType.DisplayName())
			}
		}
	})
}

func TestParseType(t *testing.T) {
	t.Run("Parses valid types", func(t *testing.T) {
		tests := []struct {
			input    string
			expected Type
		}{
			{"stock", TypeStock},
			{"etf", TypeETF},
			{"mutual_fund", TypeMutualFund},
			{"other", TypeOther},
			{"Stock", TypeStock},
			{"ETF", TypeETF},
		}
		for _, tc := range tests {
			st, err := ParseType(tc.input)
			if err != nil {
				t.Errorf("ParseType(%q) returned error: %v", tc.input, err)
			}
			if st != tc.expected {
				t.Errorf("ParseType(%q) = %q, expected %q", tc.input, st, tc.expected)
			}
		}
	})

	t.Run("Rejects invalid types", func(t *testing.T) {
		_, err := ParseType("bond")
		if err == nil {
			t.Error("ParseType('bond') should return error")
		}
	})
}

func TestTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := TypeStock.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "stock" {
			t.Errorf("Expected 'stock', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var st Type
		err := st.Scan("etf")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if st != TypeETF {
			t.Errorf("Expected TypeETF, got %q", st)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var st Type
		err := st.Scan([]byte("stock"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if st != TypeStock {
			t.Errorf("Expected TypeStock, got %q", st)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var st Type
		err := st.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if st != "" {
			t.Errorf("Expected empty string, got %q", st)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var st Type
		err := st.Scan(123)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}

// --- SM-002: AssetClass enum tests ---

func TestAssetClass(t *testing.T) {
	t.Run("AllAssetClasses returns all classes", func(t *testing.T) {
		classes := AllAssetClasses()
		expected := 12
		if len(classes) != expected {
			t.Errorf("Expected %d asset classes, got %d", expected, len(classes))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if AssetClassLargeCapStock.String() != "large_cap_stock" {
			t.Errorf("Expected 'large_cap_stock', got %q", AssetClassLargeCapStock.String())
		}
		if AssetClassUnclassified.String() != "unclassified" {
			t.Errorf("Expected 'unclassified', got %q", AssetClassUnclassified.String())
		}
	})

	t.Run("IsValid returns true for all valid classes", func(t *testing.T) {
		for _, ac := range AllAssetClasses() {
			if !ac.IsValid() {
				t.Errorf("IsValid should return true for %q", ac)
			}
		}
	})

	t.Run("IsValid returns false for invalid class", func(t *testing.T) {
		invalid := AssetClass("not_a_class")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'not_a_class'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			assetClass AssetClass
			expected   string
		}{
			{AssetClassLargeCapStock, "Large Cap Stock"},
			{AssetClassSmallCapStock, "Small Cap Stock"},
			{AssetClassInternationalStock, "International Stock"},
			{AssetClassIndex, "Index"},
			{AssetClassDomesticBond, "Domestic Bond"},
			{AssetClassForeignBond, "Foreign Bond"},
			{AssetClassCash, "Cash"},
			{AssetClassCommodity, "Commodity"},
			{AssetClassCrypto, "Crypto"},
			{AssetClassAssetMixture, "Asset Mixture"},
			{AssetClassRealEstate, "Real Estate"},
			{AssetClassUnclassified, "Unclassified"},
		}
		for _, tc := range tests {
			if tc.assetClass.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.assetClass, tc.expected, tc.assetClass.DisplayName())
			}
		}
	})
}

func TestParseAssetClass(t *testing.T) {
	t.Run("Parses valid classes", func(t *testing.T) {
		ac, err := ParseAssetClass("large_cap_stock")
		if err != nil {
			t.Fatalf("ParseAssetClass returned error: %v", err)
		}
		if ac != AssetClassLargeCapStock {
			t.Errorf("Expected AssetClassLargeCapStock, got %q", ac)
		}
	})

	t.Run("Parses real_estate", func(t *testing.T) {
		ac, err := ParseAssetClass("real_estate")
		if err != nil {
			t.Fatalf("ParseAssetClass returned error: %v", err)
		}
		if ac != AssetClassRealEstate {
			t.Errorf("Expected AssetClassRealEstate, got %q", ac)
		}
	})

	t.Run("Rejects invalid classes", func(t *testing.T) {
		_, err := ParseAssetClass("not_a_class")
		if err == nil {
			t.Error("ParseAssetClass('not_a_class') should return error")
		}
	})
}

func TestAssetClassScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := AssetClassCash.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "cash" {
			t.Errorf("Expected 'cash', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var ac AssetClass
		err := ac.Scan("crypto")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ac != AssetClassCrypto {
			t.Errorf("Expected AssetClassCrypto, got %q", ac)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var ac AssetClass
		err := ac.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if ac != "" {
			t.Errorf("Expected empty string, got %q", ac)
		}
	})
}

// --- SM-003: Security model struct and validation tests ---

func TestNewSecurity(t *testing.T) {
	t.Run("Creates security with required fields", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)

		if sec.ID.IsNil() {
			t.Error("NewSecurity should create non-nil ID")
		}
		if sec.Ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", sec.Ticker)
		}
		if sec.Name != "Apple Inc." {
			t.Errorf("Expected name 'Apple Inc.', got %q", sec.Name)
		}
		if sec.SecurityType != TypeStock {
			t.Errorf("Expected type stock, got %q", sec.SecurityType)
		}
		if sec.AssetClass != AssetClassUnclassified {
			t.Errorf("Expected asset class unclassified, got %q", sec.AssetClass)
		}
		if sec.Currency != "USD" {
			t.Errorf("Expected currency USD, got %q", sec.Currency)
		}
		if sec.Hidden {
			t.Error("Expected hidden to default to false")
		}
		if sec.Exchange.Valid {
			t.Error("Expected exchange to default to invalid/empty")
		}
		if sec.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
		if sec.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should not be zero")
		}
	})
}

func TestSecurityValidation(t *testing.T) {
	validSecurity := func() *Security {
		return NewSecurity("AAPL", "Apple Inc.", TypeStock)
	}

	t.Run("Valid security passes validation", func(t *testing.T) {
		sec := validSecurity()
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid security should pass validation: %v", errs)
		}
	})

	t.Run("Empty ticker passes validation (tickerless allowed)", func(t *testing.T) {
		// A security may have a name but no ticker (e.g. a collective trust
		// held in a 401k). The ticker is no longer required.
		sec := validSecurity()
		sec.Ticker = ""
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Empty ticker should pass validation now: %v", errs)
		}
	})

	t.Run("Ticker exceeding 20 characters fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.Ticker = strings.Repeat("A", 21)
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Ticker > 20 chars should fail validation")
		}
	})

	t.Run("Ticker at 20 characters passes validation", func(t *testing.T) {
		sec := validSecurity()
		sec.Ticker = strings.Repeat("A", 20)
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Ticker at 20 chars should pass validation: %v", errs)
		}
	})

	t.Run("Empty name fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.Name = ""
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Empty name should fail validation")
		}
	})

	t.Run("Invalid security type fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.SecurityType = Type("bond")
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid security type should fail validation")
		}
	})

	t.Run("Invalid asset class fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.AssetClass = AssetClass("not_a_class")
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid asset class should fail validation")
		}
	})

	t.Run("Invalid currency fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.Currency = "INVALID"
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid currency should fail validation")
		}
	})

	t.Run("Valid ISIN passes validation", func(t *testing.T) {
		sec := validSecurity()
		sec.ISIN = "US0378331005"
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid ISIN should pass: %v", errs)
		}
	})

	t.Run("Invalid ISIN fails validation", func(t *testing.T) {
		sec := validSecurity()
		sec.ISIN = "US0378331004" // wrong check digit
		errs := sec.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid ISIN should fail validation")
		}
	})

	t.Run("Empty ISIN passes validation", func(t *testing.T) {
		sec := validSecurity()
		sec.ISIN = ""
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Empty ISIN should pass (none recorded): %v", errs)
		}
	})

	t.Run("Valid non-USD currency passes validation", func(t *testing.T) {
		sec := validSecurity()
		sec.Currency = "CAD"
		errs := sec.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid CAD currency should pass: %v", errs)
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		sec := validSecurity()
		sec.Name = ""
		sec.ISIN = "BADISIN00000"
		sec.SecurityType = Type("invalid")
		sec.Currency = "INVALID"
		errs := sec.Validate()
		if len(errs) < 4 {
			t.Errorf("Expected at least 4 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("IsValid returns correct boolean", func(t *testing.T) {
		sec := validSecurity()
		if !sec.IsValid() {
			t.Error("Valid security should return true from IsValid")
		}

		sec.Name = ""
		if sec.IsValid() {
			t.Error("Invalid security should return false from IsValid")
		}
	})
}

// --- SM-004: Security helper methods tests ---

func TestSecurityOptionalFields(t *testing.T) {
	t.Run("SetExchange sets valid value", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec.SetExchange("NYSE")
		if !sec.Exchange.Valid {
			t.Error("Exchange should be valid after SetExchange")
		}
		if sec.Exchange.String != "NYSE" {
			t.Errorf("Expected 'NYSE', got %q", sec.Exchange.String)
		}
	})

	t.Run("SetExchange clears with empty string", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec.SetExchange("NYSE")
		sec.SetExchange("")
		if sec.Exchange.Valid {
			t.Error("Exchange should be invalid after SetExchange with empty string")
		}
	})
}

func TestSecurityCanHide(t *testing.T) {
	t.Run("CanHide returns true when no positions exist", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if !sec.CanHide() {
			t.Error("CanHide should return true when no positions exist")
		}
	})
}

func TestSecurityCanDelete(t *testing.T) {
	t.Run("CanDelete returns true as placeholder", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		if !sec.CanDelete() {
			t.Error("CanDelete should return true as placeholder")
		}
	})
}

func TestSecurityHideUnhide(t *testing.T) {
	t.Run("Hide sets hidden to true", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec.Hide()
		if !sec.Hidden {
			t.Error("Security should be hidden after Hide()")
		}
	})

	t.Run("Hide updates UpdatedAt", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		original := sec.UpdatedAt
		sec.Hide()
		if sec.UpdatedAt.Time().Before(original.Time()) {
			t.Error("Hide should update UpdatedAt")
		}
	})

	t.Run("Unhide sets hidden to false", func(t *testing.T) {
		sec := NewSecurity("AAPL", "Apple Inc.", TypeStock)
		sec.Hidden = true
		sec.Unhide()
		if sec.Hidden {
			t.Error("Security should not be hidden after Unhide()")
		}
	})
}

// Ensure types package is used.
var _ types.ID
