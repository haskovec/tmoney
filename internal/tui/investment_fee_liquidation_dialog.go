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

// feeLiquidationDialogData holds the loaded data for the fee-via-liquidation dialog.
type feeLiquidationDialogData struct {
	securities []*security.Security
}

// feeLiquidationDialogDataMsg is sent when fee-liquidation dialog data has loaded.
type feeLiquidationDialogDataMsg struct {
	data *feeLiquidationDialogData
}

// feeLiquidationDialogSavedMsg is sent when a fee-liquidation transaction is saved.
type feeLiquidationDialogSavedMsg struct {
	savedDate types.Date
	savedID   types.ID
}

// buildFeeLiquidationDialog creates the dialog for a fee-via-liquidation. Field
// order mirrors the Sell dialog minus the per-lot fields (lot allocation is
// FIFO-auto for lot-tracked accounts): Date(0), Security(1), Shares(2),
// Total(3), Price/Share(4), Commission(5), Memo(6). Total leads Price/Share
// because the statement quotes the fee total; Price/Share auto-computes.
func buildFeeLiquidationDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID) *dialog.Dialog {
	d := dialog.NewDialog("Fee via Liquidation")
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

	// Shares (index 2)
	sharesVal := ""
	if editTxn != nil && editTxn.Shares.Valid && !editTxn.Shares.Quantity.IsZero() {
		sharesVal = editTxn.Shares.Quantity.String()
	}
	f = d.AddNumericField("Shares", sharesVal, "0.123", 12)
	f.Required = true

	// Total Amount — the fee (index 3)
	totalVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		totalVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	d.AddNumericField("Total", totalVal, "5.00", 12)

	// Price Per Share (index 4)
	priceVal := ""
	if editTxn != nil && editTxn.PricePerShare.Valid {
		priceVal = fmt.Sprintf("%.2f", editTxn.PricePerShare.Money.Float64())
	}
	d.AddNumericField("Price/Share", priceVal, "40.65", 12)

	// Commission (index 5)
	commVal := ""
	if editTxn != nil && editTxn.Commission.Valid && !editTxn.Commission.Money.IsZero() {
		commVal = fmt.Sprintf("%.2f", editTxn.Commission.Money.Float64())
	}
	d.AddNumericField("Commission", commVal, "0.00", 12)

	// Memo (index 6)
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadFeeLiquidationDialogData returns a command that loads securities for the dialog.
func (a *App) loadFeeLiquidationDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &feeLiquidationDialogData{}
		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}
		return feeLiquidationDialogDataMsg{data: data}
	}
}

// closeFeeLiquidationDialog clears the fee-liquidation dialog state.
func (a *App) closeFeeLiquidationDialog() {
	a.feeLiquidationDialog = nil
	a.feeLiquidationDialogData = nil
	a.feeLiquidationDialogSecurityIDs = nil
}

// handleFeeLiquidationDialogKey routes key events to the fee-liquidation dialog.
func (a *App) handleFeeLiquidationDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.feeLiquidationDialog == nil {
		return a, nil
	}
	return a.feeLiquidationDialogAction(a.feeLiquidationDialog.HandleKey(msg))
}

// feeLiquidationDialogAction dispatches a DialogAction for the fee liquidation dialog, from either input path.
func (a *App) feeLiquidationDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitFeeLiquidationDialog()
	case dialog.DialogActionCancel:
		a.closeFeeLiquidationDialog()
		return a, nil
	}
	return a, nil
}

// submitFeeLiquidationDialog parses fields, validates, and saves the fee-via-liquidation.
func (a *App) submitFeeLiquidationDialog() (tea.Model, tea.Cmd) {
	if a.feeLiquidationDialog == nil || a.feeLiquidationDialogData == nil {
		return a, nil
	}
	fields := a.feeLiquidationDialog.Fields()
	if len(fields) < 7 {
		return a, nil
	}
	a.feeLiquidationDialog.ClearErrors()
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
	if len(a.feeLiquidationDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.feeLiquidationDialogSecurityIDs) {
		securityID = a.feeLiquidationDialogSecurityIDs[secIdx]
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

	// Total — the fee (index 3)
	totalAmount, err := parseOptionalMoneyInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid amount"
		hasErrors = true
	}

	// Price/Share (index 4)
	pricePerShare, err := parseOptionalMoneyInput(fields[4].Value)
	if err != nil {
		fields[4].Error = "Invalid price"
		hasErrors = true
	}
	if pricePerShare == nil && totalAmount == nil {
		fields[4].Error = "Enter price or total"
		fields[3].Error = "Enter price or total"
		hasErrors = true
	}

	// Commission (index 5)
	commission := types.ZeroMoney
	commStr := strings.TrimPrefix(strings.TrimSpace(fields[5].Value), "$")
	if commStr != "" {
		commission, err = types.NewMoney(commStr)
		if err != nil {
			fields[5].Error = "Invalid commission"
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	// Lot allocation is left to the service: on a lot-tracked account it
	// auto-allocates FIFO against the (post-reverse, for edits) open lots, and
	// on a non-lot account it uses the average-cost path. Passing nil here keeps
	// the dialog free of stale pre-reverse lot snapshots.

	// Memo (index 6)
	memo := strings.TrimSpace(fields[6].Value)

	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}
	editTxnID := a.investmentEditTxnID

	a.closeFeeLiquidationDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}
		var saved *investment.Transaction
		var err error
		if editTxnID != types.NilID {
			saved, err = a.investmentEditSvc.UpdateFeeLiquidation(
				editTxnID, accountID, securityID, date, shares,
				totalAmount, pricePerShare, commission, memo, nil,
			)
		} else {
			saved, err = a.investmentSvc.FeeLiquidation(
				accountID, securityID, date, shares,
				totalAmount, pricePerShare, commission, memo, nil,
			)
		}
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to save fee liquidation transaction: %w", err)}
		}
		return feeLiquidationDialogSavedMsg{savedDate: date, savedID: saved.ID}
	}
}
