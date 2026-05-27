package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// registerData holds the loaded data for the account register view.
type registerData struct {
	account       *account.Account
	transactions  []*transaction.Transaction
	balance       *account.Balance
	payeeNames    map[types.ID]string
	categoryNames map[types.ID]string
	accountNames  map[types.ID]string
}

// registerLoadedMsg is sent when register data has been loaded.
type registerLoadedMsg struct {
	data *registerData
}

// loadRegisterData returns a command that loads all data needed for the register view.
func (a *App) loadRegisterData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &registerData{
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		}

		// Load account
		if a.accountSvc != nil {
			acct, err := a.accountSvc.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct

			// Load balance
			bal, err := a.accountSvc.GetBalance(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.balance = bal

			// Load account names for transfer display
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		// Load transactions
		if a.transactionSvc != nil {
			txns, err := a.transactionSvc.ListByAccount(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.transactions = txns
		}

		// Load payee names
		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load category names
		if a.categorySvc != nil {
			categories, err := a.categorySvc.List()
			if err == nil {
				for _, c := range categories {
					data.categoryNames[c.ID] = c.Name
				}
			}
		}

		return registerLoadedMsg{data: data}
	}
}

// handleRegisterKeys handles key presses in the register view.
func (a *App) handleRegisterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.table != nil {
				a.table.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.table != nil {
				a.table.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// widget.Table-focused key handling
	if a.table == nil || a.register == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.table.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.table.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.table.PageDown(tableHeight)
	case msg.String() == "c":
		return a.toggleTransactionStatus()
	case msg.String() == "v":
		return a.showVoidConfirmation()
	case key.Matches(msg, a.keys.Delete):
		return a.showDeleteConfirmation()
	case msg.String() == "r":
		a.showStartReconciliationDialog()
		return a, nil
	case key.Matches(msg, a.keys.New):
		return a, a.loadTransactionDialogData()
	case msg.String() == "t":
		return a, a.loadTransferDialogData()
	case key.Matches(msg, a.keys.Enter):
		return a.openEditTransactionFlow()
	}

	return a, nil
}

// openEditTransactionFlow dispatches Enter on a register row to the
// appropriate edit flow based on the selected transaction's kind. Void and
// reconciled rows are surfaced as a status-bar notification rather than
// opened — un-void/un-reconcile first if you actually want to edit. Plain
// rows kick off loadEditTransactionDialogData. Transfer rows route to the
// transfer-edit flow (Phase 2). Split rows route to the split-edit flow
// (Phase 3).
func (a *App) openEditTransactionFlow() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	if txn.IsVoid() {
		a.statusbar.AddNotification("Cannot edit void transaction", widget.NotificationAlert)
		return a, nil
	}
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot edit reconciled transaction (un-reconcile first)", widget.NotificationAlert)
		return a, nil
	}

	if txn.IsTransfer() {
		return a, a.loadEditTransferDialogData(txn.ID)
	}

	return a, a.loadEditTransactionDialogData(txn.ID)
}

// toggleTransactionStatus toggles the cleared/uncleared status of the selected transaction.
func (a *App) toggleTransactionStatus() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	// Cannot toggle void transactions
	if txn.IsVoid() {
		a.statusbar.AddNotification("Cannot change status of void transaction", widget.NotificationAlert)
		return a, nil
	}

	// Cannot toggle reconciled transactions
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot change status of reconciled transaction (un-reconcile first)", widget.NotificationAlert)
		return a, nil
	}

	accountID := a.sidebar.SelectedAccountID()
	txnID := txn.ID
	currentStatus := txn.Status

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		// Get current state from DB for the edit command
		current, err := a.transactionSvc.GetByID(txnID)
		if err != nil {
			return errMsg{err: err}
		}

		// Build updated copy with toggled status
		updated := *current
		if currentStatus == transaction.StatusCleared {
			updated.MarkUncleared()
		} else {
			updated.Clear()
		}

		cmd := undo.NewEditTransactionCommand(a.transactionSvc, &updated)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}

		// Reload register data to reflect the change
		return a.loadRegisterData(accountID)()
	}
}

// showVoidConfirmation shows a confirmation dialog before voiding the selected transaction.
func (a *App) showVoidConfirmation() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	// Cannot void already-void transactions
	if txn.IsVoid() {
		a.statusbar.AddNotification("Transaction is already void", widget.NotificationAlert)
		return a, nil
	}

	// Cannot void reconciled transactions
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot void reconciled transaction (un-reconcile first)", widget.NotificationAlert)
		return a, nil
	}

	// Build confirmation message
	msg := "Void this transaction? Amount will be set to $0.00 and memo replaced with **VOID**."
	if txn.IsTransfer() {
		msg = "Void this transfer? Both sides will be voided. Amount will be set to $0.00."
	}

	accountID := a.sidebar.SelectedAccountID()
	txnID := txn.ID

	isTransfer := txn.IsTransfer()

	a.showConfirmDialog("Void Transaction", msg, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			cmd = undo.NewVoidTransferCommand(a.transactionSvc, txnID)
		} else {
			cmd = undo.NewVoidTransactionCommand(a.transactionSvc, txnID)
		}
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return a.loadRegisterData(accountID)()
	})

	return a, nil
}

// showDeleteConfirmation shows a confirmation dialog before hard-deleting the
// selected transaction. Delete is distinct from void: it removes the row(s)
// entirely rather than zeroing them out. Runs through the undo manager so
// Ctrl+Z restores the deletion.
func (a *App) showDeleteConfirmation() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	// Cannot delete void transactions (Service.Delete rejects with IsVoidError).
	// Surface a status-bar notification before opening the dialog the user can't complete.
	if txn.IsVoid() {
		a.statusbar.AddNotification("Cannot delete void transaction", widget.NotificationAlert)
		return a, nil
	}

	// Cannot delete reconciled transactions (Service.Delete rejects with IsReconciledError).
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot delete reconciled transaction (un-reconcile first)", widget.NotificationAlert)
		return a, nil
	}

	msg := "Delete this transaction? This cannot be undone except via Ctrl+Z."
	if txn.IsTransfer() {
		msg = "Delete this transfer? Both sides will be removed."
	}

	accountID := a.sidebar.SelectedAccountID()
	txnID := txn.ID
	isTransfer := txn.IsTransfer()

	a.showConfirmDialog("Delete Transaction", msg, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			cmd = undo.NewDeleteTransferCommand(a.transactionSvc, txn.TransferID.ID)
		} else {
			cmd = undo.NewDeleteTransactionCommand(a.transactionSvc, txnID)
		}
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return a.loadRegisterData(accountID)()
	})

	return a, nil
}

// buildRegisterTable creates and populates the table for the register view.
func (a *App) buildRegisterTable() {
	if a.register == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Date", Width: 10, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter},
		{Header: "Payee", MinWidth: 12, Align: widget.AlignLeft},
		{Header: "Category", MinWidth: 10, Align: widget.AlignLeft},
		{Header: "Amount", Width: 12, Align: widget.AlignRight},
	}

	if a.table == nil {
		a.table = widget.NewTable(columns)
	} else {
		a.table.SetColumns(columns)
	}

	rows := make([][]string, len(a.register.transactions))
	for i, txn := range a.register.transactions {
		rows[i] = a.formatRegisterRow(txn)
	}
	a.table.SetRows(rows)

	// After a save, move the cursor onto the just-saved row by matching its
	// transaction ID. Selecting by ID (not position) keeps the cursor on the
	// row even when it sorts into the middle of the list, e.g. a back-dated
	// entry. Cleared after applying so unrelated reloads don't move the cursor.
	if !a.pendingRegisterSelectID.IsNil() {
		for i, txn := range a.register.transactions {
			if txn.ID == a.pendingRegisterSelectID {
				a.table.SetCursor(i)
				break
			}
		}
		a.pendingRegisterSelectID = types.NilID
	}

	// Apply void row styling
	for i, txn := range a.register.transactions {
		if txn.IsVoid() {
			a.table.SetRowStyle(i, widget.RowStyleVoid)
		}
	}
}

// formatRegisterRow formats a transaction into table row strings.
func (a *App) formatRegisterRow(txn *transaction.Transaction) []string {
	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Status indicator
	status := " "
	switch txn.Status {
	case transaction.StatusCleared:
		status = "✓"
	case transaction.StatusReconciled:
		status = "R"
	case transaction.StatusVoid:
		status = "V"
	}

	// Payee
	payee := ""
	if txn.IsTransfer() {
		if name, ok := a.register.accountNames[txn.TransferAccountID.ID]; ok {
			payee = "Transfer: " + name
		} else {
			payee = "Transfer"
		}
	} else if txn.HasPayee() {
		if name, ok := a.register.payeeNames[txn.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Category
	category := ""
	if txn.HasCategory() {
		if name, ok := a.register.categoryNames[txn.CategoryID.ID]; ok {
			category = name
		}
	} else if txn.IsTransfer() {
		category = "[Transfer]"
	}

	// Amount
	amount := formatDashboardMoney(txn.Amount)

	return []string{dateStr, status, payee, category, amount}
}

// renderRegister renders the account register view.
func (a *App) renderRegister() string {
	if a.register == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading register...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: account name + balance
	acctName := strings.ToUpper(a.register.account.Name)
	balStr := ""
	if a.register.balance != nil {
		balStr = "Bal: " + formatDashboardMoney(a.register.balance.CurrentBalance)
	}
	// widget.Truncate account name if it would overflow available space
	maxNameWidth := max(
		// 4 padding + 2 gap
		contentWidth-lipgloss.Width(balStr)-6, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(balStr)-4, 1)

	balStyle := a.styles.Positive
	if a.register.balance != nil && a.register.balance.CurrentBalance.IsNegative() {
		balStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + balStyle.Render(balStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2      // title + separator
	paddingHeight := 2    // top/bottom padding
	scrollInfoHeight := 1 // reserve a row for the scroll info line so a long list doesn't overflow the status bar
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight-scrollInfoHeight, 1)

	if a.table != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.table.Render(a.styles, tableWidth, tableHeight))
		if info := a.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.register.transactions) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}
