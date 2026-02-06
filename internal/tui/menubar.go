package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MenuAction identifies a menu item action to be handled by the application.
type MenuAction int

const (
	// File menu actions
	MenuActionNone MenuAction = iota
	MenuActionNewFile
	MenuActionOpenFile
	MenuActionOpenRecent
	MenuActionCloseFile
	MenuActionExit

	// Accounts menu actions
	MenuActionNewAccount
	MenuActionEditAccount
	MenuActionCloseAccount
	MenuActionDeleteAccount

	// Transactions menu actions
	MenuActionNewTransaction
	MenuActionNewTransfer
	MenuActionEditTransaction
	MenuActionDeleteTransaction
	MenuActionSearch

	// Reports menu actions
	MenuActionDashboard
	MenuActionNetWorth
	MenuActionSpendingByCategory

	// Help menu actions
	MenuActionKeyboardShortcuts
	MenuActionAbout
)

// menuItem represents a single item in a dropdown menu.
type menuItem struct {
	label  string
	action MenuAction
}

// menu represents a top-level menu with a label and dropdown items.
type menu struct {
	label string
	items []menuItem
}

// MenuBar manages the menu bar state and rendering.
type MenuBar struct {
	menus []menu

	// Whether the menu bar is active (a dropdown is open)
	active bool

	// Index of the currently highlighted top-level menu
	cursor int

	// Index of the currently highlighted item within the open dropdown
	itemCursor int
}

// NewMenuBar creates a new MenuBar with the default menu structure.
func NewMenuBar() *MenuBar {
	return &MenuBar{
		menus: defaultMenus(),
	}
}

// defaultMenus returns the default menu structure per the spec.
func defaultMenus() []menu {
	return []menu{
		{
			label: "File",
			items: []menuItem{
				{label: "New File", action: MenuActionNewFile},
				{label: "Open File", action: MenuActionOpenFile},
				{label: "Open Recent", action: MenuActionOpenRecent},
				{label: "Close File", action: MenuActionCloseFile},
				{label: "Exit", action: MenuActionExit},
			},
		},
		{
			label: "Accounts",
			items: []menuItem{
				{label: "New Account", action: MenuActionNewAccount},
				{label: "Edit Account", action: MenuActionEditAccount},
				{label: "Close Account", action: MenuActionCloseAccount},
				{label: "Delete Account", action: MenuActionDeleteAccount},
			},
		},
		{
			label: "Transactions",
			items: []menuItem{
				{label: "New Transaction", action: MenuActionNewTransaction},
				{label: "New Transfer", action: MenuActionNewTransfer},
				{label: "Edit Transaction", action: MenuActionEditTransaction},
				{label: "Delete Transaction", action: MenuActionDeleteTransaction},
				{label: "Search...", action: MenuActionSearch},
			},
		},
		{
			label: "Reports",
			items: []menuItem{
				{label: "Dashboard", action: MenuActionDashboard},
				{label: "Net Worth", action: MenuActionNetWorth},
				{label: "Spending by Category", action: MenuActionSpendingByCategory},
			},
		},
		{
			label: "Help",
			items: []menuItem{
				{label: "Keyboard Shortcuts", action: MenuActionKeyboardShortcuts},
				{label: "About", action: MenuActionAbout},
			},
		},
	}
}

// IsActive returns whether the menu bar is active (a dropdown is open).
func (m *MenuBar) IsActive() bool {
	return m.active
}

// Activate opens the menu bar, showing the dropdown for the current cursor position.
func (m *MenuBar) Activate() {
	m.active = true
	m.itemCursor = 0
}

// Deactivate closes the menu bar.
func (m *MenuBar) Deactivate() {
	m.active = false
	m.itemCursor = 0
}

// Cursor returns the current top-level menu index.
func (m *MenuBar) Cursor() int {
	return m.cursor
}

// ItemCursor returns the current dropdown item index.
func (m *MenuBar) ItemCursor() int {
	return m.itemCursor
}

// MenuCount returns the number of top-level menus.
func (m *MenuBar) MenuCount() int {
	return len(m.menus)
}

// CurrentMenu returns the currently highlighted menu, or nil if none.
func (m *MenuBar) CurrentMenu() *menu {
	if m.cursor < 0 || m.cursor >= len(m.menus) {
		return nil
	}
	return &m.menus[m.cursor]
}

// MoveLeft moves the cursor to the previous top-level menu.
func (m *MenuBar) MoveLeft() {
	if m.cursor > 0 {
		m.cursor--
		m.itemCursor = 0
	}
}

// MoveRight moves the cursor to the next top-level menu.
func (m *MenuBar) MoveRight() {
	if m.cursor < len(m.menus)-1 {
		m.cursor++
		m.itemCursor = 0
	}
}

// MoveUp moves the item cursor up within the open dropdown.
func (m *MenuBar) MoveUp() {
	if m.itemCursor > 0 {
		m.itemCursor--
	}
}

// MoveDown moves the item cursor down within the open dropdown.
func (m *MenuBar) MoveDown() {
	current := m.CurrentMenu()
	if current == nil {
		return
	}
	if m.itemCursor < len(current.items)-1 {
		m.itemCursor++
	}
}

// Select returns the action for the currently highlighted dropdown item.
// Returns MenuActionNone if no valid selection.
func (m *MenuBar) Select() MenuAction {
	current := m.CurrentMenu()
	if current == nil {
		return MenuActionNone
	}
	if m.itemCursor < 0 || m.itemCursor >= len(current.items) {
		return MenuActionNone
	}
	action := current.items[m.itemCursor].action
	m.Deactivate()
	return action
}

// Render renders the menu bar (just the top-level labels) for the given width.
func (m *MenuBar) Render(styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	var parts []string
	for i, mn := range m.menus {
		label := " " + mn.label + " "
		if m.active && i == m.cursor {
			parts = append(parts, styles.MenuBarActive.Render(label))
		} else {
			parts = append(parts, styles.MenuBarItem.Render(label))
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// Pad the rest of the bar with the header background
	barWidth := lipgloss.Width(bar)
	if barWidth < width {
		padding := strings.Repeat(" ", width-barWidth)
		bar = bar + styles.Header.Render(padding)
	}

	return bar
}

// RenderDropdown renders the dropdown for the currently active menu.
// Returns the rendered dropdown string and the horizontal offset where it should be positioned.
// Returns empty string if the menu bar is not active.
func (m *MenuBar) RenderDropdown(styles Styles) (string, int) {
	if !m.active {
		return "", 0
	}

	current := m.CurrentMenu()
	if current == nil {
		return "", 0
	}

	// Calculate horizontal offset based on menu label positions
	offset := 0
	for i := 0; i < m.cursor; i++ {
		// Each label is " label " so len + 2 spaces
		offset += len(m.menus[i].label) + 2
	}

	// Find the widest item for dropdown width
	maxWidth := 0
	for _, item := range current.items {
		if len(item.label) > maxWidth {
			maxWidth = len(item.label)
		}
	}
	// Add padding
	dropdownWidth := maxWidth + 4

	var lines []string
	for i, item := range current.items {
		label := " " + item.label
		// Pad to dropdown width
		if len(label) < dropdownWidth-1 {
			label = label + strings.Repeat(" ", dropdownWidth-1-len(label))
		}
		label = label + " "

		if i == m.itemCursor {
			lines = append(lines, styles.MenuDropdownActive.Render(label))
		} else {
			lines = append(lines, styles.MenuDropdownItem.Render(label))
		}
	}

	dropdown := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return dropdown, offset
}
