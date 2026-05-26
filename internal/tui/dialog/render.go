package dialog

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// DialogHorizontalOverhead is the horizontal space used by dialog border (2) and padding (4).
const DialogHorizontalOverhead = 6

// Render renders the dialog as a bordered, styled box.
func (d *Dialog) Render(styles widget.Styles) string {
	contentWidth := max(d.width-DialogHorizontalOverhead, 10)

	var lines []string

	// Title
	lines = append(lines, d.renderTitle(styles, contentWidth))

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Neutral body message (multi-line). Rendered above fields (or above
	// the button separator when there are no fields), with no special
	// styling so it doesn't read as an error.
	if d.message != "" {
		for ln := range strings.SplitSeq(d.message, "\n") {
			lines = append(lines, ln)
		}
		lines = append(lines, "")
	}

	// Fields
	labelWidth := d.maxLabelWidth()
	for i, field := range d.fields {
		if field.Hidden {
			continue
		}
		focused := i == d.focusIndex
		lines = append(lines, "")
		lines = append(lines, d.renderField(styles, field, focused, labelWidth, contentWidth))
	}
	if len(d.fields) > 0 {
		lines = append(lines, "")
	}

	// Dialog-level error message
	if d.errorMsg != "" {
		lines = append(lines, styles.FieldError.Render(d.errorMsg))
		lines = append(lines, "")
	}

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Buttons
	lines = append(lines, d.renderButtonRow(styles, contentWidth))

	content := strings.Join(lines, "\n")
	// Re-emit the dialog's outer fg + bg after inner SGR resets so
	// unstyled gaps (title row right-pad, between-button gap,
	// placeholder padding, etc.) don't punch holes through the panel
	// and so raw text after a styled span (red required-`*`, muted
	// placeholder) keeps the theme's dialog.fg instead of reverting to
	// terminal default.
	content = widget.RepaintDialog(content)
	return styles.Dialog.Width(d.width).Render(content)
}

func (d *Dialog) maxLabelWidth() int {
	maxW := 0
	for _, f := range d.fields {
		if f.Type == FieldCheckbox || f.Hidden {
			continue
		}
		w := len([]rune(f.Label))
		if f.Required {
			w++ // account for "*" suffix
		}
		if w > maxW {
			maxW = w
		}
	}
	return maxW
}

func (d *Dialog) renderTitle(styles widget.Styles, contentWidth int) string {
	title := styles.DialogTitle.Render(d.title)
	titleWidth := lipgloss.Width(title)
	closeBtn := styles.Muted.Render("[x]")
	closeBtnWidth := lipgloss.Width(closeBtn)

	gap := max(contentWidth-titleWidth-closeBtnWidth, 1)

	return title + strings.Repeat(" ", gap) + closeBtn
}

func (d *Dialog) renderField(styles widget.Styles, field *Field, focused bool, labelWidth, contentWidth int) string {
	if field.Type == FieldCheckbox {
		line := d.renderCheckboxField(styles, field, focused)
		if field.Error != "" {
			line += "\n" + styles.FieldError.Render(field.Error)
		}
		return line
	}

	// Build label with required marker
	label := field.Label
	if field.Required {
		label += styles.FieldError.Render("*")
	}

	// Right-align label to labelWidth
	labelRuneLen := len([]rune(field.Label))
	if field.Required {
		labelRuneLen++ // account for "*"
	}
	padLeft := max(labelWidth-labelRuneLen, 0)
	paddedLabel := strings.Repeat(" ", padLeft) + label + ":"

	// Available width for field content area
	labelColWidth := labelWidth + 1 // label + colon
	gap := 2
	available := max(contentWidth-labelColWidth-gap, 5)

	var fieldContent string
	switch field.Type {
	case FieldText:
		fieldContent = d.renderTextFieldContent(styles, field, focused, available)
	case FieldDate:
		fieldContent = d.renderDateFieldContent(field, focused)
	case FieldSelect:
		fieldContent = d.renderSelectFieldContent(styles, field, focused, available)
	case FieldList:
		// FieldList renders vertically below the label line
		listContent := d.renderListFieldContent(field, focused, available)
		line := paddedLabel
		if field.Error != "" {
			errorIndent := strings.Repeat(" ", labelWidth+1+gap)
			line += "\n" + errorIndent + styles.FieldError.Render(field.Error)
		}
		return line + "\n" + listContent
	case FieldCombo:
		header := d.renderComboHeader(field, focused, available)
		if !focused {
			fieldContent = header
			break
		}
		panel := d.renderComboPanel(field, available)
		line := paddedLabel + "  " + header
		if field.Error != "" {
			errorIndent := strings.Repeat(" ", labelWidth+1+gap)
			line += "\n" + errorIndent + styles.FieldError.Render(field.Error)
		}
		return line + "\n" + panel
	case FieldRadio:
		fieldContent = d.renderRadioFieldContent(styles, field, focused, available)
	default:
		fieldContent = ""
	}

	line := paddedLabel + "  " + fieldContent
	if field.Error != "" {
		// Indent the error to align under the field content
		errorIndent := strings.Repeat(" ", labelWidth+1+gap)
		line += "\n" + errorIndent + styles.FieldError.Render(field.Error)
	}
	return line
}

func (d *Dialog) renderTextFieldContent(styles widget.Styles, field *Field, focused bool, available int) string {
	bracketOverhead := 4 // "[ " and " ]"
	fw := field.Width
	maxFW := max(available-bracketOverhead, 1)
	if fw <= 0 || fw > maxFW {
		fw = maxFW
	}

	runes := []rune(field.Value)

	if focused {
		cursorStyle := lipgloss.NewStyle().Reverse(true)
		var before, cursorChar, after string

		if field.cursorPos < len(runes) {
			before = string(runes[:field.cursorPos])
			cursorChar = cursorStyle.Render(string(runes[field.cursorPos]))
			if field.cursorPos+1 < len(runes) {
				after = string(runes[field.cursorPos+1:])
			}
		} else {
			before = string(runes)
			cursorChar = cursorStyle.Render(" ")
		}

		displayLen := len(runes)
		if field.cursorPos >= len(runes) {
			displayLen++
		}
		pad := max(fw-displayLen, 0)

		return "[ " + before + cursorChar + after + strings.Repeat(" ", pad) + " ]"
	}

	// Unfocused
	if len(runes) == 0 && field.Placeholder != "" {
		ph := field.Placeholder
		phRunes := []rune(ph)
		if len(phRunes) > fw {
			ph = string(phRunes[:fw])
			phRunes = phRunes[:fw]
		}
		pad := max(fw-len(phRunes), 0)
		return "[ " + styles.Placeholder.Render(ph) + strings.Repeat(" ", pad) + " ]"
	}

	displayRunes := runes
	if len(displayRunes) > fw {
		displayRunes = displayRunes[:fw]
	}
	pad := max(fw-len(displayRunes), 0)
	return "[ " + string(displayRunes) + strings.Repeat(" ", pad) + " ]"
}

func (d *Dialog) renderDateFieldContent(field *Field, focused bool) string {
	value := field.Value
	if len(value) != 10 {
		// Defensive fallback: pad/truncate to canonical shape so render stays
		// stable even if a caller hands us a malformed value.
		if len(value) < 10 {
			value += strings.Repeat(" ", 10-len(value))
		} else {
			value = value[:10]
		}
	}
	if !focused {
		return "[ " + value + " ]"
	}
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	pos := field.cursorPos
	if pos < 0 || pos > 9 || field.DateSeparators()[pos] {
		// Defensive: snap to first digit if cursor is somewhere unexpected.
		pos = 0
	}
	before := value[:pos]
	cursorChar := cursorStyle.Render(string(value[pos]))
	after := ""
	if pos+1 < 10 {
		after = value[pos+1:]
	}
	return "[ " + before + cursorChar + after + " ]"
}

func (d *Dialog) renderSelectFieldContent(_ widget.Styles, field *Field, focused bool, available int) string {
	opt := field.SelectedOption()
	if opt == "" {
		opt = "(none)"
	}
	// Reserve 3 chars for " ▼" suffix (▼ is 3 bytes but 1 rune + space)
	maxOptWidth := available - 3
	if focused {
		maxOptWidth = available - 5 // " " + opt + " " + " ▼"
	}
	if maxOptWidth < 3 {
		maxOptWidth = 3
	}
	opt = widget.TruncateRunes(opt, maxOptWidth)
	if focused {
		return lipgloss.NewStyle().Reverse(true).Render(" "+opt+" ") + " ▼"
	}
	return opt + " ▼"
}

func (d *Dialog) renderListFieldContent(field *Field, focused bool, contentWidth int) string {
	if len(field.Options) == 0 {
		return "  (empty)"
	}

	visible := field.VisibleCount
	if visible <= 0 {
		visible = 10
	}
	if visible > len(field.Options) {
		visible = len(field.Options)
	}

	// Calculate scroll offset to keep selection visible
	scrollOffset := 0
	if field.SelectedIndex >= visible {
		scrollOffset = field.SelectedIndex - visible + 1
	}
	if scrollOffset+visible > len(field.Options) {
		scrollOffset = len(field.Options) - visible
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	end := min(scrollOffset+visible, len(field.Options))

	// Item width: indent (4) + "> " or "  " (2) + item text
	maxItemWidth := max(contentWidth-6, 5)

	var lines []string
	for i := scrollOffset; i < end; i++ {
		item := widget.TruncateRunes(field.Options[i], maxItemWidth)
		if i == field.SelectedIndex {
			if focused {
				line := "    " + lipgloss.NewStyle().Reverse(true).Render("> "+item)
				lines = append(lines, line)
			} else {
				lines = append(lines, "    > "+item)
			}
		} else {
			lines = append(lines, "      "+item)
		}
	}

	// Show scroll indicators
	if scrollOffset > 0 {
		lines[0] = "  ↑ " + strings.TrimLeft(lines[0], " ")
	}
	if end < len(field.Options) {
		lines[len(lines)-1] = "  ↓ " + strings.TrimLeft(lines[len(lines)-1], " ")
	}

	return strings.Join(lines, "\n")
}

// renderComboHeader renders the single-line combo header: typed query (when
// focused and non-empty) or current selection text, followed by a chevron.
func (d *Dialog) renderComboHeader(field *Field, focused bool, available int) string {
	maxOptWidth := available - 3
	if focused {
		maxOptWidth = available - 5
	}
	if maxOptWidth < 3 {
		maxOptWidth = 3
	}
	var display string
	if focused && field.Query != "" {
		display = field.Query
	} else {
		display = field.SelectedOption()
		if display == "" {
			display = "(none)"
		}
	}
	display = widget.TruncateRunes(display, maxOptWidth)
	if focused {
		return lipgloss.NewStyle().Reverse(true).Render(" "+display+" ") + " ▼"
	}
	return display + " ▼"
}

// renderComboPanel renders the dropdown filtered-list panel shown below the
// combo header while the field is focused. Reuses FieldList scroll-window
// math. When AddNewLabel is set, an action row is appended to the bottom of
// the panel and is rendered with a dimmed style when not highlighted.
func (d *Dialog) renderComboPanel(field *Field, contentWidth int) string {
	indices := field.FilteredIndices()
	hasAction := field.AddNewLabel != ""

	totalRows := len(indices)
	if hasAction {
		totalRows++
	}
	if totalRows == 0 {
		return "      (no matches)"
	}

	visible := field.VisibleCount
	if visible <= 0 {
		visible = 8
	}
	if visible > totalRows {
		visible = totalRows
	}

	scrollOffset := 0
	if field.ComboHighlight >= visible {
		scrollOffset = field.ComboHighlight - visible + 1
	}
	if scrollOffset+visible > totalRows {
		scrollOffset = totalRows - visible
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	end := min(scrollOffset+visible, totalRows)
	maxItemWidth := max(contentWidth-6, 5)
	actionStyle := lipgloss.NewStyle().Foreground(widget.ColorMuted)

	var lines []string
	for i := scrollOffset; i < end; i++ {
		isAction := hasAction && i == len(indices)
		var item string
		if isAction {
			item = widget.TruncateRunes(field.AddNewLabel, maxItemWidth)
		} else {
			item = widget.TruncateRunes(field.Options[indices[i]], maxItemWidth)
		}
		switch {
		case i == field.ComboHighlight:
			lines = append(lines, "    "+lipgloss.NewStyle().Reverse(true).Render("> "+item))
		case isAction:
			lines = append(lines, "      "+actionStyle.Render(item))
		default:
			lines = append(lines, "      "+item)
		}
	}
	if scrollOffset > 0 {
		lines[0] = "  ↑ " + strings.TrimLeft(lines[0], " ")
	}
	if end < totalRows {
		lines[len(lines)-1] = "  ↓ " + strings.TrimLeft(lines[len(lines)-1], " ")
	}

	return strings.Join(lines, "\n")
}

func (d *Dialog) renderRadioFieldContent(styles widget.Styles, field *Field, focused bool, available int) string {
	numOpts := len(field.Options)
	if numOpts == 0 {
		return ""
	}
	// Each option has "( ) " prefix (4 chars) + gap of 2 between items
	gaps := (numOpts - 1) * 2
	bulletOverhead := numOpts * 4 // "( ) " per option
	textBudget := max(available-gaps-bulletOverhead, numOpts)
	perOpt := textBudget / numOpts

	var parts []string
	for i, opt := range field.Options {
		bullet := "( )"
		if i == field.SelectedIndex {
			bullet = "(*)"
		}
		truncOpt := widget.TruncateRunes(opt, perOpt)
		item := bullet + " " + truncOpt
		if focused && i == field.SelectedIndex {
			item = styles.Bold.Render(item)
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "  ")
}

func (d *Dialog) renderCheckboxField(styles widget.Styles, field *Field, focused bool) string {
	check := "[ ]"
	if field.Checked {
		check = "[x]"
	}
	line := check + " " + field.Label
	if focused {
		return styles.Bold.Render(line)
	}
	return line
}

func (d *Dialog) renderButtonRow(styles widget.Styles, contentWidth int) string {
	if len(d.buttons) == 0 {
		return ""
	}

	var btnStrs []string
	totalBtnWidth := 0

	for i, btn := range d.buttons {
		btnIdx := len(d.fields) + i
		focused := btnIdx == d.focusIndex
		var label string
		if focused && !widget.IsTransparent(widget.ColorDialogButtonShortcutFg) {
			label = renderFocusedButton(styles, btn.Label)
		} else if focused {
			label = styles.DialogButtonFocused.Render("[ " + btn.Label + " ]")
		} else {
			label = styles.DialogButton.Render("[ " + btn.Label + " ]")
		}
		btnStrs = append(btnStrs, label)
		totalBtnWidth += lipgloss.Width(label)
	}

	// Distribute buttons with even spacing
	numGaps := len(btnStrs) + 1
	totalGapSpace := max(contentWidth-totalBtnWidth, numGaps)
	gapSize := totalGapSpace / numGaps
	extraGap := totalGapSpace % numGaps

	var result strings.Builder
	for i, btn := range btnStrs {
		gap := gapSize
		if i < extraGap {
			gap++
		}
		result.WriteString(strings.Repeat(" ", gap))
		result.WriteString(btn)
	}

	return result.String()
}

// renderFocusedButton renders the focused-button label with a Turbo
// Vision-style shortcut-letter highlight: the first rune of the label
// is rendered in DialogButtonShortcut, the rest in DialogButtonFocused.
// When the theme leaves shortcut.fg unset, DialogButtonShortcut equals
// DialogButtonFocused, so this collapses to a uniform render.
func renderFocusedButton(styles widget.Styles, label string) string {
	runes := []rune(label)
	if len(runes) == 0 {
		return styles.DialogButtonFocused.Render("[  ]")
	}
	first := string(runes[0])
	rest := string(runes[1:])
	var b strings.Builder
	b.WriteString(styles.DialogButtonFocused.Render("[ "))
	b.WriteString(styles.DialogButtonShortcut.Render(first))
	b.WriteString(styles.DialogButtonFocused.Render(rest + " ]"))
	return b.String()
}

// dialogVerticalOverhead is the vertical space used by dialog border (2) and padding (2).
const dialogVerticalOverhead = 4

// ContentHeight returns the number of content lines inside the dialog (excluding border and padding).
func (d *Dialog) ContentHeight() int {
	h := 0
	// Title row
	h++
	// Separator after title
	h++

	// Neutral message body: one row per line + a trailing blank
	if d.message != "" {
		h += d.messageLineCount() + 1
	}

	// Fields
	for _, field := range d.fields {
		if field.Hidden {
			continue
		}
		// Blank line before each field
		h++
		// Field content rows
		h += d.fieldContentRows(field)
		// Error row
		if field.Error != "" {
			h++
		}
	}
	// Blank line after all fields (if any visible fields exist)
	if d.hasVisibleFields() {
		h++
	}

	// Dialog-level error
	if d.errorMsg != "" {
		h += 2 // error line + blank line
	}

	// Separator before buttons
	h++
	// Button row
	if len(d.buttons) > 0 {
		h++
	}

	return h
}

// messageLineCount returns the number of newline-separated lines in the
// neutral body message. Returns 0 when the message is empty.
func (d *Dialog) messageLineCount() int {
	if d.message == "" {
		return 0
	}
	return strings.Count(d.message, "\n") + 1
}

// hasVisibleFields returns true if any field is not hidden.
func (d *Dialog) hasVisibleFields() bool {
	for _, f := range d.fields {
		if !f.Hidden {
			return true
		}
	}
	return false
}

// fieldContentRows returns how many content rows a field occupies (excluding blank line before and error row).
func (d *Dialog) fieldContentRows(field *Field) int {
	if field.Type == FieldList {
		visible := field.VisibleCount
		if visible <= 0 {
			visible = 10
		}
		if len(field.Options) == 0 {
			return 2 // label line + "(empty)" line
		}
		if visible > len(field.Options) {
			visible = len(field.Options)
		}
		return 1 + visible // label line + visible item lines
	}
	if field.Type == FieldCombo && d.isFieldFocused(field) {
		visible := field.VisibleCount
		if visible <= 0 {
			visible = 8
		}
		matches := len(rankComboMatches(field.Options, field.Query))
		if field.AddNewLabel != "" {
			matches++ // the action row counts as a row
		}
		if matches == 0 {
			return 2 // header line + "(no matches)" line
		}
		if visible > matches {
			visible = matches
		}
		return 1 + visible
	}
	return 1
}

// isFieldFocused reports whether the given field has focus.
func (d *Dialog) isFieldFocused(field *Field) bool {
	if d.focusIndex < 0 || d.focusIndex >= len(d.fields) {
		return false
	}
	return d.fields[d.focusIndex] == field
}

// RenderedHeight returns the total rendered height of the dialog including border and padding.
func (d *Dialog) RenderedHeight() int {
	return d.ContentHeight() + dialogVerticalOverhead
}

// DialogBounds returns the bounding box of the dialog in screen coordinates.
func (d *Dialog) DialogBounds(screenWidth, screenHeight int) (startCol, startRow, endCol, endRow int) {
	overlayHeight := d.RenderedHeight()
	overlayWidth := d.width
	startCol = max((screenWidth-overlayWidth)/2, 0)
	startRow = max((screenHeight-overlayHeight)/2, 0)
	endCol = startCol + overlayWidth
	endRow = startRow + overlayHeight
	return
}

// listScrollOffset computes the scroll offset for a FieldList, mirroring renderListFieldContent logic.
func listScrollOffset(field *Field) int {
	visible := field.VisibleCount
	if visible <= 0 {
		visible = 10
	}
	if visible > len(field.Options) {
		visible = len(field.Options)
	}
	scrollOffset := 0
	if field.SelectedIndex >= visible {
		scrollOffset = field.SelectedIndex - visible + 1
	}
	if scrollOffset+visible > len(field.Options) {
		scrollOffset = len(field.Options) - visible
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	return scrollOffset
}
