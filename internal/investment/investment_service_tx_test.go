package investment

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

// TestService_Buy_LotFaultRollsBack forces the lot insert (the second Exec of
// the trade body) to fail on a lot-tracking account and asserts the flow errors
// and leaves no investment transaction row behind — the review's headline orphan
// (a buy row with no lot) is now impossible.
//
// The failing wrapper binds the trade tx only: a bound service skips the
// heal-before-trade step, so the injected Execs are exactly the trade body's
// (repo.Create #1, lotRepo.Create #2).
func TestService_Buy_LotFaultRollsBack(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VOO")
	date := types.NewDate(2024, time.March, 15)

	shares := types.MustNewQuantity("10")
	price := types.MustNewMoney("100.00")

	err := env.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := env.svc.InTx(fw).Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	txns, err := env.invRepo.ListByAccount(acct.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(txns) != 0 {
		t.Fatalf("expected no investment transaction rows after rollback, got %d", len(txns))
	}
	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 0 {
		t.Fatalf("expected no lots after rollback, got %d", len(lots))
	}
}

// TestService_TransferCash_FaultRollsBack forces the regular-side (second) leg
// insert to fail and asserts neither the investment row nor the bank row
// survives — no orphaned investment counterpart, which the old compensation
// branch tried (and could fail) to clean up.
func TestService_TransferCash_FaultRollsBack(t *testing.T) {
	env := createFullTestService(t)
	invAcct := createInvAccount(t, env.accountRepo, "Brokerage")
	bankAcct := createCheckAccount(t, env.accountRepo, "Checking")
	date := types.NewDate(2024, time.March, 15)
	amount := types.MustNewMoney("500.00")

	err := env.db.WithTx(func(tx db.Queryer) error {
		// Fail the second Exec: the investment leg lands, the regular leg errors,
		// so the whole tx must roll back.
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := env.svc.InTx(fw).TransferCash(invAcct.ID, bankAcct.ID, date, amount, "", types.NullableID{})
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	invTxns, err := env.invRepo.ListByAccount(invAcct.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(inv) error = %v", err)
	}
	if len(invTxns) != 0 {
		t.Fatalf("expected no investment rows after rollback, got %d", len(invTxns))
	}
	bankTxns, err := env.svc.txnRepo.ListByAccount(bankAcct.ID)
	if err != nil {
		t.Fatalf("ListByAccount(bank) error = %v", err)
	}
	if len(bankTxns) != 0 {
		t.Fatalf("expected no bank rows after rollback, got %d", len(bankTxns))
	}
}

// TestService_Buy_HappyPathAtomic exercises the new runInTx path with no
// injected fault: the unbound service opens and commits its own transaction, and
// the buy fully lands — transaction row plus lot (lot-tracking) or position
// (non-lot) all present.
func TestService_Buy_HappyPathAtomic(t *testing.T) {
	date := types.NewDate(2024, time.March, 15)
	shares := types.MustNewQuantity("10")
	price := types.MustNewMoney("100.00")

	t.Run("lot-tracking: txn + lot present", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VOO")

		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		txns, err := env.invRepo.ListByAccount(acct.ID, TransactionFilter{})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(txns) != 1 || txns[0].ID != txn.ID {
			t.Fatalf("expected exactly the buy transaction, got %d rows", len(txns))
		}
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 || lots[0].Shares.Cmp(shares) != 0 {
			t.Fatalf("expected one lot of %s shares, got %d lots", shares, len(lots))
		}
	})

	t.Run("non-lot: txn + position present", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")

		txn, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		txns, err := env.invRepo.ListByAccount(acct.ID, TransactionFilter{})
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(txns) != 1 || txns[0].ID != txn.ID {
			t.Fatalf("expected exactly the buy transaction, got %d rows", len(txns))
		}
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.Cmp(shares) != 0 {
			t.Fatalf("expected position of %s shares, got %s", shares, pos.Shares)
		}
	})
}
