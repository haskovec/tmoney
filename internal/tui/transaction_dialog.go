package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transactionDialogMode distinguishes a New-Transaction dialog from an
// Edit-Transaction dialog. The two share field layout and key handling but
// differ in title, pre-fill, and the submit command they dispatch.
type transactionDialogMode int

const (
	transactionDialogModeNew transactionDialogMode = iota
	transactionDialogModeEdit
)

// transactionDialogData holds the loaded data needed for the transaction dialog.
type transactionDialogData struct {
	payees     []*payee.Payee
	categories []*category.Category
	accounts   []*account.Account
	payeeMap   map[string]*payee.Payee // lowercase name -> payee

	// Edit-mode-only fields. All zero in new mode.
	mode           transactionDialogMode
	existing       *transaction.Transaction
	existingSplits []*transaction.Split
}

// transactionDialogDataMsg is sent when transaction dialog data has been loaded.
type transactionDialogDataMsg struct {
	data *transactionDialogData
}

// transactionDialogSavedMsg is sent when a transaction has been saved.
// savedDate carries the date of the saved transaction so the App can use it
// as the sticky seed for the next dialog open.
type transactionDialogSavedMsg struct {
	savedDate types.Date
	// savedID is the ID of the saved transaction so the register can move the
	// cursor onto its row after reload.
	savedID types.ID
}

// parseDateInput parses a date string in MM/DD/YYYY format.
func parseDateInput(input string) (types.Date, error) {
	t, err := time.Parse("01/02/2006", input)
	if err != nil {
		return types.ZeroDate, fmt.Errorf("invalid date format (expected MM/DD/YYYY): %w", err)
	}
	return types.NewDate(t.Year(), t.Month(), t.Day()), nil
}

// openingDateFieldError returns the inline date-field error to show when d
// precedes acct's opening date, mirroring the service-layer guard so the user
// can fix the date in the dialog rather than after a failed submit. Returns ""
// when the date is acceptable (or acct is nil).
func openingDateFieldError(acct *account.Account, d types.Date) string {
	if acct == nil {
		return ""
	}
	if err := acct.ValidateTransactionDate(d); err != nil {
		return fmt.Sprintf("Before %s opened (%s)", acct.Name, acct.OpeningDate.Time().Format("01/02/2006"))
	}
	return ""
}

// investmentDialogOpeningDateError applies openingDateFieldError to the account
// backing the active investment register (the account every investment entry
// dialog operates on).
func (a *App) investmentDialogOpeningDateError(d types.Date) string {
	if a.investmentRegister == nil || a.investmentRegister.account == nil {
		return ""
	}
	return openingDateFieldError(a.investmentRegister.account, d)
}

// parseAmountInput parses a money string, stripping "$" and handling negatives.
func parseAmountInput(input string) (types.Money, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return types.ZeroMoney, fmt.Errorf("amount is required")
	}

	// Handle negative with $ sign: -$50.00 or $-50.00
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	s = strings.TrimPrefix(s, "$")
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}

	if negative {
		s = "-" + s
	}

	return types.NewMoney(s)
}

// splitCategoryQuery splits a typed category query on the first ':' into a
// (parent, name) pair, trimming whitespace on each side. A query without a
// ':' is treated as a plain child name with no parent. A leading ':' is
// treated as malformed and reduces to (parent="", name=<rest>) so the user
// gets the same UX as having typed only the child name.
func splitCategoryQuery(q string) (parent, name string) {
	q = strings.TrimSpace(q)
	before, after, ok := strings.Cut(q, ":")
	if !ok {
		return "", q
	}
	parent = strings.TrimSpace(before)
	name = strings.TrimSpace(after)
	return parent, name
}

// buildSplitTransferAccountOptions builds parallel display name and ID
// slices for the split-row transfer-target picker. Closed accounts are
// skipped; the caller is expected to pass the parent transaction's
// account ID to SplitDialog.SetTransferTargets so self-transfers are
// filtered out at that stage.
func buildSplitTransferAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))
	for _, a := range accounts {
		if a == nil || !a.Active {
			continue
		}
		options = append(options, a.Name)
		ids = append(ids, a.ID)
	}
	return options, ids
}

// buildCategoryOptions builds parallel display name and ID slices for the category selector.
// First entry is "(None)" with a nil ID. Subcategories are formatted as "Parent > Child".
// System categories are excluded. Results are sorted alphabetically.
func buildCategoryOptions(categories []*category.Category) ([]string, []types.ID) {
	return buildCategoryOptionsFor(categories, false)
}

// buildCategoryOptionsForAccount is buildCategoryOptions with the Value
// Adjustment system category surfaced when acct is of type asset
// specifically (Type == TypeAsset, not the broader IsAssetType). A nil
// account behaves like buildCategoryOptions (all system categories
// hidden).
func buildCategoryOptionsForAccount(categories []*category.Category, acct *account.Account) ([]string, []types.ID) {
	return buildCategoryOptionsFor(categories, acct != nil && acct.Type == account.TypeAsset)
}

// buildCategoryOptionsFor is buildCategoryOptions with an escape hatch:
// when includeValueAdjustment is true the system Value Adjustment
// category is surfaced (for asset-account pickers). Every other system
// category — Transfer in particular — stays hidden regardless, because
// the exception is scoped to that one name.
func buildCategoryOptionsFor(categories []*category.Category, includeValueAdjustment bool) ([]string, []types.ID) {
	options := []string{"(None)"}
	ids := []types.ID{types.NilID}

	// Build parent name map
	parentNames := make(map[types.ID]string)
	for _, c := range categories {
		if c.IsTopLevel() && !c.IsSystem {
			parentNames[c.ID] = c.Name
		}
	}

	type catEntry struct {
		name string
		id   types.ID
	}
	var entries []catEntry

	for _, c := range categories {
		// System categories are hidden, with one exception: the Value
		// Adjustment category is surfaced for asset-account pickers.
		valueAdjustmentException := includeValueAdjustment && c.Name == category.ValueAdjustmentCategoryName
		if c.IsSystem && !valueAdjustmentException {
			continue
		}
		var displayName string
		if c.IsSubcategory() {
			if parentName, ok := parentNames[c.ParentID.ID]; ok {
				displayName = parentName + " > " + c.Name
			} else {
				displayName = c.Name
			}
		} else {
			displayName = c.Name
		}
		entries = append(entries, catEntry{name: displayName, id: c.ID})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	for _, e := range entries {
		options = append(options, e.name)
		ids = append(ids, e.id)
	}

	return options, ids
}

// buildTransactionDialog creates a dialog.Dialog for entering or editing a transaction.
//
// In new mode (data.mode == transactionDialogModeNew, the zero value), the
// dialog is titled "New Transaction" and the Date field is seeded from
// seedDate (or today when seedDate is zero); other fields start empty.
//
// In edit mode (data.mode == transactionDialogModeEdit, data.existing
// non-nil), the dialog is titled "Edit Transaction" and every field is
// pre-filled from data.existing — Date, Payee (resolved via data.payees),
// Category (resolved via the parallel categoryIDs slice), Amount, Memo, and
// Status. seedDate is ignored in edit mode.
func buildTransactionDialog(data *transactionDialogData, categoryOptions []string, categoryIDs []types.ID, seedDate types.Date) *dialog.Dialog {
	editing := data != nil && data.mode == transactionDialogModeEdit && data.existing != nil

	title := "New Transaction"
	if editing {
		title = "Edit Transaction"
	}
	d := dialog.NewDialog(title)

	// Date — pre-filled from existing in edit mode, otherwise from sticky
	// seed or today.
	var dateStr string
	switch {
	case editing:
		dateStr = data.existing.Date.Time().Format("01/02/2006")
	case seedDate.IsZero():
		dateStr = time.Now().Format("01/02/2006")
	default:
		dateStr = seedDate.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Date", dateStr)
	f.Required = true

	// Payee — pre-filled by looking up existing.PayeeID in data.payees.
	payeeValue := ""
	if editing && data.existing.PayeeID.Valid {
		for _, p := range data.payees {
			if p.ID == data.existing.PayeeID.ID {
				payeeValue = p.Name
				break
			}
		}
	}
	d.AddTextField("Payee", payeeValue, "Payee name", 0)

	// Category — resolve SelectedIndex from existing.CategoryID against
	// the parallel categoryIDs slice.
	catIdx := 0
	if editing && data.existing.CategoryID.Valid {
		for i, id := range categoryIDs {
			if id == data.existing.CategoryID.ID {
				catIdx = i
				break
			}
		}
	}
	catField := d.AddComboField("Category", categoryOptions, catIdx)
	catField.AddNewLabel = "[+ Add new category…]"

	// Amount
	amountValue := ""
	if editing {
		amountValue = data.existing.Amount.String()
	}
	f = d.AddTextField("Amount", amountValue, "-50.00", 12)
	f.Required = true

	// Memo
	memoValue := ""
	if editing && data.existing.Memo.Valid {
		memoValue = data.existing.Memo.String
	}
	d.AddTextField("Memo", memoValue, "Optional memo", 0)

	// Status — radio: 0 = Uncleared, 1 = Cleared.
	statusIdx := 0
	if editing && data.existing.Status == transaction.StatusCleared {
		statusIdx = 1
	}
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, statusIdx)

	// Split transaction checkbox — pre-checked when editing a transaction
	// that already carries splits.
	splitChecked := editing && len(data.existingSplits) > 0
	d.AddCheckboxField("Split transaction", splitChecked)

	d.SetVisible(true)
	return d
}

// loadTransactionDialogData returns a command that loads payees and categories for the dialog.
func (a *App) loadTransactionDialogData() tea.Cmd {
	return func() tea.Msg {
		data := &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
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

		if a.categorySvc != nil {
			categories, err := a.categorySvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.categories = categories
		}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		return transactionDialogDataMsg{data: data}
	}
}

// loadEditTransactionDialogData returns a command that loads payees,
// categories, and the existing transaction identified by txnID, then emits a
// transactionDialogDataMsg with mode = transactionDialogModeEdit so the
// transaction dialog opens pre-filled for editing.
func (a *App) loadEditTransactionDialogData(txnID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
			mode:     transactionDialogModeEdit,
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

		if a.categorySvc != nil {
			categories, err := a.categorySvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			data.categories = categories
		}

		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			data.accounts = accounts
		}

		if a.transactionSvc != nil {
			txn, err := a.transactionSvc.GetByID(txnID)
			if err != nil {
				return errMsg{err: err}
			}
			data.existing = txn

			splits, err := a.transactionSvc.GetSplits(txnID)
			if err != nil {
				return errMsg{err: err}
			}
			data.existingSplits = splits
		}

		return transactionDialogDataMsg{data: data}
	}
}

// closeTransactionDialog clears the transaction dialog state.
func (a *App) closeTransactionDialog() {
	a.txnDialog = nil
	a.txnDialogData = nil
	a.txnDialogCategoryIDs = nil
}

// checkPayeeAutoFill checks if the current payee field matches a known payee
// and auto-fills the category dropdown if that payee has a default category.
func (a *App) checkPayeeAutoFill() {
	if a.txnDialog == nil || a.txnDialogData == nil {
		return
	}

	// Get the payee field (index 1)
	fields := a.txnDialog.Fields()
	if len(fields) < 2 {
		return
	}
	payeeField := fields[1]
	payeeName := strings.ToLower(strings.TrimSpace(payeeField.Value))
	if payeeName == "" {
		return
	}

	py, ok := a.txnDialogData.payeeMap[payeeName]
	if !ok || !py.HasDefaultCategory() {
		return
	}

	// Find the category index
	defaultCatID := py.DefaultCategoryID.ID
	for i, catID := range a.txnDialogCategoryIDs {
		if catID == defaultCatID {
			// Category field is at index 2
			if len(fields) > 2 {
				fields[2].SelectedIndex = i
				// Keep the combo's highlight cursor in sync with SelectedIndex
				// so the dropdown highlights the auto-filled row when opened.
				fields[2].ComboHighlight = i
			}
			return
		}
	}
}

// handleTransactionDialogKey routes key events to the transaction dialog.
func (a *App) handleTransactionDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.txnDialog == nil {
		return a, nil
	}

	action := a.txnDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitTransactionDialog()
	case dialog.DialogActionCancel:
		a.closeTransactionDialog()
		return a, nil
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialog()
	}

	// Check for payee auto-fill after text input
	if a.txnDialog.FocusIndex() == 1 {
		a.checkPayeeAutoFill()
	}

	return a, nil
}

// openCreateCategorySubDialog hides the transaction dialog and opens the
// inline create-category sub-dialog seeded with the typed query from the
// Category combo. The transaction dialog's field state is preserved by
// keeping the dialog instance alive (just hidden) for the duration of the
// divert; restoration on cancel and post-create wiring happens through the
// createCatDialog handlers.
func (a *App) openCreateCategorySubDialog() (tea.Model, tea.Cmd) {
	if a.txnDialog == nil {
		return a, nil
	}
	fields := a.txnDialog.Fields()
	if len(fields) < 3 {
		return a, nil
	}
	catField := fields[2]
	query := catField.Query
	// Consume the trigger and clear the typed query — the create-category
	// dialog now owns it. This way, when we restore the txn dialog, its
	// Category combo doesn't carry stale typed text.
	catField.AddNewTriggered = false
	catField.Query = ""

	var parents []string
	if a.txnDialogData != nil {
		parents = topLevelParentNames(a.txnDialogData.categories)
	}
	parent, name := splitCategoryQuery(query)
	defaultType := category.TypeExpense
	if len(fields) > 3 {
		defaultType = inferCategoryTypeFromAmount(fields[3].Value)
	}
	a.createCatDialog = buildCreateCategoryDialog(name, parent, parents, defaultType)
	a.createCatSource = createCatSourceTxnDialog
	a.txnDialog.SetVisible(false)
	return a, nil
}

// topLevelParentNames returns the names of non-system top-level categories.
// Used to populate the Parent combo in the create-category sub-dialog.
// Returns an empty slice for an empty or nil input.
func topLevelParentNames(categories []*category.Category) []string {
	var names []string
	for _, c := range categories {
		if c.IsSystem || !c.IsTopLevel() {
			continue
		}
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}

// handleCreateCatDialogKey routes key events to the create-category
// sub-dialog. Esc closes the sub-dialog and re-shows the transaction dialog
// with all field state preserved. Submit produces the
// createCategoryRequestMsg that the App.Update path consumes.
func (a *App) handleCreateCatDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.createCatDialog == nil {
		return a, nil
	}
	action := a.createCatDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitCreateCatDialog()
	case dialog.DialogActionCancel:
		a.cancelCreateCatDialog()
		return a, nil
	}
	return a, nil
}

// cancelCreateCatDialog closes the sub-dialog and re-shows whichever
// originating transaction-entry surface is current. All originating-dialog
// field state is preserved (the dialog was hidden, not destroyed). Focus is
// left wherever it was (Category field). The createCatSource discriminator
// is reset so a future open from a different surface starts in a clean
// state.
func (a *App) cancelCreateCatDialog() {
	a.createCatDialog = nil
	switch a.createCatSource {
	case createCatSourceTxnDialog:
		if a.txnDialog != nil {
			a.txnDialog.SetVisible(true)
		}
	case createCatSourceSchedDialog:
		if a.schedDialog != nil {
			a.schedDialog.SetVisible(true)
		}
	case createCatSourceSchedPreview:
		if a.schedPreviewDialog != nil {
			if header := a.schedPreviewDialog.HeaderDialog(); header != nil {
				header.SetVisible(true)
			}
		}
	case createCatSourceSplitDialog:
		if a.splitDialog != nil {
			a.splitDialog.SetVisible(true)
		}
		a.createCatSplitRow = -1
	case createCatSourcePaycheckWizard:
		if a.paycheckWizard != nil {
			a.paycheckWizard.SetVisible(true)
		}
		a.createCatPaycheckLine = nil
	}
	a.createCatSource = createCatSourceNone
}

// submitCreateCatDialog validates the sub-dialog and, on success, returns a
// command that emits a createCategoryRequestMsg the App.Update path consumes
// to persist the category and reopen the transaction dialog. Validation
// failures keep the sub-dialog open with inline errors set.
func (a *App) submitCreateCatDialog() (tea.Model, tea.Cmd) {
	if a.createCatDialog == nil {
		return a, nil
	}
	parents := a.parentsForCreateCatDialog()
	cmd := submitCreateCategoryDialog(a.createCatDialog, parents)
	if cmd == nil {
		return a, nil
	}
	return a, cmd
}

// parentsForCreateCatDialog returns the existing top-level parent names that
// should be presented to the create-category sub-dialog's parent combo. The
// list is sourced from whichever transaction-entry surface opened the sub-
// dialog; for surfaces that don't cache categories (e.g. the schedule
// preview dialog) it falls back to a live categorySvc.List() call. An empty
// slice is returned when no source matches and no service is available.
func (a *App) parentsForCreateCatDialog() []string {
	switch a.createCatSource {
	case createCatSourceTxnDialog:
		if a.txnDialogData != nil {
			return topLevelParentNames(a.txnDialogData.categories)
		}
	}
	if a.categorySvc != nil {
		if cats, err := a.categorySvc.List(); err == nil {
			return topLevelParentNames(cats)
		}
	}
	return nil
}

// applyCreatedCategoryToTxn is the per-surface applier called by the
// createCategoryRequestMsg router when the originating surface was the New /
// Edit Transaction dialog. It reloads the dialog's category list with newCat
// pre-selected on the Category combo, advances focus to Amount, re-shows the
// transaction dialog, and clears the create-category sub-dialog. The actual
// persistence happens in persistCategory; the router has already called it
// and passes the freshly-created category in.
func (a *App) applyCreatedCategoryToTxn(newCat *category.Category, cats []*category.Category) {
	if a.txnDialogData != nil {
		a.txnDialogData.categories = cats
	}
	options, ids := buildCategoryOptionsForAccount(cats, a.sidebar.SelectedAccount())
	a.txnDialogCategoryIDs = ids

	if a.txnDialog != nil && len(a.txnDialog.Fields()) >= 3 {
		catField := a.txnDialog.Fields()[2]
		catField.Options = options
		newIdx := 0
		for i, id := range ids {
			if id == newCat.ID {
				newIdx = i
				break
			}
		}
		catField.SelectedIndex = newIdx
		// Focus advances to Amount (field index 3).
		a.txnDialog.SetFocusIndex(3)
		a.txnDialog.SetVisible(true)
	}
	a.createCatDialog = nil
}

// submitTransactionDialog parses dialog fields, validates, and saves the transaction.
func (a *App) submitTransactionDialog() (tea.Model, tea.Cmd) {
	if a.txnDialog == nil || a.txnDialogData == nil {
		return a, nil
	}

	fields := a.txnDialog.Fields()
	if len(fields) < 7 {
		return a, nil
	}

	a.txnDialog.ClearErrors()
	hasErrors := false

	// Parse date
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	} else if acct := a.sidebar.SelectedAccount(); acct != nil {
		if msg := openingDateFieldError(acct, date); msg != "" {
			fields[0].Error = msg
			hasErrors = true
		}
	}

	// Payee name
	payeeName := strings.TrimSpace(fields[1].Value)

	// Category
	catIdx := fields[2].SelectedIndex
	var categoryID types.ID
	if catIdx > 0 && catIdx < len(a.txnDialogCategoryIDs) {
		categoryID = a.txnDialogCategoryIDs[catIdx]
	}

	// Parse amount
	amount, err := parseAmountInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid amount"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo
	memo := strings.TrimSpace(fields[4].Value)

	// Status
	status := transaction.StatusUncleared
	if fields[5].SelectedIndex == 1 {
		status = transaction.StatusCleared
	}

	// Split transaction checkbox
	isSplit := fields[6].Checked

	// Get account ID from sidebar
	accountID := a.sidebar.SelectedAccountID()

	editing := a.txnDialogData.mode == transactionDialogModeEdit && a.txnDialogData.existing != nil
	var existing *transaction.Transaction
	hadSplits := false
	if editing {
		existing = a.txnDialogData.existing
		hadSplits = len(a.txnDialogData.existingSplits) > 0
	}

	if isSplit {
		// Save pending split transaction data and open split editor
		pending := &pendingSplitTransaction{
			accountID: accountID,
			date:      date,
			payeeName: payeeName,
			amount:    amount,
			memo:      memo,
			status:    status,
		}
		if editing {
			pending.existing = existing
		}
		a.pendingSplitTxn = pending

		// Build category options for the split dialog (reuse loaded data)
		categoryOptions, categoryIDs := buildCategoryOptions(a.txnDialogData.categories)
		seedSplits := a.txnDialogData.existingSplits
		accountOptions, accountIDs := buildSplitTransferAccountOptions(a.txnDialogData.accounts)

		// Close the transaction dialog
		a.closeTransactionDialog()

		// Open split dialog — seeded with existing splits when editing a
		// transaction that already had splits.
		if editing && hadSplits {
			a.splitDialog = NewSplitDialogFromExisting(amount, categoryOptions, categoryIDs, seedSplits)
		} else {
			a.splitDialog = NewSplitDialog(amount, categoryOptions, categoryIDs)
		}
		a.splitDialog.SetTransferTargets(accountOptions, accountIDs, accountID)

		return a, nil
	}

	// Close dialog before async save for responsive UI
	a.closeTransactionDialog()

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

		if editing {
			updated := *existing
			updated.Date = date
			updated.Amount = amount
			updated.Status = status
			updated.SetMemo(memo)
			if !payeeID.IsNil() {
				updated.PayeeID = types.NullableID{ID: payeeID, Valid: true}
			} else {
				updated.PayeeID = types.NullableID{Valid: false}
			}
			if !categoryID.IsNil() {
				updated.CategoryID = types.NullableID{ID: categoryID, Valid: true}
			} else {
				updated.CategoryID = types.NullableID{Valid: false}
			}

			if a.transactionSvc != nil && a.undoManager != nil {
				// Use the splits-aware edit command when the prior state had
				// splits — this clears them as part of the same undo unit
				// (split→plain conversion). Otherwise the plain edit command
				// is sufficient.
				var cmd undo.Command
				if hadSplits {
					cmd = undo.NewEditTransactionWithSplitsCommand(a.transactionSvc, &updated, nil)
				} else {
					cmd = undo.NewEditTransactionCommand(a.transactionSvc, &updated)
				}
				if err := a.undoManager.Execute(cmd); err != nil {
					return errMsg{err: fmt.Errorf("failed to save transaction: %w", err)}
				}
			}
			return transactionDialogSavedMsg{savedDate: date, savedID: updated.ID}
		}

		// Build transaction
		txn := transaction.NewTransactionFull(accountID, date, amount, payeeID, categoryID, memo)
		txn.Status = status

		// Save via undo manager
		if a.transactionSvc != nil && a.undoManager != nil {
			cmd := undo.NewCreateTransactionCommand(a.transactionSvc, txn)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to save transaction: %w", err)}
			}
		}

		return transactionDialogSavedMsg{savedDate: date, savedID: txn.ID}
	}
}
