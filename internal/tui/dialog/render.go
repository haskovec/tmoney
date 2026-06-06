package dialog

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// DialogHorizontalOverhead is the horizontal space used by dialog border (2) and padding (4).
const DialogHorizontalOverhead = 6

// Render renders the dialog as a bordered, styled box. When a height bound is
// set (SetMaxHeight) and the form is taller than that bound, the field region
// scrolls to keep the focused field visible and a scrollbar is drawn so the
// dialog never overflows the terminal/status bar. Otherwise the full form is
// rendered unchanged (the legacy path).
func (d *Dialog) Render(styles widget.Styles) string {
	if d.isScrolling() {
		return d.renderScrolled(styles)
	}
	d.fieldScroll = 0
	return d.renderUnscrolled(styles)
}

// renderUnscrolled renders the full form with no height clamping — the legacy
// rendering path, used whenever the dialog fits within its bound or is
// unbounded.
func (d *Dialog) renderUnscrolled(styles widget.Styles) string {
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

	// Separator + buttons. Omitted entirely for a buttonless dialog (e.g.
	// the multi-line scheduled-preview header, whose single action bar
	// lives on the embedded split panel below it) so no dangling
	// separator hangs at the bottom of the box.
	if len(d.buttons) > 0 {
		lines = append(lines, strings.Repeat("─", contentWidth))
		lines = append(lines, d.renderButtonRow(styles, contentWidth))
	}

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

// effectiveMaxContent returns the maximum number of content lines (excluding
// border + padding) the dialog may occupy, or 0 when unbounded.
func (d *Dialog) effectiveMaxContent() int {
	if d.maxHeight <= 0 {
		return 0
	}
	return max(d.maxHeight-dialogVerticalOverhead, 1)
}

// isScrolling reports whether the field region must scroll to fit the height
// bound. False when unbounded or when the whole form already fits.
func (d *Dialog) isScrolling() bool {
	mc := d.effectiveMaxContent()
	return mc > 0 && d.ContentHeight() > mc
}

// scrollPinnedTopRows is the number of content rows pinned above the scrollable
// body when a dialog scrolls: the title and its separator. Everything else
// (the message block and the fields) lives in the scrollable body so even a
// message-heavy dialog is bounded.
const scrollPinnedTopRows = 2

// bottomRowCount returns the number of pinned content rows below the field
// region (the dialog-level error block and the separator + button row).
func (d *Dialog) bottomRowCount() int {
	rows := 0
	if d.errorMsg != "" {
		rows += 2 // error line + blank
	}
	if len(d.buttons) > 0 {
		rows += 2 // separator + button row
	}
	return rows
}

// renderScrolled renders a height-bounded dialog whose form is taller than the
// bound: the title + separator stay pinned on top and the error/buttons stay
// pinned on the bottom, while the body (message block + fields) is windowed to
// keep the focused field visible, with a proportional scrollbar in the
// reserved right column. Putting the message in the body — not the pinned top —
// means a message-heavy dialog stays bounded too.
func (d *Dialog) renderScrolled(styles widget.Styles) string {
	contentWidth := max(d.width-DialogHorizontalOverhead, 10)
	maxContent := d.effectiveMaxContent()

	// Pinned top: title + separator (scrollPinnedTopRows).
	top := []string{
		d.renderTitle(styles, contentWidth),
		strings.Repeat("─", contentWidth),
	}

	// Pinned bottom: dialog-level error, separator + button row.
	var bottom []string
	if d.errorMsg != "" {
		bottom = append(bottom, styles.FieldError.Render(d.errorMsg))
		bottom = append(bottom, "")
	}
	if len(d.buttons) > 0 {
		bottom = append(bottom, strings.Repeat("─", contentWidth))
		bottom = append(bottom, d.renderButtonRow(styles, contentWidth))
	}

	// Reserve two right columns (a gutter + the scrollbar) so the bar never
	// collides with body content.
	fieldWidth := max(contentWidth-2, 5)

	// Scrollable body: the message block (matching renderUnscrolled's message +
	// trailing blank) followed by the field block. Field line ranges shift by
	// the message length.
	var body []string
	if d.message != "" {
		for ln := range strings.SplitSeq(d.message, "\n") {
			body = append(body, ln)
		}
		body = append(body, "")
	}
	msgLen := len(body)
	block, fieldStart, fieldEnd := d.buildFieldBlock(styles, fieldWidth)
	body = append(body, block...)

	viewport := max(maxContent-len(top)-len(bottom), 1)

	fStart, fEnd := d.focusedFieldRange(fieldStart, fieldEnd)
	if fStart >= 0 {
		fStart += msgLen
		fEnd += msgLen
	}
	d.fieldScroll = clampFieldScroll(d.fieldScroll, viewport, len(body), fStart, fEnd)

	endLine := min(d.fieldScroll+viewport, len(body))
	visible := make([]string, 0, viewport)
	if d.fieldScroll < len(body) {
		visible = append(visible, body[d.fieldScroll:endLine]...)
	}
	for len(visible) < viewport {
		visible = append(visible, "")
	}
	decorateScrollbar(styles, visible, len(body), viewport, d.fieldScroll, fieldWidth)

	lines := make([]string, 0, len(top)+len(visible)+len(bottom))
	lines = append(lines, top...)
	lines = append(lines, visible...)
	lines = append(lines, bottom...)

	content := strings.Join(lines, "\n")
	content = widget.RepaintDialog(content)
	return styles.Dialog.Width(d.width).Render(content)
}

// buildFieldBlock renders the scrollable field region into a flat slice of
// lines and records each field's [start,end) line range within it. The block
// mirrors renderUnscrolled's field section: a blank line before each visible
// field, the field's rendered line(s), and a trailing blank after the last
// field. fieldStart[i]/fieldEnd[i] bracket field i's rendered lines (excluding
// the leading blank); both stay -1 for hidden fields.
func (d *Dialog) buildFieldBlock(styles widget.Styles, fieldWidth int) (lines []string, fieldStart, fieldEnd []int) {
	labelWidth := d.maxLabelWidth()
	fieldStart = make([]int, len(d.fields))
	fieldEnd = make([]int, len(d.fields))
	for i := range d.fields {
		fieldStart[i], fieldEnd[i] = -1, -1
	}
	for i, field := range d.fields {
		if field.Hidden {
			continue
		}
		lines = append(lines, "") // blank line before the field
		fieldStart[i] = len(lines)
		rendered := d.renderField(styles, field, i == d.focusIndex, labelWidth, fieldWidth)
		lines = append(lines, strings.Split(rendered, "\n")...)
		fieldEnd[i] = len(lines)
	}
	if d.hasVisibleFields() {
		lines = append(lines, "") // trailing blank after all fields
	}
	return lines, fieldStart, fieldEnd
}

// focusedFieldRange returns the [start,end) block-line range of the focused
// field, or (-1,-1) when focus is on a button (buttons are pinned, so no
// scroll adjustment is needed).
func (d *Dialog) focusedFieldRange(fieldStart, fieldEnd []int) (int, int) {
	if d.focusIndex < 0 || d.focusIndex >= len(d.fields) {
		return -1, -1
	}
	return fieldStart[d.focusIndex], fieldEnd[d.focusIndex]
}

// clampFieldScroll keeps the focused field's [fStart,fEnd) line range visible
// within a viewport-high window over `total` lines, mirroring the
// Table/Sidebar cursor-follow idiom. The end check runs before the start check
// so a field taller than the viewport is anchored to its top (its remaining
// rows clip, e.g. a focused combo's own internally-scrolling panel).
func clampFieldScroll(scroll, viewport, total, fStart, fEnd int) int {
	if fStart >= 0 {
		if fEnd > scroll+viewport {
			scroll = fEnd - viewport
		}
		if fStart < scroll {
			scroll = fStart
		}
	}
	if maxOffset := max(total-viewport, 0); scroll > maxOffset {
		scroll = maxOffset
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// decorateScrollbar pads each windowed field line to fieldWidth and appends a
// gutter space plus the proportional scrollbar glyph (thumb █ / track │) for
// that row.
func decorateScrollbar(styles widget.Styles, visible []string, total, viewport, scroll, fieldWidth int) {
	track := styles.Muted.Render("│")
	thumb := styles.Bold.Render("█")
	for p := range visible {
		line := visible[p]
		// Cap over-wide body lines (e.g. a pathologically long message line)
		// so they can't soft-wrap inside the box and defeat the height clamp,
		// and so the scrollbar column stays aligned. Field lines are already
		// rendered within fieldWidth, so this only ever trims plain text.
		if lipgloss.Width(line) > fieldWidth {
			line = widget.TruncateRunes(line, fieldWidth)
		}
		glyph := track
		if scrollbarThumbAt(p, viewport, total, scroll) {
			glyph = thumb
		}
		visible[p] = widget.PadRight(line, fieldWidth) + " " + glyph
	}
}

// scrollbarThumbAt reports whether viewport row p falls within the scrollbar
// thumb when showing `viewport` rows out of `total`, scrolled by `scroll`.
func scrollbarThumbAt(p, viewport, total, scroll int) bool {
	if viewport <= 0 || total <= viewport {
		return false
	}
	thumb := max(viewport*viewport/total, 1)
	maxStart := viewport - thumb
	start := 0
	if denom := total - viewport; denom > 0 {
		start = min(scroll*maxStart/denom, maxStart)
	}
	if start < 0 {
		start = 0
	}
	return p >= start && p < start+thumb
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
	specs := make([]ButtonSpec, len(d.buttons))
	for i, btn := range d.buttons {
		btnIdx := len(d.fields) + i
		specs[i] = ButtonSpec{Label: btn.Label, Focused: btnIdx == d.focusIndex}
	}
	return RenderButtonRow(styles, specs, contentWidth)
}

// ButtonSpec describes one action button for RenderButtonRow.
type ButtonSpec struct {
	Label   string
	Focused bool
	// Disabled renders the button muted (still occupying its slot). Used
	// for an action that is temporarily unavailable, e.g. Save while a
	// split dialog is imbalanced.
	Disabled bool
}

// RenderButtonRow renders an evenly-spaced row of action buttons using the
// theme's dialog-button styles. Exposed so custom dialogs (e.g. the split
// editor) render their buttons identically to the standard dialog —
// matching spacing, theming, and the focused shortcut-letter highlight.
func RenderButtonRow(styles widget.Styles, btns []ButtonSpec, contentWidth int) string {
	if len(btns) == 0 {
		return ""
	}

	btnStrs := make([]string, len(btns))
	totalBtnWidth := 0
	for i, b := range btns {
		var label string
		switch {
		case b.Disabled:
			label = styles.Muted.Render("[ " + b.Label + " ]")
		case b.Focused && !widget.IsTransparent(widget.ColorDialogButtonShortcutFg):
			label = renderFocusedButton(styles, b.Label)
		case b.Focused:
			label = styles.DialogButtonFocused.Render("[ " + b.Label + " ]")
		default:
			label = styles.DialogButton.Render("[ " + b.Label + " ]")
		}
		btnStrs[i] = label
		totalBtnWidth += lipgloss.Width(label)
	}

	// Distribute buttons with even spacing.
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

	// Separator + button row (both omitted when the dialog has no buttons,
	// matching Render).
	if len(d.buttons) > 0 {
		h += 2
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

// RenderedHeight returns the total rendered height of the dialog including
// border and padding. When a height bound is set and the form overflows it,
// the dialog scrolls and is clamped to maxHeight, so the returned height
// matches what is actually drawn (and what DialogBounds uses to center it).
func (d *Dialog) RenderedHeight() int {
	natural := d.ContentHeight() + dialogVerticalOverhead
	if d.maxHeight <= 0 || natural <= d.maxHeight {
		return natural
	}
	// Scrolling: the box is pinned top + windowed body + pinned bottom +
	// border/padding. Mirror renderScrolled's viewport formula so the reported
	// height always equals what is actually drawn — keeping DialogBounds
	// centering and mouse mapping consistent, even on a budget too small to
	// fit the pinned rows (an absurdly short terminal), where the true height
	// can exceed maxHeight rather than silently desyncing.
	viewport := max(d.effectiveMaxContent()-scrollPinnedTopRows-d.bottomRowCount(), 1)
	return scrollPinnedTopRows + viewport + d.bottomRowCount() + dialogVerticalOverhead
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
