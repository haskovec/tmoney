package tui

import tea "charm.land/bubbletea/v2"

// undoResultMsg carries the result of an undo or redo operation.
type undoResultMsg struct {
	action      string // "Undo" or "Redo"
	description string
	err         error
}

// performUndo returns a tea.Cmd that undoes the last operation.
func (a *App) performUndo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	return func() tea.Msg {
		desc, err := a.undoManager.Undo()
		return undoResultMsg{action: "Undo", description: desc, err: err}
	}
}

// performRedo returns a tea.Cmd that redoes the last undone operation.
func (a *App) performRedo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	return func() tea.Msg {
		desc, err := a.undoManager.Redo()
		return undoResultMsg{action: "Redo", description: desc, err: err}
	}
}
