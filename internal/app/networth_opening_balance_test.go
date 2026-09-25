package app

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// TestNetWorth_InvestmentOpeningBalanceIsCash pins that an investment
// account's opening balance is cash the account holds: it counts in the
// cash balance and in net worth before and after any security is bought,
// and it is never reported as a return.
func TestNetWorth_InvestmentOpeningBalanceIsCash(t *testing.T) {
	database := createTestDB(t)
	svc := NewServices(database)

	acct := account.NewAccount("Maple Invest HSA", account.TypeHSAInvestment, "USD",
		types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	netWorth := func() types.Money {
		t.Helper()
		rpt, err := svc.Report.NetWorthReport()
		if err != nil {
			t.Fatalf("NetWorthReport: %v", err)
		}
		return rpt.NetWorth
	}

	cash, err := svc.Investment.GetCashBalance(acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cash.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("cash before any buy = %s, want 1000", cash)
	}
	if nw := netWorth(); !nw.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("net worth before any buy = %s, want 1000", nw)
	}

	sec := security.NewSecurity("ACME", "Acme Index Fund", security.TypeETF)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	total := types.MustNewMoney("400.00")
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.NewDate(2024, 1, 3),
		types.MustNewQuantity("4"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy: %v", err)
	}

	val, err := svc.InvestmentValuation.GetAccountValuation(acct.ID, types.Today(), investment.ValuationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !val.CashBalance.Equal(types.MustNewMoney("600.00")) {
		t.Errorf("cash after the buy = %s, want 600", val.CashBalance)
	}
	if !val.TotalValue.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("total value after the buy = %s, want 1000", val.TotalValue)
	}
	if !val.TotalReturn.IsZero() {
		t.Errorf("total return = %s, want 0: the opening balance is not a gain", val.TotalReturn)
	}
	if nw := netWorth(); !nw.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("net worth after the buy = %s, want 1000", nw)
	}
}
