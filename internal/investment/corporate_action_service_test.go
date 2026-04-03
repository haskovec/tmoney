package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Helpers
// =============================================================================

type testCAServiceEnv struct {
	caSvc        *CorporateActionService
	invSvc       *Service
	accountRepo  *account.Repository
	secRepo      *security.Repository
	priceRepo    *price.Repository
	positionRepo *PositionRepository
	lotRepo      *LotRepository
	caRepo       *CorporateActionRepository
}

func createCATestEnv(t *testing.T) *testCAServiceEnv {
	t.Helper()
	database := createTestDB(t)

	accountRepo := account.NewRepository(database)
	secRepo := security.NewRepository(database)
	priceRepo := price.NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)
	invRepo := NewRepository(database)

	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, nil, database)
	caSvc := NewCorporateActionService(caRepo, lotRepo, positionRepo, priceRepo, database)

	return &testCAServiceEnv{
		caSvc:        caSvc,
		invSvc:       invSvc,
		accountRepo:  accountRepo,
		secRepo:      secRepo,
		priceRepo:    priceRepo,
		positionRepo: positionRepo,
		lotRepo:      lotRepo,
		caRepo:       caRepo,
	}
}

// =============================================================================
// SM-147: Stock split — adjust lots
// =============================================================================

func TestCorporateActionService_Split_AdjustLots(t *testing.T) {
	t.Run("4:1 split adjusts lot shares and cost_per_share", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		// Deposit cash and buy shares
		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("1000.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify lot before split
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("expected 1 lot, got %d", len(lots))
		}
		if !lots[0].Shares.Equal(types.MustNewQuantity("10")) {
			t.Errorf("pre-split shares = %s, want 10", lots[0].Shares.String())
		}
		if !lots[0].CostPerShare.Equal(types.MustNewMoney("100")) {
			t.Errorf("pre-split cost_per_share = %s, want 100", lots[0].CostPerShare.String())
		}
		originalShares := lots[0].OriginalShares

		// Apply 4:1 split
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Verify lot after split
		lots, err = env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() after split error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("expected 1 lot after split, got %d", len(lots))
		}

		if !lots[0].Shares.Equal(types.MustNewQuantity("40")) {
			t.Errorf("post-split shares = %s, want 40", lots[0].Shares.String())
		}
		if !lots[0].CostPerShare.Equal(types.MustNewMoney("25")) {
			t.Errorf("post-split cost_per_share = %s, want 25", lots[0].CostPerShare.String())
		}
		// original_shares should NOT be modified
		if !lots[0].OriginalShares.Equal(originalShares) {
			t.Errorf("original_shares changed: got %s, want %s", lots[0].OriginalShares.String(), originalShares.String())
		}
	})

	t.Run("split adjusts multiple lots", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 1)
		splitDate := types.NewDate(2024, time.June, 1)

		// Deposit cash
		_, err := env.invSvc.Deposit(acct.ID, date1, types.MustNewMoney("20000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy two lots at different prices
		total1 := types.MustNewMoney("1000.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() lot 1 error = %v", err)
		}

		total2 := types.MustNewMoney("3000.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("20"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() lot 2 error = %v", err)
		}

		// Apply 4:1 split
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Verify both lots adjusted
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 2 {
			t.Fatalf("expected 2 lots, got %d", len(lots))
		}

		// Lot 1: 10 shares at $100 → 40 shares at $25
		if !lots[0].Shares.Equal(types.MustNewQuantity("40")) {
			t.Errorf("lot 1 shares = %s, want 40", lots[0].Shares.String())
		}
		if !lots[0].CostPerShare.Equal(types.MustNewMoney("25")) {
			t.Errorf("lot 1 cost_per_share = %s, want 25", lots[0].CostPerShare.String())
		}

		// Lot 2: 20 shares at $150 → 80 shares at $37.50
		if !lots[1].Shares.Equal(types.MustNewQuantity("80")) {
			t.Errorf("lot 2 shares = %s, want 80", lots[1].Shares.String())
		}
		if !lots[1].CostPerShare.Equal(types.MustNewMoney("37.5")) {
			t.Errorf("lot 2 cost_per_share = %s, want 37.5", lots[1].CostPerShare.String())
		}
	})

	t.Run("split does not affect closed lots", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		// Deposit, buy, then sell to close the lot
		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("1000.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Get the lot ID for sell
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		sellTotal := types.MustNewMoney("1200.00")
		_, err = env.invSvc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &sellTotal, nil, types.ZeroMoney, "",
			[]SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}})
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Verify lot is closed
		closedLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if len(closedLots) != 1 || !closedLots[0].Closed {
			t.Fatal("expected lot to be closed")
		}

		closedCost := closedLots[0].CostPerShare

		// Apply split — should not affect the closed lot
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Re-read closed lots — should be unchanged
		closedLots, _ = env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if !closedLots[0].CostPerShare.Equal(closedCost) {
			t.Errorf("closed lot cost_per_share changed: got %s, want %s", closedLots[0].CostPerShare.String(), closedCost.String())
		}
	})
}

// =============================================================================
// SM-148: Stock split — adjust positions (non-lot-tracking)
// =============================================================================

func TestCorporateActionService_Split_AdjustPositions(t *testing.T) {
	t.Run("4:1 split adjusts position shares and average cost", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		// Deposit and buy
		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("1000.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify position before split
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !pos.Shares.Equal(types.MustNewQuantity("10")) {
			t.Errorf("pre-split shares = %s, want 10", pos.Shares.String())
		}
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("100")) {
			t.Errorf("pre-split avg cost = %s, want 100", pos.AverageCostPerShare.String())
		}

		// Apply 4:1 split
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Verify position after split
		pos, err = env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() after split error = %v", err)
		}
		if !pos.Shares.Equal(types.MustNewQuantity("40")) {
			t.Errorf("post-split shares = %s, want 40", pos.Shares.String())
		}
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("25")) {
			t.Errorf("post-split avg cost = %s, want 25", pos.AverageCostPerShare.String())
		}
	})

	t.Run("split adjusts positions across multiple accounts", func(t *testing.T) {
		env := createCATestEnv(t)

		acct1 := createInvAccount(t, env.accountRepo, "Brokerage1")
		acct2 := createInvAccount(t, env.accountRepo, "Brokerage2")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		// Deposit and buy in both accounts
		for _, acct := range []*account.Account{acct1, acct2} {
			_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
			if err != nil {
				t.Fatalf("Deposit() error = %v", err)
			}
			total := types.MustNewMoney("500.00")
			_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
			if err != nil {
				t.Fatalf("Buy() error = %v", err)
			}
		}

		// Apply 2:1 split
		params := SplitParams{Numerator: 2, Denominator: 1}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Verify both accounts adjusted
		for _, acct := range []*account.Account{acct1, acct2} {
			pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
			if err != nil {
				t.Fatalf("GetByAccountAndSecurity() error = %v", err)
			}
			if !pos.Shares.Equal(types.MustNewQuantity("10")) {
				t.Errorf("account %s post-split shares = %s, want 10", acct.Name, pos.Shares.String())
			}
			if !pos.AverageCostPerShare.Equal(types.MustNewMoney("50")) {
				t.Errorf("account %s post-split avg cost = %s, want 50", acct.Name, pos.AverageCostPerShare.String())
			}
		}
	})
}

// =============================================================================
// SM-149: Stock split — adjust price history
// =============================================================================

func TestCorporateActionService_Split_AdjustPrices(t *testing.T) {
	t.Run("prices on or before split date are divided by ratio", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		// Create prices before and after split date
		priceBefore1 := price.NewPrice(sec.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("100.00"), price.SourceManual)
		priceBefore2 := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 15), types.MustNewMoney("120.00"), price.SourceManual)
		priceOnDate := price.NewPrice(sec.ID, splitDate, types.MustNewMoney("160.00"), price.SourceManual)
		priceAfter := price.NewPrice(sec.ID, types.NewDate(2024, time.July, 1), types.MustNewMoney("40.00"), price.SourceManual)

		for _, p := range []*price.Price{priceBefore1, priceBefore2, priceOnDate, priceAfter} {
			if err := env.priceRepo.Create(p); err != nil {
				t.Fatalf("Create price error = %v", err)
			}
		}

		// Apply 4:1 split
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// Verify prices before/on split date are divided by 4
		p1, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 1))
		if !p1.Price.Equal(types.MustNewMoney("25")) {
			t.Errorf("price 2024-01-01 = %s, want 25", p1.Price.String())
		}

		p2, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.March, 15))
		if !p2.Price.Equal(types.MustNewMoney("30")) {
			t.Errorf("price 2024-03-15 = %s, want 30", p2.Price.String())
		}

		p3, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, splitDate)
		if !p3.Price.Equal(types.MustNewMoney("40")) {
			t.Errorf("price on split date = %s, want 40", p3.Price.String())
		}

		// Price after split date should be unchanged
		p4, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.July, 1))
		if !p4.Price.Equal(types.MustNewMoney("40")) {
			t.Errorf("price after split = %s, want 40 (unchanged)", p4.Price.String())
		}
	})

	t.Run("no prices to adjust does not error", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() with no prices error = %v", err)
		}
	})
}

// =============================================================================
// SM-150: Stock split — audit log
// =============================================================================

func TestCorporateActionService_Split_AuditLog(t *testing.T) {
	t.Run("split creates corporate action record", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		params := SplitParams{Numerator: 4, Denominator: 1}
		ca, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		if ca == nil {
			t.Fatal("Split() returned nil corporate action")
		}
		if ca.ActionType != ActionTypeSplit {
			t.Errorf("action_type = %s, want %s", ca.ActionType, ActionTypeSplit)
		}
		if ca.SecurityID != sec.ID {
			t.Errorf("security_id mismatch")
		}
		if !ca.ActionDate.Equal(splitDate) {
			t.Errorf("action_date mismatch")
		}

		// Verify parameters deserialize correctly
		parsedParams, err := ParseSplitParams(ca.Parameters)
		if err != nil {
			t.Fatalf("ParseSplitParams() error = %v", err)
		}
		if parsedParams.Numerator != 4 || parsedParams.Denominator != 1 {
			t.Errorf("params = %d:%d, want 4:1", parsedParams.Numerator, parsedParams.Denominator)
		}

		// Verify record persisted
		retrieved, err := env.caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ActionType != ActionTypeSplit {
			t.Errorf("persisted action_type = %s, want %s", retrieved.ActionType, ActionTypeSplit)
		}
	})

	t.Run("audit log is queryable by security", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		params := SplitParams{Numerator: 4, Denominator: 1}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		actions, err := env.caRepo.ListBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity() error = %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].ActionType != ActionTypeSplit {
			t.Errorf("action_type = %s, want %s", actions[0].ActionType, ActionTypeSplit)
		}
	})
}

// =============================================================================
// SM-151: Reverse split
// =============================================================================

func TestCorporateActionService_ReverseSplit(t *testing.T) {
	t.Run("1:10 reverse split adjusts lots", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("500.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Apply 1:10 reverse split
		params := SplitParams{Numerator: 1, Denominator: 10}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("expected 1 lot, got %d", len(lots))
		}

		// 100 shares × 0.1 = 10 shares
		if !lots[0].Shares.Equal(types.MustNewQuantity("10")) {
			t.Errorf("post-split shares = %s, want 10", lots[0].Shares.String())
		}
		// $5/share ÷ 0.1 = $50/share
		if !lots[0].CostPerShare.Equal(types.MustNewMoney("50")) {
			t.Errorf("post-split cost_per_share = %s, want 50", lots[0].CostPerShare.String())
		}
	})

	t.Run("1:10 reverse split adjusts positions", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("500.00")
		_, err = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Apply 1:10 reverse split
		params := SplitParams{Numerator: 1, Denominator: 10}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		// 100 shares × 0.1 = 10 shares
		if !pos.Shares.Equal(types.MustNewQuantity("10")) {
			t.Errorf("post-split shares = %s, want 10", pos.Shares.String())
		}
		// $5/share ÷ 0.1 = $50/share
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("50")) {
			t.Errorf("post-split avg cost = %s, want 50", pos.AverageCostPerShare.String())
		}
	})

	t.Run("reverse split adjusts price history", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		// Price before split: $5
		priceBefore := price.NewPrice(sec.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("5.00"), price.SourceManual)
		if err := env.priceRepo.Create(priceBefore); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		// Price after split: $50 (already post-split)
		priceAfter := price.NewPrice(sec.ID, types.NewDate(2024, time.July, 1), types.MustNewMoney("50.00"), price.SourceManual)
		if err := env.priceRepo.Create(priceAfter); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		// Apply 1:10 reverse split
		params := SplitParams{Numerator: 1, Denominator: 10}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// $5 ÷ 0.1 = $50
		p1, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.January, 1))
		if !p1.Price.Equal(types.MustNewMoney("50")) {
			t.Errorf("price before split = %s, want 50", p1.Price.String())
		}

		// After split date: unchanged
		p2, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, types.NewDate(2024, time.July, 1))
		if !p2.Price.Equal(types.MustNewMoney("50")) {
			t.Errorf("price after split = %s, want 50 (unchanged)", p2.Price.String())
		}
	})

	t.Run("reverse split creates audit log with reverse_split type", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		params := SplitParams{Numerator: 1, Denominator: 10}
		ca, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		if ca.ActionType != ActionTypeReverseSplit {
			t.Errorf("action_type = %s, want %s", ca.ActionType, ActionTypeReverseSplit)
		}
	})
}

// =============================================================================
// Validation
// =============================================================================

func TestCorporateActionService_Split_Validation(t *testing.T) {
	t.Run("invalid params rejected", func(t *testing.T) {
		env := createCATestEnv(t)

		sec := createSec(t, env.secRepo, "AAPL")
		splitDate := types.NewDate(2024, time.June, 1)

		// Zero numerator
		params := SplitParams{Numerator: 0, Denominator: 1}
		_, err := env.caSvc.Split(sec.ID, splitDate, params)
		if err == nil {
			t.Error("Split() with zero numerator should error")
		}

		// Zero denominator
		params = SplitParams{Numerator: 4, Denominator: 0}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err == nil {
			t.Error("Split() with zero denominator should error")
		}

		// Negative values
		params = SplitParams{Numerator: -1, Denominator: 1}
		_, err = env.caSvc.Split(sec.ID, splitDate, params)
		if err == nil {
			t.Error("Split() with negative numerator should error")
		}
	})

	t.Run("cost basis preserved after split", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		// Get cost basis before split
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		costBasisBefore := lots[0].CostBasis()

		// Apply split
		params := SplitParams{Numerator: 4, Denominator: 1}
		_, _ = env.caSvc.Split(sec.ID, splitDate, params)

		// Verify cost basis preserved (40 shares × $25 = $1000 = 10 shares × $100)
		lots, _ = env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		costBasisAfter := lots[0].CostBasis()

		if !costBasisBefore.Equal(costBasisAfter) {
			t.Errorf("cost basis changed: before %s, after %s", costBasisBefore.String(), costBasisAfter.String())
		}
	})
}
