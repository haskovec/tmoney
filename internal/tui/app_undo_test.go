package tui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

func TestApp_UndoKeyBinding(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Press Ctrl+Z with nothing to undo
	msg := tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	if cmd == nil {
		t.Fatal("Ctrl+Z should return a command")
	}

	// Execute the command to get the result message
	result := cmd()
	undoResult, ok := result.(undoResultMsg)
	if !ok {
		t.Fatalf("expected undoResultMsg, got %T", result)
	}
	if undoResult.action != "Undo" {
		t.Errorf("action = %q, want %q", undoResult.action, "Undo")
	}
	if !errors.Is(undoResult.err, undo.ErrNothingToUndo) {
		t.Errorf("err = %v, want ErrNothingToUndo", undoResult.err)
	}
}

func TestApp_RedoKeyBinding(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Press Ctrl+Y with nothing to redo
	msg := tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	if cmd == nil {
		t.Fatal("Ctrl+Y should return a command")
	}

	result := cmd()
	undoResult, ok := result.(undoResultMsg)
	if !ok {
		t.Fatalf("expected undoResultMsg, got %T", result)
	}
	if undoResult.action != "Redo" {
		t.Errorf("action = %q, want %q", undoResult.action, "Redo")
	}
	if !errors.Is(undoResult.err, undo.ErrNothingToRedo) {
		t.Errorf("err = %v, want ErrNothingToRedo", undoResult.err)
	}
}

func TestApp_UndoResultMsg_NothingToUndo(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", err: undo.ErrNothingToUndo}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Error("nothing-to-undo should not trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Nothing to undo" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Nothing to undo")
	}
}

func TestApp_UndoResultMsg_NothingToRedo(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Redo", err: undo.ErrNothingToRedo}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Error("nothing-to-redo should not trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Nothing to redo" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Nothing to redo")
	}
}

func TestApp_UndoResultMsg_Success(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", description: "Create transaction"}
	_, cmd := app.Update(msg)

	// Should trigger a reload
	if cmd == nil {
		t.Error("successful undo should trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Undo: Create transaction" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Undo: Create transaction")
	}
}

func TestApp_RedoResultMsg_Success(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Redo", description: "Delete account"}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("successful redo should trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Redo: Delete account" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Redo: Delete account")
	}
}

func TestApp_UndoResultMsg_Error(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", err: fmt.Errorf("database error")}
	_, _ = app.Update(msg)

	// Error should be set on the app for display
	if app.err == nil {
		t.Error("error should be set on the app")
	}
	if app.err.Error() != "database error" {
		t.Errorf("err = %q, want %q", app.err.Error(), "database error")
	}
}

func TestApp_PerformUndo_NilManager(t *testing.T) {
	app := &App{
		undoManager: nil,
	}

	cmd := app.performUndo()
	if cmd != nil {
		t.Error("performUndo with nil manager should return nil")
	}
}

func TestApp_PerformRedo_NilManager(t *testing.T) {
	app := &App{
		undoManager: nil,
	}

	cmd := app.performRedo()
	if cmd != nil {
		t.Error("performRedo with nil manager should return nil")
	}
}

func TestApp_MenuUndo(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Simulate menu action
	_, cmd := app.handleMenuAction(MenuActionUndo, "")
	if cmd == nil {
		t.Fatal("MenuActionUndo should return a command")
	}

	// Menu should be deactivated
	if app.menubar.IsActive() {
		t.Error("menu should be deactivated after undo")
	}
}

func TestApp_MenuRedo(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	_, cmd := app.handleMenuAction(MenuActionRedo, "")
	if cmd == nil {
		t.Fatal("MenuActionRedo should return a command")
	}

	if app.menubar.IsActive() {
		t.Error("menu should be deactivated after redo")
	}
}

func TestApp_UndoKeyBindingNotActiveInDialogs(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Open a transaction dialog
	app.txnDialog = NewDialog("Test")
	app.txnDialog.AddTextField("Name", "", "", 0)
	app.txnDialog.SetVisible(true)
	app.txnDialogData = &transactionDialogData{}
	app.txnDialogCategoryIDs = []types.ID{}

	// Press Ctrl+Z - should be routed to dialog, not undo
	msg := tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	// The dialog should handle it (Ctrl+Z is not a dialog action, so it may just be consumed)
	// The key point is: it should NOT route to performUndo
	if cmd != nil {
		result := cmd()
		if _, ok := result.(undoResultMsg); ok {
			t.Error("Ctrl+Z should not trigger undo when dialog is open")
		}
	}
}
