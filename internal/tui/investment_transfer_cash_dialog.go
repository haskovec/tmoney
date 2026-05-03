package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
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
type transferCashDialogSavedMsg struct{}

// buildNonInvestmentAccountOptions builds parallel display name and ID slices
// for non-investment accounts (used as the linked account in cash transfers).
func buildNonInvestmentAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))

	for _, acct := range accounts {
		if acct.Type == account.TypeInvestment {
			continue
		}
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}

	return options, ids
}

// buildTransferCashDialog creates a Dialog for transferring cash between an investment
// account and a regular (non-investment) account.
// direction: "deposit" to move cash into the investment account, "withdraw" to move cash out.
func buildTransferCashDialog(direction string, accountOptions []string, editTxn *investment.Transaction, accountIDs []types.ID) *Dialog {
	title := "Transfer Cash In"
	if direction == "withdraw" {
		title = "Transfer Cash Out"
	}

	d := NewDialog(title)
	d.SetWidth(50)

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
	d.AddSelectField("Account", accountOptions, selectedIdx)

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
	dateVal := time.Now().Format("01/02/2006")
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f = d.AddTextField("Date", dateVal, "MM/DD/YYYY", 10)
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
	a.transferCashDirection = ""
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
	if len(fields) < 4 {
		return a, nil
	}

	a.transferCashDialog.ClearErrors()
	hasErrors := false

	// Account (index 0)
	if len(a.transferCashDialogAccountIDs) == 0 {
		fields[0].Error = "No accounts available"
		hasErrors = true
	}
	acctIdx := fields[0].SelectedIndex
	var regularAccountID types.ID
	if acctIdx >= 0 && acctIdx < len(a.transferCashDialogAccountIDs) {
		regularAccountID = a.transferCashDialogAccountIDs[acctIdx]
	} else {
		fields[0].Error = "Select an account"
		hasErrors = true
	}

	// Amount (index 1)
	amount, err := parseOptionalMoneyInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid amount"
		hasErrors = true
	}
	if amount == nil {
		fields[1].Error = "Amount is required"
		hasErrors = true
	}

	// Date (index 2)
	date, err := parseDateInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 3)
	memo := strings.TrimSpace(fields[3].Value)

	// Get investment account ID
	investmentAccountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		investmentAccountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID
	direction := a.transferCashDirection
	amountVal := *amount

	// Close dialog before async save
	a.closeTransferCashDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		if editTxnID != types.NilID {
			if a.investmentRepo != nil {
				if err := a.investmentRepo.Delete(editTxnID); err != nil {
					return errMsg{err: fmt.Errorf("failed to delete old transaction: %w", err)}
				}
			}
		}

		var txnErr error
		if direction == "deposit" {
			_, txnErr = a.investmentSvc.DepositFromAccount(investmentAccountID, regularAccountID, date, amountVal, memo)
		} else {
			_, txnErr = a.investmentSvc.TransferCash(investmentAccountID, regularAccountID, date, amountVal, memo)
		}

		if txnErr != nil {
			return errMsg{err: fmt.Errorf("failed to transfer cash: %w", txnErr)}
		}

		return transferCashDialogSavedMsg{}
	}
}
