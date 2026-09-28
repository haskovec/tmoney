package app

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// newDeleteEnv returns the service graph and its database handle. The raw
// handle builds the damaged states a real file can hold (a leg or a row gone)
// that no service would write.
func newDeleteEnv(t *testing.T) (*Services, *db.DB) {
	t.Helper()
	database := createTestDB(t)
	return NewServices(database), database
}

func createAccount(t *testing.T, svc *Services, name string, typ account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, typ, "USD", types.ZeroMoney, types.NewDate(2024, time.January, 1))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	return acct
}

// assertRefused checks that Delete returned the named dependents and left the
// account in place. A driver error (a foreign key firing inside DuckDB) fails
// it: every refusal must come from DeleteBlocker.
func assertRefused(t *testing.T, svc *Services, acct *account.Account, dependents string, count int) {
	t.Helper()
	err := svc.Account.Delete(acct.ID)
	var dep *dberrors.HasDependentsError
	if !errors.As(err, &dep) {
		t.Fatalf("Delete() error = %v, want HasDependentsError(%s)", err, dependents)
	}
	if dep.Dependents != dependents || dep.Count != count {
		t.Errorf("dependents = %d %q, want %d %q", dep.Count, dep.Dependents, count, dependents)
	}
	if _, err := svc.Account.GetByID(acct.ID); err != nil {
		t.Errorf("the account is gone after a refused delete: %v", err)
	}
}

func assertDeleted(t *testing.T, svc *Services, acct *account.Account) {
	t.Helper()
	if err := svc.Account.Delete(acct.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Account.GetByID(acct.ID); err == nil {
		t.Error("the account still exists")
	}
}

func TestDelete_Brokerage(t *testing.T) {
	t.Run("with a buy is refused", func(t *testing.T) {
		svc, _ := newDeleteEnv(t)
		acct := createAccount(t, svc, "Northwind Brokerage", account.TypeInvestment)
		sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
		if err := svc.Security.Create(sec); err != nil {
			t.Fatal(err)
		}
		date := types.NewDate(2024, time.February, 1)
		if _, err := svc.Investment.Deposit(acct.ID, date, types.MustNewMoney("500.00"), ""); err != nil {
			t.Fatal(err)
		}
		total := types.MustNewMoney("500.00")
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatal(err)
		}
		// Two rows (the deposit and the buy), not three: the lot and the
		// position are not transactions.
		assertRefused(t, svc, acct, "investment transactions", 2)
	})

	t.Run("holdings with no rows are refused", func(t *testing.T) {
		svc, database := newDeleteEnv(t)
		acct := createAccount(t, svc, "Northwind Brokerage", account.TypeInvestment)
		sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
		if err := svc.Security.Create(sec); err != nil {
			t.Fatal(err)
		}
		total := types.MustNewMoney("500.00")
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.NewDate(2024, time.February, 1), types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatal(err)
		}
		// A damaged file: the rows are gone, the position is not.
		if _, err := database.Conn().Exec(
			`DELETE FROM investment_transactions WHERE CAST(account_id AS VARCHAR) = ?`, acct.ID.String()); err != nil {
			t.Fatal(err)
		}
		assertRefused(t, svc, acct, "investment holdings", 1)
	})

	t.Run("empty is deleted", func(t *testing.T) {
		svc, _ := newDeleteEnv(t)
		assertDeleted(t, svc, createAccount(t, svc, "Northwind Brokerage", account.TypeInvestment))
	})
}

// A row in another account can name this one as its transfer partner — the
// surviving leg of a transfer whose other leg is gone. That row holds a
// foreign key to this account, so the delete must refuse it too.
func TestDelete_RefusedWhileAnotherAccountNamesIt(t *testing.T) {
	svc, database := newDeleteEnv(t)
	from := createAccount(t, svc, "Contoso Checking", account.TypeChecking)
	to := createAccount(t, svc, "Contoso Savings", account.TypeSavings)
	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.NewDate(2024, time.June, 1),
		Amount:        types.MustNewMoney("50.00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().Exec(
		`DELETE FROM transactions WHERE CAST(id AS VARCHAR) = ?`, res.To.RowID.String()); err != nil {
		t.Fatal(err)
	}

	assertRefused(t, svc, to, "transfer references", 1)
}

func TestDelete_ReconciliationSessions(t *testing.T) {
	t.Run("completed sessions go with the account", func(t *testing.T) {
		svc, database := newDeleteEnv(t)
		acct := createAccount(t, svc, "Contoso Checking", account.TypeChecking)
		if _, err := svc.Reconciliation.StartReconciliation(acct.ID, types.NewDate(2024, time.February, 1), types.ZeroMoney); err != nil {
			t.Fatal(err)
		}
		if err := svc.Reconciliation.FinishReconciliation(acct.ID, nil, false); err != nil {
			t.Fatal(err)
		}

		assertDeleted(t, svc, acct)
		var n int
		if err := database.Conn().QueryRow(
			`SELECT COUNT(*) FROM reconciliation_sessions WHERE CAST(account_id AS VARCHAR) = ?`, acct.ID.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("sessions left = %d, want 0", n)
		}
	})

	t.Run("an active session refuses", func(t *testing.T) {
		svc, _ := newDeleteEnv(t)
		acct := createAccount(t, svc, "Contoso Checking", account.TypeChecking)
		if _, err := svc.Reconciliation.StartReconciliation(acct.ID, types.NewDate(2024, time.February, 1), types.ZeroMoney); err != nil {
			t.Fatal(err)
		}

		assertRefused(t, svc, acct, "active reconciliation", 1)
		if s, err := svc.Reconciliation.GetActiveSession(acct.ID); err != nil || s == nil {
			t.Errorf("the active session is gone: %v, %v", s, err)
		}
	})
}
