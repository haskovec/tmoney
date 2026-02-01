package models

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
)

// MatchType represents the type of pattern matching for payee aliases.
type MatchType string

const (
	MatchTypeExact      MatchType = "exact"
	MatchTypeContains   MatchType = "contains"
	MatchTypeStartsWith MatchType = "starts_with"
	MatchTypeRegex      MatchType = "regex"
)

// AllMatchTypes returns all valid match types.
func AllMatchTypes() []MatchType {
	return []MatchType{
		MatchTypeExact,
		MatchTypeContains,
		MatchTypeStartsWith,
		MatchTypeRegex,
	}
}

// String returns the string representation of the MatchType.
func (mt MatchType) String() string {
	return string(mt)
}

// IsValid returns true if the MatchType is a valid type.
func (mt MatchType) IsValid() bool {
	switch mt {
	case MatchTypeExact, MatchTypeContains, MatchTypeStartsWith, MatchTypeRegex:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the match type.
func (mt MatchType) DisplayName() string {
	switch mt {
	case MatchTypeExact:
		return "Exact"
	case MatchTypeContains:
		return "Contains"
	case MatchTypeStartsWith:
		return "Starts With"
	case MatchTypeRegex:
		return "Regular Expression"
	default:
		return string(mt)
	}
}

// ParseMatchType parses a string into a MatchType.
func ParseMatchType(s string) (MatchType, error) {
	mt := MatchType(strings.ToLower(s))
	if !mt.IsValid() {
		return "", fmt.Errorf("invalid match type: %q", s)
	}
	return mt, nil
}

// Value implements the driver.Valuer interface for database storage.
func (mt MatchType) Value() (driver.Value, error) {
	return string(mt), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (mt *MatchType) Scan(value interface{}) error {
	if value == nil {
		*mt = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*mt = MatchType(v)
	case []byte:
		*mt = MatchType(string(v))
	default:
		return fmt.Errorf("unsupported type for MatchType: %T", value)
	}
	return nil
}

// Payee represents an entity you pay money to or receive money from.
type Payee struct {
	BaseModel

	// Core properties
	Name              string     `json:"name"`
	DefaultCategoryID NullableID `json:"default_category_id,omitempty"`
	Notes             NullableString `json:"notes,omitempty"`
}

// NewPayee creates a new Payee with generated ID and timestamps.
func NewPayee(name string) *Payee {
	return &Payee{
		BaseModel:         NewBaseModel(),
		Name:              name,
		DefaultCategoryID: NullableID{Valid: false},
		Notes:             NullableString{Valid: false},
	}
}

// NewPayeeWithCategory creates a new Payee with a default category.
func NewPayeeWithCategory(name string, categoryID ID) *Payee {
	return &Payee{
		BaseModel:         NewBaseModel(),
		Name:              name,
		DefaultCategoryID: NullableID{ID: categoryID, Valid: true},
		Notes:             NullableString{Valid: false},
	}
}

// SetDefaultCategory sets the default category for this payee.
func (p *Payee) SetDefaultCategory(categoryID ID) {
	p.DefaultCategoryID = NullableID{ID: categoryID, Valid: true}
	p.Touch()
}

// ClearDefaultCategory removes the default category.
func (p *Payee) ClearDefaultCategory() {
	p.DefaultCategoryID = NullableID{Valid: false}
	p.Touch()
}

// HasDefaultCategory returns true if the payee has a default category set.
func (p *Payee) HasDefaultCategory() bool {
	return p.DefaultCategoryID.Valid
}

// SetNotes sets the notes for this payee.
func (p *Payee) SetNotes(notes string) {
	if notes == "" {
		p.Notes = NullableString{Valid: false}
	} else {
		p.Notes = NullableString{String: notes, Valid: true}
	}
	p.Touch()
}

// ClearNotes removes the notes.
func (p *Payee) ClearNotes() {
	p.Notes = NullableString{Valid: false}
	p.Touch()
}

// Validate validates the payee and returns any validation errors.
func (p *Payee) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredString("name", p.Name)
	v.MaxLength("name", p.Name, 255)

	// Optional field length limits
	if p.Notes.Valid {
		v.MaxLength("notes", p.Notes.String, 2000)
	}

	return v.Errors()
}

// IsValid returns true if the payee passes validation.
func (p *Payee) IsValid() bool {
	return !p.Validate().HasErrors()
}

// Alias represents a pattern that maps to a payee.
// Aliases enable imported transactions to be automatically matched.
type Alias struct {
	BaseModel

	// Core properties
	PayeeID   ID        `json:"payee_id"`
	Pattern   string    `json:"pattern"`
	MatchType MatchType `json:"match_type"`
}

// NewAlias creates a new Alias with generated ID and timestamps.
func NewAlias(payeeID ID, pattern string, matchType MatchType) *Alias {
	return &Alias{
		BaseModel: NewBaseModel(),
		PayeeID:   payeeID,
		Pattern:   pattern,
		MatchType: matchType,
	}
}

// NewExactAlias creates a new exact-match Alias.
func NewExactAlias(payeeID ID, pattern string) *Alias {
	return NewAlias(payeeID, pattern, MatchTypeExact)
}

// NewContainsAlias creates a new contains-match Alias.
func NewContainsAlias(payeeID ID, pattern string) *Alias {
	return NewAlias(payeeID, pattern, MatchTypeContains)
}

// NewStartsWithAlias creates a new starts-with-match Alias.
func NewStartsWithAlias(payeeID ID, pattern string) *Alias {
	return NewAlias(payeeID, pattern, MatchTypeStartsWith)
}

// NewRegexAlias creates a new regex-match Alias.
func NewRegexAlias(payeeID ID, pattern string) *Alias {
	return NewAlias(payeeID, pattern, MatchTypeRegex)
}

// Matches tests whether the given input string matches this alias pattern.
func (a *Alias) Matches(input string) bool {
	switch a.MatchType {
	case MatchTypeExact:
		return input == a.Pattern
	case MatchTypeContains:
		return strings.Contains(input, a.Pattern)
	case MatchTypeStartsWith:
		return strings.HasPrefix(input, a.Pattern)
	case MatchTypeRegex:
		re, err := regexp.Compile(a.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(input)
	default:
		return false
	}
}

// MatchesCaseInsensitive tests whether the given input string matches this alias pattern,
// ignoring case. For regex patterns, this adds the (?i) flag.
func (a *Alias) MatchesCaseInsensitive(input string) bool {
	lowerInput := strings.ToLower(input)
	lowerPattern := strings.ToLower(a.Pattern)

	switch a.MatchType {
	case MatchTypeExact:
		return lowerInput == lowerPattern
	case MatchTypeContains:
		return strings.Contains(lowerInput, lowerPattern)
	case MatchTypeStartsWith:
		return strings.HasPrefix(lowerInput, lowerPattern)
	case MatchTypeRegex:
		re, err := regexp.Compile("(?i)" + a.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(input)
	default:
		return false
	}
}

// Validate validates the alias and returns any validation errors.
func (a *Alias) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredID("payee_id", a.PayeeID)
	v.RequiredString("pattern", a.Pattern)
	v.MaxLength("pattern", a.Pattern, 500)

	// Match type validation
	if !a.MatchType.IsValid() {
		v.errors.Add("match_type", "must be a valid match type (exact, contains, starts_with, or regex)")
	}

	// Regex pattern validation
	if a.MatchType == MatchTypeRegex && a.Pattern != "" {
		_, err := regexp.Compile(a.Pattern)
		if err != nil {
			v.errors.Add("pattern", fmt.Sprintf("invalid regular expression: %v", err))
		}
	}

	return v.Errors()
}

// IsValid returns true if the alias passes validation.
func (a *Alias) IsValid() bool {
	return !a.Validate().HasErrors()
}
