package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// Save: the single-line path and the multi-line path that lands here from the
// split editor. Both decide between create and update, and both guard the
// paycheck and loan demotions.

// submitScheduledDialog parses dialog fields, validates, and saves the scheduled transaction.
func (a *App) submitScheduledDialog() (tea.Model, tea.Cmd) {
	if a.sched.dlg == nil || a.sched.data == nil {
		return a, nil
	}

	fields := a.sched.dlg.Fields()
	if len(fields) < 14 {
		return a, nil
	}

	a.sched.dlg.ClearErrors()
	hasErrors := false

	// Account
	acctIdx := fields[schedFieldAccount].SelectedIndex
	if acctIdx < 0 || acctIdx >= len(a.sched.accountIDs) {
		fields[schedFieldAccount].Error = "Please select an account"
		hasErrors = true
	}
	accountID := types.NilID
	if acctIdx >= 0 && acctIdx < len(a.sched.accountIDs) {
		accountID = a.sched.accountIDs[acctIdx]
	}

	// Payee name
	payeeName := strings.TrimSpace(fields[schedFieldPayee].Value)

	// Category
	catIdx := fields[schedFieldCategory].SelectedIndex
	var categoryID types.ID
	if catIdx > 0 && catIdx < len(a.sched.categoryIDs) {
		categoryID = a.sched.categoryIDs[catIdx]
	}

	// Amount (empty = variable)
	amountStr := strings.TrimSpace(fields[schedFieldAmount].Value)
	var amount types.NullableMoney
	if amountStr != "" {
		m, err := parseAmountInput(amountStr)
		if err != nil {
			fields[schedFieldAmount].Error = "Invalid amount"
			hasErrors = true
		} else {
			amount = types.NullableMoney{Money: m, Valid: true}
		}
	}

	// Memo
	memo := strings.TrimSpace(fields[schedFieldMemo].Value)

	// Frequency
	frequency := frequencyFromIndex(fields[schedFieldFrequency].SelectedIndex)

	// Interval
	intervalStr := strings.TrimSpace(fields[schedFieldInterval].Value)
	interval := 1
	if intervalStr != "" {
		n, err := strconv.Atoi(intervalStr)
		if err != nil || n < 1 {
			fields[schedFieldInterval].Error = "Must be a positive number"
			hasErrors = true
		} else {
			interval = n
		}
	}

	// Start date
	startDate, err := parseDateInput(fields[schedFieldStartDate].Value)
	if err != nil {
		fields[schedFieldStartDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Duration
	durationChoice := fields[schedFieldDuration].SelectedIndex

	var endDate types.NullableDate
	var occurrences types.NullableInt

	switch durationChoice {
	case durationUntilDate:
		endDateRaw := fields[schedFieldEndDate].Value
		if dialog.IsBlankDateInput(endDateRaw) {
			fields[schedFieldEndDate].Error = "End date is required"
			hasErrors = true
		} else {
			ed, err := parseDateInput(endDateRaw)
			if err != nil {
				fields[schedFieldEndDate].Error = "Invalid date (MM/DD/YYYY)"
				hasErrors = true
			} else {
				endDate = types.NullableDate{Date: ed, Valid: true}
			}
		}

	case durationOccurrences:
		occStr := strings.TrimSpace(fields[schedFieldOccurrence].Value)
		if occStr == "" {
			fields[schedFieldOccurrence].Error = "Occurrences is required"
			hasErrors = true
		} else {
			n, err := strconv.ParseInt(occStr, 10, 64)
			if err != nil || n < 1 {
				fields[schedFieldOccurrence].Error = "Must be a positive number"
				hasErrors = true
			} else {
				occurrences = types.NullableInt{Int64: n, Valid: true}
			}
		}
	}

	// Auto-post
	autoPost := fields[schedFieldAutoPost].Checked
	leadDays := leadDaysFromIndex(fields[schedFieldLeadDays].SelectedIndex)

	// Split transaction — when checked, the scalar amount becomes the parent
	// net amount and the user finishes the schedule in the split editor. A
	// multi-line schedule requires a fixed parent amount.
	isSplit := fields[schedFieldSplit].Checked
	if isSplit && !amount.Valid {
		fields[schedFieldAmount].Error = "Amount is required for split schedules"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	mode := a.sched.data.mode
	existingSched := a.sched.data.scheduled

	if isSplit {
		pending := &pendingSplitScheduled{
			mode:        mode,
			existing:    existingSched,
			accountID:   accountID,
			payeeName:   payeeName,
			amount:      amount.Money,
			memo:        memo,
			frequency:   frequency,
			interval:    interval,
			startDate:   startDate,
			endDate:     endDate,
			occurrences: occurrences,
			autoPost:    autoPost,
			leadDays:    leadDays,
		}

		categoryOptions := fields[schedFieldCategory].Options
		categoryIDs := a.sched.categoryIDs
		accountOptions, accountIDs := buildSplitTransferAccountOptions(a.sched.data.accounts)

		open := splitOpen{
			amount:          amount.Money,
			categoryOptions: categoryOptions,
			categoryIDs:     categoryIDs,
			accountOptions:  accountOptions,
			accountIDs:      accountIDs,
			accountID:       accountID,
		}
		// Seed the split dialog from existing children when editing a
		// schedule that already carries a multi-line template.
		if mode == scheduledDialogModeEdit {
			open.seedSplits = transactionSplitsFromScheduled(existingSched)
		}
		a.closeScheduledDialog()
		a.split.openForSchedule(pending, open)
		return a, nil
	}

	// Demotion guard: unchecking Split on a loan-shaped schedule clears its
	// child splits (and their loan_section tags), demoting it to a generic
	// single-line schedule. Warn first.
	demotesLoan := mode == scheduledDialogModeEdit && existingSched != nil &&
		a.services.Scheduled != nil && a.services.Scheduled.IsLoanShaped(existingSched)

	// Demotion guard: unchecking Split on a paycheck-shaped schedule clears
	// its children (and their paycheck_section tags) too — the save closure
	// below sets st.Splits = nil unconditionally, so "is paycheck-shaped" is
	// the same thing as "will lose the tags" here.
	demotesPaycheck := mode == scheduledDialogModeEdit && existingSched != nil &&
		looksLikePaycheck(existingSched)

	// Close dialog before async save for responsive UI
	a.closeScheduledDialog()

	save := func() tea.Msg {
		// Resolve or create payee
		var payeeID types.ID
		if payeeName != "" && a.services.Payee != nil {
			py, _, err := a.services.Payee.GetOrCreate(payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = py.ID
		}

		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		if mode == scheduledDialogModeEdit && existingSched != nil {
			// Update existing scheduled transaction
			st := existingSched
			// Clear any prior multi-line children so an un-toggled Split
			// reverts the schedule to a legacy single-line template.
			st.Splits = nil
			st.AccountID = accountID
			st.Frequency = frequency
			st.Interval = interval
			st.StartDate = startDate

			// Payee
			if !payeeID.IsNil() {
				st.SetPayee(payeeID)
			} else {
				st.ClearPayee()
			}

			// Category
			if !categoryID.IsNil() {
				st.SetCategory(categoryID)
			} else {
				st.ClearCategory()
			}

			// Amount
			if amount.Valid {
				st.Amount = amount
			} else {
				st.ClearAmount()
			}

			// Memo
			st.SetMemo(memo)

			// Duration
			st.ClearEndDate()
			st.ClearOccurrences()
			if endDate.Valid {
				st.SetEndDate(endDate.Date)
			} else if occurrences.Valid {
				st.SetOccurrences(occurrences.Int64)
			}

			// Auto-post
			st.SetAutoPost(autoPost)
			st.SetPostLeadDays(leadDays)

			cmd := undo.NewEditScheduledTransactionCommand(a.services.Scheduled, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transaction: %w", err)}
			}
		} else {
			// Create new scheduled transaction
			st := scheduled.NewTransaction(accountID, frequency, startDate)
			st.Interval = interval

			// Payee
			if !payeeID.IsNil() {
				st.SetPayee(payeeID)
			}

			// Category
			if !categoryID.IsNil() {
				st.SetCategory(categoryID)
			}

			// Amount
			if amount.Valid {
				st.Amount = amount
			}

			// Memo
			st.SetMemo(memo)

			// Duration
			if endDate.Valid {
				st.SetEndDate(endDate.Date)
			} else if occurrences.Valid {
				st.SetOccurrences(occurrences.Int64)
			}

			// Auto-post
			st.SetAutoPost(autoPost)
			st.SetPostLeadDays(leadDays)

			cmd := undo.NewCreateScheduledTransactionCommand(a.services.Scheduled, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
			}
		}

		return scheduledDialogSavedMsg{}
	}

	switch {
	case demotesLoan:
		a.showConfirmDialog("Convert loan schedule?", loanDemotionWarning, save)
		return a, nil
	case demotesPaycheck:
		a.showConfirmDialog("Drop paycheck sections?", paycheckDemotionWarning, save)
		return a, nil
	}
	return a, save
}

// submitScheduledSplitDialog finalizes a multi-line scheduled transaction
// after the user has filled out the SplitDialog opened from the scheduled
// dialog's Split toggle. The split editor produces transaction.Split rows;
// this handler translates them to scheduled.Split children and dispatches
// the appropriate undo command.
func (a *App) submitScheduledSplitDialog() (tea.Model, tea.Cmd) {
	pending, splits, ok := a.split.takeScheduledSplits()
	if !ok {
		return a, nil
	}
	children := scheduledSplitsFromTransaction(splits)
	// Demotion guard: saving a loan-shaped schedule through the generic split
	// editor strips its loan_section tags, silently converting it to a generic
	// schedule that books stale template interest. Warn first.
	demotesLoan := pending.mode == scheduledDialogModeEdit && pending.existing != nil &&
		a.services.Scheduled != nil && a.services.Scheduled.IsLoanShaped(pending.existing)

	// Demotion guard: paycheck_section rides through this editor, so a save
	// loses the paycheck shape only when the resulting children are no longer
	// fully tagged — a row added here, or the last earnings row deleted. Gate
	// on that outcome, not on the shape: an edit that preserves every tag is
	// the normal supported operation and must not prompt.
	demotesPaycheck := pending.mode == scheduledDialogModeEdit && pending.existing != nil &&
		looksLikePaycheck(pending.existing) &&
		!looksLikePaycheck(&scheduled.Transaction{Splits: children})

	save := func() tea.Msg {
		var payeeID types.ID
		if pending.payeeName != "" && a.services.Payee != nil {
			py, _, err := a.services.Payee.GetOrCreate(pending.payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = py.ID
		}

		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		applyScalars := func(st *scheduled.Transaction) {
			st.AccountID = pending.accountID
			st.Frequency = pending.frequency
			st.Interval = pending.interval
			st.StartDate = pending.startDate
			if !payeeID.IsNil() {
				st.SetPayee(payeeID)
			} else {
				st.ClearPayee()
			}
			// Multi-line schedules have no scalar category.
			st.ClearCategory()
			st.SetAmount(pending.amount)
			st.SetMemo(pending.memo)
			st.ClearEndDate()
			st.ClearOccurrences()
			if pending.endDate.Valid {
				st.SetEndDate(pending.endDate.Date)
			} else if pending.occurrences.Valid {
				st.SetOccurrences(pending.occurrences.Int64)
			}
			st.SetAutoPost(pending.autoPost)
			st.SetPostLeadDays(pending.leadDays)
			st.Splits = children
		}

		if pending.mode == scheduledDialogModeEdit && pending.existing != nil {
			st := pending.existing
			applyScalars(st)
			cmd := undo.NewEditScheduledTransactionCommand(a.services.Scheduled, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transaction: %w", err)}
			}
		} else {
			st := scheduled.NewTransaction(pending.accountID, pending.frequency, pending.startDate)
			applyScalars(st)
			cmd := undo.NewCreateScheduledTransactionCommand(a.services.Scheduled, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
			}
		}

		return scheduledDialogSavedMsg{}
	}

	switch {
	case demotesLoan:
		a.showConfirmDialog("Convert loan schedule?", loanDemotionWarning, save)
		return a, nil
	case demotesPaycheck:
		a.showConfirmDialog("Drop paycheck sections?", paycheckDemotionWarning, save)
		return a, nil
	}
	return a, save
}
