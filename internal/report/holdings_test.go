package report

import (
	"errors"
	"math"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// holdingsEnv is a report service on a test file, with a fake valuer and a
// way to add securities for the row labels.
type holdingsEnv struct {
	svc    *Service
	accts  *account.Repository
	secs   *security.Repository
	valuer *figureValuer
}

func newHoldingsEnv(t *testing.T) *holdingsEnv {
	t.Helper()
	valuer := &figureValuer{results: map[types.ID]ValuationResult{}, errs: map[types.ID]error{}}
	database := createTestDB(t)
	accts := account.NewRepository(database)
	return &holdingsEnv{
		svc:    NewService(accts, database, WithInvestmentValuer(valuer)),
		accts:  accts,
		secs:   security.NewRepository(database),
		valuer: valuer,
	}
}

func (e *holdingsEnv) security(t *testing.T, ticker, name string) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, name, security.TypeETF)
	if err := e.secs.Create(sec); err != nil {
		t.Fatal(err)
	}
	return sec
}

// priced is a position with a price: its value is shares × price.
func priced(sec *security.Security, shares, price, cost string) HoldingFigure {
	q := types.MustNewQuantity(shares)
	p := types.MustNewMoney(price)
	return HoldingFigure{
		SecurityID: sec.ID, Shares: q, Price: p, MarketValue: p.Mul(q.Decimal()),
		CostBasis: types.MustNewMoney(cost), HasPricing: true,
	}
}

// unpriced is a position with no price: its value is its cost basis.
func unpriced(sec *security.Security, shares, cost string) HoldingFigure {
	c := types.MustNewMoney(cost)
	return HoldingFigure{
		SecurityID: sec.ID, Shares: types.MustNewQuantity(shares), Price: types.ZeroMoney,
		MarketValue: c, CostBasis: c,
	}
}

// value sets what the fake valuer returns for an account.
func (e *holdingsEnv) value(acct *account.Account, cash string, holdings ...HoldingFigure) {
	c := types.MustNewMoney(cash)
	total := c
	for _, h := range holdings {
		total = total.Add(h.MarketValue)
	}
	e.valuer.results[acct.ID] = ValuationResult{TotalValue: total, CashBalance: c, Holdings: holdings}
}

func (e *holdingsEnv) report(t *testing.T) *Holdings {
	t.Helper()
	rpt, err := e.svc.Holdings()
	if err != nil {
		t.Fatalf("Holdings() error = %v", err)
	}
	return rpt
}

func onlySection(t *testing.T, rpt *Holdings) HoldingsSection {
	t.Helper()
	if len(rpt.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(rpt.Sections))
	}
	return rpt.Sections[0]
}

func rowByLabel(t *testing.T, sec HoldingsSection, label string) HoldingRow {
	t.Helper()
	for _, r := range sec.Rows {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no row %q in %+v", label, sec.Rows)
	return HoldingRow{}
}

func wantMoney(t *testing.T, what string, got types.Money, want string) {
	t.Helper()
	if !got.Equal(types.MustNewMoney(want)) {
		t.Errorf("%s = %s, want %s", what, got, want)
	}
}

func wantPercent(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.05 {
		t.Errorf("%s = %.2f%%, want %.1f%%", what, got, want)
	}
}

// One security in two accounts is one row: shares, value and cost add up,
// and the split lists both accounts, largest first.
func TestHoldings_OneRowPerSecurityAcrossAccounts(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	hsa := mkAccount(t, e.accts, "Cedar HSA Investment", account.TypeHSAInvestment, "USD", "0")
	e.value(hsa, "0", priced(acme, "10", "50.00", "400.00"))
	e.value(brokerage, "0", priced(acme, "30", "50.00", "1000.00"))

	sec := onlySection(t, e.report(t))
	if len(sec.Rows) != 1 {
		t.Fatalf("rows = %+v, want one ACME row", sec.Rows)
	}
	row := sec.Rows[0]
	if row.Label != "ACME" || row.Name != "Acme Total Market Index" || row.SecurityID != acme.ID {
		t.Errorf("row = %+v, want ACME", row)
	}
	if !row.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("shares = %s, want 40", row.Shares)
	}
	wantMoney(t, "price", row.Price, "50.00")
	wantMoney(t, "value", row.Value, "2000.00")
	wantMoney(t, "cost basis", row.CostBasis, "1400.00")
	wantMoney(t, "gain", row.Gain, "600.00")
	wantPercent(t, "percent", row.Percent, 100)

	if len(row.Accounts) != 2 {
		t.Fatalf("accounts = %+v, want 2", row.Accounts)
	}
	first, second := row.Accounts[0], row.Accounts[1]
	if first.AccountID != brokerage.ID || second.AccountID != hsa.ID {
		t.Errorf("split = %s, %s; want the brokerage (larger) first", first.Name, second.Name)
	}
	if !first.Shares.Equal(types.MustNewQuantity("30")) {
		t.Errorf("brokerage shares = %s, want 30", first.Shares)
	}
	wantMoney(t, "brokerage value", first.Value, "1500.00")
	wantPercent(t, "brokerage percent", first.Percent, 75)
	wantPercent(t, "HSA percent", second.Percent, 25)
}

// The Cash row is the cash of all the accounts. It is part of the 100%, its
// cost basis is the cash, and it has no gain. Its split has only the
// accounts that hold cash.
func TestHoldings_CashRow(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	roth := mkAccount(t, e.accts, "Maple Invest Roth IRA", account.TypeInvestment, "USD", "0")
	k401 := mkAccount(t, e.accts, "Birch 401k", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "150.00", priced(acme, "10", "75.00", "500.00"))
	e.value(roth, "100.00")
	e.value(k401, "0")

	sec := onlySection(t, e.report(t))
	cash := rowByLabel(t, sec, "Cash")
	if !cash.Cash || cash.SecurityID != types.NilID || cash.Name != "Uninvested cash" {
		t.Errorf("cash row = %+v", cash)
	}
	wantMoney(t, "cash value", cash.Value, "250.00")
	wantMoney(t, "cash cost basis", cash.CostBasis, "250.00")
	wantMoney(t, "cash gain", cash.Gain, "0")
	wantPercent(t, "cash percent", cash.Percent, 25)
	if len(cash.Accounts) != 2 || cash.Accounts[0].AccountID != brokerage.ID || cash.Accounts[1].AccountID != roth.ID {
		t.Errorf("cash split = %+v, want the brokerage then the Roth, no 401k", cash.Accounts)
	}
	wantPercent(t, "brokerage share of cash", cash.Accounts[0].Percent, 60)

	wantMoney(t, "section value", sec.Value, "1000.00")
	wantMoney(t, "section cost basis", sec.CostBasis, "750.00")
	wantMoney(t, "section gain", sec.Gain, "250.00")
	if !sec.Available || sec.Estimated {
		t.Errorf("section = %+v, want available and not estimated", sec)
	}

	t.Run("no cash, no Cash row", func(t *testing.T) {
		e.value(brokerage, "0", priced(acme, "10", "75.00", "500.00"))
		e.value(roth, "0")
		for _, r := range onlySection(t, e.report(t)).Rows {
			if r.Cash {
				t.Errorf("rows = %+v, want no Cash row", r)
			}
		}
	})
}

// The rows sort by value, largest first, with the Cash row among them. Equal
// values sort by label. The percentages add up to 100.
func TestHoldings_SortAndPercent(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	glbx := e.security(t, "GLBX", "Globex International Index")
	initech := e.security(t, "INIT", "Initech Corp")
	acct := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(acct, "300.00",
		priced(initech, "1", "300.00", "200.00"),
		priced(glbx, "2", "150.00", "250.00"),
		priced(acme, "10", "40.00", "350.00"),
	)

	sec := onlySection(t, e.report(t))
	var labels []string
	sum := 0.0
	for _, r := range sec.Rows {
		labels = append(labels, r.Label)
		sum += r.Percent
	}
	want := []string{"ACME", "Cash", "GLBX", "INIT"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels = %v, want %v", labels, want)
		}
	}
	wantPercent(t, "sum of percentages", sum, 100)
	wantPercent(t, "ACME percent", sec.Rows[0].Percent, 400.0/1300*100)
	wantMoney(t, "GLBX gain", rowByLabel(t, sec, "GLBX").Gain, "50.00")
}

// A security with no ticker shows its name. A hidden security that is still
// held is a row like any other.
func TestHoldings_LabelsAndHiddenSecurities(t *testing.T) {
	e := newHoldingsEnv(t)
	fund := e.security(t, "", "Cedar 2045 Target Fund")
	old := e.security(t, "STRK", "Stark Industries")
	old.Hidden = true
	if err := e.secs.Update(old); err != nil {
		t.Fatal(err)
	}
	acct := mkAccount(t, e.accts, "Birch 401k", account.TypeInvestment, "USD", "0")
	e.value(acct, "0", priced(fund, "100", "31.00", "3000.00"), priced(old, "5", "20.00", "50.00"))

	sec := onlySection(t, e.report(t))
	if r := rowByLabel(t, sec, "Cedar 2045 Target Fund"); r.Name != "Cedar 2045 Target Fund" {
		t.Errorf("tickerless row = %+v", r)
	}
	rowByLabel(t, sec, "STRK")
}

// A security with no price is valued at its cost basis. It has no price and
// no gain, and it marks the section estimated.
func TestHoldings_UnpricedIsEstimated(t *testing.T) {
	e := newHoldingsEnv(t)
	strk := e.security(t, "STRK", "Stark Industries")
	acct := mkAccount(t, e.accts, "Maple Invest Roth IRA", account.TypeInvestment, "USD", "0")
	e.value(acct, "0", unpriced(strk, "25", "2500.00"))

	sec := onlySection(t, e.report(t))
	row := rowByLabel(t, sec, "STRK")
	if !row.Estimated || !sec.Estimated {
		t.Errorf("row estimated = %v, section estimated = %v; want both", row.Estimated, sec.Estimated)
	}
	wantMoney(t, "value", row.Value, "2500.00")
	wantMoney(t, "price", row.Price, "0")
	wantMoney(t, "gain", row.Gain, "0")
}

// Only the accounts net worth values today count: active investment accounts
// opened by today. Closed, register and not-yet-opened accounts are left out.
func TestHoldings_AccountSet(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	open := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(open, "0", priced(acme, "1", "100.00", "100.00"))

	closed := mkAccount(t, e.accts, "Old Brokerage", account.TypeInvestment, "USD", "0")
	e.value(closed, "0", priced(acme, "50", "100.00", "100.00"))
	closed.Active = false
	if err := e.accts.Update(closed); err != nil {
		t.Fatal(err)
	}

	future := account.NewAccount("Next Year IRA", account.TypeInvestment, "USD", types.ZeroMoney,
		types.Today().AddDays(30))
	if err := e.accts.Create(future); err != nil {
		t.Fatal(err)
	}
	e.value(future, "0", priced(acme, "70", "100.00", "100.00"))

	mkAccount(t, e.accts, "Contoso Checking", account.TypeChecking, "USD", "900.00")

	sec := onlySection(t, e.report(t))
	wantMoney(t, "section value", sec.Value, "100.00")
	if len(sec.Rows) != 1 || len(sec.Rows[0].Accounts) != 1 || sec.Rows[0].Accounts[0].AccountID != open.ID {
		t.Errorf("rows = %+v, want ACME from the open brokerage only", sec.Rows)
	}
}

// Two currencies are two sections, sorted by code, and are never added.
func TestHoldings_SectionPerCurrency(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	usd := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	eur := mkAccount(t, e.accts, "Fabrikam Depot", account.TypeInvestment, "EUR", "0")
	e.value(usd, "10.00", priced(acme, "1", "90.00", "50.00"))
	e.value(eur, "40.00")

	rpt := e.report(t)
	if len(rpt.Sections) != 2 || rpt.Sections[0].Currency != "EUR" || rpt.Sections[1].Currency != "USD" {
		t.Fatalf("sections = %+v, want EUR then USD", rpt.Sections)
	}
	wantMoney(t, "EUR value", rpt.Sections[0].Value, "40.00")
	wantMoney(t, "USD value", rpt.Sections[1].Value, "100.00")
}

// An account that cannot be valued is listed as failed and makes its
// currency's section not available. The other accounts are still added, and
// the other currency is not touched.
func TestHoldings_ValuationFailure(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.security(t, "ACME", "Acme Total Market Index")
	good := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	bad := mkAccount(t, e.accts, "Birch 401k", account.TypeInvestment, "USD", "0")
	eur := mkAccount(t, e.accts, "Fabrikam Depot", account.TypeInvestment, "EUR", "0")
	e.value(good, "0", priced(acme, "1", "100.00", "100.00"))
	e.value(eur, "40.00")
	e.valuer.errs[bad.ID] = errors.New("price lookup failed")

	rpt := e.report(t)
	if len(rpt.Failed) != 1 || rpt.Failed[0].AccountID != bad.ID || rpt.Failed[0].Currency != "USD" {
		t.Fatalf("failed = %+v, want the 401k", rpt.Failed)
	}
	if rpt.Failed[0].Err == nil {
		t.Error("failure has no error")
	}
	eurSec, usdSec := rpt.Sections[0], rpt.Sections[1]
	if !eurSec.Available {
		t.Error("EUR section is not available; only USD had a failure")
	}
	if usdSec.Available {
		t.Error("USD section is available with a failed account")
	}
	wantMoney(t, "USD value of the valued accounts", usdSec.Value, "100.00")
	wantPercent(t, "ACME percent of the valued accounts", usdSec.Rows[0].Percent, 100)

	t.Run("no valuer", func(t *testing.T) {
		database := createTestDB(t)
		accts := account.NewRepository(database)
		mkAccount(t, accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
		rpt, err := NewService(accts, database).Holdings()
		if err != nil {
			t.Fatal(err)
		}
		if len(rpt.Failed) != 1 || !errors.Is(rpt.Failed[0].Err, ErrNoInvestmentValuer) {
			t.Errorf("failed = %+v, want ErrNoInvestmentValuer", rpt.Failed)
		}
	})
}

// With no investment account there is no section.
func TestHoldings_NoInvestmentAccounts(t *testing.T) {
	e := newHoldingsEnv(t)
	mkAccount(t, e.accts, "Contoso Checking", account.TypeChecking, "USD", "900.00")
	if rpt := e.report(t); len(rpt.Sections) != 0 || len(rpt.Failed) != 0 {
		t.Errorf("report = %+v, want empty", rpt)
	}
}
