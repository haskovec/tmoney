package app

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// TestHoldingsReport_TotalIsNetWorthInvestmentValue pins the holdings
// report against net worth on real valuations: the section's Value is the
// sum of the investment rows in net worth, and the price reaches the row.
func TestHoldingsReport_TotalIsNetWorthInvestmentValue(t *testing.T) {
	database := createTestDB(t)
	svc := NewServices(database)

	mkAcct := func(name string, typ account.Type, opening string) *account.Account {
		t.Helper()
		acct := account.NewAccount(name, typ, "USD", types.MustNewMoney(opening), types.NewDate(2024, 1, 1))
		if err := svc.Account.Create(acct); err != nil {
			t.Fatal(err)
		}
		return acct
	}
	brokerage := mkAcct("Maple Invest Brokerage", account.TypeInvestment, "5000.00")
	hsa := mkAcct("Cedar HSA Investment", account.TypeHSAInvestment, "2000.00")
	mkAcct("Contoso Checking", account.TypeChecking, "750.00")

	acme := security.NewSecurity("ACME", "Acme Total Market Index", security.TypeETF)
	strk := security.NewSecurity("STRK", "Stark Industries", security.TypeStock)
	for _, sec := range []*security.Security{acme, strk} {
		if err := svc.Security.Create(sec); err != nil {
			t.Fatal(err)
		}
	}
	buy := func(acct *account.Account, sec *security.Security, shares, total string) {
		t.Helper()
		amount := types.MustNewMoney(total)
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.NewDate(2024, 1, 3),
			types.MustNewQuantity(shares), &amount, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy: %v", err)
		}
	}
	buy(brokerage, acme, "30", "3000.00")
	buy(hsa, acme, "10", "1000.00")
	buy(brokerage, strk, "5", "500.00")
	if err := svc.Price.AddPrice(price.NewPrice(acme.ID, types.NewDate(2024, 2, 1),
		types.MustNewMoney("120.00"), price.SourceManual)); err != nil {
		t.Fatal(err)
	}

	rpt, err := svc.Report.Holdings()
	if err != nil {
		t.Fatal(err)
	}
	if len(rpt.Sections) != 1 || len(rpt.Failed) != 0 {
		t.Fatalf("report = %+v, want one section and no failure", rpt)
	}
	sec := rpt.Sections[0]

	nw, err := svc.Report.NetWorthReport()
	if err != nil {
		t.Fatal(err)
	}
	investments := types.ZeroMoney
	for _, ab := range nw.Assets {
		if account.Type(ab.Type).IsInvestmentType() {
			investments = investments.Add(ab.Balance)
		}
	}
	if !sec.Value.Equal(investments) {
		t.Errorf("holdings total = %s, net worth investment value = %s; want equal", sec.Value, investments)
	}
	// Cash 7000 − 4500 of buys, ACME 40 × 120, STRK 5 × its 100 buy price.
	if !sec.Value.Equal(types.MustNewMoney("7800.00")) {
		t.Errorf("holdings total = %s, want 7800.00", sec.Value)
	}

	byLabel := map[string]int{}
	for i, r := range sec.Rows {
		byLabel[r.Label] = i
	}
	acmeRow := sec.Rows[byLabel["ACME"]]
	if !acmeRow.Price.Equal(types.MustNewMoney("120.00")) || !acmeRow.Shares.Equal(types.MustNewQuantity("40")) ||
		len(acmeRow.Accounts) != 2 || acmeRow.Estimated {
		t.Errorf("ACME row = %+v, want 40 shares at 120.00 in two accounts", acmeRow)
	}
	if cash := sec.Rows[byLabel["Cash"]]; !cash.Value.Equal(types.MustNewMoney("2500.00")) || len(cash.Accounts) != 2 {
		t.Errorf("cash row = %+v, want 2500.00 from two accounts", cash)
	}
}
