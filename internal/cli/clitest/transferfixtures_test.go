package clitest

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
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
