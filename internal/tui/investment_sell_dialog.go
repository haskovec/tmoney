package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
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
	// savedID is the ID of the saved transaction so the investment register
	// can move the cursor onto its row after reload.
	savedID types.ID
}

// buildLotLabel creates a display label for a lot allocation field.
func buildLotLabel(lot *investment.Lot) string {
	return fmt.Sprintf("Lot: %s - %s shares @ $%.2f",
		lot.PurchaseDate.Time().Format("01/02/2006"),
		lot.Shares.String(),
		lot.CostPerShare.Float64(),
	)
}

// buildSellDialog creates a dialog.Dialog for entering a new sell transaction.
// If lots is non-nil, lot allocation fields are added after the Shares field.
// dialog.Field order: Date(0), Security(1), Shares(2), [lots...], Total,
// Price/Share, Commission, Memo. Total leads Price/Share because the
// common workflow is to type the total (from a brokerage statement) and
// let Price/Share auto-compute.
func buildSellDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID, lots []*investment.Lot) *dialog.Dialog {
	d := dialog.NewDialog("Sell Securities")
	d.SetWidth(70)

	// Date (index 0)
	dateVal := ""
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Date", dateVal)
	f.Required = true

	// Security selector (index 1)
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

	// Shares
	sharesVal := ""
	if editTxn != nil && editTxn.Shares.Valid && !editTxn.Shares.Quantity.IsZero() {
		sharesVal = editTxn.Shares.Quantity.String()
	}
	f = d.AddNumericField("Shares", sharesVal, "10", 12)
	f.Required = true

	// Lot allocation fields (only for lot-tracking accounts)
	for _, lot := range lots {
		d.AddNumericField(buildLotLabel(lot), "", "0", 12)
	}

	// Total Amount
	totalVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		totalVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	d.AddNumericField("Total", totalVal, "1850.00", 12)

	// Price Per Share
	priceVal := ""
	if editTxn != nil && editTxn.PricePerShare.Valid {
		priceVal = fmt.Sprintf("%.2f", editTxn.PricePerShare.Money.Float64())
	}
	d.AddNumericField("Price/Share", priceVal, "185.00", 12)

	// Commission
	commVal := ""
	if editTxn != nil && editTxn.Commission.Valid && !editTxn.Commission.Money.IsZero() {
		commVal = fmt.Sprintf("%.2f", editTxn.Commission.Money.Float64())
	}
	d.AddNumericField("Commission", commVal, "0.00", 12)

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
	case dialog.DialogActionSubmit:
		return a.submitSellDialog()
	case dialog.DialogActionCancel:
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
	// Expected fields: Date(0), Security(1), Shares(2), [lots...], Total, Price/Share, Commission, Memo
	expectedFields := 7 + numLots
	if len(fields) < expectedFields {
		return a, nil
	}

	a.sellDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	} else if msg := a.investmentDialogOpeningDateError(date); msg != "" {
		fields[0].Error = msg
		hasErrors = true
	}

	// Security (index 1)
	if len(a.sellDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.sellDialogSecurityIDs) {
		securityID = a.sellDialogSecurityIDs[secIdx]
	} else {
		fields[1].Error = "Select a security"
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

	// Total (index 3+numLots)
	totalIdx := 3 + numLots
	totalAmount, err := parseOptionalMoneyInput(fields[totalIdx].Value)
	if err != nil {
		fields[totalIdx].Error = "Invalid amount"
		hasErrors = true
	}

	// Price/Share (index 4+numLots)
	priceIdx := 4 + numLots
	pricePerShare, err := parseOptionalMoneyInput(fields[priceIdx].Value)
	if err != nil {
		fields[priceIdx].Error = "Invalid price"
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

	// Lot-tracked NEW sell with no per-lot fields shown: auto-allocate FIFO
	// across the security's open lots, oldest first. The dialog only renders
	// per-lot allocation fields when editing an existing sell; for a new sell
	// we default to FIFO (matching the lot-backfill default) so the sale isn't
	// blocked with "lot allocations required".
	if !hasErrors && numLots == 0 && a.lotRepo != nil &&
		a.investmentRegister != nil && a.investmentRegister.account != nil &&
		a.investmentRegister.account.TrackLots {
		openLots, lerr := a.lotRepo.ListByAccountAndSecurity(a.investmentRegister.account.ID, securityID, false)
		if lerr != nil {
			fields[2].Error = "Could not load lots for allocation"
			hasErrors = true
		} else if allocs, aerr := fifoAllocate(openLots, shares); aerr != nil {
			fields[2].Error = aerr.Error()
			hasErrors = true
		} else {
			lotAllocations = allocs
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

		var saved *investment.Transaction
		var err error
		if editTxnID != types.NilID {
			saved, err = a.investmentEditSvc.UpdateSell(
				editTxnID,
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
		} else {
			saved, err = a.investmentSvc.Sell(
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
		}
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to save sell transaction: %w", err)}
		}

		return sellDialogSavedMsg{savedDate: date, savedID: saved.ID}
	}
}

// fifoAllocate consumes `shares` across the given open lots oldest-first (the
// caller passes them in purchase-date order, which is FIFO) and returns the
// per-lot sell allocations. It is used to auto-allocate a new sell on a
// lot-tracked account, where the dialog renders no per-lot fields. It returns
// an error if the open lots don't cover the requested shares.
func fifoAllocate(openLots []*investment.Lot, shares types.Quantity) ([]investment.SellLotAllocation, error) {
	var allocations []investment.SellLotAllocation
	remaining := shares
	for _, lot := range openLots {
		if !remaining.IsPositive() {
			break
		}
		take := lot.Shares
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		allocations = append(allocations, investment.SellLotAllocation{LotID: lot.ID, Shares: take})
		remaining = remaining.Sub(take)
	}
	if remaining.IsPositive() {
		return nil, fmt.Errorf("only %s shares available to sell", shares.Sub(remaining).String())
	}
	return allocations, nil
}
