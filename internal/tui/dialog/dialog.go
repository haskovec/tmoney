package dialog

// DialogButton represents a button at the bottom of a dialog.
type DialogButton struct {
	// Label is the button text.
	Label string
	// Primary indicates this is the primary/default action button.
	Primary bool
	// Action is the DialogAction returned by HandleKey when this
	// button is activated and is non-primary. Defaults to
	// DialogActionNone, which preserves the legacy "non-primary →
	// Cancel" mapping. Use this for a third "alternate" button (e.g.
	// MS-029's "Edit as paycheck →").
	Action DialogAction
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
	// DialogActionAddNew means the user activated a FieldCombo's AddNew
	// action row. The parent dialog is responsible for diverting into a
	// sub-dialog, then restoring focus and updating SelectedIndex.
	DialogActionAddNew
	// DialogActionAlternate means the user activated a non-primary
	// button whose Action field was set to this value. Used by the
	// scheduled-edit dialog to offer an "Edit as paycheck →" relaunch
	// path alongside Save / Cancel (MS-029).
	DialogActionAlternate
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
	// message is a neutrally-styled multi-line body block rendered between
	// the title separator and the fields (or buttons, when there are no
	// fields). Used by info/about dialogs that have no form to fill in.
	message string
	// maxHeight bounds the dialog's total rendered height (border + padding
	// included) so a form taller than the terminal scrolls its field region
	// instead of overflowing past the status bar. Zero means unbounded —
	// the legacy behavior, byte-for-byte unchanged. Set per-frame by the
	// app from the terminal height.
	maxHeight int
	// fieldScroll is the current scroll offset, in field-block lines, of the
	// scrollable field region. It is (re)clamped in Render to keep the
	// focused field visible, mirroring the Table/Sidebar cursor-follow
	// idiom; it is only meaningful when the dialog is height-bounded and
	// overflowing.
	fieldScroll int
}

// NewDialog creates a new Dialog with the given title and default Save/Cancel
// buttons. The primary action sits on the left so a keyboard user tabbing
// through the form lands on it first; Esc still cancels.
func NewDialog(title string) *Dialog {
	return &Dialog{
		title: title,
		width: 56,
		buttons: []DialogButton{
			{Label: "Save", Primary: true},
			{Label: "Cancel"},
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
//
// Nil-safe on purpose. A nil *Dialog stored in an interface value is not a nil
// interface, so the modal registry in package tui — which walks every surface
// whether or not it has been built — would dereference a nil receiver here.
// Guarding at the implementation rather than at each call site is what lets
// that registry drop the 31 hand-written `X != nil && X.IsVisible()` gates.
func (d *Dialog) IsVisible() bool {
	return d != nil && d.visible
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

// MaxHeight returns the dialog's height bound (0 = unbounded).
func (d *Dialog) MaxHeight() int {
	return d.maxHeight
}

// SetMaxHeight bounds the dialog's total rendered height (border + padding
// included). When the form's natural height exceeds this, the field region
// scrolls to keep the focused field visible and a scrollbar is drawn, so the
// dialog never overflows past the terminal/status bar. Pass 0 to disable the
// bound (legacy behavior). Negative values are treated as 0.
func (d *Dialog) SetMaxHeight(h int) {
	if h < 0 {
		h = 0
	}
	d.maxHeight = h
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

// Message returns the neutral message body, if any.
func (d *Dialog) Message() string {
	return d.message
}

// SetMessage sets a neutrally-styled multi-line body block (newline-
// separated) rendered between the title and the buttons. Used for
// informational dialogs that have no fields.
func (d *Dialog) SetMessage(msg string) {
	d.message = msg
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
	total := d.FocusableCount()
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
	total := d.FocusableCount()
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

// FocusableCount returns the total number of focusable elements.
func (d *Dialog) FocusableCount() int {
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
	total := d.FocusableCount()
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
