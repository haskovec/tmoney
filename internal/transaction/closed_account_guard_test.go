package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the freeze rule: a closed account is immutable. No new
// transactions, edits, deletes, status toggles, or voids may touch it, and a
// transfer is blocked if either leg is closed. Reopen to make changes.

// closeTestAccount marks an account closed directly via the repository (the
// account service's zero-balance check is not relevant to the transaction
// freeze gate, which keys off Active).
func closeTestAccount(t *testing.T, repo *account.Repository, acct *account.Account) {
	t.Helper()
	acct.Close(types.NewDate(2000, time.February, 1))
	if err := repo.Update(acct); err != nil {
		t.Fatalf("failed to close test account: %v", err)
	}
}

func assertAccountClosed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an AccountClosedError, got nil")
	}
	var aerr *account.AccountClosedError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *account.AccountClosedError, got %T: %v", err, err)
	}
}

func TestTransactionService_Create_RejectsClosedAccount(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	acct := createTestAccount(t, accountRepo, "Checking")
	closeTestAccount(t, accountRepo, acct)

	txn := NewTransaction(acct.ID, types.NewDate(2000, time.March, 1), types.MustNewMoney("-50.00"))
	assertAccountClosed(t, svc.Create(txn))

	txns, lerr := svc.txnRepo.ListByAccount(acct.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount() error = %v", lerr)
	}
	if len(txns) != 0 {
		t.Errorf("expected no transactions after a rejected create, got %d", len(txns))
	}
}

func TestTransactionService_MutationsRejectedOnClosedAccount(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	acct := createTestAccount(t, accountRepo, "Checking")

	// Create a transaction while the account is still open.
	txn := NewTransaction(acct.ID, types.NewDate(2000, time.March, 1), types.MustNewMoney("-50.00"))
	if err := svc.Create(txn); err != nil {
		t.Fatalf("Create (open) error = %v", err)
	}

	closeTestAccount(t, accountRepo, acct)

	t.Run("Update", func(t *testing.T) {
		edited := NewTransaction(acct.ID, txn.Date, types.MustNewMoney("-60.00"))
		edited.ID = txn.ID
		assertAccountClosed(t, svc.Update(edited))
	})
	t.Run("ClearTransaction", func(t *testing.T) {
		assertAccountClosed(t, svc.ClearTransaction(txn.ID))
	})
	t.Run("MarkTransactionUncleared", func(t *testing.T) {
		assertAccountClosed(t, svc.MarkTransactionUncleared(txn.ID))
	})
	t.Run("VoidTransaction", func(t *testing.T) {
		assertAccountClosed(t, svc.VoidTransaction(txn.ID))
	})
	t.Run("AddSplit", func(t *testing.T) {
		split := NewSplit(txn.ID, types.NewID(), types.MustNewMoney("-50.00"))
		assertAccountClosed(t, svc.AddSplit(split))
	})
	t.Run("ReplaceSplits", func(t *testing.T) {
		split := NewSplit(txn.ID, types.NewID(), types.MustNewMoney("-50.00"))
		assertAccountClosed(t, svc.ReplaceSplits(txn.ID, []*Split{split}))
	})
	t.Run("Delete", func(t *testing.T) {
		assertAccountClosed(t, svc.Delete(txn.ID))
	})
}

func TestTransactionService_Transfer_RejectedWhenEitherLegClosed(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	open := createTestAccount(t, accountRepo, "Checking")
	closed := createTestAccount(t, accountRepo, "Savings")
	closeTestAccount(t, accountRepo, closed)

	date := types.NewDate(2000, time.March, 1)

	t.Run("CreateTransfer to closed leg", func(t *testing.T) {
		_, err := svc.CreateTransfer(open.ID, closed.ID, date, types.MustNewMoney("100.00"))
		assertAccountClosed(t, err)
	})
	t.Run("CreateTransfer from closed leg", func(t *testing.T) {
		_, err := svc.CreateTransfer(closed.ID, open.ID, date, types.MustNewMoney("100.00"))
		assertAccountClosed(t, err)
	})

	// Build a real transfer between two open accounts, then close one leg and
	// confirm edit/delete of the existing transfer is refused.
	other := createTestAccount(t, accountRepo, "Savings2")
	pair, err := svc.CreateTransfer(open.ID, other.ID, date, types.MustNewMoney("100.00"))
	if err != nil {
		t.Fatalf("CreateTransfer (open) error = %v", err)
	}
	closeTestAccount(t, accountRepo, other)

	transferID := pair.FromTransaction.TransferID.ID
	t.Run("UpdateTransferAmount with closed leg", func(t *testing.T) {
		assertAccountClosed(t, svc.UpdateTransferAmount(transferID, types.MustNewMoney("120.00")))
	})
	t.Run("DeleteTransfer with closed leg", func(t *testing.T) {
		assertAccountClosed(t, svc.DeleteTransfer(transferID))
	})
}

func TestTransactionService_Create_AllowsOpenAccountWhenAnotherClosed(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	open := createTestAccount(t, accountRepo, "Checking")
	closed := createTestAccount(t, accountRepo, "Savings")
	closeTestAccount(t, accountRepo, closed)

	txn := NewTransaction(open.ID, types.NewDate(2000, time.March, 1), types.MustNewMoney("-50.00"))
	if err := svc.Create(txn); err != nil {
		t.Fatalf("Create on an open account should succeed even when another account is closed, got %v", err)
	}
}
