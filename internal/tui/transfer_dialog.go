package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
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

// accountByID returns the account with the given ID from the loaded list, or nil.
func accountByID(accounts []*account.Account, id types.ID) *account.Account {
	for _, a := range accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// transferOpeningDateError returns the inline date-field error when date is
// before the opening date of any of the named accounts, or "".
func transferOpeningDateError(accounts []*account.Account, date types.Date, ids ...types.ID) string {
	for _, id := range ids {
		if acct := accountByID(accounts, id); acct != nil {
			if msg := openingDateFieldError(acct, date); msg != "" {
				return msg
			}
		}
	}
	return ""
}

// transferDialogData holds the loaded data needed for the transfer dialog.
type transferDialogData struct {
	accounts   []*account.Account
	accountIDs []types.ID // parallel to account dropdown options

	// Edit-mode-only fields. Zero in new mode. Exactly one of existing /
	// existingInvestment is set in edit mode: existing for bank↔bank
	// transfers, existingInvestment whenever at least one leg lives in an
	// investment account (inv↔reg, reg↔inv, inv↔inv).
	mode               transferDialogMode
	existing           *transaction.TransferPair
	existingInvestment *investmentTransferEdit
}

// investmentTransferEdit is the edit-mode snapshot the unified Transfer
// dialog uses when one or both legs of the original transfer live in an
// investment account. The dialog only needs the human-visible fields plus
// the investment-side transaction ID that UpdateTransferCash takes as its
// first argument.
type investmentTransferEdit struct {
	fromAccountID   types.ID
	toAccountID     types.ID
	amount          types.Money // positive
	date            types.Date
	memo            string
	status          transaction.Status
	investmentTxnID types.ID // ID of the investment-side row, fed to UpdateTransferCash
}

// transferDialogDataMsg is sent when transfer dialog data has been loaded.
type transferDialogDataMsg struct {
	data *transferDialogData
}

// transferDialogSavedMsg is sent when a transfer has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
//
// savedID is the ID of the transfer leg living in the register the user is
// currently viewing (NilID if neither leg belongs to the current register),
// so the register can move the cursor onto the just-saved row. savedIsInvestment
// reports whether that leg is an investment transaction, which tells the handler
// whether to select it in the investment register or the regular register.
type transferDialogSavedMsg struct {
	savedDate         types.Date
	savedID           types.ID
	savedIsInvestment bool
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

// buildTransferDialog creates a dialog.Dialog for entering a new transfer.
// defaultFromIndex is the index of the currently selected account to pre-select as "From".
func buildTransferDialog(accountOptions []string, defaultFromIndex int) *dialog.Dialog {
	d := dialog.NewDialog("New Transfer")

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
// Date, Memo, and Status — all pre-filled from the supplied values.
func buildEditTransferDialog(fromName, toName string, amount types.Money, date types.Date, memo string, status transaction.Status) *dialog.Dialog {
	d := dialog.NewDialog("Edit Transfer")
	d.SetMessage(fromName + " → " + toName)

	f := d.AddTextField("Amount", amount.String(), "100.00", 12)
	f.Required = true

	f = d.AddDateField("Date", date.Time().Format("01/02/2006"))
	f.Required = true

	d.AddTextField("Memo", memo, "Optional memo", 0)

	statusIdx := 0
	if status == transaction.StatusCleared {
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
//
// When the bank-side row's counterpart lives in the investment ledger (i.e.
// the user opened the regular-side leg of an inv↔reg transfer in a bank
// register), the loader builds an existingInvestment payload instead, so
// the unified dialog's edit submit routes through the investment service.
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

		if a.transactionSvc == nil {
			return transferDialogDataMsg{data: data}
		}

		txn, err := a.transactionSvc.GetByID(transactionID)
		if err != nil {
			return errMsg{err: err}
		}
		if !txn.IsTransfer() {
			return errMsg{err: fmt.Errorf("transaction %s is not a transfer", transactionID.String())}
		}

		counterpartType := accountTypeByID(data.accounts, txn.TransferAccountID.ID)
		if counterpartType.IsInvestmentType() {
			if a.investmentRepo == nil {
				return errMsg{err: fmt.Errorf("investment repository not available")}
			}
			invLegs, err := a.investmentRepo.ListByAccount(txn.TransferAccountID.ID, investment.TransactionFilter{})
			if err != nil {
				return errMsg{err: err}
			}
			var invLeg *investment.Transaction
			for _, l := range invLegs {
				if l.TransferID.Valid && l.TransferID.ID == txn.TransferID.ID {
					invLeg = l
					break
				}
			}
			if invLeg == nil {
				return errMsg{err: fmt.Errorf("investment counterpart for transfer %s not found", txn.TransferID.ID.String())}
			}

			var fromAccountID, toAccountID types.ID
			amount := txn.Amount
			if amount.IsNegative() {
				fromAccountID = txn.AccountID
				toAccountID = txn.TransferAccountID.ID
				amount = amount.Neg()
			} else {
				fromAccountID = txn.TransferAccountID.ID
				toAccountID = txn.AccountID
			}

			memo := ""
			if txn.Memo.Valid {
				memo = txn.Memo.String
			}
			data.existingInvestment = &investmentTransferEdit{
				fromAccountID:   fromAccountID,
				toAccountID:     toAccountID,
				amount:          amount,
				date:            txn.Date,
				memo:            memo,
				status:          txn.Status,
				investmentTxnID: invLeg.ID,
			}
			return transferDialogDataMsg{data: data}
		}

		pair, err := a.transactionSvc.GetTransferPair(txn.TransferID.ID)
		if err != nil {
			return errMsg{err: err}
		}
		data.existing = pair
		return transferDialogDataMsg{data: data}
	}
}

// loadEditInvestmentTransferDialogData returns a command that loads accounts
// plus the existing inv-involving transfer pair anchored at invTxnID, then
// emits a transferDialogDataMsg in edit mode so the unified Transfer dialog
// opens pre-filled. The counterpart leg is resolved from the investment-side
// row's TransferAccountID/TransferID — when the counterpart account is also
// investment-typed the data comes from the investment repo, otherwise from
// the regular-transaction repo. From/To orientation is derived from the sign
// of the investment-side TotalAmount so the dialog's read-only "From → To"
// banner matches the cash-flow direction.
func (a *App) loadEditInvestmentTransferDialogData(invTxnID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &transferDialogData{mode: transferDialogModeEdit}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}
		_, ids := buildAccountOptions(data.accounts)
		data.accountIDs = ids

		if a.investmentRepo == nil {
			return errMsg{err: fmt.Errorf("investment repository not available")}
		}
		invTxn, err := a.investmentRepo.GetByID(invTxnID)
		if err != nil {
			return errMsg{err: err}
		}
		if !invTxn.TransferID.Valid || !invTxn.TransferAccountID.Valid {
			return errMsg{err: fmt.Errorf("investment txn %s is not a linked transfer", invTxnID.String())}
		}

		var fromAccountID, toAccountID types.ID
		amount := invTxn.TotalAmount
		if amount.IsNegative() {
			fromAccountID = invTxn.AccountID
			toAccountID = invTxn.TransferAccountID.ID
			amount = amount.Neg()
		} else {
			fromAccountID = invTxn.TransferAccountID.ID
			toAccountID = invTxn.AccountID
		}

		memo := ""
		if invTxn.Memo.Valid {
			memo = invTxn.Memo.String
		}

		data.existingInvestment = &investmentTransferEdit{
			fromAccountID:   fromAccountID,
			toAccountID:     toAccountID,
			amount:          amount,
			date:            invTxn.Date,
			memo:            memo,
			status:          statusToRegular(invTxn.Status),
			investmentTxnID: invTxnID,
		}

		return transferDialogDataMsg{data: data}
	}
}

// statusToRegular maps an investment.TransactionStatus back to the bank-side
// transaction.Status the unified Edit Transfer dialog displays. Inverse of
// the investment-package statusFromRegular helper.
func statusToRegular(s investment.TransactionStatus) transaction.Status {
	switch s {
	case investment.TransactionStatusCleared:
		return transaction.StatusCleared
	case investment.TransactionStatusReconciled:
		return transaction.StatusReconciled
	default:
		return transaction.StatusUncleared
	}
}

// transferAccountNames resolves the From/To account display names for the
// pair carried by a transferDialogData in edit mode. Falls back to "(unknown)"
// when an account isn't present in data.accounts. Handles both the regular-
// pair shape (data.existing) and the inv-involving shape (data.existingInvestment).
func transferAccountNames(data *transferDialogData) (fromName, toName string) {
	fromName = "(unknown)"
	toName = "(unknown)"
	if data == nil {
		return
	}
	var fromID, toID types.ID
	switch {
	case data.existing != nil:
		if data.existing.FromTransaction != nil {
			fromID = data.existing.FromTransaction.AccountID
		}
		if data.existing.ToTransaction != nil {
			toID = data.existing.ToTransaction.AccountID
		}
	case data.existingInvestment != nil:
		fromID = data.existingInvestment.fromAccountID
		toID = data.existingInvestment.toAccountID
	default:
		return
	}
	for _, acct := range data.accounts {
		if acct.ID == fromID {
			fromName = acct.Name
		}
		if acct.ID == toID {
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
	case dialog.DialogActionSubmit:
		return a.submitTransferDialog()
	case dialog.DialogActionCancel:
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
	} else if msg := transferOpeningDateError(a.transferDialogData.accounts, date, fromAccountID, toAccountID); msg != "" {
		fields[3].Error = msg
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
	kind := transaction.ChooseTransferDispatch(fromType, toType)

	// The leg living in the register the user is currently viewing should be
	// selected after the save, so a freshly entered transfer scrolls into view
	// just like a plain transaction does. Capture the account synchronously here
	// (the closure runs on a separate goroutine).
	currentAcct := a.currentRegisterAccountID()

	// Close dialog before async save for responsive UI
	a.closeTransferDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var savedID types.ID
		var savedIsInvestment bool

		switch kind {
		case transaction.DispatchRegToReg:
			if a.transactionSvc == nil {
				return errMsg{err: fmt.Errorf("transaction service not available")}
			}
			// CreateTransfer stamps memo (and category, once the dialog exposes
			// a picker in a later phase) on both legs at construction. Category
			// is not yet user-settable here, so an empty NullableID is passed.
			cmd := undo.NewCreateTransferCommand(a.transactionSvc, fromAccountID, toAccountID, date, amount, memo, types.NullableID{})
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
			if pair := cmd.Pair(); pair != nil {
				savedID = transferLegForAccount(currentAcct, pair.FromTransaction, pair.ToTransaction)
			}
		case transaction.DispatchInvToReg:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			cmd := undo.NewCreateInvestmentTransferCashCommand(a.investmentSvc, fromAccountID, toAccountID, date, amount, memo, types.NullableID{})
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
			savedID, savedIsInvestment = cashTransferLegForAccount(currentAcct, cmd.Result())
		case transaction.DispatchRegToInv:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			// DepositFromAccount expects (investmentAccountID, regularAccountID);
			// here the investment account is the destination.
			cmd := undo.NewCreateInvestmentDepositCommand(a.investmentSvc, toAccountID, fromAccountID, date, amount, memo, types.NullableID{})
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
			savedID, savedIsInvestment = cashTransferLegForAccount(currentAcct, cmd.Result())
		case transaction.DispatchInvToInv:
			if a.investmentSvc == nil {
				return errMsg{err: fmt.Errorf("investment service not available")}
			}
			cmd := undo.NewCreateInvestmentToInvestmentTransferCommand(a.investmentSvc, fromAccountID, toAccountID, date, amount, memo)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
			}
			if res := cmd.Result(); res != nil {
				// Both legs are investment transactions.
				if res.SourceTransaction != nil && res.SourceTransaction.AccountID == currentAcct {
					savedID, savedIsInvestment = res.SourceTransaction.ID, true
				} else if res.DestinationTransaction != nil && res.DestinationTransaction.AccountID == currentAcct {
					savedID, savedIsInvestment = res.DestinationTransaction.ID, true
				}
			}
		}

		return transferDialogSavedMsg{savedDate: date, savedID: savedID, savedIsInvestment: savedIsInvestment}
	}
}

// transferLegForAccount returns the ID of whichever regular transfer leg belongs
// to acct, or NilID if neither does (e.g. the transfer was entered from a view
// that isn't one of its two accounts' registers).
func transferLegForAccount(acct types.ID, from, to *transaction.Transaction) types.ID {
	if from != nil && from.AccountID == acct {
		return from.ID
	}
	if to != nil && to.AccountID == acct {
		return to.ID
	}
	return types.NilID
}

// cashTransferLegForAccount returns the ID of whichever leg of an investment↔bank
// cash transfer belongs to acct, plus whether that leg is the investment-side
// transaction. Returns (NilID, false) when neither leg belongs to acct.
func cashTransferLegForAccount(acct types.ID, res *investment.CashTransferResult) (types.ID, bool) {
	if res == nil {
		return types.NilID, false
	}
	if res.RegularTransaction != nil && res.RegularTransaction.AccountID == acct {
		return res.RegularTransaction.ID, false
	}
	if res.InvestmentTransaction != nil && res.InvestmentTransaction.AccountID == acct {
		return res.InvestmentTransaction.ID, true
	}
	return types.NilID, false
}

// submitEditTransferDialog validates the edit-mode transfer dialog and
// dispatches the appropriate Update path. Edit-mode field layout is
// Amount(0), Date(1), Memo(2), Status(3). For bank↔bank pairs the existing
// EditTransferCommand fires; for any pair with an investment leg the
// investment service's UpdateTransferCash handles both same-type and
// cross-type combinations.
func (a *App) submitEditTransferDialog() (tea.Model, tea.Cmd) {
	regularPair := a.transferDialogData.existing
	invEdit := a.transferDialogData.existingInvestment
	if regularPair == nil && invEdit == nil {
		return a, nil
	}
	if regularPair != nil && regularPair.FromTransaction == nil {
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
	} else {
		var ids []types.ID
		switch {
		case invEdit != nil:
			ids = append(ids, invEdit.fromAccountID, invEdit.toAccountID)
		case regularPair != nil:
			if regularPair.FromTransaction != nil {
				ids = append(ids, regularPair.FromTransaction.AccountID)
			}
			if regularPair.ToTransaction != nil {
				ids = append(ids, regularPair.ToTransaction.AccountID)
			}
		}
		if msg := transferOpeningDateError(a.transferDialogData.accounts, date, ids...); msg != "" {
			fields[1].Error = msg
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	memo := strings.TrimSpace(fields[2].Value)

	status := transaction.StatusUncleared
	if fields[3].SelectedIndex == 1 {
		status = transaction.StatusCleared
	}

	if invEdit != nil {
		fromType := accountTypeByID(a.transferDialogData.accounts, invEdit.fromAccountID)
		a.closeTransferDialog()
		return a, a.dispatchInvestmentEditTransfer(invEdit, fromType, date, amount, memo, status)
	}

	transferID := regularPair.FromTransaction.TransferID.ID
	// Keep the cursor on the edited row. An edit can change the date and so
	// re-sort the leg; selecting by ID lands on it wherever it moves. The leg's
	// ID is unchanged by an edit, so the existing pair already names it.
	savedID := transferLegForAccount(a.currentRegisterAccountID(), regularPair.FromTransaction, regularPair.ToTransaction)
	a.closeTransferDialog()
	return a, func() tea.Msg {
		if a.transactionSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("transaction service not available")}
		}
		// Preserve the existing category (from the canonical outflow leg). The
		// edit dialog gains a category picker in a later phase; until then an
		// edit must not clobber a category applied by other means (e.g. a
		// legacy transfer-link pair).
		cmd := undo.NewEditTransferCommand(a.transactionSvc, transferID, date, amount, memo, status, regularPair.FromTransaction.CategoryID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to update transfer: %w", err)}
		}
		return transferDialogSavedMsg{savedDate: date, savedID: savedID}
	}
}

// dispatchInvestmentEditTransfer routes the edit-mode submit to
// investment.Service.UpdateTransferCash with parameters derived from the
// dialog's From/To orientation. Direction is "out" when From is the
// investment account, "in" when To is — matching UpdateTransferCash's
// "in" = cash arrives at investmentAccountID convention.
func (a *App) dispatchInvestmentEditTransfer(edit *investmentTransferEdit, fromType account.Type, date types.Date, amount types.Money, memo string, status transaction.Status) tea.Cmd {
	var investmentAccountID, otherAccountID types.ID
	var direction string
	if fromType.IsInvestmentType() {
		investmentAccountID = edit.fromAccountID
		otherAccountID = edit.toAccountID
		direction = "out"
	} else {
		investmentAccountID = edit.toAccountID
		otherAccountID = edit.fromAccountID
		direction = "in"
	}

	investmentTxnID := edit.investmentTxnID

	// Keep the cursor on the edited row when viewing the investment side. The
	// investment leg's ID is unchanged by the edit, so we can select it after
	// the reload. Only do so when the investment account is the one on screen —
	// the regular leg's ID isn't available on this path, so an edit viewed from
	// the bank register simply leaves the cursor put rather than stashing an ID
	// that would later jump the wrong register.
	var savedID types.ID
	if a.currentRegisterAccountID() == investmentAccountID {
		savedID = investmentTxnID
	}

	return func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}
		if _, err := a.investmentSvc.UpdateTransferCash(
			investmentTxnID,
			investmentAccountID,
			otherAccountID,
			date,
			amount,
			memo,
			types.NullableID{}, // category picker on this path arrives in a later phase
			direction,
			status,
		); err != nil {
			return errMsg{err: fmt.Errorf("failed to update transfer: %w", err)}
		}
		return transferDialogSavedMsg{savedDate: date, savedID: savedID, savedIsInvestment: !savedID.IsNil()}
	}
}
