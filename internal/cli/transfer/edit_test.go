package transfer_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransferEdit_DispatchRegToInv(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		res, err := svc.Investment.DepositFromAccount(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("500.00"), "fund", types.NullableID{})
		if err != nil {
			t.Fatalf("DepositFromAccount: %v", err)
		}
		legID = res.RegularTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "edit", "--file", dbPath, "--txn-id", legID.String(), "--amount", "650.00",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer edit reg→inv: %v\nstderr=%s", err, stderr)
	}
	assertInvTransferAmount(t, dbPath, checking, []*account.Account{brokerage}, "650.00")
}

func TestTransferEdit_DispatchInvToReg(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	var legID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		res, err := svc.Investment.TransferCash(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("250.00"), "draw", types.NullableID{})
		if err != nil {
			t.Fatalf("TransferCash: %v", err)
		}
		legID = res.InvestmentTransaction.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "edit", "--file", dbPath, "--txn-id", legID.String(), "--amount", "300.00",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer edit inv→reg: %v\nstderr=%s", err, stderr)
	}
	assertInvTransferAmount(t, dbPath, checking, []*account.Account{brokerage}, "300.00")
}

func TestTransferEdit_DispatchInvToInv(t *testing.T) {
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
	if err := cli.ExecuteWith([]string{
		"transfer", "edit", "--file", dbPath, "--txn-id", legID.String(), "--amount", "1200.00",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer edit inv→inv: %v\nstderr=%s", err, stderr)
	}
	assertInvTransferAmount(t, dbPath, nil, []*account.Account{brokerage, ira}, "1200.00")
}

func TestTransferEdit_RefusesTransferLineSplit(t *testing.T) {
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
	err := cli.ExecuteWith([]string{"transfer", "edit", "--file", dbPath, "--txn-id", invLegID.String(), "--amount", "100.00"}, stdout, stderr)
	if err == nil {
		t.Fatal("transfer edit on a transfer-line split should refuse")
	}
	if !strings.Contains(err.Error(), "multi-line split") {
		t.Errorf("expected 'multi-line split' refusal, got: %v", err)
	}
}

// assertInvTransferAmount opens the DB and asserts the transfer's legs all
// carry the expected absolute amount across the regular and investment tables.
func assertInvTransferAmount(t *testing.T, dbPath string, regAcct *account.Account, invAccts []*account.Account, expectAmount string) {
	t.Helper()
	want := types.MustNewMoney(expectAmount).Abs()
	svc := clitest.OpenSvc(t, dbPath)

	if regAcct != nil {
		txns, err := svc.TransactionRepo.ListByAccount(regAcct.ID)
		if err != nil {
			t.Fatalf("list %s: %v", regAcct.Name, err)
		}
		var transfers int
		for _, txn := range txns {
			if txn.IsTransfer() {
				transfers++
				if !txn.Amount.Abs().Equal(want) {
					t.Errorf("%s leg amount = %s, want abs %s", regAcct.Name, txn.Amount, want)
				}
			}
		}
		if transfers != 1 {
			t.Errorf("expected 1 transfer leg in %s, got %d", regAcct.Name, transfers)
		}
	}

	for _, acct := range invAccts {
		rows, err := svc.InvestmentRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
		if err != nil {
			t.Fatalf("list inv %s: %v", acct.Name, err)
		}
		var transfers int
		for _, r := range rows {
			if r.IsTransfer() {
				transfers++
				if !r.TotalAmount.Abs().Equal(want) {
					t.Errorf("%s inv leg amount = %s, want abs %s", acct.Name, r.TotalAmount, want)
				}
			}
		}
		if transfers != 1 {
			t.Errorf("expected 1 transfer leg in %s, got %d", acct.Name, transfers)
		}
	}
}
