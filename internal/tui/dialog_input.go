package tui

import (
	tea "charm.land/bubbletea/v2"
)

// HandleKey processes a key event and returns the resulting action.
func (d *Dialog) HandleKey(msg tea.KeyPressMsg) DialogAction {
	keyStr := msg.String()

	if field := d.FocusedField(); field != nil && field.Type == FieldCombo {
		if handled, action := d.handleComboNavigationKey(field, keyStr); handled {
			return action
		}
	}

	switch keyStr {
	case "esc":
		return DialogActionCancel
	case "tab":
		d.FocusNext()
		return DialogActionNone
	case "shift+tab":
		d.FocusPrev()
		return DialogActionNone
	case "enter":
		if d.IsFocusOnButton() {
			btnIdx := d.FocusedButtonIndex()
			if btnIdx >= 0 && btnIdx < len(d.buttons) {
				btn := d.buttons[btnIdx]
				if btn.Primary {
					return DialogActionSubmit
				}
				if btn.Action != DialogActionNone {
					return btn.Action
				}
			}
			return DialogActionCancel
		}
		d.FocusNext()
		return DialogActionNone
	}

	field := d.FocusedField()
	if field == nil {
		return DialogActionNone
	}

	switch field.Type {
	case FieldText:
		d.handleTextFieldKey(field, msg)
	case FieldSelect:
		d.handleSelectFieldKey(field, msg)
	case FieldRadio:
		d.handleRadioFieldKey(field, msg)
	case FieldCheckbox:
		d.handleCheckboxFieldKey(field, msg)
	case FieldList:
		d.handleListFieldKey(field, msg)
	case FieldDate:
		d.handleDateFieldKey(field, msg)
	case FieldCombo:
		d.handleComboFieldKey(field, msg)
	}
	return DialogActionNone
}

func (d *Dialog) handleTextFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "backspace":
		field.DeleteBack()
		field.Error = ""
		return
	case "delete":
		field.DeleteForward()
		field.Error = ""
		return
	case "left":
		field.MoveCursorLeft()
		return
	case "right":
		field.MoveCursorRight()
		return
	case "home", "ctrl+a":
		field.MoveCursorHome()
		return
	case "end", "ctrl+e":
		field.MoveCursorEnd()
		return
	case "space":
		field.InsertChar(' ')
		field.Error = ""
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			field.InsertChar(r)
		}
		field.Error = ""
	}
}

func (d *Dialog) handleSelectFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up":
		field.SelectPrev()
		field.Error = ""
	case "down":
		field.SelectNext()
		field.Error = ""
	}
}

func (d *Dialog) handleRadioFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up", "left":
		field.SelectPrev()
		field.Error = ""
	case "down", "right":
		field.SelectNext()
		field.Error = ""
	}
}

func (d *Dialog) handleCheckboxFieldKey(field *Field, msg tea.KeyPressMsg) {
	if msg.String() == "space" || msg.Text == " " {
		field.Toggle()
		field.Error = ""
	}
}

func (d *Dialog) handleDateFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "left":
		field.dateCursorLeft()
		return
	case "right":
		field.dateCursorRight()
		return
	case "home", "ctrl+a":
		field.dateCursorHome()
		return
	case "end", "ctrl+e":
		field.dateCursorEnd()
		return
	case "backspace":
		field.dateBackspace()
		field.Error = ""
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			if r >= '0' && r <= '9' {
				field.dateOverwriteDigit(r)
				field.Error = ""
			}
		}
	}
}

func (d *Dialog) handleListFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up":
		field.SelectPrev()
		field.Error = ""
	case "down":
		field.SelectNext()
		field.Error = ""
	}
}

func (d *Dialog) handleComboFieldKey(field *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "up":
		field.comboHighlightUp()
		field.Error = ""
		return
	case "down":
		field.comboHighlightDown()
		field.Error = ""
		return
	case "backspace":
		field.comboQueryBackspace()
		field.Error = ""
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			field.comboQueryAppend(r)
		}
		field.Error = ""
	}
}
