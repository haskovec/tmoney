package undo

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// mockCommand is a test helper that records Execute/Undo calls.
type mockCommand struct {
	desc      string
	executed  int
	undone    int
	execErr   error
	undoErr   error
	onExecute func()
	onUndo    func()
}

func newMockCommand(desc string) *mockCommand {
	return &mockCommand{desc: desc}
}

func (c *mockCommand) Execute() error {
	c.executed++
	if c.onExecute != nil {
		c.onExecute()
	}
	return c.execErr
}

func (c *mockCommand) Undo() error {
	c.undone++
	if c.onUndo != nil {
		c.onUndo()
	}
	return c.undoErr
}

func (c *mockCommand) Description() string {
	return c.desc
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.CanUndo() {
		t.Error("new manager should not have anything to undo")
	}
	if m.CanRedo() {
		t.Error("new manager should not have anything to redo")
	}
	if m.UndoLen() != 0 {
		t.Error("undo stack should be empty")
	}
	if m.RedoLen() != 0 {
		t.Error("redo stack should be empty")
	}
}

func TestManager_Execute(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Create transaction")

	if err := m.Execute(cmd); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if cmd.executed != 1 {
		t.Errorf("command executed %d times, want 1", cmd.executed)
	}
	if !m.CanUndo() {
		t.Error("should be able to undo after Execute")
	}
	if m.CanRedo() {
		t.Error("should not be able to redo after Execute")
	}
	if m.UndoLen() != 1 {
		t.Errorf("undo stack length = %d, want 1", m.UndoLen())
	}
	if m.UndoDescription() != "Create transaction" {
		t.Errorf("UndoDescription() = %q, want %q", m.UndoDescription(), "Create transaction")
	}
}

func TestManager_ExecuteError(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Fail")
	cmd.execErr = errors.New("execution failed")

	err := m.Execute(cmd)
	if err == nil {
		t.Fatal("Execute() should return error")
	}
	if m.CanUndo() {
		t.Error("should not push failed command onto undo stack")
	}
}

func TestManager_Undo(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Create transaction")
	_ = m.Execute(cmd)

	desc, err := m.Undo()
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if desc != "Create transaction" {
		t.Errorf("Undo() desc = %q, want %q", desc, "Create transaction")
	}
	if cmd.undone != 1 {
		t.Errorf("command undone %d times, want 1", cmd.undone)
	}
	if m.CanUndo() {
		t.Error("should not be able to undo after undoing the only command")
	}
	if !m.CanRedo() {
		t.Error("should be able to redo after undo")
	}
	if m.RedoDescription() != "Create transaction" {
		t.Errorf("RedoDescription() = %q, want %q", m.RedoDescription(), "Create transaction")
	}
}

func TestManager_UndoEmpty(t *testing.T) {
	m := NewManager()

	_, err := m.Undo()
	if !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("Undo() error = %v, want ErrNothingToUndo", err)
	}
}

func TestManager_UndoError(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Fail undo")
	cmd.undoErr = errors.New("undo failed")
	_ = m.Execute(cmd)

	_, err := m.Undo()
	if err == nil {
		t.Fatal("Undo() should return error when undo fails")
	}
	// Command should remain on the undo stack since undo failed
	if !m.CanUndo() {
		t.Error("failed undo should leave command on undo stack")
	}
	if m.CanRedo() {
		t.Error("failed undo should not push to redo stack")
	}
}

func TestManager_Redo(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Create transaction")
	_ = m.Execute(cmd)
	_, _ = m.Undo()

	desc, err := m.Redo()
	if err != nil {
		t.Fatalf("Redo() error = %v", err)
	}
	if desc != "Create transaction" {
		t.Errorf("Redo() desc = %q, want %q", desc, "Create transaction")
	}
	if cmd.executed != 2 {
		t.Errorf("command executed %d times, want 2 (original + redo)", cmd.executed)
	}
	if !m.CanUndo() {
		t.Error("should be able to undo after redo")
	}
	if m.CanRedo() {
		t.Error("should not be able to redo after redoing the only command")
	}
}

func TestManager_RedoEmpty(t *testing.T) {
	m := NewManager()

	_, err := m.Redo()
	if !errors.Is(err, ErrNothingToRedo) {
		t.Errorf("Redo() error = %v, want ErrNothingToRedo", err)
	}
}

func TestManager_RedoError(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Fail redo")
	_ = m.Execute(cmd)
	// Reset the error to nil for undo, then set for redo
	_, _ = m.Undo()
	cmd.execErr = errors.New("redo failed")

	_, err := m.Redo()
	if err == nil {
		t.Fatal("Redo() should return error when re-execute fails")
	}
	// Command should remain on the redo stack since redo failed
	if !m.CanRedo() {
		t.Error("failed redo should leave command on redo stack")
	}
	if m.CanUndo() {
		t.Error("failed redo should not push to undo stack")
	}
}

func TestManager_NewOperationClearsRedoStack(t *testing.T) {
	m := NewManager()
	cmd1 := newMockCommand("First")
	cmd2 := newMockCommand("Second")
	_ = m.Execute(cmd1)
	_, _ = m.Undo()

	if !m.CanRedo() {
		t.Fatal("should be able to redo before new operation")
	}

	_ = m.Execute(cmd2)

	if m.CanRedo() {
		t.Error("new operation should clear redo stack")
	}
	if m.UndoLen() != 1 {
		t.Errorf("undo stack should have 1 item, got %d", m.UndoLen())
	}
	if m.UndoDescription() != "Second" {
		t.Errorf("top of undo stack should be %q, got %q", "Second", m.UndoDescription())
	}
}

func TestManager_MultipleUndoRedo(t *testing.T) {
	m := NewManager()
	cmd1 := newMockCommand("First")
	cmd2 := newMockCommand("Second")
	cmd3 := newMockCommand("Third")
	_ = m.Execute(cmd1)
	_ = m.Execute(cmd2)
	_ = m.Execute(cmd3)

	if m.UndoLen() != 3 {
		t.Fatalf("undo stack length = %d, want 3", m.UndoLen())
	}

	// Undo all three
	desc, _ := m.Undo()
	if desc != "Third" {
		t.Errorf("first undo desc = %q, want %q", desc, "Third")
	}
	desc, _ = m.Undo()
	if desc != "Second" {
		t.Errorf("second undo desc = %q, want %q", desc, "Second")
	}
	desc, _ = m.Undo()
	if desc != "First" {
		t.Errorf("third undo desc = %q, want %q", desc, "First")
	}

	if m.CanUndo() {
		t.Error("should not be able to undo after undoing all commands")
	}
	if m.RedoLen() != 3 {
		t.Fatalf("redo stack length = %d, want 3", m.RedoLen())
	}

	// Redo all three
	desc, _ = m.Redo()
	if desc != "First" {
		t.Errorf("first redo desc = %q, want %q", desc, "First")
	}
	desc, _ = m.Redo()
	if desc != "Second" {
		t.Errorf("second redo desc = %q, want %q", desc, "Second")
	}
	desc, _ = m.Redo()
	if desc != "Third" {
		t.Errorf("third redo desc = %q, want %q", desc, "Third")
	}

	if m.CanRedo() {
		t.Error("should not be able to redo after redoing all commands")
	}
	if m.UndoLen() != 3 {
		t.Errorf("undo stack length = %d, want 3", m.UndoLen())
	}
}

func TestManager_Clear(t *testing.T) {
	m := NewManager()
	_ = m.Execute(newMockCommand("A"))
	_ = m.Execute(newMockCommand("B"))
	_, _ = m.Undo()

	m.Clear()

	if m.CanUndo() {
		t.Error("Clear() should empty undo stack")
	}
	if m.CanRedo() {
		t.Error("Clear() should empty redo stack")
	}
	if m.UndoLen() != 0 {
		t.Errorf("UndoLen() = %d, want 0", m.UndoLen())
	}
	if m.RedoLen() != 0 {
		t.Errorf("RedoLen() = %d, want 0", m.RedoLen())
	}
}

func TestManager_Push(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Auto-post")

	m.Push(cmd)

	// Should not have executed the command
	if cmd.executed != 0 {
		t.Errorf("Push should not execute command, got %d executions", cmd.executed)
	}
	if !m.CanUndo() {
		t.Error("should be able to undo after Push")
	}
	if m.UndoLen() != 1 {
		t.Errorf("undo stack length = %d, want 1", m.UndoLen())
	}
	if m.UndoDescription() != "Auto-post" {
		t.Errorf("UndoDescription() = %q, want %q", m.UndoDescription(), "Auto-post")
	}
}

func TestManager_PushClearsRedoStack(t *testing.T) {
	m := NewManager()
	cmd1 := newMockCommand("First")
	_ = m.Execute(cmd1)
	_, _ = m.Undo()

	if !m.CanRedo() {
		t.Fatal("should be able to redo before Push")
	}

	cmd2 := newMockCommand("Pushed")
	m.Push(cmd2)

	if m.CanRedo() {
		t.Error("Push should clear redo stack")
	}
}

func TestManager_PushThenUndo(t *testing.T) {
	m := NewManager()
	cmd := newMockCommand("Pushed command")
	m.Push(cmd)

	desc, err := m.Undo()
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if desc != "Pushed command" {
		t.Errorf("Undo() desc = %q, want %q", desc, "Pushed command")
	}
	if cmd.undone != 1 {
		t.Errorf("command undone %d times, want 1", cmd.undone)
	}
	if !m.CanRedo() {
		t.Error("should be able to redo after undoing pushed command")
	}
}

func TestManager_DescriptionEmpty(t *testing.T) {
	m := NewManager()
	if m.UndoDescription() != "" {
		t.Errorf("UndoDescription() on empty stack = %q, want empty", m.UndoDescription())
	}
	if m.RedoDescription() != "" {
		t.Errorf("RedoDescription() on empty stack = %q, want empty", m.RedoDescription())
	}
}

// TestManager_ConcurrentExecute: write commands run Execute in their own
// goroutines while Update reads the stacks. No step may be lost. Run it with
// -race too.
func TestManager_ConcurrentExecute(t *testing.T) {
	m := NewManager()
	const writers, each = 8, 50
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range each {
				if err := m.Execute(newMockCommand("Fictional write")); err != nil {
					t.Error(err)
				}
			}
		})
	}
	done := make(chan struct{})
	read := make(chan struct{})
	go func() { // Update, which reads the stacks meanwhile
		defer close(read)
		for {
			select {
			case <-done:
				return
			default:
				m.CanUndo()
				m.UndoDescription()
			}
		}
	}()
	wg.Wait()
	close(done)
	<-read
	if got := m.UndoLen(); got != writers*each {
		t.Errorf("UndoLen() = %d after %d writes, want each one on the stack", got, writers*each)
	}
}

// TestManager_AWriteWaitsForAnUndoInProgress: an undo and a write run at the
// same time. The write must wait until the undo is done. If it does not, the
// undo puts its step on the redo stack after the write cleared that stack,
// and a Redo then applies a step from before the write.
func TestManager_AWriteWaitsForAnUndoInProgress(t *testing.T) {
	m := NewManager()
	first := newMockCommand("First")
	if err := m.Execute(first); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	first.onUndo = func() { close(entered); <-release }

	undone := make(chan error, 1)
	go func() { _, err := m.Undo(); undone <- err }()
	<-entered
	wrote := make(chan error, 1)
	go func() { wrote <- m.Execute(newMockCommand("Second")) }()
	select {
	case err := <-wrote: // the write did not wait
		wrote <- err
	case <-time.After(50 * time.Millisecond):
	}
	close(release)

	if err := <-undone; err != nil {
		t.Fatal(err)
	}
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	if m.CanRedo() {
		t.Errorf("the redo stack has %q after a later write; a write discards the redo steps", m.RedoDescription())
	}
	if got := m.UndoDescription(); got != "Second" {
		t.Errorf("UndoDescription() = %q, want %q", got, "Second")
	}
}
