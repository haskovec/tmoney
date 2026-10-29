package widget

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// A colored cell does not change the widths: the row with a color is the
// row with no color once the escape codes are gone, and it is as wide.
func TestTable_SetCellColor_KeepsTheWidths(t *testing.T) {
	cols := []Column{
		{Header: "Class", Width: 10},
		{Header: "Value", Width: 8, Align: AlignRight},
		{Header: ""},
	}
	rows := [][]string{{"Mid Cap", "45.00", "████▌"}, {"Cash", "17.00", "█▏"}}
	plain := NewTable(cols)
	plain.SetRows(rows)
	colored := NewTable(cols)
	colored.SetRows(rows)
	colored.SetCellColor(0, 2, color.RGBA{R: 0x30, G: 0x7c, B: 0xd7, A: 0xff})
	colored.SetCellColor(1, 2, color.RGBA{R: 0xc8, G: 0x8f, B: 0x00, A: 0xff})

	styles := NewStyles()
	for _, width := range []int{30, 22, 15} {
		want := plain.Render(styles, width, 4)
		got := colored.Render(styles, width, 4)
		if StripAnsi(got) != StripAnsi(want) {
			t.Errorf("width %d: text differs:\n%q\nwant\n%q", width, StripAnsi(got), StripAnsi(want))
		}
		for i, line := range strings.Split(got, "\n") {
			if w := lipgloss.Width(line); w != width {
				t.Errorf("width %d: line %d is %d cells wide", width, i, w)
			}
		}
	}
}

// The selected row keeps the cell's color over its own colors, and SetRows
// clears the colors.
func TestTable_SetCellColor_OnTheSelectedRow(t *testing.T) {
	tbl := NewTable([]Column{{Header: "Class", Width: 10}, {Header: ""}})
	tbl.SetRows([][]string{{"Mid Cap", "████"}})
	tbl.SetCellColor(0, 1, color.RGBA{R: 0x30, G: 0x7c, B: 0xd7, A: 0xff})
	const fg = "38;2;48;124;215"

	row := strings.Split(tbl.Render(NewStyles(), 20, 3), "\n")[2]
	if !strings.Contains(row, fg) {
		t.Errorf("selected row has no cell color: %q", row)
	}
	if !strings.Contains(row, "48;") {
		t.Errorf("selected row lost its background: %q", row)
	}

	tbl.SetRows([][]string{{"Mid Cap", "████"}})
	if row := strings.Split(tbl.Render(NewStyles(), 20, 3), "\n")[2]; strings.Contains(row, fg) {
		t.Errorf("SetRows kept the cell color: %q", row)
	}
}
