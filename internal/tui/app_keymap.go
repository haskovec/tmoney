package tui

import (
	"charm.land/bubbles/v2/key"
)

// keyMap defines the key bindings for the application.
type keyMap struct {
	Quit             key.Binding
	Help             key.Binding
	Up               key.Binding
	Down             key.Binding
	Left             key.Binding
	Right            key.Binding
	Enter            key.Binding
	Escape           key.Binding
	Tab              key.Binding
	ShiftTab         key.Binding
	New              key.Binding
	Edit             key.Binding
	Delete           key.Binding
	Search           key.Binding
	Dashboard        key.Binding
	Scheduled        key.Binding
	Reports          key.Binding
	Securities       key.Binding
	Prices           key.Binding
	Menu             key.Binding
	MenuFile         key.Binding
	MenuAccounts     key.Binding
	MenuTransactions key.Binding
	MenuSecurities   key.Binding
	MenuEdit         key.Binding
	MenuView         key.Binding
	MenuReports      key.Binding
	MenuHelp         key.Binding
	Undo             key.Binding
	Redo             key.Binding
}

// defaultKeyMap returns the default key bindings.
func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+q", "ctrl+c"),
			key.WithHelp("ctrl+q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Dashboard: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard"),
		),
		Scheduled: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "scheduled"),
		),
		Reports: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "reports"),
		),
		Securities: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "securities"),
		),
		Prices: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "prices"),
		),
		Menu: key.NewBinding(
			key.WithKeys("f10"),
			key.WithHelp("F10", "menu"),
		),
		MenuFile: key.NewBinding(
			key.WithKeys("alt+f"),
			key.WithHelp("Alt+F", "file menu"),
		),
		MenuAccounts: key.NewBinding(
			key.WithKeys("alt+a"),
			key.WithHelp("Alt+A", "accounts menu"),
		),
		MenuTransactions: key.NewBinding(
			key.WithKeys("alt+t"),
			key.WithHelp("Alt+T", "transactions menu"),
		),
		MenuSecurities: key.NewBinding(
			key.WithKeys("alt+s"),
			key.WithHelp("Alt+S", "securities menu"),
		),
		MenuReports: key.NewBinding(
			key.WithKeys("alt+r"),
			key.WithHelp("Alt+R", "reports menu"),
		),
		MenuEdit: key.NewBinding(
			key.WithKeys("alt+e"),
			key.WithHelp("Alt+E", "edit menu"),
		),
		MenuView: key.NewBinding(
			key.WithKeys("alt+v"),
			key.WithHelp("Alt+V", "view menu"),
		),
		MenuHelp: key.NewBinding(
			key.WithKeys("alt+h"),
			key.WithHelp("Alt+H", "help menu"),
		),
		Undo: undoKeyBinding(),
		Redo: redoKeyBinding(),
	}
}

// undoKeyBinding returns the undo key binding. The app listens for Ctrl+Z
// on every platform; the label matches the actual key the user must press.
func undoKeyBinding() key.Binding {
	return key.NewBinding(
		key.WithKeys("ctrl+z"),
		key.WithHelp("Ctrl+Z", "undo"),
	)
}

// redoKeyBinding returns the redo key binding. The app listens for Ctrl+Y
// on every platform; the label matches the actual key the user must press.
func redoKeyBinding() key.Binding {
	return key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("Ctrl+Y", "redo"),
	)
}
