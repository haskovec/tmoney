package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// TestRepository_UpdateStatus covers the narrow in-place status update used by
// reconcile-finish, the cleared/uncleared toggle, and un-reconcile. It must
// change only the status (and updated_at) and leave every other column intact.
func TestRepository_UpdateStatus(t *testing.T) {
	newTxn := func(t *testing.T) (*Repository, *Transaction) {
		t.Helper()
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		txnRepo := NewRepository(database)

		now := time.Now()
		date := types.NewDate(now.Year(), now.Month(), now.Day())
		acct := account.NewAccount("Test Account", account.TypeChecking, "USD", types.ZeroMoney, date)
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		txn := NewTransactionFull(acct.ID, date, types.MustNewMoney("-42.50"), types.ID{}, types.ID{}, "grocery run")
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Create transaction error = %v", err)
		}
		return txnRepo, txn
	}

	t.Run("changes status and leaves other fields intact", func(t *testing.T) {
		txnRepo, txn := newTxn(t)

		if err := txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus() error = %v", err)
		}

		got, err := txnRepo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.Status != StatusReconciled {
			t.Errorf("Status = %q, want %q", got.Status, StatusReconciled)
		}
		if !got.Amount.Equal(txn.Amount) {
			t.Errorf("Amount changed: got %s, want %s", got.Amount.String(), txn.Amount.String())
		}
		if got.Memo.String != "grocery run" {
			t.Errorf("Memo changed: got %q", got.Memo.String)
		}
		if !got.Date.Equal(txn.Date) {
			t.Errorf("Date changed: got %s, want %s", got.Date.String(), txn.Date.String())
		}
		if got.AccountID != txn.AccountID {
			t.Errorf("AccountID changed: got %v, want %v", got.AccountID, txn.AccountID)
		}
		if !got.UpdatedAt.Time().After(txn.UpdatedAt.Time()) && !got.UpdatedAt.Time().Equal(txn.UpdatedAt.Time()) {
			t.Errorf("UpdatedAt should advance: got %v, was %v", got.UpdatedAt.Time(), txn.UpdatedAt.Time())
		}
	})

	t.Run("round-trips through every status", func(t *testing.T) {
		txnRepo, txn := newTxn(t)
		for _, st := range []Status{StatusCleared, StatusReconciled, StatusUncleared} {
			if err := txnRepo.UpdateStatus(txn.ID, st); err != nil {
				t.Fatalf("UpdateStatus(%q) error = %v", st, err)
			}
			got, err := txnRepo.GetByID(txn.ID)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			if got.Status != st {
				t.Errorf("Status = %q, want %q", got.Status, st)
			}
		}
	})

	t.Run("missing transaction returns NotFoundError", func(t *testing.T) {
		txnRepo, _ := newTxn(t)
		err := txnRepo.UpdateStatus(types.NewID(), StatusReconciled)
		if err == nil {
			t.Fatal("UpdateStatus() on missing id should error")
		}
		var nf *dberrors.NotFoundError
		if !errors.As(err, &nf) {
			t.Errorf("error = %v, want NotFoundError", err)
		}
	})
}
