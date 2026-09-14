package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// The paycheck surface: the controller half of the wizard. paycheckSurface
// owns open, submit, close and the create-category hooks; App supplies the
// services through paycheckDeps and the divert into the create-category
// sub-dialog, which is a sibling surface's state. The App glue at the end of
// this file is thin on purpose — an action the surface owns must not have a
// second implementation on App.

// paycheckDeps is what the paycheck surface needs from outside itself. The two
// constraints are the ones the transfer pilot established, and both are live:
//
//   - Every dep is a func, because switchDatabase replaces App's services and
//     closes the previous *db.DB. A closure re-reads the field at call time; a
//     captured pointer would be a use-after-close.
//   - Deps are passed to each call and never stored on the surface, because
//     close() resets the surface to its zero value and would zero them.
//
// Both are pinned by tests: TestGuard_ControllerDepsAreLiveIndirections and
// TestGuard_NoSurfaceStructHoldsItsDeps.
type paycheckDeps struct {
	accounts   func() *account.Service
	categories func() *category.Service
	payees     func() *payee.Service
	scheduled  func() *scheduled.Service
	undo       func() *undo.Manager
}

// paycheckDeps binds the paycheck surface to the services App owns. Every
// accessor may legitimately return nil — an App built by a test has no services
// — so each caller keeps its own nil guard.
func (a *App) paycheckDeps() paycheckDeps {
	return paycheckDeps{
		accounts:   func() *account.Service { return a.services.Account },
		categories: func() *category.Service { return a.services.Category },
		payees:     func() *payee.Service { return a.services.Payee },
		scheduled:  func() *scheduled.Service { return a.services.Scheduled },
		undo:       func() *undo.Manager { return a.undoManager },
	}
}

// paycheckSurface is the paycheck wizard as a modal surface. Its zero value is
// closed; close() resets it to that.
//
// Unlike the dialog-backed surfaces it does not embed modalSurface: the wizard
// is its own form widget with its own Render and hit-testing, so the surface
// wraps the wizard rather than a *dialog.Dialog.
type paycheckSurface struct {
	wizard *PaycheckWizard
}

func (s *paycheckSurface) IsVisible() bool { return s != nil && s.wizard.IsVisible() }

func (s *paycheckSurface) Render(styles widget.Styles) string { return s.wizard.Render(styles) }

// paycheckWizardDataMsg carries the dependencies needed to construct
// a PaycheckWizard. Dispatched asynchronously by open.
type paycheckWizardDataMsg struct {
	accounts        []*account.Account
	categoryOptions []string
	categoryIDs     []types.ID
}

// open returns the command that loads the accounts and categories a new
// wizard needs. The wizard is built when the resulting message reaches
// applyData; there is nothing to mutate before the data lands, so the receiver
// goes unread — open is a method because opening is the surface's operation,
// not App's.
func (s *paycheckSurface) open(deps paycheckDeps) tea.Cmd {
	return func() tea.Msg {
		var accounts []*account.Account
		if svc := deps.accounts(); svc != nil {
			acs, err := svc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}

		var categories []*category.Category
		if svc := deps.categories(); svc != nil {
			cs, err := svc.List()
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

// applyData builds a fresh wizard over the loaded accounts and categories.
func (s *paycheckSurface) applyData(msg paycheckWizardDataMsg) {
	s.wizard = NewPaycheckWizard(msg.categoryOptions, msg.categoryIDs, msg.accounts)
}

// openFromSchedule builds the wizard pre-filled from an existing paycheck
// schedule, for the Edit-as-paycheck relaunch. The caller has already checked
// that every account the schedule names is one the pickers will offer.
func (s *paycheckSurface) openFromSchedule(
	st *scheduled.Transaction,
	accounts []*account.Account,
	payees []*payee.Payee,
	categoryOptions []string,
	categoryIDs []types.ID,
) {
	s.wizard = NewPaycheckWizardFromSchedule(st, accounts, payees, categoryOptions, categoryIDs)
}

// handleKey gives the wizard the key and reports what it wants done about it.
// An unbuilt surface asks for nothing.
func (s *paycheckSurface) handleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if s.wizard == nil {
		return dialog.DialogActionNone
	}
	return s.wizard.HandleKey(msg)
}

// handleMouse translates a click into the wizard's action. The wizard's
// hit-testing is style-dependent, which is why this is not the mouseTarget
// shape the dialog-backed surfaces share (see modalMouseAction).
func (s *paycheckSurface) handleMouse(msg tea.MouseMsg, styles widget.Styles, screenWidth, screenHeight int) dialog.DialogAction {
	if s.wizard == nil {
		return dialog.DialogActionNone
	}
	return s.wizard.HandleMouse(msg, styles, screenWidth, screenHeight)
}

// close clears the surface. The zero value is the closed state, so this is the
// whole of it — and it stays that way only because the deps are a call
// parameter rather than a field.
func (s *paycheckSurface) close() { *s = paycheckSurface{} }

// submit validates the wizard's state and returns the command that persists
// the schedule. A nil command means validation failed: the wizard stays open
// with errorMsg set. On success the surface is closed before the command runs.
//
// A wizard opened by Edit-as-paycheck carries the schedule it came from, and
// then this updates that record instead of creating a second one — which is
// what "two paychecks pending after one edit" was.
func (s *paycheckSurface) submit(deps paycheckDeps) tea.Cmd {
	w := s.wizard
	if w == nil {
		return nil
	}

	accountID := w.lookupAccountID(w.accountField.SelectedIndex)
	if accountID.IsNil() {
		w.errorMsg = "Pick a deposit account"
		return nil
	}

	nextDate, err := parseDateInput(w.nextPaydayField.Value)
	if err != nil {
		w.errorMsg = "Next payday: invalid date (MM/DD/YYYY)"
		return nil
	}

	freqOpt := paycheckFrequencyForIndex(w.frequencyField.SelectedIndex)

	parentAmount, splits, err := w.BuildSplits()
	if err != nil {
		w.errorMsg = err.Error()
		return nil
	}

	// Reject self-transfers: a row that targets the deposit account.
	for _, sp := range splits {
		if sp.TransferAccountID.Valid && sp.TransferAccountID.ID == accountID {
			w.errorMsg = "A transfer row's destination cannot be the deposit account"
			return nil
		}
	}

	employer := strings.TrimSpace(w.employerField.Value)
	memo := strings.TrimSpace(w.memoField.Value)
	// Read the edit target before closing the wizard clears it.
	existing := w.editSchedule

	s.close()

	return func() tea.Msg {
		undoMgr, schedSvc := deps.undo(), deps.scheduled()
		if undoMgr == nil || schedSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		var payeeID types.ID
		if payeeSvc := deps.payees(); employer != "" && payeeSvc != nil {
			py, _, err := payeeSvc.GetOrCreate(employer)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = py.ID
		}

		// applyWizard writes every field the wizard owns — in both directions —
		// and nothing else. Both the create and the edit path run it, so the
		// two cannot drift: on a fresh record each clear is a no-op, and on an
		// existing one each clear is the whole point.
		//
		// It deliberately leaves Interval, EndDate, Occurrences,
		// OccurrencesRemaining, AutoPost, PostLeadDays and AmountEstimateCount
		// alone. The wizard has no widget for any of them, so an edit must
		// preserve what is stored. Do not "reset them to be safe" either:
		// SetEndDate clears Occurrences and SetOccurrences clears EndDate.
		applyWizard := func(st *scheduled.Transaction) {
			st.AccountID = accountID
			st.Frequency = freqOpt.frequency

			// Set or clear, never just set: only the two semi-monthly presets
			// carry days, so switching away from semi-monthly has to drop the
			// stale pair. A leftover DayOfMonth would pin every future
			// occurrence of a Monthly schedule to the old payday.
			if freqOpt.dayOfMonth != 0 {
				st.SetDayOfMonth(freqOpt.dayOfMonth)
			} else {
				st.ClearDayOfMonth()
			}
			// There is no SetSecondaryDayOfMonth/ClearSecondaryDayOfMonth helper.
			st.SecondaryDayOfMonth = types.NullableInt{
				Int64: int64(freqOpt.secondaryDayOfMonth),
				Valid: freqOpt.secondaryDayOfMonth != 0,
			}

			// The field is labelled "Next payday" and is pre-filled from
			// NextDate, so NextDate is what it writes. StartDate is the
			// historical anchor and stays as loaded — except when the user
			// moves the next payday behind it, which Service.Update would
			// otherwise snap straight back and swallow the edit.
			st.NextDate = nextDate
			if st.NextDate.Before(st.StartDate) {
				st.StartDate = st.NextDate
			}

			st.SetAmount(parentAmount)
			if !payeeID.IsNil() {
				st.SetPayee(payeeID)
			} else {
				st.ClearPayee()
			}
			// A multi-line parent carries no scalar category.
			st.ClearCategory()
			// SetMemo("") clears, so blanking the field is honoured.
			st.SetMemo(memo)
			st.Splits = scheduled.SplitCollection(splits)
		}

		if existing != nil {
			applyWizard(existing)
			cmd := undo.NewEditScheduledTransactionCommand(schedSvc, existing)
			if err := undoMgr.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transaction: %w", err)}
			}
			return scheduledDialogSavedMsg{}
		}

		st := scheduled.NewTransaction(accountID, freqOpt.frequency, nextDate)
		applyWizard(st)
		cmd := undo.NewCreateScheduledTransactionCommand(schedSvc, st)
		if err := undoMgr.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
		}
		return scheduledDialogSavedMsg{}
	}
}

// beginCreateCategory hides the wizard and reports the line whose select field
// activated the [+ Add new category…] sentinel. The wizard is kept alive
// (hidden) so its row state survives the divert; applyCreatedCategory re-shows
// it with the new category selected, reshow does so after a cancel. It reports
// false when no line's select field is focused, in which case nothing was
// hidden and the caller must not open the sub-dialog.
//
// Unlike the typeahead-combo surfaces, the wizard has no typed query to
// harvest — the sub-dialog opens with empty Name and Parent fields.
func (s *paycheckSurface) beginCreateCategory() (line *PaycheckLine, ok bool) {
	w := s.wizard
	if w == nil {
		return nil, false
	}
	line = w.lineForSelectField(w.focusedTarget().field)
	if line == nil {
		return nil, false
	}
	w.SetVisible(false)
	return line, true
}

// applyCreatedCategory rebuilds the wizard's combined picker options to include
// the freshly-persisted category, points the originating line at the new
// category, preserves every other category-mode line's selection by ID, and
// shifts every transfer-mode line's SelectedIndex by the new-category-count
// delta so the same account remains selected. The wizard is re-shown.
// Persistence already happened in persistCategory; the router passes the
// fresh category in. The persist is asynchronous, so a surface the user has
// closed in the meantime is left alone.
func (s *paycheckSurface) applyCreatedCategory(newCat *category.Category, cats []*category.Category, originating *PaycheckLine) {
	w := s.wizard
	if w == nil {
		return
	}

	oldCatCount := len(w.categoryOptions)
	oldCategoryIDs := w.categoryIDs

	options, ids := buildCategoryOptions(cats)
	w.categoryOptions = options
	w.categoryIDs = ids

	combined := make([]string, 0, len(options)+len(w.accountOptions)+1)
	combined = append(combined, options...)
	for _, name := range w.accountOptions {
		combined = append(combined, "→ "+name)
	}
	combined = append(combined, paycheckAddNewSentinelLabel)
	w.combinedOptions = combined

	newCatCount := len(options)
	delta := newCatCount - oldCatCount

	idToNewIdx := make(map[types.ID]int, len(ids))
	for i, id := range ids {
		idToNewIdx[id] = i
	}

	newCatIdx := 0
	if idx, ok := idToNewIdx[newCat.ID]; ok {
		newCatIdx = idx
	}

	for sec := PaycheckEarnings; sec <= PaycheckNetPayDestination; sec++ {
		for _, line := range w.sections[sec] {
			// Lines were built with selectField.Options pointing at the prior
			// combinedOptions slice; reassigning w.combinedOptions does not
			// propagate, so each line's Options must be updated explicitly.
			line.selectField.Options = combined
			line.categoryCount = newCatCount

			if line == originating {
				line.selectField.SelectedIndex = newCatIdx
				continue
			}

			oldIdx := line.selectField.SelectedIndex
			switch {
			case oldIdx >= oldCatCount:
				// Transfer-mode line (or — defensively — the AddNew sentinel,
				// which shouldn't persist on a line). Shift by the
				// category-count delta so the same account stays selected.
				line.selectField.SelectedIndex = oldIdx + delta
			case oldIdx >= 0 && oldIdx < len(oldCategoryIDs):
				// Category-mode line. Preserve by ID — the new category may
				// have been inserted alphabetically into the middle, shifting
				// subsequent indices.
				if newIdx, ok := idToNewIdx[oldCategoryIDs[oldIdx]]; ok {
					line.selectField.SelectedIndex = newIdx
				} else {
					line.selectField.SelectedIndex = 0
				}
			}
		}
	}

	w.SetVisible(true)
}

// reshow makes the wizard visible again after a create-category divert the
// user cancelled. Nil-safe, because the divert can outlive the surface.
func (s *paycheckSurface) reshow() {
	if s.wizard != nil {
		s.wizard.SetVisible(true)
	}
}

// defaultTypeForPaycheckSection returns the create-category Type default for a
// paycheck-wizard section. Earnings and Net Pay Destination rows describe
// money flowing in (Income); Tax, Pre-Tax, and Post-Tax rows describe money
// flowing out (Expense).
func defaultTypeForPaycheckSection(s PaycheckSection) category.Type {
	switch s {
	case PaycheckEarnings, PaycheckNetPayDestination:
		return category.TypeIncome
	default:
		return category.TypeExpense
	}
}

// -----------------------------------------------------------------------------
// App glue. Everything below is a thin wrapper supplying the services and the
// divert into a sibling surface. No behaviour lives here: an action the
// surface owns must not have a second implementation on App.
// -----------------------------------------------------------------------------

// handlePaycheckWizardKey routes a key event through the wizard and
// translates the resulting action.
func (a *App) handlePaycheckWizardKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.paycheckWizardAction(a.paycheck.handleKey(msg))
}

// paycheckWizardAction dispatches a DialogAction for the paycheck wizard, from
// either input path.
//
// AddNew is the one arm that stays on App: it writes createCat, which is
// another surface's state, and a surface must not reach into a sibling.
func (a *App) paycheckWizardAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a, a.paycheck.submit(a.paycheckDeps())
	case dialog.DialogActionCancel:
		a.paycheck.close()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromPaycheck()
	}
	return a, nil
}

// openCreateCategorySubDialogFromPaycheck hides the paycheck wizard and opens
// the inline create-category sub-dialog for the wizard line that activated the
// [+ Add new category…] sentinel. Restoration on cancel and post-create wiring
// happen through the createCatDialog handlers.
func (a *App) openCreateCategorySubDialogFromPaycheck() (tea.Model, tea.Cmd) {
	line, ok := a.paycheck.beginCreateCategory()
	if !ok {
		return a, nil
	}
	origin := originFrom(createCatSourcePaycheckWizard)
	origin.line = line
	a.createCat.open(origin, "", "", a.createCatParentsFor(origin.surface), defaultTypeForPaycheckSection(line.Section))
	return a, nil
}

// applyCreatedCategoryToPaycheck is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the
// paycheck wizard. The surface applies the category to its own lines; App
// clears the sub-dialog and the originating-line handle, which are its own
// state.
func (a *App) applyCreatedCategoryToPaycheck(newCat *category.Category, cats []*category.Category) {
	a.paycheck.applyCreatedCategory(newCat, cats, a.createCat.openedFrom().line)
	a.createCat.close()
}
