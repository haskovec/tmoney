package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transferSentinelLabel is the trailing option in a split row's
// category combo that swaps the row from a categorized line into a
// transfer-line targeting another account. See
// specs/multiline-splits-and-paycheck.md ("Display").
const transferSentinelLabel = "Transfer →"

// addNewSentinelLabel is the bottom-most option in a split row's category
// picker, mirroring the [+ Add new category…] action row on typeahead
// combos in other transaction-entry surfaces. Activating it (Enter on the
// Category field while parked here) returns DialogActionAddNew so the
// App-level router can divert into the inline create-category sub-dialog.
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
//     from SplitDialog.categoryOptions. Landing on the trailing Transfer
//     sentinel (with the picker configured) auto-swaps to transfer mode.
//   - Transfer mode (transferMode = true): accountIndex picks an entry
//     from the dialog's transfer-account options (parent account already
//     filtered out). Up from accountIndex 0 reverts to category mode.
type splitRow struct {
	categoryIndex int
	transferMode  bool
	accountIndex  int
	amountField   Field
	memoField     Field
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
type splitDialogSavedMsg struct{}

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
		catIdx := 0
		for i, id := range categoryIDs {
			if id == s.CategoryID {
				catIdx = i
				break
			}
		}
		memo := ""
		if s.Memo.Valid {
			memo = s.Memo.String
		}
		sd.rows = append(sd.rows, splitRow{
			categoryIndex: catIdx,
			amountField: Field{
				Type:        FieldText,
				Value:       s.Amount.String(),
				Placeholder: "0.00",
				Width:       12,
			},
			memoField: Field{
				Type:        FieldText,
				Value:       memo,
				Placeholder: "Memo",
			},
		})
	}
	return sd
}

// IsVisible returns whether the split dialog is currently shown.
func (sd *SplitDialog) IsVisible() bool {
	return sd.visible
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

// categoryOptionCount returns the total number of selectable items in a
// row's category combo, including the trailing Transfer and AddNew
// sentinels. Index layout: real categories at [0..N-1], Transfer at N,
// AddNew at N+1.
func (sd *SplitDialog) categoryOptionCount() int {
	return len(sd.categoryOptions) + 2
}

// isTransferSentinel reports whether the given option index points
// at the trailing Transfer → row appended past the real categories.
func (sd *SplitDialog) isTransferSentinel(idx int) bool {
	return idx == len(sd.categoryOptions)
}

// isAddNewSentinel reports whether the given option index points at the
// [+ Add new category…] action row appended after the Transfer sentinel.
func (sd *SplitDialog) isAddNewSentinel(idx int) bool {
	return idx == len(sd.categoryOptions)+1
}

// categoryOptionLabel returns the display label for the given option
// index, mapping the Transfer and AddNew sentinel positions to their
// constant labels.
func (sd *SplitDialog) categoryOptionLabel(idx int) string {
	if sd.isAddNewSentinel(idx) {
		return addNewSentinelLabel
	}
	if sd.isTransferSentinel(idx) {
		return transferSentinelLabel
	}
	return sd.categoryOptions[idx]
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

// hasTransferTargets reports whether the dialog has at least one
// account configured to use as a transfer target.
func (sd *SplitDialog) hasTransferTargets() bool {
	return len(sd.transferAccountIDs) > 0
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

// remaining calculates totalAmount minus the sum of entered split amounts.
func (sd *SplitDialog) remaining() types.Money {
	sum := types.ZeroMoney
	for _, row := range sd.rows {
		amt := strings.TrimSpace(row.amountField.Value)
		if amt == "" {
			continue
		}
		m, err := parseAmountInput(amt)
		if err != nil {
			continue
		}
		sum = sum.Add(m)
	}
	return sd.totalAmount.Sub(sum)
}

// addRow appends a new empty split row.
func (sd *SplitDialog) addRow() {
	sd.rows = append(sd.rows, splitRow{
		categoryIndex: 0,
		amountField: Field{
			Type:        FieldText,
			Placeholder: "0.00",
			Width:       12,
		},
		memoField: Field{
			Type:        FieldText,
			Placeholder: "Memo",
		},
	})
}

// removeRow removes the row at the given index.
func (sd *SplitDialog) removeRow(index int) {
	if len(sd.rows) <= 1 || index < 0 || index >= len(sd.rows) {
		return
	}
	sd.rows = append(sd.rows[:index], sd.rows[index+1:]...)
	// Adjust focused row if needed
	if sd.rowIndex >= len(sd.rows) {
		sd.rowIndex = len(sd.rows) - 1
	}
}

// validate checks that all splits are valid and sum to the total.
func (sd *SplitDialog) validate() error {
	if len(sd.rows) == 0 {
		return fmt.Errorf("at least one split is required")
	}

	for i, row := range sd.rows {
		if row.transferMode {
			if row.accountIndex < 0 || row.accountIndex >= len(sd.transferAccountIDs) {
				return fmt.Errorf("split %d: pick a destination account for the transfer", i+1)
			}
		} else {
			// Category must be selected (index > 0 means not "(None)")
			if row.categoryIndex <= 0 {
				return fmt.Errorf("split %d: category is required", i+1)
			}
			// The AddNew sentinel is an action row, not a saveable
			// selection — landing here without activating it (Enter on the
			// Category field) is treated as "no category picked".
			if sd.isAddNewSentinel(row.categoryIndex) {
				return fmt.Errorf("split %d: category is required", i+1)
			}
			// The Transfer sentinel without a configured picker is not a
			// savable selection on its own. Once SetTransferTargets has
			// been called, hitting the sentinel transitions the row into
			// transfer mode (handled above).
			if sd.isTransferSentinel(row.categoryIndex) {
				return fmt.Errorf("split %d: pick a destination account for the transfer", i+1)
			}
		}

		// Amount must be present and valid
		amt := strings.TrimSpace(row.amountField.Value)
		if amt == "" {
			return fmt.Errorf("split %d: amount is required", i+1)
		}
		if _, err := parseAmountInput(amt); err != nil {
			return fmt.Errorf("split %d: invalid amount", i+1)
		}
	}

	// Check sum matches total
	rem := sd.remaining()
	if !rem.IsZero() {
		return fmt.Errorf("splits must sum to %s (remaining: %s)",
			formatDashboardMoney(sd.totalAmount), formatDashboardMoney(rem))
	}

	return nil
}

// buildSplits produces Split structs from the current rows.
//
// Transfer-line rows produce a Split with CategoryID=NilID and
// TransferAccountID set to the picked account; TransferID is left
// empty because the service layer mints it when the parent transaction
// is created (see MS-006).
func (sd *SplitDialog) buildSplits() ([]*transaction.Split, error) {
	if err := sd.validate(); err != nil {
		return nil, err
	}

	var splits []*transaction.Split
	for _, row := range sd.rows {
		amount, _ := parseAmountInput(row.amountField.Value)
		memo := strings.TrimSpace(row.memoField.Value)

		var split *transaction.Split
		if row.transferMode {
			split = &transaction.Split{
				BaseModel:     types.NewBaseModel(),
				TransactionID: types.NilID,
				CategoryID:    types.NilID,
				Amount:        amount,
				TransferAccountID: types.NullableID{
					ID:    sd.transferAccountIDs[row.accountIndex],
					Valid: true,
				},
			}
			if memo != "" {
				split.SetMemo(memo)
			}
		} else {
			categoryID := sd.categoryIDs[row.categoryIndex]
			if memo != "" {
				split = transaction.NewSplitWithMemo(types.NilID, categoryID, amount, memo)
			} else {
				split = transaction.NewSplit(types.NilID, categoryID, amount)
			}
		}
		splits = append(splits, split)
	}

	return splits, nil
}

// HandleKey processes a key event and returns the resulting action.
func (sd *SplitDialog) HandleKey(msg tea.KeyPressMsg) DialogAction {
	switch msg.String() {
	case "esc":
		return DialogActionCancel
	case "ctrl+d":
		if sd.focus == splitFocusRows && len(sd.rows) > 1 {
			sd.removeRow(sd.rowIndex)
			sd.errorMsg = ""
		}
		return DialogActionNone
	case "tab":
		sd.focusNext()
		return DialogActionNone
	case "shift+tab":
		sd.focusPrev()
		return DialogActionNone
	case "enter":
		return sd.handleEnter()
	}

	// Field-specific input when focus is on rows
	if sd.focus == splitFocusRows && sd.rowIndex >= 0 && sd.rowIndex < len(sd.rows) {
		sd.handleRowFieldKey(msg)
	}

	return DialogActionNone
}

// handleEnter processes Enter key based on current focus.
func (sd *SplitDialog) handleEnter() DialogAction {
	switch sd.focus {
	case splitFocusSaveBtn:
		if err := sd.validate(); err != nil {
			sd.errorMsg = err.Error()
			return DialogActionNone
		}
		sd.errorMsg = ""
		return DialogActionSubmit
	case splitFocusCancelBtn:
		return DialogActionCancel
	case splitFocusAddBtn:
		sd.addRow()
		sd.focus = splitFocusRows
		sd.rowIndex = len(sd.rows) - 1
		sd.fieldFocus = splitFieldCategory
		sd.errorMsg = ""
		return DialogActionNone
	case splitFocusRows:
		// Enter on the AddNew sentinel diverts into the create-category
		// sub-dialog. Other selections (real categories, Transfer, or any
		// non-Category field) fall through to the focus-advance path.
		if sd.fieldFocus == splitFieldCategory && sd.rowIndex >= 0 && sd.rowIndex < len(sd.rows) {
			row := &sd.rows[sd.rowIndex]
			if !row.transferMode && sd.isAddNewSentinel(row.categoryIndex) {
				return DialogActionAddNew
			}
		}
		// Advance to next field within row, or next row, or add button
		sd.focusNext()
		return DialogActionNone
	}
	return DialogActionNone
}

// handleRowFieldKey handles key input for the currently focused row field.
func (sd *SplitDialog) handleRowFieldKey(msg tea.KeyPressMsg) {
	row := &sd.rows[sd.rowIndex]

	switch sd.fieldFocus {
	case splitFieldCategory:
		switch {
		case row.transferMode:
			switch msg.String() {
			case "up":
				if row.accountIndex > 0 {
					row.accountIndex--
				} else {
					// Step out of transfer mode back to the last real
					// category. categoryIndex was on the sentinel; drop
					// to the previous selectable category.
					row.transferMode = false
					if len(sd.categoryOptions) > 0 {
						row.categoryIndex = len(sd.categoryOptions) - 1
					}
					row.accountIndex = 0
				}
			case "down":
				switch {
				case row.accountIndex < len(sd.transferAccountIDs)-1:
					row.accountIndex++
				default:
					// Past the last account: exit transfer mode and land
					// on the AddNew sentinel so the user can keep walking
					// the option list past the transfer block.
					row.transferMode = false
					row.categoryIndex = len(sd.categoryOptions) + 1
					row.accountIndex = 0
				}
			}
		default:
			switch msg.String() {
			case "up":
				if row.categoryIndex > 0 {
					row.categoryIndex--
				}
			case "down":
				if row.categoryIndex < sd.categoryOptionCount()-1 {
					row.categoryIndex++
					if sd.isTransferSentinel(row.categoryIndex) && sd.hasTransferTargets() {
						row.transferMode = true
						row.accountIndex = 0
					}
				}
			}
		}
	case splitFieldAmount:
		handleFieldTextKey(&row.amountField, msg)
	case splitFieldMemo:
		handleFieldTextKey(&row.memoField, msg)
	}
}

// handleFieldTextKey applies text editing keys to a Field.
func handleFieldTextKey(f *Field, msg tea.KeyPressMsg) {
	switch msg.String() {
	case "backspace":
		f.DeleteBack()
		return
	case "delete":
		f.DeleteForward()
		return
	case "left":
		f.MoveCursorLeft()
		return
	case "right":
		f.MoveCursorRight()
		return
	case "home", "ctrl+a":
		f.MoveCursorHome()
		return
	case "end", "ctrl+e":
		f.MoveCursorEnd()
		return
	}
	if msg.Text != "" {
		for _, r := range msg.Text {
			f.InsertChar(r)
		}
	}
}

// focusNext advances focus to the next focusable element.
// Order: row0.category -> row0.amount -> row0.memo -> row1.category -> ... -> addBtn -> cancelBtn -> saveBtn -> wrap
func (sd *SplitDialog) focusNext() {
	switch sd.focus {
	case splitFocusRows:
		// Try next field in current row
		if sd.fieldFocus < splitFieldMemo {
			sd.fieldFocus++
			return
		}
		// Try next row
		if sd.rowIndex < len(sd.rows)-1 {
			sd.rowIndex++
			sd.fieldFocus = splitFieldCategory
			return
		}
		// Move to add button
		sd.focus = splitFocusAddBtn
	case splitFocusAddBtn:
		sd.focus = splitFocusCancelBtn
	case splitFocusCancelBtn:
		sd.focus = splitFocusSaveBtn
	case splitFocusSaveBtn:
		// Wrap to first row
		sd.focus = splitFocusRows
		sd.rowIndex = 0
		sd.fieldFocus = splitFieldCategory
	}
}

// focusPrev moves focus to the previous focusable element.
func (sd *SplitDialog) focusPrev() {
	switch sd.focus {
	case splitFocusRows:
		// Try previous field in current row
		if sd.fieldFocus > splitFieldCategory {
			sd.fieldFocus--
			return
		}
		// Try previous row
		if sd.rowIndex > 0 {
			sd.rowIndex--
			sd.fieldFocus = splitFieldMemo
			return
		}
		// Wrap to save button
		sd.focus = splitFocusSaveBtn
	case splitFocusAddBtn:
		// Back to last row, last field
		sd.focus = splitFocusRows
		sd.rowIndex = len(sd.rows) - 1
		sd.fieldFocus = splitFieldMemo
	case splitFocusCancelBtn:
		sd.focus = splitFocusAddBtn
	case splitFocusSaveBtn:
		sd.focus = splitFocusCancelBtn
	}
}

// Render renders the split dialog as a styled overlay.
func (sd *SplitDialog) Render(styles Styles) string {
	contentWidth := max(sd.width-dialogHorizontalOverhead, 10)

	var lines []string

	// Title row
	title := styles.DialogTitle.Render("SPLIT TRANSACTION")
	closeBtn := styles.Muted.Render("[x]")
	titleGap := max(contentWidth-lipgloss.Width(title)-lipgloss.Width(closeBtn), 1)
	lines = append(lines, title+strings.Repeat(" ", titleGap)+closeBtn)

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Total and remaining
	totalStr := formatDashboardMoney(sd.totalAmount)
	remainStr := formatDashboardMoney(sd.remaining())
	remainStyle := styles.Positive
	rem := sd.remaining()
	if !rem.IsZero() {
		remainStyle = styles.Alert
	}
	summaryLine := "Total: " + totalStr + strings.Repeat(" ", max(contentWidth-len("Total: "+totalStr)-len("Remaining: "+remainStr)-2, 1)) + "Remaining: " + remainStyle.Render(remainStr)
	lines = append(lines, summaryLine)

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))
	lines = append(lines, "")

	// Column headers
	catColW := contentWidth / 3
	amtColW := 14
	memoColW := max(contentWidth-catColW-amtColW-2, 5)
	headerLine := padRight("Category", catColW) + " " + padRight("Amount", amtColW) + " " + "Memo"
	lines = append(lines, styles.Bold.Render(headerLine))
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Rows
	for i, row := range sd.rows {
		rowFocused := sd.focus == splitFocusRows && sd.rowIndex == i

		// Category — or, in transfer mode, the account picker.
		var catText string
		switch {
		case row.transferMode && row.accountIndex < len(sd.transferAccountOptions):
			catText = transferSentinelLabel + " " + sd.transferAccountOptions[row.accountIndex]
		default:
			catText = sd.categoryOptionLabel(row.categoryIndex)
		}
		if rowFocused && sd.fieldFocus == splitFieldCategory {
			catText = lipgloss.NewStyle().Reverse(true).Render(" "+catText+" ") + " ▼"
		} else {
			catText = catText + " ▼"
		}
		catText = padRight(catText, catColW)

		// Amount
		amtFocused := rowFocused && sd.fieldFocus == splitFieldAmount
		amtText := sd.renderTextField(styles, &row.amountField, amtFocused, amtColW-4)

		// Memo
		memoFocused := rowFocused && sd.fieldFocus == splitFieldMemo
		memoText := sd.renderTextField(styles, &row.memoField, memoFocused, memoColW-4)

		lines = append(lines, catText+" "+amtText+" "+memoText)
	}

	// Add split button
	addLabel := "[+ Add split]"
	if sd.focus == splitFocusAddBtn {
		addLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[+ Add split]")
	}
	lines = append(lines, addLabel)
	lines = append(lines, "")

	// Live imbalance indicator (MS-013). Renders the signed delta
	// between the parent amount and the sum of line amounts; turns red
	// while non-zero and dims when balanced. The label sits below the
	// row list so the user's eyes catch it on the way to the Save
	// button — which stays disabled while this is non-zero.
	imb := sd.remaining()
	imbalText := "Imbalance: " + formatDashboardMoney(imb)
	if imb.IsZero() {
		imbalText = styles.Muted.Render(imbalText)
	} else {
		imbalText = styles.Alert.Render(imbalText)
	}
	lines = append(lines, imbalText)
	lines = append(lines, "")

	// Error message
	if sd.errorMsg != "" {
		lines = append(lines, styles.Error.Render(sd.errorMsg))
		lines = append(lines, "")
	}

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Buttons. Save is rendered in a muted style while the dialog is
	// imbalanced (MS-013); pressing Enter on it in that state surfaces
	// the validation error rather than submitting.
	cancelLabel := "[ Cancel ]"
	saveLabel := "[ Save ]"
	saveEnabled := sd.IsSaveEnabled()
	if sd.focus == splitFocusCancelBtn {
		cancelLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ Cancel ]")
	}
	switch {
	case sd.focus == splitFocusSaveBtn && saveEnabled:
		saveLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ Save ]")
	case sd.focus == splitFocusSaveBtn && !saveEnabled:
		// Focused-but-disabled: keep it visible/focusable but dim, so
		// keyboard users see why the action isn't firing.
		saveLabel = styles.Muted.Render("[ Save ]")
	case !saveEnabled:
		saveLabel = styles.Muted.Render("[ Save ]")
	}

	btnGap := max(contentWidth-lipgloss.Width(cancelLabel)-lipgloss.Width(saveLabel), 4)
	leftPad := btnGap / 3
	midPad := btnGap - leftPad
	buttonRow := strings.Repeat(" ", leftPad) + cancelLabel + strings.Repeat(" ", midPad) + saveLabel
	lines = append(lines, buttonRow)

	content := strings.Join(lines, "\n")
	return styles.Dialog.Width(sd.width).Render(content)
}

// renderTextField renders a text field inline with cursor support.
func (sd *SplitDialog) renderTextField(styles Styles, f *Field, focused bool, width int) string {
	if width < 1 {
		width = 1
	}
	runes := []rune(f.Value)

	if focused {
		cursorStyle := lipgloss.NewStyle().Reverse(true)
		var before, cursorChar, after string

		if f.CursorPos() < len(runes) {
			before = string(runes[:f.CursorPos()])
			cursorChar = cursorStyle.Render(string(runes[f.CursorPos()]))
			if f.CursorPos()+1 < len(runes) {
				after = string(runes[f.CursorPos()+1:])
			}
		} else {
			before = string(runes)
			cursorChar = cursorStyle.Render(" ")
		}

		displayLen := len(runes)
		if f.CursorPos() >= len(runes) {
			displayLen++
		}
		pad := max(width-displayLen, 0)

		return "[ " + before + cursorChar + after + strings.Repeat(" ", pad) + " ]"
	}

	// Unfocused
	if len(runes) == 0 && f.Placeholder != "" {
		ph := f.Placeholder
		phRunes := []rune(ph)
		if len(phRunes) > width {
			ph = string(phRunes[:width])
			phRunes = phRunes[:width]
		}
		pad := max(width-len(phRunes), 0)
		return "[ " + styles.Placeholder.Render(ph) + strings.Repeat(" ", pad) + " ]"
	}

	displayRunes := runes
	if len(displayRunes) > width {
		displayRunes = displayRunes[:width]
	}
	pad := max(width-len(displayRunes), 0)
	return "[ " + string(displayRunes) + strings.Repeat(" ", pad) + " ]"
}

// handleSplitDialogKey routes key events to the split dialog.
func (a *App) handleSplitDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.splitDialog == nil {
		return a, nil
	}

	action := a.splitDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		if a.pendingSplitScheduled != nil {
			return a.submitScheduledSplitDialog()
		}
		return a.submitSplitDialog()
	case DialogActionCancel:
		a.closeSplitDialog()
		return a, nil
	case DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSplit()
	}

	return a, nil
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
	if a.splitDialog == nil {
		return a, nil
	}

	a.createCatSource = createCatSourceSplitDialog
	a.createCatSplitRow = a.splitDialog.rowIndex
	parents := a.parentsForCreateCatDialog()
	defaultType := category.TypeExpense
	if rowIdx := a.splitDialog.rowIndex; rowIdx >= 0 && rowIdx < len(a.splitDialog.rows) {
		defaultType = inferCategoryTypeFromAmount(a.splitDialog.rows[rowIdx].amountField.Value)
	}
	a.createCatDialog = buildCreateCategoryDialog("", "", parents, defaultType)
	a.splitDialog.SetVisible(false)
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
		a.createCatDialog = nil
		a.createCatSplitRow = -1
	}()
	sd := a.splitDialog
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
		if i == a.createCatSplitRow {
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
	if a.createCatSplitRow >= 0 && a.createCatSplitRow < len(sd.rows) {
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		sd.rows[a.createCatSplitRow].transferMode = false
		sd.rows[a.createCatSplitRow].categoryIndex = newIdx
	}

	sd.SetVisible(true)
}

// submitSplitDialog validates splits, builds the transaction, and saves it.
// When the pending state carries an existing transaction (edit mode) the
// flow dispatches EditTransactionWithSplitsCommand against that ID; otherwise
// it dispatches CreateTransactionWithSplitsCommand for a new transaction.
func (a *App) submitSplitDialog() (tea.Model, tea.Cmd) {
	if a.splitDialog == nil || a.pendingSplitTxn == nil {
		return a, nil
	}

	splits, err := a.splitDialog.buildSplits()
	if err != nil {
		a.splitDialog.errorMsg = err.Error()
		return a, nil
	}

	pending := a.pendingSplitTxn
	a.closeSplitDialog()

	return a, func() tea.Msg {
		// Resolve or create payee
		var payeeID types.ID
		if pending.payeeName != "" && a.payeeSvc != nil {
			payee, _, err := a.payeeSvc.GetOrCreate(pending.payeeName)
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

			if a.transactionSvc != nil && a.undoManager != nil {
				cmd := undo.NewEditTransactionWithSplitsCommand(a.transactionSvc, &updated, splits)
				if err := a.undoManager.Execute(cmd); err != nil {
					return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
				}
			}
			return splitDialogSavedMsg{}
		}

		// Build transaction (no category when using splits)
		txn := transaction.NewTransactionFull(pending.accountID, pending.date, pending.amount, payeeID, types.NilID, pending.memo)
		txn.Status = pending.status

		// Save with splits via undo manager
		if a.transactionSvc != nil && a.undoManager != nil {
			cmd := undo.NewCreateTransactionWithSplitsCommand(a.transactionSvc, txn, splits)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to save split transaction: %w", err)}
			}
		}

		return splitDialogSavedMsg{}
	}
}

// closeSplitDialog clears the split dialog state.
func (a *App) closeSplitDialog() {
	a.splitDialog = nil
	a.pendingSplitTxn = nil
	a.pendingSplitScheduled = nil
}
