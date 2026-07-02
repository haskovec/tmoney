package category

import (
	"slices"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
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

	t.Run("contains Value Adjustment system category", func(t *testing.T) {
		found := false
		for _, dc := range DefaultCategories {
			if dc.Name == ValueAdjustmentCategoryName && dc.IsSystem {
				if dc.Type != TypeExpense {
					t.Errorf("Value Adjustment should be expense-typed, got %q", dc.Type)
				}
				found = true
				break
			}
		}
		if !found {
			t.Error("DefaultCategories should contain Value Adjustment system category")
		}
	})

	t.Run("contains Loan:Interest as a non-system category", func(t *testing.T) {
		found := false
		for _, dc := range DefaultCategories {
			if dc.Name != "Loan" {
				continue
			}
			if dc.IsSystem {
				t.Error("Loan category should not be a system category")
			}
			if !slices.Contains(dc.Children, "Interest") {
				t.Errorf("Loan category should have an Interest child, got %v", dc.Children)
			}
			found = true
			break
		}
		if !found {
			t.Error("DefaultCategories should contain a Loan parent category")
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
	return dbtest.New(t)
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
			t.Fatal("NewService should not return nil")
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

// TestFileInit_BonusAndRetroPayCategoriesExist verifies that the
// paycheck-wizard seed step (EnsurePaycheckCategories, invoked on every
// database open by app.NewServices) creates Income:Bonus and
// Income:Retro Pay alongside the previously-seeded Income:Salary, Tax,
// and Insurance categories. This is the v2 paycheck wizard's
// pre-population requirement for the post-time preview dialog.
func TestFileInit_BonusAndRetroPayCategoriesExist(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	if err := svc.EnsurePaycheckCategories(); err != nil {
		t.Fatalf("EnsurePaycheckCategories: %v", err)
	}

	income, err := repo.GetByName("Income", nil)
	if err != nil {
		t.Fatalf("Income parent not found: %v", err)
	}

	t.Run("Income:Bonus exists as income subcategory", func(t *testing.T) {
		bonus, err := repo.GetByName("Bonus", &income.ID)
		if err != nil {
			t.Fatalf("Income:Bonus not found: %v", err)
		}
		if bonus.Type != TypeIncome {
			t.Errorf("Income:Bonus type = %v, want %v", bonus.Type, TypeIncome)
		}
	})

	t.Run("Income:Retro Pay exists as income subcategory", func(t *testing.T) {
		retro, err := repo.GetByName("Retro Pay", &income.ID)
		if err != nil {
			t.Fatalf("Income:Retro Pay not found: %v", err)
		}
		if retro.Type != TypeIncome {
			t.Errorf("Income:Retro Pay type = %v, want %v", retro.Type, TypeIncome)
		}
	})

	t.Run("Income:Salary seed unaffected", func(t *testing.T) {
		salary, err := repo.GetByName("Salary", &income.ID)
		if err != nil {
			t.Fatalf("Income:Salary not found: %v", err)
		}
		if salary.Type != TypeIncome {
			t.Errorf("Income:Salary type = %v, want %v", salary.Type, TypeIncome)
		}
	})

	t.Run("Tax seeds unaffected", func(t *testing.T) {
		tax, err := repo.GetByName("Tax", nil)
		if err != nil {
			t.Fatalf("Tax parent not found: %v", err)
		}
		for _, child := range []string{"Federal", "State", "Social Security", "Medicare"} {
			got, err := repo.GetByName(child, &tax.ID)
			if err != nil {
				t.Errorf("Tax:%s not found: %v", child, err)
				continue
			}
			if got.Type != TypeExpense {
				t.Errorf("Tax:%s type = %v, want %v", child, got.Type, TypeExpense)
			}
		}
	})

	t.Run("Insurance:Health seed unaffected", func(t *testing.T) {
		insurance, err := repo.GetByName("Insurance", nil)
		if err != nil {
			t.Fatalf("Insurance parent not found: %v", err)
		}
		health, err := repo.GetByName("Health", &insurance.ID)
		if err != nil {
			t.Fatalf("Insurance:Health not found: %v", err)
		}
		if health.Type != TypeExpense {
			t.Errorf("Insurance:Health type = %v, want %v", health.Type, TypeExpense)
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

func TestService_EnsureValueAdjustmentCategory(t *testing.T) {
	t.Run("seeds the system category on a fresh database", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		collision, err := svc.EnsureValueAdjustmentCategory()
		if err != nil {
			t.Fatalf("EnsureValueAdjustmentCategory() error = %v", err)
		}
		if collision {
			t.Error("expected no user collision on a fresh database")
		}

		cat, err := svc.GetByName(ValueAdjustmentCategoryName, nil)
		if err != nil {
			t.Fatalf("GetByName(%q) error = %v", ValueAdjustmentCategoryName, err)
		}
		if !cat.IsSystem {
			t.Error("Value Adjustment should be created as a system category")
		}
		if cat.Type != TypeExpense {
			t.Errorf("Value Adjustment type = %q, want expense", cat.Type)
		}
	})

	t.Run("is idempotent and does not duplicate", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		for i := range 3 {
			collision, err := svc.EnsureValueAdjustmentCategory()
			if err != nil {
				t.Fatalf("EnsureValueAdjustmentCategory() call %d error = %v", i, err)
			}
			if collision {
				t.Errorf("call %d: unexpected user collision", i)
			}
		}

		all, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		count := 0
		for _, c := range all {
			if c.Name == ValueAdjustmentCategoryName {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 Value Adjustment category, got %d", count)
		}
	})

	t.Run("reports a collision and preserves a pre-existing user category", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		// A user creates their own (non-system) category with the same name.
		userCat := NewCategory(ValueAdjustmentCategoryName, TypeExpense)
		if err := svc.Create(userCat); err != nil {
			t.Fatalf("Create user category error = %v", err)
		}

		collision, err := svc.EnsureValueAdjustmentCategory()
		if err != nil {
			t.Fatalf("EnsureValueAdjustmentCategory() error = %v", err)
		}
		if !collision {
			t.Error("expected a user collision to be reported")
		}

		// The user's category must be left untouched (still non-system),
		// and no second one created.
		got, err := svc.GetByName(ValueAdjustmentCategoryName, nil)
		if err != nil {
			t.Fatalf("GetByName error = %v", err)
		}
		if got.ID != userCat.ID {
			t.Error("the pre-existing user category should be preserved")
		}
		if got.IsSystem {
			t.Error("the user category should not have been converted to a system category")
		}
	})
}

func TestService_GetValueAdjustmentCategory(t *testing.T) {
	t.Run("returns not found before seeding", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		if _, err := svc.GetValueAdjustmentCategory(); err == nil {
			t.Error("expected a not-found error before seeding")
		}
	})

	t.Run("returns the system category after seeding", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		if _, err := svc.EnsureValueAdjustmentCategory(); err != nil {
			t.Fatalf("EnsureValueAdjustmentCategory() error = %v", err)
		}
		cat, err := svc.GetValueAdjustmentCategory()
		if err != nil {
			t.Fatalf("GetValueAdjustmentCategory() error = %v", err)
		}
		if !cat.IsSystem || cat.Name != ValueAdjustmentCategoryName {
			t.Errorf("got %+v, want the system Value Adjustment category", cat)
		}
	})

	t.Run("does not confuse Transfer and Value Adjustment", func(t *testing.T) {
		database := createTestDB(t)
		svc := NewService(NewRepository(database), database)

		if err := svc.SeedDefaultCategories(); err != nil {
			t.Fatalf("SeedDefaultCategories() error = %v", err)
		}
		va, err := svc.GetValueAdjustmentCategory()
		if err != nil {
			t.Fatalf("GetValueAdjustmentCategory() error = %v", err)
		}
		transfer, err := svc.GetTransferCategory()
		if err != nil {
			t.Fatalf("GetTransferCategory() error = %v", err)
		}
		if va.ID == transfer.ID {
			t.Error("Value Adjustment and Transfer must be distinct categories")
		}
		if va.Name != ValueAdjustmentCategoryName || transfer.Name != TransferCategoryName {
			t.Error("accessors returned mismatched categories")
		}
	})
}
