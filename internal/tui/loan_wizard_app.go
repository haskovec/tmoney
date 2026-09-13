package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// App integration: the async data loads for new and edit, the Edit-as-loan
// affordance on the scheduled dialog, the key/action dispatchers the modal
// registry calls, and the create-category divert's opener and applier.

// loadLoanWizardData fetches accounts + categories off the UI loop and emits a
// loanWizardDataMsg the app-update handler uses to construct the wizard.
func (a *App) loadLoanWizardData() tea.Cmd {
	return func() tea.Msg {
		var accounts []*account.Account
		if a.services.Account != nil {
			acs, err := a.services.Account.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}
		var categories []*category.Category
		if a.services.Category != nil {
			cs, err := a.services.Category.List()
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
		if a.services.Account != nil {
			acs, err := a.services.Account.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}
		var categories []*category.Category
		if a.services.Category != nil {
			cs, err := a.services.Category.List()
			if err != nil {
				return errMsg{err: err}
			}
			categories = cs
		}
		// The loan's live balance as of the next payment date → owed magnitude.
		owed := types.ZeroMoney
		if a.services.Account != nil {
			for _, sp := range st.Splits {
				if !sp.TransferAccountID.Valid {
					continue
				}
				bal, err := a.services.Account.BalanceAsOf(sp.TransferAccountID.ID, st.NextDate)
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
	if a.services.Scheduled == nil || st == nil {
		return false
	}
	if a.services.Scheduled.IsLoanShaped(st) {
		return true
	}
	return a.services.Scheduled.IsLoanAdoptable(st) && !looksLikePaycheck(st)
}

// maybeAddEditAsLoanButton replaces the Edit Series dialog's buttons with an
// "Edit as loan →" affordance (mirroring "Edit as paycheck →") when st wants a
// loan edit. Called after buildEditScheduledDialog, so it overrides that
// function's default Save/Cancel set.
func (a *App) maybeAddEditAsLoanButton(st *scheduled.Transaction) {
	if a.sched.dlg == nil || !a.scheduleWantsLoanEdit(st) {
		return
	}
	a.sched.dlg.SetButtons([]dialog.DialogButton{
		{Label: "Save", Primary: true},
		{Label: "Cancel"},
		{Label: "Edit as loan →", Action: dialog.DialogActionAlternate},
	})
}

// relaunchScheduledAlternate dispatches the Edit Series dialog's alternate
// action to the loan wizard for a loan-shaped / loan-adoptable schedule, else
// to the paycheck wizard (its original owner).
func (a *App) relaunchScheduledAlternate() (tea.Model, tea.Cmd) {
	if a.sched.data != nil && a.scheduleWantsLoanEdit(a.sched.data.scheduled) {
		return a.relaunchAsLoanWizard()
	}
	return a.relaunchAsPaycheckWizard()
}

// relaunchAsLoanWizard closes the scheduled-edit dialog and opens the loan
// wizard prefilled from the in-flight loan-shaped / loan-adoptable schedule.
func (a *App) relaunchAsLoanWizard() (tea.Model, tea.Cmd) {
	if a.sched.dlg == nil || a.sched.data == nil {
		return a, nil
	}
	if a.sched.data.mode != scheduledDialogModeEdit || a.sched.data.scheduled == nil {
		return a, nil
	}
	st := a.sched.data.scheduled
	a.closeScheduledDialog()
	return a, a.loadLoanWizardEditData(st)
}

// loanWizardSavedMsg is emitted after a successful save so the app reloads the
// affected views.
type loanWizardSavedMsg struct{}

// closeLoanWizard clears the wizard state.
func (a *App) closeLoanWizard() {
	a.loan = loanSurface{}
}

// handleLoanWizardKey routes a key event through the wizard dialog and
// translates the resulting action, refreshing conditional visibility and the
// payment prefill after ordinary edits.
func (a *App) handleLoanWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.loan.dlg == nil {
		return a, nil
	}
	return a.loanWizardAction(a.loan.dlg.HandleKey(msg))
}

// loanWizardAction dispatches a DialogAction for the loan wizard, from either input path.
func (a *App) loanWizardAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitLoanWizard()
	case dialog.DialogActionCancel:
		a.closeLoanWizard()
		return a, nil
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromLoan()
	}
	a.refreshLoanWizardDerived()
	return a, nil
}

// openCreateCategorySubDialogFromLoan hides the loan wizard and opens the shared
// inline create-category sub-dialog, seeded from the typed query on whichever
// category combo (interest or an escrow row) activated [+ Add new category…].
// The focused field is the trigger; its index is recorded so the applier can
// point it at the new category. The wizard's field state is preserved by
// keeping the dialog alive (just hidden); cancelCreateCatDialog and
// applyCreatedCategoryToLoan restore it.
func (a *App) openCreateCategorySubDialogFromLoan() (tea.Model, tea.Cmd) {
	if a.loan.dlg == nil {
		return a, nil
	}
	fields := a.loan.dlg.Fields()
	fieldIdx := a.loan.dlg.FocusIndex()
	if fieldIdx < 0 || fieldIdx >= len(fields) {
		return a, nil
	}
	catField := fields[fieldIdx]
	query := catField.Query
	// Consume the trigger and clear the typed query — the sub-dialog owns it now.
	catField.AddNewTriggered = false
	catField.Query = ""

	a.createCat.origin.loanField = fieldIdx
	// Set the source before parentsForCreateCatDialog so it resolves the right
	// parents (falls back to a live category list for the loan wizard).
	a.createCat.origin.surface = createCatSourceLoanWizard
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	// Loan interest and escrow lines are always expenses.
	a.createCat.dlg = buildCreateCategoryDialog(name, parent, parents, category.TypeExpense)
	a.loan.dlg.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToLoan is the per-surface applier for the loan wizard. It
// rebuilds every category combo's options to include newCat, then points the
// originating field (a.createCatLoanField) at it. Because inserting a category
// into the sorted list shifts option indices, each *other* combo's existing
// selection is re-resolved by ID rather than by its stale index — otherwise a
// filled escrow row would silently jump to a different category.
func (a *App) applyCreatedCategoryToLoan(newCat *category.Category, cats []*category.Category) {
	// The wizard may have been closed while the category was persisting.
	var d *dialog.Dialog
	var st *loanWizardData
	if a.loan.dlg != nil {
		d, st = a.loan.dlg, a.loan.state
	}
	if d == nil || st == nil || len(d.Fields()) < loanFieldFieldsCount {
		a.createCat.dlg = nil
		a.createCat.origin.loanField = -1
		return
	}
	fields := d.Fields()

	// Capture each category combo's currently-selected ID against the OLD id
	// lists, before the rebuild shifts indices.
	idAt := func(ids []types.ID, idx int) types.ID {
		if idx >= 0 && idx < len(ids) {
			return ids[idx]
		}
		return types.NilID
	}
	selectedInterestID := idAt(st.interestIDs, fields[loanFieldInterestCategory].SelectedIndex)
	principalSel := fields[loanFieldPrincipalCategory].SelectedIndex
	selectedPrincipalID := idAt(st.principalIDs, principalSel)
	// The synthetic Loan > Principal default row shares NilID with "(None)"; a
	// plain indexOf(NilID) would collapse it onto "(None)", so remember it here.
	principalWasDefault := principalSel > 0 && principalSel < len(st.principalOptions) &&
		st.principalOptions[principalSel] == loanPrincipalDefaultDisplay && selectedPrincipalID.IsNil()
	selectedEscrowIDs := make([]types.ID, loanMaxEscrowLines)
	for k := range loanMaxEscrowLines {
		selectedEscrowIDs[k] = idAt(st.categoryIDs, fields[loanEscrowCatIndex(k)].SelectedIndex)
	}

	// Rebuild the option/ID lists including the new category.
	catOptions, catIDs := buildCategoryOptions(cats)
	interestOptions, interestIDs, interestDefaultIdx := buildLoanInterestOptions(catOptions, catIDs)
	principalOptions, principalIDs, principalDefaultIdx := buildLoanPrincipalOptions(catOptions, catIDs)
	st.categoryIDs = catIDs
	st.interestOptions = interestOptions
	st.interestIDs = interestIDs
	st.principalOptions = principalOptions
	st.principalIDs = principalIDs

	indexOf := func(ids []types.ID, id types.ID) int {
		for i, x := range ids {
			if x == id {
				return i
			}
		}
		return 0 // fall back to the leading "(None)"/default row
	}
	setCombo := func(f *dialog.Field, opts []string, idx int) {
		f.Options = opts
		f.SelectedIndex = idx
		f.ComboHighlight = idx
		f.Query = ""
	}

	// Re-point every combo, preserving selections by ID. A NilID interest
	// selection means "on the synthetic default row"; keep it on the default
	// even when creating the real Loan:Interest category collapses that synthetic
	// row away (buildLoanInterestOptions then returns no NilID entry, and a plain
	// indexOf would fall through to the first-alphabetical category).
	interestIdx := indexOf(interestIDs, selectedInterestID)
	if selectedInterestID == types.NilID {
		interestIdx = interestDefaultIdx
	}
	setCombo(fields[loanFieldInterestCategory], interestOptions, interestIdx)
	// Principal: "(None)" (NilID at idx 0) round-trips via indexOf; the synthetic
	// default (also NilID) is disambiguated by the captured flag.
	principalIdx := indexOf(principalIDs, selectedPrincipalID)
	if principalWasDefault {
		principalIdx = principalDefaultIdx
	}
	setCombo(fields[loanFieldPrincipalCategory], principalOptions, principalIdx)
	for k := range loanMaxEscrowLines {
		setCombo(fields[loanEscrowCatIndex(k)], catOptions, indexOf(catIDs, selectedEscrowIDs[k]))
	}

	// Point the originating field at the freshly-created category and focus it.
	if fld := a.createCat.origin.loanField; fld >= 0 && fld < len(fields) {
		switch fld {
		case loanFieldInterestCategory:
			fields[fld].SelectedIndex = indexOf(interestIDs, newCat.ID)
		case loanFieldPrincipalCategory:
			fields[fld].SelectedIndex = indexOf(principalIDs, newCat.ID)
		default:
			fields[fld].SelectedIndex = indexOf(catIDs, newCat.ID)
		}
		fields[fld].ComboHighlight = fields[fld].SelectedIndex
		d.SetFocusIndex(fld)
	}

	updateLoanWizardVisibility(d)
	d.SetVisible(true)
	a.createCat.dlg = nil
	a.createCat.origin.loanField = -1
}
