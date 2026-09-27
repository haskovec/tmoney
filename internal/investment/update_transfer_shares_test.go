package investment

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// shareEditEnv is a source account holding 10 shares, 5 of them moved to a
// destination account by one share transfer.
type shareEditEnv struct {
	env      *testServiceEnv
	src, dst *account.Account
	secID    types.ID
	date     types.Date
	res      *ShareTransferResult
}

func newShareEditEnv(t *testing.T) shareEditEnv {
	t.Helper()
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source")
	dst := createInvAccount(t, env.accountRepo, "Dest")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2020, time.March, 1)
	price := types.MustNewMoney("50.00")
	if _, err := env.svc.Buy(src.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy error = %v", err)
	}
	res, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, date, types.MustNewQuantity("5"), "", nil)
	if err != nil {
		t.Fatalf("TransferShares error = %v", err)
	}
	return shareEditEnv{env: env, src: src, dst: dst, secID: sec.ID, date: date, res: res}
}

// shares returns the account's position in the env's security.
func (e shareEditEnv) shares(t *testing.T, acctID types.ID) types.Quantity {
	t.Helper()
	pos, err := e.env.positionRepo.GetByAccountAndSecurity(acctID, e.secID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity error = %v", err)
	}
	return pos.Shares
}

// transferRows counts the account's share-transfer rows.
func (e shareEditEnv) transferRows(t *testing.T, acctID types.ID) int {
	t.Helper()
	typ := TransactionTypeTransferShares
	rows, err := e.env.invRepo.ListByAccount(acctID, TransactionFilter{Type: &typ})
	if err != nil {
		t.Fatalf("ListByAccount error = %v", err)
	}
	return len(rows)
}

func (e shareEditEnv) assertUnchanged(t *testing.T) {
	t.Helper()
	if got := e.shares(t, e.src.ID); !got.Equal(types.MustNewQuantity("5")) {
		t.Errorf("source position = %s, want 5 (unchanged)", got)
	}
	if _, err := e.env.invRepo.GetByID(e.res.SourceTransaction.ID); err != nil {
		t.Errorf("source leg is gone after a refused edit: %v", err)
	}
}

func TestUpdateTransferShares_SameAccounts(t *testing.T) {
	e := newShareEditEnv(t)

	res, err := e.env.editSvc.UpdateTransferShares(e.res.SourceTransaction.ID, e.src.ID, e.dst.ID,
		e.date, e.secID, types.MustNewQuantity("3"), "", nil)
	if err != nil {
		t.Fatalf("UpdateTransferShares error = %v", err)
	}

	for _, id := range []types.ID{e.res.SourceTransaction.ID, e.res.DestinationTransaction.ID} {
		if _, err := e.env.invRepo.GetByID(id); err == nil {
			t.Errorf("old leg %s still exists", id)
		}
	}
	pair, err := e.env.invRepo.ListByTransferID(res.SourceTransaction.TransferID.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pair) != 2 {
		t.Errorf("new pair has %d rows, want 2", len(pair))
	}
	if n := e.transferRows(t, e.src.ID) + e.transferRows(t, e.dst.ID); n != 2 {
		t.Errorf("share-transfer rows across both accounts = %d, want 2", n)
	}
	if got := e.shares(t, e.src.ID); !got.Equal(types.MustNewQuantity("7")) {
		t.Errorf("source position = %s, want 7", got)
	}
	if got := e.shares(t, e.dst.ID); !got.Equal(types.MustNewQuantity("3")) {
		t.Errorf("destination position = %s, want 3", got)
	}
}

// The case the old code got wrong: with the other leg gone, it still reversed
// and deleted the source and wrote a new pair.
func TestUpdateTransferShares_OtherLegMissing(t *testing.T) {
	e := newShareEditEnv(t)
	if err := e.env.invRepo.Delete(e.res.DestinationTransaction.ID); err != nil {
		t.Fatal(err)
	}

	_, err := e.env.editSvc.UpdateTransferShares(e.res.SourceTransaction.ID, e.src.ID, e.dst.ID,
		e.date, e.secID, types.MustNewQuantity("3"), "", nil)
	var broken *BrokenShareTransferError
	if !errors.As(err, &broken) {
		t.Fatalf("error = %v, want a BrokenShareTransferError", err)
	}
	e.assertUnchanged(t)
	if n := e.transferRows(t, e.dst.ID); n != 0 {
		t.Errorf("destination share-transfer rows = %d, want 0 (no new pair)", n)
	}
}

func TestUpdateTransferShares_NoTransferAccount(t *testing.T) {
	e := newShareEditEnv(t)
	src := e.res.SourceTransaction
	src.TransferAccountID = types.NullableID{}
	if err := e.env.invRepo.Update(src); err != nil {
		t.Fatal(err)
	}

	_, err := e.env.editSvc.UpdateTransferShares(src.ID, e.src.ID, e.dst.ID,
		e.date, e.secID, types.MustNewQuantity("3"), "", nil)
	var broken *BrokenShareTransferError
	if !errors.As(err, &broken) {
		t.Fatalf("error = %v, want a BrokenShareTransferError", err)
	}
	e.assertUnchanged(t)
	if n := e.transferRows(t, e.src.ID) + e.transferRows(t, e.dst.ID); n != 2 {
		t.Errorf("share-transfer rows = %d, want the original 2 (no new pair)", n)
	}
}

func TestUpdateTransferShares_MovesDestination(t *testing.T) {
	e := newShareEditEnv(t)
	other := createInvAccount(t, e.env.accountRepo, "Other")

	if _, err := e.env.editSvc.UpdateTransferShares(e.res.SourceTransaction.ID, e.src.ID, other.ID,
		e.date, e.secID, types.MustNewQuantity("5"), "", nil); err != nil {
		t.Fatalf("UpdateTransferShares error = %v", err)
	}

	if got := e.shares(t, e.dst.ID); !got.IsZero() {
		t.Errorf("old destination position = %s, want 0", got)
	}
	if n := e.transferRows(t, e.dst.ID); n != 0 {
		t.Errorf("old destination share-transfer rows = %d, want 0", n)
	}
	if got := e.shares(t, other.ID); !got.Equal(types.MustNewQuantity("5")) {
		t.Errorf("new destination position = %s, want 5", got)
	}
}

// The TUI passes the register's account as the source. Started from the
// destination register, that reverses the direction of the transfer, so the
// service refuses and names the real source account.
func TestUpdateTransferShares_RefusesDestinationLeg(t *testing.T) {
	e := newShareEditEnv(t)

	_, err := e.env.editSvc.UpdateTransferShares(e.res.DestinationTransaction.ID, e.dst.ID, e.src.ID,
		e.date, e.secID, types.MustNewQuantity("5"), "", nil)
	var dest *ShareTransferDestinationLegError
	if !errors.As(err, &dest) {
		t.Fatalf("error = %v, want a ShareTransferDestinationLegError", err)
	}
	if dest.SourceAccountID != e.src.ID {
		t.Errorf("error names source %s, want %s", dest.SourceAccountID, e.src.ID)
	}
	if !strings.Contains(err.Error(), e.src.ID.String()) {
		t.Errorf("error text %q does not name the source account", err)
	}
	e.assertUnchanged(t)
	if got := e.shares(t, e.dst.ID); !got.Equal(types.MustNewQuantity("5")) {
		t.Errorf("destination position = %s, want 5 (unchanged)", got)
	}
}

// A zero-basis leg has no sign, so neither side can be told apart.
func TestUpdateTransferShares_RefusesZeroBasis(t *testing.T) {
	e := newShareEditEnv(t)
	for _, leg := range []*Transaction{e.res.SourceTransaction, e.res.DestinationTransaction} {
		leg.TotalAmount = types.ZeroMoney
		if err := e.env.invRepo.Update(leg); err != nil {
			t.Fatal(err)
		}
	}

	_, err := e.env.editSvc.UpdateTransferShares(e.res.SourceTransaction.ID, e.src.ID, e.dst.ID,
		e.date, e.secID, types.MustNewQuantity("3"), "", nil)
	var broken *BrokenShareTransferError
	if !errors.As(err, &broken) {
		t.Fatalf("error = %v, want a BrokenShareTransferError", err)
	}
	e.assertUnchanged(t)
}

// Delete finds a share transfer's other leg by transfer_id. It used to list
// the rows of the account named on the row, so a row that named no account
// deleted alone and left the other leg holding the shares.
func TestDeleteShareTransfer_FindsLegByTransferID(t *testing.T) {
	e := newShareEditEnv(t)
	src := e.res.SourceTransaction
	src.TransferAccountID = types.NullableID{}
	if err := e.env.invRepo.Update(src); err != nil {
		t.Fatal(err)
	}

	if err := e.env.svc.DeleteTransaction(src.ID); err != nil {
		t.Fatalf("DeleteTransaction error = %v", err)
	}

	if _, err := e.env.invRepo.GetByID(e.res.DestinationTransaction.ID); err == nil {
		t.Error("the destination leg survived the delete")
	}
	if got := e.shares(t, e.dst.ID); !got.IsZero() {
		t.Errorf("destination position = %s, want 0", got)
	}
	if got := e.shares(t, e.src.ID); !got.Equal(types.MustNewQuantity("10")) {
		t.Errorf("source position = %s, want 10 (restored)", got)
	}
}

// Delete finds the other leg by transfer_id, so a row that names no account
// no longer hides that leg from the freeze check. A closed account on the
// other leg refuses the delete, and the reversal of this leg rolls back.
func TestDeleteShareTransfer_RefusesClosedOtherLeg(t *testing.T) {
	e := newShareEditEnv(t)
	src := e.res.SourceTransaction
	src.TransferAccountID = types.NullableID{}
	if err := e.env.invRepo.Update(src); err != nil {
		t.Fatal(err)
	}
	closeInvAccount(t, e.env.accountRepo, e.dst)

	assertInvClosed(t, e.env.svc.DeleteTransaction(src.ID))

	e.assertUnchanged(t)
	if _, err := e.env.invRepo.GetByID(e.res.DestinationTransaction.ID); err != nil {
		t.Errorf("the closed account's leg is gone: %v", err)
	}
	if got := e.shares(t, e.dst.ID); !got.Equal(types.MustNewQuantity("5")) {
		t.Errorf("closed destination position = %s, want 5 (unchanged)", got)
	}
}
