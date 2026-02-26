package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/undo"
)

// reconciliationViewData holds the loaded data for the reconciliation view.
type reconciliationViewData struct {
	session       *models.ReconciliationSession
	account       *models.Account
	candidates    []*models.Transaction
	checkedIDs    map[models.ID]bool
	payeeNames    map[models.ID]string
	categoryNames map[models.ID]string
	accountNames  map[models.ID]string
	clearedTotal  models.Money
}

// reconciliationLoadedMsg is sent when reconciliation data has been loaded.
type reconciliationLoadedMsg struct {
	data *reconciliationViewData
}

// reconciliationStartedMsg is sent when a reconciliation session has been started.
type reconciliationStartedMsg struct {
	session *models.ReconciliationSession
	account *models.Account
}

// reconciliationFinishedMsg is sent when reconciliation completes successfully.
type reconciliationFinishedMsg struct{}

// reconciliationCancelledMsg is sent when reconciliation is cancelled.
type reconciliationCancelledMsg struct{}

// reconciliationClearedTotalMsg is sent with an updated cleared total.
type reconciliationClearedTotalMsg struct {
	clearedTotal models.Money
}

// buildStartReconciliationDialog builds the dialog for starting a reconciliation session.
func buildStartReconciliationDialog() *Dialog {
	d := NewDialog("Start Reconciliation")

	today := time.Now().Format("01/02/2006")
	f := d.AddTextField("Statement Date", today, "MM/DD/YYYY", 10)
	f.Required = true

	f = d.AddTextField("Statement Balance", "", "0.00", 12)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Start", Primary: true},
	})

	return d
}

// showStartReconciliationDialog shows the start reconciliation dialog.
func (a *App) showStartReconciliationDialog() {
	a.reconDialog = buildStartReconciliationDialog()
	a.reconDialog.SetVisible(true)
}

// handleReconDialogKey handles key presses in the reconciliation start dialog.
func (a *App) handleReconDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := a.reconDialog.HandleKey(msg)
	switch action {
	case DialogActionCancel:
		a.reconDialog.SetVisible(false)
		a.reconDialog = nil
		return a, nil
	case DialogActionSubmit:
		return a.submitStartReconciliation()
	}
	return a, nil
}

// submitStartReconciliation processes the start reconciliation dialog submission.
func (a *App) submitStartReconciliation() (tea.Model, tea.Cmd) {
	dateStr := a.reconDialog.Fields()[0].Value
	balStr := a.reconDialog.Fields()[1].Value

	// Parse statement date
	statementDate, err := parseDateInput(dateStr)
	if err != nil {
		a.reconDialog.SetErrorMsg("Invalid date format. Use MM/DD/YYYY.")
		return a, nil
	}

	// Parse statement balance
	balance, err := parseAmountInput(balStr)
	if err != nil {
		a.reconDialog.SetErrorMsg("Invalid balance. Enter a number like 1234.56.")
		return a, nil
	}

	accountID := a.sidebar.SelectedAccountID()
	if accountID == models.NilID {
		a.reconDialog.SetErrorMsg("No account selected.")
		return a, nil
	}

	a.reconDialog.SetVisible(false)
	a.reconDialog = nil

	return a, a.startReconciliation(accountID, statementDate, balance)
}

// startReconciliation starts a reconciliation session and loads view data.
func (a *App) startReconciliation(accountID models.ID, statementDate models.Date, statementBalance models.Money) tea.Cmd {
	return func() tea.Msg {
		if a.reconciliationSvc == nil {
			return errMsg{err: fmt.Errorf("reconciliation service not available")}
		}

		session, err := a.reconciliationSvc.StartReconciliation(accountID, statementDate, statementBalance)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to start reconciliation: %w", err)}
		}

		account, err := a.accountSvc.GetByID(accountID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to get account: %w", err)}
		}

		return reconciliationStartedMsg{session: session, account: account}
	}
}

// loadReconciliationData loads reconciliation view data for an active session.
func (a *App) loadReconciliationData(session *models.ReconciliationSession, account *models.Account) tea.Cmd {
	return func() tea.Msg {
		candidates, err := a.reconciliationSvc.GetCandidateTransactions(
			session.AccountID, session.StatementDate,
		)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load candidate transactions: %w", err)}
		}

		// Load payee names
		payeeNames := make(map[models.ID]string)
		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load category names
		categoryNames := make(map[models.ID]string)
		if a.categorySvc != nil {
			categories, err := a.categorySvc.List()
			if err == nil {
				for _, c := range categories {
					categoryNames[c.ID] = c.Name
				}
			}
		}

		// Load account names for transfers
		accountNames := make(map[models.ID]string)
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acct := range accounts {
					accountNames[acct.ID] = acct.Name
				}
			}
		}

		// Calculate initial cleared total (no checked transactions)
		clearedTotal, err := a.reconciliationSvc.CalculateClearedTotal(session.AccountID, nil)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to calculate cleared total: %w", err)}
		}

		data := &reconciliationViewData{
			session:       session,
			account:       account,
			candidates:    candidates,
			checkedIDs:    make(map[models.ID]bool),
			payeeNames:    payeeNames,
			categoryNames: categoryNames,
			accountNames:  accountNames,
			clearedTotal:  clearedTotal,
		}

		return reconciliationLoadedMsg{data: data}
	}
}

// recalculateClearedTotal recalculates the cleared total based on checked transactions.
func (a *App) recalculateClearedTotal() tea.Cmd {
	if a.reconciliation == nil || a.reconciliationSvc == nil {
		return nil
	}

	accountID := a.reconciliation.session.AccountID
	checkedIDs := a.getCheckedTransactionIDs()

	return func() tea.Msg {
		total, err := a.reconciliationSvc.CalculateClearedTotal(accountID, checkedIDs)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to recalculate cleared total: %w", err)}
		}
		return reconciliationClearedTotalMsg{clearedTotal: total}
	}
}

// getCheckedTransactionIDs returns a slice of checked transaction IDs.
func (a *App) getCheckedTransactionIDs() []models.ID {
	if a.reconciliation == nil {
		return nil
	}
	var ids []models.ID
	for id, checked := range a.reconciliation.checkedIDs {
		if checked {
			ids = append(ids, id)
		}
	}
	return ids
}

// buildReconciliationTable creates and populates the table for the reconciliation view.
func (a *App) buildReconciliationTable() {
	if a.reconciliation == nil {
		return
	}

	columns := []Column{
		{Header: " ", Width: 3, Align: AlignCenter},   // Checkbox
		{Header: "Date", Width: 10, Align: AlignLeft},
		{Header: "S", Width: 1, Align: AlignCenter},    // Cleared indicator
		{Header: "Payee", MinWidth: 12, Align: AlignLeft},
		{Header: "Category", MinWidth: 10, Align: AlignLeft},
		{Header: "Amount", Width: 12, Align: AlignRight},
	}

	if a.reconciliationTable == nil {
		a.reconciliationTable = NewTable(columns)
	} else {
		a.reconciliationTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.reconciliation.candidates))
	for i, txn := range a.reconciliation.candidates {
		rows[i] = a.formatReconciliationRow(txn)
	}
	a.reconciliationTable.SetRows(rows)
	a.reconciliationTable.SetFocused(true)
}

// formatReconciliationRow formats a transaction into a reconciliation table row.
func (a *App) formatReconciliationRow(txn *models.Transaction) []string {
	// Checkbox
	checkbox := "[ ]"
	if a.reconciliation.checkedIDs[txn.ID] {
		checkbox = "[✓]"
	}

	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Cleared indicator
	status := " "
	if txn.Status == models.TransactionStatusCleared {
		status = "✓"
	}

	// Payee
	payee := ""
	if txn.IsTransfer() {
		if name, ok := a.reconciliation.accountNames[txn.TransferAccountID.ID]; ok {
			payee = "Transfer: " + name
		} else {
			payee = "Transfer"
		}
	} else if txn.HasPayee() {
		if name, ok := a.reconciliation.payeeNames[txn.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Category
	category := ""
	if txn.HasCategory() {
		if name, ok := a.reconciliation.categoryNames[txn.CategoryID.ID]; ok {
			category = name
		}
	} else if txn.IsTransfer() {
		category = "[Transfer]"
	}

	// Amount
	amount := formatDashboardMoney(txn.Amount)

	return []string{checkbox, dateStr, status, payee, category, amount}
}

// updateReconciliationCheckboxes refreshes the table rows to reflect checkbox state.
func (a *App) updateReconciliationCheckboxes() {
	if a.reconciliationTable == nil || a.reconciliation == nil {
		return
	}
	rows := make([][]string, len(a.reconciliation.candidates))
	for i, txn := range a.reconciliation.candidates {
		rows[i] = a.formatReconciliationRow(txn)
	}
	a.reconciliationTable.SetRows(rows)
}

// renderReconciliation renders the reconciliation view.
func (a *App) renderReconciliation() string {
	if a.reconciliation == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading reconciliation...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Header: RECONCILE: <account name>    Statement Date: <date>
	acctName := a.reconciliation.account.Name
	stmtDate := a.reconciliation.session.StatementDate.Time().Format("01/02/2006")
	leftHeader := "RECONCILE: " + acctName
	rightHeader := "Statement Date: " + stmtDate
	padding := max(contentWidth-lipgloss.Width(leftHeader)-lipgloss.Width(rightHeader)-4, 1)
	headerRow := a.styles.Title.Render(leftHeader) + strings.Repeat(" ", padding) + a.styles.Muted.Render(rightHeader)
	sections = append(sections, headerRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	footerHeight := 3  // separator + balance line + hint line
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.reconciliationTable != nil && len(a.reconciliation.candidates) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.reconciliationTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.reconciliationTable.ScrollInfo(tableHeight - 1); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.reconciliation.candidates) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No unreconciled transactions found"))
	}

	// Sticky footer
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	stmtBal := formatDashboardMoney(a.reconciliation.session.StatementBalance)
	clrTotal := formatDashboardMoney(a.reconciliation.clearedTotal)
	difference := a.reconciliation.session.StatementBalance.Sub(a.reconciliation.clearedTotal)
	diffStr := formatDashboardMoney(difference)

	// Color the difference
	diffStyle := a.styles.Positive
	if !difference.IsZero() {
		diffStyle = a.styles.Negative
	}

	totalCount := len(a.reconciliation.candidates)
	checkedCount := 0
	for _, checked := range a.reconciliation.checkedIDs {
		if checked {
			checkedCount++
		}
	}

	balanceLine := fmt.Sprintf("Statement: %s  Cleared: %s  Difference: ", stmtBal, clrTotal)
	balanceLine = a.styles.Bold.Render(balanceLine) + diffStyle.Render(diffStr)
	sections = append(sections, balanceLine)

	countLine := fmt.Sprintf("Checked: %d of %d", checkedCount, totalCount)
	hintLine := countLine + "    Space toggle  Enter finish  Esc cancel  a all  u uncheck"
	sections = append(sections, a.styles.Muted.Render(hintLine))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// handleReconciliationKeys handles key presses in the reconciliation view.
func (a *App) handleReconciliationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.reconciliation == nil || a.reconciliationTable == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.reconciliationTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.reconciliationTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.reconciliationTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.reconciliationTable.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-10, 1)
		a.reconciliationTable.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-10, 1)
		a.reconciliationTable.PageDown(tableHeight)
	case msg.String() == " ":
		// Toggle checkbox on selected transaction
		return a.toggleReconciliationCheck()
	case msg.String() == "a":
		// Check all
		return a.checkAllReconciliation()
	case msg.String() == "u":
		// Uncheck all
		return a.uncheckAllReconciliation()
	case key.Matches(msg, a.keys.Enter):
		// Finish reconciliation
		return a.finishReconciliation()
	case key.Matches(msg, a.keys.Escape):
		// Cancel reconciliation
		return a.cancelReconciliation()
	}

	return a, nil
}

// toggleReconciliationCheck toggles the checked state of the selected transaction.
func (a *App) toggleReconciliationCheck() (tea.Model, tea.Cmd) {
	cursor := a.reconciliationTable.Cursor()
	if cursor < 0 || cursor >= len(a.reconciliation.candidates) {
		return a, nil
	}

	txn := a.reconciliation.candidates[cursor]
	a.reconciliation.checkedIDs[txn.ID] = !a.reconciliation.checkedIDs[txn.ID]
	if !a.reconciliation.checkedIDs[txn.ID] {
		delete(a.reconciliation.checkedIDs, txn.ID)
	}

	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// checkAllReconciliation checks all candidate transactions.
func (a *App) checkAllReconciliation() (tea.Model, tea.Cmd) {
	for _, txn := range a.reconciliation.candidates {
		a.reconciliation.checkedIDs[txn.ID] = true
	}
	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// uncheckAllReconciliation unchecks all candidate transactions.
func (a *App) uncheckAllReconciliation() (tea.Model, tea.Cmd) {
	a.reconciliation.checkedIDs = make(map[models.ID]bool)
	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// finishReconciliation attempts to finish the reconciliation session.
func (a *App) finishReconciliation() (tea.Model, tea.Cmd) {
	if a.reconciliation == nil {
		return a, nil
	}

	// Check difference first (before needing the service)
	difference := a.reconciliation.session.StatementBalance.Sub(a.reconciliation.clearedTotal)
	if !difference.IsZero() {
		diffStr := formatDashboardMoney(difference)
		a.statusbar.AddNotification(
			fmt.Sprintf("Cannot finish: difference is %s (must be $0.00)", diffStr),
			NotificationAlert,
		)
		return a, nil
	}

	if a.reconciliationSvc == nil || a.undoManager == nil {
		return a, nil
	}

	accountID := a.reconciliation.session.AccountID
	txnIDs := a.getCheckedTransactionIDs()

	return a, func() tea.Msg {
		cmd := undo.NewFinishReconciliationCommand(
			a.reconciliationSvc, a.transactionSvc, accountID, txnIDs,
		)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to finish reconciliation: %w", err)}
		}
		return reconciliationFinishedMsg{}
	}
}

// cancelReconciliation cancels the current reconciliation session.
func (a *App) cancelReconciliation() (tea.Model, tea.Cmd) {
	if a.reconciliation == nil || a.reconciliationSvc == nil {
		return a, nil
	}

	accountID := a.reconciliation.session.AccountID

	return a, func() tea.Msg {
		err := a.reconciliationSvc.CancelReconciliation(accountID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to cancel reconciliation: %w", err)}
		}
		return reconciliationCancelledMsg{}
	}
}
