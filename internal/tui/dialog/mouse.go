package dialog

import (
	tea "charm.land/bubbletea/v2"
)

// DialogHitZone represents the type of element hit by a mouse click within a dialog.
type DialogHitZone int

const (
	// DialogHitNone means the click did not hit any interactive element.
	DialogHitNone DialogHitZone = iota
	// DialogHitCloseButton means the [x] close button was clicked.
	DialogHitCloseButton
	// DialogHitField means a form field was clicked.
	DialogHitField
	// DialogHitButton means an action button was clicked.
	DialogHitButton
)

// DialogHitResult describes what was hit by a mouse click within a dialog.
type DialogHitResult struct {
	// Zone is the type of element hit.
	Zone DialogHitZone
	// FieldIndex is the index of the field hit (valid when Zone == DialogHitField).
	FieldIndex int
	// ButtonIndex is the index of the button hit (valid when Zone == DialogHitButton).
	ButtonIndex int
	// ListItemIndex is the absolute item index for FieldList clicks (-1 if not applicable).
	ListItemIndex int
	// ContentX is the x offset within the hit element (for text cursor positioning).
	ContentX int
}

// HitTestContent maps dialog-content-local coordinates to the element at that position.
// x and y are 0-based coordinates relative to the first content line inside border+padding.
// contentWidth is the usable content width (d.width - dialogHorizontalOverhead).
func (d *Dialog) HitTestContent(x, y, contentWidth int) DialogHitResult {
	none := DialogHitResult{Zone: DialogHitNone, ListItemIndex: -1}

	if y < 0 || x < 0 || x >= contentWidth {
		return none
	}

	row := 0

	// Title row (row 0)
	if y == row {
		// Close button is right-aligned: "[x]" = 3 chars at end
		if x >= contentWidth-3 {
			return DialogHitResult{Zone: DialogHitCloseButton, ListItemIndex: -1}
		}
		return none
	}
	row++

	// Separator (row 1)
	if y == row {
		return none
	}
	row++

	// Neutral message body lines (non-interactive)
	if d.message != "" {
		msgRows := d.messageLineCount() + 1 // body lines + trailing blank
		if y >= row && y < row+msgRows {
			return none
		}
		row += msgRows
	}

	// Fields
	for i, field := range d.fields {
		if field.Hidden {
			continue
		}
		// Blank line before field
		if y == row {
			return none
		}
		row++

		// Field content rows
		contentRows := d.fieldContentRows(field)
		if y >= row && y < row+contentRows {
			result := DialogHitResult{
				Zone:          DialogHitField,
				FieldIndex:    i,
				ListItemIndex: -1,
				ContentX:      x,
			}
			if field.Type == FieldList && y > row {
				// y == row is the label line; y > row is a list item
				itemRow := y - row - 1 // 0-based row within visible items
				scrollOffset := listScrollOffset(field)
				absIdx := scrollOffset + itemRow
				if absIdx >= 0 && absIdx < len(field.Options) {
					result.ListItemIndex = absIdx
				}
			}
			return result
		}
		row += contentRows

		// Error row
		if field.Error != "" {
			if y == row {
				return DialogHitResult{Zone: DialogHitField, FieldIndex: i, ListItemIndex: -1, ContentX: x}
			}
			row++
		}
	}

	// Blank line after fields
	if d.hasVisibleFields() {
		if y == row {
			return none
		}
		row++
	}

	// Dialog-level error
	if d.errorMsg != "" {
		// Error line + blank line
		if y == row || y == row+1 {
			return none
		}
		row += 2
	}

	// Separator before buttons
	if y == row {
		return none
	}
	row++

	// Button row
	if y == row && len(d.buttons) > 0 {
		return d.hitTestButtonRow(x, contentWidth)
	}

	return none
}

// hitTestButtonRow maps an x coordinate to a button in the button row.
func (d *Dialog) hitTestButtonRow(x, contentWidth int) DialogHitResult {
	none := DialogHitResult{Zone: DialogHitNone, ListItemIndex: -1}
	if len(d.buttons) == 0 {
		return none
	}

	// Calculate button widths (matching renderButtonRow)
	btnWidths := make([]int, len(d.buttons))
	totalBtnWidth := 0
	for i, btn := range d.buttons {
		w := len([]rune(btn.Label)) + 4 // "[ " + label + " ]"
		btnWidths[i] = w
		totalBtnWidth += w
	}

	numGaps := len(d.buttons) + 1
	totalGapSpace := max(contentWidth-totalBtnWidth, numGaps)
	gapSize := totalGapSpace / numGaps
	extraGap := totalGapSpace % numGaps

	pos := 0
	for i := range d.buttons {
		gap := gapSize
		if i < extraGap {
			gap++
		}
		pos += gap
		if x >= pos && x < pos+btnWidths[i] {
			return DialogHitResult{
				Zone:          DialogHitButton,
				ButtonIndex:   i,
				ListItemIndex: -1,
			}
		}
		pos += btnWidths[i]
	}

	return none
}

// HandleMouse processes a mouse event and returns the resulting action.
// screenWidth and screenHeight are the terminal dimensions for computing dialog position.
func (d *Dialog) HandleMouse(msg tea.MouseMsg, screenWidth, screenHeight int) DialogAction {
	startCol, startRow, endCol, endRow := d.DialogBounds(screenWidth, screenHeight)
	contentWidth := max(d.width-DialogHorizontalOverhead, 10)
	m := msg.Mouse()

	// Handle wheel events on focused list field
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		// Only scroll if wheel is within dialog bounds
		if m.X >= startCol && m.X < endCol && m.Y >= startRow && m.Y < endRow {
			field := d.FocusedField()
			if field != nil && field.Type == FieldList {
				if m.Button == tea.MouseWheelUp {
					field.SelectPrev()
				} else {
					field.SelectNext()
				}
			}
		}
		return DialogActionNone
	}

	// Only handle left-click press
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return DialogActionNone
	}

	// Check if click is within dialog bounds
	if m.X < startCol || m.X >= endCol || m.Y < startRow || m.Y >= endRow {
		return DialogActionNone
	}

	// Convert screen coords to content-local coords
	// border (1) + padding (2) = 3 horizontal offset
	// border (1) + padding (1) = 2 vertical offset
	localX := m.X - startCol - 3
	localY := m.Y - startRow - 2

	hit := d.HitTestContent(localX, localY, contentWidth)

	switch hit.Zone {
	case DialogHitCloseButton:
		return DialogActionCancel

	case DialogHitField:
		if hit.FieldIndex >= 0 && hit.FieldIndex < len(d.fields) {
			field := d.fields[hit.FieldIndex]
			if !field.Hidden {
				d.focusIndex = hit.FieldIndex

				switch field.Type {
				case FieldCheckbox:
					field.Toggle()
				case FieldList:
					if hit.ListItemIndex >= 0 && hit.ListItemIndex < len(field.Options) {
						field.SelectedIndex = hit.ListItemIndex
					}
				case FieldText:
					// Position cursor based on click position within text field
					labelWidth := d.maxLabelWidth()
					textStart := labelWidth + 1 + 2 + 2 // label + colon + gap + "[ "
					cursorPos := max(hit.ContentX-textStart, 0)
					field.cursorPos = min(cursorPos, len([]rune(field.Value)))
				}
			}
		}
		return DialogActionNone

	case DialogHitButton:
		if hit.ButtonIndex >= 0 && hit.ButtonIndex < len(d.buttons) {
			d.focusIndex = len(d.fields) + hit.ButtonIndex
			if d.buttons[hit.ButtonIndex].Primary {
				return DialogActionSubmit
			}
			return DialogActionCancel
		}
	}

	return DialogActionNone
}
