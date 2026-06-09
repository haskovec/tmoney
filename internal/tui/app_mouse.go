package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// handleMouseEvent handles mouse events (clicks, wheel scrolling).
func (a *App) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseWheelMsg:
		return a.handleMouseWheel(msg)
	case tea.MouseClickMsg:
		if msg.Mouse().Button != tea.MouseLeft {
			return a, nil
		}
	default:
		return a, nil
	}

	m := msg.Mouse()

	// Route mouse clicks to dialog when a modal dialog is visible
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	// widget.Menu bar (row 0)
	if m.Y == 0 {
		return a.handleMouseMenuBar(msg)
	}

	// If menu dropdown is open, check if click is on dropdown
	if a.menubar.IsActive() {
		colOffset, dropdownWidth, itemCount := a.menubar.DropdownBounds()
		if m.Y >= 1 && m.Y <= itemCount &&
			m.X >= colOffset && m.X < colOffset+dropdownWidth {
			itemIdx := a.menubar.HitTestDropdown(m.Y - 1)
			if itemIdx >= 0 {
				a.menubar.SetItemCursor(itemIdx)
				action, data := a.menubar.Select()
				return a.handleMenuAction(action, data)
			}
		}
		// Click outside dropdown closes it
		a.menubar.Deactivate()
		// Fall through to handle the click in the content area
	}

	// Status bar (last row) - ignore
	if m.Y >= a.height-1 {
		return a, nil
	}

	// Content area
	if m.Y >= 1 && m.Y < a.height-1 {
		return a.handleMouseContent(msg)
	}

	return a, nil
}

// handleMouseMenuBar handles mouse clicks on the menu bar (row 0).
func (a *App) handleMouseMenuBar(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	idx := a.menubar.HitTestBar(msg.Mouse().X)
	if idx >= 0 {
		if a.menubar.IsActive() && a.menubar.Cursor() == idx {
			a.menubar.Deactivate()
		} else {
			a.menubar.ActivateMenu(idx)
		}
	} else if a.menubar.IsActive() {
		a.menubar.Deactivate()
	}
	return a, nil
}

// handleMouseContent handles mouse clicks in the content area.
func (a *App) handleMouseContent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := msg.Mouse()
	contentY := m.Y - 1 // Offset for header row

	sidebarWidth := a.styles.SidebarWidth()

	// Full-screen views (no sidebar)
	if sidebarWidth == 0 || a.currentView == ViewReconciliation ||
		a.currentView == ViewSecurities || a.currentView == ViewPrices {
		return a.handleMouseTable(msg, contentY)
	}

	// Sidebar zone
	if m.X < sidebarWidth {
		return a.handleMouseSidebar(msg, contentY)
	}

	// Border column - ignore
	if m.X == sidebarWidth {
		return a, nil
	}

	// Content zone (right of sidebar)
	a.focusContent()
	return a.handleMouseTable(msg, contentY)
}

// handleMouseTable handles mouse clicks in the table/content area.
// Single click moves the cursor; on tables that support drill-in
// (currently the prices list), a second click on the same row within
// the double-click threshold opens the row.
func (a *App) handleMouseTable(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	tableY := contentY - a.tableContentRowOffset()

	tbl := a.activeTable()
	if tbl == nil {
		return a, nil
	}

	rowIdx := tbl.HitTest(tableY)
	if rowIdx < 0 {
		return a, nil
	}
	tbl.SetCursor(rowIdx)

	// Reconciliation: clicking a row toggles its cleared checkbox — the
	// primary action on this view (Space does the same on the keyboard).
	if a.currentView == ViewReconciliation {
		return a.toggleReconciliationCheck()
	}

	// Prices landing list: a single click selects the row and refreshes the
	// chart panel for the newly highlighted ticker (mirroring keyboard
	// navigation); a second click on the same row within the threshold
	// drills into that ticker's price history.
	if a.currentView == ViewPrices && a.priceView != nil && a.priceView.mode == pricesViewList {
		if a.priceListClicks == nil {
			a.priceListClicks = widget.NewClickTracker(widget.DoubleClickThreshold)
		}
		if a.priceListClicks.Click(rowIdx) {
			return a, a.drillIntoSelectedListRow()
		}
		return a, a.schedulePriceListChartFetchIfActive()
	}

	return a, nil
}

// handleMouseWheel handles mouse wheel scrolling.
func (a *App) handleMouseWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	wheelUp := msg.Mouse().Button == tea.MouseWheelUp

	if a.sidebar.IsFocused() {
		if wheelUp {
			a.sidebar.MoveUp()
		} else {
			a.sidebar.MoveDown()
		}
		return a, nil
	}

	tbl := a.activeTable()
	if tbl != nil {
		if wheelUp {
			tbl.MoveUp()
		} else {
			tbl.MoveDown()
		}
	}

	// On the prices landing list, refresh the chart panel for the row the
	// wheel scrolled to — same root cause as the single-click path above.
	// Returns nil (no-op) on every other view.
	return a, a.schedulePriceListChartFetchIfActive()
}

// handleDialogMouse routes mouse events to the currently visible dialog.
// For non-dialog.Dialog overlays (SplitDialog, mergerConfirm, corporateActionHistory),
// mouse events are blocked (returns no-op). The help overlay accepts a
// click on its [x] close button.
func (a *App) handleDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.showHelp {
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
			m := msg.Mouse()
			if helpOverlayCloseHit(a.styles, a.currentView, a.width, a.height, m.X, m.Y) {
				a.showHelp = false
			}
		}
		return a, nil
	}
	if a.mergerConfirmData != nil || a.corporateActionDetail != nil {
		return a, nil
	}
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		return a, nil
	}

	// dialog.Dialog cascade (same order as handleKeyPress)
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		action := a.confirmDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			a.confirmDialog.SetVisible(false)
			fn := a.confirmAction
			a.confirmDialog = nil
			a.confirmAction = nil
			return a, func() tea.Msg { return fn() }
		case dialog.DialogActionCancel:
			a.confirmDialog.SetVisible(false)
			a.confirmDialog = nil
			a.confirmAction = nil
		}
		return a, nil
	}

	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		action := a.aboutDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit, dialog.DialogActionCancel:
			a.aboutDialog.SetVisible(false)
			a.aboutDialog = nil
		}
		return a, nil
	}

	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		action := a.backupDialog.dialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitBackupDialog()
		case dialog.DialogActionCancel:
			a.backupDialog.dialog.SetVisible(false)
			a.backupDialog = nil
		}
		return a, nil
	}

	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		// In browse mode, a double-click on a list row activates that entry
		// (navigate into a directory or open a .tdb file) without requiring
		// a separate Open button press.
		listItemRow := -1
		if click, ok := msg.(tea.MouseClickMsg); ok &&
			a.fileDialogMode == fileDialogModeBrowse &&
			click.Button == tea.MouseLeft {
			listItemRow = a.browseDialogListHit(msg)
		}

		action := a.fileDialog.HandleMouse(msg, a.width, a.height)

		if listItemRow >= 0 {
			if a.browseDialogClicks == nil {
				a.browseDialogClicks = widget.NewClickTracker(widget.DoubleClickThreshold)
			}
			if a.browseDialogClicks.Click(listItemRow) {
				return a.submitFileDialog()
			}
		}

		switch action {
		case dialog.DialogActionSubmit:
			return a.submitFileDialog()
		case dialog.DialogActionCancel:
			a.closeFileDialog()
		}
		return a, nil
	}

	if a.importDialog != nil && a.importDialog.IsVisible() {
		action := a.importDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitImportDialog()
		case dialog.DialogActionCancel:
			a.closeImportDialog()
		}
		return a, nil
	}

	if a.linkTransfersDialog != nil && a.linkTransfersDialog.IsVisible() {
		action := a.linkTransfersDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitLinkTransfersDialog()
		case dialog.DialogActionCancel:
			a.closeLinkTransfersDialog()
		}
		return a, nil
	}

	if a.createCatDialog != nil && a.createCatDialog.IsVisible() {
		action := a.createCatDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitCreateCatDialog()
		case dialog.DialogActionCancel:
			a.cancelCreateCatDialog()
		}
		return a, nil
	}

	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		action := a.txnDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitTransactionDialog()
		case dialog.DialogActionCancel:
			a.txnDialog.SetVisible(false)
			a.txnDialog = nil
		case dialog.DialogActionAddNew:
			return a.openCreateCategorySubDialog()
		}
		return a, nil
	}

	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		action := a.transferDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitTransferDialog()
		case dialog.DialogActionCancel:
			a.transferDialog.SetVisible(false)
			a.transferDialog = nil
		}
		return a, nil
	}

	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		action := a.schedDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			if a.schedDialogData != nil && a.schedDialogData.isTransfer {
				return a.submitScheduledTransferDialog()
			}
			return a.submitScheduledDialog()
		case dialog.DialogActionCancel:
			a.schedDialog.SetVisible(false)
			a.schedDialog = nil
		case dialog.DialogActionAlternate:
			return a.relaunchAsPaycheckWizard()
		}
		return a, nil
	}

	if a.schedPreviewDialog != nil && a.schedPreviewDialog.IsVisible() {
		return a.handleSchedulePreviewMouse(msg)
	}

	if a.paycheckWizard != nil && a.paycheckWizard.IsVisible() {
		action := a.paycheckWizard.HandleMouse(msg, a.styles, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitPaycheckWizard()
		case dialog.DialogActionCancel:
			a.closePaycheckWizard()
		}
		return a, nil
	}

	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		action := a.acctDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitAccountDialog()
		case dialog.DialogActionCancel:
			a.acctDialog.SetVisible(false)
			a.acctDialog = nil
		}
		return a, nil
	}

	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		action := a.reconDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitStartReconciliation()
		case dialog.DialogActionCancel:
			a.reconDialog.SetVisible(false)
			a.reconDialog = nil
		}
		return a, nil
	}

	if a.closeAcctDialog != nil && a.closeAcctDialog.IsVisible() {
		action := a.closeAcctDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitCloseAccountDialog()
		case dialog.DialogActionCancel:
			a.closeAcctDialog.SetVisible(false)
			a.closeAcctDialog = nil
		}
		return a, nil
	}

	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		action := a.securityDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitSecurityDialog()
		case dialog.DialogActionCancel:
			a.securityDialog.SetVisible(false)
			a.securityDialog = nil
		}
		return a, nil
	}

	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		action := a.priceDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitPriceDialog()
		case dialog.DialogActionAlternate:
			return a.startPriceLookup()
		case dialog.DialogActionCancel:
			a.priceDialog.SetVisible(false)
			a.priceDialog = nil
		}
		return a, nil
	}

	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		action := a.priceImportDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitImportPriceDialog()
		case dialog.DialogActionCancel:
			a.priceImportDialog.SetVisible(false)
			a.priceImportDialog = nil
		}
		return a, nil
	}

	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		action := a.buyDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitBuyDialog()
		case dialog.DialogActionCancel:
			a.buyDialog.SetVisible(false)
			a.buyDialog = nil
		}
		return a, nil
	}

	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		action := a.sellDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitSellDialog()
		case dialog.DialogActionCancel:
			a.sellDialog.SetVisible(false)
			a.sellDialog = nil
		}
		return a, nil
	}

	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		action := a.dividendDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitDividendDialog()
		case dialog.DialogActionCancel:
			a.dividendDialog.SetVisible(false)
			a.dividendDialog = nil
		}
		return a, nil
	}

	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		action := a.transferSharesDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitTransferSharesDialog()
		case dialog.DialogActionCancel:
			a.transferSharesDialog.SetVisible(false)
			a.transferSharesDialog = nil
		}
		return a, nil
	}

	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		action := a.stockSplitDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitStockSplitDialog()
		case dialog.DialogActionCancel:
			a.stockSplitDialog.SetVisible(false)
			a.stockSplitDialog = nil
		}
		return a, nil
	}

	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		action := a.mergerDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitMergerDialog()
		case dialog.DialogActionCancel:
			a.mergerDialog.SetVisible(false)
			a.mergerDialog = nil
		}
		return a, nil
	}

	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		action := a.spinOffDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitSpinOffDialog()
		case dialog.DialogActionAlternate:
			return a.startSpinOffPriceLookup()
		case dialog.DialogActionCancel:
			a.spinOffDialog.SetVisible(false)
			a.spinOffDialog = nil
		}
		return a, nil
	}

	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		action := a.cashOperationDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			return a.submitCashOperationDialog()
		case dialog.DialogActionCancel:
			a.cashOperationDialog.SetVisible(false)
			a.cashOperationDialog = nil
		}
		return a, nil
	}

	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		action := a.investmentTypeSelector.HandleMouse(msg, a.width, a.height)
		switch action {
		case dialog.DialogActionSubmit:
			fields := a.investmentTypeSelector.Fields()
			selectedType := investmentTransactionTypeFromIndex(fields[0].SelectedIndex)
			a.investmentTypeSelector.SetVisible(false)
			a.investmentTypeSelector = nil
			switch selectedType {
			case investment.TransactionTypeBuy:
				return a, a.loadBuyDialogData()
			case investment.TransactionTypeSell:
				return a, a.loadSellDialogData()
			case investment.TransactionTypeDividend:
				a.dividendDialogReinvest = false
				return a, a.loadDividendDialogData()
			case investment.TransactionTypeReinvestDividend:
				a.dividendDialogReinvest = true
				return a, a.loadDividendDialogData()
			case investment.TransactionTypeDeposit,
				investment.TransactionTypeWithdrawal,
				investment.TransactionTypeFee,
				investment.TransactionTypeInterest:
				a.cashOperationType = selectedType
				editTxn, ok := a.loadInvestmentEditTxn()
				if !ok {
					return a, nil
				}
				a.cashOperationDialog = buildCashOperationDialog(selectedType.DisplayName(), editTxn)
				if editTxn == nil {
					a.cashOperationDialog.SeedDateField(a.txnDialogLastSavedDate)
				}
				return a, nil
			case investment.TransactionTypeTransferCash:
				if a.investmentEditTxnID != types.NilID {
					return a, a.loadEditInvestmentTransferDialogData(a.investmentEditTxnID)
				}
				return a, a.loadTransferDialogData()
			case investment.TransactionTypeTransferShares:
				return a, a.loadTransferSharesDialogData()
			}
		case dialog.DialogActionCancel:
			a.investmentTypeSelector.SetVisible(false)
			a.investmentTypeSelector = nil
		}
		return a, nil
	}

	return a, nil
}
