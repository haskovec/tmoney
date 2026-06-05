package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Pure Function Tests - SplitDialog
// =============================================================================

// TestSplitDialog_Render_TurboVisionNoNakedResets guards the dark-band
// regression: SplitDialog.Render must run its content through
// widget.RepaintDialog before wrapping in the themed panel, just like
// dialog.Dialog.Render. Under a theme with an opaque dialog.bg
// (turbo-vision), the reversed selected row, muted [x]/imbalance, and
// placeholder memo cells emit inner SGR resets that would otherwise
// expose terminal-default bands. After the repaint, no reset may be
// followed immediately by a raw space.
func TestSplitDialog_Render_TurboVisionNoNakedResets(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	turbo, _, err := theme.LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin(turbo-vision): %v", err)
	}
	styles := widget.NewStyles()
	styles.ApplyTheme(turbo)

	amount := types.MustNewMoney("-150.00")
	catOptions := []string{"(None)", "Food", "Household"}
	catIDs := []types.ID{types.NilID, types.NewID(), types.NewID()}
	sd := NewSplitDialog(amount, catOptions, catIDs)

	out := sd.Render(styles)

	for _, naked := range []string{"\x1b[m ", "\x1b[0m "} {
		if idx := strings.Index(out, naked); idx >= 0 {
			t.Errorf("found naked reset %q followed by raw space at %d (terminal-default band):\n%q", naked, idx, out)
		}
	}
}

// TestSplitDialog_Render_SaveBeforeCancel locks in the Save-first button
// order so the split panel matches the app-wide convention.
func TestSplitDialog_Render_SaveBeforeCancel(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	out := sd.Render(widget.NewStyles())
	saveIdx := strings.Index(out, "Save")
	cancelIdx := strings.Index(out, "Cancel")
	if saveIdx < 0 || cancelIdx < 0 {
		t.Fatalf("expected both buttons rendered; save=%d cancel=%d", saveIdx, cancelIdx)
	}
	if saveIdx > cancelIdx {
		t.Errorf("Save should render before Cancel; saveIdx=%d cancelIdx=%d", saveIdx, cancelIdx)
	}
}

// TestSplitDialog_Render_LongCategoryDoesNotWrap guards the mouse
// hit-testing assumption that each split row renders as exactly one
// terminal line: a long category label must be truncated, not wrapped to
// a second line (which would shift the button row below where the
// hit-test expects it).
func TestSplitDialog_Render_LongCategoryDoesNotWrap(t *testing.T) {
	styles := widget.NewStyles()
	ids := []types.ID{types.NilID, types.NewID()}

	short := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, ids)
	short.rows[0].categoryIndex = 1

	long := NewSplitDialog(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Insurance > Long Term Disability Coverage Plus Extra Riders"}, ids)
	long.rows[0].categoryIndex = 1

	shortLines := strings.Count(short.Render(styles), "\n")
	longLines := strings.Count(long.Render(styles), "\n")
	if shortLines != longLines {
		t.Errorf("long category changed rendered line count (row wrapped): short=%d long=%d", shortLines, longLines)
	}
}

// TestSplitDialog_HandleMouseLocal_CloseButton verifies the [x] close
// button (top-right of the title row) cancels.
func TestSplitDialog_HandleMouseLocal_CloseButton(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	contentWidth := sd.width - dialog.DialogHorizontalOverhead // 58
	if got := sd.HandleMouseLocal(contentWidth-2, 0); got != dialog.DialogActionCancel {
		t.Errorf("click on [x] = %d, want DialogActionCancel", got)
	}
}

// TestSplitDialog_HandleMouseLocal_CancelButton verifies a click on the
// Cancel button cancels regardless of validity.
func TestSplitDialog_HandleMouseLocal_CancelButton(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	// 1 row -> buttons on line 13. With even spacing at contentWidth 58,
	// Cancel occupies x in [35,45).
	if got := sd.HandleMouseLocal(40, 13); got != dialog.DialogActionCancel {
		t.Errorf("click on Cancel = %d, want DialogActionCancel", got)
	}
	if sd.focus != splitFocusCancelBtn {
		t.Errorf("focus = %d, want splitFocusCancelBtn", sd.focus)
	}
}

// TestSplitDialog_HandleMouseLocal_SaveButton_Invalid verifies that
// clicking Save on an imbalanced/invalid dialog is detected (sets an
// error) but does not submit.
func TestSplitDialog_HandleMouseLocal_SaveButton_Invalid(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	if got := sd.HandleMouseLocal(15, 13); got != dialog.DialogActionNone {
		t.Errorf("click on Save (invalid) = %d, want DialogActionNone", got)
	}
	if sd.errorMsg == "" {
		t.Error("expected validation errorMsg to be set after Save click on invalid dialog")
	}
}

// TestSplitDialog_HandleMouseLocal_SaveButton_Valid verifies a click on
// Save submits a balanced dialog.
func TestSplitDialog_HandleMouseLocal_SaveButton_Valid(t *testing.T) {
	foodID := types.NewID()
	existing := []*transaction.Split{{CategoryID: foodID, Amount: types.MustNewMoney("-100.00")}}
	sd := NewSplitDialogFromExisting(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"}, []types.ID{types.NilID, foodID}, existing)
	if got := sd.HandleMouseLocal(15, 13); got != dialog.DialogActionSubmit {
		t.Errorf("click on Save (valid) = %d, want DialogActionSubmit (errorMsg=%q)", got, sd.errorMsg)
	}
}

// TestSplitDialog_HandleMouseLocal_RowSelect verifies clicking a split
// row focuses that row and the field under the cursor column.
func TestSplitDialog_HandleMouseLocal_RowSelect(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	// Row 0 renders on line 7. Click in the amount column.
	contentWidth := sd.width - dialog.DialogHorizontalOverhead
	catColW := contentWidth / 3
	if got := sd.HandleMouseLocal(catColW+2, 7); got != dialog.DialogActionNone {
		t.Errorf("row click = %d, want DialogActionNone", got)
	}
	if sd.focus != splitFocusRows || sd.rowIndex != 0 || sd.fieldFocus != splitFieldAmount {
		t.Errorf("after row click: focus=%d row=%d field=%d; want rows/0/amount", sd.focus, sd.rowIndex, sd.fieldFocus)
	}
}

// TestBuildPreviewHeaderMulti_NoButtons verifies the multi-line preview
// header carries no Save/Cancel buttons of its own.
func TestBuildPreviewHeaderMulti_NoButtons(t *testing.T) {
	h := buildPreviewHeaderMulti("05/15/2026", "Employer", "")
	// 4 fields (Date, Payee, Memo, Status), 0 buttons.
	if got := h.FocusableCount(); got != 4 {
		t.Errorf("FocusableCount() = %d, want 4 (fields only, no buttons)", got)
	}
	out := h.Render(widget.NewStyles())
	if strings.Contains(out, "[ Save ]") || strings.Contains(out, "[ Cancel ]") {
		t.Errorf("multi-line header should render no buttons; got:\n%s", out)
	}
}

// TestNewSplitDialogFromExisting_SeedsTransferRow verifies a split with
// TransferAccountID set seeds a transfer-mode row (not a "(None)" category
// row), that SetTransferTargets resolves the destination to an
// accountIndex, that validate passes without a category, and that
// buildSplits round-trips the transfer. Regression for paycheck transfer
// lines failing "category is required" on post.
func TestNewSplitDialogFromExisting_SeedsTransferRow(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()
	parentAccountID := types.NewID()
	foodID := types.NewID()

	existing := []*transaction.Split{
		{BaseModel: types.NewBaseModel(), CategoryID: foodID, Amount: types.MustNewMoney("-40.00")},
		{BaseModel: types.NewBaseModel(), TransferAccountID: types.NullableID{ID: checkingID, Valid: true}, Amount: types.MustNewMoney("-60.00")},
	}

	sd := NewSplitDialogFromExisting(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"}, []types.ID{types.NilID, foodID}, existing)
	// Checking is index 0 in the (parent-excluded) transfer-target list.
	sd.SetTransferTargets([]string{"Checking", "Savings"},
		[]types.ID{checkingID, savingsID}, parentAccountID)

	rows := sd.Rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].transferMode {
		t.Error("row 0 (category split) should not be transfer mode")
	}
	if !rows[1].transferMode {
		t.Fatal("row 1 (transfer split) should be transfer mode")
	}
	if rows[1].accountIndex != 0 {
		t.Errorf("row 1 accountIndex = %d, want 0 (Checking)", rows[1].accountIndex)
	}

	if err := sd.validate(); err != nil {
		t.Errorf("validate() = %v, want nil (transfer row needs no category)", err)
	}

	built, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits() error: %v", err)
	}
	if len(built) != 2 {
		t.Fatalf("built = %d splits, want 2", len(built))
	}
	if !built[1].TransferAccountID.Valid || built[1].TransferAccountID.ID != checkingID {
		t.Errorf("built[1] TransferAccountID = %+v, want valid %v", built[1].TransferAccountID, checkingID)
	}
	if !built[1].CategoryID.IsNil() {
		t.Errorf("built[1] CategoryID = %v, want nil for a transfer line", built[1].CategoryID)
	}
}

func TestNewSplitDialog(t *testing.T) {
	amount := types.MustNewMoney("-150.00")
	catOptions := []string{"(None)", "Food", "Household"}
	catIDs := []types.ID{types.NilID, types.NewID(), types.NewID()}

	sd := NewSplitDialog(amount, catOptions, catIDs)

	if !sd.IsVisible() {
		t.Error("should be visible after creation")
	}
	if len(sd.Rows()) != 1 {
		t.Errorf("expected 1 initial row, got %d", len(sd.Rows()))
	}
	if sd.Focus() != splitFocusRows {
		t.Errorf("focus = %d, want splitFocusRows", sd.Focus())
	}
	if sd.RowIndex() != 0 {
		t.Errorf("rowIndex = %d, want 0", sd.RowIndex())
	}
	if sd.FieldFocus() != splitFieldCategory {
		t.Errorf("fieldFocus = %d, want splitFieldCategory", sd.FieldFocus())
	}
}

func TestSplitDialog_Remaining_NoAmounts(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)"}, []types.ID{types.NilID})

	rem := sd.remaining()
	if !rem.Equal(amount) {
		t.Errorf("remaining = %s, want %s", rem.String(), amount.String())
	}
}

func TestSplitDialog_Remaining_Partial(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].amountField.Value = "-60.00"

	rem := sd.remaining()
	expected := types.MustNewMoney("-40.00")
	if !rem.Equal(expected) {
		t.Errorf("remaining = %s, want %s", rem.String(), expected.String())
	}
}

func TestSplitDialog_Remaining_Exact(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	sd.addRow()
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[1].amountField.Value = "-40.00"

	rem := sd.remaining()
	if !rem.IsZero() {
		t.Errorf("remaining = %s, want 0", rem.String())
	}
}

func TestSplitDialog_Remaining_OverAllocated(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.addRow()
	sd.rows[0].amountField.Value = "-80.00"
	sd.rows[1].amountField.Value = "-40.00"

	rem := sd.remaining()
	expected := types.MustNewMoney("20.00")
	if !rem.Equal(expected) {
		t.Errorf("remaining = %s, want %s", rem.String(), expected.String())
	}
}

func TestSplitDialog_AddRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	if len(sd.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(sd.rows))
	}

	sd.addRow()
	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after addRow, got %d", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()
	sd.addRow()

	if len(sd.rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(sd.rows))
	}

	sd.removeRow(1)
	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after remove, got %d", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow_MinimumOne(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.removeRow(0)
	if len(sd.rows) != 1 {
		t.Errorf("should not remove last row, got %d rows", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow_AdjustsFocus(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()
	sd.rowIndex = 1

	sd.removeRow(1)
	if sd.rowIndex != 0 {
		t.Errorf("rowIndex = %d, want 0 after removing focused row", sd.rowIndex)
	}
}

func TestSplitDialog_Validate_Valid(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID(), types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, catIDs)
	sd.addRow()

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[1].categoryIndex = 2
	sd.rows[1].amountField.Value = "-40.00"

	err := sd.validate()
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestSplitDialog_Validate_MissingCategory(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].categoryIndex = 0 // (None)
	sd.rows[0].amountField.Value = "-100.00"

	err := sd.validate()
	if err == nil {
		t.Error("expected error for missing category")
	}
	if !strings.Contains(err.Error(), "category is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_Validate_MissingAmount(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = ""

	err := sd.validate()
	if err == nil {
		t.Error("expected error for missing amount")
	}
	if !strings.Contains(err.Error(), "amount is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_Validate_SumMismatch(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, catIDs)

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-50.00"

	err := sd.validate()
	if err == nil {
		t.Error("expected error for sum mismatch")
	}
	if !strings.Contains(err.Error(), "must sum to") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_BuildSplits(t *testing.T) {
	catID1 := types.NewID()
	catID2 := types.NewID()
	catIDs := []types.ID{types.NilID, catID1, catID2}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, catIDs)
	sd.addRow()

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[0].memoField.Value = "Groceries"
	sd.rows[1].categoryIndex = 2
	sd.rows[1].amountField.Value = "-40.00"

	splits, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits error: %v", err)
	}

	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}

	if splits[0].CategoryID != catID1 {
		t.Errorf("split[0] category = %s, want %s", splits[0].CategoryID.String(), catID1.String())
	}
	if !splits[0].Amount.Equal(types.MustNewMoney("-60.00")) {
		t.Errorf("split[0] amount = %s, want -60.00", splits[0].Amount.String())
	}
	if !splits[0].Memo.Valid || splits[0].Memo.String != "Groceries" {
		t.Errorf("split[0] memo = %v, want 'Groceries'", splits[0].Memo)
	}

	if splits[1].CategoryID != catID2 {
		t.Errorf("split[1] category = %s, want %s", splits[1].CategoryID.String(), catID2.String())
	}
	if !splits[1].Amount.Equal(types.MustNewMoney("-40.00")) {
		t.Errorf("split[1] amount = %s, want -40.00", splits[1].Amount.String())
	}
	if splits[1].Memo.Valid {
		t.Errorf("split[1] memo should be empty, got %v", splits[1].Memo)
	}
}

// =============================================================================
// HandleKey Tests
// =============================================================================

func TestSplitDialog_HandleKey_EscCancels(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != dialog.DialogActionCancel {
		t.Errorf("Esc should return dialog.DialogActionCancel, got %d", action)
	}
}

func TestSplitDialog_HandleKey_TabCycles(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	// Start: rows, row 0, category
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldCategory {
		t.Fatal("unexpected initial focus")
	}

	// Tab -> amount
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldAmount {
		t.Errorf("after tab 1: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}

	// Tab -> memo
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldMemo {
		t.Errorf("after tab 2: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}

	// Tab -> add button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusAddBtn {
		t.Errorf("after tab 3: focus=%d, want splitFocusAddBtn", sd.focus)
	}

	// Tab -> save button (Save comes before Cancel)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusSaveBtn {
		t.Errorf("after tab 4: focus=%d, want splitFocusSaveBtn", sd.focus)
	}

	// Tab -> cancel button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusCancelBtn {
		t.Errorf("after tab 5: focus=%d, want splitFocusCancelBtn", sd.focus)
	}

	// Tab -> wrap to first row
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.rowIndex != 0 || sd.fieldFocus != splitFieldCategory {
		t.Errorf("after tab 6: focus=%d, row=%d, field=%d", sd.focus, sd.rowIndex, sd.fieldFocus)
	}
}

func TestSplitDialog_HandleKey_ShiftTabCycles(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	// Start at rows, row 0, category
	// Shift-Tab should wrap to the last button (Cancel)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusCancelBtn {
		t.Errorf("shift-tab from start: focus=%d, want splitFocusCancelBtn", sd.focus)
	}

	// Shift-Tab -> save
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusSaveBtn {
		t.Errorf("shift-tab: focus=%d, want splitFocusSaveBtn", sd.focus)
	}

	// Shift-Tab -> add
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusAddBtn {
		t.Errorf("shift-tab: focus=%d, want splitFocusAddBtn", sd.focus)
	}

	// Shift-Tab -> back to rows, last row, last field (memo)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldMemo {
		t.Errorf("shift-tab: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}
}

func TestSplitDialog_HandleKey_EnterOnSave_Valid(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, catIDs)
	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-100.00"

	sd.focus = splitFocusSaveBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionSubmit {
		t.Errorf("Enter on Save with valid data should return dialog.DialogActionSubmit, got %d", action)
	}
}

func TestSplitDialog_HandleKey_EnterOnSave_Invalid(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.focus = splitFocusSaveBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionNone {
		t.Errorf("Enter on Save with invalid data should return dialog.DialogActionNone, got %d", action)
	}
	if sd.errorMsg == "" {
		t.Error("expected error message to be set")
	}
}

func TestSplitDialog_HandleKey_EnterOnCancel(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.focus = splitFocusCancelBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionCancel {
		t.Errorf("Enter on Cancel should return dialog.DialogActionCancel, got %d", action)
	}
}

func TestSplitDialog_HandleKey_EnterOnAdd(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.focus = splitFocusAddBtn
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after Enter on Add, got %d", len(sd.rows))
	}
	if sd.focus != splitFocusRows || sd.rowIndex != 1 {
		t.Errorf("focus should be on new row: focus=%d, row=%d", sd.focus, sd.rowIndex)
	}
}

func TestSplitDialog_HandleKey_CategoryUpDown(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	if sd.rows[0].categoryIndex != 0 {
		t.Fatal("expected initial categoryIndex 0")
	}

	// Down -> Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 1 {
		t.Errorf("categoryIndex after down = %d, want 1", sd.rows[0].categoryIndex)
	}

	// Down -> Household
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 2 {
		t.Errorf("categoryIndex after down = %d, want 2", sd.rows[0].categoryIndex)
	}

	// Down -> Transfer → sentinel (appended after real categories)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 3 {
		t.Errorf("categoryIndex after down = %d, want 3 (Transfer sentinel)", sd.rows[0].categoryIndex)
	}

	// Down -> [+ Add new category…] sentinel (CC-004)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 4 {
		t.Errorf("categoryIndex after down = %d, want 4 (AddNew sentinel)", sd.rows[0].categoryIndex)
	}

	// Down at max (should stay at AddNew sentinel)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 4 {
		t.Errorf("categoryIndex should stay at 4 (AddNew), got %d", sd.rows[0].categoryIndex)
	}

	// Up -> Transfer
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if sd.rows[0].categoryIndex != 3 {
		t.Errorf("categoryIndex after up = %d, want 3 (Transfer)", sd.rows[0].categoryIndex)
	}

	// Up -> Household
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if sd.rows[0].categoryIndex != 2 {
		t.Errorf("categoryIndex after up = %d, want 2", sd.rows[0].categoryIndex)
	}
}

// TestSplitDialog_TransferSentinel_PresentInCategoryCombo asserts that
// the category combo for a split row exposes a `Transfer →` option as
// the trailing entry, reachable via Down navigation and rendered when
// selected. (MS-011 wires up the account-picker swap.)
func TestSplitDialog_TransferSentinel_PresentInCategoryCombo(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	sd := NewSplitDialog(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()})

	// Navigate Down past every real category — should land on the
	// Transfer sentinel at index len(categoryOptions).
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Transfer →

	if sd.rows[0].categoryIndex != 2 {
		t.Fatalf("after two Down presses, categoryIndex = %d, want 2 (Transfer sentinel)", sd.rows[0].categoryIndex)
	}
	if !sd.isTransferSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("expected categoryIndex %d to be the Transfer sentinel", sd.rows[0].categoryIndex)
	}

	out := sd.Render(styles)
	if !strings.Contains(out, "Transfer →") {
		t.Errorf("rendered dialog should display 'Transfer →' when the sentinel is selected; got:\n%s", out)
	}
}

// TestSplitDialog_SelectTransfer_OpensAccountPicker asserts the MS-011
// contract: when transfer targets are configured and the user lands on
// the Transfer sentinel, the row swaps into transfer mode — Up/Down now
// navigates the account picker (which excludes the parent transaction's
// account), the row renders with the picked account's name, and
// buildSplits emits a transfer-line split with category_id=NilID and
// transfer_account_id set to the picked account.
func TestSplitDialog_SelectTransfer_OpensAccountPicker(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)

	parentAcctID := types.NewID()
	savingsID := types.NewID()
	k401ID := types.NewID()

	sd := NewSplitDialog(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()})

	// Configure transfer targets: dialog must filter out the parent
	// account so users cannot self-transfer.
	sd.SetTransferTargets(
		[]string{"Checking", "Savings", "401k"},
		[]types.ID{parentAcctID, savingsID, k401ID},
		parentAcctID,
	)

	// Without picker configured the sentinel would just be a static
	// option. With a picker, landing on it should swap the row into
	// transfer mode.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Transfer sentinel + auto-swap

	row := sd.rows[0]
	if !row.transferMode {
		t.Fatalf("landing on Transfer sentinel with picker configured should put the row in transfer mode")
	}
	if row.accountIndex != 0 {
		t.Errorf("first transfer pick = %d, want 0", row.accountIndex)
	}

	// The picker must exclude the parent account ("Checking"). With
	// Checking filtered out, the remaining picker is [Savings, 401k];
	// accountIndex 0 should resolve to Savings.
	if got := sd.transferAccountLabel(row.accountIndex); got != "Savings" {
		t.Errorf("first pick label = %q, want %q", got, "Savings")
	}

	// Further Down keys cycle the account picker without leaving the
	// sentinel column.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> 401k
	if sd.rows[0].accountIndex != 1 {
		t.Errorf("after second Down, accountIndex = %d, want 1 (401k)", sd.rows[0].accountIndex)
	}
	// Down past the last account exits transferMode and lands on the
	// AddNew sentinel (CC-004) so the AddNew action row is reachable
	// from the natural Down-Down-Down navigation path.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].transferMode {
		t.Errorf("Down at last account should exit transferMode")
	}
	if !sd.isAddNewSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("Down at last account should land on AddNew sentinel; got categoryIndex=%d", sd.rows[0].categoryIndex)
	}
	// Saturate at AddNew.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !sd.isAddNewSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("Down past AddNew should saturate; got categoryIndex=%d", sd.rows[0].categoryIndex)
	}

	// Re-enter transferMode to exercise the Up-from-account-zero path.
	sd.rows[0].categoryIndex = 1                     // Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Transfer + auto-swap
	if !sd.rows[0].transferMode {
		t.Fatalf("re-entry: Down from Food should auto-swap into transferMode")
	}
	// Up at accountIndex=0 reverts to category mode at the last real
	// category (existing behavior — Up never enters transferMode and
	// never lands on the Transfer sentinel; it jumps past it).
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if sd.rows[0].transferMode {
		t.Errorf("Up from first account should leave transfer mode")
	}
	if sd.rows[0].categoryIndex != 1 {
		t.Errorf("after reverting to category mode, categoryIndex = %d, want 1 (last real category)", sd.rows[0].categoryIndex)
	}

	// Re-enter transfer mode, fill in an amount, and assert buildSplits
	// produces a transfer-line split with the picked account and no
	// category.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> sentinel again, auto-swap
	sd.rows[0].amountField.Value = "-100.00"
	sd.rows[0].memoField.Value = "401(k) contribution"
	sd.rows[0].accountIndex = 1 // pick 401k explicitly

	splits, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits: %v", err)
	}
	if len(splits) != 1 {
		t.Fatalf("expected 1 split, got %d", len(splits))
	}
	got := splits[0]
	if !got.CategoryID.IsNil() {
		t.Errorf("transfer-line split should leave CategoryID NilID, got %s", got.CategoryID.String())
	}
	if !got.TransferAccountID.Valid {
		t.Fatalf("transfer-line split should set TransferAccountID")
	}
	if got.TransferAccountID.ID != k401ID {
		t.Errorf("TransferAccountID = %s, want %s (401k)", got.TransferAccountID.ID.String(), k401ID.String())
	}
	if got.TransferID.Valid {
		t.Errorf("buildSplits should leave TransferID empty; service mints it (MS-006). got %s", got.TransferID.ID.String())
	}
	if !got.Amount.Equal(types.MustNewMoney("-100.00")) {
		t.Errorf("amount = %s, want -100.00", got.Amount.String())
	}
	if !got.Memo.Valid || got.Memo.String != "401(k) contribution" {
		t.Errorf("memo = %v, want \"401(k) contribution\"", got.Memo)
	}

	// Render check: the picker label must surface as "Transfer → 401k".
	out := sd.Render(styles)
	if !strings.Contains(out, "Transfer →") || !strings.Contains(out, "401k") {
		t.Errorf("render should show Transfer → 401k, got:\n%s", out)
	}
	if strings.Contains(out, "Transfer → Checking") {
		t.Errorf("parent account 'Checking' must be excluded from the picker; got:\n%s", out)
	}
}

// TestSplitDialog_MixedSignAmounts_Accepted asserts the MS-012
// contract: the split dialog accepts split lines whose signs differ
// from the parent transaction's sign (and from each other), and
// persists them as entered. Validation only requires the signed sum
// of the lines to equal the parent amount — there is no
// same-sign-as-parent enforcement.
func TestSplitDialog_MixedSignAmounts_Accepted(t *testing.T) {
	salaryID := types.NewID()
	taxID := types.NewID()
	options := []string{"(None)", "Income:Salary", "Tax:Federal"}
	ids := []types.ID{types.NilID, salaryID, taxID}

	// Paycheck-shaped parent: +100 net deposit, with a +200 gross
	// line and a -100 tax line. Signed sum = +100, matching parent.
	sd := NewSplitDialog(types.MustNewMoney("100.00"), options, ids)
	sd.addRow()

	sd.rows[0].categoryIndex = 1 // Income:Salary
	sd.rows[0].amountField.Value = "200.00"
	sd.rows[1].categoryIndex = 2 // Tax:Federal
	sd.rows[1].amountField.Value = "-100.00"

	if err := sd.validate(); err != nil {
		t.Fatalf("mixed-sign split should validate, got: %v", err)
	}

	splits, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits: %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	if !splits[0].Amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("split[0] amount = %s, want +200.00", splits[0].Amount.String())
	}
	if !splits[1].Amount.Equal(types.MustNewMoney("-100.00")) {
		t.Errorf("split[1] amount = %s, want -100.00", splits[1].Amount.String())
	}

	// Three-line mixed-sign with a negative parent and lines whose
	// signs are independent: parent -50 = -200 (rent) + +150 (refund)
	// + 0 isn't allowed (amount required), so pick -200 / +200 / -50.
	sd2 := NewSplitDialog(types.MustNewMoney("-50.00"), options, ids)
	sd2.addRow()
	sd2.addRow()

	sd2.rows[0].categoryIndex = 2
	sd2.rows[0].amountField.Value = "-200.00"
	sd2.rows[1].categoryIndex = 1
	sd2.rows[1].amountField.Value = "200.00"
	sd2.rows[2].categoryIndex = 2
	sd2.rows[2].amountField.Value = "-50.00"

	if err := sd2.validate(); err != nil {
		t.Fatalf("three-line mixed-sign split should validate, got: %v", err)
	}
	splits2, err := sd2.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits: %v", err)
	}
	if len(splits2) != 3 {
		t.Fatalf("expected 3 splits, got %d", len(splits2))
	}

	// And: typing a leading '-' into the amount field is preserved.
	// (No filtering of the minus character.)
	sd3 := NewSplitDialog(types.MustNewMoney("0.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
	sd3.fieldFocus = splitFieldAmount
	for _, r := range "-25.00" {
		sd3.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if sd3.rows[0].amountField.Value != "-25.00" {
		t.Errorf("amount input after typing '-25.00' = %q, want %q", sd3.rows[0].amountField.Value, "-25.00")
	}
}

func TestSplitDialog_HandleKey_TextEditing(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	// Move to amount field
	sd.fieldFocus = splitFieldAmount

	// Type some characters
	sd.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})
	sd.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})

	if sd.rows[0].amountField.Value != "50" {
		t.Errorf("amount value = %q, want '50'", sd.rows[0].amountField.Value)
	}

	// Backspace
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if sd.rows[0].amountField.Value != "5" {
		t.Errorf("amount value after backspace = %q, want '5'", sd.rows[0].amountField.Value)
	}
}

func TestSplitDialog_HandleKey_CtrlD_RemovesRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()

	if len(sd.rows) != 2 {
		t.Fatal("expected 2 rows")
	}

	sd.HandleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if len(sd.rows) != 1 {
		t.Errorf("expected 1 row after Ctrl+D, got %d", len(sd.rows))
	}
}

func TestSplitDialog_HandleKey_CtrlD_NoRemoveLastRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.HandleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if len(sd.rows) != 1 {
		t.Errorf("should not remove last row, got %d", len(sd.rows))
	}
}

// TestSplitDialog_ImbalanceIndicator_VisibleAndLive asserts the MS-013
// contract: the rendered dialog exposes a labeled "Imbalance: $X.XX"
// indicator that recomputes per keystroke as line amounts change. The
// indicator sits between the line list and the action buttons so the
// imbalance is visible right where the user's eyes land before
// committing.
func TestSplitDialog_ImbalanceIndicator_VisibleAndLive(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)

	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("100.00"),
		[]string{"(None)", "Salary"}, catIDs)
	sd.rows[0].categoryIndex = 1

	// Empty amount: imbalance equals the parent amount.
	out := sd.Render(styles)
	if !strings.Contains(out, "Imbalance:") {
		t.Fatalf("render should display Imbalance indicator; got:\n%s", out)
	}
	if !strings.Contains(out, "Imbalance: $100.00") {
		t.Errorf("render should display Imbalance: $100.00 when nothing has been entered; got:\n%s", out)
	}

	// Position the indicator between the rows area and the buttons.
	idxImb := strings.Index(out, "Imbalance:")
	idxAdd := strings.Index(out, "Add split")
	idxCancel := strings.Index(out, "Cancel")
	if idxImb < idxAdd {
		t.Errorf("Imbalance indicator should render below the line list (Add split row); got Imbalance at %d, Add split at %d", idxImb, idxAdd)
	}
	if idxImb > idxCancel {
		t.Errorf("Imbalance indicator should render above the Cancel/Save action buttons; got Imbalance at %d, Cancel at %d", idxImb, idxCancel)
	}

	// Type a partial amount one keystroke at a time and assert the
	// indicator recomputes after each keystroke. The dialog parses on
	// every render via remaining(), so each keystroke must produce a
	// re-rendered indicator with the new value.
	sd.fieldFocus = splitFieldAmount
	sd.HandleKey(tea.KeyPressMsg{Code: '6', Text: "6"})
	out = sd.Render(styles)
	if !strings.Contains(out, "Imbalance: $94.00") {
		t.Errorf("after typing '6', imbalance should be $94.00; got:\n%s", out)
	}

	sd.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	out = sd.Render(styles)
	if !strings.Contains(out, "Imbalance: $40.00") {
		t.Errorf("after typing '60', imbalance should be $40.00; got:\n%s", out)
	}

	// Drive the amount to exactly the parent total: indicator zeroes out.
	sd.rows[0].amountField.Value = "100.00"
	out = sd.Render(styles)
	if !strings.Contains(out, "Imbalance: $0.00") {
		t.Errorf("with line summing to parent, imbalance should be $0.00; got:\n%s", out)
	}
}

// TestSplitDialog_SaveDisabledOnImbalance asserts the MS-013 contract:
// the Save button is disabled when the signed sum of line amounts does
// not match the parent transaction's amount, and pressing Enter on the
// disabled Save button does not submit the dialog (it surfaces an
// error message instead).
func TestSplitDialog_SaveDisabledOnImbalance(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"}, catIDs)
	sd.rows[0].categoryIndex = 1

	// No amount entered: imbalance = -$100.00 → Save disabled.
	if sd.IsSaveEnabled() {
		t.Error("Save should be disabled when imbalance ≠ 0 (empty amount)")
	}

	// Partial amount: imbalance still ≠ 0 → Save still disabled.
	sd.rows[0].amountField.Value = "-50.00"
	if sd.IsSaveEnabled() {
		t.Error("Save should be disabled when imbalance ≠ 0 (-50 vs parent -100)")
	}

	// Pressing Enter on the disabled Save button does not submit.
	sd.focus = splitFocusSaveBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action == dialog.DialogActionSubmit {
		t.Errorf("Enter on disabled Save must not submit; got action %d", action)
	}
	if sd.errorMsg == "" {
		t.Error("expected an error message after Enter on disabled Save")
	}

	// Balance the split: Save becomes enabled.
	sd.rows[0].amountField.Value = "-100.00"
	if !sd.IsSaveEnabled() {
		t.Error("Save should be enabled when imbalance = 0")
	}

	// And Enter on enabled Save submits.
	sd.focus = splitFocusSaveBtn
	action = sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionSubmit {
		t.Errorf("Enter on enabled Save should submit; got action %d", action)
	}
}

// =============================================================================
// Render Tests
// =============================================================================

func TestSplitDialog_Render(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	sd := NewSplitDialog(types.MustNewMoney("-150.00"), []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	output := sd.Render(styles)

	checks := []string{
		"SPLIT TRANSACTION",
		"$150.00",
		"Remaining",
		"Category",
		"Amount",
		"Memo",
		"Add split",
		"Cancel",
		"Save",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("render output should contain %q", check)
		}
	}
}

// =============================================================================
// App Integration Tests
// =============================================================================

func TestApp_SubmitTransactionDialog_SplitChecked(t *testing.T) {
	accountID := types.NewID()
	catID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddTextField("Date", "01/15/2024", "", 10)
			d.AddTextField("Payee", "Grocery Store", "", 0)
			d.AddSelectField("Category", []string{"(None)", "Food"}, 1)
			d.AddTextField("Amount", "-150.00", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", true) // checked!
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
			categories: []*category.Category{
				{
					BaseModel: types.BaseModel{ID: catID},
					Name:      "Food",
					Type:      category.TypeExpense,
				},
			},
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, catID},
	}

	// Set up sidebar with a selected account
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	// Submit the dialog (focus on Save button)
	app.txnDialog.SetFocusIndex(len(app.txnDialog.Fields()))
	app.txnDialog.FocusNext() // move to Save button

	model, cmd := app.submitTransactionDialog()
	updatedApp := model.(*App)

	// Transaction dialog should be closed
	if updatedApp.txnDialog != nil {
		t.Error("txnDialog should be nil after split submit")
	}

	// Split dialog should be open
	if updatedApp.splitDialog == nil {
		t.Fatal("splitDialog should be created when split is checked")
	}
	if !updatedApp.splitDialog.IsVisible() {
		t.Error("splitDialog should be visible")
	}

	// Pending transaction should be stored
	if updatedApp.pendingSplitTxn == nil {
		t.Fatal("pendingSplitTxn should be set")
	}
	if updatedApp.pendingSplitTxn.payeeName != "Grocery Store" {
		t.Errorf("pendingSplitTxn.payeeName = %q, want 'Grocery Store'", updatedApp.pendingSplitTxn.payeeName)
	}
	if !updatedApp.pendingSplitTxn.amount.Equal(types.MustNewMoney("-150.00")) {
		t.Errorf("pendingSplitTxn.amount = %s, want -150.00", updatedApp.pendingSplitTxn.amount.String())
	}

	// Should not return an async command (dialog opens synchronously)
	if cmd != nil {
		t.Error("split dialog opening should not return a cmd")
	}
}

func TestApp_HandleSplitDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID}),
		pendingSplitTxn: &pendingSplitTransaction{
			amount: types.MustNewMoney("-100.00"),
		},
	}

	// Press Escape to cancel
	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.splitDialog != nil {
		t.Error("splitDialog should be nil after cancel")
	}
	if updatedApp.pendingSplitTxn != nil {
		t.Error("pendingSplitTxn should be nil after cancel")
	}
}

func TestApp_Update_SplitDialogSavedMsg(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	msg := splitDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("splitDialogSavedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithSplitDialog(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: types.NewID()},
				Name:      "Checking",
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()}),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "SPLIT TRANSACTION") {
		t.Error("renderLayout() should contain 'SPLIT TRANSACTION' when split dialog is visible")
	}
}

func TestApp_SplitDialogKeyRouting(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		ready:       true,
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()}),
		pendingSplitTxn: &pendingSplitTransaction{
			amount: types.MustNewMoney("-100.00"),
		},
	}

	// Tab key should be routed to split dialog, not to register view
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.Update(tabKey)

	// After tab, split dialog should have advanced focus
	if app.splitDialog.fieldFocus != splitFieldAmount {
		t.Errorf("split dialog field focus should advance on Tab, got %d", app.splitDialog.fieldFocus)
	}
}

// =============================================================================
// Edit-mode tests (Phase 3: split transaction edit)
// =============================================================================

// TestNewSplitDialogFromExisting asserts that constructing a split dialog
// from an existing slice of splits seeds one row per split, with the
// matching category index pre-selected and the amount and memo
// pre-populated.
func TestNewSplitDialogFromExisting(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	cat1 := types.NewID()
	cat2 := types.NewID()
	cat3 := types.NewID()
	options := []string{"(None)", "Food", "Drink", "Snack"}
	ids := []types.ID{types.NilID, cat1, cat2, cat3}

	existing := []*transaction.Split{
		{
			BaseModel:  types.BaseModel{ID: types.NewID()},
			CategoryID: cat1,
			Amount:     types.MustNewMoney("-60.00"),
			Memo:       types.NullableString{String: "lunch", Valid: true},
		},
		{
			BaseModel:  types.BaseModel{ID: types.NewID()},
			CategoryID: cat3,
			Amount:     types.MustNewMoney("-40.00"),
			Memo:       types.NullableString{String: "tip", Valid: true},
		},
	}

	sd := NewSplitDialogFromExisting(amount, options, ids, existing)

	if !sd.IsVisible() {
		t.Error("dialog should be visible after creation")
	}
	rows := sd.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].categoryIndex != 1 { // Food at idx 1
		t.Errorf("row[0] categoryIndex = %d, want 1 (Food)", rows[0].categoryIndex)
	}
	if rows[0].amountField.Value != "-60" {
		t.Errorf("row[0] amount = %q, want %q", rows[0].amountField.Value, "-60")
	}
	if rows[0].memoField.Value != "lunch" {
		t.Errorf("row[0] memo = %q, want %q", rows[0].memoField.Value, "lunch")
	}
	if rows[1].categoryIndex != 3 { // Snack at idx 3
		t.Errorf("row[1] categoryIndex = %d, want 3 (Snack)", rows[1].categoryIndex)
	}
	if rows[1].amountField.Value != "-40" {
		t.Errorf("row[1] amount = %q, want %q", rows[1].amountField.Value, "-40")
	}
	// remaining() should be zero since the two splits sum to amount.
	if !sd.remaining().IsZero() {
		t.Errorf("remaining = %s, want 0 for fully allocated splits", sd.remaining().String())
	}
}

// TestApp_HandleRegisterKeys_EnterOnSplitTransaction_OpensEditFlow asserts
// Enter on a split (non-transfer) row in the register returns a non-nil
// load cmd. The transaction-load path inspects splits as part of the data
// fetch.
func TestApp_HandleRegisterKeys_EnterOnSplitTransaction_OpensEditFlow(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
			},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-100"),
					Status:    transaction.StatusUncleared,
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handleRegisterKeys(enter)
	if cmd == nil {
		t.Error("Enter on a (potentially split) plain row should return a load cmd")
	}
}

// TestApp_BuildTransactionDialog_EditMode_ChecksSplitBoxIfSplitsExist
// asserts that in edit mode, when the existing transaction has at least
// one split, the Split-transaction checkbox is pre-checked.
func TestApp_BuildTransactionDialog_EditMode_ChecksSplitBoxIfSplitsExist(t *testing.T) {
	cats := []*category.Category{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Auto", Type: category.TypeExpense},
	}
	options, ids := buildCategoryOptions(cats)

	existing := transaction.NewTransaction(
		types.NewID(),
		types.NewDate(2024, 3, 15),
		types.MustNewMoney("-100.00"),
	)
	splits := []*transaction.Split{
		{BaseModel: types.BaseModel{ID: types.NewID()}, CategoryID: cats[0].ID, Amount: types.MustNewMoney("-100.00")},
	}

	data := &transactionDialogData{
		categories:     cats,
		mode:           transactionDialogModeEdit,
		existing:       existing,
		existingSplits: splits,
	}

	d := buildTransactionDialog(data, options, ids, types.ZeroDate)

	fields := d.Fields()
	if !fields[6].Checked {
		t.Error("split checkbox should be checked when editing a transaction with splits")
	}
}

// =============================================================================
// CC-004 — [+ Add new category…] sentinel
// =============================================================================

// TestSplitDialog_CategoryCount_IncludesAddNewSentinel pins that the split
// row's category option space carries BOTH sentinels — `Transfer →` and
// `[+ Add new category…]` — past the real categories.
func TestSplitDialog_CategoryCount_IncludesAddNewSentinel(t *testing.T) {
	sd := NewSplitDialog(
		types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food", "Household"},
		[]types.ID{types.NilID, types.NewID(), types.NewID()},
	)

	// 3 real categories + Transfer sentinel + AddNew sentinel = 5
	if got, want := sd.categoryOptionCount(), 5; got != want {
		t.Errorf("categoryOptionCount() = %d, want %d (3 real + Transfer + AddNew)", got, want)
	}
	if !sd.isAddNewSentinel(4) {
		t.Errorf("isAddNewSentinel(4) should be true (AddNew sentinel sits past Transfer)")
	}
	if sd.isAddNewSentinel(3) {
		t.Errorf("isAddNewSentinel(3) should be false (that's the Transfer sentinel)")
	}
	if got, want := sd.categoryOptionLabel(4), "[+ Add new category…]"; got != want {
		t.Errorf("categoryOptionLabel(4) = %q, want %q", got, want)
	}
}

// TestSplitDialog_DownPastTransfer_LandsOnAddNew exercises the navigation
// path: from the last real category, Down lands on Transfer; another Down
// reveals the AddNew sentinel; subsequent Down saturates there.
// Transfer targets are NOT configured here so Transfer is inert (no
// auto-swap into transferMode).
func TestSplitDialog_DownPastTransfer_LandsOnAddNew(t *testing.T) {
	sd := NewSplitDialog(
		types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()},
	)

	// (None) → Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	// Food → Transfer
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !sd.isTransferSentinel(sd.rows[0].categoryIndex) {
		t.Fatalf("after two Down presses, expected Transfer sentinel; got %d", sd.rows[0].categoryIndex)
	}

	// Transfer → AddNew
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !sd.isAddNewSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("after three Down presses, expected AddNew sentinel; got %d", sd.rows[0].categoryIndex)
	}
	if sd.rows[0].transferMode {
		t.Errorf("landing on AddNew must not switch into transferMode")
	}

	// Down at saturation stays put.
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !sd.isAddNewSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("Down past AddNew should saturate; got categoryIndex=%d", sd.rows[0].categoryIndex)
	}

	// Up steps back to Transfer, then to last real cat (Food).
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !sd.isTransferSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("Up from AddNew should step back to Transfer; got %d", sd.rows[0].categoryIndex)
	}
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if sd.rows[0].categoryIndex != 1 {
		t.Errorf("Up from Transfer should step back to last real cat (1); got %d", sd.rows[0].categoryIndex)
	}
}

// TestSplitDialog_EnterOnAddNew_ReturnsDialogActionAddNew pins that Enter
// on the AddNew sentinel produces dialog.DialogActionAddNew (so the App-level
// router can divert into the create-category sub-dialog). Enter on a
// real category, by contrast, just advances focus to Amount.
func TestSplitDialog_EnterOnAddNew_ReturnsDialogActionAddNew(t *testing.T) {
	sd := NewSplitDialog(
		types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()},
	)
	// Park on AddNew sentinel directly.
	sd.rows[0].categoryIndex = 3 // (None)=0, Food=1, Transfer=2, AddNew=3

	got := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got != dialog.DialogActionAddNew {
		t.Errorf("Enter on AddNew sentinel = %v, want dialog.DialogActionAddNew", got)
	}
}

// TestSplitDialog_EnterOnRealCategory_AdvancesFocus pins the regression
// that the new AddNew handling on Enter doesn't break the existing
// "Enter on Category field advances to Amount" behavior for non-sentinel
// selections.
func TestSplitDialog_EnterOnRealCategory_AdvancesFocus(t *testing.T) {
	sd := NewSplitDialog(
		types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()},
	)
	sd.rows[0].categoryIndex = 1 // Food
	got := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got != dialog.DialogActionNone {
		t.Errorf("Enter on real category should not propagate as AddNew; got %v", got)
	}
	if sd.FieldFocus() != splitFieldAmount {
		t.Errorf("Enter on Category should advance focus to Amount; got %v", sd.FieldFocus())
	}
}

// TestSplitDialog_Validate_RejectsAddNewSentinel pins that landing on the
// AddNew sentinel without ever activating it is not a valid saveable
// state — validate rejects it with the same "category required" wording
// the (None) row produces.
func TestSplitDialog_Validate_RejectsAddNewSentinel(t *testing.T) {
	sd := NewSplitDialog(
		types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()},
	)
	sd.rows[0].categoryIndex = 3 // AddNew
	sd.rows[0].amountField.Value = "-100.00"

	err := sd.validate()
	if err == nil {
		t.Fatal("validate should reject the AddNew sentinel as a saveable category")
	}
}

// newAppForSplitAddNew builds an *App with a SplitDialog whose first row
// is parked on the AddNew sentinel and is otherwise pre-populated so we
// can assert that field values survive the open-cancel and open-submit
// round-trips. categorySvc may be nil for tests that don't drive submit.
func newAppForSplitAddNew(t *testing.T, categorySvc *category.Service, cats []*category.Category) *App {
	t.Helper()
	categoryOptions, categoryIDs := buildCategoryOptions(cats)
	if len(categoryOptions) == 0 {
		categoryOptions = []string{"(None)"}
		categoryIDs = []types.ID{types.NilID}
	}

	parentAcctID := types.NewID()
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), categoryOptions, categoryIDs)
	// Seed scalar state we expect to see preserved across the divert.
	sd.rows[0].amountField.Value = "-100.00"
	sd.rows[0].memoField.Value = "groceries"
	// Park the row on the AddNew sentinel.
	sd.rows[0].categoryIndex = sd.categoryOptionCount() - 1
	if !sd.isAddNewSentinel(sd.rows[0].categoryIndex) {
		t.Fatalf("test setup: rows[0].categoryIndex should be on AddNew sentinel")
	}

	app := &App{
		keys:              defaultKeyMap(),
		menubar:           widget.NewMenuBar(),
		statusbar:         widget.NewStatusBar(),
		sidebar:           NewSidebar(),
		categorySvc:       categorySvc,
		splitDialog:       sd,
		createCatSplitRow: -1,
		pendingSplitTxn: &pendingSplitTransaction{
			accountID: parentAcctID,
			amount:    types.MustNewMoney("-100.00"),
		},
	}
	return app
}

func TestApp_SplitDialog_AddNew_OpensCreateCategoryDialog(t *testing.T) {
	app := newAppForSplitAddNew(t, nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleSplitDialogKey(enter)
	updated := model.(*App)

	if updated.createCatDialog == nil || !updated.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should be visible after Enter on AddNew sentinel")
	}
	if updated.createCatSource != createCatSourceSplitDialog {
		t.Errorf("createCatSource = %d, want createCatSourceSplitDialog (%d)",
			updated.createCatSource, createCatSourceSplitDialog)
	}
	if updated.createCatSplitRow != 0 {
		t.Errorf("createCatSplitRow = %d, want 0 (the originating row)", updated.createCatSplitRow)
	}
	if updated.splitDialog == nil {
		t.Fatal("splitDialog should be kept (hidden) so its state survives the divert")
	}
	if updated.splitDialog.IsVisible() {
		t.Error("splitDialog should be hidden while createCatDialog is shown")
	}
	if updated.createCatDialog.Title() != "New Category" {
		t.Errorf("createCatDialog title = %q, want %q",
			updated.createCatDialog.Title(), "New Category")
	}
}

func TestApp_SplitDialog_AddNew_CancelRestoresState(t *testing.T) {
	app := newAppForSplitAddNew(t, nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleSplitDialogKey(enter)
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
	if app.splitDialog == nil || !app.splitDialog.IsVisible() {
		t.Fatal("splitDialog should be restored to visible after cancel")
	}
	if got := app.splitDialog.rows[0].amountField.Value; got != "-100.00" {
		t.Errorf("amount preserved? got %q, want %q", got, "-100.00")
	}
	if got := app.splitDialog.rows[0].memoField.Value; got != "groceries" {
		t.Errorf("memo preserved? got %q, want %q", got, "groceries")
	}
}

func TestApp_SplitDialog_AddNew_AppliesToCurrentRow(t *testing.T) {
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

	app := newAppForSplitAddNew(t, svc, cats)

	// Add a second row whose category we want to see preserved across the
	// rebuild. We pick "Food" if present, otherwise just any non-(None)
	// real category.
	app.splitDialog.addRow()
	preserveIdx := 1
	preserveName := app.splitDialog.categoryOptions[preserveIdx]
	app.splitDialog.rows[1].categoryIndex = preserveIdx
	app.splitDialog.rows[1].amountField.Value = "-50.00"

	// Park rowIndex back on row 0 (where AddNew is parked).
	app.splitDialog.rowIndex = 0

	// Open sub-dialog.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleSplitDialogKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	// Fill: Name=Gym, Parent=(top-level), Type=Expense.
	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "YogaStudio"
	cFields[1].SelectedIndex = 0
	cFields[2].SelectedIndex = 0

	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a tea.Cmd")
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
		if c.Name == "YogaStudio" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("'Gym' should be persisted after submit")
	}

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after submit")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after submit", app.createCatSource)
	}
	if app.splitDialog == nil || !app.splitDialog.IsVisible() {
		t.Fatal("splitDialog should be visible again after submit")
	}

	// Originating row points at the new category.
	sd := app.splitDialog
	origCatIdx := sd.rows[0].categoryIndex
	if sd.rows[0].transferMode {
		t.Errorf("originating row should be in category mode, not transfer mode")
	}
	if sd.isAddNewSentinel(origCatIdx) || sd.isTransferSentinel(origCatIdx) {
		t.Fatalf("originating row should land on a real category, not a sentinel (got idx=%d)", origCatIdx)
	}
	if sd.categoryOptions[origCatIdx] != "YogaStudio" {
		t.Errorf("rows[0] category = %q, want %q", sd.categoryOptions[origCatIdx], "YogaStudio")
	}
	if sd.categoryIDs[origCatIdx] != found.ID {
		t.Errorf("rows[0] categoryID = %s, want %s", sd.categoryIDs[origCatIdx], found.ID)
	}

	// Other row's category was preserved by name (its index may have shifted).
	otherCatIdx := sd.rows[1].categoryIndex
	if sd.isAddNewSentinel(otherCatIdx) || sd.isTransferSentinel(otherCatIdx) {
		t.Fatalf("other row drifted onto a sentinel (got idx=%d)", otherCatIdx)
	}
	if sd.categoryOptions[otherCatIdx] != preserveName {
		t.Errorf("rows[1] category = %q, want %q (preserved across rebuild)",
			sd.categoryOptions[otherCatIdx], preserveName)
	}
}
