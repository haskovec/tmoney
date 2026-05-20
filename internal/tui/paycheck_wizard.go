package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// PaycheckWizard is the guided form for creating a multi-line
// scheduled paycheck. The saved record is a standard multi-line
// scheduled transaction — there is no `kind` field or paycheck-
// specific table — so the wizard is pure UI sugar on top of the
// generic split-schedule primitive (see
// specs/multiline-splits-and-paycheck.md, "Paycheck Wizard").
//
// The wizard renders a single modal organized into:
//   - A header block of scalar fields (employer, frequency, next
//     payday, deposit account, memo).
//   - Three sections (Pre-Tax / Taxes / Post-Tax). Each section
//     starts empty; the user clicks `+ Add` to append a row, and
//     each row exposes a `−` to remove itself. Sections are purely
//     organizational — the saved schedule flattens them into one
//     list of splits.
//   - A live "net deposit" total computed from the signed sum of
//     every row's amount.
//   - Save / Cancel buttons.
//
// Each row's category-or-transfer select shows the entire category
// list followed by `→ <Account>` entries; picking an account
// converts the line to a transfer-line (category_id NULL,
// transfer_account_id set).
type PaycheckWizard struct {
	width   int
	visible bool

	// Header fields.
	employerField   *Field // text — employer payee name
	frequencyField  *Field // select — paycheck frequency picker
	nextPaydayField *Field // text — schedule start date (MM/DD/YYYY)
	accountField    *Field // select — primary deposit account
	memoField       *Field // text — optional memo

	// Three sections of rows, indexed by PaycheckSection. Each section
	// is empty by default; rows are appended via AddRow.
	sections [3][]*PaycheckLine

	// combinedOptions is the category-or-transfer picker's option list.
	// It is `categoryOptions` followed by `→ <Account>` entries — one
	// for each account in accountOptions. Indices ≥ len(categoryOptions)
	// indicate the row is a transfer-line.
	combinedOptions []string

	// Lookups used at save time to map selected indices to IDs.
	categoryOptions []string
	categoryIDs     []types.ID
	accountOptions  []string
	accountIDs      []types.ID

	// Focus state. focusIndex is an index into the focusables list
	// recomputed each render/key handle by collectFocusables().
	focusIndex int

	// errorMsg surfaces validation failures inline.
	errorMsg string

	// hitZones records click targets in content-local coordinates,
	// rebuilt on each Render so HandleMouse can dispatch clicks to
	// the matching focusable.
	hitZones []wizardHitZone
}

// wizardHitZone is one clickable region recorded during Render.
// Coordinates are content-local (relative to the inside of the
// dialog box, after border + padding).
type wizardHitZone struct {
	row    int
	colMin int
	colMax int
	target wizardFocusTarget
}

// PaycheckSection identifies which of the wizard's three visual
// groupings a row belongs to. Sections are purely organizational —
// the saved schedule is just a flat list of splits.
type PaycheckSection int

const (
	PaycheckPreTax PaycheckSection = iota
	PaycheckTax
	PaycheckPostTax
)

func (s PaycheckSection) Title() string {
	switch s {
	case PaycheckPreTax:
		return "PRE-TAX"
	case PaycheckTax:
		return "TAXES"
	case PaycheckPostTax:
		return "POST-TAX"
	}
	return ""
}

// PaycheckLine is one row in a section: a category-or-transfer
// select plus an amount input. The line is rendered with a [−]
// remove button.
type PaycheckLine struct {
	Section     PaycheckSection
	selectField *Field // category or transfer picker (combined list)
	amountField *Field // signed amount as typed by the user

	// categoryCount is captured at construction so the line can
	// self-classify (IsTransfer/CategoryIndex/AccountIndex) without
	// holding a back-reference to the wizard.
	categoryCount int
}

// SelectField exposes the line's category-or-transfer select for
// tests and key handling.
func (l *PaycheckLine) SelectField() *Field { return l.selectField }

// AmountField exposes the line's amount input.
func (l *PaycheckLine) AmountField() *Field { return l.amountField }

// IsTransfer reports whether the line's current select points at
// the transfer half of the combined picker (i.e. an account
// destination rather than a category).
func (l *PaycheckLine) IsTransfer() bool {
	if l.selectField == nil {
		return false
	}
	return l.selectField.SelectedIndex >= l.categoryCount
}

// CategoryIndex returns the index into categoryOptions for a
// categorized line (or 0 — the "(None)" sentinel — for transfer
// lines).
func (l *PaycheckLine) CategoryIndex() int {
	if l.IsTransfer() || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex
}

// AccountIndex returns the index into accountOptions for a
// transfer-line (or 0 for categorized lines).
func (l *PaycheckLine) AccountIndex() int {
	if !l.IsTransfer() || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex - l.categoryCount
}

// SetCategoryIndex makes the line categorized with the given category
// option index. Out-of-range indices are clamped to 0.
func (l *PaycheckLine) SetCategoryIndex(idx int) {
	if l.selectField == nil {
		return
	}
	if idx < 0 || idx >= l.categoryCount {
		idx = 0
	}
	l.selectField.SelectedIndex = idx
}

// SetAccountIndex converts the line into a transfer-line targeting
// the given account index.
func (l *PaycheckLine) SetAccountIndex(idx int) {
	if l.selectField == nil {
		return
	}
	target := l.categoryCount + idx
	if target < l.categoryCount || target >= len(l.selectField.Options) {
		return
	}
	l.selectField.SelectedIndex = target
}

// paycheckFrequencyOption is one entry in the wizard's frequency
// picker. Unlike the generic frequency picker (which exposes a bare
// "Semi-Monthly" option), the paycheck picker offers the two common
// preset day-pairs explicitly: 1st & 15th and 15th & last day.
type paycheckFrequencyOption struct {
	label               string
	frequency           scheduled.Frequency
	dayOfMonth          int // 0 = don't set; 1-31 = specific day; -1 = last day of month
	secondaryDayOfMonth int // 0 = don't set; 1-31 = specific day; -1 = last day of month
}

// paycheckFrequencyOptions is the wizard's frequency picker. Only
// paycheck-realistic cadences appear (no Daily / Quarterly / Yearly).
// Semi-monthly fans out into the two common day-pair variants.
var paycheckFrequencyOptions = []paycheckFrequencyOption{
	{label: "Weekly", frequency: scheduled.FrequencyWeekly},
	{label: "Fortnightly (every 2 weeks)", frequency: scheduled.FrequencyBiweekly},
	{label: "Semi-Monthly (1st & 15th)", frequency: scheduled.FrequencySemiMonthly, dayOfMonth: 1, secondaryDayOfMonth: 15},
	{label: "Semi-Monthly (15th & last day)", frequency: scheduled.FrequencySemiMonthly, dayOfMonth: 15, secondaryDayOfMonth: -1},
	{label: "Monthly", frequency: scheduled.FrequencyMonthly},
}

const defaultPaycheckFrequencyIndex = 1

func buildPaycheckFrequencyLabels() []string {
	labels := make([]string, len(paycheckFrequencyOptions))
	for i, opt := range paycheckFrequencyOptions {
		labels[i] = opt.label
	}
	return labels
}

func paycheckFrequencyForIndex(idx int) paycheckFrequencyOption {
	if idx < 0 || idx >= len(paycheckFrequencyOptions) {
		return paycheckFrequencyOptions[defaultPaycheckFrequencyIndex]
	}
	return paycheckFrequencyOptions[idx]
}

// paycheckFrequencyIndexFor maps a schedule's frequency + day fields
// back to the wizard's picker index. Used by Edit-as-paycheck.
func paycheckFrequencyIndexFor(st *scheduled.Transaction) int {
	if st == nil {
		return defaultPaycheckFrequencyIndex
	}
	primary, secondary := 0, 0
	if st.DayOfMonth.Valid {
		primary = int(st.DayOfMonth.Int64)
	}
	if st.SecondaryDayOfMonth.Valid {
		secondary = int(st.SecondaryDayOfMonth.Int64)
	}
	for i, opt := range paycheckFrequencyOptions {
		if opt.frequency != st.Frequency {
			continue
		}
		if opt.frequency == scheduled.FrequencySemiMonthly {
			if opt.dayOfMonth == primary && opt.secondaryDayOfMonth == secondary {
				return i
			}
			continue
		}
		return i
	}
	return defaultPaycheckFrequencyIndex
}

// findCategoryOptionIndex returns the index of displayName in
// options, or 0 if not found.
func findCategoryOptionIndex(options []string, displayName string) int {
	for i, s := range options {
		if s == displayName {
			return i
		}
	}
	return 0
}

// NewPaycheckWizard builds a wizard with empty sections. The user
// fills in scalar header fields and adds rows under each section
// before saving.
//
// categoryOptions / categoryIDs come from buildCategoryOptions (the
// leading "(None)" entry at index 0). accounts is filtered to
// active accounts for the picker.
func NewPaycheckWizard(categoryOptions []string, categoryIDs []types.ID, accounts []*account.Account) *PaycheckWizard {
	accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)

	combined := make([]string, 0, len(categoryOptions)+len(accountOptions))
	combined = append(combined, categoryOptions...)
	for _, name := range accountOptions {
		combined = append(combined, "→ "+name)
	}

	w := &PaycheckWizard{
		visible:         true,
		width:           80,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
		accountOptions:  accountOptions,
		accountIDs:      accountIDs,
		combinedOptions: combined,
	}

	w.employerField = &Field{
		Label:       "Employer",
		Type:        FieldText,
		Placeholder: "Payee name",
	}
	w.frequencyField = &Field{
		Label:         "Pay frequency",
		Type:          FieldSelect,
		Options:       buildPaycheckFrequencyLabels(),
		SelectedIndex: defaultPaycheckFrequencyIndex,
	}
	w.nextPaydayField = &Field{
		Label:       "Next payday",
		Type:        FieldText,
		Value:       time.Now().Format("01/02/2006"),
		Placeholder: "MM/DD/YYYY",
		Width:       12,
	}
	w.accountField = &Field{
		Label:         "Deposit account",
		Type:          FieldSelect,
		Options:       accountOptions,
		SelectedIndex: 0,
	}
	w.memoField = &Field{
		Label:       "Memo",
		Type:        FieldText,
		Placeholder: "Optional",
	}

	return w
}

// IsVisible reports whether the wizard should render.
func (w *PaycheckWizard) IsVisible() bool { return w != nil && w.visible }

// Structural accessors used by tests.
func (w *PaycheckWizard) Employer() *Field              { return w.employerField }
func (w *PaycheckWizard) Frequency() *Field             { return w.frequencyField }
func (w *PaycheckWizard) NextPayday() *Field            { return w.nextPaydayField }
func (w *PaycheckWizard) DepositAccount() *Field        { return w.accountField }
func (w *PaycheckWizard) Memo() *Field                  { return w.memoField }
func (w *PaycheckWizard) PrimaryAccount() *Field        { return w.accountField } // back-compat
func (w *PaycheckWizard) PreTaxLines() []*PaycheckLine  { return w.sections[PaycheckPreTax] }
func (w *PaycheckWizard) TaxLines() []*PaycheckLine     { return w.sections[PaycheckTax] }
func (w *PaycheckWizard) PostTaxLines() []*PaycheckLine { return w.sections[PaycheckPostTax] }
func (w *PaycheckWizard) Sections() [3][]*PaycheckLine  { return w.sections }

// AddRow appends an empty row to the given section and returns it.
// The amount and category select default to empty/(None) and the
// line is positioned as the last focusable target in its section.
func (w *PaycheckWizard) AddRow(section PaycheckSection) *PaycheckLine {
	if section < PaycheckPreTax || section > PaycheckPostTax {
		return nil
	}
	line := &PaycheckLine{
		Section: section,
		amountField: &Field{
			Type:        FieldText,
			Placeholder: "0.00",
			Width:       14,
		},
		selectField: &Field{
			Type:          FieldSelect,
			Options:       w.combinedOptions,
			SelectedIndex: 0,
		},
		categoryCount: len(w.categoryOptions),
	}
	w.sections[section] = append(w.sections[section], line)
	return line
}

// RemoveRow removes the given row from its section. Best-effort: a
// nil line or a line not found in any section is a no-op.
func (w *PaycheckWizard) RemoveRow(line *PaycheckLine) {
	if line == nil {
		return
	}
	for s := PaycheckPreTax; s <= PaycheckPostTax; s++ {
		for i, l := range w.sections[s] {
			if l == line {
				w.sections[s] = append(w.sections[s][:i], w.sections[s][i+1:]...)
				return
			}
		}
	}
}

// BuildSplits assembles the wizard's row state into a list of
// scheduled-split rows and computes the parent net amount (the signed
// sum). Empty amount fields are skipped — they don't produce zero-
// amount rows. Returns an error when validation fails (unparseable
// amount, missing category/account on a populated row).
func (w *PaycheckWizard) BuildSplits() (types.Money, []*scheduled.Split, error) {
	if w == nil {
		return types.ZeroMoney, nil, fmt.Errorf("nil wizard")
	}

	splits := make([]*scheduled.Split, 0)

	for s := PaycheckPreTax; s <= PaycheckPostTax; s++ {
		for _, line := range w.sections[s] {
			sp, err := w.buildLineSplit(line)
			if err != nil {
				return types.ZeroMoney, nil, err
			}
			if sp != nil {
				splits = append(splits, sp)
			}
		}
	}

	if len(splits) == 0 {
		return types.ZeroMoney, nil, fmt.Errorf("add at least one row")
	}

	parent := types.ZeroMoney
	for _, s := range splits {
		parent = parent.Add(s.Amount)
	}
	return parent, splits, nil
}

// buildLineSplit translates a single line into a scheduled.Split.
// Returns (nil, nil) for an empty/zero-amount row (silently skipped).
// The user's typed amount is preserved verbatim, including its sign —
// unlike the old wizard, the new flow does not flip signs because
// the user explicitly chooses positive/negative per row.
func (w *PaycheckWizard) buildLineSplit(line *PaycheckLine) (*scheduled.Split, error) {
	amtStr := strings.TrimSpace(line.amountField.Value)
	if amtStr == "" {
		return nil, nil
	}
	amt, err := parseAmountInput(amtStr)
	if err != nil {
		return nil, fmt.Errorf("%s row: %w", line.Section.Title(), err)
	}
	if amt.IsZero() {
		return nil, nil
	}

	sp := &scheduled.Split{
		BaseModel: types.NewBaseModel(),
		Amount:    amt,
	}
	if line.IsTransfer() {
		accountID := w.lookupAccountID(line.AccountIndex())
		if accountID.IsNil() {
			return nil, fmt.Errorf("%s row: pick a transfer destination", line.Section.Title())
		}
		sp.TransferAccountID = types.NullableID{ID: accountID, Valid: true}
		return sp, nil
	}
	catID := w.lookupCategoryID(line.CategoryIndex())
	if catID.IsNil() {
		return nil, fmt.Errorf("%s row: pick a category", line.Section.Title())
	}
	sp.CategoryID = types.NullableID{ID: catID, Valid: true}
	return sp, nil
}

// lookupCategoryID maps a select index to a category ID.
func (w *PaycheckWizard) lookupCategoryID(idx int) types.ID {
	if idx <= 0 || idx >= len(w.categoryIDs) {
		return types.NilID
	}
	return w.categoryIDs[idx]
}

// lookupAccountID maps a select index to an account ID.
func (w *PaycheckWizard) lookupAccountID(idx int) types.ID {
	if idx < 0 || idx >= len(w.accountIDs) {
		return types.NilID
	}
	return w.accountIDs[idx]
}

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
	field   *Field
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
	for s := PaycheckPreTax; s <= PaycheckPostTax; s++ {
		for _, line := range w.sections[s] {
			out = append(out,
				wizardFocusTarget{kind: wizardFocusField, field: line.selectField},
				wizardFocusTarget{kind: wizardFocusField, field: line.amountField},
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
// Render
// ===========================================================================

// Render returns the wizard's overlay-ready string. As a side-effect
// it rebuilds w.hitZones so HandleMouse can dispatch clicks.
func (w *PaycheckWizard) Render(styles Styles) string {
	if w == nil || !w.visible {
		return ""
	}
	w.clampFocus()
	focused := w.focusedTarget()

	contentWidth := max(w.width-dialogHorizontalOverhead, 40)
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

	// Title row with [×] close button on the right.
	closeBtn := styles.Muted.Render("[x]")
	titleText := styles.DialogTitle.Render("PAYCHECK SCHEDULE")
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
	headerFields := []*Field{w.employerField, w.frequencyField, w.nextPaydayField, w.accountField, w.memoField}
	for _, f := range headerFields {
		row, zones := w.renderFieldRow(styles, fillStyle, f, focused.field == f, contentWidth, len(lines))
		addLine(row)
		w.hitZones = append(w.hitZones, zones...)
	}
	addLine("")

	// Sections.
	for s := PaycheckPreTax; s <= PaycheckPostTax; s++ {
		addLine(fillStyle.Render(styles.Bold.Render(s.Title())))
		addLine(fillStyle.Render(strings.Repeat("─", contentWidth)))

		for _, line := range w.sections[s] {
			row, zones := w.renderLine(styles, fillStyle, line, focused, contentWidth, len(lines))
			addLine(row)
			w.hitZones = append(w.hitZones, zones...)
		}

		addLabel := fmt.Sprintf("[+ Add %s row]", strings.ToLower(s.Title()))
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
	// project convention from NewDialog.
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
	btnGap := max(contentWidth-lipgloss.Width(saveLabel)-lipgloss.Width(cancelLabel), 4)
	buttonsRow := saveLabel + gap(btnGap) + cancelLabel
	w.hitZones = append(w.hitZones,
		wizardHitZone{
			row:    len(lines),
			colMin: 0,
			colMax: lipgloss.Width(saveText),
			target: wizardFocusTarget{kind: wizardFocusSave},
		},
		wizardHitZone{
			row:    len(lines),
			colMin: lipgloss.Width(saveLabel) + btnGap,
			colMax: lipgloss.Width(saveLabel) + btnGap + lipgloss.Width(cancelText),
			target: wizardFocusTarget{kind: wizardFocusCancel},
		},
	)
	addLine(buttonsRow)

	content := strings.Join(lines, "\n")
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
// the rendered string plus any hit zones for the value cell.
func (w *PaycheckWizard) renderFieldRow(styles Styles, fill lipgloss.Style, f *Field, focused bool, contentWidth, row int) (string, []wizardHitZone) {
	if f == nil {
		return "", nil
	}
	labelW := 18
	label := padFill(fill.Render(f.Label+":"), labelW, fill)

	valueWidth := contentWidth - labelW - 1
	value := w.renderFieldValue(styles, fill, f, focused, valueWidth)
	out := label + fill.Render(" ") + value
	zones := []wizardHitZone{{
		row:    row,
		colMin: labelW + 1,
		colMax: labelW + 1 + valueWidth,
		target: wizardFocusTarget{kind: wizardFocusField, field: f},
	}}
	return out, zones
}

// renderLine renders one section row: select + amount + [−] remove.
// Returns the rendered string plus hit zones for the select, amount,
// and remove cells.
func (w *PaycheckWizard) renderLine(styles Styles, fill lipgloss.Style, line *PaycheckLine, focused wizardFocusTarget, contentWidth, row int) (string, []wizardHitZone) {
	selW := max(contentWidth/2-8, 20)
	amtW := 14

	selFocused := focused.field == line.selectField
	amtFocused := focused.field == line.amountField

	selStr := padFill(w.renderFieldValue(styles, fill, line.selectField, selFocused, selW), selW, fill)
	amtStr := padFill(w.renderFieldValue(styles, fill, line.amountField, amtFocused, amtW), amtW, fill)

	removeText := "[−]"
	var removeLabel string
	if focused.kind == wizardFocusRemove && focused.line == line {
		removeLabel = styles.DialogButtonFocused.Render(removeText)
	} else {
		removeLabel = styles.DialogButton.Render(removeText)
	}

	prefix := fill.Render("  ")
	pre := 2
	out := prefix + selStr + fill.Render(" ") + amtStr + fill.Render(" ") + removeLabel

	zones := []wizardHitZone{
		{
			row:    row,
			colMin: pre,
			colMax: pre + selW,
			target: wizardFocusTarget{kind: wizardFocusField, field: line.selectField},
		},
		{
			row:    row,
			colMin: pre + selW + 1,
			colMax: pre + selW + 1 + amtW,
			target: wizardFocusTarget{kind: wizardFocusField, field: line.amountField},
		},
		{
			row:    row,
			colMin: pre + selW + 1 + amtW + 1,
			colMax: pre + selW + 1 + amtW + 1 + lipgloss.Width(removeText),
			target: wizardFocusTarget{kind: wizardFocusRemove, line: line},
		},
	}
	return out, zones
}

// renderFieldValue draws a field's value with focus highlight as
// appropriate for its type. fill is the dialog-bg style used for
// padding inside the value cell so the brackets fill uniformly.
func (w *PaycheckWizard) renderFieldValue(styles Styles, fill lipgloss.Style, f *Field, focused bool, width int) string {
	if f == nil {
		return fill.Render(strings.Repeat(" ", width))
	}
	openBracket := fill.Render("[")
	closeBracket := fill.Render("]")
	innerWidth := width - 2
	var inner string
	switch f.Type {
	case FieldText:
		val := f.Value
		if val == "" && !focused {
			val = styles.Placeholder.Render(f.Placeholder)
		}
		inner = padFill(val, innerWidth, fill)
	case FieldSelect:
		val := ""
		if f.SelectedIndex >= 0 && f.SelectedIndex < len(f.Options) {
			val = f.Options[f.SelectedIndex]
		}
		// Reserve trailing " ▼" inside the brackets.
		inner = padFill(val, innerWidth-2, fill) + fill.Render(" ▼")
	default:
		inner = padFill(f.Value, innerWidth, fill)
	}
	out := openBracket + inner + closeBracket
	if focused {
		out = styles.SelectedRow.Render(out)
	}
	return out
}

// computeTotal returns the signed sum of every populated row.
func (w *PaycheckWizard) computeTotal() types.Money {
	total := types.ZeroMoney
	for s := PaycheckPreTax; s <= PaycheckPostTax; s++ {
		for _, line := range w.sections[s] {
			amtStr := strings.TrimSpace(line.amountField.Value)
			if amtStr == "" {
				continue
			}
			amt, err := parseAmountInput(amtStr)
			if err != nil {
				continue
			}
			total = total.Add(amt)
		}
	}
	return total
}

// ===========================================================================
// Key handling
// ===========================================================================

// HandleKey dispatches a key event into the wizard and returns the
// action the parent App should take. DialogActionSubmit fires when
// Enter on Save; DialogActionCancel fires when Esc or Enter on
// Cancel. Other actions are absorbed.
func (w *PaycheckWizard) HandleKey(msg tea.KeyPressMsg) DialogAction {
	if w == nil {
		return DialogActionNone
	}
	w.errorMsg = ""
	w.clampFocus()

	keyStr := msg.String()
	switch keyStr {
	case "esc":
		return DialogActionCancel
	case "tab":
		w.focusIndex++
		w.clampFocus()
		return DialogActionNone
	case "shift+tab":
		w.focusIndex--
		w.clampFocus()
		return DialogActionNone
	case "enter":
		return w.handleEnter()
	}

	target := w.focusedTarget()
	switch target.kind {
	case wizardFocusField:
		w.dispatchFieldKey(target.field, msg)
	}
	return DialogActionNone
}

func (w *PaycheckWizard) handleEnter() DialogAction {
	target := w.focusedTarget()
	if target.kind == wizardFocusField {
		// On a field: advance focus (don't activate).
		w.focusIndex++
		w.clampFocus()
		return DialogActionNone
	}
	return w.activate(target)
}

func (w *PaycheckWizard) dispatchFieldKey(f *Field, msg tea.KeyPressMsg) {
	if f == nil {
		return
	}
	switch f.Type {
	case FieldText:
		w.dispatchTextFieldKey(f, msg)
	case FieldSelect:
		w.dispatchSelectFieldKey(f, msg)
	}
}

func (w *PaycheckWizard) dispatchTextFieldKey(f *Field, msg tea.KeyPressMsg) {
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

func (w *PaycheckWizard) dispatchSelectFieldKey(f *Field, msg tea.KeyPressMsg) {
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
func (w *PaycheckWizard) renderedHeightFor(styles Styles) int {
	rendered := w.Render(styles)
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

// dialogBounds returns the screen-space bounding box of the wizard
// when centered on a screenWidth × screenHeight terminal.
func (w *PaycheckWizard) dialogBounds(styles Styles, screenWidth, screenHeight int) (startCol, startRow, endCol, endRow int) {
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
func (w *PaycheckWizard) HandleMouse(msg tea.MouseMsg, styles Styles, screenWidth, screenHeight int) DialogAction {
	if w == nil {
		return DialogActionNone
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return DialogActionNone
	}

	startCol, startRow, endCol, endRow := w.dialogBounds(styles, screenWidth, screenHeight)
	m := msg.Mouse()
	if m.X < startCol || m.X >= endCol || m.Y < startRow || m.Y >= endRow {
		return DialogActionNone
	}

	// Convert screen coords to content-local (inside border + padding).
	// styles.Dialog has Border(1) + Padding(1,2) by convention; match
	// the same offsets used by the standard Dialog.HandleMouse path.
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
	return DialogActionNone
}

// activate executes the action associated with a focus target. Used
// by both Enter on a focused element and HandleMouse on a clicked
// element.
func (w *PaycheckWizard) activate(target wizardFocusTarget) DialogAction {
	switch target.kind {
	case wizardFocusSave:
		w.focusToTarget(target)
		return DialogActionSubmit
	case wizardFocusCancel:
		return DialogActionCancel
	case wizardFocusAddRow:
		line := w.AddRow(target.section)
		// Move focus onto the new row's select field for convenience.
		focusables := w.collectFocusables()
		for i, f := range focusables {
			if f.kind == wizardFocusField && f.field == line.selectField {
				w.focusIndex = i
				break
			}
		}
		return DialogActionNone
	case wizardFocusRemove:
		w.RemoveRow(target.line)
		w.clampFocus()
		return DialogActionNone
	case wizardFocusField:
		w.focusToTarget(target)
		return DialogActionNone
	}
	return DialogActionNone
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

// ===========================================================================
// App integration
// ===========================================================================

// paycheckWizardDataMsg carries the dependencies needed to construct
// a PaycheckWizard. Dispatched asynchronously by loadPaycheckWizardData.
type paycheckWizardDataMsg struct {
	accounts        []*account.Account
	categoryOptions []string
	categoryIDs     []types.ID
}

// loadPaycheckWizardData fetches accounts + categories and emits a
// paycheckWizardDataMsg that the message handler in app_update.go
// uses to construct the wizard.
func (a *App) loadPaycheckWizardData() tea.Cmd {
	return func() tea.Msg {
		var accounts []*account.Account
		if a.accountSvc != nil {
			acs, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}

		var categories []*category.Category
		if a.categorySvc != nil {
			cs, err := a.categorySvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			categories = cs
		}

		categoryOptions, categoryIDs := buildCategoryOptions(categories)
		return paycheckWizardDataMsg{
			accounts:        accounts,
			categoryOptions: categoryOptions,
			categoryIDs:     categoryIDs,
		}
	}
}

// closePaycheckWizard clears the wizard state.
func (a *App) closePaycheckWizard() {
	a.paycheckWizard = nil
}

// handlePaycheckWizardKey routes a key event through the wizard and
// translates the resulting action.
func (a *App) handlePaycheckWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.paycheckWizard == nil {
		return a, nil
	}
	action := a.paycheckWizard.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitPaycheckWizard()
	case DialogActionCancel:
		a.closePaycheckWizard()
		return a, nil
	}
	return a, nil
}

// submitPaycheckWizard validates the wizard's state and persists the
// schedule. Validation errors leave the wizard open with errorMsg set.
func (a *App) submitPaycheckWizard() (tea.Model, tea.Cmd) {
	w := a.paycheckWizard
	if w == nil {
		return a, nil
	}

	accountID := w.lookupAccountID(w.accountField.SelectedIndex)
	if accountID.IsNil() {
		w.errorMsg = "Pick a deposit account"
		return a, nil
	}

	startDate, err := parseDateInput(w.nextPaydayField.Value)
	if err != nil {
		w.errorMsg = "Next payday: invalid date (MM/DD/YYYY)"
		return a, nil
	}

	freqOpt := paycheckFrequencyForIndex(w.frequencyField.SelectedIndex)

	parentAmount, splits, err := w.BuildSplits()
	if err != nil {
		w.errorMsg = err.Error()
		return a, nil
	}

	// Reject self-transfers: a row that targets the deposit account.
	for _, sp := range splits {
		if sp.TransferAccountID.Valid && sp.TransferAccountID.ID == accountID {
			w.errorMsg = "A transfer row's destination cannot be the deposit account"
			return a, nil
		}
	}

	employer := strings.TrimSpace(w.employerField.Value)
	memo := strings.TrimSpace(w.memoField.Value)

	a.closePaycheckWizard()

	return a, func() tea.Msg {
		var payeeID types.ID
		if employer != "" && a.payeeSvc != nil {
			py, _, err := a.payeeSvc.GetOrCreate(employer)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = py.ID
		}

		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		st := scheduled.NewTransaction(accountID, freqOpt.frequency, startDate)
		if freqOpt.dayOfMonth != 0 {
			st.DayOfMonth = types.NullableInt{Int64: int64(freqOpt.dayOfMonth), Valid: true}
		}
		if freqOpt.secondaryDayOfMonth != 0 {
			st.SecondaryDayOfMonth = types.NullableInt{Int64: int64(freqOpt.secondaryDayOfMonth), Valid: true}
		}
		st.SetAmount(parentAmount)
		if !payeeID.IsNil() {
			st.SetPayee(payeeID)
		}
		st.ClearCategory()
		if memo != "" {
			st.SetMemo(memo)
		}
		st.Splits = scheduled.SplitCollection(splits)

		cmd := undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, st)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
		}
		return scheduledDialogSavedMsg{}
	}
}

// ===========================================================================
// Edit-as-paycheck heuristic + relaunch
// ===========================================================================

// looksLikePaycheck reports whether a scheduled transaction matches
// the paycheck heuristic: multi-line, at least one categorized
// positive-amount split, and at least one categorized negative
// split whose category display name starts with "Tax > ".
func looksLikePaycheck(st *scheduled.Transaction, categoryOptions []string, categoryIDs []types.ID) bool {
	if st == nil || len(st.Splits) == 0 {
		return false
	}
	nameByID := make(map[types.ID]string, len(categoryIDs))
	for i, id := range categoryIDs {
		if i < len(categoryOptions) {
			nameByID[id] = categoryOptions[i]
		}
	}

	var hasIncomeLine, hasTaxLine bool
	for _, sp := range st.Splits {
		if !sp.CategoryID.Valid {
			continue
		}
		name := nameByID[sp.CategoryID.ID]
		switch {
		case sp.Amount.IsPositive():
			hasIncomeLine = true
		case sp.Amount.IsNegative() && strings.HasPrefix(name, "Tax > "):
			hasTaxLine = true
		}
	}
	return hasIncomeLine && hasTaxLine
}

// NewPaycheckWizardFromSchedule builds a paycheck wizard pre-filled
// from a multi-line scheduled transaction. Sections are inferred via
// a heuristic that matches the looksLikePaycheck classification:
//   - positive categorized → PreTax (gross income)
//   - negative categorized whose display name starts with "Tax > " → Tax
//   - everything else → PostTax (transfers, health, etc.)
func NewPaycheckWizardFromSchedule(
	st *scheduled.Transaction,
	accounts []*account.Account,
	payees []*payee.Payee,
	categoryOptions []string,
	categoryIDs []types.ID,
) *PaycheckWizard {
	w := NewPaycheckWizard(categoryOptions, categoryIDs, accounts)
	if st == nil {
		return w
	}

	if st.HasPayee() {
		for _, p := range payees {
			if p == nil {
				continue
			}
			if p.ID == st.PayeeID.ID {
				w.employerField.Value = p.Name
				break
			}
		}
	}

	w.frequencyField.SelectedIndex = paycheckFrequencyIndexFor(st)
	w.nextPaydayField.Value = st.NextDate.Time().Format("01/02/2006")

	for i := range w.accountField.Options {
		if i >= len(w.accountIDs) {
			break
		}
		if w.accountIDs[i] == st.AccountID {
			w.accountField.SelectedIndex = i
			break
		}
	}

	if st.Memo.Valid {
		w.memoField.Value = st.Memo.String
	}

	categoryNameByID := make(map[types.ID]string, len(categoryIDs))
	for i, id := range categoryIDs {
		if i < len(categoryOptions) {
			categoryNameByID[id] = categoryOptions[i]
		}
	}

	for _, sp := range st.Splits {
		if sp == nil {
			continue
		}
		section := PaycheckPostTax
		var (
			selectIdx int
		)
		if sp.TransferAccountID.Valid {
			// Find the account in accountIDs to build the combined index.
			for i, id := range w.accountIDs {
				if id == sp.TransferAccountID.ID {
					selectIdx = len(w.categoryOptions) + i
					break
				}
			}
		} else if sp.CategoryID.Valid {
			name := categoryNameByID[sp.CategoryID.ID]
			if idx := findCategoryOptionIndex(w.categoryOptions, name); idx > 0 {
				selectIdx = idx
			}
			switch {
			case sp.Amount.IsPositive():
				section = PaycheckPreTax
			case strings.HasPrefix(name, "Tax > "):
				section = PaycheckTax
			}
		}

		line := w.AddRow(section)
		line.selectField.SelectedIndex = selectIdx
		line.amountField.Value = sp.Amount.String()
	}

	return w
}

// relaunchAsPaycheckWizard closes the scheduled-edit dialog and
// opens the paycheck wizard pre-filled from the in-flight schedule.
func (a *App) relaunchAsPaycheckWizard() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return a, nil
	}
	if a.schedDialogData.mode != scheduledDialogModeEdit || a.schedDialogData.scheduled == nil {
		return a, nil
	}
	st := a.schedDialogData.scheduled
	accounts := a.schedDialogData.accounts
	payees := a.schedDialogData.payees
	categoryOptions := a.schedDialogCategoryOptions
	categoryIDs := a.schedDialogCategoryIDs

	a.closeScheduledDialog()
	a.paycheckWizard = NewPaycheckWizardFromSchedule(st, accounts, payees, categoryOptions, categoryIDs)
	return a, nil
}
