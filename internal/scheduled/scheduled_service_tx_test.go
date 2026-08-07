package scheduled

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// Nth Exec (1-based), delegating everything else. It lets a test prove that a
// mid-flow write failure rolls the whole flow back rather than leaving partial
// state — the regression that would silently reappear if a write escaped the
// caller's transaction (e.g. the old "partial success" double-post window).
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

// TestPost_DoublePostRegression is the flagship regression from the WithTx
// design (§8): posting a scheduled transaction must commit the posted row AND
// the schedule advance together. A fault injected on the LAST Exec — the
// schedule-advance UPDATE — must roll the posted transaction back too, so the
// old "transaction created but failed to update schedule" partial-success window
// (which double-posted on the next run because next_date never advanced) can
// never reappear.
func TestPost_DoublePostRegression(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	amount, _ := types.NewMoney("-50.00")
	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.NewDate(2020, time.January, 1), amount)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create() setup error = %v", err)
	}

	before, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID() before error = %v", err)
	}
	origNextDate := before.NextDate

	// Exec order inside postManually's tx: transaction INSERT (#1), then the
	// schedule-advance UPDATE (#2). Fail #2 so the posted row must roll back too.
	err = svc.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := svc.InTx(fw).Post(st.ID, nil)
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// The posted transaction must NOT exist.
	txnRepo := transaction.NewRepository(svc.db)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("expected no posted transaction after rollback, got %d", len(txns))
	}

	// next_date must be UNCHANGED — the schedule never advanced.
	after, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID() after error = %v", err)
	}
	if !after.NextDate.Equal(origNextDate) {
		t.Errorf("next_date advanced despite rollback: got %s, want %s", after.NextDate, origNextDate)
	}
}

// TestCreate_MultiLineFaultRollsBack forces the second split insert to fail
// during a multi-line Create and asserts the whole flow rolls back: neither the
// parent schedule row nor any split rows survive. Before phase 6 the parent and
// first split were left behind (best-effort compensation could itself fail).
func TestCreate_MultiLineFaultRollsBack(t *testing.T) {
	svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	amount, _ := types.NewMoney("-100.00")
	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.NewDate(2020, time.January, 1), amount)
	st.Splits = SplitCollection{
		NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("-60.00")),
		NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("-40.00")),
	}

	// Exec order inside Create's tx: parent INSERT (#1), split #1 (#2),
	// split #2 (#3). Fail #3 so the parent and first split must roll back too.
	err := svc.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 3}
		return svc.InTx(fw).Create(st)
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// No schedule row.
	if _, err := svc.GetByID(st.ID); err == nil {
		t.Error("expected no schedule row after rollback, got one")
	}
	// No split rows.
	splits, err := svc.repo.SplitRepo().ListByScheduledTransaction(st.ID)
	if err != nil {
		t.Fatalf("ListByScheduledTransaction() error = %v", err)
	}
	if len(splits) != 0 {
		t.Errorf("expected no split rows after rollback, got %d", len(splits))
	}
}

// TestPost_HappyPath exercises the single-line posting path with no
// injected fault: the unbound service opens and commits its own transaction, so
// the transaction is posted AND next_date advances by one cadence.
func TestPost_HappyPath(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	amount, _ := types.NewMoney("-50.00")
	start := types.NewDate(2020, time.January, 1)
	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, start, amount)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create() setup error = %v", err)
	}

	txn, err := svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if txn == nil {
		t.Fatal("expected a posted transaction, got nil")
	}

	// The transaction landed.
	txnRepo := transaction.NewRepository(svc.db)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 posted transaction, got %d", len(txns))
	}
	if !txns[0].Amount.Equal(amount) {
		t.Errorf("posted amount = %s, want %s", txns[0].Amount, amount)
	}

	// next_date advanced past the original occurrence.
	after, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !after.NextDate.After(start) {
		t.Errorf("next_date did not advance: got %s, want after %s", after.NextDate, start)
	}
}
