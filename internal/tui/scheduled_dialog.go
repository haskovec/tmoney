package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// scheduledDialogMode indicates whether the dialog is creating or editing.
type scheduledDialogMode int

const (
	scheduledDialogModeNew scheduledDialogMode = iota
	scheduledDialogModeEdit
)

// scheduledDialogData holds the loaded data needed for the scheduled dialog.
type scheduledDialogData struct {
	mode      scheduledDialogMode
	scheduled *scheduled.Transaction // non-nil when editing
	accounts  []*account.Account
	payees    []*payee.Payee
	payeeMap  map[string]*payee.Payee // lowercase name -> payee
}

// scheduledDialogDataMsg is sent when scheduled dialog data has been loaded.
type scheduledDialogDataMsg struct {
	data *scheduledDialogData
}

// scheduledDialogSavedMsg is sent when a scheduled transaction has been saved.
type scheduledDialogSavedMsg struct{}

// Scheduled dialog field indices.
const (
	schedFieldAccount    = 0
	schedFieldPayee      = 1
	schedFieldCategory   = 2
	schedFieldAmount     = 3
	schedFieldMemo       = 4
	schedFieldFrequency  = 5
	schedFieldInterval   = 6
	schedFieldStartDate  = 7
	schedFieldDuration   = 8
	schedFieldEndDate    = 9
	schedFieldOccurrence = 10
	schedFieldAutoPost   = 11
	schedFieldLeadDays   = 12
)

// buildFrequencyOptions returns display names for all frequencies.
func buildFrequencyOptions() []string {
	freqs := scheduled.AllFrequencies()
	options := make([]string, len(freqs))
	for i, f := range freqs {
		options[i] = f.DisplayName()
	}
	return options
}

// frequencyFromIndex returns the Frequency for a given select index.
func frequencyFromIndex(index int) scheduled.Frequency {
	freqs := scheduled.AllFrequencies()
	if index < 0 || index >= len(freqs) {
		return scheduled.FrequencyMonthly
	}
	return freqs[index]
}

// frequencyToIndex returns the select index for a given Frequency.
func frequencyToIndex(f scheduled.Frequency) int {
	freqs := scheduled.AllFrequencies()
	for i, freq := range freqs {
		if freq == f {
			return i
		}
	}
	return 3 // monthly default
}

// durationIndex constants for the radio field.
const (
	durationIndefinite  = 0
	durationUntilDate   = 1
	durationOccurrences = 2
)

// leadDays constants for the radio field.
const (
	leadDaysOnTheDay = 0
	leadDays3Days    = 1
	leadDays1Week    = 2
)

// leadDaysToIndex converts PostLeadDays value to radio index.
func leadDaysToIndex(days int) int {
	switch days {
	case 3:
		return leadDays3Days
	case 7:
		return leadDays1Week
	default:
		return leadDaysOnTheDay
	}
}

// leadDaysFromIndex converts a radio index to PostLeadDays value.
func leadDaysFromIndex(index int) int {
	switch index {
	case leadDays3Days:
		return 3
	case leadDays1Week:
		return 7
	default:
		return 0
	}
}

// buildNewScheduledDialog creates a Dialog for creating a new scheduled transaction.
func buildNewScheduledDialog(accountOptions, categoryOptions []string) *Dialog {
	d := NewDialog("New Scheduled Transaction")
	d.SetWidth(62)

	// Account
	d.AddSelectField("Account", accountOptions, 0)

	// Payee
	d.AddTextField("Payee", "", "Payee name", 0)

	// Category
	d.AddSelectField("Category", categoryOptions, 0)

	// Amount (empty = variable)
	d.AddTextField("Amount", "", "Empty = variable", 12)

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	// Frequency
	d.AddSelectField("Frequency", buildFrequencyOptions(), 3) // Monthly default

	// Interval
	f := d.AddTextField("Interval", "1", "Every N periods", 5)
	f.Required = true

	// Start date
	today := time.Now().Format("01/02/2006")
	f = d.AddDateField("Start Date", today)
	f.Required = true

	// Duration
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, 0)

	// End date (used when Duration = Until Date) — optional, may be blank.
	d.AddOptionalDateField("End Date", "")

	// Occurrences (used when Duration = Occurrences)
	d.AddTextField("Occurrences", "", "Number of times", 5)

	// Auto-post checkbox
	d.AddCheckboxField("Auto-post", false)

	// Lead days radio (only meaningful when auto-post is checked)
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, 0)

	d.SetVisible(true)
	return d
}

// buildEditScheduledDialog creates a Dialog for editing an existing scheduled transaction.
func buildEditScheduledDialog(st *scheduled.Transaction, accountOptions []string, accountIDs []types.ID, categoryOptions []string, categoryIDs []types.ID, payeeNames map[types.ID]string) *Dialog {
	d := NewDialog("Edit Scheduled Transaction")
	d.SetWidth(62)

	// Account - find the matching index
	acctIdx := 0
	for i, id := range accountIDs {
		if id == st.AccountID {
			acctIdx = i
			break
		}
	}
	d.AddSelectField("Account", accountOptions, acctIdx)

	// Payee
	payeeName := ""
	if st.HasPayee() {
		if name, ok := payeeNames[st.PayeeID.ID]; ok {
			payeeName = name
		}
	}
	d.AddTextField("Payee", payeeName, "Payee name", 0)

	// Category - find the matching index
	catIdx := 0
	if st.HasCategory() {
		for i, id := range categoryIDs {
			if id == st.CategoryID.ID {
				catIdx = i
				break
			}
		}
	}
	d.AddSelectField("Category", categoryOptions, catIdx)

	// Amount
	amountStr := ""
	if st.HasAmount() {
		amountStr = fmt.Sprintf("%.2f", st.Amount.Money.Float64())
	}
	d.AddTextField("Amount", amountStr, "Empty = variable", 12)

	// Memo
	memoStr := ""
	if st.Memo.Valid {
		memoStr = st.Memo.String
	}
	d.AddTextField("Memo", memoStr, "Optional memo", 0)

	// Frequency
	d.AddSelectField("Frequency", buildFrequencyOptions(), frequencyToIndex(st.Frequency))

	// Interval
	f := d.AddTextField("Interval", strconv.Itoa(st.Interval), "Every N periods", 5)
	f.Required = true

	// Start date
	f = d.AddDateField("Start Date", st.StartDate.Time().Format("01/02/2006"))
	f.Required = true

	// Duration
	durationIdx := durationIndefinite
	endDateStr := ""
	occurrencesStr := ""
	if st.EndDate.Valid {
		durationIdx = durationUntilDate
		endDateStr = st.EndDate.Date.Time().Format("01/02/2006")
	} else if st.Occurrences.Valid {
		durationIdx = durationOccurrences
		occurrencesStr = strconv.FormatInt(st.Occurrences.Int64, 10)
	}
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, durationIdx)

	// End date — optional, may be blank.
	d.AddOptionalDateField("End Date", endDateStr)

	// Occurrences
	d.AddTextField("Occurrences", occurrencesStr, "Number of times", 5)

	// Auto-post checkbox
	d.AddCheckboxField("Auto-post", st.IsAutoPost())

	// Lead days radio
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, leadDaysToIndex(st.PostLeadDays))

	d.SetVisible(true)
	return d
}

// loadNewScheduledDialogData returns a command that loads data for a new scheduled dialog.
func (a *App) loadNewScheduledDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &scheduledDialogData{
			mode:     scheduledDialogModeNew,
			payeeMap: make(map[string]*payee.Payee),
		}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.payees = payees
			for _, p := range payees {
				data.payeeMap[strings.ToLower(p.Name)] = p
			}
		}

		return scheduledDialogDataMsg{data: data}
	}
}

// loadEditScheduledDialogData returns a command that loads data for editing a scheduled transaction.
func (a *App) loadEditScheduledDialogData() tea.Cmd {
	if a.scheduled == nil || a.scheduledTable == nil {
		return nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return nil
	}

	st := a.scheduled.allTxns[cursor]
	return func() tea.Msg {
		data := &scheduledDialogData{
			mode:      scheduledDialogModeEdit,
			scheduled: st,
			payeeMap:  make(map[string]*payee.Payee),
		}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.payees = payees
			for _, p := range payees {
				data.payeeMap[strings.ToLower(p.Name)] = p
			}
		}

		return scheduledDialogDataMsg{data: data}
	}
}

// closeScheduledDialog clears the scheduled dialog state.
func (a *App) closeScheduledDialog() {
	a.schedDialog = nil
	a.schedDialogData = nil
	a.schedDialogAccountIDs = nil
	a.schedDialogCategoryIDs = nil
}

// handleScheduledDialogKey routes key events to the scheduled dialog.
func (a *App) handleScheduledDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.schedDialog == nil {
		return a, nil
	}

	action := a.schedDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitScheduledDialog()
	case DialogActionCancel:
		a.closeScheduledDialog()
		return a, nil
	}

	return a, nil
}

// submitScheduledDialog parses dialog fields, validates, and saves the scheduled transaction.
func (a *App) submitScheduledDialog() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return a, nil
	}

	fields := a.schedDialog.Fields()
	if len(fields) < 13 {
		return a, nil
	}

	a.schedDialog.ClearErrors()
	hasErrors := false

	// Account
	acctIdx := fields[schedFieldAccount].SelectedIndex
	if acctIdx < 0 || acctIdx >= len(a.schedDialogAccountIDs) {
		fields[schedFieldAccount].Error = "Please select an account"
		hasErrors = true
	}
	accountID := types.NilID
	if acctIdx >= 0 && acctIdx < len(a.schedDialogAccountIDs) {
		accountID = a.schedDialogAccountIDs[acctIdx]
	}

	// Payee name
	payeeName := strings.TrimSpace(fields[schedFieldPayee].Value)

	// Category
	catIdx := fields[schedFieldCategory].SelectedIndex
	var categoryID types.ID
	if catIdx > 0 && catIdx < len(a.schedDialogCategoryIDs) {
		categoryID = a.schedDialogCategoryIDs[catIdx]
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
		if isBlankDateInput(endDateRaw) {
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

	if hasErrors {
		return a, nil
	}

	mode := a.schedDialogData.mode
	existingSched := a.schedDialogData.scheduled

	// Close dialog before async save for responsive UI
	a.closeScheduledDialog()

	return a, func() tea.Msg {
		// Resolve or create payee
		var payeeID types.ID
		if payeeName != "" && a.payeeSvc != nil {
			py, _, err := a.payeeSvc.GetOrCreate(payeeName)
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

			cmd := undo.NewEditScheduledTransactionCommand(a.scheduledTxnSvc, st)
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

			cmd := undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
			}
		}

		return scheduledDialogSavedMsg{}
	}
}
