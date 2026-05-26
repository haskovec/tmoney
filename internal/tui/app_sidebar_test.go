package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

func TestApp_HandleSidebarKeys_NewAccountShortcut(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
	}
	// Sidebar is focused by default in NewSidebar()

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(msg)

	// The 'n' key in dashboard view (sidebar focused) should return a command
	// to load the new account dialog data
	if cmd == nil {
		t.Error("pressing 'n' in dashboard with sidebar focused should return a command")
	}
}

func TestApp_HandleSidebarKeys_NewAccountNotWhenUnfocused(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
	}
	app.sidebar.SetFocused(false)

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(msg)

	// When sidebar is not focused, 'n' should not trigger anything
	if cmd != nil {
		t.Error("pressing 'n' with sidebar unfocused should not return a command")
	}
}

func TestApp_MouseClick_Sidebar_SingleClick_OnlySelects(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking]

	// Click on the account row (y=2 = content row 1 = Checking item)
	msg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.cursor != 1 {
		t.Errorf("sidebar cursor = %d, want 1", updatedApp.sidebar.cursor)
	}
	// Single click selects only — no open command, view does not switch.
	if cmd != nil {
		t.Error("single click should not return an open command")
	}
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("view should still be Dashboard, got %v", updatedApp.currentView)
	}
}

func TestApp_MouseClick_Sidebar_DoubleClick_OpensAccount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	now := time.Unix(0, 0)
	app.sidebarClicks = widget.NewClickTracker(400 * time.Millisecond)
	app.sidebarClicks.SetNowFn(func() time.Time { return now })

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)

	click := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}

	// First click — selects only.
	_, cmd := app.Update(click)
	if cmd != nil {
		t.Fatal("first click should not return an open command")
	}

	// Second click within threshold on same row — drills in.
	now = now.Add(100 * time.Millisecond)
	model, cmd := app.Update(click)
	updatedApp := model.(*App)

	if cmd == nil {
		t.Fatal("double click should return an open command")
	}
	openMsg, ok := cmd().(mouseOpenAccountMsg)
	if !ok {
		t.Fatalf("expected mouseOpenAccountMsg, got %T", cmd())
	}
	if openMsg.accountID != accounts[0].ID {
		t.Errorf("opened account = %v, want %v", openMsg.accountID, accounts[0].ID)
	}
	// View switch is still deferred — currentView only changes on the message.
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("view should still be Dashboard before message processed, got %v", updatedApp.currentView)
	}
}

func TestApp_MouseOpenAccountMsg_SwitchesView(t *testing.T) {
	checking := testAccount("Checking", account.TypeChecking)
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetAccounts([]*account.Account{checking}, nil)
	app.sidebar.MoveDown()
	app.sidebar.Select()

	// Simulate the deferred message
	msg := mouseOpenAccountMsg{accountID: checking.ID}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewRegister {
		t.Errorf("currentView = %v, want ViewRegister", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load register data")
	}
}

func TestApp_MouseClick_Sidebar_GroupHeader_JustMovesCursor(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings] = 3 items

	// Click on group header (y=1 = content row 0 = Bank Accounts)
	msg := tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Items should remain unchanged (no collapse ever)
	if updatedApp.sidebar.ItemCount() != 3 {
		t.Errorf("ItemCount = %d, want 3", updatedApp.sidebar.ItemCount())
	}
	if updatedApp.sidebar.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (group header)", updatedApp.sidebar.cursor)
	}
}
