package dialog

import (
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// FieldType represents the type of a form field in a dialog.
type FieldType int

const (
	// FieldText is a text input field.
	FieldText FieldType = iota
	// FieldSelect is a dropdown selection field navigated with up/down keys.
	FieldSelect
	// FieldRadio is a radio button group.
	FieldRadio
	// FieldCheckbox is a checkbox field toggled with space.
	FieldCheckbox
	// FieldList is a vertical scrollable list showing multiple items at once.
	FieldList
	// FieldDate is a fixed-width MM/DD/YYYY masked input. The cursor only
	// lands on the eight digit positions (string indices 0,1,3,4,6,7,8,9);
	// typing a digit overwrites in place and auto-advances; Backspace
	// replaces the digit at the cursor with '0' and steps back.
	FieldDate
	// FieldCombo is a typeahead combo box. Typing filters the option list
	// (prefix-on-leaf-segment first, substring second; alphabetical within
	// each rank group); Up/Down navigate the filtered subset; Enter/Tab
	// commit the highlighted match and advance focus; Esc clears a non-empty
	// query in-place. While the query is empty, the full option list is
	// shown and the highlight tracks SelectedIndex.
	FieldCombo
)

// Date-mask format strings for the masked-input FieldDate widget. A mask is
// a 10-character template where 'M', 'D', and 'Y' mark digit positions; any
// other character is rendered verbatim as a literal separator (and the
// cursor skips over it). Adding a new format means adding a new constant
// here — the cursor / render / blank-detection logic is mask-driven.
const (
	DateMaskUS  = "MM/DD/YYYY"
	DateMaskISO = "YYYY-MM-DD"
)

// canonicalBlankDate is the all-blank canonical mask for an optional date
// field in MM/DD/YYYY format — slashes at positions 2 and 5, spaces at
// every digit position. Kept as a top-level constant for use as the
// default-construct value in helpers; ISO blanks are computed from
// dateMaskISO.
const canonicalBlankDate = "  /  /    "

// dateMaskSeparatorPositions returns the byte positions of literal
// (non-M/D/Y) characters in mask. The masked-cursor methods skip these.
func dateMaskSeparatorPositions(mask string) map[int]bool {
	out := make(map[int]bool, 2)
	for i := 0; i < len(mask); i++ {
		switch mask[i] {
		case 'M', 'D', 'Y':
			// digit position
		default:
			out[i] = true
		}
	}
	return out
}

// dateMaskBlank returns the canonical all-blank value for mask — literals
// preserved at separator positions, spaces at every digit position.
func dateMaskBlank(mask string) string {
	b := []byte(mask)
	for i := range b {
		switch b[i] {
		case 'M', 'D', 'Y':
			b[i] = ' '
		}
	}
	return string(b)
}

// IsBlankDateInput reports whether s represents an unfilled date — either an
// empty string or the 10-char canonical mask with every digit position still
// a space. Format-agnostic: a 10-char string with no digit characters is
// considered blank, which matches the canonical blank for any mask whose
// digit positions are filled with ' '.
func IsBlankDateInput(s string) bool {
	if s == "" {
		return true
	}
	if len(s) != 10 {
		return false
	}
	for i := range len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			return false
		}
	}
	return true
}

// Field represents a form field in a dialog.
type Field struct {
	// Label is the display name shown next to the field.
	Label string
	// Type is the field type.
	Type FieldType
	// Value is the current text value (for FieldText).
	Value string
	// Placeholder is shown when Value is empty (for FieldText).
	Placeholder string
	// Options is the list of choices (for FieldSelect and FieldRadio).
	Options []string
	// SelectedIndex is the currently selected option index (for FieldSelect and FieldRadio).
	SelectedIndex int
	// Checked is the checkbox state (for FieldCheckbox).
	Checked bool
	// Width is the display width of the text input area (for FieldText). 0 means auto.
	Width int
	// Required indicates this field must be filled in.
	Required bool
	// Error is an inline validation error message displayed below the field.
	Error string
	// Hidden indicates this field should not be rendered or focusable.
	Hidden bool
	// VisibleCount is the number of items to display at once (for FieldList).
	VisibleCount int
	// Query is the typed-but-not-yet-committed search string (for FieldCombo).
	Query string
	// AddNewLabel, when non-empty (FieldCombo only), appends a sentinel
	// "[+ Add new …]" action row to the bottom of the filtered list. Enter
	// on the action row sets AddNewTriggered and HandleKey returns
	// DialogActionAddNew so a parent dialog can divert into a sub-dialog.
	// The typed Query is preserved on trigger so the parent can read it.
	AddNewLabel string
	// AddNewTriggered records that the user activated the AddNew action row.
	// The parent dialog is expected to consume the trigger and reset the
	// flag (along with any other state it captures from Query).
	AddNewTriggered bool
	// OptionalBlank, when true on a FieldDate, allows the canonical 10-char
	// all-blank mask ("  /  /    ") as a meaningful "no value" state.
	// Backspace clears digits with ' ' (instead of '0') and steps back so
	// the user can return to the canonical blank. Submit handlers should
	// use isBlankDateInput to detect the unfilled state and treat it as
	// missing rather than invalid.
	OptionalBlank bool
	// NumericOnly, when true on a FieldText, restricts typed input to the
	// digits 0-9 and at most one decimal point. Any other rune (sign, '$',
	// comma, letter, space) is silently dropped at the input layer, mirroring
	// the digit-only filter on FieldDate. The field is otherwise an ordinary
	// FieldText — cursor, edit, paste, and render behavior are unchanged.
	// Set via AddNumericField; used by the investment-register dialogs whose
	// Shares/Amount/Price/Total/Commission fields are always positive
	// magnitudes. Programmatic Value prefills are not filtered.
	NumericOnly bool
	// DateMask describes the format of a FieldDate ('M', 'D', 'Y' mark
	// digit positions; any other character is a literal separator).
	// Defaults to dateMaskUS ("MM/DD/YYYY") via the helpers below; ISO
	// fields use dateMaskISO ("YYYY-MM-DD"). The masked-cursor methods
	// derive separator positions from this string so adding a new format
	// is a one-constant change.
	DateMask string
	// cursorPos is the cursor position within the text value.
	cursorPos int
	// ComboHighlight is the highlighted row index within the current
	// filtered list (for FieldCombo). When Query is empty the filtered list
	// equals the full Options in order, so this also identifies the
	// absolute option index. When AddNewLabel is set, the highlight may
	// also point one past the last filtered match — the action row.
	ComboHighlight int
}

// numericAccepts reports whether r is a permissible keystroke for a
// NumericOnly field: a digit always, or a decimal point only when the field
// does not already contain one (so the value can never grow a second dot and
// always parses as a number). Callers gate on f.NumericOnly before using it.
func (f *Field) numericAccepts(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	return r == '.' && !strings.ContainsRune(f.Value, '.')
}

// InsertChar inserts a character at the cursor position.
func (f *Field) InsertChar(ch rune) {
	if f.Type != FieldText {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos > len(runes) {
		f.cursorPos = len(runes)
	}
	result := make([]rune, 0, len(runes)+1)
	result = append(result, runes[:f.cursorPos]...)
	result = append(result, ch)
	result = append(result, runes[f.cursorPos:]...)
	f.Value = string(result)
	f.cursorPos++
}

// DeleteBack deletes the character before the cursor.
func (f *Field) DeleteBack() {
	if f.Type != FieldText || f.cursorPos <= 0 {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos > len(runes) {
		f.cursorPos = len(runes)
	}
	result := make([]rune, 0, len(runes)-1)
	result = append(result, runes[:f.cursorPos-1]...)
	result = append(result, runes[f.cursorPos:]...)
	f.Value = string(result)
	f.cursorPos--
}

// DeleteForward deletes the character at the cursor position.
func (f *Field) DeleteForward() {
	if f.Type != FieldText {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos >= len(runes) {
		return
	}
	result := make([]rune, 0, len(runes)-1)
	result = append(result, runes[:f.cursorPos]...)
	result = append(result, runes[f.cursorPos+1:]...)
	f.Value = string(result)
}

// MoveCursorLeft moves the cursor one position to the left.
func (f *Field) MoveCursorLeft() {
	if f.Type != FieldText || f.cursorPos <= 0 {
		return
	}
	f.cursorPos--
}

// MoveCursorRight moves the cursor one position to the right.
func (f *Field) MoveCursorRight() {
	if f.Type != FieldText {
		return
	}
	if f.cursorPos < len([]rune(f.Value)) {
		f.cursorPos++
	}
}

// MoveCursorHome moves the cursor to the beginning.
func (f *Field) MoveCursorHome() {
	if f.Type != FieldText {
		return
	}
	f.cursorPos = 0
}

// MoveCursorEnd moves the cursor to the end.
func (f *Field) MoveCursorEnd() {
	if f.Type != FieldText {
		return
	}
	f.cursorPos = len([]rune(f.Value))
}

// CursorPos returns the current cursor position.
func (f *Field) CursorPos() int {
	return f.cursorPos
}

// DateSeparators returns the separator-position set for this field's mask.
// Defaults to the US (MM/DD/YYYY) layout when dateMask is empty so that
// any FieldDate constructed without going through the helpers still
// behaves correctly.
func (f *Field) DateSeparators() map[int]bool {
	mask := f.DateMask
	if mask == "" {
		mask = DateMaskUS
	}
	return dateMaskSeparatorPositions(mask)
}

// DateCursorRight advances the cursor to the next digit position, skipping
// literal separator characters in the field's mask. Stops at the last
// digit (index 9).
func (f *Field) DateCursorRight() {
	if f.Type != FieldDate {
		return
	}
	seps := f.DateSeparators()
	for f.cursorPos < 9 {
		f.cursorPos++
		if !seps[f.cursorPos] {
			return
		}
	}
}

// DateCursorLeft moves the cursor to the previous digit position, skipping
// literal separator characters in the field's mask. Stops at the first
// digit (index 0).
func (f *Field) DateCursorLeft() {
	if f.Type != FieldDate {
		return
	}
	seps := f.DateSeparators()
	for f.cursorPos > 0 {
		f.cursorPos--
		if !seps[f.cursorPos] {
			return
		}
	}
}

// DateCursorHome jumps the cursor to the first digit (index 0).
func (f *Field) DateCursorHome() {
	if f.Type != FieldDate {
		return
	}
	f.cursorPos = 0
}

// DateCursorEnd jumps the cursor to the last digit (index 9).
func (f *Field) DateCursorEnd() {
	if f.Type != FieldDate {
		return
	}
	f.cursorPos = 9
}

// DateOverwriteDigit replaces the digit at the cursor with r (which must be
// '0'..'9') and advances the cursor to the next digit position. Non-digit
// runes are ignored.
func (f *Field) DateOverwriteDigit(r rune) {
	if f.Type != FieldDate || r < '0' || r > '9' {
		return
	}
	if f.DateSeparators()[f.cursorPos] {
		return
	}
	if len(f.Value) != 10 {
		return
	}
	b := []byte(f.Value)
	b[f.cursorPos] = byte(r)
	f.Value = string(b)
	f.DateCursorRight()
}

// DateBackspace handles the Backspace key on a FieldDate.
//
// For a strict (non-optional) field, it overwrites the digit at the cursor
// with '0' and steps the cursor back — preserving the canonical
// always-valid-shape MM/DD/YYYY semantics.
//
// For an OptionalBlank field, it instead deletes the digit *before* the
// cursor (writing ' ') and moves the cursor back to that position — the
// conventional editor "backspace deletes the character to the left" — so
// the user can clear typed digits all the way back to the canonical blank
// "  /  /    ".
//
// No-op at the first digit (cursorPos == 0).
func (f *Field) DateBackspace() {
	if f.Type != FieldDate || f.cursorPos <= 0 {
		return
	}
	seps := f.DateSeparators()
	if f.OptionalBlank {
		f.DateCursorLeft()
		if len(f.Value) == 10 && !seps[f.cursorPos] {
			b := []byte(f.Value)
			b[f.cursorPos] = ' '
			f.Value = string(b)
		}
		return
	}
	if len(f.Value) == 10 && !seps[f.cursorPos] {
		b := []byte(f.Value)
		b[f.cursorPos] = '0'
		f.Value = string(b)
	}
	f.DateCursorLeft()
}

// SelectNext moves to the next option (FieldSelect and FieldRadio).
func (f *Field) SelectNext() {
	if (f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList) || len(f.Options) == 0 {
		return
	}
	if f.SelectedIndex < len(f.Options)-1 {
		f.SelectedIndex++
	}
}

// SelectPrev moves to the previous option (FieldSelect, FieldRadio, and FieldList).
func (f *Field) SelectPrev() {
	if (f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList) || len(f.Options) == 0 {
		return
	}
	if f.SelectedIndex > 0 {
		f.SelectedIndex--
	}
}

// Toggle toggles the checked state (FieldCheckbox).
func (f *Field) Toggle() {
	if f.Type != FieldCheckbox {
		return
	}
	f.Checked = !f.Checked
}

// SelectedOption returns the currently selected option text.
func (f *Field) SelectedOption() string {
	if f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList && f.Type != FieldCombo {
		return ""
	}
	if len(f.Options) == 0 {
		return ""
	}
	if f.SelectedIndex < 0 || f.SelectedIndex >= len(f.Options) {
		return ""
	}
	return f.Options[f.SelectedIndex]
}

// AddTextField adds a text input field and returns it.
func (d *Dialog) AddTextField(label, value, placeholder string, width int) *Field {
	f := &Field{
		Label:       label,
		Type:        FieldText,
		Value:       value,
		Placeholder: placeholder,
		Width:       width,
		cursorPos:   len([]rune(value)),
	}
	d.fields = append(d.fields, f)
	return f
}

// AddNumericField adds a text input field that accepts only digits and a
// single decimal point (see Field.NumericOnly), returning it. It is an
// ordinary FieldText in every other respect; the input restriction is applied
// at the key-handling layer. Used by the investment-register dialogs for
// Shares/Amount/Price/Total/Commission and per-lot allocation fields.
func (d *Dialog) AddNumericField(label, value, placeholder string, width int) *Field {
	f := d.AddTextField(label, value, placeholder, width)
	f.NumericOnly = true
	return f
}

// AddSelectField adds a dropdown selection field and returns it.
func (d *Dialog) AddSelectField(label string, options []string, selected int) *Field {
	if selected < 0 {
		selected = 0
	}
	if len(options) > 0 && selected >= len(options) {
		selected = len(options) - 1
	}
	f := &Field{
		Label:         label,
		Type:          FieldSelect,
		Options:       options,
		SelectedIndex: selected,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddRadioField adds a radio button group and returns it.
func (d *Dialog) AddRadioField(label string, options []string, selected int) *Field {
	if selected < 0 {
		selected = 0
	}
	if len(options) > 0 && selected >= len(options) {
		selected = len(options) - 1
	}
	f := &Field{
		Label:         label,
		Type:          FieldRadio,
		Options:       options,
		SelectedIndex: selected,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddCheckboxField adds a checkbox field and returns it.
func (d *Dialog) AddCheckboxField(label string, checked bool) *Field {
	f := &Field{
		Label:   label,
		Type:    FieldCheckbox,
		Checked: checked,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddDateField adds a masked-input MM/DD/YYYY date field and returns it.
// The cursor lands only on digit positions (slashes at indices 2 and 5 are
// skipped); typing a digit overwrites in place and auto-advances; Backspace
// replaces with '0' and steps back. If initialValue is empty, the field is
// seeded with today's date.
func (d *Dialog) AddDateField(label, initialValue string) *Field {
	if initialValue == "" {
		initialValue = time.Now().Format("01/02/2006")
	}
	f := &Field{
		Label:     label,
		Type:      FieldDate,
		Value:     initialValue,
		Width:     10,
		DateMask:  DateMaskUS,
		cursorPos: 0,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddOptionalDateField adds a masked-input MM/DD/YYYY date field that
// permits the canonical 10-char all-blank value "  /  /    " as a
// meaningful "no value" state. An empty initialValue seeds the field with
// the canonical blank; a non-empty initialValue is taken verbatim (callers
// should pass a fully-formed date string).
//
// Backspace on an optional field clears the digit before the cursor with
// ' ' (and steps back) — the conventional editor semantic — so the user
// can return to the canonical blank. Submit handlers should call
// isBlankDateInput on the field's Value to detect the unfilled state.
func (d *Dialog) AddOptionalDateField(label, initialValue string) *Field {
	if initialValue == "" {
		initialValue = canonicalBlankDate
	}
	f := &Field{
		Label:         label,
		Type:          FieldDate,
		Value:         initialValue,
		Width:         10,
		DateMask:      DateMaskUS,
		OptionalBlank: true,
		cursorPos:     0,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddDateFieldISO adds a masked-input YYYY-MM-DD date field and returns
// it. Same widget as AddDateField with dashes at indices 4 and 7 instead
// of slashes at 2 and 5; if initialValue is empty, the field is seeded
// with today's date in YYYY-MM-DD form.
func (d *Dialog) AddDateFieldISO(label, initialValue string) *Field {
	if initialValue == "" {
		initialValue = time.Now().Format("2006-01-02")
	}
	f := &Field{
		Label:     label,
		Type:      FieldDate,
		Value:     initialValue,
		Width:     10,
		DateMask:  DateMaskISO,
		cursorPos: 0,
	}
	d.fields = append(d.fields, f)
	return f
}

// SeedDateField overwrites the first FieldDate's Value with seedDate in
// MM/DD/YYYY form. No-op when seedDate is zero, the dialog has no date
// field, or the date field uses the ISO mask. Used by the New-Transaction
// flows to apply the session's sticky last-saved date after the dialog has
// already been built with today as its default.
func (d *Dialog) SeedDateField(seedDate types.Date) {
	if seedDate.IsZero() {
		return
	}
	for _, f := range d.fields {
		if f.Type != FieldDate {
			continue
		}
		if f.DateMask != DateMaskUS {
			return
		}
		f.Value = seedDate.Time().Format("01/02/2006")
		return
	}
}

// AddOptionalDateFieldISO adds a masked-input YYYY-MM-DD date field that
// permits the canonical 10-char all-blank value "    -  -  " as a
// meaningful "no value" state. Empty initialValue seeds the canonical
// blank; submit handlers should call isBlankDateInput on the value to
// detect the unfilled state.
func (d *Dialog) AddOptionalDateFieldISO(label, initialValue string) *Field {
	if initialValue == "" {
		initialValue = dateMaskBlank(DateMaskISO)
	}
	f := &Field{
		Label:         label,
		Type:          FieldDate,
		Value:         initialValue,
		Width:         10,
		DateMask:      DateMaskISO,
		OptionalBlank: true,
		cursorPos:     0,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddListField adds a vertical scrollable list field and returns it.
// visibleCount controls how many items are shown at once.
func (d *Dialog) AddListField(label string, items []string, selected int, visibleCount int) *Field {
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) && len(items) > 0 {
		selected = len(items) - 1
	}
	if visibleCount <= 0 {
		visibleCount = 10
	}
	f := &Field{
		Label:         label,
		Type:          FieldList,
		Options:       items,
		SelectedIndex: selected,
		VisibleCount:  visibleCount,
	}
	d.fields = append(d.fields, f)
	return f
}
