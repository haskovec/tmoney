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

// buyDialogData holds the loaded data needed for the buy transaction dialog.
type buyDialogData struct {
	securities []*security.Security
}

// buyDialogDataMsg is sent when buy dialog data has been loaded.
type buyDialogDataMsg struct {
	data *buyDialogData
}

// buyDialogSavedMsg is sent when a buy transaction has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type buyDialogSavedMsg struct {
	savedDate types.Date
	// savedID is the ID of the saved transaction so the investment register
	// can move the cursor onto its row after reload.
	savedID types.ID
}

// buildSecurityOptions builds parallel display name and ID slices for the security selector.
// Non-hidden securities are listed as "TICKER - Name", sorted by ticker.
func buildSecurityOptions(securities []*security.Security) ([]string, []types.ID) {
	type secEntry struct {
		display string
		id      types.ID
	}
	var entries []secEntry

	for _, sec := range securities {
		if sec.Hidden {
			continue
		}
		entries = append(entries, secEntry{
			display: fmt.Sprintf("%s - %s", sec.Ticker, sec.Name),
			id:      sec.ID,
		})
	}

	// Sort by display name (ticker first)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].display > entries[j].display {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	options := make([]string, len(entries))
	ids := make([]types.ID, len(entries))
	for i, e := range entries {
		options[i] = e.display
		ids[i] = e.id
	}

	return options, ids
}

// buildBuyDialog creates a dialog.Dialog for entering a new buy transaction.
// dialog.Field order: Date(0), Security(1), Shares(2), Total(3), Price/Share(4),
// Commission(5), Memo(6) — Date leads for consistency with the regular
// transaction dialog and so batch-entry on the sticky date can tab
// straight through to the next field. Total leads Price/Share because
// the common workflow is to type the total (from a brokerage statement)
// and let Price/Share auto-compute.
func buildBuyDialog(securityOptions []string, editTxn *investment.Transaction, securityIDs []types.ID) *dialog.Dialog {
	d := dialog.NewDialog("Buy Securities")
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

	// Total Amount
	totalVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		// Total is stored as negative for buys; display as positive
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

// loadBuyDialogData returns a command that loads securities for the buy dialog.
func (a *App) loadBuyDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &buyDialogData{}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		return buyDialogDataMsg{data: data}
	}
}

// closeBuyDialog clears the buy dialog state.
func (a *App) closeBuyDialog() {
	a.buyDialog = nil
	a.buyDialogData = nil
	a.buyDialogSecurityIDs = nil
}

// handleBuyDialogKey routes key events to the buy dialog.
func (a *App) handleBuyDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.buyDialog == nil {
		return a, nil
	}

	action := a.buyDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitBuyDialog()
	case dialog.DialogActionCancel:
		a.closeBuyDialog()
		return a, nil
	}

	return a, nil
}

// parseSharesInput parses a shares/quantity string.
func parseSharesInput(input string) (types.Quantity, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return types.ZeroQuantity, fmt.Errorf("shares is required")
	}
	q, err := types.NewQuantity(s)
	if err != nil {
		return types.ZeroQuantity, fmt.Errorf("invalid shares: %w", err)
	}
	if q.IsZero() || q.IsNegative() {
		return types.ZeroQuantity, fmt.Errorf("shares must be positive")
	}
	return q, nil
}

// parseOptionalMoneyInput parses an optional money string. Returns nil if empty.
func parseOptionalMoneyInput(input string) (*types.Money, error) {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "$")
	if s == "" {
		return nil, nil
	}
	m, err := types.NewMoney(s)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	return &m, nil
}

// submitBuyDialog parses dialog fields, validates, and saves the buy transaction.
func (a *App) submitBuyDialog() (tea.Model, tea.Cmd) {
	if a.buyDialog == nil || a.buyDialogData == nil {
		return a, nil
	}

	fields := a.buyDialog.Fields()
	if len(fields) < 7 {
		return a, nil
	}

	a.buyDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Security (index 1)
	if len(a.buyDialogSecurityIDs) == 0 {
		fields[1].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[1].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.buyDialogSecurityIDs) {
		securityID = a.buyDialogSecurityIDs[secIdx]
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

	// Total (index 3)
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

	// Need at least one of price or total
	if pricePerShare == nil && totalAmount == nil {
		fields[3].Error = "Enter price or total"
		fields[4].Error = "Enter price or total"
		hasErrors = true
	}

	// Commission (index 5)
	commission := types.ZeroMoney
	commStr := strings.TrimSpace(fields[5].Value)
	commStr = strings.TrimPrefix(commStr, "$")
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

	// Memo (index 6)
	memo := strings.TrimSpace(fields[6].Value)

	// Get account ID
	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID

	// Close dialog before async save
	a.closeBuyDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var saved *investment.Transaction
		var err error
		if editTxnID != types.NilID {
			saved, err = a.investmentSvc.UpdateBuy(
				editTxnID,
				accountID,
				securityID,
				date,
				shares,
				totalAmount,
				pricePerShare,
				commission,
				memo,
			)
		} else {
			saved, err = a.investmentSvc.Buy(
				accountID,
				securityID,
				date,
				shares,
				totalAmount,
				pricePerShare,
				commission,
				memo,
			)
		}
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to save buy transaction: %w", err)}
		}

		return buyDialogSavedMsg{savedDate: date, savedID: saved.ID}
	}
}
