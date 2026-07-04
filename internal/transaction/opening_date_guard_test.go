package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the rule that a transaction may not be dated before its
// account's opening date — the guard that turns a mistyped year (e.g. "0018"
// for "2018") into an immediate, fixable rejection instead of corrupt data.

func TestTransactionService_Create_RejectsDateBeforeOpening(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	// createTestAccountOfType opens accounts on 2000-01-01.
	acct := createTestAccount(t, accountRepo, "Checking")

	txn := NewTransaction(acct.ID, types.NewDate(1999, time.December, 31), types.MustNewMoney("-50.00"))
	err := svc.Create(txn)
	if err == nil {
		t.Fatal("expected Create to reject a date before the account's opening date")
	}
	var derr *account.DateBeforeOpeningError
	if !errors.As(err, &derr) {
		t.Fatalf("expected *account.DateBeforeOpeningError, got %T: %v", err, err)
	}

	// Nothing should have been persisted.
	txns, lerr := svc.txnRepo.ListByAccount(acct.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount() error = %v", lerr)
	}
	if len(txns) != 0 {
		t.Errorf("expected no transactions after a rejected create, got %d", len(txns))
	}
}

func TestTransactionService_Create_AllowsDateOnOpeningDate(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	acct := createTestAccount(t, accountRepo, "Checking")

	// The opening date itself is allowed (inclusive boundary).
	txn := NewTransaction(acct.ID, types.NewDate(2000, time.January, 1), types.MustNewMoney("-50.00"))
	if err := svc.Create(txn); err != nil {
		t.Fatalf("Create on the opening date should be allowed, got %v", err)
	}
}

func TestTransactionService_CreateTransfer_RejectsDateBeforeOpening(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	from := createTestAccount(t, accountRepo, "Checking")
	to := createTestAccount(t, accountRepo, "Savings")

	_, err := svc.CreateTransfer(from.ID, to.ID, types.NewDate(1999, time.December, 31), types.MustNewMoney("100.00"), "", types.NullableID{})
	if err == nil {
		t.Fatal("expected CreateTransfer to reject a date before an account's opening date")
	}
	var derr *account.DateBeforeOpeningError
	if !errors.As(err, &derr) {
		t.Fatalf("expected *account.DateBeforeOpeningError, got %T: %v", err, err)
	}
}
