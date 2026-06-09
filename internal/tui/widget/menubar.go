package widget

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// ViewMenuIndex is the position of the View menu in defaultMenus(),
// used by the App when registering the dynamic theme-list builder.
const ViewMenuIndex = 2

// MenuAction identifies a menu item action to be handled by the application.
type MenuAction int

const (
	// File menu actions
	MenuActionNone MenuAction = iota
	MenuActionNewFile
	MenuActionOpenFile
	MenuActionOpenRecent
	MenuActionImportTransactions
	MenuActionCreateBackup
	MenuActionRestoreBackup
	MenuActionCloseFile
	MenuActionExit

	// Accounts menu actions
	MenuActionNewAccount
	MenuActionEditAccount
	MenuActionCloseAccount
	MenuActionReopenAccount
	MenuActionDeleteAccount
	MenuActionReconcileAccount

	// Transactions menu actions
	MenuActionNewTransaction
	MenuActionNewTransfer
	MenuActionEditTransaction
	MenuActionDeleteTransaction
	MenuActionSearch
	MenuActionLinkTransfers
	MenuActionNewPaycheckSchedule

	// Reports menu actions
	MenuActionDashboard
	MenuActionNetWorth
	MenuActionSpendingByCategory

	// Edit menu actions
	MenuActionUndo
	MenuActionRedo

	// Securities menu actions
	MenuActionSecurities
	MenuActionPrices
	MenuActionStockSplit
	MenuActionMerger
	MenuActionSpinOff
	MenuActionCorporateActions

	// View menu actions
	MenuActionThemeSubmenu
	MenuActionLoadTheme
	MenuActionToggleClosedPositions

	// Help menu actions
	MenuActionKeyboardShortcuts
	MenuActionAbout
)

// MenuItem represents a single item in a dropdown menu. The optional
// `data` payload carries action-specific context — for theme items it
// holds the theme ID so the dispatch path can call reloadTheme without
// re-parsing the label.
type MenuItem struct {
	Label  string
	Action MenuAction
	Data   string
}

// Menu represents a top-level Menu with a label and dropdown items.
type Menu struct {
	Label       string
	ShortcutKey rune
	Items       []MenuItem
}

// MenuBar manages the menu bar state and rendering.
type MenuBar struct {
	menus []Menu

	// Whether the menu bar is active (a dropdown is open)
	active bool

	// Index of the currently highlighted top-level menu
	cursor int

	// Index of the currently highlighted item within the open dropdown
	itemCursor int

	// itemsBuilders maps a top-level menu index to a function that
	// produces a fresh items slice every time the user opens (or
	// arrows onto) that menu. Used for menus whose contents depend on
	// runtime state — e.g. View → Theme reflecting the discovered
	// themes and the currently active one.
	itemsBuilders map[int]func() []MenuItem
}

// NewMenuBar creates a new MenuBar with the default menu structure.
func NewMenuBar() *MenuBar {
	return &MenuBar{
		menus: DefaultMenus(),
	}
}

// DefaultMenus returns the default menu structure per the spec.
func DefaultMenus() []Menu {
	return []Menu{
		{
			Label:       "File",
			ShortcutKey: 'F',
			Items: []MenuItem{
				{Label: "New File", Action: MenuActionNewFile},
				{Label: "Open File", Action: MenuActionOpenFile},
				{Label: "Open Recent", Action: MenuActionOpenRecent},
				{Label: "Import Transactions...", Action: MenuActionImportTransactions},
				{Label: "Create Backup", Action: MenuActionCreateBackup},
				{Label: "Restore from Backup", Action: MenuActionRestoreBackup},
				{Label: "Close File", Action: MenuActionCloseFile},
				{Label: "Exit", Action: MenuActionExit},
			},
		},
		{
			Label:       "Edit",
			ShortcutKey: 'E',
			Items: []MenuItem{
				{Label: "Undo", Action: MenuActionUndo},
				{Label: "Redo", Action: MenuActionRedo},
			},
		},
		{
			Label:       "View",
			ShortcutKey: 'V',
			Items: []MenuItem{
				{Label: "Theme", Action: MenuActionThemeSubmenu},
			},
		},
		{
			Label:       "Accounts",
			ShortcutKey: 'A',
			Items: []MenuItem{
				{Label: "New Account", Action: MenuActionNewAccount},
				{Label: "Edit Account", Action: MenuActionEditAccount},
				{Label: "Close Account", Action: MenuActionCloseAccount},
				{Label: "Reopen Account", Action: MenuActionReopenAccount},
				{Label: "Delete Account", Action: MenuActionDeleteAccount},
				{Label: "Reconcile Account", Action: MenuActionReconcileAccount},
			},
		},
		{
			Label:       "Transactions",
			ShortcutKey: 'T',
			Items: []MenuItem{
				{Label: "New Transaction", Action: MenuActionNewTransaction},
				{Label: "New Transfer", Action: MenuActionNewTransfer},
				{Label: "Edit Transaction", Action: MenuActionEditTransaction},
				{Label: "Delete Transaction", Action: MenuActionDeleteTransaction},
				{Label: "Search...", Action: MenuActionSearch},
				{Label: "Link Transfers...", Action: MenuActionLinkTransfers},
				{Label: "New Paycheck Schedule...", Action: MenuActionNewPaycheckSchedule},
			},
		},
		{
			Label:       "Securities",
			ShortcutKey: 'S',
			Items: []MenuItem{
				{Label: "Security Master", Action: MenuActionSecurities},
				{Label: "Prices", Action: MenuActionPrices},
				{Label: "Stock Split...", Action: MenuActionStockSplit},
				{Label: "Merger...", Action: MenuActionMerger},
				{Label: "Spin-Off...", Action: MenuActionSpinOff},
				{Label: "Corporate Action History...", Action: MenuActionCorporateActions},
			},
		},
		{
			Label:       "Reports",
			ShortcutKey: 'R',
			Items: []MenuItem{
				{Label: "Dashboard", Action: MenuActionDashboard},
				{Label: "Net Worth", Action: MenuActionNetWorth},
				{Label: "Spending by Category", Action: MenuActionSpendingByCategory},
			},
		},
		{
			Label:       "Help",
			ShortcutKey: 'H',
			Items: []MenuItem{
				{Label: "Keyboard Shortcuts", Action: MenuActionKeyboardShortcuts},
				{Label: "About", Action: MenuActionAbout},
			},
		},
	}
}

// SetMenuItemsBuilder registers a builder that produces the items
// for the menu at `index` whenever it is opened or navigated onto.
// Passing a nil fn unregisters the builder for that index, leaving
// the menu's static items in place.
func (m *MenuBar) SetMenuItemsBuilder(index int, fn func() []MenuItem) {
	if index < 0 || index >= len(m.menus) {
		return
	}
	if fn == nil {
		if m.itemsBuilders != nil {
			delete(m.itemsBuilders, index)
		}
		return
	}
	if m.itemsBuilders == nil {
		m.itemsBuilders = make(map[int]func() []MenuItem)
	}
	m.itemsBuilders[index] = fn
}

// refreshCurrentItems re-runs the registered builder (if any) for the
// currently focused menu, replacing its items in-place.
func (m *MenuBar) refreshCurrentItems() {
	if m.cursor < 0 || m.cursor >= len(m.menus) {
		return
	}
	if m.itemsBuilders == nil {
		return
	}
	if fn, ok := m.itemsBuilders[m.cursor]; ok && fn != nil {
		m.menus[m.cursor].Items = fn()
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
	m.refreshCurrentItems()
}

// Deactivate closes the menu bar.
func (m *MenuBar) Deactivate() {
	m.active = false
	m.itemCursor = 0
}

// ActivateMenu opens the menu bar at the specified menu index.
func (m *MenuBar) ActivateMenu(index int) {
	if index >= 0 && index < len(m.menus) {
		m.cursor = index
		m.Activate()
	}
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

// Menus returns the menu bar's top-level menus.
func (m *MenuBar) Menus() []Menu {
	return m.menus
}

// CurrentMenu returns the currently highlighted menu, or nil if none.
func (m *MenuBar) CurrentMenu() *Menu {
	if m.cursor < 0 || m.cursor >= len(m.menus) {
		return nil
	}
	return &m.menus[m.cursor]
}

// SetItemCursor sets the dropdown item cursor to the given position.
func (m *MenuBar) SetItemCursor(pos int) {
	m.itemCursor = pos
}

// MoveLeft moves the cursor to the previous top-level menu.
func (m *MenuBar) MoveLeft() {
	if m.cursor > 0 {
		m.cursor--
		m.itemCursor = 0
		m.refreshCurrentItems()
	}
}

// MoveRight moves the cursor to the next top-level menu.
func (m *MenuBar) MoveRight() {
	if m.cursor < len(m.menus)-1 {
		m.cursor++
		m.itemCursor = 0
		m.refreshCurrentItems()
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
	if m.itemCursor < len(current.Items)-1 {
		m.itemCursor++
	}
}

// Select returns the action and data payload for the currently
// highlighted dropdown item, then deactivates the menu. The data
// payload carries action-specific context (e.g. the theme ID for
// MenuActionLoadTheme); it is empty for actions that don't need it.
// Returns (MenuActionNone, "") if no valid selection.
func (m *MenuBar) Select() (MenuAction, string) {
	current := m.CurrentMenu()
	if current == nil {
		return MenuActionNone, ""
	}
	if m.itemCursor < 0 || m.itemCursor >= len(current.Items) {
		return MenuActionNone, ""
	}
	item := current.Items[m.itemCursor]
	m.Deactivate()
	return item.Action, item.Data
}

// HitTestBar determines which top-level menu label was clicked at position x.
// Returns the menu index (0-based), or -1 if no menu was hit.
// x is relative to the start of the menu bar (column 0).
func (m *MenuBar) HitTestBar(x int) int {
	if x < 0 {
		return -1
	}
	offset := 0
	for i, mn := range m.menus {
		w := len(mn.Label) + 2 // " label " format
		if x >= offset && x < offset+w {
			return i
		}
		offset += w
	}
	return -1
}

// HitTestDropdown determines which dropdown item was clicked at row y.
// y is the row offset within the dropdown (0-based, where 0 is the first item).
// Returns the item index, or -1 if out of range or menu is not active.
func (m *MenuBar) HitTestDropdown(y int) int {
	if !m.active {
		return -1
	}
	current := m.CurrentMenu()
	if current == nil {
		return -1
	}
	if y >= 0 && y < len(current.Items) {
		return y
	}
	return -1
}

// DropdownBounds returns the column offset, width, and item count for the active dropdown.
// Returns (0, 0, 0) if the menu bar is not active.
func (m *MenuBar) DropdownBounds() (colOffset, dropdownWidth, itemCount int) {
	if !m.active {
		return 0, 0, 0
	}
	current := m.CurrentMenu()
	if current == nil {
		return 0, 0, 0
	}

	// Calculate column offset (same as RenderDropdown)
	for i := 0; i < m.cursor; i++ {
		colOffset += len(m.menus[i].Label) + 2
	}

	// Calculate dropdown width (same as RenderDropdown)
	maxWidth := 0
	for _, item := range current.Items {
		if len(item.Label) > maxWidth {
			maxWidth = len(item.Label)
		}
	}
	dropdownWidth = maxWidth + 4

	itemCount = len(current.Items)
	return
}

// Render renders the menu bar (just the top-level labels) for the given width.
// Shortcut letters are underlined to indicate Alt+key shortcuts.
func (m *MenuBar) Render(styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	var parts []string
	for i, mn := range m.menus {
		var baseStyle, shortcutStyle lipgloss.Style
		if m.active && i == m.cursor {
			baseStyle = styles.MenuBarActive
			shortcutStyle = styles.MenuBarActiveShortcut
		} else {
			baseStyle = styles.MenuBarItem
			shortcutStyle = styles.MenuBarShortcut
		}
		parts = append(parts, renderMenuLabel(mn.Label, mn.ShortcutKey, baseStyle, shortcutStyle))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// Pad the rest of the bar with the header background.
	// Use a plain style with just the background color to avoid the Header
	// style's Width/Padding adding extra columns.
	barWidth := lipgloss.Width(bar)
	if barWidth < width {
		padStyle := lipgloss.NewStyle().
			Background(styles.Header.GetBackground())
		padding := strings.Repeat(" ", width-barWidth)
		bar = bar + padStyle.Render(padding)
	}

	return bar
}

// renderMenuLabel renders a menu label with the shortcut character underlined.
func renderMenuLabel(label string, shortcutKey rune, baseStyle, shortcutStyle lipgloss.Style) string {
	if shortcutKey == 0 {
		return baseStyle.Render(" " + label + " ")
	}

	for i, r := range label {
		if unicode.ToUpper(r) == unicode.ToUpper(shortcutKey) {
			before := " " + label[:i]
			shortcutChar := string(r)
			after := label[i+utf8.RuneLen(r):] + " "
			return baseStyle.Render(before) + shortcutStyle.Render(shortcutChar) + baseStyle.Render(after)
		}
	}

	return baseStyle.Render(" " + label + " ")
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
		offset += len(m.menus[i].Label) + 2
	}

	// Find the widest item for dropdown width
	maxWidth := 0
	for _, item := range current.Items {
		if len(item.Label) > maxWidth {
			maxWidth = len(item.Label)
		}
	}
	// Add padding
	dropdownWidth := maxWidth + 4

	var lines []string
	for i, item := range current.Items {
		label := " " + item.Label
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
