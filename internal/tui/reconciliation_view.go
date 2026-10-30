package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// reconciliationViewState is everything the Reconciliation view owns. Its zero
// value is the view with no session on screen.
type reconciliationViewState struct {
	data  *reconciliationViewData
	table *widget.Table
}

// reconciliationViewData holds the loaded data for the reconciliation view.
type reconciliationViewData struct {
	session       *reconciliation.Session
	account       *account.Account
	candidates    []*transaction.Transaction
	checkedIDs    map[types.ID]bool
	payeeNames    map[types.ID]string
	categoryNames map[types.ID]string
	accountNames  map[types.ID]string
	clearedTotal  types.Money
}

// reconciliationLoadedMsg is sent when reconciliation data has been loaded.
type reconciliationLoadedMsg struct {
	data *reconciliationViewData
	// reload is true for a refresh of the session already on screen, and
	// false for the first load after a start.
	reload bool
	fileOf
}

// reconciliationStartedMsg is sent when a reconciliation session has been started.
type reconciliationStartedMsg struct {
	session *reconciliation.Session
	account *account.Account
	fileOf
}

// reconciliationFinishedMsg is sent when reconciliation completes successfully.
type reconciliationFinishedMsg struct{ fileOf }

// reconciliationCancelledMsg is sent when reconciliation is cancelled.
type reconciliationCancelledMsg struct{ fileOf }

// reconciliationClearedTotalMsg is sent with an updated cleared total.
type reconciliationClearedTotalMsg struct {
	clearedTotal types.Money
	fileOf
}

// buildStartReconciliationDialog builds the dialog for starting a
// reconciliation session. seedDate seeds the Statement Date field; pass the
// zero value to leave the field blank (first open in a session).
func buildStartReconciliationDialog(seedDate types.Date) *dialog.Dialog {
	d := dialog.NewDialog("Start Reconciliation")

	dateStr := ""
	if !seedDate.IsZero() {
		dateStr = seedDate.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Statement Date", dateStr)
	f.Required = true

	f = d.AddTextField("Statement Balance", "", "0.00", 12)
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Start", Primary: true},
		{Label: "Cancel"},
	})

	return d
}

// refuseInvestmentReconcile shows a notice and returns true when the selected
// account is an investment account. Reconcile reads only the register ledger,
// and StartReconciliation refuses these accounts whether open or closed, so a
// caller that checks this first never opens a statement dialog to reject, and
// never tells the user to reopen an account that still cannot be reconciled.
func (a *App) refuseInvestmentReconcile() bool {
	if a.sidebar == nil {
		return false
	}
	acct := a.sidebar.SelectedAccount()
	if acct == nil || !acct.Type.IsInvestmentType() {
		return false
	}
	a.statusbar.AddNotification(
		fmt.Sprintf("%s is an investment account. Reconcile applies to register accounts only.", acct.Name),
		widget.NotificationAlert,
	)
	return true
}

// showStartReconciliationDialog shows the start reconciliation dialog, seeded
// with the statement date from the last successful Start in this session. An
// investment account gets refuseInvestmentReconcile's notice instead.
func (a *App) showStartReconciliationDialog() {
	if a.refuseInvestmentReconcile() {
		return
	}
	a.reconDialog = buildStartReconciliationDialog(a.reconDialogLastStatementDate)
	a.reconDialog.SetVisible(true)
}

// handleReconDialogKey handles key presses in the reconciliation start dialog.
func (a *App) handleReconDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.reconDialogAction(a.reconDialog.HandleKey(msg))
}

// reconDialogAction dispatches a DialogAction for the recon dialog, from either input path.
func (a *App) reconDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.reconDialog.SetVisible(false)
		a.reconDialog = nil
		return a, nil
	case dialog.DialogActionSubmit:
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
	if accountID == types.NilID {
		a.reconDialog.SetErrorMsg("No account selected.")
		return a, nil
	}

	a.reconDialog.SetVisible(false)
	a.reconDialog = nil
	a.reconDialogLastStatementDate = statementDate

	return a, a.startReconciliation(accountID, statementDate, balance)
}

// startReconciliation starts a reconciliation session and loads view data.
func (a *App) startReconciliation(accountID types.ID, statementDate types.Date, statementBalance types.Money) tea.Cmd {
	svc := a.services
	file := a.fileOfOpen()
	return func() tea.Msg {
		if svc.Reconciliation == nil {
			return errMsg{err: fmt.Errorf("reconciliation service not available"), fileOf: file}
		}

		session, err := svc.Reconciliation.StartReconciliation(accountID, statementDate, statementBalance)
		if err != nil {
			// A write failed: show it also after a switch (no tag).
			return errMsg{err: fmt.Errorf("failed to start reconciliation: %w", err)}
		}

		acct, err := svc.Account.GetByID(accountID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to get account: %w", err), fileOf: file}
		}

		return reconciliationStartedMsg{session: session, account: acct, fileOf: file}
	}
}

// loadReconciliationData loads reconciliation view data for an active session.
func (a *App) loadReconciliationData(session *reconciliation.Session, account *account.Account) tea.Cmd {
	svc := a.services
	file := a.fileOfOpen()
	return func() tea.Msg {
		candidates, err := svc.Reconciliation.GetCandidateTransactions(
			session.AccountID, session.StatementDate,
		)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load candidate transactions: %w", err), fileOf: file}
		}

		// Load payee names
		payeeNames := make(map[types.ID]string)
		if svc.Payee != nil {
			payees, err := svc.Payee.List()
			if err == nil {
				for _, p := range payees {
					payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load category names
		categoryNames := make(map[types.ID]string)
		if svc.Category != nil {
			categories, err := svc.Category.List()
			if err == nil {
				for _, c := range categories {
					categoryNames[c.ID] = c.Name
				}
			}
		}

		// Load account names for transfers
		accountNames := make(map[types.ID]string)
		if svc.Account != nil {
			accounts, err := svc.Account.List(true)
			if err == nil {
				for _, acct := range accounts {
					accountNames[acct.ID] = acct.Name
				}
			}
		}

		// Calculate initial cleared total (no checked transactions)
		clearedTotal, err := svc.Reconciliation.CalculateClearedTotal(session.AccountID, nil)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to calculate cleared total: %w", err), fileOf: file}
		}

		data := &reconciliationViewData{
			session:       session,
			account:       account,
			candidates:    candidates,
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    payeeNames,
			categoryNames: categoryNames,
			accountNames:  accountNames,
			clearedTotal:  clearedTotal,
		}

		return reconciliationLoadedMsg{data: data, fileOf: file}
	}
}

// reloadReconciliationData reloads the session on screen. Its message is
// applied by applyReconciliationReload, which keeps the check marks.
func (a *App) reloadReconciliationData(r *reconciliationViewData) tea.Cmd {
	load := a.loadReconciliationData(r.session, r.account)
	return func() tea.Msg {
		msg := load()
		if loaded, ok := msg.(reconciliationLoadedMsg); ok {
			loaded.reload = true
			return loaded
		}
		return msg
	}
}

// applyReconciliationReload swaps fresh candidates into the session on
// screen. It runs on the update loop and reads the live check marks, not a
// copy taken when the load began, so a toggle made while the load was in
// flight survives. A reload for a session no longer on screen is dropped.
func (a *App) applyReconciliationReload(data *reconciliationViewData) tea.Cmd {
	cur := a.reconciliation.data
	if a.currentView != ViewReconciliation || cur == nil || cur.session == nil ||
		data.session == nil || cur.session.ID != data.session.ID {
		return nil
	}
	for _, txn := range data.candidates {
		if cur.checkedIDs[txn.ID] {
			data.checkedIDs[txn.ID] = true
		}
	}
	// The loader's total counts no marks; show the last total until the
	// recalculation for the kept marks lands.
	data.clearedTotal = cur.clearedTotal
	a.reconciliation.data = data
	a.buildReconciliationTable()
	return a.recalculateClearedTotal()
}

// recalculateClearedTotal recalculates the cleared total based on checked transactions.
func (a *App) recalculateClearedTotal() tea.Cmd {
	if a.reconciliation.data == nil || a.services.Reconciliation == nil {
		return nil
	}

	accountID := a.reconciliation.data.session.AccountID
	checkedIDs := a.getCheckedTransactionIDs()

	svc := a.services
	file := a.fileOfOpen()
	return func() tea.Msg {
		total, err := svc.Reconciliation.CalculateClearedTotal(accountID, checkedIDs)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to recalculate cleared total: %w", err), fileOf: file}
		}
		return reconciliationClearedTotalMsg{clearedTotal: total, fileOf: file}
	}
}

// getCheckedTransactionIDs returns a slice of checked transaction IDs.
func (a *App) getCheckedTransactionIDs() []types.ID {
	if a.reconciliation.data == nil {
		return nil
	}
	var ids []types.ID
	for id, checked := range a.reconciliation.data.checkedIDs {
		if checked {
			ids = append(ids, id)
		}
	}
	return ids
}

// buildReconciliationTable creates and populates the table for the reconciliation view.
func (a *App) buildReconciliationTable() {
	if a.reconciliation.data == nil {
		return
	}

	rows := make([][]string, len(a.reconciliation.data.candidates))
	for i, txn := range a.reconciliation.data.candidates {
		rows[i] = a.formatReconciliationRow(txn)
	}

	columns := fitNumberColumns([]widget.Column{
		{Header: " ", Width: 3, Align: widget.AlignCenter}, // Checkbox
		{Header: "Date", Width: 10, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter}, // Cleared indicator
		{Header: "Payee", MinWidth: 12, Align: widget.AlignLeft},
		{Header: "Category", MinWidth: 10, Align: widget.AlignLeft},
		{Header: "Amount", Width: 12, Align: widget.AlignRight},
	}, rows)

	if a.reconciliation.table == nil {
		a.reconciliation.table = widget.NewTable(columns)
	} else {
		a.reconciliation.table.SetColumns(columns)
	}
	a.reconciliation.table.SetRows(rows)
	a.reconciliation.table.SetFocused(true)
}

// formatReconciliationRow formats a transaction into a reconciliation table row.
func (a *App) formatReconciliationRow(txn *transaction.Transaction) []string {
	// Checkbox
	checkbox := "[ ]"
	if a.reconciliation.data.checkedIDs[txn.ID] {
		checkbox = "[✓]"
	}

	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Cleared indicator
	status := " "
	if txn.Status == transaction.StatusCleared {
		status = "✓"
	}

	// Payee
	payee := ""
	if txn.IsTransfer() {
		if name, ok := a.reconciliation.data.accountNames[txn.TransferAccountID.ID]; ok {
			payee = "Transfer: " + name
		} else {
			payee = "Transfer"
		}
	} else if txn.HasPayee() {
		if name, ok := a.reconciliation.data.payeeNames[txn.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Category
	category := ""
	if txn.HasCategory() {
		if name, ok := a.reconciliation.data.categoryNames[txn.CategoryID.ID]; ok {
			category = name
		}
	} else if txn.IsTransfer() {
		category = "[Transfer]"
	}

	// Amount
	amount := accountMoney(txn.Amount, a.reconciliation.data.account)

	return []string{checkbox, dateStr, status, payee, category, amount}
}

// updateReconciliationCheckboxes refreshes the table rows to reflect checkbox state.
func (a *App) updateReconciliationCheckboxes() {
	if a.reconciliation.table == nil || a.reconciliation.data == nil {
		return
	}
	rows := make([][]string, len(a.reconciliation.data.candidates))
	for i, txn := range a.reconciliation.data.candidates {
		rows[i] = a.formatReconciliationRow(txn)
	}
	a.reconciliation.table.SetRows(rows)
}

// renderReconciliation renders the reconciliation view.
func (a *App) renderReconciliation() string {
	if a.reconciliation.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading reconciliation...")
	}

	// Reconciliation is full-screen: it takes the whole width, not
	// ContentWidth, which leaves out a sidebar this view does not show.
	contentWidth := a.width

	var sections []string

	// Header: RECONCILE: <account name>    Statement Date: <date>
	acctName := a.reconciliation.data.account.Name
	stmtDate := a.reconciliation.data.session.StatementDate.Time().Format("01/02/2006")
	const leftPrefix = "RECONCILE: "
	rightHeader := "Statement Date: " + stmtDate
	// Cut the name so the header stays one row with the date in view: a
	// wrapped header would move the table down a row under the mouse.
	maxNameWidth := max(contentWidth-4-lipgloss.Width(leftPrefix)-lipgloss.Width(rightHeader)-1, 10)
	leftHeader := leftPrefix + ansi.Truncate(acctName, maxNameWidth, "…")
	padding := max(contentWidth-lipgloss.Width(leftHeader)-lipgloss.Width(rightHeader)-4, 1)
	headerRow := a.styles.Title.Render(leftHeader) + strings.Repeat(" ", padding) + a.styles.Muted.Render(rightHeader)
	sections = append(sections, headerRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	footerHeight := 3  // separator + balance line + hint line
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.reconciliation.table != nil && len(a.reconciliation.data.candidates) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.reconciliation.table.Render(a.styles, tableWidth, tableHeight))
		if info := a.reconciliation.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.reconciliation.data.candidates) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No unreconciled transactions found"))
	}

	// Sticky footer
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	stmtBal := accountMoney(a.reconciliation.data.session.StatementBalance, a.reconciliation.data.account)
	clrTotal := accountMoney(a.reconciliation.data.clearedTotal, a.reconciliation.data.account)
	difference := a.reconciliation.data.session.StatementBalance.Sub(a.reconciliation.data.clearedTotal)
	diffStr := accountMoney(difference, a.reconciliation.data.account)

	// Color the difference
	diffStyle := a.styles.Positive
	if !difference.IsZero() {
		diffStyle = a.styles.Negative
	}

	totalCount := len(a.reconciliation.data.candidates)
	checkedCount := 0
	for _, checked := range a.reconciliation.data.checkedIDs {
		if checked {
			checkedCount++
		}
	}

	// The footer fits by leaving out whole items: a clipped amount would pass
	// for a smaller one. Cleared, then Statement go; Difference stays. The
	// hints go from the right (the status bar shows them too).
	lineWidth := max(contentWidth-4, 1)
	balanceLine := widget.FitParts([]string{
		a.styles.Bold.Render("Statement: " + stmtBal),
		a.styles.Bold.Render("Cleared: " + clrTotal),
		a.styles.Bold.Render("Difference: ") + diffStyle.Render(diffStr),
	}, "  ", lineWidth, []int{1, 0})
	sections = append(sections, balanceLine)

	countLine := fmt.Sprintf("Checked: %d of %d", checkedCount, totalCount)
	hints := []string{"Space toggle", "Enter finish", "Esc cancel", "a all", "u uncheck"}
	hintLine := countLine
	if fit := widget.FitParts(hints, "  ", lineWidth-lipgloss.Width(countLine)-4, []int{4, 3, 2, 1, 0}); fit != "" {
		hintLine += "    " + fit
	}
	sections = append(sections, a.styles.Muted.Render(hintLine))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// handleReconciliationKeys handles key presses in the reconciliation view.
func (a *App) handleReconciliationKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.reconciliation.data == nil || a.reconciliation.table == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.reconciliation.table.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.reconciliation.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.reconciliation.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.reconciliation.table.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-10, 1)
		a.reconciliation.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-10, 1)
		a.reconciliation.table.PageDown(tableHeight)
	case msg.String() == "space":
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
	cursor := a.reconciliation.table.Cursor()
	if cursor < 0 || cursor >= len(a.reconciliation.data.candidates) {
		return a, nil
	}

	txn := a.reconciliation.data.candidates[cursor]
	a.reconciliation.data.checkedIDs[txn.ID] = !a.reconciliation.data.checkedIDs[txn.ID]
	if !a.reconciliation.data.checkedIDs[txn.ID] {
		delete(a.reconciliation.data.checkedIDs, txn.ID)
	}

	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// checkAllReconciliation checks all candidate transactions.
func (a *App) checkAllReconciliation() (tea.Model, tea.Cmd) {
	for _, txn := range a.reconciliation.data.candidates {
		a.reconciliation.data.checkedIDs[txn.ID] = true
	}
	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// uncheckAllReconciliation unchecks all candidate transactions.
func (a *App) uncheckAllReconciliation() (tea.Model, tea.Cmd) {
	a.reconciliation.data.checkedIDs = make(map[types.ID]bool)
	a.updateReconciliationCheckboxes()
	return a, a.recalculateClearedTotal()
}

// finishReconciliation attempts to finish the reconciliation session.
func (a *App) finishReconciliation() (tea.Model, tea.Cmd) {
	if a.reconciliation.data == nil {
		return a, nil
	}

	// Check difference first (before needing the service)
	difference := a.reconciliation.data.session.StatementBalance.Sub(a.reconciliation.data.clearedTotal)
	if !difference.IsZero() {
		diffStr := accountMoney(difference, a.reconciliation.data.account)
		a.statusbar.AddNotification(
			fmt.Sprintf("Cannot finish: difference is %s (must be %s)", diffStr, accountMoney(types.ZeroMoney, a.reconciliation.data.account)),
			widget.NotificationAlert,
		)
		return a, nil
	}

	if a.services.Reconciliation == nil || a.undoManager == nil {
		return a, nil
	}

	accountID := a.reconciliation.data.session.AccountID
	txnIDs := a.getCheckedTransactionIDs()

	svc := a.services
	undoManager := a.undoManager
	file := a.fileOfOpen()
	return a, func() tea.Msg {
		cmd := undo.NewFinishReconciliationCommand(
			svc.Reconciliation, svc.Transaction, accountID, txnIDs,
		)
		if err := undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to finish reconciliation: %w", err)}
		}
		return reconciliationFinishedMsg{fileOf: file}
	}
}

// cancelReconciliation cancels the current reconciliation session.
func (a *App) cancelReconciliation() (tea.Model, tea.Cmd) {
	if a.reconciliation.data == nil || a.services.Reconciliation == nil {
		return a, nil
	}

	accountID := a.reconciliation.data.session.AccountID

	svc := a.services
	file := a.fileOfOpen()
	return a, func() tea.Msg {
		err := svc.Reconciliation.CancelReconciliation(accountID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to cancel reconciliation: %w", err)}
		}
		return reconciliationCancelledMsg{fileOf: file}
	}
}

// afterReconciliationFinished drops the session and returns to the register
// the account was reconciled in, reloading it so the new cleared marks show.
func (a *App) afterReconciliationFinished() tea.Cmd {
	acctName := ""
	if a.reconciliation.data != nil {
		acctName = a.reconciliation.data.account.Name
	}
	a.reconciliation.data = nil
	a.reconciliation.table = nil
	a.switchView(ViewRegister)
	a.statusbar.AddNotification(fmt.Sprintf("Reconciliation completed for %s", acctName), widget.NotificationInfo)
	return tea.Batch(
		a.loadRegisterData(a.sidebar.SelectedAccountID()),
		a.loadSidebarData(),
	)
}

// afterReconciliationCancelled drops the session and returns to the register.
// Nothing is reloaded: a cancelled session wrote nothing.
func (a *App) afterReconciliationCancelled() {
	a.reconciliation.data = nil
	a.reconciliation.table = nil
	a.switchView(ViewRegister)
	a.statusbar.AddNotification("Reconciliation cancelled", widget.NotificationInfo)
}
