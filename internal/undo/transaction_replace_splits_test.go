package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// These command-level tests exercise the Phase-2 fix through the two undo
// commands that drive ReplaceSplits — EditTransactionWithSplitsCommand and
// VoidTransactionCommand — proving a split set containing a transfer line
// survives an edit (and its undo) and a void (and its undo) with its paired
// counterpart intact rather than orphaned.

// singleSavingsCounterpart returns the sole transaction in an account, failing
// if there is not exactly one.
func singleSavingsCounterpart(t *testing.T, svc *transaction.Service, acctID types.ID) *transaction.Transaction {
	t.Helper()
	txns, err := svc.ListByAccount(acctID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("want exactly 1 counterpart transaction, got %d", len(txns))
	}
	return txns[0]
}

func transferLineSplit(parentID, targetAcctID types.ID, amount types.Money) *transaction.Split {
	return &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parentID,
		CategoryID:        types.NilID,
		Amount:            amount,
		TransferAccountID: types.NullableID{ID: targetAcctID, Valid: true},
	}
}

func TestEditTransactionWithSplitsCommand_TransferLineCounterpart(t *testing.T) {
	env := createTestEnv(t)
	checking := createTestAccount(t, env.accountRepo, "Checking")
	savings := createTestAccount(t, env.accountRepo, "Savings")
	food := createTestCategory(t, env.categoryRepo, "Food")

	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	catLine := transaction.NewSplit(parent.ID, food.ID, types.MustNewMoney("-60.00"))
	xfer := transferLineSplit(parent.ID, savings.ID, types.MustNewMoney("-40.00"))
	if err := env.txnSvc.CreateWithSplits(parent, []*transaction.Split{catLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	origCP := singleSavingsCounterpart(t, env.txnSvc, savings.ID)
	origCPID := origCP.ID
	transferID := origCP.TransferID.ID
	if !origCP.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Fatalf("counterpart amount = %s, want 40.00", origCP.Amount.String())
	}

	// TUI-shaped edit: shift $10 from the transfer line to the category line.
	after, err := env.txnSvc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	afterSplits := []*transaction.Split{
		transaction.NewSplit(parent.ID, food.ID, types.MustNewMoney("-50.00")),
		transferLineSplit(parent.ID, savings.ID, types.MustNewMoney("-50.00")),
	}
	cmd := undo.NewEditTransactionWithSplitsCommand(env.txnSvc, after, afterSplits)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute (regression: used to fail the pairing CHECK): %v", err)
	}
	cp := singleSavingsCounterpart(t, env.txnSvc, savings.ID)
	if cp.ID != origCPID {
		t.Errorf("counterpart identity churned on edit: got %s, want %s", cp.ID, origCPID)
	}
	if !cp.Amount.Equal(types.MustNewMoney("50.00")) {
		t.Errorf("counterpart amount after edit = %s, want 50.00", cp.Amount.String())
	}
	if cp.TransferID.ID != transferID {
		t.Errorf("counterpart transfer_id changed after edit")
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	cp = singleSavingsCounterpart(t, env.txnSvc, savings.ID)
	if cp.ID != origCPID {
		t.Errorf("counterpart identity churned on undo: got %s, want %s", cp.ID, origCPID)
	}
	if !cp.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("counterpart amount after undo = %s, want 40.00", cp.Amount.String())
	}
	splits, _ := env.txnSvc.GetSplits(parent.ID)
	if len(splits) != 2 {
		t.Errorf("want 2 splits after undo, got %d", len(splits))
	}
}

func TestVoidTransactionCommand_TransferLineSplitRoundTrip(t *testing.T) {
	env := createTestEnv(t)
	checking := createTestAccount(t, env.accountRepo, "Checking")
	savings := createTestAccount(t, env.accountRepo, "Savings")
	food := createTestCategory(t, env.categoryRepo, "Food")

	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	catLine := transaction.NewSplit(parent.ID, food.ID, types.MustNewMoney("-60.00"))
	xfer := transferLineSplit(parent.ID, savings.ID, types.MustNewMoney("-40.00"))
	if err := env.txnSvc.CreateWithSplits(parent, []*transaction.Split{catLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}
	transferID := singleSavingsCounterpart(t, env.txnSvc, savings.ID).TransferID.ID

	cmd := undo.NewVoidTransactionCommand(env.txnSvc, parent.ID)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Void removes the parent's splits and their counterpart.
	if txns, _ := env.txnSvc.ListByAccount(savings.ID); len(txns) != 0 {
		t.Errorf("counterpart should be deleted on void, got %d in savings", len(txns))
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo (regression: used to leave the counterpart orphaned/missing): %v", err)
	}
	restored, err := env.txnSvc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID after undo: %v", err)
	}
	if restored.IsVoid() {
		t.Error("parent should not be void after undo")
	}
	splits, _ := env.txnSvc.GetSplits(parent.ID)
	if len(splits) != 2 {
		t.Fatalf("want 2 splits after undo, got %d", len(splits))
	}
	// Counterpart is recreated, sharing the restored transfer line's id.
	cp := singleSavingsCounterpart(t, env.txnSvc, savings.ID)
	if !cp.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("restored counterpart amount = %s, want 40.00", cp.Amount.String())
	}
	if cp.TransferID.ID != transferID {
		t.Errorf("restored counterpart transfer_id = %s, want %s", cp.TransferID.ID, transferID)
	}
	var line *transaction.Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			line = s
		}
	}
	if line == nil || line.TransferID.ID != cp.TransferID.ID {
		t.Errorf("restored transfer line not linked to its counterpart: line=%+v", line)
	}
}
