package investment

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// A single-row edit deletes one row and creates one row. On a transfer leg
// that would leave the other leg without its pair, so every single-row Update*
// must refuse a leg before it deletes anything.
func TestUpdate_RefusesCashTransferLeg(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	checking := createCheckAccount(t, env.accountRepo, "Checking")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2019, time.December, 19)

	leg := NewTransaction(acct.ID, date, TransactionTypeTransferCash, types.MustNewMoney("-400.00"))
	leg.SetTransfer(types.NewID(), checking.ID)
	if err := env.invRepo.Create(leg); err != nil {
		t.Fatal(err)
	}

	amount := types.MustNewMoney("400.00")
	price := types.MustNewMoney("100.00")
	edits := map[string]func() error{
		"UpdateWithdrawal": func() error {
			_, err := env.editSvc.UpdateWithdrawal(leg.ID, acct.ID, date, amount, "")
			return err
		},
		"UpdateDeposit": func() error {
			_, err := env.editSvc.UpdateDeposit(leg.ID, acct.ID, date, amount, "")
			return err
		},
		"UpdateFee": func() error {
			_, err := env.editSvc.UpdateFee(leg.ID, acct.ID, date, amount, "")
			return err
		},
		"UpdateInterest": func() error {
			_, err := env.editSvc.UpdateInterest(leg.ID, acct.ID, date, amount, "")
			return err
		},
		"UpdateDividend": func() error {
			_, err := env.editSvc.UpdateDividend(leg.ID, acct.ID, sec.ID, date, amount, "")
			return err
		},
		"UpdateBuy": func() error {
			_, err := env.editSvc.UpdateBuy(leg.ID, acct.ID, sec.ID, date, types.MustNewQuantity("1"), nil, &price, types.ZeroMoney, "")
			return err
		},
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			var legErr *IsCashTransferLegError
			if err := edit(); !errors.As(err, &legErr) {
				t.Fatalf("err = %v, want IsCashTransferLegError", err)
			}
			if _, err := env.invRepo.GetByID(leg.ID); err != nil {
				t.Fatalf("leg was deleted: %v", err)
			}
		})
	}
}

func TestUpdate_RefusesShareTransferLeg(t *testing.T) {
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Brokerage")
	dst := createInvAccount(t, env.accountRepo, "IRA")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2019, time.December, 19)

	if _, err := env.svc.Deposit(src.ID, date, types.MustNewMoney("1000.00"), ""); err != nil {
		t.Fatal(err)
	}
	price := types.MustNewMoney("100.00")
	if _, err := env.svc.Buy(src.ID, sec.ID, date, types.MustNewQuantity("5"), nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatal(err)
	}
	res, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, date, types.MustNewQuantity("2"), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	leg := res.SourceTransaction
	_, err = env.editSvc.UpdateBuy(leg.ID, src.ID, sec.ID, date, types.MustNewQuantity("2"), nil, &price, types.ZeroMoney, "")
	var legErr *IsShareTransferLegError
	if !errors.As(err, &legErr) {
		t.Fatalf("UpdateBuy err = %v, want IsShareTransferLegError", err)
	}
	_, err = env.editSvc.UpdateWithdrawal(leg.ID, src.ID, date, types.MustNewMoney("200.00"), "")
	if !errors.As(err, &legErr) {
		t.Fatalf("UpdateWithdrawal err = %v, want IsShareTransferLegError", err)
	}
	for _, id := range []types.ID{res.SourceTransaction.ID, res.DestinationTransaction.ID} {
		if _, err := env.invRepo.GetByID(id); err != nil {
			t.Fatalf("leg %s was deleted: %v", id, err)
		}
	}
}
