package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
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
	db   *db.DB
}

// NewCategoryService creates a new CategoryService.
func NewCategoryService(repo *repository.CategoryRepository, database *db.DB) *CategoryService {
	return &CategoryService{
		repo: repo,
		db:   database,
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

// Create validates and creates a new category.
// If setting a parent, validates the parent exists and type matches.
func (s *CategoryService) Create(category *models.Category) error {
	if err := s.validateCategory(category); err != nil {
		return err
	}

	// Additional parent validation is done in the repository
	return s.repo.Create(category)
}

// GetByID retrieves a category by its ID.
func (s *CategoryService) GetByID(id models.ID) (*models.Category, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves a category by its name within a parent.
// Pass nil for parentID to search for top-level categories.
func (s *CategoryService) GetByName(name string, parentID *models.ID) (*models.Category, error) {
	return s.repo.GetByName(name, parentID)
}

// GetWithParent retrieves a category and its parent (if any) by ID.
func (s *CategoryService) GetWithParent(id models.ID) (*models.Category, *models.Category, error) {
	return s.repo.GetWithParent(id)
}

// Update validates and updates an existing category.
// System categories cannot be updated (except by seeding).
func (s *CategoryService) Update(category *models.Category) error {
	// Check if this is a system category
	existing, err := s.repo.GetByID(category.ID)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return &CategoryIsSystemError{ID: category.ID.String(), Name: existing.Name}
	}

	if err := s.validateCategory(category); err != nil {
		return err
	}

	return s.repo.Update(category)
}

// Delete removes a category.
// System categories cannot be deleted.
// Categories with transactions or subcategories cannot be deleted.
func (s *CategoryService) Delete(id models.ID) error {
	// Check if this is a system category
	category, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if category.IsSystem {
		return &CategoryIsSystemError{ID: id.String(), Name: category.Name}
	}

	// Repository handles checks for subcategories and transactions
	return s.repo.Delete(id)
}

// List returns all categories ordered by name.
func (s *CategoryService) List() ([]*models.Category, error) {
	return s.repo.List()
}

// ListByType returns all categories of a specific type.
func (s *CategoryService) ListByType(categoryType models.CategoryType) ([]*models.Category, error) {
	return s.repo.ListByType(categoryType)
}

// ListTopLevel returns all top-level categories (those without a parent).
func (s *CategoryService) ListTopLevel() ([]*models.Category, error) {
	return s.repo.ListTopLevel()
}

// ListChildren returns all child categories of a parent.
func (s *CategoryService) ListChildren(parentID models.ID) ([]*models.Category, error) {
	return s.repo.ListChildren(parentID)
}

// MergeCategories merges the source category into the target category.
// All transactions, splits, scheduled transactions, and payee defaults
// using the source category will be updated to use the target category.
// The source category is then deleted.
//
// Rules:
// - Source and target must have the same type (income/expense)
// - System categories cannot be merged
// - Cannot merge a category into itself
func (s *CategoryService) MergeCategories(sourceID, targetID models.ID) error {
	// Cannot merge into itself
	if sourceID == targetID {
		return &CategoryMergeSameError{ID: sourceID.String()}
	}

	// Get both categories
	source, err := s.repo.GetByID(sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source category: %w", err)
	}

	target, err := s.repo.GetByID(targetID)
	if err != nil {
		return fmt.Errorf("failed to get target category: %w", err)
	}

	// System categories cannot be merged
	if source.IsSystem {
		return &CategoryIsSystemError{ID: sourceID.String(), Name: source.Name}
	}
	if target.IsSystem {
		return &CategoryIsSystemError{ID: targetID.String(), Name: target.Name}
	}

	// Types must match
	if source.Type != target.Type {
		return &CategoryMergeTypeMismatchError{
			SourceID:   sourceID.String(),
			SourceType: source.Type.String(),
			TargetID:   targetID.String(),
			TargetType: target.Type.String(),
		}
	}

	// Update all references to use target category
	// Note: DuckDB doesn't support transactions, so we do best-effort updates

	// Update transactions
	_, err = s.db.Conn().Exec(`
		UPDATE transactions
		SET category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
		WHERE CAST(category_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update transactions: %w", err)
	}

	// Update transaction splits
	_, err = s.db.Conn().Exec(`
		UPDATE transaction_splits
		SET category_id = CAST(? AS UUID)
		WHERE CAST(category_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update transaction splits: %w", err)
	}

	// Update payee defaults
	_, err = s.db.Conn().Exec(`
		UPDATE payees
		SET default_category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
		WHERE CAST(default_category_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update payee defaults: %w", err)
	}

	// Update scheduled transactions
	_, err = s.db.Conn().Exec(`
		UPDATE scheduled_transactions
		SET category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
		WHERE CAST(category_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update scheduled transactions: %w", err)
	}

	// If source has children, reassign them to target
	children, err := s.repo.ListChildren(sourceID)
	if err != nil {
		return fmt.Errorf("failed to list source children: %w", err)
	}
	for _, child := range children {
		child.SetParent(targetID)
		if err := s.repo.Update(child); err != nil {
			return fmt.Errorf("failed to reassign child category %s: %w", child.Name, err)
		}
	}

	// Delete the source category (should now have no references)
	if err := s.repo.Delete(sourceID); err != nil {
		return fmt.Errorf("failed to delete source category: %w", err)
	}

	return nil
}

// validateCategory validates a category and returns any validation errors.
func (s *CategoryService) validateCategory(category *models.Category) error {
	errors := category.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
