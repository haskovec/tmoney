package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

type accountTestEnv struct {
	accountSvc  *account.Service
	accountRepo *account.Repository
}

func createAccountTestEnv(t *testing.T) *accountTestEnv {
	t.Helper()
	database := createTestDB(t)
	accountRepo := account.NewRepository(database)
	accountSvc := account.NewService(accountRepo, database)
	return &accountTestEnv{
		accountSvc:  accountSvc,
		accountRepo: accountRepo,
	}
}

// =============================================================================
// CreateAccountCommand Tests
// =============================================================================

func TestCreateAccountCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes account", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())

		cmd := undo.NewCreateAccountCommand(env.accountSvc, acct)

		// Execute: account should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Checking" {
			t.Errorf("name = %q, want %q", retrieved.Name, "Checking")
		}

		// Undo: account should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.accountSvc.GetByID(acct.ID)
		if err == nil {
			t.Error("expected error after Undo (account should be deleted)")
		}
	})
}

func TestCreateAccountCommand_Description(t *testing.T) {
	cmd := undo.NewCreateAccountCommand(nil, nil)
	if cmd.Description() != "Create account" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create account")
	}
}

func TestCreateAccountCommand_WithManager(t *testing.T) {
	t.Run("works with undo manager execute and undo", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today())

		mgr := undo.NewManager()
		cmd := undo.NewCreateAccountCommand(env.accountSvc, acct)

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
		if desc != "Create account" {
			t.Errorf("undo desc = %q, want %q", desc, "Create account")
		}

		_, err = env.accountSvc.GetByID(acct.ID)
		if err == nil {
			t.Error("account should not exist after undo")
		}

		// Redo should recreate
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Create account" {
			t.Errorf("redo desc = %q, want %q", desc, "Create account")
		}

		retrieved, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if retrieved.Name != "Savings" {
			t.Errorf("name after redo = %q, want %q", retrieved.Name, "Savings")
		}
	})
}

// =============================================================================
// EditAccountCommand Tests
// =============================================================================

func TestEditAccountCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
		acct.SetNotes("Original notes")
		if err := env.accountSvc.Create(acct); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		edited.Name = "Updated Checking"
		edited.SetNotes("Updated notes")

		cmd := undo.NewEditAccountCommand(env.accountSvc, edited)

		// Execute: should be edited
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Name != "Updated Checking" {
			t.Errorf("name after edit = %q, want %q", retrieved.Name, "Updated Checking")
		}
		if retrieved.Notes.String != "Updated notes" {
			t.Errorf("notes after edit = %q, want %q", retrieved.Notes.String, "Updated notes")
		}

		// Undo: should restore original
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Checking" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Checking")
		}
		if restored.Notes.String != "Original notes" {
			t.Errorf("notes after undo = %q, want %q", restored.Notes.String, "Original notes")
		}
	})
}

func TestEditAccountCommand_Description(t *testing.T) {
	cmd := undo.NewEditAccountCommand(nil, nil)
	if cmd.Description() != "Edit account" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit account")
	}
}

// =============================================================================
// DeleteAccountCommand Tests
// =============================================================================

func TestDeleteAccountCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes and then recreates account", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
		acct.SetNotes("Test notes")
		if err := env.accountSvc.Create(acct); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewDeleteAccountCommand(env.accountSvc, acct.ID)

		// Execute: account should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.accountSvc.GetByID(acct.ID)
		if err == nil {
			t.Error("expected error after Execute (account should be deleted)")
		}

		// Undo: account should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restored.Name != "Checking" {
			t.Errorf("name after undo = %q, want %q", restored.Name, "Checking")
		}
		if restored.Notes.String != "Test notes" {
			t.Errorf("notes after undo = %q, want %q", restored.Notes.String, "Test notes")
		}
	})
}

func TestDeleteAccountCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteAccountCommand(nil, types.NewID())
	if cmd.Description() != "Delete account" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete account")
	}
}

// =============================================================================
// CloseAccountCommand Tests
// =============================================================================

func TestCloseAccountCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("closes and then reopens account", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := env.accountSvc.Create(acct); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewCloseAccountCommand(env.accountSvc, acct.ID)

		// Execute: account should be closed
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		closed, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after close error = %v", err)
		}
		if closed.Active {
			t.Error("account should be inactive after close")
		}

		// Undo: account should be reopened
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		reopened, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !reopened.Active {
			t.Error("account should be active after undo")
		}
	})
}

func TestCloseAccountCommand_Description(t *testing.T) {
	cmd := undo.NewCloseAccountCommand(nil, types.NewID())
	if cmd.Description() != "Close account" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Close account")
	}
}

// =============================================================================
// ReopenAccountCommand Tests
// =============================================================================

func TestReopenAccountCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("reopens and then closes account", func(t *testing.T) {
		env := createAccountTestEnv(t)

		acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := env.accountSvc.Create(acct); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		// Close the account first
		if err := env.accountSvc.Close(acct.ID); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		cmd := undo.NewReopenAccountCommand(env.accountSvc, acct.ID)

		// Execute: account should be reopened
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		reopened, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after reopen error = %v", err)
		}
		if !reopened.Active {
			t.Error("account should be active after reopen")
		}

		// Undo: account should be closed again
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		closed, err := env.accountSvc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if closed.Active {
			t.Error("account should be inactive after undo")
		}
	})
}

func TestReopenAccountCommand_Description(t *testing.T) {
	cmd := undo.NewReopenAccountCommand(nil, types.NewID())
	if cmd.Description() != "Reopen account" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Reopen account")
	}
}
