package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
// the dynamic "[+ Add line]" affordances are deferred too. The wizard
// is rendered as a single tall *Dialog with all fields stacked
// vertically; the spec mockup's side-by-side row layout is
// aspirational and will follow in a polish pass.
type PaycheckWizard struct {
	// form is the underlying Dialog that owns every input field. It
	// also drives focus / Tab / Enter / Esc routing via Dialog.HandleKey.
	form *Dialog

	// Pointers into form.fields for the structural accessors. These
	// stay valid for the lifetime of the wizard because the Dialog
	// never re-slices its fields once built.
	employerField       *Field
	frequencyField      *Field
	nextPaydayField     *Field
	grossAmountField    *Field
	grossCategoryField  *Field
	primaryAccountField *Field

	// Deduction rows.
	preTax  []*PaycheckLine
	postTax []*PaycheckLine

	// Net pay destinations.
	additionalTransfers []*PaycheckLine

	// Lookups used to map selected indices to IDs at save time
	// (MS-027/MS-028) and to populate dynamically-added transfer-line
	// pickers.
	categoryOptions []string
	categoryIDs     []types.ID
	accountOptions  []string
	accountIDs      []types.ID
}

// PaycheckLine is one row in the deductions or additional-transfers
// section of the wizard. A line is either categorized (a regular
// expense/income split) or a transfer to another account, mirroring
// the two split-item shapes in the underlying data model. Both the
// amount and the category-or-account selector are real Field pointers
// into the parent wizard's *Dialog, so the Dialog's standard focus
// model edits them.
type PaycheckLine struct {
	// Label is the fixed label rendered to the left of the line's
	// fields (e.g. "Federal income tax", "401(k) contribution").
	Label string

	// amountField holds the entered amount for this line.
	amountField *Field

	// selectField is the line's category select (categorized lines)
	// or destination-account select (transfer lines). Which list it
	// holds is fixed at line construction time by isTransfer.
	selectField *Field

	// isTransfer indicates the line targets a destination account
	// (transfer-line) instead of a category.
	isTransfer bool
}

// AmountField returns a pointer to the line's amount input so the
// wizard's key handler can mutate it.
func (l *PaycheckLine) AmountField() *Field {
	return l.amountField
}

// SelectField returns the line's category-or-account select field.
// Callers should consult IsTransfer() to know which interpretation
// applies to SelectedIndex.
func (l *PaycheckLine) SelectField() *Field {
	return l.selectField
}

// IsTransfer reports whether this line targets a destination
// account instead of a category.
func (l *PaycheckLine) IsTransfer() bool {
	return l.isTransfer
}

// CategoryIndex returns the selected option index for a categorized
// line. Meaningless for transfer-lines (always 0).
func (l *PaycheckLine) CategoryIndex() int {
	if l.isTransfer || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex
}

// AccountIndex returns the selected option index for a transfer-line.
// Meaningless for categorized lines (always 0).
func (l *PaycheckLine) AccountIndex() int {
	if !l.isTransfer || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex
}

// SetCategoryIndex updates the selected option index on a categorized
// line. No-op on transfer-lines.
func (l *PaycheckLine) SetCategoryIndex(idx int) {
	if l.isTransfer || l.selectField == nil {
		return
	}
	l.selectField.SelectedIndex = idx
}

// SetAccountIndex updates the selected option index on a transfer-
// line. No-op on categorized lines.
func (l *PaycheckLine) SetAccountIndex(idx int) {
	if !l.isTransfer || l.selectField == nil {
		return
	}
	l.selectField.SelectedIndex = idx
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
//
// Internally the wizard owns a single *Dialog that holds every input
// field in display order. Tab / Shift+Tab / Enter / Esc routing comes
// for free from Dialog.HandleKey, and the wizard's Render method is a
// thin wrapper around Dialog.Render.
func NewPaycheckWizard(categoryOptions []string, categoryIDs []types.ID, accounts []*account.Account) *PaycheckWizard {
	accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)

	w := &PaycheckWizard{
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
		accountOptions:  accountOptions,
		accountIDs:      accountIDs,
	}

	d := NewDialog("Paycheck Schedule")
	d.SetWidth(72)

	w.employerField = d.AddTextField("Employer (payee)", "", "Payee name", 0)
	w.frequencyField = d.AddSelectField("Pay frequency", buildFrequencyOptions(), frequencyToIndex(scheduled.FrequencyBiweekly))
	w.nextPaydayField = d.AddDateField("Next payday", time.Now().Format("01/02/2006"))
	w.grossAmountField = d.AddTextField("Gross pay", "", "0.00", 12)
	w.grossCategoryField = d.AddSelectField("Gross pay category", categoryOptions, findCategoryOptionIndex(categoryOptions, "Income > Salary"))

	w.preTax = appendPaycheckLines(d, preTaxLineSpecs, "Pre-tax", categoryOptions, accountOptions)
	w.postTax = appendPaycheckLines(d, postTaxLineSpecs, "Post-tax", categoryOptions, accountOptions)

	w.primaryAccountField = d.AddSelectField("Primary deposit account", accountOptions, 0)

	d.SetVisible(true)
	w.form = d
	return w
}

// appendPaycheckLines materializes the deduction rows for a section
// (pre-tax or post-tax) directly onto the wizard's *Dialog. Each
// line contributes two consecutive fields: amount (text) and the
// category-or-account select. Categorized lines resolve their default
// category by display name against categoryOptions; transfer lines
// start at account index 0 (the user picks the destination).
func appendPaycheckLines(d *Dialog, specs []paycheckLineSpec, sectionLabel string, categoryOptions, accountOptions []string) []*PaycheckLine {
	lines := make([]*PaycheckLine, 0, len(specs))
	for _, spec := range specs {
		amount := d.AddTextField(sectionLabel+": "+spec.label, "", "0.00", 10)

		var sel *Field
		if spec.transfer {
			sel = d.AddSelectField(sectionLabel+": "+spec.label+" → account", accountOptions, 0)
		} else {
			defaultIdx := 0
			if spec.defaultCategory != "" {
				defaultIdx = findCategoryOptionIndex(categoryOptions, spec.defaultCategory)
			}
			sel = d.AddSelectField(sectionLabel+": "+spec.label+" category", categoryOptions, defaultIdx)
		}

		lines = append(lines, &PaycheckLine{
			Label:       spec.label,
			amountField: amount,
			selectField: sel,
			isTransfer:  spec.transfer,
		})
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
	return w != nil && w.form != nil && w.form.IsVisible()
}

// Dialog exposes the underlying form Dialog so callers (e.g. mouse
// handlers, the render path) can reach it directly. Most consumers
// should use the structural accessors below instead.
func (w *PaycheckWizard) Dialog() *Dialog {
	return w.form
}

// Render renders the wizard as an overlay-ready string. Thin wrapper
// around the underlying Dialog.Render so app_view.go can stack it
// like every other modal.
func (w *PaycheckWizard) Render(styles Styles) string {
	if w == nil || w.form == nil {
		return ""
	}
	return w.form.Render(styles)
}

// Employer returns the employer (payee) text field.
func (w *PaycheckWizard) Employer() *Field {
	return w.employerField
}

// Frequency returns the pay-frequency select field.
func (w *PaycheckWizard) Frequency() *Field {
	return w.frequencyField
}

// NextPayday returns the next-payday date field.
func (w *PaycheckWizard) NextPayday() *Field {
	return w.nextPaydayField
}

// GrossAmount returns the gross-pay amount field.
func (w *PaycheckWizard) GrossAmount() *Field {
	return w.grossAmountField
}

// GrossCategory returns the gross-pay category select.
func (w *PaycheckWizard) GrossCategory() *Field {
	return w.grossCategoryField
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
	return w.primaryAccountField
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
//
// The new line's amount and account selector are appended onto the
// underlying form Dialog, so they participate in Tab/Shift+Tab focus
// cycling just like every other field.
func (w *PaycheckWizard) AddAdditionalTransfer(label string, accountIndex int) *PaycheckLine {
	amount := w.form.AddTextField("Additional transfer: "+label, "", "0.00", 10)
	sel := w.form.AddSelectField("Additional transfer: "+label+" → account", w.accountOptions, accountIndex)
	line := &PaycheckLine{
		Label:       label,
		amountField: amount,
		selectField: sel,
		isTransfer:  true,
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
	grossStr := strings.TrimSpace(w.grossAmountField.Value)
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
	grossCatID := w.lookupCategoryID(w.grossCategoryField.SelectedIndex)
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
		accountID := w.lookupAccountID(line.AccountIndex())
		if accountID.IsNil() {
			return nil, fmt.Errorf("%s: pick a target account", line.Label)
		}
		sp.TransferAccountID = types.NullableID{ID: accountID, Valid: true}
		return sp, nil
	}
	catID := w.lookupCategoryID(line.CategoryIndex())
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

// handlePaycheckWizardKey routes a key event into the wizard's
// underlying *Dialog and translates the result. Submit (Enter on the
// Save button) dispatches into submitPaycheckWizard; Cancel (Esc or
// Enter on a non-primary button) closes the wizard without saving.
func (a *App) handlePaycheckWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.paycheckWizard == nil || a.paycheckWizard.form == nil {
		return a, nil
	}
	action := a.paycheckWizard.form.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitPaycheckWizard()
	case DialogActionCancel:
		a.closePaycheckWizard()
		return a, nil
	}
	return a, nil
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
	accountID := w.lookupAccountID(w.primaryAccountField.SelectedIndex)
	if accountID.IsNil() {
		w.primaryAccountField.Error = "Please select a deposit account"
		return a, nil
	}

	// Parse start date (next payday).
	startDate, err := parseDateInput(w.nextPaydayField.Value)
	if err != nil {
		w.nextPaydayField.Error = "Invalid date (MM/DD/YYYY)"
		return a, nil
	}

	frequency := frequencyFromIndex(w.frequencyField.SelectedIndex)

	// Build splits + parent net from the form. BuildSplits handles
	// gross / deduction / transfer line validation and the
	// remainder-equals-signed-sum invariant.
	parentAmount, splits, err := w.BuildSplits()
	if err != nil {
		w.grossAmountField.Error = err.Error()
		return a, nil
	}

	employer := strings.TrimSpace(w.employerField.Value)

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

// looksLikePaycheck reports whether a scheduled transaction matches
// the paycheck heuristic: it is multi-line, has at least one
// categorized positive-amount split (the gross income line), and has
// at least one categorized negative-amount split whose category's
// display name starts with "Tax > " — the prefix
// buildCategoryOptions emits for any subcategory under a top-level
// "Tax" parent (see specs/multiline-splits-and-paycheck.md, "Round-
// trip edits"). categoryOptions / categoryIDs are the parallel slices
// produced by buildCategoryOptions; a nil/empty pair causes the
// heuristic to return false (no way to inspect category names).
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
// from an existing multi-line scheduled transaction. The wizard is
// constructed via NewPaycheckWizard so the static layout (and its
// default category seeds) is identical, then each scalar field and
// matching deduction/transfer row is populated from the schedule's
// state. This is the round-trip path described in
// specs/multiline-splits-and-paycheck.md ("Round-trip edits"):
//
//   - Employer is resolved against st.PayeeID using the payees slice.
//   - Frequency / next payday come from the template's Frequency and
//     NextDate.
//   - PrimaryAccount points at st.AccountID (the schedule's main
//     account); falls back to index 0 when the account isn't in the
//     active list.
//   - The first positive-amount categorized split is treated as the
//     gross row; its amount and category select are populated.
//   - Remaining negative-amount categorized splits are matched to the
//     wizard's pre-tax / post-tax slots by category display name (the
//     same defaults each row was seeded with). Unmatched categorized
//     lines are dropped per the spec's best-effort note.
//   - Every transfer-line split is appended to AdditionalTransfers
//     with the destination account's name as the label. No attempt is
//     made to slot a 401(k)/HSA transfer into the static pre-tax /
//     post-tax row — the user can rearrange after relaunch.
//
// Magnitudes are stored as positive strings (the user's typed shape);
// the wizard re-applies the negative sign at save time.
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

	w.frequencyField.SelectedIndex = frequencyToIndex(st.Frequency)
	w.nextPaydayField.Value = st.NextDate.Time().Format("01/02/2006")

	for i, opt := range w.primaryAccountField.Options {
		if i >= len(w.accountIDs) {
			break
		}
		if w.accountIDs[i] == st.AccountID {
			w.primaryAccountField.SelectedIndex = i
			break
		}
		_ = opt
	}

	categoryNameByID := make(map[types.ID]string, len(categoryIDs))
	for i, id := range categoryIDs {
		if i < len(categoryOptions) {
			categoryNameByID[id] = categoryOptions[i]
		}
	}

	preTaxByCategory := make(map[string]*PaycheckLine, len(w.preTax))
	for i, spec := range preTaxLineSpecs {
		if spec.defaultCategory != "" {
			preTaxByCategory[spec.defaultCategory] = w.preTax[i]
		}
	}
	postTaxByCategory := make(map[string]*PaycheckLine, len(w.postTax))
	for i, spec := range postTaxLineSpecs {
		if spec.defaultCategory != "" {
			postTaxByCategory[spec.defaultCategory] = w.postTax[i]
		}
	}

	grossAssigned := false
	for _, sp := range st.Splits {
		if sp == nil {
			continue
		}

		if sp.TransferAccountID.Valid {
			label := "Transfer"
			acctIdx := 0
			for i, id := range w.accountIDs {
				if id == sp.TransferAccountID.ID {
					acctIdx = i
					if i < len(w.primaryAccountField.Options) {
						label = w.primaryAccountField.Options[i]
					}
					break
				}
			}
			line := w.AddAdditionalTransfer(label, acctIdx)
			line.amountField.Value = sp.Amount.Abs().String()
			continue
		}

		if !sp.CategoryID.Valid {
			continue
		}
		name := categoryNameByID[sp.CategoryID.ID]
		if !grossAssigned && sp.Amount.IsPositive() {
			w.grossAmountField.Value = sp.Amount.String()
			if idx := findCategoryOptionIndex(categoryOptions, name); idx > 0 {
				w.grossCategoryField.SelectedIndex = idx
			}
			grossAssigned = true
			continue
		}

		magnitude := sp.Amount.Abs().String()
		if line, ok := preTaxByCategory[name]; ok {
			line.amountField.Value = magnitude
			continue
		}
		if line, ok := postTaxByCategory[name]; ok {
			line.amountField.Value = magnitude
			continue
		}
		// Unmatched categorized line: best-effort drops it (the spec
		// notes the affordance is hidden when the wizard cannot
		// represent the schedule; here we've already shown the button
		// based on the looser heuristic, so silently skip).
	}

	return w
}

// relaunchAsPaycheckWizard closes the scheduled-edit dialog and opens
// the paycheck wizard pre-filled from the schedule currently being
// edited. Called when the user activates the "Edit as paycheck →"
// affordance on a paycheck-shaped scheduled-transaction edit dialog
// (see MS-029 / looksLikePaycheck).
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
