package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// reconReloadEnv is a checking account with an open reconciliation session on
// screen, its candidates loaded.
type reconReloadEnv struct {
	a     *App
	acct  *account.Account
	rent  *transaction.Transaction
	power *transaction.Transaction
}

func newReconReloadEnv(t *testing.T) reconReloadEnv {
	t.Helper()
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40

	acct := account.NewAccount("Contoso Checking", account.TypeChecking, "USD",
		types.MustNewMoney("100.00"), types.MustParseDate("2024-01-01"))
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	rent := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-02-01"), types.MustNewMoney("-10.00"))
	power := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-02-02"), types.MustNewMoney("-25.00"))
	for _, txn := range []*transaction.Transaction{rent, power} {
		if err := a.services.Transaction.Create(txn); err != nil {
			t.Fatal(err)
		}
	}

	session, err := a.services.Reconciliation.StartReconciliation(acct.ID, types.MustParseDate("2024-02-28"), types.MustNewMoney("65.00"))
	if err != nil {
		t.Fatal(err)
	}
	a.switchView(ViewReconciliation)
	runCmd(t, a, a.loadReconciliationData(session, acct), 1)
	if a.reconciliation.data == nil || len(a.reconciliation.data.candidates) != 2 {
		t.Fatal("setup: expected two candidates on screen")
	}
	return reconReloadEnv{a: a, acct: acct, rent: rent, power: power}
}

func (e reconReloadEnv) checkBoth() {
	e.a.reconciliation.data.checkedIDs[e.rent.ID] = true
	e.a.reconciliation.data.checkedIDs[e.power.ID] = true
}

// A reload on the Reconciliation view re-reads the candidates and keeps the
// check marks whose rows survived: the marks exist only in memory until
// Finish, so dropping them would erase the user's work.
func TestReloadCurrentView_Reconciliation_KeepsSurvivingChecks(t *testing.T) {
	e := newReconReloadEnv(t)
	e.checkBoth()

	if err := e.a.services.Transaction.Delete(e.rent.ID); err != nil {
		t.Fatal(err)
	}
	runCmd(t, e.a, e.a.reloadCurrentView(), 2)

	r := e.a.reconciliation.data
	if len(r.candidates) != 1 || r.candidates[0].ID != e.power.ID {
		t.Fatalf("candidates = %d rows, want only the surviving one", len(r.candidates))
	}
	if !r.checkedIDs[e.power.ID] {
		t.Error("the surviving row lost its check mark")
	}
	if r.checkedIDs[e.rent.ID] {
		t.Error("the deleted row kept its check mark")
	}
	// Opening 100.00 plus the one checked row, -25.00.
	if want := types.MustNewMoney("75.00"); !r.clearedTotal.Equal(want) {
		t.Errorf("cleared total = %s, want %s", r.clearedTotal, want)
	}
}

// Undo is the path the reload exists for: undoing a create made on another
// surface must drop that row from the candidate table on screen.
func TestReloadCurrentView_Reconciliation_UndoRefreshesCandidates(t *testing.T) {
	e := newReconReloadEnv(t)
	e.checkBoth()

	extra := transaction.NewTransaction(e.acct.ID, types.MustParseDate("2024-02-03"), types.MustNewMoney("-5.00"))
	if err := e.a.undoManager.Execute(undo.NewCreateTransactionCommand(e.a.services.Transaction, extra)); err != nil {
		t.Fatal(err)
	}
	runCmd(t, e.a, e.a.reloadCurrentView(), 2)
	if len(e.a.reconciliation.data.candidates) != 3 {
		t.Fatalf("setup: candidates = %d, want 3", len(e.a.reconciliation.data.candidates))
	}

	runCmd(t, e.a, e.a.performUndo(), 4)

	r := e.a.reconciliation.data
	if len(r.candidates) != 2 {
		t.Errorf("after undo, candidates = %d, want 2", len(r.candidates))
	}
	if !r.checkedIDs[e.rent.ID] || !r.checkedIDs[e.power.ID] {
		t.Error("undo dropped check marks on rows it did not touch")
	}
}

// The reload runs off the update loop. A mark toggled after it starts and
// before its message lands must survive: the marks are filtered when the
// message is applied, not copied when the load begins.
func TestReloadCurrentView_Reconciliation_KeepsToggleMadeDuringLoad(t *testing.T) {
	e := newReconReloadEnv(t)

	cmd := e.a.reloadCurrentView()
	e.a.reconciliation.data.checkedIDs[e.power.ID] = true
	runCmd(t, e.a, cmd, 2)

	r := e.a.reconciliation.data
	if !r.checkedIDs[e.power.ID] {
		t.Error("a mark made while the reload was in flight was lost")
	}
	if r.checkedIDs[e.rent.ID] {
		t.Error("an unchecked row came back checked")
	}
	// Opening 100.00 plus the one checked row, -25.00.
	if want := types.MustNewMoney("75.00"); !r.clearedTotal.Equal(want) {
		t.Errorf("cleared total = %s, want %s", r.clearedTotal, want)
	}
}

// A reload that lands after the session left the screen (cancelled while the
// load was in flight) must not bring the session back.
func TestReloadCurrentView_Reconciliation_DropsReloadForClosedSession(t *testing.T) {
	e := newReconReloadEnv(t)

	cmd := e.a.reloadCurrentView()
	e.a.afterReconciliationCancelled()
	runCmd(t, e.a, cmd, 2)

	if e.a.reconciliation.data != nil {
		t.Error("a late reload put a cancelled session back on screen")
	}
}

// With no session on screen (the first load is still in flight), the reload
// adds nothing and must not start one.
func TestReloadCurrentView_Reconciliation_NoSessionStartsNothing(t *testing.T) {
	a := NewApp(dbtest.New(t), nil)
	acct := account.NewAccount("Contoso Checking", account.TypeChecking, "USD",
		types.ZeroMoney, types.MustParseDate("2024-01-01"))
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	a.currentView = ViewReconciliation

	runCmd(t, a, a.reloadCurrentView(), 1)

	if a.reconciliation.data != nil {
		t.Error("reload filled the view with no session on screen")
	}
	if s, err := a.services.Reconciliation.GetActiveSession(acct.ID); err != nil || s != nil {
		t.Errorf("reload started a session: %v, %v", s, err)
	}
}

// A reload on the Corporate Actions view re-reads the history table.
func TestReloadCurrentView_CorporateActions_RefreshesHistory(t *testing.T) {
	a := NewApp(dbtest.New(t), nil)
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := a.services.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	a.switchView(ViewCorporateActions)
	runCmd(t, a, a.loadCorporateActionViewData(), 1)
	if a.corporateActions.data == nil || len(a.corporateActions.data.actions) != 0 {
		t.Fatal("setup: expected an empty history on screen")
	}

	if _, err := a.services.CorporateAction.Split(sec.ID, types.NewDate(2024, time.June, 1),
		investment.SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatal(err)
	}
	runCmd(t, a, a.reloadCurrentView(), 1)

	if got := len(a.corporateActions.data.actions); got != 1 {
		t.Errorf("history rows after reload = %d, want 1", got)
	}
}
