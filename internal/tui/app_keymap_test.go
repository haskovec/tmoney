package tui

import "testing"

func TestDefaultKeyMap(t *testing.T) {
	km := defaultKeyMap()

	// Verify key bindings are set
	if len(km.Quit.Keys()) == 0 {
		t.Error("Quit key binding not set")
	}
	if len(km.Help.Keys()) == 0 {
		t.Error("Help key binding not set")
	}
	if len(km.Up.Keys()) == 0 {
		t.Error("Up key binding not set")
	}
	if len(km.Down.Keys()) == 0 {
		t.Error("Down key binding not set")
	}
	if len(km.Enter.Keys()) == 0 {
		t.Error("Enter key binding not set")
	}
	if len(km.Escape.Keys()) == 0 {
		t.Error("Escape key binding not set")
	}
}

func TestDefaultKeyMap_MenuShortcuts(t *testing.T) {
	km := defaultKeyMap()

	if len(km.MenuFile.Keys()) == 0 {
		t.Error("MenuFile key binding not set")
	}
	if len(km.MenuAccounts.Keys()) == 0 {
		t.Error("MenuAccounts key binding not set")
	}
	if len(km.MenuTransactions.Keys()) == 0 {
		t.Error("MenuTransactions key binding not set")
	}
	if len(km.MenuReports.Keys()) == 0 {
		t.Error("MenuReports key binding not set")
	}
	if len(km.MenuHelp.Keys()) == 0 {
		t.Error("MenuHelp key binding not set")
	}
}
