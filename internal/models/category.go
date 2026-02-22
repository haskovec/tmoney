package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// CategoryType represents whether a category is for income or expenses.
type CategoryType string

const (
	CategoryTypeIncome  CategoryType = "income"
	CategoryTypeExpense CategoryType = "expense"
)

// AllCategoryTypes returns all valid category types.
func AllCategoryTypes() []CategoryType {
	return []CategoryType{
		CategoryTypeIncome,
		CategoryTypeExpense,
	}
}

// String returns the string representation of the CategoryType.
func (ct CategoryType) String() string {
	return string(ct)
}

// IsValid returns true if the CategoryType is a valid type.
func (ct CategoryType) IsValid() bool {
	switch ct {
	case CategoryTypeIncome, CategoryTypeExpense:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the category type.
func (ct CategoryType) DisplayName() string {
	switch ct {
	case CategoryTypeIncome:
		return "Income"
	case CategoryTypeExpense:
		return "Expense"
	default:
		return string(ct)
	}
}

// ParseCategoryType parses a string into a CategoryType.
func ParseCategoryType(s string) (CategoryType, error) {
	ct := CategoryType(strings.ToLower(s))
	if !ct.IsValid() {
		return "", fmt.Errorf("invalid category type: %q", s)
	}
	return ct, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ct CategoryType) Value() (driver.Value, error) {
	return string(ct), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ct *CategoryType) Scan(value any) error {
	if value == nil {
		*ct = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ct = CategoryType(v)
	case []byte:
		*ct = CategoryType(string(v))
	default:
		return fmt.Errorf("unsupported type for CategoryType: %T", value)
	}
	return nil
}

// Category represents a transaction category.
type Category struct {
	BaseModel

	// Core properties
	Name     string       `json:"name"`
	ParentID NullableID   `json:"parent_id"`
	Type     CategoryType `json:"type"`
	IsSystem bool         `json:"is_system"`
}

// NewCategory creates a new top-level Category with generated ID and timestamps.
func NewCategory(name string, categoryType CategoryType) *Category {
	return &Category{
		BaseModel: NewBaseModel(),
		Name:      name,
		ParentID:  NullableID{Valid: false},
		Type:      categoryType,
		IsSystem:  false,
	}
}

// NewSubcategory creates a new subcategory under a parent Category.
// The subcategory inherits the type from the parent.
func NewSubcategory(name string, parentID ID, categoryType CategoryType) *Category {
	return &Category{
		BaseModel: NewBaseModel(),
		Name:      name,
		ParentID:  NullableID{ID: parentID, Valid: true},
		Type:      categoryType,
		IsSystem:  false,
	}
}

// NewSystemCategory creates a new system-managed Category (like Transfer).
func NewSystemCategory(name string, categoryType CategoryType) *Category {
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
func (c *Category) SetParent(parentID ID) {
	c.ParentID = NullableID{ID: parentID, Valid: true}
	c.Touch()
}

// ClearParent removes the parent category, making this a top-level category.
func (c *Category) ClearParent() {
	c.ParentID = NullableID{Valid: false}
	c.Touch()
}

// Validate validates the category and returns any validation errors.
func (c *Category) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredString("name", c.Name)
	v.MaxLength("name", c.Name, 255)

	// Category type validation
	if !c.Type.IsValid() {
		v.errors.Add("type", "must be a valid category type (income or expense)")
	}

	return v.Errors()
}

// IsValid returns true if the category passes validation.
func (c *Category) IsValid() bool {
	return !c.Validate().HasErrors()
}
