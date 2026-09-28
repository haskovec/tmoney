package report

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Money in different currencies is never added: a USD account and a EUR
// account give two totals.
func TestNetWorth_TotalsPerCurrency(t *testing.T) {
	svc, repo := newFigureEnv(t, nil)
	mkAccount(t, repo, "Contoso Checking", account.TypeChecking, "USD", "100.00")
	mkAccount(t, repo, "Fabrikam Sparkonto", account.TypeSavings, "EUR", "40.00")
	mkAccount(t, repo, "Contoso Card", account.TypeCreditCard, "USD", "-30.00")

	nw, err := svc.NetWorthReport()
	if err != nil {
		t.Fatal(err)
	}
	if len(nw.Totals) != 2 || nw.Totals[0].Currency != "EUR" || nw.Totals[1].Currency != "USD" {
		t.Fatalf("totals = %+v, want EUR then USD", nw.Totals)
	}
	if eur := nw.Totals[0]; !eur.NetWorth.Equal(types.MustNewMoney("40.00")) || !eur.Available {
		t.Errorf("EUR = %+v, want 40.00", eur)
	}
	if usd := nw.Totals[1]; !usd.Assets.Equal(types.MustNewMoney("100.00")) ||
		!usd.Liabilities.Equal(types.MustNewMoney("-30.00")) || !usd.NetWorth.Equal(types.MustNewMoney("70.00")) {
		t.Errorf("USD = %+v, want assets 100.00, liabilities -30.00, net 70.00", usd)
	}
	for _, rows := range [][]AccountBalance{nw.Assets, nw.Liabilities} {
		for _, r := range rows {
			if r.Currency == "" {
				t.Errorf("row %s has no currency", r.Name)
			}
		}
	}
}

// One USD brokerage that cannot be valued makes the USD total unavailable,
// and only it: the EUR total still shows, and so does USD's liabilities side.
func TestNetWorth_ValuationErrorIsolatedToItsCurrency(t *testing.T) {
	valuer := &figureValuer{errs: map[types.ID]error{}}
	svc, repo := newFigureEnv(t, valuer)
	brokerage := mkAccount(t, repo, "Northwind Brokerage", account.TypeInvestment, "USD", "777.00")
	valuer.errs[brokerage.ID] = errors.New("price lookup failed")
	mkAccount(t, repo, "Contoso Card", account.TypeCreditCard, "USD", "-30.00")
	mkAccount(t, repo, "Fabrikam Sparkonto", account.TypeSavings, "EUR", "40.00")

	nw, err := svc.NetWorthReport()
	if err != nil {
		t.Fatalf("NetWorthReport() error = %v; a valuation error must not fail the report", err)
	}
	eur, usd := nw.Totals[0], nw.Totals[1]
	if !eur.Available || !eur.NetWorth.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("EUR = %+v, want available 40.00", eur)
	}
	if usd.Available || usd.AssetsAvailable {
		t.Errorf("USD = %+v, want net worth and assets not available", usd)
	}
	if !usd.LiabilitiesAvailable || !usd.Liabilities.Equal(types.MustNewMoney("-30.00")) {
		t.Errorf("USD liabilities = %+v, want available -30.00", usd)
	}
	for _, r := range nw.Assets {
		if r.AccountID == brokerage.ID && (r.Err == nil || !r.Balance.IsZero()) {
			t.Errorf("brokerage row = %+v, want an error and no amount (never the 777.00 opening balance)", r)
		}
	}
}

// Before today, an investment account is current cash and shares at the
// date's prices, not a replay, and the report says so. Today it is exact.
func TestNetWorth_InvestmentAsOfApproximate(t *testing.T) {
	valuer := &figureValuer{results: map[types.ID]ValuationResult{}}
	svc, repo := newFigureEnv(t, valuer)
	acct := account.NewAccount("Northwind Brokerage", account.TypeInvestment, "USD", types.ZeroMoney,
		types.NewDate(2020, time.January, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatal(err)
	}

	past, err := svc.NetWorthAsOf(time.Now().AddDate(-1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !past.InvestmentAsOfApproximate {
		t.Error("a report a year back with a brokerage does not say its value is approximate")
	}

	today, err := svc.NetWorthReport()
	if err != nil {
		t.Fatal(err)
	}
	if today.InvestmentAsOfApproximate {
		t.Error("today's report says the brokerage value is approximate")
	}

	t.Run("no investment account, no note", func(t *testing.T) {
		svc, repo := newFigureEnv(t, nil)
		mkAccount(t, repo, "Contoso Checking", account.TypeChecking, "USD", "100.00")
		past, err := svc.NetWorthAsOf(time.Now().AddDate(-1, 0, 0))
		if err != nil {
			t.Fatal(err)
		}
		if past.InvestmentAsOfApproximate {
			t.Error("a report with no investment account carries the note")
		}
	})
}
