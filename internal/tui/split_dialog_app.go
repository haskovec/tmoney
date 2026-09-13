package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// App integration: the key, action and mouse dispatchers the modal registry
// calls, submit, close, and the create-category divert's opener and applier.

// handleSplitDialogKey routes key events to the split dialog.
func (a *App) handleSplitDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.split.editor == nil {
		return a, nil
	}
	return a.splitDialogAction(a.split.editor.HandleKey(msg))
}

// splitDialogAction dispatches a DialogAction for the split dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) splitDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		if a.split.pendingScheduled != nil {
			return a.submitScheduledSplitDialog()
		}
		return a.submitSplitDialog()
	case dialog.DialogActionCancel:
		a.closeSplitDialog()
		return a, nil
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSplit()
	}

	return a, nil
}

// handleSplitDialogMouse routes a mouse event through the split editor and
// translates the resulting action, mirroring handleSplitDialogKey. Without this
// the editor was keyboard-only: handleDialogMouse swallowed every click, so the
// Save button did not respond to the mouse even though SplitDialog has hit
// testing for it (HandleMouseLocal, written for the post-time preview).
//
// The coordinate transform reconstructs what app_view composites — the rendered
// panel centred with widget.OverlayCenter — so a click maps to the same
// content-local position the renderer drew.
func (a *App) handleSplitDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.split.editor == nil {
		return a, nil
	}
	// Wheel events reach here too (handleMouseWheel routes through
	// handleDialogMouse); the editor has no scroll surface, so ignore them.
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}

	overlay := a.split.editor.Render(a.styles)
	startCol, startRow := widget.OverlayTopLeft(overlay, a.width, a.height)
	m := msg.Mouse()
	// Content-local offsets inside the panel: border (1) + h-padding (2) on X,
	// border (1) + v-padding (1) on Y.
	// The same dispatcher the keyboard path uses, so a click and a keypress
	// cannot drift apart.
	return a.splitDialogAction(a.split.editor.HandleMouseLocal(m.X-startCol-3, m.Y-startRow-2))
}

// openCreateCategorySubDialogFromSplit hides the split dialog and opens the
// inline create-category sub-dialog for the currently-focused row's
// Category field. The split dialog's row state (other rows, amount fields,
// etc.) is preserved by keeping the dialog alive (just hidden) for the
// duration of the divert; restoration on cancel and post-create wiring
// happen through the createCatDialog handlers.
//
// Unlike the typeahead-combo surfaces, the split dialog has no typed query
// to harvest — the sub-dialog opens with empty Name and Parent fields.
func (a *App) openCreateCategorySubDialogFromSplit() (tea.Model, tea.Cmd) {
	if a.split.editor == nil {
		return a, nil
	}

	a.createCat.origin.surface = createCatSourceSplitDialog
	a.createCat.origin.splitRow = a.split.editor.rowIndex
	parents := a.parentsForCreateCatDialog()
	defaultType := category.TypeExpense
	if rowIdx := a.split.editor.rowIndex; rowIdx >= 0 && rowIdx < len(a.split.editor.rows) {
		defaultType = inferCategoryTypeFromAmount(a.split.editor.rows[rowIdx].amountField.Value)
	}
	a.createCat.dlg = buildCreateCategoryDialog("", "", parents, defaultType)
	a.split.editor.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSplit is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the
// split dialog. It rebuilds the dialog's category option slices to include
// the freshly-persisted category, points the originating row at it, and
// re-shows the split dialog. Other rows' selections are preserved by
// looking up their previously-selected category ID in the rebuilt slice.
func (a *App) applyCreatedCategoryToSplit(newCat *category.Category, cats []*category.Category) {
	defer func() {
		a.createCat.dlg = nil
		a.createCat.origin.splitRow = -1
	}()
	sd := a.split.editor
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
		if i == a.createCat.origin.splitRow {
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
	if a.createCat.origin.splitRow >= 0 && a.createCat.origin.splitRow < len(sd.rows) {
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		sd.rows[a.createCat.origin.splitRow].transferMode = false
		sd.rows[a.createCat.origin.splitRow].categoryIndex = newIdx
	}

	sd.SetVisible(true)
}

// submitSplitDialog validates splits, builds the transaction, and saves it.
// When the pending state carries an existing transaction (edit mode) the
// flow dispatches EditTransactionWithSplitsCommand against that ID; otherwise
// it dispatches CreateTransactionWithSplitsCommand for a new transaction.
func (a *App) submitSplitDialog() (tea.Model, tea.Cmd) {
	if a.split.editor == nil || a.split.pendingTxn == nil {
		return a, nil
	}

	splits, err := a.split.editor.buildSplits()
	if err != nil {
		a.split.editor.errorMsg = err.Error()
		return a, nil
	}

	pending := a.split.pendingTxn
	a.closeSplitDialog()

	return a, func() tea.Msg {
		// Resolve or create payee
		var payeeID types.ID
		if pending.payeeName != "" && a.services.Payee != nil {
			payee, _, err := a.services.Payee.GetOrCreate(pending.payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = payee.ID
		}

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
			for _, s := range splits {
				s.TransactionID = updated.ID
			}

			if a.services.Transaction != nil && a.undoManager != nil {
				cmd := undo.NewEditTransactionWithSplitsCommand(a.services.Transaction, &updated, splits)
				if err := a.undoManager.Execute(cmd); err != nil {
					return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
				}
			}
			return splitDialogSavedMsg{savedID: updated.ID}
		}

		// Build transaction (no category when using splits)
		txn := transaction.NewTransactionFull(pending.accountID, pending.date, pending.amount, payeeID, types.NilID, pending.memo)
		txn.Status = pending.status

		// Save with splits via undo manager
		if a.services.Transaction != nil && a.undoManager != nil {
			cmd := undo.NewCreateTransactionWithSplitsCommand(a.services.Transaction, txn, splits)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
			}
		}

		return splitDialogSavedMsg{savedID: txn.ID}
	}
}

// closeSplitDialog clears the split dialog state.
func (a *App) closeSplitDialog() {
	a.split = splitSurface{}
}
