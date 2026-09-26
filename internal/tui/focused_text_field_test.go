package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
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
