package report_test

import (
	"bytes"
	"os"
	"path/filepath"
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

// allocationFile makes a file with two investment accounts: ACME (large
// cap) in both, MIDX (mid cap) and a tickerless target fund (asset mixture)
// in the brokerage, and $500 of cash left in it. It returns the file's path.
func allocationFile(t *testing.T) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "allocation.tdb")
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

	mkSec := func(ticker, name string, class security.AssetClass) *security.Security {
		t.Helper()
		sec := security.NewSecurity(ticker, name, security.TypeETF)
		sec.AssetClass = class
		if err := svc.Security.Create(sec); err != nil {
			t.Fatalf("setup: create %s: %v", name, err)
		}
		return sec
	}
	acme := mkSec("ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	midx := mkSec("MIDX", "Maple Mid Cap Index", security.AssetClassMidCapStock)
	fund := mkSec("", "Cedar 2045 Target Fund", security.AssetClassAssetMixture)

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
	buy(brokerage, midx, "20", "500.00")
	buy(brokerage, fund, "50", "1000.00")
	for sec, p := range map[*security.Security]string{acme: "120.00", midx: "30.00"} {
		if err := svc.Price.AddPrice(price.NewPrice(sec.ID, types.NewDate(2024, 2, 1),
			types.MustNewMoney(p), price.SourceManual)); err != nil {
			t.Fatalf("setup: price: %v", err)
		}
	}
	database.Close()
	return dbPath
}

func runAllocation(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"report", "allocation"}, args...), stdout, stderr)
	return stdout.String(), err
}

// The full report groups the holdings by class, largest first: ACME 40 ×
// 120, the fund at its 20.00 buy price, MIDX 20 × 30, and the cash. The
// output to a buffer is text only.
func TestReportAllocation_Full(t *testing.T) {
	out, err := runAllocation(t, "--file", allocationFile(t))
	if err != nil {
		t.Fatalf("report allocation: %v\n%s", err, out)
	}
	for _, want := range []string{
		"ASSET ALLOCATION",
		"Asset Class", "Holdings", "% Total",
		"Large Cap Stock", "$4800.00", "69.6%",
		"Asset Mixture", "$1000.00",
		"Mid Cap Stock", "$600.00",
		"Cash", "$500.00",
		"TOTAL (USD)", "$6900.00", "100.0%",
		"████████████████████",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	order := []string{"Large Cap Stock", "Asset Mixture", "Mid Cap Stock", "Cash", "TOTAL"}
	for i := 1; i < len(order); i++ {
		if strings.Index(out, order[i-1]) > strings.Index(out, order[i]) {
			t.Errorf("%s is not above %s:\n%s", order[i-1], order[i], out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("output has an escape code: %q", out)
	}
}

func TestReportAllocation_AssetClass(t *testing.T) {
	path := allocationFile(t)

	t.Run("one class", func(t *testing.T) {
		out, err := runAllocation(t, "--file", path, "--asset-class", "mid_cap_stock")
		if err != nil {
			t.Fatalf("report allocation --asset-class: %v\n%s", err, out)
		}
		for _, want := range []string{
			"ASSET CLASS: Mid Cap Stock\n==========================\n",
			"% of Class",
			"MIDX", "Maple Mid Cap Index", "$600.00",
			"Total", "100.0%", "8.7%",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "ACME") {
			t.Errorf("output has a holding of another class:\n%s", out)
		}
	})

	t.Run("cash lists the uninvested cash", func(t *testing.T) {
		out, err := runAllocation(t, "--file", path, "--asset-class", "cash")
		if err != nil || !strings.Contains(out, "Uninvested cash") {
			t.Errorf("err = %v, output:\n%s", err, out)
		}
	})

	t.Run("the removed index class", func(t *testing.T) {
		_, err := runAllocation(t, "--file", path, "--asset-class", "index")
		if err == nil || !strings.Contains(err.Error(), `invalid asset class: "index"`) {
			t.Errorf("err = %v, want the parse error", err)
		}
	})

	t.Run("an empty class", func(t *testing.T) {
		_, err := runAllocation(t, "--file", path, "--asset-class", "crypto")
		if err == nil || err.Error() != "no holdings in Crypto" {
			t.Errorf("err = %v, want no holdings in Crypto", err)
		}
	})
}

func TestReportAllocation_Empty(t *testing.T) {
	t.Run("no investment accounts", func(t *testing.T) {
		database, dbPath := dbtest.NewFile(t, "empty.tdb")
		database.Close()
		out, err := runAllocation(t, "--file", dbPath)
		if err != nil || !strings.Contains(out, "No investment accounts.") {
			t.Errorf("err = %v, output:\n%s", err, out)
		}
	})

	t.Run("accounts that hold nothing", func(t *testing.T) {
		database, dbPath := dbtest.NewFile(t, "nothing.tdb")
		if err := app.NewServices(database).Account.Create(account.NewAccount("Maple Invest Brokerage",
			account.TypeInvestment, "USD", types.ZeroMoney, types.NewDate(2024, 1, 1))); err != nil {
			t.Fatal(err)
		}
		database.Close()
		out, err := runAllocation(t, "--file", dbPath)
		if err != nil || !strings.Contains(out, "No holdings in the investment accounts.") || strings.Contains(out, "TOTAL") {
			t.Errorf("err = %v, output:\n%s", err, out)
		}
	})
}

// Output to a file is text only, also when the setting says "on".
func TestReportAllocation_ChartImagesOnToAFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "tmoney"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmoney", "config.json"), []byte(`{"chart_images":"on"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := cli.ExecuteWith([]string{"report", "allocation", "--file", allocationFile(t)}, out, &bytes.Buffer{}); err != nil {
		t.Fatalf("report allocation: %v", err)
	}
	got, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "TOTAL (USD)") || strings.Contains(string(got), "\x1b") {
		t.Errorf("want the text report with no escape code:\n%q", got)
	}
}
