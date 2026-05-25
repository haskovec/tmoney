package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// HandleKey + per-field key handlers.

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

func TestDialog_HandleKey_NoFieldsNoButtons(t *testing.T) {
	d := NewDialog("Empty")
	d.SetButtons(nil)

	// Should not panic
	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if action != DialogActionNone {
		t.Errorf("action = %d, want DialogActionNone", action)
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
