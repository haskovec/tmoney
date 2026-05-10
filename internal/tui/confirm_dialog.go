package tui

import tea "charm.land/bubbletea/v2"

// showConfirmDialog displays a confirmation dialog with the given title and message.
// The action function is called when the user confirms.
func (a *App) showConfirmDialog(title, message string, action func() tea.Msg) {
	d := NewDialog(title)
	d.SetWidth(50)
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	// Use a text field with the message as a label (read-only visual)
	// We'll render the message as the dialog error message area (repurposed for display)
	d.SetErrorMsg(message)
	d.SetFocusIndex(len(d.Fields())) // Focus on first button (No)
	d.SetVisible(true)
	a.confirmDialog = d
	a.confirmAction = action
}

// handleConfirmDialogKey handles key input for the confirmation dialog.
func (a *App) handleConfirmDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.confirmDialog.HandleKey(msg)

	switch action {
	case DialogActionSubmit:
		a.confirmDialog.SetVisible(false)
		fn := a.confirmAction
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, func() tea.Msg {
			return fn()
		}
	case DialogActionCancel:
		a.confirmDialog.SetVisible(false)
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, nil
	}

	return a, nil
}
