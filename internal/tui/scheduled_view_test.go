package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

func TestApp_Update_ScheduledDueCount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
	if notifications[0].Level != widget.NotificationAlert {
		t.Errorf("notification level = %d, want %d", notifications[0].Level, widget.NotificationAlert)
	}
}

func TestApp_Update_ScheduledDueCount_Single(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
	}

	// Add a notification first, then clear with count 0
	app.statusbar.AddNotification("old", widget.NotificationInfo)

	msg := scheduledDueCountMsg{count: 0}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if len(updatedApp.statusbar.Notifications()) != 0 {
		t.Errorf("expected 0 notifications for count=0, got %d", len(updatedApp.statusbar.Notifications()))
	}
}

func TestApp_RenderScheduled_Loading(t *testing.T) {
	styles := widget.NewStyles()
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
	styles := widget.NewStyles()
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
	styles := widget.NewStyles()
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles: widget.NewStyles(),
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
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		{scheduled.FrequencyFortnightly, "Fortnightly"},
		{scheduled.FrequencySemiMonthly, "Semi-Monthly"},
		{scheduled.FrequencyMonthly, "Monthly"},
		{scheduled.FrequencyQuarterly, "Quarterly"},
		{scheduled.FrequencyYearly, "Yearly"},
	}

	for _, tt := range frequencies {
		t.Run(string(tt.freq), func(t *testing.T) {
			app := &App{
				styles: widget.NewStyles(),
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

// TestScheduledView_EnterOnDueItem_OpensPreview covers MS-019: pressing
// Enter on a due scheduled item must open the preview dialog (replacing
// the legacy immediate-post path) so users can edit per-instance values
// before posting. The wiring is:
//
//  1. handleScheduledKeys' Enter branch returns the loadSchedulePreviewData
//     command instead of the old postSelectedScheduled command.
//  2. The command produces a schedulePreviewDataMsg.
//  3. Update's handler for that message constructs the
//     SchedulePreviewDialog and sets it on the App. No transaction is
//     posted and the schedule is not advanced — those happen at preview
//     Save time (MS-020).
func TestScheduledView_EnterOnDueItem_OpensPreview(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, payeeRepo, accountRepo, nil, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	// Transfer occurrences post through the transfer owner; production wires
	// this in app.NewServices (see scheduled/transfer_port.go).
	schedSvc.SetTransferPort(transfer.NewService(txnRepo,
		investment.NewRepository(database), transaction.NewSplitRepository(database), accountRepo,
		category.NewRepository(database), database))
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	payeeID := types.NewID()
	rentCat := category.NewCategory("Rent", category.TypeExpense)
	if err := categoryRepo.Create(rentCat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	// Build a due single-line scheduled transaction (next_date = today).
	dueTxn := &scheduled.Transaction{
		BaseModel:  types.BaseModel{ID: types.NewID()},
		AccountID:  acct.ID,
		Frequency:  scheduled.FrequencyMonthly,
		NextDate:   types.Today(),
		PayeeID:    types.NullableID{ID: payeeID, Valid: true},
		CategoryID: types.NullableID{ID: rentCat.ID, Valid: true},
		Amount:     types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
	}
	dueTxn.SetMemo("Monthly rent")

	app := &App{
		currentView:     ViewScheduled,
		width:           120,
		height:          30,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		styles:          widget.NewStyles(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{dueTxn},
			dueTxns:       []*scheduled.Transaction{dueTxn},
			dueCount:      1,
			payeeNames:    map[types.ID]string{payeeID: "Landlord"},
			accountNames:  map[types.ID]string{acct.ID: acct.Name},
			categoryNames: map[types.ID]string{rentCat.ID: "Rent"},
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Press Enter on the due item.
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, cmd := app.Update(enterKey)
	updated := model.(*App)

	if updated.schedPreviewDialog != nil {
		t.Fatal("preview dialog should not be set synchronously — the loader runs as a tea.Cmd")
	}
	if cmd == nil {
		t.Fatal("Enter on a due item must return a tea.Cmd to load preview data")
	}

	// Invoke the command. It must produce a schedulePreviewDataMsg, NOT
	// trigger a post (which would produce a scheduledPostedMsg).
	msg := cmd()
	if msg == nil {
		t.Fatal("loadSchedulePreviewData command should produce a message")
	}
	if _, ok := msg.(scheduledPostedMsg); ok {
		t.Fatal("Enter should not produce scheduledPostedMsg — preview replaces immediate post")
	}
	previewMsg, ok := msg.(schedulePreviewDataMsg)
	if !ok {
		t.Fatalf("expected schedulePreviewDataMsg, got %T", msg)
	}
	if previewMsg.template != dueTxn {
		t.Error("preview msg template should reference the selected due transaction")
	}

	// Dispatch the preview message to Update so it constructs the dialog.
	model, _ = updated.Update(previewMsg)
	final := model.(*App)

	if final.schedPreviewDialog == nil {
		t.Fatal("schedPreviewDialog should be set after schedulePreviewDataMsg")
	}
	if !final.schedPreviewDialog.IsVisible() {
		t.Error("preview dialog should be visible after construction")
	}
	if final.schedPreviewDialog.Template() != dueTxn {
		t.Error("preview dialog should reference the template that was clicked")
	}

	// Verify that no real transaction was posted and the schedule was
	// not advanced — both should wait until MS-020's save handler.
	posted, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 0 {
		t.Errorf("no transaction should have been posted when opening the preview; got %d", len(posted))
	}
	reloaded, err := schedRepo.GetByID(dueTxn.ID)
	if err == nil && reloaded != nil && !reloaded.NextDate.Equal(types.Today()) {
		t.Errorf("schedule next_date should be unchanged; got %s, want %s",
			reloaded.NextDate.Time().Format("2006-01-02"),
			types.Today().Time().Format("2006-01-02"))
	}
}
