package price

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// Source represents how a security price was entered.
type Source string

const (
	SourceManual      Source = "manual"
	SourceTransaction Source = "transaction"
	SourceImport      Source = "import"
	SourceAPI         Source = "api"
)

// AllSources returns all valid price sources.
func AllSources() []Source {
	return []Source{
		SourceManual,
		SourceTransaction,
		SourceImport,
		SourceAPI,
	}
}

// String returns the string representation of the Source.
func (ps Source) String() string {
	return string(ps)
}

// IsValid returns true if the Source is a valid source.
func (ps Source) IsValid() bool {
	switch ps {
	case SourceManual, SourceTransaction, SourceImport, SourceAPI:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the price source.
func (ps Source) DisplayName() string {
	switch ps {
	case SourceManual:
		return "Manual"
	case SourceTransaction:
		return "Transaction"
	case SourceImport:
		return "Import"
	case SourceAPI:
		return "API"
	default:
		return string(ps)
	}
}

// ParseSource parses a string into a Source.
func ParseSource(s string) (Source, error) {
	ps := Source(strings.ToLower(s))
	if !ps.IsValid() {
		return "", fmt.Errorf("invalid price source: %q", s)
	}
	return ps, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ps Source) Value() (driver.Value, error) {
	return string(ps), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ps *Source) Scan(value any) error {
	if value == nil {
		*ps = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ps = Source(v)
	case []byte:
		*ps = Source(string(v))
	default:
		return fmt.Errorf("unsupported type for Source: %T", value)
	}
	return nil
}

// Price represents a price entry for a security on a given date.
type Price struct {
	types.BaseModel

	SecurityID types.ID    `json:"security_id"`
	Date       types.Date  `json:"date"`
	Price      types.Money `json:"price"`
	Source     Source      `json:"source"`
}

// NewPrice creates a new Price with required fields.
func NewPrice(securityID types.ID, date types.Date, price types.Money, source Source) *Price {
	return &Price{
		BaseModel:  types.NewBaseModel(),
		SecurityID: securityID,
		Date:       date,
		Price:      price,
		Source:     source,
	}
}

// Validate validates the Price and returns any validation errors.
func (sp *Price) Validate() types.ValidationErrors {
	v := types.NewValidator()

	v.RequiredID("security_id", sp.SecurityID)
	v.RequiredDate("date", sp.Date)
	v.NotFutureDate("date", sp.Date)

	if sp.Price.IsZero() || !sp.Price.IsPositive() {
		v.AddError("price", "must be positive")
	}

	if !sp.Source.IsValid() {
		v.AddError("source", "must be a valid price source")
	}

	return v.Errors()
}

// IsValid returns true if the Price passes all validation rules.
func (sp *Price) IsValid() bool {
	return !sp.Validate().HasErrors()
}
