package transfer_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransferDelete_MissingTxnID(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("transfer delete without --txn-id should error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "txn-id") {
		t.Errorf("expected required-flag error mentioning txn-id, got: %v", err)
	}
}

func TestTransferDelete_RefusesTransferLineSplit(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	var invLegID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-200.00"))
		line := transaction.NewSplit(parent.ID, types.NilID, types.MustNewMoney("-200.00"))
		line.TransferAccountID = types.NullableID{ID: brokerage.ID, Valid: true}
		if err := svc.Transaction.CreateWithSplits(parent, []*transaction.Split{line}); err != nil {
			t.Fatalf("CreateWithSplits: %v", err)
		}
		invLegID = clitest.FindInvestmentLegForTest(t, svc, brokerage.ID)
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", invLegID.String()}, stdout, stderr)
	if err == nil {
		t.Fatal("transfer delete on a transfer-line split should refuse")
	}
	if !strings.Contains(err.Error(), "multi-line split") {
		t.Errorf("expected 'multi-line split' refusal, got: %v", err)
	}
}
