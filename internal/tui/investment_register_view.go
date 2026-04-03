package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// investmentRegisterData holds the loaded data for the investment account register view.
type investmentRegisterData struct {
	account       *account.Account
	transactions  []*investment.Transaction
	securityNames map[types.ID]string // SecurityID -> Ticker
	cashBalance   types.Money
}

// investmentRegisterLoadedMsg is sent when investment register data has been loaded.
type investmentRegisterLoadedMsg struct {
	data *investmentRegisterData
}

// loadInvestmentRegisterData returns a command that loads all data needed for the investment register view.
func (a *App) loadInvestmentRegisterData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &investmentRegisterData{
			securityNames: make(map[types.ID]string),
		}

		// Load account
		if a.accountSvc != nil {
			acct, err := a.accountSvc.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct
		}

		// Load investment transactions via repository
		if a.investmentRepo != nil {
			txns, err := a.investmentRepo.ListByAccount(accountID, investment.TransactionFilter{})
			if err != nil {
				return errMsg{err: err}
			}
			data.transactions = txns
		}

		// Load cash balance via service
		if a.investmentSvc != nil {
			cash, err := a.investmentSvc.GetCashBalance(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.cashBalance = cash
		}

		// Load security names for display
		if a.securitySvc != nil {
			securities, err := a.securitySvc.List(security.Filter{})
			if err == nil {
				for _, sec := range securities {
					data.securityNames[sec.ID] = sec.Ticker
				}
			}
		}

		return investmentRegisterLoadedMsg{data: data}
	}
}

// buildInvestmentRegisterTable creates and populates the table for the investment register view.
func (a *App) buildInvestmentRegisterTable() {
	if a.investmentRegister == nil {
		return
	}

	columns := []Column{
		{Header: "Date", Width: 10, Align: AlignLeft},
		{Header: "S", Width: 1, Align: AlignCenter},
		{Header: "Type", Width: 19, Align: AlignLeft},
		{Header: "Security", Width: 10, Align: AlignLeft},
		{Header: "Shares", Width: 12, Align: AlignRight},
		{Header: "Price", Width: 12, Align: AlignRight},
		{Header: "Total", Width: 12, Align: AlignRight},
	}

	if a.investmentTable == nil {
		a.investmentTable = NewTable(columns)
	} else {
		a.investmentTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.investmentRegister.transactions))
	for i, txn := range a.investmentRegister.transactions {
		rows[i] = a.formatInvestmentRegisterRow(txn)
	}
	a.investmentTable.SetRows(rows)
}

// formatInvestmentRegisterRow formats an investment transaction into table row strings.
func (a *App) formatInvestmentRegisterRow(txn *investment.Transaction) []string {
	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Status indicator
	status := " "
	switch txn.Status {
	case investment.TransactionStatusCleared:
		status = "✓"
	case investment.TransactionStatusReconciled:
		status = "R"
	}

	// Type
	txnType := txn.Type.DisplayName()

	// Security (ticker from lookup map)
	sec := ""
	if txn.SecurityID.Valid {
		if name, ok := a.investmentRegister.securityNames[txn.SecurityID.ID]; ok {
			sec = name
		}
	}

	// Shares
	shares := ""
	if txn.Shares.Valid && !txn.Shares.Quantity.IsZero() {
		shares = txn.Shares.Quantity.String()
	}

	// Price per share
	price := ""
	if txn.PricePerShare.Valid {
		price = formatDashboardMoney(txn.PricePerShare.Money)
	}

	// Total amount
	total := formatDashboardMoney(txn.TotalAmount)

	return []string{dateStr, status, txnType, sec, shares, price, total}
}

// selectedInvestmentTransaction returns the currently selected investment transaction based on table cursor.
func (a *App) selectedInvestmentTransaction() *investment.Transaction {
	if a.investmentRegister == nil || a.investmentTable == nil {
		return nil
	}

	cursor := a.investmentTable.Cursor()
	if cursor < 0 || cursor >= len(a.investmentRegister.transactions) {
		return nil
	}
	return a.investmentRegister.transactions[cursor]
}

// renderInvestmentRegister renders the investment account register view.
func (a *App) renderInvestmentRegister() string {
	if a.investmentRegister == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading investment register...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: account name + cash balance
	acctName := strings.ToUpper(a.investmentRegister.account.Name)
	cashStr := "Cash: " + formatDashboardMoney(a.investmentRegister.cashBalance)

	maxNameWidth := max(contentWidth-lipgloss.Width(cashStr)-6, 10)
	acctName = truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(cashStr)-4, 1)

	cashStyle := a.styles.Positive
	if a.investmentRegister.cashBalance.IsNegative() {
		cashStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + cashStyle.Render(cashStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight, 1)

	if a.investmentTable != nil && len(a.investmentRegister.transactions) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.investmentTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.investmentTable.ScrollInfo(tableHeight - 1); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.investmentRegister.transactions) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No investment transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// handleInvestmentRegisterKeys handles key presses in the investment register view.
func (a *App) handleInvestmentRegisterKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.investmentTable != nil {
				a.investmentTable.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.investmentTable != nil {
				a.investmentTable.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// Table-focused key handling
	if a.investmentTable == nil || a.investmentRegister == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.investmentTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.investmentTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.investmentTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.investmentTable.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.investmentTable.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.investmentTable.PageDown(tableHeight)
	case key.Matches(msg, a.keys.New):
		a.openInvestmentTypeSelector(false)
	case key.Matches(msg, a.keys.Enter):
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			a.openInvestmentTypeSelector(true)
		}
	case msg.String() == "c":
		return a.toggleInvestmentTransactionStatus()
	case key.Matches(msg, a.keys.Delete):
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			txnID := txn.ID
			a.showConfirmDialog(
				"Delete Transaction",
				fmt.Sprintf("Delete this %s transaction?", txn.Type.DisplayName()),
				func() tea.Msg {
					if a.investmentRepo == nil {
						return errMsg{err: fmt.Errorf("investment repository not available")}
					}
					if err := a.investmentRepo.Delete(txnID); err != nil {
						return errMsg{err: err}
					}
					return investmentTransactionDeletedMsg{}
				},
			)
		}
	}

	return a, nil
}

// toggleInvestmentTransactionStatus toggles the cleared status of the selected investment transaction.
func (a *App) toggleInvestmentTransactionStatus() (tea.Model, tea.Cmd) {
	txn := a.selectedInvestmentTransaction()
	if txn == nil {
		return a, nil
	}

	txnID := txn.ID
	currentStatus := txn.Status

	return a, func() tea.Msg {
		if a.investmentRepo == nil {
			return errMsg{err: fmt.Errorf("investment repository not available")}
		}

		t, err := a.investmentRepo.GetByID(txnID)
		if err != nil {
			return errMsg{err: err}
		}

		switch currentStatus {
		case investment.TransactionStatusPending:
			t.Clear()
		case investment.TransactionStatusCleared:
			t.MarkPending()
		default:
			return nil
		}

		if err := a.investmentRepo.Update(t); err != nil {
			return errMsg{err: err}
		}
		return investmentTransactionClearedMsg{}
	}
}

// investmentTransactionDeletedMsg is sent when an investment transaction has been deleted.
type investmentTransactionDeletedMsg struct{}

// investmentTransactionClearedMsg is sent when an investment transaction's status has been toggled.
type investmentTransactionClearedMsg struct{}

// investmentTransactionTypeOptions returns the display names for the investment type selector.
func investmentTransactionTypeOptions() []string {
	return []string{
		investment.TransactionTypeBuy.DisplayName(),
		investment.TransactionTypeSell.DisplayName(),
		investment.TransactionTypeDividend.DisplayName(),
		investment.TransactionTypeReinvestDividend.DisplayName(),
		investment.TransactionTypeDeposit.DisplayName(),
		investment.TransactionTypeWithdrawal.DisplayName(),
		investment.TransactionTypeInterest.DisplayName(),
		investment.TransactionTypeFee.DisplayName(),
		investment.TransactionTypeTransferCash.DisplayName(),
		investment.TransactionTypeTransferShares.DisplayName(),
	}
}

// investmentTransactionTypeFromIndex maps a selector index back to an InvestmentTransactionType.
func investmentTransactionTypeFromIndex(idx int) investment.TransactionType {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	if idx >= 0 && idx < len(types) {
		return types[idx]
	}
	return investment.TransactionTypeBuy
}

// investmentTransactionTypeIndex returns the selector index for the given transaction type.
func investmentTransactionTypeIndex(txnType investment.TransactionType) int {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	for i, t := range types {
		if t == txnType {
			return i
		}
	}
	return 0
}

// openInvestmentTypeSelector opens the transaction type selector dialog.
// If editing is true, the selector is pre-set to the currently selected transaction's type.
func (a *App) openInvestmentTypeSelector(editing bool) {
	options := investmentTransactionTypeOptions()
	selectedIdx := 0

	if editing {
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			a.investmentEditTxnID = txn.ID
			selectedIdx = investmentTransactionTypeIndex(txn.Type)
		}
	} else {
		a.investmentEditTxnID = types.NilID
	}

	title := "New Transaction"
	if editing {
		title = "Edit Transaction"
	}

	d := NewDialog(title)
	d.SetWidth(40)
	d.AddSelectField("Type", options, selectedIdx)
	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "OK", Primary: true},
	})
	d.SetVisible(true)
	a.investmentTypeSelector = d
}

// handleInvestmentTypeSelectorKey handles key presses in the investment type selector dialog.
func (a *App) handleInvestmentTypeSelectorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := a.investmentTypeSelector.HandleKey(msg)

	switch action {
	case DialogActionSubmit:
		// Get the selected type
		fields := a.investmentTypeSelector.Fields()
		selectedType := investmentTransactionTypeFromIndex(fields[0].SelectedIndex)
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil

		switch selectedType {
		case investment.TransactionTypeBuy:
			return a, a.loadBuyDialogData()
		case investment.TransactionTypeSell:
			return a, a.loadSellDialogData()
		case investment.TransactionTypeDividend:
			a.dividendDialogReinvest = false
			return a, a.loadDividendDialogData()
		case investment.TransactionTypeReinvestDividend:
			a.dividendDialogReinvest = true
			return a, a.loadDividendDialogData()
		case investment.TransactionTypeDeposit,
			investment.TransactionTypeWithdrawal,
			investment.TransactionTypeFee,
			investment.TransactionTypeInterest:
			a.cashOperationType = selectedType
			var editTxn *investment.Transaction
			if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
				editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
			}
			a.cashOperationDialog = buildCashOperationDialog(selectedType.DisplayName(), editTxn)
			return a, nil
		case investment.TransactionTypeTransferCash:
			a.transferCashDirection = "deposit"
			return a, a.loadTransferCashDialogData()
		case investment.TransactionTypeTransferShares:
			return a, a.loadTransferSharesDialogData()
		}

	case DialogActionCancel:
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil
		return a, nil
	}

	return a, nil
}

// investmentRegisterShortcuts returns the shortcut section for the investment register help overlay.
func investmentRegisterShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Investment Register",
		Entries: []shortcutEntry{
			{Key: "n", Description: "New transaction"},
			{Key: "Enter", Description: "Edit transaction"},
			{Key: "c", Description: "Toggle cleared"},
			{Key: "d", Description: "Delete transaction"},
			{Key: "Tab", Description: "Switch sidebar/table"},
			{Key: "Esc", Description: "Go back"},
		},
	}
}
