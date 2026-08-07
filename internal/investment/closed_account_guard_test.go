package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the freeze rule for investment accounts: a closed account is
// immutable for every cash/security/transfer mutation, but READ and MAINTENANCE
// paths (valuation, rebuild, lot backfill) must keep working — they fund the
// read-only register/portfolio views and startup healing of closed accounts.

func closeInvAccount(t *testing.T, repo *account.Repository, acct *account.Account) {
	t.Helper()
	acct.Close(types.NewDate(2000, time.February, 1))
	if err := repo.Update(acct); err != nil {
		t.Fatalf("failed to close test account: %v", err)
	}
}

func assertInvClosed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an AccountClosedError, got nil")
	}
	var aerr *account.AccountClosedError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *account.AccountClosedError, got %T: %v", err, err)
	}
}

func TestInvestmentService_CashOps_RejectedOnClosedAccount(t *testing.T) {
	svc, accountRepo := createTestService(t)
	acct := createInvAccount(t, accountRepo, "Brokerage")
	closeInvAccount(t, accountRepo, acct)

	date := types.NewDate(2000, time.March, 1)
	t.Run("Deposit", func(t *testing.T) {
		_, err := svc.Deposit(acct.ID, date, types.MustNewMoney("100.00"), "")
		assertInvClosed(t, err)
	})
	t.Run("Withdrawal", func(t *testing.T) {
		_, err := svc.Withdrawal(acct.ID, date, types.MustNewMoney("100.00"), "")
		assertInvClosed(t, err)
	})
	t.Run("Interest", func(t *testing.T) {
		_, err := svc.Interest(acct.ID, date, types.MustNewMoney("10.00"), "")
		assertInvClosed(t, err)
	})
	t.Run("Fee", func(t *testing.T) {
		_, err := svc.Fee(acct.ID, date, types.MustNewMoney("5.00"), "")
		assertInvClosed(t, err)
	})
}

func TestInvestmentService_SecurityOps_RejectedOnClosedAccount(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VNQ")
	closeInvAccount(t, env.accountRepo, acct)

	date := types.NewDate(2000, time.March, 1)
	price := types.MustNewMoney("50.00")
	qty := types.MustNewQuantity("1")

	t.Run("Buy", func(t *testing.T) {
		_, err := env.svc.Buy(acct.ID, sec.ID, date, qty, nil, &price, types.ZeroMoney, "")
		assertInvClosed(t, err)
	})
	t.Run("Sell", func(t *testing.T) {
		_, err := env.svc.Sell(acct.ID, sec.ID, date, qty, nil, &price, types.ZeroMoney, "", nil)
		assertInvClosed(t, err)
	})
	t.Run("Dividend", func(t *testing.T) {
		_, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("10.00"), "")
		assertInvClosed(t, err)
	})
}

func TestInvestmentService_DeleteAndClearedToggle_RejectedOnClosedAccount(t *testing.T) {
	svc, accountRepo := createTestService(t)
	acct := createInvAccount(t, accountRepo, "Brokerage")

	// Fund it while open, then close.
	txn, err := svc.Deposit(acct.ID, types.NewDate(2000, time.March, 1), types.MustNewMoney("100.00"), "")
	if err != nil {
		t.Fatalf("Deposit (open) error = %v", err)
	}
	closeInvAccount(t, accountRepo, acct)

	t.Run("DeleteTransaction refused and row preserved", func(t *testing.T) {
		assertInvClosed(t, svc.DeleteTransaction(txn.ID))
		// The early guard must run before any destructive reversal.
		if _, gerr := svc.repo.GetByID(txn.ID); gerr != nil {
			t.Errorf("transaction should still exist after a refused delete, got %v", gerr)
		}
	})
	t.Run("SetClearedStatus refused", func(t *testing.T) {
		assertInvClosed(t, svc.SetClearedStatus(txn.ID, true))
	})
}

// TestInvestmentService_UpdateTransferShares_RejectsClosedOldDestination guards
// against silently reversing/deleting the OLD destination leg of a share
// transfer when re-targeting it: a share-only account has a zero cash balance
// and can therefore be closed while still holding transferred-in shares.
func TestInvestmentService_UpdateTransferShares_RejectsClosedOldDestination(t *testing.T) {
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source")
	dstB := createInvAccount(t, env.accountRepo, "DestB")
	dstC := createInvAccount(t, env.accountRepo, "DestC")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2020, time.March, 1)
	price := types.MustNewMoney("50.00")

	if _, err := env.svc.Buy(src.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy error = %v", err)
	}
	res, err := env.svc.TransferShares(src.ID, dstB.ID, sec.ID, date, types.MustNewQuantity("5"), "", nil)
	if err != nil {
		t.Fatalf("TransferShares error = %v", err)
	}

	// Close the OLD destination while it holds the transferred-in shares.
	closeInvAccount(t, env.accountRepo, dstB)

	// Re-targeting the still-open source transfer to a new open account must be
	// refused because the OLD destination leg lives on a closed account.
	_, err = env.editSvc.UpdateTransferShares(res.SourceTransaction.ID, src.ID, dstC.ID, date, sec.ID, types.MustNewQuantity("5"), "", nil)
	assertInvClosed(t, err)

	// The old destination's leg must NOT have been reversed/deleted.
	dstTxns, lerr := env.svc.repo.ListByAccount(dstB.ID, TransactionFilter{})
	if lerr != nil {
		t.Fatalf("ListByAccount error = %v", lerr)
	}
	if len(dstTxns) == 0 {
		t.Error("the closed old-destination leg should be preserved after a refused edit")
	}
}

// TestInvestmentService_ReadsAllowedOnClosedAccount is the critical regression
// guard: closed investment accounts must remain valuable (the read-only
// register/portfolio views and net-worth --include-closed depend on it). The
// freeze gate must NOT live in getInvestmentAccount.
func TestInvestmentService_ReadsAllowedOnClosedAccount(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	if _, err := env.svc.Deposit(acct.ID, types.NewDate(2000, time.March, 1), types.MustNewMoney("100.00"), ""); err != nil {
		t.Fatalf("Deposit (open) error = %v", err)
	}
	closeInvAccount(t, env.accountRepo, acct)

	if _, err := env.valSvc.GetAccountValuation(acct.ID, types.Today(), ValuationOptions{}); err != nil {
		t.Fatalf("GetAccountValuation on a closed account should succeed, got %v", err)
	}
	if _, err := env.svc.GetCashBalance(acct.ID); err != nil {
		t.Fatalf("GetCashBalance on a closed account should succeed, got %v", err)
	}
}
