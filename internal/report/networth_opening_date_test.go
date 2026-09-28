package report

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// countingValuer records which accounts were valued.
type countingValuer struct{ calls map[types.ID]int }

func (v *countingValuer) GetAccountValuation(id types.ID, _ types.Date) (*ValuationResult, error) {
	v.calls[id]++
	return &ValuationResult{}, nil
}

// An account that was not open yet on the as-of date is not in that date's
// net worth: not as a row, not in any total, and a brokerage is not valued.
func TestNetWorthAsOf_LeavesOutAccountsNotYetOpen(t *testing.T) {
	valuer := &countingValuer{calls: map[types.ID]int{}}
	svc, repo := newFigureEnv(t, valuer)
	asOf := types.NewDate(2021, time.June, 30)

	create := func(name string, typ account.Type, opening string, opened types.Date) *account.Account {
		t.Helper()
		acct := account.NewAccount(name, typ, "USD", types.MustNewMoney(opening), opened)
		if err := repo.Create(acct); err != nil {
			t.Fatal(err)
		}
		return acct
	}
	old := create("Contoso Checking", account.TypeChecking, "100.00", types.NewDate(2020, time.January, 1))
	sameDay := create("Contoso Savings", account.TypeSavings, "10.00", asOf)
	later := create("Fabrikam Checking", account.TypeChecking, "5000.00", types.NewDate(2025, time.March, 1))
	laterBrokerage := create("Northwind Brokerage", account.TypeInvestment, "0", types.NewDate(2025, time.March, 1))

	nw, err := svc.NetWorthAsOf(asOf.Time())
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, r := range nw.Assets {
		names[r.Name] = true
	}
	for _, want := range []*account.Account{old, sameDay} {
		if !names[want.Name] {
			t.Errorf("%s (opened on or before the date) is missing", want.Name)
		}
	}
	for _, unwanted := range []*account.Account{later, laterBrokerage} {
		if names[unwanted.Name] {
			t.Errorf("%s (opened after the date) is in the report", unwanted.Name)
		}
	}
	if total := onlyTotal(t, nw); !total.NetWorth.Equal(types.MustNewMoney("110.00")) {
		t.Errorf("net worth = %s, want 110.00 (100.00 + 10.00, not the 5000.00 opened later)", total.NetWorth)
	}
	if n := valuer.calls[laterBrokerage.ID]; n != 0 {
		t.Errorf("the brokerage opened later was valued %d time(s)", n)
	}

	t.Run("including closed accounts", func(t *testing.T) {
		nw, err := svc.NetWorthAsOfIncludingClosed(asOf.Time())
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range nw.Assets {
			if r.Name == later.Name || r.Name == laterBrokerage.Name {
				t.Errorf("%s (opened after the date) is in the report", r.Name)
			}
		}
	})
}
