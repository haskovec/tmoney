package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestApp_HandleConfirmDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	d := NewDialog("Confirm")
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	d.SetVisible(true)
	app.confirmDialog = d
	app.confirmAction = func() tea.Msg { return nil }

	// Press Escape to cancel
	_, _ = app.handleConfirmDialogKey(tea.KeyPressMsg{Code: tea.KeyEsc})

	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil after cancel")
	}
	if app.confirmAction != nil {
		t.Error("confirmAction should be nil after cancel")
	}
}

func TestApp_HandleConfirmDialogKey_Confirm(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	called := false
	d := NewDialog("Confirm")
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	d.SetVisible(true)
	// Focus on the Yes button (fields count = 0, so button index 1 = focus index 1)
	d.SetFocusIndex(1)
	app.confirmDialog = d
	app.confirmAction = func() tea.Msg {
		called = true
		return nil
	}

	// Press Enter on Yes button
	_, cmd := app.handleConfirmDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil after confirm")
	}
	if cmd == nil {
		t.Error("should return a cmd after confirm")
	}

	// Execute the cmd to verify action was captured
	if cmd != nil {
		cmd()
		if !called {
			t.Error("confirm action should have been called")
		}
	}
}
