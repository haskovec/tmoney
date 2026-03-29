package account

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// Type represents the type of financial account.
type Type string

const (
	TypeChecking   Type = "checking"
	TypeSavings    Type = "savings"
	TypeCreditCard Type = "credit_card"
	TypeInvestment Type = "investment"
	TypeCash       Type = "cash"
	TypeLoan       Type = "loan"
	TypeAsset      Type = "asset"
)

// AllTypes returns all valid account types.
func AllTypes() []Type {
	return []Type{
		TypeChecking,
		TypeSavings,
		TypeCreditCard,
		TypeInvestment,
		TypeCash,
		TypeLoan,
		TypeAsset,
	}
}

// String returns the string representation of the Type.
func (at Type) String() string {
	return string(at)
}

// IsValid returns true if the Type is a valid type.
func (at Type) IsValid() bool {
	switch at {
	case TypeChecking, TypeSavings, TypeCreditCard,
		TypeInvestment, TypeCash, TypeLoan, TypeAsset:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the account type.
func (at Type) DisplayName() string {
	switch at {
	case TypeChecking:
		return "Checking"
	case TypeSavings:
		return "Savings"
	case TypeCreditCard:
		return "Credit Card"
	case TypeInvestment:
		return "Investment"
	case TypeCash:
		return "Cash"
	case TypeLoan:
		return "Loan"
	case TypeAsset:
		return "Asset"
	default:
		return string(at)
	}
}

// IsAssetType returns true if the account type represents an asset (positive balance = money you have).
func (at Type) IsAssetType() bool {
	switch at {
	case TypeChecking, TypeSavings, TypeInvestment,
		TypeCash, TypeAsset:
		return true
	}
	return false
}

// IsLiabilityType returns true if the account type represents a liability (positive balance = money you owe).
func (at Type) IsLiabilityType() bool {
	switch at {
	case TypeCreditCard, TypeLoan:
		return true
	}
	return false
}

// ParseType parses a string into a Type.
func ParseType(s string) (Type, error) {
	at := Type(strings.ToLower(s))
	if !at.IsValid() {
		return "", fmt.Errorf("invalid account type: %q", s)
	}
	return at, nil
}

// Value implements the driver.Valuer interface for database storage.
func (at Type) Value() (driver.Value, error) {
	return string(at), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (at *Type) Scan(value any) error {
	if value == nil {
		*at = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*at = Type(v)
	case []byte:
		*at = Type(string(v))
	default:
		return fmt.Errorf("unsupported type for Type: %T", value)
	}
	return nil
}

// Account represents a financial account.
type Account struct {
	types.BaseModel

	// Core properties
	Name           string      `json:"name"`
	Type           Type        `json:"type"`
	Currency       string      `json:"currency"`
	OpeningBalance types.Money `json:"opening_balance"`
	OpeningDate    types.Date  `json:"opening_date"`
	Active         bool        `json:"active"`

	// Optional properties
	Institution   types.NullableString `json:"institution"`
	AccountNumber types.NullableString `json:"account_number"`
	Notes         types.NullableString `json:"notes"`

	// Type-specific optional properties
	CreditLimit  types.NullableMoney `json:"credit_limit"`  // For credit_card accounts
	InterestRate types.NullableMoney `json:"interest_rate"` // For loan accounts (percentage)
	TrackLots    bool                `json:"track_lots"`    // For investment accounts (lot-based cost tracking)
}

// NewAccount creates a new Account with generated ID and timestamps.
func NewAccount(name string, accountType Type, currency string, openingBalance types.Money, openingDate types.Date) *Account {
	return &Account{
		BaseModel:      types.NewBaseModel(),
		Name:           name,
		Type:           accountType,
		Currency:       currency,
		OpeningBalance: openingBalance,
		OpeningDate:    openingDate,
		Active:         true,
	}
}

// Validate validates the account and returns any validation errors.
func (a *Account) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredString("name", a.Name)
	v.MaxLength("name", a.Name, 255)
	v.RequiredString("currency", a.Currency)
	v.Currency("currency", a.Currency)
	v.RequiredDate("opening_date", a.OpeningDate)
	v.NotFutureDate("opening_date", a.OpeningDate)

	// Account type validation
	if !a.Type.IsValid() {
		v.AddError("type", "must be a valid account type")
	}

	// Type-specific validations
	if a.CreditLimit.Valid && !a.CreditLimit.Money.IsPositive() {
		v.AddError("credit_limit", "must be positive")
	}

	if a.InterestRate.Valid {
		v.Percentage("interest_rate", a.InterestRate.Money)
	}

	if a.TrackLots && a.Type != TypeInvestment {
		v.AddError("track_lots", "can only be enabled for investment accounts")
	}

	// Optional field length limits
	if a.Institution.Valid {
		v.MaxLength("institution", a.Institution.String, 255)
	}
	if a.AccountNumber.Valid {
		v.MaxLength("account_number", a.AccountNumber.String, 50)
	}
	if a.Notes.Valid {
		v.MaxLength("notes", a.Notes.String, 2000)
	}

	return v.Errors()
}

// IsValid returns true if the account passes validation.
func (a *Account) IsValid() bool {
	return !a.Validate().HasErrors()
}

// SetInstitution sets the institution name.
func (a *Account) SetInstitution(institution string) {
	if institution == "" {
		a.Institution = types.NullableString{Valid: false}
	} else {
		a.Institution = types.NullableString{String: institution, Valid: true}
	}
}

// SetAccountNumber sets the account number.
func (a *Account) SetAccountNumber(number string) {
	if number == "" {
		a.AccountNumber = types.NullableString{Valid: false}
	} else {
		a.AccountNumber = types.NullableString{String: number, Valid: true}
	}
}

// SetNotes sets the notes.
func (a *Account) SetNotes(notes string) {
	if notes == "" {
		a.Notes = types.NullableString{Valid: false}
	} else {
		a.Notes = types.NullableString{String: notes, Valid: true}
	}
}

// SetCreditLimit sets the credit limit (for credit card accounts).
func (a *Account) SetCreditLimit(limit types.Money) {
	a.CreditLimit = types.NullableMoney{Money: limit, Valid: true}
}

// ClearCreditLimit clears the credit limit.
func (a *Account) ClearCreditLimit() {
	a.CreditLimit = types.NullableMoney{Valid: false}
}

// SetInterestRate sets the interest rate (for loan accounts).
func (a *Account) SetInterestRate(rate types.Money) {
	a.InterestRate = types.NullableMoney{Money: rate, Valid: true}
}

// ClearInterestRate clears the interest rate.
func (a *Account) ClearInterestRate() {
	a.InterestRate = types.NullableMoney{Valid: false}
}

// Close marks the account as inactive (closed).
func (a *Account) Close() {
	a.Active = false
	a.Touch()
}

// Reopen marks the account as active again.
func (a *Account) Reopen() {
	a.Active = true
	a.Touch()
}
