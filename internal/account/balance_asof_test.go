package account

import (
	"testing"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// insertTxn inserts a minimal transaction row directly (the account package
// cannot import the transaction package in tests without an import cycle).
func insertTxn(t *testing.T, r *Repository, acctID types.ID, date types.Date, amount string, status string) {
	t.Helper()
	amt := types.MustNewMoney(amount)
	_, err := r.db.Conn().Exec(
		`INSERT INTO transactions (id, account_id, date, amount, status) VALUES (?, ?, ?, ?, ?)`,
		types.NewID(), acctID, date, amt, status,
	)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}
}

func TestRepository_BalanceAsOf(t *testing.T) {
	t.Run("opening balance only when no transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		acct := NewAccount("Loan", TypeLoan, "USD",
			types.MustNewMoney("-250000.00"), types.NewDate(2024, 1, 1))
		if err := repo.Create(acct); err != nil {
			t.Fatalf("create account: %v", err)
		}

		bal, err := repo.BalanceAsOf(acct.ID, types.NewDate(2026, 8, 1))
		if err != nil {
			t.Fatalf("BalanceAsOf: %v", err)
		}
		if !bal.Equal(types.MustNewMoney("-250000.00")) {
			t.Errorf("balance = %s, want -250000.00", bal.String())
		}
	})

	t.Run("sums non-void transactions on or before the date", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		acct := NewAccount("Loan", TypeLoan, "USD",
			types.MustNewMoney("-100000.00"), types.NewDate(2024, 1, 1))
		if err := repo.Create(acct); err != nil {
			t.Fatalf("create account: %v", err)
		}

		// Two principal payments (positive amounts move a liability toward zero).
		insertTxn(t, repo, acct.ID, types.NewDate(2026, 6, 1), "500.00", "cleared")
		insertTxn(t, repo, acct.ID, types.NewDate(2026, 7, 1), "500.00", "uncleared")
		// A future payment that must NOT be counted as of 2026-07-15.
		insertTxn(t, repo, acct.ID, types.NewDate(2026, 8, 1), "500.00", "cleared")
		// A void payment that must never count.
		insertTxn(t, repo, acct.ID, types.NewDate(2026, 6, 15), "9999.00", "void")

		bal, err := repo.BalanceAsOf(acct.ID, types.NewDate(2026, 7, 15))
		if err != nil {
			t.Fatalf("BalanceAsOf: %v", err)
		}
		// -100000 + 500 + 500 = -99000 (future + void excluded).
		if !bal.Equal(types.MustNewMoney("-99000.00")) {
			t.Errorf("balance = %s, want -99000.00", bal.String())
		}
	})

	t.Run("includes a transaction dated exactly on the as-of date", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		acct := NewAccount("Loan", TypeLoan, "USD",
			types.MustNewMoney("-1000.00"), types.NewDate(2024, 1, 1))
		if err := repo.Create(acct); err != nil {
			t.Fatalf("create account: %v", err)
		}
		insertTxn(t, repo, acct.ID, types.NewDate(2026, 8, 1), "1000.00", "cleared")

		bal, err := repo.BalanceAsOf(acct.ID, types.NewDate(2026, 8, 1))
		if err != nil {
			t.Fatalf("BalanceAsOf: %v", err)
		}
		if !bal.IsZero() {
			t.Errorf("balance = %s, want 0", bal.String())
		}
	})

	t.Run("unknown account returns NotFoundError", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		_, err := repo.BalanceAsOf(types.NewID(), types.NewDate(2026, 1, 1))
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_Balance(t *testing.T) {
	t.Run("counts all non-void transactions regardless of date", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		acct := NewAccount("Loan", TypeLoan, "USD",
			types.MustNewMoney("-1000.00"), types.NewDate(2024, 1, 1))
		if err := repo.Create(acct); err != nil {
			t.Fatalf("create account: %v", err)
		}
		// A future-dated final payment that pays the loan off.
		insertTxn(t, repo, acct.ID, types.NewDate(2099, 1, 1), "1000.00", "cleared")
		insertTxn(t, repo, acct.ID, types.NewDate(2030, 1, 1), "5000.00", "void")

		bal, err := repo.Balance(acct.ID)
		if err != nil {
			t.Fatalf("Balance: %v", err)
		}
		if !bal.IsZero() {
			t.Errorf("full balance = %s, want 0 (future payment counted, void excluded)", bal.String())
		}
	})

	t.Run("unknown account returns NotFoundError", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		_, err := repo.Balance(types.NewID())
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}
