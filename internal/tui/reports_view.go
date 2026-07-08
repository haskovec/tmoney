package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// reportType represents which report is being displayed.
type reportType int

const (
	reportTypeNetWorth reportType = iota
	reportTypeSpending
)

// reportsViewData holds the loaded data for the reports view.
type reportsViewData struct {
	rtype    reportType
	netWorth *report.NetWorth
	spending *report.Spending
	year     int
	month    int // 1-12 for monthly, 0 for yearly
	// includeTransfers folds categorized transfers into the spending report.
	// Session-only state (like the period), reset on a fresh entry to the view.
	includeTransfers bool
}

// reportsViewDataLoadedMsg is sent when reports data has been loaded.
type reportsViewDataLoadedMsg struct {
	data *reportsViewData
}

// loadReportsViewData returns a command that loads report data for the reports view.
func (a *App) loadReportsViewData(rt reportType, year, month int, includeTransfers bool) tea.Cmd {
	return func() tea.Msg {
		data := &reportsViewData{
			rtype:            rt,
			year:             year,
			month:            month,
			includeTransfers: includeTransfers,
		}

		switch rt {
		case reportTypeNetWorth:
			if a.reportSvc != nil {
				report, err := a.reportSvc.NetWorthReport()
				if err != nil {
					return errMsg{err: err}
				}
				data.netWorth = report
			}
		case reportTypeSpending:
			if a.reportSvc != nil {
				var report *report.Spending
				var err error
				if month > 0 {
					report, err = a.reportSvc.SpendingByCategoryMonth(year, month, includeTransfers)
				} else {
					report, err = a.reportSvc.SpendingByCategoryYear(year, includeTransfers)
				}
				if err != nil {
					return errMsg{err: err}
				}
				data.spending = report
			}
		}

		return reportsViewDataLoadedMsg{data: data}
	}
}

// handleReportsKeys handles key presses in the reports view.
func (a *App) handleReportsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.reports == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Left):
		// Navigate to previous period
		return a.reportsPreviousPeriod()

	case key.Matches(msg, a.keys.Right):
		// Navigate to next period
		return a.reportsNextPeriod()

	case msg.String() == "n":
		// Switch to net worth report
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()), a.reports.includeTransfers)

	case msg.String() == "s":
		// Switch to spending report
		year := a.reports.year
		month := a.reports.month
		if month == 0 {
			month = int(time.Now().Month())
		}
		return a, a.loadReportsViewData(reportTypeSpending, year, month, a.reports.includeTransfers)

	case msg.String() == "y":
		// Toggle to yearly spending view (only for spending)
		if a.reports.rtype == reportTypeSpending {
			return a, a.loadReportsViewData(reportTypeSpending, a.reports.year, 0, a.reports.includeTransfers)
		}

	case msg.String() == "m":
		// Toggle to monthly spending view (only for spending)
		if a.reports.rtype == reportTypeSpending && a.reports.month == 0 {
			return a, a.loadReportsViewData(reportTypeSpending, a.reports.year, int(time.Now().Month()), a.reports.includeTransfers)
		}

	case msg.String() == "t":
		// Toggle folding categorized transfers into the spending report
		if a.reports.rtype == reportTypeSpending {
			return a, a.loadReportsViewData(reportTypeSpending, a.reports.year, a.reports.month, !a.reports.includeTransfers)
		}
	}

	return a, nil
}

// reportsPreviousPeriod navigates to the previous time period for reports.
func (a *App) reportsPreviousPeriod() (tea.Model, tea.Cmd) {
	if a.reports == nil || a.reports.rtype != reportTypeSpending {
		return a, nil
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly: go to previous month
		month--
		if month < 1 {
			month = 12
			year--
		}
	} else {
		// Yearly: go to previous year
		year--
	}

	return a, a.loadReportsViewData(reportTypeSpending, year, month, a.reports.includeTransfers)
}

// reportsNextPeriod navigates to the next time period for reports.
func (a *App) reportsNextPeriod() (tea.Model, tea.Cmd) {
	if a.reports == nil || a.reports.rtype != reportTypeSpending {
		return a, nil
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly: go to next month
		month++
		if month > 12 {
			month = 1
			year++
		}
	} else {
		// Yearly: go to next year
		year++
	}

	return a, a.loadReportsViewData(reportTypeSpending, year, month, a.reports.includeTransfers)
}

// renderReports renders the reports view.
func (a *App) renderReports() string {
	if a.reports == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading reports...")
	}

	switch a.reports.rtype {
	case reportTypeNetWorth:
		return a.renderNetWorthReport()
	case reportTypeSpending:
		return a.renderSpendingReport()
	default:
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Unknown report type")
	}
}

// renderNetWorthReport renders the net worth report.
func (a *App) renderNetWorthReport() string {
	if a.reports.netWorth == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No net worth data available. Add accounts to get started.")
	}

	contentWidth := a.styles.ContentWidth()
	nw := a.reports.netWorth

	var sections []string

	// Title row: NET WORTH REPORT + date
	dateStr := nw.AsOfDate.Format("Jan 2, 2006")
	titleText := "NET WORTH REPORT"
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(dateStr)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render("As of: "+dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("═", sepWidth)))

	// Net worth summary
	nwLabel := "Net Worth:  "
	nwValue := formatDashboardMoney(nw.NetWorth)
	nwStyle := a.styles.Positive
	if nw.NetWorth.IsNegative() {
		nwStyle = a.styles.Negative
	}
	sections = append(sections, "")
	sections = append(sections, a.styles.Bold.Render(nwLabel)+nwStyle.Bold(true).Render(nwValue))
	sections = append(sections, "")

	// Assets and liabilities columns. nil: the Net Worth report has no
	// expand/collapse affordance, so no mouse hit-test rows are recorded.
	sections = append(sections, a.renderAssetLiabilityColumns(nw, contentWidth, nil))

	// Navigation hints
	sections = append(sections, "")
	sections = append(sections, a.styles.Muted.Render("  n net worth  s spending  esc back"))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderSpendingReport renders the spending by category report.
func (a *App) renderSpendingReport() string {
	if a.reports.spending == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No spending data available. Add transactions to see reports.")
	}

	contentWidth := a.styles.ContentWidth()
	sr := a.reports.spending

	var sections []string

	// Title row
	titleText := "SPENDING BY CATEGORY"
	if a.reports.includeTransfers {
		titleText += "  (incl. transfers)"
	}
	periodText := sr.Period
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(periodText)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Bold.Render(periodText)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("═", sepWidth)))

	if len(sr.Categories) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No spending data for this period"))
	} else {
		// widget.Column header
		tableWidth := max(contentWidth-4, 1)
		barWidth := max(
			// Reserve space for name(20), amount(12), percent(8), gaps(2)
			tableWidth-42, 4)

		headerLine := fmt.Sprintf("  %-20s %12s %7s  %s", "Category", "Amount", "% Total", "")
		sections = append(sections, a.styles.TableHeader.Render(headerLine))

		// Category rows
		for _, cat := range sr.Categories {
			// Parent category row with bar
			name := widget.Truncate(cat.Name, 20)
			amount := formatDashboardMoney(cat.Amount)
			pct := fmt.Sprintf("%.1f%%", cat.Percentage)
			bar := renderSpendingBar(cat.Percentage, barWidth)

			line := fmt.Sprintf("  %-20s %12s %7s  %s",
				a.styles.Bold.Render(name),
				a.styles.Negative.Render(amount),
				pct,
				a.styles.Negative.Render(bar))
			sections = append(sections, line)

			// Subcategory rows
			for _, sub := range cat.Subcategories {
				subName := "  " + widget.Truncate(sub.Name, 18)
				subAmount := formatDashboardMoney(sub.Amount)
				subLine := fmt.Sprintf("  %-20s %12s",
					a.styles.Muted.Render(subName),
					a.styles.Muted.Render(subAmount))
				sections = append(sections, subLine)
			}
		}

		// Total row
		sections = append(sections, a.styles.Muted.Render("  "+strings.Repeat("─", tableWidth-2)))
		totalAmount := formatDashboardMoney(sr.TotalSpending)
		totalLine := fmt.Sprintf("  %-20s %12s %7s",
			a.styles.Bold.Render("TOTAL"),
			a.styles.Negative.Bold(true).Render(totalAmount),
			"100.0%")
		sections = append(sections, totalLine)
	}

	// Period navigation
	sections = append(sections, "")
	prevPeriod, nextPeriod := a.getAdjacentPeriods()
	navLine := fmt.Sprintf("  %s  %s  %s",
		a.styles.Muted.Render(fmt.Sprintf("< %s", prevPeriod)),
		a.styles.Bold.Render(periodText),
		a.styles.Muted.Render(fmt.Sprintf("%s >", nextPeriod)))
	sections = append(sections, navLine)

	// Navigation hints
	modeHint := "m monthly"
	if a.reports.month > 0 {
		modeHint = "y yearly"
	}
	sections = append(sections, a.styles.Muted.Render(fmt.Sprintf("  <-> period  %s  t transfers  n net worth  s spending  esc back", modeHint)))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// getAdjacentPeriods returns display strings for the previous and next periods.
func (a *App) getAdjacentPeriods() (string, string) {
	if a.reports == nil {
		return "", ""
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly
		prevMonth := month - 1
		prevYear := year
		if prevMonth < 1 {
			prevMonth = 12
			prevYear--
		}
		nextMonth := month + 1
		nextYear := year
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		prev := time.Date(prevYear, time.Month(prevMonth), 1, 0, 0, 0, 0, time.UTC).Format("Jan 2006")
		next := time.Date(nextYear, time.Month(nextMonth), 1, 0, 0, 0, 0, time.UTC).Format("Jan 2006")
		return prev, next
	}

	// Yearly
	return fmt.Sprintf("%d", year-1), fmt.Sprintf("%d", year+1)
}

// renderSpendingBar renders a horizontal bar chart segment for spending percentage.
func renderSpendingBar(percentage float64, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	filled := max(min(int(math.Round(percentage/100.0*float64(maxWidth))), maxWidth), 0)
	return strings.Repeat("█", filled) + strings.Repeat("░", maxWidth-filled)
}
