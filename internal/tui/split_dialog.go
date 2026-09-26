package tui

import (
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// transferSentinelLabel prefixes the transfer entries in a split row's
// category picker ("Transfer → Savings"). Picking one makes the row a
// transfer-line targeting that account. See
// specs/multiline-splits-and-paycheck.md ("Display").
const transferSentinelLabel = "Transfer →"

// addNewSentinelLabel is the action row at the bottom of a split row's
// category picker, the same as on the other typeahead combos. Enter on it
// returns dialog.DialogActionAddNew so the App-level router can divert into
// the inline create-category sub-dialog.
const addNewSentinelLabel = "[+ Add new category…]"

// splitDialogFocus indicates which top-level area of the split dialog has focus.
type splitDialogFocus int

const (
	splitFocusRows splitDialogFocus = iota
	splitFocusAddBtn
	splitFocusCancelBtn
	splitFocusSaveBtn
)

// splitFieldFocus indicates which field within a row has focus.
type splitFieldFocus int

const (
	splitFieldCategory splitFieldFocus = iota
	splitFieldAmount
	splitFieldMemo
)

// splitRow holds the data for one split entry row.
//
// A row is in one of two modes:
//   - Category mode (transferMode = false): categoryIndex picks an entry
//     from SplitDialog.categoryOptions.
//   - Transfer mode (transferMode = true): accountIndex picks an entry
//     from the dialog's transfer-account options (parent account already
//     filtered out).
//
// The category picker offers both in one list (see pickerOptions), so a
// pick sets the mode.
type splitRow struct {
	categoryIndex int
	transferMode  bool
	accountIndex  int
	amountField   dialog.Field
	memoField     dialog.Field

	// seedTransferAccountID remembers the transfer destination when a row
	// is seeded from an existing transfer-split (NewSplitDialogFromExisting)
	// before SetTransferTargets has supplied the account list.
	// SetTransferTargets resolves it to accountIndex and clears it.
	seedTransferAccountID types.NullableID

	// seedTransferCategoryID carries the category of a seeded transfer-line
	// (a "categorized transfer", e.g. a loan payment's principal line) through
	// the dialog unchanged. The dialog offers no picker for it (v1 non-goal),
	// so buildSplits re-emits it as-is; freshly-created transfer rows leave it
	// NilID (no category).
	seedTransferCategoryID types.ID

	// seedPaycheckSection carries a wizard-tagged line's paycheck_section
	// through this editor unchanged, so editing an amount here does not
	// demote a paycheck schedule to a generic one. The dialog offers no
	// picker for it; buildSplits re-emits it as-is. A row added here leaves
	// it NULL, which hides the Edit-as-paycheck affordance until the
	// schedule is saved through the wizard again — the behaviour
	// specs/multiline-splits-and-paycheck.md specifies.
	seedPaycheckSection types.NullableString
}

// pendingSplitTransaction holds the transaction data while the split editor is open.
type pendingSplitTransaction struct {
	accountID types.ID
	date      types.Date
	payeeName string
	amount    types.Money
	memo      string
	status    transaction.Status

	// existing is non-nil when the user is editing a transaction (vs.
	// creating a new one). The split dialog's submit dispatches
	// EditTransactionWithSplitsCommand instead of CreateTransactionWithSplits
	// when this is set.
	existing *transaction.Transaction
}

// splitDialogSavedMsg is sent when a split transaction has been saved.
// savedID is the ID of the saved parent transaction so the register can move
// the cursor onto its row after reload.
type splitDialogSavedMsg struct {
	savedID types.ID
}

// SplitDialog is a custom dialog for editing split transaction entries.
type SplitDialog struct {
	visible         bool
	width           int
	totalAmount     types.Money
	rows            []splitRow
	focus           splitDialogFocus
	rowIndex        int
	fieldFocus      splitFieldFocus
	categoryOptions []string
	categoryIDs     []types.ID

	// Transfer-line picker (configured via SetTransferTargets). When
	// transferAccountIDs is empty, the Transfer sentinel is non-functional
	// and validate() rejects rows that land on it.
	transferAccountOptions []string
	transferAccountIDs     []types.ID

	// picker is the type-ahead combo for the focused row's Category cell.
	// The row's categoryIndex / transferMode / accountIndex stay the model:
	// focusedPicker loads the picker from them when the cell takes focus,
	// and commitPicker writes the pick back and closes it. pickerRow is the
	// row it was loaded for; pickerOpen is false while no cell has it.
	picker     dialog.Field
	pickerRow  int
	pickerOpen bool

	// blurred is true while a host that embeds the editor (the scheduled
	// preview) has focus on its own fields. A blurred editor draws no
	// focused cell and no open picker. See SetBlurred.
	blurred bool

	errorMsg string
}

// NewSplitDialog creates a new SplitDialog for the given total amount.
func NewSplitDialog(amount types.Money, categoryOptions []string, categoryIDs []types.ID) *SplitDialog {
	sd := &SplitDialog{
		visible:         true,
		width:           64,
		totalAmount:     amount,
		focus:           splitFocusRows,
		rowIndex:        0,
		fieldFocus:      splitFieldCategory,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
	}
	sd.addRow()
	return sd
}

// NewSplitDialogFromExisting creates a SplitDialog seeded with one row per
// existing split. Each row's category is resolved against the parallel
// categoryIDs slice (rows whose category is unknown to the dialog land at
// index 0, "(None)"); amount and memo are pre-filled from the split.
func NewSplitDialogFromExisting(amount types.Money, categoryOptions []string, categoryIDs []types.ID, existing []*transaction.Split) *SplitDialog {
	sd := &SplitDialog{
		visible:         true,
		width:           64,
		totalAmount:     amount,
		focus:           splitFocusRows,
		rowIndex:        0,
		fieldFocus:      splitFieldCategory,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
	}
	if len(existing) == 0 {
		sd.addRow()
		return sd
	}
	for _, s := range existing {
		memo := ""
		if s.Memo.Valid {
			memo = s.Memo.String
		}
		// prefillField, not a Value: field literal, so Backspace works and a
		// typed character appends instead of prepending.
		amountField := dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "0.00",
			Width:       12,
		}
		prefillField(&amountField, s.Amount.String())
		memoField := dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "Memo",
		}
		prefillField(&memoField, memo)

		// A transfer-split (TransferAccountID set) seeds a transfer-mode row.
		// The account list isn't known until SetTransferTargets, so stash the
		// destination ID for it to resolve into an accountIndex. An existing
		// category (a categorized transfer) is carried through unchanged.
		if s.TransferAccountID.Valid {
			sd.rows = append(sd.rows, splitRow{
				transferMode:           true,
				seedTransferAccountID:  s.TransferAccountID,
				seedTransferCategoryID: s.CategoryID,
				seedPaycheckSection:    s.PaycheckSection,
				amountField:            amountField,
				memoField:              memoField,
			})
			continue
		}

		catIdx := 0
		for i, id := range categoryIDs {
			if id == s.CategoryID {
				catIdx = i
				break
			}
		}
		sd.rows = append(sd.rows, splitRow{
			categoryIndex:       catIdx,
			seedPaycheckSection: s.PaycheckSection,
			amountField:         amountField,
			memoField:           memoField,
		})
	}
	return sd
}

// IsVisible returns whether the split dialog is currently shown.
// Nil-safe so the modal registry can walk an unbuilt surface — see the note on
// dialog.Dialog.IsVisible.
func (sd *SplitDialog) IsVisible() bool {
	return sd != nil && sd.visible
}

// SetVisible toggles the rendered state of the split dialog. Used when an
// inline sub-dialog (e.g. the create-category dialog) needs to overlay it
// while keeping the instance alive so row state survives the divert.
func (sd *SplitDialog) SetVisible(v bool) {
	sd.visible = v
}

// Rows returns the split rows.
func (sd *SplitDialog) Rows() []splitRow {
	return sd.rows
}

// Focus returns the current top-level focus area.
func (sd *SplitDialog) Focus() splitDialogFocus {
	return sd.focus
}

// RowIndex returns the currently focused row index.
func (sd *SplitDialog) RowIndex() int {
	return sd.rowIndex
}

// FieldFocus returns the currently focused field within a row.
func (sd *SplitDialog) FieldFocus() splitFieldFocus {
	return sd.fieldFocus
}

// pickerOptions returns the list the Category picker filters: every
// category, then one "Transfer → <account>" entry for each transfer target.
// An index below len(categoryOptions) is a category; the rest map to
// transferAccountOptions in order.
func (sd *SplitDialog) pickerOptions() []string {
	opts := make([]string, 0, len(sd.categoryOptions)+len(sd.transferAccountOptions))
	opts = append(opts, sd.categoryOptions...)
	for _, name := range sd.transferAccountOptions {
		opts = append(opts, transferSentinelLabel+" "+name)
	}
	return opts
}

// pickerIndex returns row's selection as an index into pickerOptions.
func (sd *SplitDialog) pickerIndex(row splitRow) int {
	if row.transferMode {
		return len(sd.categoryOptions) + row.accountIndex
	}
	return row.categoryIndex
}

// rowLabel returns the text row's Category cell shows while unfocused.
func (sd *SplitDialog) rowLabel(row splitRow) string {
	if row.transferMode {
		return transferSentinelLabel + " " + sd.transferAccountLabel(row.accountIndex)
	}
	if row.categoryIndex >= 0 && row.categoryIndex < len(sd.categoryOptions) {
		return sd.categoryOptions[row.categoryIndex]
	}
	return ""
}

// SetBlurred tells the editor whether its host has moved focus away from it.
// Blurring commits an open picker first, the same as Tab, so the entry the
// panel showed is the one kept.
func (sd *SplitDialog) SetBlurred(b bool) {
	if b {
		sd.commitPicker()
	}
	sd.blurred = b
}

// focusedPicker returns the Category picker when a row's Category cell has
// focus, loading it from that row on first use; otherwise nil.
func (sd *SplitDialog) focusedPicker() *dialog.Field {
	if sd.blurred || sd.focus != splitFocusRows || sd.fieldFocus != splitFieldCategory ||
		sd.rowIndex < 0 || sd.rowIndex >= len(sd.rows) {
		return nil
	}
	if !sd.pickerOpen || sd.pickerRow != sd.rowIndex {
		idx := sd.pickerIndex(sd.rows[sd.rowIndex])
		sd.picker = dialog.Field{
			Type:           dialog.FieldCombo,
			Options:        sd.pickerOptions(),
			SelectedIndex:  idx,
			ComboHighlight: idx,
			AddNewLabel:    addNewSentinelLabel,
		}
		sd.pickerRow, sd.pickerOpen = sd.rowIndex, true
	}
	return &sd.picker
}

// commitPicker commits the picker's highlighted entry, writes it back to the
// row it was loaded for, and closes the picker. No-op while it is closed.
// Every path that moves focus off a Category cell calls it first, the same
// as Tab on a Dialog combo.
func (sd *SplitDialog) commitPicker() {
	if !sd.pickerOpen {
		return
	}
	sd.pickerOpen = false
	if sd.pickerRow < 0 || sd.pickerRow >= len(sd.rows) {
		return
	}
	sd.picker.CommitComboHighlight()
	row := &sd.rows[sd.pickerRow]
	idx := sd.picker.SelectedIndex
	if n := len(sd.categoryOptions); idx < n {
		row.transferMode = false
		row.categoryIndex = idx
		row.accountIndex = 0
	} else {
		row.transferMode = true
		row.accountIndex = idx - n
	}
}

// SetTransferTargets configures the account picker used when a split
// row enters transfer mode. Any account whose ID equals
// excludeAccountID is filtered out so users cannot self-transfer (the
// usual exclusion is the parent transaction's account). Until this is
// called with at least one remaining account, the Transfer sentinel
// stays inert and validate() rejects rows that land on it.
func (sd *SplitDialog) SetTransferTargets(options []string, ids []types.ID, excludeAccountID types.ID) {
	if len(options) != len(ids) {
		return
	}
	sd.transferAccountOptions = sd.transferAccountOptions[:0]
	sd.transferAccountIDs = sd.transferAccountIDs[:0]
	for i, id := range ids {
		if id == excludeAccountID {
			continue
		}
		sd.transferAccountOptions = append(sd.transferAccountOptions, options[i])
		sd.transferAccountIDs = append(sd.transferAccountIDs, id)
	}
	sd.pickerOpen = false // its options changed; reload on next use

	// Resolve rows seeded from existing transfer-splits to their index in
	// the now-known transfer-target list.
	for i := range sd.rows {
		row := &sd.rows[i]
		if !row.transferMode || !row.seedTransferAccountID.Valid {
			continue
		}
		found := false
		for j, id := range sd.transferAccountIDs {
			if id == row.seedTransferAccountID.ID {
				row.accountIndex = j
				found = true
				break
			}
		}
		if !found {
			// Destination unavailable (excluded as the parent account or
			// since deleted): fall back to a category row so the dialog
			// stays consistent rather than silently retargeting the cash.
			row.transferMode = false
			row.accountIndex = 0
		}
		row.seedTransferAccountID = types.NullableID{}
	}
}

// transferAccountLabel returns the display label for the account at the
// given index in the transfer-target list, or "" if the index is out of
// range.
func (sd *SplitDialog) transferAccountLabel(idx int) string {
	if idx < 0 || idx >= len(sd.transferAccountOptions) {
		return ""
	}
	return sd.transferAccountOptions[idx]
}

// ErrorMsg returns the current error message.
func (sd *SplitDialog) ErrorMsg() string {
	return sd.errorMsg
}

// IsSaveEnabled reports whether the Save button is in an actionable
// state. Per MS-013, Save is disabled whenever the signed sum of line
// amounts does not equal the parent transaction's amount; users must
// manually adjust a line to absorb the difference rather than relying
// on an auto-balancing plug.
func (sd *SplitDialog) IsSaveEnabled() bool {
	return sd.remaining().IsZero()
}

// splitSurface is the split editor and the transaction it is editing on
// behalf of. The editor is not a dialog.Dialog, so the surface does not embed
// modalSurface: it forwards Modal to the editor itself. The zero value is
// closed.
type splitSurface struct {
	editor *SplitDialog
	// pendingTxn is the transaction whose splits are being edited (nil when
	// the editor belongs to a scheduled transaction instead).
	pendingTxn *pendingSplitTransaction
	// pendingScheduled is the scheduled transaction whose splits are being
	// edited (nil when the editor belongs to a regular transaction instead).
	pendingScheduled *pendingSplitScheduled
}

func (s *splitSurface) IsVisible() bool { return s != nil && s.editor.IsVisible() }

func (s *splitSurface) Render(styles widget.Styles) string { return s.editor.Render(styles) }
