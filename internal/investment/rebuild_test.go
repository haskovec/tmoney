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

func TestSell_AutoHealsDesyncedPosition(t *testing.T) {
	t.Run("Sell succeeds against stale zero-share position when ledger has shares", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "IEMG")

		// Build the user's actual ledger shape: 3 buys, 1 sell.
		_, _ = env.svc.Deposit(acct.ID, types.NewDate(2018, time.May, 1), types.MustNewMoney("1000.00"), "")
		buy1Price := types.MustNewMoney("55.81")
		_, _ = env.svc.Buy(acct.ID, sec.ID, types.NewDate(2018, time.May, 3), types.MustNewQuantity("2"), nil, &buy1Price, types.ZeroMoney, "")
		buy2Price := types.MustNewMoney("56.07")
		_, _ = env.svc.Buy(acct.ID, sec.ID, types.NewDate(2018, time.May, 7), types.MustNewQuantity("1"), nil, &buy2Price, types.ZeroMoney, "")
		sellPrice := types.MustNewMoney("54.61")
		_, _ = env.svc.Sell(acct.ID, sec.ID, types.NewDate(2018, time.June, 15), types.MustNewQuantity("1"), nil, &sellPrice, types.ZeroMoney, "", nil)

		// Simulate the user's corrupted state: stored position says 0 shares,
		// but the ledger still shows a net 2 shares.
		corrupt := NewPositionWithShares(acct.ID, sec.ID, types.ZeroQuantity, types.ZeroMoney)
		if err := env.positionRepo.CreateOrUpdate(&corrupt); err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		// The next Sell should auto-heal and succeed.
		_, err := env.svc.Sell(acct.ID, sec.ID, types.NewDate(2018, time.June, 15), types.MustNewQuantity("2"), nil, &sellPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() unexpected error after auto-heal: %v", err)
		}

		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if !pos.Shares.IsZero() {
			t.Errorf("Expected position 0 after final sell, got %s", pos.Shares.String())
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

// =============================================================================
// Per-security corporate-action gate: healing skips only the securities that
// actually participate in a corporate action, not the whole database. This is
// what lets a clean security (e.g. VNQ) auto-heal on a database that also holds
// securities with splits/mergers/spin-offs.
// =============================================================================

func TestSyncPositionAndLots_PerSecurityCorporateActionGate(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage") // non-lot
	withCA := createSec(t, env.secRepo, "SCHB")
	clean := createSec(t, env.secRepo, "VNQ")
	date := types.NewDate(2024, time.March, 15)

	_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
	price := types.MustNewMoney("50.00")
	_, _ = env.svc.Buy(acct.ID, withCA.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, "")
	_, _ = env.svc.Buy(acct.ID, clean.ID, date, types.MustNewQuantity("20"), nil, &price, types.ZeroMoney, "")

	// Record a spin-off on SCHB only. A spin-off (cross-security cost-basis
	// reallocation) can't be reconstructed by a per-security replay, so it gates
	// the heal — unlike a split, which is now replayed.
	if err := env.caRepo.Create(NewCorporateAction(ActionTypeSpinOff, withCA.ID, date, `{"share_ratio":1,"parent_allocation_pct":90}`)); err != nil {
		t.Fatalf("caRepo.Create() error = %v", err)
	}

	// Desync both stored positions to zero.
	zClean := NewPositionWithShares(acct.ID, clean.ID, types.ZeroQuantity, types.ZeroMoney)
	if err := env.positionRepo.CreateOrUpdate(&zClean); err != nil {
		t.Fatalf("CreateOrUpdate(clean) error = %v", err)
	}
	zCA := NewPositionWithShares(acct.ID, withCA.ID, types.ZeroQuantity, types.ZeroMoney)
	if err := env.positionRepo.CreateOrUpdate(&zCA); err != nil {
		t.Fatalf("CreateOrUpdate(withCA) error = %v", err)
	}

	// Syncing the clean security heals it even though a corporate action exists.
	if err := env.svc.syncPositionAndLots(acct.ID, clean.ID); err != nil {
		t.Fatalf("syncPositionAndLots(clean) error = %v", err)
	}
	posClean, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, clean.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity(clean) error = %v", err)
	}
	if posClean.Shares.String() != "20" {
		t.Errorf("clean security should heal to 20, got %s", posClean.Shares.String())
	}

	// Syncing the CA-involved security is still a no-op (left desynced).
	if err := env.svc.syncPositionAndLots(acct.ID, withCA.ID); err != nil {
		t.Fatalf("syncPositionAndLots(withCA) error = %v", err)
	}
	posCA, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, withCA.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity(withCA) error = %v", err)
	}
	if !posCA.Shares.IsZero() {
		t.Errorf("CA-involved security must be skipped, got shares %s", posCA.Shares.String())
	}
}

func TestRebuildPositions_HealsNonCASecuritiesPerSecurity(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage") // non-lot
	withCA := createSec(t, env.secRepo, "ETHE")
	clean := createSec(t, env.secRepo, "VNQ")
	date := types.NewDate(2024, time.March, 15)

	_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
	price := types.MustNewMoney("50.00")
	_, _ = env.svc.Buy(acct.ID, withCA.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, "")
	_, _ = env.svc.Buy(acct.ID, clean.ID, date, types.MustNewQuantity("20"), nil, &price, types.ZeroMoney, "")

	// A spin-off gates the heal (cross-security cost-basis reallocation can't be
	// replayed per-security); a split would now be replayed instead.
	if err := env.caRepo.Create(NewCorporateAction(ActionTypeSpinOff, withCA.ID, date, `{"share_ratio":1,"parent_allocation_pct":90}`)); err != nil {
		t.Fatalf("caRepo.Create() error = %v", err)
	}

	// Desync both.
	zClean := NewPositionWithShares(acct.ID, clean.ID, types.ZeroQuantity, types.ZeroMoney)
	_ = env.positionRepo.CreateOrUpdate(&zClean)
	zCA := NewPositionWithShares(acct.ID, withCA.ID, types.ZeroQuantity, types.ZeroMoney)
	_ = env.positionRepo.CreateOrUpdate(&zCA)

	res, err := env.svc.RebuildPositions(acct.ID)
	if err != nil {
		t.Fatalf("RebuildPositions() error = %v", err)
	}
	if res.SkippedSecurities != 1 {
		t.Errorf("expected 1 skipped corporate-action security, got %d", res.SkippedSecurities)
	}
	if !res.HasCorporateActions {
		t.Error("HasCorporateActions should be true when a security was skipped")
	}

	posClean, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, clean.ID)
	if posClean.Shares.String() != "20" {
		t.Errorf("clean security should be healed to 20, got %s", posClean.Shares.String())
	}
	posCA, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, withCA.ID)
	if !posCA.Shares.IsZero() {
		t.Errorf("CA-involved security must be left untouched, got shares %s", posCA.Shares.String())
	}
}

func TestRebuildPositions_LotTracked_HealsCleanSecurityWithCAPresent(t *testing.T) {
	// Mirrors the reported bug: a lot-tracked account where a clean security's
	// lot was drained by a now-deleted sell (junction gone, lot left short),
	// while another security carries a corporate action. The clean security's
	// lot must be restored even though the database has corporate-action history.
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Wealthfront IRA")
	withCA := createSec(t, env.secRepo, "GBTC")
	vnq := createSec(t, env.secRepo, "VNQ")
	date := types.NewDate(2024, time.March, 15)

	_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("100000.00"), "")
	caPrice := types.MustNewMoney("50.00")
	_, _ = env.svc.Buy(acct.ID, withCA.ID, date, types.MustNewQuantity("10"), nil, &caPrice, types.ZeroMoney, "")
	vnqPrice := types.MustNewMoney("90.00")
	_, _ = env.svc.Buy(acct.ID, vnq.ID, date, types.MustNewQuantity("20"), nil, &vnqPrice, types.ZeroMoney, "")

	if err := env.caRepo.Create(NewCorporateAction(ActionTypeSplit, withCA.ID, date, `{"numerator":2,"denominator":1}`)); err != nil {
		t.Fatalf("caRepo.Create() error = %v", err)
	}

	// Simulate a deleted sell on VNQ: drain the lot's shares with no junction.
	vnqLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, vnq.ID, false)
	if len(vnqLots) != 1 {
		t.Fatalf("expected 1 VNQ lot, got %d", len(vnqLots))
	}
	vnqLotID := vnqLots[0].ID
	if err := env.lotRepo.UpdateSharesAndClosed(vnqLotID, types.MustNewQuantity("8.91689"), false); err != nil {
		t.Fatalf("UpdateSharesAndClosed() error = %v", err)
	}

	if _, err := env.svc.RebuildPositions(acct.ID); err != nil {
		t.Fatalf("RebuildPositions() error = %v", err)
	}

	// VNQ lot restored to its full original quantity (no surviving junction).
	vnqLot, _ := env.lotRepo.GetByID(vnqLotID)
	if vnqLot.Shares.String() != "20" {
		t.Errorf("VNQ lot should be restored to 20, got %s", vnqLot.Shares.String())
	}
	// The CA security's lot must be left as the corporate action set it.
	caLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, withCA.ID, false)
	if len(caLots) != 1 || caLots[0].Shares.String() != "10" {
		t.Errorf("CA security lot should be untouched at 10, got %v", caLots)
	}
}

func TestHealAllAccounts_HealsCleanSecuritiesWhenCorporateActionsPresent(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	withCA := createSec(t, env.secRepo, "BTC")
	clean := createSec(t, env.secRepo, "VOO")
	date := types.NewDate(2024, time.March, 15)

	_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
	price := types.MustNewMoney("50.00")
	_, _ = env.svc.Buy(acct.ID, withCA.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, "")
	_, _ = env.svc.Buy(acct.ID, clean.ID, date, types.MustNewQuantity("20"), nil, &price, types.ZeroMoney, "")

	if err := env.caRepo.Create(NewCorporateAction(ActionTypeSplit, withCA.ID, date, `{"numerator":2,"denominator":1}`)); err != nil {
		t.Fatalf("caRepo.Create() error = %v", err)
	}
	zClean := NewPositionWithShares(acct.ID, clean.ID, types.ZeroQuantity, types.ZeroMoney)
	_ = env.positionRepo.CreateOrUpdate(&zClean)

	healed, err := env.svc.HealAllAccounts()
	if err != nil {
		t.Fatalf("HealAllAccounts() error = %v", err)
	}
	if healed < 1 {
		t.Errorf("expected the account to be counted as healed (clean security recomputed), got %d", healed)
	}
	posClean, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, clean.ID)
	if posClean.Shares.String() != "20" {
		t.Errorf("clean security should heal on launch to 20, got %s", posClean.Shares.String())
	}
}
