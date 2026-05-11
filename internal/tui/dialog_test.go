package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	if d.Buttons()[0].Label != "Save" || !d.Buttons()[0].Primary {
		t.Error("first button should be primary Save")
	}
	if d.Buttons()[1].Label != "Cancel" {
		t.Errorf("second button = %q, want %q", d.Buttons()[1].Label, "Cancel")
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

	d.FocusNext() // -> 2 (Save button)
	d.FocusNext() // -> 3 (Cancel button)
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

	d.SetFocusIndex(1) // Save button
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

	d.SetFocusIndex(1) // Save button
	if !d.IsFocusOnButton() {
		t.Error("should be on button at index 1")
	}
}

func TestDialog_FocusedButtonIndex(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	// index 0 = field, 1 = Save (primary), 2 = Cancel

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

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("Esc action = %d, want DialogActionCancel", action)
	}
}

func TestDialog_HandleKey_Tab(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
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

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
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
	// buttons: Save/Primary (idx 1), Cancel (idx 2)
	d.SetFocusIndex(1) // Save button

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionSubmit {
		t.Errorf("Enter on primary action = %d, want DialogActionSubmit", action)
	}
}

func TestDialog_HandleKey_EnterOnCancelButton(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.SetFocusIndex(2) // Cancel button

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionCancel {
		t.Errorf("Enter on cancel action = %d, want DialogActionCancel", action)
	}
}

func TestDialog_HandleKey_EnterOnField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.AddTextField("B", "", "", 0)

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	d.HandleKey(tea.KeyPressMsg{Code: 'H', Text: "H"})
	d.HandleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})

	if d.Fields()[0].Value != "Hi" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "Hi")
	}
}

func TestDialog_HandleKey_SpaceInTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'M', Text: "M"})
	d.HandleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	d.HandleKey(tea.KeyPressMsg{Code: 'A', Text: "A"})
	d.HandleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	d.HandleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	d.HandleKey(tea.KeyPressMsg{Code: 't', Text: "t"})

	if d.Fields()[0].Value != "My Acct" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "My Acct")
	}
}

func TestDialog_HandleKey_SpaceInTextField_ClearsError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "test", "", 0)
	f.Error = "some error"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})

	if f.Error != "" {
		t.Errorf("Error should be cleared after space, got %q", f.Error)
	}
	if f.Value != "test " {
		t.Errorf("Value = %q, want %q", f.Value, "test ")
	}
}

func TestDialog_HandleKey_MultipleSpacesInTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'A', Text: "A"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	d.HandleKey(tea.KeyPressMsg{Code: 'B', Text: "B"})

	if d.Fields()[0].Value != "A  B" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "A  B")
	}
}

func TestDialog_HandleKey_BackspaceInTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "Hello", "", 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})

	if d.Fields()[0].Value != "Hell" {
		t.Errorf("Value = %q, want %q", d.Fields()[0].Value, "Hell")
	}
}

func TestDialog_HandleKey_ArrowsInTextField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "Hello", "", 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4", f.CursorPos())
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyHome})
	if f.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0 after Home", f.CursorPos())
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if f.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5 after End", f.CursorPos())
	}
}

func TestDialog_HandleKey_UpDownInSelectField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddSelectField("Color", []string{"Red", "Green", "Blue"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0", f.SelectedIndex)
	}
}

func TestDialog_HandleKey_RadioNavigation(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 after Right", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 after Left", f.SelectedIndex)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 after Down", f.SelectedIndex)
	}
}

func TestDialog_HandleKey_SpaceTogglesCheckbox(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)

	d.HandleKey(tea.KeyPressMsg{Code: ' ', Text: " "})
	if !f.Checked {
		t.Error("Checked should be true after space rune")
	}

	d.HandleKey(tea.KeyPressMsg{Code: ' ', Text: " "})
	if f.Checked {
		t.Error("Checked should be false after second space rune")
	}
}

func TestDialog_HandleKey_KeySpaceTogglesCheckbox(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if !f.Checked {
		t.Error("Checked should be true after KeySpace")
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if f.Checked {
		t.Error("Checked should be false after second KeySpace")
	}
}

func TestDialog_HandleKey_FocusOnButtonIgnoresFieldKeys(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "test", "", 0)
	d.SetFocusIndex(1) // Cancel button

	// Typing should do nothing when focus is on a button
	d.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if d.Fields()[0].Value != "test" {
		t.Errorf("Value = %q, should not change when focus is on button", d.Fields()[0].Value)
	}
}

func TestDialog_HandleKey_DeleteInTextField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "Hello", "", 0)
	f.cursorPos = 0

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDelete})
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

// TestOverlayCenter_PreservesBackgroundANSI guards the Turbo Vision
// "blue desktop" use case: when the background lines carry an SGR
// background color (here \x1b[44m for blue), the prefix and suffix
// bands on either side of a centered overlay must still carry the
// background ANSI. The previous implementation stripped ANSI from
// bgLine before slicing prefix/suffix, which collapsed the bands to
// terminal default and produced a black band around dialogs on
// colored desktops.
func TestOverlayCenter_PreservesBackgroundANSI(t *testing.T) {
	const blueBg = "\x1b[44m"
	const reset = "\x1b[0m"
	bgRow := blueBg + strings.Repeat(" ", 20) + reset
	background := strings.Join([]string{bgRow, bgRow, bgRow, bgRow, bgRow}, "\n")

	overlay := "DIALOG" // visible width 6

	result := OverlayCenter(background, overlay, 20, 5)
	lines := strings.Split(result, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Overlay sits on the middle row: startRow=(5-1)/2=2.
	middle := lines[2]
	if !strings.Contains(middle, "DIALOG") {
		t.Fatalf("middle row should contain overlay, got %q", middle)
	}
	if !strings.Contains(middle, blueBg) {
		t.Errorf("middle row should preserve blue-bg ANSI on prefix/suffix, got %q", middle)
	}

	// Sanity: untouched rows still carry the blue ANSI.
	for _, idx := range []int{0, 1, 3, 4} {
		if !strings.Contains(lines[idx], blueBg) {
			t.Errorf("row %d should still carry blue-bg ANSI, got %q", idx, lines[idx])
		}
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
	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
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

// =============================================================================
// Validation Tests
// =============================================================================

func TestDialog_Render_RequiredMarker(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "Enter name", 0)
	f.Required = true
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "*") {
		t.Error("Render() should contain '*' for required field")
	}
}

func TestDialog_Render_FieldError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "Enter name", 0)
	f.Error = "Name is required"
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Name is required") {
		t.Error("Render() should show field error message")
	}
}

func TestDialog_Render_DialogErrorMsg(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.SetErrorMsg("Cross-field error")
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Cross-field error") {
		t.Error("Render() should show dialog-level error message")
	}
}

func TestDialog_ClearErrors(t *testing.T) {
	d := NewDialog("Test")
	f1 := d.AddTextField("A", "", "", 0)
	f2 := d.AddTextField("B", "", "", 0)
	f1.Error = "error1"
	f2.Error = "error2"
	d.SetErrorMsg("dialog error")

	d.ClearErrors()

	if f1.Error != "" {
		t.Errorf("field 1 error = %q, want empty", f1.Error)
	}
	if f2.Error != "" {
		t.Errorf("field 2 error = %q, want empty", f2.Error)
	}
	if d.ErrorMsg() != "" {
		t.Errorf("dialog errorMsg = %q, want empty", d.ErrorMsg())
	}
}

func TestDialog_FieldByLabel(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("First", "", "", 0)
	d.AddTextField("Second", "", "", 0)
	d.AddTextField("Third", "", "", 0)

	f := d.FieldByLabel("Second")
	if f == nil {
		t.Fatal("FieldByLabel('Second') returned nil")
	}
	if f.Label != "Second" {
		t.Errorf("FieldByLabel('Second') returned field with label %q", f.Label)
	}
}

func TestDialog_FieldByLabel_NotFound(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("First", "", "", 0)

	f := d.FieldByLabel("Missing")
	if f != nil {
		t.Error("FieldByLabel('Missing') should return nil")
	}
}

func TestDialog_EditingFieldClearsError_Text(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "", 0)
	f.Error = "required"

	// Type a character
	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if f.Error != "" {
		t.Errorf("Error should be cleared after typing, got %q", f.Error)
	}
}

func TestDialog_EditingFieldClearsError_Backspace(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "x", "", 0)
	f.Error = "required"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})

	if f.Error != "" {
		t.Errorf("Error should be cleared after backspace, got %q", f.Error)
	}
}

func TestDialog_EditingFieldClearsError_Select(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddSelectField("Type", []string{"A", "B"}, 0)
	f.Error = "required"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})

	if f.Error != "" {
		t.Errorf("Error should be cleared after select change, got %q", f.Error)
	}
}

func TestDialog_EditingFieldClearsError_Radio(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddRadioField("Status", []string{"A", "B"}, 0)
	f.Error = "required"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})

	if f.Error != "" {
		t.Errorf("Error should be cleared after radio change, got %q", f.Error)
	}
}

func TestDialog_EditingFieldClearsError_Checkbox(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)
	f.Error = "required"

	d.HandleKey(tea.KeyPressMsg{Code: ' ', Text: " "})

	if f.Error != "" {
		t.Errorf("Error should be cleared after checkbox toggle, got %q", f.Error)
	}
}

func TestDialog_MaxLabelWidth_WithRequired(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)       // width 1
	f := d.AddTextField("BB", "", "", 0) // width 2
	f.Required = true                    // width 2 + 1 = 3

	maxW := d.maxLabelWidth()
	if maxW != 3 {
		t.Errorf("maxLabelWidth() = %d, want 3 (BB + *)", maxW)
	}
}

func TestDialog_SetErrorMsg(t *testing.T) {
	d := NewDialog("Test")
	d.SetErrorMsg("test error")
	if d.ErrorMsg() != "test error" {
		t.Errorf("ErrorMsg() = %q, want %q", d.ErrorMsg(), "test error")
	}
}

func TestDialog_Render_CheckboxFieldError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)
	f.Error = "Must accept"
	styles := NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Must accept") {
		t.Error("Render() should show checkbox field error")
	}
}

func TestDialog_Render_SelectFieldTruncatesLongOption(t *testing.T) {
	d := NewDialog("Test")
	d.AddSelectField("Account", []string{
		"My Extremely Long Account Name That Should Be Truncated For Display",
	}, 0)
	styles := NewStyles()

	result := d.Render(styles)
	// The full option name should NOT appear
	if strings.Contains(result, "My Extremely Long Account Name That Should Be Truncated For Display") {
		t.Error("Render() should truncate long select option text")
	}
	// Dropdown indicator should still appear
	if !strings.Contains(result, "▼") {
		t.Error("Render() should still show dropdown indicator")
	}
}

func TestDialog_Render_RadioFieldTruncatesLongOptions(t *testing.T) {
	d := NewDialog("Test")
	d.AddRadioField("Type", []string{
		"A Really Long Radio Option Label One",
		"A Really Long Radio Option Label Two",
		"A Really Long Radio Option Label Three",
	}, 0)
	styles := NewStyles()

	result := d.Render(styles)
	// The full option name should NOT all appear
	if strings.Contains(result, "A Really Long Radio Option Label Three") {
		t.Error("Render() should truncate long radio option text")
	}
	// Radio bullets should still appear
	if !strings.Contains(result, "(*)") {
		t.Error("Render() should still show radio bullets")
	}
}

// =============================================================================
// FieldList Tests
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

func TestDialog_HandleKey_UpDownInListField(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("File", []string{"a.tdb", "b.tdb", "c.tdb"}, 0, 10)
	d.SetFocusIndex(0) // Focus on list field

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if d.Fields()[0].SelectedIndex != 1 {
		t.Errorf("after Down: SelectedIndex = %d, want 1", d.Fields()[0].SelectedIndex)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if d.Fields()[0].SelectedIndex != 0 {
		t.Errorf("after Up: SelectedIndex = %d, want 0", d.Fields()[0].SelectedIndex)
	}
}

func TestDialog_Render_ListFieldShowsItems(t *testing.T) {
	d := NewDialog("Test")
	d.SetWidth(60)
	d.AddListField("File", []string{"../", "backups/", "default.tdb"}, 1, 10)
	d.SetFocusIndex(0)

	styles := NewStyles()
	result := d.Render(styles)

	if !strings.Contains(result, "../") {
		t.Error("Render should show '../' entry")
	}
	if !strings.Contains(result, "backups/") {
		t.Error("Render should show 'backups/' entry")
	}
	if !strings.Contains(result, "default.tdb") {
		t.Error("Render should show 'default.tdb' entry")
	}
}

// --- Mouse support tests ---

func TestDialog_ContentHeight_NoFields(t *testing.T) {
	d := NewDialog("Test")
	// Layout: title(1) + sep(1) + sep(1) + buttonRow(1) = 4
	// No fields means no blank-after-fields line
	got := d.ContentHeight()
	if got != 4 {
		t.Errorf("ContentHeight() = %d, want 4", got)
	}
}

func TestDialog_ContentHeight_OneTextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 20)
	// Layout: title(1) + sep(1) + blank(1) + field(1) + blankAfter(1) + sep(1) + buttons(1) = 7
	got := d.ContentHeight()
	if got != 7 {
		t.Errorf("ContentHeight() = %d, want 7", got)
	}
}

func TestDialog_ContentHeight_ListField(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("Items", []string{"a", "b", "c", "d", "e"}, 0, 3)
	// Layout: title(1) + sep(1) + blank(1) + label(1)+items(3) + blankAfter(1) + sep(1) + buttons(1) = 10
	got := d.ContentHeight()
	if got != 10 {
		t.Errorf("ContentHeight() = %d, want 10", got)
	}
}

func TestDialog_ContentHeight_MixedFields(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)
	d.AddCheckboxField("Active", false)
	d.AddListField("Items", []string{"a", "b", "c"}, 0, 3)
	// title(1) + sep(1) + blank(1)+text(1) + blank(1)+checkbox(1) + blank(1)+list(1+3) + blankAfter(1) + sep(1) + buttons(1) = 14
	got := d.ContentHeight()
	if got != 14 {
		t.Errorf("ContentHeight() = %d, want 14", got)
	}
}

func TestDialog_ContentHeight_HiddenFields(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "", 0)
	f.Hidden = true
	d.AddTextField("Visible", "", "", 0)
	// title(1) + sep(1) + blank(1)+field(1) + blankAfter(1) + sep(1) + buttons(1) = 7
	got := d.ContentHeight()
	if got != 7 {
		t.Errorf("ContentHeight() = %d, want 7", got)
	}
}

func TestDialog_ContentHeight_WithFieldError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "", 0)
	f.Error = "required"
	// title(1) + sep(1) + blank(1)+field(1)+error(1) + blankAfter(1) + sep(1) + buttons(1) = 8
	got := d.ContentHeight()
	if got != 8 {
		t.Errorf("ContentHeight() = %d, want 8", got)
	}
}

func TestDialog_ContentHeight_WithDialogError(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "", "", 0)
	d.SetErrorMsg("something went wrong")
	// title(1) + sep(1) + blank(1)+field(1) + blankAfter(1) + error(1)+blank(1) + sep(1) + buttons(1) = 9
	got := d.ContentHeight()
	if got != 9 {
		t.Errorf("ContentHeight() = %d, want 9", got)
	}
}

func TestDialog_RenderedHeight(t *testing.T) {
	d := NewDialog("Test")
	contentH := d.ContentHeight()
	got := d.RenderedHeight()
	want := contentH + 4 // border(2) + padding(2)
	if got != want {
		t.Errorf("RenderedHeight() = %d, want %d (ContentHeight %d + 4)", got, want, contentH)
	}
}

func TestDialog_DialogBounds(t *testing.T) {
	d := NewDialog("Test")
	d.SetWidth(56)
	startCol, startRow, endCol, endRow := d.DialogBounds(80, 24)

	renderedH := d.RenderedHeight()
	wantStartCol := (80 - 56) / 2
	wantStartRow := (24 - renderedH) / 2
	wantEndCol := wantStartCol + 56
	wantEndRow := wantStartRow + renderedH

	if startCol != wantStartCol {
		t.Errorf("startCol = %d, want %d", startCol, wantStartCol)
	}
	if startRow != wantStartRow {
		t.Errorf("startRow = %d, want %d", startRow, wantStartRow)
	}
	if endCol != wantEndCol {
		t.Errorf("endCol = %d, want %d", endCol, wantEndCol)
	}
	if endRow != wantEndRow {
		t.Errorf("endRow = %d, want %d", endRow, wantEndRow)
	}
}

func TestDialog_DialogBounds_LargeDialog(t *testing.T) {
	d := NewDialog("Test")
	d.SetWidth(100)
	startCol, startRow, _, _ := d.DialogBounds(80, 24)
	// Should clamp to 0
	if startCol != 0 {
		t.Errorf("startCol = %d, want 0 for dialog wider than screen", startCol)
	}
	if startRow < 0 {
		t.Errorf("startRow = %d, should not be negative", startRow)
	}
}

func TestDialog_HitTestContent_CloseButton(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Close button [x] is at contentWidth-3 to contentWidth-1 on row 0
	hit := d.HitTestContent(contentWidth-3, 0, contentWidth)
	if hit.Zone != DialogHitCloseButton {
		t.Errorf("expected DialogHitCloseButton, got %d", hit.Zone)
	}

	hit = d.HitTestContent(contentWidth-1, 0, contentWidth)
	if hit.Zone != DialogHitCloseButton {
		t.Errorf("expected DialogHitCloseButton at rightmost char, got %d", hit.Zone)
	}
}

func TestDialog_HitTestContent_CloseButton_Miss(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Click on title text area (not close button)
	hit := d.HitTestContent(0, 0, contentWidth)
	if hit.Zone != DialogHitNone {
		t.Errorf("expected DialogHitNone for title area, got %d", hit.Zone)
	}

	hit = d.HitTestContent(contentWidth-4, 0, contentWidth)
	if hit.Zone != DialogHitNone {
		t.Errorf("expected DialogHitNone just before close button, got %d", hit.Zone)
	}
}

func TestDialog_HitTestContent_Separator(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Row 1 is the separator
	hit := d.HitTestContent(5, 1, contentWidth)
	if hit.Zone != DialogHitNone {
		t.Errorf("expected DialogHitNone for separator, got %d", hit.Zone)
	}
}

func TestDialog_HitTestContent_TextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "hello", "", 20)
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Row 2 = blank before field, row 3 = text field
	hit := d.HitTestContent(10, 3, contentWidth)
	if hit.Zone != DialogHitField {
		t.Errorf("expected DialogHitField, got %d", hit.Zone)
	}
	if hit.FieldIndex != 0 {
		t.Errorf("expected FieldIndex 0, got %d", hit.FieldIndex)
	}
}

func TestDialog_HitTestContent_CheckboxField(t *testing.T) {
	d := NewDialog("Test")
	d.AddCheckboxField("Accept", false)
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Row 2 = blank, row 3 = checkbox
	hit := d.HitTestContent(5, 3, contentWidth)
	if hit.Zone != DialogHitField {
		t.Errorf("expected DialogHitField, got %d", hit.Zone)
	}
	if hit.FieldIndex != 0 {
		t.Errorf("expected FieldIndex 0, got %d", hit.FieldIndex)
	}
}

func TestDialog_HitTestContent_ListField_Item(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("Files", []string{"../", "docs/", "main.go", "go.mod"}, 0, 4)
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Row 2 = blank, row 3 = label line, row 4-7 = list items
	// Row 4 = item 0 ("../"), row 5 = item 1 ("docs/"), etc.
	hit := d.HitTestContent(10, 5, contentWidth)
	if hit.Zone != DialogHitField {
		t.Errorf("expected DialogHitField, got %d", hit.Zone)
	}
	if hit.FieldIndex != 0 {
		t.Errorf("expected FieldIndex 0, got %d", hit.FieldIndex)
	}
	if hit.ListItemIndex != 1 {
		t.Errorf("expected ListItemIndex 1 (docs/), got %d", hit.ListItemIndex)
	}
}

func TestDialog_HitTestContent_ListField_DifferentItems(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("Files", []string{"a", "b", "c", "d", "e"}, 0, 3)
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Row 3 = label, rows 4-6 = visible items (a, b, c since selected=0)
	tests := []struct {
		y    int
		want int
	}{
		{4, 0}, // item "a"
		{5, 1}, // item "b"
		{6, 2}, // item "c"
	}
	for _, tt := range tests {
		hit := d.HitTestContent(10, tt.y, contentWidth)
		if hit.ListItemIndex != tt.want {
			t.Errorf("row %d: ListItemIndex = %d, want %d", tt.y, hit.ListItemIndex, tt.want)
		}
	}
}

func TestDialog_HitTestContent_Button_Primary(t *testing.T) {
	d := NewDialog("Test")
	// Default buttons: Save (idx 0, primary), Cancel (idx 1)
	contentWidth := d.Width() - dialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	// Save is the primary button at index 0. Brute-force x positions until we hit it.
	found := false
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("could not find Save (primary) button via HitTestContent")
	}
}

func TestDialog_HitTestContent_Button_Cancel(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - dialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	found := false
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("could not find Cancel button via HitTestContent")
	}
}

func TestDialog_HitTestContent_OutOfBounds(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - dialogHorizontalOverhead

	tests := []struct {
		name string
		x, y int
	}{
		{"negative y", 5, -1},
		{"negative x", -1, 0},
		{"x past width", contentWidth, 0},
		{"y past content", 5, d.ContentHeight() + 10},
	}
	for _, tt := range tests {
		hit := d.HitTestContent(tt.x, tt.y, contentWidth)
		if hit.Zone != DialogHitNone {
			t.Errorf("%s: expected DialogHitNone, got %d", tt.name, hit.Zone)
		}
	}
}

func TestDialog_HandleMouse_ClickCloseButton(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - dialogHorizontalOverhead
	// Close button at content x = contentWidth-3, content y = 0
	// Screen x = startCol + 3 (border+padding) + contentWidth - 3
	// Screen y = startRow + 2 (border+padding)
	clickX := startCol + 3 + contentWidth - 2
	clickY := startRow + 2

	action := d.HandleMouse(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionCancel {
		t.Errorf("expected DialogActionCancel, got %d", action)
	}
}

func TestDialog_HandleMouse_ClickField_SetsFocus(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("First", "", "", 0)
	d.AddTextField("Second", "", "", 0)
	d.SetFocusIndex(0) // Focus on first field
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	// Second field: blank(1) at row 2, field(1) at row 3, blank(1) at row 4, field at row 5
	// Content row for second text field = 5
	clickX := startCol + 3 + 10 // some x within field area
	clickY := startRow + 2 + 5  // content row 5

	d.HandleMouse(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex() = %d, want 1", d.FocusIndex())
	}
}

func TestDialog_HandleMouse_ClickTextField_PositionsCursor(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "hello world", "", 0)
	d.SetFocusIndex(0)
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	labelWidth := d.maxLabelWidth()
	// Text content starts at: labelWidth + 1 (colon) + 2 (gap) + 2 ("[ ")
	textStart := labelWidth + 1 + 2 + 2

	// Click at position that maps to cursor position 3
	clickX := startCol + 3 + textStart + 3
	clickY := startRow + 2 + 3 // content row 3 = text field row

	d.HandleMouse(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if d.Fields()[0].CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3", d.Fields()[0].CursorPos())
	}
}

func TestDialog_HandleMouse_ClickCheckbox_Toggles(t *testing.T) {
	d := NewDialog("Test")
	d.AddCheckboxField("Accept", false)
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	// Checkbox is at content row 3 (title=0, sep=1, blank=2, checkbox=3)
	clickX := startCol + 3 + 5
	clickY := startRow + 2 + 3

	d.HandleMouse(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if !d.Fields()[0].Checked {
		t.Error("checkbox should be toggled to true")
	}
}

func TestDialog_HandleMouse_ClickListItem(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("Files", []string{"../", "docs/", "main.go", "go.mod"}, 0, 4)
	d.SetFocusIndex(0)
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	// List: content row 3 = label, rows 4-7 = items
	// Click on row 6 = item index 2 ("main.go")
	clickX := startCol + 3 + 10
	clickY := startRow + 2 + 6

	d.HandleMouse(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if d.Fields()[0].SelectedIndex != 2 {
		t.Errorf("SelectedIndex = %d, want 2", d.Fields()[0].SelectedIndex)
	}
}

func TestDialog_HandleMouse_ClickButton_Submit(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - dialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	// Find the Save (primary) button x position
	var saveX int
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 0 {
			saveX = x
			break
		}
	}

	action := d.HandleMouse(tea.MouseClickMsg{
		X:      startCol + 3 + saveX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionSubmit {
		t.Errorf("expected DialogActionSubmit, got %d", action)
	}
}

func TestDialog_HandleMouse_ClickButton_Cancel(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - dialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	// Find the Cancel button x position
	var cancelX int
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 1 {
			cancelX = x
			break
		}
	}

	action := d.HandleMouse(tea.MouseClickMsg{
		X:      startCol + 3 + cancelX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionCancel {
		t.Errorf("expected DialogActionCancel, got %d", action)
	}
}

func TestDialog_HandleMouse_OutsideDialog(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	screenW, screenH := 80, 24

	action := d.HandleMouse(tea.MouseClickMsg{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionNone {
		t.Errorf("expected DialogActionNone for click outside, got %d", action)
	}
}

func TestDialog_HandleMouse_WheelOnList(t *testing.T) {
	d := NewDialog("Test")
	d.AddListField("Items", []string{"a", "b", "c", "d", "e"}, 0, 3)
	d.SetFocusIndex(0) // Focus on list
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	// Wheel down within dialog bounds
	d.HandleMouse(tea.MouseWheelMsg{
		X:      startCol + 10,
		Y:      startRow + 5,
		Button: tea.MouseWheelDown,
	}, screenW, screenH)

	if d.Fields()[0].SelectedIndex != 1 {
		t.Errorf("SelectedIndex after wheel down = %d, want 1", d.Fields()[0].SelectedIndex)
	}

	// Wheel up
	d.HandleMouse(tea.MouseWheelMsg{
		X:      startCol + 10,
		Y:      startRow + 5,
		Button: tea.MouseWheelUp,
	}, screenW, screenH)

	if d.Fields()[0].SelectedIndex != 0 {
		t.Errorf("SelectedIndex after wheel up = %d, want 0", d.Fields()[0].SelectedIndex)
	}
}

func TestDialog_HandleMouse_MouseRelease_Ignored(t *testing.T) {
	d := NewDialog("Test")
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - dialogHorizontalOverhead

	// Mouse release on close button should be ignored
	action := d.HandleMouse(tea.MouseReleaseMsg{
		X:      startCol + 3 + contentWidth - 2,
		Y:      startRow + 2,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionNone {
		t.Errorf("expected DialogActionNone for mouse release, got %d", action)
	}
}

// FieldDate (TD-002) tests

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

func TestRankComboMatches_EmptyQueryReturnsAllInOrder(t *testing.T) {
	got := rankComboMatches([]string{"a", "b", "c"}, "")
	want := []int{0, 1, 2}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_FilterFiltersByQuery(t *testing.T) {
	opts := []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}

	got := rankComboMatches(opts, "f")
	want := []int{3, 4}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('f') = %v, want %v", got, want)
	}

	got = rankComboMatches(opts, "g")
	want = []int{3}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('g') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_CaseInsensitive(t *testing.T) {
	got := rankComboMatches([]string{"Food > Groceries", "Other"}, "Gr")
	want := []int{0}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('Gr') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_PrefixBeforeSubstring(t *testing.T) {
	// Flat names: prefix matches rank ahead of substring matches; alphabetical
	// within each rank group.
	got := rankComboMatches([]string{"Restaurant Co", "Auto Repair", "Restaurant Bar"}, "r")
	want := []int{2, 0, 1} // "Restaurant Bar", "Restaurant Co", then "Auto Repair"
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('r') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_PrefixOnLeafSegment(t *testing.T) {
	// "Food > Restaurants" leaf segment "Restaurants" prefix-matches 'r'; the
	// other 'r'-containing options are substring matches and rank below it.
	opts := []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}
	got := rankComboMatches(opts, "r")
	want := []int{4, 2, 3}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('r') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_NoMatchReturnsEmpty(t *testing.T) {
	got := rankComboMatches([]string{"a", "b", "c"}, "z")
	if len(got) != 0 {
		t.Errorf("rankComboMatches('z') = %v, want empty", got)
	}
}

func TestDialog_AddComboField_BasicConstruction(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	if f.Type != FieldCombo {
		t.Errorf("Type = %v, want FieldCombo", f.Type)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}
	if f.Query != "" {
		t.Errorf("Query = %q, want empty", f.Query)
	}
	// Empty query: highlight tracks the current selection.
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1", got)
	}
}

func TestDialog_AddComboField_ClampsSelected(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b"}, 99)
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (clamped)", f.SelectedIndex)
	}

	f = d.AddComboField("Other", []string{"a", "b"}, -3)
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (clamped)", f.SelectedIndex)
	}
}

func TestFieldCombo_TypingAppendsToQueryAndFilters(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if f.Query != "f" {
		t.Errorf("Query = %q, want %q", f.Query, "f")
	}
	if got := f.FilteredIndices(); !slices.Equal(got, []int{3, 4}) {
		t.Errorf("FilteredIndices = %v, want [3 4]", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if f.Query != "fo" {
		t.Errorf("Query = %q, want %q", f.Query, "fo")
	}
}

func TestFieldCombo_BackspaceShrinksQuery(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "ab", "abc"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	d.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if f.Query != "ab" {
		t.Fatalf("setup: Query = %q, want %q", f.Query, "ab")
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "a" {
		t.Errorf("Query after backspace = %q, want %q", f.Query, "a")
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "" {
		t.Errorf("Query after second backspace = %q, want empty", f.Query)
	}

	// Backspace at empty query is a no-op (does not leave the field).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "" {
		t.Errorf("Query after third backspace = %q, want empty", f.Query)
	}
}

func TestFieldCombo_ClearingQueryReturnsToFullList(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Food"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if got := f.FilteredIndices(); !slices.Equal(got, []int{2}) {
		t.Fatalf("setup filtered = %v, want [2]", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := f.FilteredIndices(); !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("FilteredIndices after clear = %v, want [0 1 2]", got)
	}
}

func TestFieldCombo_DownNavigatesFilteredOnly(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	// Filtered = [3, 4]; highlight starts at 0 → idx 3.
	if got := f.HighlightedIndex(); got != 3 {
		t.Fatalf("setup: HighlightedIndex = %d, want 3", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Errorf("HighlightedIndex after Down = %d, want 4", got)
	}

	// Past the last filtered row → stays put (no wrap).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Errorf("HighlightedIndex after second Down = %d, want 4 (no wrap)", got)
	}
}

func TestFieldCombo_UpNavigatesFilteredOnly(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Fatalf("setup: HighlightedIndex = %d, want 4", got)
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := f.HighlightedIndex(); got != 3 {
		t.Errorf("HighlightedIndex after Up = %d, want 3", got)
	}
	// Past the first filtered row → stays put.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := f.HighlightedIndex(); got != 3 {
		t.Errorf("HighlightedIndex after second Up = %d, want 3 (no wrap)", got)
	}
}

func TestFieldCombo_EnterCommitsHighlightedClearsQueryAdvancesFocus(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if f.Query != "" {
		t.Errorf("Query after Enter = %q, want empty", f.Query)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Enter = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex after Enter = %d, want 1 (advanced)", d.FocusIndex())
	}
}

func TestFieldCombo_TabCommitsHighlightedAndAdvancesFocus(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})

	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Tab = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex after Tab = %d, want 1 (advanced)", d.FocusIndex())
	}
	if f.Query != "" {
		t.Errorf("Query after Tab = %q, want empty", f.Query)
	}
}

func TestFieldCombo_ShiftTabCommitsAndMovesFocusBack(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Payee", "", "", 10)
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)

	// Focus on the combo field.
	d.SetFocusIndex(1)
	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Shift+Tab = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex after Shift+Tab = %d, want 0 (moved back)", d.FocusIndex())
	}
}

func TestFieldCombo_EscClearsQueryWithoutLeavingField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	d.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if f.Query != "b" {
		t.Fatalf("setup: Query = %q, want %q", f.Query, "b")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone (Esc with non-empty query)", action)
	}
	if f.Query != "" {
		t.Errorf("Query after Esc = %q, want empty", f.Query)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Esc = %d, want 1 (unchanged)", f.SelectedIndex)
	}
}

func TestFieldCombo_EscWithEmptyQueryCancelsDialog(t *testing.T) {
	d := NewDialog("Test")
	d.AddComboField("Category", []string{"a", "b", "c"}, 0)

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("action = %v, want DialogActionCancel (Esc with empty query)", action)
	}
}

func TestFieldCombo_EmptyQueryHighlightsCurrentSelection(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 2)

	if got := f.HighlightedIndex(); got != 2 {
		t.Errorf("HighlightedIndex with empty query = %d, want 2 (current selection)", got)
	}
}

func TestFieldCombo_FilteredIndicesEmptyQueryReturnsAll(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	got := f.FilteredIndices()
	want := []int{0, 1, 2}
	if !slices.Equal(got, want) {
		t.Errorf("FilteredIndices empty = %v, want %v", got, want)
	}
}

func TestFieldCombo_NoMatchEnterPreservesSelection(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if got := f.FilteredIndices(); len(got) != 0 {
		t.Fatalf("setup: FilteredIndices = %v, want empty", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Enter with no match = %d, want 1 (preserved)", f.SelectedIndex)
	}
	if f.Query != "" {
		t.Errorf("Query after Enter = %q, want empty", f.Query)
	}
}

func TestFieldCombo_QueryChangeResetsHighlight(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Apple", "Banana", "Cherry"}, 2)

	// With empty query, highlight tracks the current selection (Cherry, idx 2).
	if got := f.HighlightedIndex(); got != 2 {
		t.Fatalf("setup: HighlightedIndex = %d, want 2", got)
	}

	// Typing 'a' filters to Apple, Banana — highlight resets to 0 (Apple).
	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if got := f.HighlightedIndex(); got != 0 {
		t.Errorf("HighlightedIndex after 'a' = %d, want 0 (Apple)", got)
	}
}

func TestFieldCombo_RenderShowsFilteredListWhenFocused(t *testing.T) {
	d := NewDialog("Test")
	d.AddComboField("Category", []string{"(None)", "Food > Groceries", "Food > Restaurants"}, 0)
	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})

	styles := NewStyles()
	out := d.Render(styles)

	if !strings.Contains(out, "Food > Groceries") {
		t.Errorf("rendered output should list filtered match Food > Groceries; got:\n%s", out)
	}
	if !strings.Contains(out, "Food > Restaurants") {
		t.Errorf("rendered output should list filtered match Food > Restaurants; got:\n%s", out)
	}
}

// === FieldCombo: AddNew action row ===

func TestFieldCombo_AddNewLabel_FilteredIndicesUnchanged(t *testing.T) {
	// FilteredIndices contains only real Options; the action row is separate.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	got := f.FilteredIndices()
	want := []int{0, 1}
	if !slices.Equal(got, want) {
		t.Errorf("FilteredIndices = %v, want %v (action row not in Options)", got, want)
	}
}

func TestFieldCombo_AddNewLabel_DownNavigatesPastLastMatchToActionRow(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	// Empty query, two matches; highlight starts at SelectedIndex (0).
	if f.IsAddNewHighlighted() {
		t.Fatalf("setup: IsAddNewHighlighted = true at start, want false")
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> idx 1 (Auto)
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> action row
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false after Down past last, want true")
	}
	// One more Down stays put (no wrap past action row).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false after extra Down, want true (no wrap)")
	}
}

func TestFieldCombo_AddNewLabel_UpFromActionRowReturnsToLastMatch(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row not highlighted")
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = true after Up, want false")
	}
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1 (Auto)", got)
	}
}

func TestFieldCombo_AddNewLabel_NoMatchesActionRowHighlightedAtIndexZero(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	// Type a non-matching query: no real matches; action row is the only row.
	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if got := f.FilteredIndices(); len(got) != 0 {
		t.Fatalf("setup: FilteredIndices = %v, want empty", got)
	}
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false with no matches and AddNewLabel set, want true")
	}
}

func TestFieldCombo_AddNewLabel_EnterOnActionRowReturnsDialogActionAddNew(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	// Type "Donations" — no matches; action row is the only row.
	for _, r := range "Donations" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row should be highlighted (no matches)")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionAddNew {
		t.Errorf("action = %v, want DialogActionAddNew", action)
	}
	if !f.AddNewTriggered {
		t.Errorf("AddNewTriggered = false, want true")
	}
	if f.Query != "Donations" {
		t.Errorf("Query = %q, want %q (preserved for parent to read)", f.Query, "Donations")
	}
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (unchanged)", f.SelectedIndex)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex = %d, want 0 (focus must not advance — parent handles diversion)", d.FocusIndex())
	}
}

func TestFieldCombo_AddNewLabel_EnterOnRegularMatchCommitsNormally(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	// Filtered: ["Auto"]; highlight at first match.
	if f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row highlighted, want regular match")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone (regular commit)", action)
	}
	if f.AddNewTriggered {
		t.Errorf("AddNewTriggered = true, want false (regular commit)")
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (Auto)", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (advanced)", d.FocusIndex())
	}
}

func TestFieldCombo_AddNewLabel_RenderShowsActionRow(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	styles := NewStyles()
	out := d.Render(styles)
	if !strings.Contains(out, "[+ Add new category…]") {
		t.Errorf("render should include action row label; got:\n%s", out)
	}
}

func TestFieldCombo_AddNewLabel_RenderShowsActionRowWhenNoMatches(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})

	styles := NewStyles()
	out := d.Render(styles)
	if !strings.Contains(out, "[+ Add new category…]") {
		t.Errorf("render should include action row even when no matches; got:\n%s", out)
	}
}

func TestFieldCombo_NoAddNewLabel_DownStillStopsAtLastMatch(t *testing.T) {
	// Without AddNewLabel: behavior unchanged — Down stops at last match,
	// no action row exists, IsAddNewHighlighted is always false.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // tries to go past last
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1 (no action row, no wrap)", got)
	}
	if f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = true, want false (no action row configured)")
	}
}

func TestFieldCombo_AddNewLabel_TabOnActionRowDoesNotTriggerAddNew(t *testing.T) {
	// Tab on action row leaves the field (advances focus) without triggering
	// AddNew — only Enter triggers AddNew per spec.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row not highlighted")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if action == DialogActionAddNew {
		t.Errorf("action = DialogActionAddNew, want != AddNew (Tab does not trigger AddNew)")
	}
	if f.AddNewTriggered {
		t.Errorf("AddNewTriggered = true after Tab, want false")
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (Tab advanced focus)", d.FocusIndex())
	}
}

// FieldDate ISO format (TD-015) tests

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
