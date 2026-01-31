package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// AccountType represents the type of financial account.
type AccountType string

const (
	AccountTypeChecking   AccountType = "checking"
	AccountTypeSavings    AccountType = "savings"
	AccountTypeCreditCard AccountType = "credit_card"
	AccountTypeInvestment AccountType = "investment"
	AccountTypeCash       AccountType = "cash"
	AccountTypeLoan       AccountType = "loan"
	AccountTypeAsset      AccountType = "asset"
)

// AllAccountTypes returns all valid account types.
func AllAccountTypes() []AccountType {
	return []AccountType{
		AccountTypeChecking,
		AccountTypeSavings,
		AccountTypeCreditCard,
		AccountTypeInvestment,
		AccountTypeCash,
		AccountTypeLoan,
		AccountTypeAsset,
	}
}

// String returns the string representation of the AccountType.
func (at AccountType) String() string {
	return string(at)
}

// IsValid returns true if the AccountType is a valid type.
func (at AccountType) IsValid() bool {
	switch at {
	case AccountTypeChecking, AccountTypeSavings, AccountTypeCreditCard,
		AccountTypeInvestment, AccountTypeCash, AccountTypeLoan, AccountTypeAsset:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the account type.
func (at AccountType) DisplayName() string {
	switch at {
	case AccountTypeChecking:
		return "Checking"
	case AccountTypeSavings:
		return "Savings"
	case AccountTypeCreditCard:
		return "Credit Card"
	case AccountTypeInvestment:
		return "Investment"
	case AccountTypeCash:
		return "Cash"
	case AccountTypeLoan:
		return "Loan"
	case AccountTypeAsset:
		return "Asset"
	default:
		return string(at)
	}
}

// IsAssetType returns true if the account type represents an asset (positive balance = money you have).
func (at AccountType) IsAssetType() bool {
	switch at {
	case AccountTypeChecking, AccountTypeSavings, AccountTypeInvestment,
		AccountTypeCash, AccountTypeAsset:
		return true
	}
	return false
}

// IsLiabilityType returns true if the account type represents a liability (positive balance = money you owe).
func (at AccountType) IsLiabilityType() bool {
	switch at {
	case AccountTypeCreditCard, AccountTypeLoan:
		return true
	}
	return false
}

// ParseAccountType parses a string into an AccountType.
func ParseAccountType(s string) (AccountType, error) {
	at := AccountType(strings.ToLower(s))
	if !at.IsValid() {
		return "", fmt.Errorf("invalid account type: %q", s)
	}
	return at, nil
}

// Value implements the driver.Valuer interface for database storage.
func (at AccountType) Value() (driver.Value, error) {
	return string(at), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (at *AccountType) Scan(value interface{}) error {
	if value == nil {
		*at = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*at = AccountType(v)
	case []byte:
		*at = AccountType(string(v))
	default:
		return fmt.Errorf("unsupported type for AccountType: %T", value)
	}
	return nil
}

// Account represents a financial account.
type Account struct {
	BaseModel

	// Core properties
	Name           string      `json:"name"`
	Type           AccountType `json:"type"`
	Currency       string      `json:"currency"`
	OpeningBalance Money       `json:"opening_balance"`
	OpeningDate    Date        `json:"opening_date"`
	Active         bool        `json:"active"`

	// Optional properties
	Institution   NullableString `json:"institution,omitempty"`
	AccountNumber NullableString `json:"account_number,omitempty"`
	Notes         NullableString `json:"notes,omitempty"`

	// Type-specific optional properties
	CreditLimit  NullableMoney `json:"credit_limit,omitempty"`  // For credit_card accounts
	InterestRate NullableMoney `json:"interest_rate,omitempty"` // For loan accounts (percentage)
}

// NewAccount creates a new Account with generated ID and timestamps.
func NewAccount(name string, accountType AccountType, currency string, openingBalance Money, openingDate Date) *Account {
	return &Account{
		BaseModel:      NewBaseModel(),
		Name:           name,
		Type:           accountType,
		Currency:       currency,
		OpeningBalance: openingBalance,
		OpeningDate:    openingDate,
		Active:         true,
	}
}

// Validate validates the account and returns any validation errors.
func (a *Account) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredString("name", a.Name)
	v.MaxLength("name", a.Name, 255)
	v.RequiredString("currency", a.Currency)
	v.Currency("currency", a.Currency)
	v.RequiredDate("opening_date", a.OpeningDate)
	v.NotFutureDate("opening_date", a.OpeningDate)

	// Account type validation
	if !a.Type.IsValid() {
		v.errors.Add("type", "must be a valid account type")
	}

	// Type-specific validations
	if a.CreditLimit.Valid && !a.CreditLimit.Money.IsPositive() {
		v.errors.Add("credit_limit", "must be positive")
	}

	if a.InterestRate.Valid {
		v.Percentage("interest_rate", a.InterestRate.Money)
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
		a.Institution = NullableString{Valid: false}
	} else {
		a.Institution = NullableString{String: institution, Valid: true}
	}
}

// SetAccountNumber sets the account number.
func (a *Account) SetAccountNumber(number string) {
	if number == "" {
		a.AccountNumber = NullableString{Valid: false}
	} else {
		a.AccountNumber = NullableString{String: number, Valid: true}
	}
}

// SetNotes sets the notes.
func (a *Account) SetNotes(notes string) {
	if notes == "" {
		a.Notes = NullableString{Valid: false}
	} else {
		a.Notes = NullableString{String: notes, Valid: true}
	}
}

// SetCreditLimit sets the credit limit (for credit card accounts).
func (a *Account) SetCreditLimit(limit Money) {
	a.CreditLimit = NullableMoney{Money: limit, Valid: true}
}

// ClearCreditLimit clears the credit limit.
func (a *Account) ClearCreditLimit() {
	a.CreditLimit = NullableMoney{Valid: false}
}

// SetInterestRate sets the interest rate (for loan accounts).
func (a *Account) SetInterestRate(rate Money) {
	a.InterestRate = NullableMoney{Money: rate, Valid: true}
}

// ClearInterestRate clears the interest rate.
func (a *Account) ClearInterestRate() {
	a.InterestRate = NullableMoney{Valid: false}
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
