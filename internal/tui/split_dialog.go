package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/models"
)

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
type splitRow struct {
	categoryIndex int
	amountField   Field
	memoField     Field
}

// pendingSplitTransaction holds the transaction data while the split editor is open.
type pendingSplitTransaction struct {
	accountID models.ID
	date      models.Date
	payeeName string
	amount    models.Money
	memo      string
	status    models.TransactionStatus
}

// splitDialogSavedMsg is sent when a split transaction has been saved.
type splitDialogSavedMsg struct{}

// SplitDialog is a custom dialog for editing split transaction entries.
type SplitDialog struct {
	visible         bool
	width           int
	totalAmount     models.Money
	rows            []splitRow
	focus           splitDialogFocus
	rowIndex        int
	fieldFocus      splitFieldFocus
	categoryOptions []string
	categoryIDs     []models.ID
	errorMsg        string
}

// NewSplitDialog creates a new SplitDialog for the given total amount.
func NewSplitDialog(amount models.Money, categoryOptions []string, categoryIDs []models.ID) *SplitDialog {
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

// IsVisible returns whether the split dialog is currently shown.
func (sd *SplitDialog) IsVisible() bool {
	return sd.visible
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

// ErrorMsg returns the current error message.
func (sd *SplitDialog) ErrorMsg() string {
	return sd.errorMsg
}

// remaining calculates totalAmount minus the sum of entered split amounts.
func (sd *SplitDialog) remaining() models.Money {
	sum := models.ZeroMoney
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
		// Category must be selected (index > 0 means not "(None)")
		if row.categoryIndex <= 0 {
			return fmt.Errorf("split %d: category is required", i+1)
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

// buildSplits produces Split models from the current rows.
func (sd *SplitDialog) buildSplits() ([]*models.Split, error) {
	if err := sd.validate(); err != nil {
		return nil, err
	}

	var splits []*models.Split
	for _, row := range sd.rows {
		amount, _ := parseAmountInput(row.amountField.Value)
		categoryID := sd.categoryIDs[row.categoryIndex]
		memo := strings.TrimSpace(row.memoField.Value)

		var split *models.Split
		if memo != "" {
			split = models.NewSplitWithMemo(models.NilID, categoryID, amount, memo)
		} else {
			split = models.NewSplit(models.NilID, categoryID, amount)
		}
		splits = append(splits, split)
	}

	return splits, nil
}

// HandleKey processes a key event and returns the resulting action.
func (sd *SplitDialog) HandleKey(msg tea.KeyMsg) DialogAction {
	// Escape always cancels
	if msg.Type == tea.KeyEsc {
		return DialogActionCancel
	}

	// Ctrl+D removes current row (if more than 1)
	if msg.Type == tea.KeyCtrlD && sd.focus == splitFocusRows && len(sd.rows) > 1 {
		sd.removeRow(sd.rowIndex)
		sd.errorMsg = ""
		return DialogActionNone
	}

	// Tab/Shift-Tab cycle through all focusable elements
	if msg.Type == tea.KeyTab {
		sd.focusNext()
		return DialogActionNone
	}
	if msg.Type == tea.KeyShiftTab {
		sd.focusPrev()
		return DialogActionNone
	}

	// Enter behavior depends on focus
	if msg.Type == tea.KeyEnter {
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
		// Advance to next field within row, or next row, or add button
		sd.focusNext()
		return DialogActionNone
	}
	return DialogActionNone
}

// handleRowFieldKey handles key input for the currently focused row field.
func (sd *SplitDialog) handleRowFieldKey(msg tea.KeyMsg) {
	row := &sd.rows[sd.rowIndex]

	switch sd.fieldFocus {
	case splitFieldCategory:
		switch msg.Type {
		case tea.KeyUp:
			if row.categoryIndex > 0 {
				row.categoryIndex--
			}
		case tea.KeyDown:
			if row.categoryIndex < len(sd.categoryOptions)-1 {
				row.categoryIndex++
			}
		}
	case splitFieldAmount:
		handleFieldTextKey(&row.amountField, msg)
	case splitFieldMemo:
		handleFieldTextKey(&row.memoField, msg)
	}
}

// handleFieldTextKey applies text editing keys to a Field.
func handleFieldTextKey(f *Field, msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyBackspace:
		f.DeleteBack()
	case tea.KeyDelete:
		f.DeleteForward()
	case tea.KeyLeft:
		f.MoveCursorLeft()
	case tea.KeyRight:
		f.MoveCursorRight()
	case tea.KeyHome, tea.KeyCtrlA:
		f.MoveCursorHome()
	case tea.KeyEnd, tea.KeyCtrlE:
		f.MoveCursorEnd()
	case tea.KeyRunes:
		for _, r := range msg.Runes {
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

		// Category
		catText := sd.categoryOptions[row.categoryIndex]
		if rowFocused && sd.fieldFocus == splitFieldCategory {
			catText = lipgloss.NewStyle().Reverse(true).Render(" "+catText+" ") + " ▼"
		} else {
			catText = catText + " ▼"
		}
		catText = padRight(catText, catColW)

		// Amount
		amtFocused := rowFocused && sd.fieldFocus == splitFieldAmount
		amtText := sd.renderTextField(&row.amountField, amtFocused, amtColW-4)

		// Memo
		memoFocused := rowFocused && sd.fieldFocus == splitFieldMemo
		memoText := sd.renderTextField(&row.memoField, memoFocused, memoColW-4)

		lines = append(lines, catText+" "+amtText+" "+memoText)
	}

	// Add split button
	addLabel := "[+ Add split]"
	if sd.focus == splitFocusAddBtn {
		addLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[+ Add split]")
	}
	lines = append(lines, addLabel)
	lines = append(lines, "")

	// Error message
	if sd.errorMsg != "" {
		lines = append(lines, styles.Error.Render(sd.errorMsg))
		lines = append(lines, "")
	}

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Buttons
	cancelLabel := "[ Cancel ]"
	saveLabel := "[ Save ]"
	if sd.focus == splitFocusCancelBtn {
		cancelLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ Cancel ]")
	}
	if sd.focus == splitFocusSaveBtn {
		saveLabel = lipgloss.NewStyle().Reverse(true).Bold(true).Render("[ Save ]")
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
func (sd *SplitDialog) renderTextField(f *Field, focused bool, width int) string {
	if width < 1 {
		width = 1
	}
	runes := []rune(f.Value)

	if focused {
		cursorStyle := lipgloss.NewStyle().Reverse(true)
		var before, cursorChar, after string

		if f.cursorPos < len(runes) {
			before = string(runes[:f.cursorPos])
			cursorChar = cursorStyle.Render(string(runes[f.cursorPos]))
			if f.cursorPos+1 < len(runes) {
				after = string(runes[f.cursorPos+1:])
			}
		} else {
			before = string(runes)
			cursorChar = cursorStyle.Render(" ")
		}

		displayLen := len(runes)
		if f.cursorPos >= len(runes) {
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
		return "[ " + lipgloss.NewStyle().Foreground(ColorMuted).Render(ph) + strings.Repeat(" ", pad) + " ]"
	}

	displayRunes := runes
	if len(displayRunes) > width {
		displayRunes = displayRunes[:width]
	}
	pad := max(width-len(displayRunes), 0)
	return "[ " + string(displayRunes) + strings.Repeat(" ", pad) + " ]"
}

// handleSplitDialogKey routes key events to the split dialog.
func (a *App) handleSplitDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.splitDialog == nil {
		return a, nil
	}

	action := a.splitDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitSplitDialog()
	case DialogActionCancel:
		a.closeSplitDialog()
		return a, nil
	}

	return a, nil
}

// submitSplitDialog validates splits, builds the transaction, and saves it.
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
		var payeeID models.ID
		if pending.payeeName != "" && a.payeeSvc != nil {
			payee, _, err := a.payeeSvc.GetOrCreate(pending.payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = payee.ID
		}

		// Build transaction (no category when using splits)
		txn := models.NewTransactionFull(pending.accountID, pending.date, pending.amount, payeeID, models.NilID, pending.memo)
		txn.Status = pending.status

		// Save with splits
		if a.transactionSvc != nil {
			if err := a.transactionSvc.CreateWithSplits(txn, splits); err != nil {
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
}
