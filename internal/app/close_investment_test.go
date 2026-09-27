package app

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// closeEnv is a brokerage on the real service graph, so Close reads the
// investment ledger through the wiring NewServices does.
type closeEnv struct {
	svc   *Services
	acct  *account.Account
	secID types.ID
}

func newCloseEnv(t *testing.T, trackLots bool) closeEnv {
	t.Helper()
	svc := NewServices(createTestDB(t))
	acct := account.NewAccount("Northwind Brokerage", account.TypeInvestment, "USD",
		types.ZeroMoney, types.NewDate(2024, time.January, 1))
	acct.TrackLots = trackLots
	if err := svc.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	return closeEnv{svc: svc, acct: acct, secID: sec.ID}
}

func (e closeEnv) deposit(t *testing.T, amount string, date types.Date) {
	t.Helper()
	if _, err := e.svc.Investment.Deposit(e.acct.ID, date, types.MustNewMoney(amount), ""); err != nil {
		t.Fatal(err)
	}
}

// buyAll spends $500 of cash on 5 shares, leaving the cash at zero.
func (e closeEnv) buyAll(t *testing.T) {
	t.Helper()
	date := types.NewDate(2024, time.February, 1)
	e.deposit(t, "500.00", date)
	total := types.MustNewMoney("500.00")
	if _, err := e.svc.Investment.Buy(e.acct.ID, e.secID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatal(err)
	}
}

func (e closeEnv) assertOpen(t *testing.T) {
	t.Helper()
	acct, err := e.svc.Account.GetByID(e.acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !acct.Active {
		t.Error("the brokerage was closed")
	}
}

// A brokerage's register balance is always zero. Before W5a, Close read only
// that, so a brokerage with shares or cash closed and froze them.
func TestClose_Brokerage_RefusedWhileNotEmpty(t *testing.T) {
	for _, trackLots := range []bool{false, true} {
		name := map[bool]string{false: "positions", true: "lots"}[trackLots]

		t.Run(name+"/shares, no cash", func(t *testing.T) {
			e := newCloseEnv(t, trackLots)
			e.buyAll(t)

			err := e.svc.Account.Close(e.acct.ID, types.Today())
			var balErr *account.HasBalanceError
			if !errors.As(err, &balErr) || !balErr.HoldsShares || !balErr.Balance.IsZero() {
				t.Fatalf("Close() error = %v, want HasBalanceError holding shares with no cash", err)
			}
			e.assertOpen(t)
		})

		t.Run(name+"/cash, no shares", func(t *testing.T) {
			e := newCloseEnv(t, trackLots)
			e.deposit(t, "125.00", types.NewDate(2024, time.February, 1))

			err := e.svc.Account.Close(e.acct.ID, types.Today())
			var balErr *account.HasBalanceError
			if !errors.As(err, &balErr) || balErr.HoldsShares || !balErr.Balance.Equal(types.MustNewMoney("125.00")) {
				t.Fatalf("Close() error = %v, want HasBalanceError with cash 125.00", err)
			}
			e.assertOpen(t)
		})
	}
}

func TestClose_Brokerage_EmptyCloses(t *testing.T) {
	t.Run("never used", func(t *testing.T) {
		e := newCloseEnv(t, false)
		if err := e.svc.Account.Close(e.acct.ID, types.Today()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	// Rows that went back to zero — a sold-out position or a closed lot —
	// are not holdings.
	for _, trackLots := range []bool{false, true} {
		t.Run(map[bool]string{false: "sold out, positions", true: "sold out, lots"}[trackLots], func(t *testing.T) {
			e := newCloseEnv(t, trackLots)
			e.buyAll(t)
			var alloc []investment.SellLotAllocation
			if trackLots {
				lots, err := e.svc.LotRepo.ListByAccountAndSecurity(e.acct.ID, e.secID, false)
				if err != nil || len(lots) != 1 {
					t.Fatalf("lots = %d, %v; want 1", len(lots), err)
				}
				alloc = []investment.SellLotAllocation{{LotID: lots[0].ID, Shares: lots[0].Shares}}
			}
			date := types.NewDate(2024, time.March, 1)
			proceeds := types.MustNewMoney("500.00")
			if _, err := e.svc.Investment.Sell(e.acct.ID, e.secID, date, types.MustNewQuantity("5"), &proceeds, nil, types.ZeroMoney, "", alloc); err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.Investment.Withdrawal(e.acct.ID, date, proceeds, ""); err != nil {
				t.Fatal(err)
			}

			if err := e.svc.Account.Close(e.acct.ID, types.Today()); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

// The close date must not fall before the last row on the investment ledger.
func TestClose_Brokerage_DateAfterLastInvestmentRow(t *testing.T) {
	e := newCloseEnv(t, false)
	e.deposit(t, "100.00", types.NewDate(2024, time.March, 1))
	last := types.NewDate(2024, time.June, 1)
	if _, err := e.svc.Investment.Withdrawal(e.acct.ID, last, types.MustNewMoney("100.00"), ""); err != nil {
		t.Fatal(err)
	}

	err := e.svc.Account.Close(e.acct.ID, types.NewDate(2024, time.May, 1))
	var dateErr *account.InvalidCloseDateError
	if !errors.As(err, &dateErr) {
		t.Fatalf("Close() error = %v, want InvalidCloseDateError", err)
	}
	if !dateErr.Earliest.Equal(last) {
		t.Errorf("earliest close date = %s, want %s", dateErr.Earliest, last)
	}
}

// The TUI closes through the undo command. It must hit the same rule.
func TestClose_Brokerage_UndoCommandUsesTheSameRule(t *testing.T) {
	e := newCloseEnv(t, false)
	e.buyAll(t)

	err := undo.NewCloseAccountCommand(e.svc.Account, e.acct.ID, types.Today()).Execute()
	var balErr *account.HasBalanceError
	if !errors.As(err, &balErr) || !balErr.HoldsShares {
		t.Fatalf("Execute() error = %v, want HasBalanceError holding shares", err)
	}
	e.assertOpen(t)
}
