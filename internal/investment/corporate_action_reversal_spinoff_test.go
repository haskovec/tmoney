package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestReverseSpinOff_RestoresParentAndRemovesChild(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	parent := createSec(t, env.secRepo, "GBTC")
	child := createSec(t, env.secRepo, "BTC")
	buyDate := types.NewDate(2024, time.January, 2)
	spinDate := types.NewDate(2024, time.July, 31)

	// Buy 10 GBTC @ $100 → one parent lot, cost 100.
	price100 := types.MustNewMoney("100.00")
	if _, err := env.invSvc.Buy(acct.ID, parent.ID, buyDate, types.MustNewQuantity("10"), nil, &price100, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy: %v", err)
	}

	// Spin off 1:1, parent keeps 80% (1/0.8 = 1.25 restores exactly).
	params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
	ca, err := env.caSvc.SpinOff(parent.ID, child.ID, spinDate, params, types.MustNewMoney("5.00"))
	if err != nil {
		t.Fatalf("SpinOff: %v", err)
	}

	// Sanity: parent lot scaled to 80; one child lot created.
	parentLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, parent.ID, false)
	if len(parentLots) != 1 || parentLots[0].CostPerShare.String() != "80" {
		t.Fatalf("after spin-off, parent lot cost = %v, want 80", parentLots)
	}
	if childLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, child.ID, false); len(childLots) != 1 {
		t.Fatalf("after spin-off, want 1 child lot, got %d", len(childLots))
	}

	// Reverse.
	if err := env.caSvc.DeleteAction(ca.ID); err != nil {
		t.Fatalf("DeleteAction (reverse): %v", err)
	}

	// Parent cost restored to 100.
	parentLots, _ = env.lotRepo.ListByAccountAndSecurity(acct.ID, parent.ID, false)
	if len(parentLots) != 1 || parentLots[0].CostPerShare.String() != "100" {
		t.Errorf("after reversal, parent lot cost = %v, want 100", parentLots)
	}
	// Child lots gone.
	if childLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, child.ID, true); len(childLots) != 0 {
		t.Errorf("after reversal, want 0 child lots, got %d", len(childLots))
	}
	// Child exchange transactions gone.
	if childTxns, _ := env.invRepo.ListBySecurity(child.ID); len(childTxns) != 0 {
		t.Errorf("after reversal, want 0 child transactions, got %d", len(childTxns))
	}
	// Seeded child price gone.
	if _, perr := env.priceRepo.GetBySecurityAndDate(child.ID, spinDate); perr == nil {
		t.Error("after reversal, seeded child price should be gone")
	}
	// Audit row gone.
	if _, gerr := env.caRepo.GetByID(ca.ID); gerr == nil {
		t.Error("after reversal, corporate-action audit row should be gone")
	}
}

func TestReverseSpinOff_RefusedWhenChildSold(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	parent := createSec(t, env.secRepo, "GBTC")
	child := createSec(t, env.secRepo, "BTC")
	spinDate := types.NewDate(2024, time.July, 31)

	price100 := types.MustNewMoney("100.00")
	if _, err := env.invSvc.Buy(acct.ID, parent.ID, types.NewDate(2024, time.January, 2), types.MustNewQuantity("10"), nil, &price100, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy: %v", err)
	}
	params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
	ca, err := env.caSvc.SpinOff(parent.ID, child.ID, spinDate, params, types.MustNewMoney("5.00"))
	if err != nil {
		t.Fatalf("SpinOff: %v", err)
	}

	// Sell the spun-off child shares after the spin date.
	childLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, child.ID, false)
	if len(childLots) != 1 {
		t.Fatalf("want 1 child lot, got %d", len(childLots))
	}
	sellPrice := types.MustNewMoney("6.00")
	if _, err := env.invSvc.Sell(acct.ID, child.ID, types.NewDate(2024, time.August, 15), types.MustNewQuantity("10"), nil, &sellPrice, types.ZeroMoney, "",
		[]SellLotAllocation{{LotID: childLots[0].ID, Shares: types.MustNewQuantity("10")}}); err != nil {
		t.Fatalf("Sell child: %v", err)
	}

	// Reversal must refuse, naming the blocking sale.
	err = env.caSvc.DeleteAction(ca.ID)
	var dse *DownstreamEventsError
	if !errors.As(err, &dse) {
		t.Fatalf("expected *DownstreamEventsError, got %v", err)
	}
}
