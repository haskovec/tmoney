package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/models"
)

// transferDialogData holds the loaded data needed for the transfer dialog.
type transferDialogData struct {
	accounts   []*models.Account
	accountIDs []models.ID // parallel to account dropdown options
}

// transferDialogDataMsg is sent when transfer dialog data has been loaded.
type transferDialogDataMsg struct {
	data *transferDialogData
}

// transferDialogSavedMsg is sent when a transfer has been saved.
type transferDialogSavedMsg struct{}

// buildAccountOptions builds parallel display name and ID slices for account selectors.
func buildAccountOptions(accounts []*models.Account) ([]string, []models.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]models.ID, 0, len(accounts))

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
	d.AddTextField("Amount", "", "100.00", 12)

	// Date field - default to today in MM/DD/YYYY
	today := time.Now().Format("01/02/2006")
	d.AddTextField("Date", today, "MM/DD/YYYY", 10)

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

	// From account
	fromIdx := fields[0].SelectedIndex
	if fromIdx < 0 || fromIdx >= len(a.transferDialogAccountIDs) {
		a.err = fmt.Errorf("please select a From account")
		return a, nil
	}
	fromAccountID := a.transferDialogAccountIDs[fromIdx]

	// To account
	toIdx := fields[1].SelectedIndex
	if toIdx < 0 || toIdx >= len(a.transferDialogAccountIDs) {
		a.err = fmt.Errorf("please select a To account")
		return a, nil
	}
	toAccountID := a.transferDialogAccountIDs[toIdx]

	// Validate from != to
	if fromAccountID == toAccountID {
		a.err = fmt.Errorf("From and To accounts must be different")
		return a, nil
	}

	// Parse amount
	amount, err := parseAmountInput(fields[2].Value)
	if err != nil {
		a.err = fmt.Errorf("invalid amount: %w", err)
		return a, nil
	}

	// Amount must be positive for transfers
	if !amount.IsPositive() {
		a.err = fmt.Errorf("transfer amount must be positive")
		return a, nil
	}

	// Parse date
	date, err := parseDateInput(fields[3].Value)
	if err != nil {
		a.err = fmt.Errorf("invalid date: %w", err)
		return a, nil
	}

	// Memo
	memo := strings.TrimSpace(fields[4].Value)

	// Close dialog before async save for responsive UI
	a.closeTransferDialog()

	return a, func() tea.Msg {
		if a.transactionSvc == nil {
			return errMsg{err: fmt.Errorf("transaction service not available")}
		}

		pair, err := a.transactionSvc.CreateTransfer(fromAccountID, toAccountID, date, amount)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
		}

		// Set memo if provided
		if memo != "" {
			transferID := pair.FromTransaction.TransferID.ID
			if err := a.transactionSvc.UpdateTransfer(transferID, date, amount, memo, models.TransactionStatusPending); err != nil {
				return errMsg{err: fmt.Errorf("transfer created but failed to set memo: %w", err)}
			}
		}

		return transferDialogSavedMsg{}
	}
}
