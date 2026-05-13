package investment

import (
	"math"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// SM-093: GetAccountValuation
// =============================================================================

func TestService_GetAccountValuation(t *testing.T) {
	t.Run("returns cash balance plus market value of holdings", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		// Deposit cash
		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy 10 shares at $100 each (total $1000)
		total := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Add current price of $120
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		// Cash: 10000 - 1000 = 9000
		if val.CashBalance.String() != "9000" {
			t.Errorf("Expected cash balance '9000', got %q", val.CashBalance.String())
		}

		// Market value: 10 shares × $120 = $1200
		if val.MarketValue.String() != "1200" {
			t.Errorf("Expected market value '1200', got %q", val.MarketValue.String())
		}

		// Total value: 9000 + 1200 = 10200
		if val.TotalValue.String() != "10200" {
			t.Errorf("Expected total value '10200', got %q", val.TotalValue.String())
		}

		// Cost basis: 10 × 100 = 1000
		if val.TotalCostBasis.String() != "1000" {
			t.Errorf("Expected total cost basis '1000', got %q", val.TotalCostBasis.String())
		}

		// Gain/loss: 1200 - 1000 = 200
		if val.TotalGainLoss.String() != "200" {
			t.Errorf("Expected total gain/loss '200', got %q", val.TotalGainLoss.String())
		}

		// Gain %: (200/1000) × 100 = 20%
		if math.Abs(val.TotalGainPct-20.0) > 0.01 {
			t.Errorf("Expected total gain pct ~20.0, got %f", val.TotalGainPct)
		}
	})

	t.Run("securities with no price use cost basis", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "NOPRICE")
		date := types.NewDate(2024, time.March, 15)

		// Deposit and buy shares — no manual price added
		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("500")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Use a date before any transaction-created price to test fallback
		// Actually, the buy auto-creates a price. Let's query as-of a date before the buy.
		asOf := types.NewDate(2024, time.March, 14)
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		// Holdings should still be shown (positions exist) but with cost-basis fallback
		// The as-of date is before the buy, so GetCashBalance recalculates from all txns
		// The position still exists in the DB, so holdings are present
		// Market value = cost basis since no price available as of that date
		for _, h := range val.Holdings {
			if h.SecurityID == sec.ID {
				if h.HasPricing {
					t.Error("Expected HasPricing=false for security with no price as of date")
				}
				// When no pricing, market value should equal cost basis
				if h.MarketValue.String() != h.CostBasis.String() {
					t.Errorf("Expected market value to equal cost basis, got market=%q cost=%q", h.MarketValue.String(), h.CostBasis.String())
				}
			}
		}
	})

	t.Run("empty account returns zero values", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Empty")

		asOf := types.NewDate(2024, time.March, 15)
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		if !val.CashBalance.IsZero() {
			t.Errorf("Expected zero cash balance, got %q", val.CashBalance.String())
		}
		if !val.MarketValue.IsZero() {
			t.Errorf("Expected zero market value, got %q", val.MarketValue.String())
		}
		if !val.TotalValue.IsZero() {
			t.Errorf("Expected zero total value, got %q", val.TotalValue.String())
		}
		if len(val.Holdings) != 0 {
			t.Errorf("Expected no holdings, got %d", len(val.Holdings))
		}
	})

	t.Run("multiple securities with mixed pricing", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Multi")
		secA := createSec(t, env.secRepo, "AAA")
		secB := createSec(t, env.secRepo, "BBB")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy AAA: 10 shares at $100
		totalA := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, secA.ID, date, types.MustNewQuantity("10"), &totalA, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy AAA error = %v", err)
		}

		// Buy BBB: 20 shares at $50
		totalB := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, secB.ID, date, types.MustNewQuantity("20"), &totalB, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy BBB error = %v", err)
		}

		// Add price for AAA only ($150)
		pA := price.NewPrice(secA.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("150"), price.SourceManual)
		if err := env.priceRepo.Create(pA); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		// Cash: 20000 - 1000 - 1000 = 18000
		if val.CashBalance.String() != "18000" {
			t.Errorf("Expected cash '18000', got %q", val.CashBalance.String())
		}

		// Market value: AAA = 10×150 = 1500, BBB = cost basis 1000 (no manual price, transaction price exists)
		// Note: Buy auto-creates a price at $50 for BBB on the buy date (March 15)
		// So BBB has pricing via transaction at $50 for March 15, which is valid as-of March 20
		// Market value: AAA(1500) + BBB(20×50=1000) = 2500
		if val.MarketValue.String() != "2500" {
			t.Errorf("Expected market value '2500', got %q", val.MarketValue.String())
		}

		if len(val.Holdings) != 2 {
			t.Errorf("Expected 2 holdings, got %d", len(val.Holdings))
		}
	})

	t.Run("lot-tracking account valuation", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotBroker")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy 5 shares at $200 (lot 1)
		total1 := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Buy 3 shares at $210 (lot 2)
		date2 := types.NewDate(2024, time.March, 18)
		total2 := types.MustNewMoney("630")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("3"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		// Add current price $220
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("220"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		// Cash: 10000 - 1000 - 630 = 8370
		if val.CashBalance.String() != "8370" {
			t.Errorf("Expected cash '8370', got %q", val.CashBalance.String())
		}

		// Total shares: 5 + 3 = 8
		// Market value: 8 × 220 = 1760
		if val.MarketValue.String() != "1760" {
			t.Errorf("Expected market value '1760', got %q", val.MarketValue.String())
		}

		// Cost basis: 1000 + 630 = 1630
		if val.TotalCostBasis.String() != "1630" {
			t.Errorf("Expected cost basis '1630', got %q", val.TotalCostBasis.String())
		}

		// Gain: 1760 - 1630 = 130
		if val.TotalGainLoss.String() != "130" {
			t.Errorf("Expected gain '130', got %q", val.TotalGainLoss.String())
		}
	})

	t.Run("rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")

		asOf := types.NewDate(2024, time.March, 15)
		_, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err == nil {
			t.Fatal("Expected error for non-investment account")
		}
	})
}

// =============================================================================
// SM-094: GetHoldings
// =============================================================================

func TestService_GetHoldings(t *testing.T) {
	t.Run("returns holdings for non-lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy 10 shares at $100
		total := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Add current price $120
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}

		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}

		h := holdings[0]
		if h.SecurityID != sec.ID {
			t.Errorf("Expected security ID %s, got %s", sec.ID, h.SecurityID)
		}
		if h.Shares.String() != "10" {
			t.Errorf("Expected shares '10', got %q", h.Shares.String())
		}
		if h.AvgCost.String() != "100" {
			t.Errorf("Expected avg cost '100', got %q", h.AvgCost.String())
		}
		if h.CurrentPrice.String() != "120" {
			t.Errorf("Expected current price '120', got %q", h.CurrentPrice.String())
		}
		if h.MarketValue.String() != "1200" {
			t.Errorf("Expected market value '1200', got %q", h.MarketValue.String())
		}
		if h.CostBasis.String() != "1000" {
			t.Errorf("Expected cost basis '1000', got %q", h.CostBasis.String())
		}
		if h.GainLoss.String() != "200" {
			t.Errorf("Expected gain/loss '200', got %q", h.GainLoss.String())
		}
		if math.Abs(h.GainPct-20.0) > 0.01 {
			t.Errorf("Expected gain pct ~20.0, got %f", h.GainPct)
		}
		if !h.HasPricing {
			t.Error("Expected HasPricing=true")
		}
	})

	t.Run("returns holdings for lot-tracking account aggregated across lots", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotBroker")
		sec := createSec(t, env.secRepo, "TSLA")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Lot 1: 10 shares at $100
		total1 := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Lot 2: 10 shares at $200
		date2 := types.NewDate(2024, time.March, 18)
		total2 := types.MustNewMoney("2000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("10"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		// Add current price $180
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("180"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}

		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}

		h := holdings[0]
		// Total shares: 10 + 10 = 20
		if h.Shares.String() != "20" {
			t.Errorf("Expected shares '20', got %q", h.Shares.String())
		}

		// Cost basis: 1000 + 2000 = 3000
		if h.CostBasis.String() != "3000" {
			t.Errorf("Expected cost basis '3000', got %q", h.CostBasis.String())
		}

		// Avg cost: 3000 / 20 = 150
		if h.AvgCost.String() != "150" {
			t.Errorf("Expected avg cost '150', got %q", h.AvgCost.String())
		}

		// Market value: 20 × 180 = 3600
		if h.MarketValue.String() != "3600" {
			t.Errorf("Expected market value '3600', got %q", h.MarketValue.String())
		}

		// Gain: 3600 - 3000 = 600
		if h.GainLoss.String() != "600" {
			t.Errorf("Expected gain/loss '600', got %q", h.GainLoss.String())
		}

		if !h.HasPricing {
			t.Error("Expected HasPricing=true")
		}
	})

	t.Run("no holdings returns empty slice", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Empty")

		asOf := types.NewDate(2024, time.March, 15)
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}

		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings, got %d", len(holdings))
		}
	})

	t.Run("rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")

		asOf := types.NewDate(2024, time.March, 15)
		_, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err == nil {
			t.Fatal("Expected error for non-investment account")
		}
	})
}

// =============================================================================
// SM-174: GetHoldings via portfolio_holdings view matches manual computation
// =============================================================================

func TestService_GetHoldingsViewMatchesManual(t *testing.T) {
	t.Run("non-lot-tracking: view results match manual computation", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ViewTest")
		sec1 := createSec(t, env.secRepo, "VTI")
		sec2 := createSec(t, env.secRepo, "BND")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("50000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy VTI: 20 shares, $4000 total
		totalVTI := types.MustNewMoney("4000")
		_, err = env.svc.Buy(acct.ID, sec1.ID, date, types.MustNewQuantity("20"), &totalVTI, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy VTI error = %v", err)
		}

		// Buy BND: 50 shares, $5000 total
		totalBND := types.MustNewMoney("5000")
		_, err = env.svc.Buy(acct.ID, sec2.ID, date, types.MustNewQuantity("50"), &totalBND, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy BND error = %v", err)
		}

		// Add prices
		p1 := price.NewPrice(sec1.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("220"), price.SourceManual)
		if err := env.priceRepo.Create(p1); err != nil {
			t.Fatalf("Create VTI price error = %v", err)
		}
		p2 := price.NewPrice(sec2.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("95"), price.SourceManual)
		if err := env.priceRepo.Create(p2); err != nil {
			t.Fatalf("Create BND price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)

		// Get holdings via view (default path since holdingsRepo is wired)
		viewHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (view) error = %v", err)
		}

		// Get holdings via manual path by temporarily disabling the view
		env.svc.holdingsRepo = nil
		manualHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (manual) error = %v", err)
		}

		if len(viewHoldings) != len(manualHoldings) {
			t.Fatalf("Holdings count mismatch: view=%d manual=%d", len(viewHoldings), len(manualHoldings))
		}

		// Build maps by security ID for comparison
		viewMap := make(map[string]Holding)
		for _, h := range viewHoldings {
			viewMap[h.SecurityID.String()] = h
		}
		manualMap := make(map[string]Holding)
		for _, h := range manualHoldings {
			manualMap[h.SecurityID.String()] = h
		}

		for secID, vh := range viewMap {
			mh, ok := manualMap[secID]
			if !ok {
				t.Errorf("Security %s in view results but not manual", secID)
				continue
			}
			if vh.Shares.String() != mh.Shares.String() {
				t.Errorf("Security %s shares mismatch: view=%q manual=%q", secID, vh.Shares.String(), mh.Shares.String())
			}
			if vh.CostBasis.String() != mh.CostBasis.String() {
				t.Errorf("Security %s cost basis mismatch: view=%q manual=%q", secID, vh.CostBasis.String(), mh.CostBasis.String())
			}
			if vh.MarketValue.String() != mh.MarketValue.String() {
				t.Errorf("Security %s market value mismatch: view=%q manual=%q", secID, vh.MarketValue.String(), mh.MarketValue.String())
			}
			if vh.GainLoss.String() != mh.GainLoss.String() {
				t.Errorf("Security %s gain/loss mismatch: view=%q manual=%q", secID, vh.GainLoss.String(), mh.GainLoss.String())
			}
			if vh.HasPricing != mh.HasPricing {
				t.Errorf("Security %s has_pricing mismatch: view=%v manual=%v", secID, vh.HasPricing, mh.HasPricing)
			}
			if math.Abs(vh.GainPct-mh.GainPct) > 0.01 {
				t.Errorf("Security %s gain pct mismatch: view=%f manual=%f", secID, vh.GainPct, mh.GainPct)
			}
		}
	})

	t.Run("lot-tracking: view results match manual computation", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotViewTest")
		sec := createSec(t, env.secRepo, "NVDA")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("50000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Lot 1: 10 shares at $800
		total1 := types.MustNewMoney("8000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Lot 2: 5 shares at $850
		date2 := types.NewDate(2024, time.March, 18)
		total2 := types.MustNewMoney("4250")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("5"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		// Add current price
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("900"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)

		// Get holdings via view
		viewHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (view) error = %v", err)
		}

		// Get holdings via manual path
		env.svc.holdingsRepo = nil
		manualHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (manual) error = %v", err)
		}

		if len(viewHoldings) != len(manualHoldings) {
			t.Fatalf("Holdings count mismatch: view=%d manual=%d", len(viewHoldings), len(manualHoldings))
		}

		if len(viewHoldings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(viewHoldings))
		}

		vh := viewHoldings[0]
		mh := manualHoldings[0]

		// Shares: 10 + 5 = 15
		if vh.Shares.String() != mh.Shares.String() {
			t.Errorf("Shares mismatch: view=%q manual=%q", vh.Shares.String(), mh.Shares.String())
		}
		if vh.Shares.String() != "15" {
			t.Errorf("Expected shares '15', got %q", vh.Shares.String())
		}

		// Cost basis: 8000 + 4250 = 12250
		if vh.CostBasis.String() != mh.CostBasis.String() {
			t.Errorf("CostBasis mismatch: view=%q manual=%q", vh.CostBasis.String(), mh.CostBasis.String())
		}
		if vh.CostBasis.String() != "12250" {
			t.Errorf("Expected cost basis '12250', got %q", vh.CostBasis.String())
		}

		// Market value: 15 × 900 = 13500
		if vh.MarketValue.String() != mh.MarketValue.String() {
			t.Errorf("MarketValue mismatch: view=%q manual=%q", vh.MarketValue.String(), mh.MarketValue.String())
		}
		if vh.MarketValue.String() != "13500" {
			t.Errorf("Expected market value '13500', got %q", vh.MarketValue.String())
		}

		// Gain/loss: 13500 - 12250 = 1250
		if vh.GainLoss.String() != mh.GainLoss.String() {
			t.Errorf("GainLoss mismatch: view=%q manual=%q", vh.GainLoss.String(), mh.GainLoss.String())
		}

		if vh.HasPricing != mh.HasPricing {
			t.Errorf("HasPricing mismatch: view=%v manual=%v", vh.HasPricing, mh.HasPricing)
		}

		if math.Abs(vh.GainPct-mh.GainPct) > 0.01 {
			t.Errorf("GainPct mismatch: view=%f manual=%f", vh.GainPct, mh.GainPct)
		}
	})

	t.Run("view path: no pricing falls back to cost basis", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "NoPriceView")
		sec := createSec(t, env.secRepo, "NOPV")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("500")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		// Query as-of date before buy to avoid transaction-created price
		asOf := types.NewDate(2024, time.March, 14)

		viewHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (view) error = %v", err)
		}

		env.svc.holdingsRepo = nil
		manualHoldings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() (manual) error = %v", err)
		}

		if len(viewHoldings) != len(manualHoldings) {
			t.Fatalf("Holdings count mismatch: view=%d manual=%d", len(viewHoldings), len(manualHoldings))
		}

		for i := range viewHoldings {
			if viewHoldings[i].HasPricing != manualHoldings[i].HasPricing {
				t.Errorf("HasPricing mismatch: view=%v manual=%v", viewHoldings[i].HasPricing, manualHoldings[i].HasPricing)
			}
			if viewHoldings[i].MarketValue.String() != manualHoldings[i].MarketValue.String() {
				t.Errorf("MarketValue mismatch: view=%q manual=%q", viewHoldings[i].MarketValue.String(), manualHoldings[i].MarketValue.String())
			}
		}
	})

	t.Run("view path: empty account returns no holdings", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "EmptyView")

		asOf := types.NewDate(2024, time.March, 15)
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}

		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings, got %d", len(holdings))
		}
	})

	t.Run("view-based GetAccountValuation matches manual computation", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ValuationView")
		sec := createSec(t, env.secRepo, "SPY")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("5000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("520"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)

		// View-based valuation
		viewVal, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() (view) error = %v", err)
		}

		// Manual-based valuation
		env.svc.holdingsRepo = nil
		manualVal, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() (manual) error = %v", err)
		}

		if viewVal.CashBalance.String() != manualVal.CashBalance.String() {
			t.Errorf("Cash balance mismatch: view=%q manual=%q", viewVal.CashBalance.String(), manualVal.CashBalance.String())
		}
		if viewVal.MarketValue.String() != manualVal.MarketValue.String() {
			t.Errorf("Market value mismatch: view=%q manual=%q", viewVal.MarketValue.String(), manualVal.MarketValue.String())
		}
		if viewVal.TotalValue.String() != manualVal.TotalValue.String() {
			t.Errorf("Total value mismatch: view=%q manual=%q", viewVal.TotalValue.String(), manualVal.TotalValue.String())
		}
		if viewVal.TotalCostBasis.String() != manualVal.TotalCostBasis.String() {
			t.Errorf("Cost basis mismatch: view=%q manual=%q", viewVal.TotalCostBasis.String(), manualVal.TotalCostBasis.String())
		}
		if viewVal.TotalGainLoss.String() != manualVal.TotalGainLoss.String() {
			t.Errorf("Gain/loss mismatch: view=%q manual=%q", viewVal.TotalGainLoss.String(), manualVal.TotalGainLoss.String())
		}
		if math.Abs(viewVal.TotalGainPct-manualVal.TotalGainPct) > 0.01 {
			t.Errorf("Gain pct mismatch: view=%f manual=%f", viewVal.TotalGainPct, manualVal.TotalGainPct)
		}
	})
}

// =============================================================================
// SM-095: GetLotDetail
// =============================================================================

func TestService_GetLotDetail(t *testing.T) {
	t.Run("returns lot details for lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotBroker")
		sec := createSec(t, env.secRepo, "AMZN")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Lot 1: 5 shares at $100 on March 15
		total1 := types.MustNewMoney("500")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Lot 2: 3 shares at $110 on March 18
		date2 := types.NewDate(2024, time.March, 18)
		total2 := types.MustNewMoney("330")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("3"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		// Add current price $120
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		details, err := env.svc.GetLotDetail(acct.ID, sec.ID, asOf)
		if err != nil {
			t.Fatalf("GetLotDetail() error = %v", err)
		}

		if len(details) != 2 {
			t.Fatalf("Expected 2 lots, got %d", len(details))
		}

		// Lot 1: 5 shares at $100
		lot1 := details[0]
		if lot1.Shares.String() != "5" {
			t.Errorf("Lot 1: expected shares '5', got %q", lot1.Shares.String())
		}
		if lot1.CostPerShare.String() != "100" {
			t.Errorf("Lot 1: expected cost/share '100', got %q", lot1.CostPerShare.String())
		}
		if lot1.CostBasis.String() != "500" {
			t.Errorf("Lot 1: expected cost basis '500', got %q", lot1.CostBasis.String())
		}
		// Current value: 5 × 120 = 600
		if lot1.CurrentValue.String() != "600" {
			t.Errorf("Lot 1: expected current value '600', got %q", lot1.CurrentValue.String())
		}
		// Gain: 600 - 500 = 100
		if lot1.GainLoss.String() != "100" {
			t.Errorf("Lot 1: expected gain '100', got %q", lot1.GainLoss.String())
		}
		// Gain %: 20%
		if math.Abs(lot1.GainPct-20.0) > 0.01 {
			t.Errorf("Lot 1: expected gain pct ~20.0, got %f", lot1.GainPct)
		}

		// Lot 2: 3 shares at $110
		lot2 := details[1]
		if lot2.Shares.String() != "3" {
			t.Errorf("Lot 2: expected shares '3', got %q", lot2.Shares.String())
		}
		if lot2.CostPerShare.String() != "110" {
			t.Errorf("Lot 2: expected cost/share '110', got %q", lot2.CostPerShare.String())
		}
		// Current value: 3 × 120 = 360
		if lot2.CurrentValue.String() != "360" {
			t.Errorf("Lot 2: expected current value '360', got %q", lot2.CurrentValue.String())
		}
		// Cost basis: 3 × 110 = 330
		if lot2.CostBasis.String() != "330" {
			t.Errorf("Lot 2: expected cost basis '330', got %q", lot2.CostBasis.String())
		}
		// Gain: 360 - 330 = 30
		if lot2.GainLoss.String() != "30" {
			t.Errorf("Lot 2: expected gain '30', got %q", lot2.GainLoss.String())
		}
	})

	t.Run("excludes closed lots", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotBroker2")
		sec := createSec(t, env.secRepo, "META")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Buy 5 shares (lot 1)
		total1 := types.MustNewMoney("500")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Buy 3 shares (lot 2)
		date2 := types.NewDate(2024, time.March, 16)
		total2 := types.MustNewMoney("300")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("3"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		// Sell all 5 shares from lot 1 (closes it)
		lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil {
			t.Fatalf("ListByAccountAndSecurity error = %v", err)
		}

		sellDate := types.NewDate(2024, time.March, 17)
		sellTotal := types.MustNewMoney("600")
		_, err = env.svc.Sell(acct.ID, sec.ID, sellDate, types.MustNewQuantity("5"), &sellTotal, nil, types.ZeroMoney, "",
			[]SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}})
		if err != nil {
			t.Fatalf("Sell error = %v", err)
		}

		// Add price
		p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("110"), price.SourceManual)
		if err := env.priceRepo.Create(p); err != nil {
			t.Fatalf("Create price error = %v", err)
		}

		asOf := types.NewDate(2024, time.March, 20)
		details, err := env.svc.GetLotDetail(acct.ID, sec.ID, asOf)
		if err != nil {
			t.Fatalf("GetLotDetail() error = %v", err)
		}

		// Only lot 2 should remain (lot 1 was fully sold)
		if len(details) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(details))
		}
		if details[0].Shares.String() != "3" {
			t.Errorf("Expected remaining lot shares '3', got %q", details[0].Shares.String())
		}
	})

	t.Run("rejects non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")
		sec := createSec(t, env.secRepo, "NVDA")

		asOf := types.NewDate(2024, time.March, 15)
		_, err := env.svc.GetLotDetail(acct.ID, sec.ID, asOf)
		if err == nil {
			t.Fatal("Expected error for non-investment account")
		}
	})

	t.Run("non-lot-tracking account returns error", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "NonLot")
		sec := createSec(t, env.secRepo, "AMD")

		asOf := types.NewDate(2024, time.March, 15)
		_, err := env.svc.GetLotDetail(acct.ID, sec.ID, asOf)
		if err == nil {
			t.Fatal("Expected error for non-lot-tracking account")
		}
	})
}
