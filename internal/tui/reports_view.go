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

// reportsViewState is everything the Reports view owns. Its zero value is the
// view before its first load.
type reportsViewState struct {
	data *reportsViewData
}

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

// reportsDeps is what the Reports view needs from outside itself. Every dep is
// a func, because switchDatabase replaces App's services and closes the
// previous *db.DB; and deps are passed to each call, never stored in the view
// state. Both rules are pinned by the guards that run over viewControllers.
type reportsDeps struct {
	reports func() *report.Service
}

// reportsDeps binds the Reports view to the services App owns. The accessor
// may return nil, because an App built by a test has no services, so each
// caller keeps its own nil guard.
func (a *App) reportsDeps() reportsDeps {
	return reportsDeps{
		reports: func() *report.Service { return a.services.Report },
	}
}

// load returns a command that loads report data for the reports view. The
// service is read through the deps when the command runs.
func (s *reportsViewState) load(d reportsDeps, rt reportType, year, month int, includeTransfers bool) tea.Cmd {
	return func() tea.Msg {
		data := &reportsViewData{
			rtype:            rt,
			year:             year,
			month:            month,
			includeTransfers: includeTransfers,
		}

		switch rt {
		case reportTypeNetWorth:
			if reports := d.reports(); reports != nil {
				report, err := reports.NetWorthReport()
				if err != nil {
					return errMsg{err: err}
				}
				data.netWorth = report
			}
		case reportTypeSpending:
			if reports := d.reports(); reports != nil {
				var report *report.Spending
				var err error
				if month > 0 {
					report, err = reports.SpendingByCategoryMonth(year, month, includeTransfers)
				} else {
					report, err = reports.SpendingByCategoryYear(year, includeTransfers)
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

// handleKey handles key presses in the reports view.
func (s *reportsViewState) handleKey(d reportsDeps, msg tea.KeyPressMsg, keys keyMap) tea.Cmd {
	if s.data == nil {
		return nil
	}

	switch {
	case key.Matches(msg, keys.Left):
		// Navigate to previous period
		return s.previousPeriod(d)

	case key.Matches(msg, keys.Right):
		// Navigate to next period
		return s.nextPeriod(d)

	case msg.String() == "n":
		// Switch to net worth report
		now := time.Now()
		return s.load(d, reportTypeNetWorth, now.Year(), int(now.Month()), s.data.includeTransfers)

	case msg.String() == "s":
		// Switch to spending report
		year := s.data.year
		month := s.data.month
		if month == 0 {
			month = int(time.Now().Month())
		}
		return s.load(d, reportTypeSpending, year, month, s.data.includeTransfers)

	case msg.String() == "y":
		// Toggle to yearly spending view (only for spending)
		if s.data.rtype == reportTypeSpending {
			return s.load(d, reportTypeSpending, s.data.year, 0, s.data.includeTransfers)
		}

	case msg.String() == "m":
		// Toggle to monthly spending view (only for spending)
		if s.data.rtype == reportTypeSpending && s.data.month == 0 {
			return s.load(d, reportTypeSpending, s.data.year, int(time.Now().Month()), s.data.includeTransfers)
		}

	case msg.String() == "t":
		// Toggle folding categorized transfers into the spending report
		if s.data.rtype == reportTypeSpending {
			return s.load(d, reportTypeSpending, s.data.year, s.data.month, !s.data.includeTransfers)
		}
	}

	return nil
}

// previousPeriod navigates to the previous time period for reports.
func (s *reportsViewState) previousPeriod(d reportsDeps) tea.Cmd {
	if s.data == nil || s.data.rtype != reportTypeSpending {
		return nil
	}

	year := s.data.year
	month := s.data.month

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

	return s.load(d, reportTypeSpending, year, month, s.data.includeTransfers)
}

// nextPeriod navigates to the next time period for reports.
func (s *reportsViewState) nextPeriod(d reportsDeps) tea.Cmd {
	if s.data == nil || s.data.rtype != reportTypeSpending {
		return nil
	}

	year := s.data.year
	month := s.data.month

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

	return s.load(d, reportTypeSpending, year, month, s.data.includeTransfers)
}

// render renders the reports view. dash is the Dashboard's state: the net-worth
// report's asset and liability columns show its holdings and its expanded
// accounts (the design's §8 asks whether they should).
func (s *reportsViewState) render(styles widget.Styles, dash *dashboardViewState) string {
	if s.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading reports...")
	}

	switch s.data.rtype {
	case reportTypeNetWorth:
		return s.renderNetWorth(styles, dash)
	case reportTypeSpending:
		return s.renderSpending(styles)
	default:
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Unknown report type")
	}
}

// renderNetWorth renders the net worth report.
func (s *reportsViewState) renderNetWorth(styles widget.Styles, dash *dashboardViewState) string {
	if s.data.netWorth == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No net worth data available. Add accounts to get started.")
	}

	contentWidth := styles.ContentWidth()
	nw := s.data.netWorth

	var sections []string

	// Title row: NET WORTH REPORT + date
	dateStr := nw.AsOfDate.Format("Jan 2, 2006")
	titleText := "NET WORTH REPORT"
	asOf := "As of: " + dateStr
	// Measure the text that is rendered, prefix included: sizing the gap from the
	// bare date once left the row seven cells over and wrapped the year.
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(asOf)-4, 1)
	titleRow := styles.Title.Render(titleText) + strings.Repeat(" ", padding) + styles.Muted.Render(asOf)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("═", sepWidth)))

	// Net worth summary, one line per currency
	sections = append(sections, "")
	sections = append(sections, renderNetWorthSummary(styles, nw)...)
	sections = append(sections, "")

	// Assets and liabilities columns. nil: the Net Worth report has no
	// expand/collapse affordance, so no mouse hit-test rows are recorded.
	sections = append(sections, dash.renderAssetLiabilityColumns(styles, nw, contentWidth, nil))

	// Navigation hints
	sections = append(sections, "")
	sections = append(sections, styles.Muted.Render("  n net worth  s spending  esc back"))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderSpending renders the spending by category report.
func (s *reportsViewState) renderSpending(styles widget.Styles) string {
	if s.data.spending == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No spending data available. Add transactions to see reports.")
	}

	contentWidth := styles.ContentWidth()
	sr := s.data.spending

	var sections []string

	// Title row
	titleText := "SPENDING BY CATEGORY"
	if s.data.includeTransfers {
		titleText += "  (incl. transfers)"
	}
	periodText := sr.Period
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(periodText)-4, 1)
	titleRow := styles.Title.Render(titleText) + strings.Repeat(" ", padding) + styles.Bold.Render(periodText)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("═", sepWidth)))

	if len(sr.Categories) == 0 {
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No spending data for this period"))
	} else {
		// widget.Column header
		tableWidth := max(contentWidth-4, 1)
		barWidth := max(
			// Reserve space for name(20), amount(12), percent(8), gaps(2)
			tableWidth-42, 4)

		headerLine := fmt.Sprintf("  %-20s %12s %7s  %s", "Category", "Amount", "% Total", "")
		sections = append(sections, styles.TableHeader.Render(headerLine))

		// Category rows
		for _, cat := range sr.Categories {
			// Parent category row with bar
			name := widget.Truncate(cat.Name, 20)
			amount := formatDashboardMoney(cat.Amount)
			pct := fmt.Sprintf("%.1f%%", cat.Percentage)
			bar := renderSpendingBar(cat.Percentage, barWidth)

			line := fmt.Sprintf("  %-20s %12s %7s  %s",
				styles.Bold.Render(name),
				styles.Negative.Render(amount),
				pct,
				styles.Negative.Render(bar))
			sections = append(sections, line)

			// Subcategory rows
			for _, sub := range cat.Subcategories {
				subName := "  " + widget.Truncate(sub.Name, 18)
				subAmount := formatDashboardMoney(sub.Amount)
				subLine := fmt.Sprintf("  %-20s %12s",
					styles.Muted.Render(subName),
					styles.Muted.Render(subAmount))
				sections = append(sections, subLine)
			}
		}

		// Total row
		sections = append(sections, styles.Muted.Render("  "+strings.Repeat("─", tableWidth-2)))
		totalAmount := formatDashboardMoney(sr.TotalSpending)
		totalLine := fmt.Sprintf("  %-20s %12s %7s",
			styles.Bold.Render("TOTAL"),
			styles.Negative.Bold(true).Render(totalAmount),
			"100.0%")
		sections = append(sections, totalLine)
	}

	// Period navigation
	sections = append(sections, "")
	prevPeriod, nextPeriod := s.adjacentPeriods()
	navLine := fmt.Sprintf("  %s  %s  %s",
		styles.Muted.Render(fmt.Sprintf("< %s", prevPeriod)),
		styles.Bold.Render(periodText),
		styles.Muted.Render(fmt.Sprintf("%s >", nextPeriod)))
	sections = append(sections, navLine)

	// Navigation hints
	modeHint := "m monthly"
	if s.data.month > 0 {
		modeHint = "y yearly"
	}
	sections = append(sections, styles.Muted.Render(fmt.Sprintf("  <-> period  %s  t transfers  n net worth  s spending  esc back", modeHint)))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// adjacentPeriods returns display strings for the previous and next periods.
func (s *reportsViewState) adjacentPeriods() (string, string) {
	if s.data == nil {
		return "", ""
	}

	year := s.data.year
	month := s.data.month

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
