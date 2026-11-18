package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// searchRows names the transactions newRegisterSearchApp loads, in register
// order.
type searchRows struct {
	grocer, salary, rent, coffee *transaction.Transaction
}

// newRegisterSearchApp builds a register App over four transactions, with the
// table focused.
func newRegisterSearchApp(t *testing.T) (*App, searchRows) {
	t.Helper()
	acctID := types.NewID()
	row := func(day int, amount string) *transaction.Transaction {
		return &transaction.Transaction{
			BaseModel: types.BaseModel{ID: types.NewID()},
			AccountID: acctID,
			Date:      types.NewDate(2026, time.March, day),
			Amount:    types.MustNewMoney(amount),
			Status:    transaction.StatusUncleared,
		}
	}
	r := searchRows{
		grocer: row(1, "-125.43"),
		salary: row(2, "2500.00"),
		rent:   row(3, "-1500.00"),
		coffee: row(4, "-5.75"),
	}
	names := map[types.ID]string{}
	cats := map[types.ID]string{}
	for txn, p := range map[*transaction.Transaction][2]string{
		r.grocer: {"Fresh Grocer", "Groceries"},
		r.salary: {"Acme Corp", "Salary"},
		r.rent:   {"Landlord", "Rent"},
		r.coffee: {"Bean There", "Dining"},
	} {
		txn.PayeeID = types.NullableID{ID: types.NewID(), Valid: true}
		names[txn.PayeeID.ID] = p[0]
		txn.CategoryID = types.NullableID{ID: types.NewID(), Valid: true}
		cats[txn.CategoryID.ID] = p[1]
	}
	r.grocer.SetMemo("weekly shop")
	r.rent.CheckNumber = types.NullableString{String: "1042", Valid: true}

	styles := widget.NewStyles()
	styles.Resize(160, 40)
	sb := sidebar.New()
	sb.SetFocused(false)
	a := &App{
		currentView: ViewRegister,
		width:       160,
		height:      40,
		ready:       true,
		keys:        defaultKeyMap(),
		styles:      styles,
		sidebar:     sb,
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		// A service the row actions check for. No test runs a confirm, so it
		// is never called.
		services: app.Services{Transaction: &transaction.Service{}},
		register: registerViewState{data: &registerData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: acctID}, Name: "Checking", Type: account.TypeChecking, Active: true},
			transactions:  []*transaction.Transaction{r.grocer, r.salary, r.rent, r.coffee},
			balance:       &account.Balance{AccountID: acctID, CurrentBalance: types.ZeroMoney},
			payeeNames:    names,
			categoryNames: cats,
			accountNames:  map[types.ID]string{},
		}},
	}
	a.buildRegisterTable()
	a.register.table.SetFocused(true)
	return a, r
}

func regKey(a *App, msgs ...tea.KeyPressMsg) {
	for _, m := range msgs {
		a.handleKeyPress(m)
	}
}

func regType(a *App, s string) {
	for _, r := range s {
		regKey(a, letterKey(r))
	}
}

var (
	regEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	regEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func TestRegisterSearch_SlashNarrowsAsYouType(t *testing.T) {
	a, r := newRegisterSearchApp(t)

	regKey(a, slashKey())
	if !a.register.searching {
		t.Fatal("/ did not start the search")
	}
	if len(a.register.shown) != 4 {
		t.Fatalf("an empty query shows %d rows, want all 4", len(a.register.shown))
	}
	regType(a, "groc")

	if got := a.register.shown; len(got) != 1 || got[0] != r.grocer {
		t.Fatalf("search \"groc\" shows %d rows, want only the grocer", len(got))
	}
	out := a.renderRegister()
	if !strings.Contains(out, "Search: groc") || !strings.Contains(out, "(1 of 4)") {
		t.Errorf("the search line is missing:\n%s", out)
	}
}

// The search reads the memo, the check number and the plain amount too.
func TestRegisterSearch_Matches(t *testing.T) {
	for query, want := range map[string]string{
		"weekly":  "grocer", // memo
		"1042":    "rent",   // check number
		"-1500":   "rent",   // plain amount; the register shows -$1500.00
		"1,500":   "rent",   // thousands comma typed
		"$1500.0": "rent",   // the amount as shown
		"acme":    "salary", // payee, any case
		"dining":  "coffee", // category
		"03/04/2": "coffee", // date as shown
	} {
		a, r := newRegisterSearchApp(t)
		byName := map[string]*transaction.Transaction{"grocer": r.grocer, "salary": r.salary, "rent": r.rent, "coffee": r.coffee}
		regKey(a, slashKey())
		regType(a, query)
		if got := a.register.shown; len(got) != 1 || got[0] != byName[want] {
			t.Errorf("search %q shows %d rows, want only the %s row", query, len(got), want)
		}
	}
}

// While a query is typed, letters are text: "j" and "k" do not move the cursor.
func TestRegisterSearch_LettersAreQueryText(t *testing.T) {
	a, _ := newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "jk?2q")

	if a.register.query != "jk?2q" {
		t.Errorf("query = %q, want jk?2q", a.register.query)
	}
	if a.currentView != ViewRegister || a.showHelp || a.quitting {
		t.Error("a typed letter ran a command")
	}
}

// Enter keeps the found rows, and the register keys act on them: the cursor
// row is the found row, not the row at that position in the full register.
func TestRegisterSearch_EnterKeepsAndKeysActOnTheFoundRow(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	r.rent.Status = transaction.StatusReconciled
	a.buildRegisterTable()

	regKey(a, slashKey())
	regType(a, "rent")
	regKey(a, regEnter)

	if a.register.searching || a.register.query != "rent" {
		t.Fatalf("after Enter: searching=%v query=%q, want the query kept", a.register.searching, a.register.query)
	}
	if got := a.register.selectedTransaction(); got != r.rent {
		t.Fatal("the cursor row is not the found row")
	}
	// d on the found (reconciled) row is refused. Row 0 of the full register
	// is the grocer, which d would offer to delete.
	regKey(a, letterKey('d'))
	if a.confirm.dlg != nil {
		t.Error("d acted on the full register's row 0, not on the found row")
	}
}

// Esc clears a kept search, stays in the register, and keeps the cursor on the
// row the search found.
func TestRegisterSearch_EscClearsAndKeepsTheRow(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "rent")
	regKey(a, regEnter, regEsc)

	if a.register.searchActive() {
		t.Fatal("Esc did not clear the search")
	}
	if a.currentView != ViewRegister {
		t.Fatalf("Esc left the register for %v", a.currentView)
	}
	if len(a.register.shown) != 4 {
		t.Errorf("after Esc the register shows %d rows, want all 4", len(a.register.shown))
	}
	if got := a.register.selectedTransaction(); got != r.rent {
		t.Error("after Esc the cursor left the found row")
	}
}

func TestRegisterSearch_EscWhileTypingClears(t *testing.T) {
	a, _ := newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "zzz")
	if len(a.register.shown) != 0 {
		t.Fatalf("search \"zzz\" shows %d rows, want none", len(a.register.shown))
	}
	if out := a.renderRegister(); !strings.Contains(out, "no matches") {
		t.Errorf("no \"no matches\" on the search line:\n%s", out)
	}
	regKey(a, regEsc)

	if a.register.searchActive() || len(a.register.shown) != 4 || a.currentView != ViewRegister {
		t.Error("Esc while typing must clear the search and stay in the register")
	}
}

// The running balance is the account's balance after each row, so a found
// row shows the same balance as in the full register.
func TestRegisterSearch_BalanceStaysTrue(t *testing.T) {
	a, _ := newRegisterSearchApp(t)
	full := a.register.table.Rows()[2] // the rent row
	balance := full[len(full)-1]

	regKey(a, slashKey())
	regType(a, "rent")

	got := a.register.table.Rows()[0]
	if got[len(got)-1] != balance {
		t.Errorf("found row balance = %q, want %q as in the full register", got[len(got)-1], balance)
	}
}

// A mouse click maps to the right row: the search line is one more row above
// the table.
func TestRegisterSearch_RowOffsetCountsTheSearchLine(t *testing.T) {
	a, _ := newRegisterSearchApp(t)
	before := a.tableContentRowOffset()
	regKey(a, slashKey())
	if got := a.tableContentRowOffset(); got != before+1 {
		t.Errorf("row offset = %d with the search line, want %d", got, before+1)
	}
}

// / again starts a new search over every row, and the cursor stays on the row
// the kept search found.
func TestRegisterSearch_SlashAgainKeepsTheRow(t *testing.T) {
	for name, slash := range map[string]func(*App){
		"key":  func(a *App) { regKey(a, slashKey()) },
		"menu": func(a *App) { a.menuSearchTransactions() },
	} {
		a, r := newRegisterSearchApp(t)
		regKey(a, slashKey())
		regType(a, "rent")
		regKey(a, regEnter)

		slash(a)

		if !a.register.searching || a.register.query != "" || len(a.register.shown) != 4 {
			t.Errorf("%s: searching=%v query=%q rows=%d, want a new search over all 4",
				name, a.register.searching, a.register.query, len(a.register.shown))
		}
		if got := a.register.selectedTransaction(); got != r.rent {
			t.Errorf("%s: the new search moved the cursor off the found row", name)
		}
	}
}

// reloadRegister lands a reload of the register's ledger, as after a save.
func reloadRegister(a *App) {
	a.Update(registerLoadedMsg{data: a.register.data})
}

// A save selects the saved row when its reload lands. When the kept search
// hides that row, the search ends so the cursor can go to it.
func TestRegisterSearch_SaveSelectsTheSavedRow(t *testing.T) {
	keep := func(t *testing.T, query string) (*App, searchRows) {
		a, r := newRegisterSearchApp(t)
		regKey(a, slashKey())
		regType(a, query)
		regKey(a, regEnter)
		return a, r
	}

	t.Run("hidden by the search", func(t *testing.T) {
		a, r := keep(t, "rent")
		a.pendingRegisterSelectID = r.coffee.ID
		reloadRegister(a)
		if a.register.searchActive() {
			t.Error("the search hides the saved row and stays")
		}
		if got := a.register.selectedTransaction(); got != r.coffee {
			t.Error("the cursor is not on the saved row")
		}
		if !a.pendingRegisterSelectID.IsNil() {
			t.Error("the pending selection stays set")
		}
	})

	t.Run("shown by the search", func(t *testing.T) {
		a, r := keep(t, "an") // Landlord and Bean There
		a.pendingRegisterSelectID = r.coffee.ID
		reloadRegister(a)
		if a.register.query != "an" || len(a.register.shown) != 2 {
			t.Errorf("query=%q rows=%d, want the search kept", a.register.query, len(a.register.shown))
		}
		if got := a.register.selectedTransaction(); got != r.coffee {
			t.Error("the cursor is not on the saved row")
		}
	})

	// A reload from before the save does not hold the saved row. The search
	// and the pending selection stay for the next reload.
	t.Run("not loaded yet", func(t *testing.T) {
		a, _ := keep(t, "rent")
		pending := types.NewID()
		a.pendingRegisterSelectID = pending
		reloadRegister(a)
		if a.register.query != "rent" || a.pendingRegisterSelectID != pending {
			t.Errorf("query=%q, want the search and the pending selection kept", a.register.query)
		}
	})

	// While the user types a new search, the query has priority: the search
	// stays in typing mode, so the next keys are still query text.
	t.Run("typing a search", func(t *testing.T) {
		a, r := newRegisterSearchApp(t)
		regKey(a, slashKey())
		regType(a, "rent")
		a.pendingRegisterSelectID = r.coffee.ID
		reloadRegister(a)
		if !a.register.searching || a.register.query != "rent" {
			t.Fatalf("searching=%v query=%q, want the typed search kept", a.register.searching, a.register.query)
		}
		if !a.pendingRegisterSelectID.IsNil() {
			t.Error("the pending selection stays set after the typed search won")
		}
		regType(a, "d")
		if a.register.query != "rentd" || a.confirm.dlg != nil {
			t.Error("a key after the reload ran a register command")
		}
	})

	// The cursor does not move while the user types, also when the saved row
	// is one of the matches.
	t.Run("typing a search that shows the saved row", func(t *testing.T) {
		a, r := newRegisterSearchApp(t)
		regKey(a, slashKey())
		regType(a, "an") // Landlord, then Bean There
		a.pendingRegisterSelectID = r.coffee.ID
		reloadRegister(a)
		if got := a.register.selectedTransaction(); got != r.rent {
			t.Error("the reload moved the cursor during a typed search")
		}
		if !a.pendingRegisterSelectID.IsNil() {
			t.Error("the pending selection stays set after the typed search won")
		}
	})
}

// A rebuild that is not a reload does not use up a save's pending selection:
// the ledger can be from before the save, with the saved row at its old
// position. Leaving the register with a search active rebuilds the table.
func TestRegisterSearch_LeaveKeepsThePendingSelection(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "rent")
	regKey(a, regEnter)
	a.pendingRegisterSelectID = r.grocer.ID // an edit of a row the old ledger holds

	a.switchView(ViewDashboard)

	if a.pendingRegisterSelectID != r.grocer.ID {
		t.Error("the leave rebuild used up the pending selection before the reload")
	}
}

func TestRegisterSearch_ClearedOnLeaveAndAccountOpen(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "rent")
	regKey(a, regEnter)
	a.switchView(ViewDashboard)
	if a.register.searchActive() {
		t.Error("leaving the register kept the search")
	}
	// The table shows every row at once, not only when the reload ends.
	if n := len(a.register.table.Rows()); n != 4 {
		t.Errorf("after leaving, the table shows %d rows, want all 4", n)
	}
	if got := a.register.selectedTransaction(); got != r.rent {
		t.Error("after leaving, the cursor left the found row")
	}

	a, _ = newRegisterSearchApp(t)
	regKey(a, slashKey())
	regType(a, "rent")
	a.register.clearForLoad()
	if a.register.searchActive() {
		t.Error("loading another account kept the search")
	}
}

func TestRegisterSearch_PasteGoesToTheQuery(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	regKey(a, slashKey())

	paste(a, "Acme\n")

	if got := a.register.shown; len(got) != 1 || got[0] != r.salary {
		t.Errorf("pasted search shows %d rows, want only the salary", len(got))
	}
}

// A closed account's register can still be searched: a search only reads.
func TestRegisterSearch_WorksOnAClosedAccount(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	a.register.data.account.Active = false
	regKey(a, slashKey())
	regType(a, "acme")

	if got := a.register.shown; len(got) != 1 || got[0] != r.salary {
		t.Errorf("search on a closed account shows %d rows, want the salary", len(got))
	}
}
