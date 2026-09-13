package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// Save: posting the previewed occurrence as a transaction or a transfer, and
// the loan-blocked path when the loan it pays has nothing left to pay.

// submitSchedulePreviewTransfer posts one occurrence of a transfer schedule
// using the edited date / amount / memo / status, creating a clean linked
// transfer pair. Edits are one-off — the template is untouched.
func (a *App) submitSchedulePreviewTransfer(template *scheduled.Transaction, header *dialog.Dialog) (tea.Model, tea.Cmd) {
	header.ClearErrors()
	fields := header.Fields()
	hasErrors := false

	date, err := parseDateInput(fields[previewXferFieldDate].Value)
	if err != nil {
		fields[previewXferFieldDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	var magnitude types.Money
	if m, perr := parseAmountInput(strings.TrimSpace(fields[previewXferFieldAmount].Value)); perr != nil {
		fields[previewXferFieldAmount].Error = "Invalid amount"
		hasErrors = true
	} else {
		magnitude = m.Abs()
	}

	memo := strings.TrimSpace(fields[previewXferFieldMemo].Value)
	cleared := fields[previewXferFieldStatus].SelectedIndex == 1

	// One-off category for this occurrence. Index 0 is the "(None)" sentinel,
	// which clears the label on both posted legs without touching the template.
	categoryID := types.NullableID{}
	catIdx := fields[previewXferFieldCategory].SelectedIndex
	if catIdx > 0 && catIdx < len(a.schedPreviewDialog.categoryIDs) {
		categoryID = types.NullableID{ID: a.schedPreviewDialog.categoryIDs[catIdx], Valid: true}
	}

	if hasErrors {
		return a, nil
	}

	templateID := template.ID
	a.closeSchedulePreviewDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewPostScheduledTransferCommand(
			a.services.Scheduled,
			a.services.Transfer,
			templateID,
			date,
			magnitude,
			memo,
			cleared,
			categoryID,
		)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to post scheduled transfer: %w", err)}
		}
		return scheduledPostedMsg{}
	}
}

// submitSchedulePreviewDialog parses the preview dialog fields, builds
// the parent transaction (and, for multi-line previews, the split rows
// frozen at template values until MS-021 lands per-instance line edits)
// with any user edits applied, and dispatches a
// PostScheduledTransactionWithEditsCommand through the undo manager.
// The schedule advances by one cadence using the template's original
// next_date as the basis, so a date edit in the preview never shifts
// the schedule.
//
// Validation errors keep the dialog open and surface a per-field
// error. On success the dialog closes synchronously and a
// scheduledPostedMsg fires after the async save completes.
func (a *App) submitSchedulePreviewDialog() (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}
	template := a.schedPreviewDialog.Template()
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil || template == nil {
		return a, nil
	}

	if a.schedPreviewDialog.IsTransfer() {
		return a.submitSchedulePreviewTransfer(template, header)
	}

	header.ClearErrors()
	hasErrors := false
	fields := header.Fields()

	date, err := parseDateInput(fields[previewFieldDate].Value)
	if err != nil {
		fields[previewFieldDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	payeeName := strings.TrimSpace(fields[previewFieldPayee].Value)

	var (
		amount      types.Money
		categoryID  types.ID
		memo        string
		statusIdx   int
		multiSplits []*transaction.Split
	)

	if a.schedPreviewDialog.IsMultiLine() {
		memo = strings.TrimSpace(fields[previewMultiFieldMemo].Value)
		statusIdx = fields[previewMultiFieldStatus].SelectedIndex

		sd := a.schedPreviewDialog.SplitDialog()

		switch {
		case a.schedPreviewDialog.loanShaped && !a.schedPreviewDialog.loanSeedFrozen &&
			!hasErrors && a.services.Scheduled != nil:
			// A loan-shaped preview the user has not hand-edited is recomputed
			// authoritatively at the posting date here, not trusted from the
			// (possibly stale or reseed-refused) split editor. This closes the
			// date/seed desync: posting a date at which the loan is paid off (or
			// otherwise fails to compute) is refused with a clear error instead
			// of silently posting the old date's interest/principal.
			ls, cerr := a.services.Scheduled.ComputeLoanSplits(template, date)
			if cerr != nil {
				header.SetErrorMsg("Cannot post this loan payment: " + cerr.Error())
				return a, nil
			}
			multiSplits = ls.Splits
			amount = ls.ParentAmount
		default:
			// Generic multi-line, or a loan preview the user edited (frozen):
			// post the split editor's rows verbatim (user values win). The
			// lines are validated to sum to the editor total.
			if sd != nil {
				built, err := sd.buildSplits()
				if err != nil {
					header.SetErrorMsg(err.Error())
					return a, nil
				}
				multiSplits = built
			}
			amount = template.Amount.Money
			if a.schedPreviewDialog.loanShaped && sd != nil {
				amount = sd.totalAmount
			}
		}
	} else {
		catIdx := fields[previewSingleFieldCat].SelectedIndex
		ids := a.schedPreviewDialog.categoryIDs
		if catIdx > 0 && catIdx < len(ids) {
			categoryID = ids[catIdx]
		}

		amountStr := strings.TrimSpace(fields[previewSingleFieldAmount].Value)
		m, err := parseAmountInput(amountStr)
		if err != nil {
			fields[previewSingleFieldAmount].Error = "Invalid amount"
			hasErrors = true
		} else {
			amount = m
		}
		memo = strings.TrimSpace(fields[previewSingleFieldMemo].Value)
		statusIdx = fields[previewSingleFieldStatus].SelectedIndex
	}

	if hasErrors {
		return a, nil
	}

	// Build the parent transaction the user's edits applied. The
	// account always tracks the template — the preview does not allow
	// changing the destination account.
	parent := transaction.NewTransaction(template.AccountID, date, amount)
	if !categoryID.IsNil() {
		parent.SetCategory(categoryID)
	}
	if memo != "" {
		parent.SetMemo(memo)
	}
	if statusIdx == 1 {
		parent.Clear()
	}

	// Resolve payee ID once we're inside the async command so a new
	// name can be created via the payee service. Capture the snapshot
	// of payees by-name so we can short-circuit the common case where
	// the user didn't edit the name.
	payeeLookup := make(map[string]types.ID, len(a.schedPreviewDialog.payees))
	for _, p := range a.schedPreviewDialog.payees {
		if p == nil {
			continue
		}
		payeeLookup[strings.ToLower(p.Name)] = p.ID
	}

	templateID := template.ID
	loanShaped := a.schedPreviewDialog.loanShaped
	a.closeSchedulePreviewDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		if payeeName != "" {
			if id, ok := payeeLookup[strings.ToLower(payeeName)]; ok {
				parent.SetPayee(id)
			} else if a.services.Payee != nil {
				py, _, err := a.services.Payee.GetOrCreate(payeeName)
				if err != nil {
					return errMsg{err: fmt.Errorf("failed to resolve payee: %w", err)}
				}
				parent.SetPayee(py.ID)
			}
		}

		cmd := undo.NewPostScheduledTransactionWithEditsCommand(
			a.services.Scheduled,
			a.services.Transaction,
			templateID,
			parent,
			multiSplits,
		)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to post scheduled transaction: %w", err)}
		}

		// PostWithEdits runs finalizeLoanPayoff, which marks a loan-shaped
		// schedule completed once the loan balance reaches ≥ 0 (a normal
		// final payment, or a penny-tweaked edit that overshoots). Re-read
		// the schedule to surface that as a payoff toast; the handler runs
		// on the main loop, so it does the SetToast (never this closure).
		paidOff := false
		if loanShaped && a.services.Scheduled != nil {
			if st, gerr := a.services.Scheduled.GetByID(templateID); gerr == nil && st != nil {
				paidOff = st.IsCompleted()
			}
		}
		return scheduledPostedMsg{loanPaidOff: paidOff}
	}
}

// handleSchedulePreviewLoanBlocked reports a loan-shaped schedule that cannot
// be previewed with correct numbers: paid off (already refused-and-completed
// by the loader) or misconfigured. It toasts and refreshes the due list
// instead of opening the preview over stale template values.
func (a *App) handleSchedulePreviewLoanBlocked(paidOff bool, err error) tea.Cmd {
	if a.statusbar != nil {
		if paidOff {
			a.statusbar.SetToast(loanPaidOffToast, widget.NotificationInfo)
		} else {
			a.statusbar.SetToast(fmt.Sprintf("Cannot post loan payment: %v", err), widget.NotificationAlert)
		}
	}
	return tea.Batch(
		a.loadScheduledViewData(),
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		widget.ClearToastCmd(),
	)
}
