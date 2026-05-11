package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// transferSharesDialogData holds the loaded data needed for the share transfer dialog.
type transferSharesDialogData struct {
	securities         []*security.Security
	investmentAccounts []*account.Account
	investmentIDs      []types.ID // parallel to account dropdown options (investment accounts only)
	lots               []*investment.Lot
}

// transferSharesDialogDataMsg is sent when share transfer dialog data has been loaded.
type transferSharesDialogDataMsg struct {
	data *transferSharesDialogData
}

// transferSharesDialogSavedMsg is sent when a share transfer has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type transferSharesDialogSavedMsg struct {
	savedDate types.Date
}

// buildInvestmentAccountOptions builds parallel display name and ID slices
// for investment accounts only (excluding the current account).
func buildInvestmentAccountOptions(accounts []*account.Account, excludeID types.ID) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))

	for _, acct := range accounts {
		if acct.Type != account.TypeInvestment {
			continue
		}
		if acct.ID == excludeID {
			continue
		}
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}

	return options, ids
}

// buildTransferSharesDialog creates a Dialog for transferring shares between investment accounts.
// Field order: Date(0), Security(1), To Account(2), Shares(3), [lots...], Memo.
func buildTransferSharesDialog(
	accountOptions []string,
	securityOptions []string,
	editTxn *investment.Transaction,
	accountIDs []types.ID,
	securityIDs []types.ID,
	lots []*investment.Lot,
) *Dialog {
	d := NewDialog("Transfer Shares")
	d.SetWidth(70)

	// Date (index 0)
	dateVal := ""
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Date", dateVal)
	f.Required = true

	// Security selector (index 1)
	selectedSecIdx := 0
	if editTxn != nil && editTxn.SecurityID.Valid {
		for i, id := range securityIDs {
			if id == editTxn.SecurityID.ID {
				selectedSecIdx = i
				break
			}
		}
	}
	d.AddComboField("Security", securityOptions, selectedSecIdx)

	// Destination account selector (index 2)
	selectedAcctIdx := 0
	if editTxn != nil && editTxn.TransferAccountID.Valid {
		for i, id := range accountIDs {
			if id == editTxn.TransferAccountID.ID {
				selectedAcctIdx = i
				break
			}
		}
	}
	d.AddSelectField("To Account", accountOptions, selectedAcctIdx)

	// Shares (index 3)
	sharesVal := ""
	if editTxn != nil && editTxn.Shares.Valid && !editTxn.Shares.Quantity.IsZero() {
		sharesVal = editTxn.Shares.Quantity.String()
	}
	f = d.AddTextField("Shares", sharesVal, "10", 12)
	f.Required = true

	// Lot allocation fields (only for lot-tracking source accounts)
	for _, lot := range lots {
		d.AddTextField(buildLotLabel(lot), "", "0", 12)
	}

	// Memo
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadTransferSharesDialogData returns a command that loads securities and investment accounts
// for the share transfer dialog.
func (a *App) loadTransferSharesDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &transferSharesDialogData{}

		// Load securities
		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		// Load investment accounts (excluding current)
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			excludeID := types.NilID
			if a.investmentRegister != nil && a.investmentRegister.account != nil {
				excludeID = a.investmentRegister.account.ID
			}
			data.investmentAccounts = accounts
			_, ids := buildInvestmentAccountOptions(accounts, excludeID)
			data.investmentIDs = ids
		}

		// Load lots if source account is lot-tracking
		if a.investmentRegister != nil && a.investmentRegister.account != nil &&
			a.investmentRegister.account.TrackLots && a.lotRepo != nil {

			acctID := a.investmentRegister.account.ID

			if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
				editTxn, err := a.investmentRepo.GetByID(a.investmentEditTxnID)
				if err == nil && editTxn.SecurityID.Valid {
					lots, err := a.lotRepo.ListByAccountAndSecurity(acctID, editTxn.SecurityID.ID, false)
					if err == nil {
						data.lots = lots
					}
				}
			}
		}

		return transferSharesDialogDataMsg{data: data}
	}
}

// closeTransferSharesDialog clears the share transfer dialog state.
func (a *App) closeTransferSharesDialog() {
	a.transferSharesDialog = nil
	a.transferSharesDialogData = nil
	a.transferSharesDialogAccountIDs = nil
	a.transferSharesDialogSecurityIDs = nil
	a.transferSharesDialogLots = nil
}

// handleTransferSharesDialogKey routes key events to the share transfer dialog.
func (a *App) handleTransferSharesDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.transferSharesDialog == nil {
		return a, nil
	}

	action := a.transferSharesDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitTransferSharesDialog()
	case DialogActionCancel:
		a.closeTransferSharesDialog()
		return a, nil
	}

	return a, nil
}

// submitTransferSharesDialog parses dialog fields, validates, and saves the share transfer.
func (a *App) submitTransferSharesDialog() (tea.Model, tea.Cmd) {
	if a.transferSharesDialog == nil || a.transferSharesDialogData == nil {
		return a, nil
	}

	fields := a.transferSharesDialog.Fields()
	numLots := len(a.transferSharesDialogLots)
	// Expected: Date(0), Security(1), To Account(2), Shares(3), [lots...], Memo
	expectedFields := 5 + numLots
	if len(fields) < expectedFields {
		return a, nil
	}

	a.transferSharesDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Security (index 1)
	if len(a.transferSharesDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.transferSharesDialogSecurityIDs) {
		securityID = a.transferSharesDialogSecurityIDs[secIdx]
	} else {
		fields[1].Error = "Select a security"
		hasErrors = true
	}

	// Destination account (index 2)
	if len(a.transferSharesDialogAccountIDs) == 0 {
		fields[2].Error = "No investment accounts available"
		hasErrors = true
	}
	destIdx := fields[2].SelectedIndex
	var destAccountID types.ID
	if destIdx >= 0 && destIdx < len(a.transferSharesDialogAccountIDs) {
		destAccountID = a.transferSharesDialogAccountIDs[destIdx]
	} else {
		fields[2].Error = "Select a destination account"
		hasErrors = true
	}

	// Shares (index 3)
	shares, err := parseSharesInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Shares must be positive"
		hasErrors = true
	}

	// Lot allocations (indices 4 through 4+numLots-1)
	var lotAllocations []investment.SellLotAllocation
	if numLots > 0 {
		totalAllocated := types.ZeroQuantity
		for i := range numLots {
			fieldIdx := 4 + i
			lotField := fields[fieldIdx]
			lotVal := strings.TrimSpace(lotField.Value)

			if lotVal == "" || lotVal == "0" {
				continue
			}

			q, qErr := types.NewQuantity(lotVal)
			if qErr != nil {
				fields[fieldIdx].Error = "Invalid shares amount"
				hasErrors = true
				continue
			}
			if q.IsNegative() {
				fields[fieldIdx].Error = "Shares must be positive"
				hasErrors = true
				continue
			}
			if q.IsZero() {
				continue
			}

			lot := a.transferSharesDialogLots[i]
			if lot.Shares.Cmp(q) < 0 {
				fields[fieldIdx].Error = fmt.Sprintf("Only %s shares available", lot.Shares.String())
				hasErrors = true
				continue
			}

			totalAllocated = totalAllocated.Add(q)
			lotAllocations = append(lotAllocations, investment.SellLotAllocation{
				LotID:  lot.ID,
				Shares: q,
			})
		}

		if !hasErrors && !shares.IsZero() && totalAllocated.Cmp(shares) != 0 {
			fields[3].Error = fmt.Sprintf("Lot allocations total %s, need %s", totalAllocated.String(), shares.String())
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 4+numLots)
	memoIdx := 4 + numLots
	memo := strings.TrimSpace(fields[memoIdx].Value)

	// Get source account ID
	sourceAccountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		sourceAccountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID

	// Close dialog before async save
	a.closeTransferSharesDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var txnErr error
		if editTxnID != types.NilID {
			_, txnErr = a.investmentSvc.UpdateTransferShares(
				editTxnID,
				sourceAccountID,
				destAccountID,
				date,
				securityID,
				shares,
				memo,
				lotAllocations,
			)
		} else {
			_, txnErr = a.investmentSvc.TransferShares(
				sourceAccountID,
				destAccountID,
				securityID,
				date,
				shares,
				memo,
				lotAllocations,
			)
		}
		if txnErr != nil {
			return errMsg{err: fmt.Errorf("failed to transfer shares: %w", txnErr)}
		}

		return transferSharesDialogSavedMsg{savedDate: date}
	}
}
