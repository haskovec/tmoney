package transfer_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/investment"
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

func TestTransferDelete_DispatchRegToReg(t *testing.T) {
	dbPath, checking, savings := clitest.SetupTransferAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		pair, err := svc.Transaction.CreateTransfer(checking.ID, savings.ID, types.Today(), types.MustNewMoney("75.00"))
		if err != nil {
			t.Fatalf("CreateTransfer: %v", err)
		}
		legID = pair.FromTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", legID.String()}, stdout, stderr); err != nil {
		t.Fatalf("transfer delete reg→reg: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer deleted successfully") {
		t.Errorf("expected success line, got: %s", stdout.String())
	}

	svc := clitest.OpenSvc(t, dbPath)
	for _, acct := range []*account.Account{checking, savings} {
		txns, err := svc.TransactionRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("list %s: %v", acct.Name, err)
		}
		if len(txns) != 0 {
			t.Errorf("expected 0 transactions in %s after delete, got %d", acct.Name, len(txns))
		}
	}
}

func TestTransferDelete_DispatchRegToInv(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		res, err := svc.Investment.DepositFromAccount(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("500.00"), "fund")
		if err != nil {
			t.Fatalf("DepositFromAccount: %v", err)
		}
		legID = res.RegularTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", legID.String()}, stdout, stderr); err != nil {
		t.Fatalf("transfer delete reg→inv: %v\nstderr=%s", err, stderr)
	}
	assertTransferGone(t, dbPath, checking, []*account.Account{brokerage})
}

func TestTransferDelete_DispatchInvToReg(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		res, err := svc.Investment.TransferCash(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("250.00"), "draw")
		if err != nil {
			t.Fatalf("TransferCash: %v", err)
		}
		// Use the investment-side leg to exercise the inv-table lookup path.
		legID = res.InvestmentTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", legID.String()}, stdout, stderr); err != nil {
		t.Fatalf("transfer delete inv→reg: %v\nstderr=%s", err, stderr)
	}
	assertTransferGone(t, dbPath, checking, []*account.Account{brokerage})
}

func TestTransferDelete_DispatchInvToInv(t *testing.T) {
	dbPath, _, brokerage, ira, _ := clitest.SetupTransferDispatchAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		res, err := svc.Investment.TransferCashBetweenInvestments(brokerage.ID, ira.ID, types.Today(), types.MustNewMoney("1000.00"), "rollover")
		if err != nil {
			t.Fatalf("TransferCashBetweenInvestments: %v", err)
		}
		legID = res.SourceTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", legID.String()}, stdout, stderr); err != nil {
		t.Fatalf("transfer delete inv→inv: %v\nstderr=%s", err, stderr)
	}
	assertTransferGone(t, dbPath, nil, []*account.Account{brokerage, ira})
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

func TestTransferDelete_RefusesReconciledLeg(t *testing.T) {
	dbPath, checking, savings := clitest.SetupTransferAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		pair, err := svc.Transaction.CreateTransfer(checking.ID, savings.ID, types.Today(), types.MustNewMoney("75.00"))
		if err != nil {
			t.Fatalf("CreateTransfer: %v", err)
		}
		legID = pair.FromTransaction.ID
		// Reconcile both legs directly via the repository.
		reconcileLegs(t, svc, pair.FromTransaction.TransferID.ID)
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "delete", "--file", dbPath, "--txn-id", legID.String()}, stdout, stderr)
	if err == nil {
		t.Fatal("transfer delete on a reconciled transfer should refuse")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reconciled") {
		t.Errorf("expected reconciled refusal, got: %v", err)
	}
}

// assertTransferGone opens the DB and asserts no transfer rows remain for the
// regular account (if non-nil) or the listed investment accounts.
func assertTransferGone(t *testing.T, dbPath string, regAcct *account.Account, invAccts []*account.Account) {
	t.Helper()
	svc := clitest.OpenSvc(t, dbPath)
	if regAcct != nil {
		txns, err := svc.TransactionRepo.ListByAccount(regAcct.ID)
		if err != nil {
			t.Fatalf("list %s: %v", regAcct.Name, err)
		}
		for _, txn := range txns {
			if txn.IsTransfer() {
				t.Errorf("expected no transfer rows in %s, found one", regAcct.Name)
			}
		}
	}
	for _, acct := range invAccts {
		rows, err := svc.InvestmentRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
		if err != nil {
			t.Fatalf("list inv %s: %v", acct.Name, err)
		}
		for _, r := range rows {
			if r.IsTransfer() {
				t.Errorf("expected no transfer rows in %s, found one", acct.Name)
			}
		}
	}
}

// reconcileLegs marks both legs of a reg↔reg transfer as reconciled.
func reconcileLegs(t *testing.T, svc *app.Services, transferID types.ID) {
	t.Helper()
	if err := svc.Transaction.UpdateTransferStatus(transferID, transaction.StatusReconciled); err != nil {
		t.Fatalf("UpdateTransferStatus: %v", err)
	}
}
