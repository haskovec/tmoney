package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/dbtest"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
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

// birchFailed is a report where the only USD account could not be valued:
// its section stays, with no rows.
func birchFailed() *reportdom.Holdings {
	return &reportdom.Holdings{
		Sections: []reportdom.HoldingsSection{{Currency: "USD", Available: false, Value: types.ZeroMoney}},
		Failed: []reportdom.AccountFailure{
			{Name: "Birch 401k", Currency: "USD", Err: errors.New("valuation failed: boom")},
		},
	}
}

// When every account of a currency failed, the section still prints its
// TOTAL as "not available", and the report does not say there are no
// accounts.
func TestPrintHoldingsReport_AllAccountsFailed(t *testing.T) {
	var out bytes.Buffer
	printHoldingsReport(&out, birchFailed())
	got := out.String()
	for _, want := range []string{"TOTAL (USD)", "not available",
		"Percentages leave out accounts that could not be valued."} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "No investment accounts.") {
		t.Errorf("output says there are no accounts:\n%s", got)
	}
}

// A security in no row is "not held" only when every account was valued.
// Else it can be in a failed account, and the error names those accounts.
func TestNotHeldError(t *testing.T) {
	rpt := birchFailed()
	var out bytes.Buffer
	if printHoldingSplit(&out, rpt, types.NewID(), "ACME", "Acme Total Market Index") {
		t.Fatal("printHoldingSplit found a row in a report with no rows")
	}
	if out.Len() != 0 {
		t.Errorf("printHoldingSplit printed %q, want nothing", out.String())
	}
	err := notHeldError(rpt, "ACME").Error()
	if !strings.Contains(err, "Birch 401k: valuation failed: boom") || strings.Contains(err, "not held") {
		t.Errorf("notHeldError = %q, want the failed account and no \"not held\"", err)
	}

	err = notHeldError(&reportdom.Holdings{}, "ACME").Error()
	if err != "ACME is not held in an active investment account" {
		t.Errorf("notHeldError with no failure = %q", err)
	}
}

// The split of an unpriced security marks each value with "~" and says what
// "~" means. The split of a security in a section with a failed account says
// that its percentage is of the holdings that could be valued.
func TestPrintHoldingSplit_EstimatedAndPartial(t *testing.T) {
	m := types.MustNewMoney
	strk := types.NewID()
	rpt := &reportdom.Holdings{Sections: []reportdom.HoldingsSection{{
		Currency: "USD", Available: false, Estimated: true, Value: m("2500.00"),
		Rows: []reportdom.HoldingRow{{SecurityID: strk, Label: "STRK", Name: "Stark Industries",
			Shares: types.MustNewQuantity("25"), Value: m("2500.00"), CostBasis: m("2500.00"),
			Percent: 100, Estimated: true,
			Accounts: []reportdom.HoldingAccount{{Name: "Maple Invest Brokerage",
				Shares: types.MustNewQuantity("25"), Value: m("2500.00"), Percent: 100}}}},
	}}}

	var out bytes.Buffer
	if !printHoldingSplit(&out, rpt, strk, "STRK", "Stark Industries") {
		t.Fatal("printHoldingSplit did not find STRK")
	}
	got := out.String()
	for _, want := range []string{
		"Maple Invest Brokerage  25      ~$2500.00",
		"Total                   25      ~$2500.00",
		"STRK is 100.0% of the holdings that could be valued.",
		"~ No price on file: the value is the cost basis.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

// birchValuer values every account as cash only, except the ones in fail.
type birchValuer struct{ fail map[types.ID]error }

func (v birchValuer) GetAccountValuation(id types.ID, _ types.Date) (*reportdom.ValuationResult, error) {
	if err := v.fail[id]; err != nil {
		return nil, err
	}
	cash := types.MustNewMoney("250.00")
	return &reportdom.ValuationResult{TotalValue: cash, CashBalance: cash}, nil
}

// The command with an account that cannot be valued: it prints the report
// with its TOTAL "not available", then fails with an error that names the
// account. The split of a security that only the failed account could hold
// gets the same error.
func TestReportHoldings_FailedAccount(t *testing.T) {
	database := dbtest.New(t)
	svc := app.NewServices(database)
	mkAcct := func(name string) *account.Account {
		t.Helper()
		acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.NewDate(2024, 1, 1))
		if err := svc.Account.Create(acct); err != nil {
			t.Fatal(err)
		}
		return acct
	}
	mkAcct("Maple Invest Brokerage")
	birch := mkAcct("Birch 401k")
	if err := svc.Security.Create(security.NewSecurity("ACME", "Acme Total Market Index", security.TypeETF)); err != nil {
		t.Fatal(err)
	}
	svc.Report = reportdom.NewService(account.NewRepository(database), database,
		reportdom.WithInvestmentValuer(birchValuer{fail: map[types.ID]error{birch.ID: errors.New("boom")}}))

	var out bytes.Buffer
	err := reportHoldings(&out, svc, &reportHoldingsOptions{})
	if err == nil || !strings.Contains(err.Error(), "Birch 401k") {
		t.Errorf("err = %v, want an error that names the 401k", err)
	}
	got := out.String()
	for _, want := range []string{"Cash", "$250.00", "TOTAL (USD)", "not available",
		"Percentages leave out accounts that could not be valued."} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for line := range strings.Lines(got) {
		if strings.HasPrefix(line, "TOTAL") && strings.Contains(line, "100.0%") {
			t.Errorf("TOTAL line = %q, want no 100.0%% with a failed account", line)
		}
	}

	out.Reset()
	err = reportHoldings(&out, svc, &reportHoldingsOptions{ticker: "ACME"})
	if err == nil || !strings.Contains(err.Error(), "ACME is not in an account that could be valued") ||
		!strings.Contains(err.Error(), "Birch 401k") {
		t.Errorf("split err = %v, want the not-valued error that names the 401k", err)
	}
}

// When negative cash makes the total zero or less, every row is 0%, so the
// TOTAL does not show 100.0%.
func TestPrintHoldingsReport_TotalNotPositive(t *testing.T) {
	m := types.MustNewMoney
	rpt := &reportdom.Holdings{Sections: []reportdom.HoldingsSection{{
		Currency: "USD", Available: true,
		Value: m("-300.00"), CostBasis: m("-350.00"), Gain: m("50.00"),
		Rows: []reportdom.HoldingRow{
			{Label: "ACME", Name: "Acme Total Market Index", Shares: types.MustNewQuantity("2"),
				Price: m("100.00"), Value: m("200.00"), CostBasis: m("150.00"), Gain: m("50.00")},
			{Label: "Cash", Name: "Uninvested cash", Cash: true, Value: m("-500.00"), CostBasis: m("-500.00")},
		},
	}}}

	var out bytes.Buffer
	printHoldingsReport(&out, rpt)
	got := out.String()
	for line := range strings.Lines(got) {
		if strings.HasPrefix(line, "TOTAL") && (!strings.Contains(line, "-$300.00") || strings.Contains(line, "100.0%")) {
			t.Errorf("TOTAL line = %q, want -$300.00 and no 100.0%%", line)
		}
	}
	if !strings.Contains(got, "TOTAL (USD)") {
		t.Errorf("output has no TOTAL line:\n%s", got)
	}
}
