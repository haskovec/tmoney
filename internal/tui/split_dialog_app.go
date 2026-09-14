package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// The split surface: the controller half of the split editor. splitSurface
// owns open, submit and close and the create-category hooks; App supplies the
// services through splitDeps and the divert into the create-category
// sub-dialog, which is a sibling surface's state.
//
// Two parents open this surface — the transaction dialog and the scheduled
// dialog — and the second also owns its own save path, because a scheduled
// save may first ask the user to confirm a demotion. So the surface offers
// submit for the transaction case and takeScheduledSplits for the scheduled
// one, and never reaches into either parent.

// splitDeps is what the split surface needs from outside itself. Every dep is
// a func, because switchDatabase replaces App's services and closes the
// previous *db.DB; and deps are passed to each call, never stored, because
// close() resets the surface to its zero value. Both rules are pinned by
// TestGuard_ControllerDepsAreLiveIndirections and
// TestGuard_NoSurfaceStructHoldsItsDeps.
type splitDeps struct {
	payees       func() *payee.Service
	transactions func() *transaction.Service
	undo         func() *undo.Manager
}

// splitDeps binds the split surface to the services App owns. Every accessor
// may legitimately return nil — an App built by a test has no services — so
// each caller keeps its own nil guard.
func (a *App) splitDeps() splitDeps {
	return splitDeps{
		payees:       func() *payee.Service { return a.services.Payee },
		transactions: func() *transaction.Service { return a.services.Transaction },
		undo:         func() *undo.Manager { return a.undoManager },
	}
}

// splitOpen is what a parent hands the split editor when it opens it: the
// parent amount the rows must sum to, the category picker, the transfer
// targets, and the rows to seed from when the parent already has splits.
type splitOpen struct {
	amount          types.Money
	categoryOptions []string
	categoryIDs     []types.ID
	// seedSplits, when non-empty, pre-populates the rows — the edit case. A
	// parent that is creating passes none.
	seedSplits     []*transaction.Split
	accountOptions []string
	accountIDs     []types.ID
	// accountID is the parent's own account, excluded from the transfer
	// targets so a row cannot transfer to the account it is already in.
	accountID types.ID
}

// openForTransaction builds the editor over a regular transaction's pending
// scalar fields. Submit then saves through the transaction service.
func (s *splitSurface) openForTransaction(pending *pendingSplitTransaction, o splitOpen) {
	*s = splitSurface{pendingTxn: pending, editor: newSplitEditor(o)}
}

// openForSchedule builds the editor over a scheduled transaction's pending
// scalar fields. The scheduled dialog saves it, through takeScheduledSplits.
func (s *splitSurface) openForSchedule(pending *pendingSplitScheduled, o splitOpen) {
	*s = splitSurface{pendingScheduled: pending, editor: newSplitEditor(o)}
}

func newSplitEditor(o splitOpen) *SplitDialog {
	var editor *SplitDialog
	if len(o.seedSplits) > 0 {
		editor = NewSplitDialogFromExisting(o.amount, o.categoryOptions, o.categoryIDs, o.seedSplits)
	} else {
		editor = NewSplitDialog(o.amount, o.categoryOptions, o.categoryIDs)
	}
	editor.SetTransferTargets(o.accountOptions, o.accountIDs, o.accountID)
	return editor
}

// editsSchedule reports whether the open editor belongs to a scheduled
// transaction, whose save the scheduled dialog owns.
func (s *splitSurface) editsSchedule() bool { return s.pendingScheduled != nil }

// handleKey gives the editor the key and reports what it wants done about it.
// An unbuilt surface asks for nothing.
func (s *splitSurface) handleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if s.editor == nil {
		return dialog.DialogActionNone
	}
	return s.editor.HandleKey(msg)
}

// handleMouse translates a click into the editor's action. The coordinate
// transform reconstructs what app_view composites — the rendered panel centred
// with widget.OverlayCenter — so a click maps to the same content-local
// position the renderer drew. Wheel events and other buttons are ignored; the
// editor has no scroll surface.
func (s *splitSurface) handleMouse(msg tea.MouseMsg, styles widget.Styles, screenWidth, screenHeight int) dialog.DialogAction {
	if s.editor == nil {
		return dialog.DialogActionNone
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return dialog.DialogActionNone
	}
	overlay := s.editor.Render(styles)
	startCol, startRow := widget.OverlayTopLeft(overlay, screenWidth, screenHeight)
	m := msg.Mouse()
	// Content-local offsets inside the panel: border (1) + h-padding (2) on X,
	// border (1) + v-padding (1) on Y.
	return s.editor.HandleMouseLocal(m.X-startCol-3, m.Y-startRow-2)
}

// close clears the surface. The zero value is the closed state, so this is the
// whole of it — and it stays that way only because the deps are a call
// parameter rather than a field.
func (s *splitSurface) close() { *s = splitSurface{} }

// submit validates the rows and returns the command that saves the regular
// transaction they belong to, creating it or replacing an existing one's
// splits in one undo unit. A nil command means validation failed: the editor
// stays open carrying its inline error. On success the surface is closed
// before the command runs. A surface opened for a schedule has no transaction
// to save; its parent calls takeScheduledSplits instead.
func (s *splitSurface) submit(deps splitDeps) tea.Cmd {
	if s.editor == nil || s.pendingTxn == nil {
		return nil
	}

	splits, err := s.editor.buildSplits()
	if err != nil {
		s.editor.errorMsg = err.Error()
		return nil
	}

	pending := s.pendingTxn
	s.close()

	return func() tea.Msg {
		// Resolve or create payee
		var payeeID types.ID
		if payeeSvc := deps.payees(); pending.payeeName != "" && payeeSvc != nil {
			payee, _, err := payeeSvc.GetOrCreate(pending.payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = payee.ID
		}

		txnSvc, undoMgr := deps.transactions(), deps.undo()

		if pending.existing != nil {
			// Edit mode — update parent and replace splits in one undo unit.
			updated := *pending.existing
			updated.Date = pending.date
			updated.Amount = pending.amount
			updated.Status = pending.status
			updated.SetMemo(pending.memo)
			if !payeeID.IsNil() {
				updated.PayeeID = types.NullableID{ID: payeeID, Valid: true}
			} else {
				updated.PayeeID = types.NullableID{Valid: false}
			}
			// A split transaction has no parent-level category.
			updated.CategoryID = types.NullableID{Valid: false}
			// Re-stamp split transaction_id to the parent.
			for _, sp := range splits {
				sp.TransactionID = updated.ID
			}

			if txnSvc != nil && undoMgr != nil {
				cmd := undo.NewEditTransactionWithSplitsCommand(txnSvc, &updated, splits)
				if err := undoMgr.Execute(cmd); err != nil {
					return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
				}
			}
			return splitDialogSavedMsg{savedID: updated.ID}
		}

		// Build transaction (no category when using splits)
		txn := transaction.NewTransactionFull(pending.accountID, pending.date, pending.amount, payeeID, types.NilID, pending.memo)
		txn.Status = pending.status

		// Save with splits via undo manager
		if txnSvc != nil && undoMgr != nil {
			cmd := undo.NewCreateTransactionWithSplitsCommand(txnSvc, txn, splits)
			if err := undoMgr.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
			}
		}

		return splitDialogSavedMsg{savedID: txn.ID}
	}
}

// takeScheduledSplits validates the rows on behalf of the scheduled dialog and
// hands back the pending schedule with the rows it should save. ok is false
// when nothing is open for a schedule or validation failed; in the second case
// the editor stays open carrying its inline error. On success the surface is
// closed: what follows — a demotion confirm, then the save — is the scheduled
// dialog's, and the editor has no further part in it.
func (s *splitSurface) takeScheduledSplits() (pending *pendingSplitScheduled, splits []*transaction.Split, ok bool) {
	if s.editor == nil || s.pendingScheduled == nil {
		return nil, nil, false
	}
	splits, err := s.editor.buildSplits()
	if err != nil {
		s.editor.errorMsg = err.Error()
		return nil, nil, false
	}
	pending = s.pendingScheduled
	s.close()
	return pending, splits, true
}

// beginCreateCategory hides the editor and reports the focused row, whose
// Category field activated the [+ Add new category…] sentinel, with the
// category type its amount implies. The editor is kept alive (hidden) so its
// row state survives the divert; applyCreatedCategory re-shows it with the new
// category selected, reshow does so after a cancel. It reports false when no
// editor is open, in which case nothing was hidden and the caller must not
// open the sub-dialog.
//
// Unlike the typeahead-combo surfaces, the editor has no typed query to
// harvest — the sub-dialog opens with empty Name and Parent fields.
func (s *splitSurface) beginCreateCategory() (row int, defaultType category.Type, ok bool) {
	if s.editor == nil {
		return -1, category.TypeExpense, false
	}
	row = s.editor.rowIndex
	defaultType = category.TypeExpense
	if row >= 0 && row < len(s.editor.rows) {
		defaultType = inferCategoryTypeFromAmount(s.editor.rows[row].amountField.Value)
	}
	s.editor.SetVisible(false)
	return row, defaultType, true
}

// applyCreatedCategory rebuilds the editor's category option slices to include
// the freshly-persisted category, points the originating row at it, and
// re-shows the editor. Other rows' selections are preserved by looking up
// their previously-selected category ID in the rebuilt slice. Persistence
// already happened in persistCategory; the router passes the fresh category
// in. The persist is asynchronous, so a surface the user has closed in the
// meantime is left alone.
func (s *splitSurface) applyCreatedCategory(newCat *category.Category, cats []*category.Category, originRow int) {
	sd := s.editor
	if sd == nil {
		return
	}

	options, ids := buildCategoryOptions(cats)

	// Build a map from old categoryID to new index so non-originating
	// rows preserve their selection by identity, not by stale index.
	idToNewIdx := make(map[types.ID]int, len(ids))
	for i, id := range ids {
		idToNewIdx[id] = i
	}

	// Snapshot each row's selected categoryID before swapping slices.
	priorIDs := make([]types.ID, len(sd.rows))
	for i, row := range sd.rows {
		switch {
		case row.transferMode:
			priorIDs[i] = types.NilID
		case row.categoryIndex >= 0 && row.categoryIndex < len(sd.categoryIDs):
			priorIDs[i] = sd.categoryIDs[row.categoryIndex]
		default:
			priorIDs[i] = types.NilID
		}
	}

	sd.categoryOptions = options
	sd.categoryIDs = ids

	// Re-map non-originating rows to their preserved category by ID.
	for i := range sd.rows {
		if i == originRow {
			continue
		}
		if sd.rows[i].transferMode {
			continue
		}
		if newIdx, ok := idToNewIdx[priorIDs[i]]; ok {
			sd.rows[i].categoryIndex = newIdx
		} else {
			sd.rows[i].categoryIndex = 0 // (None)
		}
	}

	// Point the originating row at the new category.
	if originRow >= 0 && originRow < len(sd.rows) {
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		sd.rows[originRow].transferMode = false
		sd.rows[originRow].categoryIndex = newIdx
	}

	sd.SetVisible(true)
}

// reshow makes the editor visible again after a create-category divert the
// user cancelled. Nil-safe, because the divert can outlive the surface.
func (s *splitSurface) reshow() {
	if s.editor != nil {
		s.editor.SetVisible(true)
	}
}

// -----------------------------------------------------------------------------
// App glue. Everything below is a thin wrapper supplying the services, the
// screen geometry, and the divert into a sibling surface. No behaviour lives
// here: an action the surface owns must not have a second implementation on
// App.
// -----------------------------------------------------------------------------

// handleSplitDialogKey routes key events to the split dialog.
func (a *App) handleSplitDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.splitDialogAction(a.split.handleKey(msg))
}

// splitDialogAction dispatches a DialogAction for the split dialog. Both the
// keyboard and the mouse path call it, so clicking a button is exactly
// equivalent to the keyboard action -- the rule specs/tui.md states and the
// two hand-kept switches used to break.
//
// AddNew is the one arm that stays on App: it writes createCat, which is
// another surface's state, and a surface must not reach into a sibling. The
// scheduled Submit arm stays too, because that save belongs to the scheduled
// dialog.
func (a *App) splitDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		if a.split.editsSchedule() {
			return a.submitScheduledSplitDialog()
		}
		return a, a.split.submit(a.splitDeps())
	case dialog.DialogActionCancel:
		a.split.close()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSplit()
	}

	return a, nil
}

// handleSplitDialogMouse routes a mouse event through the split editor and
// translates the resulting action, mirroring handleSplitDialogKey. Without
// this the editor was keyboard-only: handleDialogMouse swallowed every click,
// so the Save button did not respond to the mouse even though SplitDialog has
// hit testing for it.
func (a *App) handleSplitDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	return a.splitDialogAction(a.split.handleMouse(msg, a.styles, a.width, a.height))
}

// openCreateCategorySubDialogFromSplit hides the split dialog and opens the
// inline create-category sub-dialog for the currently-focused row's Category
// field. Restoration on cancel and post-create wiring happen through the
// createCatDialog handlers.
func (a *App) openCreateCategorySubDialogFromSplit() (tea.Model, tea.Cmd) {
	row, defaultType, ok := a.split.beginCreateCategory()
	if !ok {
		return a, nil
	}
	a.createCat.origin.surface = createCatSourceSplitDialog
	a.createCat.origin.splitRow = row
	parents := a.parentsForCreateCatDialog()
	a.createCat.dlg = buildCreateCategoryDialog("", "", parents, defaultType)
	return a, nil
}

// applyCreatedCategoryToSplit is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the split
// dialog. The surface applies the category to its own rows; App clears the
// sub-dialog and the originating-row handle, which are its own state.
func (a *App) applyCreatedCategoryToSplit(newCat *category.Category, cats []*category.Category) {
	a.split.applyCreatedCategory(newCat, cats, a.createCat.origin.splitRow)
	a.createCat.dlg = nil
	a.createCat.origin.splitRow = -1
}
