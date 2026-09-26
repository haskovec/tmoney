package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// A focused text field whose value is longer than the field shows a window of
// the value around the cursor. It must never draw wider than the field, or the
// line wraps inside the dialog box.

const longValue = "Northwind Brokerage Individual" // 30 runes

// focusedText renders field as focused in a 10-column input and returns the
// plain text, "[ " + 10 cells + " ]".
func focusedText(d *Dialog, f *Field) string {
	const available = 10 + 4 // field width + "[ " and " ]"
	return ansi.Strip(d.renderTextFieldContent(widget.NewStyles(), f, true, available))
}

func newScrollField(value string) (*Dialog, *Field) {
	d := NewDialog("Test")
	f := d.AddTextField("Name", value, "", 10)
	return d, f
}

func TestTextField_FocusedLongValue_ShowsTailInWidth(t *testing.T) {
	d, f := newScrollField(longValue)

	got := focusedText(d, f)

	// Cursor at the end: the last 9 runes plus the cursor cell.
	if want := "[ ndividual  ]"; got != want {
		t.Errorf("focused text = %q, want %q", got, want)
	}
}

func TestTextField_FocusedLongValue_HomeShowsStart(t *testing.T) {
	d, f := newScrollField(longValue)
	focusedText(d, f)

	f.MoveCursorHome()

	if got, want := focusedText(d, f), "[ Northwind  ]"; got != want {
		t.Errorf("focused text = %q, want %q", got, want)
	}
}

// The window moves only when the cursor leaves it, so the text does not shift
// on every key press.
func TestTextField_WindowMovesOnlyAtEdge(t *testing.T) {
	d, f := newScrollField(longValue)
	focusedText(d, f)
	start := f.viewOffset // 30 + 1 cursor cell - 10 = 21
	if start != 21 {
		t.Fatalf("viewOffset = %d, want 21", start)
	}

	for range 9 { // cursor 30 -> 21, still inside the window
		f.MoveCursorLeft()
		focusedText(d, f)
	}
	if f.viewOffset != start {
		t.Errorf("viewOffset = %d after moving inside the window, want %d", f.viewOffset, start)
	}

	f.MoveCursorLeft() // cursor 20, one left of the window
	focusedText(d, f)
	if f.viewOffset != 20 {
		t.Errorf("viewOffset = %d after leaving the window, want 20", f.viewOffset)
	}
}

// Deleting from the end pulls the window back, so no empty cells open up on
// the right while text is hidden on the left.
func TestTextField_DeleteBackAtEndPullsWindowBack(t *testing.T) {
	d, f := newScrollField(longValue)
	focusedText(d, f)

	for range 5 {
		f.DeleteBack()
	}
	got := focusedText(d, f)

	if f.viewOffset != 16 { // 25 + 1 - 10
		t.Errorf("viewOffset = %d, want 16", f.viewOffset)
	}
	if want := "[ age Indiv  ]"; got != want {
		t.Errorf("focused text = %q, want %q", got, want)
	}
}

// The regression the Edit Account screen showed: a focused value longer than
// its field wrapped onto a second line.
func TestTextField_FocusedLongValue_DoesNotWrapDialog(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("A very long label for a field", "", "", 0)
	d.AddTextField("Name", strings.Repeat("x", 60), "", 0)
	d.SetFocusIndex(1)

	out := d.Render(widget.NewStyles())

	if got, want := lipgloss.Height(out), d.RenderedHeight(); got != want {
		t.Errorf("rendered height = %d, want %d (a line wrapped):\n%s", got, want, out)
	}
}

// clickTextColumn clicks column col of the first field's text area.
func clickTextColumn(d *Dialog, col int) {
	screenW, screenH := 80, 24
	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	textStart := d.maxLabelWidth() + 1 + 2 + 2 // label + colon + gap + "[ "
	d.HandleMouse(tea.MouseClickMsg{
		X:      startCol + 3 + textStart + col,
		Y:      startRow + 2 + 3, // content row 3 = first field row
		Button: tea.MouseLeft,
	}, screenW, screenH)
}

func TestTextField_ClickOnScrolledField_AddsWindowStart(t *testing.T) {
	d, f := newScrollField(longValue)
	d.SetVisible(true)
	d.SetFocusIndex(0)
	d.Render(widget.NewStyles()) // window starts at 21

	clickTextColumn(d, 2)

	if f.CursorPos() != 23 {
		t.Errorf("CursorPos() = %d, want 23", f.CursorPos())
	}
}

// An unfocused field draws the start of its value, so a click that focuses it
// maps from the start, whatever window it had before.
func TestTextField_ClickOnUnfocusedField_MapsFromStart(t *testing.T) {
	d, f := newScrollField(longValue)
	d.AddTextField("Other", "", "", 10)
	d.SetVisible(true)
	d.SetFocusIndex(0)
	d.Render(widget.NewStyles()) // window starts at 21
	d.SetFocusIndex(1)
	d.Render(widget.NewStyles())

	clickTextColumn(d, 2)

	if d.FocusIndex() != 0 {
		t.Fatalf("FocusIndex() = %d, want 0", d.FocusIndex())
	}
	if f.CursorPos() != 2 {
		t.Errorf("CursorPos() = %d, want 2", f.CursorPos())
	}
	if got, want := focusedText(d, f), "[ Northwind  ]"; got != want {
		t.Errorf("focused text after the click = %q, want %q", got, want)
	}
}
