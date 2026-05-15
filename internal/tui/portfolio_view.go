package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// portfolioViewData holds the loaded data for the portfolio view.
type portfolioViewData struct {
	account       *account.Account
	valuation     *investment.AccountValuation
	securityNames map[types.ID]string // SecurityID -> Ticker
	lotDetails    []investment.LotDetail
	lotSecurityID types.ID // which security's lots are showing
}

// portfolioLoadedMsg is sent when portfolio data has been loaded.
type portfolioLoadedMsg struct {
	data *portfolioViewData
}

// portfolioLotDetailMsg is sent when lot detail has been loaded.
type portfolioLotDetailMsg struct {
	lots       []investment.LotDetail
	securityID types.ID
}

// portfolioViewMode tracks whether we're showing holdings or lot detail.
type portfolioViewMode int

const (
	portfolioViewHoldings portfolioViewMode = iota
	portfolioViewLots
)

// loadPortfolioData returns a command that loads all data needed for the portfolio view.
func (a *App) loadPortfolioData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &portfolioViewData{
			securityNames: make(map[types.ID]string),
		}

		// Load account
		if a.accountSvc != nil {
			acct, err := a.accountSvc.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct
		}

		// Load account valuation
		if a.investmentSvc != nil {
			asOf := types.Today()
			val, err := a.investmentSvc.GetAccountValuation(accountID, asOf, investment.ValuationOptions{})
			if err != nil {
				return errMsg{err: err}
			}
			data.valuation = val
		}

		// Load security names for display
		if a.securitySvc != nil {
			securities, err := a.securitySvc.List(security.Filter{})
			if err == nil {
				for _, sec := range securities {
					data.securityNames[sec.ID] = sec.Ticker
				}
			}
		}

		return portfolioLoadedMsg{data: data}
	}
}

// loadLotDetail returns a command that loads lot detail for a specific security.
func (a *App) loadLotDetail(accountID, securityID types.ID) tea.Cmd {
	return func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		asOf := types.Today()
		lots, err := a.investmentSvc.GetLotDetail(accountID, securityID, asOf)
		if err != nil {
			return errMsg{err: err}
		}

		return portfolioLotDetailMsg{lots: lots, securityID: securityID}
	}
}

// buildPortfolioHoldingsTable creates and populates the holdings table.
func (a *App) buildPortfolioHoldingsTable() {
	if a.portfolioData == nil || a.portfolioData.valuation == nil {
		return
	}

	columns := []Column{
		{Header: "Ticker", Width: 10, Align: AlignLeft},
		{Header: "Shares", Width: 12, Align: AlignRight},
		{Header: "Avg Cost", Width: 12, Align: AlignRight},
		{Header: "Price", Width: 12, Align: AlignRight},
		{Header: "Price Date", Width: 10, Align: AlignLeft},
		{Header: "Mkt Value", Width: 14, Align: AlignRight},
		{Header: "Cost Basis", Width: 14, Align: AlignRight},
		{Header: "Gain/Loss", Width: 14, Align: AlignRight},
		{Header: "G/L %", Width: 8, Align: AlignRight},
	}

	if a.portfolioHoldingsTable == nil {
		a.portfolioHoldingsTable = NewTable(columns)
	} else {
		a.portfolioHoldingsTable.SetColumns(columns)
	}

	holdings := a.portfolioData.valuation.Holdings
	rows := make([][]string, len(holdings))
	for i, h := range holdings {
		rows[i] = a.formatHoldingRow(&h)
	}
	a.portfolioHoldingsTable.SetRows(rows)
}

// formatHoldingRow formats a holding into table row strings.
func (a *App) formatHoldingRow(h *investment.Holding) []string {
	// Ticker
	ticker := ""
	if name, ok := a.portfolioData.securityNames[h.SecurityID]; ok {
		ticker = name
	}
	if !h.HasPricing {
		ticker = "~" + ticker
	}

	// Shares
	shares := h.Shares.String()

	// Avg Cost
	avgCost := formatDashboardMoney(h.AvgCost)

	// Current price
	price := ""
	if h.HasPricing {
		price = formatDashboardMoney(h.CurrentPrice)
	} else {
		price = "N/A"
	}

	// Price date
	priceDate := ""
	if h.HasPricing && !h.PriceDate.Time().IsZero() {
		priceDate = h.PriceDate.Time().Format("01/02/06")
	}

	// Market value
	mktValue := formatDashboardMoney(h.MarketValue)

	// Cost basis
	costBasis := formatDashboardMoney(h.CostBasis)

	// Gain/loss
	gainLoss := formatDashboardMoney(h.GainLoss)

	// Gain/loss %
	glPct := fmt.Sprintf("%.2f%%", h.GainPct)

	return []string{ticker, shares, avgCost, price, priceDate, mktValue, costBasis, gainLoss, glPct}
}

// buildPortfolioLotsTable creates and populates the lot detail table.
func (a *App) buildPortfolioLotsTable() {
	if a.portfolioData == nil || a.portfolioData.lotDetails == nil {
		return
	}

	columns := []Column{
		{Header: "Purchase Date", Width: 13, Align: AlignLeft},
		{Header: "Shares", Width: 12, Align: AlignRight},
		{Header: "Cost/Share", Width: 12, Align: AlignRight},
		{Header: "Cost Basis", Width: 14, Align: AlignRight},
		{Header: "Cur. Value", Width: 14, Align: AlignRight},
		{Header: "Gain/Loss", Width: 14, Align: AlignRight},
		{Header: "G/L %", Width: 8, Align: AlignRight},
	}

	if a.portfolioLotsTable == nil {
		a.portfolioLotsTable = NewTable(columns)
	} else {
		a.portfolioLotsTable.SetColumns(columns)
	}

	lots := a.portfolioData.lotDetails
	rows := make([][]string, len(lots))
	for i, lot := range lots {
		rows[i] = formatLotDetailRow(&lot)
	}
	a.portfolioLotsTable.SetRows(rows)
}

// formatLotDetailRow formats a lot detail into table row strings.
func formatLotDetailRow(lot *investment.LotDetail) []string {
	purchaseDate := lot.PurchaseDate.Time().Format("01/02/06")
	shares := lot.Shares.String()
	costPerShare := formatDashboardMoney(lot.CostPerShare)
	costBasis := formatDashboardMoney(lot.CostBasis)
	currentValue := formatDashboardMoney(lot.CurrentValue)
	gainLoss := formatDashboardMoney(lot.GainLoss)
	glPct := fmt.Sprintf("%.2f%%", lot.GainPct)

	return []string{purchaseDate, shares, costPerShare, costBasis, currentValue, gainLoss, glPct}
}

// renderPortfolioSummary renders the summary bar showing account totals.
func (a *App) renderPortfolioSummary(contentWidth int) string {
	if a.portfolioData == nil || a.portfolioData.valuation == nil {
		return ""
	}

	v := a.portfolioData.valuation

	type metric struct {
		label string
		value string
		money types.Money
	}

	metrics := []metric{
		{label: "Cash", value: formatDashboardMoney(v.CashBalance), money: v.CashBalance},
		{label: "Mkt Value", value: formatDashboardMoney(v.MarketValue), money: v.MarketValue},
		{label: "Total", value: formatDashboardMoney(v.TotalValue), money: v.TotalValue},
		{label: "Cost Basis", value: formatDashboardMoney(v.TotalCostBasis), money: v.TotalCostBasis},
		{label: "Gain/Loss", value: formatDashboardMoney(v.TotalGainLoss), money: v.TotalGainLoss},
		{label: "G/L %", value: fmt.Sprintf("%.2f%%", v.TotalGainPct)},
	}

	// Build summary line: label: value pairs separated by spaces
	var parts []string
	for _, m := range metrics {
		labelStr := a.styles.Muted.Render(m.label + ":")
		valueStyle := a.styles.Bold
		if m.label == "Gain/Loss" || m.label == "G/L %" {
			if v.TotalGainLoss.IsNegative() {
				valueStyle = a.styles.Negative
			} else if !v.TotalGainLoss.IsZero() {
				valueStyle = a.styles.Positive
			}
		}
		parts = append(parts, labelStr+" "+valueStyle.Render(m.value))
	}

	return strings.Join(parts, "  ")
}

// renderPortfolioView renders the portfolio view.
func (a *App) renderPortfolioView() string {
	if a.portfolioData == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading portfolio...")
	}

	contentWidth := a.styles.ContentWidth()
	var sections []string

	// Title row: account name + "PORTFOLIO"
	acctName := strings.ToUpper(a.portfolioData.account.Name)
	titleSuffix := " PORTFOLIO"
	maxNameWidth := max(contentWidth-lipgloss.Width(titleSuffix)-4, 10)
	acctName = truncate(acctName, maxNameWidth)
	titleRow := a.styles.Title.Render(acctName + titleSuffix)
	sections = append(sections, titleRow)

	// Summary bar
	summary := a.renderPortfolioSummary(contentWidth)
	if summary != "" {
		sections = append(sections, summary)
	}

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Calculate table height
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 1
	summaryHeight := 1
	separatorHeight := 1
	paddingHeight := 2 // top/bottom padding
	hintHeight := 1
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-summaryHeight-separatorHeight-paddingHeight-hintHeight, 1)

	if a.portfolioMode == portfolioViewLots {
		// Show lot detail
		secTicker := ""
		if name, ok := a.portfolioData.securityNames[a.portfolioData.lotSecurityID]; ok {
			secTicker = name
		}
		sections = append(sections, a.styles.Bold.Render("  Lots for "+secTicker))

		if a.portfolioLotsTable != nil && len(a.portfolioData.lotDetails) > 0 {
			tableWidth := max(contentWidth-4, 1)
			sections = append(sections, a.portfolioLotsTable.Render(a.styles, tableWidth, tableHeight-1))
			if info := a.portfolioLotsTable.ScrollInfo(tableHeight - 2); info != "" {
				sections = append(sections, a.styles.Muted.Render("  "+info))
			}
		} else {
			sections = append(sections, a.styles.Muted.Render("  No lots"))
		}
	} else {
		// Show holdings table
		if a.portfolioHoldingsTable != nil && len(a.portfolioData.valuation.Holdings) > 0 {
			tableWidth := max(contentWidth-4, 1)
			sections = append(sections, a.portfolioHoldingsTable.Render(a.styles, tableWidth, tableHeight))
			if info := a.portfolioHoldingsTable.ScrollInfo(tableHeight - 2); info != "" {
				sections = append(sections, a.styles.Muted.Render("  "+info))
			}
		} else {
			sections = append(sections, "")
			sections = append(sections, a.styles.Muted.Render("  No holdings"))
			sections = append(sections, "")
			sections = append(sections, a.styles.Muted.Render("  Press 'r' to switch to register view"))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// selectedHolding returns the currently selected holding based on the table cursor.
func (a *App) selectedHolding() *investment.Holding {
	if a.portfolioData == nil || a.portfolioData.valuation == nil || a.portfolioHoldingsTable == nil {
		return nil
	}

	cursor := a.portfolioHoldingsTable.Cursor()
	holdings := a.portfolioData.valuation.Holdings
	if cursor < 0 || cursor >= len(holdings) {
		return nil
	}
	return &holdings[cursor]
}

// handlePortfolioKeys handles key presses in the portfolio view.
func (a *App) handlePortfolioKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			a.setPortfolioTableFocused(true)
		} else {
			a.sidebar.SetFocused(true)
			a.setPortfolioTableFocused(false)
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// Table-focused key handling
	if a.portfolioData == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.activePortfolioTable().MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.activePortfolioTable().MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.activePortfolioTable().MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.activePortfolioTable().MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.activePortfolioTable().PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.activePortfolioTable().PageDown(tableHeight)
	case key.Matches(msg, a.keys.Enter):
		// Drill down into lot detail for lot-tracking accounts
		if a.portfolioMode == portfolioViewHoldings {
			h := a.selectedHolding()
			if h != nil && a.portfolioData.account.TrackLots {
				a.portfolioMode = portfolioViewLots
				return a, a.loadLotDetail(a.portfolioData.account.ID, h.SecurityID)
			}
		}
	case key.Matches(msg, a.keys.Escape):
		if a.portfolioMode == portfolioViewLots {
			// Go back to holdings
			a.portfolioMode = portfolioViewHoldings
			a.portfolioData.lotDetails = nil
			a.portfolioData.lotSecurityID = types.NilID
			if a.portfolioHoldingsTable != nil {
				a.portfolioHoldingsTable.SetFocused(true)
			}
			if a.portfolioLotsTable != nil {
				a.portfolioLotsTable.SetFocused(false)
			}
			return a, nil
		}
		// Escape from holdings goes back to investment register
		a.switchView(ViewInvestmentRegister)
		return a, a.loadInvestmentRegisterData(a.portfolioData.account.ID)
	case msg.String() == "r":
		// Switch to register view
		a.switchView(ViewInvestmentRegister)
		return a, a.loadInvestmentRegisterData(a.portfolioData.account.ID)
	case msg.String() == "s":
		// Open stock split dialog pre-selected to the highlighted holding's security
		if a.portfolioMode == portfolioViewHoldings {
			if h := a.selectedHolding(); h != nil {
				secID := h.SecurityID
				a.stockSplitDialogPreSelectedID = &secID
				return a, a.loadStockSplitDialogData()
			}
		}
	}

	return a, nil
}

// activePortfolioTable returns whichever portfolio table is currently active.
func (a *App) activePortfolioTable() *Table {
	if a.portfolioMode == portfolioViewLots && a.portfolioLotsTable != nil {
		return a.portfolioLotsTable
	}
	if a.portfolioHoldingsTable != nil {
		return a.portfolioHoldingsTable
	}
	// Return a placeholder to avoid nil panics
	return NewTable(nil)
}

// setPortfolioTableFocused sets focus on the appropriate portfolio table.
func (a *App) setPortfolioTableFocused(focused bool) {
	if a.portfolioMode == portfolioViewLots {
		if a.portfolioLotsTable != nil {
			a.portfolioLotsTable.SetFocused(focused)
		}
	} else {
		if a.portfolioHoldingsTable != nil {
			a.portfolioHoldingsTable.SetFocused(focused)
		}
	}
}

// portfolioShortcuts returns the shortcut section for the portfolio help overlay.
func portfolioShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Portfolio",
		Entries: []shortcutEntry{
			{Key: "Enter", Description: "Lot detail (lot-tracking)"},
			{Key: "s", Description: "Stock split for selected position"},
			{Key: "r", Description: "Switch to register"},
			{Key: "Tab", Description: "Switch sidebar/table"},
			{Key: "Esc", Description: "Go back"},
		},
	}
}
