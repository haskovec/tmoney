package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// The split dialog and the paycheck wizard draw their own text inputs. A
// focused value longer than the input must scroll inside it, not widen the
// line, the same as in a standard dialog.

func longFocusedField() *dialog.Field {
	d := dialog.NewDialog("Test")
	f := d.AddTextField("Memo", "Northwind Brokerage Individual", "", 0) // 30 runes, cursor at the end
	return f
}

func TestSplitDialog_FocusedLongText_StaysInWidth(t *testing.T) {
	sd := &SplitDialog{}

	got := ansi.Strip(sd.renderTextField(widget.NewStyles(), longFocusedField(), true, 10))

	if want := "[ ndividual  ]"; got != want {
		t.Errorf("focused text = %q, want %q", got, want)
	}
}

func TestPaycheckWizard_FocusedLongText_StaysInWidth(t *testing.T) {
	w := &PaycheckWizard{}

	got := ansi.Strip(w.renderFieldValue(widget.NewStyles(), lipgloss.NewStyle(), longFocusedField(), true, 14))

	if want := "[ ndividual  ]"; got != want {
		t.Errorf("focused text = %q, want %q", got, want)
	}
}

// The window start lives on the row's own field. Render must pass that field,
// not the field of a loop copy, or the start is lost after every frame and the
// text shifts on every key press.
func TestSplitDialog_FocusedLongText_WindowSurvivesRender(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.fieldFocus = splitFieldMemo
	memo := &sd.rows[0].memoField
	memo.Value = strings.Repeat("abcdefghij", 10)
	memo.MoveCursorEnd()
	styles := widget.NewStyles()

	before := ansi.Strip(sd.Render(styles))
	memo.MoveCursorLeft() // still inside the window
	after := ansi.Strip(sd.Render(styles))

	if before != after {
		t.Errorf("the text shifted on a cursor move inside the window:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
