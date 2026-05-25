package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Field types: state methods, factory funcs, date masks.

func TestDialog_AddTextField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "John", "Enter name", 20)

	if f == nil {
		t.Fatal("AddTextField returned nil")
	}
	if f.Label != "Name" {
		t.Errorf("Label = %q, want %q", f.Label, "Name")
	}
	if f.Type != FieldText {
		t.Errorf("Type = %d, want FieldText", f.Type)
	}
	if f.Value != "John" {
		t.Errorf("Value = %q, want %q", f.Value, "John")
	}
	if f.Placeholder != "Enter name" {
		t.Errorf("Placeholder = %q, want %q", f.Placeholder, "Enter name")
	}
	if f.Width != 20 {
		t.Errorf("Width = %d, want 20", f.Width)
	}
	if f.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4 (end of 'John')", f.CursorPos())
	}
	if len(d.Fields()) != 1 {
		t.Errorf("Fields() length = %d, want 1", len(d.Fields()))
	}
}

func TestDialog_AddSelectField(t *testing.T) {
	d := NewDialog("Test")
	opts := []string{"Red", "Green", "Blue"}
	f := d.AddSelectField("Color", opts, 1)

	if f.Type != FieldSelect {
		t.Errorf("Type = %d, want FieldSelect", f.Type)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}
	if f.SelectedOption() != "Green" {
		t.Errorf("SelectedOption() = %q, want %q", f.SelectedOption(), "Green")
	}
}

func TestDialog_AddSelectField_ClampIndex(t *testing.T) {
	d := NewDialog("Test")
	opts := []string{"A", "B"}

	f := d.AddSelectField("Test", opts, 10)
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (clamped)", f.SelectedIndex)
	}

	f2 := d.AddSelectField("Test2", opts, -5)
	if f2.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (clamped)", f2.SelectedIndex)
	}
}

func TestDialog_AddRadioField(t *testing.T) {
	d := NewDialog("Test")
	opts := []string{"Yes", "No"}
	f := d.AddRadioField("Confirm", opts, 0)

	if f.Type != FieldRadio {
		t.Errorf("Type = %d, want FieldRadio", f.Type)
	}
	if f.SelectedOption() != "Yes" {
		t.Errorf("SelectedOption() = %q, want %q", f.SelectedOption(), "Yes")
	}
}

func TestDialog_AddRadioField_ClampIndex(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddRadioField("Test", []string{"A"}, 5)
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (clamped)", f.SelectedIndex)
	}
}

func TestDialog_AddCheckboxField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept terms", false)

	if f.Type != FieldCheckbox {
		t.Errorf("Type = %d, want FieldCheckbox", f.Type)
	}
	if f.Checked {
		t.Error("Checked should be false")
	}
	if f.Label != "Accept terms" {
		t.Errorf("Label = %q, want %q", f.Label, "Accept terms")
	}
}

func TestField_InsertChar(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 5}

	f.InsertChar('!')
	if f.Value != "hello!" {
		t.Errorf("Value = %q, want %q", f.Value, "hello!")
	}
	if f.CursorPos() != 6 {
		t.Errorf("CursorPos() = %d, want 6", f.CursorPos())
	}
}

func TestField_InsertChar_AtMiddle(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hllo", cursorPos: 1}

	f.InsertChar('e')
	if f.Value != "hello" {
		t.Errorf("Value = %q, want %q", f.Value, "hello")
	}
	if f.CursorPos() != 2 {
		t.Errorf("CursorPos() = %d, want 2", f.CursorPos())
	}
}

func TestField_InsertChar_AtStart(t *testing.T) {
	f := &Field{Type: FieldText, Value: "ello", cursorPos: 0}

	f.InsertChar('h')
	if f.Value != "hello" {
		t.Errorf("Value = %q, want %q", f.Value, "hello")
	}
	if f.CursorPos() != 1 {
		t.Errorf("CursorPos() = %d, want 1", f.CursorPos())
	}
}

func TestField_InsertChar_NonTextFieldIgnored(t *testing.T) {
	f := &Field{Type: FieldCheckbox, Value: ""}
	f.InsertChar('x')
	if f.Value != "" {
		t.Error("InsertChar should be ignored for non-text fields")
	}
}

func TestField_DeleteBack(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 5}

	f.DeleteBack()
	if f.Value != "hell" {
		t.Errorf("Value = %q, want %q", f.Value, "hell")
	}
	if f.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4", f.CursorPos())
	}
}

func TestField_DeleteBack_AtStart(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 0}

	f.DeleteBack()
	if f.Value != "hello" {
		t.Error("DeleteBack at start should not change value")
	}
}

func TestField_DeleteBack_Middle(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 3}

	f.DeleteBack()
	if f.Value != "helo" {
		t.Errorf("Value = %q, want %q", f.Value, "helo")
	}
	if f.CursorPos() != 2 {
		t.Errorf("CursorPos() = %d, want 2", f.CursorPos())
	}
}

func TestField_DeleteForward(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 0}

	f.DeleteForward()
	if f.Value != "ello" {
		t.Errorf("Value = %q, want %q", f.Value, "ello")
	}
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", f.CursorPos())
	}
}

func TestField_DeleteForward_AtEnd(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 5}

	f.DeleteForward()
	if f.Value != "hello" {
		t.Error("DeleteForward at end should not change value")
	}
}

func TestField_MoveCursor(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 2}

	f.MoveCursorLeft()
	if f.CursorPos() != 1 {
		t.Errorf("after MoveCursorLeft: CursorPos() = %d, want 1", f.CursorPos())
	}

	f.MoveCursorRight()
	f.MoveCursorRight()
	if f.CursorPos() != 3 {
		t.Errorf("after 2x MoveCursorRight: CursorPos() = %d, want 3", f.CursorPos())
	}

	f.MoveCursorHome()
	if f.CursorPos() != 0 {
		t.Errorf("after MoveCursorHome: CursorPos() = %d, want 0", f.CursorPos())
	}

	f.MoveCursorEnd()
	if f.CursorPos() != 5 {
		t.Errorf("after MoveCursorEnd: CursorPos() = %d, want 5", f.CursorPos())
	}
}

func TestField_MoveCursorLeft_AtStart(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 0}
	f.MoveCursorLeft()
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", f.CursorPos())
	}
}

func TestField_MoveCursorRight_AtEnd(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello", cursorPos: 5}
	f.MoveCursorRight()
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5", f.CursorPos())
	}
}

func TestField_MoveCursor_NonTextIgnored(t *testing.T) {
	f := &Field{Type: FieldSelect}
	f.MoveCursorLeft()
	f.MoveCursorRight()
	f.MoveCursorHome()
	f.MoveCursorEnd()
	// Should not panic
}

func TestField_SelectNext(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: []string{"A", "B", "C"}, SelectedIndex: 0}

	f.SelectNext()
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}

	f.SelectNext()
	if f.SelectedIndex != 2 {
		t.Errorf("SelectedIndex = %d, want 2", f.SelectedIndex)
	}

	f.SelectNext() // should not go past end
	if f.SelectedIndex != 2 {
		t.Errorf("SelectedIndex = %d, want 2 (clamped)", f.SelectedIndex)
	}
}

func TestField_SelectPrev(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: []string{"A", "B", "C"}, SelectedIndex: 2}

	f.SelectPrev()
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}

	f.SelectPrev()
	f.SelectPrev() // should not go below 0
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (clamped)", f.SelectedIndex)
	}
}

func TestField_SelectNext_RadioField(t *testing.T) {
	f := &Field{Type: FieldRadio, Options: []string{"X", "Y"}, SelectedIndex: 0}
	f.SelectNext()
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}
}

func TestField_SelectNext_EmptyOptions(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: nil}
	f.SelectNext()
	f.SelectPrev()
	// Should not panic
}

func TestField_SelectNext_WrongType(t *testing.T) {
	f := &Field{Type: FieldText, Options: []string{"A"}}
	f.SelectNext()
	// Should not change anything
}

func TestField_Toggle(t *testing.T) {
	f := &Field{Type: FieldCheckbox, Checked: false}

	f.Toggle()
	if !f.Checked {
		t.Error("Checked should be true after Toggle")
	}

	f.Toggle()
	if f.Checked {
		t.Error("Checked should be false after second Toggle")
	}
}

func TestField_Toggle_NonCheckboxIgnored(t *testing.T) {
	f := &Field{Type: FieldText, Checked: false}
	f.Toggle()
	if f.Checked {
		t.Error("Toggle should be ignored for non-checkbox")
	}
}

func TestField_SelectedOption(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: []string{"A", "B"}, SelectedIndex: 1}
	if f.SelectedOption() != "B" {
		t.Errorf("SelectedOption() = %q, want %q", f.SelectedOption(), "B")
	}
}

func TestField_SelectedOption_Empty(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: nil}
	if f.SelectedOption() != "" {
		t.Errorf("SelectedOption() = %q, want empty", f.SelectedOption())
	}
}

func TestField_SelectedOption_OutOfRange(t *testing.T) {
	f := &Field{Type: FieldSelect, Options: []string{"A"}, SelectedIndex: 5}
	if f.SelectedOption() != "" {
		t.Errorf("SelectedOption() = %q, want empty (out of range)", f.SelectedOption())
	}
}

func TestField_SelectedOption_WrongType(t *testing.T) {
	f := &Field{Type: FieldText, Options: []string{"A"}}
	if f.SelectedOption() != "" {
		t.Error("SelectedOption() should be empty for text fields")
	}
}

// HandleKey tests

func TestField_InsertChar_Unicode(t *testing.T) {
	f := &Field{Type: FieldText, Value: "cafe", cursorPos: 4}

	f.InsertChar('\u0301') // combining acute accent
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d after unicode insert", f.CursorPos())
	}
}

func TestField_DeleteBack_Unicode(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hello\u00e9", cursorPos: 6} // hello + e-acute

	f.DeleteBack()
	if f.Value != "hello" {
		t.Errorf("Value = %q, want %q after deleting unicode char", f.Value, "hello")
	}
}

// Edge case tests

func TestField_InsertChar_CursorPastEnd(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hi", cursorPos: 100}

	f.InsertChar('!')
	// Should clamp cursor and append
	if f.Value != "hi!" {
		t.Errorf("Value = %q, want %q", f.Value, "hi!")
	}
}

func TestField_DeleteBack_CursorPastEnd(t *testing.T) {
	f := &Field{Type: FieldText, Value: "hi", cursorPos: 100}

	f.DeleteBack()
	if f.Value != "h" {
		t.Errorf("Value = %q, want %q", f.Value, "h")
	}
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestDialog_AddListField(t *testing.T) {
	d := NewDialog("Test")
	items := []string{"file1.tdb", "file2.tdb", "file3.tdb"}
	f := d.AddListField("File", items, 0, 10)

	if f.Type != FieldList {
		t.Errorf("Type = %v, want FieldList", f.Type)
	}
	if len(f.Options) != 3 {
		t.Errorf("Options length = %d, want 3", len(f.Options))
	}
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0", f.SelectedIndex)
	}
	if f.VisibleCount != 10 {
		t.Errorf("VisibleCount = %d, want 10", f.VisibleCount)
	}
}

func TestFieldList_SelectNavigation(t *testing.T) {
	f := &Field{
		Type:          FieldList,
		Options:       []string{"a", "b", "c"},
		SelectedIndex: 0,
	}

	f.SelectNext()
	if f.SelectedIndex != 1 {
		t.Errorf("after SelectNext: SelectedIndex = %d, want 1", f.SelectedIndex)
	}

	f.SelectNext()
	if f.SelectedIndex != 2 {
		t.Errorf("after SelectNext: SelectedIndex = %d, want 2", f.SelectedIndex)
	}

	// Should not go past end
	f.SelectNext()
	if f.SelectedIndex != 2 {
		t.Errorf("past end: SelectedIndex = %d, want 2", f.SelectedIndex)
	}

	f.SelectPrev()
	if f.SelectedIndex != 1 {
		t.Errorf("after SelectPrev: SelectedIndex = %d, want 1", f.SelectedIndex)
	}
}

func TestDialog_AddDateField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")

	if f == nil {
		t.Fatal("AddDateField returned nil")
	}
	if f.Type != FieldDate {
		t.Errorf("Type = %d, want FieldDate", f.Type)
	}
	if f.Value != "01/15/2024" {
		t.Errorf("Value = %q, want %q", f.Value, "01/15/2024")
	}
	if f.Width != 10 {
		t.Errorf("Width = %d, want 10", f.Width)
	}
	if f.CursorPos() != 0 {
		t.Errorf("initial CursorPos() = %d, want 0 (first digit)", f.CursorPos())
	}
}

func TestDialog_AddDateField_EmptyDefaultsToToday(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "")

	if len(f.Value) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical mask shape)", len(f.Value))
	}
	// Slashes always at positions 2 and 5
	if f.Value[2] != '/' || f.Value[5] != '/' {
		t.Errorf("Value = %q, slashes not at positions 2 and 5", f.Value)
	}
}

func TestFieldDate_TypingOverwritesAndAdvances(t *testing.T) {
	d := NewDialog("Test")
	d.AddDateField("Date", "01/15/2024")

	// Type "0" then "2" — overwrites "01" with "02", cursor advances skipping the slash
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})

	if d.Fields()[0].Value != "02/15/2024" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "02/15/2024")
	}
	// After typing two digits, cursor sits on first digit of DD (string index 3)
	if d.Fields()[0].CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3 (first digit of DD)", d.Fields()[0].CursorPos())
	}
}

func TestFieldDate_CursorRightSkipsSlashes(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	// Position cursor at last digit of MM (index 1)
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 1 {
		t.Fatalf("setup: CursorPos() = %d, want 1", f.CursorPos())
	}

	// Right from index 1 should skip slash at index 2 → land on index 3
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3 (skipped slash at index 2)", f.CursorPos())
	}

	// Move to index 4, then right should skip slash at index 5 → land on index 6
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 4 {
		t.Fatalf("CursorPos() = %d, want 4", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 6 {
		t.Errorf("CursorPos() = %d, want 6 (skipped slash at index 5)", f.CursorPos())
	}
}

func TestFieldDate_CursorLeftSkipsSlashes(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 9 {
		t.Fatalf("End: CursorPos() = %d, want 9", f.CursorPos())
	}

	// Stepping left across index 6 → 5 should skip slash, landing at 4.
	for range 3 {
		d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	if f.CursorPos() != 6 {
		t.Fatalf("after 3 lefts: CursorPos() = %d, want 6", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4 (skipped slash at index 5)", f.CursorPos())
	}

	// And from index 3, left skips slash at index 2 → land at 1
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 3 {
		t.Fatalf("CursorPos() = %d, want 3", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 1 {
		t.Errorf("CursorPos() = %d, want 1 (skipped slash at index 2)", f.CursorPos())
	}
}

func TestFieldDate_BackspaceReplacesWithZeroAndStepsBack(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	// Move cursor to first digit of DD (string index 3)
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 3 {
		t.Fatalf("setup: CursorPos() = %d, want 3", f.CursorPos())
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})

	if f.Value != "01/05/2024" {
		t.Errorf("Value = %q, want %q (digit at index 3 replaced with '0')", f.Value, "01/05/2024")
	}
	// Cursor moves back, skipping slash at index 2 → land at 1
	if f.CursorPos() != 1 {
		t.Errorf("CursorPos() = %d, want 1 (backspaced past slash)", f.CursorPos())
	}
}

func TestFieldDate_HomeAndEnd(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 9 {
		t.Errorf("End: CursorPos() = %d, want 9 (last digit)", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyHome})
	if f.CursorPos() != 0 {
		t.Errorf("Home: CursorPos() = %d, want 0 (first digit)", f.CursorPos())
	}
}

func TestFieldDate_NonDigitInputIgnored(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	d.HandleKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	d.HandleKey(tea.KeyPressMsg{Code: '-', Text: "-"})

	if f.Value != "01/15/2024" {
		t.Errorf("Value = %q, want %q (non-digits ignored)", f.Value, "01/15/2024")
	}
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0 (cursor unchanged on ignored input)", f.CursorPos())
	}
}

func TestFieldDate_ValueAlwaysTenChars(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")

	// Type a digit at every position
	for range 8 {
		d.HandleKey(tea.KeyPressMsg{Code: '9', Text: "9"})
	}

	if len(f.Value) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical mask shape preserved)", len(f.Value))
	}
	if f.Value[2] != '/' || f.Value[5] != '/' {
		t.Errorf("Value = %q, slashes not at positions 2 and 5", f.Value)
	}
}

func TestFieldDate_TypingPastEndStops(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 9 {
		t.Fatalf("End: CursorPos() = %d, want 9", f.CursorPos())
	}

	// Typing at the last digit overwrites it; cursor stays at 9 (no further advance)
	d.HandleKey(tea.KeyPressMsg{Code: '7', Text: "7"})
	if f.Value != "01/15/2027" {
		t.Errorf("Value = %q, want %q", f.Value, "01/15/2027")
	}
	if f.CursorPos() != 9 {
		t.Errorf("CursorPos() = %d, want 9 (stays at last digit)", f.CursorPos())
	}
}

func TestFieldDate_BackspaceAtFirstDigitStops(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	// cursor is at 0 from AddDateField

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "01/15/2024" {
		t.Errorf("Value = %q, want %q (no change at first digit)", f.Value, "01/15/2024")
	}
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", f.CursorPos())
	}
}

func TestFieldDate_RenderContainsValue(t *testing.T) {
	d := NewDialog("Test")
	d.AddDateField("Date", "01/15/2024")
	// Move focus off the date field so the cursor highlight doesn't split the
	// rendered date with ANSI escapes.
	d.SetFocusIndex(1)
	styles := NewStyles()

	out := d.Render(styles)
	if !strings.Contains(out, "01/15/2024") {
		t.Errorf("rendered output should contain date value; got:\n%s", out)
	}
}

// FieldDate optional-blank (TD-011) tests

func TestDialog_AddOptionalDateField_EmptyDefaultsToCanonicalBlank(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "")

	if f == nil {
		t.Fatal("AddOptionalDateField returned nil")
	}
	if f.Type != FieldDate {
		t.Errorf("Type = %d, want FieldDate", f.Type)
	}
	if !f.OptionalBlank {
		t.Error("OptionalBlank should be true on a field built by AddOptionalDateField")
	}
	if f.Value != "  /  /    " {
		t.Errorf("Value = %q, want canonical blank %q", f.Value, "  /  /    ")
	}
	if f.Width != 10 {
		t.Errorf("Width = %d, want 10", f.Width)
	}
	if f.CursorPos() != 0 {
		t.Errorf("initial CursorPos() = %d, want 0", f.CursorPos())
	}
}

func TestDialog_AddOptionalDateField_PreservesNonEmptyInitialValue(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "12/31/2024")

	if !f.OptionalBlank {
		t.Error("OptionalBlank should be true even when seeded with a value")
	}
	if f.Value != "12/31/2024" {
		t.Errorf("Value = %q, want %q", f.Value, "12/31/2024")
	}
}

func TestDialog_AddDateField_NotOptionalBlank(t *testing.T) {
	// Regression guard: strict AddDateField fields default to OptionalBlank=false.
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	if f.OptionalBlank {
		t.Error("strict AddDateField must not set OptionalBlank")
	}
}

func TestFieldDate_OptionalBlank_BackspaceClearsTypedDigit(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "")
	if f.Value != "  /  /    " {
		t.Fatalf("setup: Value = %q, want canonical blank", f.Value)
	}

	d.HandleKey(tea.KeyPressMsg{Code: '1', Text: "1"})
	if f.Value != "1 /  /    " {
		t.Fatalf("after type '1': Value = %q, want %q", f.Value, "1 /  /    ")
	}
	if f.CursorPos() != 1 {
		t.Fatalf("after type '1': CursorPos = %d, want 1", f.CursorPos())
	}

	// Conventional editor backspace: deletes the digit before the cursor and
	// steps the cursor back, returning the field to canonical blank.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "  /  /    " {
		t.Errorf("after backspace: Value = %q, want canonical blank", f.Value)
	}
	if f.CursorPos() != 0 {
		t.Errorf("after backspace: CursorPos = %d, want 0", f.CursorPos())
	}
}

func TestFieldDate_OptionalBlank_BackspaceSkipsSlash(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "12/31/2024")
	// Move cursor to first digit of DD (string index 3) — two right-presses
	// from the initial cursor at 0 (skipping the slash at index 2).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 3 {
		t.Fatalf("setup: CursorPos = %d, want 3", f.CursorPos())
	}

	// Backspace at cursor 3: skip slash at 2, delete digit at index 1 with ' '.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "1 /31/2024" {
		t.Errorf("Value = %q, want %q", f.Value, "1 /31/2024")
	}
	if f.CursorPos() != 1 {
		t.Errorf("CursorPos = %d, want 1", f.CursorPos())
	}
}

func TestFieldDate_OptionalBlank_BackspaceAtFirstDigitNoOps(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "")
	// Cursor starts at 0.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "  /  /    " {
		t.Errorf("Value changed by backspace at cursor 0: %q", f.Value)
	}
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos = %d, want 0", f.CursorPos())
	}
}

func TestFieldDate_NonOptional_BackspaceStillUsesZero(t *testing.T) {
	// Regression guard: TD-002 backspace semantic for strict AddDateField is
	// "overwrite digit at cursor with '0', step back". TD-011 must not change
	// this for non-optional fields.
	d := NewDialog("Test")
	f := d.AddDateField("Date", "01/15/2024")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 3 {
		t.Fatalf("setup: CursorPos = %d, want 3", f.CursorPos())
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "01/05/2024" {
		t.Errorf("Value = %q, want %q (digit at cursor replaced with '0')", f.Value, "01/05/2024")
	}
	if f.CursorPos() != 1 {
		t.Errorf("CursorPos = %d, want 1", f.CursorPos())
	}
}

func TestIsBlankDateInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty string", "", true},
		{"canonical blank", "  /  /    ", true},
		{"valid date", "12/31/2024", false},
		{"partial state", "1 /  /    ", false},
		{"wrong length", "  /  /   ", false},
		{"non-space digit position", "00/00/0000", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBlankDateInput(tc.in); got != tc.want {
				t.Errorf("isBlankDateInput(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFieldDate_OptionalBlank_TypingFillsCanonicalBlank(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateField("End Date", "")
	// Type "12312024".
	for _, r := range "12312024" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if f.Value != "12/31/2024" {
		t.Errorf("Value = %q, want %q", f.Value, "12/31/2024")
	}
}

// === FieldCombo (typeahead + filtered list) ===

func TestDialog_AddDateFieldISO(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")

	if f == nil {
		t.Fatal("AddDateFieldISO returned nil")
	}
	if f.Type != FieldDate {
		t.Errorf("Type = %d, want FieldDate", f.Type)
	}
	if f.Value != "2024-01-15" {
		t.Errorf("Value = %q, want %q", f.Value, "2024-01-15")
	}
	if f.Width != 10 {
		t.Errorf("Width = %d, want 10", f.Width)
	}
	if f.CursorPos() != 0 {
		t.Errorf("initial CursorPos() = %d, want 0", f.CursorPos())
	}
}

func TestDialog_AddDateFieldISO_EmptyDefaultsToToday(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "")

	if len(f.Value) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical mask shape)", len(f.Value))
	}
	// Dashes always at positions 4 and 7 in YYYY-MM-DD
	if f.Value[4] != '-' || f.Value[7] != '-' {
		t.Errorf("Value = %q, dashes not at positions 4 and 7", f.Value)
	}
}

func TestFieldDateISO_TypingOverwritesAndAdvances(t *testing.T) {
	d := NewDialog("Test")
	d.AddDateFieldISO("Date", "2024-01-15")

	// Type "2025" — overwrites "2024", cursor advances over the dash to index 5
	d.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	d.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})

	if d.Fields()[0].Value != "2025-01-15" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "2025-01-15")
	}
	// After typing four digits, cursor sits on first digit of MM (string index 5)
	if d.Fields()[0].CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5 (first digit of MM)", d.Fields()[0].CursorPos())
	}
}

func TestFieldDateISO_CursorRightSkipsDashes(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")

	// Move cursor to last digit of YYYY (index 3)
	for range 3 {
		d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if f.CursorPos() != 3 {
		t.Fatalf("setup: CursorPos() = %d, want 3", f.CursorPos())
	}

	// Right from index 3 should skip dash at index 4 → land on index 5
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5 (skipped dash at index 4)", f.CursorPos())
	}

	// Move to index 6, then right should skip dash at index 7 → land on index 8
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 6 {
		t.Fatalf("CursorPos() = %d, want 6", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.CursorPos() != 8 {
		t.Errorf("CursorPos() = %d, want 8 (skipped dash at index 7)", f.CursorPos())
	}
}

func TestFieldDateISO_CursorLeftSkipsDashes(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 9 {
		t.Fatalf("End: CursorPos() = %d, want 9", f.CursorPos())
	}

	// From index 9, left → 8. left → skip dash at 7 → 6. left → 5. left → skip dash at 4 → 3.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 8 {
		t.Fatalf("CursorPos() = %d, want 8", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 6 {
		t.Errorf("CursorPos() = %d, want 6 (skipped dash at index 7)", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 5 {
		t.Fatalf("CursorPos() = %d, want 5", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3 (skipped dash at index 4)", f.CursorPos())
	}
}

func TestFieldDateISO_BackspaceReplacesWithZeroAndStepsBack(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-11-15")
	// Move cursor to second digit of MM (string index 6) — value[6] is '1',
	// so the backspace overwrite is visible (becomes '0').
	for range 5 {
		d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if f.CursorPos() != 6 {
		t.Fatalf("setup: CursorPos() = %d, want 6", f.CursorPos())
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})

	if f.Value != "2024-10-15" {
		t.Errorf("Value = %q, want %q (digit at index 6 replaced with '0')", f.Value, "2024-10-15")
	}
	// Cursor moves back: 6 → 5 (not a separator)
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5", f.CursorPos())
	}

	// Backspace again: cursor=5 → digit replaced with '0' → "2024-00-15";
	// cursor steps left, skipping the dash at index 4 → lands at 3.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Value != "2024-00-15" {
		t.Errorf("Value = %q, want %q (digit at index 5 replaced with '0')", f.Value, "2024-00-15")
	}
	if f.CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3 (backspaced past dash)", f.CursorPos())
	}
}

func TestFieldDateISO_HomeAndEnd(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 9 {
		t.Errorf("End: CursorPos() = %d, want 9 (last digit)", f.CursorPos())
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyHome})
	if f.CursorPos() != 0 {
		t.Errorf("Home: CursorPos() = %d, want 0 (first digit)", f.CursorPos())
	}
}

func TestFieldDateISO_ValueAlwaysTenChars(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")

	// Type a digit at every position
	for range 8 {
		d.HandleKey(tea.KeyPressMsg{Code: '9', Text: "9"})
	}

	if len(f.Value) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical mask shape preserved)", len(f.Value))
	}
	if f.Value[4] != '-' || f.Value[7] != '-' {
		t.Errorf("Value = %q, dashes not at positions 4 and 7", f.Value)
	}
}

func TestFieldDateISO_RenderContainsValue(t *testing.T) {
	d := NewDialog("Test")
	d.AddDateFieldISO("Date", "2024-01-15")
	// Move focus off the date field so the cursor highlight doesn't split the
	// rendered date with ANSI escapes.
	d.SetFocusIndex(1)
	styles := NewStyles()

	out := d.Render(styles)
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("rendered output should contain date value; got:\n%s", out)
	}
}

func TestFieldDateISO_NonDigitInputIgnored(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddDateFieldISO("Date", "2024-01-15")

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	d.HandleKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	d.HandleKey(tea.KeyPressMsg{Code: '-', Text: "-"})

	if f.Value != "2024-01-15" {
		t.Errorf("Value = %q, want %q (non-digits ignored)", f.Value, "2024-01-15")
	}
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0 (cursor unchanged on ignored input)", f.CursorPos())
	}
}

func TestDialog_AddOptionalDateFieldISO_EmptyDefaultsToCanonicalBlank(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddOptionalDateFieldISO("End Date", "")

	if f == nil {
		t.Fatal("AddOptionalDateFieldISO returned nil")
	}
	if f.Type != FieldDate {
		t.Errorf("Type = %d, want FieldDate", f.Type)
	}
	if !f.OptionalBlank {
		t.Error("OptionalBlank should be true on a field built by AddOptionalDateFieldISO")
	}
	if f.Value != "    -  -  " {
		t.Errorf("Value = %q, want canonical ISO blank %q", f.Value, "    -  -  ")
	}
	if f.Width != 10 {
		t.Errorf("Width = %d, want 10", f.Width)
	}
}

func TestIsBlankDateInput_ISO(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"ISO canonical blank", "    -  -  ", true},
		{"ISO valid date", "2024-12-31", false},
		{"ISO partial state", "2   -  -  ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBlankDateInput(tc.in); got != tc.want {
				t.Errorf("isBlankDateInput(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
