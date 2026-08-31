package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
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
		a.fileDialogMode = fileDialogModeNew
		a.fileDialog = buildNewFileDialog()
		return a, nil

	case widget.MenuActionOpenFile:
		a.menubar.Deactivate()
		a.openBrowseDialog(db.DefaultDirectory())
		return a, nil

	case widget.MenuActionOpenRecent:
		a.menubar.Deactivate()
		a.fileDialogMode = fileDialogModeOpenRecent
		var recent []string
		if a.cfg != nil {
			recent = a.cfg.RecentFiles
		}
		a.fileDialog = buildOpenRecentDialog(recent)
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
		return a, a.loadPriceViewData()

	case widget.MenuActionStockSplit:
		a.stockSplitDialogPreSelectedID = nil
		return a, a.loadStockSplitDialogData()

	case widget.MenuActionMerger:
		a.mergerDialogPreSelectedID = nil
		return a, a.loadMergerDialogData()

	case widget.MenuActionSpinOff:
		a.spinOffDialogPreSelectedID = nil
		return a, a.loadSpinOffDialogData()

	case widget.MenuActionCorporateActions:
		a.corporateActionViewFilter = ""
		a.switchView(ViewCorporateActions)
		return a, a.loadCorporateActionViewData()

	case widget.MenuActionNewAccount:
		return a, a.loadNewAccountDialogData()

	case widget.MenuActionNewLoan:
		a.menubar.Deactivate()
		return a, a.loadLoanWizardData()

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
			if a.selectedAccountClosed() {
				a.statusbar.AddNotification("Account is closed — reopen to reconcile", widget.NotificationAlert)
			} else {
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
			return a, a.loadTransferDialogData()
		}

	case widget.MenuActionLinkTransfers:
		a.menubar.Deactivate()
		return a, a.startLinkTransfers()

	case widget.MenuActionNewPaycheckSchedule:
		a.menubar.Deactivate()
		return a, a.loadPaycheckWizardData()

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

// valuationOptions returns the ValuationOptions struct that callers
// should pass when requesting an investment account valuation. The
// IncludeClosed flag is sourced from cfg.ShowClosedPositions so the
// View → Show closed positions toggle plumbs through to every
// valuation-bearing view (dashboard cards, register header, portfolio
// holdings list). A nil cfg falls back to IncludeClosed=false so the
// helper is safe to call from tests that don't construct a config.
func (a *App) valuationOptions() investment.ValuationOptions {
	if a.cfg == nil {
		return investment.ValuationOptions{}
	}
	return investment.ValuationOptions{IncludeClosed: a.cfg.ShowClosedPositions}
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
		return a, a.loadDashboardData()
	case ViewInvestmentRegister:
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
	case ViewPortfolio:
		if a.portfolioData != nil && a.portfolioData.account != nil {
			return a, a.loadPortfolioData(a.portfolioData.account.ID)
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
		// Leaving the investment register drops its (transient) security
		// filter, so reopening the account later shows the full register.
		if a.currentView == ViewInvestmentRegister {
			a.resetInvestmentRegisterFilter()
		}
		// Leaving the corporate-action register drops its details overlay.
		// Without this the overlay stays counted by isDialogVisible after a
		// view switch, which routes every click on the next view into the
		// dialog cascade — killing the mouse with no modal on screen.
		if a.currentView == ViewCorporateActions {
			a.corporateActionDetail = nil
		}
		a.previousView = a.currentView
		a.currentView = v
		a.updateStatusBar()

		// Set focus appropriately for the new view
		if a.sidebar != nil {
			switch v {
			case ViewRegister:
				// Start with table focused when entering register
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(true)
				}
			case ViewScheduled:
				// Start with scheduled table focused
				a.sidebar.SetFocused(false)
				if a.scheduledTable != nil {
					a.scheduledTable.SetFocused(true)
				}
			case ViewDashboard:
				// Dashboard uses sidebar navigation
				a.sidebar.SetFocused(true)
				if a.table != nil {
					a.table.SetFocused(false)
				}
			case ViewReports:
				// Reports view doesn't use sidebar focus
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(false)
				}
			case ViewReconciliation:
				// Reconciliation is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.reconciliationTable != nil {
					a.reconciliationTable.SetFocused(true)
				}
			case ViewSecurities:
				// Securities is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.securityTable != nil {
					a.securityTable.SetFocused(true)
				}
			case ViewCorporateActions:
				// Corporate Actions is full-screen, no sidebar (the table is
				// (re)built and focused by buildCorporateActionTable).
				a.sidebar.SetFocused(false)
				if a.corporateActionViewTable != nil {
					a.corporateActionViewTable.SetFocused(true)
				}
			case ViewPrices:
				// Prices is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.priceTable != nil {
					a.priceTable.SetFocused(true)
				}
			case ViewAmortization:
				// Amortization is full-screen, no sidebar. The table is built
				// once its data loads (buildAmortizationTable focuses it then).
				a.sidebar.SetFocused(false)
			case ViewInvestmentRegister:
				// Start with investment table focused
				a.sidebar.SetFocused(false)
				if a.investmentTable != nil {
					a.investmentTable.SetFocused(true)
				}
			case ViewPortfolio:
				// Start with portfolio table focused
				a.sidebar.SetFocused(false)
				a.setPortfolioTableFocused(true)
			}
		}
	}
}
