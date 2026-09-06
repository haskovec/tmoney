package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// spinOffDialogData holds the loaded data for the spin-off dialog.
type spinOffDialogData struct {
	securities []*security.Security
}

// spinOffDialogDataMsg is sent when spin-off dialog data has been loaded.
type spinOffDialogDataMsg struct {
	data *spinOffDialogData
}

// spinOffDialogSavedMsg is sent when a spin-off has been executed. savedDate
// carries the executed date so the handler can update the session sticky date.
type spinOffDialogSavedMsg struct{ savedDate types.Date }

// buildSpinOffDialog creates a dialog.Dialog for executing a corporate spin-off.
// If preSelectedParentID is non-nil, the parent security selector is pre-selected.
func buildSpinOffDialog(securityOptions []string, securityIDs []types.ID, preSelectedParentID *types.ID) *dialog.Dialog {
	d := dialog.NewDialog("Spin-Off")
	d.SetWidth(60)

	// Parent Security selector
	parentIdx := 0
	if preSelectedParentID != nil {
		for i, id := range securityIDs {
			if id == *preSelectedParentID {
				parentIdx = i
				break
			}
		}
	}
	d.AddComboField("Parent Security", securityOptions, parentIdx)

	// Spin-Off Security selector
	d.AddComboField("Spin-Off Security", securityOptions, 0)

	// Date
	f := d.AddDateField("Date", "")
	f.Required = true

	// Resulting Shares — the spun-off share count from the statement. The
	// engine's share ratio is derived as resulting_shares / parent_shares_held.
	f = d.AddTextField("Resulting Shares", "", "227", 12)
	f.Required = true

	// Parent Allocation % (e.g., "80" means parent keeps 80% of cost basis)
	f = d.AddTextField("Parent Allocation %", "", "80", 10)
	f.Required = true

	// Spin-Off Price (initial price per share of spin-off security)
	f = d.AddTextField("Spin-Off Price", "", "25.00", 10)
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Execute", Primary: true},
		{Label: "Lookup", Action: dialog.DialogActionAlternate},
		{Label: "Cancel"},
	})

	d.SetVisible(true)
	return d
}

// loadSpinOffDialogData returns a command that loads securities for the spin-off dialog.
func (a *App) loadSpinOffDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &spinOffDialogData{}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		return spinOffDialogDataMsg{data: data}
	}
}

// closeSpinOffDialog clears the spin-off dialog state.
func (a *App) closeSpinOffDialog() {
	a.spinOffDialog = nil
	a.spinOffDialogData = nil
	a.spinOffDialogSecurityIDs = nil
}

// handleSpinOffDialogKey routes key events to the spin-off dialog.
func (a *App) handleSpinOffDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.spinOffDialog == nil {
		return a, nil
	}
	return a.spinOffDialogAction(a.spinOffDialog.HandleKey(msg))
}

// spinOffDialogAction dispatches a DialogAction for the spin off dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) spinOffDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitSpinOffDialog()
	case dialog.DialogActionAlternate:
		return a.startSpinOffPriceLookup()
	case dialog.DialogActionCancel:
		a.closeSpinOffDialog()
		return a, nil
	}

	return a, nil
}

// spinOffPriceLookupMsg carries the outcome of the spin-off dialog's Lookup.
type spinOffPriceLookupMsg struct {
	price    types.Money
	currency string
	err      error
}

// startSpinOffPriceLookup fetches the spin-off (child) security's close on the
// dialog's date and fills the Spin-Off Price field.
func (a *App) startSpinOffPriceLookup() (tea.Model, tea.Cmd) {
	if a.spinOffDialog == nil {
		return a, nil
	}
	fields := a.spinOffDialog.Fields()
	if len(fields) < 6 || len(a.spinOffDialogSecurityIDs) == 0 {
		return a, nil
	}
	idx := fields[1].SelectedIndex // Spin-Off Security
	if idx < 0 || idx >= len(a.spinOffDialogSecurityIDs) {
		a.spinOffDialog.SetErrorMsg("Select a spin-off security first")
		return a, nil
	}
	a.spinOffDialog.SetErrorMsg("")
	return a, a.spinOffPriceLookupCmd(a.spinOffDialogSecurityIDs[idx], strings.TrimSpace(fields[2].Value))
}

// spinOffPriceLookupCmd resolves the child ticker and fetches its as-of close.
func (a *App) spinOffPriceLookupCmd(childID types.ID, dateStr string) tea.Cmd {
	return func() tea.Msg {
		date, err := parseDateInput(dateStr)
		if err != nil {
			return spinOffPriceLookupMsg{err: fmt.Errorf("enter a valid date first (MM/DD/YYYY)")}
		}
		if a.securitySvc == nil || a.priceSvc == nil {
			return spinOffPriceLookupMsg{err: fmt.Errorf("price/security service not available")}
		}
		sec, err := a.securitySvc.GetByID(childID)
		if err != nil {
			return spinOffPriceLookupMsg{err: err}
		}
		provider, err := a.priceSvc.ProviderRegistry().Get(defaultRefreshProviderName)
		if err != nil {
			return spinOffPriceLookupMsg{err: err}
		}
		quote, err := provider.FetchQuoteOn(sec.Ticker, date)
		if err != nil {
			return spinOffPriceLookupMsg{err: err}
		}
		return spinOffPriceLookupMsg{price: quote.Price, currency: quote.Currency}
	}
}

// handleSpinOffPriceLookupResult fills the Spin-Off Price field from a completed
// lookup, or surfaces the error on the still-open dialog.
func (a *App) handleSpinOffPriceLookupResult(msg spinOffPriceLookupMsg) (tea.Model, tea.Cmd) {
	if a.spinOffDialog == nil {
		return a, nil
	}
	if msg.err != nil {
		a.spinOffDialog.SetErrorMsg("Lookup failed: " + msg.err.Error())
		return a, nil
	}
	fields := a.spinOffDialog.Fields()
	if len(fields) >= 6 {
		prefillField(fields[5], fmt.Sprintf("%.2f", msg.price.Float64()))
	}
	a.statusbar.AddNotification(fmt.Sprintf("Fetched %.2f %s", msg.price.Float64(), msg.currency), widget.NotificationInfo)
	return a, nil
}

// submitSpinOffDialog validates and executes the spin-off.
func (a *App) submitSpinOffDialog() (tea.Model, tea.Cmd) {
	if a.spinOffDialog == nil || a.spinOffDialogData == nil {
		return a, nil
	}

	fields := a.spinOffDialog.Fields()
	if len(fields) < 6 {
		return a, nil
	}

	a.spinOffDialog.ClearErrors()
	hasErrors := false

	// Parent Security (index 0)
	if len(a.spinOffDialogSecurityIDs) == 0 {
		fields[0].Error = "No securities available"
		hasErrors = true
	}
	parentIdx := fields[0].SelectedIndex
	var parentSecurityID types.ID
	if parentIdx >= 0 && parentIdx < len(a.spinOffDialogSecurityIDs) {
		parentSecurityID = a.spinOffDialogSecurityIDs[parentIdx]
	} else {
		fields[0].Error = "Select a parent security"
		hasErrors = true
	}

	// Spin-Off Security (index 1)
	spinOffIdx := fields[1].SelectedIndex
	var spinOffSecurityID types.ID
	if spinOffIdx >= 0 && spinOffIdx < len(a.spinOffDialogSecurityIDs) {
		spinOffSecurityID = a.spinOffDialogSecurityIDs[spinOffIdx]
	} else {
		fields[1].Error = "Select a spin-off security"
		hasErrors = true
	}

	// Parent and spin-off must differ
	if !hasErrors && parentSecurityID == spinOffSecurityID {
		fields[1].Error = "Spin-off must differ from parent"
		hasErrors = true
	}

	// Date (index 2)
	spinOffDate, err := parseDateInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Resulting Shares (index 3). The engine's share ratio is derived at
	// execution time from the parent's current total shares
	// (ratio = resulting_shares / parent_shares).
	resultingStr := strings.TrimSpace(fields[3].Value)
	var resultingShares float64
	if resultingStr == "" {
		fields[3].Error = "Resulting shares is required"
		hasErrors = true
	} else if resultingShares, err = strconv.ParseFloat(resultingStr, 64); err != nil {
		fields[3].Error = "Invalid number"
		hasErrors = true
	} else if resultingShares <= 0 {
		fields[3].Error = "Must be positive"
		hasErrors = true
	}

	// Parent Allocation % (index 4)
	allocStr := strings.TrimSpace(fields[4].Value)
	if allocStr == "" {
		fields[4].Error = "Parent allocation is required"
		hasErrors = true
	}
	var parentAllocPct float64
	if allocStr != "" {
		parentAllocPct, err = strconv.ParseFloat(allocStr, 64)
		if err != nil {
			fields[4].Error = "Invalid number"
			hasErrors = true
		} else if parentAllocPct <= 0 || parentAllocPct >= 100 {
			fields[4].Error = "Must be between 0 and 100 (exclusive)"
			hasErrors = true
		}
	}

	// Spin-Off Price (index 5)
	priceStr := strings.TrimSpace(fields[5].Value)
	if priceStr == "" {
		fields[5].Error = "Spin-off price is required"
		hasErrors = true
	}
	var spinOffPrice float64
	if priceStr != "" {
		spinOffPrice, err = strconv.ParseFloat(priceStr, 64)
		if err != nil {
			fields[5].Error = "Invalid number"
			hasErrors = true
		} else if spinOffPrice <= 0 {
			fields[5].Error = "Must be positive"
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	priceMoney := types.NewMoneyFromFloat(spinOffPrice)

	// Close dialog before async execution
	a.closeSpinOffDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil || a.corporateActionSvc == nil {
			return errMsg{err: fmt.Errorf("investment services not available")}
		}

		// Derive the engine's share ratio from the parent's current total shares.
		total, terr := a.investmentSvc.TotalSharesForSecurity(parentSecurityID)
		if terr != nil {
			return errMsg{err: fmt.Errorf("failed to read parent holding: %w", terr)}
		}
		totalF, _ := strconv.ParseFloat(total.String(), 64)
		if totalF <= 0 {
			return errMsg{err: fmt.Errorf("parent security has no shares to spin off from")}
		}

		params := investment.SpinOffParams{
			ShareRatio:          resultingShares / totalF,
			ParentAllocationPct: parentAllocPct,
		}
		_, err := a.corporateActionSvc.SpinOff(parentSecurityID, spinOffSecurityID, spinOffDate, params, priceMoney)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to execute spin-off: %w", err)}
		}

		return spinOffDialogSavedMsg{savedDate: spinOffDate}
	}
}
