package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// sidebarLoadedMsg is sent when sidebar data has been loaded.
type sidebarLoadedMsg struct {
	accounts []*account.Account
	balances map[types.ID]*account.Balance
}

// loadSidebarData returns a command that loads accounts and balances for the sidebar.
func (a *App) loadSidebarData() tea.Cmd {
	return func() tea.Msg {
		if a.accountSvc == nil {
			return nil
		}
		accounts, err := a.accountSvc.List(true)
		if err != nil {
			return errMsg{err: err}
		}
		balances, err := a.accountSvc.GetAllBalances()
		if err != nil {
			return errMsg{err: err}
		}
		return sidebarLoadedMsg{accounts: accounts, balances: balances}
	}
}

// handleSidebarKeys handles keyboard navigation for the sidebar.
func (a *App) handleSidebarKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !a.sidebar.IsFocused() {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.sidebar.MoveUp()
		return a, nil

	case key.Matches(msg, a.keys.Down):
		a.sidebar.MoveDown()
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		if a.sidebar.Select() {
			accountID := a.sidebar.SelectedAccountID()
			acct := a.sidebar.SelectedAccount()
			if acct != nil && acct.Type.IsInvestmentType() {
				a.portfolioData = nil // Clear old data while loading
				a.switchView(ViewPortfolio)
				return a, a.loadPortfolioData(accountID)
			}
			a.register = nil // Clear old data while loading
			a.switchView(ViewRegister)
			return a, a.loadRegisterData(accountID)
		}
		return a, nil

	case key.Matches(msg, a.keys.New):
		return a, a.loadNewAccountDialogData()
	}

	return a, nil
}

// handleMouseSidebar handles mouse clicks in the sidebar area.
// Single click on an account moves the cursor; a double click on the same
// account opens the register/portfolio.
func (a *App) handleMouseSidebar(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	idx := a.sidebar.HitTest(contentY)
	if idx < 0 {
		return a, nil
	}

	a.focusSidebar()
	a.sidebar.SetCursor(idx)

	item := a.sidebar.CursorItem()
	if item == nil {
		return a, nil
	}

	// Group headers: just move cursor
	if item.kind == sidebarItemGroup {
		return a, nil
	}

	// Account item - require a double click to drill in.
	if a.sidebarClicks == nil {
		a.sidebarClicks = NewClickTracker(doubleClickThreshold)
	}
	if !a.sidebarClicks.Click(idx) {
		return a, nil
	}

	// Defer the view switch to the next Update cycle.
	// This avoids a Bubbletea renderer issue where switching views directly
	// inside a mouse event handler causes the menu bar to disappear.
	if a.sidebar.Select() {
		accountID := a.sidebar.SelectedAccountID()
		return a, func() tea.Msg {
			return mouseOpenAccountMsg{accountID: accountID}
		}
	}

	return a, nil
}
