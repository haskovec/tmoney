package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildMergerDialog_NewDialog(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildMergerDialog(options, ids, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Merger / Acquisition" {
		t.Errorf("title = %q, want %q", d.Title(), "Merger / Acquisition")
	}

	fields := d.Fields()
	if len(fields) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(fields))
	}

	// Field 0: Source Security (select)
	if fields[0].Type != FieldSelect {
		t.Errorf("field 0 type = %d, want FieldSelect (%d)", fields[0].Type, FieldSelect)
	}
	if fields[0].Label != "Source Security" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Source Security")
	}
	if len(fields[0].Options) != 2 {
		t.Errorf("expected 2 source options, got %d", len(fields[0].Options))
	}
	if fields[0].SelectedIndex != 0 {
		t.Errorf("default source index = %d, want 0", fields[0].SelectedIndex)
	}

	// Field 1: Target Security (select)
	if fields[1].Type != FieldSelect {
		t.Errorf("field 1 type = %d, want FieldSelect (%d)", fields[1].Type, FieldSelect)
	}
	if fields[1].Label != "Target Security" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Target Security")
	}
	if len(fields[1].Options) != 2 {
		t.Errorf("expected 2 target options, got %d", len(fields[1].Options))
	}

	// Field 2: Date (masked, required, default today)
	if fields[2].Type != FieldDate {
		t.Errorf("field 2 type = %d, want FieldDate (%d)", fields[2].Type, FieldDate)
	}
	if fields[2].Label != "Date" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Date")
	}
	if !fields[2].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[2].Value != today {
		t.Errorf("date default = %q, want %q", fields[2].Value, today)
	}

	// Field 3: Exchange Ratio (text, required)
	if fields[3].Type != FieldText {
		t.Errorf("field 3 type = %d, want FieldText (%d)", fields[3].Type, FieldText)
	}
	if fields[3].Label != "Exchange Ratio" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Exchange Ratio")
	}
	if !fields[3].Required {
		t.Error("exchange ratio field should be required")
	}
	if fields[3].Value != "" {
		t.Errorf("exchange ratio default = %q, want empty", fields[3].Value)
	}
	if fields[3].Placeholder != "2.0" {
		t.Errorf("exchange ratio placeholder = %q, want %q", fields[3].Placeholder, "2.0")
	}

	// Field 4: Cash Per Share (text, optional)
	if fields[4].Type != FieldText {
		t.Errorf("field 4 type = %d, want FieldText (%d)", fields[4].Type, FieldText)
	}
	if fields[4].Label != "Cash Per Share" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Cash Per Share")
	}
	if fields[4].Required {
		t.Error("cash per share field should not be required")
	}
	if fields[4].Placeholder != "0.00" {
		t.Errorf("cash per share placeholder = %q, want %q", fields[4].Placeholder, "0.00")
	}

	// Buttons
	buttons := d.Buttons()
	if len(buttons) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(buttons))
	}
	if buttons[0].Label != "Execute" {
		t.Errorf("button 0 = %q, want Execute", buttons[0].Label)
	}
	if !buttons[0].Primary {
		t.Error("Execute button should be primary")
	}
	if buttons[1].Label != "Cancel" {
		t.Errorf("button 1 = %q, want Cancel", buttons[1].Label)
	}
}

func TestBuildMergerDialog_PreSelectedSource(t *testing.T) {
	id1 := types.NewID()
	id2 := types.NewID()
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{id1, id2}

	d := buildMergerDialog(options, ids, &id2)

	fields := d.Fields()
	if fields[0].SelectedIndex != 1 {
		t.Errorf("source selected index = %d, want 1 (MSFT)", fields[0].SelectedIndex)
	}
	// Target should default to 0
	if fields[1].SelectedIndex != 0 {
		t.Errorf("target selected index = %d, want 0", fields[1].SelectedIndex)
	}
}

func TestBuildMergerDialog_PreSelectedNotFound(t *testing.T) {
	id1 := types.NewID()
	unknownID := types.NewID()
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{id1}

	d := buildMergerDialog(options, ids, &unknownID)

	fields := d.Fields()
	if fields[0].SelectedIndex != 0 {
		t.Errorf("source selected index = %d, want 0 (fallback)", fields[0].SelectedIndex)
	}
}

func TestBuildMergerDialog_EmptySecurities(t *testing.T) {
	d := buildMergerDialog([]string{}, []types.ID{}, nil)

	if d == nil {
		t.Fatal("dialog should not be nil even with no securities")
	}
	fields := d.Fields()
	if len(fields[0].Options) != 0 {
		t.Errorf("expected 0 source options, got %d", len(fields[0].Options))
	}
	if len(fields[1].Options) != 0 {
		t.Errorf("expected 0 target options, got %d", len(fields[1].Options))
	}
}

func TestSubmitMergerDialog_ValidationErrors_AllEmpty(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	// Set invalid values
	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1 // different target so no same-security error
	fields[2].Value = "not-a-date"
	fields[3].Value = ""

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[2].Error == "" {
		t.Error("date field should have error")
	}
	if fields[3].Error == "" {
		t.Error("exchange ratio field should have error")
	}
}

func TestSubmitMergerDialog_SameSourceAndTarget(t *testing.T) {
	secID := types.NewID()
	secIDs := []types.ID{secID}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0 // source = AAPL
	fields[1].SelectedIndex = 0 // target = AAPL (same!)
	fields[2].Value = "03/15/2024"
	fields[3].Value = "2.0"

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open when source == target")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[1].Error == "" {
		t.Error("target field should have error when same as source")
	}
}

func TestSubmitMergerDialog_InvalidExchangeRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "abc" // not a number

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on invalid ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[3].Error == "" {
		t.Error("exchange ratio field should have error for invalid format")
	}
}

func TestSubmitMergerDialog_ZeroExchangeRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0" // zero ratio

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on zero ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[3].Error == "" {
		t.Error("exchange ratio field should have error for zero value")
	}
}

func TestSubmitMergerDialog_NegativeExchangeRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "-1.5" // negative

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on negative ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[3].Error == "" {
		t.Error("exchange ratio field should have error for negative value")
	}
}

func TestSubmitMergerDialog_NegativeCashPerShare(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "2.0"
	fields[4].Value = "-5.00" // negative cash

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on negative cash")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[4].Error == "" {
		t.Error("cash per share field should have error for negative value")
	}
}

func TestSubmitMergerDialog_InvalidCashPerShare(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "2.0"
	fields[4].Value = "abc" // not a number

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on invalid cash")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[4].Error == "" {
		t.Error("cash per share field should have error for invalid format")
	}
}

func TestSubmitMergerDialog_NoSecurities(t *testing.T) {
	app := &App{
		mergerDialog: buildMergerDialog([]string{}, []types.ID{}, nil),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{},
	}

	fields := app.mergerDialog.Fields()
	fields[2].Value = "03/15/2024"
	fields[3].Value = "2.0"

	model, _ := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.mergerDialog.Fields()
	if fields[0].Error == "" {
		t.Error("source security field should have error when no securities available")
	}
}

func TestSubmitMergerDialog_ValidSubmit(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0 // source = AAPL
	fields[1].SelectedIndex = 1 // target = MSFT
	fields[2].Value = "06/10/2024"
	fields[3].Value = "2.5"

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async execution")
	}
}

func TestSubmitMergerDialog_ValidSubmitWithCash(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0
	fields[1].SelectedIndex = 1
	fields[2].Value = "06/10/2024"
	fields[3].Value = "1.5"
	fields[4].Value = "10.00" // cash consideration

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed after valid submit with cash")
	}
	if cmd == nil {
		t.Error("should return a command for async execution")
	}
}

func TestSubmitMergerDialog_RatioWithSpaces(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0
	fields[1].SelectedIndex = 1
	fields[2].Value = "06/10/2024"
	fields[3].Value = "  2.0  " // spaces around ratio

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed (spaces trimmed)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitMergerDialog_InvalidDate(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	fields := app.mergerDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "13/45/2024" // invalid date
	fields[3].Value = "2.0"

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Error("dialog should remain open on invalid date")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.mergerDialog.Fields()
	if fields[2].Error == "" {
		t.Error("date field should have error for invalid date")
	}
}

func TestHandleMergerDialogKey_Cancel(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleMergerDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.mergerDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.mergerDialogSecurityIDs != nil {
		t.Error("dialog security IDs should be cleared after cancel")
	}
}

func TestHandleMergerDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleMergerDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitMergerDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitMergerDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseMergerDialog(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData:        &mergerDialogData{},
		mergerDialogSecurityIDs: secIDs,
	}

	app.closeMergerDialog()

	if app.mergerDialog != nil {
		t.Error("mergerDialog should be nil after close")
	}
	if app.mergerDialogData != nil {
		t.Error("mergerDialogData should be nil after close")
	}
	if app.mergerDialogSecurityIDs != nil {
		t.Error("mergerDialogSecurityIDs should be nil after close")
	}
}

func TestMergerDialogDataMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &mergerDialogData{
		securities: []*security.Security{sec},
	}

	msg := mergerDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
}

func TestMergerDialogSavedMsg(t *testing.T) {
	msg := mergerDialogSavedMsg{}
	_ = msg
}

func TestSubmitMergerDialog_ClearsErrorsBeforeValidation(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	// Set a pre-existing error
	fields := app.mergerDialog.Fields()
	fields[0].Error = "old error"
	fields[1].SelectedIndex = 1
	fields[2].Value = "not-a-date"
	fields[3].Value = "2.0"

	app.submitMergerDialog()

	fields = app.mergerDialog.Fields()
	// Old error on field 0 should be cleared
	if fields[0].Error != "" {
		t.Errorf("field 0 error should be cleared, got %q", fields[0].Error)
	}
	// Date should have new error
	if fields[2].Error == "" {
		t.Error("date field should have error")
	}
}

func TestHandleMergerDialogKey_TabNavigates(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: secIDs,
	}

	if app.mergerDialog.FocusIndex() != 0 {
		t.Errorf("initial focus = %d, want 0", app.mergerDialog.FocusIndex())
	}

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.handleMergerDialogKey(tabKey)

	if app.mergerDialog.FocusIndex() != 1 {
		t.Errorf("focus after tab = %d, want 1", app.mergerDialog.FocusIndex())
	}
}

func TestSubmitMergerDialog_EmptyCashPerShareIsValid(t *testing.T) {
	sourceID := types.NewID()
	targetID := types.NewID()

	app := &App{
		mergerDialog: buildMergerDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			[]types.ID{sourceID, targetID},
			nil,
		),
		mergerDialogData: &mergerDialogData{
			securities: []*security.Security{},
		},
		mergerDialogSecurityIDs: []types.ID{sourceID, targetID},
	}

	fields := app.mergerDialog.Fields()
	fields[0].SelectedIndex = 0
	fields[1].SelectedIndex = 1
	fields[2].Value = "06/10/2024"
	fields[3].Value = "2.0"
	fields[4].Value = "" // empty cash = 0, valid

	model, cmd := app.submitMergerDialog()
	updatedApp := model.(*App)

	if updatedApp.mergerDialog != nil {
		t.Error("dialog should be closed (empty cash is valid)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestApp_Update_MergerDialogSavedMsg(t *testing.T) {
	app := &App{
		statusbar: NewStatusBar(),
	}

	msg := mergerDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("mergerDialogSavedMsg should return a reload command")
	}
}
