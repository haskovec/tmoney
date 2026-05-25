package tui

import (
	"testing"
)

// Dialog core: NewDialog, accessors, focus management, buttons, error clearing.

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

func TestDialog_SetErrorMsg(t *testing.T) {
	d := NewDialog("Test")
	d.SetErrorMsg("test error")
	if d.ErrorMsg() != "test error" {
		t.Errorf("ErrorMsg() = %q, want %q", d.ErrorMsg(), "test error")
	}
}
