package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// corporateActionViewData holds the loaded data for the global
// corporate-action register.
type corporateActionViewData struct {
	actions []*investment.CorporateAction
	secMap  map[types.ID]*security.Security // resolves source + target tickers
}

// corporateActionViewLoadedMsg is sent when the register's data has loaded.
type corporateActionViewLoadedMsg struct {
	data *corporateActionViewData
}

// corporateActionDeletedMsg is sent after a successful reversal+delete.
type corporateActionDeletedMsg struct{}

// loadCorporateActionViewData fetches every corporate action and the
// security map used to resolve tickers. The view's filter query is left
// untouched so callers may pre-populate it before dispatching the load.
func (a *App) loadCorporateActionViewData() tea.Cmd {
	return func() tea.Msg {
		if a.corporateActionSvc == nil || a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		actions, err := a.corporateActionSvc.ListAll()
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load corporate actions: %w", err)}
		}

		allSecurities, err := a.securitySvc.List(security.Filter{})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}
		secMap := make(map[types.ID]*security.Security, len(allSecurities))
		for _, s := range allSecurities {
			secMap[s.ID] = s
		}

		return corporateActionViewLoadedMsg{
			data: &corporateActionViewData{actions: actions, secMap: secMap},
		}
	}
}

// closeCorporateActionView clears the register's state.
func (a *App) closeCorporateActionView() {
	a.corporateActionView = nil
	a.corporateActionViewTable = nil
	a.corporateActionViewFilter = ""
	a.corporateActionDetail = nil
}

// filteredCorporateActions returns the subset of loaded actions whose
// ticker, type, or details match the current filter query
// (case-insensitive substring).
func (a *App) filteredCorporateActions() []*investment.CorporateAction {
	if a.corporateActionView == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(a.corporateActionViewFilter))
	if q == "" {
		return a.corporateActionView.actions
	}
	filtered := make([]*investment.CorporateAction, 0, len(a.corporateActionView.actions))
	for _, ca := range a.corporateActionView.actions {
		ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, a.corporateActionView.secMap)
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, a.corporateActionView.secMap)
		details := formatCorporateActionDetails(ca, a.corporateActionView.secMap)
		hay := strings.ToLower(strings.Join([]string{ticker, targetTicker, string(ca.ActionType), details}, " "))
		if strings.Contains(hay, q) {
			filtered = append(filtered, ca)
		}
	}
	return filtered
}

// buildCorporateActionViewTable creates and populates the table.
func (a *App) buildCorporateActionViewTable() {
	if a.corporateActionView == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
		{Header: "Ticker", Width: 10, Align: widget.AlignLeft},
		{Header: "Type", Width: 14, Align: widget.AlignLeft},
		{Header: "Details", MinWidth: 24, Align: widget.AlignLeft},
	}

	if a.corporateActionViewTable == nil {
		a.corporateActionViewTable = widget.NewTable(columns)
	} else {
		a.corporateActionViewTable.SetColumns(columns)
	}

	visible := a.filteredCorporateActions()
	rows := make([][]string, len(visible))
	for i, ca := range visible {
		rows[i] = formatGlobalCorporateActionRow(ca, a.corporateActionView.secMap)
	}
	a.corporateActionViewTable.SetRows(rows)
	a.corporateActionViewTable.SetFocused(true)
}

// selectedCorporateAction returns the action under the table cursor, or
// nil if the table is empty or out of range.
func (a *App) selectedCorporateAction() *investment.CorporateAction {
	if a.corporateActionViewTable == nil {
		return nil
	}
	visible := a.filteredCorporateActions()
	cursor := a.corporateActionViewTable.Cursor()
	if cursor < 0 || cursor >= len(visible) {
		return nil
	}
	return visible[cursor]
}

// formatGlobalCorporateActionRow formats a row for the global register
// (includes the source ticker column).
func formatGlobalCorporateActionRow(ca *investment.CorporateAction, secMap map[types.ID]*security.Security) []string {
	ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, secMap)
	return []string{
		ca.ActionDate.Time().Format("2006-01-02"),
		ticker,
		ca.ActionType.DisplayName(),
		formatCorporateActionDetails(ca, secMap),
	}
}

// formatCorporateActionDetails formats the parameters of a corporate action into a readable string.
func formatCorporateActionDetails(ca *investment.CorporateAction, secMap map[types.ID]*security.Security) string {
	switch ca.ActionType {
	case investment.ActionTypeSplit, investment.ActionTypeReverseSplit:
		params, err := investment.ParseSplitParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		return fmt.Sprintf("Ratio %s", params.RatioString())

	case investment.ActionTypeMerger:
		params, err := investment.ParseMergerParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, secMap)
		if params.HasCashConsideration() {
			return fmt.Sprintf("→ %s, ratio %.2f, cash $%.2f/sh", targetTicker, params.ExchangeRatio, params.CashPerShare)
		}
		return fmt.Sprintf("→ %s, ratio %.2f", targetTicker, params.ExchangeRatio)

	case investment.ActionTypeSpinOff:
		params, err := investment.ParseSpinOffParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, secMap)
		return fmt.Sprintf("→ %s, ratio %.2f, parent %.1f%%", targetTicker, params.ShareRatio, params.ParentAllocationPct)
	}

	return ca.Parameters
}

// resolveSecurityTicker looks up a security ticker from a nullable ID in the security map.
func resolveSecurityTicker(id types.NullableID, secMap map[types.ID]*security.Security) string {
	if !id.Valid {
		return "???"
	}
	if secMap != nil {
		if sec, ok := secMap[id.ID]; ok {
			return sec.Ticker
		}
	}
	return "???"
}

// renderCorporateActionView renders the full view (used as content body).
func (a *App) renderCorporateActionView() string {
	if a.corporateActionView == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading corporate actions...")
	}

	contentWidth := a.styles.ContentWidth()
	var sections []string

	titleRow := a.styles.Title.Render("CORPORATE ACTIONS")
	sections = append(sections, titleRow)

	filterLine := ""
	if a.corporateActionViewFilter != "" {
		filterLine = a.styles.Muted.Render(fmt.Sprintf("Filter: %s", a.corporateActionViewFilter))
	} else {
		filterLine = a.styles.Muted.Render("Press / to filter by ticker or type")
	}
	sections = append(sections, filterLine)

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	tableHeight := max(a.height-8, 2)

	visible := a.filteredCorporateActions()
	if a.corporateActionViewTable != nil && len(visible) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.corporateActionViewTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.corporateActionViewTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "", a.styles.Muted.Render("  No corporate actions"))
	}

	// The details modal is composited by renderLayout's overlay cascade, not
	// here: inside this body the 1-row header shifted it down by one, a
	// menubar dropdown painted through it, and a short terminal clipped its
	// bottom border and the esc hint clean off the screen.
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// corporateActionDetailWidth is the details overlay's total width, border
// included. The renderer and the hit test share it so the two cannot drift.
func corporateActionDetailWidth(screenWidth int) int {
	return max(min(screenWidth-8, 70), 30)
}

// renderCorporateActionDetails renders the read-only details overlay.
func (a *App) renderCorporateActionDetails() string {
	ca := a.corporateActionDetail
	// This used to be guarded by its caller inside renderCorporateActionView.
	// It is called from the app-level cascade now, so it carries its own: the
	// ticker lookups below dereference a.corporateActionView.secMap.
	if ca == nil || a.corporateActionView == nil {
		return ""
	}
	overlayWidth := corporateActionDetailWidth(a.width)
	// Border (1 each side) plus padding (2 each side) — see OverlayBox in
	// widget/styles.go. The previous overlayWidth-4 wrapped the separator.
	innerWidth := max(overlayWidth-dialog.DialogHorizontalOverhead, 10)

	var lines []string
	title := a.styles.Title.Render("Action Details")
	closeBtn := a.styles.Muted.Render("[x]")
	titleGap := max(innerWidth-lipgloss.Width(title)-lipgloss.Width(closeBtn), 1)
	lines = append(lines, title+strings.Repeat(" ", titleGap)+closeBtn)
	lines = append(lines, a.styles.Muted.Render(strings.Repeat("─", innerWidth)))

	ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, a.corporateActionView.secMap)
	lines = append(lines, fmt.Sprintf("Type:    %s", ca.ActionType.DisplayName()))
	lines = append(lines, fmt.Sprintf("Date:    %s", ca.ActionDate.Time().Format("2006-01-02")))
	lines = append(lines, fmt.Sprintf("Ticker:  %s", ticker))
	if ca.TargetSecurityID.Valid {
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, a.corporateActionView.secMap)
		lines = append(lines, fmt.Sprintf("Target:  %s", targetTicker))
	}
	lines = append(lines, fmt.Sprintf("Details: %s", formatCorporateActionDetails(ca, a.corporateActionView.secMap)))
	lines = append(lines, "", a.styles.Muted.Render("esc close"))

	return a.styles.OverlayBox.Width(overlayWidth).Render(strings.Join(lines, "\n"))
}

// handleCorporateActionViewKeys handles key presses in the global register.
func (a *App) handleCorporateActionViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Details modal handling takes precedence
	if a.corporateActionDetail != nil {
		if key.Matches(msg, a.keys.Escape) {
			a.corporateActionDetail = nil
		}
		return a, nil
	}

	// Filter-entry mode (when filter is being typed)
	if a.corporateActionViewFilterEditing {
		switch {
		case key.Matches(msg, a.keys.Escape):
			a.corporateActionViewFilterEditing = false
		case key.Matches(msg, a.keys.Enter):
			a.corporateActionViewFilterEditing = false
		default:
			if msg.String() == "backspace" {
				if len(a.corporateActionViewFilter) > 0 {
					a.corporateActionViewFilter = a.corporateActionViewFilter[:len(a.corporateActionViewFilter)-1]
					a.buildCorporateActionViewTable()
				}
			} else if msg.Text != "" {
				a.corporateActionViewFilter += msg.Text
				a.buildCorporateActionViewTable()
			}
		}
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.closeCorporateActionView()
		a.switchView(a.previousView)
		return a, nil
	case key.Matches(msg, a.keys.Up):
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.PageUp(a.height - 10)
		}
	case msg.String() == "pgdown":
		if a.corporateActionViewTable != nil {
			a.corporateActionViewTable.PageDown(a.height - 10)
		}
	case msg.String() == "/":
		a.corporateActionViewFilterEditing = true
	case key.Matches(msg, a.keys.Enter):
		if ca := a.selectedCorporateAction(); ca != nil {
			a.corporateActionDetail = ca
		}
	case msg.String() == "d":
		if ca := a.selectedCorporateAction(); ca != nil {
			a.confirmDeleteCorporateAction(ca)
		}
	}
	return a, nil
}

// confirmDeleteCorporateAction shows a confirmation dialog that names
// the action being reversed. On confirm, dispatches the reversal cmd.
func (a *App) confirmDeleteCorporateAction(ca *investment.CorporateAction) {
	ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, a.corporateActionView.secMap)
	msg := fmt.Sprintf(
		"Reverse this %s on %s (%s) and delete the audit row? Lots, positions, and prices will be restored to their pre-action state.",
		ca.ActionType.DisplayName(), ticker, ca.ActionDate.Time().Format("2006-01-02"),
	)
	actionID := ca.ID
	a.showConfirmDialog(
		"Reverse Corporate Action",
		msg,
		func() tea.Msg {
			if a.corporateActionSvc == nil {
				return errMsg{err: fmt.Errorf("corporate action service not available")}
			}
			if err := a.corporateActionSvc.DeleteAction(actionID); err != nil {
				var dse *investment.DownstreamEventsError
				var ure *investment.UnsupportedReversalError
				switch {
				case errors.As(err, &dse), errors.As(err, &ure):
					return errMsg{err: err}
				default:
					return errMsg{err: fmt.Errorf("failed to reverse corporate action: %w", err)}
				}
			}
			return corporateActionDeletedMsg{}
		},
	)
}

// corporateActionDetailMouseAction maps a screen click to the read-only
// details overlay. Only the title row's [x] does anything; every other click,
// inside the panel or outside it, is inert — so the register underneath cannot
// move while the modal is up.
//
// renderLayout composites this overlay with widget.OverlayCenter at app level,
// so the transform is the standard one: OverlayTopLeft gives the panel's
// top-left corner, then border (1) + h-padding (2) on X and border (1) +
// v-padding (1) on Y reach the content band. Nothing offsets Y — that is the
// point of moving the render out of renderCorporateActionView, where the
// 1-row header added a permanent +1.
func (a *App) corporateActionDetailMouseAction(x, y int) dialog.DialogAction {
	overlay := a.renderCorporateActionDetails()
	if overlay == "" {
		return dialog.DialogActionNone
	}
	startCol, startRow := widget.OverlayTopLeft(overlay, a.width, a.height)
	localX, localY := x-startCol-3, y-startRow-2
	innerWidth := max(corporateActionDetailWidth(a.width)-dialog.DialogHorizontalOverhead, 10)
	if localY == 0 && localX >= innerWidth-3 && localX < innerWidth {
		return dialog.DialogActionCancel
	}
	return dialog.DialogActionNone
}

// handleCorporateActionDetailMouse closes the details overlay on a click on
// its [x]. Wheel events reach here too (handleMouseWheel routes through
// handleDialogMouse) and are swallowed: the overlay has no scroll surface and
// the register behind it must not move.
func (a *App) handleCorporateActionDetailMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}
	m := msg.Mouse()
	if a.corporateActionDetailMouseAction(m.X, m.Y) == dialog.DialogActionCancel {
		a.corporateActionDetail = nil
	}
	return a, nil
}
