package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

type payeeTestEnv struct {
	payeeSvc  *payee.Service
	payeeRepo *payee.Repository
}

func createPayeeTestEnv(t *testing.T) *payeeTestEnv {
	t.Helper()
	database := createTestDB(t)
	payeeRepo := payee.NewRepository(database)
	payeeSvc := payee.NewService(payeeRepo, database)
	return &payeeTestEnv{
		payeeSvc:  payeeSvc,
		payeeRepo: payeeRepo,
	}
}

// =============================================================================
// CreatePayeeCommand Tests
// =============================================================================

func TestCreatePayeeCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes payee", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		py := payee.NewPayee("Coffee Shop")

		cmd := undo.NewCreatePayeeCommand(env.payeeSvc, py)

		// Execute: payee should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Coffee Shop" {
			t.Errorf("name = %q, want %q", retrieved.Name, "Coffee Shop")
		}

		// Undo: payee should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.payeeSvc.GetByID(py.ID)
		if err == nil {
			t.Error("expected error after Undo (payee should be deleted)")
		}
	})
}

func TestCreatePayeeCommand_WithDefaultCategory(t *testing.T) {
	t.Run("creates payee with default category and undoes", func(t *testing.T) {
		database := createTestDB(t)
		categoryRepo := category.NewRepository(database)
		payeeRepo := payee.NewRepository(database)
		payeeSvc := payee.NewService(payeeRepo, database)

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		py := payee.NewPayeeWithCategory("Grocery Store", cat.ID)

		cmd := undo.NewCreatePayeeCommand(payeeSvc, py)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !retrieved.HasDefaultCategory() {
			t.Error("payee should have default category")
		}

		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = payeeSvc.GetByID(py.ID)
		if err == nil {
			t.Error("expected error after Undo (payee should be deleted)")
		}
	})
}

func TestCreatePayeeCommand_Description(t *testing.T) {
	cmd := undo.NewCreatePayeeCommand(nil, nil)
	if cmd.Description() != "Create payee" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create payee")
	}
}

func TestCreatePayeeCommand_WithManager(t *testing.T) {
	t.Run("works with undo manager execute and undo", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		py := payee.NewPayee("Gas Station")

		mgr := undo.NewManager()
		cmd := undo.NewCreatePayeeCommand(env.payeeSvc, py)

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
		if desc != "Create payee" {
			t.Errorf("undo desc = %q, want %q", desc, "Create payee")
		}

		_, err = env.payeeSvc.GetByID(py.ID)
		if err == nil {
			t.Error("payee should not exist after undo")
		}

		// Redo should recreate
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Create payee" {
			t.Errorf("redo desc = %q, want %q", desc, "Create payee")
		}

		retrieved, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if retrieved.Name != "Gas Station" {
			t.Errorf("name after redo = %q, want %q", retrieved.Name, "Gas Station")
		}
	})
}

// =============================================================================
// EditPayeeCommand Tests
// =============================================================================

func TestEditPayeeCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		py := payee.NewPayee("Coffee Shop")
		py.SetNotes("Original notes")
		if err := env.payeeSvc.Create(py); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		edited.Name = "Updated Coffee Shop"
		edited.SetNotes("Updated notes")

		cmd := undo.NewEditPayeeCommand(env.payeeSvc, edited)

		// Execute: should be edited
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Updated Coffee Shop" {
			t.Errorf("name after edit = %q, want %q", retrieved.Name, "Updated Coffee Shop")
		}
		if retrieved.Notes.String != "Updated notes" {
			t.Errorf("notes after edit = %q, want %q", retrieved.Notes.String, "Updated notes")
		}

		// Undo: should restore original
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Coffee Shop" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Coffee Shop")
		}
		if restored.Notes.String != "Original notes" {
			t.Errorf("notes after undo = %q, want %q", restored.Notes.String, "Original notes")
		}
	})
}

func TestEditPayeeCommand_Description(t *testing.T) {
	cmd := undo.NewEditPayeeCommand(nil, nil)
	if cmd.Description() != "Edit payee" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit payee")
	}
}

// =============================================================================
// DeletePayeeCommand Tests
// =============================================================================

func TestDeletePayeeCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes and then recreates payee", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		py := payee.NewPayee("Coffee Shop")
		py.SetNotes("Test notes")
		if err := env.payeeSvc.Create(py); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewDeletePayeeCommand(env.payeeSvc, py.ID)

		// Execute: payee should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.payeeSvc.GetByID(py.ID)
		if err == nil {
			t.Error("expected error after Execute (payee should be deleted)")
		}

		// Undo: payee should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Coffee Shop" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Coffee Shop")
		}
		if restored.Notes.String != "Test notes" {
			t.Errorf("notes after undo = %q, want %q", restored.Notes.String, "Test notes")
		}
	})
}

func TestDeletePayeeCommand_WithAliases(t *testing.T) {
	t.Run("deletes payee with aliases and restores both on undo", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		py := payee.NewPayee("Coffee Shop")
		if err := env.payeeSvc.Create(py); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias1 := payee.NewExactAlias(py.ID, "COFFEE SHOP #123")
		alias2 := payee.NewContainsAlias(py.ID, "coffee")
		if err := env.payeeSvc.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias(1) error = %v", err)
		}
		if err := env.payeeSvc.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias(2) error = %v", err)
		}

		cmd := undo.NewDeletePayeeCommand(env.payeeSvc, py.ID)

		// Execute: payee and aliases should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.payeeSvc.GetByID(py.ID)
		if err == nil {
			t.Error("expected error after Execute (payee should be deleted)")
		}

		// Undo: payee and aliases should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.payeeSvc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Coffee Shop" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Coffee Shop")
		}

		aliases, err := env.payeeSvc.GetAliasesByPayee(py.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() after Undo error = %v", err)
		}
		if len(aliases) != 2 {
			t.Errorf("expected 2 aliases after undo, got %d", len(aliases))
		}
	})
}

func TestDeletePayeeCommand_Description(t *testing.T) {
	cmd := undo.NewDeletePayeeCommand(nil, types.NewID())
	if cmd.Description() != "Delete payee" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete payee")
	}
}

// =============================================================================
// MergePayeesCommand Tests
// =============================================================================

func TestMergePayeesCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("merges payees and undo returns error", func(t *testing.T) {
		env := createPayeeTestEnv(t)

		source := payee.NewPayee("Coffee House")
		if err := env.payeeSvc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}

		target := payee.NewPayee("Coffee Shop")
		if err := env.payeeSvc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		cmd := undo.NewMergePayeesCommand(env.payeeSvc, source.ID, target.ID)

		// Execute: source should be merged into target
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		// Source should be gone
		_, err := env.payeeSvc.GetByID(source.ID)
		if err == nil {
			t.Error("source payee should not exist after merge")
		}

		// Target should still exist
		_, err = env.payeeSvc.GetByID(target.ID)
		if err != nil {
			t.Fatalf("target payee should still exist after merge: %v", err)
		}

		// Undo: should return error (merge is not reversible)
		if err := cmd.Undo(); err == nil {
			t.Error("Undo() should return error for merge operation")
		}
	})
}

func TestMergePayeesCommand_Description(t *testing.T) {
	cmd := undo.NewMergePayeesCommand(nil, types.NewID(), types.NewID())
	if cmd.Description() != "Merge payees" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Merge payees")
	}
}
