package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// scheduledDueCountMsg is sent when the count of due scheduled transactions is loaded.
type scheduledDueCountMsg struct {
	count int
}

// scheduledViewData holds the loaded data for the scheduled transactions view.
type scheduledViewData struct {
	dueTxns       []*scheduled.Transaction
	upcomingTxns  []*scheduled.Transaction
	allTxns       []*scheduled.Transaction // combined: due first, then upcoming
	dueCount      int                      // number of due items (index boundary)
	payeeNames    map[types.ID]string
	accountNames  map[types.ID]string
	categoryNames map[types.ID]string
}

// scheduledViewDataLoadedMsg is sent when scheduled view data has been loaded.
type scheduledViewDataLoadedMsg struct {
	data *scheduledViewData
}

// scheduledPostedMsg is sent when a scheduled transaction has been posted.
type scheduledPostedMsg struct{}

// scheduledSkippedMsg is sent when a scheduled transaction has been skipped.
type scheduledSkippedMsg struct{}

// scheduledDeletedMsg is sent when a scheduled transaction has been deleted.
type scheduledDeletedMsg struct{}

// autoPostCompletedMsg is sent when auto-posting on file open completes.
type autoPostCompletedMsg struct {
	summary *scheduled.AutoPostSummary
}

// autoPostOnFileOpen returns a command that runs auto-posting on startup.
func (a *App) autoPostOnFileOpen() tea.Cmd {
	if a.scheduledTxnSvc == nil {
		return nil
	}
	return func() tea.Msg {
		summary, err := a.scheduledTxnSvc.AutoPost()
		if err != nil {
			return errMsg{err: err}
		}
		return autoPostCompletedMsg{summary: summary}
	}
}

// loadScheduledDueCount returns a command that loads the count of due scheduled transactions.
func (a *App) loadScheduledDueCount() tea.Cmd {
	return func() tea.Msg {
		if a.scheduledTxnSvc == nil {
			return nil
		}
		due, err := a.scheduledTxnSvc.ListDue()
		if err != nil {
			return errMsg{err: err}
		}
		return scheduledDueCountMsg{count: len(due)}
	}
}

// loadScheduledViewData returns a command that loads all data needed for the scheduled transactions view.
func (a *App) loadScheduledViewData() tea.Cmd {
	return func() tea.Msg {
		data := &scheduledViewData{
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		}

		// Load due scheduled transactions
		if a.scheduledTxnSvc != nil {
			due, err := a.scheduledTxnSvc.ListDue()
			if err != nil {
				return errMsg{err: err}
			}
			data.dueTxns = due
			data.dueCount = len(due)

			// Every active schedule belongs on this view — the user wants to
			// see the next occurrence of each, regardless of how far out it
			// is. A bounded "upcoming" window hid monthly+ schedules from
			// view right after posting, since the next occurrence was past
			// the cutoff.
			all, err := a.scheduledTxnSvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			dueIDs := make(map[string]bool, len(due))
			for _, d := range due {
				dueIDs[d.ID.String()] = true
			}
			var filteredUpcoming []*scheduled.Transaction
			for _, u := range all {
				if dueIDs[u.ID.String()] {
					continue
				}
				if u.IsCompleted() {
					continue
				}
				filteredUpcoming = append(filteredUpcoming, u)
			}
			data.upcomingTxns = filteredUpcoming

			// Build combined list: due first, then upcoming
			data.allTxns = make([]*scheduled.Transaction, 0, len(due)+len(filteredUpcoming))
			data.allTxns = append(data.allTxns, due...)
			data.allTxns = append(data.allTxns, filteredUpcoming...)
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

		// Load account names
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
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

		return scheduledViewDataLoadedMsg{data: data}
	}
}

// handleScheduledKeys handles key presses in the scheduled transactions view.
func (a *App) handleScheduledKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.scheduledTable != nil {
				a.scheduledTable.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.scheduledTable != nil {
				a.scheduledTable.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// widget.Table-focused key handling
	if a.scheduledTable == nil || a.scheduled == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.scheduledTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.scheduledTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.scheduledTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.scheduledTable.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.scheduledTable.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.scheduledTable.PageDown(tableHeight)
	case key.Matches(msg, a.keys.Enter):
		// MS-019: Enter opens the preview dialog instead of posting
		// directly. The save handler (which creates the real
		// transaction and advances the schedule) lands in MS-020 via
		// the preview dialog's Submit action.
		return a, a.loadSchedulePreviewData()
	case msg.String() == "s":
		return a.skipSelectedScheduled()
	case key.Matches(msg, a.keys.Delete):
		return a.deleteSelectedScheduled()
	case key.Matches(msg, a.keys.New):
		return a, a.loadNewScheduledDialogData()
	case msg.String() == "t":
		return a, a.loadNewScheduledTransferDialogData()
	case key.Matches(msg, a.keys.Edit):
		return a, a.loadEditScheduledDialogData()
	}

	return a, nil
}

// skipSelectedScheduled skips the currently selected scheduled transaction.
func (a *App) skipSelectedScheduled() (tea.Model, tea.Cmd) {
	if a.scheduled == nil || a.scheduledTable == nil || a.scheduledTxnSvc == nil {
		return a, nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return a, nil
	}

	st := a.scheduled.allTxns[cursor]
	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewSkipScheduledTransactionCommand(a.scheduledTxnSvc, st.ID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return scheduledSkippedMsg{}
	}
}

// deleteSelectedScheduled deletes the currently selected scheduled transaction.
func (a *App) deleteSelectedScheduled() (tea.Model, tea.Cmd) {
	if a.scheduled == nil || a.scheduledTable == nil || a.scheduledTxnSvc == nil {
		return a, nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return a, nil
	}

	st := a.scheduled.allTxns[cursor]
	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewDeleteScheduledTransactionCommand(a.scheduledTxnSvc, st.ID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return scheduledDeletedMsg{}
	}
}

// renderScheduled renders the scheduled transactions view.
func (a *App) renderScheduled() string {
	if a.scheduled == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading scheduled transactions...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: SCHEDULED + counts
	titleText := "SCHEDULED TRANSACTIONS"
	countText := ""
	if a.scheduled.dueCount > 0 {
		countText = fmt.Sprintf("%d due", a.scheduled.dueCount)
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(countText)-4, 1)
	titleRow := a.styles.Title.Render(titleText)
	if countText != "" {
		titleRow += strings.Repeat(" ", padding) + a.styles.Alert.Render(countText)
	}
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if len(a.scheduled.allTxns) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No scheduled transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to create a new scheduled transaction"))
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight, 1)

	if a.scheduledTable != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.scheduledTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.scheduledTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// buildScheduledTable creates and populates the table for the scheduled view.
func (a *App) buildScheduledTable() {
	if a.scheduled == nil {
		return
	}

	columns := []widget.Column{
		{Header: " ", Width: 3, Align: widget.AlignCenter},
		{Header: "Next Date", Width: 10, Align: widget.AlignLeft},
		{Header: "Payee", MinWidth: 12, Align: widget.AlignLeft},
		{Header: "Amount", Width: 12, Align: widget.AlignRight},
		{Header: "Frequency", Width: 10, Align: widget.AlignLeft},
		{Header: "Account", MinWidth: 10, Align: widget.AlignLeft},
		{Header: "Auto", Width: 10, Align: widget.AlignLeft},
	}

	if a.scheduledTable == nil {
		a.scheduledTable = widget.NewTable(columns)
	} else {
		a.scheduledTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.scheduled.allTxns))
	for i, st := range a.scheduled.allTxns {
		rows[i] = a.formatScheduledRow(st, i < a.scheduled.dueCount)
	}
	a.scheduledTable.SetRows(rows)
}

// formatScheduledRow formats a scheduled transaction into table row strings.
func (a *App) formatScheduledRow(st *scheduled.Transaction, isDue bool) []string {
	// Status indicator
	status := " ○"
	if isDue {
		today := types.Today()
		if st.NextDate.Equal(today) {
			status = " ●"
		} else {
			status = "!●"
		}
	}

	// Next date
	dateStr := st.NextDate.Time().Format("01/02/06")

	// Payee — for a transfer schedule there is no payee; show the
	// destination account as "→ To" in this column instead.
	payee := ""
	if st.IsTransfer() {
		if name, ok := a.scheduled.accountNames[st.TransferAccountID.ID]; ok {
			payee = "→ " + name
		} else {
			payee = "→ transfer"
		}
	} else if st.HasPayee() {
		if name, ok := a.scheduled.payeeNames[st.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Amount
	amount := "~variable"
	if st.HasAmount() {
		amount = formatDashboardMoney(st.Amount.Money)
	}

	// Frequency
	freq := st.Frequency.DisplayName()

	// Account
	account := ""
	if name, ok := a.scheduled.accountNames[st.AccountID]; ok {
		account = name
	}

	// Auto-post indicator
	autoIndicator := ""
	if st.IsAutoPost() {
		switch st.PostLeadDays {
		case 3:
			autoIndicator = "[Auto 3d]"
		case 7:
			autoIndicator = "[Auto 7d]"
		default:
			autoIndicator = "[Auto]"
		}
	}

	return []string{status, dateStr, payee, amount, freq, account, autoIndicator}
}
