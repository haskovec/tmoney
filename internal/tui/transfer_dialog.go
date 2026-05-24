package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transferDialogMode distinguishes a New-Transfer dialog from an
// Edit-Transfer dialog.
type transferDialogMode int

const (
	transferDialogModeNew transferDialogMode = iota
	transferDialogModeEdit
)

// transferDispatchKind identifies which service path the unified Transfer
// dialog should take, based on the (From.Type, To.Type) combination. Mapped
// 1:1 to the four undo commands in the new-transfer flow.
type transferDispatchKind int

const (
	// transferDispatchRegToReg covers bank↔bank — both legs are non-investment.
	transferDispatchRegToReg transferDispatchKind = iota
	// transferDispatchInvToReg covers cash leaving an investment account for a
	// regular account (e.g. brokerage → checking withdrawal).
	transferDispatchInvToReg
	// transferDispatchRegToInv covers cash flowing from a regular account into
	// an investment account (e.g. checking → 401k contribution).
	transferDispatchRegToInv
	// transferDispatchInvToInv covers cash moving between two investment
	// accounts (e.g. IRA → IRA rollover).
	transferDispatchInvToInv
)

// chooseTransferDispatch picks the service path for the unified Transfer
// dialog from the From/To account types. HSA counts as an investment type
// (see account.Type.IsInvestmentType).
func chooseTransferDispatch(fromType, toType account.Type) transferDispatchKind {
	fromInv := fromType.IsInvestmentType()
	toInv := toType.IsInvestmentType()
	switch {
	case fromInv && toInv:
		return transferDispatchInvToInv
	case fromInv:
		return transferDispatchInvToReg
	case toInv:
		return transferDispatchRegToInv
	default:
		return transferDispatchRegToReg
	}
}

// accountTypeByID returns the Type for the account with the given ID, or the
// zero value if the account is not in the slice. Unknown accounts dispatch as
// non-investment, falling through to the regular transfer path where the
// existing service-layer guards take over.
func accountTypeByID(accounts []*account.Account, id types.ID) account.Type {
	for _, a := range accounts {
		if a.ID == id {
			return a.Type
		}
	}
	return ""
}

// transferDialogData holds the loaded data needed for the transfer dialog.
type transferDialogData struct {
	accounts   []*account.Account
	accountIDs []types.ID // parallel to account dropdown options

	// Edit-mode-only fields. Both are zero in new mode.
	mode     transferDialogMode
	existing *transaction.TransferPair
}

// transferDialogDataMsg is sent when transfer dialog data has been loaded.
type transferDialogDataMsg struct {
	data *transferDialogData
}

// transferDialogSavedMsg is sent when a transfer has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type transferDialogSavedMsg struct {
	savedDate types.Date
}

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

	// Date field - masked MM/DD/YYYY, defaults to today
	f = d.AddDateField("Date", "")
	f.Required = true

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// buildEditTransferDialog builds the edit-mode transfer dialog. Title is
// "Edit Transfer"; the From/To accounts are rendered as a read-only body
// message ("Checking → Savings") since UpdateTransfer cannot move a
// transfer between accounts. Editable fields are Amount (positive),
// Date, Memo, and Status — all pre-filled from the existing pair.
func buildEditTransferDialog(fromName, toName string, pair *transaction.TransferPair) *Dialog {
	d := NewDialog("Edit Transfer")
	d.SetMessage(fromName + " → " + toName)

	// Amount — use the positive (to) side so the field is always positive,
	// matching the New-Transfer semantics.
	amountValue := ""
	if pair != nil && pair.ToTransaction != nil {
		amountValue = pair.ToTransaction.Amount.String()
	}
	f := d.AddTextField("Amount", amountValue, "100.00", 12)
	f.Required = true

	// Date
	dateStr := ""
	if pair != nil && pair.FromTransaction != nil {
		dateStr = pair.FromTransaction.Date.Time().Format("01/02/2006")
	}
	f = d.AddDateField("Date", dateStr)
	f.Required = true

	// Memo
	memo := ""
	if pair != nil && pair.FromTransaction != nil && pair.FromTransaction.Memo.Valid {
		memo = pair.FromTransaction.Memo.String
	}
	d.AddTextField("Memo", memo, "Optional memo", 0)

	// Status — radio: 0 = Uncleared, 1 = Cleared.
	statusIdx := 0
	if pair != nil && pair.FromTransaction != nil && pair.FromTransaction.Status == transaction.StatusCleared {
		statusIdx = 1
	}
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, statusIdx)

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

// loadEditTransferDialogData returns a command that loads accounts and the
// existing transfer pair (resolved from any one transaction ID belonging to
// the pair), then emits a transferDialogDataMsg in edit mode so the
// transfer dialog opens pre-filled for editing.
func (a *App) loadEditTransferDialogData(transactionID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &transferDialogData{
			mode: transferDialogModeEdit,
		}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}
		_, ids := buildAccountOptions(data.accounts)
		data.accountIDs = ids

		if a.transactionSvc != nil {
			txn, err := a.transactionSvc.GetByID(transactionID)
			if err != nil {
				return errMsg{err: err}
			}
			if !txn.IsTransfer() {
				return errMsg{err: fmt.Errorf("transaction %s is not a transfer", transactionID.String())}
			}
			pair, err := a.transactionSvc.GetTransferPair(txn.TransferID.ID)
			if err != nil {
				return errMsg{err: err}
			}
			data.existing = pair
		}

		return transferDialogDataMsg{data: data}
	}
}

// transferAccountNames resolves the From/To account display names for the
// pair carried by a transferDialogData in edit mode. Falls back to "(unknown)"
// when an account isn't present in data.accounts.
func transferAccountNames(data *transferDialogData) (fromName, toName string) {
	fromName = "(unknown)"
	toName = "(unknown)"
	if data == nil || data.existing == nil {
		return
	}
	for _, acct := range data.accounts {
		if data.existing.FromTransaction != nil && acct.ID == data.existing.FromTransaction.AccountID {
			fromName = acct.Name
		}
		if data.existing.ToTransaction != nil && acct.ID == data.existing.ToTransaction.AccountID {
			toName = acct.Name
		}
	}
	return
}

// closeTransferDialog clears the transfer dialog state.
func (a *App) closeTransferDialog() {
	a.transferDialog = nil
	a.transferDialogData = nil
	a.transferDialogAccountIDs = nil
}

// handleTransferDialogKey routes key events to the transfer dialog.
func (a *App) handleTransferDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

	if a.transferDialogData.mode == transferDialogModeEdit {
		return a.submitEditTransferDialog()
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

	// Dispatch on the (From, To) account types so each combination posts via
	// the right service. Account types come from the loaded accounts list,
	// which the dialog data populates from accountSvc.List(true).
	fromType := accountTypeByID(a.transferDialogData.accounts, fromAccountID)
	toType := accountTypeByID(a.transferDialogData.accounts, toAccountID)
	kind := chooseTransferDispatch(fromType, toType)

	// Close dialog before async save for responsive UI
	a.closeTransferDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		switch kind {
		case transferDispatchRegToReg:
			if a.transactionSvc == nil {
				return errMsg{err: fmt.Errorf("transaction service not available")}
			}
			cmd := undo.NewCreateTransferCommand(a.transactionSvc, fromAccountID, toAccountID, date, amount)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
			// Set memo if provided. CreateTransfer doesn't accept memo, so we
			// apply it via UpdateTransfer after the create. Undo of the transfer
			// deletes both sides, so the memo-set step needs no separate undo.
			if memo != "" {
				pair := cmd.Pair()
				if pair != nil {
					transferID := pair.FromTransaction.TransferID.ID
					if err := a.transactionSvc.UpdateTransfer(transferID, date, amount, memo, transaction.StatusUncleared); err != nil {
						return errMsg{err: fmt.Errorf("transfer created but failed to set memo: %w", err)}
					}
				}
			}
		case transferDispatchInvToReg:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			cmd := undo.NewCreateInvestmentTransferCashCommand(a.investmentSvc, fromAccountID, toAccountID, date, amount, memo)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
		case transferDispatchRegToInv:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			// DepositFromAccount expects (investmentAccountID, regularAccountID);
			// here the investment account is the destination.
			cmd := undo.NewCreateInvestmentDepositCommand(a.investmentSvc, toAccountID, fromAccountID, date, amount, memo)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
		case transferDispatchInvToInv:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			cmd := undo.NewCreateInvestmentToInvestmentTransferCommand(a.investmentSvc, fromAccountID, toAccountID, date, amount, memo)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
		}

		return transferDialogSavedMsg{savedDate: date}
	}
}

// submitEditTransferDialog validates the edit-mode transfer dialog and
// dispatches an EditTransferCommand. Edit-mode field layout is Amount(0),
// Date(1), Memo(2), Status(3).
func (a *App) submitEditTransferDialog() (tea.Model, tea.Cmd) {
	pair := a.transferDialogData.existing
	if pair == nil || pair.FromTransaction == nil {
		return a, nil
	}

	fields := a.transferDialog.Fields()
	if len(fields) < 4 {
		return a, nil
	}

	a.transferDialog.ClearErrors()
	hasErrors := false

	amount, err := parseAmountInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid amount"
		hasErrors = true
	} else if !amount.IsPositive() {
		fields[0].Error = "Amount must be positive"
		hasErrors = true
	}

	date, err := parseDateInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	memo := strings.TrimSpace(fields[2].Value)

	status := transaction.StatusUncleared
	if fields[3].SelectedIndex == 1 {
		status = transaction.StatusCleared
	}

	transferID := pair.FromTransaction.TransferID.ID

	a.closeTransferDialog()

	return a, func() tea.Msg {
		if a.transactionSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("transaction service not available")}
		}
		cmd := undo.NewEditTransferCommand(a.transactionSvc, transferID, date, amount, memo, status)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to update transfer: %w", err)}
		}
		return transferDialogSavedMsg{}
	}
}
