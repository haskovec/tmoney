package investment

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// Nth Exec (1-based) or on the first Exec whose query contains failSubstr,
// delegating everything else. It lets a test prove that a mid-flow write
// failure rolls the whole flow back rather than leaving partial state — the
// regression that would silently reappear if a write escaped the caller's
// transaction.
type failingQueryer struct {
	inner      db.Queryer
	failOn     int    // fail on the failOn-th Exec; 0 disables count injection
	failSubstr string // fail on the first Exec whose query contains this; "" disables
	execN      int
}

func (f *failingQueryer) Exec(query string, args ...any) (sql.Result, error) {
	f.execN++
	if f.failSubstr != "" && strings.Contains(query, f.failSubstr) {
		return nil, fmt.Errorf("injected fault on query containing %q", f.failSubstr)
	}
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

// TestService_UpdateBuy_ReapplyFaultLeavesOriginalIntact is THE edit test: it
// creates a lot-tracked buy, then edits it with a fault injected during the
// re-create (the new Buy's INSERT), after the reverse+delete of the original
// have already executed inside the edit tx. Because reverse → delete → re-create
// now run in ONE transaction, the fault rolls all three back and the ORIGINAL
// buy (row + lot) is fully intact. This is exactly the state the old
// reverse-then-reapply code could corrupt with a "rollback failed" error.
func TestService_UpdateBuy_ReapplyFaultLeavesOriginalIntact(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VOO")
	date := types.NewDate(2024, time.March, 15)

	shares := types.MustNewQuantity("10")
	price := types.MustNewMoney("100.00")

	orig, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	// Edit on a bound service so the whole reverse/delete/re-create runs inside
	// one failing tx. Fault the recreate: the INSERT into investment_transactions
	// is the first (and only) such insert in the edit tx — the reverse/delete
	// phase issues DELETEs only.
	newPrice := types.MustNewMoney("105.00")
	err = env.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failSubstr: "INSERT INTO investment_transactions"}
		_, e := env.svc.InTx(fw).UpdateBuy(orig.ID, acct.ID, sec.ID, date, shares, nil, &newPrice, types.ZeroMoney, "")
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// The original buy row must still exist, unchanged.
	txns, err := env.invRepo.ListByAccount(acct.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(txns) != 1 || txns[0].ID != orig.ID {
		t.Fatalf("expected the original buy row to survive, got %d rows", len(txns))
	}
	if !txns[0].PricePerShare.Valid || txns[0].PricePerShare.Money.String() != "100" {
		t.Errorf("original buy price = %v, want 100", txns[0].PricePerShare)
	}

	// The original lot must still exist, unchanged.
	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected the original lot to survive, got %d lots", len(lots))
	}
	if lots[0].Shares.Cmp(shares) != 0 || lots[0].CostPerShare.String() != "100" {
		t.Errorf("original lot = %s shares @ %s, want 10 @ 100", lots[0].Shares, lots[0].CostPerShare)
	}
}

// TestCorporateActionService_Split_FaultRollsBack forces a fault mid lot-loop of
// a stock split (the second lot's UPDATE) and asserts the whole action rolled
// back: NO lot changed (the first lot's already-executed update is undone), no
// audit row was created, and the price row was not adjusted.
func TestCorporateActionService_Split_FaultRollsBack(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.January, 15)
	splitDate := types.NewDate(2024, time.June, 1)

	// Two lots so the fault can land mid-loop after the first was updated.
	p := types.MustNewMoney("100.00")
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &p, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() 1 error = %v", err)
	}
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), nil, &p, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() 2 error = %v", err)
	}

	// adjustLots is the first write phase; with two eligible lots the two lot
	// UPDATEs are Exec #1 and #2. Fault the second.
	params := SplitParams{Numerator: 2, Denominator: 1}
	err := env.invRepo.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := env.caSvc.InTx(fw).Split(sec.ID, splitDate, params)
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// No lot changed: both still 10/5 shares at cost 100 (a 2:1 split would have
	// doubled shares and halved cost).
	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 2 {
		t.Fatalf("expected 2 lots, got %d", len(lots))
	}
	for _, l := range lots {
		if l.CostPerShare.String() != "100" {
			t.Errorf("lot cost = %s, want 100 (unchanged)", l.CostPerShare)
		}
		if s := l.Shares.String(); s != "10" && s != "5" {
			t.Errorf("lot shares = %s, want 10 or 5 (unchanged)", s)
		}
	}

	// No audit row was created.
	actions, err := env.caRepo.ListBySecurity(sec.ID)
	if err != nil {
		t.Fatalf("ListBySecurity() error = %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected no corporate-action row after rollback, got %d", len(actions))
	}

	// The auto price from the buys was not adjusted (2:1 would have halved it).
	pr, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if err != nil {
		t.Fatalf("GetBySecurityAndDate() error = %v", err)
	}
	if pr == nil || pr.Price.String() != "100" {
		t.Errorf("price = %v, want 100 (unadjusted)", pr)
	}
}

// TestService_RebuildPositions_FaultLeavesPositionsUnchanged forces a fault mid
// rebuild — during persistRebuiltPosition, which upserts as DELETE-then-INSERT.
// The fault hits the INSERT after the DELETE already ran, so without the
// per-account tx the position would be lost. With the wrap it rolls back and the
// account's position is unchanged.
func TestService_RebuildPositions_FaultLeavesPositionsUnchanged(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2024, time.March, 15)

	shares := types.MustNewQuantity("10")
	price := types.MustNewMoney("100.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	err := env.db.WithTx(func(tx db.Queryer) error {
		fw := &failingQueryer{inner: tx, failSubstr: "INSERT INTO investment_positions"}
		_, e := env.svc.InTx(fw).RebuildPositions(acct.ID)
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// The position survived the faulted rebuild intact.
	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if pos.Shares.Cmp(shares) != 0 {
		t.Errorf("position shares = %s, want %s (unchanged)", pos.Shares, shares)
	}
}
