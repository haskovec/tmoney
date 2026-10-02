package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// handleMenuKeys handles keyboard input when the menu bar is active.
func (a *App) handleMenuKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Menu):
		a.menubar.Deactivate()
		return a, nil

	case key.Matches(msg, a.keys.Left):
		a.menubar.MoveLeft()
		return a, nil

	case key.Matches(msg, a.keys.Right):
		a.menubar.MoveRight()
		return a, nil

	case key.Matches(msg, a.keys.Up):
		a.menubar.MoveUp()
		return a, nil

	case key.Matches(msg, a.keys.Down):
		a.menubar.MoveDown()
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		action, data := a.menubar.Select()
		return a.handleMenuAction(action, data)

	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit
	}

	return a, nil
}

// handleMenuAction processes a menu item selection. The data string
// carries action-specific context populated from menuItem.data — for
// example, widget.MenuActionLoadTheme uses it to carry the theme ID. It is
// the empty string for actions that don't need a payload.
func (a *App) handleMenuAction(action widget.MenuAction, data string) (tea.Model, tea.Cmd) {
	switch action {
	case widget.MenuActionNewFile:
		a.menubar.Deactivate()
		a.file.mode = fileDialogModeNew
		a.file.dlg = buildNewFileDialog()
		return a, nil

	case widget.MenuActionOpenFile:
		a.menubar.Deactivate()
		a.openBrowseDialog(db.DefaultDirectory())
		return a, nil

	case widget.MenuActionOpenRecent:
		a.menubar.Deactivate()
		a.file.mode = fileDialogModeOpenRecent
		var recent []string
		if a.cfg != nil {
			recent = a.cfg.RecentFiles
		}
		a.file.dlg = buildOpenRecentDialog(recent)
		return a, nil

	case widget.MenuActionImportTransactions:
		a.menubar.Deactivate()
		return a, a.startImport()

	case widget.MenuActionCreateBackup:
		a.menubar.Deactivate()
		return a, a.createManualBackupCmd()

	case widget.MenuActionRestoreBackup:
		a.menubar.Deactivate()
		d, backups, err := buildRestoreBackupDialog(a.db.Path())
		if err != nil {
			a.err = err
			return a, nil
		}
		a.backupDialog = &backupDialogState{dialog: d, backups: backups}
		return a, nil

	case widget.MenuActionCloseFile:
		a.quitting = true
		return a, tea.Quit

	case widget.MenuActionExit:
		a.quitting = true
		return a, tea.Quit

	case widget.MenuActionDashboard:
		a.switchView(ViewDashboard)

	case widget.MenuActionNetWorth:
		a.switchView(ViewReports)
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()), false)

	case widget.MenuActionSpendingByCategory:
		a.switchView(ViewReports)
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeSpending, now.Year(), int(now.Month()), false)

	case widget.MenuActionSecurities:
		a.switchView(ViewSecurities)
		return a, a.loadSecurityViewData()

	case widget.MenuActionPrices:
		a.switchView(ViewPrices)
		return a, a.prices.load(a.priceDeps())

	case widget.MenuActionStockSplit:
		a.stockSplit.preSelectedID = nil
		return a, a.loadStockSplitDialogData()

	case widget.MenuActionMerger:
		a.merger.preSelectedID = nil
		return a, a.loadMergerDialogData()

	case widget.MenuActionSpinOff:
		a.spinOff.preSelectedID = nil
		return a, a.loadSpinOffDialogData()

	case widget.MenuActionCorporateActions:
		a.corporateActionViewFilter = ""
		a.switchView(ViewCorporateActions)
		return a, a.loadCorporateActionViewData()

	case widget.MenuActionNewAccount:
		return a, a.loadNewAccountDialogData()

	case widget.MenuActionNewLoan:
		a.menubar.Deactivate()
		return a, a.loan.open(a.loanDeps())

	case widget.MenuActionEditAccount:
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.loadEditAccountDialogData()
		}

	case widget.MenuActionCloseAccount:
		a.menubar.Deactivate()
		if a.sidebar.SelectedAccountID() != types.NilID {
			a.showCloseAccountDialog()
		}
		return a, nil

	case widget.MenuActionReopenAccount:
		a.menubar.Deactivate()
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.reopenSelectedAccount()
		}
		return a, nil

	case widget.MenuActionDeleteAccount:
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.deleteSelectedAccount()
		}

	case widget.MenuActionReconcileAccount:
		a.menubar.Deactivate()
		if a.sidebar.SelectedAccountID() != types.NilID {
			switch {
			case a.refuseInvestmentReconcile():
				// Before the closed check: reopening would not help.
			case a.selectedAccountClosed():
				a.statusbar.AddNotification("Account is closed — reopen to reconcile", widget.NotificationAlert)
			default:
				a.showStartReconciliationDialog()
			}
		}
		return a, nil

	case widget.MenuActionNewTransaction:
		if a.currentView == ViewRegister {
			if a.selectedAccountClosed() {
				a.statusbar.AddNotification("Account is closed — reopen to add transactions", widget.NotificationAlert)
				return a, nil
			}
			return a, a.loadTransactionDialogData()
		}

	case widget.MenuActionNewTransfer:
		if a.currentView == ViewRegister {
			if a.selectedAccountClosed() {
				a.statusbar.AddNotification("Account is closed — reopen to add transfers", widget.NotificationAlert)
				return a, nil
			}
			return a, a.transfer.open(a.transferDeps())
		}

	case widget.MenuActionLinkTransfers:
		a.menubar.Deactivate()
		return a, a.startLinkTransfers()

	case widget.MenuActionNewPaycheckSchedule:
		a.menubar.Deactivate()
		return a, a.paycheck.open(a.paycheckDeps())

	case widget.MenuActionUndo:
		a.menubar.Deactivate()
		return a, a.performUndo()

	case widget.MenuActionRedo:
		a.menubar.Deactivate()
		return a, a.performRedo()

	case widget.MenuActionKeyboardShortcuts:
		a.menubar.Deactivate()
		a.showHelp = true
		return a, nil

	case widget.MenuActionAbout:
		a.menubar.Deactivate()
		a.showAboutDialog()
		return a, nil

	case widget.MenuActionLoadTheme:
		a.menubar.Deactivate()
		if data == "" {
			return a, nil
		}
		return a, a.reloadTheme(data)

	case widget.MenuActionToggleClosedPositions:
		return a.toggleClosedPositions()

	case widget.MenuActionNone:
		// No action
	}

	return a, nil
}

// toggleClosedPositions flips cfg.ShowClosedPositions, persists the
// change (best-effort — Save() is a no-op under `go test`), and
// reloads whichever view is currently displaying a valuation so the
// new IncludeClosed setting takes effect immediately. Views that
// don't read the flag are left untouched.
func (a *App) toggleClosedPositions() (tea.Model, tea.Cmd) {
	a.menubar.Deactivate()
	if a.cfg == nil {
		return a, nil
	}
	a.cfg.ShowClosedPositions = !a.cfg.ShowClosedPositions
	_ = a.cfg.Save()

	// Reload the active view so its valuation reflects the new toggle.
	switch a.currentView {
	case ViewDashboard:
		return a, a.dashboard.load(a.dashboardDeps())
	case ViewInvestmentRegister:
		if a.investmentRegister.data != nil && a.investmentRegister.data.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.data.account.ID)
		}
	case ViewPortfolio:
		if a.portfolio.data != nil && a.portfolio.data.account != nil {
			return a, a.portfolio.load(a.portfolioDeps(), a.portfolio.data.account.ID)
		}
	}
	return a, nil
}

// toggleMenu opens the menu at the given index, or closes it if already open at that index.
func (a *App) toggleMenu(index int) {
	if a.menubar.IsActive() && a.menubar.Cursor() == index {
		a.menubar.Deactivate()
	} else {
		a.menubar.ActivateMenu(index)
	}
}

// switchView changes the current view and stores the previous view.
func (a *App) switchView(v View) {
	if a.currentView != v {
		if e, ok := viewFor(a.currentView); ok && e.leave != nil {
			e.leave(a)
		}
		a.previousView = a.currentView
		a.currentView = v
		a.updateStatusBar()

		// Set focus appropriately for the new view
		if a.sidebar != nil {
			if e, ok := viewFor(v); ok {
				e.focus(a)
			}
		}
	}
}
