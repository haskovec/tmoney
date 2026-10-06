package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// sidebarLoadedMsg is sent when sidebar data has been loaded.
type sidebarLoadedMsg struct {
	accounts []*account.Account
}

// loadSidebarData returns a command that loads accounts and balances for the sidebar.
func (a *App) loadSidebarData() tea.Cmd {
	return func() tea.Msg {
		if a.services.Account == nil {
			return nil
		}
		// Load all accounts (including closed) so the sidebar can show a
		// dimmed "Closed Accounts" section at the bottom. Pickers elsewhere
		// still use List(true) to exclude closed accounts.
		accounts, err := a.services.Account.List(false)
		if err != nil {
			return errMsg{err: err}
		}
		return sidebarLoadedMsg{accounts: accounts}
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
				a.portfolio.data = nil // Clear old data while loading
				a.switchView(ViewPortfolio)
				return a, a.portfolio.load(a.portfolioDeps(), accountID)
			}
			a.register.data = nil // Clear old data while loading
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
// A single click moves the cursor; a double click on an account opens the
// register/portfolio. A double click is two clicks in a row on the same row,
// so a click on a group header goes to the tracker too, and Select opens
// nothing on a header.
func (a *App) handleMouseSidebar(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	idx := a.sidebar.HitTest(contentY)
	if idx < 0 {
		return a, nil
	}

	a.focusSidebar()
	a.sidebar.SetCursor(idx)

	if a.sidebarClicks == nil {
		a.sidebarClicks = widget.NewClickTracker(widget.DoubleClickThreshold)
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
