package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// PriceSource represents how a security price was entered.
type PriceSource string

const (
	PriceSourceManual      PriceSource = "manual"
	PriceSourceTransaction PriceSource = "transaction"
	PriceSourceImport      PriceSource = "import"
	PriceSourceAPI         PriceSource = "api"
)

// AllPriceSources returns all valid price sources.
func AllPriceSources() []PriceSource {
	return []PriceSource{
		PriceSourceManual,
		PriceSourceTransaction,
		PriceSourceImport,
		PriceSourceAPI,
	}
}

// String returns the string representation of the PriceSource.
func (ps PriceSource) String() string {
	return string(ps)
}

// IsValid returns true if the PriceSource is a valid source.
func (ps PriceSource) IsValid() bool {
	switch ps {
	case PriceSourceManual, PriceSourceTransaction, PriceSourceImport, PriceSourceAPI:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the price source.
func (ps PriceSource) DisplayName() string {
	switch ps {
	case PriceSourceManual:
		return "Manual"
	case PriceSourceTransaction:
		return "Transaction"
	case PriceSourceImport:
		return "Import"
	case PriceSourceAPI:
		return "API"
	default:
		return string(ps)
	}
}

// ParsePriceSource parses a string into a PriceSource.
func ParsePriceSource(s string) (PriceSource, error) {
	ps := PriceSource(strings.ToLower(s))
	if !ps.IsValid() {
		return "", fmt.Errorf("invalid price source: %q", s)
	}
	return ps, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ps PriceSource) Value() (driver.Value, error) {
	return string(ps), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ps *PriceSource) Scan(value any) error {
	if value == nil {
		*ps = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ps = PriceSource(v)
	case []byte:
		*ps = PriceSource(string(v))
	default:
		return fmt.Errorf("unsupported type for PriceSource: %T", value)
	}
	return nil
}

// SecurityPrice represents a price entry for a security on a given date.
type SecurityPrice struct {
	BaseModel

	SecurityID ID          `json:"security_id"`
	Date       Date        `json:"date"`
	Price      Money       `json:"price"`
	Source     PriceSource `json:"source"`
}

// NewSecurityPrice creates a new SecurityPrice with required fields.
func NewSecurityPrice(securityID ID, date Date, price Money, source PriceSource) *SecurityPrice {
	return &SecurityPrice{
		BaseModel:  NewBaseModel(),
		SecurityID: securityID,
		Date:       date,
		Price:      price,
		Source:     source,
	}
}

// Validate validates the SecurityPrice and returns any validation errors.
func (sp *SecurityPrice) Validate() ValidationErrors {
	v := NewValidator()

	v.RequiredID("security_id", sp.SecurityID)
	v.RequiredDate("date", sp.Date)
	v.NotFutureDate("date", sp.Date)

	if sp.Price.IsZero() || !sp.Price.IsPositive() {
		v.errors.Add("price", "must be positive")
	}

	if !sp.Source.IsValid() {
		v.errors.Add("source", "must be a valid price source")
	}

	return v.Errors()
}

// IsValid returns true if the SecurityPrice passes all validation rules.
func (sp *SecurityPrice) IsValid() bool {
	return !sp.Validate().HasErrors()
}
