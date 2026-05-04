package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FieldType represents the type of a form field in a dialog.
type FieldType int

const (
	// FieldText is a text input field.
	FieldText FieldType = iota
	// FieldSelect is a dropdown selection field navigated with up/down keys.
	FieldSelect
	// FieldRadio is a radio button group.
	FieldRadio
	// FieldCheckbox is a checkbox field toggled with space.
	FieldCheckbox
	// FieldList is a vertical scrollable list showing multiple items at once.
	FieldList
)

// Field represents a form field in a dialog.
type Field struct {
	// Label is the display name shown next to the field.
	Label string
	// Type is the field type.
	Type FieldType
	// Value is the current text value (for FieldText).
	Value string
	// Placeholder is shown when Value is empty (for FieldText).
	Placeholder string
	// Options is the list of choices (for FieldSelect and FieldRadio).
	Options []string
	// SelectedIndex is the currently selected option index (for FieldSelect and FieldRadio).
	SelectedIndex int
	// Checked is the checkbox state (for FieldCheckbox).
	Checked bool
	// Width is the display width of the text input area (for FieldText). 0 means auto.
	Width int
	// Required indicates this field must be filled in.
	Required bool
	// Error is an inline validation error message displayed below the field.
	Error string
	// Hidden indicates this field should not be rendered or focusable.
	Hidden bool
	// VisibleCount is the number of items to display at once (for FieldList).
	VisibleCount int
	// cursorPos is the cursor position within the text value.
	cursorPos int
}

// InsertChar inserts a character at the cursor position.
func (f *Field) InsertChar(ch rune) {
	if f.Type != FieldText {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos > len(runes) {
		f.cursorPos = len(runes)
	}
	result := make([]rune, 0, len(runes)+1)
	result = append(result, runes[:f.cursorPos]...)
	result = append(result, ch)
	result = append(result, runes[f.cursorPos:]...)
	f.Value = string(result)
	f.cursorPos++
}

// DeleteBack deletes the character before the cursor.
func (f *Field) DeleteBack() {
	if f.Type != FieldText || f.cursorPos <= 0 {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos > len(runes) {
		f.cursorPos = len(runes)
	}
	result := make([]rune, 0, len(runes)-1)
	result = append(result, runes[:f.cursorPos-1]...)
	result = append(result, runes[f.cursorPos:]...)
	f.Value = string(result)
	f.cursorPos--
}

// DeleteForward deletes the character at the cursor position.
func (f *Field) DeleteForward() {
	if f.Type != FieldText {
		return
	}
	runes := []rune(f.Value)
	if f.cursorPos >= len(runes) {
		return
	}
	result := make([]rune, 0, len(runes)-1)
	result = append(result, runes[:f.cursorPos]...)
	result = append(result, runes[f.cursorPos+1:]...)
	f.Value = string(result)
}

// MoveCursorLeft moves the cursor one position to the left.
func (f *Field) MoveCursorLeft() {
	if f.Type != FieldText || f.cursorPos <= 0 {
		return
	}
	f.cursorPos--
}

// MoveCursorRight moves the cursor one position to the right.
func (f *Field) MoveCursorRight() {
	if f.Type != FieldText {
		return
	}
	if f.cursorPos < len([]rune(f.Value)) {
		f.cursorPos++
	}
}

// MoveCursorHome moves the cursor to the beginning.
func (f *Field) MoveCursorHome() {
	if f.Type != FieldText {
		return
	}
	f.cursorPos = 0
}

// MoveCursorEnd moves the cursor to the end.
func (f *Field) MoveCursorEnd() {
	if f.Type != FieldText {
		return
	}
	f.cursorPos = len([]rune(f.Value))
}

// CursorPos returns the current cursor position.
func (f *Field) CursorPos() int {
	return f.cursorPos
}

// SelectNext moves to the next option (FieldSelect and FieldRadio).
func (f *Field) SelectNext() {
	if (f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList) || len(f.Options) == 0 {
		return
	}
	if f.SelectedIndex < len(f.Options)-1 {
		f.SelectedIndex++
	}
}

// SelectPrev moves to the previous option (FieldSelect, FieldRadio, and FieldList).
func (f *Field) SelectPrev() {
	if (f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList) || len(f.Options) == 0 {
		return
	}
	if f.SelectedIndex > 0 {
		f.SelectedIndex--
	}
}

// Toggle toggles the checked state (FieldCheckbox).
func (f *Field) Toggle() {
	if f.Type != FieldCheckbox {
		return
	}
	f.Checked = !f.Checked
}

// SelectedOption returns the currently selected option text.
func (f *Field) SelectedOption() string {
	if (f.Type != FieldSelect && f.Type != FieldRadio && f.Type != FieldList) || len(f.Options) == 0 {
		return ""
	}
	if f.SelectedIndex < 0 || f.SelectedIndex >= len(f.Options) {
		return ""
	}
	return f.Options[f.SelectedIndex]
}

// DialogButton represents a button at the bottom of a dialog.
type DialogButton struct {
	// Label is the button text.
	Label string
	// Primary indicates this is the primary/default action button.
	Primary bool
}

// DialogAction represents the result of a dialog interaction.
type DialogAction int

const (
	// DialogActionNone means no action was taken.
	DialogActionNone DialogAction = iota
	// DialogActionSubmit means the user confirmed/saved.
	DialogActionSubmit
	// DialogActionCancel means the user cancelled.
	DialogActionCancel
)

// Dialog is a modal dialog component with form fields and buttons.
type Dialog struct {
	title      string
	fields     []*Field
	buttons    []DialogButton
	focusIndex int
	visible    bool
	width      int
	errorMsg   string
}

// dialogHorizontalOverhead is the horizontal space used by dialog border (2) and padding (4).
const dialogHorizontalOverhead = 6

// NewDialog creates a new Dialog with the given title and default Cancel/Save buttons.
func NewDialog(title string) *Dialog {
	return &Dialog{
		title: title,
		width: 56,
		buttons: []DialogButton{
			{Label: "Cancel"},
			{Label: "Save", Primary: true},
		},
	}
}

// Title returns the dialog title.
func (d *Dialog) Title() string {
	return d.title
}

// SetTitle sets the dialog title.
func (d *Dialog) SetTitle(title string) {
	d.title = title
}

// IsVisible returns whether the dialog is currently shown.
func (d *Dialog) IsVisible() bool {
	return d.visible
}

// SetVisible sets the dialog visibility.
func (d *Dialog) SetVisible(v bool) {
	d.visible = v
}

// Width returns the total dialog width.
func (d *Dialog) Width() int {
	return d.width
}

// SetWidth sets the total dialog width.
func (d *Dialog) SetWidth(w int) {
	d.width = w
}

// Fields returns the dialog fields.
func (d *Dialog) Fields() []*Field {
	return d.fields
}

// Buttons returns the dialog buttons.
func (d *Dialog) Buttons() []DialogButton {
	return d.buttons
}

// ErrorMsg returns the dialog-level error message.
func (d *Dialog) ErrorMsg() string {
	return d.errorMsg
}

// SetErrorMsg sets a dialog-level error message (for cross-field validation).
func (d *Dialog) SetErrorMsg(msg string) {
	d.errorMsg = msg
}

// ClearErrors clears the dialog-level error and all field-level errors.
func (d *Dialog) ClearErrors() {
	d.errorMsg = ""
	for _, f := range d.fields {
		f.Error = ""
	}
}

// FieldByLabel returns the first field with the given label, or nil.
func (d *Dialog) FieldByLabel(label string) *Field {
	for _, f := range d.fields {
		if f.Label == label {
			return f
		}
	}
	return nil
}

// SetButtons replaces the dialog buttons.
func (d *Dialog) SetButtons(buttons []DialogButton) {
	d.buttons = buttons
	d.clampFocusIndex()
}

// AddTextField adds a text input field and returns it.
func (d *Dialog) AddTextField(label, value, placeholder string, width int) *Field {
	f := &Field{
		Label:       label,
		Type:        FieldText,
		Value:       value,
		Placeholder: placeholder,
		Width:       width,
		cursorPos:   len([]rune(value)),
	}
	d.fields = append(d.fields, f)
	return f
}

// AddSelectField adds a dropdown selection field and returns it.
func (d *Dialog) AddSelectField(label string, options []string, selected int) *Field {
	if selected < 0 {
		selected = 0
	}
	if len(options) > 0 && selected >= len(options) {
		selected = len(options) - 1
	}
	f := &Field{
		Label:         label,
		Type:          FieldSelect,
		Options:       options,
		SelectedIndex: selected,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddRadioField adds a radio button group and returns it.
func (d *Dialog) AddRadioField(label string, options []string, selected int) *Field {
	if selected < 0 {
		selected = 0
	}
	if len(options) > 0 && selected >= len(options) {
		selected = len(options) - 1
	}
	f := &Field{
		Label:         label,
		Type:          FieldRadio,
		Options:       options,
		SelectedIndex: selected,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddCheckboxField adds a checkbox field and returns it.
func (d *Dialog) AddCheckboxField(label string, checked bool) *Field {
	f := &Field{
		Label:   label,
		Type:    FieldCheckbox,
		Checked: checked,
	}
	d.fields = append(d.fields, f)
	return f
}

// AddListField adds a vertical scrollable list field and returns it.
// visibleCount controls how many items are shown at once.
func (d *Dialog) AddListField(label string, items []string, selected int, visibleCount int) *Field {
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) && len(items) > 0 {
		selected = len(items) - 1
	}
	if visibleCount <= 0 {
		visibleCount = 10
	}
	f := &Field{
		Label:         label,
		Type:          FieldList,
		Options:       items,
		SelectedIndex: selected,
		VisibleCount:  visibleCount,
	}
	d.fields = append(d.fields, f)
	return f
}

// FocusIndex returns the current focus index.
func (d *Dialog) FocusIndex() int {
	return d.focusIndex
}

// SetFocusIndex sets the focus index, clamped to valid range.
func (d *Dialog) SetFocusIndex(idx int) {
	d.focusIndex = idx
	d.clampFocusIndex()
}

// FocusNext moves focus to the next element, wrapping around and skipping hidden fields.
func (d *Dialog) FocusNext() {
	total := d.focusableCount()
	if total == 0 {
		return
	}
	start := d.focusIndex
	for {
		d.focusIndex = (d.focusIndex + 1) % total
		if d.focusIndex == start {
			break
		}
		if !d.isFocusIndexHidden() {
			break
		}
	}
}

// FocusPrev moves focus to the previous element, wrapping around and skipping hidden fields.
func (d *Dialog) FocusPrev() {
	total := d.focusableCount()
	if total == 0 {
		return
	}
	start := d.focusIndex
	for {
		d.focusIndex = (d.focusIndex - 1 + total) % total
		if d.focusIndex == start {
			break
		}
		if !d.isFocusIndexHidden() {
			break
		}
	}
}

// isFocusIndexHidden returns true if the current focus index points to a hidden field.
func (d *Dialog) isFocusIndexHidden() bool {
	if d.focusIndex >= len(d.fields) {
		return false // buttons are never hidden
	}
	return d.fields[d.focusIndex].Hidden
}

// focusableCount returns the total number of focusable elements.
func (d *Dialog) focusableCount() int {
	return len(d.fields) + len(d.buttons)
}

// FocusedField returns the field with focus, or nil if focus is on a button.
func (d *Dialog) FocusedField() *Field {
	if d.focusIndex < 0 || d.focusIndex >= len(d.fields) {
		return nil
	}
	return d.fields[d.focusIndex]
}

// IsFocusOnButton returns true if focus is on a button.
func (d *Dialog) IsFocusOnButton() bool {
	return d.focusIndex >= len(d.fields)
}

// FocusedButtonIndex returns the button index that has focus, or -1.
func (d *Dialog) FocusedButtonIndex() int {
	if !d.IsFocusOnButton() {
		return -1
	}
	return d.focusIndex - len(d.fields)
}

// clampFocusIndex ensures focusIndex is within valid bounds.
func (d *Dialog) clampFocusIndex() {
	total := d.focusableCount()
	if total == 0 {
		d.focusIndex = 0
		return
	}
	if d.focusIndex >= total {
		d.focusIndex = total - 1
	}
	if d.focusIndex < 0 {
		d.focusIndex = 0
	}
}

// HandleKey processes a key event and returns the resulting action.
func (d *Dialog) HandleKey(msg tea.KeyPressMsg) DialogAction {
	switch msg.String() {
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
			if btnIdx >= 0 && btnIdx < len(d.buttons) && d.buttons[btnIdx].Primary {
				return DialogActionSubmit
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

// Render renders the dialog as a bordered, styled box.
func (d *Dialog) Render(styles Styles) string {
	contentWidth := max(d.width-dialogHorizontalOverhead, 10)

	var lines []string

	// Title
	lines = append(lines, d.renderTitle(styles, contentWidth))

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

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
	lines = append(lines, d.renderButtonRow(contentWidth))

	content := strings.Join(lines, "\n")
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

func (d *Dialog) renderTitle(styles Styles, contentWidth int) string {
	title := styles.DialogTitle.Render(d.title)
	titleWidth := lipgloss.Width(title)
	closeBtn := styles.Muted.Render("[x]")
	closeBtnWidth := lipgloss.Width(closeBtn)

	gap := max(contentWidth-titleWidth-closeBtnWidth, 1)

	return title + strings.Repeat(" ", gap) + closeBtn
}

func (d *Dialog) renderField(styles Styles, field *Field, focused bool, labelWidth, contentWidth int) string {
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

func (d *Dialog) renderTextFieldContent(styles Styles, field *Field, focused bool, available int) string {
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

func (d *Dialog) renderSelectFieldContent(_ Styles, field *Field, focused bool, available int) string {
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
	opt = truncateRunes(opt, maxOptWidth)
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
		item := truncateRunes(field.Options[i], maxItemWidth)
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

func (d *Dialog) renderRadioFieldContent(styles Styles, field *Field, focused bool, available int) string {
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
		truncOpt := truncateRunes(opt, perOpt)
		item := bullet + " " + truncOpt
		if focused && i == field.SelectedIndex {
			item = styles.Bold.Render(item)
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "  ")
}

func (d *Dialog) renderCheckboxField(styles Styles, field *Field, focused bool) string {
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

func (d *Dialog) renderButtonRow(contentWidth int) string {
	if len(d.buttons) == 0 {
		return ""
	}

	var btnStrs []string
	totalBtnWidth := 0

	for i, btn := range d.buttons {
		btnIdx := len(d.fields) + i
		focused := btnIdx == d.focusIndex
		label := "[ " + btn.Label + " ]"
		if focused {
			label = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ " + btn.Label + " ]")
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

// dialogVerticalOverhead is the vertical space used by dialog border (2) and padding (2).
const dialogVerticalOverhead = 4

// ContentHeight returns the number of content lines inside the dialog (excluding border and padding).
func (d *Dialog) ContentHeight() int {
	h := 0
	// Title row
	h++
	// Separator after title
	h++

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
	return 1
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
	contentWidth := max(d.width-dialogHorizontalOverhead, 10)
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

// OverlayCenter places the overlay string centered on top of the background string.
func OverlayCenter(background, overlay string, screenWidth, screenHeight int) string {
	bgLines := strings.Split(background, "\n")
	ovLines := strings.Split(overlay, "\n")

	overlayWidth := 0
	for _, line := range ovLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}
	overlayHeight := len(ovLines)

	startCol := (screenWidth - overlayWidth) / 2
	startRow := (screenHeight - overlayHeight) / 2
	if startCol < 0 {
		startCol = 0
	}
	if startRow < 0 {
		startRow = 0
	}

	for i, ovLine := range ovLines {
		targetRow := startRow + i
		if targetRow >= len(bgLines) {
			break
		}

		bgLine := bgLines[targetRow]
		bgRunes := []rune(stripAnsi(bgLine))

		prefix := ""
		if startCol > 0 {
			if startCol <= len(bgRunes) {
				prefix = string(bgRunes[:startCol])
			} else {
				prefix = string(bgRunes) + strings.Repeat(" ", startCol-len(bgRunes))
			}
		}

		ovWidth := lipgloss.Width(ovLine)
		endCol := startCol + ovWidth
		suffix := ""
		if endCol < len(bgRunes) {
			suffix = string(bgRunes[endCol:])
		}

		bgLines[targetRow] = prefix + ovLine + suffix
	}

	return strings.Join(bgLines, "\n")
}
