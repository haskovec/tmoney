package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	if (f.Type != FieldSelect && f.Type != FieldRadio) || len(f.Options) == 0 {
		return
	}
	if f.SelectedIndex < len(f.Options)-1 {
		f.SelectedIndex++
	}
}

// SelectPrev moves to the previous option (FieldSelect and FieldRadio).
func (f *Field) SelectPrev() {
	if (f.Type != FieldSelect && f.Type != FieldRadio) || len(f.Options) == 0 {
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
	if (f.Type != FieldSelect && f.Type != FieldRadio) || len(f.Options) == 0 {
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

// FocusIndex returns the current focus index.
func (d *Dialog) FocusIndex() int {
	return d.focusIndex
}

// SetFocusIndex sets the focus index, clamped to valid range.
func (d *Dialog) SetFocusIndex(idx int) {
	d.focusIndex = idx
	d.clampFocusIndex()
}

// FocusNext moves focus to the next element, wrapping around.
func (d *Dialog) FocusNext() {
	total := d.focusableCount()
	if total == 0 {
		return
	}
	d.focusIndex = (d.focusIndex + 1) % total
}

// FocusPrev moves focus to the previous element, wrapping around.
func (d *Dialog) FocusPrev() {
	total := d.focusableCount()
	if total == 0 {
		return
	}
	d.focusIndex = (d.focusIndex - 1 + total) % total
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
func (d *Dialog) HandleKey(msg tea.KeyMsg) DialogAction {
	switch msg.Type {
	case tea.KeyEsc:
		return DialogActionCancel
	case tea.KeyTab:
		d.FocusNext()
		return DialogActionNone
	case tea.KeyShiftTab:
		d.FocusPrev()
		return DialogActionNone
	case tea.KeyEnter:
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
	}
	return DialogActionNone
}

func (d *Dialog) handleTextFieldKey(field *Field, msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyBackspace:
		field.DeleteBack()
		field.Error = ""
	case tea.KeyDelete:
		field.DeleteForward()
		field.Error = ""
	case tea.KeyLeft:
		field.MoveCursorLeft()
	case tea.KeyRight:
		field.MoveCursorRight()
	case tea.KeyHome, tea.KeyCtrlA:
		field.MoveCursorHome()
	case tea.KeyEnd, tea.KeyCtrlE:
		field.MoveCursorEnd()
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			field.InsertChar(r)
		}
		field.Error = ""
	}
}

func (d *Dialog) handleSelectFieldKey(field *Field, msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyUp:
		field.SelectPrev()
		field.Error = ""
	case tea.KeyDown:
		field.SelectNext()
		field.Error = ""
	}
}

func (d *Dialog) handleRadioFieldKey(field *Field, msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyLeft:
		field.SelectPrev()
		field.Error = ""
	case tea.KeyDown, tea.KeyRight:
		field.SelectNext()
		field.Error = ""
	}
}

func (d *Dialog) handleCheckboxFieldKey(field *Field, msg tea.KeyMsg) {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
		field.Toggle()
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
		if f.Type == FieldCheckbox {
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
		fieldContent = d.renderTextFieldContent(field, focused, available)
	case FieldSelect:
		fieldContent = d.renderSelectFieldContent(styles, field, focused)
	case FieldRadio:
		fieldContent = d.renderRadioFieldContent(styles, field, focused)
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

func (d *Dialog) renderTextFieldContent(field *Field, focused bool, available int) string {
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
		return "[ " + lipgloss.NewStyle().Foreground(ColorMuted).Render(ph) + strings.Repeat(" ", pad) + " ]"
	}

	displayRunes := runes
	if len(displayRunes) > fw {
		displayRunes = displayRunes[:fw]
	}
	pad := max(fw-len(displayRunes), 0)
	return "[ " + string(displayRunes) + strings.Repeat(" ", pad) + " ]"
}

func (d *Dialog) renderSelectFieldContent(_ Styles, field *Field, focused bool) string {
	opt := field.SelectedOption()
	if opt == "" {
		opt = "(none)"
	}
	if focused {
		return lipgloss.NewStyle().Reverse(true).Render(" " + opt + " ") + " ▼"
	}
	return opt + " ▼"
}

func (d *Dialog) renderRadioFieldContent(styles Styles, field *Field, focused bool) string {
	var parts []string
	for i, opt := range field.Options {
		bullet := "( )"
		if i == field.SelectedIndex {
			bullet = "(*)"
		}
		item := bullet + " " + opt
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
