package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// HitTestContent and HandleMouse.

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
