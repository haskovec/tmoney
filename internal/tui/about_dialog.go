package tui

import (
	"github.com/haskovec/tmoney/internal/tui/dialog"

	tea "charm.land/bubbletea/v2"
)

// showAboutDialog displays the Help → About dialog.
func (a *App) showAboutDialog() {
	d := dialog.NewDialog("Terminal Money")
	d.SetWidth(44)
	d.SetMessage("Author: Jeffrey Haskovec\nCopyright 2026")
	d.SetButtons([]dialog.DialogButton{
		{Label: "OK", Primary: true},
	})
	d.SetFocusIndex(len(d.Fields())) // focus the OK button
	d.SetVisible(true)
	a.aboutDialog = d
}

// handleAboutDialogKey handles key input for the About dialog.
func (a *App) handleAboutDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.aboutDialogAction(a.aboutDialog.HandleKey(msg))
}

// aboutDialogAction dispatches a DialogAction for the about dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) aboutDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit, dialog.DialogActionCancel:
		a.aboutDialog.SetVisible(false)
		a.aboutDialog = nil
	}
	return a, nil
}
