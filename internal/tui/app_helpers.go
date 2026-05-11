package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// updateStatusBar updates the status bar context and key hints for the current view.
func (a *App) updateStatusBar() {
	context := a.currentView.String()
	if a.db != nil {
		context += " - " + filepath.Base(a.db.Path())
	}
	a.statusbar.SetContext(context)
	a.statusbar.SetKeyHints(a.getKeyHints())
}

// activeTable returns the currently active table for the current view, or nil.
func (a *App) activeTable() *Table {
	switch a.currentView {
	case ViewRegister:
		return a.table
	case ViewReconciliation:
		return a.reconciliationTable
	case ViewScheduled:
		return a.scheduledTable
	case ViewInvestmentRegister:
		return a.investmentTable
	case ViewPortfolio:
		if a.portfolioData != nil {
			return a.activePortfolioTable()
		}
	case ViewSecurities:
		return a.securityTable
	case ViewPrices:
		if a.priceView != nil && a.priceView.mode == pricesViewList {
			return a.priceListTable
		}
		return a.priceTable
	}
	return nil
}

// focusSidebar switches focus to the sidebar and unfocuses the active table.
func (a *App) focusSidebar() {
	a.sidebar.SetFocused(true)
	if tbl := a.activeTable(); tbl != nil {
		tbl.SetFocused(false)
	}
}

// focusContent switches focus to the content table and unfocuses the sidebar.
func (a *App) focusContent() {
	a.sidebar.SetFocused(false)
	if tbl := a.activeTable(); tbl != nil {
		tbl.SetFocused(true)
	}
}

// isDialogVisible returns true if any modal dialog is currently visible.
func (a *App) isDialogVisible() bool {
	return (a.confirmDialog != nil && a.confirmDialog.IsVisible()) ||
		(a.aboutDialog != nil && a.aboutDialog.IsVisible()) ||
		(a.backupDialog != nil && a.backupDialog.dialog.IsVisible()) ||
		(a.fileDialog != nil && a.fileDialog.IsVisible()) ||
		(a.splitDialog != nil && a.splitDialog.IsVisible()) ||
		(a.txnDialog != nil && a.txnDialog.IsVisible()) ||
		(a.createCatDialog != nil && a.createCatDialog.IsVisible()) ||
		(a.transferDialog != nil && a.transferDialog.IsVisible()) ||
		(a.schedDialog != nil && a.schedDialog.IsVisible()) ||
		(a.acctDialog != nil && a.acctDialog.IsVisible()) ||
		(a.reconDialog != nil && a.reconDialog.IsVisible()) ||
		(a.securityDialog != nil && a.securityDialog.IsVisible()) ||
		(a.priceDialog != nil && a.priceDialog.IsVisible()) ||
		(a.priceImportDialog != nil && a.priceImportDialog.IsVisible()) ||
		(a.buyDialog != nil && a.buyDialog.IsVisible()) ||
		(a.sellDialog != nil && a.sellDialog.IsVisible()) ||
		(a.dividendDialog != nil && a.dividendDialog.IsVisible()) ||
		(a.transferCashDialog != nil && a.transferCashDialog.IsVisible()) ||
		(a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible()) ||
		(a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible()) ||
		(a.mergerDialog != nil && a.mergerDialog.IsVisible()) ||
		(a.spinOffDialog != nil && a.spinOffDialog.IsVisible()) ||
		(a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible()) ||
		(a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible()) ||
		a.mergerConfirmData != nil ||
		a.corporateActionHistory != nil ||
		a.showHelp
}

// loadInvestmentEditTxn fetches the transaction currently being edited
// (a.investmentEditTxnID), returning nil when no edit is in progress. On a
// real lookup error (e.g. not-found, scan failure) it surfaces the error
// to the user via a.err AND resets investmentEditTxnID to NilID so the
// subsequent submit treats the dialog as new-mode rather than attempting
// to Delete a phantom row. The caller checks the returned (txn, ok); when
// ok is false the caller should abort dialog construction.
func (a *App) loadInvestmentEditTxn() (*investment.Transaction, bool) {
	if a.investmentEditTxnID == types.NilID || a.investmentRepo == nil {
		return nil, true
	}
	txn, err := a.investmentRepo.GetByID(a.investmentEditTxnID)
	if err != nil {
		a.err = fmt.Errorf("failed to load transaction for editing: %w", err)
		a.investmentEditTxnID = types.NilID
		return nil, false
	}
	return txn, true
}

// invalidatePriceHistoryCache drops every cached chart-history slice so the
// next chart render re-fetches from the price service. Called after any
// investment transaction that may have inserted a new price row
// (Buy/Sell/Reinvest Dividend all auto-create a price record), so a user
// who edits a chart and then returns sees the fresh data point.
func (a *App) invalidatePriceHistoryCache() {
	if a.priceView != nil && a.priceView.historyCache != nil {
		a.priceView.historyCache.Clear()
	}
}

// reloadCurrentView returns a tea.Cmd that reloads data for the active view
// and the sidebar. Used after undo/redo, and on Esc-back navigation so the
// destination view shows fresh state (e.g. a position created in the
// investment register shows up on Esc → portfolio).
func (a *App) reloadCurrentView() tea.Cmd {
	cmds := []tea.Cmd{a.loadSidebarData()}
	switch a.currentView {
	case ViewDashboard:
		cmds = append(cmds, a.loadDashboardData(), a.loadScheduledDueCount())
	case ViewRegister:
		accountID := a.sidebar.SelectedAccountID()
		cmds = append(cmds, a.loadRegisterData(accountID))
	case ViewInvestmentRegister:
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			cmds = append(cmds, a.loadInvestmentRegisterData(a.investmentRegister.account.ID))
		}
	case ViewPortfolio:
		if a.portfolioData != nil && a.portfolioData.account != nil {
			cmds = append(cmds, a.loadPortfolioData(a.portfolioData.account.ID))
		}
	case ViewScheduled:
		cmds = append(cmds, a.loadScheduledViewData(), a.loadScheduledDueCount())
	case ViewReports:
		if a.reports != nil {
			cmds = append(cmds, a.loadReportsViewData(
				a.reports.rtype, a.reports.year, a.reports.month,
			))
		}
	case ViewSecurities:
		cmds = append(cmds, a.loadSecurityViewData())
	case ViewPrices:
		cmds = append(cmds, a.loadPriceViewData())
	}
	return tea.Batch(cmds...)
}
