package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// TestUpdateDropsALoadOfAnotherFile: a message tagged with a handle that is
// not the open one comes from a load of an earlier file, and changes nothing.
// A message tagged with the open handle, or not tagged, applies.
func TestUpdateDropsALoadOfAnotherFile(t *testing.T) {
	a := NewApp(dbtest.New(t), &config.Config{})
	fictional := errors.New("fictional load failure")

	a.Update(errMsg{err: fictional, fileOf: fileOf{db: &db.DB{}}})
	if a.err != nil {
		t.Errorf("the error of a load of another file was shown: %v", a.err)
	}
	a.Update(errMsg{err: fictional, fileOf: a.fileOfOpen()})
	if !errors.Is(a.err, fictional) {
		t.Errorf("the error of a load of the open file = %v, want it shown", a.err)
	}
	a.err = nil
	a.Update(errMsg{err: fictional})
	if !errors.Is(a.err, fictional) {
		t.Errorf("an error with no file = %v, want it shown", a.err)
	}
}

// TestALoadOfTheOldFileShowsNothing: a load of A is made, and File > Open
// switches to B before it runs. It read B's services when it ran: the
// dashboard of B got data that the load of A made, and the register load of
// an account of A failed in B ("not found") and showed that error. Now the
// load reads A, as it was made for, and Update drops its result, or its error
// when the handle of A is closed by then.
func TestALoadOfTheOldFileShowsNothing(t *testing.T) {
	for _, switches := range []int{1, 2} {
		for name, load := range map[string]func(a *App, acct *account.Account) tea.Cmd{
			"dashboard": func(a *App, _ *account.Account) tea.Cmd { return a.dashboard.load(a.dashboardDeps()) },
			"register":  func(a *App, acct *account.Account) tea.Cmd { return a.loadRegisterData(acct.ID) },
		} {
			t.Run(fmt.Sprintf("%s after %d switches", name, switches), func(t *testing.T) {
				first, _ := dbtest.NewFile(t, "fictional-a.tdb")
				acct := account.NewAccount("Cedar Fictional", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
				if err := account.NewRepository(first).Create(acct); err != nil {
					t.Fatal(err)
				}
				a := NewApp(first, &config.Config{})
				closeAll(t, a)
				cmd := load(a, acct)
				switchTo(t, a, closedFile(t, "fictional-b.tdb"))
				if switches == 2 {
					switchTo(t, a, closedFile(t, "fictional-c.tdb"))
				}

				msg := cmd()
				readA := false
				switch m := msg.(type) {
				case registerLoadedMsg:
					readA = m.data.account.ID == acct.ID
				case dashboardLoadedMsg:
					_, readA = m.data.accountNames[acct.ID]
				case errMsg:
					readA = switches == 2 && strings.Contains(m.err.Error(), "database is closed")
				}
				if !readA {
					t.Errorf("the %s load of A did not read A: %#v", name, msg)
				}
				a.Update(msg)
				if a.err != nil {
					t.Errorf("the %s load of A showed an error in another file: %v", name, a.err)
				}
				if a.dashboard.data != nil || a.register.data != nil {
					t.Errorf("the %s load of A filled a view of another file", name)
				}
			})
		}
	}
}

// TestAWriteRunsOnItsOwnFile: a write is asked for in A, and File > Open
// switches to B before it runs. It read B's services when it ran, so the
// security that the user added in A went into B.
func TestAWriteRunsOnItsOwnFile(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	cmd := a.createSecurity("FICT", "Fictional Index Fund", "", security.TypeETF,
		security.AssetClassLargeCapStock, "USD", "")
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	if msg, ok := cmd().(errMsg); ok {
		t.Fatalf("the write failed: %v", msg.err)
	}
	inA, err := security.NewRepository(first).List(security.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	inB, err := security.NewRepository(a.db).List(security.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inA) != 1 || len(inB) != 0 {
		t.Errorf("A has %d securities and B has %d; want the new one in A only", len(inA), len(inB))
	}
}

// selectAccount makes acct the selected account of the sidebar of a.
func selectAccount(t *testing.T, a *App, acct *account.Account) {
	t.Helper()
	a.sidebar.SetAccounts([]*account.Account{acct})
	if !a.sidebar.SetCursorToAccount(acct.ID) || !a.sidebar.Select() {
		t.Fatalf("could not select %s in the sidebar", acct.Name)
	}
}

// newAccountIn adds a fictional checking account to the file of d.
func newAccountIn(t *testing.T, d *db.DB) *account.Account {
	t.Helper()
	acct := account.NewAccount("Cedar Fictional", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := account.NewRepository(d).Create(acct); err != nil {
		t.Fatal(err)
	}
	return acct
}

// TestAWriteOfTheOldFileStaysOutOfTheNewFile: a write of A ends after a
// switch to B. Its undo step went into B's history, so an undo in B changed A
// with no sign on screen; and its result reloaded and changed the view of B.
// The undo step stays with A, and Update drops the result.
func TestAWriteOfTheOldFileStaysOutOfTheNewFile(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	acct := newAccountIn(t, first)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	selectAccount(t, a, acct)
	cmd := a.deleteSelectedAccount()
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	msg := cmd()
	if m, ok := msg.(fromFile); !ok || m.loadedFrom() != first {
		t.Errorf("the result of the delete in A = %#v, want it tagged with A", msg)
	}
	if a.fromAnotherFile(msg) == false {
		t.Error("Update would apply the result of a write of A in B")
	}
	if a.undoManager.CanUndo() {
		t.Errorf("the undo history of B has %q, a step of A", a.undoManager.UndoDescription())
	}
}

// TestASwitchClearsTheSelectionOfTheOldFile: the sidebar kept the account
// selected in A, so a save in B (or a late result) reloaded A's account in B
// and showed "account not found".
func TestASwitchClearsTheSelectionOfTheOldFile(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	acct := newAccountIn(t, first)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	selectAccount(t, a, acct)
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	if id := a.sidebar.SelectedAccountID(); !id.IsNil() {
		t.Errorf("the sidebar of B has A's account %s selected", id)
	}
	_, cmd := a.Update(transactionDialogSavedMsg{})
	for _, m := range runBatch(cmd) {
		a.Update(m)
	}
	if a.err != nil {
		t.Errorf("a save in B showed: %v", a.err)
	}
}

// runBatch runs cmd, and each command of a tea.Batch, and returns their
// messages.
func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runBatch(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// TestAWriteFailureOfTheOldFileStillShows: the start of a reconciliation is a
// write inside a load. When it fails after a switch, the user must still see
// that it failed; the failure of a write is not tagged.
func TestAWriteFailureOfTheOldFileStillShows(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	acct := newAccountIn(t, first)
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	future := types.Today().AddDays(30)
	cmd := a.startReconciliation(acct.ID, future, types.ZeroMoney)
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	a.Update(cmd())
	if a.err == nil || !strings.Contains(a.err.Error(), "failed to start reconciliation") {
		t.Errorf("a.err = %v, want the failure of the start of the reconciliation of A", a.err)
	}
}

// TestDepsCopyTheConfig: a load reads the config while Update can change it
// (toggleClosedPositions); it must read the config as it was when the load
// was made.
func TestDepsCopyTheConfig(t *testing.T) {
	a := &App{cfg: &config.Config{}}
	for name, deps := range map[string]func() func() *config.Config{
		"dashboard":           func() func() *config.Config { return a.dashboardDeps().config },
		"portfolio":           func() func() *config.Config { return a.portfolioDeps().config },
		"investment register": func() func() *config.Config { return a.investmentRegisterDeps().config },
	} {
		a.cfg.ShowClosedPositions = false
		cfg := deps()
		a.cfg.ShowClosedPositions = true // Update flips it after the deps are made
		if cfg().ShowClosedPositions {
			t.Errorf("the %s deps saw a change that Update made after they were made", name)
		}
	}
}

// TestSecurityReloadKeepsTheShowHiddenToggle: "f" flips show-hidden while a
// reload of the Securities view runs. The reload must not undo the flip.
func TestSecurityReloadKeepsTheShowHiddenToggle(t *testing.T) {
	a := NewApp(dbtest.New(t), &config.Config{})
	a.securities.data = &securityViewData{}
	cmd := a.loadSecurityViewData()
	a.securities.data.showHidden = true // the "f" during the reload

	a.Update(cmd())
	if a.securities.data == nil || !a.securities.data.showHidden {
		t.Error("the reload of the Securities view undid the show-hidden toggle")
	}
}

// fictionalStep is an undo step that changes no data. Its Undo and its Redo
// return fail.
type fictionalStep struct{ fail error }

func (s fictionalStep) Execute() error    { return s.fail }
func (s fictionalStep) Undo() error       { return s.fail }
func (fictionalStep) Description() string { return "fictional step" }

var _ undo.Command = fictionalStep{}

// TestAnUndoOfTheOldFileShowsNothing: an undo in A ends after a switch to B.
// It showed "Undo: <the step of A>" in B and reloaded the view of B. Its
// success is dropped, as the success of a write is.
func TestAnUndoOfTheOldFileShowsNothing(t *testing.T) {
	for action, perform := range map[string]func(*App) tea.Cmd{
		"Undo": (*App).performUndo,
		"Redo": (*App).performRedo,
	} {
		t.Run(action, func(t *testing.T) {
			first, _ := dbtest.NewFile(t, "fictional-a.tdb")
			a := NewApp(first, &config.Config{})
			closeAll(t, a)
			a.undoManager.Push(fictionalStep{})
			if action == "Redo" {
				if _, err := a.undoManager.Undo(); err != nil {
					t.Fatal(err)
				}
			}
			cmd := perform(a)
			switchTo(t, a, closedFile(t, "fictional-b.tdb"))

			msg := cmd()
			if !a.fromAnotherFile(msg) {
				t.Errorf("the %s result of A = %#v, want it tagged with A", action, msg)
			}
			_, reload := a.Update(msg)
			for _, n := range a.statusbar.Notifications() {
				if strings.Contains(n.Text, "fictional step") {
					t.Errorf("B shows %q, the %s of a step of A", n.Text, action)
				}
			}
			if reload != nil {
				t.Errorf("the %s of A reloaded the view of B", action)
			}
		})
	}
}

// TestAnUndoFailureOfTheOldFileStillShows: the failure of an undo is not
// tagged, as the failure of a write is not.
func TestAnUndoFailureOfTheOldFileStillShows(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	fictional := errors.New("fictional undo failure")
	a.undoManager.Push(fictionalStep{fail: fictional})
	cmd := a.performUndo()
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	a.Update(cmd())
	if !errors.Is(a.err, fictional) {
		t.Errorf("a.err = %v, want the failure of the undo in A", a.err)
	}
}

// TestANewAccountOfTheOldFileOpensNothing: New Account is asked for on the
// screen of A, and a File > Open of B ends before its message comes back.
// The form must not open on the screen of B.
func TestANewAccountOfTheOldFileOpensNothing(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	cmd := a.loadNewAccountDialogData()
	switchTo(t, a, closedFile(t, "fictional-b.tdb"))

	a.Update(cmd())
	if a.acct.IsVisible() {
		t.Error("the New Account form of A opened on the screen of B")
	}
}
