package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildSpinOffDialog_NewDialog(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildSpinOffDialog(options, ids, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Spin-Off" {
		t.Errorf("title = %q, want %q", d.Title(), "Spin-Off")
	}

	fields := d.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}

	// Field 0: Parent Security (select)
	if fields[0].Type != FieldSelect {
		t.Errorf("field 0 type = %d, want FieldSelect (%d)", fields[0].Type, FieldSelect)
	}
	if fields[0].Label != "Parent Security" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Parent Security")
	}
	if len(fields[0].Options) != 2 {
		t.Errorf("expected 2 parent options, got %d", len(fields[0].Options))
	}
	if fields[0].SelectedIndex != 0 {
		t.Errorf("default parent index = %d, want 0", fields[0].SelectedIndex)
	}

	// Field 1: Spin-Off Security (select)
	if fields[1].Type != FieldSelect {
		t.Errorf("field 1 type = %d, want FieldSelect (%d)", fields[1].Type, FieldSelect)
	}
	if fields[1].Label != "Spin-Off Security" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Spin-Off Security")
	}
	if len(fields[1].Options) != 2 {
		t.Errorf("expected 2 spin-off options, got %d", len(fields[1].Options))
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

	// Field 3: Share Ratio (text, required)
	if fields[3].Type != FieldText {
		t.Errorf("field 3 type = %d, want FieldText (%d)", fields[3].Type, FieldText)
	}
	if fields[3].Label != "Share Ratio" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Share Ratio")
	}
	if !fields[3].Required {
		t.Error("share ratio field should be required")
	}
	if fields[3].Value != "" {
		t.Errorf("share ratio default = %q, want empty", fields[3].Value)
	}
	if fields[3].Placeholder != "0.5" {
		t.Errorf("share ratio placeholder = %q, want %q", fields[3].Placeholder, "0.5")
	}

	// Field 4: Parent Allocation % (text, required)
	if fields[4].Type != FieldText {
		t.Errorf("field 4 type = %d, want FieldText (%d)", fields[4].Type, FieldText)
	}
	if fields[4].Label != "Parent Allocation %" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Parent Allocation %")
	}
	if !fields[4].Required {
		t.Error("parent allocation field should be required")
	}
	if fields[4].Value != "" {
		t.Errorf("parent allocation default = %q, want empty", fields[4].Value)
	}
	if fields[4].Placeholder != "80" {
		t.Errorf("parent allocation placeholder = %q, want %q", fields[4].Placeholder, "80")
	}

	// Field 5: Spin-Off Price (text, required)
	if fields[5].Type != FieldText {
		t.Errorf("field 5 type = %d, want FieldText (%d)", fields[5].Type, FieldText)
	}
	if fields[5].Label != "Spin-Off Price" {
		t.Errorf("field 5 label = %q, want %q", fields[5].Label, "Spin-Off Price")
	}
	if !fields[5].Required {
		t.Error("spin-off price field should be required")
	}
	if fields[5].Value != "" {
		t.Errorf("spin-off price default = %q, want empty", fields[5].Value)
	}
	if fields[5].Placeholder != "25.00" {
		t.Errorf("spin-off price placeholder = %q, want %q", fields[5].Placeholder, "25.00")
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

func TestBuildSpinOffDialog_PreSelectedParent(t *testing.T) {
	id1 := types.NewID()
	id2 := types.NewID()
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{id1, id2}

	d := buildSpinOffDialog(options, ids, &id2)

	fields := d.Fields()
	if fields[0].SelectedIndex != 1 {
		t.Errorf("parent selected index = %d, want 1 (MSFT)", fields[0].SelectedIndex)
	}
	// Spin-off security should default to 0
	if fields[1].SelectedIndex != 0 {
		t.Errorf("spin-off selected index = %d, want 0", fields[1].SelectedIndex)
	}
}

func TestBuildSpinOffDialog_PreSelectedNotFound(t *testing.T) {
	id1 := types.NewID()
	unknownID := types.NewID()
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{id1}

	d := buildSpinOffDialog(options, ids, &unknownID)

	fields := d.Fields()
	if fields[0].SelectedIndex != 0 {
		t.Errorf("parent selected index = %d, want 0 (fallback)", fields[0].SelectedIndex)
	}
}

func TestBuildSpinOffDialog_EmptySecurities(t *testing.T) {
	d := buildSpinOffDialog([]string{}, []types.ID{}, nil)

	if d == nil {
		t.Fatal("dialog should not be nil even with no securities")
	}
	fields := d.Fields()
	if len(fields[0].Options) != 0 {
		t.Errorf("expected 0 parent options, got %d", len(fields[0].Options))
	}
	if len(fields[1].Options) != 0 {
		t.Errorf("expected 0 spin-off options, got %d", len(fields[1].Options))
	}
}

func TestSubmitSpinOffDialog_ValidationErrors_AllEmpty(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	// Set invalid values
	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1 // different target so no same-security error
	fields[2].Value = "not-a-date"
	fields[3].Value = ""
	fields[4].Value = ""
	fields[5].Value = ""

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[2].Error == "" {
		t.Error("date field should have error")
	}
	if fields[3].Error == "" {
		t.Error("share ratio field should have error")
	}
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error")
	}
	if fields[5].Error == "" {
		t.Error("spin-off price field should have error")
	}
}

func TestSubmitSpinOffDialog_SameParentAndSpinOff(t *testing.T) {
	secID := types.NewID()
	secIDs := []types.ID{secID}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[0].SelectedIndex = 0 // parent = AAPL
	fields[1].SelectedIndex = 0 // spin-off = AAPL (same!)
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open when parent == spin-off")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[1].Error == "" {
		t.Error("spin-off field should have error when same as parent")
	}
}

func TestSubmitSpinOffDialog_InvalidShareRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "abc" // not a number
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on invalid share ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[3].Error == "" {
		t.Error("share ratio field should have error for invalid format")
	}
}

func TestSubmitSpinOffDialog_ZeroShareRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0" // zero
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on zero share ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[3].Error == "" {
		t.Error("share ratio field should have error for zero value")
	}
}

func TestSubmitSpinOffDialog_NegativeShareRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "-0.5" // negative
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on negative share ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[3].Error == "" {
		t.Error("share ratio field should have error for negative value")
	}
}

func TestSubmitSpinOffDialog_InvalidParentAllocation(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "abc" // not a number
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on invalid allocation")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error for invalid format")
	}
}

func TestSubmitSpinOffDialog_ParentAllocationZero(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "0" // zero
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on zero allocation")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error for zero value")
	}
}

func TestSubmitSpinOffDialog_ParentAllocation100(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "100" // 100% means nothing for spin-off
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on 100% allocation")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error for 100% value")
	}
}

func TestSubmitSpinOffDialog_ParentAllocationOver100(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "150" // over 100%
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on >100% allocation")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error for >100% value")
	}
}

func TestSubmitSpinOffDialog_InvalidSpinOffPrice(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "abc" // not a number

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on invalid price")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[5].Error == "" {
		t.Error("spin-off price field should have error for invalid format")
	}
}

func TestSubmitSpinOffDialog_ZeroSpinOffPrice(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "0" // zero price

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on zero price")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[5].Error == "" {
		t.Error("spin-off price field should have error for zero value")
	}
}

func TestSubmitSpinOffDialog_NegativeSpinOffPrice(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "-10.00" // negative

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on negative price")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[5].Error == "" {
		t.Error("spin-off price field should have error for negative value")
	}
}

func TestSubmitSpinOffDialog_NoSecurities(t *testing.T) {
	app := &App{
		spinOffDialog: buildSpinOffDialog([]string{}, []types.ID{}, nil),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: []types.ID{},
	}

	fields := app.spinOffDialog.Fields()
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, _ := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.spinOffDialog.Fields()
	if fields[0].Error == "" {
		t.Error("parent security field should have error when no securities available")
	}
}

func TestSubmitSpinOffDialog_ValidSubmit(t *testing.T) {
	parentID := types.NewID()
	spinOffID := types.NewID()

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "NEWCO - NewCo Inc."},
			[]types.ID{parentID, spinOffID},
			nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: []types.ID{parentID, spinOffID},
	}

	fields := app.spinOffDialog.Fields()
	fields[0].SelectedIndex = 0 // parent = AAPL
	fields[1].SelectedIndex = 1 // spin-off = NEWCO
	fields[2].Value = "06/10/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async execution")
	}
}

func TestSubmitSpinOffDialog_ValidSubmitWithSpaces(t *testing.T) {
	parentID := types.NewID()
	spinOffID := types.NewID()

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "NEWCO - NewCo Inc."},
			[]types.ID{parentID, spinOffID},
			nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: []types.ID{parentID, spinOffID},
	}

	fields := app.spinOffDialog.Fields()
	fields[0].SelectedIndex = 0
	fields[1].SelectedIndex = 1
	fields[2].Value = "06/10/2024"
	fields[3].Value = "  0.5  " // spaces
	fields[4].Value = "  80  "  // spaces
	fields[5].Value = " 25.00 " // spaces

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog != nil {
		t.Error("dialog should be closed (spaces trimmed)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitSpinOffDialog_InvalidDate(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "13/45/2024" // invalid date
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on invalid date")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[2].Error == "" {
		t.Error("date field should have error for invalid date")
	}
}

func TestHandleSpinOffDialogKey_Cancel(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleSpinOffDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.spinOffDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.spinOffDialogSecurityIDs != nil {
		t.Error("dialog security IDs should be cleared after cancel")
	}
}

func TestHandleSpinOffDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleSpinOffDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitSpinOffDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitSpinOffDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseSpinOffDialog(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData:        &spinOffDialogData{},
		spinOffDialogSecurityIDs: secIDs,
	}

	app.closeSpinOffDialog()

	if app.spinOffDialog != nil {
		t.Error("spinOffDialog should be nil after close")
	}
	if app.spinOffDialogData != nil {
		t.Error("spinOffDialogData should be nil after close")
	}
	if app.spinOffDialogSecurityIDs != nil {
		t.Error("spinOffDialogSecurityIDs should be nil after close")
	}
}

func TestSpinOffDialogDataMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &spinOffDialogData{
		securities: []*security.Security{sec},
	}

	msg := spinOffDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
}

func TestSpinOffDialogSavedMsg(t *testing.T) {
	msg := spinOffDialogSavedMsg{}
	_ = msg
}

func TestSubmitSpinOffDialog_ClearsErrorsBeforeValidation(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	// Set a pre-existing error
	fields := app.spinOffDialog.Fields()
	fields[0].Error = "old error"
	fields[1].SelectedIndex = 1
	fields[2].Value = "not-a-date"
	fields[3].Value = "0.5"
	fields[4].Value = "80"
	fields[5].Value = "25.00"

	app.submitSpinOffDialog()

	fields = app.spinOffDialog.Fields()
	// Old error on field 0 should be cleared
	if fields[0].Error != "" {
		t.Errorf("field 0 error should be cleared, got %q", fields[0].Error)
	}
	// Date should have new error
	if fields[2].Error == "" {
		t.Error("date field should have error")
	}
}

func TestHandleSpinOffDialogKey_TabNavigates(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	if app.spinOffDialog.FocusIndex() != 0 {
		t.Errorf("initial focus = %d, want 0", app.spinOffDialog.FocusIndex())
	}

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.handleSpinOffDialogKey(tabKey)

	if app.spinOffDialog.FocusIndex() != 1 {
		t.Errorf("focus after tab = %d, want 1", app.spinOffDialog.FocusIndex())
	}
}

func TestApp_Update_SpinOffDialogSavedMsg(t *testing.T) {
	app := &App{
		statusbar: NewStatusBar(),
	}

	msg := spinOffDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("spinOffDialogSavedMsg should return a reload command")
	}
}

func TestSubmitSpinOffDialog_NegativeParentAllocation(t *testing.T) {
	secIDs := []types.ID{types.NewID(), types.NewID()}

	app := &App{
		spinOffDialog: buildSpinOffDialog(
			[]string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."},
			secIDs, nil,
		),
		spinOffDialogData: &spinOffDialogData{
			securities: []*security.Security{},
		},
		spinOffDialogSecurityIDs: secIDs,
	}

	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "03/15/2024"
	fields[3].Value = "0.5"
	fields[4].Value = "-10" // negative
	fields[5].Value = "25.00"

	model, cmd := app.submitSpinOffDialog()
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Error("dialog should remain open on negative allocation")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.spinOffDialog.Fields()
	if fields[4].Error == "" {
		t.Error("parent allocation field should have error for negative value")
	}
}
