package category

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

// TestMergeCategories_FaultRollsBack seeds source and target categories, a
// transaction and a split that both reference the source, then forces the third
// merge UPDATE (payee defaults) to fail after the transaction and split updates
// have already run inside the tx. It asserts the whole merge rolls back: the
// source category still exists and both references still point to source.
func TestMergeCategories_FaultRollsBack(t *testing.T) {
	database := createTestDB(t)
	repo := NewRepository(database)
	svc := NewService(repo, database)

	source := NewCategory("Source", TypeExpense)
	target := NewCategory("Target", TypeExpense)
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
		nil, source.ID, nil, nil, "uncleared", nil, nil, nil, now, now); err != nil {
		t.Fatalf("insert transaction error = %v", err)
	}

	splitID := types.NewID()
	if _, err := database.Conn().Exec(`
		INSERT INTO transaction_splits (
			id, transaction_id, category_id, transfer_account_id, transfer_id,
			amount, memo, paycheck_section, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, splitID, txnID, source.ID, nil, nil, types.MustNewMoney("-10.00"), nil, nil, now); err != nil {
		t.Fatalf("insert split error = %v", err)
	}

	// Exec order inside the merge: UPDATE transactions (#1), UPDATE
	// transaction_splits (#2), UPDATE payees (#3). Fail #3 so the first two
	// updates must roll back with it.
	err := database.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 3}
		return svc.InTx(fw).MergeCategories(source.ID, target.ID)
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// Source category must still exist.
	if _, err := svc.GetByID(source.ID); err != nil {
		t.Errorf("source category should still exist after rollback: %v", err)
	}

	// Transaction reference unchanged.
	var txnCat string
	if err := database.Conn().QueryRow(
		`SELECT CAST(category_id AS VARCHAR) FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		txnID.String(),
	).Scan(&txnCat); err != nil {
		t.Fatalf("read back transaction category error = %v", err)
	}
	if txnCat != source.ID.String() {
		t.Errorf("transaction category = %s, want source %s (reference was not rolled back)", txnCat, source.ID.String())
	}

	// Split reference unchanged.
	var splitCat string
	if err := database.Conn().QueryRow(
		`SELECT CAST(category_id AS VARCHAR) FROM transaction_splits WHERE CAST(id AS VARCHAR) = ?`,
		splitID.String(),
	).Scan(&splitCat); err != nil {
		t.Fatalf("read back split category error = %v", err)
	}
	if splitCat != source.ID.String() {
		t.Errorf("split category = %s, want source %s (reference was not rolled back)", splitCat, source.ID.String())
	}
}
