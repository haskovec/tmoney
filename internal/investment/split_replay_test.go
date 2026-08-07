package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// Now that splits are a dated ratio transform, a non-lot account's positions
// and realized gains can be reconstructed across a split by replaying the
// ledger split-aware — so the heal and realized-gain gates no longer apply to
// split-only securities.

func TestRebuildPositions_SplitAware_HealsNonLotPosition(t *testing.T) {
	env := createCATestEnv(t)
	acct := createInvAccount(t, env.accountRepo, "401k") // non-lot
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	dSplit := types.NewDate(2024, time.June, 1)

	if _, err := env.invSvc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"), nil, ptrMoney("100"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, dSplit, SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	// Corrupt the stored position to simulate desync, then heal.
	zero := NewPositionWithShares(acct.ID, sec.ID, types.ZeroQuantity, types.ZeroMoney)
	if err := env.positionRepo.CreateOrUpdate(&zero); err != nil {
		t.Fatalf("CreateOrUpdate(corrupt) error = %v", err)
	}

	if _, err := env.invSvc.RebuildPositions(acct.ID); err != nil {
		t.Fatalf("RebuildPositions() error = %v", err)
	}

	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	// 10 @ $100, 4:1 split → 40 @ $25 (total cost basis preserved).
	if pos.Shares.String() != "40" {
		t.Errorf("healed shares = %s, want 40 (split-aware replay)", pos.Shares.String())
	}
	if pos.AverageCostPerShare.String() != "25" {
		t.Errorf("healed avg cost = %s, want 25", pos.AverageCostPerShare.String())
	}
}

func TestRealizedGain_SplitAware_NonLotAvailableAcrossSplit(t *testing.T) {
	env := createCATestEnv(t)
	acct := createInvAccount(t, env.accountRepo, "401k") // non-lot
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	dSplit := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.July, 1)

	if _, err := env.invSvc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"), nil, ptrMoney("100"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, dSplit, SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	// Post-split: 40 sh @ $25. Sell 20 @ $30.
	if _, err := env.invSvc.Sell(acct.ID, sec.ID, d2, types.MustNewQuantity("20"), nil, ptrMoney("30"), types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	gain, unavailable, err := env.valSvc.realizedGain(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("realizedGain() error = %v", err)
	}
	if unavailable {
		t.Fatal("realized gain should be available across a split (not unavailable)")
	}
	// (30 − 25) × 20 = 100.
	if gain.String() != "100" {
		t.Errorf("realized gain = %s, want 100", gain.String())
	}
}

func TestRealizedGain_SpinOffKeepsNonLotUnavailable(t *testing.T) {
	env := createCATestEnv(t)
	acct := createInvAccount(t, env.accountRepo, "401k") // non-lot
	parent := createSec(t, env.secRepo, "AAPL")
	child := createSec(t, env.secRepo, "SPIN")
	d1 := types.NewDate(2024, time.March, 1)
	dSpin := types.NewDate(2024, time.June, 1)

	if _, err := env.invSvc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.invSvc.Buy(acct.ID, parent.ID, d1, types.MustNewQuantity("10"), nil, ptrMoney("100"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.caSvc.SpinOff(parent.ID, child.ID, dSpin, SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}, types.MustNewMoney("25")); err != nil {
		t.Fatalf("SpinOff() error = %v", err)
	}

	_, unavailable, err := env.valSvc.realizedGain(acct.ID, parent.ID, false)
	if err != nil {
		t.Fatalf("realizedGain() error = %v", err)
	}
	if !unavailable {
		t.Error("a spin-off (cross-security cost-basis reallocation) must keep realized gain unavailable")
	}
}

func ptrMoney(s string) *types.Money {
	m := types.MustNewMoney(s)
	return &m
}
