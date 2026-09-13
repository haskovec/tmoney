package tui

import (
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// The paycheck wizard's declarations: the wizard and section types, the
// frequency picker table, the constructor, and the accessors the tests read.
// The other paycheck_wizard_*.go files each hold one cluster of its behaviour;
// the App glue is in paycheck_wizard_app.go.

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
//   - Five sections mirroring US pay-stub structure (Earnings /
//     Pre-Tax / Taxes / Post-Tax / Net Pay Destinations). Earnings
//     opens with one Income:Salary row; Taxes opens with three rows
//     (Federal, Social Security, Medicare); the rest start empty.
//     The user clicks `+ Add` to append a row in any mutable
//     section, and each row exposes a `−` to remove itself.
//     Net Pay Destinations holds *additional* transfers — the
//     primary deposit lives in the header's account picker.
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
	employerField   *dialog.Field // text — employer payee name
	frequencyField  *dialog.Field // select — paycheck frequency picker
	nextPaydayField *dialog.Field // text — schedule start date (MM/DD/YYYY)
	accountField    *dialog.Field // select — primary deposit account
	memoField       *dialog.Field // text — optional memo

	// Five sections of rows, indexed by PaycheckSection. Earnings and
	// Taxes are pre-populated per the v2 spec; the other sections
	// start empty. Additional rows are appended via AddRow.
	sections [5][]*PaycheckLine

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

	// editSchedule is the schedule this wizard was launched from via
	// Edit-as-paycheck. nil means create mode. In edit mode the save path
	// mutates this record in place and dispatches an Edit command, so the
	// schedule keeps its identity and every field the wizard has no widget
	// for (interval, end date, occurrences + remaining, auto-post, lead
	// days, amount-estimate count) survives the save.
	editSchedule *scheduled.Transaction

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

// paycheckAddNewSentinelLabel is the action-row label appended to every
// paycheck-line select field's option list. Enter on this entry diverts into
// the inline create-category sub-dialog (mirrors the [+ Add new category…]
// row that appears in the typeahead-combo surfaces, but here the user
// navigates to it with Up/Down rather than typing to filter).
const paycheckAddNewSentinelLabel = "[+ Add new category…]"

// PaycheckSection identifies which of the wizard's five visual
// groupings a row belongs to. Sections are organizational in the UI
// but also drive the `paycheck_section` tag persisted on each split
// for exact round-trip in the Edit-as-paycheck flow (see
// specs/multiline-splits-and-paycheck.md, "Section tagging").
type PaycheckSection int

const (
	PaycheckEarnings PaycheckSection = iota
	PaycheckPreTax
	PaycheckTax
	PaycheckPostTax
	PaycheckNetPayDestination
)

// tagString returns the value persisted in the split's
// `paycheck_section` column for rows belonging to this section. The
// strings match the CHECK constraint in migration 020 and are read
// back by NewPaycheckWizardFromSchedule (PW2-008) to route lines to
// their original section on Edit-as-paycheck.
func (s PaycheckSection) tagString() string {
	switch s {
	case PaycheckEarnings:
		return "earnings"
	case PaycheckPreTax:
		return "pre_tax"
	case PaycheckTax:
		return "tax"
	case PaycheckPostTax:
		return "post_tax"
	case PaycheckNetPayDestination:
		return "net_pay_destination"
	}
	return ""
}

func (s PaycheckSection) Title() string {
	switch s {
	case PaycheckEarnings:
		return "EARNINGS"
	case PaycheckPreTax:
		return "PRE-TAX"
	case PaycheckTax:
		return "TAXES"
	case PaycheckPostTax:
		return "POST-TAX"
	case PaycheckNetPayDestination:
		return "NET PAY DESTINATIONS"
	}
	return ""
}

// addRowLabel returns the "+ Add ..." button label shown at the
// bottom of the section. Matches the v2 spec mockup in
// specs/multiline-splits-and-paycheck.md (Net Pay Destinations uses
// "[+ Add transfer]" since rows there are always transfers).
func (s PaycheckSection) addRowLabel() string {
	switch s {
	case PaycheckEarnings:
		return "[+ Add earnings line]"
	case PaycheckPreTax:
		return "[+ Add pre-tax line]"
	case PaycheckTax:
		return "[+ Add tax line]"
	case PaycheckPostTax:
		return "[+ Add post-tax line]"
	case PaycheckNetPayDestination:
		return "[+ Add transfer]"
	}
	return "[+ Add row]"
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
// paycheck-realistic cadences appear (no Daily / Quarterly).
// Semi-monthly fans out into the two common day-pair variants;
// Yearly covers annual bonuses.
var paycheckFrequencyOptions = []paycheckFrequencyOption{
	{label: "Weekly", frequency: scheduled.FrequencyWeekly},
	{label: "Fortnightly (every 2 weeks)", frequency: scheduled.FrequencyFortnightly},
	{label: "Semi-Monthly (1st & 15th)", frequency: scheduled.FrequencySemiMonthly, dayOfMonth: 1, secondaryDayOfMonth: 15},
	{label: "Semi-Monthly (15th & last day)", frequency: scheduled.FrequencySemiMonthly, dayOfMonth: 15, secondaryDayOfMonth: -1},
	{label: "Monthly", frequency: scheduled.FrequencyMonthly},
	{label: "Yearly", frequency: scheduled.FrequencyYearly},
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

// NewPaycheckWizard builds a wizard with five sections, pre-populating
// Earnings with one Income:Salary row and Taxes with three rows
// (Federal/Social Security/Medicare) per the v2 spec
// (specs/multiline-splits-and-paycheck.md, "Pre-populated rows").
// Pre-tax, Post-tax, and Net Pay Destinations start empty — those
// items vary by employer and are added via the "[+ Add line]" button.
//
// categoryOptions / categoryIDs come from buildCategoryOptions (the
// leading "(None)" entry at index 0). accounts is filtered to
// active accounts for the picker.
func NewPaycheckWizard(categoryOptions []string, categoryIDs []types.ID, accounts []*account.Account) *PaycheckWizard {
	accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)

	combined := make([]string, 0, len(categoryOptions)+len(accountOptions)+1)
	combined = append(combined, categoryOptions...)
	for _, name := range accountOptions {
		combined = append(combined, "→ "+name)
	}
	combined = append(combined, paycheckAddNewSentinelLabel)

	w := &PaycheckWizard{
		visible:         true,
		width:           96,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
		accountOptions:  accountOptions,
		accountIDs:      accountIDs,
		combinedOptions: combined,
	}

	w.employerField = &dialog.Field{
		Label:       "Employer",
		Type:        dialog.FieldText,
		Placeholder: "Payee name",
	}
	w.frequencyField = &dialog.Field{
		Label:         "Pay frequency",
		Type:          dialog.FieldSelect,
		Options:       buildPaycheckFrequencyLabels(),
		SelectedIndex: defaultPaycheckFrequencyIndex,
	}
	w.nextPaydayField = &dialog.Field{
		Label:    "Next payday",
		Type:     dialog.FieldDate,
		Value:    time.Now().Format("01/02/2006"),
		Width:    10,
		DateMask: dialog.DateMaskUS,
	}
	w.accountField = &dialog.Field{
		Label:         "Deposit account",
		Type:          dialog.FieldSelect,
		Options:       accountOptions,
		SelectedIndex: 0,
	}
	w.memoField = &dialog.Field{
		Label:       "Memo",
		Type:        dialog.FieldText,
		Placeholder: "Optional",
	}

	// Pre-populate Earnings + Tax per the v2 spec. Rows whose default
	// category isn't found in categoryOptions still get added — the
	// select falls back to "(None)" (index 0) and the row's empty
	// amount means it's elided on save unless the user fills it in.
	w.seedSection(PaycheckEarnings, "Income > Salary")
	w.seedSection(PaycheckTax, "Tax > Federal", "Tax > Social Security", "Tax > Medicare")

	return w
}

// seedSection appends a row to the given section for each provided
// default category display name. Used by the v2 pre-population in
// NewPaycheckWizard.
func (w *PaycheckWizard) seedSection(section PaycheckSection, defaults ...string) {
	for _, name := range defaults {
		line := w.AddRow(section)
		if line == nil {
			continue
		}
		if idx := findCategoryOptionIndex(w.categoryOptions, name); idx > 0 {
			line.SetCategoryIndex(idx)
		}
	}
}

// IsVisible reports whether the wizard should render.
func (w *PaycheckWizard) IsVisible() bool { return w != nil && w.visible }

// SetVisible toggles the wizard's render flag. Used by the
// inline create-category sub-dialog flow to hide the wizard during the
// divert and restore it on cancel or post-create.
func (w *PaycheckWizard) SetVisible(v bool) {
	if w == nil {
		return
	}
	w.visible = v
}

// Structural accessors used by tests.
func (w *PaycheckWizard) Employer() *dialog.Field        { return w.employerField }
func (w *PaycheckWizard) Frequency() *dialog.Field       { return w.frequencyField }
func (w *PaycheckWizard) NextPayday() *dialog.Field      { return w.nextPaydayField }
func (w *PaycheckWizard) DepositAccount() *dialog.Field  { return w.accountField }
func (w *PaycheckWizard) Memo() *dialog.Field            { return w.memoField }
func (w *PaycheckWizard) PrimaryAccount() *dialog.Field  { return w.accountField } // back-compat
func (w *PaycheckWizard) EarningsLines() []*PaycheckLine { return w.sections[PaycheckEarnings] }
func (w *PaycheckWizard) PreTaxLines() []*PaycheckLine   { return w.sections[PaycheckPreTax] }
func (w *PaycheckWizard) TaxLines() []*PaycheckLine      { return w.sections[PaycheckTax] }
func (w *PaycheckWizard) PostTaxLines() []*PaycheckLine  { return w.sections[PaycheckPostTax] }
func (w *PaycheckWizard) AdditionalTransfers() []*PaycheckLine {
	return w.sections[PaycheckNetPayDestination]
}
func (w *PaycheckWizard) Sections() [5][]*PaycheckLine { return w.sections }
