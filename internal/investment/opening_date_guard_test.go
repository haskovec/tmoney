package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the rule that an investment transaction may not be dated
// before its account's opening date. The shared helpers open accounts on
// 2000-01-01, so a 1999 date is the "before opening" case.

func TestService_Buy_RejectsDateBeforeAccountOpening(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VNQ")

	price := types.MustNewMoney("50.00")
	_, err := env.svc.Buy(acct.ID, sec.ID, types.NewDate(1999, time.December, 31),
		types.MustNewQuantity("1"), nil, &price, types.ZeroMoney, "")
	if err == nil {
		t.Fatal("expected Buy before the opening date to be rejected")
	}
	var derr *account.DateBeforeOpeningError
	if !errors.As(err, &derr) {
		t.Fatalf("expected *account.DateBeforeOpeningError, got %T: %v", err, err)
	}

	// A rejected buy must not leave any holding behind.
	total, terr := env.svc.TotalSharesForSecurity(sec.ID)
	if terr != nil {
		t.Fatalf("TotalSharesForSecurity() error = %v", terr)
	}
	if !total.IsZero() {
		t.Errorf("expected no shares after a rejected buy, got %s", total.String())
	}
}

func TestService_Deposit_RejectsDateBeforeAccountOpening(t *testing.T) {
	svc, accountRepo := createTestService(t)
	acct := createInvAccount(t, accountRepo, "Brokerage")

	_, err := svc.Deposit(acct.ID, types.NewDate(1999, time.December, 31), types.MustNewMoney("1000.00"), "")
	if err == nil {
		t.Fatal("expected Deposit before the opening date to be rejected")
	}
	var derr *account.DateBeforeOpeningError
	if !errors.As(err, &derr) {
		t.Fatalf("expected *account.DateBeforeOpeningError, got %T: %v", err, err)
	}
}

func TestService_Buy_AllowsDateOnOpeningDate(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VNQ")

	// 2000-01-01 is the opening date; the inclusive boundary must be allowed.
	if _, err := env.svc.Deposit(acct.ID, types.NewDate(2000, time.January, 1), types.MustNewMoney("1000.00"), ""); err != nil {
		t.Fatalf("Deposit on the opening date should be allowed, got %v", err)
	}
	price := types.MustNewMoney("50.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, types.NewDate(2000, time.January, 1),
		types.MustNewQuantity("1"), nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy on the opening date should be allowed, got %v", err)
	}
}
