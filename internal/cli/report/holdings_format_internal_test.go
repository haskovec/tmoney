package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

// An unpriced row has "~" on its label and N/A for its price and gain, the
// total gets "~", and the note says what "~" means.
func TestPrintHoldingsReport_Estimated(t *testing.T) {
	m := types.MustNewMoney
	rpt := &reportdom.Holdings{Sections: []reportdom.HoldingsSection{{
		Currency: "USD", Available: true, Estimated: true,
		Value: m("2600.00"), CostBasis: m("2600.00"), Gain: m("0"),
		Rows: []reportdom.HoldingRow{
			{Label: "STRK", Name: "Stark Industries", Shares: types.MustNewQuantity("25"),
				Value: m("2500.00"), CostBasis: m("2500.00"), Percent: 96.15, Estimated: true},
			{Label: "Cash", Name: "Uninvested cash", Cash: true, Value: m("100.00"), CostBasis: m("100.00"), Percent: 3.85},
		},
	}}}

	var out bytes.Buffer
	printHoldingsReport(&out, rpt)
	got := out.String()
	for _, want := range []string{
		"~STRK ", "N/A    $2500.00",
		"TOTAL (USD)",
		"~$2600.00",
		"~ No price on file: the value is the cost basis.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for line := range strings.Lines(got) {
		if strings.TrimRight(line, "\n") != strings.TrimRight(line, " \n") {
			t.Errorf("line ends in spaces: %q", line)
		}
	}
	if err := holdingsErrors(rpt); err != nil {
		t.Errorf("holdingsErrors = %v, want nil", err)
	}
}

// A failed account makes its currency's total "not available", adds the
// percentages note, and gives the command an error that names the account.
// Each currency has its own table and total.
func TestPrintHoldingsReport_FailedAndCurrencies(t *testing.T) {
	m := types.MustNewMoney
	rpt := &reportdom.Holdings{
		Sections: []reportdom.HoldingsSection{
			{Currency: "EUR", Available: true, Value: m("40.00"), CostBasis: m("40.00"), Gain: m("0"),
				Rows: []reportdom.HoldingRow{{Label: "Cash", Name: "Uninvested cash", Cash: true,
					Value: m("40.00"), CostBasis: m("40.00"), Percent: 100}}},
			{Currency: "USD", Available: false, Value: m("100.00"), CostBasis: m("80.00"), Gain: m("20.00"),
				Rows: []reportdom.HoldingRow{{Label: "ACME", Name: "Acme Total Market Index",
					Shares: types.MustNewQuantity("1"), Price: m("100.00"), Value: m("100.00"),
					CostBasis: m("80.00"), Gain: m("20.00"), Percent: 100}}},
		},
		Failed: []reportdom.AccountFailure{
			{Name: "Birch 401k", Currency: "USD", Err: errors.New("valuation failed: boom")},
		},
	}

	var out bytes.Buffer
	printHoldingsReport(&out, rpt)
	got := out.String()
	for _, want := range []string{
		"TOTAL (EUR)", "€40.00",
		"TOTAL (USD)", "not available",
		"Percentages leave out accounts that could not be valued.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "TOTAL (EUR)") > strings.Index(got, "ACME") {
		t.Errorf("the EUR table is not first:\n%s", got)
	}
	if strings.Contains(got, "~ No price") {
		t.Errorf("output has the estimate note with no estimate:\n%s", got)
	}

	err := holdingsErrors(rpt)
	if err == nil || !strings.Contains(err.Error(), "Birch 401k: valuation failed: boom") {
		t.Errorf("holdingsErrors = %v, want it to name the 401k", err)
	}
}
