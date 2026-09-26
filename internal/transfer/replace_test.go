package transfer

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReplaceWithTransfer_WithdrawalBecomesTransfer(t *testing.T) {
	h := newHarness(t)
	wd, err := h.invSvc.Withdrawal(h.brokerage.ID, testDate(), types.MustNewMoney("400.00"), "to checking")
	if err != nil {
		t.Fatal(err)
	}

	res, err := h.svc.ReplaceWithTransfer(wd.ID, Spec{
		FromAccountID: h.brokerage.ID,
		ToAccountID:   h.checking.ID,
		Date:          testDate(),
		Amount:        types.MustNewMoney("400.00"),
		Memo:          "to checking",
	})
	if err != nil {
		t.Fatalf("ReplaceWithTransfer: %v", err)
	}

	if _, err := h.invRepo.GetByID(wd.ID); err == nil {
		t.Error("the withdrawal row should be gone")
	}
	tr, err := h.svc.Get(res.TransferID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tr.From.AccountID != h.brokerage.ID || tr.To.AccountID != h.checking.ID {
		t.Errorf("transfer %s -> %s, want brokerage -> checking", tr.From.AccountID, tr.To.AccountID)
	}
	if !tr.Amount.Equal(types.MustNewMoney("400.00")) {
		t.Errorf("amount = %s, want 400.00", tr.Amount)
	}
}

func TestReplaceWithTransfer_DepositBecomesTransfer(t *testing.T) {
	h := newHarness(t)
	dep, err := h.invSvc.Deposit(h.brokerage.ID, testDate(), types.MustNewMoney("250.00"), "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.svc.ReplaceWithTransfer(dep.ID, Spec{
		FromAccountID: h.checking.ID,
		ToAccountID:   h.brokerage.ID,
		Date:          testDate(),
		Amount:        types.MustNewMoney("250.00"),
	}); err != nil {
		t.Fatalf("ReplaceWithTransfer: %v", err)
	}
	if _, err := h.invRepo.GetByID(dep.ID); err == nil {
		t.Error("the deposit row should be gone")
	}
}

func TestReplaceWithTransfer_Refusals(t *testing.T) {
	h := newHarness(t)
	_, legID, _ := h.seed(h.brokerage, h.checking, "100.00", "", types.NullableID{})
	fee, err := h.invSvc.Fee(h.brokerage.ID, testDate(), types.MustNewMoney("5.00"), "")
	if err != nil {
		t.Fatal(err)
	}
	wd, err := h.invSvc.Withdrawal(h.brokerage.ID, testDate(), types.MustNewMoney("40.00"), "")
	if err != nil {
		t.Fatal(err)
	}

	spec := Spec{FromAccountID: h.brokerage.ID, ToAccountID: h.checking.ID, Date: testDate(), Amount: types.MustNewMoney("40.00")}
	cases := []struct {
		name  string
		rowID types.ID
		spec  Spec
	}{
		{"a row that is already a transfer leg", legID, spec},
		{"a fee", fee.ID, spec},
		{"a transfer that does not touch the row's account", wd.ID,
			Spec{FromAccountID: h.savings.ID, ToAccountID: h.checking.ID, Date: testDate(), Amount: types.MustNewMoney("40.00")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var refused *NotReplaceableError
			if _, err := h.svc.ReplaceWithTransfer(tc.rowID, tc.spec); !errors.As(err, &refused) {
				t.Fatalf("err = %v, want NotReplaceableError", err)
			}
			if _, err := h.invRepo.GetByID(tc.rowID); err != nil {
				t.Errorf("row was deleted: %v", err)
			}
		})
	}
}

// A failed write must leave the old row in place: the delete and the two new
// legs commit together or not at all.
func TestReplaceWithTransfer_FailedGuardKeepsRow(t *testing.T) {
	h := newHarness(t)
	wd, err := h.invSvc.Withdrawal(h.brokerage.ID, testDate(), types.MustNewMoney("40.00"), "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.svc.ReplaceWithTransfer(wd.ID, Spec{
		FromAccountID: h.brokerage.ID,
		ToAccountID:   h.checking.ID,
		Date:          testDate(),
		Amount:        types.ZeroMoney,
	})
	var amountErr *InvalidAmountError
	if !errors.As(err, &amountErr) {
		t.Fatalf("err = %v, want InvalidAmountError", err)
	}
	row, err := h.invRepo.GetByID(wd.ID)
	if err != nil {
		t.Fatalf("row was deleted: %v", err)
	}
	if row.Type != investment.TransactionTypeWithdrawal {
		t.Errorf("row type = %s, want withdrawal", row.Type)
	}
}
