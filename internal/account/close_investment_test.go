package account

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// fakeLedger is an InvestmentLedger that returns fixed values and counts
// its calls.
type fakeLedger struct {
	cash  types.Money
	held  bool
	err   error
	calls int
}

func (f *fakeLedger) LedgerState(types.ID) (types.Money, bool, error) {
	f.calls++
	return f.cash, f.held, f.err
}

func newCloseEnv(t *testing.T, typ Type, ledger InvestmentLedger) (*Service, *Account) {
	t.Helper()
	database := createTestDB(t)
	repo := NewRepository(database)
	var opts []ServiceOption
	if ledger != nil {
		opts = append(opts, WithInvestmentLedger(ledger))
	}
	svc := NewService(repo, database, opts...)
	acct := NewAccount("Northwind Brokerage", typ, "USD", types.ZeroMoney, types.MustParseDate("2024-01-01"))
	if err := svc.Create(acct); err != nil {
		t.Fatal(err)
	}
	return svc, acct
}

func assertStillOpen(t *testing.T, svc *Service, id types.ID) {
	t.Helper()
	acct, err := svc.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if !acct.Active {
		t.Error("the account was closed")
	}
}

// An investment account's register balance is always zero: its cash and
// shares live on the investment ledger. Close must ask that ledger.
func TestService_Close_InvestmentAccount(t *testing.T) {
	cases := []struct {
		name      string
		ledger    *fakeLedger
		wantShare bool
		wantCash  string
	}{
		{"shares, no cash", &fakeLedger{cash: types.ZeroMoney, held: true}, true, "0"},
		{"cash, no shares", &fakeLedger{cash: types.MustNewMoney("125.00"), held: false}, false, "125.00"},
		{"shares and cash", &fakeLedger{cash: types.MustNewMoney("125.00"), held: true}, true, "125.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, acct := newCloseEnv(t, TypeInvestment, tc.ledger)

			err := svc.Close(acct.ID, types.Today())
			var balErr *HasBalanceError
			if !errors.As(err, &balErr) {
				t.Fatalf("Close() error = %v, want a HasBalanceError", err)
			}
			if balErr.HoldsShares != tc.wantShare {
				t.Errorf("HoldsShares = %v, want %v", balErr.HoldsShares, tc.wantShare)
			}
			if !balErr.Balance.Equal(types.MustNewMoney(tc.wantCash)) {
				t.Errorf("Balance = %s, want %s", balErr.Balance, tc.wantCash)
			}
			assertStillOpen(t, svc, acct.ID)
		})
	}

	t.Run("empty closes", func(t *testing.T) {
		ledger := &fakeLedger{cash: types.ZeroMoney}
		svc, acct := newCloseEnv(t, TypeHSAInvestment, ledger)
		if err := svc.Close(acct.ID, types.Today()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if ledger.calls != 1 {
			t.Errorf("ledger calls = %d, want 1", ledger.calls)
		}
	})

	t.Run("ledger error is returned", func(t *testing.T) {
		boom := errors.New("boom")
		svc, acct := newCloseEnv(t, TypeInvestment, &fakeLedger{err: boom})
		if err := svc.Close(acct.ID, types.Today()); !errors.Is(err, boom) {
			t.Fatalf("Close() error = %v, want %v", err, boom)
		}
		assertStillOpen(t, svc, acct.ID)
	})

	t.Run("no ledger wired fails closed", func(t *testing.T) {
		svc, acct := newCloseEnv(t, TypeInvestment, nil)
		if err := svc.Close(acct.ID, types.Today()); !errors.Is(err, ErrNoInvestmentLedger) {
			t.Fatalf("Close() error = %v, want ErrNoInvestmentLedger", err)
		}
		assertStillOpen(t, svc, acct.ID)
	})
}

// A register account never asks the investment ledger.
func TestService_Close_RegisterAccountSkipsLedger(t *testing.T) {
	ledger := &fakeLedger{held: true}
	svc, acct := newCloseEnv(t, TypeChecking, ledger)
	if err := svc.Close(acct.ID, types.Today()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if ledger.calls != 0 {
		t.Errorf("ledger calls = %d, want 0", ledger.calls)
	}
}

func TestHasBalanceError_NamesTheCause(t *testing.T) {
	cases := []struct {
		err  HasBalanceError
		want string
	}{
		{HasBalanceError{ID: "a", Balance: types.MustNewMoney("5")}, "cannot close account a: has balance of 5"},
		{HasBalanceError{ID: "a", HoldsShares: true}, "cannot close account a: it holds shares"},
		{HasBalanceError{ID: "a", Balance: types.MustNewMoney("5"), HoldsShares: true}, "cannot close account a: it holds shares and has a cash balance of 5"},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}
