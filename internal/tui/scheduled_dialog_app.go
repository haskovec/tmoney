package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// App integration: the async data loads for new and edit, the key/action
// dispatchers the modal registry calls, the per-account category refresh, the
// create-category divert's opener and applier, and the relaunch into the
// paycheck wizard.

// loadNewScheduledDialogData returns a command that loads data for a new scheduled dialog.
func (a *App) loadNewScheduledDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &scheduledDialogData{
			mode:     scheduledDialogModeNew,
			payeeMap: make(map[string]*payee.Payee),
		}

		if a.services.Account != nil {
			accounts, err := a.services.Account.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		if a.services.Payee != nil {
			payees, err := a.services.Payee.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.payees = payees
			for _, p := range payees {
				data.payeeMap[strings.ToLower(p.Name)] = p
			}
		}

		return scheduledDialogDataMsg{data: data}
	}
}

// loadEditScheduledDialogData returns a command that loads data for editing a scheduled transaction.
func (a *App) loadEditScheduledDialogData() tea.Cmd {
	if a.scheduled == nil || a.scheduledTable == nil {
		return nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return nil
	}

	st := a.scheduled.allTxns[cursor]
	return func() tea.Msg {
		data := &scheduledDialogData{
			mode:       scheduledDialogModeEdit,
			scheduled:  st,
			payeeMap:   make(map[string]*payee.Payee),
			isTransfer: st.IsTransfer(),
		}

		if a.services.Account != nil {
			accounts, err := a.services.Account.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		if a.services.Payee != nil {
			payees, err := a.services.Payee.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.payees = payees
			for _, p := range payees {
				data.payeeMap[strings.ToLower(p.Name)] = p
			}
		}

		return scheduledDialogDataMsg{data: data}
	}
}

// categoriesOrNil lists every category, or nil when the service is absent or
// the lookup fails. A combo that has to be built now treats a failed lookup as
// an empty list rather than as an error worth an error page.
func (a *App) categoriesOrNil() []*category.Category {
	if a.services.Category == nil {
		return nil
	}
	cats, err := a.services.Category.List()
	if err != nil {
		return nil
	}
	return cats
}

// closeScheduledDialog clears the scheduled dialog state.
func (a *App) closeScheduledDialog() {
	a.sched = schedSurface{}
}

// handleScheduledDialogKey routes key events to the scheduled dialog.
func (a *App) handleScheduledDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.sched.dlg == nil {
		return a, nil
	}
	return a.scheduledDialogAction(a.sched.dlg.HandleKey(msg))
}

// scheduledDialogAction dispatches a DialogAction for the scheduled dialog, from either input path.
func (a *App) scheduledDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		if a.sched.data != nil && a.sched.data.isTransfer {
			return a.submitScheduledTransferDialog()
		}
		return a.submitScheduledDialog()
	case dialog.DialogActionCancel:
		a.closeScheduledDialog()
		return a, nil
	case dialog.DialogActionAlternate:
		return a.relaunchScheduledAlternate()
	case dialog.DialogActionAddNew:
		if a.sched.data != nil && a.sched.data.isTransfer {
			return a.openCreateCategorySubDialogFromSchedTransfer()
		}
		return a.openCreateCategorySubDialogFromSched()
	}

	// The account picker is editable in this dialog; if the user just
	// switched it to (or away from) an asset account, refresh the
	// category options so Value Adjustment appears/disappears. No-op
	// unless the asset-ness actually changed. Skipped for the transfer
	// dialog, which has no category field.
	if a.sched.data != nil && !a.sched.data.isTransfer {
		a.refreshSchedCategoryOptionsForAccount()
	}

	return a, nil
}

// schedDialogIncludeValueAdjustment reports whether the scheduled
// dialog's currently selected account is an asset account, and thus
// whether the Value Adjustment category should be offered in its
// category picker.
func (a *App) schedDialogIncludeValueAdjustment() bool {
	if a.sched.dlg == nil || a.sched.data == nil {
		return false
	}
	fields := a.sched.dlg.Fields()
	if len(fields) <= schedFieldAccount {
		return false
	}
	acctIdx := fields[schedFieldAccount].SelectedIndex
	if acctIdx < 0 || acctIdx >= len(a.sched.accountIDs) {
		return false
	}
	return accountIsAssetByID(a.sched.data.accounts, a.sched.accountIDs[acctIdx])
}

// refreshSchedCategoryOptionsForAccount rebuilds the scheduled dialog's
// category combo when the selected account's asset-ness has changed
// since the options were last built — surfacing or hiding the Value
// Adjustment category accordingly. It preserves the current category
// selection by ID and is a no-op when nothing changed (so it is cheap
// to call on every keypress).
func (a *App) refreshSchedCategoryOptionsForAccount() {
	if a.sched.dlg == nil || a.services.Category == nil {
		return
	}
	fields := a.sched.dlg.Fields()
	if len(fields) <= schedFieldCategory {
		return
	}

	includeVA := a.schedDialogIncludeValueAdjustment()
	hasVA := slices.Contains(a.sched.categoryOptions, category.ValueAdjustmentCategoryName)
	if hasVA == includeVA {
		return
	}

	catField := fields[schedFieldCategory]
	selectedID := types.NilID
	if catField.SelectedIndex >= 0 && catField.SelectedIndex < len(a.sched.categoryIDs) {
		selectedID = a.sched.categoryIDs[catField.SelectedIndex]
	}

	cats, err := a.services.Category.List()
	if err != nil {
		return
	}
	options, ids := buildCategoryOptionsFor(cats, includeVA)
	a.sched.categoryOptions = options
	a.sched.categoryIDs = ids
	catField.Options = options

	newIdx := 0
	for i, id := range ids {
		if id == selectedID {
			newIdx = i
			break
		}
	}
	catField.SelectedIndex = newIdx
}

// openCreateCategorySubDialogFromSched hides the scheduled dialog and opens
// the inline create-category sub-dialog seeded with the typed query from the
// Category combo. The scheduled dialog's field state is preserved by keeping
// the dialog alive (just hidden) for the duration of the divert; restoration
// on cancel and post-create wiring happens through the createCatDialog
// handlers.
func (a *App) openCreateCategorySubDialogFromSched() (tea.Model, tea.Cmd) {
	if a.sched.dlg == nil {
		return a, nil
	}
	fields := a.sched.dlg.Fields()
	if len(fields) <= schedFieldCategory {
		return a, nil
	}
	catField := fields[schedFieldCategory]
	query := catField.Query
	catField.AddNewTriggered = false
	catField.Query = ""

	// createCatSource must be set before parentsForCreateCatDialog so the
	// helper picks the right parents source.
	a.createCat.origin.surface = createCatSourceSchedDialog
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	defaultType := category.TypeExpense
	if len(fields) > schedFieldAmount {
		defaultType = inferCategoryTypeFromAmount(fields[schedFieldAmount].Value)
	}
	a.createCat.dlg = buildCreateCategoryDialog(name, parent, parents, defaultType)
	a.sched.dlg.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSched is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the New /
// Edit Scheduled Transaction dialog. It reloads the dialog's category list
// with newCat pre-selected on the Category combo, advances focus to Amount,
// re-shows the scheduled dialog, and clears the create-category sub-dialog.
func (a *App) applyCreatedCategoryToSched(newCat *category.Category, cats []*category.Category) {
	if a.sched.dlg == nil {
		a.createCat.dlg = nil
		return
	}
	options, ids := buildCategoryOptionsFor(cats, a.schedDialogIncludeValueAdjustment())
	a.sched.categoryIDs = ids
	a.sched.categoryOptions = options

	if len(a.sched.dlg.Fields()) > schedFieldCategory {
		catField := a.sched.dlg.Fields()[schedFieldCategory]
		catField.Options = options
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		catField.SelectedIndex = newIdx
		// Focus advances to Amount so the user can keep typing.
		a.sched.dlg.SetFocusIndex(schedFieldAmount)
		a.sched.dlg.SetVisible(true)
	}
	a.createCat.dlg = nil
}

// relaunchAsPaycheckWizard closes the scheduled-edit dialog and
// opens the paycheck wizard pre-filled from the in-flight schedule.
func (a *App) relaunchAsPaycheckWizard() (tea.Model, tea.Cmd) {
	if a.sched.dlg == nil || a.sched.data == nil {
		return a, nil
	}
	if a.sched.data.mode != scheduledDialogModeEdit || a.sched.data.scheduled == nil {
		return a, nil
	}
	st := a.sched.data.scheduled
	accounts := a.sched.data.accounts
	payees := a.sched.data.payees
	categoryOptions := a.sched.categoryOptions
	categoryIDs := a.sched.categoryIDs

	// Refuse rather than pre-fill wrong. The wizard's pickers only offer
	// active accounts, so a closed deposit account would silently resolve to
	// the first active one and a closed transfer destination to "(None)" —
	// and saving now rewrites the live schedule rather than misfiling a
	// duplicate.
	if missingAccountForPaycheckEdit(st, accounts) {
		if a.statusbar != nil {
			a.statusbar.SetToast(
				"This paycheck uses a closed account. Reopen the account to edit it as a paycheck.",
				widget.NotificationAlert,
			)
		}
		return a, widget.ClearToastCmd()
	}

	a.closeScheduledDialog()
	a.paycheckWizard = NewPaycheckWizardFromSchedule(st, accounts, payees, categoryOptions, categoryIDs)
	return a, nil
}
