package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// DefaultCategory represents a default category to be seeded.
type DefaultCategory struct {
	Name     string
	Type     models.CategoryType
	IsSystem bool
	Children []string // Names of subcategories
}

// DefaultCategories defines the default categories to seed for new files.
// These are based on common personal finance categories.
var DefaultCategories = []DefaultCategory{
	// System category for transfers between accounts
	{
		Name:     "Transfer",
		Type:     models.CategoryTypeExpense,
		IsSystem: true,
	},

	// Income categories
	{
		Name: "Income",
		Type: models.CategoryTypeIncome,
		Children: []string{
			"Salary",
			"Bonus",
			"Interest",
			"Dividends",
			"Gifts Received",
			"Other Income",
		},
	},

	// Expense categories
	{
		Name: "Housing",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Rent",
			"Mortgage",
			"Property Tax",
			"Home Insurance",
			"Repairs & Maintenance",
		},
	},
	{
		Name: "Utilities",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Electric",
			"Gas",
			"Water",
			"Internet",
			"Phone",
		},
	},
	{
		Name: "Food",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Groceries",
			"Dining Out",
			"Coffee & Snacks",
		},
	},
	{
		Name: "Transportation",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Gas & Fuel",
			"Auto Insurance",
			"Auto Repairs",
			"Parking",
			"Public Transit",
		},
	},
	{
		Name: "Healthcare",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Doctor",
			"Dentist",
			"Pharmacy",
			"Health Insurance",
		},
	},
	{
		Name: "Personal",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Clothing",
			"Haircut",
			"Gym",
		},
	},
	{
		Name: "Entertainment",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Movies",
			"Music & Streaming",
			"Books & Magazines",
			"Hobbies",
		},
	},
	{
		Name: "Shopping",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Electronics",
			"Home Goods",
			"Gifts Given",
		},
	},
	{
		Name: "Financial",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Bank Fees",
			"Late Fees",
			"Interest Paid",
		},
	},
	{
		Name: "Taxes",
		Type: models.CategoryTypeExpense,
		Children: []string{
			"Federal Tax",
			"State Tax",
			"Local Tax",
		},
	},
	{
		Name: "Miscellaneous",
		Type: models.CategoryTypeExpense,
	},
}

// CategoryService provides business logic for category operations.
type CategoryService struct {
	repo *repository.CategoryRepository
}

// NewCategoryService creates a new CategoryService.
func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{
		repo: repo,
	}
}

// SeedDefaultCategories creates all default categories in the database.
// This should be called when a new TMoney file is created.
// It skips categories that already exist (based on name and parent).
func (s *CategoryService) SeedDefaultCategories() error {
	for _, dc := range DefaultCategories {
		if err := s.seedCategory(dc); err != nil {
			return fmt.Errorf("failed to seed category %q: %w", dc.Name, err)
		}
	}
	return nil
}

// seedCategory creates a single parent category and its children.
func (s *CategoryService) seedCategory(dc DefaultCategory) error {
	// Check if parent category already exists
	existing, err := s.repo.GetByName(dc.Name, nil)
	if err == nil {
		// Category exists, skip seeding children since they should already exist
		_ = existing
		return nil
	}

	// Check if error is "not found" - that's expected for seeding
	if _, ok := err.(*repository.NotFoundError); !ok {
		return fmt.Errorf("failed to check existing category: %w", err)
	}

	// Create the parent category
	var parent *models.Category
	if dc.IsSystem {
		parent = models.NewSystemCategory(dc.Name, dc.Type)
	} else {
		parent = models.NewCategory(dc.Name, dc.Type)
	}

	if err := s.repo.Create(parent); err != nil {
		return fmt.Errorf("failed to create parent category: %w", err)
	}

	// Create child categories
	for _, childName := range dc.Children {
		child := models.NewSubcategory(childName, parent.ID, dc.Type)
		if err := s.repo.Create(child); err != nil {
			return fmt.Errorf("failed to create subcategory %q: %w", childName, err)
		}
	}

	return nil
}

// GetTransferCategory returns the system Transfer category.
// This category is used for transfers between accounts.
func (s *CategoryService) GetTransferCategory() (*models.Category, error) {
	categories, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	for _, cat := range categories {
		if cat.IsSystem && cat.Name == "Transfer" {
			return cat, nil
		}
	}

	return nil, &repository.NotFoundError{Entity: "category", ID: "Transfer"}
}

// HasDefaultCategories checks if default categories have been seeded.
func (s *CategoryService) HasDefaultCategories() (bool, error) {
	categories, err := s.repo.ListTopLevel()
	if err != nil {
		return false, err
	}
	return len(categories) > 0, nil
}
