package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

const splitConventionExplainer = "Ratio is N:M — N new shares for every M held. e.g. 2:1 = forward 2-for-1, 1:2 = halves shares."

// stockSplitDialogData holds the loaded data for the stock split dialog.
type stockSplitDialogData struct {
	securities []*security.Security
	// sharesMap is keyed by security ID; the value is the per-account
	// share total used to drive the dialog's live preview.
	sharesMap map[types.ID][]investment.AccountShares
}

// stockSplitDialogDataMsg is sent when stock split dialog data has been loaded.
type stockSplitDialogDataMsg struct {
	data *stockSplitDialogData
}

// stockSplitDialogSavedMsg is sent when a stock split has been executed.
// savedDate carries the executed date so the handler can update the session
// sticky date.
type stockSplitDialogSavedMsg struct{ savedDate types.Date }

// buildStockSplitDialog creates a dialog.Dialog for executing a stock split.
// If preSelectedSecurityID is non-nil, the security selector is pre-selected.
func buildStockSplitDialog(securityOptions []string, securityIDs []types.ID, sharesMap map[types.ID][]investment.AccountShares, preSelectedSecurityID *types.ID) *dialog.Dialog {
	d := dialog.NewDialog("Stock Split")
	d.SetWidth(60)

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
	d.AddComboField("Security", securityOptions, selectedIdx)

	// Date
	f := d.AddDateField("Date", "")
	f.Required = true

	// Ratio (e.g., "4:1" for a 4-for-1 split, "1:10" for a reverse split)
	f = d.AddTextField("Ratio", "", "4:1", 10)
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Execute", Primary: true},
		{Label: "Cancel"},
	})

	// No date entered yet at build time; the per-account "affected as of date"
	// projection is filled in by refreshStockSplitDialogMessage once the dialog
	// (and its seeded date) is live.
	d.SetMessage(renderSplitDialogMessage(securityIDs, sharesMap, selectedIdx, "", nil))

	d.SetVisible(true)
	return d
}

// renderSplitDialogMessage produces the dialog body message: a convention
// explainer plus a per-account "before → after" preview when the
// currently-entered ratio parses successfully.
func renderSplitDialogMessage(secIDs []types.ID, sharesMap map[types.ID][]investment.AccountShares, secIdx int, ratioStr string, affected map[types.ID]types.Quantity) string {
	lines := []string{splitConventionExplainer}

	if secIdx < 0 || secIdx >= len(secIDs) {
		return strings.Join(lines, "\n")
	}
	shares := sharesMap[secIDs[secIdx]]
	if len(shares) == 0 {
		lines = append(lines, "", "No current positions in this security.")
		return strings.Join(lines, "\n")
	}

	params, perr := investment.ParseSplitRatio(strings.TrimSpace(ratioStr))
	validRatio := perr == nil && params != nil && !params.Validate().HasErrors()

	if !validRatio {
		lines = append(lines, "", "Current positions:")
		for _, as := range shares {
			lines = append(lines, fmt.Sprintf("  %s: %s shares", as.AccountName, as.Shares.String()))
		}
		return strings.Join(lines, "\n")
	}

	ratio := alpacadecimal.NewFromInt(int64(params.Numerator)).
		Div(alpacadecimal.NewFromInt(int64(params.Denominator)))
	one := alpacadecimal.NewFromInt(1)
	lines = append(lines, "", "After split:")
	for _, as := range shares {
		// Only the shares held as of the split date are adjusted; shares acquired
		// afterward keep their post-split quantity. projected = current +
		// affected × (ratio − 1). With no affected entry (nothing held as of the
		// date) the holding is unchanged.
		aff, ok := affected[as.AccountID]
		if !ok {
			aff = types.ZeroQuantity
		}
		projected := as.Shares.Add(aff.Mul(ratio.Sub(one)))
		lines = append(lines, fmt.Sprintf("  %s: %s → %s shares", as.AccountName, as.Shares.String(), projected.String()))
	}
	return strings.Join(lines, "\n")
}

// refreshStockSplitDialogMessage updates the dialog's message body to
// reflect the currently-selected security and typed ratio.
func (a *App) refreshStockSplitDialogMessage() {
	if a.stockSplitDialog == nil || a.stockSplitDialogData == nil {
		return
	}
	fields := a.stockSplitDialog.Fields()
	if len(fields) < 3 {
		return
	}
	secIdx := fields[0].SelectedIndex
	dateStr := fields[1].Value
	ratioStr := fields[2].Value

	// Compute the shares each account actually held as of the entered split
	// date — the only shares the split will adjust — so the projection reflects
	// the date-scoped engine instead of naively scaling every holding.
	affected := map[types.ID]types.Quantity{}
	if a.investmentSvc != nil && secIdx >= 0 && secIdx < len(a.stockSplitDialogSecurityIDs) {
		if d, err := parseDateInput(dateStr); err == nil {
			if asOf, err := a.investmentSvc.SharesBySecurityAsOf(a.stockSplitDialogSecurityIDs[secIdx], d); err == nil {
				for _, as := range asOf {
					affected[as.AccountID] = as.Shares
				}
			}
		}
	}

	a.stockSplitDialog.SetMessage(renderSplitDialogMessage(
		a.stockSplitDialogSecurityIDs,
		a.stockSplitDialogData.sharesMap,
		secIdx,
		ratioStr,
		affected,
	))
}

// loadStockSplitDialogData returns a command that loads securities and
// per-account share totals for the stock split dialog.
func (a *App) loadStockSplitDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &stockSplitDialogData{
			sharesMap: make(map[types.ID][]investment.AccountShares),
		}

		if a.securitySvc != nil {
			excludeHidden := true
			securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
			if err != nil {
				return errMsg{err: err}
			}
			data.securities = securities
		}

		if a.investmentSvc != nil {
			for _, sec := range data.securities {
				shares, err := a.investmentSvc.SharesBySecurity(sec.ID)
				if err != nil {
					return errMsg{err: err}
				}
				if len(shares) > 0 {
					data.sharesMap[sec.ID] = shares
				}
			}
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
	return a.stockSplitDialogAction(a.stockSplitDialog.HandleKey(msg))
}

// stockSplitDialogAction dispatches a DialogAction for the stock split dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) stockSplitDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitStockSplitDialog()
	case dialog.DialogActionCancel:
		a.closeStockSplitDialog()
		return a, nil
	}

	a.refreshStockSplitDialogMessage()
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

		return stockSplitDialogSavedMsg{savedDate: splitDate}
	}
}
