package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// TR-001: New total-return fields default to zero values when callers pass
// ValuationOptions{}. Existing valuation semantics (CashBalance, MarketValue,
// TotalValue, TotalCostBasis, TotalGainLoss, TotalGainPct, per-holding values)
// remain unchanged.
func TestValuation_NewFieldsZeroValue_BackCompat(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price error = %v", err)
	}

	asOf := types.NewDate(2024, time.March, 20)

	t.Run("GetAccountValuation new fields zero", func(t *testing.T) {
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		if !val.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero, got %s", val.RealizedGain.String())
		}
		if !val.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero, got %s", val.DividendsReceived.String())
		}
		if !val.InterestReceived.IsZero() {
			t.Errorf("Expected InterestReceived to be zero, got %s", val.InterestReceived.String())
		}
		if !val.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero, got %s", val.FeesPaid.String())
		}
		if !val.TotalCostDeployed.IsZero() {
			t.Errorf("Expected TotalCostDeployed to be zero, got %s", val.TotalCostDeployed.String())
		}
		if !val.TotalReturn.IsZero() {
			t.Errorf("Expected TotalReturn to be zero, got %s", val.TotalReturn.String())
		}
		if val.TotalReturnPct != nil {
			t.Errorf("Expected TotalReturnPct to be nil, got %v", *val.TotalReturnPct)
		}
		if val.HasClosedPositions {
			t.Errorf("Expected HasClosedPositions to be false, got true")
		}

		// Existing behavior must be unchanged.
		if val.CashBalance.String() != "4000" {
			t.Errorf("Expected CashBalance '4000', got %q", val.CashBalance.String())
		}
		if val.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", val.MarketValue.String())
		}
		if val.TotalCostBasis.String() != "1000" {
			t.Errorf("Expected TotalCostBasis '1000', got %q", val.TotalCostBasis.String())
		}
		if val.TotalGainLoss.String() != "200" {
			t.Errorf("Expected TotalGainLoss '200', got %q", val.TotalGainLoss.String())
		}
	})

	t.Run("GetHoldings new fields zero", func(t *testing.T) {
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}
		h := holdings[0]

		if !h.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero, got %s", h.RealizedGain.String())
		}
		if !h.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero, got %s", h.DividendsReceived.String())
		}
		if !h.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero, got %s", h.FeesPaid.String())
		}
		if !h.TotalCostDeployed.IsZero() {
			t.Errorf("Expected TotalCostDeployed to be zero, got %s", h.TotalCostDeployed.String())
		}
		if !h.TotalReturn.IsZero() {
			t.Errorf("Expected TotalReturn to be zero, got %s", h.TotalReturn.String())
		}
		if h.TotalReturnPct != nil {
			t.Errorf("Expected TotalReturnPct to be nil, got %v", *h.TotalReturnPct)
		}
		if h.IsClosed {
			t.Errorf("Expected IsClosed to be false, got true")
		}
		if h.RealizedGainUnavailable {
			t.Errorf("Expected RealizedGainUnavailable to be false, got true")
		}

		// Existing per-holding values unchanged.
		if h.Shares.String() != "10" {
			t.Errorf("Expected Shares '10', got %q", h.Shares.String())
		}
		if h.CostBasis.String() != "1000" {
			t.Errorf("Expected CostBasis '1000', got %q", h.CostBasis.String())
		}
		if h.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", h.MarketValue.String())
		}
		if h.GainLoss.String() != "200" {
			t.Errorf("Expected GainLoss '200', got %q", h.GainLoss.String())
		}
	})
}

// TR-002: sumDividendsForSecurity sums `dividend` transactions for a single
// (account, security) pair. Reinvested dividends and other securities must
// not contribute.

func TestSumDividendsForSecurity_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("100"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("75"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "175" {
		t.Errorf("Expected total dividends '175', got %q", got.String())
	}

	other := createSec(t, env.secRepo, "MSFT")
	got, err = env.svc.sumDividendsForSecurity(acct.ID, other.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for security with no dividends, got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresReinvest(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	reinvestTotal := types.MustNewMoney("110")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date, types.MustNewQuantity("1"), &reinvestTotal, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected only cash dividend '50' (reinvest excluded), got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresOtherSecurities(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, aapl.ID, date, types.MustNewMoney("80"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, msft.ID, date, types.MustNewMoney("40"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, aapl.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "80" {
		t.Errorf("Expected AAPL-only dividends '80', got %q", got.String())
	}
}

// TR-003: sumInterestForAccount sums `interest` transactions for an investment
// account. No security filter — interest is paid on the cash sweep, not a
// holding. Interest in sibling accounts must not leak in.

func TestSumInterestForAccount_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("12.50"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("7.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("30.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected total interest '50', got %q", got.String())
	}

	empty := createInvAccount(t, env.accountRepo, "Empty")
	got, err = env.svc.sumInterestForAccount(empty.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for account with no interest, got %q", got.String())
	}
}

func TestSumInterestForAccount_OtherAccountIgnored(t *testing.T) {
	env := createFullTestService(t)
	a := createInvAccount(t, env.accountRepo, "Brokerage A")
	b := createInvAccount(t, env.accountRepo, "Brokerage B")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(a.ID, date, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(b.ID, date, types.MustNewMoney("99"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(a.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "20" {
		t.Errorf("Expected account A interest '20', got %q", got.String())
	}
}
