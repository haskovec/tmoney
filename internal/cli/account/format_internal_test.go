package account

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

// A row whose valuation failed prints "error", its currency has no total,
// other currencies still total, and the command gets an error that names the
// account. The opening balance never stands in for the value.
func TestPrintBalancesTable_FailedRow(t *testing.T) {
	date := types.MustParseDate("2020-01-01")
	checking := accountdom.NewAccount("Contoso Checking", accountdom.TypeChecking, "USD", types.ZeroMoney, date)
	brokerage := accountdom.NewAccount("Northwind Brokerage", accountdom.TypeInvestment, "USD", types.MustNewMoney("777.00"), date)
	savings := accountdom.NewAccount("Fabrikam Sparkonto", accountdom.TypeSavings, "EUR", types.ZeroMoney, date)
	accounts := []*accountdom.Account{checking, brokerage, savings}
	figs := []reportdom.AccountFigure{
		{Type: accountdom.TypeChecking, Currency: "USD", Displayed: types.MustNewMoney("100.00")},
		{Type: accountdom.TypeInvestment, Currency: "USD", Err: errors.New("valuation failed: boom")},
		{Type: accountdom.TypeSavings, Currency: "EUR", Displayed: types.MustNewMoney("40.00")},
	}

	var out bytes.Buffer
	printBalancesTable(&out, accounts, figs)
	got := out.String()
	for _, want := range []string{
		"Contoso Checking:    $100.00",
		"Northwind Brokerage: error",
		"Net Worth (EUR):     €40.00",
		"Net Worth (USD):     not available",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "777") {
		t.Errorf("the opening balance stood in for the value:\n%s", got)
	}

	err := figureErrors(accounts, figs)
	if err == nil || !strings.Contains(err.Error(), "Northwind Brokerage: valuation failed: boom") {
		t.Errorf("figureErrors() = %v, want one naming the brokerage", err)
	}
	if figureErrors(accounts[:1], figs[:1]) != nil {
		t.Error("figureErrors() reported an error with no failed rows")
	}
}

func TestPrintAccountsTable_FailedRow(t *testing.T) {
	acct := accountdom.NewAccount("Northwind Brokerage", accountdom.TypeInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	var out bytes.Buffer
	printAccountsTable(&out, []*accountdom.Account{acct}, []reportdom.AccountFigure{
		{Type: accountdom.TypeInvestment, Currency: "USD", Err: errors.New("boom")},
	})
	if !strings.Contains(out.String(), "Northwind Brokerage  Investment  error") {
		t.Errorf("row does not show error:\n%s", out.String())
	}
}

func TestPrintAccountDetails_InvestmentFailed(t *testing.T) {
	acct := accountdom.NewAccount("Northwind Brokerage", accountdom.TypeInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	var out bytes.Buffer
	printAccountDetails(&out, acct, &accountdom.Balance{}, reportdom.AccountFigure{
		Type: accountdom.TypeInvestment, Currency: "USD", Err: errors.New("boom"),
	})
	for _, want := range []string{"Cash:            error", "Total Value:     error"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
