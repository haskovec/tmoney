package category

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// Type represents whether a category is for income or expenses.
type Type string

const (
	TypeIncome  Type = "income"
	TypeExpense Type = "expense"
)

// AllTypes returns all valid category types.
func AllTypes() []Type {
	return []Type{
		TypeIncome,
		TypeExpense,
	}
}

// String returns the string representation of the Type.
func (ct Type) String() string {
	return string(ct)
}

// IsValid returns true if the Type is a valid type.
func (ct Type) IsValid() bool {
	switch ct {
	case TypeIncome, TypeExpense:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the category type.
func (ct Type) DisplayName() string {
	switch ct {
	case TypeIncome:
		return "Income"
	case TypeExpense:
		return "Expense"
	default:
		return string(ct)
	}
}

// ParseType parses a string into a Type.
func ParseType(s string) (Type, error) {
	ct := Type(strings.ToLower(s))
	if !ct.IsValid() {
		return "", fmt.Errorf("invalid category type: %q", s)
	}
	return ct, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ct Type) Value() (driver.Value, error) {
	return string(ct), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ct *Type) Scan(value any) error {
	if value == nil {
		*ct = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ct = Type(v)
	case []byte:
		*ct = Type(string(v))
	default:
		return fmt.Errorf("unsupported type for Type: %T", value)
	}
	return nil
}

// System category names. These are stable identifiers for the
// engine-managed categories; code that special-cases a system category
// (seeding, picker visibility, transfer assignment) keys off these
// rather than repeating the literal.
const (
	// TransferCategoryName is the system category assigned to the
	// engine-managed side of a linked transfer.
	TransferCategoryName = "Transfer"
	// ValueAdjustmentCategoryName is the system category used for asset
	// revaluations (home value updates, straight-line depreciation).
	// Like Transfer it is excluded from spending reports; unlike
	// Transfer it is offered in the category picker, but only for
	// asset-type accounts.
	ValueAdjustmentCategoryName = "Value Adjustment"
)

// Default loan-category names. Unlike the system categories above these are
// ordinary user categories (renamable/mergeable), but their names are stable
// identifiers the loan wizard and `loan add` get-or-create at save time so the
// default interest category is always available. See
// GetOrCreateLoanInterestCategory.
const (
	// LoanCategoryName is the parent of the default loan-interest category.
	LoanCategoryName = "Loan"
	// LoanInterestChildName is the child (under LoanCategoryName) that a loan
	// payment's interest portion is booked to by default.
	LoanInterestChildName = "Interest"
)

// Category represents a transaction category.
type Category struct {
	types.BaseModel

	// Core properties
	Name     string           `json:"name"`
	ParentID types.NullableID `json:"parent_id"`
	Type     Type             `json:"type"`
	IsSystem bool             `json:"is_system"`
}

// NewCategory creates a new top-level Category with generated ID and timestamps.
func NewCategory(name string, categoryType Type) *Category {
	return &Category{
		BaseModel: types.NewBaseModel(),
		Name:      name,
		ParentID:  types.NullableID{Valid: false},
		Type:      categoryType,
		IsSystem:  false,
	}
}

// NewSubcategory creates a new subcategory under a parent Category.
// The subcategory inherits the type from the parent.
func NewSubcategory(name string, parentID types.ID, categoryType Type) *Category {
	return &Category{
		BaseModel: types.NewBaseModel(),
		Name:      name,
		ParentID:  types.NullableID{ID: parentID, Valid: true},
		Type:      categoryType,
		IsSystem:  false,
	}
}

// NewSystemCategory creates a new system-managed Category (like Transfer).
func NewSystemCategory(name string, categoryType Type) *Category {
	c := NewCategory(name, categoryType)
	c.IsSystem = true
	return c
}

// IsTopLevel returns true if the category has no parent.
func (c *Category) IsTopLevel() bool {
	return !c.ParentID.Valid
}

// IsSubcategory returns true if the category has a parent.
func (c *Category) IsSubcategory() bool {
	return c.ParentID.Valid
}

// SetParent sets the parent category ID.
func (c *Category) SetParent(parentID types.ID) {
	c.ParentID = types.NullableID{ID: parentID, Valid: true}
	c.Touch()
}

// ClearParent removes the parent category, making this a top-level category.
func (c *Category) ClearParent() {
	c.ParentID = types.NullableID{Valid: false}
	c.Touch()
}

// Validate validates the category and returns any validation errors.
func (c *Category) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredString("name", c.Name)
	v.MaxLength("name", c.Name, 255)

	// Category type validation
	if !c.Type.IsValid() {
		v.AddError("type", "must be a valid category type (income or expense)")
	}

	return v.Errors()
}

// IsValid returns true if the category passes validation.
func (c *Category) IsValid() bool {
	return !c.Validate().HasErrors()
}
