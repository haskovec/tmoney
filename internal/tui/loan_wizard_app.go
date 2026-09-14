package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// The loan surface: the controller half of the wizard. loanSurface owns open,
// submit (loan_wizard_submit.go), close, the derived-state refresh
// (loan_wizard_derive.go) and the create-category hooks; App supplies the
// services through loanDeps and the divert into the create-category
// sub-dialog, which is a sibling surface's state. The Edit-as-loan affordance
// on the scheduled dialog lives with that dialog (scheduled_dialog_app.go),
// because it reads and writes the scheduled dialog's state, not this surface's.

// loanDeps is what the loan surface needs from outside itself. Every dep is a
// func, because switchDatabase replaces App's services and closes the previous
// *db.DB; and deps are passed to each call, never stored, because close()
// resets the surface to its zero value. Both rules are pinned by
// TestGuard_ControllerDepsAreLiveIndirections and
// TestGuard_NoSurfaceStructHoldsItsDeps.
type loanDeps struct {
	accounts   func() *account.Service
	categories func() *category.Service
	payees     func() *payee.Service
	scheduled  func() *scheduled.Service
	undo       func() *undo.Manager
}

// loanDeps binds the loan surface to the services App owns. Every accessor may
// legitimately return nil — an App built by a test has no services — so each
// caller keeps its own nil guard.
func (a *App) loanDeps() loanDeps {
	return loanDeps{
		accounts:   func() *account.Service { return a.services.Account },
		categories: func() *category.Service { return a.services.Category },
		payees:     func() *payee.Service { return a.services.Payee },
		scheduled:  func() *scheduled.Service { return a.services.Scheduled },
		undo:       func() *undo.Manager { return a.undoManager },
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

// loanWizardSavedMsg is emitted after a successful save so the app reloads the
// affected views.
type loanWizardSavedMsg struct{}

// open returns the command that loads the accounts and categories a new
// wizard needs. The wizard is built when the resulting message reaches
// applyData; there is nothing to mutate before the data lands, so the receiver
// goes unread — open is a method because opening is the surface's operation,
// not App's.
func (s *loanSurface) open(deps loanDeps) tea.Cmd {
	return func() tea.Msg {
		accounts, categories, err := loadLoanAccountsAndCategories(deps)
		if err != nil {
			return errMsg{err: err}
		}
		return loanWizardDataMsg{accounts: accounts, categories: categories}
	}
}

// openForEdit returns the command that loads the accounts and categories and
// computes the loan's live balance for the Edit-as-loan flow, emitting a
// loanWizardDataMsg carrying the schedule so applyData builds the wizard in
// edit mode.
func (s *loanSurface) openForEdit(deps loanDeps, st *scheduled.Transaction) tea.Cmd {
	return func() tea.Msg {
		accounts, categories, err := loadLoanAccountsAndCategories(deps)
		if err != nil {
			return errMsg{err: err}
		}
		// The loan's live balance as of the next payment date → owed magnitude.
		owed := types.ZeroMoney
		if svc := deps.accounts(); svc != nil {
			for _, sp := range st.Splits {
				if !sp.TransferAccountID.Valid {
					continue
				}
				bal, err := svc.BalanceAsOf(sp.TransferAccountID.ID, st.NextDate)
				if err == nil {
					owed = bal.Neg()
				}
				break
			}
		}
		return loanWizardDataMsg{accounts: accounts, categories: categories, editSchedule: st, editOwed: owed}
	}
}

// loadLoanAccountsAndCategories fetches what both open paths need identically.
// It runs on the command's own goroutine, so it reaches every service through
// deps rather than holding one.
func loadLoanAccountsAndCategories(deps loanDeps) ([]*account.Account, []*category.Category, error) {
	var accounts []*account.Account
	if svc := deps.accounts(); svc != nil {
		acs, err := svc.List(true)
		if err != nil {
			return nil, nil, err
		}
		accounts = acs
	}
	var categories []*category.Category
	if svc := deps.categories(); svc != nil {
		cs, err := svc.List()
		if err != nil {
			return nil, nil, err
		}
		categories = cs
	}
	return accounts, categories, nil
}

// handleKey gives the dialog the key and reports what it wants done about it.
// An unbuilt surface asks for nothing.
func (s *loanSurface) handleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if s.dlg == nil {
		return dialog.DialogActionNone
	}
	return s.dlg.HandleKey(msg)
}

// close clears the surface. The zero value is the closed state, so this is the
// whole of it — and it stays that way only because the deps are a call
// parameter rather than a field.
func (s *loanSurface) close() { *s = loanSurface{} }

// beginCreateCategory takes the typed query off whichever category combo
// (interest, principal or an escrow row) activated [+ Add new category…],
// consumes the trigger, hides the dialog and reports the field's index so the
// applier can point it at the new category. The dialog is kept alive (hidden)
// so its field state survives the divert; applyCreatedCategory re-shows it,
// reshow does so after a cancel. It reports false when no field is focused, in
// which case nothing was hidden and the caller must not open the sub-dialog.
func (s *loanSurface) beginCreateCategory() (query string, fieldIdx int, ok bool) {
	if s.dlg == nil {
		return "", -1, false
	}
	fields := s.dlg.Fields()
	fieldIdx = s.dlg.FocusIndex()
	if fieldIdx < 0 || fieldIdx >= len(fields) {
		return "", -1, false
	}
	catField := fields[fieldIdx]
	query = catField.Query
	// Consume the trigger and clear the typed query — the sub-dialog owns it now.
	catField.AddNewTriggered = false
	catField.Query = ""
	s.dlg.SetVisible(false)
	return query, fieldIdx, true
}

// applyCreatedCategory rebuilds every category combo's options to include
// newCat, then points the originating field at it. Because inserting a
// category into the sorted list shifts option indices, each *other* combo's
// existing selection is re-resolved by ID rather than by its stale index —
// otherwise a filled escrow row would silently jump to a different category.
// Persistence already happened in persistCategory; the router passes the fresh
// category in. The persist is asynchronous, so a surface the user has closed
// in the meantime is left alone.
func (s *loanSurface) applyCreatedCategory(newCat *category.Category, cats []*category.Category, originField int) {
	d, st := s.dlg, s.state
	if d == nil || st == nil || len(d.Fields()) < loanFieldFieldsCount {
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
	if fld := originField; fld >= 0 && fld < len(fields) {
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
}

// reshow makes the dialog visible again after a create-category divert the
// user cancelled. Nil-safe, because the divert can outlive the surface.
func (s *loanSurface) reshow() {
	if s.dlg != nil {
		s.dlg.SetVisible(true)
	}
}

// -----------------------------------------------------------------------------
// App glue. Everything below is a thin wrapper supplying the services and the
// divert into a sibling surface. No behaviour lives here: an action the
// surface owns must not have a second implementation on App. The save's
// after-effects (afterLoanWizardSave, loan_wizard_submit.go) are App's too,
// because they touch the status bar and reload views.
// -----------------------------------------------------------------------------

// handleLoanWizardKey routes a key event through the wizard dialog and
// translates the resulting action.
func (a *App) handleLoanWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.loanWizardAction(a.loan.handleKey(msg))
}

// loanWizardAction dispatches a DialogAction for the loan wizard, from either
// input path. An ordinary edit refreshes conditional visibility and the
// payment prefill.
//
// AddNew is the one arm that stays on App: it writes createCat, which is
// another surface's state, and a surface must not reach into a sibling.
func (a *App) loanWizardAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a, a.loan.submit(a.loanDeps())
	case dialog.DialogActionCancel:
		a.loan.close()
		return a, nil
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromLoan()
	}
	a.loan.refreshDerived()
	return a, nil
}

// openCreateCategorySubDialogFromLoan hides the loan wizard and opens the shared
// inline create-category sub-dialog, seeded from the typed query on whichever
// category combo activated [+ Add new category…]. Loan interest and escrow
// lines are always expenses, so the sub-dialog defaults to an Expense type.
func (a *App) openCreateCategorySubDialogFromLoan() (tea.Model, tea.Cmd) {
	query, fieldIdx, ok := a.loan.beginCreateCategory()
	if !ok {
		return a, nil
	}
	a.createCat.origin.loanField = fieldIdx
	// Set the source before parentsForCreateCatDialog so it resolves the right
	// parents (falls back to a live category list for the loan wizard).
	a.createCat.origin.surface = createCatSourceLoanWizard
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	a.createCat.dlg = buildCreateCategoryDialog(name, parent, parents, category.TypeExpense)
	return a, nil
}

// applyCreatedCategoryToLoan is the per-surface applier for the loan wizard.
// The surface applies the category to its own combos; App clears the
// sub-dialog and the originating-field handle, which are its own state.
func (a *App) applyCreatedCategoryToLoan(newCat *category.Category, cats []*category.Category) {
	a.loan.applyCreatedCategory(newCat, cats, a.createCat.origin.loanField)
	a.createCat.dlg = nil
	a.createCat.origin.loanField = -1
}
