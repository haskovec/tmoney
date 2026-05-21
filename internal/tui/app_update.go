package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.styles.Resize(msg.Width, msg.Height)
		a.ready = true
		return a, nil

	case tea.KeyPressMsg:
		return a.handleKeyPress(msg)

	case tea.MouseMsg:
		return a.handleMouseEvent(msg)

	case mouseOpenAccountMsg:
		acct := a.sidebar.SelectedAccount()
		if acct != nil && acct.Type == account.TypeInvestment {
			a.portfolioData = nil
			a.switchView(ViewPortfolio)
			return a, a.loadPortfolioData(msg.accountID)
		}
		a.register = nil
		a.switchView(ViewRegister)
		return a, a.loadRegisterData(msg.accountID)

	case sidebarLoadedMsg:
		a.sidebar.SetAccounts(msg.accounts, msg.balances)
		return a, nil

	case scheduledDueCountMsg:
		a.statusbar.ClearNotifications()
		if msg.count > 0 {
			text := fmt.Sprintf("%d scheduled due", msg.count)
			if msg.count == 1 {
				text = "1 scheduled due"
			}
			a.statusbar.AddNotification(text, NotificationAlert)
		}
		return a, nil

	case ToastClearMsg:
		// Auto-clear after ToastDuration. If a newer toast was set in
		// the meantime, it carries its own clear cmd, so dropping the
		// current toast here is safe — the next clear will fire when
		// that one's timer expires.
		a.statusbar.ClearToast()
		return a, nil

	case dashboardLoadedMsg:
		a.dashboard = msg.data
		// Auto-expand investment accounts that have holdings
		if a.dashboardExpandedAccounts == nil {
			a.dashboardExpandedAccounts = make(map[types.ID]bool)
		}
		if msg.data != nil && msg.data.investmentHoldings != nil {
			for accountID, val := range msg.data.investmentHoldings {
				if len(val.Holdings) > 0 {
					// Only set if not already explicitly toggled by user
					if _, exists := a.dashboardExpandedAccounts[accountID]; !exists {
						a.dashboardExpandedAccounts[accountID] = true
					}
				}
			}
		}
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

	case portfolioLotDetailMsg:
		if a.portfolioData != nil {
			a.portfolioData.lotDetails = msg.lots
			a.portfolioData.lotSecurityID = msg.securityID
			a.buildPortfolioLotsTable()
			if a.portfolioLotsTable != nil {
				a.portfolioLotsTable.SetFocused(true)
			}
			if a.portfolioHoldingsTable != nil {
				a.portfolioHoldingsTable.SetFocused(false)
			}
		}
		return a, nil

	case investmentTransactionDeletedMsg:
		a.statusbar.AddNotification("Transaction deleted", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case investmentTransactionClearedMsg:
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case buyDialogDataMsg:
		a.buyDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.buyDialogSecurityIDs = secIDs
		editTxn, ok := a.loadInvestmentEditTxn()
		if !ok {
			return a, nil
		}
		a.buyDialog = buildBuyDialog(secOptions, editTxn, secIDs)
		if editTxn == nil {
			a.buyDialog.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil

	case buyDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		a.invalidatePriceHistoryCache()
		a.statusbar.AddNotification("Buy transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case sellDialogDataMsg:
		a.sellDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.sellDialogSecurityIDs = secIDs
		a.sellDialogLots = msg.data.lots
		editTxn, ok := a.loadInvestmentEditTxn()
		if !ok {
			return a, nil
		}
		a.sellDialog = buildSellDialog(secOptions, editTxn, secIDs, msg.data.lots)
		if editTxn == nil {
			a.sellDialog.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil

	case sellDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		a.invalidatePriceHistoryCache()
		a.statusbar.AddNotification("Sell transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case dividendDialogDataMsg:
		a.dividendDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.dividendDialogSecurityIDs = secIDs
		editTxn, ok := a.loadInvestmentEditTxn()
		if !ok {
			return a, nil
		}
		if a.dividendDialogReinvest {
			a.dividendDialog = buildReinvestDividendDialog(secOptions, editTxn, secIDs)
		} else {
			a.dividendDialog = buildDividendDialog(secOptions, editTxn, secIDs)
		}
		if editTxn == nil {
			a.dividendDialog.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil

	case dividendDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		// Reinvest dividends auto-create a price row; cash dividends do
		// not. The chart history cache is cheap to rebuild, so clear
		// unconditionally rather than branching on dividendDialogReinvest.
		a.invalidatePriceHistoryCache()
		label := "Dividend"
		if a.dividendDialogReinvest {
			label = "Reinvest dividend"
		}
		a.statusbar.AddNotification(label+" transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case cashOperationDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		label := string(a.cashOperationType)
		if label == "" {
			label = "Cash operation"
		} else {
			label = a.cashOperationType.DisplayName()
		}
		a.statusbar.AddNotification(label+" transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case transferCashDialogDataMsg:
		a.transferCashDialogData = msg.data
		acctOptions, acctIDs := buildNonInvestmentAccountOptions(msg.data.accounts)
		a.transferCashDialogAccountIDs = acctIDs
		editTxn, ok := a.loadInvestmentEditTxn()
		if !ok {
			return a, nil
		}
		a.transferCashDialog = buildTransferCashDialog(a.transferCashDirection, acctOptions, editTxn, acctIDs)
		if editTxn == nil {
			a.transferCashDialog.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil

	case transferCashDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		a.statusbar.AddNotification("Cash transfer saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case transferSharesDialogDataMsg:
		a.transferSharesDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.transferSharesDialogSecurityIDs = secIDs
		excludeID := types.NilID
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			excludeID = a.investmentRegister.account.ID
		}
		acctOptions, acctIDs := buildInvestmentAccountOptions(msg.data.investmentAccounts, excludeID)
		a.transferSharesDialogAccountIDs = acctIDs
		a.transferSharesDialogLots = msg.data.lots
		editTxn, ok := a.loadInvestmentEditTxn()
		if !ok {
			return a, nil
		}
		a.transferSharesDialog = buildTransferSharesDialog(acctOptions, secOptions, editTxn, acctIDs, secIDs, msg.data.lots)
		if editTxn == nil {
			a.transferSharesDialog.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil

	case transferSharesDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		a.investmentEditTxnID = types.NilID
		a.statusbar.AddNotification("Share transfer saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case stockSplitDialogDataMsg:
		a.stockSplitDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.stockSplitDialogSecurityIDs = secIDs
		a.stockSplitDialog = buildStockSplitDialog(secOptions, secIDs, msg.data.sharesMap, a.stockSplitDialogPreSelectedID)
		a.stockSplitDialogPreSelectedID = nil
		return a, nil

	case stockSplitDialogSavedMsg:
		a.statusbar.AddNotification("Stock split executed", NotificationInfo)
		return a, a.refreshAfterCorporateAction()

	case mergerDialogDataMsg:
		a.mergerDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.mergerDialogSecurityIDs = secIDs
		a.mergerDialog = buildMergerDialog(secOptions, secIDs, a.mergerDialogPreSelectedID)
		a.mergerDialogPreSelectedID = nil
		return a, nil

	case mergerConfirmDataMsg:
		a.mergerConfirmData = msg.data
		return a, nil

	case mergerDialogSavedMsg:
		a.statusbar.AddNotification("Merger executed", NotificationInfo)
		return a, a.refreshAfterCorporateAction()

	case spinOffDialogDataMsg:
		a.spinOffDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.spinOffDialogSecurityIDs = secIDs
		a.spinOffDialog = buildSpinOffDialog(secOptions, secIDs, a.spinOffDialogPreSelectedID)
		a.spinOffDialogPreSelectedID = nil
		return a, nil

	case spinOffDialogSavedMsg:
		a.statusbar.AddNotification("Spin-off executed", NotificationInfo)
		return a, a.refreshAfterCorporateAction()

	case corporateActionViewLoadedMsg:
		a.corporateActionView = msg.data
		a.buildCorporateActionViewTable()
		return a, nil

	case corporateActionDeletedMsg:
		a.statusbar.AddNotification("Corporate action reversed", NotificationInfo)
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
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadSidebarData(),
			a.loadScheduledDueCount(),
		)

	case scheduledSkippedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
		)

	case scheduledDeletedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
		)

	case transactionDialogDataMsg:
		a.txnDialogData = msg.data
		categoryOptions, categoryIDs := buildCategoryOptions(msg.data.categories)
		a.txnDialogCategoryIDs = categoryIDs
		a.txnDialog = buildTransactionDialog(msg.data, categoryOptions, categoryIDs, a.txnDialogLastSavedDate)
		return a, nil

	case transactionDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case createCategoryRequestMsg:
		if err := a.applyCreatedCategory(msg.request); err != nil {
			a.err = err
		}
		return a, nil

	case splitDialogSavedMsg:
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case transferDialogDataMsg:
		a.transferDialogData = msg.data
		accountOptions, accountIDs := buildAccountOptions(msg.data.accounts)
		a.transferDialogAccountIDs = accountIDs

		if msg.data.mode == transferDialogModeEdit && msg.data.existing != nil {
			fromName, toName := transferAccountNames(msg.data)
			a.transferDialog = buildEditTransferDialog(fromName, toName, msg.data.existing)
			return a, nil
		}

		// Pre-select the currently selected sidebar account as "From"
		defaultFromIndex := 0
		selectedID := a.sidebar.SelectedAccountID()
		for i, id := range accountIDs {
			if id == selectedID {
				defaultFromIndex = i
				break
			}
		}
		a.transferDialog = buildTransferDialog(accountOptions, defaultFromIndex)
		a.transferDialog.SeedDateField(a.txnDialogLastSavedDate)
		return a, nil

	case transferDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case scheduledDialogDataMsg:
		a.schedDialogData = msg.data
		accountOptions, accountIDs := buildAccountOptions(msg.data.accounts)
		a.schedDialogAccountIDs = accountIDs

		var categories []*category.Category
		if a.categorySvc != nil {
			cats, err := a.categorySvc.List()
			if err == nil {
				categories = cats
			}
		}
		categoryOptions, categoryIDs := buildCategoryOptions(categories)
		a.schedDialogCategoryIDs = categoryIDs
		a.schedDialogCategoryOptions = categoryOptions

		if msg.data.mode == scheduledDialogModeEdit && msg.data.scheduled != nil {
			// Build payee name map for edit dialog
			payeeNames := make(map[types.ID]string)
			for _, p := range msg.data.payees {
				payeeNames[p.ID] = p.Name
			}
			a.schedDialog = buildEditScheduledDialog(msg.data.scheduled, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
		} else {
			a.schedDialog = buildNewScheduledDialog(accountOptions, categoryOptions)
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
			msg.template,
			msg.accounts,
			msg.payees,
			msg.categoryOptions,
			msg.categoryIDs,
		)
		return a, nil

	case paycheckWizardDataMsg:
		a.paycheckWizard = NewPaycheckWizard(
			msg.categoryOptions,
			msg.categoryIDs,
			msg.accounts,
		)
		return a, nil

	case autoPostCompletedMsg:
		if msg.summary != nil && msg.summary.PostedCount > 0 {
			text := fmt.Sprintf("Auto-posted %d scheduled transaction(s)", msg.summary.PostedCount)
			a.statusbar.AddNotification(text, NotificationInfo)
			// Register auto-post as a single undo step
			if a.undoManager != nil && a.transactionSvc != nil && a.scheduledTxnSvc != nil {
				cmd := undo.NewAutoPostCommand(a.transactionSvc, a.scheduledTxnSvc, msg.summary)
				a.undoManager.Push(cmd)
			}
			// Reload data since auto-posting created transactions
			return a, tea.Batch(
				a.loadSidebarData(),
				a.loadScheduledDueCount(),
				a.loadDashboardData(),
			)
		}
		return a, nil

	case accountDialogDataMsg:
		a.acctDialogData = msg.data
		if msg.data.mode == accountDialogModeEdit && msg.data.account != nil {
			a.acctDialog = buildEditAccountDialog(msg.data.account)
		} else {
			a.acctDialog = buildNewAccountDialog()
		}
		return a, nil

	case accountDialogSavedMsg:
		cmds := []tea.Cmd{a.loadSidebarData(), a.loadDashboardData()}
		if a.currentView == ViewRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadRegisterData(accountID))
		}
		return a, tea.Batch(cmds...)

	case fileDialogSavedMsg:
		return a.switchDatabase(msg.db)

	case backupCreatedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Backup created: %s", backupFilename(msg.path)),
			NotificationInfo,
		)
		return a, nil

	case restoreConfirmedMsg:
		// Switch to the reopened database
		model, cmd := a.switchDatabase(msg.db)
		a.statusbar.AddNotification(
			fmt.Sprintf("Restored from backup (safety backup: %s)", backupFilename(msg.safetyBackupPath)),
			NotificationInfo,
		)
		return model, cmd

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
		acctName := ""
		if a.reconciliation != nil {
			acctName = a.reconciliation.account.Name
		}
		a.reconciliation = nil
		a.reconciliationTable = nil
		a.switchView(ViewRegister)
		a.statusbar.AddNotification(
			fmt.Sprintf("Reconciliation completed for %s", acctName),
			NotificationInfo,
		)
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case reconciliationCancelledMsg:
		a.reconciliation = nil
		a.reconciliationTable = nil
		a.switchView(ViewRegister)
		a.statusbar.AddNotification("Reconciliation cancelled", NotificationInfo)
		return a, nil

	case accountDeletedMsg:
		a.switchView(ViewDashboard)
		return a, tea.Batch(a.loadSidebarData(), a.loadDashboardData())

	case accountClosedMsg:
		cmds := []tea.Cmd{a.loadSidebarData(), a.loadDashboardData()}
		if a.currentView == ViewRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadRegisterData(accountID))
		}
		if a.currentView == ViewInvestmentRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadInvestmentRegisterData(accountID))
		}
		if a.currentView == ViewPortfolio && a.portfolioData != nil && a.portfolioData.account != nil {
			cmds = append(cmds, a.loadPortfolioData(a.portfolioData.account.ID))
		}
		return a, tea.Batch(cmds...)

	case undoResultMsg:
		if errors.Is(msg.err, undo.ErrNothingToUndo) {
			a.statusbar.AddNotification("Nothing to undo", NotificationInfo)
			return a, nil
		}
		if errors.Is(msg.err, undo.ErrNothingToRedo) {
			a.statusbar.AddNotification("Nothing to redo", NotificationInfo)
			return a, nil
		}
		if msg.err != nil {
			a.err = msg.err
			return a, nil
		}
		a.statusbar.AddNotification(
			fmt.Sprintf("%s: %s", msg.action, msg.description),
			NotificationInfo,
		)
		// Reload current view data after undo/redo
		return a, a.reloadCurrentView()

	case securityViewDataLoadedMsg:
		a.securityView = msg.data
		a.buildSecurityTable()
		return a, nil

	case securityAddedMsg:
		a.statusbar.AddNotification("Security added", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityUpdatedMsg:
		a.statusbar.AddNotification("Security updated", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityDeletedMsg:
		a.statusbar.AddNotification("Security deleted", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityHiddenMsg:
		if msg.hidden {
			a.statusbar.AddNotification("Security hidden", NotificationInfo)
		} else {
			a.statusbar.AddNotification("Security unhidden", NotificationInfo)
		}
		return a, a.loadSecurityViewData()

	case priceViewDataLoadedMsg:
		// Preserve the existing historyCache across reload so that the
		// per-security evictions performed by the price-CRUD handlers
		// (PC-015) and the full clear performed by bulk refresh (PC-016)
		// are not silently undone by the fresh empty cache that
		// loadPriceViewData/loadPriceViewDataForSecurity construct.
		if a.priceView != nil && a.priceView.historyCache != nil {
			msg.data.historyCache = a.priceView.historyCache
		}
		a.priceView = msg.data
		var cmd tea.Cmd
		switch msg.data.mode {
		case pricesViewList:
			a.buildPriceListTable()
			// Kick off the initial debounced fetch for the row under the
			// cursor so the chart panel populates without requiring a
			// keystroke. Subsequent cursor movement reschedules.
			if secID := a.listCursorSecurityID(); !secID.IsNil() {
				cmd = a.schedulePriceChartFetch(secID)
			}
		case pricesViewDetail:
			a.buildPriceTable()
		}
		return a, cmd

	case priceChartDebounceTickMsg:
		if a.priceView == nil {
			return a, nil
		}
		// Stale: a later schedule has superseded this tick.
		if msg.gen != a.priceView.chartDebounceGen {
			return a, nil
		}
		// Cursor moved off the row this tick was scheduled for. Drop
		// silently — the move that triggered the change scheduled its
		// own fresh tick.
		if a.listCursorSecurityID() != msg.secID {
			return a, nil
		}
		// Already cached (e.g. user scrolled away and back, or a CRUD
		// invalidation re-fired before the tick). No fetch needed; just
		// promote to displayed.
		if a.priceView.historyCache != nil {
			if _, ok := a.priceView.historyCache.Lookup(msg.secID); ok {
				a.priceView.chartDisplayedID = msg.secID
				return a, nil
			}
		}
		return a, a.fetchPriceChartHistory(msg.secID)

	case priceChartHistoryLoadedMsg:
		if a.priceView == nil {
			return a, nil
		}
		if a.priceView.historyCache != nil {
			a.priceView.historyCache.Put(msg.secID, msg.prices)
		}
		a.priceView.chartDisplayedID = msg.secID
		return a, nil

	case priceAddedMsg:
		a.statusbar.AddNotification("Price added", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceUpdatedMsg:
		a.statusbar.AddNotification("Price updated", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceDeletedMsg:
		a.statusbar.AddNotification("Price deleted", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceImportedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Imported %d prices (%d skipped)", msg.imported, msg.skipped),
			NotificationInfo,
		)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case importDialogOpenMsg:
		d, ids := buildImportOptionsDialog(msg.accounts, msg.defaultAccountID)
		a.importDialog = d
		a.importDialogState = &importDialogState{
			step:       importStepOptions,
			accountIDs: ids,
		}
		return a, nil

	case importPreviewedMsg:
		state := msg.state
		state.preview = msg.result
		state.step = importStepConfirm
		a.importDialog = buildImportConfirmDialog(state)
		a.importDialogState = state
		return a, nil

	case importNeedsSourceMsg:
		state := msg.state
		state.step = importStepSourcePicker
		state.sourceOptions = msg.sources
		a.importDialog = buildImportSourcePickerDialog(msg.sources, state.accountName)
		a.importDialogState = state
		return a, nil

	case importCompletedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Imported: %d created, %d updated, %d skipped", msg.created, msg.updated, msg.skipped),
			NotificationInfo,
		)
		// Reload data so the new transactions appear in the dashboard /
		// register without the user having to navigate away and back.
		var cmds []tea.Cmd
		cmds = append(cmds, a.loadSidebarData(), a.loadDashboardData())
		if a.currentView == ViewRegister && a.register != nil {
			cmds = append(cmds, a.loadRegisterData(a.register.account.ID))
		}
		if len(msg.errors) > 0 {
			a.err = fmt.Errorf("import completed with %d errors:\n%s",
				len(msg.errors), strings.Join(msg.errors, "\n"))
		}
		return a, tea.Batch(cmds...)

	case linkTransfersPreviewedMsg:
		a.linkTransfersResult = msg.result
		a.linkTransfersDialog = buildLinkTransfersDialog(msg.result)
		return a, nil

	case linkTransfersCompletedMsg:
		summary := fmt.Sprintf("Linked %d transfer pairs", msg.linked)
		if msg.ambiguous > 0 {
			summary += fmt.Sprintf(" (%d ambiguous left for review)", msg.ambiguous)
		}
		a.statusbar.AddNotification(summary, NotificationInfo)
		var cmds []tea.Cmd
		cmds = append(cmds, a.loadSidebarData(), a.loadDashboardData())
		if a.currentView == ViewRegister && a.register != nil {
			cmds = append(cmds, a.loadRegisterData(a.register.account.ID))
		}
		if len(msg.errors) > 0 {
			parts := make([]string, len(msg.errors))
			for i, e := range msg.errors {
				parts[i] = e.Error()
			}
			a.err = fmt.Errorf("link transfers had %d errors:\n%s",
				len(msg.errors), strings.Join(parts, "\n"))
		}
		return a, tea.Batch(cmds...)

	case priceRefreshCompleteMsg:
		// Always retire the in-progress notification and clear the
		// guard, regardless of success/failure, so the `u` shortcut
		// becomes responsive again.
		a.statusbar.RemoveNotification(a.refreshNotifID)
		a.refreshNotifID = 0
		a.refreshingPrices = false
		if msg.err != nil {
			a.err = msg.err
			return a, nil
		}
		a.statusbar.AddNotification(summarizeRefreshResult(msg.result), NotificationInfo)
		// PC-016: bulk refresh can silently change any subset of
		// tickers' prices, and the result doesn't enumerate which.
		// Drop every chart-history cache entry so the next render
		// re-fetches from the price service.
		if a.priceView != nil && a.priceView.historyCache != nil {
			a.priceView.historyCache.Clear()
		}
		// Re-load any data views that may now be stale.
		var cmds []tea.Cmd
		if a.currentView == ViewSecurities {
			cmds = append(cmds, a.loadSecurityViewData())
		}
		if a.currentView == ViewPrices {
			cmds = append(cmds, a.loadPriceViewData())
		}
		return a, tea.Batch(cmds...)

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}
