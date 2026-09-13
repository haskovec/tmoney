package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Rendering: the overlay string, its per-row and per-field pieces, and the
// hit zones Render records for HandleMouse.


// Render returns the wizard's overlay-ready string. As a side-effect
// it rebuilds w.hitZones so HandleMouse can dispatch clicks.
func (w *PaycheckWizard) Render(styles widget.Styles) string {
	if w == nil || !w.visible {
		return ""
	}
	w.clampFocus()
	focused := w.focusedTarget()

	contentWidth := max(w.width-dialog.DialogHorizontalOverhead, 40)
	w.hitZones = w.hitZones[:0]

	// fillStyle is used for both internal gap spaces and trailing
	// padding so every line of the wizard's content is uniformly
	// covered with the dialog's background. Without this, gaps
	// between locally-styled elements (title vs [x], button vs
	// button, empty trailing text-field padding, etc.) show through
	// to the terminal background.
	fillStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Foreground(styles.Dialog.GetForeground())
	gap := func(n int) string {
		if n <= 0 {
			return ""
		}
		return fillStyle.Render(strings.Repeat(" ", n))
	}
	padToWidth := func(s string) string {
		extra := contentWidth - lipgloss.Width(s)
		if extra <= 0 {
			return s
		}
		return s + gap(extra)
	}

	var lines []string
	addLine := func(s string) {
		lines = append(lines, padToWidth(s))
	}

	// Title row with [x] close button on the right.
	closeBtn := styles.Muted.Render("[x]")
	title := "Paycheck Schedule"
	if w.editSchedule != nil {
		title = "Edit Paycheck Schedule"
	}
	titleText := styles.DialogTitle.Render(title)
	titleGap := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(closeBtn), 1)
	addLine(titleText + gap(titleGap) + closeBtn)
	// Hit zone for [x]: same row as title, right edge.
	w.hitZones = append(w.hitZones, wizardHitZone{
		row:    len(lines) - 1,
		colMin: contentWidth - lipgloss.Width(closeBtn),
		colMax: contentWidth,
		target: wizardFocusTarget{kind: wizardFocusCancel},
	})

	addLine(fillStyle.Render(strings.Repeat("─", contentWidth)))
	addLine("")

	// Header rows.
	headerFields := []*dialog.Field{w.employerField, w.frequencyField, w.nextPaydayField, w.accountField, w.memoField}
	for _, f := range headerFields {
		row, zones := w.renderFieldRow(styles, fillStyle, f, focused.field == f, contentWidth, len(lines))
		addLine(row)
		w.hitZones = append(w.hitZones, zones...)
	}
	addLine("")

	// Sections.
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		addLine(fillStyle.Render(styles.Bold.Render(s.Title())))
		addLine(fillStyle.Render(strings.Repeat("─", contentWidth)))

		for _, line := range w.sections[s] {
			row, zones := w.renderLine(styles, fillStyle, line, focused, contentWidth, len(lines))
			addLine(row)
			w.hitZones = append(w.hitZones, zones...)
		}

		addLabel := s.addRowLabel()
		var addRow string
		if focused.kind == wizardFocusAddRow && focused.section == s {
			addRow = "  " + styles.DialogButtonFocused.Render(addLabel)
		} else {
			addRow = "  " + styles.DialogButton.Render(addLabel)
		}
		w.hitZones = append(w.hitZones, wizardHitZone{
			row:    len(lines),
			colMin: 2,
			colMax: 2 + lipgloss.Width(addLabel),
			target: wizardFocusTarget{kind: wizardFocusAddRow, section: s},
		})
		addLine(addRow)
		addLine("")
	}

	// Net total.
	total := w.computeTotal()
	depositName := ""
	if w.accountField != nil && w.accountField.SelectedIndex >= 0 && w.accountField.SelectedIndex < len(w.accountField.Options) {
		depositName = w.accountField.Options[w.accountField.SelectedIndex]
	}
	addLine(fillStyle.Render(strings.Repeat("─", contentWidth)))
	totalLabel := "Net to deposit account"
	if depositName != "" {
		totalLabel = "Net to " + depositName
	}
	addLine(fillStyle.Render(fmt.Sprintf("%s: %s", totalLabel, formatDashboardMoney(total))))
	addLine("")

	// Error.
	if w.errorMsg != "" {
		addLine(styles.Error.Render(w.errorMsg))
		addLine("")
	}

	// Buttons. Save sits on the left (Primary, default) so a keyboard
	// user tabbing through the form lands on it first; matches the
	// project convention from dialog.NewDialog.
	addLine(fillStyle.Render(strings.Repeat("─", contentWidth)))
	saveText := "[ Save ]"
	cancelText := "[ Cancel ]"
	var saveLabel, cancelLabel string
	if focused.kind == wizardFocusSave {
		saveLabel = styles.DialogButtonFocused.Render(saveText)
	} else {
		saveLabel = styles.DialogButton.Render(saveText)
	}
	if focused.kind == wizardFocusCancel {
		cancelLabel = styles.DialogButtonFocused.Render(cancelText)
	} else {
		cancelLabel = styles.DialogButton.Render(cancelText)
	}
	// Center the Save / Cancel pair together. Save sits on the left
	// of the pair (Primary, default focus); a short gap separates it
	// from Cancel.
	innerGap := 4
	pairWidth := lipgloss.Width(saveLabel) + innerGap + lipgloss.Width(cancelLabel)
	leftPad := max((contentWidth-pairWidth)/2, 0)
	rightPad := max(contentWidth-leftPad-pairWidth, 0)
	buttonsRow := gap(leftPad) + saveLabel + gap(innerGap) + cancelLabel + gap(rightPad)
	w.hitZones = append(w.hitZones,
		wizardHitZone{
			row:    len(lines),
			colMin: leftPad,
			colMax: leftPad + lipgloss.Width(saveText),
			target: wizardFocusTarget{kind: wizardFocusSave},
		},
		wizardHitZone{
			row:    len(lines),
			colMin: leftPad + lipgloss.Width(saveLabel) + innerGap,
			colMax: leftPad + lipgloss.Width(saveLabel) + innerGap + lipgloss.Width(cancelText),
			target: wizardFocusTarget{kind: wizardFocusCancel},
		},
	)
	addLine(buttonsRow)

	content := strings.Join(lines, "\n")
	// Re-emit the dialog's outer fg + bg after inner SGR resets so
	// styled spans (Muted "[x]", Placeholder "Optional", etc.) don't
	// punch holes through the panel and raw text after a styled span
	// keeps the theme's dialog.fg — matches *dialog.Dialog.Render.
	content = widget.RepaintDialog(content)
	return styles.Dialog.Width(w.width).Render(content)
}

// padFill pads s with styled spaces (fill background) up to n cells.
func padFill(s string, n int, fill lipgloss.Style) string {
	extra := n - lipgloss.Width(s)
	if extra <= 0 {
		return s
	}
	return s + fill.Render(strings.Repeat(" ", extra))
}

// renderFieldRow renders a single labeled scalar field and returns
// the rendered string plus any hit zones for the value cell. Labels
// are right-aligned to labelW so the colons line up vertically (the
// same convention the generic *dialog.Dialog uses for its form fields).
func (w *PaycheckWizard) renderFieldRow(styles widget.Styles, fill lipgloss.Style, f *dialog.Field, focused bool, contentWidth, row int) (string, []wizardHitZone) {
	if f == nil {
		return "", nil
	}
	labelW := w.headerLabelWidth()
	gapStr := "  "
	rawLabel := f.Label + ":"
	labelRuneLen := lipgloss.Width(rawLabel)
	padLeft := max(labelW-labelRuneLen, 0)
	label := fill.Render(strings.Repeat(" ", padLeft) + rawLabel)

	valueWidth := contentWidth - labelW - len(gapStr)
	value := w.renderFieldValue(styles, fill, f, focused, valueWidth)
	out := label + fill.Render(gapStr) + value
	zones := []wizardHitZone{{
		row:    row,
		colMin: labelW + len(gapStr),
		colMax: labelW + len(gapStr) + valueWidth,
		target: wizardFocusTarget{kind: wizardFocusField, field: f},
	}}
	return out, zones
}

// headerLabelWidth returns the column width to use when right-aligning
// header field labels. Computed once from the longest label.
func (w *PaycheckWizard) headerLabelWidth() int {
	labels := []string{
		w.employerField.Label,
		w.frequencyField.Label,
		w.nextPaydayField.Label,
		w.accountField.Label,
		w.memoField.Label,
	}
	maxLen := 0
	for _, l := range labels {
		if n := lipgloss.Width(l + ":"); n > maxLen {
			maxLen = n
		}
	}
	return maxLen
}

// renderLine renders one section row: select + amount + notes + [−]
// remove. Returns the rendered string plus hit zones for the select,
// amount, notes, and remove cells.
func (w *PaycheckWizard) renderLine(styles widget.Styles, fill lipgloss.Style, line *PaycheckLine, focused wizardFocusTarget, contentWidth, row int) (string, []wizardHitZone) {
	amtW := 12
	removeText := "[−]"
	removeW := lipgloss.Width(removeText)
	// Reserve the row's prefix (2) + three single-space gaps + the
	// remove button, then split what's left between select and notes.
	const prefixW = 2
	remaining := max(contentWidth-prefixW-3-removeW-amtW, 24)
	selW := max(remaining*4/7, 20)
	notesW := max(remaining-selW, 10)

	selFocused := focused.field == line.selectField
	amtFocused := focused.field == line.amountField
	notesFocused := focused.field == line.notesField

	selStr := padFill(w.renderFieldValue(styles, fill, line.selectField, selFocused, selW), selW, fill)
	amtStr := padFill(w.renderFieldValue(styles, fill, line.amountField, amtFocused, amtW), amtW, fill)
	notesStr := padFill(w.renderFieldValue(styles, fill, line.notesField, notesFocused, notesW), notesW, fill)

	var removeLabel string
	if focused.kind == wizardFocusRemove && focused.line == line {
		removeLabel = styles.DialogButtonFocused.Render(removeText)
	} else {
		removeLabel = styles.DialogButton.Render(removeText)
	}

	prefix := fill.Render("  ")
	out := prefix + selStr + fill.Render(" ") + amtStr + fill.Render(" ") + notesStr + fill.Render(" ") + removeLabel

	selStart := prefixW
	amtStart := selStart + selW + 1
	notesStart := amtStart + amtW + 1
	removeStart := notesStart + notesW + 1

	zones := []wizardHitZone{
		{
			row:    row,
			colMin: selStart,
			colMax: selStart + selW,
			target: wizardFocusTarget{kind: wizardFocusField, field: line.selectField},
		},
		{
			row:    row,
			colMin: amtStart,
			colMax: amtStart + amtW,
			target: wizardFocusTarget{kind: wizardFocusField, field: line.amountField},
		},
		{
			row:    row,
			colMin: notesStart,
			colMax: notesStart + notesW,
			target: wizardFocusTarget{kind: wizardFocusField, field: line.notesField},
		},
		{
			row:    row,
			colMin: removeStart,
			colMax: removeStart + removeW,
			target: wizardFocusTarget{kind: wizardFocusRemove, line: line},
		},
	}
	return out, zones
}

// renderFieldValue draws a field's value matching the generic
// *dialog.Dialog field rendering conventions:
//   - FieldText: `[ value ]` (bracketed with spaces inside).
//   - FieldSelect: `value ▼` (no brackets; focused value gets a
//     reverse-highlight inside the surrounding fill).
//
// fill is the dialog-bg style used for padding so the cell fills
// uniformly with the dialog's background.
func (w *PaycheckWizard) renderFieldValue(styles widget.Styles, fill lipgloss.Style, f *dialog.Field, focused bool, width int) string {
	if f == nil {
		return fill.Render(strings.Repeat(" ", width))
	}
	switch f.Type {
	case dialog.FieldText:
		// `[ value ]` — dialog.Dialog convention. Inner pad fills with bg.
		bracketOverhead := 4 // "[ " + " ]"
		inner := max(width-bracketOverhead, 1)
		val := f.Value
		runes := []rune(val)
		if len(runes) == 0 && !focused && f.Placeholder != "" {
			ph := f.Placeholder
			phRunes := []rune(ph)
			if len(phRunes) > inner {
				ph = string(phRunes[:inner])
				phRunes = phRunes[:inner]
			}
			padN := max(inner-len(phRunes), 0)
			return fill.Render("[ ") + styles.Placeholder.Render(ph) + fill.Render(strings.Repeat(" ", padN)) + fill.Render(" ]")
		}
		if focused {
			cursorStyle := lipgloss.NewStyle().Reverse(true)
			var before, cursorChar, after string
			if f.CursorPos() < len(runes) {
				before = string(runes[:f.CursorPos()])
				cursorChar = cursorStyle.Render(string(runes[f.CursorPos()]))
				if f.CursorPos()+1 < len(runes) {
					after = string(runes[f.CursorPos()+1:])
				}
			} else {
				before = string(runes)
				cursorChar = cursorStyle.Render(" ")
			}
			displayLen := len(runes)
			if f.CursorPos() >= len(runes) {
				displayLen++
			}
			padN := max(inner-displayLen, 0)
			return fill.Render("[ ") + fill.Render(before) + cursorChar + fill.Render(after+strings.Repeat(" ", padN)) + fill.Render(" ]")
		}
		// Unfocused with a value.
		if len(runes) > inner {
			runes = runes[:inner]
		}
		padN := max(inner-len(runes), 0)
		return fill.Render("[ " + string(runes) + strings.Repeat(" ", padN) + " ]")
	case dialog.FieldDate:
		// Fixed-width 10-char masked date inside `[ ... ]`. Mirrors the
		// generic *dialog.Dialog.renderDateFieldContent so the widget behaves
		// identically across dialogs.
		value := f.Value
		if len(value) != 10 {
			if len(value) < 10 {
				value += strings.Repeat(" ", 10-len(value))
			} else {
				value = value[:10]
			}
		}
		bracketOverhead := 4 // "[ " + " ]"
		inner := max(width-bracketOverhead, 10)
		padN := max(inner-10, 0)
		if !focused {
			return fill.Render("[ " + value + strings.Repeat(" ", padN) + " ]")
		}
		cursorStyle := lipgloss.NewStyle().Reverse(true)
		pos := f.CursorPos()
		if pos < 0 || pos > 9 || f.DateSeparators()[pos] {
			pos = 0
		}
		before := value[:pos]
		cursorChar := cursorStyle.Render(string(value[pos]))
		after := ""
		if pos+1 < 10 {
			after = value[pos+1:]
		}
		return fill.Render("[ ") + fill.Render(before) + cursorChar + fill.Render(after+strings.Repeat(" ", padN)) + fill.Render(" ]")
	case dialog.FieldSelect:
		// `value ▼` — no brackets; focused option gets a reverse
		// highlight just over the value cell.
		opt := ""
		if f.SelectedIndex >= 0 && f.SelectedIndex < len(f.Options) {
			opt = f.Options[f.SelectedIndex]
		}
		// Reserve " ▼" suffix (always 2 cells: space + arrow).
		suffix := " ▼"
		maxOpt := max(width-lipgloss.Width(suffix), 3)
		optRunes := []rune(opt)
		if len(optRunes) > maxOpt {
			opt = string(optRunes[:maxOpt])
		}
		if focused {
			return lipgloss.NewStyle().Reverse(true).Render(" "+opt+" ") + fill.Render(suffix)
		}
		return fill.Render(opt + suffix)
	default:
		return fill.Render(widget.PadRight(f.Value, width))
	}
}
