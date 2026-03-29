package category

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
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
			if dc.Type == TypeIncome {
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
			if dc.Type == TypeExpense {
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

func TestService_SeedDefaultCategories(t *testing.T) {
	t.Run("seeds all default categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

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
		repo := NewRepository(database)
		svc := NewService(repo, database)

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
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Verify income categories
		incomeCategories, err := repo.ListByType(TypeIncome)
		if err != nil {
			t.Fatalf("ListByType(income) error = %v", err)
		}

		if len(incomeCategories) == 0 {
			t.Error("SeedDefaultCategories should create income categories")
		}
	})

	t.Run("creates expense categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.SeedDefaultCategories()
		if err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		// Verify expense categories
		expenseCategories, err := repo.ListByType(TypeExpense)
		if err != nil {
			t.Fatalf("ListByType(expense) error = %v", err)
		}

		if len(expenseCategories) == 0 {
			t.Error("SeedDefaultCategories should create expense categories")
		}
	})

	t.Run("creates parent-child relationships", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

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
			if child.Type != TypeExpense {
				t.Errorf("Child %q should have expense type, got %q", child.Name, child.Type)
			}
		}
	})

	t.Run("is idempotent - does not duplicate categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

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

func TestService_GetTransferCategory(t *testing.T) {
	t.Run("returns Transfer category after seeding", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

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
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Don't seed
		_, err := svc.GetTransferCategory()
		if err == nil {
			t.Error("GetTransferCategory() expected error when Transfer not seeded")
		}
	})
}

func TestService_HasDefaultCategories(t *testing.T) {
	t.Run("returns false for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

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
		repo := NewRepository(database)
		svc := NewService(repo, database)

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

func TestNewService(t *testing.T) {
	t.Run("creates service with repository", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		if svc == nil {
			t.Error("NewService should not return nil")
		}
		if svc.repo != repo {
			t.Error("NewService should store repository")
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
			found := slices.Contains(income.Children, expected)
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
			found := slices.Contains(food.Children, expected)
			if !found {
				t.Errorf("Food missing expected subcategory: %q", expected)
			}
		}
	})
}

func TestService_Create(t *testing.T) {
	t.Run("creates valid category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		cat := NewCategory("Test Category", TypeExpense)
		err := svc.Create(cat)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := svc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Test Category" {
			t.Errorf("Expected name 'Test Category', got %q", retrieved.Name)
		}
	})

	t.Run("validates category before creating", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		cat := NewCategory("", TypeExpense) // Invalid: empty name
		err := svc.Create(cat)
		if err == nil {
			t.Error("Create() expected error for invalid category")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("creates subcategory with valid parent", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create parent
		parent := NewCategory("Parent", TypeExpense)
		if err := svc.Create(parent); err != nil {
			t.Fatalf("Create parent error = %v", err)
		}

		// Create child
		child := NewSubcategory("Child", parent.ID, TypeExpense)
		if err := svc.Create(child); err != nil {
			t.Fatalf("Create child error = %v", err)
		}

		// Verify relationship
		retrieved, retrievedParent, err := svc.GetWithParent(child.ID)
		if err != nil {
			t.Fatalf("GetWithParent() error = %v", err)
		}
		if retrieved.Name != "Child" {
			t.Errorf("Expected name 'Child', got %q", retrieved.Name)
		}
		if retrievedParent == nil {
			t.Fatal("Expected parent, got nil")
		}
		if retrievedParent.Name != "Parent" {
			t.Errorf("Expected parent name 'Parent', got %q", retrievedParent.Name)
		}
	})
}

func TestService_Update(t *testing.T) {
	t.Run("updates non-system category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		cat := NewCategory("Original", TypeExpense)
		if err := svc.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cat.Name = "Updated"
		if err := svc.Update(cat); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := svc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Updated" {
			t.Errorf("Expected name 'Updated', got %q", retrieved.Name)
		}
	})

	t.Run("rejects update to system category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Seed to create Transfer system category
		if err := svc.SeedDefaultCategories(); err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		transfer, err := svc.GetTransferCategory()
		if err != nil {
			t.Fatalf("GetTransferCategory() error = %v", err)
		}

		transfer.Name = "Modified Transfer"
		err = svc.Update(transfer)
		if err == nil {
			t.Error("Update() expected error for system category")
		}
		if _, ok := err.(*IsSystemError); !ok {
			t.Errorf("Expected IsSystemError, got %T: %v", err, err)
		}
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("deletes non-system category without dependencies", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		cat := NewCategory("ToDelete", TypeExpense)
		if err := svc.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(cat.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(cat.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("rejects delete of system category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Seed to create Transfer system category
		if err := svc.SeedDefaultCategories(); err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		transfer, err := svc.GetTransferCategory()
		if err != nil {
			t.Fatalf("GetTransferCategory() error = %v", err)
		}

		err = svc.Delete(transfer.ID)
		if err == nil {
			t.Error("Delete() expected error for system category")
		}
		if _, ok := err.(*IsSystemError); !ok {
			t.Errorf("Expected IsSystemError, got %T: %v", err, err)
		}
	})

	t.Run("rejects delete of category with children", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create parent
		parent := NewCategory("Parent", TypeExpense)
		if err := svc.Create(parent); err != nil {
			t.Fatalf("Create parent error = %v", err)
		}

		// Create child
		child := NewSubcategory("Child", parent.ID, TypeExpense)
		if err := svc.Create(child); err != nil {
			t.Fatalf("Create child error = %v", err)
		}

		// Try to delete parent
		err := svc.Delete(parent.ID)
		if err == nil {
			t.Error("Delete() expected error for category with children")
		}
	})
}

func TestService_List(t *testing.T) {
	t.Run("returns all categories", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create some categories
		if err := svc.Create(NewCategory("Cat1", TypeExpense)); err != nil {
			t.Fatalf("Create error = %v", err)
		}
		if err := svc.Create(NewCategory("Cat2", TypeIncome)); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		categories, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(categories) != 2 {
			t.Errorf("Expected 2 categories, got %d", len(categories))
		}
	})

	t.Run("ListByType returns correct type", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create categories of different types
		if err := svc.Create(NewCategory("Expense1", TypeExpense)); err != nil {
			t.Fatalf("Create error = %v", err)
		}
		if err := svc.Create(NewCategory("Income1", TypeIncome)); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		expenses, err := svc.ListByType(TypeExpense)
		if err != nil {
			t.Fatalf("ListByType() error = %v", err)
		}
		if len(expenses) != 1 {
			t.Errorf("Expected 1 expense category, got %d", len(expenses))
		}
		if expenses[0].Name != "Expense1" {
			t.Errorf("Expected 'Expense1', got %q", expenses[0].Name)
		}
	})

	t.Run("ListTopLevel returns only top-level", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create parent and child
		parent := NewCategory("Parent", TypeExpense)
		if err := svc.Create(parent); err != nil {
			t.Fatalf("Create parent error = %v", err)
		}
		if err := svc.Create(NewSubcategory("Child", parent.ID, TypeExpense)); err != nil {
			t.Fatalf("Create child error = %v", err)
		}

		topLevel, err := svc.ListTopLevel()
		if err != nil {
			t.Fatalf("ListTopLevel() error = %v", err)
		}
		if len(topLevel) != 1 {
			t.Errorf("Expected 1 top-level category, got %d", len(topLevel))
		}
		if topLevel[0].Name != "Parent" {
			t.Errorf("Expected 'Parent', got %q", topLevel[0].Name)
		}
	})

	t.Run("ListChildren returns only children", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create parent and children
		parent := NewCategory("Parent", TypeExpense)
		if err := svc.Create(parent); err != nil {
			t.Fatalf("Create parent error = %v", err)
		}
		if err := svc.Create(NewSubcategory("Child1", parent.ID, TypeExpense)); err != nil {
			t.Fatalf("Create child1 error = %v", err)
		}
		if err := svc.Create(NewSubcategory("Child2", parent.ID, TypeExpense)); err != nil {
			t.Fatalf("Create child2 error = %v", err)
		}

		children, err := svc.ListChildren(parent.ID)
		if err != nil {
			t.Fatalf("ListChildren() error = %v", err)
		}
		if len(children) != 2 {
			t.Errorf("Expected 2 children, got %d", len(children))
		}
	})
}

func TestService_MergeCategories(t *testing.T) {
	t.Run("merges source into target", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create source and target
		source := NewCategory("Source", TypeExpense)
		target := NewCategory("Target", TypeExpense)
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		// Merge
		if err := svc.MergeCategories(source.ID, target.ID); err != nil {
			t.Fatalf("MergeCategories() error = %v", err)
		}

		// Source should be deleted
		_, err := svc.GetByID(source.ID)
		if err == nil {
			t.Error("Source category should be deleted after merge")
		}

		// Target should still exist
		_, err = svc.GetByID(target.ID)
		if err != nil {
			t.Errorf("Target category should still exist: %v", err)
		}
	})

	t.Run("rejects merge into self", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		cat := NewCategory("Self", TypeExpense)
		if err := svc.Create(cat); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		err := svc.MergeCategories(cat.ID, cat.ID)
		if err == nil {
			t.Error("MergeCategories() expected error when merging into self")
		}
		if _, ok := err.(*MergeSameError); !ok {
			t.Errorf("Expected MergeSameError, got %T: %v", err, err)
		}
	})

	t.Run("rejects merge of different types", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		expense := NewCategory("Expense", TypeExpense)
		income := NewCategory("Income", TypeIncome)
		if err := svc.Create(expense); err != nil {
			t.Fatalf("Create expense error = %v", err)
		}
		if err := svc.Create(income); err != nil {
			t.Fatalf("Create income error = %v", err)
		}

		err := svc.MergeCategories(expense.ID, income.ID)
		if err == nil {
			t.Error("MergeCategories() expected error for type mismatch")
		}
		if _, ok := err.(*MergeTypeMismatchError); !ok {
			t.Errorf("Expected MergeTypeMismatchError, got %T: %v", err, err)
		}
	})

	t.Run("rejects merge of system category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Seed to create Transfer system category
		if err := svc.SeedDefaultCategories(); err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}

		transfer, err := svc.GetTransferCategory()
		if err != nil {
			t.Fatalf("GetTransferCategory() error = %v", err)
		}

		target := NewCategory("Target", TypeExpense)
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		// Try to merge Transfer (system) into target
		err = svc.MergeCategories(transfer.ID, target.ID)
		if err == nil {
			t.Error("MergeCategories() expected error for system category as source")
		}
		if _, ok := err.(*IsSystemError); !ok {
			t.Errorf("Expected IsSystemError, got %T: %v", err, err)
		}

		// Try to merge into Transfer (system)
		source := NewCategory("Source", TypeExpense)
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}

		err = svc.MergeCategories(source.ID, transfer.ID)
		if err == nil {
			t.Error("MergeCategories() expected error for system category as target")
		}
		if _, ok := err.(*IsSystemError); !ok {
			t.Errorf("Expected IsSystemError, got %T: %v", err, err)
		}
	})

	t.Run("reassigns children during merge", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create source with child
		source := NewCategory("Source", TypeExpense)
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}
		child := NewSubcategory("Child", source.ID, TypeExpense)
		if err := svc.Create(child); err != nil {
			t.Fatalf("Create child error = %v", err)
		}

		// Create target
		target := NewCategory("Target", TypeExpense)
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		// Merge
		if err := svc.MergeCategories(source.ID, target.ID); err != nil {
			t.Fatalf("MergeCategories() error = %v", err)
		}

		// Child should now be under target
		updatedChild, err := svc.GetByID(child.ID)
		if err != nil {
			t.Fatalf("GetByID child error = %v", err)
		}
		if !updatedChild.ParentID.Valid || updatedChild.ParentID.ID != target.ID {
			t.Error("Child should be reassigned to target")
		}
	})
}
