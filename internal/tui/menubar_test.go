package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestNewMenuBar(t *testing.T) {
	m := NewMenuBar()
	if m == nil {
		t.Fatal("NewMenuBar() returned nil")
	}
	if m.active {
		t.Error("menu bar should not be active by default")
	}
	if m.cursor != 0 {
		t.Error("cursor should start at 0")
	}
	if m.itemCursor != 0 {
		t.Error("itemCursor should start at 0")
	}
}

func TestMenuBar_DefaultMenus(t *testing.T) {
	m := NewMenuBar()

	if m.MenuCount() != 8 {
		t.Fatalf("expected 8 menus, got %d", m.MenuCount())
	}

	expectedLabels := []string{"File", "Edit", "View", "Accounts", "Transactions", "Securities", "Reports", "Help"}
	for i, mn := range m.menus {
		if mn.label != expectedLabels[i] {
			t.Errorf("menu[%d].label = %q, want %q", i, mn.label, expectedLabels[i])
		}
	}
}

func TestMenuBar_FileMenuItems(t *testing.T) {
	m := NewMenuBar()

	fileMenu := m.menus[0]
	if len(fileMenu.items) != 8 {
		t.Fatalf("File menu: expected 8 items, got %d", len(fileMenu.items))
	}

	expectedItems := []struct {
		label  string
		action MenuAction
	}{
		{"New File", MenuActionNewFile},
		{"Open File", MenuActionOpenFile},
		{"Open Recent", MenuActionOpenRecent},
		{"Import Transactions...", MenuActionImportTransactions},
		{"Create Backup", MenuActionCreateBackup},
		{"Restore from Backup", MenuActionRestoreBackup},
		{"Close File", MenuActionCloseFile},
		{"Exit", MenuActionExit},
	}

	for i, exp := range expectedItems {
		if fileMenu.items[i].label != exp.label {
			t.Errorf("File[%d].label = %q, want %q", i, fileMenu.items[i].label, exp.label)
		}
		if fileMenu.items[i].action != exp.action {
			t.Errorf("File[%d].action = %d, want %d", i, fileMenu.items[i].action, exp.action)
		}
	}
}

func TestMenuBar_ActivateDeactivate(t *testing.T) {
	m := NewMenuBar()

	if m.IsActive() {
		t.Error("should not be active initially")
	}

	m.Activate()
	if !m.IsActive() {
		t.Error("should be active after Activate()")
	}

	m.Deactivate()
	if m.IsActive() {
		t.Error("should not be active after Deactivate()")
	}
}

func TestMenuBar_ActivateResetsItemCursor(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	m.MoveDown()
	m.MoveDown()

	if m.ItemCursor() != 2 {
		t.Fatalf("itemCursor = %d, want 2", m.ItemCursor())
	}

	// Deactivating and reactivating should reset item cursor
	m.Deactivate()
	m.Activate()

	if m.ItemCursor() != 0 {
		t.Errorf("itemCursor after reactivate = %d, want 0", m.ItemCursor())
	}
}

func TestMenuBar_MoveLeftRight(t *testing.T) {
	m := NewMenuBar()

	if m.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0", m.Cursor())
	}

	m.MoveRight()
	if m.Cursor() != 1 {
		t.Errorf("after MoveRight, cursor = %d, want 1", m.Cursor())
	}

	m.MoveRight()
	if m.Cursor() != 2 {
		t.Errorf("after 2x MoveRight, cursor = %d, want 2", m.Cursor())
	}

	m.MoveLeft()
	if m.Cursor() != 1 {
		t.Errorf("after MoveLeft, cursor = %d, want 1", m.Cursor())
	}

	// Move all the way to the beginning
	m.MoveLeft()
	m.MoveLeft()
	if m.Cursor() != 0 {
		t.Errorf("cursor should not go below 0, got %d", m.Cursor())
	}
}

func TestMenuBar_MoveRightBound(t *testing.T) {
	m := NewMenuBar()

	// Move all the way to the end
	for range m.menus {
		m.MoveRight()
	}

	// Should be capped at last menu
	if m.Cursor() != len(m.menus)-1 {
		t.Errorf("cursor = %d, want %d", m.Cursor(), len(m.menus)-1)
	}
}

func TestMenuBar_MoveLeftResetsItemCursor(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	m.MoveDown()
	m.MoveDown()

	if m.ItemCursor() != 2 {
		t.Fatalf("itemCursor = %d, want 2", m.ItemCursor())
	}

	m.MoveRight()
	if m.ItemCursor() != 0 {
		t.Errorf("itemCursor should reset to 0 on MoveRight, got %d", m.ItemCursor())
	}
}

func TestMenuBar_MoveUpDown(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// File menu has 8 items
	m.MoveDown()
	if m.ItemCursor() != 1 {
		t.Errorf("after MoveDown, itemCursor = %d, want 1", m.ItemCursor())
	}

	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	if m.ItemCursor() != 7 {
		t.Errorf("after 7x MoveDown, itemCursor = %d, want 7", m.ItemCursor())
	}

	// Should not go past last item
	m.MoveDown()
	if m.ItemCursor() != 7 {
		t.Errorf("itemCursor should stay at 7, got %d", m.ItemCursor())
	}

	m.MoveUp()
	if m.ItemCursor() != 6 {
		t.Errorf("after MoveUp, itemCursor = %d, want 6", m.ItemCursor())
	}

	// Move all the way up
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()
	if m.ItemCursor() != 0 {
		t.Errorf("itemCursor should not go below 0, got %d", m.ItemCursor())
	}
}

func TestMenuBar_Select(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// First item in File menu is "New File"
	action, data := m.Select()
	if action != MenuActionNewFile {
		t.Errorf("expected MenuActionNewFile, got %d", action)
	}
	if data != "" {
		t.Errorf("expected empty data, got %q", data)
	}

	// Select should deactivate the menu
	if m.IsActive() {
		t.Error("menu should be deactivated after Select()")
	}
}

func TestMenuBar_SelectExitAction(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// Move to "Exit" (index 7 in File menu)
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()

	action, _ := m.Select()
	if action != MenuActionExit {
		t.Errorf("expected MenuActionExit, got %d", action)
	}
}

func TestMenuBar_SelectFromDifferentMenu(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// Move to Accounts menu (index 3, past Edit and View)
	m.MoveRight()
	m.MoveRight()
	m.MoveRight()

	// First item is "New Account"
	action, _ := m.Select()
	if action != MenuActionNewAccount {
		t.Errorf("expected MenuActionNewAccount, got %d", action)
	}
}

func TestMenuBar_SelectNoMenu(t *testing.T) {
	m := &MenuBar{} // Empty menu bar

	action, data := m.Select()
	if action != MenuActionNone {
		t.Errorf("expected MenuActionNone from empty menu bar, got %d", action)
	}
	if data != "" {
		t.Errorf("expected empty data, got %q", data)
	}
}

func TestMenuBar_CurrentMenu(t *testing.T) {
	m := NewMenuBar()

	current := m.CurrentMenu()
	if current == nil {
		t.Fatal("CurrentMenu() should not be nil")
	}
	if current.label != "File" {
		t.Errorf("CurrentMenu().label = %q, want %q", current.label, "File")
	}

	m.MoveRight()
	current = m.CurrentMenu()
	if current.label != "Edit" {
		t.Errorf("after MoveRight, CurrentMenu().label = %q, want %q", current.label, "Edit")
	}
}

func TestMenuBar_CurrentMenuEmpty(t *testing.T) {
	m := &MenuBar{cursor: -1}

	if m.CurrentMenu() != nil {
		t.Error("CurrentMenu() should return nil for invalid cursor")
	}
}

func TestMenuBar_Render_NotActive(t *testing.T) {
	m := NewMenuBar()
	styles := NewStyles()
	styles.Resize(80, 24)

	result := m.Render(styles, 80)
	if result == "" {
		t.Error("Render() should not return empty string")
	}
}

func TestMenuBar_Render_Active(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	styles := NewStyles()
	styles.Resize(80, 24)

	result := m.Render(styles, 80)
	if result == "" {
		t.Error("Render() with active menu should not return empty string")
	}
}

func TestMenuBar_Render_ZeroWidth(t *testing.T) {
	m := NewMenuBar()
	styles := NewStyles()

	result := m.Render(styles, 0)
	if result != "" {
		t.Error("Render() with width=0 should return empty string")
	}
}

func TestMenuBar_RenderDropdown_NotActive(t *testing.T) {
	m := NewMenuBar()
	styles := NewStyles()

	dropdown, offset := m.RenderDropdown(styles)
	if dropdown != "" {
		t.Error("RenderDropdown() should return empty when not active")
	}
	if offset != 0 {
		t.Error("offset should be 0 when not active")
	}
}

func TestMenuBar_RenderDropdown_Active(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	styles := NewStyles()

	dropdown, offset := m.RenderDropdown(styles)
	if dropdown == "" {
		t.Error("RenderDropdown() should return non-empty when active")
	}
	if offset != 0 {
		t.Error("offset for first menu should be 0")
	}
}

func TestMenuBar_RenderDropdown_SecondMenu(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	m.MoveRight() // Move to Edit
	styles := NewStyles()

	dropdown, offset := m.RenderDropdown(styles)
	if dropdown == "" {
		t.Error("RenderDropdown() should return non-empty for Edit menu")
	}

	// Offset should be the width of " File " = 6
	expectedOffset := len("File") + 2
	if offset != expectedOffset {
		t.Errorf("offset = %d, want %d", offset, expectedOffset)
	}
}

func TestMenuBar_RenderDropdown_ThirdMenu(t *testing.T) {
	m := NewMenuBar()
	m.Activate()
	m.MoveRight() // Edit
	m.MoveRight() // Accounts
	styles := NewStyles()

	_, offset := m.RenderDropdown(styles)

	// Offset = " File " + " Edit " = 6 + 6 = 12
	expectedOffset := (len("File") + 2) + (len("Edit") + 2)
	if offset != expectedOffset {
		t.Errorf("offset = %d, want %d", offset, expectedOffset)
	}
}

func TestMenuBar_AllMenuActions(t *testing.T) {
	m := NewMenuBar()

	// Verify all menus have items and all items have valid actions
	for i, mn := range m.menus {
		if len(mn.items) == 0 {
			t.Errorf("menu %q (index %d) has no items", mn.label, i)
		}
		for j, item := range mn.items {
			if item.action == MenuActionNone {
				t.Errorf("menu %q item %q (index %d) has MenuActionNone", mn.label, item.label, j)
			}
			if item.label == "" {
				t.Errorf("menu %q item index %d has empty label", mn.label, j)
			}
		}
	}
}

func TestMenuBar_EditMenuItems(t *testing.T) {
	m := NewMenuBar()

	editMenu := m.menus[1]
	if editMenu.label != "Edit" {
		t.Fatalf("expected Edit menu at index 1, got %q", editMenu.label)
	}

	expectedItems := []struct {
		label  string
		action MenuAction
	}{
		{"Undo", MenuActionUndo},
		{"Redo", MenuActionRedo},
	}

	if len(editMenu.items) != len(expectedItems) {
		t.Fatalf("Edit menu: expected %d items, got %d", len(expectedItems), len(editMenu.items))
	}

	for i, exp := range expectedItems {
		if editMenu.items[i].label != exp.label {
			t.Errorf("Edit[%d].label = %q, want %q", i, editMenu.items[i].label, exp.label)
		}
		if editMenu.items[i].action != exp.action {
			t.Errorf("Edit[%d].action = %d, want %d", i, editMenu.items[i].action, exp.action)
		}
	}
}

func TestMenuBar_ViewMenuItems(t *testing.T) {
	m := NewMenuBar()

	viewMenu := m.menus[2]
	if viewMenu.label != "View" {
		t.Fatalf("expected View menu at index 2, got %q", viewMenu.label)
	}
	if viewMenu.shortcutKey != 'V' {
		t.Errorf("View menu shortcutKey = %q, want %q", string(viewMenu.shortcutKey), "V")
	}

	if len(viewMenu.items) != 1 {
		t.Fatalf("View menu: expected 1 item, got %d", len(viewMenu.items))
	}
	if viewMenu.items[0].label != "Theme" {
		t.Errorf("View[0].label = %q, want %q", viewMenu.items[0].label, "Theme")
	}
	if viewMenu.items[0].action != MenuActionThemeSubmenu {
		t.Errorf("View[0].action = %d, want MenuActionThemeSubmenu", viewMenu.items[0].action)
	}
}

func TestMenuBar_TransactionsMenuItems(t *testing.T) {
	m := NewMenuBar()

	txnMenu := m.menus[4]
	if txnMenu.label != "Transactions" {
		t.Fatalf("expected Transactions menu at index 4, got %q", txnMenu.label)
	}

	expectedItems := []struct {
		label  string
		action MenuAction
	}{
		{"New Transaction", MenuActionNewTransaction},
		{"New Transfer", MenuActionNewTransfer},
		{"Edit Transaction", MenuActionEditTransaction},
		{"Delete Transaction", MenuActionDeleteTransaction},
		{"Search...", MenuActionSearch},
		{"Link Transfers...", MenuActionLinkTransfers},
	}

	if len(txnMenu.items) != len(expectedItems) {
		t.Fatalf("Transactions menu: expected %d items, got %d", len(expectedItems), len(txnMenu.items))
	}

	for i, exp := range expectedItems {
		if txnMenu.items[i].label != exp.label {
			t.Errorf("Transactions[%d].label = %q, want %q", i, txnMenu.items[i].label, exp.label)
		}
		if txnMenu.items[i].action != exp.action {
			t.Errorf("Transactions[%d].action = %d, want %d", i, txnMenu.items[i].action, exp.action)
		}
	}
}

func TestMenuBar_SecuritiesMenuItems(t *testing.T) {
	m := NewMenuBar()

	securitiesMenu := m.menus[5]
	if securitiesMenu.label != "Securities" {
		t.Fatalf("expected Securities menu at index 5, got %q", securitiesMenu.label)
	}

	if len(securitiesMenu.items) != 6 {
		t.Fatalf("Securities menu: expected 6 items, got %d", len(securitiesMenu.items))
	}

	if securitiesMenu.items[0].action != MenuActionSecurities {
		t.Error("first Securities item should be Security Master")
	}
	if securitiesMenu.items[1].action != MenuActionPrices {
		t.Error("second Securities item should be Prices")
	}
	if securitiesMenu.items[2].action != MenuActionStockSplit {
		t.Error("third Securities item should be Stock Split")
	}
	if securitiesMenu.items[3].action != MenuActionMerger {
		t.Error("fourth Securities item should be Merger")
	}
	if securitiesMenu.items[4].action != MenuActionSpinOff {
		t.Error("fifth Securities item should be Spin-Off")
	}
	if securitiesMenu.items[5].action != MenuActionCorporateActions {
		t.Error("sixth Securities item should be Corporate Action History")
	}
}

func TestMenuBar_ReportsMenuItems(t *testing.T) {
	m := NewMenuBar()

	reportsMenu := m.menus[6]
	if reportsMenu.label != "Reports" {
		t.Fatalf("expected Reports menu at index 6, got %q", reportsMenu.label)
	}

	if len(reportsMenu.items) != 3 {
		t.Fatalf("Reports menu: expected 3 items, got %d", len(reportsMenu.items))
	}

	if reportsMenu.items[0].action != MenuActionDashboard {
		t.Error("first Reports item should be Dashboard")
	}
	if reportsMenu.items[1].action != MenuActionNetWorth {
		t.Error("second Reports item should be Net Worth")
	}
	if reportsMenu.items[2].action != MenuActionSpendingByCategory {
		t.Error("third Reports item should be Spending by Category")
	}
}

func TestMenuBar_HelpMenuItems(t *testing.T) {
	m := NewMenuBar()

	helpMenu := m.menus[7]
	if helpMenu.label != "Help" {
		t.Fatalf("expected Help menu at index 7, got %q", helpMenu.label)
	}

	if len(helpMenu.items) != 2 {
		t.Fatalf("Help menu: expected 2 items, got %d", len(helpMenu.items))
	}

	if helpMenu.items[0].action != MenuActionKeyboardShortcuts {
		t.Error("first Help item should be Keyboard Shortcuts")
	}
	if helpMenu.items[1].action != MenuActionAbout {
		t.Error("second Help item should be About")
	}
}

func TestMenuBar_NavigateFullCycle(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// Start at File, navigate through all menus
	for i := 0; i < m.MenuCount()-1; i++ {
		m.MoveRight()
	}

	if m.Cursor() != m.MenuCount()-1 {
		t.Errorf("cursor = %d, want %d", m.Cursor(), m.MenuCount()-1)
	}

	// Navigate back to the beginning
	for i := 0; i < m.MenuCount()-1; i++ {
		m.MoveLeft()
	}

	if m.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor())
	}
}

func TestMenuBar_MoveDownOnEmptyMenuBar(t *testing.T) {
	m := &MenuBar{}

	// Should not panic
	m.MoveDown()
	m.MoveUp()

	if m.ItemCursor() != 0 {
		t.Error("itemCursor should remain 0 on empty menu bar")
	}
}

func TestMenuBar_ShortcutKeys(t *testing.T) {
	m := NewMenuBar()

	expectedShortcuts := []struct {
		label       string
		shortcutKey rune
	}{
		{"File", 'F'},
		{"Edit", 'E'},
		{"View", 'V'},
		{"Accounts", 'A'},
		{"Transactions", 'T'},
		{"Securities", 'S'},
		{"Reports", 'R'},
		{"Help", 'H'},
	}

	for i, exp := range expectedShortcuts {
		if m.menus[i].shortcutKey != exp.shortcutKey {
			t.Errorf("menu %q shortcutKey = %q, want %q", exp.label, string(m.menus[i].shortcutKey), string(exp.shortcutKey))
		}
	}
}

func TestMenuBar_ActivateMenu(t *testing.T) {
	m := NewMenuBar()

	m.ActivateMenu(2)
	if !m.IsActive() {
		t.Error("should be active after ActivateMenu()")
	}
	if m.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2", m.Cursor())
	}
	if m.ItemCursor() != 0 {
		t.Errorf("itemCursor = %d, want 0", m.ItemCursor())
	}
}

func TestMenuBar_ActivateMenu_SwitchWhileActive(t *testing.T) {
	m := NewMenuBar()

	m.ActivateMenu(0)
	m.MoveDown()
	m.MoveDown()

	// Switch to a different menu - should reset item cursor
	m.ActivateMenu(3)
	if m.Cursor() != 3 {
		t.Errorf("cursor = %d, want 3", m.Cursor())
	}
	if m.ItemCursor() != 0 {
		t.Errorf("itemCursor = %d, want 0 after switching menus", m.ItemCursor())
	}
}

func TestMenuBar_ActivateMenu_InvalidIndex(t *testing.T) {
	m := NewMenuBar()

	m.ActivateMenu(-1)
	if m.IsActive() {
		t.Error("should not activate with negative index")
	}

	m.ActivateMenu(100)
	if m.IsActive() {
		t.Error("should not activate with out-of-range index")
	}
}

func TestRenderMenuLabel_WithShortcut(t *testing.T) {
	baseStyle := lipgloss.NewStyle()
	shortcutStyle := lipgloss.NewStyle().Underline(true)

	result := renderMenuLabel("File", 'F', baseStyle, shortcutStyle)
	if result == "" {
		t.Error("renderMenuLabel should not return empty string")
	}

	// Visible width should be " File " = 6
	width := lipgloss.Width(result)
	if width != 6 {
		t.Errorf("visible width = %d, want 6", width)
	}
}

func TestRenderMenuLabel_WithoutShortcut(t *testing.T) {
	baseStyle := lipgloss.NewStyle()
	shortcutStyle := lipgloss.NewStyle().Underline(true)

	result := renderMenuLabel("File", 0, baseStyle, shortcutStyle)
	if result == "" {
		t.Error("renderMenuLabel should not return empty string")
	}

	width := lipgloss.Width(result)
	if width != 6 {
		t.Errorf("visible width = %d, want 6", width)
	}
}

func TestRenderMenuLabel_ShortcutNotFound(t *testing.T) {
	baseStyle := lipgloss.NewStyle()
	shortcutStyle := lipgloss.NewStyle().Underline(true)

	// 'Z' is not in "File"
	result := renderMenuLabel("File", 'Z', baseStyle, shortcutStyle)
	if result == "" {
		t.Error("renderMenuLabel should not return empty string")
	}

	width := lipgloss.Width(result)
	if width != 6 {
		t.Errorf("visible width = %d, want 6", width)
	}
}

func TestRenderMenuLabel_AllMenuLabels(t *testing.T) {
	baseStyle := lipgloss.NewStyle()
	shortcutStyle := lipgloss.NewStyle().Underline(true)

	menus := defaultMenus()
	for _, mn := range menus {
		result := renderMenuLabel(mn.label, mn.shortcutKey, baseStyle, shortcutStyle)
		expectedWidth := len(mn.label) + 2 // " label "
		width := lipgloss.Width(result)
		if width != expectedWidth {
			t.Errorf("renderMenuLabel(%q, %q): visible width = %d, want %d",
				mn.label, string(mn.shortcutKey), width, expectedWidth)
		}
	}
}

func TestMenuBar_Render_ShortcutUnderline(t *testing.T) {
	m := NewMenuBar()
	styles := NewStyles()

	result := m.Render(styles, 80)
	if result == "" {
		t.Error("Render() should not return empty string")
	}

	// The visible width should be at least the sum of label widths
	width := lipgloss.Width(result)
	minWidth := 0
	for _, mn := range m.menus {
		minWidth += len(mn.label) + 2
	}
	if width < minWidth {
		t.Errorf("rendered bar width = %d, should be at least %d", width, minWidth)
	}
}

func TestMenuBar_HitTestBar(t *testing.T) {
	m := NewMenuBar()
	// Menu labels: " File " (6), " Edit " (6), " View " (6), " Accounts " (10), " Transactions " (14), " Securities " (12), " Reports " (9), " Help " (6)
	// Cumulative: 0-5=File, 6-11=Edit, 12-17=View, 18-27=Accounts, 28-41=Transactions, 42-53=Securities, 54-62=Reports, 63-68=Help

	tests := []struct {
		name string
		x    int
		want int
	}{
		{"File start", 0, 0},
		{"File end", 5, 0},
		{"Edit start", 6, 1},
		{"Edit end", 11, 1},
		{"View start", 12, 2},
		{"View end", 17, 2},
		{"Accounts start", 18, 3},
		{"Accounts end", 27, 3},
		{"Transactions start", 28, 4},
		{"Transactions end", 41, 4},
		{"Securities start", 42, 5},
		{"Securities end", 53, 5},
		{"Reports start", 54, 6},
		{"Reports end", 62, 6},
		{"Help start", 63, 7},
		{"Help end", 68, 7},
		{"Beyond menus", 69, -1},
		{"Negative x", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.HitTestBar(tt.x)
			if got != tt.want {
				t.Errorf("HitTestBar(%d) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

func TestMenuBar_HitTestBar_EmptyMenuBar(t *testing.T) {
	m := &MenuBar{}
	if got := m.HitTestBar(0); got != -1 {
		t.Errorf("HitTestBar(0) on empty menu bar = %d, want -1", got)
	}
}

func TestMenuBar_HitTestDropdown(t *testing.T) {
	m := NewMenuBar()
	m.Activate() // File menu active (7 items)

	tests := []struct {
		name string
		y    int
		want int
	}{
		{"first item", 0, 0},
		{"third item", 2, 2},
		{"last item", 7, 7},
		{"out of range", 8, -1},
		{"negative", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.HitTestDropdown(tt.y)
			if got != tt.want {
				t.Errorf("HitTestDropdown(%d) = %d, want %d", tt.y, got, tt.want)
			}
		})
	}
}

func TestMenuBar_HitTestDropdown_NotActive(t *testing.T) {
	m := NewMenuBar()
	if got := m.HitTestDropdown(0); got != -1 {
		t.Errorf("HitTestDropdown when not active = %d, want -1", got)
	}
}

func TestMenuBar_HitTestDropdown_EditMenu(t *testing.T) {
	m := NewMenuBar()
	m.ActivateMenu(1) // Edit menu (2 items: Undo, Redo)

	if got := m.HitTestDropdown(0); got != 0 {
		t.Errorf("HitTestDropdown(0) on Edit = %d, want 0", got)
	}
	if got := m.HitTestDropdown(1); got != 1 {
		t.Errorf("HitTestDropdown(1) on Edit = %d, want 1", got)
	}
	if got := m.HitTestDropdown(2); got != -1 {
		t.Errorf("HitTestDropdown(2) on Edit = %d, want -1", got)
	}
}

func TestMenuBar_DropdownBounds(t *testing.T) {
	m := NewMenuBar()

	// Not active
	colOffset, dropdownWidth, itemCount := m.DropdownBounds()
	if colOffset != 0 || dropdownWidth != 0 || itemCount != 0 {
		t.Errorf("DropdownBounds when not active = (%d, %d, %d), want (0, 0, 0)",
			colOffset, dropdownWidth, itemCount)
	}

	// Activate File menu (index 0)
	m.Activate()
	colOffset, dropdownWidth, itemCount = m.DropdownBounds()
	if colOffset != 0 {
		t.Errorf("File menu colOffset = %d, want 0", colOffset)
	}
	if itemCount != 8 {
		t.Errorf("File menu itemCount = %d, want 8", itemCount)
	}
	// Widest item is "Import Transactions..." (22 chars) + 4 padding = 26
	if dropdownWidth != 26 {
		t.Errorf("File menu dropdownWidth = %d, want 26", dropdownWidth)
	}
}

func TestMenuBar_DropdownBounds_SecondMenu(t *testing.T) {
	m := NewMenuBar()
	m.ActivateMenu(1) // Edit menu

	colOffset, dropdownWidth, itemCount := m.DropdownBounds()
	// Offset = " File " = 6
	if colOffset != 6 {
		t.Errorf("Edit menu colOffset = %d, want 6", colOffset)
	}
	if itemCount != 2 {
		t.Errorf("Edit menu itemCount = %d, want 2", itemCount)
	}
	// Widest item is "Undo" or "Redo" (4 chars) + 4 padding = 8
	if dropdownWidth != 8 {
		t.Errorf("Edit menu dropdownWidth = %d, want 8", dropdownWidth)
	}
}

func TestMenuBar_DropdownBounds_ThirdMenu(t *testing.T) {
	m := NewMenuBar()
	m.ActivateMenu(3) // Accounts menu

	colOffset, _, _ := m.DropdownBounds()
	// Offset = " File " + " Edit " + " View " = 6 + 6 + 6 = 18
	if colOffset != 18 {
		t.Errorf("Accounts menu colOffset = %d, want 18", colOffset)
	}
}

// TestMenuBar_SetMenuItemsBuilder_RebuildOnActivate covers TH-022's
// dynamic-population path: when a builder is registered for a menu
// index, opening that menu via Activate() replaces its items with the
// builder's output. The View menu starts as a single "Theme"
// placeholder; once the App registers a builder, opening the menu
// shows the actual list of themes.
func TestMenuBar_SetMenuItemsBuilder_RebuildOnActivate(t *testing.T) {
	m := NewMenuBar()
	m.SetMenuItemsBuilder(2, func() []menuItem {
		return []menuItem{
			{label: "default", action: MenuActionLoadTheme, data: "default"},
			{label: "✓ light", action: MenuActionLoadTheme, data: "light"},
		}
	})

	// Before activation the placeholder is still in place — the builder
	// only fires on user-triggered open.
	if len(m.menus[2].items) != 1 || m.menus[2].items[0].label != "Theme" {
		t.Fatalf("View menu initial items = %+v, want the static Theme placeholder", m.menus[2].items)
	}

	m.ActivateMenu(2)

	got := m.menus[2].items
	if len(got) != 2 {
		t.Fatalf("after ActivateMenu(2), len(items) = %d, want 2", len(got))
	}
	if got[1].label != "✓ light" {
		t.Errorf("got[1].label = %q, want %q", got[1].label, "✓ light")
	}
	if got[1].data != "light" {
		t.Errorf("got[1].data = %q, want %q", got[1].data, "light")
	}
}

// TestMenuBar_SetMenuItemsBuilder_RebuildOnNavigate — moving the
// cursor onto a menu with a builder via MoveLeft/MoveRight triggers a
// rebuild too, so the dropdown shows fresh content even when the user
// arrows over from a sibling menu instead of activating directly.
func TestMenuBar_SetMenuItemsBuilder_RebuildOnNavigate(t *testing.T) {
	m := NewMenuBar()
	calls := 0
	m.SetMenuItemsBuilder(2, func() []menuItem {
		calls++
		return []menuItem{{label: "fresh", action: MenuActionLoadTheme, data: "fresh"}}
	})

	m.ActivateMenu(0) // File
	if calls != 0 {
		t.Errorf("activating File should not call View's builder; calls = %d", calls)
	}

	m.MoveRight() // Edit
	m.MoveRight() // View
	if calls == 0 {
		t.Error("arrowing onto View should rebuild its items")
	}
	if got := m.menus[2].items[0].label; got != "fresh" {
		t.Errorf("View items[0].label = %q, want %q", got, "fresh")
	}

	prevCalls := calls
	m.MoveLeft()  // back to Edit
	m.MoveRight() // back onto View — should rebuild again
	if calls <= prevCalls {
		t.Error("returning to View should rebuild again so the active marker stays current")
	}
}

// TestMenuBar_SetMenuItemsBuilder_StaticMenusUnaffected — menus
// without a registered builder must keep their statically defined
// items even after activation, so the existing File/Edit/etc menus
// don't accidentally get cleared.
func TestMenuBar_SetMenuItemsBuilder_StaticMenusUnaffected(t *testing.T) {
	m := NewMenuBar()
	m.SetMenuItemsBuilder(2, func() []menuItem {
		return []menuItem{{label: "fresh", action: MenuActionLoadTheme, data: "fresh"}}
	})

	originalFileLen := len(m.menus[0].items)

	m.ActivateMenu(0)

	if got := len(m.menus[0].items); got != originalFileLen {
		t.Errorf("File items len = %d, want %d (no builder = unchanged)", got, originalFileLen)
	}
	if m.menus[0].items[0].label != "New File" {
		t.Errorf("File items[0].label = %q, want %q", m.menus[0].items[0].label, "New File")
	}
}

// TestMenuBar_SetMenuItemsBuilder_NilBuilder — passing a nil builder
// effectively unregisters; opening the menu must not panic and the
// static items must remain.
func TestMenuBar_SetMenuItemsBuilder_NilBuilder(t *testing.T) {
	m := NewMenuBar()
	m.SetMenuItemsBuilder(2, nil)
	m.ActivateMenu(2)

	if len(m.menus[2].items) != 1 || m.menus[2].items[0].label != "Theme" {
		t.Errorf("nil builder should leave static items intact, got %+v", m.menus[2].items)
	}
}

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"\033[31mred\033[0m", "red"},
		{"no escape", "no escape"},
		{"\033[1;31mbold red\033[0m normal", "bold red normal"},
	}

	for _, tt := range tests {
		result := stripAnsi(tt.input)
		if result != tt.expected {
			t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
