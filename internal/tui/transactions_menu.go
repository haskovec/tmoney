// Transactions menu: the items that act on the open register.

package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

const (
	// closedAccountChangesNotice answers a change to a closed account's
	// register, from a register key or from the Transactions menu.
	closedAccountChangesNotice = "Account is closed — reopen to make changes"
	// needRegisterNotice answers a Transactions menu item picked where no
	// register is open.
	needRegisterNotice = "Open an account register first"
)

// Each Transactions menu item below does what the register's own key does,
// with the same checks, so a menu pick and a key press cannot differ. In the
// investment register the items do what its keys do. Anywhere else they say
// where they work instead of doing nothing.

// menuNewTransaction is n in the open register.
func (a *App) menuNewTransaction() (tea.Model, tea.Cmd) {
	switch a.currentView {
	case ViewRegister:
		if a.selectedAccountClosed() {
			return a.notice("Account is closed — reopen to add transactions")
		}
		return a, a.loadTransactionDialogData()
	case ViewInvestmentRegister:
		if a.investmentRegisterClosed() {
			return a.notice(closedAccountChangesNotice)
		}
		a.openInvestmentTypeSelector(false)
		return a, nil
	}
	return a.notice(needRegisterNotice)
}

// menuNewTransfer is t in the open register. An investment register moves
// cash with its own Transfer Cash transaction.
func (a *App) menuNewTransfer() (tea.Model, tea.Cmd) {
	switch a.currentView {
	case ViewRegister:
		if a.selectedAccountClosed() {
			return a.notice("Account is closed — reopen to add transfers")
		}
		return a, a.transfer.open(a.transferDeps())
	case ViewInvestmentRegister:
		return a.notice("In an investment register, use New Transaction → Transfer Cash")
	}
	return a.notice(needRegisterNotice)
}

// menuEditTransaction is Enter on the selected register row.
func (a *App) menuEditTransaction() (tea.Model, tea.Cmd) {
	switch a.currentView {
	case ViewRegister:
		if a.cashRegisterClosed() {
			return a.notice(closedAccountChangesNotice)
		}
		return a.openEditTransactionFlow()
	case ViewInvestmentRegister:
		if a.investmentRegisterClosed() {
			return a.notice(closedAccountChangesNotice)
		}
		return a.editInvestmentTransaction()
	}
	return a.notice(needRegisterNotice)
}

// menuDeleteTransaction is d on the selected register row.
func (a *App) menuDeleteTransaction() (tea.Model, tea.Cmd) {
	switch a.currentView {
	case ViewRegister:
		if a.cashRegisterClosed() {
			return a.notice(closedAccountChangesNotice)
		}
		return a.showDeleteConfirmation()
	case ViewInvestmentRegister:
		if a.investmentRegisterClosed() {
			return a.notice(closedAccountChangesNotice)
		}
		return a.confirmDeleteInvestmentTransaction()
	}
	return a.notice(needRegisterNotice)
}

// menuSearchTransactions is / in the open register: the search, or the
// investment register's security filter. The table takes focus, so the keys
// after the search act on the rows it found.
func (a *App) menuSearchTransactions() (tea.Model, tea.Cmd) {
	switch a.currentView {
	case ViewRegister:
		a.focusRegisterTable(a.register.table)
		a.startRegisterSearch()
		return a, nil
	case ViewInvestmentRegister:
		a.focusRegisterTable(a.investmentRegister.table)
		a.startInvestmentRegisterFilter()
		return a, nil
	}
	return a.notice(needRegisterNotice)
}

// focusRegisterTable moves focus from the sidebar to a register's table.
func (a *App) focusRegisterTable(t *widget.Table) {
	a.sidebar.SetFocused(false)
	if t != nil {
		t.SetFocused(true)
	}
}

// cashRegisterClosed reports whether the open register's account is closed.
func (a *App) cashRegisterClosed() bool {
	d := a.register.data
	return d != nil && d.account != nil && d.account.IsClosed()
}

// investmentRegisterClosed reports whether the open investment register's
// account is closed.
func (a *App) investmentRegisterClosed() bool {
	return a.investmentRegister.showsClosedBanner()
}

// notice shows text as an alert in the status bar.
func (a *App) notice(text string) (tea.Model, tea.Cmd) {
	a.statusbar.AddNotification(text, widget.NotificationAlert)
	return a, nil
}
