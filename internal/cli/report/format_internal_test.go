package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

// A row that could not be valued prints "error", its currency's totals print
// "not available" (the liabilities side too only if it failed there), and
// the command gets an error that names the account.
func TestPrintNetWorthReport_FailedRow(t *testing.T) {
	m := types.MustNewMoney
	rpt := &reportdom.NetWorth{
		Assets: []reportdom.AccountBalance{
			{Name: "Northwind Brokerage", Currency: "USD", Err: errors.New("valuation failed: boom")},
		},
		Liabilities: []reportdom.AccountBalance{
			{Name: "Contoso Card", Currency: "USD", Balance: m("-30.00")},
		},
		Totals: []reportdom.CurrencyTotal{{
			Currency: "USD", Liabilities: m("-30.00"), NetWorth: m("-30.00"),
			Available: false, AssetsAvailable: false, LiabilitiesAvailable: true,
		}},
	}

	var out bytes.Buffer
	printNetWorthReport(&out, rpt)
	got := out.String()
	for _, want := range []string{
		"Northwind Brokerage  error",
		"Total Assets (USD):\tnot available",
		"Total Liabilities (USD):\t-$30.00",
		"NET WORTH (USD):\tnot available",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}

	err := netWorthErrors(rpt)
	if err == nil || !strings.Contains(err.Error(), "Northwind Brokerage: valuation failed: boom") {
		t.Errorf("netWorthErrors() = %v, want one naming the brokerage", err)
	}
}
