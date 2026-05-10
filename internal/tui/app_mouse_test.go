package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
)

func TestApp_MouseClick_MenuBar_OpensDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on "File" label (x=2, y=0)
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Error("menu bar should be active after clicking File label")
	}
	if updatedApp.menubar.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 (File)", updatedApp.menubar.Cursor())
	}
}

func TestApp_MouseClick_MenuBar_ToggleDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click File to open
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Fatal("menu should be active after first click")
	}

	// Click File again to close
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("menu should be deactivated after clicking same label again")
	}
}

func TestApp_MouseClick_MenuBar_SwitchMenu(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click File to open
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Click Edit (x=8, offset for " File " = 6)
	msg = tea.MouseClickMsg{X: 8, Y: 0, Button: tea.MouseLeft}
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Error("menu should still be active")
	}
	if updatedApp.menubar.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 (Edit)", updatedApp.menubar.Cursor())
	}
}

func TestApp_MouseClick_Dropdown_SelectsItem(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Open Edit menu
	app.menubar.ActivateMenu(1)

	// Click first item in Edit dropdown (Undo) at y=1 (first dropdown row)
	// Edit dropdown offset = 6 (width of " File ")
	msg := tea.MouseClickMsg{X: 8, Y: 1, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should be deactivated after selection
	if updatedApp.menubar.IsActive() {
		t.Error("menu should be deactivated after selecting a dropdown item")
	}
}

func TestApp_MouseClick_OutsideMenu_ClosesDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Open File menu
	app.menubar.Activate()

	// Click in content area (far from menu)
	msg := tea.MouseClickMsg{X: 50, Y: 10, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("menu should close when clicking outside dropdown")
	}
}

func TestApp_MouseClick_Table_SelectsRow(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetFocused(false)
	app.table.SetRows([][]string{{"row1"}, {"row2"}, {"row3"}})

	sidebarWidth := app.styles.SidebarWidth()

	// Click on the second data row in the table
	// Table starts at content-relative row 3 (1 padding + 1 title + 1 separator)
	// So screen row = 1 (header) + 3 (offset) + 1 (header row of table) + 1 (second data row) = row 5+1=6
	// Actually: screen Y = 1 (menu bar) + 3 (content offset) + 0 (table header) + 2 (second data row)
	// contentY = Y - 1 = 4, tableY = contentY - 3 = 1, which is the header. For data row 1, we need tableY=2
	// So Y = 1 + 3 + 2 = 6
	msg := tea.MouseClickMsg{
		X:      sidebarWidth + 5,
		Y:      6,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.table.Cursor() != 1 {
		t.Errorf("table cursor = %d, want 1", updatedApp.table.Cursor())
	}
}

func TestApp_MouseClick_FocusSwitchToTable(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	// Start with sidebar focused
	app.sidebar.SetFocused(true)
	app.table.SetFocused(false)

	sidebarWidth := app.styles.SidebarWidth()

	// Click in content area (right of sidebar)
	msg := tea.MouseClickMsg{
		X:      sidebarWidth + 5,
		Y:      5,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.IsFocused() {
		t.Error("sidebar should not be focused after clicking content area")
	}
	if !updatedApp.table.IsFocused() {
		t.Error("table should be focused after clicking content area")
	}
}

func TestApp_MouseClick_FocusSwitchToSidebar(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Click in sidebar area
	msg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.sidebar.IsFocused() {
		t.Error("sidebar should be focused after clicking in sidebar area")
	}
	if updatedApp.table.IsFocused() {
		t.Error("table should not be focused after clicking in sidebar area")
	}
}

func TestApp_MouseWheel_ScrollsTable(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)
	app.table.SetRows([][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}})

	// Scroll down
	msg := tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelDown}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.table.Cursor() != 1 {
		t.Errorf("after wheel down, cursor = %d, want 1", updatedApp.table.Cursor())
	}

	// Scroll up
	msg = tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelUp}
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if updatedApp.table.Cursor() != 0 {
		t.Errorf("after wheel up, cursor = %d, want 0", updatedApp.table.Cursor())
	}
}

func TestApp_MouseWheel_ScrollsSidebar(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Visa", account.TypeCreditCard),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// Sidebar focused by default

	// Scroll down
	msg := tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.cursor != 1 {
		t.Errorf("after wheel down, sidebar cursor = %d, want 1", updatedApp.sidebar.cursor)
	}
}

func TestApp_MouseClick_IgnoredDuringHelpOverlay(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		showHelp:    true,
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on menu bar while help overlay is visible
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should not open
	if updatedApp.menubar.IsActive() {
		t.Error("menu should not open while help overlay is visible")
	}
}

func TestApp_MouseClick_Dialog_CloseButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	// Click the [x] close button
	contentWidth := dlg.Width() - dialogHorizontalOverhead
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)
	clickX := startCol + 3 + contentWidth - 2
	clickY := startRow + 2

	msg := tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking [x]")
	}
}

func TestApp_MouseClick_Dialog_SubmitButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	submitted := false
	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { submitted = true; return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	contentWidth := dlg.Width() - dialogHorizontalOverhead
	buttonRow := dlg.ContentHeight() - 1
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Find OK button position
	var okX int
	for x := range contentWidth {
		hit := dlg.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 1 {
			okX = x
			break
		}
	}

	msg := tea.MouseClickMsg{
		X:      startCol + 3 + okX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking OK")
	}
	// Execute the command to trigger the confirm action
	if cmd != nil {
		cmd()
	}
	if !submitted {
		t.Error("confirm action should have been triggered")
	}
}

func TestApp_MouseClick_Dialog_CancelButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	contentWidth := dlg.Width() - dialogHorizontalOverhead
	buttonRow := dlg.ContentHeight() - 1
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Find Cancel button position
	var cancelX int
	for x := range contentWidth {
		hit := dlg.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 0 {
			cancelX = x
			break
		}
	}

	msg := tea.MouseClickMsg{
		X:      startCol + 3 + cancelX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking Cancel")
	}
}

func TestApp_MouseClick_Dialog_OutsideNoAction(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	// Click outside the dialog (top-left corner)
	msg := tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog == nil || !updatedApp.confirmDialog.IsVisible() {
		t.Error("dialog should remain open when clicking outside")
	}
}

func TestApp_MouseClick_HelpOverlay_StillBlocked(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		showHelp:    true,
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on menu bar while help overlay is visible
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should not open
	if updatedApp.menubar.IsActive() {
		t.Error("menu should not open while help overlay is visible")
	}
}

func TestApp_MouseWheel_Dialog_ListField(t *testing.T) {
	dlg := NewDialog("Browse")
	dlg.AddListField("File", []string{"../", "docs/", "main.go", "go.mod", "go.sum"}, 0, 3)
	dlg.SetFocusIndex(0) // Focus on list field
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Wheel down within dialog bounds
	msg := tea.MouseWheelMsg{
		X:      startCol + 10,
		Y:      startRow + 5,
		Button: tea.MouseWheelDown,
	}
	app.Update(msg)

	if dlg.Fields()[0].SelectedIndex != 1 {
		t.Errorf("SelectedIndex after wheel down = %d, want 1", dlg.Fields()[0].SelectedIndex)
	}
}

func TestApp_MouseRelease_Ignored(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Mouse release should be ignored
	msg := tea.MouseReleaseMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("mouse release should not activate menu")
	}
}
