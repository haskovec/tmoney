package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

type categoryTestEnv struct {
	categorySvc  *category.Service
	categoryRepo *category.Repository
}

func createCategoryTestEnv(t *testing.T) *categoryTestEnv {
	t.Helper()
	database := createTestDB(t)
	categoryRepo := category.NewRepository(database)
	categorySvc := category.NewService(categoryRepo, database)
	return &categoryTestEnv{
		categorySvc:  categorySvc,
		categoryRepo: categoryRepo,
	}
}

// =============================================================================
// CreateCategoryCommand Tests
// =============================================================================

func TestCreateCategoryCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes category", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		cat := category.NewCategory("Food", category.TypeExpense)

		cmd := undo.NewCreateCategoryCommand(env.categorySvc, cat)

		// Execute: category should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Food" {
			t.Errorf("name = %q, want %q", retrieved.Name, "Food")
		}
		if retrieved.Type != category.TypeExpense {
			t.Errorf("type = %q, want %q", retrieved.Type, category.TypeExpense)
		}

		// Undo: category should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.categorySvc.GetByID(cat.ID)
		if err == nil {
			t.Error("expected error after Undo (category should be deleted)")
		}
	})
}

func TestCreateCategoryCommand_Subcategory(t *testing.T) {
	t.Run("creates and undoes subcategory", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		parent := category.NewCategory("Food", category.TypeExpense)
		if err := env.categorySvc.Create(parent); err != nil {
			t.Fatalf("Create parent error = %v", err)
		}

		child := category.NewSubcategory("Groceries", parent.ID, category.TypeExpense)

		cmd := undo.NewCreateCategoryCommand(env.categorySvc, child)

		// Execute: subcategory should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.categorySvc.GetByID(child.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !retrieved.IsSubcategory() {
			t.Error("expected subcategory")
		}

		// Undo: subcategory should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.categorySvc.GetByID(child.ID)
		if err == nil {
			t.Error("expected error after Undo (subcategory should be deleted)")
		}
	})
}

func TestCreateCategoryCommand_Description(t *testing.T) {
	cmd := undo.NewCreateCategoryCommand(nil, nil)
	if cmd.Description() != "Create category" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create category")
	}
}

func TestCreateCategoryCommand_WithManager(t *testing.T) {
	t.Run("works with undo manager execute and undo", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		cat := category.NewCategory("Entertainment", category.TypeExpense)

		mgr := undo.NewManager()
		cmd := undo.NewCreateCategoryCommand(env.categorySvc, cat)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		if !mgr.CanUndo() {
			t.Error("should be able to undo after execute")
		}

		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Create category" {
			t.Errorf("undo desc = %q, want %q", desc, "Create category")
		}

		_, err = env.categorySvc.GetByID(cat.ID)
		if err == nil {
			t.Error("category should not exist after undo")
		}

		// Redo should recreate
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Create category" {
			t.Errorf("redo desc = %q, want %q", desc, "Create category")
		}

		retrieved, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if retrieved.Name != "Entertainment" {
			t.Errorf("name after redo = %q, want %q", retrieved.Name, "Entertainment")
		}
	})
}

// =============================================================================
// EditCategoryCommand Tests
// =============================================================================

func TestEditCategoryCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := env.categorySvc.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		edited.Name = "Dining"

		cmd := undo.NewEditCategoryCommand(env.categorySvc, edited)

		// Execute: should be edited
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Dining" {
			t.Errorf("name after edit = %q, want %q", retrieved.Name, "Dining")
		}

		// Undo: should restore original
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Food" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Food")
		}
	})
}

func TestEditCategoryCommand_Description(t *testing.T) {
	cmd := undo.NewEditCategoryCommand(nil, nil)
	if cmd.Description() != "Edit category" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit category")
	}
}

// =============================================================================
// DeleteCategoryCommand Tests
// =============================================================================

func TestDeleteCategoryCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes and then recreates category", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := env.categorySvc.Create(cat); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewDeleteCategoryCommand(env.categorySvc, cat.ID)

		// Execute: category should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.categorySvc.GetByID(cat.ID)
		if err == nil {
			t.Error("expected error after Execute (category should be deleted)")
		}

		// Undo: category should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.categorySvc.GetByID(cat.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Food" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Food")
		}
		if restored.Type != category.TypeExpense {
			t.Errorf("type after undo = %q, want %q", restored.Type, category.TypeExpense)
		}
	})
}

func TestDeleteCategoryCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteCategoryCommand(nil, types.NewID())
	if cmd.Description() != "Delete category" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete category")
	}
}

// =============================================================================
// MergeCategoriesCommand Tests
// =============================================================================

func TestMergeCategoriesCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("merges categories and undo returns error", func(t *testing.T) {
		env := createCategoryTestEnv(t)

		source := category.NewCategory("Food", category.TypeExpense)
		if err := env.categorySvc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}

		target := category.NewCategory("Dining", category.TypeExpense)
		if err := env.categorySvc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		cmd := undo.NewMergeCategoriesCommand(env.categorySvc, source.ID, target.ID)

		// Execute: source should be merged into target
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		// Source should be gone
		_, err := env.categorySvc.GetByID(source.ID)
		if err == nil {
			t.Error("source category should not exist after merge")
		}

		// Target should still exist
		_, err = env.categorySvc.GetByID(target.ID)
		if err != nil {
			t.Fatalf("target category should still exist after merge: %v", err)
		}

		// Undo: should return error (merge is not reversible)
		if err := cmd.Undo(); err == nil {
			t.Error("Undo() should return error for merge operation")
		}
	})
}

func TestMergeCategoriesCommand_Description(t *testing.T) {
	cmd := undo.NewMergeCategoriesCommand(nil, types.NewID(), types.NewID())
	if cmd.Description() != "Merge categories" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Merge categories")
	}
}
