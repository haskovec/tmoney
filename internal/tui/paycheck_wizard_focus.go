package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Input: the focus model that orders the wizard's Tab stops, and the key and
// mouse handlers that move focus and dispatch keystrokes to the focused field.

// ===========================================================================
// Focus model
// ===========================================================================

type wizardFocusKind int

const (
	wizardFocusField wizardFocusKind = iota
	wizardFocusRemove
	wizardFocusAddRow
	wizardFocusSave
	wizardFocusCancel
)

type wizardFocusTarget struct {
	kind    wizardFocusKind
	field   *dialog.Field
	section PaycheckSection
	line    *PaycheckLine
}

// collectFocusables returns the ordered list of focusable elements
// in the wizard. The list shape determines Tab order: header fields,
// then for each section the row cells + `+ Add` button, then Save
// and Cancel.
func (w *PaycheckWizard) collectFocusables() []wizardFocusTarget {
	out := []wizardFocusTarget{
		{kind: wizardFocusField, field: w.employerField},
		{kind: wizardFocusField, field: w.frequencyField},
		{kind: wizardFocusField, field: w.nextPaydayField},
		{kind: wizardFocusField, field: w.accountField},
		{kind: wizardFocusField, field: w.memoField},
	}
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for _, line := range w.sections[s] {
			out = append(out,
				wizardFocusTarget{kind: wizardFocusField, field: line.selectField},
				wizardFocusTarget{kind: wizardFocusField, field: line.amountField},
				wizardFocusTarget{kind: wizardFocusField, field: line.notesField},
				wizardFocusTarget{kind: wizardFocusRemove, line: line, section: s},
			)
		}
		out = append(out, wizardFocusTarget{kind: wizardFocusAddRow, section: s})
	}
	out = append(out,
		wizardFocusTarget{kind: wizardFocusSave},
		wizardFocusTarget{kind: wizardFocusCancel},
	)
	return out
}

func (w *PaycheckWizard) clampFocus() {
	focusables := w.collectFocusables()
	if w.focusIndex < 0 {
		w.focusIndex = 0
	}
	if w.focusIndex >= len(focusables) {
		w.focusIndex = len(focusables) - 1
	}
}

func (w *PaycheckWizard) focusedTarget() wizardFocusTarget {
	focusables := w.collectFocusables()
	if w.focusIndex < 0 || w.focusIndex >= len(focusables) {
		return wizardFocusTarget{}
	}
	return focusables[w.focusIndex]
}

// ===========================================================================
// Key handling
// ===========================================================================

// HandleKey dispatches a key event into the wizard and returns the
// action the parent App should take. dialog.DialogActionSubmit fires when
// Enter on Save; dialog.DialogActionCancel fires when Esc or Enter on
// Cancel. Other actions are absorbed.
func (w *PaycheckWizard) HandleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if w == nil {
		return dialog.DialogActionNone
	}
	w.errorMsg = ""
	w.clampFocus()

	keyStr := msg.String()
	switch keyStr {
	case "esc":
		return dialog.DialogActionCancel
	case "tab":
		w.focusIndex++
		w.clampFocus()
		return dialog.DialogActionNone
	case "shift+tab":
		w.focusIndex--
		w.clampFocus()
		return dialog.DialogActionNone
	case "enter":
		return w.handleEnter()
	}

	target := w.focusedTarget()
	switch target.kind {
	case wizardFocusField:
		w.dispatchFieldKey(target.field, msg)
	}
	return dialog.DialogActionNone
}

func (w *PaycheckWizard) handleEnter() dialog.DialogAction {
	target := w.focusedTarget()
	if target.kind == wizardFocusField {
		// Enter on a section-line select field that is parked on the
		// [+ Add new category…] sentinel diverts into the inline create-
		// category sub-dialog instead of advancing focus.
		if line := w.lineForSelectField(target.field); line != nil && line.IsAddNew() {
			return dialog.DialogActionAddNew
		}
		// Otherwise: advance focus (don't activate).
		w.focusIndex++
		w.clampFocus()
		return dialog.DialogActionNone
	}
	return w.activate(target)
}

// lineForSelectField returns the PaycheckLine that owns f when f is one
// of a section-line's select fields, or nil when f is a header field (or
// not a select field at all).
func (w *PaycheckWizard) lineForSelectField(f *dialog.Field) *PaycheckLine {
	if f == nil {
		return nil
	}
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for _, line := range w.sections[s] {
			if line.selectField == f {
				return line
			}
		}
	}
	return nil
}

func (w *PaycheckWizard) dispatchFieldKey(f *dialog.Field, msg tea.KeyPressMsg) {
	if f == nil {
		return
	}
	switch f.Type {
	case dialog.FieldText:
		w.dispatchTextFieldKey(f, msg)
	case dialog.FieldSelect:
		w.dispatchSelectFieldKey(f, msg)
	case dialog.FieldDate:
		w.dispatchDateFieldKey(f, msg)
	}
}

func (w *PaycheckWizard) dispatchTextFieldKey(f *dialog.Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "backspace":
		f.DeleteBack()
	case "delete":
		f.DeleteForward()
	case "left":
		f.MoveCursorLeft()
	case "right":
		f.MoveCursorRight()
	case "home", "ctrl+a":
		f.MoveCursorHome()
	case "end", "ctrl+e":
		f.MoveCursorEnd()
	case "space":
		f.InsertChar(' ')
	default:
		if msg.Text != "" {
			for _, r := range msg.Text {
				f.InsertChar(r)
			}
		}
	}
}

func (w *PaycheckWizard) dispatchDateFieldKey(f *dialog.Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "left":
		f.DateCursorLeft()
		return
	case "right":
		f.DateCursorRight()
		return
	case "home", "ctrl+a":
		f.DateCursorHome()
		return
	case "end", "ctrl+e":
		f.DateCursorEnd()
		return
	case "backspace":
		f.DateBackspace()
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			if r >= '0' && r <= '9' {
				f.DateOverwriteDigit(r)
			}
		}
	}
}

func (w *PaycheckWizard) dispatchSelectFieldKey(f *dialog.Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up":
		f.SelectPrev()
	case "down":
		f.SelectNext()
	}
}

// ===========================================================================
// Mouse handling
// ===========================================================================

// RenderedHeight returns the wizard's rendered height including
// border + padding. Computed by counting newlines in a fresh render
// (the wizard caches no layout state, so this is the simplest
// reliable approach).
func (w *PaycheckWizard) renderedHeightFor(styles widget.Styles) int {
	rendered := w.Render(styles)
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

// dialogBounds returns the screen-space bounding box of the wizard
// when centered on a screenWidth × screenHeight terminal.
func (w *PaycheckWizard) dialogBounds(styles widget.Styles, screenWidth, screenHeight int) (startCol, startRow, endCol, endRow int) {
	overlayHeight := w.renderedHeightFor(styles)
	overlayWidth := w.width
	startCol = max((screenWidth-overlayWidth)/2, 0)
	startRow = max((screenHeight-overlayHeight)/2, 0)
	endCol = startCol + overlayWidth
	endRow = startRow + overlayHeight
	return
}

// HandleMouse processes a mouse event and returns the resulting
// action. The wizard records hit zones during Render; this routine
// translates a click to a focus target and acts on it.
func (w *PaycheckWizard) HandleMouse(msg tea.MouseMsg, styles widget.Styles, screenWidth, screenHeight int) dialog.DialogAction {
	if w == nil {
		return dialog.DialogActionNone
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return dialog.DialogActionNone
	}

	startCol, startRow, endCol, endRow := w.dialogBounds(styles, screenWidth, screenHeight)
	m := msg.Mouse()
	if m.X < startCol || m.X >= endCol || m.Y < startRow || m.Y >= endRow {
		return dialog.DialogActionNone
	}

	// Convert screen coords to content-local (inside border + padding).
	// styles.Dialog has Border(1) + Padding(1,2) by convention; match
	// the same offsets used by the standard dialog.Dialog.HandleMouse path.
	localX := m.X - startCol - 3
	localY := m.Y - startRow - 2

	for _, zone := range w.hitZones {
		if zone.row != localY {
			continue
		}
		if localX < zone.colMin || localX >= zone.colMax {
			continue
		}
		return w.activate(zone.target)
	}
	return dialog.DialogActionNone
}

// activate executes the action associated with a focus target. Used
// by both Enter on a focused element and HandleMouse on a clicked
// element.
func (w *PaycheckWizard) activate(target wizardFocusTarget) dialog.DialogAction {
	switch target.kind {
	case wizardFocusSave:
		w.focusToTarget(target)
		return dialog.DialogActionSubmit
	case wizardFocusCancel:
		return dialog.DialogActionCancel
	case wizardFocusAddRow:
		line := w.addLineForSection(target.section)
		if line == nil {
			return dialog.DialogActionNone
		}
		// Move focus onto the new row's select field for convenience.
		focusables := w.collectFocusables()
		for i, f := range focusables {
			if f.kind == wizardFocusField && f.field == line.selectField {
				w.focusIndex = i
				break
			}
		}
		return dialog.DialogActionNone
	case wizardFocusRemove:
		w.RemoveRow(target.line)
		w.clampFocus()
		return dialog.DialogActionNone
	case wizardFocusField:
		w.focusToTarget(target)
		return dialog.DialogActionNone
	}
	return dialog.DialogActionNone
}

// focusToTarget walks the current focusables and moves focusIndex
// onto the matching target. No-op when not found.
func (w *PaycheckWizard) focusToTarget(target wizardFocusTarget) {
	focusables := w.collectFocusables()
	for i, f := range focusables {
		if f.kind != target.kind {
			continue
		}
		if target.kind == wizardFocusField && f.field != target.field {
			continue
		}
		if target.kind == wizardFocusRemove && f.line != target.line {
			continue
		}
		if (target.kind == wizardFocusAddRow) && f.section != target.section {
			continue
		}
		w.focusIndex = i
		return
	}
}
