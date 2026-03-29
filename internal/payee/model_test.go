package payee

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestMatchType(t *testing.T) {
	t.Run("AllMatchTypes returns all types", func(t *testing.T) {
		allTypes := AllMatchTypes()
		expected := 4
		if len(allTypes) != expected {
			t.Errorf("Expected %d match types, got %d", expected, len(allTypes))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			matchType MatchType
			expected  string
		}{
			{MatchTypeExact, "exact"},
			{MatchTypeContains, "contains"},
			{MatchTypeStartsWith, "starts_with"},
			{MatchTypeRegex, "regex"},
		}
		for _, tc := range tests {
			if tc.matchType.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.matchType.String())
			}
		}
	})

	t.Run("IsValid returns true for valid types", func(t *testing.T) {
		validTypes := []MatchType{
			MatchTypeExact,
			MatchTypeContains,
			MatchTypeStartsWith,
			MatchTypeRegex,
		}
		for _, mt := range validTypes {
			if !mt.IsValid() {
				t.Errorf("IsValid should return true for %q", mt)
			}
		}
	})

	t.Run("IsValid returns false for invalid type", func(t *testing.T) {
		invalid := MatchType("unknown")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'unknown'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			matchType MatchType
			expected  string
		}{
			{MatchTypeExact, "Exact"},
			{MatchTypeContains, "Contains"},
			{MatchTypeStartsWith, "Starts With"},
			{MatchTypeRegex, "Regular Expression"},
		}
		for _, tc := range tests {
			if tc.matchType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.matchType, tc.expected, tc.matchType.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown type", func(t *testing.T) {
		unknown := MatchType("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestParseMatchType(t *testing.T) {
	t.Run("Parses valid exact type", func(t *testing.T) {
		mt, err := ParseMatchType("exact")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeExact {
			t.Errorf("Expected MatchTypeExact, got %q", mt)
		}
	})

	t.Run("Parses valid contains type", func(t *testing.T) {
		mt, err := ParseMatchType("contains")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeContains {
			t.Errorf("Expected MatchTypeContains, got %q", mt)
		}
	})

	t.Run("Parses valid starts_with type", func(t *testing.T) {
		mt, err := ParseMatchType("starts_with")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeStartsWith {
			t.Errorf("Expected MatchTypeStartsWith, got %q", mt)
		}
	})

	t.Run("Parses valid regex type", func(t *testing.T) {
		mt, err := ParseMatchType("regex")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeRegex {
			t.Errorf("Expected MatchTypeRegex, got %q", mt)
		}
	})

	t.Run("Parses uppercase type", func(t *testing.T) {
		mt, err := ParseMatchType("EXACT")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeExact {
			t.Errorf("Expected MatchTypeExact, got %q", mt)
		}
	})

	t.Run("Parses mixed case type", func(t *testing.T) {
		mt, err := ParseMatchType("Contains")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeContains {
			t.Errorf("Expected MatchTypeContains, got %q", mt)
		}
	})

	t.Run("Returns error for invalid type", func(t *testing.T) {
		_, err := ParseMatchType("invalid")
		if err == nil {
			t.Error("Expected error for invalid match type")
		}
	})

	t.Run("Returns error for empty string", func(t *testing.T) {
		_, err := ParseMatchType("")
		if err == nil {
			t.Error("Expected error for empty string")
		}
	})
}

func TestMatchTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		mt := MatchTypeExact
		v, err := mt.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "exact" {
			t.Errorf("Expected 'exact', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var mt MatchType
		err := mt.Scan("contains")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeContains {
			t.Errorf("Expected MatchTypeContains, got %q", mt)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var mt MatchType
		err := mt.Scan([]byte("starts_with"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != MatchTypeStartsWith {
			t.Errorf("Expected MatchTypeStartsWith, got %q", mt)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var mt MatchType
		err := mt.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if mt != "" {
			t.Errorf("Expected empty string, got %q", mt)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var mt MatchType
		err := mt.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewPayee(t *testing.T) {
	t.Run("Creates payee with name", func(t *testing.T) {
		p := NewPayee("Amazon")

		if p.ID.IsNil() {
			t.Error("NewPayee should create non-nil ID")
		}
		if p.Name != "Amazon" {
			t.Errorf("Expected name 'Amazon', got %q", p.Name)
		}
		if p.DefaultCategoryID.Valid {
			t.Error("NewPayee should not set default category")
		}
		if p.Notes.Valid {
			t.Error("NewPayee should not set notes")
		}
		if p.CreatedAt.IsZero() {
			t.Error("NewPayee should set CreatedAt")
		}
		if p.UpdatedAt.IsZero() {
			t.Error("NewPayee should set UpdatedAt")
		}
	})
}

func TestNewPayeeWithCategory(t *testing.T) {
	t.Run("Creates payee with category", func(t *testing.T) {
		categoryID := types.NewID()
		p := NewPayeeWithCategory("Kroger", categoryID)

		if p.ID.IsNil() {
			t.Error("NewPayeeWithCategory should create non-nil ID")
		}
		if p.Name != "Kroger" {
			t.Errorf("Expected name 'Kroger', got %q", p.Name)
		}
		if !p.DefaultCategoryID.Valid {
			t.Error("NewPayeeWithCategory should set default category")
		}
		if p.DefaultCategoryID.ID != categoryID {
			t.Errorf("Expected category ID %s, got %s", categoryID.String(), p.DefaultCategoryID.ID.String())
		}
		if p.Notes.Valid {
			t.Error("NewPayeeWithCategory should not set notes")
		}
	})
}

func TestPayeeDefaultCategory(t *testing.T) {
	t.Run("SetDefaultCategory sets category", func(t *testing.T) {
		p := NewPayee("Test")
		categoryID := types.NewID()

		if p.HasDefaultCategory() {
			t.Error("Payee should start without default category")
		}

		p.SetDefaultCategory(categoryID)

		if !p.HasDefaultCategory() {
			t.Error("HasDefaultCategory should return true after setting")
		}
		if p.DefaultCategoryID.ID != categoryID {
			t.Errorf("Expected category ID %s, got %s", categoryID.String(), p.DefaultCategoryID.ID.String())
		}
	})

	t.Run("SetDefaultCategory updates UpdatedAt", func(t *testing.T) {
		p := NewPayee("Test")
		original := p.UpdatedAt
		categoryID := types.NewID()

		p.SetDefaultCategory(categoryID)

		if !p.UpdatedAt.After(original) && !p.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetDefaultCategory should update UpdatedAt")
		}
	})

	t.Run("ClearDefaultCategory removes category", func(t *testing.T) {
		categoryID := types.NewID()
		p := NewPayeeWithCategory("Test", categoryID)

		if !p.HasDefaultCategory() {
			t.Error("Payee should start with default category")
		}

		p.ClearDefaultCategory()

		if p.HasDefaultCategory() {
			t.Error("HasDefaultCategory should return false after clearing")
		}
		if p.DefaultCategoryID.Valid {
			t.Error("DefaultCategoryID should not be valid after clearing")
		}
	})

	t.Run("ClearDefaultCategory updates UpdatedAt", func(t *testing.T) {
		categoryID := types.NewID()
		p := NewPayeeWithCategory("Test", categoryID)
		original := p.UpdatedAt

		p.ClearDefaultCategory()

		if !p.UpdatedAt.After(original) && !p.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("ClearDefaultCategory should update UpdatedAt")
		}
	})
}

func TestPayeeNotes(t *testing.T) {
	t.Run("SetNotes sets notes", func(t *testing.T) {
		p := NewPayee("Test")

		p.SetNotes("Some notes about this payee")

		if !p.Notes.Valid {
			t.Error("Notes should be valid after setting")
		}
		if p.Notes.String != "Some notes about this payee" {
			t.Errorf("Expected notes to be set, got %q", p.Notes.String)
		}
	})

	t.Run("SetNotes with empty string clears notes", func(t *testing.T) {
		p := NewPayee("Test")
		p.SetNotes("Some notes")

		p.SetNotes("")

		if p.Notes.Valid {
			t.Error("Notes should not be valid after setting empty string")
		}
	})

	t.Run("SetNotes updates UpdatedAt", func(t *testing.T) {
		p := NewPayee("Test")
		original := p.UpdatedAt

		p.SetNotes("Some notes")

		if !p.UpdatedAt.After(original) && !p.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetNotes should update UpdatedAt")
		}
	})

	t.Run("ClearNotes removes notes", func(t *testing.T) {
		p := NewPayee("Test")
		p.SetNotes("Some notes")

		p.ClearNotes()

		if p.Notes.Valid {
			t.Error("Notes should not be valid after clearing")
		}
	})

	t.Run("ClearNotes updates UpdatedAt", func(t *testing.T) {
		p := NewPayee("Test")
		p.SetNotes("Some notes")
		original := p.UpdatedAt

		p.ClearNotes()

		if !p.UpdatedAt.After(original) && !p.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("ClearNotes should update UpdatedAt")
		}
	})
}

func TestPayeeValidation(t *testing.T) {
	validPayee := func() *Payee {
		return NewPayee("Test Payee")
	}

	t.Run("Valid payee passes validation", func(t *testing.T) {
		p := validPayee()
		errs := p.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid payee should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid payee", func(t *testing.T) {
		p := validPayee()
		if !p.IsValid() {
			t.Error("IsValid should return true for valid payee")
		}
	})

	t.Run("Empty name fails validation", func(t *testing.T) {
		p := validPayee()
		p.Name = ""
		errs := p.Validate()
		if !errs.HasErrors() {
			t.Error("Empty name should fail validation")
		}
	})

	t.Run("Whitespace-only name fails validation", func(t *testing.T) {
		p := validPayee()
		p.Name = "   "
		errs := p.Validate()
		if !errs.HasErrors() {
			t.Error("Whitespace-only name should fail validation")
		}
	})

	t.Run("Name exceeding max length fails validation", func(t *testing.T) {
		p := validPayee()
		p.Name = string(make([]byte, 256))
		errs := p.Validate()
		if !errs.HasErrors() {
			t.Error("Name exceeding 255 chars should fail validation")
		}
	})

	t.Run("Name at max length passes validation", func(t *testing.T) {
		p := validPayee()
		p.Name = string(make([]byte, 255))
		errs := p.Validate()
		if errs.HasErrors() {
			t.Errorf("Name at 255 chars should pass validation: %v", errs)
		}
	})

	t.Run("Notes exceeding max length fails validation", func(t *testing.T) {
		p := validPayee()
		p.Notes = types.NullableString{String: string(make([]byte, 2001)), Valid: true}
		errs := p.Validate()
		if !errs.HasErrors() {
			t.Error("Notes exceeding 2000 chars should fail validation")
		}
	})

	t.Run("Notes at max length passes validation", func(t *testing.T) {
		p := validPayee()
		p.Notes = types.NullableString{String: string(make([]byte, 2000)), Valid: true}
		errs := p.Validate()
		if errs.HasErrors() {
			t.Errorf("Notes at 2000 chars should pass validation: %v", errs)
		}
	})

	t.Run("Payee with category passes validation", func(t *testing.T) {
		categoryID := types.NewID()
		p := NewPayeeWithCategory("Test", categoryID)
		errs := p.Validate()
		if errs.HasErrors() {
			t.Errorf("Payee with category should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns false for invalid payee", func(t *testing.T) {
		p := NewPayee("")
		if p.IsValid() {
			t.Error("IsValid should return false for payee with empty name")
		}
	})
}

func TestNewAlias(t *testing.T) {
	t.Run("Creates alias with all properties", func(t *testing.T) {
		payeeID := types.NewID()
		alias := NewAlias(payeeID, "AMAZON.COM", MatchTypeExact)

		if alias.ID.IsNil() {
			t.Error("NewAlias should create non-nil ID")
		}
		if alias.PayeeID != payeeID {
			t.Errorf("Expected payee ID %s, got %s", payeeID.String(), alias.PayeeID.String())
		}
		if alias.Pattern != "AMAZON.COM" {
			t.Errorf("Expected pattern 'AMAZON.COM', got %q", alias.Pattern)
		}
		if alias.MatchType != MatchTypeExact {
			t.Errorf("Expected match type exact, got %q", alias.MatchType)
		}
		if alias.CreatedAt.IsZero() {
			t.Error("NewAlias should set CreatedAt")
		}
		if alias.UpdatedAt.IsZero() {
			t.Error("NewAlias should set UpdatedAt")
		}
	})
}

func TestAliasConstructors(t *testing.T) {
	payeeID := types.NewID()

	t.Run("NewExactAlias creates exact match alias", func(t *testing.T) {
		alias := NewExactAlias(payeeID, "TEST")
		if alias.MatchType != MatchTypeExact {
			t.Errorf("Expected MatchTypeExact, got %q", alias.MatchType)
		}
	})

	t.Run("NewContainsAlias creates contains match alias", func(t *testing.T) {
		alias := NewContainsAlias(payeeID, "TEST")
		if alias.MatchType != MatchTypeContains {
			t.Errorf("Expected MatchTypeContains, got %q", alias.MatchType)
		}
	})

	t.Run("NewStartsWithAlias creates starts_with match alias", func(t *testing.T) {
		alias := NewStartsWithAlias(payeeID, "TEST")
		if alias.MatchType != MatchTypeStartsWith {
			t.Errorf("Expected MatchTypeStartsWith, got %q", alias.MatchType)
		}
	})

	t.Run("NewRegexAlias creates regex match alias", func(t *testing.T) {
		alias := NewRegexAlias(payeeID, "TEST.*")
		if alias.MatchType != MatchTypeRegex {
			t.Errorf("Expected MatchTypeRegex, got %q", alias.MatchType)
		}
	})
}

func TestAliasMatches(t *testing.T) {
	payeeID := types.NewID()

	t.Run("Exact match works correctly", func(t *testing.T) {
		alias := NewExactAlias(payeeID, "AMAZON.COM")

		if !alias.Matches("AMAZON.COM") {
			t.Error("Should match exact string")
		}
		if alias.Matches("amazon.com") {
			t.Error("Should not match different case")
		}
		if alias.Matches("AMAZON.COM MARKETPLACE") {
			t.Error("Should not match with extra characters")
		}
	})

	t.Run("Contains match works correctly", func(t *testing.T) {
		alias := NewContainsAlias(payeeID, "AMZN")

		if !alias.Matches("AMZN") {
			t.Error("Should match exact string")
		}
		if !alias.Matches("AMZN*1234") {
			t.Error("Should match substring")
		}
		if !alias.Matches("SOMETHING AMZN SOMETHING") {
			t.Error("Should match in middle")
		}
		if alias.Matches("AMAZON") {
			t.Error("Should not match different string")
		}
	})

	t.Run("StartsWith match works correctly", func(t *testing.T) {
		alias := NewStartsWithAlias(payeeID, "KROGER")

		if !alias.Matches("KROGER") {
			t.Error("Should match exact string")
		}
		if !alias.Matches("KROGER #123") {
			t.Error("Should match prefix")
		}
		if !alias.Matches("KROGER FUEL") {
			t.Error("Should match prefix with suffix")
		}
		if alias.Matches("SOMETHING KROGER") {
			t.Error("Should not match when not at start")
		}
	})

	t.Run("Regex match works correctly", func(t *testing.T) {
		alias := NewRegexAlias(payeeID, "^AMZN.*MKTP")

		if !alias.Matches("AMZN MKTP US") {
			t.Error("Should match regex pattern")
		}
		if !alias.Matches("AMZN*MKTP") {
			t.Error("Should match regex pattern with different middle")
		}
		if alias.Matches("AMAZON MARKETPLACE") {
			t.Error("Should not match non-matching string")
		}
	})

	t.Run("Invalid regex returns false", func(t *testing.T) {
		alias := NewRegexAlias(payeeID, "[invalid")
		if alias.Matches("anything") {
			t.Error("Invalid regex should not match")
		}
	})

	t.Run("Unknown match type returns false", func(t *testing.T) {
		alias := &Alias{
			PayeeID:   payeeID,
			Pattern:   "test",
			MatchType: MatchType("unknown"),
		}
		if alias.Matches("test") {
			t.Error("Unknown match type should not match")
		}
	})
}

func TestAliasMatchesCaseInsensitive(t *testing.T) {
	payeeID := types.NewID()

	t.Run("Exact match ignores case", func(t *testing.T) {
		alias := NewExactAlias(payeeID, "AMAZON.COM")

		if !alias.MatchesCaseInsensitive("AMAZON.COM") {
			t.Error("Should match exact string")
		}
		if !alias.MatchesCaseInsensitive("amazon.com") {
			t.Error("Should match lowercase")
		}
		if !alias.MatchesCaseInsensitive("Amazon.Com") {
			t.Error("Should match mixed case")
		}
	})

	t.Run("Contains match ignores case", func(t *testing.T) {
		alias := NewContainsAlias(payeeID, "AMZN")

		if !alias.MatchesCaseInsensitive("amzn*1234") {
			t.Error("Should match lowercase substring")
		}
		if !alias.MatchesCaseInsensitive("Amzn MKTP") {
			t.Error("Should match mixed case")
		}
	})

	t.Run("StartsWith match ignores case", func(t *testing.T) {
		alias := NewStartsWithAlias(payeeID, "KROGER")

		if !alias.MatchesCaseInsensitive("kroger #123") {
			t.Error("Should match lowercase prefix")
		}
		if !alias.MatchesCaseInsensitive("Kroger Fuel") {
			t.Error("Should match mixed case")
		}
	})

	t.Run("Regex match with case insensitive flag", func(t *testing.T) {
		alias := NewRegexAlias(payeeID, "^AMZN.*MKTP")

		if !alias.MatchesCaseInsensitive("amzn mktp us") {
			t.Error("Should match lowercase with (?i) flag")
		}
		if !alias.MatchesCaseInsensitive("Amzn*Mktp") {
			t.Error("Should match mixed case")
		}
	})

	t.Run("Invalid regex returns false for case insensitive", func(t *testing.T) {
		alias := NewRegexAlias(payeeID, "[invalid")
		if alias.MatchesCaseInsensitive("anything") {
			t.Error("Invalid regex should not match")
		}
	})
}

func TestAliasValidation(t *testing.T) {
	validAlias := func() *Alias {
		return NewExactAlias(types.NewID(), "TEST PATTERN")
	}

	t.Run("Valid alias passes validation", func(t *testing.T) {
		alias := validAlias()
		errs := alias.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid alias should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid alias", func(t *testing.T) {
		alias := validAlias()
		if !alias.IsValid() {
			t.Error("IsValid should return true for valid alias")
		}
	})

	t.Run("Nil payee ID fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.PayeeID = types.NilID
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Nil payee ID should fail validation")
		}
	})

	t.Run("Empty pattern fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.Pattern = ""
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Empty pattern should fail validation")
		}
	})

	t.Run("Whitespace-only pattern fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.Pattern = "   "
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Whitespace-only pattern should fail validation")
		}
	})

	t.Run("Pattern exceeding max length fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.Pattern = string(make([]byte, 501))
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Pattern exceeding 500 chars should fail validation")
		}
	})

	t.Run("Pattern at max length passes validation", func(t *testing.T) {
		alias := validAlias()
		alias.Pattern = string(make([]byte, 500))
		errs := alias.Validate()
		if errs.HasErrors() {
			t.Errorf("Pattern at 500 chars should pass validation: %v", errs)
		}
	})

	t.Run("Invalid match type fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.MatchType = MatchType("invalid")
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid match type should fail validation")
		}
	})

	t.Run("Empty match type fails validation", func(t *testing.T) {
		alias := validAlias()
		alias.MatchType = MatchType("")
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Empty match type should fail validation")
		}
	})

	t.Run("Invalid regex pattern fails validation", func(t *testing.T) {
		alias := NewRegexAlias(types.NewID(), "[invalid")
		errs := alias.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid regex pattern should fail validation")
		}
	})

	t.Run("Valid regex pattern passes validation", func(t *testing.T) {
		alias := NewRegexAlias(types.NewID(), "^AMZN.*MKTP$")
		errs := alias.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid regex pattern should pass validation: %v", errs)
		}
	})

	t.Run("All match types pass validation", func(t *testing.T) {
		payeeID := types.NewID()
		for _, mt := range AllMatchTypes() {
			alias := NewAlias(payeeID, "TEST", mt)
			errs := alias.Validate()
			if errs.HasErrors() {
				t.Errorf("Match type %q should pass validation: %v", mt, errs)
			}
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		alias := &Alias{
			PayeeID:   types.NilID,
			Pattern:   "",
			MatchType: MatchType("bad"),
		}
		errs := alias.Validate()
		if len(errs) < 3 {
			t.Errorf("Expected at least 3 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("IsValid returns false for invalid alias", func(t *testing.T) {
		alias := NewExactAlias(types.NilID, "")
		if alias.IsValid() {
			t.Error("IsValid should return false for alias with nil payee ID and empty pattern")
		}
	})
}
