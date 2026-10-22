package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// The split editor and the paycheck wizard draw their own rows and keep their
// own hit math. Their inline error must take the rows that math counts, or a
// click on [ Save ] or [ Cancel ] lands two rows off.

// findLabel returns the row and the cell column of label in a rendered box.
func findLabel(t *testing.T, rendered, label string) (row, col int) {
	t.Helper()
	rows := strings.Split(widget.StripAnsi(rendered), "\n")
	for r, line := range rows {
		if before, _, ok := strings.Cut(line, label); ok {
			return r, lipgloss.Width(before)
		}
	}
	t.Fatalf("%q not drawn:\n%s", label, strings.Join(rows, "\n"))
	return 0, 0
}

var inlineErrors = []string{
	"split 1: category is required",
	"Next payday: invalid date (MM/DD/YYYY). " + strings.Repeat("A long validation message wraps. ", 3),
}

func TestSplitDialog_ClickButtonsAfterError(t *testing.T) {
	for i, msg := range inlineErrors {
		t.Run(fmt.Sprintf("error %d", i), func(t *testing.T) {
			sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})
			sd.errorMsg = msg
			rendered := sd.Render(widget.NewStyles())

			// Border (1) + padding (1 row, 2 columns) sit outside the local space.
			row, col := findLabel(t, rendered, "Cancel")
			if got := sd.HandleMouseLocal(col+1-3, row-2); got != dialog.DialogActionCancel {
				t.Errorf("click on the drawn Cancel = %v, want DialogActionCancel", got)
			}
			words := strings.Fields(msg)
			if last := words[len(words)-1]; !strings.Contains(widget.StripAnsi(rendered), last) {
				t.Errorf("the end of the error (%q) is not drawn:\n%s", last, widget.StripAnsi(rendered))
			}
		})
	}
}

func TestPaycheckWizard_ClickButtonsAfterError(t *testing.T) {
	for i, msg := range inlineErrors {
		for _, label := range []string{"[ Cancel ]", "[ Save ]"} {
			t.Run(fmt.Sprintf("error %d/%s", i, label), func(t *testing.T) {
				fx := newPaycheckWizardFixture()
				w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
				w.errorMsg = msg
				styles := widget.NewStyles()
				rendered := w.Render(styles) // records the hit zones

				const screenWidth, screenHeight = 120, 70
				startCol, startRow, _, _ := w.dialogBounds(styles, screenWidth, screenHeight)
				row, col := findLabel(t, rendered, label)
				click := tea.MouseClickMsg{X: startCol + col + 2, Y: startRow + row, Button: tea.MouseLeft}
				want := dialog.DialogActionCancel
				if label == "[ Save ]" {
					want = dialog.DialogActionSubmit
				}
				if got := w.HandleMouse(click, styles, screenWidth, screenHeight); got != want {
					t.Errorf("click on the drawn %s = %v, want %v", label, got, want)
				}
			})
		}
	}
}
