package tui

import (
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// PaycheckWizard is the guided form for creating a multi-line
// scheduled paycheck. The saved record is a standard multi-line
// scheduled transaction — there is no `kind` field or paycheck-
// specific table — so the wizard is pure UI sugar on top of the
// generic split-schedule primitive (see
// specs/multiline-splits-and-paycheck.md, "Paycheck Wizard").
//
// MS-025 introduces the scaffolding only:
//   - The form opens visible with the static layout from the spec
//     (employer / frequency / next payday / gross pay, five pre-tax
//     deduction rows, two post-tax deduction rows, primary deposit
//     account, empty additional-transfers list).
//   - Each input field starts empty (amounts blank); category and
//     account selectors are seeded to the wizard's preset defaults
//     so the user can press Save without re-picking the obvious
//     choices.
//
// Save logic and the computed-remainder line land in MS-027/MS-028;
// the dynamic "[+ Add line]" affordances are deferred too.
type PaycheckWizard struct {
	visible bool
	width   int

	// Header fields.
	employer   Field // text — employer payee name
	frequency  Field // select — Frequency choices via scheduled.AllFrequencies
	nextPayday Field // date — schedule start date (today by default)

	// Gross pay row.
	grossAmount   Field // text — gross pay amount
	grossCategory Field // select — category for gross (default Income > Salary)

	// Deduction rows.
	preTax  []*PaycheckLine
	postTax []*PaycheckLine

	// Net pay destinations.
	primaryAccount      Field // select — first active account by default
	additionalTransfers []*PaycheckLine

	// Lookups used to map selected indices to IDs at save time
	// (MS-027/MS-028).
	categoryOptions []string
	categoryIDs     []types.ID
	accountOptions  []string
	accountIDs      []types.ID
}

// PaycheckLine is one row in the deductions or additional-transfers
// section of the wizard. A line is either categorized (a regular
// expense/income split) or a transfer to another account, mirroring
// the two split-item shapes in the underlying data model.
type PaycheckLine struct {
	// Label is the fixed label rendered to the left of the line's
	// fields (e.g. "Federal income tax", "401(k) contribution").
	Label string

	// amountField holds the entered amount for this line.
	amountField Field

	// isTransfer indicates the line targets a destination account
	// (transfer-line) instead of a category.
	isTransfer bool

	// categoryIndex is the selected index into the wizard's
	// categoryOptions slice when the line is categorized. The
	// wizard's constructor seeds this to the line's default
	// category when one is configured.
	categoryIndex int

	// accountIndex is the selected index into the wizard's
	// accountOptions slice when the line is a transfer-line. Zero
	// by default — the user picks the destination account
	// explicitly.
	accountIndex int
}

// AmountField returns a pointer to the line's amount input so the
// wizard's key handler (added in a later slice) can mutate it.
func (l *PaycheckLine) AmountField() *Field {
	return &l.amountField
}

// IsTransfer reports whether this line targets a destination
// account instead of a category.
func (l *PaycheckLine) IsTransfer() bool {
	return l.isTransfer
}

// CategoryIndex returns the selected option index for a categorized
// line. Meaningless for transfer-lines (always 0).
func (l *PaycheckLine) CategoryIndex() int {
	return l.categoryIndex
}

// AccountIndex returns the selected option index for a transfer-line.
// Meaningless for categorized lines (always 0).
func (l *PaycheckLine) AccountIndex() int {
	return l.accountIndex
}

// paycheckLineSpec configures one of the wizard's hardcoded
// deduction rows. The wizard's constructor walks these to build the
// pre-tax and post-tax slices in the order the spec lists.
type paycheckLineSpec struct {
	label string
	// defaultCategory is the display name (as produced by
	// buildCategoryOptions, e.g. "Tax > Federal") of the default
	// category for this line. Empty when transfer is true.
	defaultCategory string
	// transfer flags the line as a transfer-line (account picker
	// instead of category picker).
	transfer bool
}

// preTaxLineSpecs is the static set of pre-tax deduction rows the
// wizard renders, in the order the spec lists. Per MS-025 this is
// fixed; dynamic "+ Add pre-tax line" support is deferred.
var preTaxLineSpecs = []paycheckLineSpec{
	{label: "Federal income tax", defaultCategory: "Tax > Federal"},
	{label: "State income tax", defaultCategory: "Tax > State"},
	{label: "Social Security", defaultCategory: "Tax > Social Security"},
	{label: "Medicare", defaultCategory: "Tax > Medicare"},
	{label: "401(k) contribution", transfer: true},
}

// postTaxLineSpecs is the static set of post-tax deduction rows
// (health insurance, HSA transfer).
var postTaxLineSpecs = []paycheckLineSpec{
	{label: "Health insurance", defaultCategory: "Insurance > Health"},
	{label: "HSA contribution", transfer: true},
}

// NewPaycheckWizard builds the wizard with the static layout from
// the spec, seeded with sensible defaults: frequency = biweekly,
// next payday = today, gross category = Income > Salary, each
// pre-tax / post-tax line at its configured default category, and
// primary deposit account = first active account.
//
// categoryOptions and categoryIDs must be parallel slices in the
// shape produced by buildCategoryOptions (the leading "(None)" entry
// is expected at index 0). accounts is filtered to active accounts
// for the picker; an empty accounts slice still produces a wizard,
// just with no selectable accounts.
func NewPaycheckWizard(categoryOptions []string, categoryIDs []types.ID, accounts []*account.Account) *PaycheckWizard {
	accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)

	w := &PaycheckWizard{
		visible:         true,
		width:           72,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
		accountOptions:  accountOptions,
		accountIDs:      accountIDs,
	}

	w.employer = Field{
		Label:       "Employer",
		Type:        FieldText,
		Placeholder: "Payee name",
	}

	w.frequency = Field{
		Label:         "Pay frequency",
		Type:          FieldSelect,
		Options:       buildFrequencyOptions(),
		SelectedIndex: frequencyToIndex(scheduled.FrequencyBiweekly),
	}

	w.nextPayday = Field{
		Label:    "Next payday",
		Type:     FieldDate,
		Value:    time.Now().Format("01/02/2006"),
		Width:    10,
		dateMask: dateMaskUS,
	}

	w.grossAmount = Field{
		Label:       "Gross pay",
		Type:        FieldText,
		Placeholder: "0.00",
		Width:       12,
	}
	w.grossCategory = Field{
		Label:         "Gross pay category",
		Type:          FieldSelect,
		Options:       categoryOptions,
		SelectedIndex: findCategoryOptionIndex(categoryOptions, "Income > Salary"),
	}

	w.preTax = buildPaycheckLines(preTaxLineSpecs, categoryOptions)
	w.postTax = buildPaycheckLines(postTaxLineSpecs, categoryOptions)

	w.primaryAccount = Field{
		Label:         "Primary deposit account",
		Type:          FieldSelect,
		Options:       accountOptions,
		SelectedIndex: 0,
	}

	return w
}

// buildPaycheckLines materializes the deduction rows for a section
// (pre-tax or post-tax). Categorized lines resolve their default
// category by display name against categoryOptions; transfer lines
// start at account index 0 (the user picks the destination).
func buildPaycheckLines(specs []paycheckLineSpec, categoryOptions []string) []*PaycheckLine {
	lines := make([]*PaycheckLine, 0, len(specs))
	for _, spec := range specs {
		line := &PaycheckLine{
			Label: spec.label,
			amountField: Field{
				Type:        FieldText,
				Placeholder: "0.00",
				Width:       10,
			},
			isTransfer: spec.transfer,
		}
		if !spec.transfer && spec.defaultCategory != "" {
			line.categoryIndex = findCategoryOptionIndex(categoryOptions, spec.defaultCategory)
		}
		lines = append(lines, line)
	}
	return lines
}

// findCategoryOptionIndex returns the index of displayName in
// options, or 0 ("(None)") if the option is missing. Used to seed
// default category selections without forcing the caller to thread
// IDs through specs.
func findCategoryOptionIndex(options []string, displayName string) int {
	for i, s := range options {
		if s == displayName {
			return i
		}
	}
	return 0
}

// IsVisible reports whether the wizard should currently render.
func (w *PaycheckWizard) IsVisible() bool {
	return w.visible
}

// Employer returns the employer (payee) text field for read or
// mutation by the wizard's key handler (added in a later slice).
func (w *PaycheckWizard) Employer() *Field {
	return &w.employer
}

// Frequency returns the pay-frequency select field.
func (w *PaycheckWizard) Frequency() *Field {
	return &w.frequency
}

// NextPayday returns the next-payday date field.
func (w *PaycheckWizard) NextPayday() *Field {
	return &w.nextPayday
}

// GrossAmount returns the gross-pay amount field.
func (w *PaycheckWizard) GrossAmount() *Field {
	return &w.grossAmount
}

// GrossCategory returns the gross-pay category select.
func (w *PaycheckWizard) GrossCategory() *Field {
	return &w.grossCategory
}

// PreTaxLines returns the wizard's pre-tax deduction rows in spec
// order.
func (w *PaycheckWizard) PreTaxLines() []*PaycheckLine {
	return w.preTax
}

// PostTaxLines returns the wizard's post-tax deduction rows in spec
// order.
func (w *PaycheckWizard) PostTaxLines() []*PaycheckLine {
	return w.postTax
}

// PrimaryAccount returns the primary-deposit-account select field.
func (w *PaycheckWizard) PrimaryAccount() *Field {
	return &w.primaryAccount
}

// AdditionalTransfers returns the additional-transfer rows added by
// the user. Empty until the dynamic "+ Add transfer" affordance
// lands.
func (w *PaycheckWizard) AdditionalTransfers() []*PaycheckLine {
	return w.additionalTransfers
}
