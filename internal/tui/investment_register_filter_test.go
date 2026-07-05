package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// filterTestIDs bundles the security IDs seeded by newFilterTestApp so tests
// can assert on specific holdings.
type filterTestIDs struct {
	account types.ID
	fxaix   types.ID // ticker "FXAIX",  name "Fidelity 500 Index Fund"
	fskax   types.ID // ticker "FSKAX",  name "Fidelity Total Market Index"
	mfs     types.ID // tickerless,      name "MFS Mid Cap Value CT"
}

// newFilterTestApp builds an investment-register App seeded with three
// holdings (two tickered, one tickerless) plus a cash deposit, mirroring
// the inline-literal pattern used by the other investment-register tests.
// The optional width overrides the default 120-col layout (use a wide value
// when a test needs the running-balance column to be present unfiltered).
func newFilterTestApp(t *testing.T, width int) (*App, filterTestIDs) {
	t.Helper()
	if width == 0 {
		width = 120
	}

	ids := filterTestIDs{
		account: types.NewID(),
		fxaix:   types.NewID(),
		fskax:   types.NewID(),
		mfs:     types.NewID(),
	}
	date := types.NewDate(2024, time.March, 15)

	txns := []*investment.Transaction{
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeBuy, types.MustNewMoney("-1850.00"), ids.fxaix, types.MustNewQuantity("10")),
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeReinvestDividend, types.MustNewMoney("50.00"), ids.fxaix, types.MustNewQuantity("0.25")),
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeSell, types.MustNewMoney("400.00"), ids.fxaix, types.MustNewQuantity("2")),
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeBuy, types.MustNewMoney("-900.00"), ids.fskax, types.MustNewQuantity("5")),
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeBuy, types.MustNewMoney("-450.00"), ids.fskax, types.MustNewQuantity("2.5")),
		investment.NewTransactionWithSecurity(ids.account, date, investment.TransactionTypeBuy, types.MustNewMoney("-1200.00"), ids.mfs, types.MustNewQuantity("48")),
		investment.NewTransaction(ids.account, date, investment.TransactionTypeDeposit, types.MustNewMoney("5000.00")),
	}

	styles := widget.NewStyles()
	styles.Resize(width, 40)

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	zero := types.ZeroMoney
	app := &App{
		currentView: ViewInvestmentRegister,
		width:       width,
		height:      40,
		ready:       true,
		keys:        defaultKeyMap(),
		styles:      styles,
		sidebar:     sidebar,
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: ids.account},
				Name:      "Fidelity 401k",
				Type:      account.TypeInvestment,
				Active:    true,
			},
			transactions: txns,
			securityNames: map[types.ID]string{
				ids.fxaix: "FXAIX",
				ids.fskax: "FSKAX",
				ids.mfs:   "MFS Mid Cap Value CT",
			},
			securityFullNames: map[types.ID]string{
				ids.fxaix: "Fidelity 500 Index Fund",
				ids.fskax: "Fidelity Total Market Index",
				ids.mfs:   "MFS Mid Cap Value CT",
			},
			cashBalance: types.MustNewMoney("200.00"),
			valuation: &investment.AccountValuation{
				AccountID:         ids.account,
				CashBalance:       zero,
				MarketValue:       zero,
				TotalValue:        types.MustNewMoney("1000.00"),
				TotalGainLoss:     zero,
				RealizedGain:      zero,
				DividendsReceived: zero,
				InterestReceived:  zero,
				FeesPaid:          zero,
				TotalReturn:       types.MustNewMoney("100.00"),
			},
		},
	}
	app.buildInvestmentRegisterTable()
	return app, ids
}

func slashKey() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: '/', Text: "/"} }
func letterKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func enterKey() tea.KeyPressMsg        { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func escapeKey() tea.KeyPressMsg       { return tea.KeyPressMsg{Code: tea.KeyEscape} }

// typeFilter presses '/' then each rune of query through the register key
// handler, driving the same path the running app takes.
func typeFilter(app *App, query string) {
	app.handleInvestmentRegisterKeys(slashKey())
	for _, r := range query {
		app.handleInvestmentRegisterKeys(letterKey(r))
	}
}

func TestInvestmentFilter_SlashEntersSearchMode(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	if app.investmentRegisterFilterActive() {
		t.Fatal("filter should be inactive before pressing /")
	}
	app.handleInvestmentRegisterKeys(slashKey())

	if !app.investmentFilterSearching {
		t.Error("expected investmentFilterSearching=true after /")
	}
	if !app.investmentRegisterFilterActive() {
		t.Error("expected filter active after /")
	}
	// Empty query shows every row (including the cash deposit).
	if got := app.investmentTable.RowCount(); got != 7 {
		t.Errorf("empty-query filter: RowCount = %d, want 7", got)
	}
}

func TestInvestmentFilter_LiveNarrowSingleSecurity(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	typeFilter(app, "fx") // matches FXAIX ticker only

	visible := app.visibleInvestmentTransactions()
	if len(visible) != 3 {
		t.Fatalf("query 'fx': visible = %d, want 3 (FXAIX rows)", len(visible))
	}
	for _, txn := range visible {
		if !txn.SecurityID.Valid || txn.SecurityID.ID != ids.fxaix {
			t.Errorf("query 'fx': got a non-FXAIX row %v", txn.Type)
		}
	}
	if got := app.investmentTable.RowCount(); got != 3 {
		t.Errorf("table RowCount = %d, want 3", got)
	}
}

func TestInvestmentFilter_LiveNarrowMatchesFullName(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fi") // matches "Fidelity ..." names of FXAIX and FSKAX

	visible := app.visibleInvestmentTransactions()
	if len(visible) != 5 {
		t.Fatalf("query 'fi': visible = %d, want 5 (FXAIX 3 + FSKAX 2)", len(visible))
	}
	line := app.investmentFilterStatusLine()
	if !strings.Contains(line, "2 securities") {
		t.Errorf("status line %q should report 2 securities", line)
	}
}

func TestInvestmentFilter_CashRowsExcludedWhenQuerySet(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	for _, txn := range app.visibleInvestmentTransactions() {
		if txn.Type == investment.TransactionTypeDeposit {
			t.Error("cash deposit (no security) should be excluded from a non-empty filter")
		}
	}
}

func TestInvestmentFilter_EnterLocksSingleMatch(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey())

	if app.investmentFilterSearching {
		t.Error("Enter should exit searching mode when locking")
	}
	if app.investmentFilterLockedSec != ids.fxaix {
		t.Errorf("locked security = %v, want FXAIX", app.investmentFilterLockedSec)
	}
	if app.investmentFilterQuery != "" {
		t.Errorf("query should be cleared on lock, got %q", app.investmentFilterQuery)
	}
	if len(app.visibleInvestmentTransactions()) != 3 {
		t.Errorf("locked FXAIX: visible = %d, want 3", len(app.visibleInvestmentTransactions()))
	}
	line := app.investmentFilterStatusLine()
	if !strings.Contains(line, "FXAIX") || !strings.Contains(line, "Fidelity 500 Index Fund") {
		t.Errorf("locked status line %q should show ticker and full name", line)
	}
	if !strings.Contains(line, "(3 of 7)") {
		t.Errorf("locked status line %q should show (3 of 7) count", line)
	}
}

func TestInvestmentFilter_EnterDoesNotLockAmbiguousMatch(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fi") // 2 securities
	app.handleInvestmentRegisterKeys(enterKey())

	if !app.investmentFilterSearching {
		t.Error("Enter on an ambiguous (multi-security) query should stay in searching mode")
	}
	if !app.investmentFilterLockedSec.IsNil() {
		t.Error("Enter on an ambiguous query should not lock a security")
	}
}

func TestInvestmentFilter_VimLetterKeysAppendNotNavigate(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	// FSKAX's ticker contains 'k' (a vim Up alias) and its name "Fidelity Total
	// Market Index" contains 'k'/... — these letters must append to the query,
	// not be consumed as cursor navigation.
	typeFilter(app, "fsk")
	if app.investmentFilterQuery != "fsk" {
		t.Fatalf("query = %q, want \"fsk\" (j/k must append to the query, not navigate)", app.investmentFilterQuery)
	}
	vis := app.visibleInvestmentTransactions()
	if len(vis) != 2 {
		t.Fatalf("query 'fsk': visible = %d, want 2 (FSKAX rows)", len(vis))
	}
	for _, txn := range vis {
		if !txn.SecurityID.Valid || txn.SecurityID.ID != ids.fskax {
			t.Errorf("expected only FSKAX rows, got %v", txn.Type)
		}
	}

	// A bare 'j' must also append.
	app2, _ := newFilterTestApp(t, 0)
	app2.handleInvestmentRegisterKeys(slashKey())
	app2.handleInvestmentRegisterKeys(letterKey('j'))
	if app2.investmentFilterQuery != "j" {
		t.Errorf("bare 'j' should append, query = %q, want \"j\"", app2.investmentFilterQuery)
	}
}

func TestInvestmentFilter_ArrowKeysNavigateWhileTyping(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	app.handleInvestmentRegisterKeys(slashKey()) // empty query -> all rows shown
	app.handleInvestmentRegisterKeys(tea.KeyPressMsg{Code: tea.KeyDown})
	if app.investmentTable.Cursor() != 1 {
		t.Errorf("real Down arrow while typing should navigate: cursor = %d, want 1", app.investmentTable.Cursor())
	}
	if app.investmentFilterQuery != "" {
		t.Errorf("arrow key must not append to the query, got %q", app.investmentFilterQuery)
	}
}

func TestInvestmentFilter_EscClearsWhileSidebarFocused(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock FXAIX
	app.sidebar.SetFocused(true)                 // Tab to the sidebar, filter still locked

	app.handleInvestmentRegisterKeys(escapeKey())
	if app.investmentRegisterFilterActive() {
		t.Error("Esc should clear the locked filter even when the sidebar is focused")
	}
}

func TestInvestmentFilter_EscClearsFromSearching(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(escapeKey())

	if app.investmentRegisterFilterActive() {
		t.Error("Esc should clear the filter from searching mode")
	}
	if len(app.visibleInvestmentTransactions()) != 7 {
		t.Errorf("after Esc: visible = %d, want 7 (all rows)", len(app.visibleInvestmentTransactions()))
	}
}

func TestInvestmentFilter_EscClearsFromLocked(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock
	app.handleInvestmentRegisterKeys(escapeKey())

	if app.investmentRegisterFilterActive() {
		t.Error("Esc should clear a locked filter")
	}
	if len(app.visibleInvestmentTransactions()) != 7 {
		t.Errorf("after Esc from locked: visible = %d, want 7", len(app.visibleInvestmentTransactions()))
	}
}

func TestInvestmentFilter_BackspaceEditsQuery(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(tea.KeyPressMsg{Code: tea.KeyBackspace})

	if app.investmentFilterQuery != "f" {
		t.Errorf("after backspace query = %q, want \"f\"", app.investmentFilterQuery)
	}
}

func TestInvestmentFilter_TickerlessSecurity(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	typeFilter(app, "mid cap") // matches the tickerless MFS name only
	if len(app.visibleInvestmentTransactions()) != 1 {
		t.Fatalf("query 'mid cap': visible = %d, want 1", len(app.visibleInvestmentTransactions()))
	}
	app.handleInvestmentRegisterKeys(enterKey())
	if app.investmentFilterLockedSec != ids.mfs {
		t.Errorf("locked security = %v, want MFS", app.investmentFilterLockedSec)
	}
	// Tickerless: display name is the plain name with no " — ticker" prefix.
	if name := app.securityDisplayName(ids.mfs); name != "MFS Mid Cap Value CT" {
		t.Errorf("tickerless display name = %q, want %q", name, "MFS Mid Cap Value CT")
	}
}

func TestInvestmentFilter_SecurityDisplayNameWithTicker(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)
	if got := app.securityDisplayName(ids.fxaix); got != "FXAIX — Fidelity 500 Index Fund" {
		t.Errorf("display name = %q, want %q", got, "FXAIX — Fidelity 500 Index Fund")
	}
}

func TestInvestmentFilter_SelectedTransactionIndexesFilteredSlice(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock FXAIX, cursor at 0

	app.investmentTable.MoveDown() // to filtered row index 1
	sel := app.selectedInvestmentTransaction()
	if sel == nil {
		t.Fatal("selectedInvestmentTransaction returned nil")
	}
	if !sel.SecurityID.Valid || sel.SecurityID.ID != ids.fxaix {
		t.Errorf("selected row is not an FXAIX row: %v", sel.Type)
	}
	visible := app.visibleInvestmentTransactions()
	if sel != visible[1] {
		t.Error("selected transaction must index the filtered slice, not the full ledger")
	}
}

func TestInvestmentFilter_BalanceColumnHiddenWhileFiltered(t *testing.T) {
	app, _ := newFilterTestApp(t, 200) // wide enough for the Balance column

	// Sanity: unfiltered, the Balance column is present.
	cols := app.investmentTable.Columns()
	if cols[len(cols)-1].Header != "Balance" {
		t.Fatalf("precondition: unfiltered register should show Balance column, got %q", cols[len(cols)-1].Header)
	}

	app.handleInvestmentRegisterKeys(slashKey())
	cols = app.investmentTable.Columns()
	for _, c := range cols {
		if c.Header == "Balance" {
			t.Error("Balance column should be hidden while filtering")
		}
	}
}

func TestInvestmentFilter_TotalReturnHeaderHiddenWhileFiltered(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	unfiltered := widget.StripAnsi(app.renderInvestmentRegister())
	if !strings.Contains(unfiltered, "Total return") {
		t.Fatalf("precondition: unfiltered render should show the total-return header")
	}

	typeFilter(app, "fx")
	filtered := widget.StripAnsi(app.renderInvestmentRegister())
	if strings.Contains(filtered, "Total return") {
		t.Error("total-return header should be hidden while filtered")
	}
	if !strings.Contains(filtered, "Filter:") {
		t.Error("filtered render should show the Filter: status line")
	}
}

func TestInvestmentFilter_NoMatchStatusLine(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "zzzz")
	if len(app.visibleInvestmentTransactions()) != 0 {
		t.Errorf("query 'zzzz': visible = %d, want 0", len(app.visibleInvestmentTransactions()))
	}
	if line := app.investmentFilterStatusLine(); !strings.Contains(line, "no matches") {
		t.Errorf("status line %q should report no matches", line)
	}
}

func TestInvestmentFilter_NewPressSeedsPreselectWhenLocked(t *testing.T) {
	app, ids := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock FXAIX

	app.handleInvestmentRegisterKeys(letterKey('n')) // open type selector
	if app.investmentTypeSelector == nil {
		t.Fatal("pressing n should open the investment type selector")
	}
	if app.investmentNewTxnSecurityID != ids.fxaix {
		t.Errorf("new-txn preselect = %v, want FXAIX (the locked security)", app.investmentNewTxnSecurityID)
	}
}

func TestInvestmentFilter_PreselectSecurityCombo(t *testing.T) {
	secA, secB := types.NewID(), types.NewID()
	secIDs := []types.ID{secA, secB}
	d := buildBuyDialog([]string{"AAPL", "MSFT"}, nil, secIDs)

	preselectSecurityCombo(d, secIDs, secB)
	f := d.FieldByLabel("Security")
	if f == nil {
		t.Fatal("Security field not found")
	}
	if f.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1 (secB)", f.SelectedIndex)
	}

	// Nil / unknown security is a no-op.
	preselectSecurityCombo(d, secIDs, types.NilID)
	if f.SelectedIndex != 1 {
		t.Errorf("nil preselect should be a no-op, SelectedIndex = %d", f.SelectedIndex)
	}
}

func TestInvestmentFilter_EarlyGuardCapturesGlobalKeys(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	app.handleInvestmentRegisterKeys(slashKey()) // enter searching

	// '5' is the global Prices-view shortcut; while searching it must be
	// captured as query text, not switch views.
	app.handleKeyPress(tea.KeyPressMsg{Code: '5', Text: "5"})
	if app.currentView != ViewInvestmentRegister {
		t.Errorf("global digit key switched views while searching: view = %v", app.currentView)
	}
	if app.investmentFilterQuery != "5" {
		t.Errorf("digit should append to query, got %q", app.investmentFilterQuery)
	}
}

func TestInvestmentFilter_ResizeWhileFilteredDoesNotRebuild(t *testing.T) {
	app, _ := newFilterTestApp(t, 200) // wide: balance column shown unfiltered

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock FXAIX (3 rows)

	// Plant a row-style marker. buildInvestmentRegisterTable -> SetRows clears
	// row styles, so the marker's survival proves the resize tick did NOT
	// rebuild the table — which is the whole point of the filter-aware resize
	// check (an unfiltered-only check would rebuild on every resize while
	// filtered because the balance column is hidden).
	app.investmentTable.SetRowStyle(0, widget.RowStyleVoid)

	app.Update(tea.WindowSizeMsg{Width: 200, Height: 40})

	if _, ok := app.investmentTable.RowStyles()[0]; !ok {
		t.Error("resize while filtered rebuilt the table (row style was cleared) — the resize check is not filter-aware")
	}
	// And the balance column stays hidden regardless of width while filtered.
	cols := app.investmentTable.Columns()
	for _, c := range cols {
		if c.Header == "Balance" {
			t.Error("Balance column should remain hidden while filtered after resize")
		}
	}
}

func TestInvestmentFilter_ClearedOnLeavingView(t *testing.T) {
	app, _ := newFilterTestApp(t, 0)

	typeFilter(app, "fx")
	app.handleInvestmentRegisterKeys(enterKey()) // lock

	app.switchView(ViewDashboard)
	if app.investmentRegisterFilterActive() {
		t.Error("leaving the investment register should clear the filter")
	}
}

// preselectSecurityCombo must tolerate a dialog whose Security combo does not
// contain the requested id.
func TestInvestmentFilter_PreselectUnknownSecurityIsNoOp(t *testing.T) {
	secA := types.NewID()
	secIDs := []types.ID{secA}
	d := buildBuyDialog([]string{"AAPL"}, nil, secIDs)
	preselectSecurityCombo(d, secIDs, types.NewID()) // not in secIDs
	if f := d.FieldByLabel("Security"); f.SelectedIndex != 0 {
		t.Errorf("unknown preselect should leave SelectedIndex at 0, got %d", f.SelectedIndex)
	}
}
