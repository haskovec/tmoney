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

// corporateActionViewState is everything the Corporate Actions view owns. Its
// zero value is the view before its first load, with no details panel open
// and no filter entry in progress.
type corporateActionViewState struct {
	data  *corporateActionViewData
	table *widget.Table
	// detail is the action whose details panel is open. The panel is part of
	// the view, not a modal, but isDialogVisible counts it.
	detail *investment.CorporateAction
	// filterEditing is true while the user types the ticker filter.
	filterEditing bool
}

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

// corporateActionDeps is what the Corporate Actions view needs from outside
// itself. Every dep is a func, because switchDatabase replaces App's services
// and closes the previous *db.DB; and deps are passed to each call, never
// stored in the view state. Both rules are pinned by the guards that run over
// viewControllers.
type corporateActionDeps struct {
	corporateActions func() *investment.CorporateActionService
	securities       func() *security.Service
}

// corporateActionDeps binds the Corporate Actions view to the services App
// owns. Every accessor may return nil, because an App built by a test has no
// services, so each caller keeps its own nil guard.
func (a *App) corporateActionDeps() corporateActionDeps {
	return corporateActionDeps{
		corporateActions: func() *investment.CorporateActionService { return a.services.CorporateAction },
		securities:       func() *security.Service { return a.services.Security },
	}
}

// loadCorporateActionViewData fetches every corporate action and the
// security map used to resolve tickers. The view's filter query is left
// untouched so callers may pre-populate it before dispatching the load.
func (s *corporateActionViewState) load(d corporateActionDeps) tea.Cmd {
	return func() tea.Msg {
		corporateActions, securities := d.corporateActions(), d.securities()
		if corporateActions == nil || securities == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		actions, err := corporateActions.ListAll()
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load corporate actions: %w", err)}
		}

		allSecurities, err := securities.List(security.Filter{})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}
		secMap := make(map[types.ID]*security.Security, len(allSecurities))
		for _, sec := range allSecurities {
			secMap[sec.ID] = sec
		}

		return corporateActionViewLoadedMsg{
			data: &corporateActionViewData{actions: actions, secMap: secMap},
		}
	}
}

// filtered returns the subset of loaded actions whose
// ticker, type, or details match the current filter query
// (case-insensitive substring).
func (s *corporateActionViewState) filtered(filter string) []*investment.CorporateAction {
	if s.data == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(filter))
	if q == "" {
		return s.data.actions
	}
	filtered := make([]*investment.CorporateAction, 0, len(s.data.actions))
	for _, ca := range s.data.actions {
		ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, s.data.secMap)
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, s.data.secMap)
		details := formatCorporateActionDetails(ca, s.data.secMap)
		hay := strings.ToLower(strings.Join([]string{ticker, targetTicker, string(ca.ActionType), details}, " "))
		if strings.Contains(hay, q) {
			filtered = append(filtered, ca)
		}
	}
	return filtered
}

// buildTable creates and populates the table.
func (s *corporateActionViewState) buildTable(filter string) {
	if s.data == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
		{Header: "Ticker", Width: 10, Align: widget.AlignLeft},
		{Header: "Type", Width: 14, Align: widget.AlignLeft},
		{Header: "Details", MinWidth: 24, Align: widget.AlignLeft},
	}

	if s.table == nil {
		s.table = widget.NewTable(columns)
	} else {
		s.table.SetColumns(columns)
	}

	visible := s.filtered(filter)
	rows := make([][]string, len(visible))
	for i, ca := range visible {
		rows[i] = formatGlobalCorporateActionRow(ca, s.data.secMap)
	}
	s.table.SetRows(rows)
	s.table.SetFocused(true)
}

// selected returns the action under the table cursor, or
// nil if the table is empty or out of range.
func (s *corporateActionViewState) selected(filter string) *investment.CorporateAction {
	if s.table == nil {
		return nil
	}
	visible := s.filtered(filter)
	cursor := s.table.Cursor()
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
			return fmt.Sprintf("→ %s, ratio %s, cash $%s/sh", targetTicker,
				params.ExchangeRatio.Decimal().StringFixed(2), params.CashPerShare.Decimal().StringFixed(2))
		}
		return fmt.Sprintf("→ %s, ratio %s", targetTicker, params.ExchangeRatio.Decimal().StringFixed(2))

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

// render renders the full view (used as content body).
func (s *corporateActionViewState) render(styles widget.Styles, height int, filter string) string {
	if s.data == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading corporate actions...")
	}

	contentWidth := styles.ContentWidth()
	var sections []string

	titleRow := styles.Title.Render("CORPORATE ACTIONS")
	sections = append(sections, titleRow)

	filterLine := ""
	if filter != "" {
		filterLine = styles.Muted.Render(fmt.Sprintf("Filter: %s", filter))
	} else {
		filterLine = styles.Muted.Render("Press / to filter by ticker or type")
	}
	sections = append(sections, filterLine)

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	tableHeight := max(height-8, 2)

	visible := s.filtered(filter)
	if s.table != nil && len(visible) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, s.table.Render(styles, tableWidth, tableHeight))
		if info := s.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "", styles.Muted.Render("  No corporate actions"))
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

// renderDetails renders the read-only details overlay.
func (s *corporateActionViewState) renderDetails(styles widget.Styles, width int) string {
	ca := s.detail
	// This used to be guarded by its caller inside the view's render.
	// It is called from the app-level cascade now, so it carries its own: the
	// ticker lookups below dereference s.data.secMap.
	if ca == nil || s.data == nil {
		return ""
	}
	overlayWidth := corporateActionDetailWidth(width)
	// Border (1 each side) plus padding (2 each side) — see OverlayBox in
	// widget/styles.go. The previous overlayWidth-4 wrapped the separator.
	innerWidth := max(overlayWidth-dialog.DialogHorizontalOverhead, 10)

	var lines []string
	title := styles.Title.Render("Action Details")
	closeBtn := styles.Muted.Render("[x]")
	titleGap := max(innerWidth-lipgloss.Width(title)-lipgloss.Width(closeBtn), 1)
	lines = append(lines, title+strings.Repeat(" ", titleGap)+closeBtn)
	lines = append(lines, styles.Muted.Render(strings.Repeat("─", innerWidth)))

	ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, s.data.secMap)
	lines = append(lines, fmt.Sprintf("Type:    %s", ca.ActionType.DisplayName()))
	lines = append(lines, fmt.Sprintf("Date:    %s", ca.ActionDate.Time().Format("2006-01-02")))
	lines = append(lines, fmt.Sprintf("Ticker:  %s", ticker))
	if ca.TargetSecurityID.Valid {
		targetTicker := resolveSecurityTicker(ca.TargetSecurityID, s.data.secMap)
		lines = append(lines, fmt.Sprintf("Target:  %s", targetTicker))
	}
	lines = append(lines, fmt.Sprintf("Details: %s", formatCorporateActionDetails(ca, s.data.secMap)))
	lines = append(lines, "", styles.Muted.Render("esc close"))

	return styles.OverlayBox.Width(overlayWidth).Render(strings.Join(lines, "\n"))
}

// corporateActionShortcuts returns the shortcut section for the Corporate
// Actions help overlay. It lists the keys handleCorporateActionViewKeys binds;
// keep it in step with the view's status-bar hint in getKeyHints. Esc closes
// the details panel or ends a filter entry (handleKeyPress routes those to
// this view), and otherwise goes back.
func corporateActionShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Corporate Actions",
		Entries: []shortcutEntry{
			{"↑↓ / j k", "Navigate actions"},
			{"g / G", "First / last action"},
			{"PgUp/PgDn", "Page through actions"},
			{"/", "Filter actions"},
			{"Enter", "Show details"},
			{"d", "Reverse and delete action (asks first)"},
			{"Esc", "Close details or filter entry, else back"},
		},
	}
}

// handleCorporateActionViewKeys handles key presses in the global register.
func (a *App) handleCorporateActionViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Details modal handling takes precedence
	if a.corporateActions.detail != nil {
		if key.Matches(msg, a.keys.Escape) {
			a.corporateActions.detail = nil
		}
		return a, nil
	}

	// Filter-entry mode (when filter is being typed)
	if a.corporateActions.filterEditing {
		switch {
		case key.Matches(msg, a.keys.Escape):
			a.corporateActions.filterEditing = false
		case key.Matches(msg, a.keys.Enter):
			a.corporateActions.filterEditing = false
		default:
			if msg.String() == "backspace" {
				if len(a.corporateActionViewFilter) > 0 {
					a.corporateActionViewFilter = a.corporateActionViewFilter[:len(a.corporateActionViewFilter)-1]
					a.corporateActions.buildTable(a.corporateActionViewFilter)
				}
			} else if msg.Text != "" {
				a.corporateActionViewFilter += msg.Text
				a.corporateActions.buildTable(a.corporateActionViewFilter)
			}
		}
		return a, nil
	}

	// Esc on the list is not handled here: handleKeyPress's global Esc goes
	// back (and refreshes the view it returns to), the one back path. It
	// sends Esc here only to close the details panel, and the early guard
	// sends every key here while the filter is typed.
	switch {
	case key.Matches(msg, a.keys.Up):
		if a.corporateActions.table != nil {
			a.corporateActions.table.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.corporateActions.table != nil {
			a.corporateActions.table.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.corporateActions.table != nil {
			a.corporateActions.table.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.corporateActions.table != nil {
			a.corporateActions.table.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.corporateActions.table != nil {
			a.corporateActions.table.PageUp(a.height - 10)
		}
	case msg.String() == "pgdown":
		if a.corporateActions.table != nil {
			a.corporateActions.table.PageDown(a.height - 10)
		}
	case msg.String() == "/":
		a.corporateActions.filterEditing = true
	case key.Matches(msg, a.keys.Enter):
		if ca := a.corporateActions.selected(a.corporateActionViewFilter); ca != nil {
			a.corporateActions.detail = ca
		}
	case msg.String() == "d":
		if ca := a.corporateActions.selected(a.corporateActionViewFilter); ca != nil {
			a.confirmDeleteCorporateAction(ca)
		}
	}
	return a, nil
}

// confirmDeleteCorporateAction shows a confirmation dialog that names
// the action being reversed. On confirm, dispatches the reversal cmd.
func (a *App) confirmDeleteCorporateAction(ca *investment.CorporateAction) {
	ticker := resolveSecurityTicker(types.NullableID{ID: ca.SecurityID, Valid: true}, a.corporateActions.data.secMap)
	msg := fmt.Sprintf(
		"Reverse this %s on %s (%s) and delete the audit row? Lots, positions, and prices will be restored to their pre-action state.",
		ca.ActionType.DisplayName(), ticker, ca.ActionDate.Time().Format("2006-01-02"),
	)
	actionID := ca.ID
	a.showConfirmDialog(
		"Reverse Corporate Action",
		msg,
		func() tea.Msg {
			if a.services.CorporateAction == nil {
				return errMsg{err: fmt.Errorf("corporate action service not available")}
			}
			if err := a.services.CorporateAction.DeleteAction(actionID); err != nil {
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

// detailMouseAction maps a screen click to the read-only
// details overlay. Only the title row's [x] does anything; every other click,
// inside the panel or outside it, is inert — so the register underneath cannot
// move while the modal is up.
//
// renderLayout composites this overlay with widget.OverlayCenter at app level,
// so the transform is the standard one: OverlayTopLeft gives the panel's
// top-left corner, then border (1) + h-padding (2) on X and border (1) +
// v-padding (1) on Y reach the content band. Nothing offsets Y — that is the
// point of moving the render out of the view's render, where the
// 1-row header added a permanent +1.
func (s *corporateActionViewState) detailMouseAction(styles widget.Styles, width, height, x, y int) dialog.DialogAction {
	overlay := s.renderDetails(styles, width)
	if overlay == "" {
		return dialog.DialogActionNone
	}
	startCol, startRow := widget.OverlayTopLeft(overlay, width, height)
	localX, localY := x-startCol-3, y-startRow-2
	innerWidth := max(corporateActionDetailWidth(width)-dialog.DialogHorizontalOverhead, 10)
	if localY == 0 && localX >= innerWidth-3 && localX < innerWidth {
		return dialog.DialogActionCancel
	}
	return dialog.DialogActionNone
}

// handleDetailMouse closes the details overlay on a click on
// its [x]. Wheel events reach here too (handleMouseWheel routes through
// handleDialogMouse) and are swallowed: the overlay has no scroll surface and
// the register behind it must not move.
func (s *corporateActionViewState) handleDetailMouse(styles widget.Styles, width, height int, msg tea.MouseMsg) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return
	}
	m := msg.Mouse()
	if s.detailMouseAction(styles, width, height, m.X, m.Y) == dialog.DialogActionCancel {
		s.detail = nil
	}
}
