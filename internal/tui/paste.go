package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// pasteFocus identifies where a paste goes: the text field that has focus on
// the frontmost surface, or the search that is being typed in a view.
type pasteFocus struct {
	field *dialog.Field
	view  View
}

// handlePaste types pasted text into the text field that has focus: a text,
// date or combo field of the frontmost surface, or the search or filter that
// is being typed in the view. Anywhere else the paste is dropped, so pasted
// text never runs a command ("q" quits, "2" switches the view).
//
// The text goes in one rune at a time, through the same key handler that
// typing uses. So every rule for typed text applies to a paste: one sign in an
// amount, digits only in a numeric or date field, the combo filter, and what a
// surface does after each key (the preview keeps its split total in step, for
// example). A key of one rune is safe where a key of many runes is not: a
// key's String() is its text, so a paste of "esc" sent as one key would close
// the dialog. The paste stops if focus leaves the field.
func (a *App) handlePaste(content string) (tea.Model, tea.Cmd) {
	// An error on the screen goes away on the next key press, not on a paste.
	if a.err != nil {
		return a, nil
	}
	focus, _, ok := a.pasteTarget()
	if !ok {
		return a, nil
	}
	var cmds []tea.Cmd
	for _, r := range pasteText(content) {
		at, send, ok := a.pasteTarget()
		if !ok || at != focus {
			break
		}
		_, cmd := send(tea.KeyPressMsg{Code: r, Text: string(r)})
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// pasteTarget returns where a paste goes now and the key handler that types
// into it. ok is false when nothing that takes text has focus.
func (a *App) pasteTarget() (at pasteFocus, send func(tea.KeyPressMsg) (tea.Model, tea.Cmd), ok bool) {
	if e := a.frontmostModal(); e != nil {
		t, isTarget := e.modal.(textTarget)
		if !isTarget {
			return pasteFocus{}, nil, false
		}
		f := t.TextFocus()
		if f == nil {
			return pasteFocus{}, nil, false
		}
		return pasteFocus{field: f}, func(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) { return e.onKey(a, msg) }, true
	}
	if a.menubar.IsActive() {
		return pasteFocus{}, nil, false
	}
	if h := a.searchKeyHandler(); h != nil {
		return pasteFocus{view: a.currentView}, h, true
	}
	return pasteFocus{}, nil, false
}

// pasteText makes pasted text fit a one-line field. Line breaks at the end (a
// copied spreadsheet cell has one) are dropped. Any other line break or tab
// becomes a space, and other control characters are dropped. So no rune of the
// result acts as Enter, Tab or Esc.
func pasteText(s string) string {
	s = strings.TrimRight(s, "\r\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}
