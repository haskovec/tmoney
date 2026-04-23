package tui

import (
	"strings"
	"testing"
)

func TestNewTable(t *testing.T) {
	cols := []Column{
		{Header: "Name", Width: 10},
		{Header: "Amount", Width: 8, Align: AlignRight},
	}

	tbl := NewTable(cols)
	if tbl == nil {
		t.Fatal("NewTable() returned nil")
	}
	if !tbl.focused {
		t.Error("table should be focused by default")
	}
	if len(tbl.columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(tbl.columns))
	}
	if tbl.cursor != 0 {
		t.Error("cursor should start at 0")
	}
	if tbl.RowCount() != 0 {
		t.Error("row count should start at 0")
	}
}

func TestTable_SetRows(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	rows := [][]string{
		{"one"},
		{"two"},
		{"three"},
	}
	tbl.SetRows(rows)

	if tbl.RowCount() != 3 {
		t.Errorf("expected 3 rows, got %d", tbl.RowCount())
	}
}

func TestTable_SetRows_ResetsCursor(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	tbl.SetRows([][]string{{"a"}, {"b"}, {"c"}})
	tbl.cursor = 2

	// Setting new rows should clamp cursor
	tbl.SetRows([][]string{{"x"}})
	if tbl.cursor != 0 {
		t.Errorf("cursor should clamp to 0 after SetRows with fewer rows, got %d", tbl.cursor)
	}
}

func TestTable_MoveUpDown(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"a"}, {"b"}, {"c"}})

	if tbl.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", tbl.cursor)
	}

	tbl.MoveDown()
	if tbl.cursor != 1 {
		t.Errorf("after MoveDown, cursor = %d, want 1", tbl.cursor)
	}

	tbl.MoveDown()
	if tbl.cursor != 2 {
		t.Errorf("after 2x MoveDown, cursor = %d, want 2", tbl.cursor)
	}

	// Should not go past last row
	tbl.MoveDown()
	if tbl.cursor != 2 {
		t.Errorf("cursor should stay at 2 (last row), got %d", tbl.cursor)
	}

	tbl.MoveUp()
	if tbl.cursor != 1 {
		t.Errorf("after MoveUp, cursor = %d, want 1", tbl.cursor)
	}

	tbl.MoveUp()
	if tbl.cursor != 0 {
		t.Errorf("after 2x MoveUp, cursor = %d, want 0", tbl.cursor)
	}

	// Should not go before first row
	tbl.MoveUp()
	if tbl.cursor != 0 {
		t.Errorf("cursor should stay at 0 (first row), got %d", tbl.cursor)
	}
}

func TestTable_MoveUpDown_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	tbl.MoveDown()
	if tbl.cursor != 0 {
		t.Errorf("MoveDown on empty table: cursor = %d, want 0", tbl.cursor)
	}

	tbl.MoveUp()
	if tbl.cursor != 0 {
		t.Errorf("MoveUp on empty table: cursor = %d, want 0", tbl.cursor)
	}
}

func TestTable_MoveToTopBottom(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}})

	tbl.MoveToBottom()
	if tbl.cursor != 4 {
		t.Errorf("MoveToBottom: cursor = %d, want 4", tbl.cursor)
	}

	tbl.MoveToTop()
	if tbl.cursor != 0 {
		t.Errorf("MoveToTop: cursor = %d, want 0", tbl.cursor)
	}
}

func TestTable_MoveToBottom_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	tbl.MoveToBottom()
	if tbl.cursor != 0 {
		t.Errorf("MoveToBottom on empty table: cursor = %d, want 0", tbl.cursor)
	}
}

func TestTable_PageUpDown(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	tbl.PageDown(5)
	if tbl.cursor != 5 {
		t.Errorf("after PageDown(5), cursor = %d, want 5", tbl.cursor)
	}

	tbl.PageDown(5)
	if tbl.cursor != 10 {
		t.Errorf("after 2x PageDown(5), cursor = %d, want 10", tbl.cursor)
	}

	tbl.PageUp(5)
	if tbl.cursor != 5 {
		t.Errorf("after PageUp(5), cursor = %d, want 5", tbl.cursor)
	}

	// Page up past beginning should clamp to 0
	tbl.PageUp(10)
	if tbl.cursor != 0 {
		t.Errorf("PageUp past beginning: cursor = %d, want 0", tbl.cursor)
	}

	// Page down past end should clamp to last row
	tbl.cursor = 15
	tbl.PageDown(10)
	if tbl.cursor != 19 {
		t.Errorf("PageDown past end: cursor = %d, want 19", tbl.cursor)
	}
}

func TestTable_SelectedRow(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	// Empty table
	if tbl.SelectedRow() != nil {
		t.Error("SelectedRow() should be nil for empty table")
	}

	tbl.SetRows([][]string{{"a"}, {"b"}, {"c"}})

	row := tbl.SelectedRow()
	if row == nil {
		t.Fatal("SelectedRow() should not be nil")
	}
	if row[0] != "a" {
		t.Errorf("SelectedRow()[0] = %q, want %q", row[0], "a")
	}

	tbl.MoveDown()
	row = tbl.SelectedRow()
	if row[0] != "b" {
		t.Errorf("after MoveDown, SelectedRow()[0] = %q, want %q", row[0], "b")
	}
}

func TestTable_SetCursor(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"a"}, {"b"}, {"c"}})

	tbl.SetCursor(2)
	if tbl.cursor != 2 {
		t.Errorf("SetCursor(2): cursor = %d, want 2", tbl.cursor)
	}

	// Clamps to valid range
	tbl.SetCursor(10)
	if tbl.cursor != 2 {
		t.Errorf("SetCursor(10): cursor = %d, want 2 (clamped)", tbl.cursor)
	}

	tbl.SetCursor(-1)
	if tbl.cursor != 0 {
		t.Errorf("SetCursor(-1): cursor = %d, want 0 (clamped)", tbl.cursor)
	}
}

func TestTable_Focus(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	if !tbl.IsFocused() {
		t.Error("table should be focused by default")
	}

	tbl.SetFocused(false)
	if tbl.IsFocused() {
		t.Error("table should not be focused after SetFocused(false)")
	}

	tbl.SetFocused(true)
	if !tbl.IsFocused() {
		t.Error("table should be focused after SetFocused(true)")
	}
}

func TestTable_SetColumns(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	if len(tbl.columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(tbl.columns))
	}

	tbl.SetColumns([]Column{
		{Header: "X", Width: 10},
		{Header: "Y", Width: 20},
	})
	if len(tbl.columns) != 2 {
		t.Errorf("expected 2 columns after SetColumns, got %d", len(tbl.columns))
	}
}

func TestTable_ComputeColumnWidths_FixedOnly(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A", Width: 10},
		{Header: "B", Width: 20},
	})

	widths := tbl.computeColumnWidths(40)

	if widths[0] != 10 {
		t.Errorf("col 0 width = %d, want 10", widths[0])
	}
	if widths[1] != 20 {
		t.Errorf("col 1 width = %d, want 20", widths[1])
	}
}

func TestTable_ComputeColumnWidths_WithFlex(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A", Width: 10},
		{Header: "B"},           // flex
		{Header: "C", Width: 8}, // fixed
	})

	// Total width 40, separators = 2, fixed = 18, remaining = 20
	widths := tbl.computeColumnWidths(40)

	if widths[0] != 10 {
		t.Errorf("col 0 width = %d, want 10", widths[0])
	}
	if widths[1] != 20 {
		t.Errorf("col 1 (flex) width = %d, want 20", widths[1])
	}
	if widths[2] != 8 {
		t.Errorf("col 2 width = %d, want 8", widths[2])
	}
}

func TestTable_ComputeColumnWidths_MultipleFlexColumns(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A"},
		{Header: "B"},
	})

	// Total width 21, separators = 1, remaining = 20
	widths := tbl.computeColumnWidths(21)

	// Each flex column gets 10
	if widths[0] != 10 {
		t.Errorf("col 0 width = %d, want 10", widths[0])
	}
	if widths[1] != 10 {
		t.Errorf("col 1 width = %d, want 10", widths[1])
	}
}

func TestTable_ComputeColumnWidths_MinWidth(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A", Width: 30},
		{Header: "B", MinWidth: 10}, // flex with min
	})

	// Total width 35, separator = 1, available = 34, fixed = 30, remaining = 4
	// Flex would get 4 but min is 10
	widths := tbl.computeColumnWidths(35)

	if widths[1] != 10 {
		t.Errorf("flex col with MinWidth: width = %d, want 10", widths[1])
	}
}

func TestTable_ComputeColumnWidths_NoColumns(t *testing.T) {
	tbl := NewTable(nil)

	widths := tbl.computeColumnWidths(40)
	if widths != nil {
		t.Errorf("expected nil widths for no columns, got %v", widths)
	}
}

func TestAlignText_Left(t *testing.T) {
	result := alignText("hello", 10, AlignLeft)
	if result != "hello     " {
		t.Errorf("alignText left = %q, want %q", result, "hello     ")
	}
}

func TestAlignText_Right(t *testing.T) {
	result := alignText("hello", 10, AlignRight)
	if result != "     hello" {
		t.Errorf("alignText right = %q, want %q", result, "     hello")
	}
}

func TestAlignText_Center(t *testing.T) {
	result := alignText("hi", 10, AlignCenter)
	if result != "    hi    " {
		t.Errorf("alignText center = %q, want %q", result, "    hi    ")
	}
}

func TestAlignText_Truncate(t *testing.T) {
	result := alignText("hello world", 5, AlignLeft)
	if result != "hell…" {
		t.Errorf("alignText truncate = %q, want %q", result, "hell…")
	}
}

func TestAlignText_ExactFit(t *testing.T) {
	result := alignText("hello", 5, AlignLeft)
	if result != "hello" {
		t.Errorf("alignText exact = %q, want %q", result, "hello")
	}
}

func TestAlignText_ZeroWidth(t *testing.T) {
	result := alignText("hello", 0, AlignLeft)
	if result != "" {
		t.Errorf("alignText zero width = %q, want empty", result)
	}
}

func TestAlignText_Width1Truncate(t *testing.T) {
	result := alignText("hello", 1, AlignLeft)
	if result != "h" {
		t.Errorf("alignText width 1 = %q, want %q", result, "h")
	}
}

func TestPadOrTruncate(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello world", 5, "hell…"},
		{"hi", 0, ""},
		{"", 5, "     "},
	}

	for _, tt := range tests {
		got := padOrTruncate(tt.input, tt.width)
		if got != tt.want {
			t.Errorf("padOrTruncate(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
		}
	}
}

func TestTable_Render_ZeroDimensions(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	if tbl.Render(NewStyles(), 0, 10) != "" {
		t.Error("Render() with width=0 should return empty string")
	}
	if tbl.Render(NewStyles(), 10, 0) != "" {
		t.Error("Render() with height=0 should return empty string")
	}
}

func TestTable_Render_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "Name", Width: 10},
		{Header: "Value", Width: 10},
	})
	styles := NewStyles()

	result := tbl.Render(styles, 21, 5)
	if result == "" {
		t.Error("Render() should not return empty string for empty table")
	}

	// Should contain header text
	if !strings.Contains(result, "Name") {
		t.Error("Render() should contain header 'Name'")
	}
	if !strings.Contains(result, "Value") {
		t.Error("Render() should contain header 'Value'")
	}
}

func TestTable_Render_WithRows(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "Name", Width: 10},
		{Header: "Amount", Width: 10, Align: AlignRight},
	})
	tbl.SetRows([][]string{
		{"Coffee", "5.00"},
		{"Groceries", "42.50"},
	})
	styles := NewStyles()

	result := tbl.Render(styles, 21, 5)

	if !strings.Contains(result, "Name") {
		t.Error("should contain header 'Name'")
	}
	if !strings.Contains(result, "Amount") {
		t.Error("should contain header 'Amount'")
	}
	if !strings.Contains(result, "Coffee") {
		t.Error("should contain row data 'Coffee'")
	}
	if !strings.Contains(result, "Groceries") {
		t.Error("should contain row data 'Groceries'")
	}
	if !strings.Contains(result, "5.00") {
		t.Error("should contain row data '5.00'")
	}
}

func TestTable_Render_RowCountMatchesHeight(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A", Width: 10},
	})
	tbl.SetRows([][]string{{"one"}, {"two"}})
	styles := NewStyles()

	// Height 5 = 1 header + 4 data rows (2 real + 2 padded)
	// Note: TableHeader style has BorderBottom which adds a line,
	// so actual output is header + border + data rows
	result := tbl.Render(styles, 10, 5)
	lines := strings.Split(result, "\n")

	// The header with border bottom adds 2 lines, plus 4 data rows = 6 lines
	if len(lines) < 5 {
		t.Errorf("expected at least 5 lines, got %d", len(lines))
	}
}

func TestTable_Scrolling(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	// Move cursor to row 10
	for range 10 {
		tbl.MoveDown()
	}

	// Render with height 6 (1 header + 5 data rows)
	styles := NewStyles()
	result := tbl.Render(styles, 10, 6)

	if result == "" {
		t.Error("Render() should not return empty string")
	}

	// Scroll offset should have adjusted
	if tbl.scrollOffset < 1 {
		t.Errorf("scrollOffset should be > 0 when cursor at 10 with viewport 5, got %d", tbl.scrollOffset)
	}
}

func TestTable_ScrollOffset_CursorAboveViewport(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	// Manually set scroll offset past cursor
	tbl.scrollOffset = 5
	tbl.cursor = 2

	tbl.clampScroll(5)

	if tbl.scrollOffset != 2 {
		t.Errorf("scrollOffset should adjust to show cursor at 2, got %d", tbl.scrollOffset)
	}
}

func TestTable_ScrollOffset_CursorBelowViewport(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	tbl.scrollOffset = 0
	tbl.cursor = 8

	tbl.clampScroll(5)

	// cursor 8 should be visible: offset should be at least 4 (8 - 5 + 1)
	if tbl.scrollOffset != 4 {
		t.Errorf("scrollOffset = %d, want 4", tbl.scrollOffset)
	}
}

func TestTable_ScrollOffset_MaxOffset(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 10)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	tbl.scrollOffset = 100
	tbl.cursor = 9

	tbl.clampScroll(5)

	// Max offset = 10 - 5 = 5
	if tbl.scrollOffset != 5 {
		t.Errorf("scrollOffset = %d, want 5 (max offset)", tbl.scrollOffset)
	}
}

func TestTable_Render_MissingCellData(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "A", Width: 5},
		{Header: "B", Width: 5},
		{Header: "C", Width: 5},
	})
	// Row has fewer cells than columns
	tbl.SetRows([][]string{
		{"only one"},
	})
	styles := NewStyles()

	// Should not panic
	result := tbl.Render(styles, 17, 3)
	if result == "" {
		t.Error("Render() should not return empty string")
	}
}

func TestTable_Render_UnfocusedNoHighlight(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{{"one"}, {"two"}})
	tbl.SetFocused(false)

	styles := NewStyles()
	result := tbl.Render(styles, 10, 5)

	// When unfocused, the first row should not have the SelectedRow style
	// (This is a basic sanity check - the exact styling is hard to assert)
	if result == "" {
		t.Error("Render() should produce output even when unfocused")
	}
}

func TestTable_ClampCursor_EmptyRows(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.cursor = 5

	tbl.clampCursor()
	if tbl.cursor != 0 {
		t.Errorf("clampCursor with empty rows: cursor = %d, want 0", tbl.cursor)
	}
}

func TestTable_ClampCursor_NegativeCursor(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"a"}})
	tbl.cursor = -1

	tbl.clampCursor()
	if tbl.cursor != 0 {
		t.Errorf("clampCursor with negative cursor: cursor = %d, want 0", tbl.cursor)
	}
}

func TestTable_ClampScroll_ZeroViewport(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"a"}})
	tbl.scrollOffset = 5

	tbl.clampScroll(0)
	if tbl.scrollOffset != 0 {
		t.Errorf("clampScroll with zero viewport: offset = %d, want 0", tbl.scrollOffset)
	}
}

func TestTable_ClampScroll_EmptyRows(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.scrollOffset = 5

	tbl.clampScroll(10)
	if tbl.scrollOffset != 0 {
		t.Errorf("clampScroll with empty rows: offset = %d, want 0", tbl.scrollOffset)
	}
}

func TestTable_Render_FlexColumns(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "Fixed", Width: 10},
		{Header: "Flex"},
	})
	tbl.SetRows([][]string{
		{"hello", "world of data"},
	})
	styles := NewStyles()

	result := tbl.Render(styles, 30, 4)
	if !strings.Contains(result, "Fixed") {
		t.Error("should contain header 'Fixed'")
	}
	if !strings.Contains(result, "Flex") {
		t.Error("should contain header 'Flex'")
	}
	if !strings.Contains(result, "hello") {
		t.Error("should contain row data 'hello'")
	}
}

func TestTable_Render_SingleRow(t *testing.T) {
	tbl := NewTable([]Column{
		{Header: "Item", Width: 15},
	})
	tbl.SetRows([][]string{
		{"only item"},
	})
	styles := NewStyles()

	result := tbl.Render(styles, 15, 3)
	if !strings.Contains(result, "Item") {
		t.Error("should contain header 'Item'")
	}
	if !strings.Contains(result, "only item") {
		t.Error("should contain row data 'only item'")
	}
}

func TestTable_Render_HeightOnlyForHeader(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}})
	styles := NewStyles()

	// Height 1 means only header fits
	result := tbl.Render(styles, 5, 1)
	if !strings.Contains(result, "A") {
		t.Error("should still render header with height=1")
	}
}

func TestTable_ScrollInfo_AllFit(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{{"one"}, {"two"}, {"three"}})

	// Viewport fits all rows
	info := tbl.ScrollInfo(5)
	if info != "" {
		t.Errorf("ScrollInfo() = %q, want empty when all rows fit", info)
	}
}

func TestTable_ScrollInfo_Scrolled(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 50)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)

	styles := NewStyles()
	// Render to set scroll offset (cursor at 0)
	tbl.Render(styles, 10, 11) // height 11 = 1 header + 1 border + 9 data rows

	info := tbl.ScrollInfo(9)
	if info != "1-9 of 50" {
		t.Errorf("ScrollInfo() = %q, want %q", info, "1-9 of 50")
	}
}

func TestTable_ScrollInfo_AtEnd(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	rows := make([][]string, 25)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.MoveToBottom()

	styles := NewStyles()
	tbl.Render(styles, 10, 11) // height 11 = 1 header + 1 border + 9 data rows

	info := tbl.ScrollInfo(9)
	if info != "17-25 of 25" {
		t.Errorf("ScrollInfo() = %q, want %q", info, "17-25 of 25")
	}
}

func TestTable_ScrollInfo_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{})

	info := tbl.ScrollInfo(10)
	if info != "" {
		t.Errorf("ScrollInfo() = %q, want empty for empty table", info)
	}
}

func TestTable_SetRowStyle(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{{"row1"}, {"row2"}, {"row3"}})

	tbl.SetRowStyle(1, RowStyleVoid)

	if style, ok := tbl.rowStyles[1]; !ok || style != RowStyleVoid {
		t.Errorf("row style at index 1 = %v (ok=%v), want RowStyleVoid", style, ok)
	}

	// Default rows should not be in the map
	if _, ok := tbl.rowStyles[0]; ok {
		t.Error("row 0 should not have a style override")
	}
}

func TestTable_ClearRowStyles(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{{"row1"}, {"row2"}})

	tbl.SetRowStyle(0, RowStyleVoid)
	tbl.SetRowStyle(1, RowStyleVoid)

	tbl.ClearRowStyles()
	if tbl.rowStyles != nil {
		t.Error("rowStyles should be nil after ClearRowStyles()")
	}
}

func TestTable_SetRows_ClearsRowStyles(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 10}})
	tbl.SetRows([][]string{{"row1"}})
	tbl.SetRowStyle(0, RowStyleVoid)

	// Setting new rows should clear styles
	tbl.SetRows([][]string{{"new1"}, {"new2"}})
	if tbl.rowStyles != nil {
		t.Error("SetRows should clear rowStyles")
	}
}

func TestTable_HitTest_Header(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}, {"three"}})

	if got := tbl.HitTest(0); got != -1 {
		t.Errorf("HitTest(0) = %d, want -1 (header)", got)
	}
}

func TestTable_HitTest_FirstRow(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}, {"three"}})

	if got := tbl.HitTest(1); got != 0 {
		t.Errorf("HitTest(1) = %d, want 0", got)
	}
}

func TestTable_HitTest_SecondRow(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}, {"three"}})

	if got := tbl.HitTest(2); got != 1 {
		t.Errorf("HitTest(2) = %d, want 1", got)
	}
}

func TestTable_HitTest_WithScroll(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"row"}
	}
	tbl.SetRows(rows)
	tbl.scrollOffset = 5

	// y=1 should map to data row 5 (scrollOffset + 0)
	if got := tbl.HitTest(1); got != 5 {
		t.Errorf("HitTest(1) with scrollOffset=5 = %d, want 5", got)
	}

	// y=3 should map to data row 7
	if got := tbl.HitTest(3); got != 7 {
		t.Errorf("HitTest(3) with scrollOffset=5 = %d, want 7", got)
	}
}

func TestTable_HitTest_OutOfRange(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}})

	// y=3 with 2 rows and scrollOffset=0 -> data row 2, which is out of range
	if got := tbl.HitTest(3); got != -1 {
		t.Errorf("HitTest(3) with 2 rows = %d, want -1", got)
	}
}

func TestTable_HitTest_Negative(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}})

	if got := tbl.HitTest(-1); got != -1 {
		t.Errorf("HitTest(-1) = %d, want -1", got)
	}
}

func TestTable_HitTest_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})

	if got := tbl.HitTest(0); got != -1 {
		t.Errorf("HitTest(0) on empty table = %d, want -1", got)
	}
	if got := tbl.HitTest(1); got != -1 {
		t.Errorf("HitTest(1) on empty table = %d, want -1", got)
	}
}

func TestTable_HitTest_ScrollPastEnd(t *testing.T) {
	tbl := NewTable([]Column{{Header: "A", Width: 5}})
	tbl.SetRows([][]string{{"one"}, {"two"}, {"three"}})
	tbl.scrollOffset = 2

	// y=1 -> data row 2 (valid, last row)
	if got := tbl.HitTest(1); got != 2 {
		t.Errorf("HitTest(1) with scrollOffset=2 = %d, want 2", got)
	}

	// y=2 -> data row 3 (out of range)
	if got := tbl.HitTest(2); got != -1 {
		t.Errorf("HitTest(2) with scrollOffset=2 = %d, want -1", got)
	}
}

func TestTable_Render_VoidRowStyle(t *testing.T) {
	styles := NewStyles()
	cols := []Column{{Header: "Name", Width: 20}}
	tbl := NewTable(cols)
	tbl.SetRows([][]string{{"Normal"}, {"Void Row"}, {"Normal 2"}})
	tbl.SetRowStyle(1, RowStyleVoid)
	tbl.SetFocused(false) // Unfocused so no selected row styling

	rendered := tbl.Render(styles, 20, 5)

	// The void row should be rendered with void styling (dimmed/strikethrough)
	// The exact appearance depends on terminal, but we verify it renders without panic
	if rendered == "" {
		t.Error("Render() should produce output with void row styles")
	}

	// Verify the void row content is still present (even if styled)
	if !strings.Contains(rendered, "Void") {
		t.Error("void row content should still be visible in rendered output")
	}
}
