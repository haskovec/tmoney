package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// TestListIncomeOnlyTransactionPrices pins the rule used by `price cleanup`:
// a source=transaction price is a candidate only when its date carries a
// reinvest_dividend or fee_liquidation and NO buy/sell, and the fallback is the
// immediately preceding recorded price.
func TestListIncomeOnlyTransactionPrices(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")

	d1 := types.NewDate(2023, time.May, 1)    // buy → justified transaction price (the fallback)
	d2 := types.NewDate(2023, time.June, 1)   // reinvest + legacy transaction price → CANDIDATE
	d3 := types.NewDate(2023, time.July, 1)   // reinvest + buy same day → execution justifies, not a candidate
	d4 := types.NewDate(2023, time.August, 1) // reinvest + manual price → not transaction-sourced

	p100 := types.MustNewMoney("100.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"), nil, &p100, types.ZeroMoney, ""); err != nil {
		t.Fatalf("buy d1: %v", err)
	}

	// d2: reinvest creates no price now; seed the legacy transaction price by hand.
	r2 := types.MustNewMoney("0.16")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, d2, types.MustNewQuantity("0.002"), &r2, nil, ""); err != nil {
		t.Fatalf("reinvest d2: %v", err)
	}
	if err := env.priceRepo.Create(price.NewPrice(sec.ID, d2, types.MustNewMoney("80.00"), price.SourceTransaction)); err != nil {
		t.Fatalf("seed d2 price: %v", err)
	}

	// d3: a buy on the same day as a reinvest — the buy's price is justified.
	p150 := types.MustNewMoney("150.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d3, types.MustNewQuantity("5"), nil, &p150, types.ZeroMoney, ""); err != nil {
		t.Fatalf("buy d3: %v", err)
	}
	r3 := types.MustNewMoney("1.50")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, d3, types.MustNewQuantity("0.01"), &r3, nil, ""); err != nil {
		t.Fatalf("reinvest d3: %v", err)
	}

	// d4: reinvest with a MANUAL price — not transaction-sourced, so never a candidate.
	r4 := types.MustNewMoney("0.50")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, d4, types.MustNewQuantity("0.005"), &r4, nil, ""); err != nil {
		t.Fatalf("reinvest d4: %v", err)
	}
	if err := env.priceRepo.Create(price.NewPrice(sec.ID, d4, types.MustNewMoney("90.00"), price.SourceManual)); err != nil {
		t.Fatalf("seed d4 manual price: %v", err)
	}

	got, err := env.svc.ListIncomeOnlyTransactionPrices(sec.ID)
	if err != nil {
		t.Fatalf("ListIncomeOnlyTransactionPrices: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 income-only candidate (d2), got %d: %+v", len(got), got)
	}
	c := got[0]
	if !c.Price.Date.Equal(d2) {
		t.Errorf("candidate date = %s, want %s", c.Price.Date.String(), d2.String())
	}
	if c.Price.Price.String() != "80" {
		t.Errorf("candidate price = %s, want 80", c.Price.Price.String())
	}
	if c.Fallback == nil {
		t.Fatal("expected a fallback price (the d1 buy), got nil")
	}
	if !c.Fallback.Date.Equal(d1) || c.Fallback.Price.String() != "100" {
		t.Errorf("fallback = %s @ %s, want 100 @ %s", c.Fallback.Price.String(), c.Fallback.Date.String(), d1.String())
	}
}
