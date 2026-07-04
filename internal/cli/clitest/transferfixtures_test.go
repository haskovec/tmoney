package clitest

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

func TestSetupTransferAccounts(t *testing.T) {
	path, checking, savings := SetupTransferAccounts(t)
	if checking == nil || checking.Name != "Checking" {
		t.Fatalf("expected Checking account, got %+v", checking)
	}
	if savings == nil || savings.Name != "Savings" {
		t.Fatalf("expected Savings account, got %+v", savings)
	}

	repo := account.NewRepository(reopen(t, path))
	for _, name := range []string{"Checking", "Savings"} {
		if _, err := repo.GetByName(name); err != nil {
			t.Errorf("account %q not persisted: %v", name, err)
		}
	}
}

func TestSetupTransferDispatchAccounts(t *testing.T) {
	path, checking, brokerage, ira, hsa := SetupTransferDispatchAccounts(t)
	for _, a := range []*account.Account{checking, brokerage, ira, hsa} {
		if a == nil {
			t.Fatal("dispatch fixture returned a nil account")
		}
	}

	repo := account.NewRepository(reopen(t, path))
	for _, name := range []string{"Checking", "Brokerage", "Rollover IRA", "HSA"} {
		if _, err := repo.GetByName(name); err != nil {
			t.Errorf("account %q not persisted: %v", name, err)
		}
	}
}

func TestOpenSvcAndFindInvestmentLeg(t *testing.T) {
	path, _, brokerage, _, _ := SetupTransferDispatchAccounts(t)

	svc := OpenSvc(t, path)
	if svc == nil {
		t.Fatal("OpenSvc returned nil services")
	}

	checking, err := svc.Account.GetByName("Checking")
	if err != nil {
		t.Fatalf("GetByName Checking: %v", err)
	}

	// Create an investment-side transfer leg (inv→reg cash transfer) so the
	// account has exactly one investment transaction to find.
	res, err := svc.Investment.TransferCash(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("250.00"), "draw", types.NullableID{})
	if err != nil {
		t.Fatalf("TransferCash: %v", err)
	}

	got := FindInvestmentLegForTest(t, svc, brokerage.ID)
	if got != res.InvestmentTransaction.ID {
		t.Errorf("FindInvestmentLegForTest = %s, want %s", got, res.InvestmentTransaction.ID)
	}
}
