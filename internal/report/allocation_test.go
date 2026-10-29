package report

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/chart"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// classed adds a security in an asset class.
func (e *holdingsEnv) classed(t *testing.T, ticker, name string, class security.AssetClass) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, name, security.TypeETF)
	sec.AssetClass = class
	if err := e.secs.Create(sec); err != nil {
		t.Fatal(err)
	}
	return sec
}

func (e *holdingsEnv) allocation(t *testing.T) *Allocation {
	t.Helper()
	rpt, err := e.svc.Allocation()
	if err != nil {
		t.Fatalf("Allocation() error = %v", err)
	}
	return rpt
}

func onlyAllocationSection(t *testing.T, rpt *Allocation) AllocationSection {
	t.Helper()
	if len(rpt.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(rpt.Sections))
	}
	return rpt.Sections[0]
}

func classOf(t *testing.T, sec AllocationSection, ac security.AssetClass) ClassRow {
	t.Helper()
	for _, c := range sec.Classes {
		if c.Class == ac {
			return c
		}
	}
	t.Fatalf("no class %s in %+v", ac, sec.Classes)
	return ClassRow{}
}

func classOrder(sec AllocationSection) []security.AssetClass {
	out := make([]security.AssetClass, len(sec.Classes))
	for i, c := range sec.Classes {
		out[i] = c.Class
	}
	return out
}

// The holdings rows carry the class of their security, and the Cash row is
// in the Cash class.
func TestHoldings_RowsCarryTheAssetClass(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.classed(t, "ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "25.00", priced(acme, "1", "75.00", "60.00"))

	sec := onlySection(t, e.report(t))
	if got := rowByLabel(t, sec, "ACME").AssetClass; got != security.AssetClassLargeCapStock {
		t.Errorf("ACME class = %s, want large_cap_stock", got)
	}
	if got := rowByLabel(t, sec, "Cash").AssetClass; got != security.AssetClassCash {
		t.Errorf("Cash row class = %s, want cash", got)
	}
}

// Two securities of one class are one class row: Value, Count and the
// percentages add up.
func TestAllocation_OneRowPerClass(t *testing.T) {
	e := newHoldingsEnv(t)
	midx := e.classed(t, "MIDX", "Maple Mid Cap Index", security.AssetClassMidCapStock)
	midv := e.classed(t, "MIDV", "Maple Mid Cap Value", security.AssetClassMidCapStock)
	bond := e.classed(t, "UMBR", "Umbrella Total Bond", security.AssetClassDomesticBond)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "0",
		priced(midx, "10", "30.00", "250.00"),
		priced(midv, "10", "15.00", "120.00"),
		priced(bond, "5", "50.00", "260.00"))

	sec := onlyAllocationSection(t, e.allocation(t))
	if len(sec.Classes) != 2 {
		t.Fatalf("classes = %v, want mid cap and domestic bond", classOrder(sec))
	}
	wantMoney(t, "section value", sec.Value, "700.00")
	mid := classOf(t, sec, security.AssetClassMidCapStock)
	wantMoney(t, "mid cap value", mid.Value, "450.00")
	wantPercent(t, "mid cap percent", mid.Percent, 450.0/700*100)
	if mid.Count() != 2 || sec.Count() != 3 {
		t.Errorf("counts = %d of %d, want 2 of 3", mid.Count(), sec.Count())
	}
	if mid.Holdings[0].Label != "MIDX" || mid.Holdings[1].Label != "MIDV" {
		t.Errorf("mid cap holdings = %+v, want MIDX then MIDV", mid.Holdings)
	}
	wantPercent(t, "MIDX of class", mid.Holdings[0].PercentOfClass, 300.0/450*100)
	wantPercent(t, "MIDX of total", mid.Holdings[0].PercentOfTotal, 300.0/700*100)
	wantPercent(t, "MIDV of class", mid.Holdings[1].PercentOfClass, 150.0/450*100)
}

// The Cash class is the uninvested cash plus each security of class Cash.
// The uninvested cash is not a security, so it does not count.
func TestAllocation_CashClass(t *testing.T) {
	e := newHoldingsEnv(t)
	mmkt := e.classed(t, "MAPXX", "Maple Money Market", security.AssetClassCash)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "40.00", priced(mmkt, "60", "1.00", "60.00"))

	cash := classOf(t, onlyAllocationSection(t, e.allocation(t)), security.AssetClassCash)
	wantMoney(t, "cash value", cash.Value, "100.00")
	if cash.Count() != 1 {
		t.Errorf("cash count = %d, want 1: the money market only", cash.Count())
	}
	if len(cash.Holdings) != 2 || cash.Holdings[0].Label != "MAPXX" || !cash.Holdings[1].Cash {
		t.Errorf("cash holdings = %+v, want MAPXX then the uninvested cash", cash.Holdings)
	}
}

// A holding with no price is in no value, count or percentage: it is in its
// class's NoPrice. A class whose holdings all have no price has Value 0, and
// it is not shown.
func TestAllocation_NoPrice(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.classed(t, "ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	strk := e.classed(t, "STRK", "Stark Industries", security.AssetClassLargeCapStock)
	coin := e.classed(t, "GLBC", "Globex Coin Trust", security.AssetClassCrypto)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "0",
		priced(acme, "4", "50.00", "150.00"),
		unpriced(strk, "3", "90.00"),
		unpriced(coin, "1", "500.00"))

	sec := onlyAllocationSection(t, e.allocation(t))
	wantMoney(t, "section value", sec.Value, "200.00")
	large := classOf(t, sec, security.AssetClassLargeCapStock)
	wantMoney(t, "large cap value", large.Value, "200.00")
	wantPercent(t, "large cap percent", large.Percent, 100)
	if large.Count() != 1 || len(large.NoPrice) != 1 || large.NoPrice[0].Label != "STRK" {
		t.Errorf("large cap = %+v, want ACME priced and STRK with no price", large)
	}
	wantPercent(t, "ACME of class", large.Holdings[0].PercentOfClass, 100)

	crypto := classOf(t, sec, security.AssetClassCrypto)
	wantMoney(t, "crypto value", crypto.Value, "0")
	if crypto.Count() != 0 || len(crypto.Holdings) != 0 || len(crypto.NoPrice) != 1 {
		t.Errorf("crypto = %+v, want only GLBC with no price", crypto)
	}
	shown := sec.Shown()
	if len(shown) != 1 || shown[0].Class != security.AssetClassLargeCapStock {
		t.Errorf("shown = %+v, want large cap only", shown)
	}
}

// Classes sort by value, largest first, and equal values by display name.
// Unclassified is last, also when it is the largest.
func TestAllocation_ClassOrder(t *testing.T) {
	e := newHoldingsEnv(t)
	mystery := e.classed(t, "MYST", "Mystery Fund", security.AssetClassUnclassified)
	reit := e.classed(t, "RLTY", "Cedar Realty Trust", security.AssetClassRealEstate)
	coin := e.classed(t, "GLBC", "Globex Coin Trust", security.AssetClassCrypto)
	gold := e.classed(t, "GOLD", "Initech Gold Trust", security.AssetClassCommodity)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "0",
		priced(mystery, "1", "900.00", "900.00"),
		priced(reit, "1", "100.00", "100.00"),
		priced(coin, "1", "100.00", "100.00"),
		priced(gold, "1", "300.00", "300.00"))

	got := classOrder(onlyAllocationSection(t, e.allocation(t)))
	want := []security.AssetClass{
		security.AssetClassCommodity,    // 300
		security.AssetClassCrypto,       // 100, "Crypto" < "Real Estate"
		security.AssetClassRealEstate,   // 100
		security.AssetClassUnclassified, // 900, but always last
	}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// With every account valued and every holding priced, the allocation total
// is the holdings total (rule 6), negative cash included.
func TestAllocation_TotalIsTheHoldingsTotal(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.classed(t, "ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	intl := e.classed(t, "GLBX", "Globex International", security.AssetClassInternationalStock)
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	roth := mkAccount(t, e.accts, "Maple Invest Roth IRA", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "-30.00", priced(acme, "12", "41.25", "300.00"))
	e.value(roth, "12.34", priced(intl, "7", "19.99", "100.00"), priced(acme, "1", "41.25", "40.00"))

	holdings := onlySection(t, e.report(t))
	sec := onlyAllocationSection(t, e.allocation(t))
	if !sec.Value.Equal(holdings.Value) {
		t.Errorf("allocation total = %s, holdings total = %s", sec.Value, holdings.Value)
	}
	wantMoney(t, "negative cash class", classOf(t, sec, security.AssetClassCash).Value, "-17.66")
}

// Two currencies are two sections, sorted by code.
func TestAllocation_SectionPerCurrency(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.classed(t, "ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	usd := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	eur := mkAccount(t, e.accts, "Fabrikam Depot", account.TypeInvestment, "EUR", "0")
	e.value(usd, "0", priced(acme, "1", "90.00", "50.00"))
	e.value(eur, "40.00")

	rpt := e.allocation(t)
	if len(rpt.Sections) != 2 || rpt.Sections[0].Currency != "EUR" || rpt.Sections[1].Currency != "USD" {
		t.Fatalf("sections = %+v, want EUR then USD", rpt.Sections)
	}
	wantMoney(t, "EUR value", rpt.Sections[0].Value, "40.00")
	wantMoney(t, "USD value", rpt.Sections[1].Value, "90.00")
}

// A failed account makes its section not Available and is copied to Failed.
// A currency whose every account failed keeps its section, with no class.
func TestAllocation_ValuationFailure(t *testing.T) {
	e := newHoldingsEnv(t)
	acme := e.classed(t, "ACME", "Acme Total Market Index", security.AssetClassLargeCapStock)
	good := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	bad := mkAccount(t, e.accts, "Birch 401k", account.TypeInvestment, "USD", "0")
	eur := mkAccount(t, e.accts, "Fabrikam Depot", account.TypeInvestment, "EUR", "0")
	e.value(good, "0", priced(acme, "1", "100.00", "100.00"))
	e.valuer.errs[bad.ID] = errors.New("price lookup failed")
	e.valuer.errs[eur.ID] = errors.New("price lookup failed")

	rpt := e.allocation(t)
	if len(rpt.Failed) != 2 || rpt.AccountCount != 3 {
		t.Fatalf("failed = %+v, accounts = %d; want 2 of 3", rpt.Failed, rpt.AccountCount)
	}
	eurSec, usdSec := rpt.Sections[0], rpt.Sections[1]
	if eurSec.Currency != "EUR" || eurSec.Available || len(eurSec.Classes) != 0 {
		t.Errorf("EUR section = %+v, want not available with no class", eurSec)
	}
	if usdSec.Available {
		t.Error("USD section is available with a failed account")
	}
	wantPercent(t, "large cap of the valued accounts", classOf(t, usdSec, security.AssetClassLargeCapStock).Percent, 100)
}

// No investment account, or accounts that hold nothing, give no section; the
// account count tells the two apart.
func TestAllocation_Empty(t *testing.T) {
	e := newHoldingsEnv(t)
	if rpt := e.allocation(t); len(rpt.Sections) != 0 || rpt.AccountCount != 0 {
		t.Errorf("report = %+v, want empty", rpt)
	}
	brokerage := mkAccount(t, e.accts, "Maple Invest Brokerage", account.TypeInvestment, "USD", "0")
	e.value(brokerage, "0")
	if rpt := e.allocation(t); len(rpt.Sections) != 0 || rpt.AccountCount != 1 {
		t.Errorf("report = %+v, want no section and one account", rpt)
	}
}

// A pie can show a section with a positive total and no negative class.
func TestAllocationSection_Drawable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		classes []int64
		want    bool
	}{
		{"all positive", []int64{100, 50}, true},
		{"a class of zero", []int64{100, 0}, true},
		{"negative cash", []int64{100, -5}, false},
		{"a total of zero", []int64{0}, false},
		{"no classes", nil, false},
	} {
		sec := AllocationSection{Value: types.ZeroMoney}
		for _, v := range tc.classes {
			sec.Classes = append(sec.Classes, ClassRow{Value: types.NewMoneyFromInt(v)})
			sec.Value = sec.Value.Add(types.NewMoneyFromInt(v))
		}
		if got := sec.Drawable(); got != tc.want {
			t.Errorf("%s: Drawable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The note says why there is no pie, and the slices leave out a class with
// no value.
func TestAllocationSection_NoPieNoteAndSlices(t *testing.T) {
	cash := ClassRow{Class: security.AssetClassCash, Value: types.NewMoneyFromInt(-5), Holdings: []ClassHolding{{}}}
	large := ClassRow{Class: security.AssetClassLargeCapStock, Value: types.NewMoneyFromInt(100), Holdings: []ClassHolding{{}}}
	empty := ClassRow{Class: security.AssetClassCrypto, Value: types.ZeroMoney, Holdings: []ClassHolding{{}}}
	if got := (AllocationSection{Classes: []ClassRow{large, cash}}).NoPieNote(); got != "No pie: Cash is negative." {
		t.Errorf("negative cash: note = %q", got)
	}
	if got := (AllocationSection{Classes: []ClassRow{empty}}).NoPieNote(); got != "No pie: the total is zero." {
		t.Errorf("zero total: note = %q", got)
	}
	slices := (AllocationSection{Classes: []ClassRow{large, empty}}).PieSlices(true)
	if len(slices) != 1 || slices[0].Value != 100 || slices[0].Color != chart.ClassColor(security.AssetClassLargeCapStock, true) {
		t.Errorf("slices = %+v, want one Large Cap slice of 100 in the dark color", slices)
	}
}
