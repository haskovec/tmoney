package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
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

// spinOffDialogSavedMsg is sent when a spin-off has been executed.
type spinOffDialogSavedMsg struct{}

// buildSpinOffDialog creates a Dialog for executing a corporate spin-off.
// If preSelectedParentID is non-nil, the parent security selector is pre-selected.
func buildSpinOffDialog(securityOptions []string, securityIDs []types.ID, preSelectedParentID *types.ID) *Dialog {
	d := NewDialog("Spin-Off")
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

	// Share Ratio (e.g., "0.5" means 1 parent share = 0.5 spin-off shares)
	f = d.AddTextField("Share Ratio", "", "0.5", 10)
	f.Required = true

	// Parent Allocation % (e.g., "80" means parent keeps 80% of cost basis)
	f = d.AddTextField("Parent Allocation %", "", "80", 10)
	f.Required = true

	// Spin-Off Price (initial price per share of spin-off security)
	f = d.AddTextField("Spin-Off Price", "", "25.00", 10)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Execute", Primary: true},
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

	action := a.spinOffDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitSpinOffDialog()
	case DialogActionCancel:
		a.closeSpinOffDialog()
		return a, nil
	}

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

	// Share Ratio (index 3)
	ratioStr := strings.TrimSpace(fields[3].Value)
	if ratioStr == "" {
		fields[3].Error = "Share ratio is required"
		hasErrors = true
	}
	var shareRatio float64
	if ratioStr != "" {
		shareRatio, err = strconv.ParseFloat(ratioStr, 64)
		if err != nil {
			fields[3].Error = "Invalid number"
			hasErrors = true
		} else if shareRatio <= 0 {
			fields[3].Error = "Must be positive"
			hasErrors = true
		}
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

	params := investment.SpinOffParams{
		ShareRatio:          shareRatio,
		ParentAllocationPct: parentAllocPct,
	}
	priceMoney := types.NewMoneyFromFloat(spinOffPrice)

	// Close dialog before async execution
	a.closeSpinOffDialog()

	return a, func() tea.Msg {
		if a.corporateActionSvc == nil {
			return errMsg{err: fmt.Errorf("corporate action service not available")}
		}

		_, err := a.corporateActionSvc.SpinOff(parentSecurityID, spinOffSecurityID, spinOffDate, params, priceMoney)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to execute spin-off: %w", err)}
		}

		return spinOffDialogSavedMsg{}
	}
}
