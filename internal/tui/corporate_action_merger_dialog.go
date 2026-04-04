package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// mergerDialogData holds the loaded data for the merger dialog.
type mergerDialogData struct {
	securities []*security.Security
}

// mergerDialogDataMsg is sent when merger dialog data has been loaded.
type mergerDialogDataMsg struct {
	data *mergerDialogData
}

// mergerDialogSavedMsg is sent when a merger has been executed.
type mergerDialogSavedMsg struct{}

// buildMergerDialog creates a Dialog for executing a merger/acquisition.
// If preSelectedSourceID is non-nil, the source security selector is pre-selected.
func buildMergerDialog(securityOptions []string, securityIDs []types.ID, preSelectedSourceID *types.ID) *Dialog {
	d := NewDialog("Merger / Acquisition")
	d.SetWidth(55)

	// Source Security selector
	sourceIdx := 0
	if preSelectedSourceID != nil {
		for i, id := range securityIDs {
			if id == *preSelectedSourceID {
				sourceIdx = i
				break
			}
		}
	}
	d.AddSelectField("Source Security", securityOptions, sourceIdx)

	// Target Security selector
	d.AddSelectField("Target Security", securityOptions, 0)

	// Date
	f := d.AddTextField("Date", time.Now().Format("01/02/2006"), "MM/DD/YYYY", 10)
	f.Required = true

	// Exchange Ratio (e.g., "2.0" means 1 source share = 2.0 target shares)
	f = d.AddTextField("Exchange Ratio", "", "2.0", 10)
	f.Required = true

	// Cash Per Share (optional)
	d.AddTextField("Cash Per Share", "", "0.00", 10)

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Execute", Primary: true},
	})

	d.SetVisible(true)
	return d
}

// loadMergerDialogData returns a command that loads securities for the merger dialog.
func (a *App) loadMergerDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &mergerDialogData{}

		if a.securitySvc != nil {
			securities, err := a.securitySvc.List(security.Filter{})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		return mergerDialogDataMsg{data: data}
	}
}

// closeMergerDialog clears the merger dialog state.
func (a *App) closeMergerDialog() {
	a.mergerDialog = nil
	a.mergerDialogData = nil
	a.mergerDialogSecurityIDs = nil
}

// handleMergerDialogKey routes key events to the merger dialog.
func (a *App) handleMergerDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.mergerDialog == nil {
		return a, nil
	}

	action := a.mergerDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitMergerDialog()
	case DialogActionCancel:
		a.closeMergerDialog()
		return a, nil
	}

	return a, nil
}

// submitMergerDialog validates and executes the merger.
func (a *App) submitMergerDialog() (tea.Model, tea.Cmd) {
	if a.mergerDialog == nil || a.mergerDialogData == nil {
		return a, nil
	}

	fields := a.mergerDialog.Fields()
	if len(fields) < 5 {
		return a, nil
	}

	a.mergerDialog.ClearErrors()
	hasErrors := false

	// Source Security (index 0)
	if len(a.mergerDialogSecurityIDs) == 0 {
		fields[0].Error = "No securities available"
		hasErrors = true
	}
	sourceIdx := fields[0].SelectedIndex
	var sourceSecurityID types.ID
	if sourceIdx >= 0 && sourceIdx < len(a.mergerDialogSecurityIDs) {
		sourceSecurityID = a.mergerDialogSecurityIDs[sourceIdx]
	} else {
		fields[0].Error = "Select a source security"
		hasErrors = true
	}

	// Target Security (index 1)
	targetIdx := fields[1].SelectedIndex
	var targetSecurityID types.ID
	if targetIdx >= 0 && targetIdx < len(a.mergerDialogSecurityIDs) {
		targetSecurityID = a.mergerDialogSecurityIDs[targetIdx]
	} else {
		fields[1].Error = "Select a target security"
		hasErrors = true
	}

	// Source and target must differ
	if !hasErrors && sourceSecurityID == targetSecurityID {
		fields[1].Error = "Target must differ from source"
		hasErrors = true
	}

	// Date (index 2)
	mergerDate, err := parseDateInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Exchange Ratio (index 3)
	ratioStr := strings.TrimSpace(fields[3].Value)
	if ratioStr == "" {
		fields[3].Error = "Exchange ratio is required"
		hasErrors = true
	}
	var exchangeRatio float64
	if ratioStr != "" {
		exchangeRatio, err = strconv.ParseFloat(ratioStr, 64)
		if err != nil {
			fields[3].Error = "Invalid number"
			hasErrors = true
		} else if exchangeRatio <= 0 {
			fields[3].Error = "Must be positive"
			hasErrors = true
		}
	}

	// Cash Per Share (index 4, optional)
	var cashPerShare float64
	cashStr := strings.TrimSpace(fields[4].Value)
	if cashStr != "" {
		cashPerShare, err = strconv.ParseFloat(cashStr, 64)
		if err != nil {
			fields[4].Error = "Invalid number"
			hasErrors = true
		} else if cashPerShare < 0 {
			fields[4].Error = "Must not be negative"
			hasErrors = true
		}
	}

	if hasErrors {
		return a, nil
	}

	params := investment.MergerParams{
		ExchangeRatio: exchangeRatio,
		CashPerShare:  cashPerShare,
	}

	// Close dialog before async execution
	a.closeMergerDialog()

	return a, func() tea.Msg {
		if a.corporateActionSvc == nil {
			return errMsg{err: fmt.Errorf("corporate action service not available")}
		}

		_, err := a.corporateActionSvc.Merger(sourceSecurityID, targetSecurityID, mergerDate, params)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to execute merger: %w", err)}
		}

		return mergerDialogSavedMsg{}
	}
}
