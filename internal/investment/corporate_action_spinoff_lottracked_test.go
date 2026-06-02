package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// Regression: a lot-tracked account that also carries an aggregate position row
// for the parent (as enable-lots or the position heal can leave) must not have
// the spin-off applied twice — once via the lots and once via the redundant
// position. The position path must skip lot-tracked accounts.
func TestSpinOff_LotTrackedAccount_DoesNotDoubleCountPosition(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Wealthfront IRA")
	parent := createSec(t, env.secRepo, "ETHE")
	child := createSec(t, env.secRepo, "ETH")
	spinDate := types.NewDate(2024, time.July, 23)

	// Buy 10 ETHE @ $100 → one parent lot.
	price100 := types.MustNewMoney("100.00")
	if _, err := env.invSvc.Buy(acct.ID, parent.ID, types.NewDate(2024, time.January, 2), types.MustNewQuantity("10"), nil, &price100, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy: %v", err)
	}
	// Simulate the leftover aggregate position on a lot-tracked account.
	stale := NewPositionWithShares(acct.ID, parent.ID, types.MustNewQuantity("10"), types.MustNewMoney("100"))
	if err := env.positionRepo.CreateOrUpdate(&stale); err != nil {
		t.Fatalf("seed stale position: %v", err)
	}

	params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
	if _, err := env.caSvc.SpinOff(parent.ID, child.ID, spinDate, params, types.MustNewMoney("5.00")); err != nil {
		t.Fatalf("SpinOff: %v", err)
	}

	// Exactly one ETH exchange transaction (from the single parent lot), not two.
	if childTxns, _ := env.invRepo.ListBySecurity(child.ID); len(childTxns) != 1 {
		t.Errorf("child exchange transactions = %d, want 1 (no position double-count)", len(childTxns))
	}
	// One child lot, totaling 10 shares.
	childLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, child.ID, true)
	total := types.ZeroQuantity
	for _, l := range childLots {
		total = total.Add(l.Shares)
	}
	if total.String() != "10" {
		t.Errorf("child lot shares total = %s, want 10", total.String())
	}
	// No child position row (the position path was skipped for the lot-tracked account).
	if childPositions, _ := env.positionRepo.GetPositionsBySecurity(child.ID); len(childPositions) != 0 {
		t.Errorf("child positions = %d, want 0 (position path skipped for lot-tracked account)", len(childPositions))
	}
}
