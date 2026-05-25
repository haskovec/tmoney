package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// FieldCombo state, ranking, navigation, and AddNew action row.

func TestRankComboMatches_EmptyQueryReturnsAllInOrder(t *testing.T) {
	got := rankComboMatches([]string{"a", "b", "c"}, "")
	want := []int{0, 1, 2}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_FilterFiltersByQuery(t *testing.T) {
	opts := []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}

	got := rankComboMatches(opts, "f")
	want := []int{3, 4}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('f') = %v, want %v", got, want)
	}

	got = rankComboMatches(opts, "g")
	want = []int{3}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('g') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_CaseInsensitive(t *testing.T) {
	got := rankComboMatches([]string{"Food > Groceries", "Other"}, "Gr")
	want := []int{0}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('Gr') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_PrefixBeforeSubstring(t *testing.T) {
	// Flat names: prefix matches rank ahead of substring matches; alphabetical
	// within each rank group.
	got := rankComboMatches([]string{"Restaurant Co", "Auto Repair", "Restaurant Bar"}, "r")
	want := []int{2, 0, 1} // "Restaurant Bar", "Restaurant Co", then "Auto Repair"
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('r') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_PrefixOnLeafSegment(t *testing.T) {
	// "Food > Restaurants" leaf segment "Restaurants" prefix-matches 'r'; the
	// other 'r'-containing options are substring matches and rank below it.
	opts := []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}
	got := rankComboMatches(opts, "r")
	want := []int{4, 2, 3}
	if !slices.Equal(got, want) {
		t.Errorf("rankComboMatches('r') = %v, want %v", got, want)
	}
}

func TestRankComboMatches_NoMatchReturnsEmpty(t *testing.T) {
	got := rankComboMatches([]string{"a", "b", "c"}, "z")
	if len(got) != 0 {
		t.Errorf("rankComboMatches('z') = %v, want empty", got)
	}
}

func TestDialog_AddComboField_BasicConstruction(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	if f.Type != FieldCombo {
		t.Errorf("Type = %v, want FieldCombo", f.Type)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", f.SelectedIndex)
	}
	if f.Query != "" {
		t.Errorf("Query = %q, want empty", f.Query)
	}
	// Empty query: highlight tracks the current selection.
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1", got)
	}
}

func TestDialog_AddComboField_ClampsSelected(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b"}, 99)
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (clamped)", f.SelectedIndex)
	}

	f = d.AddComboField("Other", []string{"a", "b"}, -3)
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (clamped)", f.SelectedIndex)
	}
}

func TestFieldCombo_TypingAppendsToQueryAndFilters(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if f.Query != "f" {
		t.Errorf("Query = %q, want %q", f.Query, "f")
	}
	if got := f.FilteredIndices(); !slices.Equal(got, []int{3, 4}) {
		t.Errorf("FilteredIndices = %v, want [3 4]", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if f.Query != "fo" {
		t.Errorf("Query = %q, want %q", f.Query, "fo")
	}
}

func TestFieldCombo_BackspaceShrinksQuery(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "ab", "abc"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	d.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if f.Query != "ab" {
		t.Fatalf("setup: Query = %q, want %q", f.Query, "ab")
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "a" {
		t.Errorf("Query after backspace = %q, want %q", f.Query, "a")
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "" {
		t.Errorf("Query after second backspace = %q, want empty", f.Query)
	}

	// Backspace at empty query is a no-op (does not leave the field).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if f.Query != "" {
		t.Errorf("Query after third backspace = %q, want empty", f.Query)
	}
}

func TestFieldCombo_ClearingQueryReturnsToFullList(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Food"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if got := f.FilteredIndices(); !slices.Equal(got, []int{2}) {
		t.Fatalf("setup filtered = %v, want [2]", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := f.FilteredIndices(); !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("FilteredIndices after clear = %v, want [0 1 2]", got)
	}
}

func TestFieldCombo_DownNavigatesFilteredOnly(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	// Filtered = [3, 4]; highlight starts at 0 → idx 3.
	if got := f.HighlightedIndex(); got != 3 {
		t.Fatalf("setup: HighlightedIndex = %d, want 3", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Errorf("HighlightedIndex after Down = %d, want 4", got)
	}

	// Past the last filtered row → stays put (no wrap).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Errorf("HighlightedIndex after second Down = %d, want 4 (no wrap)", got)
	}
}

func TestFieldCombo_UpNavigatesFilteredOnly(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := f.HighlightedIndex(); got != 4 {
		t.Fatalf("setup: HighlightedIndex = %d, want 4", got)
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := f.HighlightedIndex(); got != 3 {
		t.Errorf("HighlightedIndex after Up = %d, want 3", got)
	}
	// Past the first filtered row → stays put.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := f.HighlightedIndex(); got != 3 {
		t.Errorf("HighlightedIndex after second Up = %d, want 3 (no wrap)", got)
	}
}

func TestFieldCombo_EnterCommitsHighlightedClearsQueryAdvancesFocus(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if f.Query != "" {
		t.Errorf("Query after Enter = %q, want empty", f.Query)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Enter = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex after Enter = %d, want 1 (advanced)", d.FocusIndex())
	}
}

func TestFieldCombo_TabCommitsHighlightedAndAdvancesFocus(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})

	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Tab = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex after Tab = %d, want 1 (advanced)", d.FocusIndex())
	}
	if f.Query != "" {
		t.Errorf("Query after Tab = %q, want empty", f.Query)
	}
}

func TestFieldCombo_ShiftTabCommitsAndMovesFocusBack(t *testing.T) {
	d := NewDialog("Test")
	d.AddTextField("Payee", "", "", 10)
	f := d.AddComboField("Category", []string{"(None)", "Food > Groceries"}, 0)

	// Focus on the combo field.
	d.SetFocusIndex(1)
	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Shift+Tab = %d, want 1", f.SelectedIndex)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex after Shift+Tab = %d, want 0 (moved back)", d.FocusIndex())
	}
}

func TestFieldCombo_EscClearsQueryWithoutLeavingField(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	d.HandleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if f.Query != "b" {
		t.Fatalf("setup: Query = %q, want %q", f.Query, "b")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone (Esc with non-empty query)", action)
	}
	if f.Query != "" {
		t.Errorf("Query after Esc = %q, want empty", f.Query)
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Esc = %d, want 1 (unchanged)", f.SelectedIndex)
	}
}

func TestFieldCombo_EscWithEmptyQueryCancelsDialog(t *testing.T) {
	d := NewDialog("Test")
	d.AddComboField("Category", []string{"a", "b", "c"}, 0)

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("action = %v, want DialogActionCancel (Esc with empty query)", action)
	}
}

func TestFieldCombo_EmptyQueryHighlightsCurrentSelection(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 2)

	if got := f.HighlightedIndex(); got != 2 {
		t.Errorf("HighlightedIndex with empty query = %d, want 2 (current selection)", got)
	}
}

func TestFieldCombo_FilteredIndicesEmptyQueryReturnsAll(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)

	got := f.FilteredIndices()
	want := []int{0, 1, 2}
	if !slices.Equal(got, want) {
		t.Errorf("FilteredIndices empty = %v, want %v", got, want)
	}
}

func TestFieldCombo_NoMatchEnterPreservesSelection(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"a", "b", "c"}, 1)
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if got := f.FilteredIndices(); len(got) != 0 {
		t.Fatalf("setup: FilteredIndices = %v, want empty", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex after Enter with no match = %d, want 1 (preserved)", f.SelectedIndex)
	}
	if f.Query != "" {
		t.Errorf("Query after Enter = %q, want empty", f.Query)
	}
}

func TestFieldCombo_QueryChangeResetsHighlight(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Apple", "Banana", "Cherry"}, 2)

	// With empty query, highlight tracks the current selection (Cherry, idx 2).
	if got := f.HighlightedIndex(); got != 2 {
		t.Fatalf("setup: HighlightedIndex = %d, want 2", got)
	}

	// Typing 'a' filters to Apple, Banana — highlight resets to 0 (Apple).
	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if got := f.HighlightedIndex(); got != 0 {
		t.Errorf("HighlightedIndex after 'a' = %d, want 0 (Apple)", got)
	}
}

func TestFieldCombo_RenderShowsFilteredListWhenFocused(t *testing.T) {
	d := NewDialog("Test")
	d.AddComboField("Category", []string{"(None)", "Food > Groceries", "Food > Restaurants"}, 0)
	d.HandleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})

	styles := NewStyles()
	out := d.Render(styles)

	if !strings.Contains(out, "Food > Groceries") {
		t.Errorf("rendered output should list filtered match Food > Groceries; got:\n%s", out)
	}
	if !strings.Contains(out, "Food > Restaurants") {
		t.Errorf("rendered output should list filtered match Food > Restaurants; got:\n%s", out)
	}
}

// === FieldCombo: AddNew action row ===

func TestFieldCombo_AddNewLabel_FilteredIndicesUnchanged(t *testing.T) {
	// FilteredIndices contains only real Options; the action row is separate.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	got := f.FilteredIndices()
	want := []int{0, 1}
	if !slices.Equal(got, want) {
		t.Errorf("FilteredIndices = %v, want %v (action row not in Options)", got, want)
	}
}

func TestFieldCombo_AddNewLabel_DownNavigatesPastLastMatchToActionRow(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	// Empty query, two matches; highlight starts at SelectedIndex (0).
	if f.IsAddNewHighlighted() {
		t.Fatalf("setup: IsAddNewHighlighted = true at start, want false")
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> idx 1 (Auto)
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> action row
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false after Down past last, want true")
	}
	// One more Down stays put (no wrap past action row).
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false after extra Down, want true (no wrap)")
	}
}

func TestFieldCombo_AddNewLabel_UpFromActionRowReturnsToLastMatch(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row not highlighted")
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = true after Up, want false")
	}
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1 (Auto)", got)
	}
}

func TestFieldCombo_AddNewLabel_NoMatchesActionRowHighlightedAtIndexZero(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	// Type a non-matching query: no real matches; action row is the only row.
	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if got := f.FilteredIndices(); len(got) != 0 {
		t.Fatalf("setup: FilteredIndices = %v, want empty", got)
	}
	if !f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = false with no matches and AddNewLabel set, want true")
	}
}

func TestFieldCombo_AddNewLabel_EnterOnActionRowReturnsDialogActionAddNew(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	// Type "Donations" — no matches; action row is the only row.
	for _, r := range "Donations" {
		d.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row should be highlighted (no matches)")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionAddNew {
		t.Errorf("action = %v, want DialogActionAddNew", action)
	}
	if !f.AddNewTriggered {
		t.Errorf("AddNewTriggered = false, want true")
	}
	if f.Query != "Donations" {
		t.Errorf("Query = %q, want %q (preserved for parent to read)", f.Query, "Donations")
	}
	if f.SelectedIndex != 0 {
		t.Errorf("SelectedIndex = %d, want 0 (unchanged)", f.SelectedIndex)
	}
	if d.FocusIndex() != 0 {
		t.Errorf("FocusIndex = %d, want 0 (focus must not advance — parent handles diversion)", d.FocusIndex())
	}
}

func TestFieldCombo_AddNewLabel_EnterOnRegularMatchCommitsNormally(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	// Filtered: ["Auto"]; highlight at first match.
	if f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row highlighted, want regular match")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionNone {
		t.Errorf("action = %v, want DialogActionNone (regular commit)", action)
	}
	if f.AddNewTriggered {
		t.Errorf("AddNewTriggered = true, want false (regular commit)")
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (Auto)", f.SelectedIndex)
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (advanced)", d.FocusIndex())
	}
}

func TestFieldCombo_AddNewLabel_RenderShowsActionRow(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	styles := NewStyles()
	out := d.Render(styles)
	if !strings.Contains(out, "[+ Add new category…]") {
		t.Errorf("render should include action row label; got:\n%s", out)
	}
}

func TestFieldCombo_AddNewLabel_RenderShowsActionRowWhenNoMatches(t *testing.T) {
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})

	styles := NewStyles()
	out := d.Render(styles)
	if !strings.Contains(out, "[+ Add new category…]") {
		t.Errorf("render should include action row even when no matches; got:\n%s", out)
	}
}

func TestFieldCombo_NoAddNewLabel_DownStillStopsAtLastMatch(t *testing.T) {
	// Without AddNewLabel: behavior unchanged — Down stops at last match,
	// no action row exists, IsAddNewHighlighted is always false.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food", "Auto"}, 0)

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // tries to go past last
	if got := f.HighlightedIndex(); got != 1 {
		t.Errorf("HighlightedIndex = %d, want 1 (no action row, no wrap)", got)
	}
	if f.IsAddNewHighlighted() {
		t.Errorf("IsAddNewHighlighted = true, want false (no action row configured)")
	}
}

func TestFieldCombo_AddNewLabel_TabOnActionRowDoesNotTriggerAddNew(t *testing.T) {
	// Tab on action row leaves the field (advances focus) without triggering
	// AddNew — only Enter triggers AddNew per spec.
	d := NewDialog("Test")
	f := d.AddComboField("Category", []string{"Food"}, 0)
	f.AddNewLabel = "[+ Add new category…]"
	d.AddTextField("Memo", "", "", 10)

	d.HandleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !f.IsAddNewHighlighted() {
		t.Fatalf("setup: action row not highlighted")
	}

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if action == DialogActionAddNew {
		t.Errorf("action = DialogActionAddNew, want != AddNew (Tab does not trigger AddNew)")
	}
	if f.AddNewTriggered {
		t.Errorf("AddNewTriggered = true after Tab, want false")
	}
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (Tab advanced focus)", d.FocusIndex())
	}
}

// FieldDate ISO format (TD-015) tests
