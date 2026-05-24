package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// transferCashDialogData holds the loaded data needed for the investment cash transfer dialog.
type transferCashDialogData struct {
	accounts   []*account.Account
	accountIDs []types.ID // parallel to account dropdown options (non-investment accounts only)
}

// transferCashDialogDataMsg is sent when cash transfer dialog data has been loaded.
type transferCashDialogDataMsg struct {
	data *transferCashDialogData
}

// transferCashDialogSavedMsg is sent when a cash transfer has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type transferCashDialogSavedMsg struct {
	savedDate types.Date
}

// buildNonInvestmentAccountOptions builds parallel display name and ID slices
// for non-investment accounts (used as the linked account in cash transfers).
func buildNonInvestmentAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))

	for _, acct := range accounts {
		if acct.Type.IsInvestmentType() {
			continue
		}
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}

	return options, ids
}

// buildTransferCashDialog creates a Dialog for transferring cash between an
// investment account and a regular (non-investment) account. The dialog
// carries an in-dialog Direction toggle so the user picks "deposit into" vs
// "withdraw from" the investment account inside the dialog rather than via a
// separate menu choice; the linked Amount stays positive in both directions.
//
// investmentAccountName is interpolated into the Direction option labels so
// users see e.g. "Deposit into Brokerage" / "Withdraw from Brokerage".
// When editTxn is non-nil the Direction defaults to match the stored
// transaction's sign (negative TotalAmount → withdraw).
func buildTransferCashDialog(investmentAccountName string, accountOptions []string, editTxn *investment.Transaction, accountIDs []types.ID) *Dialog {
	d := NewDialog("Transfer Cash")
	d.SetWidth(70)

	// Direction selector (first field). Default to deposit; flip to
	// withdraw if we're editing a stored negative-amount transfer.
	acctLabel := investmentAccountName
	if acctLabel == "" {
		acctLabel = "this account"
	}
	directionOptions := []string{
		"Deposit into " + acctLabel,
		"Withdraw from " + acctLabel,
	}
	directionIdx := 0
	if editTxn != nil && editTxn.TotalAmount.IsNegative() {
		directionIdx = 1
	}
	d.AddSelectField("Direction", directionOptions, directionIdx)

	// Linked account selector
	selectedIdx := 0
	if editTxn != nil && editTxn.TransferAccountID.Valid {
		for i, id := range accountIDs {
			if id == editTxn.TransferAccountID.ID {
				selectedIdx = i
				break
			}
		}
	}
	d.AddSelectField("Other account", accountOptions, selectedIdx)

	// Amount
	amountVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		amountVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	f := d.AddTextField("Amount", amountVal, "500.00", 12)
	f.Required = true

	// Date
	dateVal := ""
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f = d.AddDateField("Date", dateVal)
	f.Required = true

	// Memo
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadTransferCashDialogData returns a command that loads non-investment accounts for the cash transfer dialog.
func (a *App) loadTransferCashDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &transferCashDialogData{}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		_, ids := buildNonInvestmentAccountOptions(data.accounts)
		data.accountIDs = ids

		return transferCashDialogDataMsg{data: data}
	}
}

// closeTransferCashDialog clears the cash transfer dialog state.
func (a *App) closeTransferCashDialog() {
	a.transferCashDialog = nil
	a.transferCashDialogData = nil
	a.transferCashDialogAccountIDs = nil
}

// handleTransferCashDialogKey routes key events to the cash transfer dialog.
func (a *App) handleTransferCashDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.transferCashDialog == nil {
		return a, nil
	}

	action := a.transferCashDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitTransferCashDialog()
	case DialogActionCancel:
		a.closeTransferCashDialog()
		return a, nil
	}

	return a, nil
}

// submitTransferCashDialog parses dialog fields, validates, and saves the cash transfer.
func (a *App) submitTransferCashDialog() (tea.Model, tea.Cmd) {
	if a.transferCashDialog == nil || a.transferCashDialogData == nil {
		return a, nil
	}

	fields := a.transferCashDialog.Fields()
	if len(fields) < 5 {
		return a, nil
	}

	a.transferCashDialog.ClearErrors()
	hasErrors := false

	// Direction (index 0): 0 = deposit into investment, 1 = withdraw from investment.
	depositIntoInvestment := fields[0].SelectedIndex == 0

	// Account (index 1)
	if len(a.transferCashDialogAccountIDs) == 0 {
		fields[1].Error = "No accounts available"
		hasErrors = true
	}
	acctIdx := fields[1].SelectedIndex
	var regularAccountID types.ID
	if acctIdx >= 0 && acctIdx < len(a.transferCashDialogAccountIDs) {
		regularAccountID = a.transferCashDialogAccountIDs[acctIdx]
	} else {
		fields[1].Error = "Select an account"
		hasErrors = true
	}

	// Amount (index 2)
	amount, err := parseOptionalMoneyInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid amount"
		hasErrors = true
	}
	if amount == nil {
		fields[2].Error = "Amount is required"
		hasErrors = true
	}

	// Date (index 3)
	date, err := parseDateInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 4)
	memo := strings.TrimSpace(fields[4].Value)

	// Get investment account ID
	investmentAccountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		investmentAccountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID
	amountVal := *amount

	// Close dialog before async save
	a.closeTransferCashDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var txnErr error
		if editTxnID != types.NilID {
			directionToken := "out"
			if depositIntoInvestment {
				directionToken = "in"
			}
			// Legacy dialog has no Status field; preserve historic behavior
			// (status resets to Uncleared/Pending on edit). The unified
			// Transfer dialog is what threads user-selected status through.
			_, txnErr = a.investmentSvc.UpdateTransferCash(
				editTxnID,
				investmentAccountID,
				regularAccountID,
				date,
				amountVal,
				memo,
				directionToken,
				transaction.StatusUncleared,
			)
		} else if depositIntoInvestment {
			_, txnErr = a.investmentSvc.DepositFromAccount(investmentAccountID, regularAccountID, date, amountVal, memo)
		} else {
			_, txnErr = a.investmentSvc.TransferCash(investmentAccountID, regularAccountID, date, amountVal, memo)
		}

		if txnErr != nil {
			return errMsg{err: fmt.Errorf("failed to transfer cash: %w", txnErr)}
		}

		return transferCashDialogSavedMsg{savedDate: date}
	}
}
