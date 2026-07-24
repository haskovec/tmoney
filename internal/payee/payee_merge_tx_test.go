package payee

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// Nth Exec (1-based), delegating everything else. It lets a test prove that a
// mid-merge write failure rolls the whole flow back rather than leaving partial
// state — the regression that would silently reappear if a write escaped the
// caller's transaction.
type failingQueryer struct {
	inner  db.Queryer
	failOn int // fail on the failOn-th Exec; 0 disables injection
	execN  int
}

func (f *failingQueryer) Exec(query string, args ...any) (sql.Result, error) {
	f.execN++
	if f.failOn != 0 && f.execN == f.failOn {
		return nil, fmt.Errorf("injected fault on exec #%d", f.execN)
	}
	return f.inner.Exec(query, args...)
}

func (f *failingQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	return f.inner.Query(query, args...)
}

func (f *failingQueryer) QueryRow(query string, args ...any) *sql.Row {
	return f.inner.QueryRow(query, args...)
}

// TestMergePayees_FaultRollsBack seeds source and target payees plus a
// transaction referencing the source, then forces the re-insert of the staged
// transactions to fail after the source rows have already been deleted inside
// the tx (temp-table create + delete having run). It asserts the whole merge
// rolls back: both payees still exist and the transaction still references the
// source payee.
func TestMergePayees_FaultRollsBack(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	source := NewPayee("Old Store")
	target := NewPayee("New Store")
	if err := svc.Create(source); err != nil {
		t.Fatalf("Create source error = %v", err)
	}
	if err := svc.Create(target); err != nil {
		t.Fatalf("Create target error = %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.NewDate(2020, time.January, 1))
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("Create account error = %v", err)
	}

	now := time.Now()
	txnID := types.NewID()
	if _, err := database.Conn().Exec(`
		INSERT INTO transactions (
			id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, txnID, acct.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("-10.00"),
		source.ID, nil, nil, nil, "uncleared", nil, nil, nil, now, now); err != nil {
		t.Fatalf("insert transaction error = %v", err)
	}

	// Exec order inside the merge: CREATE TEMPORARY TABLE _merge_txns (#1),
	// DELETE source transactions (#2), INSERT re-insert from temp table (#3).
	// Fail #3 so the delete of the source's transactions must roll back.
	err := database.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 3}
		return svc.InTx(fw).MergePayees(source.ID, target.ID)
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// Both payees must still exist.
	if _, err := svc.GetByID(source.ID); err != nil {
		t.Errorf("source payee should still exist after rollback: %v", err)
	}
	if _, err := svc.GetByID(target.ID); err != nil {
		t.Errorf("target payee should still exist after rollback: %v", err)
	}

	// Transaction reference unchanged: still points to source, and exactly one
	// such row survives (the delete was rolled back).
	var payeeID string
	if err := database.Conn().QueryRow(
		`SELECT CAST(payee_id AS VARCHAR) FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		txnID.String(),
	).Scan(&payeeID); err != nil {
		t.Fatalf("read back transaction payee error = %v", err)
	}
	if payeeID != source.ID.String() {
		t.Errorf("transaction payee = %s, want source %s (reference was not rolled back)", payeeID, source.ID.String())
	}

	var count int
	if err := database.Conn().QueryRow(
		`SELECT COUNT(*) FROM transactions WHERE CAST(payee_id AS VARCHAR) = ?`,
		source.ID.String(),
	).Scan(&count); err != nil {
		t.Fatalf("count source transactions error = %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 source-referencing transaction after rollback, got %d", count)
	}
}
