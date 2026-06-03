package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

func TestApp_HandleConfirmDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	d := dialog.NewDialog("Confirm")
	d.SetButtons([]dialog.DialogButton{
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
		statusbar:   widget.NewStatusBar(),
	}

	called := false
	d := dialog.NewDialog("Confirm")
	d.SetButtons([]dialog.DialogButton{
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

// Regression: a long confirm prompt must render in the line-counted message
// body (not the error area, which the layout treats as a single line and which
// misplaced the button row so mouse clicks missed). The button row at its
// layout position must hit-test to the Yes (primary) button.
func TestApp_ShowConfirmDialog_WrappedPromptIsMouseClickable(t *testing.T) {
	app := &App{statusbar: widget.NewStatusBar()}
	long := "Reverse this Spin-Off on ETHE (2024-07-23) and delete the audit row? Lots, positions, and prices will be restored to their pre-action state."
	app.showConfirmDialog("Reverse Corporate Action", long, func() tea.Msg { return nil })

	d := app.confirmDialog
	if d == nil {
		t.Fatal("confirm dialog not set")
	}
	if !strings.Contains(d.Message(), "\n") {
		t.Fatalf("long prompt should be wrapped into the multi-line message body, got %q", d.Message())
	}

	contentWidth := max(50-dialog.DialogHorizontalOverhead, 10)
	msgRows := strings.Count(d.Message(), "\n") + 1 + 1 // wrapped lines + trailing blank
	buttonRowY := 2 + msgRows + 1                       // title + separator + message + separator

	foundSubmit := false
	for x := range contentWidth {
		if d.HandleMouseLocal(x, buttonRowY) == dialog.DialogActionSubmit {
			foundSubmit = true
			break
		}
	}
	if !foundSubmit {
		t.Errorf("Yes button not reachable by mouse at the button row (y=%d)", buttonRowY)
	}
}
