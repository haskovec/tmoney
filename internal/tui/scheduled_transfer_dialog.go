package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// Scheduled-transfer dialog field indices. The dialog mirrors the register's
// New Transfer dialog (From / To / Amount / Category / Memo) and appends the
// recurrence fields shared with the regular scheduled dialog. The optional
// Category combo labels the transfer for spending tracking (a monthly
// credit-card payment as Bills:Credit Card, etc.); it flows onto both posted
// legs.
const (
	schedXferFieldFrom       = 0
	schedXferFieldTo         = 1
	schedXferFieldAmount     = 2
	schedXferFieldCategory   = 3
	schedXferFieldMemo       = 4
	schedXferFieldFrequency  = 5
	schedXferFieldInterval   = 6
	schedXferFieldStartDate  = 7
	schedXferFieldDuration   = 8
	schedXferFieldEndDate    = 9
	schedXferFieldOccurrence = 10
	schedXferFieldAutoPost   = 11
	schedXferFieldLeadDays   = 12
	schedXferFieldCount      = 13
)

// buildNonInvestmentAccountOptions builds parallel name/ID slices for the
// transfer pickers, excluding investment-type accounts (investment, hsa).
// Scheduled transfers are regular↔regular only; funding an investment account
// on a schedule goes through the paycheck / multi-line flow.
func buildNonInvestmentAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))
	for _, acct := range accounts {
		if acct.Type.IsInvestmentType() {
			continue
		}
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}
	return options, ids
}

// addScheduleRecurrenceFields appends the recurrence fields shared by the new
// and edit transfer dialogs, pre-filling from st when non-nil.
func addScheduleRecurrenceFields(d *dialog.Dialog, st *scheduled.Transaction) {
	freqIdx := frequencyToIndex(scheduled.FrequencyMonthly)
	intervalStr := "1"
	startStr := time.Now().Format("01/02/2006")
	durationIdx := durationIndefinite
	endStr := ""
	occStr := ""
	autoPost := false
	leadIdx := leadDaysOnTheDay

	if st != nil {
		freqIdx = frequencyToIndex(st.Frequency)
		intervalStr = strconv.Itoa(st.Interval)
		startStr = st.StartDate.Time().Format("01/02/2006")
		if st.EndDate.Valid {
			durationIdx = durationUntilDate
			endStr = st.EndDate.Date.Time().Format("01/02/2006")
		} else if st.Occurrences.Valid {
			durationIdx = durationOccurrences
			occStr = strconv.FormatInt(st.Occurrences.Int64, 10)
		}
		autoPost = st.AutoPost
		leadIdx = leadDaysToIndex(st.PostLeadDays)
	}

	d.AddSelectField("Frequency", buildFrequencyOptions(), freqIdx)
	f := d.AddTextField("Interval", intervalStr, "Every N periods", 5)
	f.Required = true
	f = d.AddDateField("Start Date", startStr)
	f.Required = true
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, durationIdx)
	d.AddOptionalDateField("End Date", endStr)
	d.AddTextField("Occurrences", occStr, "Number of times", 5)
	d.AddCheckboxField("Auto-post", autoPost)
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, leadIdx)
}

// buildNewScheduledTransferDialog creates the New Scheduled Transfer dialog.
// categoryOptions is the "(None)"-led combo list produced by
// buildCategoryOptions; the parallel ID slice is held on the App as
// schedDialogCategoryIDs.
func buildNewScheduledTransferDialog(accountOptions, categoryOptions []string) *dialog.Dialog {
	d := dialog.NewDialog("New Scheduled Transfer")
	d.SetWidth(62)

	d.AddSelectField("From", accountOptions, 0)
	toIndex := 0
	if len(accountOptions) > 1 {
		toIndex = 1
	}
	d.AddSelectField("To", accountOptions, toIndex)

	f := d.AddTextField("Amount", "", "100.00", 12)
	f.Required = true

	// Category — optional label for the transfer, with inline creation.
	catField := d.AddComboField("Category", categoryOptions, 0)
	catField.AddNewLabel = "[+ Add new category…]"

	d.AddTextField("Memo", "", "Optional memo", 0)

	addScheduleRecurrenceFields(d, nil)

	d.SetVisible(true)
	return d
}

// buildEditScheduledTransferDialog creates the edit-mode transfer dialog,
// pre-filled from an existing transfer schedule. From/To are editable here —
// editing the series may re-orient the transfer.
func buildEditScheduledTransferDialog(st *scheduled.Transaction, accountOptions, categoryOptions []string, accountIDs, categoryIDs []types.ID) *dialog.Dialog {
	d := dialog.NewDialog("Edit Scheduled Transfer")
	d.SetWidth(62)

	fromIdx := indexOfID(accountIDs, st.AccountID)
	toIdx := 0
	if st.TransferAccountID.Valid {
		toIdx = indexOfID(accountIDs, st.TransferAccountID.ID)
	}
	d.AddSelectField("From", accountOptions, fromIdx)
	d.AddSelectField("To", accountOptions, toIdx)

	amountStr := ""
	if st.HasAmount() {
		amountStr = st.Amount.Money.Abs().String()
	}
	f := d.AddTextField("Amount", amountStr, "100.00", 12)
	f.Required = true

	// Category — seeded from the schedule's existing category (0 = "(None)").
	catIdx := 0
	if st.HasCategory() {
		catIdx = indexOfID(categoryIDs, st.CategoryID.ID)
	}
	catField := d.AddComboField("Category", categoryOptions, catIdx)
	catField.AddNewLabel = "[+ Add new category…]"

	memo := ""
	if st.Memo.Valid {
		memo = st.Memo.String
	}
	d.AddTextField("Memo", memo, "Optional memo", 0)

	addScheduleRecurrenceFields(d, st)

	d.SetVisible(true)
	return d
}

// accountNameByID returns the display name of the account with the given ID,
// or an empty string if not found.
func accountNameByID(accounts []*account.Account, id types.ID) string {
	for _, acct := range accounts {
		if acct.ID == id {
			return acct.Name
		}
	}
	return ""
}

// indexOfID returns the position of id in ids, or 0 if not found.
func indexOfID(ids []types.ID, id types.ID) int {
	for i, candidate := range ids {
		if candidate == id {
			return i
		}
	}
	return 0
}

// loadNewScheduledTransferDialogData loads accounts for a new scheduled
// transfer and dispatches a scheduledDialogDataMsg flagged as a transfer so
// the data handler builds the transfer-shaped dialog.
func (a *App) loadNewScheduledTransferDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &scheduledDialogData{
			mode:       scheduledDialogModeNew,
			isTransfer: true,
		}
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}
		return scheduledDialogDataMsg{data: data}
	}
}

// submitScheduledTransferDialog parses the transfer dialog, validates, and
// saves the transfer schedule.
func (a *App) submitScheduledTransferDialog() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return a, nil
	}

	fields := a.schedDialog.Fields()
	if len(fields) < schedXferFieldCount {
		return a, nil
	}

	a.schedDialog.ClearErrors()
	hasErrors := false

	fromIdx := fields[schedXferFieldFrom].SelectedIndex
	toIdx := fields[schedXferFieldTo].SelectedIndex
	fromID, toID := types.NilID, types.NilID
	if fromIdx >= 0 && fromIdx < len(a.schedDialogAccountIDs) {
		fromID = a.schedDialogAccountIDs[fromIdx]
	} else {
		fields[schedXferFieldFrom].Error = "Please select an account"
		hasErrors = true
	}
	if toIdx >= 0 && toIdx < len(a.schedDialogAccountIDs) {
		toID = a.schedDialogAccountIDs[toIdx]
	} else {
		fields[schedXferFieldTo].Error = "Please select an account"
		hasErrors = true
	}
	if !fromID.IsNil() && fromID == toID {
		a.schedDialog.SetErrorMsg("From and To must be different accounts")
		hasErrors = true
	}

	// Amount is required and entered as a positive magnitude; stored as the
	// signed effect on the source (negative).
	amountStr := strings.TrimSpace(fields[schedXferFieldAmount].Value)
	var magnitude types.Money
	if amountStr == "" {
		fields[schedXferFieldAmount].Error = "Amount is required"
		hasErrors = true
	} else if m, err := parseAmountInput(amountStr); err != nil {
		fields[schedXferFieldAmount].Error = "Invalid amount"
		hasErrors = true
	} else {
		magnitude = m.Abs()
	}

	// Category is optional; index 0 is the "(None)" sentinel (NilID).
	categoryID := types.NilID
	catIdx := fields[schedXferFieldCategory].SelectedIndex
	if catIdx > 0 && catIdx < len(a.schedDialogCategoryIDs) {
		categoryID = a.schedDialogCategoryIDs[catIdx]
	}

	memo := strings.TrimSpace(fields[schedXferFieldMemo].Value)
	frequency := frequencyFromIndex(fields[schedXferFieldFrequency].SelectedIndex)

	interval := 1
	if s := strings.TrimSpace(fields[schedXferFieldInterval].Value); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			fields[schedXferFieldInterval].Error = "Must be a positive number"
			hasErrors = true
		} else {
			interval = n
		}
	}

	startDate, err := parseDateInput(fields[schedXferFieldStartDate].Value)
	if err != nil {
		fields[schedXferFieldStartDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	var endDate types.NullableDate
	var occurrences types.NullableInt
	switch fields[schedXferFieldDuration].SelectedIndex {
	case durationUntilDate:
		raw := fields[schedXferFieldEndDate].Value
		if dialog.IsBlankDateInput(raw) {
			fields[schedXferFieldEndDate].Error = "End date is required"
			hasErrors = true
		} else if ed, err := parseDateInput(raw); err != nil {
			fields[schedXferFieldEndDate].Error = "Invalid date (MM/DD/YYYY)"
			hasErrors = true
		} else {
			endDate = types.NullableDate{Date: ed, Valid: true}
		}
	case durationOccurrences:
		s := strings.TrimSpace(fields[schedXferFieldOccurrence].Value)
		if s == "" {
			fields[schedXferFieldOccurrence].Error = "Occurrences is required"
			hasErrors = true
		} else if n, err := strconv.ParseInt(s, 10, 64); err != nil || n < 1 {
			fields[schedXferFieldOccurrence].Error = "Must be a positive number"
			hasErrors = true
		} else {
			occurrences = types.NullableInt{Int64: n, Valid: true}
		}
	}

	autoPost := fields[schedXferFieldAutoPost].Checked
	leadDays := leadDaysFromIndex(fields[schedXferFieldLeadDays].SelectedIndex)

	if hasErrors {
		return a, nil
	}

	mode := a.schedDialogData.mode
	existing := a.schedDialogData.scheduled
	signedAmount := magnitude.Neg()

	a.closeScheduledDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		if mode == scheduledDialogModeEdit && existing != nil {
			st := existing
			st.Splits = nil
			st.ClearPayee()
			st.AccountID = fromID
			st.Frequency = frequency
			st.Interval = interval
			st.StartDate = startDate
			st.SetTransfer(toID)
			st.SetAmount(signedAmount)
			st.SetMemo(memo)
			if categoryID.IsNil() {
				st.ClearCategory()
			} else {
				st.SetCategory(categoryID)
			}
			st.ClearEndDate()
			st.ClearOccurrences()
			if endDate.Valid {
				st.SetEndDate(endDate.Date)
			} else if occurrences.Valid {
				st.SetOccurrences(occurrences.Int64)
			}
			st.SetAutoPost(autoPost)
			st.SetPostLeadDays(leadDays)

			cmd := undo.NewEditScheduledTransactionCommand(a.scheduledTxnSvc, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transfer: %w", err)}
			}
			return scheduledDialogSavedMsg{}
		}

		st := scheduled.NewTransaction(fromID, frequency, startDate)
		st.Interval = interval
		st.SetTransfer(toID)
		st.SetAmount(signedAmount)
		st.SetMemo(memo)
		if !categoryID.IsNil() {
			st.SetCategory(categoryID)
		}
		if endDate.Valid {
			st.SetEndDate(endDate.Date)
		} else if occurrences.Valid {
			st.SetOccurrences(occurrences.Int64)
		}
		st.SetAutoPost(autoPost)
		st.SetPostLeadDays(leadDays)

		cmd := undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, st)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create scheduled transfer: %w", err)}
		}
		return scheduledDialogSavedMsg{}
	}
}

// openCreateCategorySubDialogFromSchedTransfer hides the scheduled transfer
// dialog and opens the inline create-category sub-dialog seeded from the
// Category combo's typed query. The dialog is kept alive (hidden) so its field
// state survives the divert; applyCreatedCategoryToSchedTransfer re-shows it
// with the new category selected. Transfers are labeled for spending tracking
// and their always-positive amount carries no income/expense signal, so the
// sub-dialog defaults to an Expense type (matching the register Transfer
// dialog).
func (a *App) openCreateCategorySubDialogFromSchedTransfer() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil {
		return a, nil
	}
	fields := a.schedDialog.Fields()
	if len(fields) <= schedXferFieldCategory {
		return a, nil
	}
	catField := fields[schedXferFieldCategory]
	query := catField.Query
	catField.AddNewTriggered = false
	catField.Query = ""

	// createCatSource must be set before parentsForCreateCatDialog so the
	// helper picks the right parents source.
	a.createCatSource = createCatSourceSchedTransferDialog
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	a.createCatDialog = buildCreateCategoryDialog(name, parent, parents, category.TypeExpense)
	a.schedDialog.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSchedTransfer is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the scheduled
// transfer dialog. It reloads the Category combo with newCat pre-selected,
// advances focus to Memo, re-shows the dialog, and clears the sub-dialog.
func (a *App) applyCreatedCategoryToSchedTransfer(newCat *category.Category, cats []*category.Category) {
	if a.schedDialog == nil {
		a.createCatDialog = nil
		return
	}
	options, ids := buildCategoryOptions(cats)
	a.schedDialogCategoryIDs = ids
	a.schedDialogCategoryOptions = options

	if len(a.schedDialog.Fields()) > schedXferFieldCategory {
		catField := a.schedDialog.Fields()[schedXferFieldCategory]
		catField.Options = options
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		catField.SelectedIndex = newIdx
		// Focus advances to Memo so the user can keep typing.
		a.schedDialog.SetFocusIndex(schedXferFieldMemo)
		a.schedDialog.SetVisible(true)
	}
	a.createCatDialog = nil
}
