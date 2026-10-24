package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/undo"
)

// undoResultMsg carries the result of an undo or redo operation. A success
// carries the tag of its file, as the success of a write does; a failure is
// not tagged, so that it shows also after a switch.
type undoResultMsg struct {
	action      string // "Undo" or "Redo"
	description string
	err         error
	fileOf
}

// performUndo returns a tea.Cmd that undoes the last operation.
func (a *App) performUndo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	undoManager, file := a.undoManager, a.fileOfOpen()
	return func() tea.Msg {
		desc, err := undoManager.Undo()
		return undoResult("Undo", desc, err, file)
	}
}

// performRedo returns a tea.Cmd that redoes the last undone operation.
func (a *App) performRedo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	undoManager, file := a.undoManager, a.fileOfOpen()
	return func() tea.Msg {
		desc, err := undoManager.Redo()
		return undoResult("Redo", desc, err, file)
	}
}

// undoResult makes the message of an undo or redo of file.
func undoResult(action, desc string, err error, file fileOf) undoResultMsg {
	msg := undoResultMsg{action: action, description: desc, err: err}
	if err == nil {
		msg.fileOf = file
	}
	return msg
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
