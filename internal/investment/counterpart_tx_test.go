package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// CounterpartService is the first type extracted out of investment.Service, and
// the two tests here are what the extraction has to earn: that the new type
// really joins the caller's transaction instead of quietly opening a second one,
// and that a mid-write failure leaves no partial state behind.
//
// The first is the check the suite did not have before. Section 2.2's failure
// mode is silent — since SetMaxOpenConns(1) was removed there is no deadlock to
// trip over, so a pool access inside an open transaction just misses the
// caller's uncommitted writes. Nothing about that shows up as a test failure
// unless a test goes looking for it.

func newCounterpartFixture(t *testing.T) (*CounterpartService, *account.Repository, *Repository, *db.DB, *account.Account, *account.Account) {
	t.Helper()
	database := createTestDB(t)
	invRepo := NewRepository(database)
	accountRepo := account.NewRepository(database)

	open := types.NewDate(2000, time.January, 1)
	brokerage := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, open)
	if err := accountRepo.Create(brokerage); err != nil {
		t.Fatalf("create brokerage: %v", err)
	}
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, open)
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}

	return NewCounterpartService(invRepo, accountRepo), accountRepo, invRepo, database, brokerage, checking
}

// TestCounterpart_JoinsTheCallersTransaction is the binding test. It calls the
// extracted type from inside an open transaction and then rolls that transaction
// back. If CreateCounterpart joined it, the row disappears; if it had opened a
// second transaction of its own, or written through the pool, the row would
// survive the rollback.
//
// This is the only automated check that section 2.2's silent failure did not
// happen. It is worth having per extracted type, because the mistake is a
// one-character one — calling a method on the captured outer receiver instead of
// the bound copy — and the compiler cannot see it.
func TestCounterpart_JoinsTheCallersTransaction(t *testing.T) {
	cp, _, invRepo, database, brokerage, checking := newCounterpartFixture(t)

	transferID := types.NewID()
	var rowID types.ID

	// Roll the outer transaction back deliberately.
	err := database.WithTx(func(tx db.Queryer) error {
		id, cerr := cp.CreateCounterpart(tx, brokerage.ID, checking.ID,
			types.NewDate(2024, time.March, 1), types.MustNewMoney("250.00"), "401k line", transferID)
		if cerr != nil {
			return cerr
		}
		rowID = id

		// Inside the same transaction the row must already be visible, which is
		// only true if the read is also bound.
		_, _, found, ferr := cp.FindCounterpart(tx, transferID)
		if ferr != nil {
			return ferr
		}
		if !found {
			t.Error("FindCounterpart could not see the row written moments earlier in the same transaction")
		}
		return errRollbackOnPurpose
	})
	if err != errRollbackOnPurpose {
		t.Fatalf("WithTx() error = %v, want the deliberate rollback", err)
	}

	if _, gerr := invRepo.GetByID(rowID); gerr == nil {
		t.Error("the counterpart row survived the caller's rollback — CreateCounterpart did not join the transaction")
	}

	rows, lerr := invRepo.ListByTransferID(transferID)
	if lerr != nil {
		t.Fatalf("ListByTransferID(): %v", lerr)
	}
	if len(rows) != 0 {
		t.Errorf("%d counterpart rows survived the rollback, want 0", len(rows))
	}
}

// TestCounterpart_FaultLeavesNoPartialState injects a write failure and asserts
// nothing lands. CreateCounterpart writes one row, so this also pins that it
// does not write anything BEFORE that row — a guard that wrote first and
// validated second would show up here.
func TestCounterpart_FaultLeavesNoPartialState(t *testing.T) {
	cp, _, invRepo, database, brokerage, checking := newCounterpartFixture(t)

	transferID := types.NewID()
	err := database.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 1}
		_, cerr := cp.CreateCounterpart(fw, brokerage.ID, checking.ID,
			types.NewDate(2024, time.March, 1), types.MustNewMoney("250.00"), "", transferID)
		return cerr
	})
	if err == nil {
		t.Fatal("expected the injected fault to surface, got nil")
	}

	rows, lerr := invRepo.ListByTransferID(transferID)
	if lerr != nil {
		t.Fatalf("ListByTransferID(): %v", lerr)
	}
	if len(rows) != 0 {
		t.Errorf("%d rows survived the injected fault, want 0", len(rows))
	}
}

// TestCounterpart_UpdateAndDeleteJoinTheCallersTransaction covers the other two
// write methods. Both are single writes on the caller's queryer, so the property
// to prove is the same one: an outer rollback must undo them.
func TestCounterpart_UpdateAndDeleteJoinTheCallersTransaction(t *testing.T) {
	cp, _, invRepo, database, brokerage, checking := newCounterpartFixture(t)

	// Commit one counterpart to work against.
	transferID := types.NewID()
	var rowID types.ID
	if err := database.WithTx(func(tx db.Queryer) error {
		id, cerr := cp.CreateCounterpart(tx, brokerage.ID, checking.ID,
			types.NewDate(2024, time.March, 1), types.MustNewMoney("250.00"), "", transferID)
		rowID = id
		return cerr
	}); err != nil {
		t.Fatalf("seed CreateCounterpart: %v", err)
	}

	// An amount edit inside a rolled-back transaction must not stick.
	if err := database.WithTx(func(tx db.Queryer) error {
		if uerr := cp.UpdateCounterpartAmount(tx, rowID, types.MustNewMoney("999.00")); uerr != nil {
			return uerr
		}
		return errRollbackOnPurpose
	}); err != errRollbackOnPurpose {
		t.Fatalf("WithTx() error = %v, want the deliberate rollback", err)
	}
	row, gerr := invRepo.GetByID(rowID)
	if gerr != nil {
		t.Fatalf("GetByID(): %v", gerr)
	}
	if !row.TotalAmount.Equal(types.MustNewMoney("250.00")) {
		t.Errorf("amount = %s after rollback, want 250.00 — the update did not join the transaction", row.TotalAmount)
	}

	// Nor must a delete.
	if err := database.WithTx(func(tx db.Queryer) error {
		if derr := cp.DeleteCounterpart(tx, rowID); derr != nil {
			return derr
		}
		return errRollbackOnPurpose
	}); err != errRollbackOnPurpose {
		t.Fatalf("WithTx() error = %v, want the deliberate rollback", err)
	}
	if _, gerr := invRepo.GetByID(rowID); gerr != nil {
		t.Error("the row stayed deleted after rollback — DeleteCounterpart did not join the transaction")
	}
}

// errRollbackOnPurpose is returned from a WithTx closure whose writes succeeded,
// to force a rollback the test can then observe.
var errRollbackOnPurpose = errors.New("rollback on purpose")
