package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// pendingSplitScheduled carries scalar field values from the scheduled
// dialog into the multi-line split editor. The SplitDialog produces
// transaction.Split rows; submitScheduledSplitDialog translates them
// into scheduled.Split children at save time.
type pendingSplitScheduled struct {
	mode        scheduledDialogMode
	existing    *scheduled.Transaction
	accountID   types.ID
	payeeName   string
	amount      types.Money
	memo        string
	frequency   scheduled.Frequency
	interval    int
	startDate   types.Date
	endDate     types.NullableDate
	occurrences types.NullableInt
	autoPost    bool
	leadDays    int
}

// transactionSplitsFromScheduled converts the children of an existing
// scheduled.Transaction into transaction.Split rows so the SplitDialog
// can seed itself from a previously-saved multi-line template. The
// returned rows carry only the fields the split editor uses
// (category/transfer target, amount, memo).
func transactionSplitsFromScheduled(st *scheduled.Transaction) []*transaction.Split {
	if st == nil || len(st.Splits) == 0 {
		return nil
	}
	rows := make([]*transaction.Split, 0, len(st.Splits))
	for _, sp := range st.Splits {
		t := &transaction.Split{
			BaseModel: types.NewBaseModel(),
			Amount:    sp.Amount,
		}
		if sp.CategoryID.Valid {
			t.CategoryID = sp.CategoryID.ID
		}
		if sp.TransferAccountID.Valid {
			t.TransferAccountID = sp.TransferAccountID
		}
		if sp.Memo.Valid {
			t.SetMemo(sp.Memo.String)
		}
		rows = append(rows, t)
	}
	return rows
}

// scheduledDialogMode indicates whether the dialog is creating or editing.
type scheduledDialogMode int

const (
	scheduledDialogModeNew scheduledDialogMode = iota
	scheduledDialogModeEdit
)

// scheduledDialogData holds the loaded data needed for the scheduled dialog.
type scheduledDialogData struct {
	mode       scheduledDialogMode
	scheduled  *scheduled.Transaction // non-nil when editing
	accounts   []*account.Account
	payees     []*payee.Payee
	payeeMap   map[string]*payee.Payee // lowercase name -> payee
	isTransfer bool                    // true for the single-line transfer dialog
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
	schedFieldSplit      = 13
)

// accountIsAssetByID reports whether the account with the given ID in
// accounts is of type asset specifically (Type == TypeAsset, not the
// broader IsAssetType). Used to decide whether the Value Adjustment
// category is offered in a scheduled-transaction category picker.
func accountIsAssetByID(accounts []*account.Account, id types.ID) bool {
	if id.IsNil() {
		return false
	}
	for _, acc := range accounts {
		if acc != nil && acc.ID == id {
			return acc.Type == account.TypeAsset
		}
	}
	return false
}

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
// Unknown frequencies fall back to the index of FrequencyMonthly in
// AllFrequencies so the dialog opens on a sensible default.
func frequencyToIndex(f scheduled.Frequency) int {
	freqs := scheduled.AllFrequencies()
	for i, freq := range freqs {
		if freq == f {
			return i
		}
	}
	for i, freq := range freqs {
		if freq == scheduled.FrequencyMonthly {
			return i
		}
	}
	return 0
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

// buildNewScheduledDialog creates a dialog.Dialog for creating a new scheduled transaction.
func buildNewScheduledDialog(accountOptions, categoryOptions []string) *dialog.Dialog {
	d := dialog.NewDialog("New Scheduled Transaction")
	d.SetWidth(62)

	// Account
	d.AddSelectField("Account", accountOptions, 0)

	// Payee
	d.AddTextField("Payee", "", "Payee name", 0)

	// Category — typeahead combo so the [+ Add new category…] action row
	// surfaces for inline creation (CC-003).
	catField := d.AddComboField("Category", categoryOptions, 0)
	catField.AddNewLabel = "[+ Add new category…]"

	// Amount (empty = variable)
	d.AddTextField("Amount", "", "Empty = variable", 12)

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	// Frequency
	d.AddSelectField("Frequency", buildFrequencyOptions(), frequencyToIndex(scheduled.FrequencyMonthly))

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

	// Split transaction — toggles the multi-line split editor on Save.
	d.AddCheckboxField("Split transaction", false)

	d.SetVisible(true)
	return d
}

// buildEditScheduledDialog creates a dialog.Dialog for editing an existing scheduled transaction.
func buildEditScheduledDialog(st *scheduled.Transaction, accountOptions []string, accountIDs []types.ID, categoryOptions []string, categoryIDs []types.ID, payeeNames map[types.ID]string) *dialog.Dialog {
	d := dialog.NewDialog("Edit Scheduled Transaction")
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
	editCatField := d.AddComboField("Category", categoryOptions, catIdx)
	editCatField.AddNewLabel = "[+ Add new category…]"

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

	// Split transaction — pre-checked when the schedule already carries
	// child split lines (multi-line template).
	d.AddCheckboxField("Split transaction", len(st.Splits) > 0)

	// "Edit as paycheck →" affordance per MS-029: an alternative entry
	// point that closes this dialog and reopens the schedule in the
	// paycheck wizard with values pre-filled. Visible only when the
	// schedule matches the paycheck heuristic.
	if looksLikePaycheck(st) {
		d.SetButtons([]dialog.DialogButton{
			{Label: "Save", Primary: true},
			{Label: "Cancel"},
			{Label: "Edit as paycheck →", Action: dialog.DialogActionAlternate},
		})
	}

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
			mode:       scheduledDialogModeEdit,
			scheduled:  st,
			payeeMap:   make(map[string]*payee.Payee),
			isTransfer: st.IsTransfer(),
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
	a.schedDialogCategoryOptions = nil
}

// handleScheduledDialogKey routes key events to the scheduled dialog.
func (a *App) handleScheduledDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.schedDialog == nil {
		return a, nil
	}

	action := a.schedDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		if a.schedDialogData != nil && a.schedDialogData.isTransfer {
			return a.submitScheduledTransferDialog()
		}
		return a.submitScheduledDialog()
	case dialog.DialogActionCancel:
		a.closeScheduledDialog()
		return a, nil
	case dialog.DialogActionAlternate:
		return a.relaunchAsPaycheckWizard()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSched()
	}

	// The account picker is editable in this dialog; if the user just
	// switched it to (or away from) an asset account, refresh the
	// category options so Value Adjustment appears/disappears. No-op
	// unless the asset-ness actually changed. Skipped for the transfer
	// dialog, which has no category field.
	if a.schedDialogData != nil && !a.schedDialogData.isTransfer {
		a.refreshSchedCategoryOptionsForAccount()
	}

	return a, nil
}

// schedDialogIncludeValueAdjustment reports whether the scheduled
// dialog's currently selected account is an asset account, and thus
// whether the Value Adjustment category should be offered in its
// category picker.
func (a *App) schedDialogIncludeValueAdjustment() bool {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return false
	}
	fields := a.schedDialog.Fields()
	if len(fields) <= schedFieldAccount {
		return false
	}
	acctIdx := fields[schedFieldAccount].SelectedIndex
	if acctIdx < 0 || acctIdx >= len(a.schedDialogAccountIDs) {
		return false
	}
	return accountIsAssetByID(a.schedDialogData.accounts, a.schedDialogAccountIDs[acctIdx])
}

// refreshSchedCategoryOptionsForAccount rebuilds the scheduled dialog's
// category combo when the selected account's asset-ness has changed
// since the options were last built — surfacing or hiding the Value
// Adjustment category accordingly. It preserves the current category
// selection by ID and is a no-op when nothing changed (so it is cheap
// to call on every keypress).
func (a *App) refreshSchedCategoryOptionsForAccount() {
	if a.schedDialog == nil || a.categorySvc == nil {
		return
	}
	fields := a.schedDialog.Fields()
	if len(fields) <= schedFieldCategory {
		return
	}

	includeVA := a.schedDialogIncludeValueAdjustment()
	hasVA := slices.Contains(a.schedDialogCategoryOptions, category.ValueAdjustmentCategoryName)
	if hasVA == includeVA {
		return
	}

	catField := fields[schedFieldCategory]
	selectedID := types.NilID
	if catField.SelectedIndex >= 0 && catField.SelectedIndex < len(a.schedDialogCategoryIDs) {
		selectedID = a.schedDialogCategoryIDs[catField.SelectedIndex]
	}

	cats, err := a.categorySvc.List()
	if err != nil {
		return
	}
	options, ids := buildCategoryOptionsFor(cats, includeVA)
	a.schedDialogCategoryOptions = options
	a.schedDialogCategoryIDs = ids
	catField.Options = options

	newIdx := 0
	for i, id := range ids {
		if id == selectedID {
			newIdx = i
			break
		}
	}
	catField.SelectedIndex = newIdx
}

// openCreateCategorySubDialogFromSched hides the scheduled dialog and opens
// the inline create-category sub-dialog seeded with the typed query from the
// Category combo. The scheduled dialog's field state is preserved by keeping
// the dialog alive (just hidden) for the duration of the divert; restoration
// on cancel and post-create wiring happens through the createCatDialog
// handlers.
func (a *App) openCreateCategorySubDialogFromSched() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil {
		return a, nil
	}
	fields := a.schedDialog.Fields()
	if len(fields) <= schedFieldCategory {
		return a, nil
	}
	catField := fields[schedFieldCategory]
	query := catField.Query
	catField.AddNewTriggered = false
	catField.Query = ""

	// createCatSource must be set before parentsForCreateCatDialog so the
	// helper picks the right parents source.
	a.createCatSource = createCatSourceSchedDialog
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	defaultType := category.TypeExpense
	if len(fields) > schedFieldAmount {
		defaultType = inferCategoryTypeFromAmount(fields[schedFieldAmount].Value)
	}
	a.createCatDialog = buildCreateCategoryDialog(name, parent, parents, defaultType)
	a.schedDialog.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSched is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the New /
// Edit Scheduled Transaction dialog. It reloads the dialog's category list
// with newCat pre-selected on the Category combo, advances focus to Amount,
// re-shows the scheduled dialog, and clears the create-category sub-dialog.
func (a *App) applyCreatedCategoryToSched(newCat *category.Category, cats []*category.Category) {
	if a.schedDialog == nil {
		a.createCatDialog = nil
		return
	}
	options, ids := buildCategoryOptionsFor(cats, a.schedDialogIncludeValueAdjustment())
	a.schedDialogCategoryIDs = ids
	a.schedDialogCategoryOptions = options

	if len(a.schedDialog.Fields()) > schedFieldCategory {
		catField := a.schedDialog.Fields()[schedFieldCategory]
		catField.Options = options
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		catField.SelectedIndex = newIdx
		// Focus advances to Amount so the user can keep typing.
		a.schedDialog.SetFocusIndex(schedFieldAmount)
		a.schedDialog.SetVisible(true)
	}
	a.createCatDialog = nil
}

// submitScheduledDialog parses dialog fields, validates, and saves the scheduled transaction.
func (a *App) submitScheduledDialog() (tea.Model, tea.Cmd) {
	if a.schedDialog == nil || a.schedDialogData == nil {
		return a, nil
	}

	fields := a.schedDialog.Fields()
	if len(fields) < 14 {
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

	mode := a.schedDialogData.mode
	existingSched := a.schedDialogData.scheduled

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
		categoryIDs := a.schedDialogCategoryIDs
		accountOptions, accountIDs := buildSplitTransferAccountOptions(a.schedDialogData.accounts)

		// Seed the split dialog from existing children when editing a
		// schedule that already carries a multi-line template.
		seedSplits := transactionSplitsFromScheduled(existingSched)
		a.closeScheduledDialog()
		a.pendingSplitScheduled = pending
		if mode == scheduledDialogModeEdit && len(seedSplits) > 0 {
			a.splitDialog = NewSplitDialogFromExisting(amount.Money, categoryOptions, categoryIDs, seedSplits)
		} else {
			a.splitDialog = NewSplitDialog(amount.Money, categoryOptions, categoryIDs)
		}
		a.splitDialog.SetTransferTargets(accountOptions, accountIDs, accountID)
		return a, nil
	}

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

// submitScheduledSplitDialog finalizes a multi-line scheduled transaction
// after the user has filled out the SplitDialog opened from the scheduled
// dialog's Split toggle. The split editor produces transaction.Split rows;
// this handler translates them to scheduled.Split children and dispatches
// the appropriate undo command.
func (a *App) submitScheduledSplitDialog() (tea.Model, tea.Cmd) {
	if a.splitDialog == nil || a.pendingSplitScheduled == nil {
		return a, nil
	}

	splits, err := a.splitDialog.buildSplits()
	if err != nil {
		a.splitDialog.errorMsg = err.Error()
		return a, nil
	}

	pending := a.pendingSplitScheduled
	a.closeSplitDialog()

	return a, func() tea.Msg {
		var payeeID types.ID
		if pending.payeeName != "" && a.payeeSvc != nil {
			py, _, err := a.payeeSvc.GetOrCreate(pending.payeeName)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", err)}
			}
			payeeID = py.ID
		}

		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		children := scheduled.SplitCollection{}
		for _, ts := range splits {
			child := &scheduled.Split{
				BaseModel: types.NewBaseModel(),
				Amount:    ts.Amount,
			}
			switch {
			case ts.TransferAccountID.Valid:
				child.TransferAccountID = ts.TransferAccountID
			default:
				child.CategoryID = types.NullableID{ID: ts.CategoryID, Valid: true}
			}
			if ts.Memo.Valid {
				child.SetMemo(ts.Memo.String)
			}
			children = append(children, child)
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
			cmd := undo.NewEditScheduledTransactionCommand(a.scheduledTxnSvc, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update scheduled transaction: %w", err)}
			}
		} else {
			st := scheduled.NewTransaction(pending.accountID, pending.frequency, pending.startDate)
			applyScalars(st)
			cmd := undo.NewCreateScheduledTransactionCommand(a.scheduledTxnSvc, st)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create scheduled transaction: %w", err)}
			}
		}

		return scheduledDialogSavedMsg{}
	}
}
