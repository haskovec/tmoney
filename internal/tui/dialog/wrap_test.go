package dialog

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
		{"tabs and a line break in the title", func() *Dialog {
			d := NewDialog("Edit\tContoso\tStable\tValue\nFund\tPrice")
			d.AddTextField("Price", "", "", 0)
			return d
		}, ""},
		{"hyphen and negative amount in errors", func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 0).Error = "Before Woodgrove Bank Everyday Checking - Joint opened (01/15/2024)"
			d.SetErrorMsg("Lookup failed: bad response from the quote server - status 503; balance -5 now")
			return d
		}, "status 503; balance -5 now"},
		{"CRLF and tabs in errors", func() *Dialog {
			d := NewDialog("Add Price")
			d.AddTextField("Price", "", "", 0).Error = "bad\tvalue\tin\tthe\tfield,\tsee\tthe\tlog"
			d.SetErrorMsg("Lookup failed: HTTP 500\r\nInternal Server Error\rretry later")
			return d
		}, "retry later"},
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
				if w := lipgloss.Width(out); w != d.width {
					t.Errorf("drawn width = %d, want the dialog width %d", w, d.width)
				}
				if strings.ContainsAny(out, "\r\t") {
					t.Errorf("a CR or a tab reached the terminal: %q", out)
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

// A wrap breaks at spaces only: a break at a hyphen would split a negative
// amount ("-" / "12.50"). No row is wider than the width, and an indent stays
// on the continuation rows.
func TestWrapRows(t *testing.T) {
	text := "Before Woodgrove Bank Everyday Checking - Joint: payment -12.50 is due, balance -5"
	for width := 5; width <= 60; width++ {
		rows := WrapRows(text, width)
		for i, row := range rows {
			if w := lipgloss.Width(row); w > width {
				t.Errorf("width %d: row %d %q is %d cells", width, i, row, w)
			}
			if strings.HasSuffix(row, "-") && i+1 < len(rows) && strings.HasPrefix(rows[i+1], "12.50") {
				t.Errorf("width %d: the amount -12.50 was split: %q", width, rows)
			}
		}
		if got := strings.ReplaceAll(strings.Join(rows, ""), " ", ""); got != strings.ReplaceAll(text, " ", "") {
			t.Errorf("width %d: characters lost: %q", width, rows)
		}
		// From width 10 every word fits a row, so no word is cut.
		if got := strings.Join(rows, " "); width >= 10 && strings.Join(strings.Fields(got), " ") != text {
			t.Errorf("width %d: a word was cut: %q", width, rows)
		}
	}

	rows := WrapRows("  Woodgrove Bank Everyday Checking: 10 shares", 20)
	for i, row := range rows {
		if !strings.HasPrefix(row, "  ") {
			t.Errorf("row %d %q lost the indent", i, row)
		}
	}
	if got := WrapRows("a\r\nb\rc\td", 10); strings.Join(got, "|") != "a|b|c d" {
		t.Errorf("WrapRows(CR, tab) = %q, want [a b \"c d\"]", got)
	}
}

// In scroll mode the message is in the scrolling body: the body rows and the
// hit-test must count its wrapped rows the same way.
func TestDialog_ScrollingLongMessage_ClickFocusesField(t *testing.T) {
	d := NewDialog("Edit Transfer")
	d.SetMessage("Woodgrove Bank Everyday Checking → Fabrikam High Yield Savings Account, joint")
	for _, label := range []string{"Amount", "Date", "Memo", "Notes", "Ref", "Tag"} {
		d.AddTextField(label, "", "", 0)
	}
	d.SetMaxHeight(18)
	if !d.isScrolling() {
		t.Fatal("the dialog should scroll at height 18")
	}
	if clickLabel(t, d, "Memo:", 100, 24); d.FocusedField() == nil || d.FocusedField().Label != "Memo" {
		t.Errorf("click on Memo: focused %v, want Memo", d.FocusedField())
	}
}

// A row with room for 1 cell must not stop the wrap: a zero-width character
// or a bare escape code once made a step that took nothing, and the loop
// never ended. A character wider than the row gets a row of its own.
func TestWrapRows_NarrowRoomEnds(t *testing.T) {
	tests := []struct {
		text  string
		width int
	}{
		{"\u200b東京", 1},
		{"\x1b[31m東京\x1b[m", 1},
		{"  \u200b東 x", 3},
		{"東京", 1},
		{" 東京", 2},
	}
	for _, tt := range tests {
		done := make(chan []string, 1)
		go func() { done <- WrapRows(tt.text, tt.width) }()
		select {
		case rows := <-done:
			for _, row := range rows {
				// One wide character may pass a 1-cell room by one cell.
				if w := lipgloss.Width(row); w > tt.width+1 {
					t.Errorf("WrapRows(%q, %d): row %q is %d cells", tt.text, tt.width, row, w)
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("WrapRows(%q, %d) does not return", tt.text, tt.width)
		}
	}
}
