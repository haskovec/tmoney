package transfer

import (
	"errors"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestResolveTransferPair_NotFound(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	_, err := resolveTransferPair(svc, types.NewID())
	if err == nil {
		t.Fatal("expected error for unknown leg id")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestResolveTransferPair_NonTransfer(t *testing.T) {
	dbPath, checking, _ := clitest.SetupTransferAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	// A plain (non-transfer) transaction.
	txn := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-25.00"))
	if err := svc.Transaction.Create(txn); err != nil {
		t.Fatalf("create plain txn: %v", err)
	}

	_, err := resolveTransferPair(svc, txn.ID)
	if err == nil {
		t.Fatal("expected error for non-transfer transaction")
	}
	if !strings.Contains(err.Error(), "not a transfer") {
		t.Errorf("expected 'not a transfer', got: %v", err)
	}
}

func TestResolveTransferPair_RefusesTransferLineSplit(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)
	svc := clitest.OpenSvc(t, dbPath)

	// Build a parent split transaction in Checking with one transfer-line
	// targeting the Brokerage investment account (paycheck → 401k shape).
	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-200.00"))
	line := transaction.NewSplit(parent.ID, types.NilID, types.MustNewMoney("-200.00"))
	line.TransferAccountID = types.NullableID{ID: brokerage.ID, Valid: true}
	if err := svc.Transaction.CreateWithSplits(parent, []*transaction.Split{line}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	// The investment-side counterpart row carries the split's transfer_id.
	invLeg := clitest.FindInvestmentLegForTest(t, svc, brokerage.ID)

	_, err := resolveTransferPair(svc, invLeg)
	if err == nil {
		t.Fatal("expected refusal for transfer-line split")
	}
	var split *errTransferLineSplit
	if !errors.As(err, &split) {
		t.Fatalf("expected *errTransferLineSplit, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "multi-line split") {
		t.Errorf("expected 'multi-line split' in message, got: %v", err)
	}
}
