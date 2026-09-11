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
		r, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(365), 1100}})
		if !ok {
			t.Fatal("solveXIRR() ok = false")
		}
		if math.Abs(r-0.10007) > 1e-4 {
			t.Errorf("r = %.6f, want ≈ 0.10007", r)
		}
	})
	t.Run("loss", func(t *testing.T) {
		r, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(365), 500}})
		if !ok || r >= 0 {
			t.Errorf("r, ok = %.4f, %v; want negative root", r, ok)
		}
	})
	t.Run("same-day flows undefined", func(t *testing.T) {
		if _, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0, 1100}}); ok {
			t.Error("solveXIRR() ok = true for zero span")
		}
	})
	t.Run("no sign change undefined", func(t *testing.T) {
		if _, ok := solveXIRR([]cashFlow{{d0, -1000}, {d0.AddDays(30), -1000}}); ok {
			t.Error("solveXIRR() ok = true with no positive flow")
		}
	})
}
