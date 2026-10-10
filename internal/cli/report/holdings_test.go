package report_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// holdingsFile makes a file with two investment accounts that both hold
// ACME, a tickerless fund in one of them, and a checking account that the
// report leaves out. It returns the file's path.
func holdingsFile(t *testing.T) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "holdings.tdb")
	svc := app.NewServices(database)

	mkAcct := func(name string, typ account.Type, opening string) *account.Account {
		t.Helper()
		acct := account.NewAccount(name, typ, "USD", types.MustNewMoney(opening), types.NewDate(2024, 1, 1))
		if err := svc.Account.Create(acct); err != nil {
			t.Fatalf("setup: create %s: %v", name, err)
		}
		return acct
	}
	brokerage := mkAcct("Maple Invest Brokerage", account.TypeInvestment, "5000.00")
	hsa := mkAcct("Cedar HSA Investment", account.TypeHSAInvestment, "1000.00")
	mkAcct("Contoso Checking", account.TypeChecking, "750.00")

	acme := security.NewSecurity("ACME", "Acme Total Market Index", security.TypeETF)
	fund := security.NewSecurity("", "Cedar 2045 Target Fund", security.TypeMutualFund)
	for _, sec := range []*security.Security{acme, fund} {
		if err := svc.Security.Create(sec); err != nil {
			t.Fatalf("setup: create %s: %v", sec.Name, err)
		}
	}
	buy := func(acct *account.Account, sec *security.Security, shares, total string) {
		t.Helper()
		amount := types.MustNewMoney(total)
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.NewDate(2024, 1, 3),
			types.MustNewQuantity(shares), &amount, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("setup: buy: %v", err)
		}
	}
	buy(brokerage, acme, "30", "3000.00")
	buy(hsa, acme, "10", "1000.00")
	buy(brokerage, fund, "50", "1000.00")
	if err := svc.Price.AddPrice(price.NewPrice(acme.ID, types.NewDate(2024, 2, 1),
		types.MustNewMoney("120.00"), price.SourceManual)); err != nil {
		t.Fatalf("setup: price: %v", err)
	}
	database.Close()
	return dbPath
}

func runHoldings(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"report", "holdings"}, args...), stdout, stderr)
	return stdout.String(), err
}

func TestReportHoldings_MissingFile(t *testing.T) {
	if _, err := runHoldings(t); err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("report holdings without --file: err = %v, want a --file error", err)
	}
}

func TestReportHoldings_NoInvestmentAccounts(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "empty.tdb")
	database.Close()

	out, err := runHoldings(t, "--file", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "HOLDINGS REPORT") || !strings.Contains(out, "No investment accounts.") {
		t.Errorf("output:\n%s", out)
	}
}

// The full report adds ACME up across both accounts, puts the cash in one
// row, and totals the investment value: cash 6000 − 5000 of buys, ACME
// 40 × 120, the fund at its 20.00 buy price.
func TestReportHoldings_Full(t *testing.T) {
	out, err := runHoldings(t, "--file", holdingsFile(t))
	if err != nil {
		t.Fatalf("report holdings: %v\n%s", err, out)
	}
	for _, want := range []string{
		"HOLDINGS REPORT",
		"Security", "% Total", "Cost Basis",
		"ACME", "Acme Total Market Index", "40", "$120.00", "$4800.00", "$4000.00", "$800.00",
		"Cedar 2045 Target Fund",
		"Cash", "Uninvested cash", "$1000.00",
		"TOTAL (USD)", "$6800.00", "100.0%",
		"████████████████████",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Contoso Checking") || strings.Contains(out, "~") {
		t.Errorf("output has the checking account or an estimate note:\n%s", out)
	}
	// ACME is the largest row, so it comes first.
	if strings.Index(out, "ACME") > strings.Index(out, "Cash") {
		t.Errorf("ACME is not above Cash:\n%s", out)
	}
}

func TestReportHoldings_Split(t *testing.T) {
	path := holdingsFile(t)

	t.Run("by ticker", func(t *testing.T) {
		out, err := runHoldings(t, "--file", path, "--ticker", "ACME")
		if err != nil {
			t.Fatalf("report holdings --ticker: %v\n%s", err, out)
		}
		for _, want := range []string{
			"HOLDING: ACME (Acme Total Market Index)",
			"% of Holding",
			"Maple Invest Brokerage", "$3600.00", "75.0%",
			"Cedar HSA Investment", "$1200.00", "25.0%",
			"ACME is 70.6% of all holdings ($6800.00).",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		if strings.Index(out, "Maple Invest Brokerage") > strings.Index(out, "Cedar HSA Investment") {
			t.Errorf("the larger account is not first:\n%s", out)
		}
	})

	t.Run("by name, no ticker", func(t *testing.T) {
		out, err := runHoldings(t, "--file", path, "--name", "Cedar 2045 Target Fund")
		if err != nil {
			t.Fatalf("report holdings --name: %v\n%s", err, out)
		}
		if !strings.Contains(out, "HOLDING: Cedar 2045 Target Fund\n") ||
			!strings.Contains(out, "Cedar 2045 Target Fund is 14.7% of all holdings") {
			t.Errorf("output:\n%s", out)
		}
	})

	t.Run("not held", func(t *testing.T) {
		database, dbPath := dbtest.NewFile(t, "notheld.tdb")
		if err := app.NewServices(database).Security.Create(
			security.NewSecurity("GLBX", "Globex International Index", security.TypeETF)); err != nil {
			t.Fatal(err)
		}
		database.Close()
		_, err := runHoldings(t, "--file", dbPath, "--ticker", "GLBX")
		if err == nil || !strings.Contains(err.Error(), "GLBX is not held in an active investment account") {
			t.Errorf("err = %v, want not held", err)
		}
	})

	t.Run("unknown ticker", func(t *testing.T) {
		if _, err := runHoldings(t, "--file", path, "--ticker", "NOPE"); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("err = %v, want not found", err)
		}
	})

	t.Run("two selectors", func(t *testing.T) {
		if _, err := runHoldings(t, "--file", path, "--ticker", "ACME", "--name", "Cedar 2045 Target Fund"); err == nil ||
			!strings.Contains(err.Error(), "only one") {
			t.Errorf("err = %v, want only one selector", err)
		}
	})
}
