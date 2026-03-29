package types

import (
	"regexp"
	"testing"
)

func TestValidationError(t *testing.T) {
	t.Run("Error message format", func(t *testing.T) {
		e := ValidationError{Field: "name", Message: "is required"}
		expected := "name: is required"
		if e.Error() != expected {
			t.Errorf("Expected %q, got %q", expected, e.Error())
		}
	})
}

func TestValidationErrors(t *testing.T) {
	t.Run("Single error message", func(t *testing.T) {
		errs := ValidationErrors{
			{Field: "name", Message: "is required"},
		}
		expected := "name: is required"
		if errs.Error() != expected {
			t.Errorf("Expected %q, got %q", expected, errs.Error())
		}
	})

	t.Run("Multiple errors message", func(t *testing.T) {
		errs := ValidationErrors{
			{Field: "name", Message: "is required"},
			{Field: "amount", Message: "must be positive"},
		}
		result := errs.Error()
		if result != "multiple validation errors: name: is required; amount: must be positive" {
			t.Errorf("Unexpected error message: %s", result)
		}
	})

	t.Run("Empty errors", func(t *testing.T) {
		errs := ValidationErrors{}
		if errs.HasErrors() {
			t.Error("Empty ValidationErrors should not have errors")
		}
	})

	t.Run("Add error", func(t *testing.T) {
		var errs ValidationErrors
		errs.Add("field", "message")
		if !errs.HasErrors() {
			t.Error("ValidationErrors should have errors after Add")
		}
		if len(errs) != 1 {
			t.Errorf("Expected 1 error, got %d", len(errs))
		}
	})
}

func TestValidator(t *testing.T) {
	t.Run("RequiredString passes for non-empty", func(t *testing.T) {
		v := NewValidator()
		v.RequiredString("name", "John")
		if v.HasErrors() {
			t.Error("RequiredString should pass for non-empty string")
		}
	})

	t.Run("RequiredString fails for empty", func(t *testing.T) {
		v := NewValidator()
		v.RequiredString("name", "")
		if !v.HasErrors() {
			t.Error("RequiredString should fail for empty string")
		}
	})

	t.Run("RequiredString fails for whitespace only", func(t *testing.T) {
		v := NewValidator()
		v.RequiredString("name", "   ")
		if !v.HasErrors() {
			t.Error("RequiredString should fail for whitespace-only string")
		}
	})

	t.Run("MaxLength passes for valid length", func(t *testing.T) {
		v := NewValidator()
		v.MaxLength("name", "John", 10)
		if v.HasErrors() {
			t.Error("MaxLength should pass for string under limit")
		}
	})

	t.Run("MaxLength fails for excessive length", func(t *testing.T) {
		v := NewValidator()
		v.MaxLength("name", "John Doe Smith", 5)
		if !v.HasErrors() {
			t.Error("MaxLength should fail for string over limit")
		}
	})

	t.Run("MinLength passes for valid length", func(t *testing.T) {
		v := NewValidator()
		v.MinLength("name", "John", 3)
		if v.HasErrors() {
			t.Error("MinLength should pass for string at or above limit")
		}
	})

	t.Run("MinLength fails for short string", func(t *testing.T) {
		v := NewValidator()
		v.MinLength("name", "Jo", 3)
		if !v.HasErrors() {
			t.Error("MinLength should fail for string below limit")
		}
	})

	t.Run("Positive passes for positive value", func(t *testing.T) {
		v := NewValidator()
		v.Positive("amount", MustNewMoney("100"))
		if v.HasErrors() {
			t.Error("Positive should pass for positive value")
		}
	})

	t.Run("Positive fails for zero", func(t *testing.T) {
		v := NewValidator()
		v.Positive("amount", ZeroMoney)
		if !v.HasErrors() {
			t.Error("Positive should fail for zero")
		}
	})

	t.Run("Positive fails for negative", func(t *testing.T) {
		v := NewValidator()
		v.Positive("amount", MustNewMoney("-50"))
		if !v.HasErrors() {
			t.Error("Positive should fail for negative value")
		}
	})

	t.Run("NonNegative passes for zero", func(t *testing.T) {
		v := NewValidator()
		v.NonNegative("amount", ZeroMoney)
		if v.HasErrors() {
			t.Error("NonNegative should pass for zero")
		}
	})

	t.Run("NonNegative passes for positive", func(t *testing.T) {
		v := NewValidator()
		v.NonNegative("amount", MustNewMoney("100"))
		if v.HasErrors() {
			t.Error("NonNegative should pass for positive value")
		}
	})

	t.Run("NonNegative fails for negative", func(t *testing.T) {
		v := NewValidator()
		v.NonNegative("amount", MustNewMoney("-50"))
		if !v.HasErrors() {
			t.Error("NonNegative should fail for negative value")
		}
	})

	t.Run("RequiredID passes for valid ID", func(t *testing.T) {
		v := NewValidator()
		v.RequiredID("id", NewID())
		if v.HasErrors() {
			t.Error("RequiredID should pass for valid ID")
		}
	})

	t.Run("RequiredID fails for nil ID", func(t *testing.T) {
		v := NewValidator()
		v.RequiredID("id", NilID)
		if !v.HasErrors() {
			t.Error("RequiredID should fail for nil ID")
		}
	})

	t.Run("RequiredDate passes for valid date", func(t *testing.T) {
		v := NewValidator()
		v.RequiredDate("date", Today())
		if v.HasErrors() {
			t.Error("RequiredDate should pass for valid date")
		}
	})

	t.Run("RequiredDate fails for zero date", func(t *testing.T) {
		v := NewValidator()
		v.RequiredDate("date", ZeroDate)
		if !v.HasErrors() {
			t.Error("RequiredDate should fail for zero date")
		}
	})

	t.Run("NotFutureDate passes for today", func(t *testing.T) {
		v := NewValidator()
		v.NotFutureDate("date", Today())
		if v.HasErrors() {
			t.Error("NotFutureDate should pass for today")
		}
	})

	t.Run("NotFutureDate passes for past date", func(t *testing.T) {
		v := NewValidator()
		v.NotFutureDate("date", NewDate(2020, 1, 1))
		if v.HasErrors() {
			t.Error("NotFutureDate should pass for past date")
		}
	})

	t.Run("NotFutureDate fails for future date", func(t *testing.T) {
		v := NewValidator()
		futureDate := Today().AddYears(1)
		v.NotFutureDate("date", futureDate)
		if !v.HasErrors() {
			t.Error("NotFutureDate should fail for future date")
		}
	})

	t.Run("NotFutureDate skips zero date", func(t *testing.T) {
		v := NewValidator()
		v.NotFutureDate("date", ZeroDate)
		if v.HasErrors() {
			t.Error("NotFutureDate should skip zero date")
		}
	})

	t.Run("Percentage passes for valid range", func(t *testing.T) {
		v := NewValidator()
		v.Percentage("rate", MustNewMoney("50"))
		if v.HasErrors() {
			t.Error("Percentage should pass for 50")
		}
	})

	t.Run("Percentage passes for zero", func(t *testing.T) {
		v := NewValidator()
		v.Percentage("rate", ZeroMoney)
		if v.HasErrors() {
			t.Error("Percentage should pass for 0")
		}
	})

	t.Run("Percentage passes for 100", func(t *testing.T) {
		v := NewValidator()
		v.Percentage("rate", MustNewMoney("100"))
		if v.HasErrors() {
			t.Error("Percentage should pass for 100")
		}
	})

	t.Run("Percentage fails for negative", func(t *testing.T) {
		v := NewValidator()
		v.Percentage("rate", MustNewMoney("-5"))
		if !v.HasErrors() {
			t.Error("Percentage should fail for negative value")
		}
	})

	t.Run("Percentage fails for over 100", func(t *testing.T) {
		v := NewValidator()
		v.Percentage("rate", MustNewMoney("105"))
		if !v.HasErrors() {
			t.Error("Percentage should fail for value over 100")
		}
	})

	t.Run("Currency passes for valid code", func(t *testing.T) {
		v := NewValidator()
		v.Currency("currency", "USD")
		if v.HasErrors() {
			t.Error("Currency should pass for USD")
		}
	})

	t.Run("Currency passes for lowercase", func(t *testing.T) {
		v := NewValidator()
		v.Currency("currency", "usd")
		if v.HasErrors() {
			t.Error("Currency should pass for lowercase usd")
		}
	})

	t.Run("Currency fails for invalid code", func(t *testing.T) {
		v := NewValidator()
		v.Currency("currency", "XXX")
		if !v.HasErrors() {
			t.Error("Currency should fail for invalid code")
		}
	})

	t.Run("OneOf passes for valid value", func(t *testing.T) {
		v := NewValidator()
		allowed := []string{"checking", "savings", "credit_card"}
		v.OneOf("type", "checking", allowed)
		if v.HasErrors() {
			t.Error("OneOf should pass for value in list")
		}
	})

	t.Run("OneOf fails for invalid value", func(t *testing.T) {
		v := NewValidator()
		allowed := []string{"checking", "savings", "credit_card"}
		v.OneOf("type", "unknown", allowed)
		if !v.HasErrors() {
			t.Error("OneOf should fail for value not in list")
		}
	})

	t.Run("Matches passes for matching pattern", func(t *testing.T) {
		v := NewValidator()
		pattern := regexp.MustCompile(`^[A-Z]{3}$`)
		v.Matches("code", "USD", pattern, "must be 3 uppercase letters")
		if v.HasErrors() {
			t.Error("Matches should pass for matching pattern")
		}
	})

	t.Run("Matches fails for non-matching pattern", func(t *testing.T) {
		v := NewValidator()
		pattern := regexp.MustCompile(`^[A-Z]{3}$`)
		v.Matches("code", "us", pattern, "must be 3 uppercase letters")
		if !v.HasErrors() {
			t.Error("Matches should fail for non-matching pattern")
		}
	})

	t.Run("Custom passes when condition is true", func(t *testing.T) {
		v := NewValidator()
		v.Custom("field", true, "custom message")
		if v.HasErrors() {
			t.Error("Custom should pass when condition is true")
		}
	})

	t.Run("Custom fails when condition is false", func(t *testing.T) {
		v := NewValidator()
		v.Custom("field", false, "custom message")
		if !v.HasErrors() {
			t.Error("Custom should fail when condition is false")
		}
	})

	t.Run("Chained validations", func(t *testing.T) {
		v := NewValidator().
			RequiredString("name", "John").
			MaxLength("name", "John", 100).
			Currency("currency", "USD")

		if v.HasErrors() {
			t.Error("Chained validations should all pass")
		}
	})

	t.Run("Multiple failures collected", func(t *testing.T) {
		v := NewValidator().
			RequiredString("name", "").
			Currency("currency", "XXX")

		if !v.HasErrors() {
			t.Error("Should have errors")
		}
		if len(v.Errors()) != 2 {
			t.Errorf("Expected 2 errors, got %d", len(v.Errors()))
		}
	})

	t.Run("AddError adds error directly", func(t *testing.T) {
		v := NewValidator()
		v.AddError("field", "message")
		if !v.HasErrors() {
			t.Error("AddError should add an error")
		}
		if len(v.Errors()) != 1 {
			t.Errorf("Expected 1 error, got %d", len(v.Errors()))
		}
	})
}

func TestIsValidCurrency(t *testing.T) {
	validCodes := []string{"USD", "EUR", "GBP", "JPY", "CAD", "AUD"}
	for _, code := range validCodes {
		if !IsValidCurrency(code) {
			t.Errorf("IsValidCurrency should return true for %s", code)
		}
	}

	invalidCodes := []string{"XXX", "ABC", "ZZZ", "INVALID"}
	for _, code := range invalidCodes {
		if IsValidCurrency(code) {
			t.Errorf("IsValidCurrency should return false for %s", code)
		}
	}

	if !IsValidCurrency("usd") {
		t.Error("IsValidCurrency should be case-insensitive")
	}
}

func TestStandaloneValidators(t *testing.T) {
	t.Run("ValidateRequiredString", func(t *testing.T) {
		if err := ValidateRequiredString("name", "John"); err != nil {
			t.Error("ValidateRequiredString should pass for non-empty string")
		}
		if err := ValidateRequiredString("name", ""); err == nil {
			t.Error("ValidateRequiredString should fail for empty string")
		}
	})

	t.Run("ValidateCurrency", func(t *testing.T) {
		if err := ValidateCurrency("currency", "USD"); err != nil {
			t.Error("ValidateCurrency should pass for USD")
		}
		if err := ValidateCurrency("currency", "XXX"); err == nil {
			t.Error("ValidateCurrency should fail for XXX")
		}
	})

	t.Run("ValidateNotFutureDate", func(t *testing.T) {
		if err := ValidateNotFutureDate("date", Today()); err != nil {
			t.Error("ValidateNotFutureDate should pass for today")
		}
		if err := ValidateNotFutureDate("date", Today().AddYears(1)); err == nil {
			t.Error("ValidateNotFutureDate should fail for future date")
		}
	})

	t.Run("ValidatePositive", func(t *testing.T) {
		if err := ValidatePositive("amount", MustNewMoney("100")); err != nil {
			t.Error("ValidatePositive should pass for positive value")
		}
		if err := ValidatePositive("amount", ZeroMoney); err == nil {
			t.Error("ValidatePositive should fail for zero")
		}
	})

	t.Run("ValidatePercentage", func(t *testing.T) {
		if err := ValidatePercentage("rate", MustNewMoney("50")); err != nil {
			t.Error("ValidatePercentage should pass for 50")
		}
		if err := ValidatePercentage("rate", MustNewMoney("150")); err == nil {
			t.Error("ValidatePercentage should fail for 150")
		}
	})
}

func TestBaseModel(t *testing.T) {
	t.Run("NewBaseModel creates valid model", func(t *testing.T) {
		m := NewBaseModel()
		if m.ID.IsNil() {
			t.Error("NewBaseModel should create non-nil ID")
		}
		if m.CreatedAt.IsZero() {
			t.Error("NewBaseModel should set CreatedAt")
		}
		if m.UpdatedAt.IsZero() {
			t.Error("NewBaseModel should set UpdatedAt")
		}
	})

	t.Run("Touch updates UpdatedAt", func(t *testing.T) {
		m := NewBaseModel()
		original := m.UpdatedAt
		m.Touch()
		if m.UpdatedAt.Before(original) {
			t.Error("Touch should update UpdatedAt to a later time")
		}
	})
}

func TestDateRange(t *testing.T) {
	t.Run("DateRange passes for date in range", func(t *testing.T) {
		v := NewValidator()
		min := NewDate(2024, 1, 1)
		max := NewDate(2024, 12, 31)
		date := NewDate(2024, 6, 15)
		v.DateRange("date", date, min, max)
		if v.HasErrors() {
			t.Error("DateRange should pass for date in range")
		}
	})

	t.Run("DateRange fails for date before min", func(t *testing.T) {
		v := NewValidator()
		min := NewDate(2024, 1, 1)
		max := NewDate(2024, 12, 31)
		date := NewDate(2023, 6, 15)
		v.DateRange("date", date, min, max)
		if !v.HasErrors() {
			t.Error("DateRange should fail for date before min")
		}
	})

	t.Run("DateRange fails for date after max", func(t *testing.T) {
		v := NewValidator()
		min := NewDate(2024, 1, 1)
		max := NewDate(2024, 12, 31)
		date := NewDate(2025, 6, 15)
		v.DateRange("date", date, min, max)
		if !v.HasErrors() {
			t.Error("DateRange should fail for date after max")
		}
	})

	t.Run("DateRange skips zero date", func(t *testing.T) {
		v := NewValidator()
		min := NewDate(2024, 1, 1)
		max := NewDate(2024, 12, 31)
		v.DateRange("date", ZeroDate, min, max)
		if v.HasErrors() {
			t.Error("DateRange should skip zero date")
		}
	})
}
