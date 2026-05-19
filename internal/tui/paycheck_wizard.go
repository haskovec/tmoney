package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
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

// AddAdditionalTransfer appends a transfer-line to the "Additional
// transfers" section under Net Pay Destinations. label is rendered to
// the left of the amount field (e.g. "Savings"); accountIndex selects
// the destination account in the wizard's account picker (parallel to
// PrimaryAccount().Options). Returns the new line so callers (tests
// and, eventually, the [+ Add transfer] key handler) can configure it.
func (w *PaycheckWizard) AddAdditionalTransfer(label string, accountIndex int) *PaycheckLine {
	line := &PaycheckLine{
		Label: label,
		amountField: Field{
			Type:        FieldText,
			Placeholder: "0.00",
			Width:       10,
		},
		isTransfer:   true,
		accountIndex: accountIndex,
	}
	w.additionalTransfers = append(w.additionalTransfers, line)
	return line
}

// BuildSplits assembles the wizard's form state into a list of
// scheduled-split rows and computes the parent's net amount — the
// "primary deposit account" remainder per the spec:
//
//	remainder = gross − sum(pre-tax + post-tax deductions) − sum(additional transfers)
//
// The remainder is derived from the signed sum of the assembled splits,
// so the resulting schedule satisfies the data-model invariant
// parent.amount == signed_sum(line.amounts) (see
// specs/multiline-splits-and-paycheck.md). The gross row is stored as a
// positive signed amount; every deduction and additional-transfer line is
// stored as a negative signed amount (the user types positive magnitudes;
// the wizard flips the sign). Empty amount fields are skipped — they
// don't produce zero-amount split rows.
//
// The returned splits carry no ScheduledTransactionID; the caller (MS-028)
// will stamp it on after the parent schedule is created.
func (w *PaycheckWizard) BuildSplits() (types.Money, []*scheduled.Split, error) {
	if w == nil {
		return types.ZeroMoney, nil, fmt.Errorf("nil wizard")
	}

	splits := make([]*scheduled.Split, 0)

	// Gross row — positive signed amount, categorized.
	grossStr := strings.TrimSpace(w.grossAmount.Value)
	if grossStr == "" {
		return types.ZeroMoney, nil, fmt.Errorf("gross pay is required")
	}
	gross, err := parseAmountInput(grossStr)
	if err != nil {
		return types.ZeroMoney, nil, fmt.Errorf("gross pay: %w", err)
	}
	if !gross.IsPositive() {
		return types.ZeroMoney, nil, fmt.Errorf("gross pay must be positive")
	}
	grossCatID := w.lookupCategoryID(w.grossCategory.SelectedIndex)
	if grossCatID.IsNil() {
		return types.ZeroMoney, nil, fmt.Errorf("gross pay needs a category")
	}
	splits = append(splits, &scheduled.Split{
		BaseModel:  types.NewBaseModel(),
		Amount:     gross,
		CategoryID: types.NullableID{ID: grossCatID, Valid: true},
	})

	// Deduction lines (pre-tax then post-tax). Each entered amount is
	// stored as a negative signed split.
	deductionSections := [][]*PaycheckLine{w.preTax, w.postTax}
	for _, section := range deductionSections {
		for _, line := range section {
			sp, err := w.buildDeductionSplit(line)
			if err != nil {
				return types.ZeroMoney, nil, err
			}
			if sp != nil {
				splits = append(splits, sp)
			}
		}
	}

	// Additional transfers (Net Pay Destinations → Additional transfers).
	// Always transfer-lines; same negative-signed convention.
	for _, line := range w.additionalTransfers {
		sp, err := w.buildDeductionSplit(line)
		if err != nil {
			return types.ZeroMoney, nil, err
		}
		if sp != nil {
			splits = append(splits, sp)
		}
	}

	parent := types.ZeroMoney
	for _, s := range splits {
		parent = parent.Add(s.Amount)
	}
	return parent, splits, nil
}

// buildDeductionSplit produces one signed-negative split row from a
// deduction/transfer line, or returns (nil, nil) when the line's amount
// field is empty or parses to zero. The line is rendered as a
// transfer-line when line.isTransfer is set (uses accountIndex) and as
// a categorized line otherwise (uses categoryIndex). The user's typed
// magnitude is always stored as a negative signed amount — matching the
// spec example where every deduction/transfer reduces the net deposit.
func (w *PaycheckWizard) buildDeductionSplit(line *PaycheckLine) (*scheduled.Split, error) {
	amtStr := strings.TrimSpace(line.amountField.Value)
	if amtStr == "" {
		return nil, nil
	}
	amt, err := parseAmountInput(amtStr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", line.Label, err)
	}
	if amt.IsZero() {
		return nil, nil
	}
	amt = amt.Abs().Neg()

	sp := &scheduled.Split{
		BaseModel: types.NewBaseModel(),
		Amount:    amt,
	}
	if line.isTransfer {
		accountID := w.lookupAccountID(line.accountIndex)
		if accountID.IsNil() {
			return nil, fmt.Errorf("%s: pick a target account", line.Label)
		}
		sp.TransferAccountID = types.NullableID{ID: accountID, Valid: true}
		return sp, nil
	}
	catID := w.lookupCategoryID(line.categoryIndex)
	if catID.IsNil() {
		return nil, fmt.Errorf("%s: pick a category", line.Label)
	}
	sp.CategoryID = types.NullableID{ID: catID, Valid: true}
	return sp, nil
}

// lookupCategoryID maps a select index to the corresponding category ID,
// returning NilID when the index is out of range or refers to the
// leading "(None)" placeholder.
func (w *PaycheckWizard) lookupCategoryID(idx int) types.ID {
	if idx < 0 || idx >= len(w.categoryIDs) {
		return types.NilID
	}
	return w.categoryIDs[idx]
}

// lookupAccountID maps a select index to the corresponding account ID,
// returning NilID when the index is out of range.
func (w *PaycheckWizard) lookupAccountID(idx int) types.ID {
	if idx < 0 || idx >= len(w.accountIDs) {
		return types.NilID
	}
	return w.accountIDs[idx]
}

// paycheckWizardDataMsg carries the dependencies needed to construct a
// PaycheckWizard (active accounts + category options). Dispatched
// asynchronously by loadPaycheckWizardData so the lookup doesn't block
// the menu-handler return path.
type paycheckWizardDataMsg struct {
	accounts        []*account.Account
	categoryOptions []string
	categoryIDs     []types.ID
}

// loadPaycheckWizardData fetches the active accounts and category list
// the wizard needs and emits a paycheckWizardDataMsg. The message
// handler (in app_update.go) constructs the wizard from the loaded
// data. Per MS-026 this is the open-only path — save logic lands in
// MS-027/MS-028.
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

// submitPaycheckWizard validates the wizard's form, assembles a multi-line
// scheduled.Transaction from it, and dispatches the create command through
// the scheduled service. The wizard is pure UI sugar — the saved record is
// a standard multi-line scheduled transaction with no paycheck-specific
// fields, indistinguishable from one created via the generic scheduled
// dialog with the Split toggle.
//
// On success the wizard is cleared and a scheduledDialogSavedMsg is emitted
// so the scheduled view reloads. Validation errors leave the wizard open
// with the error rendered on the offending field.
func (a *App) submitPaycheckWizard() (tea.Model, tea.Cmd) {
	w := a.paycheckWizard
	if w == nil {
		return a, nil
	}

	// Account is required.
	accountID := w.lookupAccountID(w.primaryAccount.SelectedIndex)
	if accountID.IsNil() {
		w.primaryAccount.Error = "Please select a deposit account"
		return a, nil
	}

	// Parse start date (next payday).
	startDate, err := parseDateInput(w.nextPayday.Value)
	if err != nil {
		w.nextPayday.Error = "Invalid date (MM/DD/YYYY)"
		return a, nil
	}

	frequency := frequencyFromIndex(w.frequency.SelectedIndex)

	// Build splits + parent net from the form. BuildSplits handles
	// gross / deduction / transfer line validation and the
	// remainder-equals-signed-sum invariant.
	parentAmount, splits, err := w.BuildSplits()
	if err != nil {
		w.grossAmount.Error = err.Error()
		return a, nil
	}

	employer := strings.TrimSpace(w.employer.Value)

	// Close the wizard before the async save for responsive UI.
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

		st := scheduled.NewTransaction(accountID, frequency, startDate)
		st.SetAmount(parentAmount)
		if !payeeID.IsNil() {
			st.SetPayee(payeeID)
		}
		// Multi-line schedules have no scalar category.
		st.ClearCategory()
		st.Splits = scheduled.SplitCollection(splits)

		cmd := undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, st)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
		}
		return scheduledDialogSavedMsg{}
	}
}
