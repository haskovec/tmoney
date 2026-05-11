package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// dividendDialogData holds the loaded data needed for the dividend dialog.
type dividendDialogData struct {
	securities []*security.Security
}

// dividendDialogDataMsg is sent when dividend dialog data has been loaded.
type dividendDialogDataMsg struct {
	data *dividendDialogData
}

// dividendDialogSavedMsg is sent when a dividend transaction has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type dividendDialogSavedMsg struct {
	savedDate types.Date
}

// buildDividendDialog creates a Dialog for entering a cash dividend transaction.
// Field order: Date(0), Security(1), Amount(2), Memo(3).
func buildDividendDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID) *Dialog {
	d := NewDialog("Cash Dividend")
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

	// Amount
	amountVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		amountVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	f = d.AddTextField("Amount", amountVal, "50.00", 12)
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

// buildReinvestDividendDialog creates a Dialog for entering a reinvested dividend transaction.
// Field order: Date(0), Security(1), Shares(2), Price/Share(3), Total(4), Memo(5).
func buildReinvestDividendDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID) *Dialog {
	d := NewDialog("Reinvest Dividend")
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
	f = d.AddTextField("Shares", sharesVal, "10", 12)
	f.Required = true

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

	// Memo
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// loadDividendDialogData returns a command that loads securities for the dividend dialog.
func (a *App) loadDividendDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &dividendDialogData{}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		return dividendDialogDataMsg{data: data}
	}
}

// closeDividendDialog clears the dividend dialog state.
func (a *App) closeDividendDialog() {
	a.dividendDialog = nil
	a.dividendDialogData = nil
	a.dividendDialogSecurityIDs = nil
	a.dividendDialogReinvest = false
}

// handleDividendDialogKey routes key events to the dividend dialog.
func (a *App) handleDividendDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.dividendDialog == nil {
		return a, nil
	}

	action := a.dividendDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		if a.dividendDialogReinvest {
			return a.submitReinvestDividendDialog()
		}
		return a.submitDividendDialog()
	case DialogActionCancel:
		a.closeDividendDialog()
		return a, nil
	}

	return a, nil
}

// submitDividendDialog parses dialog fields, validates, and saves a cash dividend transaction.
func (a *App) submitDividendDialog() (tea.Model, tea.Cmd) {
	if a.dividendDialog == nil || a.dividendDialogData == nil {
		return a, nil
	}

	fields := a.dividendDialog.Fields()
	if len(fields) < 4 {
		return a, nil
	}

	a.dividendDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Security (index 1)
	if len(a.dividendDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.dividendDialogSecurityIDs) {
		securityID = a.dividendDialogSecurityIDs[secIdx]
	} else {
		fields[1].Error = "Select a security"
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

	if hasErrors {
		return a, nil
	}

	// Memo (index 3)
	memo := strings.TrimSpace(fields[3].Value)

	// Get account ID
	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID

	// Close dialog before async save
	a.closeDividendDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var err error
		if editTxnID != types.NilID {
			_, err = a.investmentSvc.UpdateDividend(editTxnID, accountID, securityID, date, *amount, memo)
		} else {
			_, err = a.investmentSvc.Dividend(accountID, securityID, date, *amount, memo)
		}
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to save dividend transaction: %w", err)}
		}

		return dividendDialogSavedMsg{savedDate: date}
	}
}

// submitReinvestDividendDialog parses dialog fields, validates, and saves a reinvest dividend transaction.
func (a *App) submitReinvestDividendDialog() (tea.Model, tea.Cmd) {
	if a.dividendDialog == nil || a.dividendDialogData == nil {
		return a, nil
	}

	fields := a.dividendDialog.Fields()
	if len(fields) < 6 {
		return a, nil
	}

	a.dividendDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Security (index 1)
	if len(a.dividendDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.dividendDialogSecurityIDs) {
		securityID = a.dividendDialogSecurityIDs[secIdx]
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

	// Price/Share (index 3)
	pricePerShare, err := parseOptionalMoneyInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid price"
		hasErrors = true
	}

	// Total (index 4)
	totalAmount, err := parseOptionalMoneyInput(fields[4].Value)
	if err != nil {
		fields[4].Error = "Invalid amount"
		hasErrors = true
	}

	// Need at least one of price or total
	if pricePerShare == nil && totalAmount == nil {
		fields[3].Error = "Enter price or total"
		fields[4].Error = "Enter price or total"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 5)
	memo := strings.TrimSpace(fields[5].Value)

	// Get account ID
	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID

	// Close dialog before async save
	a.closeDividendDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var err error
		if editTxnID != types.NilID {
			_, err = a.investmentSvc.UpdateReinvestDividend(
				editTxnID,
				accountID,
				securityID,
				date,
				shares,
				totalAmount,
				pricePerShare,
				memo,
			)
		} else {
			_, err = a.investmentSvc.ReinvestDividend(
				accountID,
				securityID,
				date,
				shares,
				totalAmount,
				pricePerShare,
				memo,
			)
		}
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to save reinvest dividend transaction: %w", err)}
		}

		return dividendDialogSavedMsg{savedDate: date}
	}
}
