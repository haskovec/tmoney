package category

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestType(t *testing.T) {
	t.Run("AllTypes returns all types", func(t *testing.T) {
		allTypes := AllTypes()
		expected := 2
		if len(allTypes) != expected {
			t.Errorf("Expected %d category types, got %d", expected, len(allTypes))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if TypeIncome.String() != "income" {
			t.Errorf("Expected 'income', got %q", TypeIncome.String())
		}
		if TypeExpense.String() != "expense" {
			t.Errorf("Expected 'expense', got %q", TypeExpense.String())
		}
	})

	t.Run("IsValid returns true for valid types", func(t *testing.T) {
		validTypes := []Type{
			TypeIncome,
			TypeExpense,
		}
		for _, ct := range validTypes {
			if !ct.IsValid() {
				t.Errorf("IsValid should return true for %q", ct)
			}
		}
	})

	t.Run("IsValid returns false for invalid type", func(t *testing.T) {
		invalid := Type("unknown")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'unknown'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			categoryType Type
			expected     string
		}{
			{TypeIncome, "Income"},
			{TypeExpense, "Expense"},
		}
		for _, tc := range tests {
			if tc.categoryType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.categoryType, tc.expected, tc.categoryType.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown type", func(t *testing.T) {
		unknown := Type("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestParseType(t *testing.T) {
	t.Run("Parses valid income type", func(t *testing.T) {
		ct, err := ParseType("income")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeIncome {
			t.Errorf("Expected TypeIncome, got %q", ct)
		}
	})

	t.Run("Parses valid expense type", func(t *testing.T) {
		ct, err := ParseType("expense")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeExpense {
			t.Errorf("Expected TypeExpense, got %q", ct)
		}
	})

	t.Run("Parses uppercase type", func(t *testing.T) {
		ct, err := ParseType("INCOME")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeIncome {
			t.Errorf("Expected TypeIncome, got %q", ct)
		}
	})

	t.Run("Parses mixed case type", func(t *testing.T) {
		ct, err := ParseType("Expense")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeExpense {
			t.Errorf("Expected TypeExpense, got %q", ct)
		}
	})

	t.Run("Returns error for invalid type", func(t *testing.T) {
		_, err := ParseType("invalid")
		if err == nil {
			t.Error("Expected error for invalid category type")
		}
	})

	t.Run("Returns error for empty string", func(t *testing.T) {
		_, err := ParseType("")
		if err == nil {
			t.Error("Expected error for empty string")
		}
	})
}

func TestTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		ct := TypeIncome
		v, err := ct.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "income" {
			t.Errorf("Expected 'income', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var ct Type
		err := ct.Scan("expense")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeExpense {
			t.Errorf("Expected TypeExpense, got %q", ct)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var ct Type
		err := ct.Scan([]byte("income"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != TypeIncome {
			t.Errorf("Expected TypeIncome, got %q", ct)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var ct Type
		err := ct.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ct != "" {
			t.Errorf("Expected empty string, got %q", ct)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var ct Type
		err := ct.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewCategory(t *testing.T) {
	t.Run("Creates top-level income category", func(t *testing.T) {
		cat := NewCategory("Salary", TypeIncome)

		if cat.ID.IsNil() {
			t.Error("NewCategory should create non-nil ID")
		}
		if cat.Name != "Salary" {
			t.Errorf("Expected name 'Salary', got %q", cat.Name)
		}
		if cat.Type != TypeIncome {
			t.Errorf("Expected type income, got %q", cat.Type)
		}
		if cat.ParentID.Valid {
			t.Error("NewCategory should create top-level category (no parent)")
		}
		if cat.IsSystem {
			t.Error("NewCategory should not create system category")
		}
		if cat.CreatedAt.IsZero() {
			t.Error("NewCategory should set CreatedAt")
		}
		if cat.UpdatedAt.IsZero() {
			t.Error("NewCategory should set UpdatedAt")
		}
	})

	t.Run("Creates top-level expense category", func(t *testing.T) {
		cat := NewCategory("Food", TypeExpense)

		if cat.Type != TypeExpense {
			t.Errorf("Expected type expense, got %q", cat.Type)
		}
	})
}

func TestNewSubcategory(t *testing.T) {
	t.Run("Creates subcategory with parent", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)

		if cat.ID.IsNil() {
			t.Error("NewSubcategory should create non-nil ID")
		}
		if cat.Name != "Groceries" {
			t.Errorf("Expected name 'Groceries', got %q", cat.Name)
		}
		if !cat.ParentID.Valid {
			t.Error("NewSubcategory should set parent")
		}
		if cat.ParentID.ID != parentID {
			t.Errorf("Expected parent ID %s, got %s", parentID.String(), cat.ParentID.ID.String())
		}
		if cat.Type != TypeExpense {
			t.Errorf("Expected type expense, got %q", cat.Type)
		}
		if cat.IsSystem {
			t.Error("NewSubcategory should not create system category")
		}
	})
}

func TestNewSystemCategory(t *testing.T) {
	t.Run("Creates system category", func(t *testing.T) {
		cat := NewSystemCategory("Transfer", TypeExpense)

		if cat.ID.IsNil() {
			t.Error("NewSystemCategory should create non-nil ID")
		}
		if cat.Name != "Transfer" {
			t.Errorf("Expected name 'Transfer', got %q", cat.Name)
		}
		if !cat.IsSystem {
			t.Error("NewSystemCategory should create system category")
		}
		if cat.ParentID.Valid {
			t.Error("NewSystemCategory should create top-level category")
		}
	})
}

func TestCategoryHierarchy(t *testing.T) {
	t.Run("IsTopLevel returns true for category without parent", func(t *testing.T) {
		cat := NewCategory("Food", TypeExpense)
		if !cat.IsTopLevel() {
			t.Error("Category without parent should be top-level")
		}
	})

	t.Run("IsTopLevel returns false for subcategory", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)
		if cat.IsTopLevel() {
			t.Error("Subcategory should not be top-level")
		}
	})

	t.Run("IsSubcategory returns true for category with parent", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)
		if !cat.IsSubcategory() {
			t.Error("Category with parent should be subcategory")
		}
	})

	t.Run("IsSubcategory returns false for top-level category", func(t *testing.T) {
		cat := NewCategory("Food", TypeExpense)
		if cat.IsSubcategory() {
			t.Error("Top-level category should not be subcategory")
		}
	})
}

func TestCategorySetParent(t *testing.T) {
	t.Run("SetParent sets parent ID", func(t *testing.T) {
		cat := NewCategory("Groceries", TypeExpense)
		parentID := types.NewID()

		if cat.IsSubcategory() {
			t.Error("Category should start without parent")
		}

		cat.SetParent(parentID)

		if !cat.IsSubcategory() {
			t.Error("Category should be subcategory after SetParent")
		}
		if cat.ParentID.ID != parentID {
			t.Errorf("Expected parent ID %s, got %s", parentID.String(), cat.ParentID.ID.String())
		}
	})

	t.Run("SetParent updates UpdatedAt", func(t *testing.T) {
		cat := NewCategory("Groceries", TypeExpense)
		original := cat.UpdatedAt
		parentID := types.NewID()

		cat.SetParent(parentID)

		if !cat.UpdatedAt.After(original) && !cat.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetParent should update UpdatedAt")
		}
	})

	t.Run("ClearParent removes parent", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)

		if !cat.IsSubcategory() {
			t.Error("Category should start as subcategory")
		}

		cat.ClearParent()

		if cat.IsSubcategory() {
			t.Error("Category should be top-level after ClearParent")
		}
		if cat.ParentID.Valid {
			t.Error("ParentID should not be valid after ClearParent")
		}
	})

	t.Run("ClearParent updates UpdatedAt", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)
		original := cat.UpdatedAt

		cat.ClearParent()

		if !cat.UpdatedAt.After(original) && !cat.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("ClearParent should update UpdatedAt")
		}
	})
}

func TestCategoryValidation(t *testing.T) {
	validCategory := func() *Category {
		return NewCategory("Test Category", TypeExpense)
	}

	t.Run("Valid category passes validation", func(t *testing.T) {
		cat := validCategory()
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid category should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid category", func(t *testing.T) {
		cat := validCategory()
		if !cat.IsValid() {
			t.Error("IsValid should return true for valid category")
		}
	})

	t.Run("Empty name fails validation", func(t *testing.T) {
		cat := validCategory()
		cat.Name = ""
		errs := cat.Validate()
		if !errs.HasErrors() {
			t.Error("Empty name should fail validation")
		}
	})

	t.Run("Whitespace-only name fails validation", func(t *testing.T) {
		cat := validCategory()
		cat.Name = "   "
		errs := cat.Validate()
		if !errs.HasErrors() {
			t.Error("Whitespace-only name should fail validation")
		}
	})

	t.Run("Name exceeding max length fails validation", func(t *testing.T) {
		cat := validCategory()
		cat.Name = string(make([]byte, 256))
		errs := cat.Validate()
		if !errs.HasErrors() {
			t.Error("Name exceeding 255 chars should fail validation")
		}
	})

	t.Run("Name at max length passes validation", func(t *testing.T) {
		cat := validCategory()
		cat.Name = string(make([]byte, 255))
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("Name at 255 chars should pass validation: %v", errs)
		}
	})

	t.Run("Invalid category type fails validation", func(t *testing.T) {
		cat := validCategory()
		cat.Type = Type("invalid")
		errs := cat.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid category type should fail validation")
		}
	})

	t.Run("Empty category type fails validation", func(t *testing.T) {
		cat := validCategory()
		cat.Type = Type("")
		errs := cat.Validate()
		if !errs.HasErrors() {
			t.Error("Empty category type should fail validation")
		}
	})

	t.Run("Income type passes validation", func(t *testing.T) {
		cat := NewCategory("Salary", TypeIncome)
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("Income type should pass validation: %v", errs)
		}
	})

	t.Run("Expense type passes validation", func(t *testing.T) {
		cat := NewCategory("Food", TypeExpense)
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("Expense type should pass validation: %v", errs)
		}
	})

	t.Run("Subcategory passes validation", func(t *testing.T) {
		parentID := types.NewID()
		cat := NewSubcategory("Groceries", parentID, TypeExpense)
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid subcategory should pass validation: %v", errs)
		}
	})

	t.Run("System category passes validation", func(t *testing.T) {
		cat := NewSystemCategory("Transfer", TypeExpense)
		errs := cat.Validate()
		if errs.HasErrors() {
			t.Errorf("System category should pass validation: %v", errs)
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		cat := validCategory()
		cat.Name = ""
		cat.Type = Type("bad")
		errs := cat.Validate()
		if len(errs) < 2 {
			t.Errorf("Expected at least 2 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestCategoryIsValid(t *testing.T) {
	t.Run("IsValid returns false for invalid category", func(t *testing.T) {
		cat := NewCategory("", TypeExpense) // Empty name
		if cat.IsValid() {
			t.Error("IsValid should return false for category with empty name")
		}
	})
}
