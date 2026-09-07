package tui

import (
	"github.com/haskovec/tmoney/internal/tui/dialog"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// showConfirmDialog displays a confirmation dialog with the given title and message.
// The action function is called when the user confirms.
func (a *App) showConfirmDialog(title, message string, action func() tea.Msg) {
	d := dialog.NewDialog(title)
	d.SetWidth(50)
	d.SetButtons([]dialog.DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	// Render the prompt in the neutral message body (newline-separated),
	// pre-wrapped to the content width, NOT the error area. The error area is
	// counted as a single line by the layout/hit-test, so a multi-line prompt
	// there misplaces the button row and breaks mouse clicks; the message body
	// is line-counted correctly.
	contentWidth := max(50-dialog.DialogHorizontalOverhead, 10)
	d.SetMessage(lipgloss.NewStyle().Width(contentWidth).Render(message))
	d.SetFocusIndex(len(d.Fields())) // Focus on first button (No)
	d.SetVisible(true)
	a.confirmDialog = d
	a.confirmAction = action
}

// handleConfirmDialogKey handles key input for the confirmation dialog.
func (a *App) handleConfirmDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.confirmDialogAction(a.confirmDialog.HandleKey(msg))
}

// confirmDialogAction dispatches a DialogAction for the confirm dialog, from either input path.
func (a *App) confirmDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		a.confirmDialog.SetVisible(false)
		fn := a.confirmAction
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, func() tea.Msg {
			return fn()
		}
	case dialog.DialogActionCancel:
		a.confirmDialog.SetVisible(false)
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, nil
	}

	return a, nil
}
