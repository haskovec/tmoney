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
	action := a.aboutDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit, dialog.DialogActionCancel:
		a.aboutDialog.SetVisible(false)
		a.aboutDialog = nil
	}
	return a, nil
}
