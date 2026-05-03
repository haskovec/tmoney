package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transactionDialogData holds the loaded data needed for the transaction dialog.
type transactionDialogData struct {
	payees     []*payee.Payee
	categories []*category.Category
	payeeMap   map[string]*payee.Payee // lowercase name -> payee
}

// transactionDialogDataMsg is sent when transaction dialog data has been loaded.
type transactionDialogDataMsg struct {
	data *transactionDialogData
}

// transactionDialogSavedMsg is sent when a transaction has been saved.
type transactionDialogSavedMsg struct{}

// parseDateInput parses a date string in MM/DD/YYYY format.
func parseDateInput(input string) (types.Date, error) {
	t, err := time.Parse("01/02/2006", input)
	if err != nil {
		return types.ZeroDate, fmt.Errorf("invalid date format (expected MM/DD/YYYY): %w", err)
	}
	return types.NewDate(t.Year(), t.Month(), t.Day()), nil
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

// buildCategoryOptions builds parallel display name and ID slices for the category selector.
// First entry is "(None)" with a nil ID. Subcategories are formatted as "Parent > Child".
// System categories are excluded. Results are sorted alphabetically.
func buildCategoryOptions(categories []*category.Category) ([]string, []types.ID) {
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
		if c.IsSystem {
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

// buildTransactionDialog creates a Dialog for entering a new transaction.
func buildTransactionDialog(data *transactionDialogData, categoryOptions []string) *Dialog {
	d := NewDialog("New Transaction")

	// Date field - default to today in MM/DD/YYYY
	today := time.Now().Format("01/02/2006")
	f := d.AddTextField("Date", today, "MM/DD/YYYY", 10)
	f.Required = true

	// Payee
	d.AddTextField("Payee", "", "Payee name", 0)

	// Category
	d.AddSelectField("Category", categoryOptions, 0)

	// Amount
	f = d.AddTextField("Amount", "", "-50.00", 12)
	f.Required = true

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	// Status
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, 0)

	// Split transaction checkbox
	d.AddCheckboxField("Split transaction", false)

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
	case DialogActionSubmit:
		return a.submitTransactionDialog()
	case DialogActionCancel:
		a.closeTransactionDialog()
		return a, nil
	}

	// Check for payee auto-fill after text input
	if a.txnDialog.FocusIndex() == 1 {
		a.checkPayeeAutoFill()
	}

	return a, nil
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

	if isSplit {
		// Save pending split transaction data and open split editor
		a.pendingSplitTxn = &pendingSplitTransaction{
			accountID: accountID,
			date:      date,
			payeeName: payeeName,
			amount:    amount,
			memo:      memo,
			status:    status,
		}

		// Build category options for the split dialog (reuse loaded data)
		categoryOptions, categoryIDs := buildCategoryOptions(a.txnDialogData.categories)

		// Close the transaction dialog
		a.closeTransactionDialog()

		// Open split dialog
		a.splitDialog = NewSplitDialog(amount, categoryOptions, categoryIDs)

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

		return transactionDialogSavedMsg{}
	}
}
