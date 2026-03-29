package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transferDialogData holds the loaded data needed for the transfer dialog.
type transferDialogData struct {
	accounts   []*account.Account
	accountIDs []types.ID // parallel to account dropdown options
}

// transferDialogDataMsg is sent when transfer dialog data has been loaded.
type transferDialogDataMsg struct {
	data *transferDialogData
}

// transferDialogSavedMsg is sent when a transfer has been saved.
type transferDialogSavedMsg struct{}

// buildAccountOptions builds parallel display name and ID slices for account selectors.
func buildAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))

	for _, acct := range accounts {
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}

	return options, ids
}

// buildTransferDialog creates a Dialog for entering a new transfer.
// defaultFromIndex is the index of the currently selected account to pre-select as "From".
func buildTransferDialog(accountOptions []string, defaultFromIndex int) *Dialog {
	d := NewDialog("New Transfer")

	// From account
	d.AddSelectField("From", accountOptions, defaultFromIndex)

	// To account - default to 0 (first account)
	toIndex := 0
	if defaultFromIndex == 0 && len(accountOptions) > 1 {
		toIndex = 1
	}
	d.AddSelectField("To", accountOptions, toIndex)

	// Amount (positive)
	f := d.AddTextField("Amount", "", "100.00", 12)
	f.Required = true

	// Date field - default to today in MM/DD/YYYY
	today := time.Now().Format("01/02/2006")
	f = d.AddTextField("Date", today, "MM/DD/YYYY", 10)
	f.Required = true

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadTransferDialogData returns a command that loads accounts for the transfer dialog.
func (a *App) loadTransferDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &transferDialogData{}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		_, ids := buildAccountOptions(data.accounts)
		data.accountIDs = ids

		return transferDialogDataMsg{data: data}
	}
}

// closeTransferDialog clears the transfer dialog state.
func (a *App) closeTransferDialog() {
	a.transferDialog = nil
	a.transferDialogData = nil
	a.transferDialogAccountIDs = nil
}

// handleTransferDialogKey routes key events to the transfer dialog.
func (a *App) handleTransferDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.transferDialog == nil {
		return a, nil
	}

	action := a.transferDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitTransferDialog()
	case DialogActionCancel:
		a.closeTransferDialog()
		return a, nil
	}

	return a, nil
}

// submitTransferDialog parses dialog fields, validates, and saves the transfer.
func (a *App) submitTransferDialog() (tea.Model, tea.Cmd) {
	if a.transferDialog == nil || a.transferDialogData == nil {
		return a, nil
	}

	fields := a.transferDialog.Fields()
	if len(fields) < 5 {
		return a, nil
	}

	a.transferDialog.ClearErrors()
	hasErrors := false

	// From account
	fromIdx := fields[0].SelectedIndex
	if fromIdx < 0 || fromIdx >= len(a.transferDialogAccountIDs) {
		fields[0].Error = "Please select a From account"
		hasErrors = true
	}
	fromAccountID := types.NilID
	if fromIdx >= 0 && fromIdx < len(a.transferDialogAccountIDs) {
		fromAccountID = a.transferDialogAccountIDs[fromIdx]
	}

	// To account
	toIdx := fields[1].SelectedIndex
	if toIdx < 0 || toIdx >= len(a.transferDialogAccountIDs) {
		fields[1].Error = "Please select a To account"
		hasErrors = true
	}
	toAccountID := types.NilID
	if toIdx >= 0 && toIdx < len(a.transferDialogAccountIDs) {
		toAccountID = a.transferDialogAccountIDs[toIdx]
	}

	// Validate from != to
	if !fromAccountID.IsNil() && !toAccountID.IsNil() && fromAccountID == toAccountID {
		a.transferDialog.SetErrorMsg("From and To accounts must be different")
		hasErrors = true
	}

	// Parse amount
	amount, err := parseAmountInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid amount"
		hasErrors = true
	} else if !amount.IsPositive() {
		fields[2].Error = "Amount must be positive"
		hasErrors = true
	}

	// Parse date
	date, err := parseDateInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo
	memo := strings.TrimSpace(fields[4].Value)

	// Close dialog before async save for responsive UI
	a.closeTransferDialog()

	return a, func() tea.Msg {
		if a.transactionSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("transaction service not available")}
		}

		cmd := undo.NewCreateTransferCommand(a.transactionSvc, fromAccountID, toAccountID, date, amount)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
		}

		// Set memo if provided (undo of the transfer deletes both sides, so no separate undo needed)
		if memo != "" {
			pair := cmd.Pair()
			if pair != nil {
				transferID := pair.FromTransaction.TransferID.ID
				if err := a.transactionSvc.UpdateTransfer(transferID, date, amount, memo, transaction.StatusUncleared); err != nil {
					return errMsg{err: fmt.Errorf("transfer created but failed to set memo: %w", err)}
				}
			}
		}

		return transferDialogSavedMsg{}
	}
}
