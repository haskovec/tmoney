package transaction

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// Nth Exec (1-based), delegating everything else. It lets a test prove that a
// mid-flow write failure rolls the whole flow back rather than leaving partial
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

// TestTransactionService_CreateTransfer_FaultRollsBack forces the second Exec
// (the to-leg insert) to fail and asserts the flow errors and leaves no rows
// for either leg — no orphaned from-leg.
func TestTransactionService_CreateTransfer_FaultRollsBack(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	from := createTestAccount(t, accountRepo, "Checking")
	to := createTestAccount(t, accountRepo, "Savings")

	amount, _ := types.NewMoney("100.00")
	date := types.NewDate(2020, time.January, 1)

	err := svc.db.WithTx(func(tx db.Queryer) error {
		// Fail the second Exec: the from-leg insert lands, the to-leg insert
		// errors, so the whole tx must roll back.
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := svc.InTx(fw).CreateTransfer(from.ID, to.ID, date, amount, "", types.NullableID{})
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	fromTxns, err := svc.ListByAccount(from.ID)
	if err != nil {
		t.Fatalf("ListByAccount(from) error = %v", err)
	}
	toTxns, err := svc.ListByAccount(to.ID)
	if err != nil {
		t.Fatalf("ListByAccount(to) error = %v", err)
	}
	if len(fromTxns) != 0 || len(toTxns) != 0 {
		t.Fatalf("expected no rows after rollback, got from=%d to=%d", len(fromTxns), len(toTxns))
	}
}

// TestTransactionService_CreateTransfer_HappyPath exercises the new runInTx
// path with no injected fault: the unbound service opens and commits its own
// transaction, and both legs land linked by a shared transfer_id.
func TestTransactionService_CreateTransfer_HappyPath(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	from := createTestAccount(t, accountRepo, "Checking")
	to := createTestAccount(t, accountRepo, "Savings")

	amount, _ := types.NewMoney("100.00")
	date := types.NewDate(2020, time.January, 1)

	pair, err := svc.CreateTransfer(from.ID, to.ID, date, amount, "", types.NullableID{})
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}

	if !pair.FromTransaction.TransferID.Valid {
		t.Fatal("expected from leg to carry a transfer_id")
	}

	got, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
	if err != nil {
		t.Fatalf("GetTransferPair() error = %v", err)
	}
	if got.FromTransaction == nil || got.ToTransaction == nil {
		t.Fatal("expected both legs present after commit")
	}
	if got.FromTransaction.TransferID.ID != got.ToTransaction.TransferID.ID {
		t.Fatalf("legs not linked: from transfer_id=%s to transfer_id=%s",
			got.FromTransaction.TransferID.ID, got.ToTransaction.TransferID.ID)
	}
	if !got.FromTransaction.Amount.Neg().Equal(got.ToTransaction.Amount) {
		t.Fatalf("legs not equal-and-opposite: from=%s to=%s",
			got.FromTransaction.Amount, got.ToTransaction.Amount)
	}
}
