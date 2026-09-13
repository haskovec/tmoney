package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// App integration: the async data load, the key/action dispatchers the modal
// registry calls, submit, and the create-category divert's opener and applier.
// Every function here is a method on *App or is called only by one.

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
	return a.paycheckWizardAction(a.paycheckWizard.HandleKey(msg))
}

// paycheckWizardAction dispatches a DialogAction for the paycheck wizard, from either input path.
func (a *App) paycheckWizardAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitPaycheckWizard()
	case dialog.DialogActionCancel:
		a.closePaycheckWizard()
		return a, nil
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromPaycheck()
	}
	return a, nil
}

// submitPaycheckWizard validates the wizard's state and persists the
// schedule. Validation errors leave the wizard open with errorMsg set.
//
// A wizard opened by Edit-as-paycheck carries the schedule it came from, and
// then this updates that record instead of creating a second one — which is
// what "two paychecks pending after one edit" was.
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

	nextDate, err := parseDateInput(w.nextPaydayField.Value)
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
	// Read the edit target before closing the wizard clears it.
	existing := w.editSchedule

	a.closePaycheckWizard()

	return a, func() tea.Msg {
		if a.undoManager == nil || a.services.Scheduled == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		var payeeID types.ID
		if employer != "" && a.services.Payee != nil {
			py, _, err := a.services.Payee.GetOrCreate(employer)
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
			cmd := undo.NewEditScheduledTransactionCommand(a.services.Scheduled, existing)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transaction: %w", err)}
			}
			return scheduledDialogSavedMsg{}
		}

		st := scheduled.NewTransaction(accountID, freqOpt.frequency, nextDate)
		applyWizard(st)
		cmd := undo.NewCreateScheduledTransactionCommand(a.services.Scheduled, st)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
		}
		return scheduledDialogSavedMsg{}
	}
}

// openCreateCategorySubDialogFromPaycheck hides the paycheck wizard and opens
// the inline create-category sub-dialog for the wizard line that activated the
// [+ Add new category…] sentinel. The wizard's row state is preserved by
// keeping the wizard instance alive (just hidden) for the duration of the
// divert; restoration on cancel and post-create wiring happen through the
// createCatDialog handlers.
//
// Unlike the typeahead-combo surfaces, the wizard has no typed query to
// harvest — the sub-dialog opens with empty Name and Parent fields.
func (a *App) openCreateCategorySubDialogFromPaycheck() (tea.Model, tea.Cmd) {
	w := a.paycheckWizard
	if w == nil {
		return a, nil
	}
	line := w.lineForSelectField(w.focusedTarget().field)
	if line == nil {
		return a, nil
	}

	a.createCat.origin.surface = createCatSourcePaycheckWizard
	a.createCat.origin.line = line
	parents := a.parentsForCreateCatDialog()
	a.createCat.dlg = buildCreateCategoryDialog("", "", parents, defaultTypeForPaycheckSection(line.Section))
	w.SetVisible(false)
	return a, nil
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

// applyCreatedCategoryToPaycheck is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the
// paycheck wizard. It rebuilds the wizard's combined picker options to include
// the freshly-persisted category, points the originating line at the new
// category, preserves every other category-mode line's selection by ID, and
// shifts every transfer-mode line's SelectedIndex by the new-category-count
// delta so the same account remains selected. The wizard is re-shown and the
// sub-dialog is cleared.
func (a *App) applyCreatedCategoryToPaycheck(newCat *category.Category, cats []*category.Category) {
	defer func() {
		a.createCat.dlg = nil
		a.createCat.origin.line = nil
	}()
	w := a.paycheckWizard
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

	originating := a.createCat.origin.line
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for _, line := range w.sections[s] {
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
