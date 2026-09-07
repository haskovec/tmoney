package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/undo"
)

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

// applyUndoResult reports the outcome of an undo or redo. An empty stack is a
// note, not an error; a real failure becomes a.err. A success reloads the
// active view, because the reverted command may have touched anything on it.
func (a *App) applyUndoResult(msg undoResultMsg) tea.Cmd {
	switch {
	case errors.Is(msg.err, undo.ErrNothingToUndo):
		a.statusbar.AddNotification("Nothing to undo", widget.NotificationInfo)
	case errors.Is(msg.err, undo.ErrNothingToRedo):
		a.statusbar.AddNotification("Nothing to redo", widget.NotificationInfo)
	case msg.err != nil:
		a.err = msg.err
	default:
		a.statusbar.AddNotification(
			fmt.Sprintf("%s: %s", msg.action, msg.description),
			widget.NotificationInfo,
		)
		return a.reloadCurrentView()
	}
	return nil
}
