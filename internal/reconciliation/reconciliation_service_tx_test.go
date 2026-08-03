package reconciliation

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// first Exec whose SQL contains failSubstr. Reads pass through untouched, so a
// flow proceeds normally until the targeted write.
type failingQueryer struct {
	inner      db.Queryer
	failSubstr string
	failed     bool
}

func (f *failingQueryer) Exec(query string, args ...any) (sql.Result, error) {
	if !f.failed && f.failSubstr != "" && strings.Contains(query, f.failSubstr) {
		f.failed = true
		return nil, fmt.Errorf("injected failure on %q", f.failSubstr)
	}
	return f.inner.Exec(query, args...)
}

func (f *failingQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	return f.inner.Query(query, args...)
}

func (f *failingQueryer) QueryRow(query string, args ...any) *sql.Row {
	return f.inner.QueryRow(query, args...)
}

// setup: an account with two cleared transactions and an in-progress session
// whose statement balance matches, so FinishReconciliation passes validation.
func setupFinishFixture(t *testing.T) (*db.DB, *Service, *Session, []types.ID) {
	t.Helper()
	database := createTestDB(t)
	reconRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)

	svc := NewService(reconRepo, txnRepo, accountRepo, database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo, database)

	acct := createTestCheckingAccount(t, accountRepo, "TxChecking", "1000.00")

	var ids []types.ID
	for _, amount := range []string{"-100.00", "-50.00"} {
		txn := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney(amount))
		txn.SetStatus(transaction.StatusCleared)
		if err := txnSvc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		ids = append(ids, txn.ID)
	}

	session, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 1, 31), types.MustNewMoney("850.00"))
	if err != nil {
		t.Fatalf("StartReconciliation() error = %v", err)
	}
	return database, svc, session, ids
}

// TestFinishReconciliation_FaultRollsBack injects a failure on the session
// completion write, after the per-transaction status updates have run in-tx.
// The whole finish must roll back: no transaction may be left reconciled with
// the session still open (the old multi-autocommit partial state).
func TestFinishReconciliation_FaultRollsBack(t *testing.T) {
	database, svc, session, ids := setupFinishFixture(t)

	err := database.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failSubstr: "reconciliation_sessions"}
		return svc.InTx(fw).FinishReconciliation(session.AccountID, ids, false)
	})
	if err == nil {
		t.Fatal("FinishReconciliation() expected injected error, got nil")
	}

	// No transaction may be reconciled.
	txnRepo := transaction.NewRepository(database)
	for _, id := range ids {
		txn, err := txnRepo.GetByID(id)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", id, err)
		}
		if txn.IsReconciled() {
			t.Errorf("transaction %s is reconciled after failed finish; want rolled back", id)
		}
	}

	// The session must still be the active in-progress one.
	active, err := svc.GetActiveSession(session.AccountID)
	if err != nil {
		t.Fatalf("GetActiveSession() error = %v", err)
	}
	if active == nil || active.ID != session.ID {
		t.Fatal("active session missing after failed finish; want the original in-progress session")
	}
}

// TestFinishReconciliation_HappyPathAtomic confirms the converted flow still
// reconciles every transaction and completes the session.
func TestFinishReconciliation_HappyPathAtomic(t *testing.T) {
	database, svc, session, ids := setupFinishFixture(t)

	if err := svc.FinishReconciliation(session.AccountID, ids, false); err != nil {
		t.Fatalf("FinishReconciliation() error = %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	for _, id := range ids {
		txn, err := txnRepo.GetByID(id)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", id, err)
		}
		if !txn.IsReconciled() {
			t.Errorf("transaction %s not reconciled", id)
		}
	}

	if active, err := svc.GetActiveSession(session.AccountID); err != nil {
		t.Fatalf("GetActiveSession() error = %v", err)
	} else if active != nil {
		t.Error("session still active after finish; want completed")
	}
}

// TestUndoFinish_FaultRollsBack injects a failure on the session reopen write,
// after the status restores ran in-tx. The undo must roll back whole: the
// transactions stay reconciled and the session stays completed.
func TestUndoFinish_FaultRollsBack(t *testing.T) {
	database, svc, session, ids := setupFinishFixture(t)

	if err := svc.FinishReconciliation(session.AccountID, ids, false); err != nil {
		t.Fatalf("FinishReconciliation() error = %v", err)
	}

	prev := map[types.ID]transaction.Status{}
	for _, id := range ids {
		prev[id] = transaction.StatusCleared
	}

	err := database.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failSubstr: "reconciliation_sessions"}
		return svc.InTx(fw).UndoFinish(session.ID, prev)
	})
	if err == nil {
		t.Fatal("UndoFinish() expected injected error, got nil")
	}

	txnRepo := transaction.NewRepository(database)
	for _, id := range ids {
		txn, err := txnRepo.GetByID(id)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", id, err)
		}
		if !txn.IsReconciled() {
			t.Errorf("transaction %s lost reconciled status after failed undo; want rolled back", id)
		}
	}

	completed, err := svc.GetLastCompletedSession(session.AccountID)
	if err != nil {
		t.Fatalf("GetLastCompletedSession() error = %v", err)
	}
	if completed == nil || completed.ID != session.ID {
		t.Fatal("session no longer completed after failed undo; want rolled back")
	}
}

// TestUndoFinish_HappyPath restores statuses and reopens the session in one call.
func TestUndoFinish_HappyPath(t *testing.T) {
	database, svc, session, ids := setupFinishFixture(t)

	if err := svc.FinishReconciliation(session.AccountID, ids, false); err != nil {
		t.Fatalf("FinishReconciliation() error = %v", err)
	}

	prev := map[types.ID]transaction.Status{}
	for _, id := range ids {
		prev[id] = transaction.StatusCleared
	}

	if err := svc.UndoFinish(session.ID, prev); err != nil {
		t.Fatalf("UndoFinish() error = %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	for _, id := range ids {
		txn, err := txnRepo.GetByID(id)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", id, err)
		}
		if txn.Status != transaction.StatusCleared {
			t.Errorf("transaction %s status = %s, want cleared", id, txn.Status)
		}
	}

	if active, err := svc.GetActiveSession(session.AccountID); err != nil {
		t.Fatalf("GetActiveSession() error = %v", err)
	} else if active == nil || active.ID != session.ID {
		t.Fatal("session not reopened after UndoFinish")
	}
}
