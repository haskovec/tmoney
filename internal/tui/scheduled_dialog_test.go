package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestBuildFrequencyOptions(t *testing.T) {
	options := buildFrequencyOptions()

	allFreqs := scheduled.AllFrequencies()
	if len(options) != len(allFreqs) {
		t.Fatalf("expected %d options, got %d", len(allFreqs), len(options))
	}

	for i, f := range allFreqs {
		if options[i] != f.DisplayName() {
			t.Errorf("options[%d] = %q, want %q", i, options[i], f.DisplayName())
		}
	}
}

func TestFrequencyFromIndex(t *testing.T) {
	tests := []struct {
		index    int
		expected scheduled.Frequency
	}{
		{0, scheduled.FrequencyDaily},
		{1, scheduled.FrequencyWeekly},
		{2, scheduled.FrequencyBiweekly},
		{3, scheduled.FrequencyMonthly},
		{4, scheduled.FrequencyQuarterly},
		{5, scheduled.FrequencyYearly},
		{-1, scheduled.FrequencyMonthly},  // out of range defaults to monthly
		{100, scheduled.FrequencyMonthly}, // out of range defaults to monthly
	}

	for _, tc := range tests {
		got := frequencyFromIndex(tc.index)
		if got != tc.expected {
			t.Errorf("frequencyFromIndex(%d) = %q, want %q", tc.index, got, tc.expected)
		}
	}
}

func TestFrequencyToIndex(t *testing.T) {
	tests := []struct {
		freq     scheduled.Frequency
		expected int
	}{
		{scheduled.FrequencyDaily, 0},
		{scheduled.FrequencyWeekly, 1},
		{scheduled.FrequencyBiweekly, 2},
		{scheduled.FrequencyMonthly, 3},
		{scheduled.FrequencyQuarterly, 4},
		{scheduled.FrequencyYearly, 5},
		{scheduled.Frequency("unknown"), 3}, // unknown defaults to monthly index
	}

	for _, tc := range tests {
		got := frequencyToIndex(tc.freq)
		if got != tc.expected {
			t.Errorf("frequencyToIndex(%q) = %d, want %d", tc.freq, got, tc.expected)
		}
	}
}

func TestFrequencyRoundTrip(t *testing.T) {
	for i, f := range scheduled.AllFrequencies() {
		idx := frequencyToIndex(f)
		if idx != i {
			t.Errorf("frequencyToIndex(%q) = %d, want %d", f, idx, i)
		}
		back := frequencyFromIndex(idx)
		if back != f {
			t.Errorf("frequencyFromIndex(%d) = %q, want %q", idx, back, f)
		}
	}
}

func TestBuildNewScheduledDialog(t *testing.T) {
	accountOptions := []string{"Checking", "Savings"}
	categoryOptions := []string{"(None)", "Groceries", "Rent"}

	d := buildNewScheduledDialog(accountOptions, categoryOptions)

	if d.Title() != "New Scheduled Transaction" {
		t.Errorf("title = %q, want %q", d.Title(), "New Scheduled Transaction")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}

	fields := d.Fields()
	if len(fields) != 13 {
		t.Fatalf("expected 13 fields, got %d", len(fields))
	}

	if d.Width() != 62 {
		t.Errorf("width = %d, want 62", d.Width())
	}
}

func TestBuildNewScheduledDialog_FieldTypes(t *testing.T) {
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	d := buildNewScheduledDialog(accountOptions, categoryOptions)
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType FieldType
	}{
		{"Account", FieldSelect},
		{"Payee", FieldText},
		{"Category", FieldSelect},
		{"Amount", FieldText},
		{"Memo", FieldText},
		{"Frequency", FieldSelect},
		{"Interval", FieldText},
		{"Start Date", FieldText},
		{"Duration", FieldRadio},
		{"End Date", FieldText},
		{"Occurrences", FieldText},
		{"Auto-post", FieldCheckbox},
		{"Lead time", FieldRadio},
	}

	for i, exp := range expected {
		if fields[i].Label != exp.label {
			t.Errorf("field[%d] label = %q, want %q", i, fields[i].Label, exp.label)
		}
		if fields[i].Type != exp.fieldType {
			t.Errorf("field[%d] type = %v, want %v", i, fields[i].Type, exp.fieldType)
		}
	}
}

func TestBuildNewScheduledDialog_Defaults(t *testing.T) {
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	d := buildNewScheduledDialog(accountOptions, categoryOptions)
	fields := d.Fields()

	// Account default to first (index 0)
	if fields[schedFieldAccount].SelectedIndex != 0 {
		t.Errorf("account selectedIndex = %d, want 0", fields[schedFieldAccount].SelectedIndex)
	}

	// Frequency defaults to Monthly (index 3)
	if fields[schedFieldFrequency].SelectedIndex != 3 {
		t.Errorf("frequency selectedIndex = %d, want 3", fields[schedFieldFrequency].SelectedIndex)
	}

	// Interval defaults to "1"
	if fields[schedFieldInterval].Value != "1" {
		t.Errorf("interval = %q, want %q", fields[schedFieldInterval].Value, "1")
	}

	// Start date defaults to today
	today := time.Now().Format("01/02/2006")
	if fields[schedFieldStartDate].Value != today {
		t.Errorf("start date = %q, want %q", fields[schedFieldStartDate].Value, today)
	}

	// Duration defaults to Indefinite (index 0)
	if fields[schedFieldDuration].SelectedIndex != 0 {
		t.Errorf("duration selectedIndex = %d, want 0", fields[schedFieldDuration].SelectedIndex)
	}

	// Auto-post defaults to unchecked
	if fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should default to unchecked")
	}

	// Lead time defaults to "On the day" (index 0)
	if fields[schedFieldLeadDays].SelectedIndex != 0 {
		t.Errorf("lead days selectedIndex = %d, want 0", fields[schedFieldLeadDays].SelectedIndex)
	}
}

func TestBuildEditScheduledDialog(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()
	categoryID := types.NewID()
	amount := types.MustNewMoney("50.00")

	st := scheduled.NewTransactionFull(
		accountID,
		scheduled.FrequencyWeekly,
		types.NewDate(2024, time.March, 15),
		amount,
		payeeID,
		categoryID,
		"Test memo",
	)
	st.SetInterval(2)

	accountOptions := []string{"Checking", "Savings"}
	accountIDs := []types.ID{accountID, types.NewID()}
	categoryOptions := []string{"(None)", "Groceries"}
	categoryIDs := []types.ID{types.NilID, categoryID}
	payeeNames := map[types.ID]string{payeeID: "Test Payee"}

	d := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)

	if d.Title() != "Edit Scheduled Transaction" {
		t.Errorf("title = %q, want %q", d.Title(), "Edit Scheduled Transaction")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()

	// Account should be pre-selected
	if fields[schedFieldAccount].SelectedIndex != 0 {
		t.Errorf("account selectedIndex = %d, want 0", fields[schedFieldAccount].SelectedIndex)
	}

	// Payee should be pre-filled
	if fields[schedFieldPayee].Value != "Test Payee" {
		t.Errorf("payee = %q, want %q", fields[schedFieldPayee].Value, "Test Payee")
	}

	// Category should be pre-selected
	if fields[schedFieldCategory].SelectedIndex != 1 {
		t.Errorf("category selectedIndex = %d, want 1", fields[schedFieldCategory].SelectedIndex)
	}

	// Amount should be pre-filled
	if fields[schedFieldAmount].Value != "50.00" {
		t.Errorf("amount = %q, want %q", fields[schedFieldAmount].Value, "50.00")
	}

	// Memo should be pre-filled
	if fields[schedFieldMemo].Value != "Test memo" {
		t.Errorf("memo = %q, want %q", fields[schedFieldMemo].Value, "Test memo")
	}

	// Frequency should be Weekly (index 1)
	if fields[schedFieldFrequency].SelectedIndex != 1 {
		t.Errorf("frequency selectedIndex = %d, want 1", fields[schedFieldFrequency].SelectedIndex)
	}

	// Interval should be "2"
	if fields[schedFieldInterval].Value != "2" {
		t.Errorf("interval = %q, want %q", fields[schedFieldInterval].Value, "2")
	}

	// Start date should be pre-filled
	if fields[schedFieldStartDate].Value != "03/15/2024" {
		t.Errorf("start date = %q, want %q", fields[schedFieldStartDate].Value, "03/15/2024")
	}

	// Duration should be Indefinite (no end date, no occurrences)
	if fields[schedFieldDuration].SelectedIndex != 0 {
		t.Errorf("duration selectedIndex = %d, want 0", fields[schedFieldDuration].SelectedIndex)
	}
}

func TestBuildEditScheduledDialog_WithEndDate(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	st.SetEndDate(types.NewDate(2024, time.December, 31))

	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{accountID}
	categoryOptions := []string{"(None)"}
	categoryIDs := []types.ID{types.NilID}
	payeeNames := map[types.ID]string{}

	d := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
	fields := d.Fields()

	// Duration should be "Until Date" (index 1)
	if fields[schedFieldDuration].SelectedIndex != 1 {
		t.Errorf("duration selectedIndex = %d, want 1", fields[schedFieldDuration].SelectedIndex)
	}

	// End date should be pre-filled
	if fields[schedFieldEndDate].Value != "12/31/2024" {
		t.Errorf("end date = %q, want %q", fields[schedFieldEndDate].Value, "12/31/2024")
	}
}

func TestBuildEditScheduledDialog_WithOccurrences(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	st.SetOccurrences(12)

	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{accountID}
	categoryOptions := []string{"(None)"}
	categoryIDs := []types.ID{types.NilID}
	payeeNames := map[types.ID]string{}

	d := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
	fields := d.Fields()

	// Duration should be "Occurrences" (index 2)
	if fields[schedFieldDuration].SelectedIndex != 2 {
		t.Errorf("duration selectedIndex = %d, want 2", fields[schedFieldDuration].SelectedIndex)
	}

	// Occurrences should be pre-filled
	if fields[schedFieldOccurrence].Value != "12" {
		t.Errorf("occurrences = %q, want %q", fields[schedFieldOccurrence].Value, "12")
	}
}

func TestBuildEditScheduledDialog_VariableAmount(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	// No amount set - variable

	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{accountID}
	categoryOptions := []string{"(None)"}
	categoryIDs := []types.ID{types.NilID}
	payeeNames := map[types.ID]string{}

	d := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
	fields := d.Fields()

	// Amount should be empty for variable
	if fields[schedFieldAmount].Value != "" {
		t.Errorf("amount = %q, want empty string for variable amount", fields[schedFieldAmount].Value)
	}
}

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_HandleScheduledKeys_NewKey(t *testing.T) {
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
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Press 'n' for new scheduled
	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(nKey)

	if cmd == nil {
		t.Error("pressing 'n' in scheduled view should return a non-nil cmd")
	}
}

func TestApp_HandleScheduledKeys_EditKey(t *testing.T) {
	st := scheduled.NewTransaction(types.NewID(), scheduled.FrequencyMonthly, types.Today())
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
			allTxns:       []*scheduled.Transaction{st},
			dueCount:      0,
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Press 'e' for edit scheduled
	eKey := tea.KeyPressMsg{Code: 'e', Text: "e"}
	_, cmd := app.Update(eKey)

	if cmd == nil {
		t.Error("pressing 'e' in scheduled view should return a non-nil cmd")
	}
}

func TestApp_Update_ScheduledDialogDataMsg_New(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &scheduledDialogData{
		mode:     scheduledDialogModeNew,
		accounts: []*account.Account{{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking"}},
		payees:   []*payee.Payee{},
		payeeMap: make(map[string]*payee.Payee),
	}

	msg := scheduledDialogDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.schedDialog == nil {
		t.Fatal("scheduled dialog should be created")
	}
	if !updatedApp.schedDialog.IsVisible() {
		t.Error("scheduled dialog should be visible")
	}
	if updatedApp.schedDialog.Title() != "New Scheduled Transaction" {
		t.Errorf("title = %q, want %q", updatedApp.schedDialog.Title(), "New Scheduled Transaction")
	}
	if updatedApp.schedDialogData == nil {
		t.Error("scheduled dialog data should be set")
	}
	if updatedApp.schedDialogAccountIDs == nil {
		t.Error("scheduled dialog account IDs should be set")
	}
	if updatedApp.schedDialogCategoryIDs == nil {
		t.Error("scheduled dialog category IDs should be set")
	}
}

func TestApp_Update_ScheduledDialogDataMsg_Edit(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.Today())
	st.SetPayee(payeeID)

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &scheduledDialogData{
		mode:      scheduledDialogModeEdit,
		scheduled: st,
		accounts:  []*account.Account{{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking"}},
		payees:    []*payee.Payee{{BaseModel: types.BaseModel{ID: payeeID}, Name: "Test Payee"}},
		payeeMap:  map[string]*payee.Payee{"test payee": {BaseModel: types.BaseModel{ID: payeeID}, Name: "Test Payee"}},
	}

	msg := scheduledDialogDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.schedDialog == nil {
		t.Fatal("scheduled dialog should be created")
	}
	if updatedApp.schedDialog.Title() != "Edit Scheduled Transaction" {
		t.Errorf("title = %q, want %q", updatedApp.schedDialog.Title(), "Edit Scheduled Transaction")
	}

	// Verify payee is pre-filled
	fields := updatedApp.schedDialog.Fields()
	if fields[schedFieldPayee].Value != "Test Payee" {
		t.Errorf("payee = %q, want %q", fields[schedFieldPayee].Value, "Test Payee")
	}
}

func TestApp_HandleScheduledDialogKey_Cancel(t *testing.T) {
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{types.NewID()},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.schedDialog != nil {
		t.Error("scheduled dialog should be nil after cancel")
	}
	if updatedApp.schedDialogData != nil {
		t.Error("scheduled dialog data should be nil after cancel")
	}
	if updatedApp.schedDialogAccountIDs != nil {
		t.Error("scheduled dialog account IDs should be nil after cancel")
	}
	if updatedApp.schedDialogCategoryIDs != nil {
		t.Error("scheduled dialog category IDs should be nil after cancel")
	}
}

func TestApp_HandleScheduledDialogKey_TabCycles(t *testing.T) {
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{types.NewID()},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	initialFocus := app.schedDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	model, _ := app.Update(tabKey)
	updatedApp := model.(*App)

	if updatedApp.schedDialog.FocusIndex() != 1 {
		t.Errorf("focus after Tab = %d, want 1", updatedApp.schedDialog.FocusIndex())
	}
}

func TestApp_SubmitScheduledDialog_InvalidStartDate(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldStartDate].Value = "not-a-date"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("invalid start date should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldStartDate].Error == "" {
		t.Error("invalid start date should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_InvalidAmount(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "not-a-number"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("invalid amount should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldAmount].Error == "" {
		t.Error("invalid amount should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_InvalidInterval(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldInterval].Value = "abc"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("invalid interval should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldInterval].Error == "" {
		t.Error("invalid interval should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_ZeroInterval(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldInterval].Value = "0"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("zero interval should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldInterval].Error == "" {
		t.Error("zero interval should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_DurationUntilDate_MissingEndDate(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			// Set duration to "Until Date" (index 1)
			d.Fields()[schedFieldDuration].SelectedIndex = durationUntilDate
			// Leave end date empty
			d.Fields()[schedFieldEndDate].Value = ""
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("missing end date should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldEndDate].Error == "" {
		t.Error("missing end date should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_DurationUntilDate_InvalidEndDate(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldDuration].SelectedIndex = durationUntilDate
			d.Fields()[schedFieldEndDate].Value = "invalid"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("invalid end date should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldEndDate].Error == "" {
		t.Error("invalid end date should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_DurationOccurrences_MissingCount(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldDuration].SelectedIndex = durationOccurrences
			d.Fields()[schedFieldOccurrence].Value = ""
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("missing occurrences should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldOccurrence].Error == "" {
		t.Error("missing occurrences should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_DurationOccurrences_InvalidCount(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldDuration].SelectedIndex = durationOccurrences
			d.Fields()[schedFieldOccurrence].Value = "abc"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	_, cmd := app.submitScheduledDialog()

	if cmd != nil {
		t.Error("invalid occurrences should not return a cmd")
	}
	if app.schedDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.schedDialog.Fields()[schedFieldOccurrence].Error == "" {
		t.Error("invalid occurrences should set field-level error")
	}
}

func TestApp_SubmitScheduledDialog_ValidNew(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)", "Groceries"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "100.00"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID, types.NewID()},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new scheduled should return a non-nil cmd")
	}
	if updatedApp.schedDialog != nil {
		t.Error("dialog should be closed after submit")
	}
	if updatedApp.schedDialogData != nil {
		t.Error("dialog data should be nil after submit")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_SubmitScheduledDialog_ValidNew_VariableAmount(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			// Leave amount empty for variable
			d.Fields()[schedFieldAmount].Value = ""
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new scheduled (variable amount) should return a non-nil cmd")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_SubmitScheduledDialog_ValidNew_WithEndDate(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "50.00"
			d.Fields()[schedFieldDuration].SelectedIndex = durationUntilDate
			d.Fields()[schedFieldEndDate].Value = "12/31/2025"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new scheduled with end date should return a non-nil cmd")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_SubmitScheduledDialog_ValidNew_WithOccurrences(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "50.00"
			d.Fields()[schedFieldDuration].SelectedIndex = durationOccurrences
			d.Fields()[schedFieldOccurrence].Value = "12"
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new scheduled with occurrences should return a non-nil cmd")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_SubmitScheduledDialog_ValidEdit(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.Today())

	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildEditScheduledDialog(st,
				accountOptions, []types.ID{accountID},
				categoryOptions, []types.ID{types.NilID},
				map[types.ID]string{})
			d.Fields()[schedFieldAmount].Value = "200.00"
			return d
		}(),
		schedDialogData: &scheduledDialogData{
			mode:      scheduledDialogModeEdit,
			scheduled: st,
			payeeMap:  make(map[string]*payee.Payee),
		},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid edit should return a non-nil cmd")
	}
	if updatedApp.schedDialog != nil {
		t.Error("dialog should be closed after submit")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_CloseScheduledDialog(t *testing.T) {
	app := &App{
		schedDialog: func() *Dialog {
			d := NewDialog("New Scheduled Transaction")
			d.SetVisible(true)
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew},
		schedDialogAccountIDs:  []types.ID{types.NewID()},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	app.closeScheduledDialog()

	if app.schedDialog != nil {
		t.Error("dialog should be nil after close")
	}
	if app.schedDialogData != nil {
		t.Error("dialog data should be nil after close")
	}
	if app.schedDialogAccountIDs != nil {
		t.Error("account IDs should be nil after close")
	}
	if app.schedDialogCategoryIDs != nil {
		t.Error("category IDs should be nil after close")
	}
}

func TestApp_Update_ScheduledDialogSavedMsg(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := scheduledDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("scheduledDialogSavedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithScheduledDialog(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{},
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog([]string{"Checking"}, []string{"(None)"})
			return d
		}(),
	}
	app.buildScheduledTable()

	output := app.renderLayout()
	if !strings.Contains(output, "New Scheduled Transaction") {
		t.Error("renderLayout() should contain 'New Scheduled Transaction' when dialog is visible")
	}
}

func TestApp_GetKeyHints_Scheduled(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	hints := app.getKeyHints()
	expectedKeys := []string{"n new", "e edit", "d delete", "enter post", "s skip"}
	for _, key := range expectedKeys {
		if !strings.Contains(hints, key) {
			t.Errorf("key hints should contain %q, got: %s", key, hints)
		}
	}
}

// Test that the empty state message mentions 'n' for new
func TestApp_RenderScheduled_EmptyState(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{},
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
	}

	output := app.renderScheduled()
	if !strings.Contains(output, "'n'") {
		t.Error("empty scheduled view should mention 'n' key for creating new")
	}
}

// =============================================================================
// Auto-post tests
// =============================================================================

func TestLeadDaysToIndex(t *testing.T) {
	tests := []struct {
		days     int
		expected int
	}{
		{0, leadDaysOnTheDay},
		{3, leadDays3Days},
		{7, leadDays1Week},
		{1, leadDaysOnTheDay}, // invalid value defaults to on-the-day
	}

	for _, tc := range tests {
		got := leadDaysToIndex(tc.days)
		if got != tc.expected {
			t.Errorf("leadDaysToIndex(%d) = %d, want %d", tc.days, got, tc.expected)
		}
	}
}

func TestLeadDaysFromIndex(t *testing.T) {
	tests := []struct {
		index    int
		expected int
	}{
		{leadDaysOnTheDay, 0},
		{leadDays3Days, 3},
		{leadDays1Week, 7},
		{99, 0}, // invalid index defaults to 0
	}

	for _, tc := range tests {
		got := leadDaysFromIndex(tc.index)
		if got != tc.expected {
			t.Errorf("leadDaysFromIndex(%d) = %d, want %d", tc.index, got, tc.expected)
		}
	}
}

func TestLeadDaysRoundTrip(t *testing.T) {
	for _, days := range []int{0, 3, 7} {
		idx := leadDaysToIndex(days)
		back := leadDaysFromIndex(idx)
		if back != days {
			t.Errorf("leadDays round-trip failed for %d: got %d", days, back)
		}
	}
}

func TestBuildNewScheduledDialog_AutoPostFields(t *testing.T) {
	d := buildNewScheduledDialog([]string{"Checking"}, []string{"(None)"})
	fields := d.Fields()

	// Auto-post checkbox should be unchecked by default
	if fields[schedFieldAutoPost].Type != FieldCheckbox {
		t.Errorf("auto-post field type = %v, want %v", fields[schedFieldAutoPost].Type, FieldCheckbox)
	}
	if fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should default to unchecked")
	}

	// Lead time radio
	if fields[schedFieldLeadDays].Type != FieldRadio {
		t.Errorf("lead time field type = %v, want %v", fields[schedFieldLeadDays].Type, FieldRadio)
	}
	if fields[schedFieldLeadDays].SelectedIndex != 0 {
		t.Errorf("lead time default = %d, want 0", fields[schedFieldLeadDays].SelectedIndex)
	}
	if len(fields[schedFieldLeadDays].Options) != 3 {
		t.Errorf("lead time options = %d, want 3", len(fields[schedFieldLeadDays].Options))
	}
}

func TestBuildEditScheduledDialog_AutoPostEnabled(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	st.SetAutoPost(true)
	st.SetPostLeadDays(3)

	d := buildEditScheduledDialog(st,
		[]string{"Checking"}, []types.ID{accountID},
		[]string{"(None)"}, []types.ID{types.NilID},
		map[types.ID]string{})
	fields := d.Fields()

	// Auto-post should be checked
	if !fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should be checked when editing auto-post transaction")
	}

	// Lead time should be "3 days early" (index 1)
	if fields[schedFieldLeadDays].SelectedIndex != leadDays3Days {
		t.Errorf("lead time = %d, want %d", fields[schedFieldLeadDays].SelectedIndex, leadDays3Days)
	}
}

func TestBuildEditScheduledDialog_AutoPostWith7DayLead(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	st.SetAutoPost(true)
	st.SetPostLeadDays(7)

	d := buildEditScheduledDialog(st,
		[]string{"Checking"}, []types.ID{accountID},
		[]string{"(None)"}, []types.ID{types.NilID},
		map[types.ID]string{})
	fields := d.Fields()

	if !fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should be checked")
	}
	if fields[schedFieldLeadDays].SelectedIndex != leadDays1Week {
		t.Errorf("lead time = %d, want %d", fields[schedFieldLeadDays].SelectedIndex, leadDays1Week)
	}
}

func TestBuildEditScheduledDialog_AutoPostDisabled(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	// AutoPost defaults to false

	d := buildEditScheduledDialog(st,
		[]string{"Checking"}, []types.ID{accountID},
		[]string{"(None)"}, []types.ID{types.NilID},
		map[types.ID]string{})
	fields := d.Fields()

	if fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should be unchecked for non-auto-post transaction")
	}
	if fields[schedFieldLeadDays].SelectedIndex != leadDaysOnTheDay {
		t.Errorf("lead time = %d, want %d", fields[schedFieldLeadDays].SelectedIndex, leadDaysOnTheDay)
	}
}

func TestFormatScheduledRow_AutoPostIndicator(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	tests := []struct {
		name     string
		autoPost bool
		leadDays int
		wantAuto string
	}{
		{"no auto-post", false, 0, ""},
		{"auto-post 0 lead", true, 0, "[Auto]"},
		{"auto-post 3 lead", true, 3, "[Auto 3d]"},
		{"auto-post 7 lead", true, 7, "[Auto 7d]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := scheduled.NewTransaction(types.NewID(), scheduled.FrequencyMonthly, types.Today())
			st.SetAutoPost(tc.autoPost)
			st.SetPostLeadDays(tc.leadDays)

			app := &App{
				styles: styles,
				scheduled: &scheduledViewData{
					payeeNames:    make(map[types.ID]string),
					accountNames:  make(map[types.ID]string),
					categoryNames: make(map[types.ID]string),
				},
			}

			row := app.formatScheduledRow(st, false)
			// Auto indicator is the 7th column (index 6)
			if len(row) < 7 {
				t.Fatalf("row has %d columns, want at least 7", len(row))
			}
			if row[6] != tc.wantAuto {
				t.Errorf("auto indicator = %q, want %q", row[6], tc.wantAuto)
			}
		})
	}
}

func TestApp_AutoPostCompletedMsg_WithPosts(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	summary := &scheduled.AutoPostSummary{
		PostedCount: 3,
	}

	msg := autoPostCompletedMsg{summary: summary}
	_, cmd := app.Update(msg)

	// Should trigger a data reload
	if cmd == nil {
		t.Error("autoPostCompletedMsg with posts should return reload commands")
	}

	// Status bar should show notification
	rendered := app.statusbar.Render(NewStyles(), 80)
	if !strings.Contains(rendered, "Auto-posted 3") {
		t.Errorf("status bar should show auto-post notification, got: %s", rendered)
	}
}

func TestApp_AutoPostCompletedMsg_NoPosts(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	summary := &scheduled.AutoPostSummary{
		PostedCount: 0,
	}

	msg := autoPostCompletedMsg{summary: summary}
	_, cmd := app.Update(msg)

	// Should not trigger reload when nothing posted
	if cmd != nil {
		t.Error("autoPostCompletedMsg with 0 posts should not trigger reload")
	}
}

func TestApp_AutoPostCompletedMsg_NilSummary(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := autoPostCompletedMsg{summary: nil}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Error("autoPostCompletedMsg with nil summary should not trigger reload")
	}
}

func TestApp_SubmitScheduledDialog_ValidNew_WithAutoPost(t *testing.T) {
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "100.00"
			d.Fields()[schedFieldAutoPost].Checked = true
			d.Fields()[schedFieldLeadDays].SelectedIndex = leadDays3Days
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new scheduled with auto-post should return a non-nil cmd")
	}
	if updatedApp.schedDialog != nil {
		t.Error("dialog should be closed after submit")
	}
}
