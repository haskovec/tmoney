package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// Input: key dispatch to the focused cell, Tab order across rows and buttons,
// and the content-local mouse hit-testing HandleMouseLocal performs.

// HandleKey processes a key event and returns the resulting action.
func (sd *SplitDialog) HandleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if p := sd.focusedPicker(); p != nil {
		if handled, action := sd.handlePickerNavigationKey(p, msg.String()); handled {
			return action
		}
	}

	switch msg.String() {
	case "esc":
		return dialog.DialogActionCancel
	case "ctrl+d":
		if sd.focus == splitFocusRows && len(sd.rows) > 1 {
			sd.removeRow(sd.rowIndex)
			sd.errorMsg = ""
		}
		return dialog.DialogActionNone
	case "tab":
		sd.focusNext()
		return dialog.DialogActionNone
	case "shift+tab":
		sd.focusPrev()
		return dialog.DialogActionNone
	case "enter":
		return sd.handleEnter()
	}

	// dialog.Field-specific input when focus is on rows
	if sd.focus == splitFocusRows && sd.rowIndex >= 0 && sd.rowIndex < len(sd.rows) {
		sd.handleRowFieldKey(msg)
	}

	return dialog.DialogActionNone
}

// handlePickerNavigationKey handles the keys that close the focused Category
// picker, the same as handleComboNavigationKey does for a Dialog combo: Esc
// clears a typed query; Enter on the AddNew row returns
// DialogActionAddNew; Enter, Tab, and Shift+Tab commit the highlighted entry
// and move focus. It reports handled=false for every other key.
func (sd *SplitDialog) handlePickerNavigationKey(p *dialog.Field, keyStr string) (handled bool, action dialog.DialogAction) {
	switch keyStr {
	case "esc":
		if p.Query != "" {
			p.ClearComboQuery()
			return true, dialog.DialogActionNone
		}
	case "enter":
		if p.IsAddNewHighlighted() {
			return true, dialog.DialogActionAddNew
		}
		sd.commitPicker()
		sd.focusNext()
		return true, dialog.DialogActionNone
	case "tab":
		sd.commitPicker()
		sd.focusNext()
		return true, dialog.DialogActionNone
	case "shift+tab":
		sd.commitPicker()
		sd.focusPrev()
		return true, dialog.DialogActionNone
	}
	return false, dialog.DialogActionNone
}

// handleEnter processes Enter key based on current focus.
func (sd *SplitDialog) handleEnter() dialog.DialogAction {
	switch sd.focus {
	case splitFocusSaveBtn:
		if err := sd.validate(); err != nil {
			sd.errorMsg = err.Error()
			return dialog.DialogActionNone
		}
		sd.errorMsg = ""
		return dialog.DialogActionSubmit
	case splitFocusCancelBtn:
		return dialog.DialogActionCancel
	case splitFocusAddBtn:
		sd.addRow()
		sd.focus = splitFocusRows
		sd.rowIndex = len(sd.rows) - 1
		sd.fieldFocus = splitFieldCategory
		sd.errorMsg = ""
		return dialog.DialogActionNone
	case splitFocusRows:
		// Advance to next field within row, or next row, or add button.
		// Enter on a Category cell never gets here: the picker handles it.
		sd.focusNext()
		return dialog.DialogActionNone
	}
	return dialog.DialogActionNone
}

// handleRowFieldKey handles key input for the currently focused row field.
func (sd *SplitDialog) handleRowFieldKey(msg tea.KeyPressMsg) {
	row := &sd.rows[sd.rowIndex]

	switch sd.fieldFocus {
	case splitFieldCategory:
		if p := sd.focusedPicker(); p != nil {
			p.HandleComboKey(msg)
		}
	case splitFieldAmount:
		handleFieldTextKey(&row.amountField, msg)
	case splitFieldMemo:
		handleFieldTextKey(&row.memoField, msg)
	}
}

// handleFieldTextKey applies text editing keys to a dialog.Field.
func handleFieldTextKey(f *dialog.Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "backspace":
		f.DeleteBack()
		return
	case "delete":
		f.DeleteForward()
		return
	case "left":
		f.MoveCursorLeft()
		return
	case "right":
		f.MoveCursorRight()
		return
	case "home", "ctrl+a":
		f.MoveCursorHome()
		return
	case "end", "ctrl+e":
		f.MoveCursorEnd()
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			f.InsertChar(r)
		}
	}
}

// focusNext advances focus to the next focusable element.
// Order: row0.category -> row0.amount -> row0.memo -> row1.category -> ... -> addBtn -> saveBtn -> cancelBtn -> wrap
func (sd *SplitDialog) focusNext() {
	switch sd.focus {
	case splitFocusRows:
		// Try next field in current row
		if sd.fieldFocus < splitFieldMemo {
			sd.fieldFocus++
			return
		}
		// Try next row
		if sd.rowIndex < len(sd.rows)-1 {
			sd.rowIndex++
			sd.fieldFocus = splitFieldCategory
			return
		}
		// Move to add button
		sd.focus = splitFocusAddBtn
	case splitFocusAddBtn:
		sd.focus = splitFocusSaveBtn
	case splitFocusSaveBtn:
		sd.focus = splitFocusCancelBtn
	case splitFocusCancelBtn:
		// Wrap to first row
		sd.focus = splitFocusRows
		sd.rowIndex = 0
		sd.fieldFocus = splitFieldCategory
	}
}

// focusPrev moves focus to the previous focusable element.
func (sd *SplitDialog) focusPrev() {
	switch sd.focus {
	case splitFocusRows:
		// Try previous field in current row
		if sd.fieldFocus > splitFieldCategory {
			sd.fieldFocus--
			return
		}
		// Try previous row
		if sd.rowIndex > 0 {
			sd.rowIndex--
			sd.fieldFocus = splitFieldMemo
			return
		}
		// Wrap to the last button (Cancel)
		sd.focus = splitFocusCancelBtn
	case splitFocusAddBtn:
		// Back to last row, last field
		sd.focus = splitFocusRows
		sd.rowIndex = len(sd.rows) - 1
		sd.fieldFocus = splitFieldMemo
	case splitFocusSaveBtn:
		sd.focus = splitFocusAddBtn
	case splitFocusCancelBtn:
		sd.focus = splitFocusSaveBtn
	}
}

// HandleMouseLocal applies a left-click at content-local coordinates
// (relative to the first content line inside the split panel's
// border+padding) and returns the resulting action. The row layout
// mirrors Render: title(0), sep(1), summary(2), sep(3), blank(4),
// headers(5), sep(6), then one line per split row (with an open picker's
// panel under the focused row), the [+ Add split] line, blank, imbalance,
// blank, optional error+blank, sep, button row.
func (sd *SplitDialog) HandleMouseLocal(localX, localY int) dialog.DialogAction {
	contentWidth := max(sd.width-dialog.DialogHorizontalOverhead, 10)
	if localY < 0 || localX < 0 || localX >= contentWidth {
		return dialog.DialogActionNone
	}

	// Title row: [x] close button is right-aligned ("[x]" = 3 chars).
	if localY == 0 {
		if localX >= contentWidth-3 {
			return dialog.DialogActionCancel
		}
		return dialog.DialogActionNone
	}

	const rowsStart = 7

	// An open Category picker draws its panel under the focused row, which
	// pushes the rows below it and everything after them down.
	picker := sd.focusedPicker()
	panelRows := 0
	if picker != nil {
		panelRows = picker.ComboPanelRows()
	}

	// Split rows: clicking one focuses that row and the field under the
	// cursor (category / amount / memo columns). A click on a panel line
	// picks that entry, the same as Enter.
	if localY >= rowsStart && localY < rowsStart+len(sd.rows)+panelRows {
		rel := localY - rowsStart
		if picker != nil {
			panelStart := sd.rowIndex + 1
			switch {
			case rel >= panelStart && rel < panelStart+panelRows:
				return sd.clickPickerLine(picker, picker.ComboPanelLineAt(rel-panelStart))
			case rel >= panelStart+panelRows:
				rel -= panelRows
			}
		}
		field := sd.columnFieldAt(localX, contentWidth)
		if picker != nil && rel == sd.rowIndex && field == splitFieldCategory {
			return dialog.DialogActionNone // the open picker's own cell
		}
		sd.commitPicker()
		sd.focus = splitFocusRows
		sd.rowIndex = rel
		sd.fieldFocus = field
		return dialog.DialogActionNone
	}

	// The add line and the buttons below move focus off an open picker, so
	// they commit it first, the same as Tab.
	addLine := rowsStart + len(sd.rows) + panelRows
	if localY == addLine {
		sd.commitPicker()
		sd.addRow()
		sd.focus = splitFocusRows
		sd.rowIndex = len(sd.rows) - 1
		sd.fieldFocus = splitFieldCategory
		sd.errorMsg = ""
		return dialog.DialogActionNone
	}

	// Button row sits after add-split, blank, imbalance, blank (+ optional
	// error + blank), and a separator.
	buttonsLine := addLine + 5
	if sd.errorMsg != "" {
		buttonsLine += 2
	}
	if localY == buttonsLine {
		return sd.hitTestButtonRow(localX, contentWidth)
	}

	return dialog.DialogActionNone
}

// clickPickerLine applies a click on line of the open Category picker's
// panel: a pick commits it and moves focus to Amount, and the AddNew row
// returns DialogActionAddNew with focus kept on the cell.
func (sd *SplitDialog) clickPickerLine(p *dialog.Field, line int) dialog.DialogAction {
	picked, addNew := p.ClickComboLine(line)
	switch {
	case addNew:
		return dialog.DialogActionAddNew
	case picked:
		sd.commitPicker()
		sd.focusNext()
	}
	return dialog.DialogActionNone
}

// columnFieldAt maps an x offset within a split row to its field. The
// layout matches Render: category occupies [0, catColW), amount occupies
// a 14-wide column after a single-space gap, and memo fills the rest.
func (sd *SplitDialog) columnFieldAt(localX, contentWidth int) splitFieldFocus {
	catColW := contentWidth / 3
	amtStart := catColW + 1
	amtEnd := amtStart + 14
	switch {
	case localX < catColW:
		return splitFieldCategory
	case localX >= amtStart && localX < amtEnd:
		return splitFieldAmount
	case localX >= amtEnd+1:
		return splitFieldMemo
	default:
		return splitFieldCategory
	}
}

// hitTestButtonRow maps an x offset on the button row to Save or Cancel,
// using the same shared layout as Render. A click on Save validates
// first: an imbalanced/invalid dialog surfaces the error and stays open.
func (sd *SplitDialog) hitTestButtonRow(localX, contentWidth int) dialog.DialogAction {
	switch dialog.ButtonRowHitTest([]string{"Save", "Cancel"}, localX, contentWidth) {
	case 0: // Save
		// Save the entry an open picker shows, not the one it was opened on.
		sd.commitPicker()
		if err := sd.validate(); err != nil {
			sd.errorMsg = err.Error()
			return dialog.DialogActionNone
		}
		sd.errorMsg = ""
		sd.focus = splitFocusSaveBtn
		return dialog.DialogActionSubmit
	case 1: // Cancel
		sd.focus = splitFocusCancelBtn
		return dialog.DialogActionCancel
	}
	return dialog.DialogActionNone
}
