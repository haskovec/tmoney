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

// registerViewState is everything the account register view owns. Its zero
// value is the view before its first load.
type registerViewState struct {
	data  *registerData
	table *widget.Table
	// withBalance is the column set with the running-balance column, its
	// number columns fitted to the loaded rows. buildRegisterTable sets it,
	// and the resize-time fit check uses it.
	withBalance []widget.Column
	// shown holds the transaction of each table row, in table order. It is
	// all the transactions, or under a search only the ones that match.
	// buildRegisterTable sets it; every action on "the selected row" reads
	// it through selectedTransaction.
	shown []*transaction.Transaction
	// The search (the `/` key, or Transactions → Search…). While searching is
	// true every key edits the query. Enter keeps the query and gives the
	// keys back to the register; Esc clears it. See register_search.go.
	searching bool
	query     string
}

// selectedTransaction returns the transaction on the cursor row, or nil.
func (s *registerViewState) selectedTransaction() *transaction.Transaction {
	if s.table == nil {
		return nil
	}
	cursor := s.table.Cursor()
	if cursor < 0 || cursor >= len(s.shown) {
		return nil
	}
	return s.shown[cursor]
}

// clearForLoad drops the register's data before another account loads into
// it, together with the search: a search belongs to the account it was typed
// in.
func (s *registerViewState) clearForLoad() {
	s.data = nil
	s.resetSearch()
}

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
	fileOf
}

// loadRegisterData returns a command that loads all data needed for the register view.
func (a *App) loadRegisterData(accountID types.ID) tea.Cmd {
	svc := a.services
	file := a.fileOfOpen()
	return func() tea.Msg {
		if accountID.IsNil() {
			return nil // no account selected, as after a file switch
		}
		data := &registerData{
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		}

		// Load account
		if svc.Account != nil {
			acct, err := svc.Account.GetByID(accountID)
			if err != nil {
				return errMsg{err: err, fileOf: file}
			}
			data.account = acct

			// Load balance
			bal, err := svc.Account.GetBalance(accountID)
			if err != nil {
				return errMsg{err: err, fileOf: file}
			}
			data.balance = bal

			// Load account names for transfer display
			accounts, err := svc.Account.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		// Load transactions
		if svc.Transaction != nil {
			txns, err := svc.Transaction.ListByAccount(accountID)
			if err != nil {
				return errMsg{err: err, fileOf: file}
			}
			data.transactions = txns
		}

		// Load payee names
		if svc.Payee != nil {
			payees, err := svc.Payee.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load category names
		if svc.Category != nil {
			categories, err := svc.Category.List()
			if err == nil {
				for _, c := range categories {
					data.categoryNames[c.ID] = c.Name
				}
			}
		}

		return registerLoadedMsg{data: data, fileOf: file}
	}
}

// handleRegisterKeys handles key presses in the register view.
func (a *App) handleRegisterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While a search is typed, every key edits it (handleKeyPress routes the
	// keys here first, so the global keys do not take them).
	if a.register.searching {
		a.handleRegisterSearchKey(msg)
		return a, nil
	}
	// Esc clears a kept search before anything else: with the sidebar
	// focused it would otherwise go to the sidebar, which ignores it.
	if key.Matches(msg, a.keys.Escape) && a.register.searchActive() {
		a.clearRegisterSearch()
		return a, nil
	}

	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.register.table != nil {
				a.register.table.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.register.table != nil {
				a.register.table.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// widget.Table-focused key handling
	if a.register.table == nil || a.register.data == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.register.table.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.register.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.register.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.register.table.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.register.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.register.table.PageDown(tableHeight)
	case key.Matches(msg, a.keys.Search):
		// A search only reads, so it works on a closed account too.
		a.startRegisterSearch()
		return a, nil
	case msg.String() == "a":
		// Amortization drill-in — loan accounts only (read-only, so it works
		// on a closed loan too). A no-op on non-loan accounts.
		if a.register.data.account != nil && a.register.data.account.Type == account.TypeLoan {
			a.switchView(ViewAmortization)
			return a, a.amortization.load(a.amortizationDeps(), a.register.data.account.ID)
		}
		return a, nil
	case a.register.data.account != nil && a.register.data.account.IsClosed() &&
		(msg.String() == "c" || msg.String() == "v" || msg.String() == "t" || msg.String() == "r" ||
			key.Matches(msg, a.keys.Delete) || key.Matches(msg, a.keys.New) || key.Matches(msg, a.keys.Enter)):
		// A closed account is frozen: navigation still works, but every
		// mutating action is a no-op with an explanatory toast.
		a.statusbar.AddNotification(closedAccountChangesNotice, widget.NotificationAlert)
		return a, nil
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
		return a, a.transfer.open(a.transferDeps())
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
	if a.register.table == nil || a.register.data == nil {
		return a, nil
	}

	txn := a.register.selectedTransaction()
	if txn == nil {
		return a, nil
	}

	if txn.IsVoid() {
		a.statusbar.AddNotification("Cannot edit void transaction", widget.NotificationAlert)
		return a, nil
	}
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot edit reconciled transaction (un-reconcile first)", widget.NotificationAlert)
		return a, nil
	}

	if txn.IsTransfer() {
		return a, a.transfer.openForEdit(a.transferDeps(), txn.ID)
	}

	return a, a.loadEditTransactionDialogData(txn.ID)
}

// toggleTransactionStatus toggles the cleared/uncleared status of the selected transaction.
func (a *App) toggleTransactionStatus() (tea.Model, tea.Cmd) {
	if a.register.table == nil || a.register.data == nil || a.services.Transaction == nil {
		return a, nil
	}

	txn := a.register.selectedTransaction()
	if txn == nil {
		return a, nil
	}

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
	isTransfer := txn.IsTransfer()

	// The toggled-to status, computed the same way for both paths.
	nextStatus := transaction.StatusCleared
	if currentStatus == transaction.StatusCleared {
		nextStatus = transaction.StatusUncleared
	}

	svc := a.services
	undoManager := a.undoManager
	reload := a.loadRegisterData(accountID) // made now; runs after the write
	return a, func() tea.Msg {
		if undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			// A transfer leg's status is owned by the transfer service, which
			// writes only the status column of the one leg. Clearing your side
			// says your bank posted it, which is independent of the other
			// account's side, so the sibling leg is deliberately untouched.
			//
			// This used to go through EditTransactionCommand →
			// transaction.Service.Update, which rewrites the whole row and has
			// no transfer-aware path at all.
			if svc.Transfer == nil {
				return errMsg{err: fmt.Errorf("transfer service not available")}
			}
			cmd = undo.NewSetTransferLegStatusCommand(svc.Transfer, txnID, nextStatus)
		} else {
			current, err := svc.Transaction.GetByID(txnID)
			if err != nil {
				return errMsg{err: err}
			}
			updated := *current
			if currentStatus == transaction.StatusCleared {
				updated.MarkUncleared()
			} else {
				updated.Clear()
			}
			cmd = undo.NewEditTransactionCommand(svc.Transaction, &updated)
		}

		if err := undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}

		// Reload register data to reflect the change
		return reload()
	}
}

// showVoidConfirmation shows a confirmation dialog before voiding the selected transaction.
func (a *App) showVoidConfirmation() (tea.Model, tea.Cmd) {
	if a.register.table == nil || a.register.data == nil || a.services.Transaction == nil {
		return a, nil
	}

	txn := a.register.selectedTransaction()
	if txn == nil {
		return a, nil
	}

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
	var transferID types.ID
	if isTransfer {
		transferID = txn.TransferID.ID
	}

	svc := a.services
	undoManager := a.undoManager
	reload := a.loadRegisterData(accountID) // made now; runs after the write
	a.showConfirmDialog("Void Transaction", msg, func() tea.Msg {
		if undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			// Addressed by transfer_id rather than by a leg's row id: the void
			// applies to the whole transfer, and an investment-involving one is
			// refused by name instead of failing with "expected 2 transactions".
			cmd = undo.NewVoidTransferCommand(svc.Transfer, transferID)
		} else {
			cmd = undo.NewVoidTransactionCommand(svc.Transaction, txnID)
		}
		if err := undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return reload()
	})

	return a, nil
}

// showDeleteConfirmation shows a confirmation dialog before hard-deleting the
// selected transaction. Delete is distinct from void: it removes the row(s)
// entirely rather than zeroing them out. Runs through the undo manager so
// Ctrl+Z restores the deletion.
func (a *App) showDeleteConfirmation() (tea.Model, tea.Cmd) {
	if a.register.table == nil || a.register.data == nil || a.services.Transaction == nil {
		return a, nil
	}

	txn := a.register.selectedTransaction()
	if txn == nil {
		return a, nil
	}

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

	undoManager := a.undoManager
	svc := a.services
	reload := a.loadRegisterData(accountID) // made now; runs after the write
	a.showConfirmDialog("Delete Transaction", msg, func() tea.Msg {
		if undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			cmd = undo.NewDeleteTransferCommand(svc.Transfer, txn.TransferID.ID)
		} else {
			cmd = undo.NewDeleteTransactionCommand(svc.Transaction, txnID)
		}
		if err := undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return reload()
	})

	return a, nil
}

// registerColumns returns the regular register's column set at its usual
// widths; when withBalance is true it appends the trailing running-balance
// column. buildRegisterTable fits the number columns to the rows.
func registerColumns(withBalance bool) []widget.Column {
	cols := []widget.Column{
		{Header: "Date", Width: 10, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter},
		{Header: "Payee", MinWidth: 12, Align: widget.AlignLeft},
		{Header: "Category", MinWidth: 10, Align: widget.AlignLeft},
		{Header: "Amount", Width: 12, Align: widget.AlignRight},
	}
	if withBalance {
		cols = append(cols, widget.Column{Header: "Balance", Width: balanceColWidth, Align: widget.AlignRight})
	}
	return cols
}

// shouldShowRegisterBalance reports whether the register is currently wide
// enough to include the running-balance column, with the Amount and Balance
// columns as wide as the loaded rows need. The table width here must match
// renderRegister's (ContentWidth - 4).
func (a *App) shouldShowRegisterBalance() bool {
	tableWidth := max(a.styles.ContentWidth()-4, 1)
	return columnsFitWidth(a.register.withBalance, tableWidth, registerFlexMargin)
}

// buildRegisterTable creates and populates the table for the register view.
func (a *App) buildRegisterTable() {
	if a.register.data == nil {
		return
	}

	opening := types.ZeroMoney
	if a.register.data.account != nil {
		opening = a.register.data.account.OpeningBalance
	}
	// The running balance is the account's balance after each row, so it is
	// computed over every row and stays true on the rows a search shows.
	balances := runningBalances(a.register.data.transactions, opening)
	query := strings.ToLower(strings.TrimSpace(a.register.query))
	var rows [][]string
	a.register.shown = a.register.shown[:0]
	for i, txn := range a.register.data.transactions {
		cells := a.register.formatRow(txn)
		if query != "" && !a.register.rowMatches(txn, cells, query) {
			continue
		}
		rows = append(rows, append(cells, accountMoney(balances[i], a.register.data.account)))
		a.register.shown = append(a.register.shown, txn)
	}

	// Running-balance column — shown only when the register is wide enough to
	// fit it without squeezing Payee/Category below a readable width. The
	// Amount and Balance columns are as wide as their widest amount.
	a.register.withBalance = fitNumberColumns(registerColumns(true), rows)
	columns := a.register.withBalance
	if !a.shouldShowRegisterBalance() {
		columns = columns[:len(columns)-1]
		for i := range rows {
			rows[i] = rows[i][:len(columns)]
		}
	}

	if a.register.table == nil {
		a.register.table = widget.NewTable(columns)
	} else {
		a.register.table.SetColumns(columns)
	}
	a.register.table.SetRows(rows)

	// Apply void row styling
	for i, txn := range a.register.shown {
		if txn.IsVoid() {
			a.register.table.SetRowStyle(i, widget.RowStyleVoid)
		}
	}
}

// registerAmountCol is the Amount cell's index in a formatRow row.
const registerAmountCol = 4

// formatRow formats a transaction into table row strings: date, status, payee,
// category, amount.
func (s *registerViewState) formatRow(txn *transaction.Transaction) []string {
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
		if name, ok := s.data.accountNames[txn.TransferAccountID.ID]; ok {
			payee = "Transfer: " + name
		} else {
			payee = "Transfer"
		}
	} else if txn.HasPayee() {
		if name, ok := s.data.payeeNames[txn.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Category
	category := ""
	if txn.HasCategory() {
		if name, ok := s.data.categoryNames[txn.CategoryID.ID]; ok {
			category = name
		}
	} else if txn.IsTransfer() {
		category = "[Transfer]"
	}

	// Amount
	amount := accountMoney(txn.Amount, s.data.account)

	return []string{dateStr, status, payee, category, amount}
}

// registerShowsClosedBanner reports whether the register draws the read-only
// banner row under its title. renderRegister and tableContentRowOffset both
// use it, so the mouse math follows the render.
func (a *App) registerShowsClosedBanner() bool {
	return a.cashRegisterClosed()
}

// renderRegister renders the account register view.
func (a *App) renderRegister() string {
	if a.register.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading register...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: account name + balance
	acctName := strings.ToUpper(a.register.data.account.Name)
	balStr := ""
	if a.register.data.balance != nil {
		balStr = "Bal: " + accountMoney(a.register.data.balance.CurrentBalance, a.register.data.account)
	}
	// widget.Truncate account name if it would overflow available space
	maxNameWidth := max(
		// 4 padding + 2 gap
		contentWidth-lipgloss.Width(balStr)-6, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(balStr)-4, 1)

	balStyle := a.styles.Positive
	if a.register.data.balance != nil && a.register.data.balance.CurrentBalance.IsNegative() {
		balStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + balStyle.Render(balStr)
	sections = append(sections, titleRow)

	// Closed-account banner: a closed account's register is read-only.
	closedBanner := a.registerShowsClosedBanner()
	if closedBanner {
		label := "Closed · read-only"
		if a.register.data.account.ClosedDate.Valid {
			label = "Closed " + a.register.data.account.ClosedDate.Date.String() + " · read-only"
		}
		sections = append(sections, a.styles.Muted.Render(label))
	}
	if a.register.searchActive() {
		sections = append(sections, a.styles.Muted.Render(a.register.searchLine(max(contentWidth-4, 1))))
	}

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 // title + separator
	if closedBanner {
		titleHeight++ // account for the read-only banner line
	}
	if a.register.searchActive() {
		titleHeight++ // the search line
	}
	paddingHeight := 2    // top/bottom padding
	scrollInfoHeight := 1 // reserve a row for the scroll info line so a long list doesn't overflow the status bar
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight-scrollInfoHeight, 1)

	if a.register.table != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.register.table.Render(a.styles, tableWidth, tableHeight))
		if info := a.register.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.register.data.transactions) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}
