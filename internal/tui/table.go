package tui

import (
	"fmt"
	"strings"
)

// ColumnAlign specifies the text alignment within a table column.
type ColumnAlign int

const (
	// AlignLeft aligns text to the left (default).
	AlignLeft ColumnAlign = iota
	// AlignRight aligns text to the right (useful for amounts).
	AlignRight
	// AlignCenter aligns text to the center.
	AlignCenter
)

// Column defines a table column's properties.
type Column struct {
	// Header is the column header text.
	Header string
	// Width is the fixed width of the column in characters.
	// If 0, the column will flex to fill remaining space.
	Width int
	// MinWidth is the minimum width for flex columns.
	MinWidth int
	// Align specifies the text alignment within the column.
	Align ColumnAlign
}

// RowStyle identifies a named row style variant for custom rendering.
type RowStyle int

const (
	// RowStyleDefault is the normal row style.
	RowStyleDefault RowStyle = iota
	// RowStyleVoid is a dimmed style for void/inactive rows.
	RowStyleVoid
)

// Table is a generic table component with row selection and scrolling.
type Table struct {
	// Column definitions
	columns []Column

	// Row data (each row is a slice of cell strings, one per column)
	rows [][]string

	// Row style overrides (row index -> RowStyle)
	rowStyles map[int]RowStyle

	// Navigation state
	cursor       int
	scrollOffset int

	// Focus state
	focused bool
}

// NewTable creates a new Table with the given column definitions.
func NewTable(columns []Column) *Table {
	return &Table{
		columns: columns,
		focused: true,
	}
}

// SetRowStyle sets the style variant for a specific row.
func (t *Table) SetRowStyle(rowIndex int, style RowStyle) {
	if t.rowStyles == nil {
		t.rowStyles = make(map[int]RowStyle)
	}
	t.rowStyles[rowIndex] = style
}

// ClearRowStyles removes all row style overrides.
func (t *Table) ClearRowStyles() {
	t.rowStyles = nil
}

// SetRows replaces all row data. Resets scroll offset, clamps cursor, and clears row styles.
func (t *Table) SetRows(rows [][]string) {
	t.rows = rows
	t.rowStyles = nil
	t.clampCursor()
	t.clampScroll(0)
}

// Columns returns the current column definitions.
func (t *Table) Columns() []Column {
	return t.columns
}

// SetColumns replaces the column definitions.
func (t *Table) SetColumns(columns []Column) {
	t.columns = columns
}

// RowCount returns the number of rows.
func (t *Table) RowCount() int {
	return len(t.rows)
}

// Cursor returns the current cursor position.
func (t *Table) Cursor() int {
	return t.cursor
}

// SetCursor sets the cursor to the given position.
func (t *Table) SetCursor(pos int) {
	t.cursor = pos
	t.clampCursor()
}

// SelectedRow returns the data for the currently selected row, or nil if empty.
func (t *Table) SelectedRow() []string {
	if len(t.rows) == 0 || t.cursor < 0 || t.cursor >= len(t.rows) {
		return nil
	}
	return t.rows[t.cursor]
}

// HitTest determines which data row was clicked at row y within the rendered table.
// y is relative to the top of the table output (0-based), where row 0 is the header.
// Returns the data row index, or -1 if the click is on the header or out of range.
func (t *Table) HitTest(y int) int {
	if y <= 0 || len(t.rows) == 0 {
		return -1
	}
	dataRow := t.scrollOffset + (y - 1)
	if dataRow >= 0 && dataRow < len(t.rows) {
		return dataRow
	}
	return -1
}

// SetFocused sets whether the table has input focus.
func (t *Table) SetFocused(focused bool) {
	t.focused = focused
}

// IsFocused returns whether the table has input focus.
func (t *Table) IsFocused() bool {
	return t.focused
}

// MoveUp moves the cursor up one row.
func (t *Table) MoveUp() {
	if t.cursor > 0 {
		t.cursor--
	}
}

// MoveDown moves the cursor down one row.
func (t *Table) MoveDown() {
	if t.cursor < len(t.rows)-1 {
		t.cursor++
	}
}

// MoveToTop moves the cursor to the first row.
func (t *Table) MoveToTop() {
	t.cursor = 0
}

// MoveToBottom moves the cursor to the last row.
func (t *Table) MoveToBottom() {
	if len(t.rows) > 0 {
		t.cursor = len(t.rows) - 1
	}
}

// PageUp moves the cursor up by one page (viewport height).
func (t *Table) PageUp(viewportHeight int) {
	t.cursor -= viewportHeight
	t.clampCursor()
}

// PageDown moves the cursor down by one page (viewport height).
func (t *Table) PageDown(viewportHeight int) {
	t.cursor += viewportHeight
	t.clampCursor()
}

// Render renders the table for the given dimensions using the provided styles.
func (t *Table) Render(styles Styles, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	colWidths := t.computeColumnWidths(width)

	var lines []string

	// Render header (BorderBottom adds a border line, so header is 2 visual lines)
	headerLine := t.renderHeader(styles, colWidths, width)
	lines = append(lines, headerLine)

	// Available height for data rows (subtract header + border line)
	rowHeight := max(height-2, 0)

	// Adjust scroll offset to keep cursor visible
	t.clampScroll(rowHeight)

	// Render visible rows
	endRow := min(t.scrollOffset+rowHeight, len(t.rows))

	for i := t.scrollOffset; i < endRow; i++ {
		line := t.renderRow(styles, i, colWidths, width)
		lines = append(lines, line)
	}

	// Pad with empty lines if fewer rows than available height
	rendered := len(lines) - 1 // subtract header
	for rendered < rowHeight {
		lines = append(lines, strings.Repeat(" ", width))
		rendered++
	}

	return strings.Join(lines, "\n")
}

// computeColumnWidths calculates the actual width of each column.
// Fixed-width columns use their Width value; flex columns (Width == 0)
// share remaining space equally.
func (t *Table) computeColumnWidths(totalWidth int) []int {
	if len(t.columns) == 0 {
		return nil
	}

	widths := make([]int, len(t.columns))

	// Account for column separators: one space between each column
	separatorWidth := len(t.columns) - 1
	available := max(totalWidth-separatorWidth, 0)

	// First pass: assign fixed widths
	fixedTotal := 0
	flexCount := 0
	for i, col := range t.columns {
		if col.Width > 0 {
			widths[i] = col.Width
			fixedTotal += col.Width
		} else {
			flexCount++
		}
	}

	// Second pass: distribute remaining space to flex columns
	remaining := max(available-fixedTotal, 0)

	if flexCount > 0 {
		flexWidth := remaining / flexCount
		extraPixels := remaining % flexCount
		for i, col := range t.columns {
			if col.Width == 0 {
				w := flexWidth
				if extraPixels > 0 {
					w++
					extraPixels--
				}
				// Apply minimum width
				if col.MinWidth > 0 && w < col.MinWidth {
					w = col.MinWidth
				}
				widths[i] = w
			}
		}
	}

	return widths
}

// renderHeader renders the table header row.
func (t *Table) renderHeader(styles Styles, colWidths []int, totalWidth int) string {
	cells := make([]string, len(t.columns))
	for i, col := range t.columns {
		w := 0
		if i < len(colWidths) {
			w = colWidths[i]
		}
		cells[i] = alignText(col.Header, w, col.Align)
	}

	line := strings.Join(cells, " ")

	// Pad or truncate to total width
	line = padOrTruncate(line, totalWidth)

	return styles.TableHeader.Render(line)
}

// renderRow renders a single data row.
func (t *Table) renderRow(styles Styles, rowIndex int, colWidths []int, totalWidth int) string {
	row := t.rows[rowIndex]

	cells := make([]string, len(t.columns))
	for i, col := range t.columns {
		w := 0
		if i < len(colWidths) {
			w = colWidths[i]
		}
		cellValue := ""
		if i < len(row) {
			cellValue = row[i]
		}
		cells[i] = alignText(cellValue, w, col.Align)
	}

	line := strings.Join(cells, " ")

	// Pad or truncate to total width
	line = padOrTruncate(line, totalWidth)

	isCursor := t.focused && rowIndex == t.cursor
	if isCursor {
		return styles.SelectedRow.Render(line)
	}
	if rs, ok := t.rowStyles[rowIndex]; ok && rs == RowStyleVoid {
		return styles.VoidRow.Render(line)
	}
	return styles.TableRow.Render(line)
}

// clampCursor ensures the cursor is within valid bounds.
func (t *Table) clampCursor() {
	if len(t.rows) == 0 {
		t.cursor = 0
		return
	}
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

// clampScroll adjusts the scroll offset to keep the cursor visible
// within the viewport of the given height.
func (t *Table) clampScroll(viewportHeight int) {
	if viewportHeight <= 0 || len(t.rows) == 0 {
		t.scrollOffset = 0
		return
	}

	// Scroll up if cursor is above the viewport
	if t.cursor < t.scrollOffset {
		t.scrollOffset = t.cursor
	}

	// Scroll down if cursor is below the viewport
	if t.cursor >= t.scrollOffset+viewportHeight {
		t.scrollOffset = t.cursor - viewportHeight + 1
	}

	// Don't scroll past the end
	maxOffset := max(len(t.rows)-viewportHeight, 0)
	if t.scrollOffset > maxOffset {
		t.scrollOffset = maxOffset
	}
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
}

// ScrollInfo returns a scroll position string like "1-20 of 50" when rows
// exceed the last rendered viewport height. Returns empty string when all
// rows fit or the table is empty.
func (t *Table) ScrollInfo(viewportHeight int) string {
	if len(t.rows) == 0 || viewportHeight <= 0 || len(t.rows) <= viewportHeight {
		return ""
	}
	start := t.scrollOffset + 1
	end := min(t.scrollOffset+viewportHeight, len(t.rows))
	return fmt.Sprintf("%d-%d of %d", start, end, len(t.rows))
}

// alignText aligns text within a given width.
func alignText(text string, width int, align ColumnAlign) string {
	if width <= 0 {
		return ""
	}

	// Truncate if too long
	runes := []rune(text)
	if len(runes) > width {
		if width > 1 {
			return string(runes[:width-1]) + "…"
		}
		return string(runes[:width])
	}

	padding := width - len(runes)
	switch align {
	case AlignRight:
		return strings.Repeat(" ", padding) + text
	case AlignCenter:
		left := padding / 2
		right := padding - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	default: // AlignLeft
		return text + strings.Repeat(" ", padding)
	}
}

// padOrTruncate ensures a string is exactly the given width.
func padOrTruncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		if width > 1 {
			return string(runes[:width-1]) + "…"
		}
		if width > 0 {
			return string(runes[:width])
		}
		return ""
	}
	if len(runes) < width {
		return s + strings.Repeat(" ", width-len(runes))
	}
	return s
}
