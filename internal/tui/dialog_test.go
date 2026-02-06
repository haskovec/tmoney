package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewDialog(t *testing.T) {
	d := NewDialog("Test Dialog")
	if d == nil {
		t.Fatal("NewDialog() returned nil")
	}
	if d.Title() != "Test Dialog" {
		t.Errorf("Title() = %q, want %q", d.Title(), "Test Dialog")
	}
	if d.IsVisible() {
		t.Error("dialog should not be visible by default")
	}
	if d.Width() != 56 {
		t.Errorf("Width() = %d, want 56", d.Width())
	}
	if len(d.Fields()) != 0 {
		t.Errorf("Fields() should be empty, got %d", len(d.Fields()))
	}
	if len(d.Buttons()) != 2 {
		t.Errorf("Buttons() should have 2 defaults, got %d", len(d.Buttons()))
	}
	if d.Buttons()[0].Label != "Cancel" {
		t.Errorf("first button = %q, want %q", d.Buttons()[0].Label, "Cancel")
	}
	if d.Buttons()[1].Label != "Save" || !d.Buttons()[1].Primary {
		t.Error("second button should be primary Save")
	}
}

func TestDialog_SetTitle(t *testing.T) {
	d := NewDialog("Old")
	d.SetTitle("New")
	if d.Title() != "New" {
		t.Errorf("Title() = %q, want %q", d.Title(), "New")
	}
}

func TestDialog_Visibility(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	if !d.IsVisible() {
		t.Error("should be visible after SetVisible(true)")
	}
	d.SetVisible(false)
	if d.IsVisible() {
		t.Error("should not be visible after SetVisible(false)")
	}
}

func TestDialog_SetWidth(t *testing.T) {
	d := NewDialog("Test")
	d.SetWidth(80)
	if d.Width() != 80 {
		t.Errorf("Width() = %d, want 80", d.Width())
	}
}

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

func TestDialog_SetButtons(t *testing.T) {
	d := NewDialog("Test")
	d.SetButtons([]DialogButton{
		{Label: "OK", Primary: true},
	})
	if len(d.Buttons()) != 1 {
		t.Errorf("Buttons() length = %d, want 1", len(d.Buttons()))
	}
	if d.Buttons()[0].Label != "OK" {
		t.Errorf("button label = %q, want %q", d.Buttons()[0].Label, "OK")
	}
}

func TestDialog_FocusNext_WrapsAround(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)
	// 2 fields + 2 buttons = 4 focusable elements

	if d.FocusIndex() != 0 {
		t.Errorf("initial FocusIndex() = %d, want 0", d.FocusIndex())
	}

	d.FocusNext() // -> 1
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex() = %d, want 1", d.FocusIndex())
	}

	d.FocusNext() // -> 2 (Cancel button)
	d.FocusNext() // -> 3 (Save button)
	d.FocusNext() // -> 0 (wrap)
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex() after wrap = %d, want 0", d.FocusIndex())
	}
}

func TestDialog_FocusPrev_WrapsAround(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	// 1 field + 2 buttons = 3 focusable

	d.FocusPrev() // 0 -> 2 (wrap to last)
	if d.FocusIndex() != 2 {
		t.Errorf("FocusIndex() = %d, want 2", d.FocusIndex())
	}

	d.FocusPrev() // 2 -> 1
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex() = %d, want 1", d.FocusIndex())
	}
}

func TestDialog_FocusNext_NoElements(t *testing.T) {
	d := NewDialog("Test")
	d.SetButtons(nil)
	d.FocusNext()
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex() = %d, want 0", d.FocusIndex())
	}
}

func TestDialog_SetFocusIndex_Clamp(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	// 1 field + 2 buttons = 3

	d.SetFocusIndex(10)
	if d.FocusIndex() != 2 {
		t.Errorf("FocusIndex() = %d, want 2 (clamped)", d.FocusIndex())
	}

	d.SetFocusIndex(-1)
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex() = %d, want 0 (clamped)", d.FocusIndex())
	}
}

func TestDialog_FocusedField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "", 0)

	focused := d.FocusedField()
	if focused != f {
		t.Error("FocusedField() should return the first field")
	}

	d.SetFocusIndex(1) // Cancel button
	if d.FocusedField() != nil {
		t.Error("FocusedField() should be nil when focus is on button")
	}
}

func TestDialog_IsFocusOnButton(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)

	if d.IsFocusOnButton() {
		t.Error("should not be on button initially")
	}

	d.SetFocusIndex(1) // Cancel button
	if !d.IsFocusOnButton() {
		t.Error("should be on button at index 1")
	}
}

func TestDialog_FocusedButtonIndex(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	// index 0 = field, 1 = Cancel, 2 = Save

	if d.FocusedButtonIndex() != -1 {
		t.Errorf("FocusedButtonIndex() = %d, want -1 (on field)", d.FocusedButtonIndex())
	}

	d.SetFocusIndex(1)
	if d.FocusedButtonIndex() != 0 {
		t.Errorf("FocusedButtonIndex() = %d, want 0", d.FocusedButtonIndex())
	}

	d.SetFocusIndex(2)
	if d.FocusedButtonIndex() != 1 {
		t.Errorf("FocusedButtonIndex() = %d, want 1", d.FocusedButtonIndex())
	}
}

// Field text editing tests

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

func TestDialog_HandleKey_Esc(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("Esc action = %d, want DialogActionCancel", action)
	}
}

func TestDialog_HandleKey_Tab(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if action != DialogActionNone {
		t.Errorf("Tab action = %d, want DialogActionNone", action)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex() = %d, want 1 after Tab", d.FocusIndex())
	}
}

func TestDialog_HandleKey_ShiftTab(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)
	d.SetFocusIndex(1)

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if action != DialogActionNone {
		t.Errorf("ShiftTab action = %d, want DialogActionNone", action)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex() = %d, want 0 after ShiftTab", d.FocusIndex())
	}
}

func TestDialog_HandleKey_EnterOnPrimaryButton(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	// buttons: Cancel (idx 1), Save/Primary (idx 2)
	d.SetFocusIndex(2) // Save button

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action != DialogActionSubmit {
		t.Errorf("Enter on primary action = %d, want DialogActionSubmit", action)
	}
}

func TestDialog_HandleKey_EnterOnCancelButton(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.SetFocusIndex(1) // Cancel button

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action != DialogActionCancel {
		t.Errorf("Enter on cancel action = %d, want DialogActionCancel", action)
	}
}

func TestDialog_HandleKey_EnterOnField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)

	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if action != DialogActionNone {
		t.Errorf("Enter on field action = %d, want DialogActionNone", action)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex() = %d, want 1 (should advance)", d.FocusIndex())
	}
}

func TestDialog_HandleKey_TypingInTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	d.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if d.Fields()[0].Value != "Hi" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "Hi")
	}
}

func TestDialog_HandleKey_BackspaceInTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "Hello", "", 0)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})

	if d.Fields()[0].Value != "Hell" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "Hell")
	}
}

func TestDialog_HandleKey_ArrowsInTextField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "Hello", "", 0)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if f.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4", f.CursorPos())
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0 after Home", f.CursorPos())
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyEnd})
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5 after End", f.CursorPos())
	}
}

func TestDialog_HandleKey_UpDownInSelectField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddSelectField("Color", []string{"Red", "Green", "Blue"}, 0)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0", f.SelectedIndex)
	}
}

func TestDialog_HandleKey_RadioNavigation(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 after Right", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 after Left", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 after Down", f.SelectedIndex)
	}
}

func TestDialog_HandleKey_SpaceTogglesCheckbox(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)

	d.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !f.Checked {
		t.Error("Checked should be true after space")
	}

	d.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if f.Checked {
		t.Error("Checked should be false after second space")
	}
}

func TestDialog_HandleKey_FocusOnButtonIgnoresFieldKeys(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "test", "", 0)
	d.SetFocusIndex(1) // Cancel button

	// Typing should do nothing when focus is on a button
	d.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if d.Fields()[0].Value != "test" {
		t.Errorf("Value = %q, should not change when focus is on button", d.Fields()[0].Value)
	}
}

func TestDialog_HandleKey_DeleteInTextField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "Hello", "", 0)
	f.cursorPos = 0

	d.HandleKey(tea.KeyMsg{Type: tea.KeyDelete})
	if f.Value != "ello" {
		t.Errorf("Value = %q, want %q", f.Value, "ello")
	}
}

// Render tests

func TestDialog_Render_NonEmpty(t *testing.T) {
	d := NewDialog("Test Dialog")
	d.AddTextField("Name", "John", "", 0)
	d.AddCheckboxField("Active", true)
	styles := NewStyles()

	result := d.Render(styles)
	if result == "" {
		t.Fatal("Render() returned empty string")
	}
}

func TestDialog_Render_ContainsTitle(t *testing.T) {
	d := NewDialog("My Title")
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "My Title") {
		t.Error("Render() should contain the title")
	}
}

func TestDialog_Render_ContainsFieldLabels(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Username", "", "", 0)
	d.AddSelectField("Role", []string{"Admin", "User"}, 0)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Username") {
		t.Error("Render() should contain field label 'Username'")
	}
	if !strings.Contains(result, "Role") {
		t.Error("Render() should contain field label 'Role'")
	}
}

func TestDialog_Render_ContainsButtonLabels(t *testing.T) {
	d := NewDialog("Test")
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Cancel") {
		t.Error("Render() should contain 'Cancel' button")
	}
	if !strings.Contains(result, "Save") {
		t.Error("Render() should contain 'Save' button")
	}
}

func TestDialog_Render_ContainsCloseButton(t *testing.T) {
	d := NewDialog("Test")
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "[x]") {
		t.Error("Render() should contain close button [x]")
	}
}

func TestDialog_Render_ContainsCheckboxState(t *testing.T) {
	d := NewDialog("Test")
	d.AddCheckboxField("Enabled", true)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "[x]") {
		t.Error("Render() should contain checked checkbox [x]")
	}
}

func TestDialog_Render_ContainsRadioOptions(t *testing.T) {
	d := NewDialog("Test")
	d.AddRadioField("Status", []string{"Pending", "Done"}, 1)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Pending") {
		t.Error("Render() should contain radio option 'Pending'")
	}
	if !strings.Contains(result, "Done") {
		t.Error("Render() should contain radio option 'Done'")
	}
	if !strings.Contains(result, "(*)") {
		t.Error("Render() should contain selected radio bullet (*)")
	}
}

func TestDialog_Render_ContainsSelectOption(t *testing.T) {
	d := NewDialog("Test")
	d.AddSelectField("Type", []string{"Checking", "Savings"}, 0)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Checking") {
		t.Error("Render() should contain selected option 'Checking'")
	}
}

func TestDialog_Render_EmptyDialog(t *testing.T) {
	d := NewDialog("Empty")
	d.SetButtons(nil)
	styles := NewStyles()

	result := d.Render(styles)
	if result == "" {
		t.Error("Render() should produce output even for empty dialog")
	}
	if !strings.Contains(result, "Empty") {
		t.Error("Render() should contain the title")
	}
}

func TestDialog_Render_TextFieldWithPlaceholder(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "Enter name", 0)
	// Move focus to a button so the text field is unfocused (placeholder visible)
	d.SetFocusIndex(1)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Enter name") {
		t.Error("Render() should show placeholder when value is empty and field is unfocused")
	}
}

func TestDialog_Render_TextFieldWithValue(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "Alice", "", 0)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Alice") {
		t.Error("Render() should show the field value")
	}
}

func TestDialog_Render_ContainsSeparators(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "─") {
		t.Error("Render() should contain separator lines")
	}
}

func TestDialog_MaxLabelWidth(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("Long Label", "", "", 0)
	d.AddCheckboxField("Checkbox", false) // checkboxes excluded

	maxW := d.maxLabelWidth()
	if maxW != 10 {
		t.Errorf("maxLabelWidth() = %d, want 10", maxW)
	}
}

func TestDialog_MaxLabelWidth_NoFields(t *testing.T) {
	d := NewDialog("Test")
	if d.maxLabelWidth() != 0 {
		t.Errorf("maxLabelWidth() = %d, want 0", d.maxLabelWidth())
	}
}

func TestDialog_MaxLabelWidth_OnlyCheckboxes(t *testing.T) {
	d := NewDialog("Test")
	d.AddCheckboxField("Check", false)
	if d.maxLabelWidth() != 0 {
		t.Errorf("maxLabelWidth() = %d, want 0", d.maxLabelWidth())
	}
}

// OverlayCenter tests

func TestOverlayCenter_CentersOverlay(t *testing.T) {
	// Create a 10x5 background of dots
	bgLines := make([]string, 5)
	for i := range bgLines {
		bgLines[i] = strings.Repeat(".", 10)
	}
	background := strings.Join(bgLines, "\n")

	overlay := "XX\nXX"

	result := OverlayCenter(background, overlay, 10, 5)
	lines := strings.Split(result, "\n")

	// Overlay is 2x2, screen is 10x5
	// startCol = (10-2)/2 = 4
	// startRow = (5-2)/2 = 1
	if len(lines) < 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Row 1 should have XX at position 4-5
	if !strings.Contains(lines[1], "XX") {
		t.Errorf("row 1 should contain 'XX', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "XX") {
		t.Errorf("row 2 should contain 'XX', got %q", lines[2])
	}

	// Row 0 should be unchanged
	if lines[0] != ".........." {
		t.Errorf("row 0 should be unchanged, got %q", lines[0])
	}
}

func TestOverlayCenter_SmallBackground(t *testing.T) {
	background := ".\n."
	overlay := "XXXXX\nXXXXX\nXXXXX"

	// Should not panic even if overlay is larger
	result := OverlayCenter(background, overlay, 2, 2)
	if result == "" {
		t.Error("should produce non-empty output")
	}
}

func TestOverlayCenter_EmptyOverlay(t *testing.T) {
	background := "hello\nworld"
	result := OverlayCenter(background, "", 5, 2)
	if result != background {
		t.Error("empty overlay should not change background")
	}
}

// Unicode text editing tests

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

func TestDialog_HandleKey_NoFieldsNoButtons(t *testing.T) {
	d := NewDialog("Empty")
	d.SetButtons(nil)

	// Should not panic
	action := d.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if action != DialogActionNone {
		t.Errorf("action = %d, want DialogActionNone", action)
	}
}

func TestDialog_SetButtons_ClampsFocus(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.SetFocusIndex(2) // Save button

	d.SetButtons(nil) // Now only 1 focusable element (field)
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex() = %d, want 0 (clamped after removing buttons)", d.FocusIndex())
	}
}

func TestDialog_FocusableCount(t *testing.T) {
	d := NewDialog("Test")
	// 0 fields + 2 default buttons = 2
	if d.focusableCount() != 2 {
		t.Errorf("focusableCount() = %d, want 2", d.focusableCount())
	}

	d.AddTextField("A", "", "", 0)
	// 1 field + 2 buttons = 3
	if d.focusableCount() != 3 {
		t.Errorf("focusableCount() = %d, want 3", d.focusableCount())
	}

	d.SetButtons(nil)
	// 1 field + 0 buttons = 1
	if d.focusableCount() != 1 {
		t.Errorf("focusableCount() = %d, want 1", d.focusableCount())
	}
}

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
