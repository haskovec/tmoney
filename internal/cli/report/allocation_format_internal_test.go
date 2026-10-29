package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// specClass is one class of the fixture of the spec's §5.3.1 example.
type specClass struct {
	class security.AssetClass
	count int
	value string
}

// specAllocation is the report of the spec's examples: 12 holdings in ten
// classes, and STRK with no price.
func specAllocation() *reportdom.Allocation {
	m := types.MustNewMoney
	classes := []specClass{
		{security.AssetClassLargeCapStock, 2, "110175.75"},
		{security.AssetClassAssetMixture, 1, "58226.45"},
		{security.AssetClassMidCapStock, 2, "45000.00"},
		{security.AssetClassInternationalStock, 1, "40240.80"},
		{security.AssetClassDomesticBond, 1, "29581.50"},
		{security.AssetClassCash, 1, "17345.67"},
		{security.AssetClassRealEstate, 1, "12500.00"},
		{security.AssetClassSmallCapStock, 1, "8000.00"},
		{security.AssetClassCrypto, 1, "6000.00"},
		{security.AssetClassUnclassified, 1, "1200.00"},
	}
	total := m("328270.17")
	sec := reportdom.AllocationSection{Currency: "USD", Value: total, Available: true}
	for _, c := range classes {
		row := reportdom.ClassRow{Class: c.class, Value: m(c.value), Percent: m(c.value).Float64() / total.Float64() * 100}
		for range c.count {
			row.Holdings = append(row.Holdings, reportdom.ClassHolding{})
		}
		sec.Classes = append(sec.Classes, row)
	}
	large := &sec.Classes[0]
	large.Holdings = []reportdom.ClassHolding{
		{HoldingRow: reportdom.HoldingRow{Label: "ACME", Name: "Acme Total Market Index", Value: m("101103.75")},
			PercentOfClass: 91.77, PercentOfTotal: 30.80},
		{HoldingRow: reportdom.HoldingRow{Label: "INIT", Name: "Initech Corp", Value: m("9072.00")},
			PercentOfClass: 8.23, PercentOfTotal: 2.76},
	}
	large.NoPrice = []reportdom.HoldingRow{{Label: "STRK", Name: "Stark Industries", Estimated: true}}
	return &reportdom.Allocation{AsOfDate: types.NewDate(2024, 1, 15), AccountCount: 3, Sections: []reportdom.AllocationSection{sec}}
}

// The full report is the spec's §5.3.1 example, with both notes.
func TestPrintAllocationReport_SpecExample(t *testing.T) {
	var out bytes.Buffer
	printAllocationReport(&out, specAllocation(), nil)
	want := `ASSET ALLOCATION
================
As of: January 15, 2024

Asset Class          Holdings  Value       % Total
Large Cap Stock      2         $110175.75  33.6%    ████████████████████
Asset Mixture        1         $58226.45   17.7%    ██████████▌
Mid Cap Stock        2         $45000.00   13.7%    ████████▏
International Stock  1         $40240.80   12.3%    ███████▎
Domestic Bond        1         $29581.50   9.0%     █████▎
Cash                 1         $17345.67   5.3%     ███▏
Real Estate          1         $12500.00   3.8%     ██▎
Small Cap Stock      1         $8000.00    2.4%     █▍
Crypto               1         $6000.00    1.8%     █
Unclassified         1         $1200.00    0.4%     ▏
TOTAL (USD)          12        $328270.17  100.0%

Not included (no price): STRK (Large Cap Stock). Add a price to include it.
1 holding has no asset class. Set one with: tmoney security edit <ticker> --asset-class <class>
`
	if got := out.String(); got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

// One class is the spec's §5.3.2 example.
func TestPrintAllocationClass_SpecExample(t *testing.T) {
	var out bytes.Buffer
	if !printAllocationClass(&out, specAllocation(), security.AssetClassLargeCapStock) {
		t.Fatal("printAllocationClass = false, want true")
	}
	want := `ASSET CLASS: Large Cap Stock
============================
As of: January 15, 2024

Security  Name                     Value       % of Class  % Total
ACME      Acme Total Market Index  $101103.75  91.8%       30.8%
INIT      Initech Corp             $9072.00    8.2%        2.8%
STRK      Stark Industries         no price
Total                              $110175.75  100.0%      33.6%

Not included (no price): STRK. Add a price to include it.
`
	if got := out.String(); got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
	if printAllocationClass(&bytes.Buffer{}, specAllocation(), security.AssetClassCommodity) {
		t.Error("printAllocationClass(commodity) = true, want false: no section has it")
	}
}

// The Unclassified note says "holdings have" for more than one, and is not
// printed when Unclassified has no priced holding.
func TestPrintAllocationReport_UnclassifiedNote(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{
		{0, ""},
		{1, "1 holding has no asset class."},
		{3, "3 holdings have no asset class."},
	} {
		rpt := specAllocation()
		sec := &rpt.Sections[0]
		un := &sec.Classes[len(sec.Classes)-1]
		un.Holdings = make([]reportdom.ClassHolding, tc.count)
		var out bytes.Buffer
		printAllocationReport(&out, rpt, nil)
		got := out.String()
		if tc.want == "" {
			if strings.Contains(got, "no asset class") {
				t.Errorf("count %d: output has the note:\n%s", tc.count, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want+" Set one with: tmoney security edit <ticker> --asset-class <class>\n") {
			t.Errorf("count %d: output lacks %q:\n%s", tc.count, tc.want, got)
		}
	}
}

// A failed account makes the TOTAL "not available", adds the percentages
// note, and gives the command an error that names the account. Each currency
// has its own table and total. A class total stays a value.
func TestPrintAllocationReport_FailedAndCurrencies(t *testing.T) {
	m := types.MustNewMoney
	rpt := &reportdom.Allocation{
		AccountCount: 3,
		Sections: []reportdom.AllocationSection{
			{Currency: "EUR", Value: m("40.00"), Available: true, Classes: []reportdom.ClassRow{
				{Class: security.AssetClassCash, Value: m("40.00"), Percent: 100,
					Holdings: []reportdom.ClassHolding{{HoldingRow: reportdom.HoldingRow{Cash: true}}}},
			}},
			{Currency: "USD", Value: m("100.00"), Available: false, Classes: []reportdom.ClassRow{
				{Class: security.AssetClassLargeCapStock, Value: m("100.00"), Percent: 100,
					Holdings: []reportdom.ClassHolding{{}}},
			}},
		},
		Failed: []reportdom.AccountFailure{{Name: "Birch 401k", Currency: "USD", Err: errors.New("price lookup failed")}},
	}
	var out bytes.Buffer
	printAllocationReport(&out, rpt, nil)
	got := out.String()
	for _, want := range []string{
		"TOTAL (EUR)  0         €40.00",
		"TOTAL (USD)                not available\n",
		"\nPercentages leave out accounts that could not be valued.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "EUR") > strings.Index(got, "USD") {
		t.Errorf("EUR is not first:\n%s", got)
	}
	err := failedAccountsError(rpt.Failed)
	if err == nil || !strings.Contains(err.Error(), "Birch 401k: price lookup failed") {
		t.Errorf("error = %v, want the 401k", err)
	}
	if err := notInClassError(rpt, security.AssetClassMidCapStock); err == nil ||
		!strings.HasPrefix(err.Error(), "no holdings in Mid Cap Stock in the accounts that could be valued: could not value 1 account(s): Birch 401k") {
		t.Errorf("notInClassError = %v, want the C12 error", err)
	}

	// The class view shows the class total, as the class row above and the
	// TUI do, with the note.
	out.Reset()
	printAllocationClass(&out, rpt, security.AssetClassLargeCapStock)
	got = out.String()
	for _, want := range []string{
		"Total           $100.00  100.0%      100.0%\n",
		"\nPercentages leave out accounts that could not be valued.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("class output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not available") {
		t.Errorf("class output has \"not available\":\n%s", got)
	}
}
