package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Update implements tea.Model.
//
// A case body that mutates one surface belongs on that surface; anything
// reaching past it — the status bar, a view reload, a service, switchView —
// is an App method in the feature's own file.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.handleWindowSize(msg.Width, msg.Height)
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKeyPress(msg)

	case tea.MouseMsg:
		return a.handleMouseEvent(msg)

	case mouseOpenAccountMsg:
		return a, a.openAccountFromMouse(msg.accountID)

	case sidebarLoadedMsg:
		a.sidebar.SetAccounts(msg.accounts, msg.balances)
		return a, nil

	case scheduledDueCountMsg:
		a.applyScheduledDueCount(msg.count)
		return a, nil

	case widget.ToastClearMsg:
		// Auto-clear after widget.ToastDuration. If a newer toast was set in
		// the meantime, it carries its own clear cmd, so dropping the
		// current toast here is safe — the next clear will fire when
		// that one's timer expires.
		a.statusbar.ClearToast()
		return a, nil

	case dashboardLoadedMsg:
		a.dashboard = msg.data
		// Investment accounts start collapsed on the dashboard; the user
		// expands the ones they care about with the ←/→ toggle or a mouse
		// click (setDashboardAccountExpanded / handleMouseDashboard). Those
		// choices live in a.dashboardExpandedAccounts and are deliberately
		// left untouched here, so an expand/collapse survives a dashboard
		// reload (e.g. after posting a transaction) within the session.
		return a, nil

	case registerLoadedMsg:
		a.register = msg.data
		a.buildRegisterTable()
		return a, nil

	case investmentRegisterLoadedMsg:
		a.investmentRegister = msg.data
		a.buildInvestmentRegisterTable()
		return a, nil

	case portfolioLoadedMsg:
		a.portfolioData = msg.data
		a.portfolioMode = portfolioViewHoldings
		a.buildPortfolioHoldingsTable()
		return a, nil

	case amortizationLoadedMsg:
		a.amortizationData = msg.data
		a.buildAmortizationTable()
		return a, nil

	case portfolioLotDetailMsg:
		a.applyPortfolioLotDetail(msg.securityID, msg.lots)
		return a, nil

	case investmentTransactionDeletedMsg:
		a.statusbar.AddNotification("Transaction deleted", widget.NotificationInfo)
		return a, a.reloadInvestmentRegisterCmd()

	case investmentTransactionClearedMsg:
		return a, a.reloadInvestmentRegisterCmd()

	case buyDialogDataMsg:
		if seed, ok := a.takeInvestmentDialogSeed(); ok {
			a.buy.applyData(msg.data, seed)
		}
		return a, nil

	case buyDialogSavedMsg:
		a.invalidatePriceHistoryCache()
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, "Buy transaction saved")

	case sellDialogDataMsg:
		if seed, ok := a.takeInvestmentDialogSeed(); ok {
			a.sell.applyData(msg.data, seed)
		}
		return a, nil

	case sellDialogSavedMsg:
		a.invalidatePriceHistoryCache()
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, "Sell transaction saved")

	case feeLiquidationDialogDataMsg:
		if seed, ok := a.takeInvestmentDialogSeed(); ok {
			a.feeLiquidation.applyData(msg.data, seed)
		}
		return a, nil

	case feeLiquidationDialogSavedMsg:
		a.invalidatePriceHistoryCache()
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, "Fee via liquidation saved")

	case dividendDialogDataMsg:
		if seed, ok := a.takeInvestmentDialogSeed(); ok {
			a.dividend.applyData(msg.data, seed)
		}
		return a, nil

	case dividendDialogSavedMsg:
		// Reinvest dividends auto-create a price row; cash dividends do
		// not. The chart history cache is cheap to rebuild, so clear
		// unconditionally rather than branching on the variant.
		a.invalidatePriceHistoryCache()
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, msg.note)

	case cashOperationDialogSavedMsg:
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, msg.note)

	case transferSharesDialogDataMsg:
		if seed, ok := a.takeInvestmentDialogSeed(); ok {
			a.transferShares.applyData(msg.data, seed, a.investmentRegisterAccountID())
		}
		return a, nil

	case transferSharesDialogSavedMsg:
		return a, a.afterInvestmentSave(msg.savedDate, msg.savedID, "Share transfer saved")

	case stockSplitDialogDataMsg:
		a.stockSplit.applyData(msg.data, a.txnDialogLastSavedDate)
		return a, nil

	case stockSplitDialogSavedMsg:
		return a, a.afterCorporateActionSaved(msg.savedDate, "Stock split executed")

	case mergerDialogDataMsg:
		a.merger.applyData(msg.data, a.txnDialogLastSavedDate)
		return a, nil

	case mergerConfirmDataMsg:
		a.mergerConfirm.data = msg.data
		return a, nil

	case mergerDialogSavedMsg:
		return a, a.afterCorporateActionSaved(msg.savedDate, "Merger executed")

	case spinOffDialogDataMsg:
		a.spinOff.applyData(msg.data, a.txnDialogLastSavedDate)
		return a, nil

	case spinOffDialogSavedMsg:
		return a, a.afterCorporateActionSaved(msg.savedDate, "Spin-off executed")

	case spinOffPriceLookupMsg:
		return a.handleSpinOffPriceLookupResult(msg)

	case corporateActionViewLoadedMsg:
		a.corporateActionView = msg.data
		a.buildCorporateActionViewTable()
		return a, nil

	case corporateActionDeletedMsg:
		a.statusbar.AddNotification("Corporate action reversed", widget.NotificationInfo)
		// Invalidate downstream view caches so re-entering them refetches.
		a.portfolioData = nil
		return a, a.loadCorporateActionViewData()

	case scheduledViewDataLoadedMsg:
		a.scheduled = msg.data
		a.buildScheduledTable()
		return a, nil

	case reportsViewDataLoadedMsg:
		a.reports = msg.data
		return a, nil

	case scheduledPostedMsg:
		return a, a.afterScheduledPosted(msg.loanPaidOff)

	// Skipping and deleting differ only in what the service did; both leave
	// the same two lists stale.
	case scheduledSkippedMsg, scheduledDeletedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
		)

	case transactionDialogDataMsg:
		a.txn.applyData(msg.data, a.sidebar.SelectedAccount(), a.txnDialogLastSavedDate)
		return a, nil

	case transactionDialogSavedMsg:
		a.rememberSavedDate(msg.savedDate)
		return a, a.afterRegisterSave(msg.savedID)

	case createCategoryRequestMsg:
		if err := a.applyCreatedCategory(msg.request); err != nil {
			a.err = err
		}
		return a, nil

	case splitDialogSavedMsg:
		return a, a.afterRegisterSave(msg.savedID)

	case transferDialogDataMsg:
		a.transfer.applyData(msg.data, a.sidebar.SelectedAccountID(), a.txnDialogLastSavedDate)
		return a, nil

	case transferDialogSavedMsg:
		return a, a.afterTransferSave(msg)

	case scheduledDialogDataMsg:
		if a.sched.applyData(msg.data, a.categoriesOrNil()) {
			a.maybeAddEditAsLoanButton(msg.data.scheduled)
		}
		return a, nil

	case scheduledDialogSavedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
			a.loadSidebarData(),
		)

	case schedulePreviewDataMsg:
		a.schedPreviewDialog = NewSchedulePreviewDialog(
			msg.template, msg.accounts, msg.payees,
			msg.categoryOptions, msg.categoryIDs, msg.loanSplits,
		)
		return a, nil

	case schedulePreviewLoanBlockedMsg:
		return a, a.handleSchedulePreviewLoanBlocked(msg.paidOff, msg.err)

	case paycheckWizardDataMsg:
		a.paycheckWizard = NewPaycheckWizard(msg.categoryOptions, msg.categoryIDs, msg.accounts)
		return a, nil

	case loanWizardDataMsg:
		a.loan.applyData(msg)
		return a, nil

	case loanWizardSavedMsg:
		return a, a.afterLoanWizardSave()

	case autoPostCompletedMsg:
		return a, a.applyAutoPostResult(msg.summary)

	case accountDialogDataMsg:
		a.acct.applyData(msg.data)
		return a, nil

	case accountDialogSavedMsg:
		return a, a.afterAccountDialogSave()

	case fileDialogSavedMsg:
		return a.switchDatabase(msg.db)

	case backupCreatedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Backup created: %s", backupFilename(msg.path)),
			widget.NotificationInfo,
		)
		return a, nil

	case restoreConfirmedMsg:
		return a.afterRestore(msg.safetyBackupPath)

	case reconciliationStartedMsg:
		// Session started, switch to reconciliation view and load data
		a.switchView(ViewReconciliation)
		return a, a.loadReconciliationData(msg.session, msg.account)

	case reconciliationLoadedMsg:
		a.reconciliation = msg.data
		a.buildReconciliationTable()
		return a, nil

	case reconciliationClearedTotalMsg:
		if a.reconciliation != nil {
			a.reconciliation.clearedTotal = msg.clearedTotal
		}
		return a, nil

	case reconciliationFinishedMsg:
		return a, a.afterReconciliationFinished()

	case reconciliationCancelledMsg:
		a.afterReconciliationCancelled()
		return a, nil

	case accountDeletedMsg:
		a.switchView(ViewDashboard)
		return a, tea.Batch(a.loadSidebarData(), a.loadDashboardData())

	case accountClosedMsg:
		return a, a.afterAccountClosed()

	case undoResultMsg:
		return a, a.applyUndoResult(msg)

	case securityViewDataLoadedMsg:
		a.securityView = msg.data
		a.buildSecurityTable()
		return a, nil

	case securityAddedMsg:
		// Select the new security after the reload so it scrolls into view,
		// even if it sorts off-screen in a long list.
		a.pendingSecuritySelectID = msg.id
		return a, a.afterSecurityChange("Security added")

	case securityUpdatedMsg:
		return a, a.afterSecurityChange("Security updated")

	case securityDeletedMsg:
		return a, a.afterSecurityChange("Security deleted")

	case securityHiddenMsg:
		note := "Security unhidden"
		if msg.hidden {
			note = "Security hidden"
		}
		return a, a.afterSecurityChange(note)

	case priceViewDataLoadedMsg:
		return a, a.applyPriceViewData(msg.data)

	case priceChartDebounceTickMsg:
		return a, a.handlePriceChartDebounceTick(msg)

	case priceChartHistoryLoadedMsg:
		a.applyPriceChartHistory(msg)
		return a, nil

	case priceAddedMsg:
		return a, a.afterPriceChange("Price added")

	case priceUpdatedMsg:
		return a, a.afterPriceChange("Price updated")

	case priceDeletedMsg:
		return a, a.afterPriceChange("Price deleted")

	case priceLookupResultMsg:
		return a.handlePriceLookupResult(msg)

	case priceImportedMsg:
		return a, a.afterPriceChange(
			fmt.Sprintf("Imported %d prices (%d skipped)", msg.imported, msg.skipped),
		)

	case importDialogOpenMsg:
		a.importer.showOptions(msg.accounts, msg.defaultAccountID)
		return a, nil

	case importPreviewedMsg:
		a.importer.showConfirm(msg.state, msg.result)
		return a, nil

	case importNeedsSourceMsg:
		a.importer.showSourcePicker(msg.state, msg.sources)
		return a, nil

	case importCompletedMsg:
		return a, a.applyImportResult(msg)

	case linkTransfersPreviewedMsg:
		a.linkTransfers.showPreview(msg.result)
		return a, nil

	case linkTransfersCompletedMsg:
		return a, a.applyLinkTransfersResult(msg)

	case priceRefreshCompleteMsg:
		return a, a.applyPriceRefreshResult(msg)

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}
