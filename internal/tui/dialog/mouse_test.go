package dialog

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// HitTestContent and HandleMouse.

func TestDialog_HitTestContent_CloseButton(t *testing.T) {
	d := NewDialog("Test")
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead

	// Row 1 is the separator
	hit := d.HitTestContent(5, 1, contentWidth)
	if hit.Zone != DialogHitNone {
		t.Errorf("expected DialogHitNone for separator, got %d", hit.Zone)
	}
}

func TestDialog_HitTestContent_TextField(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "hello", "", 20)
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead
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
	contentWidth := d.Width() - DialogHorizontalOverhead
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
	contentWidth := d.Width() - DialogHorizontalOverhead

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
	contentWidth := d.Width() - DialogHorizontalOverhead
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
	contentWidth := d.Width() - DialogHorizontalOverhead
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
	contentWidth := d.Width() - DialogHorizontalOverhead
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

func TestDialog_HandleMouse_ClickButton_Alternate(t *testing.T) {
	d := NewDialog("Test")
	d.SetButtons([]DialogButton{
		{Label: "Save", Primary: true},
		{Label: "Cancel"},
		{Label: "Edit as paycheck →", Action: DialogActionAlternate},
	})
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - DialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	// Find the alternate (3rd) button x position.
	var altX int
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 2 {
			altX = x
			break
		}
	}

	action := d.HandleMouse(tea.MouseClickMsg{
		X:      startCol + 3 + altX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}, screenW, screenH)

	if action != DialogActionAlternate {
		t.Errorf("expected DialogActionAlternate, got %d", action)
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
	contentWidth := d.Width() - DialogHorizontalOverhead

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

// TestDialog_HandleMouse_ClickCancel_WhenScrolling reproduces the stock-split
// dialog symptom: a dialog whose content overflows its height bound scrolls,
// pinning the button row to the bottom. Clicking Cancel must still close it.
func TestDialog_HandleMouse_ClickCancel_WhenScrolling(t *testing.T) {
	d := NewDialog("Stock Split")
	d.SetMessage(strings.Repeat("preview line\n", 30)) // force overflow
	d.AddTextField("Date", "06/18/2008", "", 0)
	d.AddTextField("Ratio", "2:1", "", 0)
	d.SetVisible(true)

	screenW, screenH := 80, 24
	d.SetMaxHeight(screenH - 2) // what the app applies via overlayDialog

	if !d.isScrolling() {
		t.Fatalf("expected the dialog to be scrolling with maxHeight=%d", screenH-2)
	}

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.width - DialogHorizontalOverhead
	renderedContentRows := d.RenderedHeight() - dialogVerticalOverhead
	buttonScreenY := startRow + 2 + (renderedContentRows - 1) // last content row = button row

	got := DialogActionNone
	for x := range contentWidth {
		action := d.HandleMouse(tea.MouseClickMsg{
			X:      startCol + 3 + x,
			Y:      buttonScreenY,
			Button: tea.MouseLeft,
		}, screenW, screenH)
		if action == DialogActionCancel {
			got = DialogActionCancel
			break
		}
	}
	if got != DialogActionCancel {
		t.Error("clicking Cancel on a scrolling dialog did not return DialogActionCancel")
	}
}

// TestDialog_HandleMouse_ClickCancel_WithWrappingMessage covers the stock-split
// dialog shape: a long message line that wraps to several rows. If the row-walk
// hit-test counts the message differently from the renderer, the button row
// shifts and clicks miss.
func TestDialog_HandleMouse_ClickCancel_WithWrappingMessage(t *testing.T) {
	d := NewDialog("Stock Split")
	d.SetMessage("Ratio is N:M — N new shares for every M held. e.g. 2:1 = forward 2-for-1, 1:2 = halves shares.\n\nAfter split:\n  E*Trade Rollover IRA: 10 → 20 shares\n  Wealthfront IRA: 656.09894 → 1312.19788 shares")
	d.AddTextField("Date", "06/18/2008", "", 0)
	d.AddTextField("Ratio", "2:1", "", 0)
	d.SetVisible(true)

	screenW, screenH := 80, 40 // tall: should not scroll
	if d.isScrolling() {
		t.Fatalf("did not expect scrolling at height %d", screenH)
	}

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.width - DialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1

	got := DialogActionNone
	for x := range contentWidth {
		action := d.HandleMouse(tea.MouseClickMsg{
			X:      startCol + 3 + x,
			Y:      startRow + 2 + buttonRow,
			Button: tea.MouseLeft,
		}, screenW, screenH)
		if action == DialogActionCancel {
			got = DialogActionCancel
			break
		}
	}
	if got != DialogActionCancel {
		t.Error("clicking Cancel with a wrapping message did not return DialogActionCancel")
	}
}

// FieldCombo mouse tests — mouse support for the typeahead combo dropdown
// (e.g. the Security picker on the Buy/Sell/Dividend dialogs).

// TestDialog_HitTestContent_ComboPanel_MapsRows verifies that, while a combo
// is focused, clicks below the header line map to the dropdown-panel line
// index and the header line itself maps to no panel row.
func TestDialog_HitTestContent_ComboPanel_MapsRows(t *testing.T) {
	d := NewDialog("Buy Securities")
	d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 0)
	d.AddTextField("Shares", "", "", 10)
	contentWidth := d.Width() - DialogHorizontalOverhead

	// Combo is focused by default (field 0). Row 2 = blank before field,
	// row 3 = combo header (input line), rows 4/5/6 = the three panel rows.
	if hit := d.HitTestContent(5, 3, contentWidth); hit.Zone != DialogHitField || hit.FieldIndex != 0 || hit.ListItemIndex != -1 {
		t.Errorf("header row: got Zone=%d Field=%d Item=%d, want field 0 item -1", hit.Zone, hit.FieldIndex, hit.ListItemIndex)
	}
	for _, tt := range []struct{ y, want int }{{4, 0}, {5, 1}, {6, 2}} {
		hit := d.HitTestContent(5, tt.y, contentWidth)
		if hit.Zone != DialogHitField || hit.FieldIndex != 0 {
			t.Errorf("row %d: got Zone=%d Field=%d, want field 0", tt.y, hit.Zone, hit.FieldIndex)
		}
		if hit.ListItemIndex != tt.want {
			t.Errorf("row %d: ListItemIndex = %d, want %d", tt.y, hit.ListItemIndex, tt.want)
		}
	}
}

// TestDialog_HitTestContent_ComboPanel_NotExposedWhenUnfocused verifies the
// panel rows only exist while the combo is focused: an unfocused combo
// occupies a single header row, so a click one row below lands on the next
// element's blank spacer, not a phantom panel row.
func TestDialog_HitTestContent_ComboPanel_NotExposedWhenUnfocused(t *testing.T) {
	d := NewDialog("Buy Securities")
	d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 0)
	d.AddTextField("Shares", "", "", 10)
	d.SetFocusIndex(1) // focus the text field, not the combo
	contentWidth := d.Width() - DialogHorizontalOverhead

	// Row 3 = combo header (only row it occupies unfocused).
	if hit := d.HitTestContent(5, 3, contentWidth); hit.FieldIndex != 0 || hit.ListItemIndex != -1 {
		t.Errorf("header row: got Field=%d Item=%d, want field 0 item -1", hit.FieldIndex, hit.ListItemIndex)
	}
	// Row 4 = blank spacer before the text field — must not resolve to a
	// combo panel row.
	if hit := d.HitTestContent(5, 4, contentWidth); hit.Zone != DialogHitNone {
		t.Errorf("row below unfocused combo: got Zone=%d Field=%d Item=%d, want DialogHitNone", hit.Zone, hit.FieldIndex, hit.ListItemIndex)
	}
}

// TestDialog_ComboMouseClick_CommitsSelection reproduces the reported bug:
// after typing a filter, clicking a matching security row commits that
// selection (like Enter) rather than being ignored.
func TestDialog_ComboMouseClick_CommitsSelection(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{
		"VSGAX - Vanguard Small Cap Growth Index Adm",
		"VSIAX - Vanguard Small Cap Value Index Adm",
		"AAPL - Apple Inc",
	}, 0)
	d.AddTextField("Shares", "", "", 10)

	// Type the filter shown in the screenshot; leaves VSGAX (line 0) and
	// VSIAX (line 1), AAPL filtered out.
	for _, r := range "vanguard sm" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := f.FilteredIndices(); len(got) != 2 {
		t.Fatalf("filtered = %v, want 2 matches", got)
	}

	// Click the first match row (VSGAX): header = row 3, first panel = row 4.
	action := d.HandleMouseLocal(5, 4)
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone", action)
	}
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (VSGAX)", f.SelectedIndex)
	}
	if f.Query != "" {
		t.Errorf("Query = %q, want cleared after commit", f.Query)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (advanced past the combo, like Enter)", d.FocusIndex())
	}
}

// TestDialog_ComboMouseClick_SecondFilteredRow confirms the row-to-option
// mapping is correct when the clicked row is not the highlighted one.
func TestDialog_ComboMouseClick_SecondFilteredRow(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{
		"VSGAX - Vanguard Small Cap Growth Index Adm",
		"VSIAX - Vanguard Small Cap Value Index Adm",
		"AAPL - Apple Inc",
	}, 0)
	d.AddTextField("Shares", "", "", 10)

	for _, r := range "vanguard sm" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	// Click the second match row (VSIAX) at row 5.
	d.HandleMouseLocal(5, 5)
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (VSIAX)", f.SelectedIndex)
	}
}

// TestDialog_ComboMouseClick_AddNewRow verifies a click on the AddNew action
// row returns DialogActionAddNew and sets AddNewTriggered without advancing
// focus, mirroring Enter on that row.
func TestDialog_ComboMouseClick_AddNewRow(t *testing.T) {
	d := NewDialog("New Transaction")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Amount", "", "", 10)

	// Empty query: header = row 3, row 4 = Food, row 5 = Auto, row 6 = action.
	action := d.HandleMouseLocal(5, 6)
	if action != DialogActionAddNew {
		t.Errorf("action = %v, want DialogActionAddNew", action)
	}
	if !f.AddNewTriggered {
		t.Error("AddNewTriggered = false, want true")
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex = %d, want 0 (stays on the combo)", d.FocusIndex())
	}
}

// TestDialog_ComboMouseClick_NoMatchesInert verifies clicking the
// "(no matches)" placeholder does nothing.
func TestDialog_ComboMouseClick_NoMatchesInert(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{"VSGAX", "VSIAX"}, 0)
	d.AddTextField("Shares", "", "", 10)

	for _, r := range "zzz" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	// header = row 3, row 4 = "(no matches)" placeholder.
	action := d.HandleMouseLocal(5, 4)
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone", action)
	}
	if f.Query != "zzz" {
		t.Errorf("Query = %q, want unchanged", f.Query)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex = %d, want 0 (unchanged)", d.FocusIndex())
	}
}

// TestDialog_ComboMouseClick_WithScrollOffset verifies the click mapping
// tracks the dropdown's scroll window: after the highlight scrolls the panel,
// the visible rows map to the scrolled-into options, not the top of the list.
func TestDialog_ComboMouseClick_WithScrollOffset(t *testing.T) {
	opts := make([]string, 20)
	for i := range opts {
		opts[i] = fmt.Sprintf("SEC%02d", i)
	}
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", opts, 0)
	f.VisibleCount = 5
	d.AddTextField("Shares", "", "", 10)
	contentWidth := d.Width() - DialogHorizontalOverhead

	// Move the highlight to index 10; with visible=5 the window scrolls to
	// show lines 6..10 (scrollOffset = 10-5+1 = 6).
	for range 10 {
		d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	// First visible panel row (row 4) is line 6; last (row 8) is line 10.
	if hit := d.HitTestContent(5, 4, contentWidth); hit.ListItemIndex != 6 {
		t.Errorf("first visible row: ListItemIndex = %d, want 6", hit.ListItemIndex)
	}
	if hit := d.HitTestContent(5, 8, contentWidth); hit.ListItemIndex != 10 {
		t.Errorf("last visible row: ListItemIndex = %d, want 10", hit.ListItemIndex)
	}

	// Commit the first visible row → SEC06 (option index 6), not SEC00.
	d.HandleMouseLocal(5, 4)
	if f.SelectedIndex != 6 {
		t.Errorf("SelectedIndex = %d, want 6 (SEC06, first visible after scroll)", f.SelectedIndex)
	}
}

// TestDialog_ComboMouseClick_WithFieldError verifies the panel-row mapping
// stays aligned when the focused combo carries an inline validation error.
// The error renders after the panel (not between header and panel), so a
// click still commits the option drawn under the cursor — not its neighbor —
// and clicking the error line itself is inert.
func TestDialog_ComboMouseClick_WithFieldError(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 0)
	d.AddTextField("Shares", "", "", 10)
	f.Error = "Select a security" // as a failed submit would set

	contentWidth := d.Width() - DialogHorizontalOverhead
	// Row 2 = blank, row 3 = header, rows 4/5/6 = panel (VSGAX/VSIAX/AAPL),
	// row 7 = the trailing error line.
	for _, tt := range []struct{ y, want int }{{4, 0}, {5, 1}, {6, 2}, {7, -1}} {
		if hit := d.HitTestContent(5, tt.y, contentWidth); hit.ListItemIndex != tt.want {
			t.Errorf("row %d: ListItemIndex = %d, want %d", tt.y, hit.ListItemIndex, tt.want)
		}
	}

	// Click the first option (row 4) → commits VSGAX (index 0), not VSIAX.
	d.HandleMouseLocal(5, 4)
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (VSGAX, the row clicked)", f.SelectedIndex)
	}
}

// TestDialog_ComboMouseClick_WithFieldError_LastOptionClickable guards the
// old failure mode where the bottom visible option was hit-tested as the
// error row and became dead.
func TestDialog_ComboMouseClick_WithFieldError_LastOptionClickable(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 0)
	d.AddTextField("Shares", "", "", 10)
	f.Error = "Select a security"

	// Row 6 = AAPL (last option). Clicking it must commit index 2.
	d.HandleMouseLocal(5, 6)
	if f.SelectedIndex != 2 {
		t.Errorf("SelectedIndex = %d, want 2 (AAPL, last option)", f.SelectedIndex)
	}
}

// TestDialog_ComboMouseClick_ErrorLineInert verifies clicking the trailing
// error text focuses the combo without committing any option.
func TestDialog_ComboMouseClick_ErrorLineInert(t *testing.T) {
	d := NewDialog("Buy Securities")
	d.AddTextField("Shares", "", "", 10)
	f := d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 1)
	d.SetFocusIndex(1) // focus the combo (field index 1)
	f.Error = "Select a security"

	// Field layout: row 2 blank, row 3 Shares(text), row 4 blank,
	// row 5 combo header, rows 6/7/8 panel, row 9 error line.
	action := d.HandleMouseLocal(5, 9)
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone (error line inert)", action)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (unchanged)", f.SelectedIndex)
	}
}

// TestDialog_HandleMouse_WheelOnCombo verifies the scroll wheel moves the
// dropdown highlight while a combo is focused.
func TestDialog_HandleMouse_WheelOnCombo(t *testing.T) {
	d := NewDialog("Buy Securities")
	f := d.AddComboField("Security", []string{"VSGAX", "VSIAX", "AAPL"}, 0)
	d.SetFocusIndex(0)
	d.SetVisible(true)
	screenW, screenH := 80, 24

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	d.HandleMouse(tea.MouseWheelMsg{X: startCol + 10, Y: startRow + 5, Button: tea.MouseWheelDown}, screenW, screenH)
	if f.ComboHighlight != 1 {
		t.Errorf("ComboHighlight after wheel down = %d, want 1", f.ComboHighlight)
	}
	d.HandleMouse(tea.MouseWheelMsg{X: startCol + 10, Y: startRow + 5, Button: tea.MouseWheelUp}, screenW, screenH)
	if f.ComboHighlight != 0 {
		t.Errorf("ComboHighlight after wheel up = %d, want 0", f.ComboHighlight)
	}
}
