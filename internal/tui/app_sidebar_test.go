package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// testAccount creates an open USD account with the given name and type.
func testAccount(name string, accountType account.Type) *account.Account {
	return account.NewAccount(name, accountType, "USD", types.ZeroMoney, types.Today())
}

func TestApp_HandleSidebarKeys_NewAccountShortcut(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     sidebar.New(),
		statusbar:   widget.NewStatusBar(),
	}
	// Sidebar is focused by default in sidebar.New()

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
		sidebar:     sidebar.New(),
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
		sidebar:     sidebar.New(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts)
	// items: [Bank Accounts, Checking]

	// Click on the account row (y=2 = content row 1 = Checking item)
	msg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.Cursor() != 1 {
		t.Errorf("sidebar cursor = %d, want 1", updatedApp.sidebar.Cursor())
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
		sidebar:     sidebar.New(),
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
	app.sidebar.SetAccounts(accounts)

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
		sidebar:     sidebar.New(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetAccounts([]*account.Account{checking})
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
		sidebar:     sidebar.New(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
	}
	app.sidebar.SetAccounts(accounts)
	// items: [Bank Accounts, Checking, Savings] = 3 items

	// Click on group header (y=1 = content row 0 = Bank Accounts)
	msg := tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Items should remain unchanged (no collapse ever)
	if updatedApp.sidebar.ItemCount() != 3 {
		t.Errorf("ItemCount = %d, want 3", updatedApp.sidebar.ItemCount())
	}
	if updatedApp.sidebar.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 (group header)", updatedApp.sidebar.Cursor())
	}
}

// sidebarClickApp returns an App whose sidebar lists Checking under its group
// header, and a func that clicks a sidebar row 100ms after the last click.
// Row 0 is the header and row 1 is Checking.
func sidebarClickApp(t *testing.T) (*App, func(row int) tea.Cmd) {
	t.Helper()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		sidebar:     sidebar.New(),
		statusbar:   widget.NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetAccounts([]*account.Account{testAccount("Checking", account.TypeChecking)})

	now := time.Unix(0, 0)
	app.sidebarClicks = widget.NewClickTracker(400 * time.Millisecond)
	app.sidebarClicks.SetNowFn(func() time.Time { return now })
	click := func(row int) tea.Cmd {
		now = now.Add(100 * time.Millisecond)
		_, cmd := app.Update(tea.MouseClickMsg{X: 5, Y: row + 1, Button: tea.MouseLeft})
		return cmd
	}
	return app, click
}

func TestApp_MouseClick_Sidebar_GroupHeader_DoubleClickOpensNothing(t *testing.T) {
	app, click := sidebarClickApp(t)
	click(0)
	if cmd := click(0); cmd != nil {
		t.Error("a double click on a group header should not return an open command")
	}
	if !app.sidebar.SelectedAccountID().IsNil() {
		t.Error("a double click on a group header should not select an account")
	}
}

// A double click is two clicks in a row on the same row. A click on a group
// header between two clicks on an account breaks it.
func TestApp_MouseClick_Sidebar_HeaderClickBreaksADoubleClick(t *testing.T) {
	_, click := sidebarClickApp(t)
	click(1)
	click(0)
	if cmd := click(1); cmd != nil {
		t.Error("account, header, account should not open the account")
	}
}
