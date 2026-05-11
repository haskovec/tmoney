package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestRebuildPositions_NonLotTracking(t *testing.T) {
	t.Run("rebuilds desynced position from transaction history", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "IEMG")
		date := types.NewDate(2018, time.June, 15)

		// Buy 3 shares total via two buys (mimics the user's IEMG scenario).
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		shares2 := types.MustNewQuantity("2")
		buyPrice1 := types.MustNewMoney("55.81")
		_, _ = env.svc.Buy(acct.ID, sec.ID, types.NewDate(2018, time.May, 3), shares2, nil, &buyPrice1, types.ZeroMoney, "")
		shares1 := types.MustNewQuantity("1")
		buyPrice2 := types.MustNewMoney("56.07")
		_, _ = env.svc.Buy(acct.ID, sec.ID, types.NewDate(2018, time.May, 7), shares1, nil, &buyPrice2, types.ZeroMoney, "")

		// One sell of 1 share
		sellPrice := types.MustNewMoney("54.61")
		_, _ = env.svc.Sell(acct.ID, sec.ID, date, shares1, nil, &sellPrice, types.ZeroMoney, "", nil)

		// Simulate the corruption: blow away the stored position so it
		// disagrees with the txn ledger.
		zeroPos := NewPositionWithShares(acct.ID, sec.ID, types.ZeroQuantity, types.ZeroMoney)
		if err := env.positionRepo.CreateOrUpdate(&zeroPos); err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		result, err := env.svc.RebuildPositions(acct.ID)
		if err != nil {
			t.Fatalf("RebuildPositions() error = %v", err)
		}
		if result.HasCorporateActions {
			t.Fatal("Unexpected HasCorporateActions=true")
		}

		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "2" {
			t.Errorf("Expected rebuilt position shares 2, got %s", pos.Shares.String())
		}
	})

	t.Run("removes orphan position with no transactions", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "ORPHAN")

		// Manually create a position that has no backing transactions.
		stalePos := NewPositionWithShares(acct.ID, sec.ID, types.MustNewQuantity("99"), types.MustNewMoney("99"))
		if err := env.positionRepo.CreateOrUpdate(&stalePos); err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		_, err := env.svc.RebuildPositions(acct.ID)
		if err != nil {
			t.Fatalf("RebuildPositions() error = %v", err)
		}

		// After rebuild, the orphan must be gone (Get returns a zero-shares stub for missing rows).
		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if !pos.Shares.IsZero() {
			t.Errorf("Expected orphan position to be removed (zero shares), got %s", pos.Shares.String())
		}
	})
}

func TestRebuildPositions_LotTracking(t *testing.T) {
	t.Run("rebuilds lot shares/closed from junctions", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
		sec := createSec(t, env.secRepo, "QQQ")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		shares := types.MustNewQuantity("10")
		price := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(lots))
		}
		lotID := lots[0].ID

		// Sell 4 shares against the lot
		sellShares := types.MustNewQuantity("4")
		sellPrice := types.MustNewMoney("110.00")
		_, _ = env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &sellPrice, types.ZeroMoney, "",
			[]SellLotAllocation{{LotID: lotID, Shares: sellShares}})

		// Corrupt the lot's stored shares to mimic a desynced state
		if err := env.lotRepo.UpdateSharesAndClosed(lotID, types.MustNewQuantity("99"), false); err != nil {
			t.Fatalf("UpdateSharesAndClosed() error = %v", err)
		}

		result, err := env.svc.RebuildPositions(acct.ID)
		if err != nil {
			t.Fatalf("RebuildPositions() error = %v", err)
		}
		if result.LotsRecomputed < 1 {
			t.Errorf("Expected at least 1 lot recomputed, got %d", result.LotsRecomputed)
		}

		lot, _ := env.lotRepo.GetByID(lotID)
		if lot.Shares.String() != "6" {
			t.Errorf("Expected lot shares 6 (10 - 4), got %s", lot.Shares.String())
		}
		if lot.Closed {
			t.Error("Expected lot to remain open")
		}
	})
}
