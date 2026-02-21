package undo

import (
	"errors"
	"testing"
)

func TestCompoundCommand_Execute(t *testing.T) {
	cmd1 := newMockCommand("Step 1")
	cmd2 := newMockCommand("Step 2")
	cmd3 := newMockCommand("Step 3")

	compound := NewCompoundCommand("Void transfer", cmd1, cmd2, cmd3)

	if err := compound.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if cmd1.executed != 1 || cmd2.executed != 1 || cmd3.executed != 1 {
		t.Error("all sub-commands should be executed once")
	}
}

func TestCompoundCommand_Undo(t *testing.T) {
	cmd1 := newMockCommand("Step 1")
	cmd2 := newMockCommand("Step 2")
	cmd3 := newMockCommand("Step 3")

	compound := NewCompoundCommand("Void transfer", cmd1, cmd2, cmd3)
	_ = compound.Execute()

	if err := compound.Undo(); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if cmd1.undone != 1 || cmd2.undone != 1 || cmd3.undone != 1 {
		t.Error("all sub-commands should be undone once")
	}
}

func TestCompoundCommand_UndoReverseOrder(t *testing.T) {
	var order []string
	cmd1 := newMockCommand("Step 1")
	cmd1.onUndo = func() { order = append(order, "1") }
	cmd2 := newMockCommand("Step 2")
	cmd2.onUndo = func() { order = append(order, "2") }
	cmd3 := newMockCommand("Step 3")
	cmd3.onUndo = func() { order = append(order, "3") }

	compound := NewCompoundCommand("Test", cmd1, cmd2, cmd3)
	_ = compound.Execute()
	_ = compound.Undo()

	if len(order) != 3 {
		t.Fatalf("expected 3 undo calls, got %d", len(order))
	}
	if order[0] != "3" || order[1] != "2" || order[2] != "1" {
		t.Errorf("undo order = %v, want [3, 2, 1]", order)
	}
}

func TestCompoundCommand_ExecutePartialFailureRollback(t *testing.T) {
	cmd1 := newMockCommand("Step 1")
	cmd2 := newMockCommand("Step 2")
	cmd2.execErr = errors.New("step 2 failed")
	cmd3 := newMockCommand("Step 3")

	compound := NewCompoundCommand("Partial failure", cmd1, cmd2, cmd3)

	err := compound.Execute()
	if err == nil {
		t.Fatal("Execute() should return error on partial failure")
	}

	// cmd1 should have been executed then rolled back
	if cmd1.executed != 1 {
		t.Errorf("cmd1 executed %d times, want 1", cmd1.executed)
	}
	if cmd1.undone != 1 {
		t.Errorf("cmd1 should be undone during rollback, got undone=%d", cmd1.undone)
	}

	// cmd2 failed, so it should have been attempted but not undone
	if cmd2.executed != 1 {
		t.Errorf("cmd2 executed %d times, want 1", cmd2.executed)
	}
	if cmd2.undone != 0 {
		t.Errorf("cmd2 should not be undone (it failed), got undone=%d", cmd2.undone)
	}

	// cmd3 should not have been executed at all
	if cmd3.executed != 0 {
		t.Errorf("cmd3 should not be executed after earlier failure, got executed=%d", cmd3.executed)
	}
}

func TestCompoundCommand_ExecuteFirstCommandFails(t *testing.T) {
	cmd1 := newMockCommand("Step 1")
	cmd1.execErr = errors.New("first step failed")
	cmd2 := newMockCommand("Step 2")

	compound := NewCompoundCommand("First fails", cmd1, cmd2)

	err := compound.Execute()
	if err == nil {
		t.Fatal("Execute() should return error")
	}

	// No rollback needed since first command failed
	if cmd1.undone != 0 {
		t.Errorf("cmd1 should not be undone (nothing to rollback), got undone=%d", cmd1.undone)
	}
	if cmd2.executed != 0 {
		t.Errorf("cmd2 should not be executed, got executed=%d", cmd2.executed)
	}
}

func TestCompoundCommand_Description(t *testing.T) {
	compound := NewCompoundCommand("Void transfer")
	if compound.Description() != "Void transfer" {
		t.Errorf("Description() = %q, want %q", compound.Description(), "Void transfer")
	}
}

func TestCompoundCommand_Empty(t *testing.T) {
	compound := NewCompoundCommand("Empty")

	if err := compound.Execute(); err != nil {
		t.Fatalf("Execute() on empty compound should succeed, got %v", err)
	}
	if err := compound.Undo(); err != nil {
		t.Fatalf("Undo() on empty compound should succeed, got %v", err)
	}
}

func TestCompoundCommand_UndoError(t *testing.T) {
	cmd1 := newMockCommand("Step 1")
	cmd2 := newMockCommand("Step 2")
	cmd2.undoErr = errors.New("undo failed")

	compound := NewCompoundCommand("Undo error", cmd1, cmd2)
	_ = compound.Execute()

	err := compound.Undo()
	if err == nil {
		t.Fatal("Undo() should return error when sub-command undo fails")
	}
}

func TestCompoundCommand_WithManager(t *testing.T) {
	m := NewManager()
	cmd1 := newMockCommand("Void A")
	cmd2 := newMockCommand("Void B")

	compound := NewCompoundCommand("Void transfer", cmd1, cmd2)

	if err := m.Execute(compound); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if m.UndoDescription() != "Void transfer" {
		t.Errorf("UndoDescription() = %q, want %q", m.UndoDescription(), "Void transfer")
	}

	desc, err := m.Undo()
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if desc != "Void transfer" {
		t.Errorf("Undo() desc = %q, want %q", desc, "Void transfer")
	}

	if cmd1.undone != 1 || cmd2.undone != 1 {
		t.Error("both sub-commands should be undone")
	}
}
