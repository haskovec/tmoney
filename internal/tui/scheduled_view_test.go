package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

func TestApp_Update_ScheduledDueCount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	// Test with 3 due transactions
	msg := scheduledDueCountMsg{count: 3}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("scheduledDueCountMsg should not return a command")
	}
	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "3 scheduled due" {
		t.Errorf("notification text = %q, want %q", notifications[0].Text, "3 scheduled due")
	}
	if notifications[0].Level != NotificationAlert {
		t.Errorf("notification level = %d, want %d", notifications[0].Level, NotificationAlert)
	}
}

func TestApp_Update_ScheduledDueCount_Single(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := scheduledDueCountMsg{count: 1}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "1 scheduled due" {
		t.Errorf("notification text = %q, want %q", notifications[0].Text, "1 scheduled due")
	}
}

func TestApp_Update_ScheduledDueCount_Zero(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	// Add a notification first, then clear with count 0
	app.statusbar.AddNotification("old", NotificationInfo)

	msg := scheduledDueCountMsg{count: 0}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if len(updatedApp.statusbar.Notifications()) != 0 {
		t.Errorf("expected 0 notifications for count=0, got %d", len(updatedApp.statusbar.Notifications()))
	}
}

func TestApp_RenderScheduled_Loading(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		styles:      styles,
		scheduled:   nil,
	}

	view := app.renderScheduled()
	if !contains(view, "Loading") {
		t.Errorf("renderScheduled() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderScheduled_Empty(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		styles:      styles,
		scheduled: &scheduledViewData{
			allTxns:       nil,
			dueCount:      0,
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
	}

	view := app.renderScheduled()
	if !contains(view, "No scheduled transactions") {
		t.Error("renderScheduled() should show 'No scheduled transactions' when empty")
	}
	if !contains(view, "SCHEDULED TRANSACTIONS") {
		t.Error("renderScheduled() should contain title 'SCHEDULED TRANSACTIONS'")
	}
}

func TestApp_RenderScheduled_WithDueAndUpcoming(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	payeeID1 := types.NewID()
	payeeID2 := types.NewID()
	accountID := types.NewID()

	dueTxn := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: accountID,
		Frequency: scheduled.FrequencyMonthly,
		NextDate:  types.Today(),
		PayeeID:   types.NullableID{ID: payeeID1, Valid: true},
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
	}

	upcomingTxn := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: accountID,
		Frequency: scheduled.FrequencyWeekly,
		NextDate:  types.Today().AddDays(7),
		PayeeID:   types.NullableID{ID: payeeID2, Valid: true},
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-50.00"), Valid: true},
	}

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      styles,
		scheduled: &scheduledViewData{
			dueTxns:       []*scheduled.Transaction{dueTxn},
			upcomingTxns:  []*scheduled.Transaction{upcomingTxn},
			allTxns:       []*scheduled.Transaction{dueTxn, upcomingTxn},
			dueCount:      1,
			payeeNames:    map[types.ID]string{payeeID1: "Landlord", payeeID2: "Netflix"},
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()
	view := app.renderScheduled()

	if !contains(view, "SCHEDULED TRANSACTIONS") {
		t.Error("renderScheduled() should contain title")
	}
	if !contains(view, "1 due") {
		t.Error("renderScheduled() should show '1 due' count")
	}
	if !contains(view, "Landlord") {
		t.Error("renderScheduled() should contain payee 'Landlord'")
	}
	if !contains(view, "Netflix") {
		t.Error("renderScheduled() should contain payee 'Netflix'")
	}
	if !contains(view, "$1500.00") {
		t.Error("renderScheduled() should contain amount '$1500.00'")
	}
	if !contains(view, "Monthly") {
		t.Error("renderScheduled() should contain frequency 'Monthly'")
	}
	if !contains(view, "Checking") {
		t.Error("renderScheduled() should contain account 'Checking'")
	}
}

func TestApp_BuildScheduledTable(t *testing.T) {
	payeeID := types.NewID()
	accountID := types.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  types.Today(),
					PayeeID:   types.NullableID{ID: payeeID, Valid: true},
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-100.00"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    map[types.ID]string{payeeID: "Electric Co"},
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	if app.scheduledTable == nil {
		t.Fatal("scheduledTable should be created")
	}
	if app.scheduledTable.RowCount() != 1 {
		t.Errorf("expected 1 row, got %d", app.scheduledTable.RowCount())
	}

	row := app.scheduledTable.SelectedRow()
	if row == nil {
		t.Fatal("selected row should not be nil")
	}

	// Check row content: Status, Date, Payee, Amount, Frequency, Account
	if row[0] != " ●" {
		t.Errorf("status = %q, want %q (due today)", row[0], " ●")
	}
	if row[2] != "Electric Co" {
		t.Errorf("payee = %q, want %q", row[2], "Electric Co")
	}
	if row[3] != "-$100.00" {
		t.Errorf("amount = %q, want %q", row[3], "-$100.00")
	}
	if row[4] != "Monthly" {
		t.Errorf("frequency = %q, want %q", row[4], "Monthly")
	}
	if row[5] != "Checking" {
		t.Errorf("account = %q, want %q", row[5], "Checking")
	}
}

func TestApp_BuildScheduledTable_VariableAmount(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  types.Today(),
					// No amount set - variable
				},
			},
			dueCount:      1,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[3] != "~variable" {
		t.Errorf("amount = %q, want %q for variable amount", row[3], "~variable")
	}
}

func TestApp_BuildScheduledTable_OverdueIndicator(t *testing.T) {
	accountID := types.NewID()
	pastDate := types.Today().AddDays(-3)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  pastDate,
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-50"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != "!●" {
		t.Errorf("status = %q, want %q for overdue", row[0], "!●")
	}
}

func TestApp_BuildScheduledTable_UpcomingIndicator(t *testing.T) {
	accountID := types.NewID()
	futureDate := types.Today().AddDays(7)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyWeekly,
					NextDate:  futureDate,
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-25"), Valid: true},
				},
			},
			dueCount:      0, // not due, so index 0 >= dueCount (0)
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != " ○" {
		t.Errorf("status = %q, want %q for upcoming", row[0], " ○")
	}
}

func TestApp_HandleScheduledKeys_TableNavigation(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyMonthly, NextDate: types.Today()},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyWeekly, NextDate: types.Today()},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyYearly, NextDate: types.Today()},
			},
			dueCount:      3,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused, sidebar not
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Move down
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.Update(downKey)
	if app.scheduledTable.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", app.scheduledTable.Cursor())
	}

	// Move down again
	app.Update(downKey)
	if app.scheduledTable.Cursor() != 2 {
		t.Errorf("cursor should be 2 after two downs, got %d", app.scheduledTable.Cursor())
	}

	// Move up
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	app.Update(upKey)
	if app.scheduledTable.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", app.scheduledTable.Cursor())
	}
}

func TestApp_HandleScheduledKeys_TabFocus(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{},
			dueCount:      0,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Tab should switch focus to sidebar
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.Update(tabKey)

	if !app.sidebar.IsFocused() {
		t.Error("sidebar should be focused after Tab")
	}
	if app.scheduledTable.IsFocused() {
		t.Error("scheduled table should not be focused after Tab")
	}

	// Tab again should switch back to table
	app.Update(tabKey)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused after second Tab")
	}
	if !app.scheduledTable.IsFocused() {
		t.Error("scheduled table should be focused after second Tab")
	}
}

func TestApp_Update_ScheduledViewDataLoaded(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	accountID := types.NewID()
	payeeID := types.NewID()

	data := &scheduledViewData{
		allTxns: []*scheduled.Transaction{
			{
				BaseModel: types.BaseModel{ID: types.NewID()},
				AccountID: accountID,
				Frequency: scheduled.FrequencyMonthly,
				NextDate:  types.Today(),
				PayeeID:   types.NullableID{ID: payeeID, Valid: true},
				Amount:    types.NullableMoney{Money: types.MustNewMoney("-100"), Valid: true},
			},
		},
		dueCount:      1,
		payeeNames:    map[types.ID]string{payeeID: "Landlord"},
		accountNames:  map[types.ID]string{accountID: "Checking"},
		categoryNames: make(map[types.ID]string),
	}

	msg := scheduledViewDataLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("scheduledViewDataLoadedMsg should not return a command")
	}
	if updatedApp.scheduled == nil {
		t.Fatal("scheduled data should be set")
	}
	if len(updatedApp.scheduled.allTxns) != 1 {
		t.Errorf("expected 1 scheduled txn, got %d", len(updatedApp.scheduled.allTxns))
	}
	if updatedApp.scheduledTable == nil {
		t.Fatal("scheduled table should be created")
	}
	if updatedApp.scheduledTable.RowCount() != 1 {
		t.Errorf("scheduled table row count = %d, want 1", updatedApp.scheduledTable.RowCount())
	}
}

func TestApp_FormatScheduledRow_AllFrequencies(t *testing.T) {
	accountID := types.NewID()

	frequencies := []struct {
		freq     scheduled.Frequency
		expected string
	}{
		{scheduled.FrequencyDaily, "Daily"},
		{scheduled.FrequencyWeekly, "Weekly"},
		{scheduled.FrequencyBiweekly, "Biweekly"},
		{scheduled.FrequencyMonthly, "Monthly"},
		{scheduled.FrequencyQuarterly, "Quarterly"},
		{scheduled.FrequencyYearly, "Yearly"},
	}

	for _, tt := range frequencies {
		t.Run(string(tt.freq), func(t *testing.T) {
			app := &App{
				styles: NewStyles(),
				scheduled: &scheduledViewData{
					payeeNames:    make(map[types.ID]string),
					accountNames:  map[types.ID]string{accountID: "Checking"},
					categoryNames: make(map[types.ID]string),
				},
			}

			st := &scheduled.Transaction{
				BaseModel: types.BaseModel{ID: types.NewID()},
				AccountID: accountID,
				Frequency: tt.freq,
				NextDate:  types.Today(),
				Amount:    types.NullableMoney{Money: types.MustNewMoney("-25"), Valid: true},
			}

			row := app.formatScheduledRow(st, false)
			if row[4] != tt.expected {
				t.Errorf("frequency = %q, want %q", row[4], tt.expected)
			}
		})
	}
}
