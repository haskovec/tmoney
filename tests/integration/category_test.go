package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// TestCategoryLifecycle tests the complete category lifecycle:
// create database -> create category -> list categories -> update -> delete -> cleanup
func TestCategoryLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	var categoryID models.ID

	// Step 1: Create a test category
	t.Run("Create category", func(t *testing.T) {
		category := models.NewCategory("Groceries", models.CategoryTypeExpense)
		categoryID = category.ID

		err = repo.Create(category)
		if err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
	})

	// Step 2: List all categories
	t.Run("List categories", func(t *testing.T) {
		categories, err := repo.List()
		if err != nil {
			t.Fatalf("Failed to list categories: %v", err)
		}

		if len(categories) != 1 {
			t.Fatalf("Expected 1 category, got %d", len(categories))
		}

		retrieved := categories[0]
		if retrieved.Name != "Groceries" {
			t.Errorf("Expected name 'Groceries', got %q", retrieved.Name)
		}
		if retrieved.Type != models.CategoryTypeExpense {
			t.Errorf("Expected type 'expense', got %q", retrieved.Type)
		}
		if retrieved.ParentID.Valid {
			t.Error("Expected category to have no parent")
		}
		if retrieved.IsSystem {
			t.Error("Expected category to not be a system category")
		}
	})

	// Step 3: Retrieve by ID and by name
	t.Run("Get category by ID and name", func(t *testing.T) {
		// Get by name (nil parent for top-level)
		category, err := repo.GetByName("Groceries", nil)
		if err != nil {
			t.Fatalf("Failed to get category by name: %v", err)
		}
		if category.Name != "Groceries" {
			t.Errorf("Expected name 'Groceries', got %q", category.Name)
		}

		// Get by ID
		categoryByID, err := repo.GetByID(category.ID)
		if err != nil {
			t.Fatalf("Failed to get category by ID: %v", err)
		}
		if categoryByID.Name != category.Name {
			t.Errorf("Expected same category, got different names: %q vs %q", category.Name, categoryByID.Name)
		}
	})

	// Step 4: Update the category
	t.Run("Update category", func(t *testing.T) {
		category, err := repo.GetByID(categoryID)
		if err != nil {
			t.Fatalf("Failed to get category: %v", err)
		}

		category.Name = "Food & Groceries"

		err = repo.Update(category)
		if err != nil {
			t.Fatalf("Failed to update category: %v", err)
		}

		updated, err := repo.GetByID(category.ID)
		if err != nil {
			t.Fatalf("Failed to get updated category: %v", err)
		}
		if updated.Name != "Food & Groceries" {
			t.Errorf("Expected name 'Food & Groceries', got %q", updated.Name)
		}
	})

	// Step 5: Delete the category
	t.Run("Delete category", func(t *testing.T) {
		err = repo.Delete(categoryID)
		if err != nil {
			t.Fatalf("Failed to delete category: %v", err)
		}

		categories, err := repo.List()
		if err != nil {
			t.Fatalf("Failed to list categories: %v", err)
		}
		if len(categories) != 0 {
			t.Errorf("Expected 0 categories after deletion, got %d", len(categories))
		}
	})
}

// TestCategoryHierarchy tests parent/child category relationships.
func TestCategoryHierarchy(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Create parent category
	parent := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := repo.Create(parent); err != nil {
		t.Fatalf("Failed to create parent category: %v", err)
	}

	// Create subcategories
	groceries := models.NewSubcategory("Groceries", parent.ID, models.CategoryTypeExpense)
	if err := repo.Create(groceries); err != nil {
		t.Fatalf("Failed to create groceries subcategory: %v", err)
	}

	restaurants := models.NewSubcategory("Restaurants", parent.ID, models.CategoryTypeExpense)
	if err := repo.Create(restaurants); err != nil {
		t.Fatalf("Failed to create restaurants subcategory: %v", err)
	}

	// Test ListTopLevel
	t.Run("List top-level categories", func(t *testing.T) {
		topLevel, err := repo.ListTopLevel()
		if err != nil {
			t.Fatalf("Failed to list top-level categories: %v", err)
		}

		if len(topLevel) != 1 {
			t.Errorf("Expected 1 top-level category, got %d", len(topLevel))
		}
		if topLevel[0].Name != "Food" {
			t.Errorf("Expected 'Food', got %q", topLevel[0].Name)
		}
	})

	// Test ListChildren
	t.Run("List children", func(t *testing.T) {
		children, err := repo.ListChildren(parent.ID)
		if err != nil {
			t.Fatalf("Failed to list children: %v", err)
		}

		if len(children) != 2 {
			t.Errorf("Expected 2 children, got %d", len(children))
		}

		// Children should be ordered by name
		if children[0].Name != "Groceries" {
			t.Errorf("Expected first child 'Groceries', got %q", children[0].Name)
		}
		if children[1].Name != "Restaurants" {
			t.Errorf("Expected second child 'Restaurants', got %q", children[1].Name)
		}
	})

	// Test GetWithParent
	t.Run("Get with parent", func(t *testing.T) {
		category, categoryParent, err := repo.GetWithParent(groceries.ID)
		if err != nil {
			t.Fatalf("Failed to get with parent: %v", err)
		}

		if category.Name != "Groceries" {
			t.Errorf("Expected category 'Groceries', got %q", category.Name)
		}
		if categoryParent == nil {
			t.Fatal("Expected parent category, got nil")
		}
		if categoryParent.Name != "Food" {
			t.Errorf("Expected parent 'Food', got %q", categoryParent.Name)
		}
	})

	// Test GetWithParent for top-level category
	t.Run("Get with parent (top-level)", func(t *testing.T) {
		category, categoryParent, err := repo.GetWithParent(parent.ID)
		if err != nil {
			t.Fatalf("Failed to get with parent: %v", err)
		}

		if category.Name != "Food" {
			t.Errorf("Expected category 'Food', got %q", category.Name)
		}
		if categoryParent != nil {
			t.Errorf("Expected nil parent for top-level category, got %v", categoryParent)
		}
	})

	// Test that parent cannot be deleted while children exist
	t.Run("Cannot delete parent with children", func(t *testing.T) {
		err := repo.Delete(parent.ID)
		if err == nil {
			t.Fatal("Expected error when deleting parent with children")
		}
		if _, ok := err.(*repository.HasDependentsError); !ok {
			t.Errorf("Expected HasDependentsError, got %T: %v", err, err)
		}
	})
}

// TestCategoryByType tests listing categories by type.
func TestCategoryByType(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Create expense categories
	groceries := models.NewCategory("Groceries", models.CategoryTypeExpense)
	if err := repo.Create(groceries); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	utilities := models.NewCategory("Utilities", models.CategoryTypeExpense)
	if err := repo.Create(utilities); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	// Create income categories
	salary := models.NewCategory("Salary", models.CategoryTypeIncome)
	if err := repo.Create(salary); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	// Test ListByType
	t.Run("List expense categories", func(t *testing.T) {
		expenses, err := repo.ListByType(models.CategoryTypeExpense)
		if err != nil {
			t.Fatalf("Failed to list expense categories: %v", err)
		}

		if len(expenses) != 2 {
			t.Errorf("Expected 2 expense categories, got %d", len(expenses))
		}
	})

	t.Run("List income categories", func(t *testing.T) {
		income, err := repo.ListByType(models.CategoryTypeIncome)
		if err != nil {
			t.Fatalf("Failed to list income categories: %v", err)
		}

		if len(income) != 1 {
			t.Errorf("Expected 1 income category, got %d", len(income))
		}
		if income[0].Name != "Salary" {
			t.Errorf("Expected 'Salary', got %q", income[0].Name)
		}
	})
}

// TestCategoryDuplicateNameValidation tests that duplicate names are rejected.
func TestCategoryDuplicateNameValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Create first category
	first := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := repo.Create(first); err != nil {
		t.Fatalf("Failed to create first category: %v", err)
	}

	// Try to create duplicate at top level
	t.Run("Duplicate top-level name rejected", func(t *testing.T) {
		duplicate := models.NewCategory("Food", models.CategoryTypeExpense)
		err := repo.Create(duplicate)
		if err == nil {
			t.Fatal("Expected error for duplicate name")
		}
		if _, ok := err.(*repository.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})

	// Same name as subcategory is allowed
	t.Run("Same name under different parent allowed", func(t *testing.T) {
		subcategory := models.NewSubcategory("Food", first.ID, models.CategoryTypeExpense)
		err := repo.Create(subcategory)
		if err != nil {
			t.Errorf("Expected same name under parent to be allowed, got error: %v", err)
		}
	})
}

// TestCategorySubcategoryValidation tests subcategory creation rules.
func TestCategorySubcategoryValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Create expense parent
	expenseParent := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := repo.Create(expenseParent); err != nil {
		t.Fatalf("Failed to create parent category: %v", err)
	}

	// Create valid subcategory
	groceries := models.NewSubcategory("Groceries", expenseParent.ID, models.CategoryTypeExpense)
	if err := repo.Create(groceries); err != nil {
		t.Fatalf("Failed to create subcategory: %v", err)
	}

	// Test type mismatch
	t.Run("Subcategory type must match parent", func(t *testing.T) {
		mismatch := models.NewSubcategory("Income Item", expenseParent.ID, models.CategoryTypeIncome)
		err := repo.Create(mismatch)
		if err == nil {
			t.Fatal("Expected error for type mismatch")
		}
	})

	// Test three-level nesting rejected
	t.Run("Three-level nesting rejected", func(t *testing.T) {
		nested := models.NewSubcategory("Organic", groceries.ID, models.CategoryTypeExpense)
		err := repo.Create(nested)
		if err == nil {
			t.Fatal("Expected error for three-level nesting")
		}
	})

	// Test non-existent parent
	t.Run("Non-existent parent rejected", func(t *testing.T) {
		fakeParentID := models.NewID()
		orphan := models.NewSubcategory("Orphan", fakeParentID, models.CategoryTypeExpense)
		err := repo.Create(orphan)
		if err == nil {
			t.Fatal("Expected error for non-existent parent")
		}
	})
}

// TestSystemCategory tests system category creation.
func TestSystemCategory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Create system category
	transfer := models.NewSystemCategory("Transfer", models.CategoryTypeExpense)
	if err := repo.Create(transfer); err != nil {
		t.Fatalf("Failed to create system category: %v", err)
	}

	// Verify it's marked as system
	retrieved, err := repo.GetByID(transfer.ID)
	if err != nil {
		t.Fatalf("Failed to get system category: %v", err)
	}
	if !retrieved.IsSystem {
		t.Error("Expected category to be a system category")
	}
}

// TestCategoryNotFound tests error handling for non-existent categories.
func TestCategoryNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-category-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewCategoryRepository(database)

	// Try to get non-existent category by ID
	t.Run("Get by ID not found", func(t *testing.T) {
		_, err := repo.GetByID(models.NewID())
		if err == nil {
			t.Error("Expected error for non-existent category")
		}
		if _, ok := err.(*repository.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	// Try to get non-existent category by name
	t.Run("Get by name not found", func(t *testing.T) {
		_, err := repo.GetByName("Does Not Exist", nil)
		if err == nil {
			t.Error("Expected error for non-existent category")
		}
		if _, ok := err.(*repository.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	// Try to delete non-existent category
	t.Run("Delete not found", func(t *testing.T) {
		err := repo.Delete(models.NewID())
		if err == nil {
			t.Error("Expected error for deleting non-existent category")
		}
		if _, ok := err.(*repository.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	// Try to update non-existent category
	t.Run("Update not found", func(t *testing.T) {
		category := models.NewCategory("Fake", models.CategoryTypeExpense)
		err := repo.Update(category)
		if err == nil {
			t.Error("Expected error for updating non-existent category")
		}
		if _, ok := err.(*repository.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}
