package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// stockSplitDialogData holds the loaded data for the stock split dialog.
type stockSplitDialogData struct {
	securities []*security.Security
}

// stockSplitDialogDataMsg is sent when stock split dialog data has been loaded.
type stockSplitDialogDataMsg struct {
	data *stockSplitDialogData
}

// stockSplitDialogSavedMsg is sent when a stock split has been executed.
type stockSplitDialogSavedMsg struct{}

// buildStockSplitDialog creates a Dialog for executing a stock split.
// If preSelectedSecurityID is non-nil, the security selector is pre-selected.
func buildStockSplitDialog(securityOptions []string, securityIDs []types.ID, preSelectedSecurityID *types.ID) *Dialog {
	d := NewDialog("Stock Split")
	d.SetWidth(50)

	// Security selector
	selectedIdx := 0
	if preSelectedSecurityID != nil {
		for i, id := range securityIDs {
			if id == *preSelectedSecurityID {
				selectedIdx = i
				break
			}
		}
	}
	d.AddSelectField("Security", securityOptions, selectedIdx)

	// Date
	f := d.AddTextField("Date", time.Now().Format("01/02/2006"), "MM/DD/YYYY", 10)
	f.Required = true

	// Ratio (e.g., "4:1" for a 4-for-1 split, "1:10" for a reverse split)
	f = d.AddTextField("Ratio", "", "4:1", 10)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Execute", Primary: true},
	})

	d.SetVisible(true)
	return d
}

// loadStockSplitDialogData returns a command that loads securities for the stock split dialog.
func (a *App) loadStockSplitDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &stockSplitDialogData{}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		return stockSplitDialogDataMsg{data: data}
	}
}

// closeStockSplitDialog clears the stock split dialog state.
func (a *App) closeStockSplitDialog() {
	a.stockSplitDialog = nil
	a.stockSplitDialogData = nil
	a.stockSplitDialogSecurityIDs = nil
}

// handleStockSplitDialogKey routes key events to the stock split dialog.
func (a *App) handleStockSplitDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.stockSplitDialog == nil {
		return a, nil
	}

	action := a.stockSplitDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitStockSplitDialog()
	case DialogActionCancel:
		a.closeStockSplitDialog()
		return a, nil
	}

	return a, nil
}

// submitStockSplitDialog validates and executes the stock split.
func (a *App) submitStockSplitDialog() (tea.Model, tea.Cmd) {
	if a.stockSplitDialog == nil || a.stockSplitDialogData == nil {
		return a, nil
	}

	fields := a.stockSplitDialog.Fields()
	if len(fields) < 3 {
		return a, nil
	}

	a.stockSplitDialog.ClearErrors()
	hasErrors := false

	// Security (index 0)
	if len(a.stockSplitDialogSecurityIDs) == 0 {
		fields[0].Error = "No securities available"
		hasErrors = true
	}
	secIdx := fields[0].SelectedIndex
	var securityID types.ID
	if secIdx >= 0 && secIdx < len(a.stockSplitDialogSecurityIDs) {
		securityID = a.stockSplitDialogSecurityIDs[secIdx]
	} else {
		fields[0].Error = "Select a security"
		hasErrors = true
	}

	// Date (index 1)
	splitDate, err := parseDateInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Ratio (index 2)
	ratioStr := strings.TrimSpace(fields[2].Value)
	if ratioStr == "" {
		fields[2].Error = "Ratio is required (e.g., 4:1)"
		hasErrors = true
	}
	var splitParams *investment.SplitParams
	if ratioStr != "" {
		splitParams, err = investment.ParseSplitRatio(ratioStr)
		if err != nil {
			fields[2].Error = fmt.Sprintf("Invalid ratio: %s", err.Error())
			hasErrors = true
		} else if errs := splitParams.Validate(); errs.HasErrors() {
			fields[2].Error = fmt.Sprintf("Invalid ratio: %s", errs.Error())
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	// Close dialog before async execution
	a.closeStockSplitDialog()

	return a, func() tea.Msg {
		if a.corporateActionSvc == nil {
			return errMsg{err: fmt.Errorf("corporate action service not available")}
		}

		_, err := a.corporateActionSvc.Split(securityID, splitDate, *splitParams)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to execute stock split: %w", err)}
		}

		return stockSplitDialogSavedMsg{}
	}
}
