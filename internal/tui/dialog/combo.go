package dialog

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleComboNavigationKey processes the combo-specific navigation keys that
// must fire before the dialog's default key dispatch: Esc clears a non-empty
// query in place; Enter on the AddNew action row triggers
// DialogActionAddNew without advancing focus or clearing the query (the
// parent dialog reads Query at the moment of trigger); Enter/Tab/Shift+Tab
// on a regular match commit the highlighted match before advancing focus.
//
// Returns handled=true when the key was consumed and the caller should
// return action; handled=false when the key should fall through to the
// dialog's default dispatch (e.g. Esc on an empty query falls through to
// Cancel; any other key falls through to Field.HandleComboKey).
func (d *Dialog) handleComboNavigationKey(field *Field, keyStr string) (handled bool, action DialogAction) {
	switch keyStr {
	case "esc":
		if field.Query != "" {
			field.ClearComboQuery()
			return true, DialogActionNone
		}
	case "enter":
		if field.IsAddNewHighlighted() {
			field.AddNewTriggered = true
			return true, DialogActionAddNew
		}
		field.CommitComboHighlight()
		d.FocusNext()
		return true, DialogActionNone
	case "tab":
		field.CommitComboHighlight()
		d.FocusNext()
		return true, DialogActionNone
	case "shift+tab":
		field.CommitComboHighlight()
		d.FocusPrev()
		return true, DialogActionNone
	}
	return false, DialogActionNone
}

// handleComboClick commits a left-click on the combo's dropdown panel at the
// given panel line index (as produced by ComboPanelLineAt). A click on a real
// match commits it and advances focus, mirroring Enter on a highlighted match
// in handleComboNavigationKey; a click on the AddNew action row sets
// AddNewTriggered and returns DialogActionAddNew, mirroring Enter on that row
// (the parent dialog reads Query and diverts into the create sub-dialog). An
// out-of-range index (e.g. the header line or a "(no matches)" row) is a
// no-op that leaves the combo focused with its dropdown open.
func (d *Dialog) handleComboClick(field *Field, line int) DialogAction {
	picked, addNew := field.ClickComboLine(line)
	switch {
	case addNew:
		return DialogActionAddNew
	case picked:
		d.FocusNext()
	}
	return DialogActionNone
}

// ClickComboLine applies a click on the given panel line (as produced by
// ComboPanelLineAt). A click on a real match commits it and reports
// picked; a click on the AddNew action row highlights it, sets
// AddNewTriggered, and reports addNew. Any other line is a no-op. The caller
// moves focus: after a pick it advances, after AddNew it stays.
func (f *Field) ClickComboLine(line int) (picked, addNew bool) {
	if f.Type != FieldCombo || line < 0 {
		return false, false
	}
	indices := f.FilteredIndices()
	if f.AddNewLabel != "" && line == len(indices) {
		f.ComboHighlight = line
		f.AddNewTriggered = true
		return false, true
	}
	if line < len(indices) {
		f.ComboHighlight = line
		f.CommitComboHighlight()
		return true, false
	}
	return false, false
}

// ComboPanelRows returns how many lines the combo's dropdown panel takes
// below its header while the combo is focused: the visible window of
// matches (plus the AddNew row), or 1 for the "(no matches)" line.
func (f *Field) ComboPanelRows() int {
	_, visible, totalRows := f.comboPanelWindow()
	if totalRows == 0 {
		return 1
	}
	return visible
}

// ComboHeaderText returns the text a combo shows on its header line: the
// typed query while focused with a non-empty query, otherwise the selected
// option, or "(none)" when nothing is selected.
func (f *Field) ComboHeaderText(focused bool) string {
	if focused && f.Query != "" {
		return f.Query
	}
	if opt := f.SelectedOption(); opt != "" {
		return opt
	}
	return "(none)"
}

// HandleComboKey applies an editing key to a focused combo: Up/Down move the
// highlight, Backspace trims the query, and typed text extends it. Esc,
// Enter, and Tab are the caller's, because they close the combo or move
// focus. Surfaces that draw their own combos (the split editor and the
// paycheck wizard) call this so their pickers act the same as a Dialog's.
func (f *Field) HandleComboKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up":
		f.comboHighlightUp()
		f.Error = ""
		return
	case "down":
		f.comboHighlightDown()
		f.Error = ""
		return
	case "backspace":
		f.comboQueryBackspace()
		f.Error = ""
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			f.comboQueryAppend(r)
		}
		f.Error = ""
	}
}

// comboPanelWindow returns the scroll offset, visible-row count, and total
// row count (filtered matches plus the optional AddNew action row) for the
// combo's dropdown panel. RenderComboPanel draws the panel from these values
// and the mouse hit-test maps clicks back through them, so keeping the math
// in one place ensures the rendered rows and the clickable rows never
// diverge. Returns zeros when there is nothing to show.
func (f *Field) comboPanelWindow() (scrollOffset, visible, totalRows int) {
	if f.Type != FieldCombo {
		return 0, 0, 0
	}
	totalRows = len(rankComboMatches(f.Options, f.Query))
	if f.AddNewLabel != "" {
		totalRows++
	}
	if totalRows == 0 {
		return 0, 0, 0
	}
	visible = f.VisibleCount
	if visible <= 0 {
		visible = 8
	}
	if visible > totalRows {
		visible = totalRows
	}
	if f.ComboHighlight >= visible {
		scrollOffset = f.ComboHighlight - visible + 1
	}
	if scrollOffset+visible > totalRows {
		scrollOffset = totalRows - visible
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	return scrollOffset, visible, totalRows
}

// ComboPanelLineAt maps a 0-based visible-row offset within the rendered
// dropdown panel to a panel line index: an index into FilteredIndices for a
// real option, or len(FilteredIndices) for the AddNew action row. Returns -1
// when the offset falls outside the rendered window or when there are no rows
// (the "(no matches)" placeholder is inert). Uses comboPanelWindow so the
// mapping tracks RenderComboPanel exactly.
func (f *Field) ComboPanelLineAt(visibleRow int) int {
	scrollOffset, visible, totalRows := f.comboPanelWindow()
	if totalRows == 0 || visibleRow < 0 || visibleRow >= visible {
		return -1
	}
	line := scrollOffset + visibleRow
	if line >= totalRows {
		return -1
	}
	return line
}

// AddComboField adds a typeahead combo box and returns it. Typing filters
// the option list with leaf-prefix-first ranking; Up/Down navigate the
// filtered subset; Enter/Tab commit the highlighted match.
func (d *Dialog) AddComboField(label string, options []string, selected int) *Field {
	if selected < 0 {
		selected = 0
	}
	if len(options) > 0 && selected >= len(options) {
		selected = len(options) - 1
	}
	f := &Field{
		Label:          label,
		Type:           FieldCombo,
		Options:        options,
		SelectedIndex:  selected,
		ComboHighlight: selected,
	}
	d.fields = append(d.fields, f)
	return f
}

// rankComboMatches returns indices into options for entries matching query
// (case-insensitive substring), ordered by:
//   - prefix matches on the leaf segment (after the last " > " or ":") first,
//     then substring matches; lexical (lowercase) order within each group.
//
// An empty query returns all indices in their original order.
func rankComboMatches(options []string, query string) []int {
	if query == "" {
		idx := make([]int, len(options))
		for i := range options {
			idx[i] = i
		}
		return idx
	}
	q := strings.ToLower(query)

	type entry struct {
		idx      int
		isPrefix bool
		display  string
	}
	var matches []entry
	for i, opt := range options {
		lower := strings.ToLower(opt)
		if !strings.Contains(lower, q) {
			continue
		}
		leaf := lower
		if j := strings.LastIndex(lower, " > "); j >= 0 {
			leaf = lower[j+len(" > "):]
		} else if j := strings.LastIndex(lower, ":"); j >= 0 {
			leaf = lower[j+1:]
		}
		matches = append(matches, entry{
			idx:      i,
			isPrefix: strings.HasPrefix(leaf, q),
			display:  lower,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].isPrefix != matches[j].isPrefix {
			return matches[i].isPrefix
		}
		return matches[i].display < matches[j].display
	})
	result := make([]int, len(matches))
	for i, m := range matches {
		result[i] = m.idx
	}
	return result
}

// FilteredIndices returns the indices into Options of options matching the
// current Query, ordered by leaf-prefix-first ranking. Empty query returns
// all indices in original order.
func (f *Field) FilteredIndices() []int {
	if f.Type != FieldCombo {
		return nil
	}
	return rankComboMatches(f.Options, f.Query)
}

// HighlightedIndex returns the index in Options of the currently-highlighted
// row in the filtered list, or -1 when no row maps to a real option (no
// matches, or the AddNew action row is highlighted).
func (f *Field) HighlightedIndex() int {
	if f.Type != FieldCombo {
		return -1
	}
	indices := f.FilteredIndices()
	if len(indices) == 0 {
		return -1
	}
	hi := max(f.ComboHighlight, 0)
	if hi >= len(indices) {
		// Either out-of-bounds or pointing at the AddNew action row.
		return -1
	}
	return indices[hi]
}

// IsAddNewHighlighted reports whether the FieldCombo's AddNew action row is
// the current highlight target. Always false when AddNewLabel is empty.
func (f *Field) IsAddNewHighlighted() bool {
	if f.Type != FieldCombo || f.AddNewLabel == "" {
		return false
	}
	indices := rankComboMatches(f.Options, f.Query)
	return f.ComboHighlight == len(indices)
}

// CommitComboHighlight sets SelectedIndex to the highlighted row in the
// filtered list (if any) and clears the query. No-op when no rows match
// (preserves the previous selection).
func (f *Field) CommitComboHighlight() {
	if f.Type != FieldCombo {
		return
	}
	if idx := f.HighlightedIndex(); idx >= 0 {
		f.SelectedIndex = idx
	}
	f.Query = ""
	f.ComboHighlight = f.SelectedIndex
}

// ClearComboQuery resets Query and snaps the highlight back to SelectedIndex.
// Used by Esc when the query is non-empty.
func (f *Field) ClearComboQuery() {
	if f.Type != FieldCombo {
		return
	}
	f.Query = ""
	f.ComboHighlight = f.SelectedIndex
}

// comboQueryAppend adds a typed character to Query and resets the highlight
// to the first filtered match.
func (f *Field) comboQueryAppend(r rune) {
	if f.Type != FieldCombo {
		return
	}
	f.Query += string(r)
	f.ComboHighlight = 0
}

// comboQueryBackspace removes the last character of Query (if any) and
// resets the highlight. No-op when Query is already empty.
func (f *Field) comboQueryBackspace() {
	if f.Type != FieldCombo || f.Query == "" {
		return
	}
	runes := []rune(f.Query)
	f.Query = string(runes[:len(runes)-1])
	if f.Query == "" {
		f.ComboHighlight = f.SelectedIndex
	} else {
		f.ComboHighlight = 0
	}
}

// comboHighlightDown moves the highlight down within the filtered list,
// stopping at the last row (no wrap). When AddNewLabel is set, the highlight
// may step one past the last match onto the action row.
func (f *Field) comboHighlightDown() {
	if f.Type != FieldCombo {
		return
	}
	indices := f.FilteredIndices()
	maxIdx := len(indices) - 1
	if f.AddNewLabel != "" {
		// Action row sits at len(indices); allow one more step.
		maxIdx = len(indices)
	}
	if maxIdx < 0 {
		return
	}
	if f.ComboHighlight < maxIdx {
		f.ComboHighlight++
	}
}

// comboHighlightUp moves the highlight up within the filtered list, stopping
// at the first row.
func (f *Field) comboHighlightUp() {
	if f.Type != FieldCombo {
		return
	}
	if f.ComboHighlight > 0 {
		f.ComboHighlight--
	}
}
