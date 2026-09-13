package tui

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
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
	loanFieldName              = 0 // Loan section
	loanFieldInstitution       = 1
	loanFieldCurrentBalance    = 2
	loanFieldAPR               = 3
	loanFieldOrigPrincipal     = 4 // prefill-only
	loanFieldOpenDate          = 5 // prefill-only (optional)
	loanFieldTermMonths        = 6 // prefill-only
	loanFieldPayment           = 7 // Payment section
	loanFieldNextPaymentDate   = 8
	loanFieldFromAccount       = 9
	loanFieldPayee             = 10
	loanFieldInterestCategory  = 11
	loanFieldPrincipalCategory = 12
	loanFieldEscrowStart       = 13

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

// loanPrincipalDefaultDisplay is the picker label for the default principal
// category (Loan > Principal). Unlike interest the principal label is optional
// (the combo keeps "(None)"), but it defaults to this row; selecting it while
// it is the synthetic entry resolves via
// category.Service.GetOrCreateLoanPrincipalCategory at save time.
var loanPrincipalDefaultDisplay = category.LoanCategoryName + " > " + category.LoanPrincipalChildName

// loanAddNewCategoryLabel is the [+ Add new category…] action-row label on the
// wizard's interest and escrow category combos. Activating it diverts into the
// shared inline create-category sub-dialog (mirrors the transaction/scheduled
// dialogs).
const loanAddNewCategoryLabel = "[+ Add new category…]"

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

	principalOptions []string   // principal-category picker labels ("(None)" at 0)
	principalIDs     []types.ID // parallel; NilID at the synthetic default row marks get-or-create

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

// buildLoanPrincipalOptions returns the principal-category combo's labels and
// IDs plus the index to default to. Unlike interest the principal label is
// optional, so the leading "(None)" row is kept (clearable). The default
// Loan > Principal entry is guaranteed present: if a real Loan:Principal
// category already exists it defaults to that row (its real ID); otherwise a
// synthetic row (NilID, distinct from "(None)" by its label) is inserted right
// after "(None)" and resolved via get-or-create at save time.
func buildLoanPrincipalOptions(catOptions []string, catIDs []types.ID) (opts []string, ids []types.ID, defaultIdx int) {
	opts = append(opts, catOptions...)
	ids = append(ids, catIDs...)
	for i, name := range opts {
		if name == loanPrincipalDefaultDisplay {
			return opts, ids, i
		}
	}
	// No real Loan:Principal category — insert a synthetic default after
	// "(None)" (index 0). Its NilID collides with "(None)"'s NilID, so callers
	// that re-resolve by ID must disambiguate the synthetic default by label.
	withDefault := make([]string, 0, len(opts)+1)
	withDefault = append(withDefault, opts[0], loanPrincipalDefaultDisplay)
	withDefault = append(withDefault, opts[1:]...)
	idsWithDefault := make([]types.ID, 0, len(ids)+1)
	idsWithDefault = append(idsWithDefault, ids[0], types.NilID)
	idsWithDefault = append(idsWithDefault, ids[1:]...)
	return withDefault, idsWithDefault, 1
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
	principalOptions, principalIDs, principalDefault := buildLoanPrincipalOptions(catOptions, catIDs)

	state := &loanWizardData{
		accounts:         accounts,
		accountIDs:       fromIDs,
		categoryIDs:      catIDs,
		interestOptions:  interestOptions,
		interestIDs:      interestIDs,
		principalOptions: principalOptions,
		principalIDs:     principalIDs,
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
	interestField := d.AddComboField("Interest Category", interestOptions, interestDefault)
	interestField.AddNewLabel = loanAddNewCategoryLabel
	principalField := d.AddComboField("Principal Category", principalOptions, principalDefault)
	principalField.AddNewLabel = loanAddNewCategoryLabel

	// Escrow pool (category + amount pairs), progressively revealed. Each
	// category picker is a combo with an inline [+ Add new category…] row.
	for k := range loanMaxEscrowLines {
		escrowField := d.AddComboField(fmt.Sprintf("Escrow %d", k+1), catOptions, 0)
		escrowField.AddNewLabel = loanAddNewCategoryLabel
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
		prefillField(fields[loanFieldName], state.loanAccount.Name)
		if state.loanAccount.Institution.Valid {
			prefillField(fields[loanFieldInstitution], state.loanAccount.Institution.String)
		}
		if state.loanAccount.InterestRate.Valid {
			prefillField(fields[loanFieldAPR], state.loanAccount.InterestRate.Money.String())
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
	principalCatID := types.NilID
	escrowIdx := 0
	for _, sp := range st.Splits {
		if sp == nil {
			continue
		}
		if sp.TransferAccountID.Valid {
			principalAmt = sp.Amount.Abs()
			if sp.CategoryID.Valid {
				principalCatID = sp.CategoryID.ID
			}
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
			prefillField(fields[loanEscrowAmtIndex(escrowIdx)], sp.Amount.Abs().String())
			escrowIdx++
		}
	}
	prefillField(fields[loanFieldPayment], principalAmt.Add(interestAmt).String())
	if !interestCatID.IsNil() {
		setSelectByID(fields[loanFieldInterestCategory], state.interestIDs, interestCatID)
	}
	// Principal category: round-trip the existing line's label, or "(None)" when
	// the principal line is uncategorized (an old-shape loan) — otherwise the
	// build-time Loan:Principal default would silently *add* a label on edit.
	if !principalCatID.IsNil() {
		setSelectByID(fields[loanFieldPrincipalCategory], state.principalIDs, principalCatID)
	} else {
		fields[loanFieldPrincipalCategory].SelectedIndex = 0
		fields[loanFieldPrincipalCategory].ComboHighlight = 0
	}
}

// setSelectByID points a combo/select field at the option whose parallel ID
// matches target. No-op when target is not found. ComboHighlight is synced to
// the selection: the interest and escrow pickers are combos, and a plain
// Tab/Enter over a combo commits its highlighted row — so a prefilled selection
// (Edit-as-loan) whose highlight still pointed at row 0 would otherwise be
// silently reset the first time the user tabbed through it.
func setSelectByID(f *dialog.Field, ids []types.ID, target types.ID) {
	for i, id := range ids {
		if id == target {
			f.SelectedIndex = i
			f.ComboHighlight = i
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

// loanSurface is the loan wizard's state: the dialog and the data its derived
// fields are recomputed from.
type loanSurface struct {
	modalSurface
	state *loanWizardData
}

// IsVisible must be declared here rather than promoted from modalSurface — see
// the note on modalSurface.
func (s *loanSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// applyData builds the wizard over the loaded accounts and categories: the
// edit form when the message carries a schedule to adopt, the new-loan form
// otherwise. Both builders return the derived-field state alongside the
// dialog, so the surface is replaced whole.
func (s *loanSurface) applyData(msg loanWizardDataMsg) {
	var d *dialog.Dialog
	var st *loanWizardData
	if msg.editSchedule != nil {
		d, st = buildEditLoanWizard(msg.accounts, msg.categories, msg.editSchedule, msg.editOwed)
	} else {
		d, st = buildNewLoanWizard(msg.accounts, msg.categories)
	}
	*s = loanSurface{modalSurface: modalSurface{dlg: d}, state: st}
}
