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
	invRepo      *Repository
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

	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, caRepo, database)
	caSvc := NewCorporateActionService(caRepo, lotRepo, positionRepo, priceRepo, invRepo, secRepo, database)

	return &testCAServiceEnv{
		caSvc:        caSvc,
		invSvc:       invSvc,
		accountRepo:  accountRepo,
		secRepo:      secRepo,
		priceRepo:    priceRepo,
		positionRepo: positionRepo,
		lotRepo:      lotRepo,
		caRepo:       caRepo,
		invRepo:      invRepo,
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
		// original_shares is scaled in lock-step with shares (4:1 → 40), so the
		// "remaining = original − consumed" invariant survives the split.
		if !lots[0].OriginalShares.Equal(types.MustNewQuantity("40")) {
			t.Errorf("post-split original_shares = %s, want 40 (scaled with shares)", lots[0].OriginalShares.String())
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

// =============================================================================
// SM-152: Merger — exchange shares across accounts (lot-tracking)
// =============================================================================

func TestCorporateActionService_Merger_ExchangeShares(t *testing.T) {
	t.Run("2:1 merger exchanges lots correctly", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		// Deposit cash and buy 100 shares of source at $50/share
		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		total := types.MustNewMoney("5000.00")
		_, err = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify source lot before merger
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sourceSec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity() error = %v", err)
		}
		if len(lots) != 1 {
			t.Fatalf("expected 1 source lot, got %d", len(lots))
		}
		costBasisBefore := lots[0].CostBasis()

		// Apply 2:1 merger (2 old shares = 1 new share)
		params := MergerParams{ExchangeRatio: 2.0}
		_, err = env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Source lots should be closed
		sourceLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sourceSec.ID, true)
		if len(sourceLots) != 1 {
			t.Fatalf("expected 1 source lot (closed), got %d", len(sourceLots))
		}
		if !sourceLots[0].Closed {
			t.Error("source lot should be closed")
		}
		if !sourceLots[0].Shares.IsZero() {
			t.Errorf("source lot shares = %s, want 0", sourceLots[0].Shares.String())
		}

		// Target lots should be created: 100/2 = 50 shares
		targetLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, targetSec.ID, false)
		if len(targetLots) != 1 {
			t.Fatalf("expected 1 target lot, got %d", len(targetLots))
		}
		if !targetLots[0].Shares.Equal(types.MustNewQuantity("50")) {
			t.Errorf("target lot shares = %s, want 50", targetLots[0].Shares.String())
		}
		// Cost per share should be 50*2=100 to preserve cost basis
		if !targetLots[0].CostPerShare.Equal(types.MustNewMoney("100")) {
			t.Errorf("target lot cost_per_share = %s, want 100", targetLots[0].CostPerShare.String())
		}
		// Cost basis preserved: 50 × $100 = $5000 = 100 × $50
		costBasisAfter := targetLots[0].CostBasis()
		if !costBasisBefore.Equal(costBasisAfter) {
			t.Errorf("cost basis changed: before %s, after %s", costBasisBefore.String(), costBasisAfter.String())
		}
	})

	t.Run("merger exchanges multiple lots", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 1)
		mergerDate := types.NewDate(2024, time.June, 1)

		// Deposit cash
		_, err := env.invSvc.Deposit(acct.ID, date1, types.MustNewMoney("20000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy two lots at different prices
		total1 := types.MustNewMoney("1000.00") // 10 shares at $100
		_, err = env.invSvc.Buy(acct.ID, sourceSec.ID, date1, types.MustNewQuantity("10"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() lot 1 error = %v", err)
		}
		total2 := types.MustNewMoney("3000.00") // 20 shares at $150
		_, err = env.invSvc.Buy(acct.ID, sourceSec.ID, date2, types.MustNewQuantity("20"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() lot 2 error = %v", err)
		}

		// Apply 2:1 merger
		params := MergerParams{ExchangeRatio: 2.0}
		_, err = env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Source lots should all be closed
		sourceLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sourceSec.ID, false)
		if len(sourceLots) != 0 {
			t.Errorf("expected 0 open source lots, got %d", len(sourceLots))
		}

		// Target lots created: lot1: 10/2=5 at $200, lot2: 20/2=10 at $300
		targetLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, targetSec.ID, false)
		if len(targetLots) != 2 {
			t.Fatalf("expected 2 target lots, got %d", len(targetLots))
		}

		// Lot 1: 5 shares at $200 (cost basis = $1000)
		if !targetLots[0].Shares.Equal(types.MustNewQuantity("5")) {
			t.Errorf("target lot 1 shares = %s, want 5", targetLots[0].Shares.String())
		}
		if !targetLots[0].CostPerShare.Equal(types.MustNewMoney("200")) {
			t.Errorf("target lot 1 cost_per_share = %s, want 200", targetLots[0].CostPerShare.String())
		}

		// Lot 2: 10 shares at $300 (cost basis = $3000)
		if !targetLots[1].Shares.Equal(types.MustNewQuantity("10")) {
			t.Errorf("target lot 2 shares = %s, want 10", targetLots[1].Shares.String())
		}
		if !targetLots[1].CostPerShare.Equal(types.MustNewMoney("300")) {
			t.Errorf("target lot 2 cost_per_share = %s, want 300", targetLots[1].CostPerShare.String())
		}
	})

	t.Run("merger preserves purchase date on target lots", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		purchaseDate := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, purchaseDate, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, purchaseDate, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		targetLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, targetSec.ID, false)
		if len(targetLots) != 1 {
			t.Fatalf("expected 1 target lot, got %d", len(targetLots))
		}
		if !targetLots[0].PurchaseDate.Equal(purchaseDate) {
			t.Errorf("target lot purchase_date = %s, want %s", targetLots[0].PurchaseDate.String(), purchaseDate.String())
		}
	})

	t.Run("merger across multiple accounts", func(t *testing.T) {
		env := createCATestEnv(t)

		acct1 := createLotTrackingAccount(t, env.accountRepo, "Brokerage1")
		acct2 := createLotTrackingAccount(t, env.accountRepo, "Brokerage2")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		// Both accounts buy source security
		for _, acct := range []*account.Account{acct1, acct2} {
			_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
			total := types.MustNewMoney("1000.00")
			_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		}

		// Apply merger
		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Both accounts should have target lots
		for _, acct := range []*account.Account{acct1, acct2} {
			targetLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, targetSec.ID, false)
			if len(targetLots) != 1 {
				t.Errorf("account %s: expected 1 target lot, got %d", acct.Name, len(targetLots))
			}
			if !targetLots[0].Shares.Equal(types.MustNewQuantity("5")) {
				t.Errorf("account %s target shares = %s, want 5", acct.Name, targetLots[0].Shares.String())
			}
		}
	})
}

// =============================================================================
// SM-153: Merger — cash consideration
// =============================================================================

func TestCorporateActionService_Merger_CashConsideration(t *testing.T) {
	t.Run("cash consideration adds to cash balance (lot-tracking)", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("5000.00") // 100 shares at $50
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		// Cash balance before merger: 10000 - 5000 = 5000
		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		// Apply merger with $5/share cash consideration
		params := MergerParams{ExchangeRatio: 2.0, CashPerShare: 5.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Cash consideration: $5 × 100 old shares = $500
		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		expectedCash := cashBefore.Add(types.MustNewMoney("500"))
		if !cashAfter.Equal(expectedCash) {
			t.Errorf("cash balance = %s, want %s", cashAfter.String(), expectedCash.String())
		}
	})

	t.Run("cash consideration adds to cash balance (non-lot-tracking)", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("5000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		params := MergerParams{ExchangeRatio: 2.0, CashPerShare: 5.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		expectedCash := cashBefore.Add(types.MustNewMoney("500"))
		if !cashAfter.Equal(expectedCash) {
			t.Errorf("cash balance = %s, want %s", cashAfter.String(), expectedCash.String())
		}
	})

	t.Run("no cash consideration when not specified", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("5000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		params := MergerParams{ExchangeRatio: 2.0} // No cash
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		if !cashAfter.Equal(cashBefore) {
			t.Errorf("cash balance changed: before %s, after %s", cashBefore.String(), cashAfter.String())
		}
	})
}

// =============================================================================
// SM-154: Merger — auto-hide source security
// =============================================================================

func TestCorporateActionService_Merger_AutoHide(t *testing.T) {
	t.Run("source security hidden after merger", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		// Source not hidden before merger
		sec, _ := env.secRepo.GetByID(sourceSec.ID)
		if sec.Hidden {
			t.Fatal("source security should not be hidden before merger")
		}

		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Source should be hidden after merger
		sec, _ = env.secRepo.GetByID(sourceSec.ID)
		if !sec.Hidden {
			t.Error("source security should be hidden after merger")
		}

		// Target should not be hidden
		tgtSec, _ := env.secRepo.GetByID(targetSec.ID)
		if tgtSec.Hidden {
			t.Error("target security should not be hidden")
		}
	})

	t.Run("already hidden source stays hidden", func(t *testing.T) {
		env := createCATestEnv(t)

		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")

		// Pre-hide source security
		sourceSec.Hide()
		_ = env.secRepo.Update(sourceSec)

		mergerDate := types.NewDate(2024, time.June, 1)
		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		sec, _ := env.secRepo.GetByID(sourceSec.ID)
		if !sec.Hidden {
			t.Error("source security should remain hidden")
		}
	})
}

// =============================================================================
// SM-155: Merger — non-lot-tracking accounts (positions)
// =============================================================================

func TestCorporateActionService_Merger_Positions(t *testing.T) {
	t.Run("2:1 merger converts position shares and cost", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		// Deposit and buy 100 shares at $50
		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("5000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		// Verify position before merger
		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sourceSec.ID)
		costBasisBefore := pos.CostBasis()

		// Apply 2:1 merger
		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Source position should be zeroed
		sourcePos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, sourceSec.ID)
		if !sourcePos.Shares.IsZero() {
			t.Errorf("source position shares = %s, want 0", sourcePos.Shares.String())
		}

		// Target position: 100/2 = 50 shares at $100 avg cost
		targetPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, targetSec.ID)
		if !targetPos.Shares.Equal(types.MustNewQuantity("50")) {
			t.Errorf("target position shares = %s, want 50", targetPos.Shares.String())
		}
		if !targetPos.AverageCostPerShare.Equal(types.MustNewMoney("100")) {
			t.Errorf("target position avg cost = %s, want 100", targetPos.AverageCostPerShare.String())
		}

		// Cost basis preserved
		costBasisAfter := targetPos.CostBasis()
		if !costBasisBefore.Equal(costBasisAfter) {
			t.Errorf("cost basis changed: before %s, after %s", costBasisBefore.String(), costBasisAfter.String())
		}
	})

	t.Run("merger across multiple non-lot accounts", func(t *testing.T) {
		env := createCATestEnv(t)

		acct1 := createInvAccount(t, env.accountRepo, "Brokerage1")
		acct2 := createInvAccount(t, env.accountRepo, "Brokerage2")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		for _, acct := range []*account.Account{acct1, acct2} {
			_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
			total := types.MustNewMoney("500.00")
			_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
		}

		params := MergerParams{ExchangeRatio: 1.0} // 1:1 exchange for simplicity
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		for _, acct := range []*account.Account{acct1, acct2} {
			targetPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, targetSec.ID)
			if !targetPos.Shares.Equal(types.MustNewQuantity("5")) {
				t.Errorf("account %s target shares = %s, want 5", acct.Name, targetPos.Shares.String())
			}
			if !targetPos.AverageCostPerShare.Equal(types.MustNewMoney("100")) {
				t.Errorf("account %s target avg cost = %s, want 100", acct.Name, targetPos.AverageCostPerShare.String())
			}
		}
	})

	t.Run("merger into existing target position merges cost basis", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.January, 15)
		mergerDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")

		// Already holding 10 shares of target at $200
		targetTotal := types.MustNewMoney("2000.00")
		_, _ = env.invSvc.Buy(acct.ID, targetSec.ID, date, types.MustNewQuantity("10"), &targetTotal, nil, types.ZeroMoney, "")

		// Buy 20 shares of source at $50
		sourceTotal := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, sourceSec.ID, date, types.MustNewQuantity("20"), &sourceTotal, nil, types.ZeroMoney, "")

		// Apply 2:1 merger: 20 source → 10 target at $100 each
		params := MergerParams{ExchangeRatio: 2.0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		// Target: existing 10@$200 + merged 10@$100 = 20 shares
		// Weighted avg: ($2000 + $1000) / 20 = $150
		targetPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, targetSec.ID)
		if !targetPos.Shares.Equal(types.MustNewQuantity("20")) {
			t.Errorf("target shares = %s, want 20", targetPos.Shares.String())
		}
		if !targetPos.AverageCostPerShare.Equal(types.MustNewMoney("150")) {
			t.Errorf("target avg cost = %s, want 150", targetPos.AverageCostPerShare.String())
		}
	})
}

// =============================================================================
// SM-156: Merger — audit log
// =============================================================================

func TestCorporateActionService_Merger_AuditLog(t *testing.T) {
	t.Run("merger creates corporate action record", func(t *testing.T) {
		env := createCATestEnv(t)

		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		mergerDate := types.NewDate(2024, time.June, 15)

		params := MergerParams{ExchangeRatio: 2.0, CashPerShare: 5.0}
		ca, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}

		if ca == nil {
			t.Fatal("Merger() returned nil corporate action")
		}
		if ca.ActionType != ActionTypeMerger {
			t.Errorf("action_type = %s, want %s", ca.ActionType, ActionTypeMerger)
		}
		if ca.SecurityID != sourceSec.ID {
			t.Error("security_id should be source security")
		}
		if !ca.TargetSecurityID.Valid || ca.TargetSecurityID.ID != targetSec.ID {
			t.Error("target_security_id should be set to target security")
		}
		if !ca.ActionDate.Equal(mergerDate) {
			t.Error("action_date mismatch")
		}

		// Verify parameters deserialize correctly
		parsedParams, err := ParseMergerParams(ca.Parameters)
		if err != nil {
			t.Fatalf("ParseMergerParams() error = %v", err)
		}
		if parsedParams.ExchangeRatio != 2.0 {
			t.Errorf("exchange_ratio = %f, want 2.0", parsedParams.ExchangeRatio)
		}
		if parsedParams.CashPerShare != 5.0 {
			t.Errorf("cash_per_share = %f, want 5.0", parsedParams.CashPerShare)
		}
	})

	t.Run("audit log persisted and queryable", func(t *testing.T) {
		env := createCATestEnv(t)

		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		mergerDate := types.NewDate(2024, time.June, 15)

		params := MergerParams{ExchangeRatio: 2.0}
		ca, _ := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)

		// Verify persisted
		retrieved, err := env.caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ActionType != ActionTypeMerger {
			t.Errorf("persisted action_type = %s, want %s", retrieved.ActionType, ActionTypeMerger)
		}

		// Queryable by source security
		actions, err := env.caRepo.ListBySecurity(sourceSec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(source) error = %v", err)
		}
		if len(actions) != 1 {
			t.Errorf("expected 1 action for source, got %d", len(actions))
		}

		// Queryable by target security
		actions, err = env.caRepo.ListBySecurity(targetSec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(target) error = %v", err)
		}
		if len(actions) != 1 {
			t.Errorf("expected 1 action for target, got %d", len(actions))
		}
	})
}

// =============================================================================
// Merger validation
// =============================================================================

func TestCorporateActionService_Merger_Validation(t *testing.T) {
	t.Run("invalid params rejected", func(t *testing.T) {
		env := createCATestEnv(t)

		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		mergerDate := types.NewDate(2024, time.June, 1)

		// Zero exchange ratio
		params := MergerParams{ExchangeRatio: 0}
		_, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err == nil {
			t.Error("Merger() with zero exchange ratio should error")
		}

		// Negative exchange ratio
		params = MergerParams{ExchangeRatio: -1.0}
		_, err = env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err == nil {
			t.Error("Merger() with negative exchange ratio should error")
		}

		// Negative cash per share
		params = MergerParams{ExchangeRatio: 2.0, CashPerShare: -5.0}
		_, err = env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err == nil {
			t.Error("Merger() with negative cash per share should error")
		}
	})

	t.Run("merger with no holdings succeeds", func(t *testing.T) {
		env := createCATestEnv(t)

		sourceSec := createSec(t, env.secRepo, "OLD")
		targetSec := createSec(t, env.secRepo, "NEW")
		mergerDate := types.NewDate(2024, time.June, 1)

		params := MergerParams{ExchangeRatio: 2.0}
		ca, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, mergerDate, params)
		if err != nil {
			t.Fatalf("Merger() with no holdings error = %v", err)
		}
		if ca == nil {
			t.Error("Merger() should return corporate action even with no holdings")
		}
	})
}

// =============================================================================
// SM-157: Spin-off — cost basis allocation to parent
// =============================================================================

func TestCorporateActionService_SpinOff_CostBasisAllocation(t *testing.T) {
	t.Run("parent lot cost reduced by allocation percentage", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		// Deposit cash and buy 100 shares at $100/share
		_, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		total := types.MustNewMoney("10000.00")
		_, err = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Verify lot before spin-off
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, parentSec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("expected 1 lot, got %d", len(lots))
		}
		if !lots[0].CostPerShare.Equal(types.MustNewMoney("100")) {
			t.Fatalf("pre-spinoff cost_per_share = %s, want 100", lots[0].CostPerShare.String())
		}

		// Apply spin-off: 80% allocation to parent, 20% to spin-off
		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		_, err = env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		// Parent lot cost should be reduced: $100 × 0.80 = $80
		parentLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, parentSec.ID, false)
		if len(parentLots) != 1 {
			t.Fatalf("expected 1 parent lot, got %d", len(parentLots))
		}
		if !parentLots[0].CostPerShare.Equal(types.MustNewMoney("80")) {
			t.Errorf("parent lot cost_per_share = %s, want 80", parentLots[0].CostPerShare.String())
		}
		// Parent shares unchanged
		if !parentLots[0].Shares.Equal(types.MustNewQuantity("100")) {
			t.Errorf("parent lot shares = %s, want 100", parentLots[0].Shares.String())
		}
	})

	t.Run("multiple lots each reduced by allocation percentage", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date1 := types.NewDate(2024, time.January, 15)
		date2 := types.NewDate(2024, time.March, 1)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date1, types.MustNewMoney("30000.00"), "")

		// Lot 1: 10 shares at $100
		total1 := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date1, types.MustNewQuantity("10"), &total1, nil, types.ZeroMoney, "")

		// Lot 2: 20 shares at $150
		total2 := types.MustNewMoney("3000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date2, types.MustNewQuantity("20"), &total2, nil, types.ZeroMoney, "")

		// 80% parent allocation
		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("10.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		parentLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, parentSec.ID, false)
		if len(parentLots) != 2 {
			t.Fatalf("expected 2 parent lots, got %d", len(parentLots))
		}
		// Lot 1: $100 × 0.80 = $80
		if !parentLots[0].CostPerShare.Equal(types.MustNewMoney("80")) {
			t.Errorf("lot 1 cost_per_share = %s, want 80", parentLots[0].CostPerShare.String())
		}
		// Lot 2: $150 × 0.80 = $120
		if !parentLots[1].CostPerShare.Equal(types.MustNewMoney("120")) {
			t.Errorf("lot 2 cost_per_share = %s, want 120", parentLots[1].CostPerShare.String())
		}
	})
}

// =============================================================================
// SM-158: Spin-off — create spin-off lots
// =============================================================================

func TestCorporateActionService_SpinOff_CreateLots(t *testing.T) {
	t.Run("spin-off lots created with correct shares and cost", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")

		// Buy 100 shares at $100
		total := types.MustNewMoney("10000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		// Spin-off: 0.5 share ratio, 80/20 allocation, price $25
		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		// Spin-off lot: shares = 100 × 0.5 = 50
		spinOffLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, spinOffSec.ID, false)
		if len(spinOffLots) != 1 {
			t.Fatalf("expected 1 spin-off lot, got %d", len(spinOffLots))
		}
		if !spinOffLots[0].Shares.Equal(types.MustNewQuantity("50")) {
			t.Errorf("spin-off lot shares = %s, want 50", spinOffLots[0].Shares.String())
		}

		// Cost basis allocated to spin-off: $100 × 0.20 × 100 shares = $2000
		// Cost per share: $2000 / 50 = $40
		if !spinOffLots[0].CostPerShare.Equal(types.MustNewMoney("40")) {
			t.Errorf("spin-off lot cost_per_share = %s, want 40", spinOffLots[0].CostPerShare.String())
		}

		// Total cost basis preserved: parent $8000 + spinoff $2000 = $10000
		parentLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, parentSec.ID, false)
		totalCostBasis := parentLots[0].CostBasis().Add(spinOffLots[0].CostBasis())
		if !totalCostBasis.Equal(types.MustNewMoney("10000")) {
			t.Errorf("total cost basis = %s, want 10000", totalCostBasis.String())
		}
	})

	t.Run("spin-off preserves purchase date from parent lot", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		purchaseDate := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, purchaseDate, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, purchaseDate, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("10.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		spinOffLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, spinOffSec.ID, false)
		if len(spinOffLots) != 1 {
			t.Fatalf("expected 1 spin-off lot, got %d", len(spinOffLots))
		}
		if !spinOffLots[0].PurchaseDate.Equal(purchaseDate) {
			t.Errorf("spin-off lot purchase_date = %s, want %s", spinOffLots[0].PurchaseDate.String(), purchaseDate.String())
		}
	})

	t.Run("spin-off across multiple accounts", func(t *testing.T) {
		env := createCATestEnv(t)

		acct1 := createLotTrackingAccount(t, env.accountRepo, "Brokerage1")
		acct2 := createLotTrackingAccount(t, env.accountRepo, "Brokerage2")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		for _, acct := range []*account.Account{acct1, acct2} {
			_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
			total := types.MustNewMoney("1000.00")
			_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		}

		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("10.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		for _, acct := range []*account.Account{acct1, acct2} {
			spinOffLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, spinOffSec.ID, false)
			if len(spinOffLots) != 1 {
				t.Errorf("account %s: expected 1 spin-off lot, got %d", acct.Name, len(spinOffLots))
			}
			if !spinOffLots[0].Shares.Equal(types.MustNewQuantity("10")) {
				t.Errorf("account %s: spin-off shares = %s, want 10", acct.Name, spinOffLots[0].Shares.String())
			}
		}
	})
}

// =============================================================================
// SM-159: Spin-off — fractional shares handling
// =============================================================================

func TestCorporateActionService_SpinOff_FractionalShares(t *testing.T) {
	t.Run("fractional shares rounded down with cash-in-lieu", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")

		// Buy 10 shares at $100
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		// Share ratio 0.33 → 10 × 0.33 = 3.3 → 3 whole + 0.3 fractional
		params := SpinOffParams{ShareRatio: 0.33, ParentAllocationPct: 80}
		spinOffPrice := types.MustNewMoney("25.00")
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, spinOffPrice)
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		// Should get 3 whole shares (floor of 3.3)
		spinOffLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, spinOffSec.ID, false)
		if len(spinOffLots) != 1 {
			t.Fatalf("expected 1 spin-off lot, got %d", len(spinOffLots))
		}
		if !spinOffLots[0].Shares.Equal(types.MustNewQuantity("3")) {
			t.Errorf("spin-off shares = %s, want 3", spinOffLots[0].Shares.String())
		}

		// Cash-in-lieu: 0.3 shares × $25 = $7.50
		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		expectedCash := cashBefore.Add(types.MustNewMoney("7.5"))
		if !cashAfter.Equal(expectedCash) {
			t.Errorf("cash balance = %s, want %s (cash-in-lieu for fractional shares)", cashAfter.String(), expectedCash.String())
		}
	})

	t.Run("no cash-in-lieu when shares are whole", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		// 10 × 0.5 = 5.0 exactly — no fractional
		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		if !cashAfter.Equal(cashBefore) {
			t.Errorf("cash balance changed: before %s, after %s (no fractional expected)", cashBefore.String(), cashAfter.String())
		}
	})
}

// =============================================================================
// SM-160: Spin-off — non-lot-tracking accounts (positions)
// =============================================================================

func TestCorporateActionService_SpinOff_Positions(t *testing.T) {
	t.Run("parent position cost adjusted and spin-off position created", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")
		total := types.MustNewMoney("10000.00") // 100 shares at $100
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, "")

		// Verify position before spin-off
		pos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, parentSec.ID)
		costBasisBefore := pos.CostBasis()

		// 80% parent, 20% spin-off, 0.5 share ratio
		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		// Parent position: avg cost = $100 × 0.80 = $80, shares unchanged
		parentPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, parentSec.ID)
		if !parentPos.AverageCostPerShare.Equal(types.MustNewMoney("80")) {
			t.Errorf("parent avg cost = %s, want 80", parentPos.AverageCostPerShare.String())
		}
		if !parentPos.Shares.Equal(types.MustNewQuantity("100")) {
			t.Errorf("parent shares = %s, want 100", parentPos.Shares.String())
		}

		// Spin-off position: 100 × 0.5 = 50 shares
		// Cost basis: $100 × 0.20 × 100 = $2000
		// Cost per share: $2000 / 50 = $40
		spinOffPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, spinOffSec.ID)
		if !spinOffPos.Shares.Equal(types.MustNewQuantity("50")) {
			t.Errorf("spin-off shares = %s, want 50", spinOffPos.Shares.String())
		}
		if !spinOffPos.AverageCostPerShare.Equal(types.MustNewMoney("40")) {
			t.Errorf("spin-off avg cost = %s, want 40", spinOffPos.AverageCostPerShare.String())
		}

		// Total cost basis preserved
		totalCostBasis := parentPos.CostBasis().Add(spinOffPos.CostBasis())
		if !totalCostBasis.Equal(costBasisBefore) {
			t.Errorf("total cost basis = %s, want %s", totalCostBasis.String(), costBasisBefore.String())
		}
	})

	t.Run("non-lot position with fractional shares gets cash-in-lieu", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")
		total := types.MustNewMoney("1000.00") // 10 shares at $100
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		cashBefore, _ := env.invSvc.GetCashBalance(acct.ID)

		// 10 × 0.33 = 3.3 → 3 whole + 0.3 fractional
		params := SpinOffParams{ShareRatio: 0.33, ParentAllocationPct: 80}
		spinOffPrice := types.MustNewMoney("25.00")
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, spinOffPrice)
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		spinOffPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, spinOffSec.ID)
		if !spinOffPos.Shares.Equal(types.MustNewQuantity("3")) {
			t.Errorf("spin-off shares = %s, want 3", spinOffPos.Shares.String())
		}

		// Cash-in-lieu: 0.3 × $25 = $7.50
		cashAfter, _ := env.invSvc.GetCashBalance(acct.ID)
		expectedCash := cashBefore.Add(types.MustNewMoney("7.5"))
		if !cashAfter.Equal(expectedCash) {
			t.Errorf("cash balance = %s, want %s", cashAfter.String(), expectedCash.String())
		}
	})

	t.Run("spin-off across multiple non-lot accounts", func(t *testing.T) {
		env := createCATestEnv(t)

		acct1 := createInvAccount(t, env.accountRepo, "Brokerage1")
		acct2 := createInvAccount(t, env.accountRepo, "Brokerage2")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		for _, acct := range []*account.Account{acct1, acct2} {
			_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
			total := types.MustNewMoney("500.00")
			_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
		}

		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("10.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		for _, acct := range []*account.Account{acct1, acct2} {
			spinOffPos, _ := env.positionRepo.GetByAccountAndSecurity(acct.ID, spinOffSec.ID)
			if !spinOffPos.Shares.Equal(types.MustNewQuantity("5")) {
				t.Errorf("account %s: spin-off shares = %s, want 5", acct.Name, spinOffPos.Shares.String())
			}
		}
	})
}

// =============================================================================
// SM-161: Spin-off — price record for spin-off security
// =============================================================================

func TestCorporateActionService_SpinOff_PriceRecord(t *testing.T) {
	t.Run("price record created for spin-off security on spin-off date", func(t *testing.T) {
		env := createCATestEnv(t)

		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		date := types.NewDate(2024, time.January, 15)
		spinOffDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("1000.00")
		_, _ = env.invSvc.Buy(acct.ID, parentSec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")

		spinOffPrice := types.MustNewMoney("25.00")
		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, spinOffPrice)
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		// Verify price record exists for spin-off security
		p, err := env.priceRepo.GetBySecurityAndDate(spinOffSec.ID, spinOffDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p == nil {
			t.Fatal("expected price record for spin-off security")
		}
		if !p.Price.Equal(spinOffPrice) {
			t.Errorf("spin-off price = %s, want %s", p.Price.String(), spinOffPrice.String())
		}
	})

	t.Run("price record created even with no holdings", func(t *testing.T) {
		env := createCATestEnv(t)

		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		spinOffDate := types.NewDate(2024, time.June, 1)

		spinOffPrice := types.MustNewMoney("25.00")
		params := SpinOffParams{ShareRatio: 1.0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, spinOffPrice)
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		p, err := env.priceRepo.GetBySecurityAndDate(spinOffSec.ID, spinOffDate)
		if err != nil {
			t.Fatalf("GetBySecurityAndDate() error = %v", err)
		}
		if p == nil {
			t.Fatal("expected price record even with no holdings")
		}
	})
}

// =============================================================================
// SM-162: Spin-off — audit log
// =============================================================================

func TestCorporateActionService_SpinOff_AuditLog(t *testing.T) {
	t.Run("spin-off creates corporate action record", func(t *testing.T) {
		env := createCATestEnv(t)

		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		spinOffDate := types.NewDate(2024, time.June, 15)

		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		ca, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() error = %v", err)
		}

		if ca == nil {
			t.Fatal("SpinOff() returned nil corporate action")
		}
		if ca.ActionType != ActionTypeSpinOff {
			t.Errorf("action_type = %s, want %s", ca.ActionType, ActionTypeSpinOff)
		}
		if ca.SecurityID != parentSec.ID {
			t.Error("security_id should be parent security")
		}
		if !ca.TargetSecurityID.Valid || ca.TargetSecurityID.ID != spinOffSec.ID {
			t.Error("target_security_id should be set to spin-off security")
		}
		if !ca.ActionDate.Equal(spinOffDate) {
			t.Error("action_date mismatch")
		}

		// Verify parameters deserialize correctly
		parsedParams, err := ParseSpinOffParams(ca.Parameters)
		if err != nil {
			t.Fatalf("ParseSpinOffParams() error = %v", err)
		}
		if parsedParams.ShareRatio != 0.5 {
			t.Errorf("share_ratio = %f, want 0.5", parsedParams.ShareRatio)
		}
		if parsedParams.ParentAllocationPct != 80 {
			t.Errorf("parent_allocation_pct = %f, want 80", parsedParams.ParentAllocationPct)
		}
	})

	t.Run("audit log persisted and queryable", func(t *testing.T) {
		env := createCATestEnv(t)

		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		spinOffDate := types.NewDate(2024, time.June, 15)

		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		ca, _ := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))

		// Verify persisted
		retrieved, err := env.caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ActionType != ActionTypeSpinOff {
			t.Errorf("persisted action_type = %s, want %s", retrieved.ActionType, ActionTypeSpinOff)
		}

		// Queryable by parent security
		actions, err := env.caRepo.ListBySecurity(parentSec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(parent) error = %v", err)
		}
		if len(actions) != 1 {
			t.Errorf("expected 1 action for parent, got %d", len(actions))
		}

		// Queryable by spin-off security
		actions, err = env.caRepo.ListBySecurity(spinOffSec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(spinoff) error = %v", err)
		}
		if len(actions) != 1 {
			t.Errorf("expected 1 action for spin-off, got %d", len(actions))
		}
	})
}

// =============================================================================
// Spin-off validation
// =============================================================================

func TestCorporateActionService_SpinOff_Validation(t *testing.T) {
	t.Run("invalid params rejected", func(t *testing.T) {
		env := createCATestEnv(t)

		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		spinOffDate := types.NewDate(2024, time.June, 1)

		// Zero share ratio
		params := SpinOffParams{ShareRatio: 0, ParentAllocationPct: 80}
		_, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err == nil {
			t.Error("SpinOff() with zero share ratio should error")
		}

		// Negative share ratio
		params = SpinOffParams{ShareRatio: -1.0, ParentAllocationPct: 80}
		_, err = env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err == nil {
			t.Error("SpinOff() with negative share ratio should error")
		}

		// Allocation at 0%
		params = SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 0}
		_, err = env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err == nil {
			t.Error("SpinOff() with 0% allocation should error")
		}

		// Allocation at 100%
		params = SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 100}
		_, err = env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err == nil {
			t.Error("SpinOff() with 100% allocation should error")
		}

		// Zero spin-off price
		params = SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		_, err = env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.ZeroMoney)
		if err == nil {
			t.Error("SpinOff() with zero price should error")
		}
	})

	t.Run("spin-off with no holdings succeeds", func(t *testing.T) {
		env := createCATestEnv(t)

		parentSec := createSec(t, env.secRepo, "PARENT")
		spinOffSec := createSec(t, env.secRepo, "SPINOFF")
		spinOffDate := types.NewDate(2024, time.June, 1)

		params := SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}
		ca, err := env.caSvc.SpinOff(parentSec.ID, spinOffSec.ID, spinOffDate, params, types.MustNewMoney("25.00"))
		if err != nil {
			t.Fatalf("SpinOff() with no holdings error = %v", err)
		}
		if ca == nil {
			t.Error("SpinOff() should return corporate action even with no holdings")
		}
	})
}

// =============================================================================
// DeleteAction (reverse a corporate action)
// =============================================================================

func TestCorporateActionService_DeleteAction_ReverseSplit(t *testing.T) {
	t.Run("undoes a 2:1 split: lots and prices restored", func(t *testing.T) {
		env := createCATestEnv(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		buyDate := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)

		_, _ = env.invSvc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), "")
		total := types.MustNewMoney("500.00")
		if _, err := env.invSvc.Buy(acct.ID, sec.ID, buyDate, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		ca, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1})
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}
		// Sanity: post-split lot shows 10 shares @ $50.
		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 || lots[0].Shares.String() != "10" {
			t.Fatalf("post-split lot shares = %v, want 10", lots)
		}

		if err := env.caSvc.DeleteAction(ca.ID); err != nil {
			t.Fatalf("DeleteAction() error = %v", err)
		}

		lots, _ = env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("expected 1 lot after reversal, got %d", len(lots))
		}
		if lots[0].Shares.String() != "5" {
			t.Errorf("post-reverse shares = %q, want 5", lots[0].Shares.String())
		}
		if lots[0].CostPerShare.String() != "100" {
			t.Errorf("post-reverse cost = %q, want 100", lots[0].CostPerShare.String())
		}

		// Audit row gone
		if _, err := env.caRepo.GetByID(ca.ID); err == nil {
			t.Error("expected GetByID to return NotFound after deletion")
		}
	})

	t.Run("refuses when a later transaction exists on the security", func(t *testing.T) {
		env := createCATestEnv(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		buyDate := types.NewDate(2024, time.January, 15)
		splitDate := types.NewDate(2024, time.June, 1)
		postSplitBuy := types.NewDate(2024, time.July, 10)

		_, _ = env.invSvc.Deposit(acct.ID, buyDate, types.MustNewMoney("20000.00"), "")
		total := types.MustNewMoney("500.00")
		_, _ = env.invSvc.Buy(acct.ID, sec.ID, buyDate, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")

		ca, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1})
		if err != nil {
			t.Fatalf("Split() error = %v", err)
		}

		// A post-split buy adds 3 more shares at the new price.
		postTotal := types.MustNewMoney("150.00")
		if _, err := env.invSvc.Buy(acct.ID, sec.ID, postSplitBuy, types.MustNewQuantity("3"), &postTotal, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("post-split Buy() error = %v", err)
		}

		err = env.caSvc.DeleteAction(ca.ID)
		if err == nil {
			t.Fatal("expected DeleteAction to refuse with downstream events")
		}
		var dse *DownstreamEventsError
		if !errorsAs(err, &dse) {
			t.Fatalf("expected *DownstreamEventsError, got %T: %v", err, err)
		}
		if dse.BlockerTicker != "AAPL" {
			t.Errorf("BlockerTicker = %q, want AAPL", dse.BlockerTicker)
		}
	})

	t.Run("merger is not yet reversible", func(t *testing.T) {
		env := createCATestEnv(t)
		sec := createSec(t, env.secRepo, "OLD")
		target := createSec(t, env.secRepo, "NEW")
		date := types.NewDate(2024, time.June, 1)

		ca, err := env.caSvc.Merger(sec.ID, target.ID, date, MergerParams{ExchangeRatio: 1.0})
		if err != nil {
			t.Fatalf("Merger() error = %v", err)
		}
		err = env.caSvc.DeleteAction(ca.ID)
		if err == nil {
			t.Fatal("expected UnsupportedReversalError for merger")
		}
		var ure *UnsupportedReversalError
		if !errorsAs(err, &ure) {
			t.Fatalf("expected *UnsupportedReversalError, got %T: %v", err, err)
		}
	})
}

// errorsAs is a tiny shim so the test file does not need to import "errors"
// solely for one call.
func errorsAs(err error, target any) bool {
	for err != nil {
		switch tgt := target.(type) {
		case **DownstreamEventsError:
			if dse, ok := err.(*DownstreamEventsError); ok {
				*tgt = dse
				return true
			}
		case **UnsupportedReversalError:
			if ure, ok := err.(*UnsupportedReversalError); ok {
				*tgt = ure
				return true
			}
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}
