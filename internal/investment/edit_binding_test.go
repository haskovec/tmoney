package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// EditService is the only one of the three extracted types that must SHARE a
// transaction with the core rather than merely joining a caller's, so it is the
// only one where design section 2.2's silent failure is reachable. These two
// tests are the automated check that it did not happen.
//
// The failure is silent in both directions. If EditService.InTx forgot to rebind
// the core, the edit's writes would go to the pool: they would still succeed, and
// the only symptom would be that a caller's rollback failed to undo them. If
// instead an edit opened a SECOND transaction while one was already open, it
// would deadlock on db.WithTx's mutex — loud, but only if a test ever binds the
// service, which is exactly what these do.
//
// TestService_UpdateBuy_ReapplyFaultLeavesOriginalIntact covers the same ground
// with an injected fault; these two cover the clean commit and the clean
// rollback, which is where a pool write would hide.

func seedEditableBuy(t *testing.T) (*testServiceEnv, types.ID, types.ID, *Transaction, types.Date) {
	t.Helper()
	env := createFullTestService(t)
	// Lot-tracked on purpose: the reversal has to undo a LOT as well as the row,
	// which is the write most likely to escape the caller.s transaction.
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VOO")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit(): %v", err)
	}
	price := types.MustNewMoney("100.00")
	orig, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy(): %v", err)
	}
	return env, acct.ID, sec.ID, orig, date
}

// TestEdit_BoundEditJoinsAndCommitsOnce runs an edit on a BOUND EditService
// inside an open transaction and lets that transaction commit.
//
// Reaching the end at all is half the assertion: every Update* calls the core's
// runInTx, which JOINS when the core is bound. Had InTx failed to rebind the
// core, runInTx would have reached db.WithTx a second time and deadlocked on
// txMu — the test would hang rather than fail.
func TestEdit_BoundEditJoinsAndCommitsOnce(t *testing.T) {
	env, acctID, secID, orig, date := seedEditableBuy(t)
	buyType := TransactionTypeBuy

	newPrice := types.MustNewMoney("105.00")
	if err := env.db.WithTx(func(tx db.Queryer) error {
		_, uerr := env.editSvc.InTx(tx).UpdateBuy(orig.ID, acctID, secID, date,
			types.MustNewQuantity("10"), nil, &newPrice, types.ZeroMoney, "")
		return uerr
	}); err != nil {
		t.Fatalf("bound UpdateBuy(): %v", err)
	}

	txns, err := env.invRepo.ListByAccount(acctID, TransactionFilter{Type: &buyType})
	if err != nil {
		t.Fatalf("ListByAccount(): %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("%d buy rows after the edit, want 1 — the edit did not replace the original", len(txns))
	}
	if !txns[0].PricePerShare.Valid || txns[0].PricePerShare.Money.String() != "105" {
		t.Errorf("price per share = %v, want 105 — the edit did not commit with the caller's transaction",
			txns[0].PricePerShare)
	}
}

// TestEdit_BoundEditIsUndoneByTheCallersRollback is the direction that catches a
// missing rebind. The edit succeeds, then the caller rolls back. If every write
// really joined the caller's transaction, the original buy is restored exactly.
// If any of them reached the pool instead, that write survives — which is the
// silent non-atomicity design section 2.2 warns about, and nothing else in the
// suite would notice.
func TestEdit_BoundEditIsUndoneByTheCallersRollback(t *testing.T) {
	env, acctID, secID, orig, date := seedEditableBuy(t)
	buyType := TransactionTypeBuy

	rollback := errors.New("rollback on purpose")
	newPrice := types.MustNewMoney("105.00")
	err := env.db.WithTx(func(tx db.Queryer) error {
		if _, uerr := env.editSvc.InTx(tx).UpdateBuy(orig.ID, acctID, secID, date,
			types.MustNewQuantity("10"), nil, &newPrice, types.ZeroMoney, ""); uerr != nil {
			return uerr
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("WithTx() error = %v, want the deliberate rollback", err)
	}

	// The original row, which the edit deleted and re-created, must be back.
	txns, lerr := env.invRepo.ListByAccount(acctID, TransactionFilter{Type: &buyType})
	if lerr != nil {
		t.Fatalf("ListByAccount(): %v", lerr)
	}
	if len(txns) != 1 {
		t.Fatalf("%d buy rows after rollback, want 1", len(txns))
	}
	if txns[0].ID != orig.ID {
		t.Errorf("surviving row is %s, want the original %s — the delete escaped the caller's transaction",
			txns[0].ID, orig.ID)
	}
	if !txns[0].PricePerShare.Valid || txns[0].PricePerShare.Money.String() != "100" {
		t.Errorf("price per share = %v, want the original 100 — a write escaped the caller's transaction",
			txns[0].PricePerShare)
	}

	// And so must the lot the reversal removed.
	lots, lotErr := env.lotRepo.ListByAccountAndSecurity(acctID, secID, true)
	if lotErr != nil {
		t.Fatalf("ListByAccountAndSecurity(): %v", lotErr)
	}
	if len(lots) != 1 {
		t.Errorf("%d open lots after rollback, want 1 — the lot reversal escaped the caller's transaction", len(lots))
	}
}
