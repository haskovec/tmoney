package tui

import "github.com/haskovec/tmoney/internal/tui/dialog"

// prefillField sets a field's text and anchors the cursor after it. Assigning
// Field.Value directly leaves cursorPos at 0, which makes Backspace a dead key
// (Field.DeleteBack returns early while cursorPos <= 0) and makes every typed
// character prepend — so a pre-filled field could not be edited, only retyped.
//
// MoveCursorEnd is a no-op on anything but a text field, which is what makes
// this safe on a masked FieldDate: that overwrites digits from the first one,
// so its cursor must stay at 0. Use it for every programmatic pre-fill of an
// editable text field; the dialog.Add*Field constructors already anchor their
// own initial value.
func prefillField(f *dialog.Field, value string) {
	if f == nil {
		return
	}
	f.Value = value
	f.MoveCursorEnd()
}
