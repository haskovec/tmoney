package dialog

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Long titles and errors: the drawn box, RenderedHeight and the hit-test must
// agree on every row, or a click lands on the row above or below.

// clickLabel renders d, finds label in the drawn box, and clicks it through
// HandleMouse. The position comes from the render, so the test fails when the
// drawn rows and the hit-test drift apart.
func clickLabel(t *testing.T, d *Dialog, label string, screenWidth, screenHeight int) DialogAction {
	t.Helper()
	rows := strings.Split(widget.StripAnsi(d.Render(widget.NewStyles())), "\n")
	startCol, startRow, _, _ := d.DialogBounds(screenWidth, screenHeight)
	for r, row := range rows {
		if before, _, ok := strings.Cut(row, label); ok {
			x := startCol + lipgloss.Width(before) + 1
			return d.HandleMouse(tea.MouseClickMsg{X: x, Y: startRow + r, Button: tea.MouseLeft}, screenWidth, screenHeight)
		}
	}
	t.Fatalf("%q not in the dialog:\n%s", label, strings.Join(rows, "\n"))
	return DialogActionNone
}

const (
	longLookupError = `Lookup failed: yahoo: Get "https://query1.example.com/v8/finance/chart/EXMPL": dial tcp: lookup query1.example.com: no such host`
	longFieldError  = "Before Contoso Everyday Checking opened (01/15/2024)"
)

func TestDialog_LongTitleAndErrors_DrawnRowsMatchHitTest(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Dialog
		want  string // text that must stay visible in full
	}{
		{"long title", func() *Dialog {
			d := NewDialog("Add Price — Contoso Stable Value Collective Trust Fund")
			d.AddTextField("Price", "", "", 0)
			return d
		}, ""},
		{"wide-character title", func() *Dialog {
			d := NewDialog("価格の追加 — " + strings.Repeat("東京", 20))
			d.AddTextField("Price", "", "", 0)
			return d
		}, ""},
		{"dialog error", func() *Dialog {
			d := NewDialog("Add Price")
			d.AddTextField("Price", "", "", 0)
			d.SetErrorMsg(longLookupError)
			return d
		}, "no such host"},
		{"text field error", func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 0).Error = longFieldError
			d.AddTextField("Amount", "", "", 0)
			return d
		}, "(01/15/2024)"},
		{"checkbox error", func() *Dialog {
			d := NewDialog("Options")
			d.AddCheckboxField("Active", true).Error = longFieldError
			return d
		}, "(01/15/2024)"},
		{"list field error", func() *Dialog {
			d := NewDialog("Pick")
			d.AddListField("Account", []string{"One", "Two", "Three"}, 0, 3).Error = longFieldError
			return d
		}, "(01/15/2024)"},
		{"scrolling with errors", func() *Dialog {
			d := NewDialog("Many Fields")
			for i := range 12 {
				f := d.AddTextField(fmt.Sprintf("Field %d", i), "", "", 0)
				if i == 0 {
					f.Error = longFieldError
				}
			}
			d.SetErrorMsg(longLookupError)
			d.SetMaxHeight(20)
			return d
		}, "no such host"},
	}
	for _, tt := range tests {
		for _, screenHeight := range []int{40, 41} {
			t.Run(fmt.Sprintf("%s/height %d", tt.name, screenHeight), func(t *testing.T) {
				d := tt.build()
				out := d.Render(widget.NewStyles())
				if got, want := lipgloss.Height(out), d.RenderedHeight(); got != want {
					t.Errorf("drawn height = %d, RenderedHeight() = %d", got, want)
				}
				if d.maxHeight > 0 && lipgloss.Height(out) > d.maxHeight {
					t.Errorf("drawn height = %d, want at most the bound %d", lipgloss.Height(out), d.maxHeight)
				}
				rows := strings.Split(widget.StripAnsi(out), "\n")
				if !strings.Contains(rows[2], "[x]") {
					t.Errorf("title row = %q, want [x] on it", rows[2])
				}
				// Words of a wrapped error sit on rows between border runes.
				text := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(rows, " "), "│", " ")), " ")
				if tt.want != "" && !strings.Contains(text, tt.want) {
					t.Errorf("dialog lost %q:\n%s", tt.want, strings.Join(rows, "\n"))
				}

				if got := clickLabel(t, d, "[x]", 100, screenHeight); got != DialogActionCancel {
					t.Errorf("click on [x] = %v, want DialogActionCancel", got)
				}
				if got := clickLabel(t, tt.build(), "Save", 100, screenHeight); got != DialogActionSubmit {
					t.Errorf("click on Save = %v, want DialogActionSubmit", got)
				}
				if got := clickLabel(t, tt.build(), "Cancel", 100, screenHeight); got != DialogActionCancel {
					t.Errorf("click on Cancel = %v, want DialogActionCancel", got)
				}
			})
		}
	}
}
