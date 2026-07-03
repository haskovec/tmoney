package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// The Loan Wizard (Accounts → New Loan…) sets up an amortized loan in one
// guided flow: a loan account, an optional linked asset account, and a monthly
// loan-payment schedule whose interest/principal split is recomputed from the
// live balance every time it posts (see specs/loan-wizard.md). It is built on
// the generic dialog.Dialog form widget (like the account dialog), with three
// visual sections — Loan / Payment / Asset — laid out as an ordered field list.
// Prefill-only fields (original principal, open date, term) drive the payment
// estimate but are never stored; conditional fields (interest category, asset
// name/value, later escrow rows) toggle via the dialog's Hidden mechanism.
//
// Save is one atomic, single-undo operation: the loan account, the optional
// asset account, and the schedule are created inside one undo.CompoundCommand,
// which rolls back the already-created records if a later step fails — so a
// failure never strands an orphaned loan account swinging net worth.

// loanMaxEscrowLines bounds the escrow rows the wizard offers. A real mortgage
// draft has at most a handful of escrow items (property tax, insurance, PMI,
// HOA); the pool reveals progressively (an empty row appears once the prior row
// has a category) so the form is not cluttered when escrow is unused.
const loanMaxEscrowLines = 6

// Field indices into the loan-wizard dialog's ordered field list. The escrow
// pool occupies a contiguous block of (category, amount) pairs; the trailing
// scalar fields are offset past it.
const (
	loanFieldName             = 0 // Loan section
	loanFieldInstitution      = 1
	loanFieldCurrentBalance   = 2
	loanFieldAPR              = 3
	loanFieldOrigPrincipal    = 4 // prefill-only
	loanFieldOpenDate         = 5 // prefill-only (optional)
	loanFieldTermMonths       = 6 // prefill-only
	loanFieldPayment          = 7 // Payment section
	loanFieldNextPaymentDate  = 8
	loanFieldFromAccount      = 9
	loanFieldPayee            = 10
	loanFieldInterestCategory = 11
	loanFieldEscrowStart      = 12

	loanFieldAutoPost    = loanFieldEscrowStart + 2*loanMaxEscrowLines
	loanFieldTrackAsset  = loanFieldAutoPost + 1 // Asset section
	loanFieldAssetName   = loanFieldAutoPost + 2
	loanFieldAssetValue  = loanFieldAutoPost + 3
	loanFieldFieldsCount = loanFieldAutoPost + 4
)

// loanEscrowCatIndex / loanEscrowAmtIndex map an escrow slot number (0-based)
// to its category-select and amount-input field indices.
func loanEscrowCatIndex(k int) int { return loanFieldEscrowStart + 2*k }
func loanEscrowAmtIndex(k int) int { return loanFieldEscrowStart + 2*k + 1 }

// loanDemotionWarning is shown before saving a loan-shaped schedule through the
// generic Edit Series editor (which strips the loan_section tags). Demoting is
// behavioral, not cosmetic — a demoted schedule silently books the stale
// template interest every month — so the guard confirms before proceeding.
const loanDemotionWarning = "This converts the loan schedule to a generic schedule — payments will no longer compute interest automatically. Continue?"

// loanInterestDefaultDisplay is the picker label for the default interest
// category (Loan > Interest). Selecting it resolves via
// category.Service.GetOrCreateLoanInterestCategory at save time, so the default
// is always available even on files where it was never seeded or was deleted.
var loanInterestDefaultDisplay = category.LoanCategoryName + " > " + category.LoanInterestChildName

// loanWizardMode distinguishes creating a new loan from editing an existing
// loan-shaped schedule (Edit as loan →). Only the new path is wired today.
type loanWizardMode int

const (
	loanWizardModeNew loanWizardMode = iota
	loanWizardModeEdit
)

// loanWizardData is the loan wizard's companion state — the lookups it needs to
// map picker indices back to IDs at save time, plus the mode and the running
// auto-computed payment used by the prefill logic.
type loanWizardData struct {
	mode loanWizardMode

	accounts   []*account.Account // active accounts (for currency + from-account lookup)
	accountIDs []types.ID         // parallel to the From-account picker options

	categoryIDs []types.ID // parallel to the escrow category picker options ("(None)" at 0)

	interestOptions []string   // interest-category picker labels (no "(None)")
	interestIDs     []types.ID // parallel; NilID marks the get-or-create default

	// lastComputedPayment is the most recent amortization prefill written into
	// the Payment field. While the field still equals it (or is empty) the
	// prefill keeps updating as principal/APR/term change; once the user types
	// something else the field is considered touched and prefill stops.
	lastComputedPayment string

	// Edit-mode state (Edit as loan →). existingSchedule is the schedule being
	// re-promoted; loanAccount is its principal transfer target; owed is that
	// account's live balance as of the schedule's next payment date (loaded
	// once, so the negative-amortization guard and the rebuilt month-one
	// snapshot both use the real balance). Nil / zero in new mode.
	existingSchedule *scheduled.Transaction
	loanAccount      *account.Account
	owed             types.Money
}

// buildLoanFromAccountOptions returns parallel display-name and ID slices for
// the funding-account picker: active, non-investment accounts (a loan payment
// is drafted from a bank/cash/credit account, not an investment one).
func buildLoanFromAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))
	for _, a := range accounts {
		if a == nil || !a.Active || a.Type.IsInvestmentType() {
			continue
		}
		options = append(options, a.Name)
		ids = append(ids, a.ID)
	}
	return options, ids
}

// buildLoanInterestOptions returns the interest-category picker's labels and IDs
// plus the index to default to. It drops the leading "(None)" (interest
// requires a real category) and guarantees the default Loan > Interest entry is
// present: if a real Loan:Interest category already exists it defaults to that
// row, otherwise a synthetic row (NilID) is prepended and resolved via
// get-or-create at save time.
func buildLoanInterestOptions(catOptions []string, catIDs []types.ID) (opts []string, ids []types.ID, defaultIdx int) {
	if len(catOptions) > 0 {
		opts = append(opts, catOptions[1:]...) // skip "(None)"
		ids = append(ids, catIDs[1:]...)
	}
	for i, name := range opts {
		if name == loanInterestDefaultDisplay {
			return opts, ids, i
		}
	}
	opts = append([]string{loanInterestDefaultDisplay}, opts...)
	ids = append([]types.ID{types.NilID}, ids...)
	return opts, ids, 0
}

// buildNewLoanWizard constructs the loan wizard dialog and its companion state
// for creating a new loan.
func buildNewLoanWizard(accounts []*account.Account, categories []*category.Category) (*dialog.Dialog, *loanWizardData) {
	d, state := buildLoanWizardFields("New Loan", accounts, categories)
	state.mode = loanWizardModeNew
	updateLoanWizardVisibility(d)
	d.SetVisible(true)
	return d, state
}

// buildLoanWizardFields lays out the shared loan-wizard field list and the
// companion state (option/ID lookups). Both the new and the Edit-as-loan paths
// build from this; the caller sets the mode, applies any prefill, and toggles
// visibility.
func buildLoanWizardFields(title string, accounts []*account.Account, categories []*category.Category) (*dialog.Dialog, *loanWizardData) {
	catOptions, catIDs := buildCategoryOptions(categories)
	fromOptions, fromIDs := buildLoanFromAccountOptions(accounts)
	interestOptions, interestIDs, interestDefault := buildLoanInterestOptions(catOptions, catIDs)

	state := &loanWizardData{
		accounts:        accounts,
		accountIDs:      fromIDs,
		categoryIDs:     catIDs,
		interestOptions: interestOptions,
		interestIDs:     interestIDs,
	}

	d := dialog.NewDialog(title)
	d.SetWidth(64)

	// --- Loan section ---
	f := d.AddTextField("Name", "", "e.g. Mortgage — 123 Main St", 0)
	f.Required = true
	d.AddTextField("Institution", "", "Servicer (optional)", 0)
	f = d.AddTextField("Current Balance", "", "what you owe today", 14)
	f.Required = true
	f = d.AddTextField("APR %", "", "e.g. 6.5 (0 allowed)", 8)
	f.Required = true
	d.AddTextField("Original Principal", "", "optional — prefills payment", 14)
	d.AddOptionalDateField("Open Date", "")
	d.AddTextField("Term (months)", "", "optional — prefills payment", 8)

	// --- Payment section ---
	f = d.AddTextField("Payment (P&I)", "", "principal + interest, no escrow", 14)
	f.Required = true
	d.AddDateField("Next Payment Date", "")
	d.AddSelectField("From Account", fromOptions, 0)
	d.AddTextField("Payee", "", "Servicer (optional)", 0)
	d.AddSelectField("Interest Category", interestOptions, interestDefault)

	// Escrow pool (category + amount pairs), progressively revealed.
	for k := range loanMaxEscrowLines {
		d.AddSelectField(fmt.Sprintf("Escrow %d", k+1), catOptions, 0)
		d.AddTextField(fmt.Sprintf("Escrow %d Amount", k+1), "", "monthly amount", 12)
	}

	d.AddCheckboxField("Auto-post", false)

	// --- Asset section ---
	d.AddCheckboxField("Track an asset", false)
	d.AddTextField("Asset Name", "", "e.g. 123 Main St", 0)
	d.AddTextField("Asset Value", "", "current value", 14)

	return d, state
}

// loanEditHiddenFields are the fields not editable in Edit-as-loan mode: the
// current balance (derived from the account), the prefill-only origination
// inputs, the schedule's fixed cadence/routing (next date, from account, payee
// — preserved as-is), and the asset section (there is no loan↔asset link to
// edit). Interest category and escrow rows stay visible via the normal
// visibility rules.
var loanEditHiddenFields = []int{
	loanFieldCurrentBalance,
	loanFieldOrigPrincipal,
	loanFieldOpenDate,
	loanFieldTermMonths,
	loanFieldNextPaymentDate,
	loanFieldFromAccount,
	loanFieldPayee,
	loanFieldTrackAsset,
	loanFieldAssetName,
	loanFieldAssetValue,
}

// buildEditLoanWizard constructs the loan wizard prefilled from a loan-shaped or
// loan-adoptable schedule and its loan account, for the Edit-as-loan flow. Only
// the editable fields are shown (loan-account name/institution/APR, P&I payment,
// interest category, escrow rows, auto-post); the rest are hidden and preserved.
// owed is the loan's live balance as of the schedule's next payment date, used
// at save to rebuild the month-one snapshot.
func buildEditLoanWizard(accounts []*account.Account, categories []*category.Category, st *scheduled.Transaction, owed types.Money) (*dialog.Dialog, *loanWizardData) {
	d, state := buildLoanWizardFields("Edit Loan", accounts, categories)
	state.mode = loanWizardModeEdit
	state.existingSchedule = st
	state.owed = owed
	fields := d.Fields()

	// Resolve the loan account (the principal transfer target).
	for _, sp := range st.Splits {
		if !sp.TransferAccountID.Valid {
			continue
		}
		for _, acc := range accounts {
			if acc != nil && acc.ID == sp.TransferAccountID.ID && acc.Type == account.TypeLoan {
				state.loanAccount = acc
			}
		}
	}
	if state.loanAccount != nil {
		fields[loanFieldName].Value = state.loanAccount.Name
		if state.loanAccount.Institution.Valid {
			fields[loanFieldInstitution].Value = state.loanAccount.Institution.String
		}
		if state.loanAccount.InterestRate.Valid {
			fields[loanFieldAPR].Value = state.loanAccount.InterestRate.Money.String()
		}
	}

	prefillLoanPaymentFields(fields, state, st, findLoanInterestCategoryID(categories))
	fields[loanFieldAutoPost].Checked = st.AutoPost

	for _, idx := range loanEditHiddenFields {
		fields[idx].Hidden = true
		fields[idx].Required = false
	}

	updateLoanWizardVisibility(d)
	d.SetVisible(true)
	return d, state
}

// prefillLoanPaymentFields seeds the Payment (P&I), interest-category, and
// escrow fields from an existing schedule. It handles both a strictly
// loan-shaped template (lines carry loan_section tags) and a loose,
// loan-adoptable one (untagged): the principal is the transfer line, the
// interest line is the tagged interest split or — untagged — a categorized line
// matching the default Loan:Interest category, and every other categorized line
// is escrow. P&I is reconstructed as |principal| + |interest| (which equals the
// parent-magnitude minus escrow), so a 0% loan prefills the full payment as
// principal.
func prefillLoanPaymentFields(fields []*dialog.Field, state *loanWizardData, st *scheduled.Transaction, loanInterestCatID types.ID) {
	var principalAmt, interestAmt types.Money
	interestCatID := types.NilID
	escrowIdx := 0
	for _, sp := range st.Splits {
		if sp == nil {
			continue
		}
		if sp.TransferAccountID.Valid {
			principalAmt = sp.Amount.Abs()
			continue
		}
		tagged := sp.LoanSection.Valid
		isInterest := (tagged && sp.LoanSection.String == scheduled.LoanSectionInterest) ||
			(!tagged && !loanInterestCatID.IsNil() && sp.CategoryID.Valid && sp.CategoryID.ID == loanInterestCatID)
		if isInterest && interestAmt.IsZero() {
			interestAmt = sp.Amount.Abs()
			if sp.CategoryID.Valid {
				interestCatID = sp.CategoryID.ID
			}
			continue
		}
		// Escrow line.
		if escrowIdx < loanMaxEscrowLines && sp.CategoryID.Valid {
			setSelectByID(fields[loanEscrowCatIndex(escrowIdx)], state.categoryIDs, sp.CategoryID.ID)
			fields[loanEscrowAmtIndex(escrowIdx)].Value = sp.Amount.Abs().String()
			escrowIdx++
		}
	}
	fields[loanFieldPayment].Value = principalAmt.Add(interestAmt).String()
	if !interestCatID.IsNil() {
		setSelectByID(fields[loanFieldInterestCategory], state.interestIDs, interestCatID)
	}
}

// setSelectByID points a select field at the option whose parallel ID matches
// target. No-op when target is not found.
func setSelectByID(f *dialog.Field, ids []types.ID, target types.ID) {
	for i, id := range ids {
		if id == target {
			f.SelectedIndex = i
			return
		}
	}
}

// findLoanInterestCategoryID returns the ID of the default Loan:Interest
// category if it exists, else NilID. Used to identify the interest line when
// adopting an untagged loan schedule.
func findLoanInterestCategoryID(categories []*category.Category) types.ID {
	loanParent := types.NilID
	for _, c := range categories {
		if c != nil && c.IsTopLevel() && c.Name == category.LoanCategoryName {
			loanParent = c.ID
			break
		}
	}
	if loanParent.IsNil() {
		return types.NilID
	}
	for _, c := range categories {
		if c != nil && c.IsSubcategory() && c.ParentID.ID == loanParent && c.Name == category.LoanInterestChildName {
			return c.ID
		}
	}
	return types.NilID
}

// loadLoanWizardData fetches accounts + categories off the UI loop and emits a
// loanWizardDataMsg the app-update handler uses to construct the wizard.
func (a *App) loadLoanWizardData() tea.Cmd {
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
		return loanWizardDataMsg{accounts: accounts, categories: categories}
	}
}

// loanWizardDataMsg carries the dependencies needed to construct the wizard.
// editSchedule is non-nil for the Edit-as-loan flow, in which case editOwed
// carries the loan's live balance as of the schedule's next payment date.
type loanWizardDataMsg struct {
	accounts     []*account.Account
	categories   []*category.Category
	editSchedule *scheduled.Transaction
	editOwed     types.Money
}

// loadLoanWizardEditData fetches accounts + categories and computes the loan's
// live balance for the Edit-as-loan flow, emitting a loanWizardDataMsg carrying
// the schedule so the app-update handler builds the wizard in edit mode.
func (a *App) loadLoanWizardEditData(st *scheduled.Transaction) tea.Cmd {
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
		// The loan's live balance as of the next payment date → owed magnitude.
		owed := types.ZeroMoney
		if a.accountSvc != nil {
			for _, sp := range st.Splits {
				if !sp.TransferAccountID.Valid {
					continue
				}
				bal, err := a.accountSvc.BalanceAsOf(sp.TransferAccountID.ID, st.NextDate)
				if err == nil {
					owed = bal.Neg()
				}
				break
			}
		}
		return loanWizardDataMsg{accounts: accounts, categories: categories, editSchedule: st, editOwed: owed}
	}
}

// scheduleWantsLoanEdit reports whether the Edit Series dialog should offer
// "Edit as loan →" for st (and route the alternate action to the loan wizard):
// loan-shaped, or loan-adoptable and not paycheck-shaped. Loan-shaped takes
// precedence; loan-shaped and paycheck-shaped are mutually exclusive (their
// split tags cannot coexist), so this only guards the rare untagged-adoptable
// schedule that also passes the paycheck heuristic.
func (a *App) scheduleWantsLoanEdit(st *scheduled.Transaction) bool {
	if a.scheduledTxnSvc == nil || st == nil {
		return false
	}
	if a.scheduledTxnSvc.IsLoanShaped(st) {
		return true
	}
	return a.scheduledTxnSvc.IsLoanAdoptable(st) && !looksLikePaycheck(st)
}

// maybeAddEditAsLoanButton replaces the Edit Series dialog's buttons with an
// "Edit as loan →" affordance (mirroring "Edit as paycheck →") when st wants a
// loan edit. Called after buildEditScheduledDialog, so it overrides that
// function's default Save/Cancel set.
func (a *App) maybeAddEditAsLoanButton(st *scheduled.Transaction) {
	if a.schedDialog == nil || !a.scheduleWantsLoanEdit(st) {
		return
	}
	a.schedDialog.SetButtons([]dialog.DialogButton{
		{Label: "Save", Primary: true},
		{Label: "Cancel"},
		{Label: "Edit as loan →", Action: dialog.DialogActionAlternate},
	})
}

// relaunchScheduledAlternate dispatches the Edit Series dialog's alternate
// action to the loan wizard for a loan-shaped / loan-adoptable schedule, else
// to the paycheck wizard (its original owner).
func (a *App) relaunchScheduledAlternate() (tea.Model, tea.Cmd) {
	if a.schedDialogData != nil && a.scheduleWantsLoanEdit(a.schedDialogData.scheduled) {
		return a.relaunchAsLoanWizard()
	}
	return a.relaunchAsPaycheckWizard()
}

// relaunchAsLoanWizard closes the scheduled-edit dialog and opens the loan
// wizard prefilled from the in-flight loan-shaped / loan-adoptable schedule.
func (a *App) relaunchAsLoanWizard() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return a, nil
	}
	if a.schedDialogData.mode != scheduledDialogModeEdit || a.schedDialogData.scheduled == nil {
		return a, nil
	}
	st := a.schedDialogData.scheduled
	a.closeScheduledDialog()
	return a, a.loadLoanWizardEditData(st)
}

// loanWizardSavedMsg is emitted after a successful save so the app reloads the
// affected views.
type loanWizardSavedMsg struct{}

// closeLoanWizard clears the wizard state.
func (a *App) closeLoanWizard() {
	a.loanWizard = nil
	a.loanWizardState = nil
}

// handleLoanWizardKey routes a key event through the wizard dialog and
// translates the resulting action, refreshing conditional visibility and the
// payment prefill after ordinary edits.
func (a *App) handleLoanWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.loanWizard == nil {
		return a, nil
	}
	action := a.loanWizard.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitLoanWizard()
	case dialog.DialogActionCancel:
		a.closeLoanWizard()
		return a, nil
	}
	a.refreshLoanWizardDerived()
	return a, nil
}

// refreshLoanWizardDerived recomputes conditional field visibility and the
// payment prefill. Called after every key/mouse edit.
func (a *App) refreshLoanWizardDerived() {
	if a.loanWizard == nil || a.loanWizardState == nil {
		return
	}
	updateLoanWizardVisibility(a.loanWizard)
	a.updateLoanPaymentPrefill()
}

// updateLoanWizardVisibility toggles the Hidden state of conditional fields:
// the interest category is hidden at 0% APR; the asset fields are hidden unless
// "Track an asset" is checked; escrow rows reveal progressively.
func updateLoanWizardVisibility(d *dialog.Dialog) {
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return
	}

	// Interest category: shown only when APR > 0.
	showInterest := loanAPRPositive(fields[loanFieldAPR].Value)
	fields[loanFieldInterestCategory].Hidden = !showInterest
	if !showInterest {
		fields[loanFieldInterestCategory].Error = ""
	}

	// Escrow rows: row 0 always visible; row k visible once row k-1 has a
	// category, and a row that itself has a category stays visible.
	for k := range loanMaxEscrowLines {
		catIdx := loanEscrowCatIndex(k)
		amtIdx := loanEscrowAmtIndex(k)
		visible := k == 0
		if k > 0 && fields[loanEscrowCatIndex(k-1)].SelectedIndex > 0 {
			visible = true
		}
		if fields[catIdx].SelectedIndex > 0 {
			visible = true
		}
		fields[catIdx].Hidden = !visible
		fields[amtIdx].Hidden = !visible
		if !visible {
			fields[amtIdx].Error = ""
		}
	}

	// Asset name/value: shown only when tracking an asset.
	trackAsset := fields[loanFieldTrackAsset].Checked
	fields[loanFieldAssetName].Hidden = !trackAsset
	fields[loanFieldAssetValue].Hidden = !trackAsset
	if !trackAsset {
		fields[loanFieldAssetName].Error = ""
		fields[loanFieldAssetValue].Error = ""
	}
}

// updateLoanPaymentPrefill refreshes the Payment field from the amortization
// formula while the field is untouched (empty, or still equal to the last
// auto-computed value). Once the user types a different value the field is
// considered edited and the prefill stops overwriting it.
func (a *App) updateLoanPaymentPrefill() {
	d, st := a.loanWizard, a.loanWizardState
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return
	}
	payment := fields[loanFieldPayment]
	if payment.Value != "" && payment.Value != st.lastComputedPayment {
		return // user-edited; leave it alone
	}
	prefill, ok := computeLoanPaymentPrefill(fields)
	if !ok {
		return
	}
	payment.Value = prefill
	st.lastComputedPayment = prefill
}

// computeLoanPaymentPrefill computes the standard amortization P&I payment from
// the prefill-only fields (original principal, APR, term). Returns ok=false
// when the inputs are incomplete or invalid so the caller leaves the field
// untouched.
func computeLoanPaymentPrefill(fields []*dialog.Field) (string, bool) {
	principalStr := strings.TrimSpace(fields[loanFieldOrigPrincipal].Value)
	termStr := strings.TrimSpace(fields[loanFieldTermMonths].Value)
	if principalStr == "" || termStr == "" {
		return "", false
	}
	principal, err := parseAmountInput(principalStr)
	if err != nil || !principal.IsPositive() {
		return "", false
	}
	term, err := strconv.Atoi(termStr)
	if err != nil || term <= 0 {
		return "", false
	}
	apr := types.ZeroMoney
	if aprStr := strings.TrimSpace(fields[loanFieldAPR].Value); aprStr != "" {
		apr, err = parseAmountInput(aprStr)
		if err != nil {
			return "", false
		}
	}
	pay, err := loan.Payment(principal, apr, term)
	if err != nil {
		return "", false
	}
	return pay.String(), true
}

// loanAPRPositive reports whether the APR field value parses to a positive rate.
func loanAPRPositive(value string) bool {
	s := strings.TrimSpace(value)
	if s == "" {
		return false
	}
	apr, err := parseAmountInput(s)
	if err != nil {
		return false
	}
	return apr.IsPositive()
}

// submitLoanWizard dispatches to the new-loan or Edit-as-loan save path.
func (a *App) submitLoanWizard() (tea.Model, tea.Cmd) {
	if a.loanWizardState != nil && a.loanWizardState.mode == loanWizardModeEdit {
		return a.submitEditLoanWizard()
	}
	return a.submitNewLoanWizard()
}

// submitNewLoanWizard validates the wizard's fields and, on success, persists
// the loan account, optional asset account, and monthly loan-shaped schedule as
// one atomic, single-undo operation. Validation errors leave the wizard open
// with per-field errors set.
func (a *App) submitNewLoanWizard() (tea.Model, tea.Cmd) {
	d, st := a.loanWizard, a.loanWizardState
	if d == nil || st == nil {
		return a, nil
	}
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return a, nil
	}
	d.ClearErrors()
	hasErr := false

	setErr := func(idx int, msg string) {
		fields[idx].Error = msg
		hasErr = true
	}

	name := strings.TrimSpace(fields[loanFieldName].Value)
	if name == "" {
		setErr(loanFieldName, "Loan name is required")
	}

	owed, err := parseAmountInput(fields[loanFieldCurrentBalance].Value)
	if err != nil || !owed.IsPositive() {
		setErr(loanFieldCurrentBalance, "Enter the (positive) balance you owe today")
	}

	apr, err := parseAmountInput(fields[loanFieldAPR].Value)
	if err != nil {
		setErr(loanFieldAPR, "Invalid APR")
	} else if apr.Float64() < 0 || apr.Float64() >= 100 {
		setErr(loanFieldAPR, "APR must be between 0 and 100")
	}

	pi, err := parseAmountInput(fields[loanFieldPayment].Value)
	if err != nil || !pi.IsPositive() {
		setErr(loanFieldPayment, "Enter the monthly P&I payment")
	}

	nextDate, err := parseDateInput(fields[loanFieldNextPaymentDate].Value)
	if err != nil {
		setErr(loanFieldNextPaymentDate, "Invalid date (MM/DD/YYYY)")
	}

	fromIdx := fields[loanFieldFromAccount].SelectedIndex
	if fromIdx < 0 || fromIdx >= len(st.accountIDs) {
		d.SetErrorMsg("Pick a funding account (no eligible accounts found)")
		hasErr = true
	}

	// Negative-amortization guard: the P&I payment must exceed the first
	// month's interest. Validated up front so the user sees a field error
	// rather than a failed save.
	if !hasErr {
		if _, _, _, sErr := loan.SplitPayment(owed, apr, pi); sErr != nil {
			setErr(loanFieldPayment, "Payment does not cover the first month's interest")
		}
	}

	// Escrow lines: each row with a category selected needs a positive amount.
	var escrow []scheduled.LoanEscrowLine
	for k := range loanMaxEscrowLines {
		catField := fields[loanEscrowCatIndex(k)]
		if catField.SelectedIndex <= 0 {
			continue // (None): an unused escrow row
		}
		catID := st.categoryIDs[catField.SelectedIndex]
		amtIdx := loanEscrowAmtIndex(k)
		amt, aErr := parseAmountInput(fields[amtIdx].Value)
		if aErr != nil || !amt.IsPositive() {
			setErr(amtIdx, "Enter a positive escrow amount")
			continue
		}
		escrow = append(escrow, scheduled.LoanEscrowLine{CategoryID: catID, Amount: amt})
	}

	// Asset section.
	trackAsset := fields[loanFieldTrackAsset].Checked
	assetName := strings.TrimSpace(fields[loanFieldAssetName].Value)
	var assetValue types.Money
	if trackAsset {
		if assetName == "" {
			setErr(loanFieldAssetName, "Asset name is required")
		}
		assetValue, err = parseAmountInput(fields[loanFieldAssetValue].Value)
		if err != nil {
			setErr(loanFieldAssetValue, "Invalid amount")
		}
	}

	if hasErr {
		return a, nil
	}

	// Resolve the funding account (for currency inheritance) and prefill-only
	// origination inputs.
	fromID := st.accountIDs[fromIdx]
	currency := "USD"
	for _, acc := range st.accounts {
		if acc != nil && acc.ID == fromID {
			currency = acc.Currency
			break
		}
	}

	openingDate := loanOpeningDate(fields, owed)

	institution := strings.TrimSpace(fields[loanFieldInstitution].Value)
	payeeName := strings.TrimSpace(fields[loanFieldPayee].Value)
	autoPost := fields[loanFieldAutoPost].Checked

	// Resolve the interest category selection: the default row get-or-creates
	// Loan:Interest at save time; any other row uses the selected category.
	aprPositive := apr.IsPositive()
	interestField := fields[loanFieldInterestCategory]
	interestDefault := false
	interestPickedID := types.NilID
	if aprPositive {
		idx := interestField.SelectedIndex
		if idx >= 0 && idx < len(st.interestOptions) {
			if st.interestOptions[idx] == loanInterestDefaultDisplay {
				interestDefault = true
			} else {
				interestPickedID = st.interestIDs[idx]
			}
		} else {
			interestDefault = true
		}
	}

	a.closeLoanWizard()

	return a, func() tea.Msg {
		if a.accountSvc == nil || a.scheduledTxnSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		// Payee (get-or-create outside the atomic unit, matching the paycheck
		// wizard: a shared payee must not be deleted on undo).
		var payeeID types.ID
		if payeeName != "" && a.payeeSvc != nil {
			py, _, pErr := a.payeeSvc.GetOrCreate(payeeName)
			if pErr != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", pErr)}
			}
			payeeID = py.ID
		}

		// Interest category (also get-or-create outside the atomic unit).
		interestCatID := interestPickedID
		if aprPositive && interestDefault {
			if a.categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := a.categorySvc.GetOrCreateLoanInterestCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve interest category: %w", cErr)}
			}
			interestCatID = cat.ID
		}

		// Loan account: liabilities are stored negative.
		loanAcct := account.NewAccount(name, account.TypeLoan, currency, owed.Neg(), openingDate)
		loanAcct.SetInterestRate(apr)
		if institution != "" {
			loanAcct.SetInstitution(institution)
		}

		// Month-one snapshot + schedule assembly (mirrors ComputeLoanSplits;
		// clamped final + zero-interest omission handled inside the shared
		// BuildLoanSchedule, which the CLI `loan add` also uses so both create an
		// identical loan-shaped schedule).
		schedule, _, bErr := scheduled.BuildLoanSchedule(fromID, nextDate, payeeID, autoPost, scheduled.LoanSnapshotInput{
			LoanAccountID: loanAcct.ID,
			APR:           apr,
			Owed:          owed,
			PIPayment:     pi,
			InterestCatID: interestCatID,
			Escrow:        escrow,
		})
		if bErr != nil {
			return errMsg{err: fmt.Errorf("failed to build loan schedule: %w", bErr)}
		}

		// Assemble the atomic, single-undo compound: loan account → optional
		// asset account → schedule. CompoundCommand rolls back earlier steps if
		// a later one fails.
		cmds := []undo.Command{undo.NewCreateAccountCommand(a.accountSvc, loanAcct)}
		if trackAsset {
			assetAcct := account.NewAccount(assetName, account.TypeAsset, currency, assetValue, openingDate)
			cmds = append(cmds, undo.NewCreateAccountCommand(a.accountSvc, assetAcct))
		}
		cmds = append(cmds, undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, schedule))

		compound := undo.NewCompoundCommand("Create loan", cmds...)
		if err := a.undoManager.Execute(compound); err != nil {
			return errMsg{err: fmt.Errorf("failed to create loan: %w", err)}
		}
		return loanWizardSavedMsg{}
	}
}

// submitEditLoanWizard validates the Edit-as-loan form and, on success, applies
// the loan-account edits (name / institution / APR) and rewrites the schedule's
// month-one template snapshot (rebalanced, freshly tagged — this also (re)tags a
// loan-adoptable schedule, promoting it to strictly loan-shaped) as one atomic,
// single-undo operation. owed is the loan's live balance loaded when the wizard
// opened, so the rebuilt snapshot matches what the next post would compute.
func (a *App) submitEditLoanWizard() (tea.Model, tea.Cmd) {
	d, st := a.loanWizard, a.loanWizardState
	if d == nil || st == nil || st.existingSchedule == nil || st.loanAccount == nil {
		return a, nil
	}
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return a, nil
	}
	d.ClearErrors()
	hasErr := false
	setErr := func(idx int, msg string) {
		fields[idx].Error = msg
		hasErr = true
	}

	name := strings.TrimSpace(fields[loanFieldName].Value)
	if name == "" {
		setErr(loanFieldName, "Loan name is required")
	}

	apr, err := parseAmountInput(fields[loanFieldAPR].Value)
	if err != nil {
		setErr(loanFieldAPR, "Invalid APR")
	} else if apr.Float64() < 0 || apr.Float64() >= 100 {
		setErr(loanFieldAPR, "APR must be between 0 and 100")
	}

	pi, err := parseAmountInput(fields[loanFieldPayment].Value)
	if err != nil || !pi.IsPositive() {
		setErr(loanFieldPayment, "Enter the monthly P&I payment")
	}

	owed := st.owed
	if !owed.IsPositive() {
		d.SetErrorMsg("This loan appears to be paid off — nothing to edit.")
		hasErr = true
	}

	if !hasErr {
		if _, _, _, sErr := loan.SplitPayment(owed, apr, pi); sErr != nil {
			setErr(loanFieldPayment, "Payment does not cover the first month's interest")
		}
	}

	var escrow []scheduled.LoanEscrowLine
	for k := range loanMaxEscrowLines {
		catField := fields[loanEscrowCatIndex(k)]
		if catField.SelectedIndex <= 0 {
			continue
		}
		catID := st.categoryIDs[catField.SelectedIndex]
		amtIdx := loanEscrowAmtIndex(k)
		amt, aErr := parseAmountInput(fields[amtIdx].Value)
		if aErr != nil || !amt.IsPositive() {
			setErr(amtIdx, "Enter a positive escrow amount")
			continue
		}
		escrow = append(escrow, scheduled.LoanEscrowLine{CategoryID: catID, Amount: amt})
	}

	if hasErr {
		return a, nil
	}

	institution := strings.TrimSpace(fields[loanFieldInstitution].Value)
	autoPost := fields[loanFieldAutoPost].Checked

	aprPositive := apr.IsPositive()
	interestField := fields[loanFieldInterestCategory]
	interestDefault := false
	interestPickedID := types.NilID
	if aprPositive {
		idx := interestField.SelectedIndex
		if idx >= 0 && idx < len(st.interestOptions) {
			if st.interestOptions[idx] == loanInterestDefaultDisplay {
				interestDefault = true
			} else {
				interestPickedID = st.interestIDs[idx]
			}
		} else {
			interestDefault = true
		}
	}

	schedule := st.existingSchedule
	loanAcct := st.loanAccount

	a.closeLoanWizard()

	return a, func() tea.Msg {
		if a.accountSvc == nil || a.scheduledTxnSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		interestCatID := interestPickedID
		if aprPositive && interestDefault {
			if a.categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := a.categorySvc.GetOrCreateLoanInterestCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve interest category: %w", cErr)}
			}
			interestCatID = cat.ID
		}

		parent, splits, _, bErr := scheduled.BuildLoanSnapshot(scheduled.LoanSnapshotInput{
			LoanAccountID: loanAcct.ID,
			APR:           apr,
			Owed:          owed,
			PIPayment:     pi,
			InterestCatID: interestCatID,
			Escrow:        escrow,
		})
		if bErr != nil {
			return errMsg{err: fmt.Errorf("failed to rebuild loan schedule: %w", bErr)}
		}

		// Apply loan-account edits.
		loanAcct.Name = name
		loanAcct.SetInterestRate(apr)
		loanAcct.SetInstitution(institution)

		// Rewrite the template snapshot with fresh tags; preserve cadence,
		// routing (account/next date), payee, memo, and duration.
		schedule.SetAmount(parent)
		schedule.ClearCategory()
		schedule.SetAutoPost(autoPost)
		schedule.Splits = scheduled.SplitCollection(splits)

		// One atomic, single-undo operation: account edit + template rewrite.
		compound := undo.NewCompoundCommand("Edit loan",
			undo.NewEditAccountCommand(a.accountSvc, loanAcct),
			undo.NewEditScheduledTransactionCommand(a.scheduledTxnSvc, schedule),
		)
		if err := a.undoManager.Execute(compound); err != nil {
			return errMsg{err: fmt.Errorf("failed to save loan edit: %w", err)}
		}
		return loanWizardSavedMsg{}
	}
}

// loanOpeningDate applies the spec's opening-date rule: use the provided open
// date only for a new loan at origination — the open date is set and the
// current balance equals the original principal — otherwise today (a mid-life
// balance is a today snapshot with no history behind it).
func loanOpeningDate(fields []*dialog.Field, owed types.Money) types.Date {
	openField := fields[loanFieldOpenDate]
	if dialog.IsBlankDateInput(openField.Value) {
		return types.Today()
	}
	openDate, err := parseDateInput(openField.Value)
	if err != nil {
		return types.Today()
	}
	origStr := strings.TrimSpace(fields[loanFieldOrigPrincipal].Value)
	if origStr == "" {
		return types.Today()
	}
	orig, err := parseAmountInput(origStr)
	if err != nil || !orig.Equal(owed) {
		return types.Today()
	}
	return openDate
}
