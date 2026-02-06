package tui

import "testing"

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

	if m.MenuCount() != 5 {
		t.Fatalf("expected 5 menus, got %d", m.MenuCount())
	}

	expectedLabels := []string{"File", "Accounts", "Transactions", "Reports", "Help"}
	for i, mn := range m.menus {
		if mn.label != expectedLabels[i] {
			t.Errorf("menu[%d].label = %q, want %q", i, mn.label, expectedLabels[i])
		}
	}
}

func TestMenuBar_FileMenuItems(t *testing.T) {
	m := NewMenuBar()

	fileMenu := m.menus[0]
	if len(fileMenu.items) != 5 {
		t.Fatalf("File menu: expected 5 items, got %d", len(fileMenu.items))
	}

	expectedItems := []struct {
		label  string
		action MenuAction
	}{
		{"New File", MenuActionNewFile},
		{"Open File", MenuActionOpenFile},
		{"Open Recent", MenuActionOpenRecent},
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

	// File menu has 5 items
	m.MoveDown()
	if m.ItemCursor() != 1 {
		t.Errorf("after MoveDown, itemCursor = %d, want 1", m.ItemCursor())
	}

	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	if m.ItemCursor() != 4 {
		t.Errorf("after 4x MoveDown, itemCursor = %d, want 4", m.ItemCursor())
	}

	// Should not go past last item
	m.MoveDown()
	if m.ItemCursor() != 4 {
		t.Errorf("itemCursor should stay at 4, got %d", m.ItemCursor())
	}

	m.MoveUp()
	if m.ItemCursor() != 3 {
		t.Errorf("after MoveUp, itemCursor = %d, want 3", m.ItemCursor())
	}

	// Move all the way up
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
	action := m.Select()
	if action != MenuActionNewFile {
		t.Errorf("expected MenuActionNewFile, got %d", action)
	}

	// Select should deactivate the menu
	if m.IsActive() {
		t.Error("menu should be deactivated after Select()")
	}
}

func TestMenuBar_SelectExitAction(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// Move to "Exit" (index 4 in File menu)
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()
	m.MoveDown()

	action := m.Select()
	if action != MenuActionExit {
		t.Errorf("expected MenuActionExit, got %d", action)
	}
}

func TestMenuBar_SelectFromDifferentMenu(t *testing.T) {
	m := NewMenuBar()
	m.Activate()

	// Move to Accounts menu
	m.MoveRight()

	// First item is "New Account"
	action := m.Select()
	if action != MenuActionNewAccount {
		t.Errorf("expected MenuActionNewAccount, got %d", action)
	}
}

func TestMenuBar_SelectNoMenu(t *testing.T) {
	m := &MenuBar{} // Empty menu bar

	action := m.Select()
	if action != MenuActionNone {
		t.Errorf("expected MenuActionNone from empty menu bar, got %d", action)
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
	if current.label != "Accounts" {
		t.Errorf("after MoveRight, CurrentMenu().label = %q, want %q", current.label, "Accounts")
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
	m.MoveRight() // Move to Accounts
	styles := NewStyles()

	dropdown, offset := m.RenderDropdown(styles)
	if dropdown == "" {
		t.Error("RenderDropdown() should return non-empty for Accounts menu")
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
	m.MoveRight() // Accounts
	m.MoveRight() // Transactions
	styles := NewStyles()

	_, offset := m.RenderDropdown(styles)

	// Offset = " File " + " Accounts " = 6 + 10 = 16
	expectedOffset := (len("File") + 2) + (len("Accounts") + 2)
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

func TestMenuBar_TransactionsMenuItems(t *testing.T) {
	m := NewMenuBar()

	txnMenu := m.menus[2]
	if txnMenu.label != "Transactions" {
		t.Fatalf("expected Transactions menu at index 2, got %q", txnMenu.label)
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

func TestMenuBar_ReportsMenuItems(t *testing.T) {
	m := NewMenuBar()

	reportsMenu := m.menus[3]
	if reportsMenu.label != "Reports" {
		t.Fatalf("expected Reports menu at index 3, got %q", reportsMenu.label)
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

	helpMenu := m.menus[4]
	if helpMenu.label != "Help" {
		t.Fatalf("expected Help menu at index 4, got %q", helpMenu.label)
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
