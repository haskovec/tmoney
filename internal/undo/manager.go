package undo

import "errors"

// Sentinel errors for undo/redo operations.
var (
	ErrNothingToUndo = errors.New("nothing to undo")
	ErrNothingToRedo = errors.New("nothing to redo")
)

// Manager maintains undo and redo stacks for the current session.
// The stacks are not persisted — they are cleared when the application exits.
type Manager struct {
	undoStack []Command
	redoStack []Command
}

// NewManager creates a new undo/redo manager.
func NewManager() *Manager {
	return &Manager{}
}

// Execute runs a command and pushes it onto the undo stack.
// Any pending redo operations are discarded.
func (m *Manager) Execute(cmd Command) error {
	if err := cmd.Execute(); err != nil {
		return err
	}
	m.undoStack = append(m.undoStack, cmd)
	m.redoStack = nil
	return nil
}

// Undo reverses the most recent operation. Returns the command description
// on success.
func (m *Manager) Undo() (string, error) {
	if len(m.undoStack) == 0 {
		return "", ErrNothingToUndo
	}
	cmd := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	if err := cmd.Undo(); err != nil {
		// Undo failed — push it back so the stack is not corrupted
		m.undoStack = append(m.undoStack, cmd)
		return "", err
	}
	m.redoStack = append(m.redoStack, cmd)
	return cmd.Description(), nil
}

// Redo re-applies the most recently undone operation. Returns the command
// description on success.
func (m *Manager) Redo() (string, error) {
	if len(m.redoStack) == 0 {
		return "", ErrNothingToRedo
	}
	cmd := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	if err := cmd.Execute(); err != nil {
		// Redo failed — push it back so the stack is not corrupted
		m.redoStack = append(m.redoStack, cmd)
		return "", err
	}
	m.undoStack = append(m.undoStack, cmd)
	return cmd.Description(), nil
}

// CanUndo returns true if there is at least one operation to undo.
func (m *Manager) CanUndo() bool {
	return len(m.undoStack) > 0
}

// CanRedo returns true if there is at least one operation to redo.
func (m *Manager) CanRedo() bool {
	return len(m.redoStack) > 0
}

// UndoDescription returns the description of the operation that would be
// undone, or an empty string if the undo stack is empty.
func (m *Manager) UndoDescription() string {
	if len(m.undoStack) == 0 {
		return ""
	}
	return m.undoStack[len(m.undoStack)-1].Description()
}

// RedoDescription returns the description of the operation that would be
// redone, or an empty string if the redo stack is empty.
func (m *Manager) RedoDescription() string {
	if len(m.redoStack) == 0 {
		return ""
	}
	return m.redoStack[len(m.redoStack)-1].Description()
}

// Clear empties both the undo and redo stacks.
func (m *Manager) Clear() {
	m.undoStack = nil
	m.redoStack = nil
}

// UndoLen returns the number of commands on the undo stack.
func (m *Manager) UndoLen() int {
	return len(m.undoStack)
}

// RedoLen returns the number of commands on the redo stack.
func (m *Manager) RedoLen() int {
	return len(m.redoStack)
}
