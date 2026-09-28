package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func runAcct(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"account"}, args...), stdout, stderr)
	return stdout.String(), err
}

func createAcct(t *testing.T, svc *app.Services, name string, typ accountdom.Type, currency, opening string) *accountdom.Account {
	t.Helper()
	acct := accountdom.NewAccount(name, typ, currency, types.MustNewMoney(opening), types.MustParseDate("2020-01-01"))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return acct
}

// A brokerage whose only activity is a deposit has a register balance of 0
// and investment cash of 125. The commands must show the cash.
func TestAccountFigures_BrokerageCashOnly(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	svc := app.NewServices(database)
	acct := createAcct(t, svc, "Northwind Brokerage", accountdom.TypeInvestment, "USD", "0")
	if _, err := svc.Investment.Deposit(acct.ID, types.MustParseDate("2020-02-01"), types.MustNewMoney("125.00"), ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	out, err := runAcct(t, "show", "Northwind Brokerage", "--file", dbPath)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"Cash:            $125.00", "Total Value:     $125.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Cleared Balance") {
		t.Errorf("show prints a cleared balance for an investment account:\n%s", out)
	}

	out, err = runAcct(t, "balance", "--file", dbPath)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	for _, want := range []string{"Northwind Brokerage: $125.00", "Net Worth (USD):     $125.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("balance output lacks %q:\n%s", want, out)
		}
	}
}

// A holding with no price is valued at cost and marked "~".
func TestAccountFigures_UnpricedHoldingIsEstimated(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	svc := app.NewServices(database)
	acct := createAcct(t, svc, "Northwind Brokerage", accountdom.TypeInvestment, "USD", "0")
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatalf("setup: %v", err)
	}
	date := types.MustParseDate("2020-02-01")
	if _, err := svc.Investment.Deposit(acct.ID, date, types.MustNewMoney("500.00"), ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	total := types.MustNewMoney("500.00")
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	dropPrices(t, database, sec.ID)
	database.Close()

	out, err := runAcct(t, "list", "--file", dbPath)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "~$500.00") {
		t.Errorf("list does not show the estimated value ~$500.00:\n%s", out)
	}
	if !strings.Contains(out, "~ estimated") {
		t.Errorf("list does not explain the ~ mark:\n%s", out)
	}
}

// dropPrices removes the price the buy recorded, leaving a holding with none.
func dropPrices(t *testing.T, database *db.DB, securityID types.ID) {
	t.Helper()
	if _, err := database.Conn().Exec(
		`DELETE FROM security_prices WHERE CAST(security_id AS VARCHAR) = ?`, securityID.String()); err != nil {
		t.Fatalf("drop prices: %v", err)
	}
}

// Money in different currencies is never added, and a EUR total is never
// labelled USD.
func TestAccountFigures_TotalsPerCurrency(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	svc := app.NewServices(database)
	createAcct(t, svc, "Contoso Checking", accountdom.TypeChecking, "USD", "100.00")
	createAcct(t, svc, "Fabrikam Sparkonto", accountdom.TypeSavings, "EUR", "40.00")
	database.Close()

	out, err := runAcct(t, "balance", "--file", dbPath)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	for _, want := range []string{"Net Worth (EUR):     €40.00", "Net Worth (USD):     $100.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("balance output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "140") {
		t.Errorf("balance added across currencies:\n%s", out)
	}
}
