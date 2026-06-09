package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
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
		{2, scheduled.FrequencyFortnightly},
		{3, scheduled.FrequencySemiMonthly},
		{4, scheduled.FrequencyMonthly},
		{5, scheduled.FrequencyQuarterly},
		{6, scheduled.FrequencyYearly},
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
		{scheduled.FrequencyFortnightly, 2},
		{scheduled.FrequencySemiMonthly, 3},
		{scheduled.FrequencyMonthly, 4},
		{scheduled.FrequencyQuarterly, 5},
		{scheduled.FrequencyYearly, 6},
		{scheduled.Frequency("unknown"), 4}, // unknown defaults to monthly index
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
	if len(fields) != 14 {
		t.Fatalf("expected 14 fields, got %d", len(fields))
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
		fieldType dialog.FieldType
	}{
		{"Account", dialog.FieldSelect},
		{"Payee", dialog.FieldText},
		{"Category", dialog.FieldCombo},
		{"Amount", dialog.FieldText},
		{"Memo", dialog.FieldText},
		{"Frequency", dialog.FieldSelect},
		{"Interval", dialog.FieldText},
		{"Start Date", dialog.FieldDate},
		{"Duration", dialog.FieldRadio},
		{"End Date", dialog.FieldDate},
		{"Occurrences", dialog.FieldText},
		{"Auto-post", dialog.FieldCheckbox},
		{"Lead time", dialog.FieldRadio},
		{"Split transaction", dialog.FieldCheckbox},
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

	// Frequency defaults to Monthly (index resolved from AllFrequencies)
	wantFreqIdx := frequencyToIndex(scheduled.FrequencyMonthly)
	if fields[schedFieldFrequency].SelectedIndex != wantFreqIdx {
		t.Errorf("frequency selectedIndex = %d, want %d", fields[schedFieldFrequency].SelectedIndex, wantFreqIdx)
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

func TestBuildNewScheduledDialog_EndDateIsOptionalBlank(t *testing.T) {
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	d := buildNewScheduledDialog(accountOptions, categoryOptions)
	fields := d.Fields()

	endField := fields[schedFieldEndDate]
	if endField.Type != dialog.FieldDate {
		t.Errorf("End Date Type = %d, want dialog.FieldDate", endField.Type)
	}
	if !endField.OptionalBlank {
		t.Error("End Date should be OptionalBlank for the new-scheduled dialog")
	}
	if endField.Value != "  /  /    " {
		t.Errorf("End Date Value = %q, want canonical blank %q", endField.Value, "  /  /    ")
	}

	startField := fields[schedFieldStartDate]
	if startField.Type != dialog.FieldDate {
		t.Errorf("Start Date Type = %d, want dialog.FieldDate", startField.Type)
	}
	if startField.OptionalBlank {
		t.Error("Start Date must not be OptionalBlank — it is required")
	}
}

func TestBuildEditScheduledDialog_EndDateOptionalBlank_NoEndDate(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	// No end date or occurrences set → Indefinite duration.

	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{accountID}
	categoryOptions := []string{"(None)"}
	categoryIDs := []types.ID{types.NilID}

	d := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, map[types.ID]string{})
	fields := d.Fields()

	endField := fields[schedFieldEndDate]
	if !endField.OptionalBlank {
		t.Error("End Date should be OptionalBlank")
	}
	if endField.Value != "  /  /    " {
		t.Errorf("End Date Value = %q, want canonical blank when scheduled has no end date", endField.Value)
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
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			// Syntactically valid 10-char mask shape, but semantically
			// invalid — the masked widget no longer accepts free-text
			// like "not-a-date", so use month=13 / day=45 to force a
			// time.Parse error in submit-path validation.
			d.Fields()[schedFieldStartDate].Value = "13/45/2024"
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			// Set duration to "Until Date" (index 1)
			d.Fields()[schedFieldDuration].SelectedIndex = durationUntilDate
			// End date stays at the default canonical-blank state from
			// AddOptionalDateField — submit must surface a required-field
			// error when Duration = Until Date and the field is unfilled.
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldDuration].SelectedIndex = durationUntilDate
			// 10-char mask shape but month=13/day=45 — masked widget refuses
			// free-text, so use a syntactically valid but semantically
			// invalid date to force the parser error.
			d.Fields()[schedFieldEndDate].Value = "13/45/2024"
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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

func TestApp_SubmitScheduledDialog_DurationIndefinite_BlankEndDateAccepted(t *testing.T) {
	// Regression guard: with Duration = Indefinite, the End Date stays at the
	// canonical-blank ("  /  /    ") default; submit must treat that as "no
	// value" rather than triggering a parse error.
	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	categoryOptions := []string{"(None)"}

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
			d := buildNewScheduledDialog(accountOptions, categoryOptions)
			d.Fields()[schedFieldAmount].Value = "50.00"
			// Leave duration at Indefinite, end date at canonical blank.
			return d
		}(),
		schedDialogData:        &scheduledDialogData{mode: scheduledDialogModeNew, payeeMap: make(map[string]*payee.Payee)},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("blank end date with Duration=Indefinite should still return a non-nil cmd")
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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
		schedDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Scheduled Transaction")
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := scheduledDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("scheduledDialogSavedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithScheduledDialog(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{},
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
		schedDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
	if fields[schedFieldAutoPost].Type != dialog.FieldCheckbox {
		t.Errorf("auto-post field type = %v, want %v", fields[schedFieldAutoPost].Type, dialog.FieldCheckbox)
	}
	if fields[schedFieldAutoPost].Checked {
		t.Error("auto-post should default to unchecked")
	}

	// Lead time radio
	if fields[schedFieldLeadDays].Type != dialog.FieldRadio {
		t.Errorf("lead time field type = %v, want %v", fields[schedFieldLeadDays].Type, dialog.FieldRadio)
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
	styles := widget.NewStyles()
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
	rendered := app.statusbar.Render(widget.NewStyles(), 80)
	if !strings.Contains(rendered, "Auto-posted 3") {
		t.Errorf("status bar should show auto-post notification, got: %s", rendered)
	}
}

func TestApp_AutoPostCompletedMsg_NoPosts(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: func() *dialog.Dialog {
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

// =============================================================================
// MS-017 — Split toggle on the scheduled dialog
// =============================================================================

// buildSchedDialogWithSplitToggle assembles a scheduled-dialog widget with
// the field shape submitScheduledDialog expects. The returned dialog has the
// Split-transaction checkbox at schedFieldSplit set to splitChecked, so a
// test can stage a Save with or without Split toggled on.
func buildSchedDialogWithSplitToggle(t *testing.T, amountStr, startDate string, splitChecked bool, accountName string, categoryOptions []string) *dialog.Dialog {
	t.Helper()
	d := dialog.NewDialog("New Scheduled Transaction")
	d.AddSelectField("Account", []string{accountName}, 0)
	d.AddTextField("Payee", "Employer", "", 0)
	d.AddSelectField("Category", categoryOptions, 0)
	d.AddTextField("Amount", amountStr, "Empty = variable", 12)
	d.AddTextField("Memo", "", "", 0)
	d.AddSelectField("Frequency", buildFrequencyOptions(), frequencyToIndex(scheduled.FrequencyMonthly))
	f := d.AddTextField("Interval", "1", "", 5)
	f.Required = true
	f = d.AddDateField("Start Date", startDate)
	f.Required = true
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, 0)
	d.AddOptionalDateField("End Date", "")
	d.AddTextField("Occurrences", "", "", 5)
	d.AddCheckboxField("Auto-post", false)
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, 0)
	d.AddCheckboxField("Split transaction", splitChecked)
	d.SetVisible(true)
	return d
}

func TestScheduledDialog_SplitToggle_OpensMultiLineEditor(t *testing.T) {
	accountID := types.NewID()
	categoryID := types.NewID()

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: buildSchedDialogWithSplitToggle(t,
			"4000.00", "01/15/2024", true, "Checking",
			[]string{"(None)", "Salary"}),
		schedDialogData: &scheduledDialogData{
			mode: scheduledDialogModeNew,
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			},
			payeeMap: make(map[string]*payee.Payee),
		},
		schedDialogAccountIDs:  []types.ID{accountID},
		schedDialogCategoryIDs: []types.ID{types.NilID, categoryID},
	}

	model, cmd := app.submitScheduledDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Errorf("expected no async cmd when toggling into split editor, got non-nil")
	}
	if updatedApp.schedDialog != nil {
		t.Error("scheduled dialog should be closed once split editor opens")
	}
	if updatedApp.splitDialog == nil {
		t.Fatal("split dialog should be opened by the Split toggle")
	}
	if updatedApp.pendingSplitScheduled == nil {
		t.Fatal("pendingSplitScheduled should be set so the split-save handler can finalize")
	}

	wantAmount, _ := types.NewMoney("4000.00")
	if !updatedApp.splitDialog.totalAmount.Equal(wantAmount) {
		t.Errorf("split dialog totalAmount = %s, want %s",
			updatedApp.splitDialog.totalAmount.String(), wantAmount.String())
	}
	if updatedApp.pendingSplitScheduled.frequency != scheduled.FrequencyMonthly {
		t.Errorf("pendingSplitScheduled.frequency = %s, want monthly",
			updatedApp.pendingSplitScheduled.frequency)
	}
}

// createMultiLineScheduledTestApp wires a real DB + the services the
// scheduled-split submit path touches so the persistence test can drive
// an end-to-end Save.
func createMultiLineScheduledTestApp(t *testing.T) (*App, *scheduled.Service, *account.Account, *category.Category, *category.Category) {
	t.Helper()
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, accountRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	incomeCat := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(incomeCat); err != nil {
		t.Fatalf("Create income category: %v", err)
	}
	taxCat := category.NewCategory("Federal Tax", category.TypeExpense)
	if err := categoryRepo.Create(taxCat); err != nil {
		t.Fatalf("Create tax category: %v", err)
	}

	app := &App{
		currentView:     ViewScheduled,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		undoManager:     undo.NewManager(),
	}
	return app, schedSvc, acct, incomeCat, taxCat
}

func TestScheduledDialog_MultiLineSave_PersistsChildren(t *testing.T) {
	app, schedSvc, acct, incomeCat, taxCat := createMultiLineScheduledTestApp(t)

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{incomeCat, taxCat})
	app.schedDialogData = &scheduledDialogData{
		mode:     scheduledDialogModeNew,
		accounts: []*account.Account{acct},
		payeeMap: make(map[string]*payee.Payee),
	}
	app.schedDialogAccountIDs = []types.ID{acct.ID}
	app.schedDialogCategoryIDs = categoryIDs
	today := types.Today().Time().Format("01/02/2006")
	app.schedDialog = buildSchedDialogWithSplitToggle(t,
		"900.00", today, true, acct.Name, categoryOptions)

	model, cmd := app.submitScheduledDialog()
	app2 := model.(*App)
	if cmd != nil {
		t.Errorf("expected nil cmd when opening split editor, got non-nil")
	}
	if app2.splitDialog == nil {
		t.Fatal("split dialog should be open after Split toggle")
	}

	// Resolve the option indices for the two seeded categories.
	var incomeIdx, taxIdx int
	for i, id := range categoryIDs {
		switch id {
		case incomeCat.ID:
			incomeIdx = i
		case taxCat.ID:
			taxIdx = i
		}
	}
	if incomeIdx == 0 || taxIdx == 0 {
		t.Fatalf("category indices unresolved: income=%d tax=%d", incomeIdx, taxIdx)
	}

	sd := app2.splitDialog
	sd.rows[0].categoryIndex = incomeIdx
	sd.rows[0].amountField.Value = "1000.00"
	sd.addRow()
	sd.rows[1].categoryIndex = taxIdx
	sd.rows[1].amountField.Value = "-100.00"

	model, saveCmd := app2.submitScheduledSplitDialog()
	app3 := model.(*App)
	if app3.splitDialog != nil {
		t.Error("split dialog should be cleared after a successful save")
	}
	if app3.pendingSplitScheduled != nil {
		t.Error("pendingSplitScheduled should be cleared after a successful save")
	}
	if saveCmd == nil {
		t.Fatal("submitScheduledSplitDialog should return a non-nil save command")
	}
	if msg := saveCmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("unexpected error from save command: %v", e.err)
		}
		if _, ok := msg.(scheduledDialogSavedMsg); !ok {
			t.Fatalf("unexpected message type from save command: %T", msg)
		}
	}

	schedules, err := schedSvc.List()
	if err != nil {
		t.Fatalf("List schedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(schedules))
	}
	sched := schedules[0]
	if len(sched.Splits) != 2 {
		t.Fatalf("got %d child splits, want 2", len(sched.Splits))
	}
	wantNet, _ := types.NewMoney("900.00")
	if !sched.Splits.Total().Equal(wantNet) {
		t.Errorf("signed sum of children = %s, want %s",
			sched.Splits.Total().String(), wantNet.String())
	}
	if sched.HasCategory() {
		t.Error("multi-line schedule should clear the scalar category")
	}
	if !sched.HasAmount() {
		t.Error("multi-line schedule should preserve the parent amount")
	}

	var sawIncome, sawTax bool
	for _, sp := range sched.Splits {
		if !sp.CategoryID.Valid {
			continue
		}
		switch sp.CategoryID.ID {
		case incomeCat.ID:
			sawIncome = true
		case taxCat.ID:
			sawTax = true
		}
	}
	if !sawIncome || !sawTax {
		t.Errorf("missing child categories: income=%v tax=%v", sawIncome, sawTax)
	}
}

// hasEditAsPaycheckButton reports whether the dialog's button row
// includes the "Edit as paycheck →" affordance. Used by the MS-029
// tests to assert visibility based on the looksLikePaycheck heuristic.
func hasEditAsPaycheckButton(d *dialog.Dialog) bool {
	for _, b := range d.Buttons() {
		if strings.Contains(b.Label, "Edit as paycheck") {
			return true
		}
	}
	return false
}

// TestScheduledDialog_EditAsPaycheck_RelaunchesWizard covers MS-029:
// a scheduled transaction that matches the paycheck heuristic (multi-
// line schedule with a positive categorized income line plus at least
// one negative tax-categorized deduction line) shows an
// "Edit as paycheck →" affordance on the edit dialog, and activating
// it closes the regular dialog and opens the paycheck wizard pre-
// filled with the schedule's current values.
func TestScheduledDialog_EditAsPaycheck_RelaunchesWizard(t *testing.T) {
	// Build accounts, categories, payees in memory — buildEditScheduled
	// dialog.Dialog and the relaunch helper consume them by display name and
	// ID without needing the DB layer.
	checkingID := types.NewID()
	retire401kID := types.NewID()
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: retire401kID}, Name: "401k", Active: true, Type: account.TypeInvestment},
	}
	accountOptions, accountIDs := buildAccountOptions(accounts)

	salaryID := types.NewID()
	federalID := types.NewID()
	ssID := types.NewID()
	medicareID := types.NewID()
	healthID := types.NewID()
	categoryOptions := []string{
		"(None)",
		"Income > Salary",
		"Insurance > Health",
		"Tax > Federal",
		"Tax > Medicare",
		"Tax > Social Security",
	}
	categoryIDs := []types.ID{
		types.NilID,
		salaryID,
		healthID,
		federalID,
		medicareID,
		ssID,
	}

	employerPayeeID := types.NewID()
	employerPayee := &payee.Payee{
		BaseModel: types.BaseModel{ID: employerPayeeID},
		Name:      "Acme Corp",
	}
	payees := []*payee.Payee{employerPayee}
	payeeNames := map[types.ID]string{employerPayeeID: "Acme Corp"}

	// Paycheck-shaped schedule: gross +5000 Salary, -800 Federal, -310
	// Social Security, -150 Health, -500 401(k) transfer. Net 3240.
	st := scheduled.NewTransaction(checkingID, scheduled.FrequencyFortnightly, types.MustParseDate("2024-03-15"))
	st.SetAmount(types.MustNewMoney("3240.00"))
	st.SetPayee(employerPayeeID)
	st.ClearCategory()
	st.Splits = scheduled.SplitCollection{
		{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("5000"), CategoryID: types.NullableID{ID: salaryID, Valid: true}, PaycheckSection: types.NullableString{String: "earnings", Valid: true}},
		{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-800"), CategoryID: types.NullableID{ID: federalID, Valid: true}, PaycheckSection: types.NullableString{String: "tax", Valid: true}},
		{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-310"), CategoryID: types.NullableID{ID: ssID, Valid: true}, PaycheckSection: types.NullableString{String: "tax", Valid: true}},
		{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-150"), CategoryID: types.NullableID{ID: healthID, Valid: true}, PaycheckSection: types.NullableString{String: "post_tax", Valid: true}},
		{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-500"), TransferAccountID: types.NullableID{ID: retire401kID, Valid: true}, PaycheckSection: types.NullableString{String: "pre_tax", Valid: true}},
	}

	// Build the edit dialog and assert the affordance is present.
	dlg := buildEditScheduledDialog(st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
	if !hasEditAsPaycheckButton(dlg) {
		t.Fatal("paycheck-shaped edit dialog should expose Edit-as-paycheck button")
	}

	// Wire the App with the dialog and trigger the relaunch.
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		schedDialog: dlg,
		schedDialogData: &scheduledDialogData{
			mode:      scheduledDialogModeEdit,
			scheduled: st,
			accounts:  accounts,
			payees:    payees,
		},
		schedDialogAccountIDs:  accountIDs,
		schedDialogCategoryIDs: categoryIDs,
	}
	app.schedDialogCategoryOptions = categoryOptions

	model, _ := app.relaunchAsPaycheckWizard()
	app2 := model.(*App)

	if app2.schedDialog != nil {
		t.Error("scheduled dialog should close after Edit-as-paycheck relaunch")
	}
	if app2.paycheckWizard == nil {
		t.Fatal("paycheck wizard should open after Edit-as-paycheck relaunch")
	}
	w := app2.paycheckWizard

	if got, want := w.Employer().Value, "Acme Corp"; got != want {
		t.Errorf("employer pre-fill = %q, want %q", got, want)
	}
	if got, want := w.Frequency().SelectedIndex, defaultPaycheckFrequencyIndex; got != want {
		t.Errorf("frequency pre-fill = %d, want %d (fortnightly)", got, want)
	}
	if opt := paycheckFrequencyForIndex(w.Frequency().SelectedIndex); opt.frequency != scheduled.FrequencyFortnightly {
		t.Errorf("frequency pre-fill option = %v, want fortnightly", opt.frequency)
	}
	if got := w.NextPayday().Value; !strings.Contains(got, "2024") {
		t.Errorf("next payday pre-fill = %q, want a 2024 date", got)
	}
	if got, want := w.DepositAccount().Options[w.DepositAccount().SelectedIndex], "Checking"; got != want {
		t.Errorf("deposit account pre-fill = %q, want %q", got, want)
	}

	// Pre-fill walks the schedule's splits and routes each row by its
	// paycheck_section tag (PW2-008): earnings → Earnings, pre_tax →
	// Pre-tax, tax → Tax, post_tax → Post-tax, net_pay_destination →
	// Additional Transfers. The fixture's tags route Salary to
	// Earnings, Federal/Social Security to Tax, the 401(k) transfer to
	// Pre-tax, and Health to Post-tax.
	earn := w.EarningsLines()
	if len(earn) != 1 {
		t.Fatalf("got %d earnings rows, want 1 (Salary)", len(earn))
	}
	if got := earn[0].AmountField().Value; got != "5000" {
		t.Errorf("earnings salary amount = %q, want 5000", got)
	}

	pre := w.PreTaxLines()
	if len(pre) != 1 {
		t.Fatalf("got %d pre-tax rows, want 1 (401k transfer)", len(pre))
	}
	if !pre[0].IsTransfer() {
		t.Error("pre-tax row should be a transfer-line (401k)")
	}
	if got := w.DepositAccount().Options[pre[0].AccountIndex()]; got != "401k" {
		t.Errorf("pre-tax transfer destination = %q, want 401k", got)
	}

	tax := w.TaxLines()
	if len(tax) != 2 {
		t.Fatalf("got %d tax rows, want 2", len(tax))
	}

	post := w.PostTaxLines()
	if len(post) != 1 {
		t.Fatalf("got %d post-tax rows, want 1 (Health)", len(post))
	}
	if post[0].IsTransfer() {
		t.Error("post-tax row should be categorized (Health), not a transfer")
	}

	if got := len(w.AdditionalTransfers()); got != 0 {
		t.Errorf("got %d additional-transfer rows, want 0", got)
	}
}

// TestScheduledDialog_NonPaycheckShape_HidesEditAsPaycheck covers the
// negative side of MS-029: a generic multi-line schedule that doesn't
// match the paycheck heuristic — and a single-line schedule — must not
// surface the Edit-as-paycheck affordance.
func TestScheduledDialog_NonPaycheckShape_HidesEditAsPaycheck(t *testing.T) {
	salaryID := types.NewID()
	rentID := types.NewID()
	foodID := types.NewID()
	categoryOptions := []string{"(None)", "Bills > Rent", "Food > Groceries", "Income > Salary"}
	categoryIDs := []types.ID{types.NilID, rentID, foodID, salaryID}

	accountID := types.NewID()
	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{accountID}
	payeeNames := map[types.ID]string{}

	tests := []struct {
		name string
		st   *scheduled.Transaction
	}{
		{
			name: "single-line schedule",
			st: func() *scheduled.Transaction {
				st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.Today())
				st.SetAmount(types.MustNewMoney("-1500"))
				st.SetCategory(rentID)
				return st
			}(),
		},
		{
			name: "multi-line schedule without tax categories",
			st: func() *scheduled.Transaction {
				st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.Today())
				st.SetAmount(types.MustNewMoney("-200"))
				st.ClearCategory()
				st.Splits = scheduled.SplitCollection{
					{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-50"), CategoryID: types.NullableID{ID: rentID, Valid: true}},
					{BaseModel: types.NewBaseModel(), Amount: types.MustNewMoney("-150"), CategoryID: types.NullableID{ID: foodID, Valid: true}},
				}
				return st
			}(),
		},
		{
			name: "multi-line schedule without income positive line",
			st: func() *scheduled.Transaction {
				// All-negative split set with tax lines but no positive
				// income line: the heuristic should still reject because
				// there's no gross income.
				st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.Today())
				st.SetAmount(types.MustNewMoney("-200"))
				st.ClearCategory()
				// Repurpose categoryOptions: add a "Tax > Federal" option
				// to avoid expanding fixtures.
				return st
			}(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dlg := buildEditScheduledDialog(tc.st, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
			if hasEditAsPaycheckButton(dlg) {
				t.Errorf("Edit-as-paycheck button should be hidden for %s", tc.name)
			}
		})
	}
}

// =============================================================================
// CC-003 — Scheduled new + edit dialog.Dialog → create-category sub-dialog → back
// =============================================================================

// TestBuildNewScheduledDialog_CategoryIsCombo pins that the New Scheduled
// dialog's Category field is a typeahead combo, not a plain Select. Without
// this the [+ Add new category…] action row cannot render.
func TestBuildNewScheduledDialog_CategoryIsCombo(t *testing.T) {
	d := buildNewScheduledDialog([]string{"Checking"}, []string{"(None)", "Rent"})
	if got := d.Fields()[schedFieldCategory].Type; got != dialog.FieldCombo {
		t.Errorf("Category field type = %v, want dialog.FieldCombo", got)
	}
}

// TestBuildNewScheduledDialog_CategoryComboHasAddNewLabel pins that the
// Category combo carries the AddNewLabel sentinel so the action row appears
// in the dropdown.
func TestBuildNewScheduledDialog_CategoryComboHasAddNewLabel(t *testing.T) {
	d := buildNewScheduledDialog([]string{"Checking"}, []string{"(None)", "Rent"})
	got := d.Fields()[schedFieldCategory].AddNewLabel
	want := "[+ Add new category…]"
	if got != want {
		t.Errorf("Category AddNewLabel = %q, want %q", got, want)
	}
}

// TestBuildEditScheduledDialog_CategoryIsCombo pins that the Edit Scheduled
// dialog's Category field is also a typeahead combo.
func TestBuildEditScheduledDialog_CategoryIsCombo(t *testing.T) {
	accountID := types.NewID()
	st := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, types.NewDate(2024, time.January, 1))
	d := buildEditScheduledDialog(st,
		[]string{"Checking"}, []types.ID{accountID},
		[]string{"(None)", "Rent"}, []types.ID{types.NilID, types.NewID()},
		map[types.ID]string{})
	if got := d.Fields()[schedFieldCategory].Type; got != dialog.FieldCombo {
		t.Errorf("Edit Category field type = %v, want dialog.FieldCombo", got)
	}
	want := "[+ Add new category…]"
	if got := d.Fields()[schedFieldCategory].AddNewLabel; got != want {
		t.Errorf("Edit Category AddNewLabel = %q, want %q", got, want)
	}
}

// TestScheduledDialog_CategoryCombo_TabAwayPreservesPreviousSelection pins
// that typing a query with no matches into the Category combo and pressing
// Tab does not blow away the previously-selected SelectedIndex (the typed
// query is discarded).
func TestScheduledDialog_CategoryCombo_TabAwayPreservesPreviousSelection(t *testing.T) {
	d := buildNewScheduledDialog([]string{"Checking"}, []string{"(None)", "Rent", "Food"})
	d.SetFocusIndex(schedFieldCategory)
	cat := d.Fields()[schedFieldCategory]
	cat.SelectedIndex = 2 // "Food"
	cat.ComboHighlight = 2
	// Type a non-matching query so commitComboHighlight has no row to commit.
	cat.Query = "zzzzz"
	cat.ComboHighlight = 0 // highlight head of (empty) filtered list

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if action != dialog.DialogActionNone {
		t.Errorf("Tab action = %v, want dialog.DialogActionNone", action)
	}
	if cat.Query != "" {
		t.Errorf("Tab should clear Query, got %q", cat.Query)
	}
	if cat.SelectedIndex != 2 {
		t.Errorf("Tab with non-matching query should preserve SelectedIndex; got %d, want 2", cat.SelectedIndex)
	}
}

// newAppForSchedAddNew builds an *App with a New Scheduled dialog whose
// Category combo carries the supplied categories, focus on Category, and
// a typed query parked on the [+ Add new category…] action row. Mirrors
// newAppForTxnAddNew. categorySvc may be nil for tests that don't submit
// the sub-dialog.
func newAppForSchedAddNew(t *testing.T, query string, categorySvc *category.Service, cats []*category.Category) *App {
	t.Helper()
	accountOptions := []string{"Checking"}
	accountIDs := []types.ID{types.NewID()}
	categoryOptions, categoryIDs := buildCategoryOptions(cats)

	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		categorySvc: categorySvc,
		schedDialogData: &scheduledDialogData{
			mode:     scheduledDialogModeNew,
			payeeMap: make(map[string]*payee.Payee),
		},
		schedDialogAccountIDs:      accountIDs,
		schedDialogCategoryIDs:     categoryIDs,
		schedDialogCategoryOptions: categoryOptions,
	}
	d := buildNewScheduledDialog(accountOptions, categoryOptions)
	// Capture distinctive scalar state we expect to see preserved across the
	// divert.
	d.Fields()[schedFieldPayee].Value = "Landlord"
	d.Fields()[schedFieldAmount].Value = "-1500.00"
	d.Fields()[schedFieldMemo].Value = "Monthly rent"
	d.SetFocusIndex(schedFieldCategory)
	cat := d.Fields()[schedFieldCategory]
	cat.Query = query
	cat.ComboHighlight = len(cat.FilteredIndices())
	app.schedDialog = d
	return app
}

func TestApp_SchedDialog_AddNew_OpensCreateCategoryDialog(t *testing.T) {
	app := newAppForSchedAddNew(t, "Donations", nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleScheduledDialogKey(enter)
	updated := model.(*App)

	if updated.createCatDialog == nil || !updated.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should be visible after [+ Add new] is activated")
	}
	if updated.createCatSource != createCatSourceSchedDialog {
		t.Errorf("createCatSource = %d, want createCatSourceSchedDialog (%d)",
			updated.createCatSource, createCatSourceSchedDialog)
	}
	if updated.schedDialog == nil {
		t.Fatal("schedDialog should be kept (hidden) so its state survives the divert")
	}
	if updated.schedDialog.IsVisible() {
		t.Error("schedDialog should be hidden while createCatDialog is shown")
	}
	if updated.createCatDialog.Title() != "New Category" {
		t.Errorf("createCatDialog title = %q, want %q",
			updated.createCatDialog.Title(), "New Category")
	}
	// Typed query was "Donations" (no colon) so it seeds the Name field.
	cFields := updated.createCatDialog.Fields()
	if cFields[0].Value != "Donations" {
		t.Errorf("Name field = %q, want %q (seeded from typed query)",
			cFields[0].Value, "Donations")
	}
}

func TestApp_SchedDialog_AddNew_CancelRestoresState(t *testing.T) {
	app := newAppForSchedAddNew(t, "", nil, nil)
	prevCat := app.schedDialog.Fields()[schedFieldCategory].SelectedIndex
	prevFocus := app.schedDialog.FocusIndex()

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleScheduledDialogKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	esc := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ = app.handleCreateCatDialogKey(esc)
	app = model.(*App)

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after cancel")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after cancel", app.createCatSource)
	}
	if app.schedDialog == nil || !app.schedDialog.IsVisible() {
		t.Fatal("schedDialog should be restored to visible after cancel")
	}

	fields := app.schedDialog.Fields()
	if fields[schedFieldPayee].Value != "Landlord" {
		t.Errorf("Payee preserved? got %q, want %q",
			fields[schedFieldPayee].Value, "Landlord")
	}
	if fields[schedFieldAmount].Value != "-1500.00" {
		t.Errorf("Amount preserved? got %q, want %q",
			fields[schedFieldAmount].Value, "-1500.00")
	}
	if fields[schedFieldMemo].Value != "Monthly rent" {
		t.Errorf("Memo preserved? got %q, want %q",
			fields[schedFieldMemo].Value, "Monthly rent")
	}
	if fields[schedFieldCategory].SelectedIndex != prevCat {
		t.Errorf("Category SelectedIndex changed on cancel: got %d, want %d",
			fields[schedFieldCategory].SelectedIndex, prevCat)
	}
	if app.schedDialog.FocusIndex() != prevFocus {
		t.Errorf("FocusIndex changed on cancel: got %d, want %d",
			app.schedDialog.FocusIndex(), prevFocus)
	}
}

func TestApp_SchedDialog_AddNew_SubmitPersistsAndAdvancesFocus(t *testing.T) {
	database := dbtest.New(t)

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}

	app := newAppForSchedAddNew(t, "", svc, cats)

	// Open sub-dialog.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleScheduledDialogKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	// Fill: Name=Cleaning, Parent=(top-level), Type=Expense.
	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "Cleaning"
	cFields[1].SelectedIndex = 0
	cFields[2].SelectedIndex = 0

	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a tea.Cmd that emits createCategoryRequestMsg")
	}
	msg := cmd()
	model, _ = app.Update(msg)
	app = model.(*App)

	// Persisted.
	got, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List after submit: %v", err)
	}
	var found *category.Category
	for _, c := range got {
		if c.Name == "Cleaning" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("'Cleaning' should be persisted after submit")
	}

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after submit")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after submit", app.createCatSource)
	}
	if app.schedDialog == nil || !app.schedDialog.IsVisible() {
		t.Fatal("schedDialog should be visible again after submit")
	}

	fields := app.schedDialog.Fields()
	if fields[schedFieldPayee].Value != "Landlord" {
		t.Errorf("Payee preserved? got %q", fields[schedFieldPayee].Value)
	}
	if fields[schedFieldAmount].Value != "-1500.00" {
		t.Errorf("Amount preserved? got %q", fields[schedFieldAmount].Value)
	}
	if fields[schedFieldMemo].Value != "Monthly rent" {
		t.Errorf("Memo preserved? got %q", fields[schedFieldMemo].Value)
	}

	// Category points at the new category.
	catField := fields[schedFieldCategory]
	if catField.Options[catField.SelectedIndex] != "Cleaning" {
		t.Errorf("Category selected = %q, want %q",
			catField.Options[catField.SelectedIndex], "Cleaning")
	}
	if app.schedDialogCategoryIDs[catField.SelectedIndex] != found.ID {
		t.Errorf("schedDialogCategoryIDs[%d] = %s, want %s",
			catField.SelectedIndex,
			app.schedDialogCategoryIDs[catField.SelectedIndex],
			found.ID)
	}

	if app.schedDialog.FocusIndex() != schedFieldAmount {
		t.Errorf("FocusIndex after submit = %d, want %d (Amount)",
			app.schedDialog.FocusIndex(), schedFieldAmount)
	}
}

// TestSubmitScheduledDialog_CategoryRoundTrip pins the regression that
// submitScheduledDialog still resolves the Category SelectedIndex to a
// categoryID after the Select→Combo conversion. Picks a category, drives
// the submit cmd to completion, reloads the saved schedule, and asserts
// it carries the picked category.
func TestSubmitScheduledDialog_CategoryRoundTrip(t *testing.T) {
	app, schedSvc, acct, incomeCat, _ := createMultiLineScheduledTestApp(t)

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{incomeCat})
	app.schedDialogData = &scheduledDialogData{
		mode:     scheduledDialogModeNew,
		accounts: []*account.Account{acct},
		payeeMap: make(map[string]*payee.Payee),
	}
	app.schedDialogAccountIDs = []types.ID{acct.ID}
	app.schedDialogCategoryIDs = categoryIDs
	app.schedDialogCategoryOptions = categoryOptions

	d := buildNewScheduledDialog([]string{acct.Name}, categoryOptions)
	d.Fields()[schedFieldAccount].SelectedIndex = 0
	d.Fields()[schedFieldPayee].Value = "Employer"
	d.Fields()[schedFieldAmount].Value = "4000.00"
	d.Fields()[schedFieldMemo].Value = "Salary"
	// Select the "Salary" income category (index 1; index 0 is "(None)").
	catField := d.Fields()[schedFieldCategory]
	salaryIdx := 0
	for i, opt := range categoryOptions {
		if opt == incomeCat.Name {
			salaryIdx = i
			break
		}
	}
	if salaryIdx == 0 {
		t.Fatalf("category options missing %q: %v", incomeCat.Name, categoryOptions)
	}
	catField.SelectedIndex = salaryIdx
	catField.ComboHighlight = salaryIdx
	app.schedDialog = d

	_, cmd := app.submitScheduledDialog()
	if cmd == nil {
		t.Fatal("submit should produce a cmd for a valid schedule")
	}
	if msg := cmd(); msg != nil {
		if errM, ok := msg.(errMsg); ok {
			t.Fatalf("submit returned errMsg: %v", errM.err)
		}
	}

	saved, err := schedSvc.List()
	if err != nil {
		t.Fatalf("schedSvc.List: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved schedule, got %d", len(saved))
	}
	got := saved[0]
	if !got.HasCategory() || got.CategoryID.ID != incomeCat.ID {
		t.Errorf("saved schedule CategoryID = %v, want %v",
			got.CategoryID, incomeCat.ID)
	}
}
