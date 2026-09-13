package tui

import (
	"errors"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// App integration: the async data load, the key, action and mouse dispatchers
// the modal registry calls, the multi-line key path, the loan reseed logic,
// and the create-category divert's opener and applier.

// loadSchedulePreviewData loads accounts/payees/categories for the
// currently selected scheduled transaction and emits a
// schedulePreviewDataMsg. Per MS-019 this replaces the legacy
// immediate-post path on Enter — the message handler then constructs
// the preview dialog from the template values.
//
// If the cursor is out of range or the required state is missing the
// returned command is nil and Enter is a no-op.
func (a *App) loadSchedulePreviewData() tea.Cmd {
	if a.scheduled == nil || a.scheduledTable == nil {
		return nil
	}
	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return nil
	}
	template := a.scheduled.allTxns[cursor]
	if template == nil {
		return nil
	}

	return func() tea.Msg {
		var accounts []*account.Account
		if a.services.Account != nil {
			acs, err := a.services.Account.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}

		var payees []*payee.Payee
		if a.services.Payee != nil {
			ps, err := a.services.Payee.List()
			if err != nil {
				return errMsg{err: err}
			}
			payees = ps
		}

		var categories []*category.Category
		if a.services.Category != nil {
			cs, err := a.services.Category.List()
			if err != nil {
				return errMsg{err: err}
			}
			categories = cs
		}

		// Offer Value Adjustment when the schedule posts to an asset
		// account, so a value-adjustment line (e.g. depreciation)
		// survives an edit-at-post rather than reverting to (None). A
		// transfer schedule never gets it: Value Adjustment is a system
		// category and a transfer may be labeled with non-system categories
		// only.
		includeVA := accountIsAssetByID(accounts, template.AccountID) && !template.IsTransfer()
		categoryOptions, categoryIDs := buildCategoryOptionsFor(categories, includeVA)

		// Loan-shaped schedules seed the preview from a live-balance recompute
		// (interest/principal split as of the next payment date) instead of the
		// stored month-one snapshot. A recompute failure means the payment
		// cannot be previewed with correct numbers, so the loader blocks the
		// open with a reason rather than seeding stale template values.
		var loanSplits *scheduled.LoanSplits
		if a.services.Scheduled != nil && a.services.Scheduled.IsLoanShaped(template) {
			ls, err := a.services.Scheduled.ComputeLoanSplits(template, template.NextDate)
			switch {
			case err == nil:
				loanSplits = ls
			case errors.Is(err, scheduled.ErrLoanPaidOff):
				// Opening the preview is the TUI's only manual-post door; a
				// paid-off loan is a terminal state, so refuse and complete
				// the schedule on the spot (Post refuses + marks completed via
				// the same path the CLI uses) rather than stranding a
				// never-postable due schedule. Post returns ErrLoanPaidOff on
				// the successful refuse-and-complete; a *different* error (e.g.
				// a closed funding account tripping the pre-check) means the
				// schedule was NOT completed, so surface that instead of a
				// misleading "paid off" toast.
				if _, perr := a.services.Scheduled.Post(template.ID, nil); perr != nil &&
					!errors.Is(perr, scheduled.ErrLoanPaidOff) {
					return schedulePreviewLoanBlockedMsg{err: perr}
				}
				return schedulePreviewLoanBlockedMsg{paidOff: true}
			default:
				return schedulePreviewLoanBlockedMsg{err: err}
			}
		}

		return schedulePreviewDataMsg{
			template:        template,
			accounts:        accounts,
			payees:          payees,
			categoryOptions: categoryOptions,
			categoryIDs:     categoryIDs,
			loanSplits:      loanSplits,
		}
	}
}

// closeSchedulePreviewDialog clears preview dialog state.
func (a *App) closeSchedulePreviewDialog() {
	a.schedPreviewDialog = nil
}

// handleSchedulePreviewMouse routes a left-click to the schedule preview
// dialog. For a single-line preview the header dialog is centered alone
// and handled directly. For a multi-line preview the header is stacked
// over the embedded split panel and centered together, so the click is
// mapped to whichever panel it lands in using the same overlay-centering
// math the view uses to render them.
func (a *App) handleSchedulePreviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	p := a.schedPreviewDialog
	if p == nil {
		return a, nil
	}
	header := p.HeaderDialog()
	if header == nil {
		return a, nil
	}

	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}

	if !p.IsMultiLine() {
		return a.schedulePreviewAction(header.HandleMouse(msg, a.width, a.height))
	}

	// Map the click against the same composite the paint walk draws (Render),
	// so hit-testing cannot drift from what is on screen.
	overlay := p.Render(a.styles)
	startCol, startRow := widget.OverlayTopLeft(overlay, a.width, a.height)
	headerLines := strings.Count(header.Render(a.styles), "\n") + 1

	m := msg.Mouse()
	relY := m.Y - startRow
	if relY < 0 {
		return a, nil
	}

	// Content-local offsets within a panel: border (1) + h-pad (2) on X,
	// border (1) + v-pad (1) on Y.
	if relY < headerLines {
		p.splitFocus = false
		action := header.HandleMouseLocal(m.X-startCol-3, relY-2)
		// Same tail as the keyboard path: a Date change by click reseeds a
		// loan-shaped preview exactly as a typed one does.
		a.maybeReseedLoanPreview()
		return a.schedulePreviewAction(action)
	}

	p.splitFocus = true
	return a.schedulePreviewSplitAction(p.SplitDialog().HandleMouseLocal(m.X-startCol-3, relY-headerLines-2))
}

// handleSchedulePreviewDialogKey routes keys to the preview dialog. Esc
// cancels (the dialog closes and no transaction is created); Enter on
// the Save button submits — the real transaction is created with any
// user edits, and the schedule advances by one cadence using the
// template's original next_date.
//
// For multi-line previews, keys route to either the header dialog or
// the embedded split editor based on the preview's splitFocus toggle.
// Tab past the header's last focusable element transitions focus into
// the split editor; Shift+Tab from the split editor's first focus
// transitions back to the header. The split editor's MS-013 imbalance
// indicator and disabled-Save behavior apply: a Save attempt with
// imbalanced lines is rejected with the validation error on the
// header.
func (a *App) handleSchedulePreviewDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}

	if a.schedPreviewDialog.IsMultiLine() {
		return a.handleSchedulePreviewMultiLineKey(msg)
	}

	return a.schedulePreviewAction(a.schedPreviewDialog.HeaderDialog().HandleKey(msg))
}

// schedulePreviewAction dispatches a DialogAction from the preview's header
// dialog, from either input path and for both the single-line and the
// multi-line shape.
func (a *App) schedulePreviewAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.closeSchedulePreviewDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitSchedulePreviewDialog()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSchedPreview()
	}
	return a, nil
}

// schedulePreviewSplitAction dispatches a DialogAction from the multi-line
// preview's embedded split editor, from either input path. Any edit there
// (including one that submits) counts as a user edit of the lines, which
// freezes loan reseeding so the user's values win.
func (a *App) schedulePreviewSplitAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	a.freezeLoanSeedIfEdited()
	switch action {
	case dialog.DialogActionCancel:
		a.closeSchedulePreviewDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitSchedulePreviewDialog()
	}
	return a, nil
}

// openCreateCategorySubDialogFromSchedPreview hides the schedule preview
// header dialog and opens the inline create-category sub-dialog seeded with
// the typed query from the Category combo. The preview's field state is
// preserved by keeping the dialog alive (just hidden) for the duration of
// the divert; restoration on cancel and post-create wiring happens through
// the createCatDialog handlers.
func (a *App) openCreateCategorySubDialogFromSchedPreview() (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil {
		return a, nil
	}
	fields := header.Fields()
	catIdx := a.schedPreviewDialog.categoryFieldIndex()
	if catIdx < 0 || catIdx >= len(fields) {
		return a, nil
	}
	catField := fields[catIdx]
	query := catField.Query
	// Consume the trigger and clear the typed query — the create-category
	// dialog now owns it. This way, when we restore the preview, its
	// Category combo doesn't carry stale typed text.
	catField.AddNewTriggered = false
	catField.Query = ""

	// createCatSource must be set before parentsForCreateCatDialog so the
	// helper picks the right source for the parents list.
	a.createCat.origin.surface = createCatSourceSchedPreview
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	// A transfer's always-positive amount carries no income/expense signal, so
	// its create-category divert defaults to Expense; the single-line preview
	// infers the type from the typed amount.
	defaultType := category.TypeExpense
	if !a.schedPreviewDialog.IsTransfer() && len(fields) > previewSingleFieldAmount {
		defaultType = inferCategoryTypeFromAmount(fields[previewSingleFieldAmount].Value)
	}
	a.createCat.dlg = buildCreateCategoryDialog(name, parent, parents, defaultType)
	header.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSchedPreview is the per-surface applier called by
// the createCategoryRequestMsg router when the originating surface was the
// (single-line) schedule preview dialog. It reloads the dialog's category
// list with newCat pre-selected on the Category combo, advances focus to
// Amount, re-shows the preview, and clears the create-category sub-dialog.
// Persistence happened in persistCategory; the router passes in the
// freshly-created category.
func (a *App) applyCreatedCategoryToSchedPreview(newCat *category.Category, cats []*category.Category) {
	if a.schedPreviewDialog == nil {
		a.createCat.dlg = nil
		return
	}
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil {
		a.createCat.dlg = nil
		return
	}
	catIdx := a.schedPreviewDialog.categoryFieldIndex()
	if catIdx < 0 || catIdx >= len(header.Fields()) {
		a.createCat.dlg = nil
		return
	}

	// A transfer preview offers only non-system categories; a single-line
	// preview may additionally surface Value Adjustment (asset-account
	// previews), so preserve that build-time decision across the rebuild.
	var options []string
	var ids []types.ID
	if a.schedPreviewDialog.IsTransfer() {
		options, ids = buildCategoryOptions(cats)
	} else {
		includeVA := slices.Contains(header.Fields()[catIdx].Options, category.ValueAdjustmentCategoryName)
		options, ids = buildCategoryOptionsFor(cats, includeVA)
	}
	a.schedPreviewDialog.categoryIDs = ids

	catField := header.Fields()[catIdx]
	catField.Options = options
	newIdx := 0
	for i, id := range ids {
		if id == newCat.ID {
			newIdx = i
			break
		}
	}
	catField.SelectedIndex = newIdx
	// Focus advances to the field after Category so the user can keep typing.
	header.SetFocusIndex(catIdx + 1)
	header.SetVisible(true)
	a.createCat.dlg = nil
}

// handleSchedulePreviewMultiLineKey routes keys for a multi-line
// preview to either the header dialog or the embedded split editor.
// The two surfaces have independent focus models — the preview's
// splitFocus toggles between them on Tab from the header's last
// focusable element and Shift+Tab from the split editor's first focus.
func (a *App) handleSchedulePreviewMultiLineKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := a.schedPreviewDialog
	header := p.HeaderDialog()
	splits := p.SplitDialog()

	keyStr := msg.String()

	// Esc always cancels regardless of which surface has focus.
	if keyStr == "esc" {
		a.closeSchedulePreviewDialog()
		return a, nil
	}

	if !p.splitFocus {
		// Tab past the header's last focusable element transitions
		// into the split editor instead of wrapping back to field 0.
		if keyStr == "tab" && header.FocusIndex() == header.FocusableCount()-1 {
			p.splitFocus = true
			splits.focus = splitFocusRows
			splits.rowIndex = 0
			splits.fieldFocus = splitFieldCategory
			return a, nil
		}
		action := header.HandleKey(msg)
		// A Date edit reseeds a loan-shaped preview's split from the balance
		// as of the new date, until the user edits a line amount.
		a.maybeReseedLoanPreview()
		return a.schedulePreviewAction(action)
	}

	// Shift+Tab from the split editor's first focus transitions back
	// to the header at field 0.
	if keyStr == "shift+tab" && splits.focus == splitFocusRows && splits.rowIndex == 0 && splits.fieldFocus == splitFieldCategory {
		p.splitFocus = false
		header.SetFocusIndex(0)
		return a, nil
	}

	return a.schedulePreviewSplitAction(splits.HandleKey(msg))
}

// maybeReseedLoanPreview recomputes a loan-shaped preview's interest/principal
// split when the Date field has been edited to a new occurrence date — until
// the user edits a line amount, after which user values win and Date edits no
// longer reseed (the reseed rule in specs/loan-wizard.md). A recompute failure
// (e.g. the loan is already paid off at the new date) leaves the current seed
// in place.
func (a *App) maybeReseedLoanPreview() {
	p := a.schedPreviewDialog
	if p == nil || !p.loanShaped || p.loanSeedFrozen || a.services.Scheduled == nil {
		return
	}
	if p.userEditedLines() {
		p.loanSeedFrozen = true
		return
	}
	header := p.HeaderDialog()
	if header == nil {
		return
	}
	fields := header.Fields()
	if len(fields) <= previewFieldDate {
		return
	}
	newDate, err := parseDateInput(fields[previewFieldDate].Value)
	if err != nil || newDate.Equal(p.loanSeedDate) {
		return
	}
	ls, err := a.services.Scheduled.ComputeLoanSplits(p.template, newDate)
	if err != nil {
		return
	}
	p.reseedLoanSplits(ls, newDate)
}

// freezeLoanSeedIfEdited permanently freezes loan reseeding once the user has
// edited any line amount (or added/removed a row) in a loan-shaped preview.
func (a *App) freezeLoanSeedIfEdited() {
	p := a.schedPreviewDialog
	if p == nil || !p.loanShaped || p.loanSeedFrozen {
		return
	}
	if p.userEditedLines() {
		p.loanSeedFrozen = true
	}
}
