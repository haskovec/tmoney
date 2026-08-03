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

// The whole-transfer opening-date case moved with its subject: transfer.Service
// now guards BOTH legs against their own account's opening date, which this test
// could not check (it only had bank↔bank). See
// TestCreate_GuardMatrix/date_before_opening_date_either_side in
// internal/transfer, which runs all four shapes.
