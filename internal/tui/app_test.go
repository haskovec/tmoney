package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

func TestViewString(t *testing.T) {
	tests := []struct {
		view     View
		expected string
	}{
		{ViewDashboard, "Dashboard"},
		{ViewRegister, "Register"},
		{ViewScheduled, "Scheduled"},
		{ViewReports, "Reports"},
		{View(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.view.String(); got != tt.expected {
				t.Errorf("View.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestApp_Init(t *testing.T) {
	// Create a minimal app without database for testing Init
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	cmd := app.Init()
	if cmd == nil {
		t.Error("Init() should return a command")
	}
}

func TestApp_Update_WindowSize(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
	}

	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.width != 80 {
		t.Errorf("width = %d, want 80", updatedApp.width)
	}
	if updatedApp.height != 24 {
		t.Errorf("height = %d, want 24", updatedApp.height)
	}
	if !updatedApp.ready {
		t.Error("ready should be true after WindowSizeMsg")
	}
	if cmd != nil {
		t.Error("WindowSizeMsg should not return a command")
	}
}

func TestApp_Update_QuitKey(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
	}

	msg := tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if !updatedApp.quitting {
		t.Error("quitting should be true after Ctrl+Q")
	}
	if cmd == nil {
		t.Error("Quit should return a tea.Quit command")
	}
}

func TestApp_Update_ViewSwitchKeys(t *testing.T) {
	tests := []struct {
		name         string
		key          tea.KeyMsg
		expectedView View
	}{
		{"Dashboard key", tea.KeyPressMsg{Code: '1', Text: "1"}, ViewDashboard},
		{"Scheduled key", tea.KeyPressMsg{Code: '2', Text: "2"}, ViewScheduled},
		{"Reports key", tea.KeyPressMsg{Code: '3', Text: "3"}, ViewReports},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				currentView: ViewDashboard,
				keys:        defaultKeyMap(),
				menubar:     NewMenuBar(),
				statusbar:   NewStatusBar(),
			}

			model, _ := app.Update(tt.key)
			updatedApp := model.(*App)

			if updatedApp.currentView != tt.expectedView {
				t.Errorf("currentView = %v, want %v", updatedApp.currentView, tt.expectedView)
			}
		})
	}
}

func TestApp_Update_EscapeKey(t *testing.T) {
	app := &App{
		currentView:  ViewRegister,
		previousView: ViewDashboard,
		keys:         defaultKeyMap(),
		menubar:      NewMenuBar(),
		statusbar:    NewStatusBar(),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("currentView = %v, want %v (should go back to previous)", updatedApp.currentView, ViewDashboard)
	}
}

// =============================================================================
// Register View Tests
// =============================================================================

// =============================================================================
// Scheduled View Tests
// =============================================================================

// =============================================================================
// Reports View Tests
// =============================================================================

func TestApp_RenderReports_Loading(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports:     nil,
	}

	view := app.renderReports()
	if !contains(view, "Loading") {
		t.Errorf("renderReports() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderNetWorthReport(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate: types.Today().Time(),
				Assets: []report.AccountBalance{
					{Name: "Checking", Balance: types.MustNewMoney("5000.00")},
					{Name: "Savings", Balance: types.MustNewMoney("10000.00")},
				},
				Liabilities: []report.AccountBalance{
					{Name: "Visa", Balance: types.MustNewMoney("1500.00")},
				},
				TotalAssets:      types.MustNewMoney("15000.00"),
				TotalLiabilities: types.MustNewMoney("1500.00"),
				NetWorth:         types.MustNewMoney("13500.00"),
			},
		},
	}

	view := app.renderNetWorthReport()

	if !contains(view, "NET WORTH REPORT") {
		t.Error("renderNetWorthReport() should contain 'NET WORTH REPORT'")
	}
	if !contains(view, "$13500.00") {
		t.Error("renderNetWorthReport() should contain net worth '$13500.00'")
	}
	if !contains(view, "ASSETS") {
		t.Error("renderNetWorthReport() should contain 'ASSETS'")
	}
	if !contains(view, "LIABILITIES") {
		t.Error("renderNetWorthReport() should contain 'LIABILITIES'")
	}
	if !contains(view, "Checking") {
		t.Error("renderNetWorthReport() should contain 'Checking'")
	}
	if !contains(view, "Savings") {
		t.Error("renderNetWorthReport() should contain 'Savings'")
	}
	if !contains(view, "Visa") {
		t.Error("renderNetWorthReport() should contain 'Visa'")
	}
}

func TestApp_RenderNetWorthReport_NegativeNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate:         types.Today().Time(),
				Assets:           nil,
				Liabilities:      []report.AccountBalance{{Name: "Loan", Balance: types.MustNewMoney("5000.00")}},
				TotalAssets:      types.MustNewMoney("0"),
				TotalLiabilities: types.MustNewMoney("5000.00"),
				NetWorth:         types.MustNewMoney("-5000.00"),
			},
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "-$5000.00") {
		t.Error("renderNetWorthReport() should show negative net worth")
	}
}

func TestApp_RenderNetWorthReport_NoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeNetWorth,
			netWorth: nil,
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "No net worth data") {
		t.Error("renderNetWorthReport() should show 'No net worth data' when nil")
	}
}

func TestApp_RenderSpendingReport(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 1,
			spending: &report.Spending{
				Period:        "January 2024",
				TotalSpending: types.MustNewMoney("3000.00"),
				Categories: []report.CategorySpending{
					{
						Name:       "Housing",
						Amount:     types.MustNewMoney("1500.00"),
						Percentage: 50.0,
						Subcategories: []report.CategorySpending{
							{Name: "Rent", Amount: types.MustNewMoney("1500.00")},
						},
					},
					{
						Name:       "Food",
						Amount:     types.MustNewMoney("1000.00"),
						Percentage: 33.3,
						Subcategories: []report.CategorySpending{
							{Name: "Groceries", Amount: types.MustNewMoney("700.00")},
							{Name: "Restaurants", Amount: types.MustNewMoney("300.00")},
						},
					},
					{
						Name:       "Transportation",
						Amount:     types.MustNewMoney("500.00"),
						Percentage: 16.7,
					},
				},
			},
		},
	}

	view := app.renderSpendingReport()

	if !contains(view, "SPENDING BY CATEGORY") {
		t.Error("renderSpendingReport() should contain 'SPENDING BY CATEGORY'")
	}
	if !contains(view, "January 2024") {
		t.Error("renderSpendingReport() should contain 'January 2024'")
	}
	if !contains(view, "Housing") {
		t.Error("renderSpendingReport() should contain 'Housing'")
	}
	if !contains(view, "Food") {
		t.Error("renderSpendingReport() should contain 'Food'")
	}
	if !contains(view, "Transportation") {
		t.Error("renderSpendingReport() should contain 'Transportation'")
	}
	if !contains(view, "Rent") {
		t.Error("renderSpendingReport() should contain subcategory 'Rent'")
	}
	if !contains(view, "Groceries") {
		t.Error("renderSpendingReport() should contain subcategory 'Groceries'")
	}
	if !contains(view, "TOTAL") {
		t.Error("renderSpendingReport() should contain 'TOTAL'")
	}
	if !contains(view, "$3000.00") {
		t.Error("renderSpendingReport() should contain total '$3000.00'")
	}
}

func TestApp_RenderSpendingReport_Empty(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
			spending: &report.Spending{
				Period:        "June 2024",
				Categories:    nil,
				TotalSpending: types.ZeroMoney,
			},
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "No spending data") {
		t.Error("renderSpendingReport() should show 'No spending data' when empty")
	}
}

func TestApp_RenderSpendingReport_NoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeSpending,
			spending: nil,
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "No spending data") {
		t.Error("renderSpendingReport() should show 'No spending data' when nil")
	}
}

func TestRenderSpendingBar(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		maxWidth int
		filled   int
		unfilled int
	}{
		{"50% of 20", 50.0, 20, 10, 10},
		{"100% of 10", 100.0, 10, 10, 0},
		{"0% of 10", 0.0, 10, 0, 10},
		{"25% of 20", 25.0, 20, 5, 15},
		{"zero width", 50.0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := renderSpendingBar(tt.pct, tt.maxWidth)
			if tt.maxWidth == 0 {
				if bar != "" {
					t.Errorf("expected empty string for zero width, got %q", bar)
				}
				return
			}
			expectedLen := tt.maxWidth
			// Count runes since we use Unicode block chars
			runeCount := 0
			for range bar {
				runeCount++
			}
			if runeCount != expectedLen {
				t.Errorf("bar length = %d runes, want %d", runeCount, expectedLen)
			}
		})
	}
}

func TestApp_GetAdjacentPeriods_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 3, // March
		},
	}

	prev, next := app.getAdjacentPeriods()
	if !contains(prev, "Feb") || !contains(prev, "2024") {
		t.Errorf("previous period = %q, want Feb 2024", prev)
	}
	if !contains(next, "Apr") || !contains(next, "2024") {
		t.Errorf("next period = %q, want Apr 2024", next)
	}
}

func TestApp_GetAdjacentPeriods_MonthlyYearWrap(t *testing.T) {
	// January wraps to December of previous year
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 1,
		},
	}

	prev, next := app.getAdjacentPeriods()
	if !contains(prev, "Dec") || !contains(prev, "2023") {
		t.Errorf("previous period = %q, want Dec 2023", prev)
	}
	if !contains(next, "Feb") || !contains(next, "2024") {
		t.Errorf("next period = %q, want Feb 2024", next)
	}

	// December wraps to January of next year
	app.reports.month = 12
	prev, next = app.getAdjacentPeriods()
	if !contains(prev, "Nov") || !contains(prev, "2024") {
		t.Errorf("previous period = %q, want Nov 2024", prev)
	}
	if !contains(next, "Jan") || !contains(next, "2025") {
		t.Errorf("next period = %q, want Jan 2025", next)
	}
}

func TestApp_GetAdjacentPeriods_Yearly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 0, // yearly
		},
	}

	prev, next := app.getAdjacentPeriods()
	if prev != "2023" {
		t.Errorf("previous period = %q, want %q", prev, "2023")
	}
	if next != "2025" {
		t.Errorf("next period = %q, want %q", next, "2025")
	}
}

func TestApp_GetAdjacentPeriods_Nil(t *testing.T) {
	app := &App{
		reports: nil,
	}

	prev, next := app.getAdjacentPeriods()
	if prev != "" || next != "" {
		t.Errorf("expected empty strings for nil reports, got %q, %q", prev, next)
	}
}

func TestApp_HandleReportsKeys_SwitchReportTypes(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			year:  2024,
			month: 6,
		},
	}

	// Press 's' to switch to spending
	sKey := tea.KeyPressMsg{Code: 's', Text: "s"}
	_, cmd := app.Update(sKey)
	if cmd == nil {
		t.Error("pressing 's' should return a command to load spending data")
	}

	// Now set to spending and press 'n' to switch to net worth
	app.reports.rtype = reportTypeSpending
	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd = app.Update(nKey)
	if cmd == nil {
		t.Error("pressing 'n' should return a command to load net worth data")
	}
}

func TestApp_HandleReportsKeys_PeriodNavigation(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
		},
	}

	// Press left to go to previous period
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd == nil {
		t.Error("pressing left should return a command for previous period")
	}

	// Press right to go to next period
	rightKey := tea.KeyPressMsg{Code: tea.KeyRight}
	_, cmd = app.Update(rightKey)
	if cmd == nil {
		t.Error("pressing right should return a command for next period")
	}
}

func TestApp_HandleReportsKeys_PeriodNav_NetWorthIgnored(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			year:  2024,
			month: 6,
		},
	}

	// Period navigation should be ignored for net worth reports
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd != nil {
		t.Error("period navigation should be ignored for net worth reports")
	}
}

func TestApp_HandleReportsKeys_YearlyToggle(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
		},
	}

	// Press 'y' to toggle to yearly view
	yKey := tea.KeyPressMsg{Code: 'y', Text: "y"}
	_, cmd := app.Update(yKey)
	if cmd == nil {
		t.Error("pressing 'y' should return a command to load yearly data")
	}
}

func TestApp_HandleReportsKeys_MonthlyToggle(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 0, // yearly
		},
	}

	// Press 'm' to toggle to monthly view
	mKey := tea.KeyPressMsg{Code: 'm', Text: "m"}
	_, cmd := app.Update(mKey)
	if cmd == nil {
		t.Error("pressing 'm' should return a command to load monthly data")
	}
}

func TestApp_HandleReportsKeys_NilReports(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports:     nil,
	}

	// Should not panic
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd != nil {
		t.Error("should return nil command when reports is nil")
	}
}

func TestApp_Update_ReportsViewDataLoaded(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	data := &reportsViewData{
		rtype: reportTypeNetWorth,
		netWorth: &report.NetWorth{
			NetWorth: types.MustNewMoney("10000"),
		},
	}

	msg := reportsViewDataLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("reportsViewDataLoadedMsg should not return a command")
	}
	if updatedApp.reports == nil {
		t.Fatal("reports data should be set")
	}
	if updatedApp.reports.rtype != reportTypeNetWorth {
		t.Errorf("report type = %v, want net worth", updatedApp.reports.rtype)
	}
}

func TestApp_ReportsPreviousPeriod_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 3,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd == nil {
		t.Error("reportsPreviousPeriod should return a command")
	}
}

func TestApp_ReportsPreviousPeriod_MonthlyJanuaryWrap(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 1,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd == nil {
		t.Error("reportsPreviousPeriod should return a command for January wrap")
	}
}

func TestApp_ReportsNextPeriod_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 3,
		},
	}

	_, cmd := app.reportsNextPeriod()
	if cmd == nil {
		t.Error("reportsNextPeriod should return a command")
	}
}

func TestApp_ReportsNextPeriod_MonthlyDecemberWrap(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 12,
		},
	}

	_, cmd := app.reportsNextPeriod()
	if cmd == nil {
		t.Error("reportsNextPeriod should return a command for December wrap")
	}
}

func TestApp_ReportsPeriodNav_Nil(t *testing.T) {
	app := &App{
		reports: nil,
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd != nil {
		t.Error("reportsPreviousPeriod should return nil for nil reports")
	}

	_, cmd = app.reportsNextPeriod()
	if cmd != nil {
		t.Error("reportsNextPeriod should return nil for nil reports")
	}
}

func TestApp_ReportsPeriodNav_NetWorthIgnored(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd != nil {
		t.Error("reportsPreviousPeriod should return nil for net worth")
	}

	_, cmd = app.reportsNextPeriod()
	if cmd != nil {
		t.Error("reportsNextPeriod should return nil for net worth")
	}
}

func TestApp_RenderReports_DispatchesCorrectly(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	// Test net worth dispatch
	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate:         types.Today().Time(),
				TotalAssets:      types.MustNewMoney("1000"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000"),
			},
		},
	}

	view := app.renderReports()
	if !contains(view, "NET WORTH REPORT") {
		t.Error("renderReports() should dispatch to net worth report")
	}

	// Test spending dispatch
	app.reports = &reportsViewData{
		rtype: reportTypeSpending,
		year:  2024,
		month: 1,
		spending: &report.Spending{
			Period:        "January 2024",
			Categories:    nil,
			TotalSpending: types.ZeroMoney,
		},
	}

	view = app.renderReports()
	if !contains(view, "SPENDING BY CATEGORY") {
		t.Error("renderReports() should dispatch to spending report")
	}
}

// =============================================================================
// Error Display and Dismissal Tests
// =============================================================================

func TestApp_Update_ErrorDismissedByKeyPress(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// Any key press should clear the error
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after key press")
	}
	if cmd != nil {
		t.Error("dismissing error should not return a command")
	}
}

func TestApp_Update_ErrorDismissedByEnter(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Enter key")
	}
}

func TestApp_Update_ErrorDismissedByEscape(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Escape key")
	}
}

func TestApp_Update_ErrorDismissedBySpace(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeySpace}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Space key")
	}
}

func TestApp_Update_ErrorDoesNotQuit(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// Ctrl+Q should dismiss the error, not quit the app
	msg := tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Ctrl+Q")
	}
	if updatedApp.quitting {
		t.Error("app should not quit when dismissing an error")
	}
	if cmd != nil {
		t.Error("dismissing error should not return a command")
	}
}

func TestApp_Update_ErrMsg(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := errMsg{err: fmt.Errorf("test error")}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err == nil {
		t.Error("err should be set after errMsg")
	}
	if updatedApp.err.Error() != "test error" {
		t.Errorf("err = %q, want %q", updatedApp.err.Error(), "test error")
	}
	if cmd != nil {
		t.Error("errMsg should not return a command")
	}
}

func TestApp_Update_ErrorThenNormalOperation(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// First key press dismisses the error
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.err != nil {
		t.Fatal("error should be cleared after first key press")
	}

	// Second key press should work normally (not get stuck)
	msg = tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := updatedApp.Update(msg)
	updatedApp = model.(*App)

	if !updatedApp.quitting {
		t.Error("app should quit on Ctrl+Q after error is dismissed")
	}
	if cmd == nil {
		t.Error("Ctrl+Q should return tea.Quit command")
	}
}

func TestApp_RenderNetWorthReport_ImprovedNoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeNetWorth,
			netWorth: nil,
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "Add accounts to get started") {
		t.Error("renderNetWorthReport() should show helpful message when nil")
	}
}

func TestApp_RenderSpendingReport_ImprovedNoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeSpending,
			spending: nil,
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "Add transactions to see reports") {
		t.Error("renderSpendingReport() should show helpful message when nil")
	}
}

// =============================================================================
// Transaction Status TUI Tests (Task 062)
// =============================================================================

// =============================================================================
// Dashboard Investment Holdings Tests (SM-175)
// =============================================================================

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestApp_Update_ToastClearMsg covers TH-031's clearing leg: a
// ToastClearMsg (delivered by the tea.Cmd ClearToastCmd produces) must
// drop whatever toast is currently set on the status bar. We pre-set a
// toast on a minimally-constructed App, dispatch the message through
// Update, and assert the status bar is back to no-toast state.
func TestApp_Update_ToastClearMsg(t *testing.T) {
	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		styles:      NewStyles(),
		width:       80,
		height:      24,
	}
	a.statusbar.SetToast("hello", NotificationInfo)
	if a.statusbar.Toast() == nil {
		t.Fatal("precondition: SetToast did not register a toast")
	}

	model, cmd := a.Update(ToastClearMsg{})
	if cmd != nil {
		t.Errorf("Update(ToastClearMsg) cmd = %T, want nil", cmd)
	}
	got := model.(*App)
	if got.statusbar.Toast() != nil {
		t.Errorf("Toast() = %+v after ToastClearMsg, want nil", got.statusbar.Toast())
	}
}

