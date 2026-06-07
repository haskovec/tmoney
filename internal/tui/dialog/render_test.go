package dialog

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Render output, label width, content/rendered height, dialog bounds.

func TestDialog_Render_NonEmpty(t *testing.T) {
	d := NewDialog("Test Dialog")
	d.AddTextField("Name", "John", "", 0)
	d.AddCheckboxField("Active", true)
	styles := widget.NewStyles()

	result := d.Render(styles)
	if result == "" {
		t.Fatal("Render() returned empty string")
	}
}

func TestDialog_Render_ContainsTitle(t *testing.T) {
	d := NewDialog("My Title")
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "My Title") {
		t.Error("Render() should contain the title")
	}
}

func TestDialog_Render_ContainsFieldLabels(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Username", "", "", 0)
	d.AddSelectField("Role", []string{"Admin", "User"}, 0)
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "[x]") {
		t.Error("Render() should contain close button [x]")
	}
}

func TestDialog_Render_ContainsCheckboxState(t *testing.T) {
	d := NewDialog("Test")
	d.AddCheckboxField("Enabled", true)
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "[x]") {
		t.Error("Render() should contain checked checkbox [x]")
	}
}

func TestDialog_Render_ContainsRadioOptions(t *testing.T) {
	d := NewDialog("Test")
	d.AddRadioField("Status", []string{"Pending", "Done"}, 1)
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Checking") {
		t.Error("Render() should contain selected option 'Checking'")
	}
}

func TestDialog_Render_EmptyDialog(t *testing.T) {
	d := NewDialog("Empty")
	d.SetButtons(nil)
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Enter name") {
		t.Error("Render() should show placeholder when value is empty and field is unfocused")
	}
}

func TestDialog_Render_TextFieldWithValue(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Name", "Alice", "", 0)
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Alice") {
		t.Error("Render() should show the field value")
	}
}

func TestDialog_Render_ContainsSeparators(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	styles := widget.NewStyles()

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

// widget.OverlayCenter tests

func TestDialog_Render_RequiredMarker(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "Enter name", 0)
	f.Required = true
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "*") {
		t.Error("Render() should contain '*' for required field")
	}
}

func TestDialog_Render_FieldError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", "", "Enter name", 0)
	f.Error = "Name is required"
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Name is required") {
		t.Error("Render() should show field error message")
	}
}

func TestDialog_Render_DialogErrorMsg(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A", "", "", 0)
	d.SetErrorMsg("Cross-field error")
	styles := widget.NewStyles()

	result := d.Render(styles)
	if !strings.Contains(result, "Cross-field error") {
		t.Error("Render() should show dialog-level error message")
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

func TestDialog_Render_CheckboxFieldError(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddCheckboxField("Accept", false)
	f.Error = "Must accept"
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

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
	styles := widget.NewStyles()

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

func TestDialog_Render_ListFieldShowsItems(t *testing.T) {
	d := NewDialog("Test")
	d.SetWidth(60)
	d.AddListField("File", []string{"../", "backups/", "default.tdb"}, 1, 10)
	d.SetFocusIndex(0)

	styles := widget.NewStyles()
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

// TestDialog_RenderedHeight_MatchesActualRender_WrappingMessage guards the
// mouse-geometry bug: a message line longer than the content width soft-wraps
// when lipgloss renders the dialog box, so RenderedHeight() must count those
// wrapped rows. If it under-counts, DialogBounds places the box's bottom above
// the real button row and clicks there are rejected (the reported "mouse does
// not work in the split dialog" symptom).
func TestDialog_RenderedHeight_MatchesActualRender_WrappingMessage(t *testing.T) {
	d := NewDialog("Stock Split")
	// A long single line that must wrap, plus normal lines.
	d.SetMessage("Ratio is N:M — N new shares for every M held. e.g. 2:1 = forward 2-for-1, 1:2 = halves shares.\n\nAfter split:\n  Wealthfront IRA: 656.09894 → 1312.19788 shares")
	d.AddTextField("Date", "06/18/2008", "", 0)
	d.AddTextField("Ratio", "2:1", "", 0)
	d.SetVisible(true)

	for _, maxH := range []int{0, 12, 18, 40} {
		d.SetMaxHeight(maxH)
		rendered := d.Render(widget.NewStyles())
		if got, want := lipgloss.Height(rendered), d.RenderedHeight(); got != want {
			t.Errorf("maxHeight=%d: actual rendered height %d != RenderedHeight() %d", maxH, got, want)
		}
	}
}
