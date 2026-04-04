package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// corporateActionHistoryData holds the loaded data for the corporate action history overlay.
type corporateActionHistoryData struct {
	security *security.Security
	actions  []*investment.CorporateAction
	secMap   map[types.ID]*security.Security // for resolving target security tickers
}

// corporateActionHistoryDataLoadedMsg is sent when corporate action history has been loaded.
type corporateActionHistoryDataLoadedMsg struct {
	data *corporateActionHistoryData
}

// loadCorporateActionHistory returns a command that loads corporate action history for a security.
func (a *App) loadCorporateActionHistory(sec *security.Security) tea.Cmd {
	secID := sec.ID
	return func() tea.Msg {
		if a.corporateActionSvc == nil || a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		actions, err := a.corporateActionSvc.ListBySecurity(secID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load corporate actions: %w", err)}
		}

		// Build security map for resolving target tickers
		allSecurities, err := a.securitySvc.List(security.Filter{})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}
		secMap := make(map[types.ID]*security.Security, len(allSecurities))
		for _, s := range allSecurities {
			secMap[s.ID] = s
		}

		return corporateActionHistoryDataLoadedMsg{
			data: &corporateActionHistoryData{
				security: sec,
				actions:  actions,
				secMap:   secMap,
			},
		}
	}
}

// closeCorporateActionHistory clears the corporate action history state.
func (a *App) closeCorporateActionHistory() {
	a.corporateActionHistory = nil
	a.corporateActionHistoryTable = nil
}

// buildCorporateActionHistoryTable creates and populates the table for the history overlay.
func (a *App) buildCorporateActionHistoryTable() {
	if a.corporateActionHistory == nil {
		return
	}

	columns := []Column{
		{Header: "Date", Width: 12, Align: AlignLeft},
		{Header: "Type", Width: 14, Align: AlignLeft},
		{Header: "Details", MinWidth: 20, Align: AlignLeft},
	}

	if a.corporateActionHistoryTable == nil {
		a.corporateActionHistoryTable = NewTable(columns)
	} else {
		a.corporateActionHistoryTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.corporateActionHistory.actions))
	for i, ca := range a.corporateActionHistory.actions {
		rows[i] = formatCorporateActionRow(ca, a.corporateActionHistory.secMap)
	}
	a.corporateActionHistoryTable.SetRows(rows)
	a.corporateActionHistoryTable.SetFocused(true)
}

// formatCorporateActionRow formats a corporate action into a table row.
func formatCorporateActionRow(ca *investment.CorporateAction, secMap map[types.ID]*security.Security) []string {
	return []string{
		ca.ActionDate.Time().Format("2006-01-02"),
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

// renderCorporateActionHistory renders the corporate action history overlay.
func (a *App) renderCorporateActionHistory() string {
	if a.corporateActionHistory == nil {
		return ""
	}

	overlayWidth := max(min(a.width-8, 70), 30)
	innerWidth := overlayWidth - 4

	var sections []string

	// Title
	titleText := fmt.Sprintf("Corporate Actions — %s", a.corporateActionHistory.security.Ticker)
	sections = append(sections, a.styles.Title.Render(titleText))

	// Separator
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", innerWidth)))

	// Table or empty message
	if a.corporateActionHistoryTable != nil && len(a.corporateActionHistory.actions) > 0 {
		tableHeight := max(min(len(a.corporateActionHistory.actions)+1, a.height-10), 2) // +1 for header row
		sections = append(sections, a.corporateActionHistoryTable.Render(a.styles, innerWidth, tableHeight))
		if info := a.corporateActionHistoryTable.ScrollInfo(tableHeight - 1); info != "" {
			sections = append(sections, a.styles.Muted.Render(info))
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("No corporate actions found"))
	}

	// Hint
	sections = append(sections, "")
	sections = append(sections, a.styles.Muted.Render("↑↓ navigate  esc close"))

	content := strings.Join(sections, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Width(overlayWidth)

	return boxStyle.Render(content)
}

// handleCorporateActionHistoryKeys handles key presses in the corporate action history overlay.
func (a *App) handleCorporateActionHistoryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.corporateActionHistory == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.closeCorporateActionHistory()
		return a, nil
	case key.Matches(msg, a.keys.Up):
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.PageUp(a.height - 10)
		}
	case msg.String() == "pgdown":
		if a.corporateActionHistoryTable != nil {
			a.corporateActionHistoryTable.PageDown(a.height - 10)
		}
	}

	return a, nil
}
