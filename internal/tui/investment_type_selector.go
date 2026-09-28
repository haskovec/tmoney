// The investment type selector: a small modal dialog that picks which
// investment transaction dialog to open, for a new row or an edit. It is a modal
// surface registered in modals(), not view code.

package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// investmentTransactionTypeOptions returns the display names for the investment type selector.
func investmentTransactionTypeOptions() []string {
	return []string{
		investment.TransactionTypeBuy.DisplayName(),
		investment.TransactionTypeSell.DisplayName(),
		investment.TransactionTypeDividend.DisplayName(),
		investment.TransactionTypeReinvestDividend.DisplayName(),
		investment.TransactionTypeDeposit.DisplayName(),
		investment.TransactionTypeWithdrawal.DisplayName(),
		investment.TransactionTypeInterest.DisplayName(),
		investment.TransactionTypeFee.DisplayName(),
		investment.TransactionTypeFeeLiquidation.DisplayName(),
		investment.TransactionTypeTransferCash.DisplayName(),
		investment.TransactionTypeTransferShares.DisplayName(),
	}
}

// investmentTransactionTypeFromIndex maps a selector index back to an InvestmentTransactionType.
func investmentTransactionTypeFromIndex(idx int) investment.TransactionType {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeFeeLiquidation,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	if idx >= 0 && idx < len(types) {
		return types[idx]
	}
	return investment.TransactionTypeBuy
}

// investmentTransactionTypeIndex returns the selector index for the given transaction type.
func investmentTransactionTypeIndex(txnType investment.TransactionType) int {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeFeeLiquidation,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	for i, t := range types {
		if t == txnType {
			return i
		}
	}
	return 0
}

// openInvestmentTypeSelector opens the transaction type selector dialog.
// If editing is true, the selector is pre-set to the currently selected transaction's type.
func (a *App) openInvestmentTypeSelector(editing bool) {
	options := investmentTransactionTypeOptions()
	selectedIdx := 0

	if editing {
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			a.investmentEditTxnID = txn.ID
			selectedIdx = investmentTransactionTypeIndex(txn.Type)
		}
		// Editing selects the security from the edited row, not the filter.
		a.investmentNewTxnSecurityID = types.NilID
	} else {
		a.investmentEditTxnID = types.NilID
		// A new transaction opened while the register is locked to a security
		// pre-selects that security in the security-bearing dialogs.
		a.investmentNewTxnSecurityID = a.investmentFilterLockedSec
		// Spin-Off is a corporate action, not a transaction type; offer it as a
		// convenience entry on the New selector (handled by index on submit).
		options = append(options, "Spin-Off…")
	}

	title := "New Transaction"
	if editing {
		title = "Edit Transaction"
	}

	d := dialog.NewDialog(title)
	d.SetWidth(40)
	d.AddSelectField("Type", options, selectedIdx)
	d.SetButtons([]dialog.DialogButton{
		{Label: "OK", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	a.investmentTypeSelector = d
}

// handleInvestmentTypeSelectorKey handles key presses in the investment type selector dialog.
func (a *App) handleInvestmentTypeSelectorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.investmentTypeSelectorAction(a.investmentTypeSelector.HandleKey(msg))
}

// investmentTypeSelectorAction dispatches a DialogAction for the investment type selector, from either input path.
func (a *App) investmentTypeSelectorAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		fields := a.investmentTypeSelector.Fields()
		idx := fields[0].SelectedIndex
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil
		return a.dispatchInvestmentTypeSelection(idx)

	case dialog.DialogActionCancel:
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil
		return a, nil
	}

	return a, nil
}

// dispatchInvestmentTypeSelection opens the dialog for the transaction type
// chosen in the investment type selector. Shared by the keyboard and mouse
// selector handlers so the two paths cannot drift apart.
func (a *App) dispatchInvestmentTypeSelection(idx int) (tea.Model, tea.Cmd) {
	// "Spin-Off…" is appended after the transaction types on the New
	// selector; it opens the (global) spin-off dialog with the selected
	// holding pre-filled as the parent security.
	if idx >= len(investmentTransactionTypeOptions()) {
		a.spinOff.preSelectedID = nil
		if txn := a.selectedInvestmentTransaction(); txn != nil && txn.SecurityID.Valid {
			secID := txn.SecurityID.ID
			a.spinOff.preSelectedID = &secID
		}
		return a, a.loadSpinOffDialogData()
	}

	selectedType := investmentTransactionTypeFromIndex(idx)

	// A transfer leg keeps its type. Every other type's edit deletes one row
	// and writes one row, which would leave the other leg without its pair.
	editTxn, ok := a.loadInvestmentEditTxn()
	if !ok {
		return a, nil
	}
	if editTxn != nil && editTxn.TransferID.Valid && editTxn.Type != selectedType {
		a.statusbar.AddNotification("A transfer row cannot change type. Delete the transfer and enter a new row.", widget.NotificationAlert)
		return a, nil
	}

	switch selectedType {
	case investment.TransactionTypeBuy:
		return a, a.loadBuyDialogData()
	case investment.TransactionTypeSell:
		return a, a.loadSellDialogData()
	case investment.TransactionTypeDividend:
		a.dividend.reinvest = false
		return a, a.loadDividendDialogData()
	case investment.TransactionTypeReinvestDividend:
		a.dividend.reinvest = true
		return a, a.loadDividendDialogData()
	case investment.TransactionTypeFeeLiquidation:
		return a, a.loadFeeLiquidationDialogData()
	case investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeFee,
		investment.TransactionTypeInterest:
		a.cashOperation.opType = selectedType
		a.cashOperation.dlg = buildCashOperationDialog(selectedType.DisplayName(), editTxn)
		if editTxn == nil {
			a.cashOperation.dlg.SeedDateField(a.txnDialogLastSavedDate)
		}
		return a, nil
	case investment.TransactionTypeTransferCash:
		switch {
		case editTxn == nil:
			return a, a.transfer.open(a.transferDeps())
		case isCashTransferLeg(editTxn):
			return a, a.transfer.openForEdit(a.transferDeps(), editTxn.ID)
		case editTxn.Type == investment.TransactionTypeDeposit || editTxn.Type == investment.TransactionTypeWithdrawal:
			return a, a.transfer.openToReplace(a.transferDeps(), editTxn)
		default:
			a.statusbar.AddNotification("Only a Deposit or a Withdrawal can change to a transfer.", widget.NotificationAlert)
			return a, nil
		}
	case investment.TransactionTypeTransferShares:
		return a, a.loadTransferSharesDialogData()
	}

	return a, nil
}
