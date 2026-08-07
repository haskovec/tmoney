package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Update/edit flow tests
//
// Verify that Service.Update* methods correctly reverse an existing
// transaction's position/lot side-effects before applying the new
// transaction. The naive "delete + re-create" pattern leaves positions
// and lots desynced — these tests pin the correct behaviour so we don't
// regress.
// =============================================================================

func TestUpdateSell_NonLotTracking(t *testing.T) {
	t.Run("edit sell with same shares but different price succeeds", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "IEMG")
		date := types.NewDate(2018, time.June, 15)

		// Buy 3 shares total
		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		buyShares := types.MustNewQuantity("3")
		buyPrice := types.MustNewMoney("50.00")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, buyShares, nil, &buyPrice, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// First sell: 1 share at $54.61
		sellShares1 := types.MustNewQuantity("1")
		sellPrice1 := types.MustNewMoney("54.61")
		_, err = env.svc.Sell(acct.ID, sec.ID, date, sellShares1, nil, &sellPrice1, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() first error = %v", err)
		}

		// Second sell: 2 shares at $54.61 (mistake price)
		sellShares2 := types.MustNewQuantity("2")
		sell2, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares2, nil, &sellPrice1, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() second error = %v", err)
		}

		// Edit the second sell to the correct price ($55.00) — same shares
		newPrice := types.MustNewMoney("55.00")
		_, err = env.editSvc.UpdateSell(sell2.ID, acct.ID, sec.ID, date, sellShares2, nil, &newPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("UpdateSell() unexpected error: %v", err)
		}

		// Verify the position: 3 bought, 3 sold → 0 shares remaining
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !pos.Shares.IsZero() {
			t.Errorf("Expected position 0 shares, got %s", pos.Shares.String())
		}
	})

	t.Run("edit sell to fewer shares restores correct position", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		shares := types.MustNewQuantity("10")
		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &buyPrice, types.ZeroMoney, "")

		// Sell 5 shares
		sellShares := types.MustNewQuantity("5")
		sellPrice := types.MustNewMoney("110.00")
		sellTxn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &sellPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Edit sell down to 3 shares
		newSellShares := types.MustNewQuantity("3")
		_, err = env.editSvc.UpdateSell(sellTxn.ID, acct.ID, sec.ID, date, newSellShares, nil, &sellPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("UpdateSell() error = %v", err)
		}

		// Position: 10 bought, 3 sold → 7 shares
		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if pos.Shares.String() != "7" {
			t.Errorf("Expected 7 shares after edit, got %s", pos.Shares.String())
		}
	})

	t.Run("edit sell to more shares than originally sold succeeds when position permits", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "SPY")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		shares := types.MustNewQuantity("10")
		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &buyPrice, types.ZeroMoney, "")

		sellShares := types.MustNewQuantity("3")
		sellPrice := types.MustNewMoney("110.00")
		sellTxn, _ := env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &sellPrice, types.ZeroMoney, "", nil)

		// Edit the sell up to 7 shares (well within remaining position)
		newSellShares := types.MustNewQuantity("7")
		_, err := env.editSvc.UpdateSell(sellTxn.ID, acct.ID, sec.ID, date, newSellShares, nil, &sellPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("UpdateSell() error = %v", err)
		}

		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if pos.Shares.String() != "3" {
			t.Errorf("Expected 3 shares after edit, got %s", pos.Shares.String())
		}
	})

	t.Run("edit sell rejects shares exceeding holdings after reverse", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), "")
		buyShares := types.MustNewQuantity("3")
		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, buyShares, nil, &buyPrice, types.ZeroMoney, "")

		sellShares := types.MustNewQuantity("1")
		sellPrice := types.MustNewMoney("110.00")
		sellTxn, _ := env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &sellPrice, types.ZeroMoney, "", nil)

		// Try to edit the sell to 10 shares (only 3 ever held)
		newSellShares := types.MustNewQuantity("10")
		_, err := env.editSvc.UpdateSell(sellTxn.ID, acct.ID, sec.ID, date, newSellShares, nil, &sellPrice, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("UpdateSell() expected error for shares exceeding holdings")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected InsufficientSharesError, got %T: %v", err, err)
		}
	})
}

func TestUpdateBuy_NonLotTracking(t *testing.T) {
	t.Run("edit buy with different price recomputes average cost", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Buy 1: 1 share at $40
		shares1 := types.MustNewQuantity("1")
		price1 := types.MustNewMoney("40.00")
		buy1, err := env.svc.Buy(acct.ID, sec.ID, date, shares1, nil, &price1, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() 1 error = %v", err)
		}

		// Buy 2: 2 shares at $55 → position: 3 shares avg $50
		shares2 := types.MustNewQuantity("2")
		price2 := types.MustNewMoney("55.00")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, shares2, nil, &price2, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() 2 error = %v", err)
		}

		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if pos.Shares.String() != "3" {
			t.Fatalf("Expected 3 shares, got %s", pos.Shares.String())
		}

		// Edit Buy 1: 1 share at $40 → 1 share at $45
		// Expected new avg = ((45 * 1) + (55 * 2)) / 3 = 155 / 3 ≈ 51.6667
		newPrice := types.MustNewMoney("45.00")
		_, err = env.editSvc.UpdateBuy(buy1.ID, acct.ID, sec.ID, date, shares1, nil, &newPrice, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("UpdateBuy() error = %v", err)
		}

		pos2, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if pos2.Shares.String() != "3" {
			t.Errorf("Expected 3 shares after edit, got %s", pos2.Shares.String())
		}
		// Average cost expectation
		expectedAvg := "51.666666666666666666"
		got := pos2.AverageCostPerShare.String()
		if got != expectedAvg && got != "51.6666666666666667" && got != "51.67" {
			// Be tolerant of trailing-digit variations across decimal scales
			// but ensure the leading digits are ~51.66
			if len(got) < 5 || got[:5] != "51.66" {
				t.Errorf("Expected average cost ~51.6666…, got %s", got)
			}
		}
	})
}

func TestUpdateBuy_LotTracking(t *testing.T) {
	t.Run("edit buy replaces lot when lot has not been sold against", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
		sec := createSec(t, env.secRepo, "QQQ")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		shares := types.MustNewQuantity("5")
		price := types.MustNewMoney("100.00")
		buyTxn, _ := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")

		// Edit the buy to 5 shares at $105
		newPrice := types.MustNewMoney("105.00")
		_, err := env.editSvc.UpdateBuy(buyTxn.ID, acct.ID, sec.ID, date, shares, nil, &newPrice, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("UpdateBuy() error = %v", err)
		}

		// Exactly one open lot, at the new cost
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("Expected 1 lot after edit, got %d", len(lots))
		}
		if lots[0].CostPerShare.String() != "105" {
			t.Errorf("Expected lot cost 105, got %s", lots[0].CostPerShare.String())
		}
	})
}

func TestUpdateSell_LotTracking(t *testing.T) {
	t.Run("edit lot-tracked sell with same shares restores lot then reduces correctly", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
		sec := createSec(t, env.secRepo, "VOO")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Buy 5 shares — creates one lot of 5
		shares := types.MustNewQuantity("5")
		price := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &price, types.ZeroMoney, "")

		openLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(openLots) != 1 {
			t.Fatalf("Expected 1 lot, got %d", len(openLots))
		}
		lotID := openLots[0].ID

		// Sell 2 shares from that lot
		sellShares := types.MustNewQuantity("2")
		sellPrice := types.MustNewMoney("110.00")
		allocations := []SellLotAllocation{{LotID: lotID, Shares: sellShares}}
		sellTxn, err := env.svc.Sell(acct.ID, sec.ID, date, sellShares, nil, &sellPrice, types.ZeroMoney, "", allocations)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Edit the sell to use a different price
		newPrice := types.MustNewMoney("115.00")
		_, err = env.editSvc.UpdateSell(sellTxn.ID, acct.ID, sec.ID, date, sellShares, nil, &newPrice, types.ZeroMoney, "", allocations)
		if err != nil {
			t.Fatalf("UpdateSell() error = %v", err)
		}

		// Lot should have 3 shares remaining (5 - 2 = 3)
		updatedLot, _ := env.lotRepo.GetByID(lotID)
		if updatedLot.Shares.String() != "3" {
			t.Errorf("Expected lot shares 3, got %s", updatedLot.Shares.String())
		}
		if updatedLot.Closed {
			t.Error("Expected lot to remain open")
		}
	})
}

func TestUpdateDividend_NoSideEffects(t *testing.T) {
	t.Run("edit dividend amount only changes cash effect", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "T")
		date := types.NewDate(2024, time.March, 15)

		divTxn, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("125.00"), "Q1")
		if err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}

		_, err = env.editSvc.UpdateDividend(divTxn.ID, acct.ID, sec.ID, date, types.MustNewMoney("150.00"), "Q1 corrected")
		if err != nil {
			t.Fatalf("UpdateDividend() error = %v", err)
		}

		balance, _ := env.svc.GetCashBalance(acct.ID)
		if balance.String() != "150" {
			t.Errorf("Expected balance 150 after edit, got %s", balance.String())
		}
	})
}

func TestUpdateFee_AllowsNegativeBalance(t *testing.T) {
	t.Run("edit fee amount up succeeds even past available cash", func(t *testing.T) {
		svc, accountRepo := createTestService(t)
		acct := createInvAccount(t, accountRepo, "Brokerage")
		date := types.NewDate(2024, time.March, 15)

		_, _ = svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")
		feeTxn, _ := svc.Fee(acct.ID, date, types.MustNewMoney("25.00"), "")

		_, err := NewEditService(svc).UpdateFee(feeTxn.ID, acct.ID, date, types.MustNewMoney("200.00"), "")
		if err != nil {
			t.Fatalf("UpdateFee() error = %v", err)
		}

		balance, _ := svc.GetCashBalance(acct.ID)
		if balance.String() != "-100" {
			t.Errorf("Expected balance -100 after edit, got %s", balance.String())
		}
	})
}

func TestUpdateFeeLiquidation_NonLotTracking(t *testing.T) {
	t.Run("edit fee-liquidation to fewer shares restores correct position", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		date := types.NewDate(2024, time.March, 15)

		// fee_liquidation has no cash effect, so no deposit is needed.
		shares := types.MustNewQuantity("10")
		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &buyPrice, types.ZeroMoney, "")

		flShares := types.MustNewQuantity("5")
		flPrice := types.MustNewMoney("110.00")
		flTxn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, flShares, nil, &flPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		newShares := types.MustNewQuantity("3")
		_, err = env.editSvc.UpdateFeeLiquidation(flTxn.ID, acct.ID, sec.ID, date, newShares, nil, &flPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("UpdateFeeLiquidation() error = %v", err)
		}

		// Position: 10 bought, 3 liquidated → 7 shares
		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if pos.Shares.String() != "7" {
			t.Errorf("Expected 7 shares after edit, got %s", pos.Shares.String())
		}
	})

	t.Run("edit fee-liquidation rejects shares exceeding holdings", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("3"), nil, &buyPrice, types.ZeroMoney, "")
		flPrice := types.MustNewMoney("110.00")
		flTxn, _ := env.svc.FeeLiquidation(acct.ID, sec.ID, date, types.MustNewQuantity("1"), nil, &flPrice, types.ZeroMoney, "", nil)

		_, err := env.editSvc.UpdateFeeLiquidation(flTxn.ID, acct.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &flPrice, types.ZeroMoney, "", nil)
		if err == nil {
			t.Fatal("UpdateFeeLiquidation() expected error for shares exceeding holdings")
		}
		if _, ok := err.(*InsufficientSharesError); !ok {
			t.Errorf("Expected *InsufficientSharesError, got %T: %v", err, err)
		}
	})
}

func TestUpdateFeeLiquidation_LotTracking(t *testing.T) {
	t.Run("edit lot-tracked fee-liquidation restores lot then reduces correctly", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
		sec := createSec(t, env.secRepo, "VOO")
		date := types.NewDate(2024, time.March, 15)

		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), nil, &buyPrice, types.ZeroMoney, "")
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(openLots) != 1 {
			t.Fatalf("expected 1 open lot, got %d", len(openLots))
		}
		lotID := openLots[0].ID

		flShares := types.MustNewQuantity("2")
		flPrice := types.MustNewMoney("110.00")
		alloc := []SellLotAllocation{{LotID: lotID, Shares: flShares}}
		flTxn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, flShares, nil, &flPrice, types.ZeroMoney, "", alloc)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		newPrice := types.MustNewMoney("115.00")
		_, err = env.editSvc.UpdateFeeLiquidation(flTxn.ID, acct.ID, sec.ID, date, flShares, nil, &newPrice, types.ZeroMoney, "", alloc)
		if err != nil {
			t.Fatalf("UpdateFeeLiquidation() error = %v", err)
		}

		// Lot restored to 5 on reverse, reduced by 2 by the re-created
		// fee-liquidation → 3 open shares.
		updatedLot, err := env.lotRepo.GetByID(lotID)
		if err != nil {
			t.Fatalf("GetByID(lot) error = %v", err)
		}
		if updatedLot.Shares.String() != "3" {
			t.Errorf("Expected lot shares 3, got %s", updatedLot.Shares.String())
		}
		if updatedLot.Closed {
			t.Error("Expected lot to remain open")
		}
	})

	t.Run("edit auto-FIFO increases shares beyond pre-reverse remaining", func(t *testing.T) {
		// Regression: the FIFO allocation must be computed AFTER the edit reverses
		// the old fee-liquidation (restoring the lot), not from the pre-reverse
		// snapshot — otherwise growing the share count is wrongly rejected.
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
		sec := createSec(t, env.secRepo, "VOO")
		date := types.NewDate(2024, time.March, 15)

		buyPrice := types.MustNewMoney("100.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &buyPrice, types.ZeroMoney, "")
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		lotID := openLots[0].ID

		// Original fee-liquidation of 3 (auto-FIFO → lot down to 7).
		flPrice := types.MustNewMoney("110.00")
		flTxn, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, types.MustNewQuantity("3"), nil, &flPrice, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("FeeLiquidation() error = %v", err)
		}

		// Edit UP to 9 shares. 9 > the 7 remaining pre-reverse, but the reverse
		// restores the lot to 10 first, so 9 is valid and must succeed.
		if _, err := env.editSvc.UpdateFeeLiquidation(flTxn.ID, acct.ID, sec.ID, date, types.MustNewQuantity("9"), nil, &flPrice, types.ZeroMoney, "", nil); err != nil {
			t.Fatalf("UpdateFeeLiquidation() grow-shares error = %v", err)
		}
		updatedLot, _ := env.lotRepo.GetByID(lotID)
		if updatedLot.Shares.String() != "1" {
			t.Errorf("Expected lot shares 1 (10 - 9), got %s", updatedLot.Shares.String())
		}
	})
}
