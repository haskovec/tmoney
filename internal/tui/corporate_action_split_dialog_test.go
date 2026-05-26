package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildStockSplitDialog_NewDialog(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildStockSplitDialog(options, ids, nil, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Stock Split" {
		t.Errorf("title = %q, want %q", d.Title(), "Stock Split")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// dialog.Field 0: Security (typeahead combo)
	if fields[0].Type != dialog.FieldCombo {
		t.Errorf("field 0 type = %d, want dialog.FieldCombo (%d)", fields[0].Type, dialog.FieldCombo)
	}
	if fields[0].Label != "Security" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Security")
	}
	if len(fields[0].Options) != 2 {
		t.Errorf("expected 2 security options, got %d", len(fields[0].Options))
	}
	if fields[0].SelectedIndex != 0 {
		t.Errorf("default selected index = %d, want 0", fields[0].SelectedIndex)
	}

	// dialog.Field 1: Date (masked, required, default today)
	if fields[1].Type != dialog.FieldDate {
		t.Errorf("field 1 type = %d, want dialog.FieldDate (%d)", fields[1].Type, dialog.FieldDate)
	}
	if fields[1].Label != "Date" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Date")
	}
	if !fields[1].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[1].Value != today {
		t.Errorf("date default = %q, want %q", fields[1].Value, today)
	}

	// dialog.Field 2: Ratio (text, required)
	if fields[2].Type != dialog.FieldText {
		t.Errorf("field 2 type = %d, want dialog.FieldText (%d)", fields[2].Type, dialog.FieldText)
	}
	if fields[2].Label != "Ratio" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Ratio")
	}
	if !fields[2].Required {
		t.Error("ratio field should be required")
	}
	if fields[2].Value != "" {
		t.Errorf("ratio default = %q, want empty", fields[2].Value)
	}
	if fields[2].Placeholder != "4:1" {
		t.Errorf("ratio placeholder = %q, want %q", fields[2].Placeholder, "4:1")
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

// TestBuildStockSplitDialog_DateFieldMaskedOverwrite verifies the split
// dialog's Date field uses overwrite-style masked input — typing a digit
// replaces the digit at the cursor and auto-advances over the slash.
func TestBuildStockSplitDialog_DateFieldMaskedOverwrite(t *testing.T) {
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{types.NewID()}

	d := buildStockSplitDialog(options, ids, nil, nil)
	d.SetFocusIndex(1) // focus the Date field

	// Seed Value to a known date so the overwrite is deterministic.
	d.Fields()[1].Value = "03/15/2024"

	// Type "0" then "5" — overwrites "03" with "05", cursor advances skipping the slash.
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})

	if got := d.Fields()[1].Value; got != "05/15/2024" {
		t.Errorf("Value = %q, want %q (overwrite + skip slash)", got, "05/15/2024")
	}
}

func TestBuildStockSplitDialog_PreSelectedSecurity(t *testing.T) {
	id1 := types.NewID()
	id2 := types.NewID()
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{id1, id2}

	d := buildStockSplitDialog(options, ids, nil, &id2)

	fields := d.Fields()
	if fields[0].SelectedIndex != 1 {
		t.Errorf("security selected index = %d, want 1 (MSFT)", fields[0].SelectedIndex)
	}
}

func TestBuildStockSplitDialog_PreSelectedNotFound(t *testing.T) {
	id1 := types.NewID()
	unknownID := types.NewID()
	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{id1}

	d := buildStockSplitDialog(options, ids, nil, &unknownID)

	fields := d.Fields()
	if fields[0].SelectedIndex != 0 {
		t.Errorf("security selected index = %d, want 0 (fallback)", fields[0].SelectedIndex)
	}
}

func TestBuildStockSplitDialog_EmptySecurities(t *testing.T) {
	d := buildStockSplitDialog([]string{}, []types.ID{}, nil, nil)

	if d == nil {
		t.Fatal("dialog should not be nil even with no securities")
	}
	fields := d.Fields()
	if len(fields[0].Options) != 0 {
		t.Errorf("expected 0 options, got %d", len(fields[0].Options))
	}
}

func TestSubmitStockSplitDialog_ValidationErrors_AllEmpty(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{"AAPL - Apple Inc."}, secIDs, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: secIDs,
	}

	// Set invalid values
	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "not-a-date" // invalid date
	fields[2].Value = ""           // empty ratio

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	// Should not close dialog when errors exist
	if updatedApp.stockSplitDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	// Check field errors
	fields = updatedApp.stockSplitDialog.Fields()
	if fields[1].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("ratio field should have error")
	}
}

func TestSubmitStockSplitDialog_InvalidRatio(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{"AAPL - Apple Inc."}, secIDs, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: secIDs,
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "invalid" // not N:D format

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog == nil {
		t.Error("dialog should remain open on invalid ratio")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.stockSplitDialog.Fields()
	if fields[2].Error == "" {
		t.Error("ratio field should have error for invalid format")
	}
}

func TestSubmitStockSplitDialog_InvalidRatio_ZeroDenominator(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{"AAPL - Apple Inc."}, secIDs, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: secIDs,
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "4:0" // zero denominator

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog == nil {
		t.Error("dialog should remain open on zero denominator")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.stockSplitDialog.Fields()
	if fields[2].Error == "" {
		t.Error("ratio field should have error for zero denominator")
	}
}

func TestSubmitStockSplitDialog_NoSecurities(t *testing.T) {
	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{}, []types.ID{}, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{},
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "03/15/2024"
	fields[2].Value = "4:1"

	model, _ := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.stockSplitDialog.Fields()
	if fields[0].Error == "" {
		t.Error("security field should have error when no securities available")
	}
}

func TestSubmitStockSplitDialog_ValidForwardSplit(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "06/10/2024"
	fields[2].Value = "4:1"

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	// Should close dialog on valid submit
	if updatedApp.stockSplitDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async execution")
	}
}

func TestSubmitStockSplitDialog_ValidReverseSplit(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "06/10/2024"
	fields[2].Value = "1:10" // reverse split

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async execution")
	}
}

func TestHandleStockSplitDialogKey_Cancel(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleStockSplitDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.stockSplitDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.stockSplitDialogSecurityIDs != nil {
		t.Error("dialog security IDs should be cleared after cancel")
	}
}

func TestHandleStockSplitDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleStockSplitDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitStockSplitDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitStockSplitDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseStockSplitDialog(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData:        &stockSplitDialogData{},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	app.closeStockSplitDialog()

	if app.stockSplitDialog != nil {
		t.Error("stockSplitDialog should be nil after close")
	}
	if app.stockSplitDialogData != nil {
		t.Error("stockSplitDialogData should be nil after close")
	}
	if app.stockSplitDialogSecurityIDs != nil {
		t.Error("stockSplitDialogSecurityIDs should be nil after close")
	}
}

func TestStockSplitDialogDataMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &stockSplitDialogData{
		securities: []*security.Security{sec},
	}

	msg := stockSplitDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
}

func TestStockSplitDialogSavedMsg(t *testing.T) {
	msg := stockSplitDialogSavedMsg{}
	_ = msg
}

func TestSubmitStockSplitDialog_InvalidDate(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{"AAPL - Apple Inc."}, secIDs, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: secIDs,
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "13/45/2024" // invalid date
	fields[2].Value = "4:1"

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog == nil {
		t.Error("dialog should remain open on invalid date")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.stockSplitDialog.Fields()
	if fields[1].Error == "" {
		t.Error("date field should have error for invalid date")
	}
}

func TestSubmitStockSplitDialog_RatioWithSpaces(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	fields := app.stockSplitDialog.Fields()
	fields[1].Value = "06/10/2024"
	fields[2].Value = "  4:1  " // spaces around ratio

	model, cmd := app.submitStockSplitDialog()
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog != nil {
		t.Error("dialog should be closed (spaces trimmed)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestHandleStockSplitDialogKey_TabNavigates(t *testing.T) {
	secID := types.NewID()

	app := &App{
		stockSplitDialog: buildStockSplitDialog(
			[]string{"AAPL - Apple Inc."},
			[]types.ID{secID},
			nil,
			nil,
		),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: []types.ID{secID},
	}

	// Initial focus should be on field 0 (Security)
	if app.stockSplitDialog.FocusIndex() != 0 {
		t.Errorf("initial focus = %d, want 0", app.stockSplitDialog.FocusIndex())
	}

	// Tab to next field
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.handleStockSplitDialogKey(tabKey)

	if app.stockSplitDialog.FocusIndex() != 1 {
		t.Errorf("focus after tab = %d, want 1", app.stockSplitDialog.FocusIndex())
	}
}

func TestSubmitStockSplitDialog_ClearsErrorsBeforeValidation(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		stockSplitDialog: buildStockSplitDialog([]string{"AAPL - Apple Inc."}, secIDs, nil, nil),
		stockSplitDialogData: &stockSplitDialogData{
			securities: []*security.Security{},
		},
		stockSplitDialogSecurityIDs: secIDs,
	}

	// Set a pre-existing error
	fields := app.stockSplitDialog.Fields()
	fields[0].Error = "old error"
	fields[1].Value = "not-a-date"
	fields[2].Value = "4:1"

	app.submitStockSplitDialog()

	fields = app.stockSplitDialog.Fields()
	// Old error on field 0 should be cleared (no new error for security since it's valid)
	if fields[0].Error != "" {
		t.Errorf("field 0 error should be cleared, got %q", fields[0].Error)
	}
	// Date should have new error
	if fields[1].Error == "" {
		t.Error("date field should have error")
	}
}

// Test that ParseSplitRatio correctly parses valid and invalid ratios.
func TestParseSplitRatio_Integration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"forward split", "4:1", false},
		{"reverse split", "1:10", false},
		{"2:1 split", "2:1", false},
		{"invalid format", "abc", true},
		{"missing denominator", "4:", true},
		{"missing numerator", ":1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := investment.ParseSplitRatio(tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestApp_Update_StockSplitDialogSavedMsg(t *testing.T) {
	app := &App{
		statusbar: widget.NewStatusBar(),
	}

	msg := stockSplitDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("stockSplitDialogSavedMsg should return a reload command")
	}
}

func TestRenderSplitDialogMessage(t *testing.T) {
	secID := types.NewID()
	secIDs := []types.ID{secID}
	shares := []investment.AccountShares{
		{AccountName: "Brokerage", Shares: types.MustNewQuantity("5")},
		{AccountName: "IRA", Shares: types.MustNewQuantity("10")},
	}
	sharesMap := map[types.ID][]investment.AccountShares{secID: shares}

	t.Run("explainer always present", func(t *testing.T) {
		got := renderSplitDialogMessage(secIDs, sharesMap, 0, "")
		if !strings.Contains(got, "N:M") {
			t.Errorf("missing convention explainer: %q", got)
		}
	})

	t.Run("invalid ratio shows current positions only", func(t *testing.T) {
		got := renderSplitDialogMessage(secIDs, sharesMap, 0, "")
		if !strings.Contains(got, "Current positions:") {
			t.Errorf("expected 'Current positions:' header, got: %q", got)
		}
		if strings.Contains(got, "→") {
			t.Errorf("should not show projection arrow with empty ratio: %q", got)
		}
		if !strings.Contains(got, "Brokerage: 5 shares") {
			t.Errorf("missing Brokerage current shares: %q", got)
		}
	})

	t.Run("valid ratio projects shares per account", func(t *testing.T) {
		got := renderSplitDialogMessage(secIDs, sharesMap, 0, "2:1")
		if !strings.Contains(got, "After split:") {
			t.Errorf("expected 'After split:' header, got: %q", got)
		}
		if !strings.Contains(got, "Brokerage: 5 → 10 shares") {
			t.Errorf("missing forward projection for Brokerage: %q", got)
		}
		if !strings.Contains(got, "IRA: 10 → 20 shares") {
			t.Errorf("missing forward projection for IRA: %q", got)
		}
	})

	t.Run("reverse-split projection catches a backwards ratio", func(t *testing.T) {
		// The original user error: typing 1:2 instead of 2:1 for a doubling split.
		got := renderSplitDialogMessage(secIDs, sharesMap, 0, "1:2")
		if !strings.Contains(got, "Brokerage: 5 → 2.5 shares") {
			t.Errorf("expected 5 → 2.5 to surface the reversed ratio: %q", got)
		}
	})

	t.Run("no positions yields explanatory note", func(t *testing.T) {
		emptyMap := map[types.ID][]investment.AccountShares{}
		got := renderSplitDialogMessage(secIDs, emptyMap, 0, "2:1")
		if !strings.Contains(got, "No current positions") {
			t.Errorf("expected no-positions note, got: %q", got)
		}
	})
}
