package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// assertPrefillEditable asserts that a pre-filled text field can be edited.
// A field whose Value was assigned directly keeps cursorPos 0, and then
// Backspace is a dead key and a typed character prepends — so the user can
// only retype the value, never adjust it.
func assertPrefillEditable(t *testing.T, f *dialog.Field, label string) {
	t.Helper()
	if f == nil {
		t.Fatalf("%s: nil field", label)
	}
	if f.Value == "" {
		t.Fatalf("%s: not pre-filled, so there is nothing to anchor", label)
	}
	if got, want := f.CursorPos(), len([]rune(f.Value)); got != want {
		t.Errorf("%s: CursorPos() = %d, want %d (end of %q)", label, got, want, f.Value)
	}

	original := f.Value
	runes := []rune(original)
	f.DeleteBack()
	if got, want := f.Value, string(runes[:len(runes)-1]); got != want {
		t.Errorf("%s: after Backspace = %q, want %q", label, got, want)
	}
	f.InsertChar('7')
	if got, want := f.Value, string(runes[:len(runes)-1])+"7"; got != want {
		t.Errorf("%s: after typing = %q, want %q — the edit landed in the wrong place", label, got, want)
	}
	f.Value = original
	f.MoveCursorEnd()
}

// TestPrefillField_AnchorsTextButNotDate covers the helper itself: a text field
// ends anchored, and a masked date field must stay at 0 because
// DateOverwriteDigit writes digits from the first position.
func TestPrefillField_AnchorsTextButNotDate(t *testing.T) {
	text := &dialog.Field{Type: dialog.FieldText}
	prefillField(text, "123.45")
	if got, want := text.CursorPos(), 6; got != want {
		t.Errorf("text field CursorPos() = %d, want %d", got, want)
	}

	date := &dialog.Field{Type: dialog.FieldDate, DateMask: dialog.DateMaskUS}
	prefillField(date, "05/15/2026")
	if got := date.CursorPos(); got != 0 {
		t.Errorf("date field CursorPos() = %d, want 0 (the mask overwrites from the first digit)", got)
	}
	if date.Value != "05/15/2026" {
		t.Errorf("date field Value = %q, want the assigned value", date.Value)
	}

	// A nil field must not panic.
	prefillField(nil, "x")
}
