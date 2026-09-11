package investment

import (
	"math"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

func addPrice(t *testing.T, env *testServiceEnv, secID types.ID, date types.Date, p string) {
	t.Helper()
	if err := env.priceRepo.Create(price.NewPrice(secID, date, types.MustNewMoney(p), price.SourceManual)); err != nil {
		t.Fatalf("Create price error = %v", err)
	}
}

func buy(t *testing.T, env *testServiceEnv, acctID, secID types.ID, date types.Date, shares, total string) {
	t.Helper()
	amt := types.MustNewMoney(total)
	if _, err := env.svc.Buy(acctID, secID, date, types.MustNewQuantity(shares), &amt, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
}

func deposit(t *testing.T, env *testServiceEnv, acctID types.ID, date types.Date, amount string) {
	t.Helper()
	if _, err := env.svc.Deposit(acctID, date, types.MustNewMoney(amount), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
}

func requirePct(t *testing.T, name string, got *float64, want, tol float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %.4f", name, want)
	}
	if math.Abs(*got-want) > tol {
		t.Errorf("%s = %.4f, want %.4f (±%.4f)", name, *got, want, tol)
	}
}

// A single deposit invested at once and held one year: IRR, cumulative TWR
// and annualized TWR all equal the price change.
func TestPerformance_SingleDepositOneYear(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "FXAIX")
	start := types.NewDate(2024, time.January, 1)
	end := types.NewDate(2025, time.January, 1) // 366 days: leap year

	deposit(t, env, acct.ID, start, "10000")
	buy(t, env, acct.ID, sec.ID, start, "100", "10000")
	addPrice(t, env, sec.ID, end, "110")

	val, err := env.valSvc.GetAccountValuation(acct.ID, end, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// 366 days on a 365.25 basis: (1.1)^(365.25/366) − 1 ≈ 9.98 %.
	requirePct(t, "IRR", val.MoneyWeightedReturnPct, 9.98, 0.05)
	requirePct(t, "TWR", val.TimeWeightedReturnPct, 10.0, 0.001)
	requirePct(t, "TWR annualized", val.TimeWeightedReturnAnnualizedPct, 9.98, 0.05)
}

// Buying shares with cash already in the account is not an external flow,
// so neither IRR nor TWR moves — the very case TotalReturnPct gets wrong.
func TestPerformance_BuyFromCashDoesNotChangeReturns(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "401k")
	sec := createSec(t, env.secRepo, "FXAIX")
	start := types.NewDate(2024, time.January, 1)
	end := types.NewDate(2024, time.July, 1)

	deposit(t, env, acct.ID, start, "20000")
	buy(t, env, acct.ID, sec.ID, start, "100", "10000")
	addPrice(t, env, sec.ID, end, "120")

	before, err := env.valSvc.GetAccountValuation(acct.ID, end, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// Put the idle cash to work at today's price.
	buy(t, env, acct.ID, sec.ID, end, "50", "6000")
	after, err := env.valSvc.GetAccountValuation(acct.ID, end, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}

	requirePct(t, "IRR before", before.MoneyWeightedReturnPct, *after.MoneyWeightedReturnPct, 1e-6)
	requirePct(t, "TWR before", before.TimeWeightedReturnPct, *after.TimeWeightedReturnPct, 1e-6)
	// Half a year: no annualized TWR yet.
	if after.TimeWeightedReturnAnnualizedPct != nil {
		t.Errorf("TWR annualized = %v, want nil for a ledger under one year", *after.TimeWeightedReturnAnnualizedPct)
	}
	// Sanity: the total-return percent DID change, which is why IRR/TWR exist.
	if *before.TotalReturnPct == *after.TotalReturnPct {
		t.Errorf("TotalReturnPct unchanged at %.4f; expected the buy to dilute it", *after.TotalReturnPct)
	}
}

// A second deposit right before a fall: TWR ignores the timing and shows the
// holdings' own (flat) round trip, IRR shows the investor's loss.
func TestPerformance_TimingSeparatesIRRFromTWR(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	d0 := types.NewDate(2024, time.January, 1)
	d1 := types.NewDate(2024, time.July, 1)
	d2 := types.NewDate(2024, time.December, 31)

	deposit(t, env, acct.ID, d0, "1000")
	buy(t, env, acct.ID, sec.ID, d0, "10", "1000") // $100/sh
	deposit(t, env, acct.ID, d1, "1000")
	buy(t, env, acct.ID, sec.ID, d1, "5", "1000") // $200/sh, auto-price on d1
	addPrice(t, env, sec.ID, d2, "100")

	val, err := env.valSvc.GetAccountValuation(acct.ID, d2, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// Period 1: 1000 → 2000 (×2). Period 2: 3000 → 1500 (×0.5). Chain = 1.
	requirePct(t, "TWR", val.TimeWeightedReturnPct, 0, 0.001)
	if val.MoneyWeightedReturnPct == nil || *val.MoneyWeightedReturnPct >= 0 {
		t.Errorf("IRR = %v, want negative (paid 2000, hold 1500)", val.MoneyWeightedReturnPct)
	}
}

// A withdrawal is a positive flow to the investor and lowers the value the
// next sub-period starts from, without counting as a loss.
func TestPerformance_WithdrawalIsNotALoss(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	d0 := types.NewDate(2024, time.January, 1)
	d1 := types.NewDate(2024, time.July, 1)
	d2 := types.NewDate(2025, time.January, 1)

	deposit(t, env, acct.ID, d0, "10000")
	buy(t, env, acct.ID, sec.ID, d0, "50", "5000")
	if _, err := env.svc.Withdrawal(acct.ID, d1, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Withdrawal() error = %v", err)
	}
	addPrice(t, env, sec.ID, d2, "110")

	val, err := env.valSvc.GetAccountValuation(acct.ID, d2, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// Value 10000 → 10000 at d1 (pre-flow) → 5500 at d2 from 5000: +10 %.
	requirePct(t, "TWR", val.TimeWeightedReturnPct, 10.0, 0.001)
	if val.MoneyWeightedReturnPct == nil || *val.MoneyWeightedReturnPct <= 0 {
		t.Errorf("IRR = %v, want positive", val.MoneyWeightedReturnPct)
	}
}

// No external flow at all (shares bought on margin from a zero cash balance)
// leaves both figures undefined rather than fabricated.
func TestPerformance_NoFlowsIsNil(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	d0 := types.NewDate(2024, time.January, 1)
	buy(t, env, acct.ID, sec.ID, d0, "10", "1000")
	addPrice(t, env, sec.ID, d0.AddMonths(6), "150")

	val, err := env.valSvc.GetAccountValuation(acct.ID, d0.AddMonths(6), ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	if val.MoneyWeightedReturnPct != nil {
		t.Errorf("IRR = %v, want nil with no external flows", *val.MoneyWeightedReturnPct)
	}
	if val.TimeWeightedReturnPct != nil {
		t.Errorf("TWR = %v, want nil: opening value is zero (shares − cash)", *val.TimeWeightedReturnPct)
	}
}

func TestSolveXIRR(t *testing.T) {
	d0 := types.NewDate(2024, time.January, 1)
	t.Run("ten percent over exactly a year basis", func(t *testing.T) {
		// 365.25 days is not a calendar date; use 365 days and expect
		// (1.1)^(365.25/365) − 1 ≈ 10.007 %.
		x, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(365), 1100}})
		if !ok {
			t.Fatal("solveXIRR() ok = false")
		}
		if r := math.Exp(x) - 1; math.Abs(r-0.10007) > 1e-4 {
			t.Errorf("r = %.6f, want ≈ 0.10007", r)
		}
	})
	t.Run("loss", func(t *testing.T) {
		x, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(365), 500}})
		if !ok || x >= 0 {
			t.Errorf("x, ok = %.4f, %v; want negative root", x, ok)
		}
	})
	t.Run("same-day flows undefined", func(t *testing.T) {
		if _, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0, 1100}}); ok {
			t.Error("solveXIRR() ok = true for zero span")
		}
	})
	t.Run("no negative flow undefined", func(t *testing.T) {
		if _, ok := solveXIRR([]cashFlow{{d0, 1000}, {d0.AddDays(30), 1000}}); ok {
			t.Error("solveXIRR() ok = true with no negative flow")
		}
	})
}

func newCASvc(env *testServiceEnv) *CorporateActionService {
	return NewCorporateActionService(env.caRepo, env.lotRepo, env.positionRepo, env.priceRepo, env.invRepo, env.secRepo, env.db)
}

// Split processing rewrites the stored pre-split prices into post-split
// units. A checkpoint before the split must still value pre-split shares at
// the pre-split price, or the chain starts from half the true value.
func TestPerformance_SplitDoesNotDistortTWR(t *testing.T) {
	for _, tc := range []struct {
		name  string
		num   int
		den   int
		final string // post-split price worth $120 pre-split per share
	}{
		{"2:1 forward", 2, 1, "60"},
		{"1:2 reverse", 1, 2, "240"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := createFullTestService(t)
			acct := createInvAccount(t, env.accountRepo, "Brokerage")
			sec := createSec(t, env.secRepo, "NVDA")
			d0 := types.NewDate(2024, time.January, 1)
			dSplit := types.NewDate(2024, time.June, 1)
			d2 := types.NewDate(2024, time.December, 1)

			deposit(t, env, acct.ID, d0, "1000")
			buy(t, env, acct.ID, sec.ID, d0, "10", "1000") // auto-price $100 on d0
			// A flow on the split day exercises the on-date rule too.
			deposit(t, env, acct.ID, dSplit, "500")
			if _, err := newCASvc(env).Split(sec.ID, dSplit, SplitParams{Numerator: tc.num, Denominator: tc.den}); err != nil {
				t.Fatalf("Split() error = %v", err)
			}
			addPrice(t, env, sec.ID, d2, tc.final)

			val, err := env.valSvc.GetAccountValuation(acct.ID, d2, ValuationOptions{})
			if err != nil {
				t.Fatalf("GetAccountValuation() error = %v", err)
			}
			// 1000 → 1000 (flat to the split day) → 1200 + 500 cash from
			// 1500: shares gained 20 %, cash none: (1700/1500) − 1 = 13.33 %.
			requirePct(t, "TWR", val.TimeWeightedReturnPct, 13.3333, 0.01)
		})
	}
}

// Merger cash consideration is posted as a deposit but is proceeds of the
// holding: it must count as return, not as a contribution.
func TestPerformance_MergerCashIsReturnNotContribution(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	src := createSec(t, env.secRepo, "OLD")
	dst := createSec(t, env.secRepo, "NEW")
	d0 := types.NewDate(2024, time.January, 1)
	d1 := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.December, 1)

	deposit(t, env, acct.ID, d0, "1000")
	buy(t, env, acct.ID, src.ID, d0, "10", "1000")
	// 1 NEW per OLD at the same cost, plus $20/share cash.
	if _, err := newCASvc(env).Merger(src.ID, dst.ID, d1, MergerParams{ExchangeRatio: 1, CashPerShare: 20}); err != nil {
		t.Fatalf("Merger() error = %v", err)
	}
	addPrice(t, env, dst.ID, d2, "120")

	val, err := env.valSvc.GetAccountValuation(acct.ID, d2, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// 1000 in; 10 NEW × 120 + 200 cash = 1400 out: +40 % with no flow at d1.
	requirePct(t, "TWR", val.TimeWeightedReturnPct, 40.0, 0.01)
	requirePct(t, "IRR", val.MoneyWeightedReturnPct, 40.0, 0.01)
	if !val.TotalValue.Equal(types.MustNewMoney("1400")) {
		t.Errorf("TotalValue = %s, want 1400", val.TotalValue)
	}
}

// After a spin-off the parent keeps only its allocated share of cost. With
// no parent price on file the cost-basis fallback must reflect that cut, or
// the spun-off slice is counted twice (parent at full cost + child).
func TestPerformance_SpinOffParentBasisCutMatchesLiveValue(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	parent := createSec(t, env.secRepo, "PARENT")
	child := createSec(t, env.secRepo, "CHILD")
	d0 := types.NewDate(2024, time.January, 1)
	dSpin := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.December, 1)

	deposit(t, env, acct.ID, d0, "1000")
	buy(t, env, acct.ID, parent.ID, d0, "10", "1000")
	// Delete the buy's auto price so the parent is carried at cost.
	hist, err := env.priceRepo.GetPriceHistory(parent.ID, nil, nil)
	if err != nil {
		t.Fatalf("GetPriceHistory() error = %v", err)
	}
	for _, p := range hist {
		if err := env.priceRepo.Delete(p.ID); err != nil {
			t.Fatalf("Delete price error = %v", err)
		}
	}
	// 0.5 CHILD per PARENT (5 whole shares, no fraction), parent keeps 80 %.
	if _, err := newCASvc(env).SpinOff(parent.ID, child.ID, dSpin, SpinOffParams{ShareRatio: 0.5, ParentAllocationPct: 80}, types.MustNewMoney("40")); err != nil {
		t.Fatalf("SpinOff() error = %v", err)
	}

	val, err := env.valSvc.GetAccountValuation(acct.ID, d2, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}
	// Parent at 800 cost + 5 CHILD × $40 = 1000: the live book. The chain
	// from a single 1000 deposit must land on that same value.
	wantTWR := (val.TotalValue.Float64()/1000 - 1) * 100
	requirePct(t, "TWR", val.TimeWeightedReturnPct, wantTWR, 0.01)
	if !val.TotalValue.Equal(types.MustNewMoney("1000")) {
		t.Errorf("TotalValue = %s, want 1000 (800 parent cost + 200 child)", val.TotalValue)
	}
}

// Short spans and total losses: the holding-period IRR is defined and
// sensible where an annual-only solver would report nothing or nonsense.
func TestPerformance_ShortSpanAndTotalLossIRR(t *testing.T) {
	d0 := types.NewDate(2024, time.January, 1)
	t.Run("one-day ten percent gain", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		deposit(t, env, acct.ID, d0, "1000")
		buy(t, env, acct.ID, sec.ID, d0, "10", "1000")
		addPrice(t, env, sec.ID, d0.AddDays(1), "110")
		val, err := env.valSvc.GetAccountValuation(acct.ID, d0.AddDays(1), ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}
		requirePct(t, "IRR", val.MoneyWeightedReturnPct, 10.0, 0.01)
		if val.MoneyWeightedReturnAnnualizedPct != nil {
			t.Errorf("IRR annualized = %v, want nil under a year", *val.MoneyWeightedReturnAnnualizedPct)
		}
	})
	t.Run("one-day half loss", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		deposit(t, env, acct.ID, d0, "1000")
		buy(t, env, acct.ID, sec.ID, d0, "10", "1000")
		addPrice(t, env, sec.ID, d0.AddDays(1), "50")
		val, err := env.valSvc.GetAccountValuation(acct.ID, d0.AddDays(1), ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}
		requirePct(t, "IRR", val.MoneyWeightedReturnPct, -50.0, 0.01)
	})
	t.Run("year-long wipeout", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		deposit(t, env, acct.ID, d0, "1000")
		buy(t, env, acct.ID, sec.ID, d0, "10", "1000")
		addPrice(t, env, sec.ID, d0.AddYears(1), "0.0001")
		// Sell everything for the last cent: cash is now (almost) zero.
		proceeds := types.MustNewMoney("0.001")
		if _, err := env.svc.Sell(acct.ID, sec.ID, d0.AddYears(1), types.MustNewQuantity("10"), &proceeds, nil, types.ZeroMoney, "", nil); err != nil {
			t.Fatalf("Sell() error = %v", err)
		}
		val, err := env.valSvc.GetAccountValuation(acct.ID, d0.AddYears(1), ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}
		requirePct(t, "IRR", val.MoneyWeightedReturnPct, -100.0, 0.01)
		requirePct(t, "IRR annualized", val.MoneyWeightedReturnAnnualizedPct, -100.0, 0.01)
		requirePct(t, "TWR", val.TimeWeightedReturnPct, -100.0, 0.01)
	})
}

func TestSolveXIRR_LogSpace(t *testing.T) {
	d0 := types.NewDate(2024, time.January, 1)
	holding := func(x float64, days float64) float64 { return math.Exp(x*days/daysPerYear) - 1 }
	t.Run("one-day ten percent gain has a root", func(t *testing.T) {
		x, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(1), 1100}})
		if !ok {
			t.Fatal("solveXIRR() ok = false")
		}
		if got := holding(x, 1); math.Abs(got-0.10) > 1e-6 {
			t.Errorf("holding-period return = %.6f, want 0.10", got)
		}
	})
	t.Run("one-day half loss has a root", func(t *testing.T) {
		x, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(1), 500}})
		if !ok {
			t.Fatal("solveXIRR() ok = false")
		}
		if got := holding(x, 1); math.Abs(got+0.5) > 1e-6 {
			t.Errorf("holding-period return = %.6f, want -0.50", got)
		}
	})
	t.Run("zero terminal is a total loss", func(t *testing.T) {
		x, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(365), 0}})
		if !ok || !math.IsInf(x, -1) {
			t.Errorf("x, ok = %v, %v; want -Inf, true", x, ok)
		}
	})
}
