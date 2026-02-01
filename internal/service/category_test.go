package service

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func TestDefaultCategories(t *testing.T) {
	t.Run("contains Transfer system category", func(t *testing.T) {
		found := false
		for _, dc := range DefaultCategories {
			if dc.Name == "Transfer" && dc.IsSystem {
				found = true
				break
			}
		}
		if !found {
			t.Error("DefaultCategories should contain Transfer system category")
		}
	})

	t.Run("contains income categories", func(t *testing.T) {
		incomeCount := 0
		for _, dc := range DefaultCategories {
			if dc.Type == models.CategoryTypeIncome {
				incomeCount++
			}
		}
		if incomeCount == 0 {
			t.Error("DefaultCategories should contain income categories")
		}
	})

	t.Run("contains expense categories", func(t *testing.T) {
		expenseCount := 0
		for _, dc := range DefaultCategories {
			if dc.Type == models.CategoryTypeExpense {
				expenseCount++
			}
		}
		if expenseCount == 0 {
			t.Error("DefaultCategories should contain expense categories")
		}
	})

	t.Run("has no empty names", func(t *testing.T) {
		for _, dc := range DefaultCategories {
			if dc.Name == "" {
				t.Error("DefaultCategories should not have empty names")
			}
			for _, child := range dc.Children {
				if child == "" {
					t.Errorf("Category %q has empty child name", dc.Name)
				}
			}
		}
	})
}

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

func TestCategoryService_SeedDefaultCategories(t *testing.T) {
	t.Run("seeds all default categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Verify categories were created
		categories, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		if len(categories) == 0 {
			t.Error("SeedDefaultCategories should create categories")
		}

		// Count expected categories
		expectedCount := 0
		for _, dc := range DefaultCategories {
			expectedCount++ // Parent
			expectedCount += len(dc.Children)
		}

		if len(categories) != expectedCount {
			t.Errorf("Expected %d categories, got %d", expectedCount, len(categories))
		}
	})

	t.Run("creates Transfer as system category", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Find Transfer category
		transfer, err := repo.GetByName("Transfer", nil)
		if err != nil {
			t.Fatalf("GetByName(Transfer) error = %v", err)
		}

		if !transfer.IsSystem {
			t.Error("Transfer category should be a system category")
		}
	})

	t.Run("creates income categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Verify income categories
		incomeCategories, err := repo.ListByType(models.CategoryTypeIncome)
		if err != nil {
			t.Fatalf("ListByType(income) error = %v", err)
		}

		if len(incomeCategories) == 0 {
			t.Error("SeedDefaultCategories should create income categories")
		}
	})

	t.Run("creates expense categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Verify expense categories
		expenseCategories, err := repo.ListByType(models.CategoryTypeExpense)
		if err != nil {
			t.Fatalf("ListByType(expense) error = %v", err)
		}

		if len(expenseCategories) == 0 {
			t.Error("SeedDefaultCategories should create expense categories")
		}
	})

	t.Run("creates parent-child relationships", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Find a parent with known children (e.g., Food)
		food, err := repo.GetByName("Food", nil)
		if err != nil {
			t.Fatalf("GetByName(Food) error = %v", err)
		}

		children, err := repo.ListChildren(food.ID)
		if err != nil {
			t.Fatalf("ListChildren() error = %v", err)
		}

		// Food should have children: Groceries, Dining Out, Coffee & Snacks
		if len(children) != 3 {
			t.Errorf("Food should have 3 children, got %d", len(children))
		}

		// Verify children have correct type
		for _, child := range children {
			if child.Type != models.CategoryTypeExpense {
				t.Errorf("Child %q should have expense type, got %q", child.Name, child.Type)
			}
		}
	})

	t.Run("is idempotent - does not duplicate categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		// Seed twice
		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("First SeedDefaultCategories() error = %v", err)
		}

		err = svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("Second SeedDefaultCategories() error = %v", err)
		}

		// Count categories
		categories, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}

		// Should have same count as single seed
		expectedCount := 0
		for _, dc := range DefaultCategories {
			expectedCount++
			expectedCount += len(dc.Children)
		}

		if len(categories) != expectedCount {
			t.Errorf("Expected %d categories after double seed, got %d", expectedCount, len(categories))
		}
	})
}

func TestCategoryService_GetTransferCategory(t *testing.T) {
	t.Run("returns Transfer category after seeding", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		// Seed first
		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		transfer, err := svc.GetTransferCategory()
		if err != nil {
			t.Fatalf("GetTransferCategory() error = %v", err)
		}

		if transfer.Name != "Transfer" {
			t.Errorf("Expected name 'Transfer', got %q", transfer.Name)
		}
		if !transfer.IsSystem {
			t.Error("Transfer should be a system category")
		}
	})

	t.Run("returns error when Transfer not found", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		// Don't seed
		_, err := svc.GetTransferCategory()
		if err == nil {
			t.Error("GetTransferCategory() expected error when Transfer not seeded")
		}
	})
}

func TestCategoryService_HasDefaultCategories(t *testing.T) {
	t.Run("returns false for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		has, err := svc.HasDefaultCategories()
		if err != nil {
			t.Fatalf("HasDefaultCategories() error = %v", err)
		}

		if has {
			t.Error("HasDefaultCategories() should return false for empty database")
		}
	})

	t.Run("returns true after seeding", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		has, err := svc.HasDefaultCategories()
		if err != nil {
			t.Fatalf("HasDefaultCategories() error = %v", err)
		}

		if !has {
			t.Error("HasDefaultCategories() should return true after seeding")
		}
	})
}

func TestNewCategoryService(t *testing.T) {
	t.Run("creates service with repository", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewCategoryRepository(database)
		svc := NewCategoryService(repo)

		if svc == nil {
			t.Error("NewCategoryService should not return nil")
		}
		if svc.repo != repo {
			t.Error("NewCategoryService should store repository")
		}
	})
}

func TestDefaultCategoriesContent(t *testing.T) {
	// Test specific expected categories exist
	expectedParents := []string{
		"Transfer",
		"Income",
		"Housing",
		"Utilities",
		"Food",
		"Transportation",
		"Healthcare",
		"Personal",
		"Entertainment",
		"Shopping",
		"Financial",
		"Taxes",
		"Miscellaneous",
	}

	t.Run("contains all expected parent categories", func(t *testing.T) {
		for _, expected := range expectedParents {
			found := false
			for _, dc := range DefaultCategories {
				if dc.Name == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Missing expected category: %q", expected)
			}
		}
	})

	t.Run("Income category has expected subcategories", func(t *testing.T) {
		var income *DefaultCategory
		for i := range DefaultCategories {
			if DefaultCategories[i].Name == "Income" {
				income = &DefaultCategories[i]
				break
			}
		}

		if income == nil {
			t.Fatal("Income category not found")
		}

		expectedChildren := []string{"Salary", "Bonus", "Interest", "Dividends"}
		for _, expected := range expectedChildren {
			found := false
			for _, child := range income.Children {
				if child == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Income missing expected subcategory: %q", expected)
			}
		}
	})

	t.Run("Food category has expected subcategories", func(t *testing.T) {
		var food *DefaultCategory
		for i := range DefaultCategories {
			if DefaultCategories[i].Name == "Food" {
				food = &DefaultCategories[i]
				break
			}
		}

		if food == nil {
			t.Fatal("Food category not found")
		}

		expectedChildren := []string{"Groceries", "Dining Out"}
		for _, expected := range expectedChildren {
			found := false
			for _, child := range food.Children {
				if child == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Food missing expected subcategory: %q", expected)
			}
		}
	})
}
