package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildStartReconciliationDialog(t *testing.T) {
	d := buildStartReconciliationDialog()

	if d == nil {
		t.Fatal("buildStartReconciliationDialog() returned nil")
	}

	if !d.IsVisible() {
		// Not yet visible until shown - that's OK
	}

	fields := d.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	if fields[0].Label != "Statement Date" {
		t.Errorf("field[0].Label = %q, want %q", fields[0].Label, "Statement Date")
	}
	if !fields[0].Required {
		t.Error("Statement Date field should be required")
	}

	if fields[1].Label != "Statement Balance" {
		t.Errorf("field[1].Label = %q, want %q", fields[1].Label, "Statement Balance")
	}
	if !fields[1].Required {
		t.Error("Statement Balance field should be required")
	}
}

func TestReconciliationViewData(t *testing.T) {
	data := &reconciliationViewData{
		checkedIDs: make(map[types.ID]bool),
	}

	id1 := types.NewID()
	id2 := types.NewID()
	id3 := types.NewID()

	// Nothing checked initially
	if len(data.checkedIDs) != 0 {
		t.Errorf("expected 0 checked, got %d", len(data.checkedIDs))
	}

	// Check some
	data.checkedIDs[id1] = true
	data.checkedIDs[id2] = true

	if !data.checkedIDs[id1] {
		t.Error("id1 should be checked")
	}
	if !data.checkedIDs[id2] {
		t.Error("id2 should be checked")
	}
	if data.checkedIDs[id3] {
		t.Error("id3 should not be checked")
	}

	// Uncheck
	delete(data.checkedIDs, id1)
	if data.checkedIDs[id1] {
		t.Error("id1 should not be checked after delete")
	}
}

func TestGetCheckedTransactionIDs(t *testing.T) {
	id1 := types.NewID()
	id2 := types.NewID()
	id3 := types.NewID()

	app := &App{
		reconciliation: &reconciliationViewData{
			checkedIDs: map[types.ID]bool{
				id1: true,
				id2: true,
				id3: false, // unchecked
			},
		},
	}

	ids := app.getCheckedTransactionIDs()

	// Should only include true entries
	idSet := make(map[types.ID]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	if len(ids) != 2 {
		t.Errorf("expected 2 checked IDs, got %d", len(ids))
	}
	if !idSet[id1] {
		t.Error("id1 should be in checked IDs")
	}
	if !idSet[id2] {
		t.Error("id2 should be in checked IDs")
	}
}

func TestGetCheckedTransactionIDs_NilReconciliation(t *testing.T) {
	app := &App{}
	ids := app.getCheckedTransactionIDs()
	if ids != nil {
		t.Errorf("expected nil, got %v", ids)
	}
}

func TestFormatReconciliationRow(t *testing.T) {
	payeeID := types.NewID()
	categoryID := types.NewID()

	app := &App{
		reconciliation: &reconciliationViewData{
			checkedIDs: make(map[types.ID]bool),
			payeeNames: map[types.ID]string{
				payeeID: "Coffee Shop",
			},
			categoryNames: map[types.ID]string{
				categoryID: "Food",
			},
			accountNames: make(map[types.ID]string),
		},
	}

	txn := &transaction.Transaction{
		Status:     transaction.StatusUncleared,
		PayeeID:    types.NullableID{ID: payeeID, Valid: true},
		CategoryID: types.NullableID{ID: categoryID, Valid: true},
	}
	txn.Amount, _ = types.NewMoney("-45.50")
	txn.Date = types.Today()

	row := app.formatReconciliationRow(txn)

	if len(row) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(row))
	}

	// Checkbox (not checked)
	if row[0] != "[ ]" {
		t.Errorf("checkbox = %q, want %q", row[0], "[ ]")
	}

	// Status (uncleared = space)
	if row[2] != " " {
		t.Errorf("status = %q, want %q", row[2], " ")
	}

	// Payee
	if row[3] != "Coffee Shop" {
		t.Errorf("payee = %q, want %q", row[3], "Coffee Shop")
	}

	// Category
	if row[4] != "Food" {
		t.Errorf("category = %q, want %q", row[4], "Food")
	}

	// Amount
	if row[5] != "-$45.50" {
		t.Errorf("amount = %q, want %q", row[5], "-$45.50")
	}
}

func TestFormatReconciliationRow_Checked(t *testing.T) {
	txnID := types.NewID()

	app := &App{
		reconciliation: &reconciliationViewData{
			checkedIDs:    map[types.ID]bool{txnID: true},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	txn := &transaction.Transaction{
		Status: transaction.StatusCleared,
	}
	txn.ID = txnID
	txn.Amount = types.ZeroMoney
	txn.Date = types.Today()

	row := app.formatReconciliationRow(txn)

	// Checkbox (checked)
	if row[0] != "[✓]" {
		t.Errorf("checkbox = %q, want %q", row[0], "[✓]")
	}

	// Cleared indicator
	if row[2] != "✓" {
		t.Errorf("status = %q, want %q", row[2], "✓")
	}
}

func TestFormatReconciliationRow_Transfer(t *testing.T) {
	transferAcctID := types.NewID()

	app := &App{
		reconciliation: &reconciliationViewData{
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames: map[types.ID]string{
				transferAcctID: "Savings",
			},
		},
	}

	txn := &transaction.Transaction{
		Status:            transaction.StatusUncleared,
		TransferID:        types.NullableID{ID: types.NewID(), Valid: true},
		TransferAccountID: types.NullableID{ID: transferAcctID, Valid: true},
	}
	txn.Amount, _ = types.NewMoney("-500.00")
	txn.Date = types.Today()

	row := app.formatReconciliationRow(txn)

	if row[3] != "Transfer: Savings" {
		t.Errorf("payee = %q, want %q", row[3], "Transfer: Savings")
	}
	if row[4] != "[Transfer]" {
		t.Errorf("category = %q, want %q", row[4], "[Transfer]")
	}
}

func TestBuildReconciliationTable(t *testing.T) {
	txn1 := &transaction.Transaction{
		Status: transaction.StatusUncleared,
	}
	txn1.ID = types.NewID()
	txn1.Amount = types.ZeroMoney
	txn1.Date = types.Today()

	txn2 := &transaction.Transaction{
		Status: transaction.StatusCleared,
	}
	txn2.ID = types.NewID()
	txn2.Amount = types.ZeroMoney
	txn2.Date = types.Today()

	app := &App{
		reconciliation: &reconciliationViewData{
			candidates:    []*transaction.Transaction{txn1, txn2},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildReconciliationTable()

	if app.reconciliationTable == nil {
		t.Fatal("reconciliationTable should not be nil after build")
	}
	if app.reconciliationTable.RowCount() != 2 {
		t.Errorf("expected 2 rows, got %d", app.reconciliationTable.RowCount())
	}
}

func TestToggleReconciliationCheck(t *testing.T) {
	txn := &transaction.Transaction{
		Status: transaction.StatusUncleared,
	}
	txn.ID = types.NewID()
	txn.Amount = types.ZeroMoney
	txn.Date = types.Today()

	app := &App{
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	// Toggle on
	app.toggleReconciliationCheck()
	if !app.reconciliation.checkedIDs[txn.ID] {
		t.Error("transaction should be checked after toggle")
	}

	// Toggle off
	app.toggleReconciliationCheck()
	if app.reconciliation.checkedIDs[txn.ID] {
		t.Error("transaction should be unchecked after second toggle")
	}
}

func TestCheckAllReconciliation(t *testing.T) {
	txn1 := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn1.ID = types.NewID()
	txn1.Amount = types.ZeroMoney
	txn1.Date = types.Today()

	txn2 := &transaction.Transaction{Status: transaction.StatusCleared}
	txn2.ID = types.NewID()
	txn2.Amount = types.ZeroMoney
	txn2.Date = types.Today()

	app := &App{
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn1, txn2},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	app.checkAllReconciliation()

	if !app.reconciliation.checkedIDs[txn1.ID] {
		t.Error("txn1 should be checked after check all")
	}
	if !app.reconciliation.checkedIDs[txn2.ID] {
		t.Error("txn2 should be checked after check all")
	}
}

func TestUncheckAllReconciliation(t *testing.T) {
	txn1 := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn1.ID = types.NewID()
	txn1.Amount = types.ZeroMoney
	txn1.Date = types.Today()

	app := &App{
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn1},
			checkedIDs:    map[types.ID]bool{txn1.ID: true},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	app.uncheckAllReconciliation()

	if app.reconciliation.checkedIDs[txn1.ID] {
		t.Error("txn1 should be unchecked after uncheck all")
	}
	if len(app.reconciliation.checkedIDs) != 0 {
		t.Errorf("expected 0 checked, got %d", len(app.reconciliation.checkedIDs))
	}
}

func TestRenderReconciliation_Loading(t *testing.T) {
	app := &App{
		styles: NewStyles(),
	}
	app.styles.Resize(80, 24)

	output := app.renderReconciliation()
	if !strings.Contains(output, "Loading reconciliation") {
		t.Error("should show loading message when reconciliation data is nil")
	}
}

func TestRenderReconciliation_NoCandidates(t *testing.T) {
	stmtBal, _ := types.NewMoney("5000.00")
	session := &reconciliation.Session{
		StatementBalance: stmtBal,
	}
	session.StatementDate = types.Today()

	app := &App{
		width:  80,
		height: 24,
		styles: NewStyles(),
		reconciliation: &reconciliationViewData{
			session: session,
			account: &account.Account{Name: "Checking"},
			candidates:    []*transaction.Transaction{},
			checkedIDs:    make(map[types.ID]bool),
			clearedTotal:  types.ZeroMoney,
		},
	}
	app.styles.Resize(80, 24)

	output := app.renderReconciliation()
	if !strings.Contains(output, "No unreconciled transactions") {
		t.Error("should show 'no unreconciled transactions' message")
	}
	if !strings.Contains(output, "RECONCILE: Checking") {
		t.Error("should show account name in header")
	}
}

func TestRenderReconciliation_WithData(t *testing.T) {
	stmtBal, _ := types.NewMoney("5000.00")
	clrTotal, _ := types.NewMoney("4500.00")
	session := &reconciliation.Session{
		StatementBalance: stmtBal,
	}
	session.StatementDate = types.Today()

	txn := &transaction.Transaction{
		Status: transaction.StatusUncleared,
	}
	txn.ID = types.NewID()
	txn.Amount, _ = types.NewMoney("-50.00")
	txn.Date = types.Today()

	app := &App{
		width:  100,
		height: 30,
		styles: NewStyles(),
		reconciliation: &reconciliationViewData{
			session:       session,
			account:       &account.Account{Name: "Checking"},
			candidates:    []*transaction.Transaction{txn},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			clearedTotal:  clrTotal,
		},
	}
	app.styles.Resize(100, 30)
	app.buildReconciliationTable()

	output := app.renderReconciliation()

	// Should contain the footer info
	if !strings.Contains(output, "Statement:") {
		t.Error("should contain 'Statement:' in footer")
	}
	if !strings.Contains(output, "Cleared:") {
		t.Error("should contain 'Cleared:' in footer")
	}
	if !strings.Contains(output, "Difference:") {
		t.Error("should contain 'Difference:' in footer")
	}
	if !strings.Contains(output, "Checked: 0 of 1") {
		t.Error("should show checked count")
	}
	if !strings.Contains(output, "Space toggle") {
		t.Error("should show key hints in footer")
	}
}

func TestHandleReconciliationKeys_Navigation(t *testing.T) {
	txn1 := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn1.ID = types.NewID()
	txn1.Amount = types.ZeroMoney
	txn1.Date = types.Today()

	txn2 := &transaction.Transaction{Status: transaction.StatusCleared}
	txn2.ID = types.NewID()
	txn2.Amount = types.ZeroMoney
	txn2.Date = types.Today()

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn1, txn2},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	// Move down
	downKey := tea.KeyMsg{Type: tea.KeyDown}
	app.handleReconciliationKeys(downKey)

	if app.reconciliationTable.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 after down", app.reconciliationTable.Cursor())
	}

	// Move up
	upKey := tea.KeyMsg{Type: tea.KeyUp}
	app.handleReconciliationKeys(upKey)

	if app.reconciliationTable.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 after up", app.reconciliationTable.Cursor())
	}
}

func TestHandleReconciliationKeys_Space(t *testing.T) {
	txn := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn.ID = types.NewID()
	txn.Amount = types.ZeroMoney
	txn.Date = types.Today()

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	// Press space to toggle
	spaceKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	app.handleReconciliationKeys(spaceKey)

	if !app.reconciliation.checkedIDs[txn.ID] {
		t.Error("transaction should be checked after space")
	}
}

func TestHandleReconciliationKeys_CheckAll(t *testing.T) {
	txn1 := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn1.ID = types.NewID()
	txn1.Amount = types.ZeroMoney
	txn1.Date = types.Today()

	txn2 := &transaction.Transaction{Status: transaction.StatusCleared}
	txn2.ID = types.NewID()
	txn2.Amount = types.ZeroMoney
	txn2.Date = types.Today()

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		reconciliation: &reconciliationViewData{
			session: &reconciliation.Session{
				AccountID: types.NewID(),
			},
			candidates:    []*transaction.Transaction{txn1, txn2},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	// Press 'a' to check all
	aKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	app.handleReconciliationKeys(aKey)

	if !app.reconciliation.checkedIDs[txn1.ID] {
		t.Error("txn1 should be checked after 'a'")
	}
	if !app.reconciliation.checkedIDs[txn2.ID] {
		t.Error("txn2 should be checked after 'a'")
	}

	// Press 'u' to uncheck all
	uKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}
	app.handleReconciliationKeys(uKey)

	if app.reconciliation.checkedIDs[txn1.ID] {
		t.Error("txn1 should be unchecked after 'u'")
	}
	if app.reconciliation.checkedIDs[txn2.ID] {
		t.Error("txn2 should be unchecked after 'u'")
	}
}

func TestHandleReconciliationKeys_FinishWithDifference(t *testing.T) {
	stmtBal, _ := types.NewMoney("5000.00")
	session := &reconciliation.Session{
		StatementBalance: stmtBal,
	}
	session.StatementDate = types.Today()

	app := &App{
		width:     80,
		height:    24,
		keys:      defaultKeyMap(),
		statusbar: NewStatusBar(),
		reconciliation: &reconciliationViewData{
			session:       session,
			candidates:    []*transaction.Transaction{},
			checkedIDs:    make(map[types.ID]bool),
			clearedTotal:  types.ZeroMoney,
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildReconciliationTable()

	// Press Enter to finish (should fail because difference is not $0)
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := app.handleReconciliationKeys(enterKey)

	// Should not return a command (difference is not zero)
	if cmd != nil {
		t.Error("finish with non-zero difference should not return a command")
	}

	// Should have a notification
	notifications := app.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should show notification about non-zero difference")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "Cannot finish") {
		t.Errorf("notification = %q, should contain 'Cannot finish'", notifications[0].Text)
	}
}

func TestReconciliationView_KeyHints(t *testing.T) {
	app := &App{
		currentView: ViewReconciliation,
	}

	hints := app.getKeyHints()
	if !strings.Contains(hints, "space toggle") {
		t.Errorf("hints should contain 'space toggle', got: %s", hints)
	}
	if !strings.Contains(hints, "enter finish") {
		t.Errorf("hints should contain 'enter finish', got: %s", hints)
	}
}

func TestReconciliationView_String(t *testing.T) {
	v := ViewReconciliation
	if v.String() != "Reconciliation" {
		t.Errorf("ViewReconciliation.String() = %q, want %q", v.String(), "Reconciliation")
	}
}

func TestReconciliationView_SwitchView(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.switchView(ViewReconciliation)

	if app.currentView != ViewReconciliation {
		t.Errorf("currentView = %v, want ViewReconciliation", app.currentView)
	}
	if app.previousView != ViewRegister {
		t.Errorf("previousView = %v, want ViewRegister", app.previousView)
	}
}

func TestReconciliationUpdate_StartedMsg(t *testing.T) {
	session := reconciliation.NewSession(types.NewID(), types.Today(), types.ZeroMoney)
	account := &account.Account{Name: "Checking"}

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := reconciliationStartedMsg{session: session, account: account}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.currentView != ViewReconciliation {
		t.Errorf("currentView = %v, want ViewReconciliation", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load reconciliation data")
	}
}

func TestReconciliationUpdate_LoadedMsg(t *testing.T) {
	txn := &transaction.Transaction{Status: transaction.StatusUncleared}
	txn.ID = types.NewID()
	txn.Amount = types.ZeroMoney
	txn.Date = types.Today()

	data := &reconciliationViewData{
		session: &reconciliation.Session{},
		account: &account.Account{Name: "Checking"},
		candidates:    []*transaction.Transaction{txn},
		checkedIDs:    make(map[types.ID]bool),
		payeeNames:    make(map[types.ID]string),
		categoryNames: make(map[types.ID]string),
		accountNames:  make(map[types.ID]string),
	}

	app := &App{
		currentView: ViewReconciliation,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	msg := reconciliationLoadedMsg{data: data}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.reconciliation == nil {
		t.Fatal("reconciliation data should be set")
	}
	if updatedApp.reconciliationTable == nil {
		t.Error("reconciliation table should be built")
	}
}

func TestReconciliationUpdate_ClearedTotalMsg(t *testing.T) {
	total, _ := types.NewMoney("1234.56")

	app := &App{
		currentView: ViewReconciliation,
		keys:        defaultKeyMap(),
		reconciliation: &reconciliationViewData{
			clearedTotal: types.ZeroMoney,
		},
	}

	msg := reconciliationClearedTotalMsg{clearedTotal: total}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.reconciliation.clearedTotal.String() != total.String() {
		t.Errorf("clearedTotal = %s, want %s", updatedApp.reconciliation.clearedTotal.String(), total.String())
	}
}

func TestReconciliationUpdate_FinishedMsg(t *testing.T) {
	app := &App{
		currentView: ViewReconciliation,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reconciliation: &reconciliationViewData{
			account: &account.Account{Name: "Checking"},
		},
	}

	msg := reconciliationFinishedMsg{}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.currentView != ViewRegister {
		t.Errorf("currentView = %v, want ViewRegister", updatedApp.currentView)
	}
	if updatedApp.reconciliation != nil {
		t.Error("reconciliation should be nil after finish")
	}
	if cmd == nil {
		t.Error("should return commands to reload data")
	}

	// Should have success notification
	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about completion")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "completed") {
		t.Errorf("notification = %q, should contain 'completed'", notifications[0].Text)
	}
}

func TestReconciliationUpdate_CancelledMsg(t *testing.T) {
	app := &App{
		currentView:  ViewReconciliation,
		previousView: ViewRegister,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		sidebar:      NewSidebar(),
		reconciliation: &reconciliationViewData{
			account: &account.Account{Name: "Checking"},
		},
	}

	msg := reconciliationCancelledMsg{}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.currentView != ViewRegister {
		t.Errorf("currentView = %v, want ViewRegister", updatedApp.currentView)
	}
	if updatedApp.reconciliation != nil {
		t.Error("reconciliation should be nil after cancel")
	}

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about cancellation")
	}
}

func TestReconciliationFullScreen(t *testing.T) {
	// Verify that reconciliation view renders full-screen (no sidebar)
	stmtBal, _ := types.NewMoney("5000.00")
	session := &reconciliation.Session{
		StatementBalance: stmtBal,
	}
	session.StatementDate = types.Today()

	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReconciliation,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		reconciliation: &reconciliationViewData{
			session:       session,
			account:       &account.Account{Name: "Test Account"},
			candidates:    []*transaction.Transaction{},
			checkedIDs:    make(map[types.ID]bool),
			clearedTotal:  types.ZeroMoney,
		},
	}

	content := app.renderContent(28)
	// Full-screen should use the full width, not leave room for sidebar
	// We verify this indirectly by checking the content contains our view
	if !strings.Contains(content, "RECONCILE: Test Account") {
		t.Error("renderContent should contain reconciliation view content")
	}
}

func TestReconciliationBlocksViewSwitching(t *testing.T) {
	app := &App{
		currentView: ViewReconciliation,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		reconciliation: &reconciliationViewData{
			session:    &reconciliation.Session{AccountID: types.NewID()},
			candidates: []*transaction.Transaction{},
			checkedIDs: make(map[types.ID]bool),
		},
	}
	app.buildReconciliationTable()

	// Pressing '1' should not switch to Dashboard
	dashKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	model, _ := app.Update(dashKey)
	updatedApp := model.(*App)
	if updatedApp.currentView != ViewReconciliation {
		t.Errorf("view switched to %v, should stay in ViewReconciliation", updatedApp.currentView)
	}

	// Pressing '2' should not switch to Scheduled
	schedKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}
	model, _ = app.Update(schedKey)
	updatedApp = model.(*App)
	if updatedApp.currentView != ViewReconciliation {
		t.Errorf("view switched to %v, should stay in ViewReconciliation", updatedApp.currentView)
	}
}

func TestReconciliationHelpOverlay(t *testing.T) {
	sections := viewShortcutSections(ViewReconciliation)

	found := false
	for _, s := range sections {
		if s.Title == "Reconciliation" {
			found = true
			// Check for expected shortcuts
			hasSpace := false
			hasEnter := false
			hasEsc := false
			hasCheckAll := false
			for _, e := range s.Entries {
				switch e.Key {
				case "Space":
					hasSpace = true
				case "Enter":
					hasEnter = true
				case "Esc":
					hasEsc = true
				case "a":
					hasCheckAll = true
				}
			}
			if !hasSpace {
				t.Error("reconciliation shortcuts should include Space")
			}
			if !hasEnter {
				t.Error("reconciliation shortcuts should include Enter")
			}
			if !hasEsc {
				t.Error("reconciliation shortcuts should include Esc")
			}
			if !hasCheckAll {
				t.Error("reconciliation shortcuts should include 'a' for check all")
			}
		}
	}
	if !found {
		t.Error("reconciliation shortcuts section not found in help overlay")
	}
}

func TestMenuBarHasReconcileAccount(t *testing.T) {
	mb := NewMenuBar()
	found := false
	for _, m := range mb.menus {
		for _, item := range m.items {
			if item.action == MenuActionReconcileAccount {
				found = true
				if item.label != "Reconcile Account" {
					t.Errorf("menu label = %q, want %q", item.label, "Reconcile Account")
				}
			}
		}
	}
	if !found {
		t.Error("MenuActionReconcileAccount not found in menu bar")
	}
}

