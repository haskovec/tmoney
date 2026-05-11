package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// sellDialogData holds the loaded data needed for the sell transaction dialog.
type sellDialogData struct {
	securities []*security.Security
	lots       []*investment.Lot // nil for non-lot-tracking accounts
}

// sellDialogDataMsg is sent when sell dialog data has been loaded.
type sellDialogDataMsg struct {
	data *sellDialogData
}

// sellDialogSavedMsg is sent when a sell transaction has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type sellDialogSavedMsg struct {
	savedDate types.Date
}

// buildLotLabel creates a display label for a lot allocation field.
func buildLotLabel(lot *investment.Lot) string {
	return fmt.Sprintf("Lot: %s - %s shares @ $%.2f",
		lot.PurchaseDate.Time().Format("01/02/2006"),
		lot.Shares.String(),
		lot.CostPerShare.Float64(),
	)
}

// buildSellDialog creates a Dialog for entering a new sell transaction.
// If lots is non-nil, lot allocation fields are added after the Shares field.
func buildSellDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID, lots []*investment.Lot) *Dialog {
	d := NewDialog("Sell Securities")
	d.SetWidth(50)

	// Security selector
	selectedIdx := 0
	if editTxn != nil && editTxn.SecurityID.Valid {
		for i, id := range securityIDs {
			if id == editTxn.SecurityID.ID {
				selectedIdx = i
				break
			}
		}
	}
	d.AddComboField("Security", securityOptions, selectedIdx)

	// Date
	dateVal := ""
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Date", dateVal)
	f.Required = true

	// Shares
	sharesVal := ""
	if editTxn != nil && editTxn.Shares.Valid && !editTxn.Shares.Quantity.IsZero() {
		sharesVal = editTxn.Shares.Quantity.String()
	}
	f = d.AddTextField("Shares", sharesVal, "10", 12)
	f.Required = true

	// Lot allocation fields (only for lot-tracking accounts)
	for _, lot := range lots {
		d.AddTextField(buildLotLabel(lot), "", "0", 12)
	}

	// Price Per Share
	priceVal := ""
	if editTxn != nil && editTxn.PricePerShare.Valid {
		priceVal = fmt.Sprintf("%.2f", editTxn.PricePerShare.Money.Float64())
	}
	d.AddTextField("Price/Share", priceVal, "185.00", 12)

	// Total Amount
	totalVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		totalVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	d.AddTextField("Total", totalVal, "1850.00", 12)

	// Commission
	commVal := ""
	if editTxn != nil && editTxn.Commission.Valid && !editTxn.Commission.Money.IsZero() {
		commVal = fmt.Sprintf("%.2f", editTxn.Commission.Money.Float64())
	}
	d.AddTextField("Commission", commVal, "0.00", 12)

	// Memo
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadSellDialogData returns a command that loads securities and lots for the sell dialog.
func (a *App) loadSellDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &sellDialogData{}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		// Load lots if the account is lot-tracking
		if a.investmentRegister != nil && a.investmentRegister.account != nil &&
			a.investmentRegister.account.TrackLots && a.lotRepo != nil {

			acctID := a.investmentRegister.account.ID

			// If editing, get the security from the existing transaction
			// For new transactions, lots will be loaded after security selection
			// For now, if editing, load lots for that security
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

		return sellDialogDataMsg{data: data}
	}
}

// closeSellDialog clears the sell dialog state.
func (a *App) closeSellDialog() {
	a.sellDialog = nil
	a.sellDialogData = nil
	a.sellDialogSecurityIDs = nil
	a.sellDialogLots = nil
}

// handleSellDialogKey routes key events to the sell dialog.
func (a *App) handleSellDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.sellDialog == nil {
		return a, nil
	}

	action := a.sellDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitSellDialog()
	case DialogActionCancel:
		a.closeSellDialog()
		return a, nil
	}

	return a, nil
}

// submitSellDialog parses dialog fields, validates, and saves the sell transaction.
func (a *App) submitSellDialog() (tea.Model, tea.Cmd) {
	if a.sellDialog == nil || a.sellDialogData == nil {
		return a, nil
	}

	fields := a.sellDialog.Fields()
	numLots := len(a.sellDialogLots)
	// Expected fields: Security(0), Date(1), Shares(2), [lots...], Price/Share, Total, Commission, Memo
	expectedFields := 7 + numLots
	if len(fields) < expectedFields {
		return a, nil
	}

	a.sellDialog.ClearErrors()
	hasErrors := false

	// Security (index 0)
	if len(a.sellDialogSecurityIDs) == 0 {
		fields[0].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[0].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.sellDialogSecurityIDs) {
		securityID = a.sellDialogSecurityIDs[secIdx]
	} else {
		fields[0].Error = "Select a security"
		hasErrors = true
	}

	// Date (index 1)
	date, err := parseDateInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Shares (index 2)
	shares, err := parseSharesInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Shares must be positive"
		hasErrors = true
	}

	// Lot allocations (indices 3 through 3+numLots-1)
	var lotAllocations []investment.SellLotAllocation
	if numLots > 0 {
		totalAllocated := types.ZeroQuantity
		for i := range numLots {
			fieldIdx := 3 + i
			lotField := fields[fieldIdx]
			lotVal := strings.TrimSpace(lotField.Value)

			if lotVal == "" || lotVal == "0" {
				continue
			}

			q, err := types.NewQuantity(lotVal)
			if err != nil {
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

			// Check lot has enough shares
			lot := a.sellDialogLots[i]
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

		// Validate total allocation matches sell shares (only if no other errors)
		if !hasErrors && !shares.IsZero() && totalAllocated.Cmp(shares) != 0 {
			fields[2].Error = fmt.Sprintf("Lot allocations total %s, need %s", totalAllocated.String(), shares.String())
			hasErrors = true
		}
	}

	// Price/Share (index 3+numLots)
	priceIdx := 3 + numLots
	pricePerShare, err := parseOptionalMoneyInput(fields[priceIdx].Value)
	if err != nil {
		fields[priceIdx].Error = "Invalid price"
		hasErrors = true
	}

	// Total (index 4+numLots)
	totalIdx := 4 + numLots
	totalAmount, err := parseOptionalMoneyInput(fields[totalIdx].Value)
	if err != nil {
		fields[totalIdx].Error = "Invalid amount"
		hasErrors = true
	}

	// Need at least one of price or total
	if pricePerShare == nil && totalAmount == nil {
		fields[priceIdx].Error = "Enter price or total"
		fields[totalIdx].Error = "Enter price or total"
		hasErrors = true
	}

	// Commission (index 5+numLots)
	commIdx := 5 + numLots
	commission := types.ZeroMoney
	commStr := strings.TrimSpace(fields[commIdx].Value)
	commStr = strings.TrimPrefix(commStr, "$")
	if commStr != "" {
		commission, err = types.NewMoney(commStr)
		if err != nil {
			fields[commIdx].Error = "Invalid commission"
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 6+numLots)
	memoIdx := 6 + numLots
	memo := strings.TrimSpace(fields[memoIdx].Value)

	// Get account ID
	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID

	// Close dialog before async save
	a.closeSellDialog()

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

		_, err := a.investmentSvc.Sell(
			accountID,
			securityID,
			date,
			shares,
			totalAmount,
			pricePerShare,
			commission,
			memo,
			lotAllocations,
		)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to create sell transaction: %w", err)}
		}

		return sellDialogSavedMsg{savedDate: date}
	}
}
