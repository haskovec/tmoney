package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// registerAppWide builds a register-view App at the given terminal width with a
// fixed two-transaction ledger (newest -$100, oldest +$500, opening 0).
func registerAppWide(width int) *App {
	styles := widget.NewStyles()
	styles.Resize(width, 30)
	accountID := types.NewID()
	return &App{
		currentView: ViewRegister,
		width:       width,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
				Active:    true,
			},
			transactions: []*transaction.Transaction{
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-100"), Status: transaction.StatusUncleared},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("500"), Status: transaction.StatusUncleared},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("400")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
}

func TestApp_BuildRegisterTable_ShowsBalanceColumn(t *testing.T) {
	app := registerAppWide(120) // tableWidth ~92 >= 72 -> Balance shown
	app.buildRegisterTable()

	rows := app.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if len(rows[0]) != 6 {
		t.Fatalf("row[0] cells = %d, want 6 (Balance column present)", len(rows[0]))
	}
	// Newest row (index 0): opening 0 + 500 - 100 = 400.
	if rows[0][5] != "$400.00" {
		t.Errorf("row[0] balance = %q, want %q", rows[0][5], "$400.00")
	}
	// Oldest row (index 1): opening 0 + 500 = 500.
	if rows[1][5] != "$500.00" {
		t.Errorf("row[1] balance = %q, want %q", rows[1][5], "$500.00")
	}

	view := app.renderRegister()
	if !contains(view, "Balance") {
		t.Error("renderRegister() should contain the 'Balance' column header")
	}
}

func TestApp_BuildRegisterTable_LargeBalanceNotTruncated(t *testing.T) {
	// A running balance accumulates to magnitudes larger than any single
	// amount; the Balance column must be wide enough not to truncate it (the
	// title bar shows the full figure, so the column must agree).
	styles := widget.NewStyles()
	styles.Resize(120, 30)
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      styles,
		register: &registerData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Brokerage", Active: true},
			transactions:  []*transaction.Transaction{{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-10000000"), Status: transaction.StatusUncleared}},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("-10000000")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()
	view := app.renderRegister()
	// "-$10000000.00" is 13 chars — would be truncated to "-$10000000.…" at
	// the old width-12 column. The full figure must survive in the rendered view.
	if !contains(view, "-$10000000.00") {
		t.Errorf("rendered register truncated the running balance; want full %q in view", "-$10000000.00")
	}
}

func TestApp_WindowResize_PreservesPendingSelectionAgainstStaleLedger(t *testing.T) {
	// Regression: a resize landing in the async save→reload window rebuilds the
	// table against the STALE ledger (which lacks the just-saved row). It must
	// NOT clear pendingRegisterSelectID, so the eventual real reload can still
	// move the cursor onto the saved row.
	app := registerAppWide(120)
	savedID := types.NewID() // an ID not present in the current (stale) ledger
	app.pendingRegisterSelectID = savedID
	app.buildRegisterTable()

	// A resize that flips the Balance column's visibility (120→80) rebuilds the
	// table against the stale ledger. The just-saved row is absent, so the
	// match fails — but the pending ID must survive for the real reload.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if app.pendingRegisterSelectID != savedID {
		t.Fatalf("pending selection cleared by stale rebuild: got %v, want %v", app.pendingRegisterSelectID, savedID)
	}

	// The real reload arrives with a ledger that DOES contain the saved row.
	accountID := app.register.account.ID
	app.register.transactions = append([]*transaction.Transaction{
		{BaseModel: types.BaseModel{ID: savedID}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-7"), Status: transaction.StatusUncleared},
	}, app.register.transactions...)
	app.buildRegisterTable()
	if app.table.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 (the just-saved row)", app.table.Cursor())
	}
	if !app.pendingRegisterSelectID.IsNil() {
		t.Errorf("pending selection should be cleared after the match, got %v", app.pendingRegisterSelectID)
	}
}

func TestApp_BuildRegisterTable_HidesBalanceWhenNarrow(t *testing.T) {
	app := registerAppWide(80) // tableWidth ~56 < 72 -> Balance hidden
	app.buildRegisterTable()

	rows := app.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if len(rows[0]) != 5 {
		t.Errorf("row[0] cells = %d, want 5 (Balance column hidden)", len(rows[0]))
	}
}

func TestApp_WindowResize_TogglesBalanceColumn(t *testing.T) {
	app := registerAppWide(80) // start narrow: Balance hidden
	app.buildRegisterTable()
	if got := len(app.table.Rows()[0]); got != 5 {
		t.Fatalf("narrow row cells = %d, want 5", got)
	}

	// Resizing wide must rebuild the table and reveal the Balance column.
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := len(app.table.Rows()[0]); got != 6 {
		t.Errorf("after widen: row cells = %d, want 6 (Balance revealed)", got)
	}

	// Resizing back narrow must drop it again.
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if got := len(app.table.Rows()[0]); got != 5 {
		t.Errorf("after narrow: row cells = %d, want 5 (Balance hidden)", got)
	}
}

func TestApp_BuildInvestmentRegisterTable_ShowsCashColumn(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(140, 30) // tableWidth ~110 >= 95 -> Balance shown
	accountID := types.NewID()
	app := &App{
		currentView: ViewInvestmentRegister,
		width:       140,
		height:      30,
		styles:      styles,
		investmentRegister: &investmentRegisterData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Brokerage", Active: true},
			securityNames: make(map[types.ID]string),
			transactions: []*investment.Transaction{
				invTxn(investment.TransactionTypeDividend, "50"),  // newest: +cash
				invTxn(investment.TransactionTypeDeposit, "1000"), // oldest: +cash
			},
		},
	}
	app.buildInvestmentRegisterTable()

	rows := app.investmentTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if len(rows[0]) != 8 {
		t.Fatalf("row[0] cells = %d, want 8 (Balance column present)", len(rows[0]))
	}
	// Newest row: deposit 1000 then dividend 50 => 1050.
	if rows[0][7] != "$1050.00" {
		t.Errorf("row[0] cash = %q, want %q", rows[0][7], "$1050.00")
	}
	if rows[1][7] != "$1000.00" {
		t.Errorf("row[1] cash = %q, want %q", rows[1][7], "$1000.00")
	}
}

func TestApp_BuildInvestmentRegisterTable_HidesCashWhenNarrow(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(120, 30) // tableWidth ~92 < 95 -> Balance hidden
	accountID := types.NewID()
	app := &App{
		currentView: ViewInvestmentRegister,
		width:       120,
		height:      30,
		styles:      styles,
		investmentRegister: &investmentRegisterData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Brokerage", Active: true},
			securityNames: make(map[types.ID]string),
			transactions: []*investment.Transaction{
				invTxn(investment.TransactionTypeDeposit, "1000"),
			},
		},
	}
	app.buildInvestmentRegisterTable()

	rows := app.investmentTable.Rows()
	if len(rows[0]) != 7 {
		t.Errorf("row[0] cells = %d, want 7 (Balance hidden on 120-col terminal)", len(rows[0]))
	}
}
