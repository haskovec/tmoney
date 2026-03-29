package models

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ValidationError represents a validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "no validation errors"
	}
	if len(ve) == 1 {
		return ve[0].Error()
	}
	var sb strings.Builder
	sb.WriteString("multiple validation errors: ")
	for i, e := range ve {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(e.Error())
	}
	return sb.String()
}

// HasErrors returns true if there are any validation errors.
func (ve ValidationErrors) HasErrors() bool {
	return len(ve) > 0
}

// Add adds a validation error to the collection.
func (ve *ValidationErrors) Add(field, message string) {
	*ve = append(*ve, ValidationError{Field: field, Message: message})
}

// Validator provides a fluent interface for building validation rules.
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Errors returns the collected validation errors.
func (v *Validator) Errors() ValidationErrors {
	return v.errors
}

// HasErrors returns true if any validation errors were recorded.
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// RequiredString validates that a string is not empty.
func (v *Validator) RequiredString(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.errors.Add(field, "is required")
	}
	return v
}

// MaxLength validates that a string doesn't exceed the maximum length.
func (v *Validator) MaxLength(field, value string, max int) *Validator {
	if len(value) > max {
		v.errors.Add(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return v
}

// MinLength validates that a string meets the minimum length.
func (v *Validator) MinLength(field, value string, min int) *Validator {
	if len(value) < min {
		v.errors.Add(field, fmt.Sprintf("must be at least %d characters", min))
	}
	return v
}

// Positive validates that a Money value is positive (greater than zero).
func (v *Validator) Positive(field string, value Money) *Validator {
	if !value.IsPositive() {
		v.errors.Add(field, "must be positive")
	}
	return v
}

// NonNegative validates that a Money value is not negative.
func (v *Validator) NonNegative(field string, value Money) *Validator {
	if value.IsNegative() {
		v.errors.Add(field, "must not be negative")
	}
	return v
}

// PositiveQuantity validates that a Quantity value is positive (greater than zero).
func (v *Validator) PositiveQuantity(field string, value Quantity) *Validator {
	if !value.IsPositive() {
		v.errors.Add(field, "must be positive")
	}
	return v
}

// NonNegativeQuantity validates that a Quantity value is not negative.
func (v *Validator) NonNegativeQuantity(field string, value Quantity) *Validator {
	if value.IsNegative() {
		v.errors.Add(field, "must not be negative")
	}
	return v
}

// RequiredID validates that an ID is not nil.
func (v *Validator) RequiredID(field string, id ID) *Validator {
	if id.IsNil() {
		v.errors.Add(field, "is required")
	}
	return v
}

// RequiredDate validates that a Date is not zero.
func (v *Validator) RequiredDate(field string, date Date) *Validator {
	if date.IsZero() {
		v.errors.Add(field, "is required")
	}
	return v
}

// NotFutureDate validates that a Date is not in the future.
func (v *Validator) NotFutureDate(field string, date Date) *Validator {
	if !date.IsZero() && date.After(Today()) {
		v.errors.Add(field, "cannot be in the future")
	}
	return v
}

// DateRange validates that a date falls within a range.
func (v *Validator) DateRange(field string, date, min, max Date) *Validator {
	if !date.IsZero() {
		if !min.IsZero() && date.Before(min) {
			v.errors.Add(field, fmt.Sprintf("must be on or after %s", min.String()))
		}
		if !max.IsZero() && date.After(max) {
			v.errors.Add(field, fmt.Sprintf("must be on or before %s", max.String()))
		}
	}
	return v
}

// Percentage validates that a value is between 0 and 100.
func (v *Validator) Percentage(field string, value Money) *Validator {
	zero := ZeroMoney
	hundred := MustNewMoney("100")
	if value.Cmp(zero) < 0 || value.Cmp(hundred) > 0 {
		v.errors.Add(field, "must be between 0 and 100")
	}
	return v
}

// Currency validates that a string is a valid ISO 4217 currency code.
func (v *Validator) Currency(field, value string) *Validator {
	if !IsValidCurrency(value) {
		v.errors.Add(field, "must be a valid ISO 4217 currency code")
	}
	return v
}

// OneOf validates that a string is one of the allowed values.
func (v *Validator) OneOf(field, value string, allowed []string) *Validator {
	if slices.Contains(allowed, value) {
		return v
	}
	v.errors.Add(field, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
	return v
}

// Matches validates that a string matches a regex pattern.
func (v *Validator) Matches(field, value string, pattern *regexp.Regexp, description string) *Validator {
	if !pattern.MatchString(value) {
		v.errors.Add(field, description)
	}
	return v
}

// Custom adds a custom validation with a condition.
func (v *Validator) Custom(field string, condition bool, message string) *Validator {
	if !condition {
		v.errors.Add(field, message)
	}
	return v
}

// ValidateCurrency checks if a string is a valid ISO 4217 currency code.
// Note: This is a subset of common currencies; expand as needed.
var validCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true, "CNY": true,
	"AUD": true, "CAD": true, "CHF": true, "HKD": true, "SGD": true,
	"SEK": true, "KRW": true, "NOK": true, "NZD": true, "INR": true,
	"MXN": true, "TWD": true, "ZAR": true, "BRL": true, "DKK": true,
	"PLN": true, "THB": true, "ILS": true, "IDR": true, "CZK": true,
	"AED": true, "TRY": true, "HUF": true, "CLP": true, "SAR": true,
	"PHP": true, "MYR": true, "COP": true, "RUB": true, "RON": true,
	"PEN": true, "BHD": true, "BGN": true, "ARS": true,
}

// IsValidCurrency checks if a string is a valid ISO 4217 currency code.
func IsValidCurrency(code string) bool {
	return validCurrencies[strings.ToUpper(code)]
}

// ValidateRequiredString validates that a string is not empty and returns an error if it is.
func ValidateRequiredString(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

// ValidateCurrency validates that a string is a valid currency code.
func ValidateCurrency(field, value string) error {
	if !IsValidCurrency(value) {
		return &ValidationError{Field: field, Message: "must be a valid ISO 4217 currency code"}
	}
	return nil
}

// ValidateNotFutureDate validates that a date is not in the future.
func ValidateNotFutureDate(field string, date Date) error {
	if !date.IsZero() && date.After(Today()) {
		return &ValidationError{Field: field, Message: "cannot be in the future"}
	}
	return nil
}

// ValidatePositive validates that a money value is positive.
func ValidatePositive(field string, value Money) error {
	if !value.IsPositive() {
		return &ValidationError{Field: field, Message: "must be positive"}
	}
	return nil
}

// ValidatePercentage validates that a value is between 0 and 100.
func ValidatePercentage(field string, value Money) error {
	zero := ZeroMoney
	hundred := MustNewMoney("100")
	if value.Cmp(zero) < 0 || value.Cmp(hundred) > 0 {
		return &ValidationError{Field: field, Message: "must be between 0 and 100"}
	}
	return nil
}

// BaseModel contains common fields for all models.
type BaseModel struct {
	ID        ID        `json:"id"`
	CreatedAt Timestamp `json:"created_at"`
	UpdatedAt Timestamp `json:"updated_at"`
}

// NewBaseModel creates a new BaseModel with generated ID and current timestamps.
func NewBaseModel() BaseModel {
	now := Now()
	return BaseModel{
		ID:        NewID(),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Touch updates the UpdatedAt timestamp to now.
func (b *BaseModel) Touch() {
	b.UpdatedAt = Now()
}

// TodayAsDate returns today's date in the local timezone.
func TodayAsDate() Date {
	now := time.Now()
	return NewDate(now.Year(), now.Month(), now.Day())
}
