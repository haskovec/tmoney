package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Rendering: the overlay string and the per-cell text field it is built from.

// Render renders the split dialog as a styled overlay.
func (sd *SplitDialog) Render(styles widget.Styles) string {
	contentWidth := max(sd.width-dialog.DialogHorizontalOverhead, 10)

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

	// widget.Column headers
	catColW := contentWidth / 3
	amtColW := 14
	memoColW := max(contentWidth-catColW-amtColW-2, 5)
	headerLine := widget.PadRight("Category", catColW) + " " + widget.PadRight("Amount", amtColW) + " " + "Memo"
	lines = append(lines, styles.Bold.Render(headerLine))
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Rows
	for i, row := range sd.rows {
		rowFocused := sd.focus == splitFocusRows && sd.rowIndex == i

		// Category — or, in transfer mode, the account picker. Truncate the
		// label so the cell (label + " ▼", plus reverse-pad when focused)
		// never exceeds catColW; an overflowing row would wrap to a second
		// terminal line and break both the layout and mouse hit-testing.
		var catLabel string
		switch {
		case row.transferMode && row.accountIndex < len(sd.transferAccountOptions):
			catLabel = transferSentinelLabel + " " + sd.transferAccountOptions[row.accountIndex]
		default:
			catLabel = sd.categoryOptionLabel(row.categoryIndex)
		}
		var catText string
		if rowFocused && sd.fieldFocus == splitFieldCategory {
			catLabel = widget.TruncateRunes(catLabel, max(catColW-4, 1)) // " " + label + " " + " ▼"
			catText = lipgloss.NewStyle().Reverse(true).Render(" "+catLabel+" ") + " ▼"
		} else {
			catLabel = widget.TruncateRunes(catLabel, max(catColW-2, 1)) // label + " ▼"
			catText = catLabel + " ▼"
		}
		catText = widget.PadRight(catText, catColW)

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

	// Buttons. Save first, then Cancel — rendered through the shared
	// dialog button row so spacing, theming, and the focused
	// shortcut-letter highlight match every other dialog. Save renders
	// muted while the dialog is imbalanced (MS-013); clicking/Enter on it
	// in that state surfaces the validation error rather than submitting.
	buttonRow := dialog.RenderButtonRow(styles, []dialog.ButtonSpec{
		{Label: "Save", Focused: sd.focus == splitFocusSaveBtn, Disabled: !sd.IsSaveEnabled()},
		{Label: "Cancel", Focused: sd.focus == splitFocusCancelBtn},
	}, contentWidth)
	lines = append(lines, buttonRow)

	content := strings.Join(lines, "\n")
	// Re-emit the dialog's outer fg + bg after inner SGR resets so styled
	// spans (reversed selected row, muted [x]/imbalance, placeholder memo
	// cells, bold headers) don't punch terminal-default holes through a
	// themed dialog panel. Mirrors dialog.Dialog.Render; no-op on the
	// transparent default theme.
	content = widget.RepaintDialog(content)
	return styles.Dialog.Width(sd.width).Render(content)
}

// renderTextField renders a text field inline with cursor support.
func (sd *SplitDialog) renderTextField(styles widget.Styles, f *dialog.Field, focused bool, width int) string {
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
