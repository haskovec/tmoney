package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// openSvc opens the application services for a database file and registers a
// cleanup that closes the underlying connection.
func openSvc(t *testing.T, dbPath string) *app.Services {
	t.Helper()
	database, svc, err := openServices(dbPath)
	if err != nil {
		t.Fatalf("openServices: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return svc
}

func TestResolveTransferPair_NotFound(t *testing.T) {
	dbPath, _, _ := setupTransferAccounts(t)
	svc := openSvc(t, dbPath)

	_, err := resolveTransferPair(svc, types.NewID())
	if err == nil {
		t.Fatal("expected error for unknown leg id")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestResolveTransferPair_NonTransfer(t *testing.T) {
	dbPath, checking, _ := setupTransferAccounts(t)
	svc := openSvc(t, dbPath)

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

func TestResolveTransferPair_RegToReg(t *testing.T) {
	dbPath, checking, savings := setupTransferAccounts(t)
	svc := openSvc(t, dbPath)

	pair, err := svc.Transaction.CreateTransfer(checking.ID, savings.ID, types.Today(), types.MustNewMoney("75.00"))
	if err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}

	// Resolving from either leg yields the same pair.
	for _, legID := range []types.ID{pair.FromTransaction.ID, pair.ToTransaction.ID} {
		res, err := resolveTransferPair(svc, legID)
		if err != nil {
			t.Fatalf("resolveTransferPair(%s): %v", legID, err)
		}
		if res.kind != transaction.DispatchRegToReg {
			t.Errorf("kind = %v, want DispatchRegToReg", res.kind)
		}
		if res.fromAccount.ID != checking.ID || res.toAccount.ID != savings.ID {
			t.Errorf("from/to = %s/%s, want %s/%s", res.fromAccount.Name, res.toAccount.Name, checking.Name, savings.Name)
		}
		if !res.amount.Equal(types.MustNewMoney("75.00")) {
			t.Errorf("amount = %s, want 75.00", res.amount)
		}
		if res.transferID != pair.FromTransaction.TransferID.ID {
			t.Errorf("transferID mismatch")
		}
	}
}

func TestResolveTransferPair_RegToInv(t *testing.T) {
	dbPath, checking, brokerage, _, _ := setupTransferDispatchAccounts(t)
	svc := openSvc(t, dbPath)

	res, err := svc.Investment.DepositFromAccount(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("500.00"), "fund")
	if err != nil {
		t.Fatalf("DepositFromAccount: %v", err)
	}

	// Resolve from the regular leg and from the investment leg.
	for _, legID := range []types.ID{res.RegularTransaction.ID, res.InvestmentTransaction.ID} {
		got, err := resolveTransferPair(svc, legID)
		if err != nil {
			t.Fatalf("resolveTransferPair(%s): %v", legID, err)
		}
		if got.kind != transaction.DispatchRegToInv {
			t.Errorf("kind = %v, want DispatchRegToInv", got.kind)
		}
		if got.fromAccount.ID != checking.ID || got.toAccount.ID != brokerage.ID {
			t.Errorf("from/to = %s/%s, want Checking/Brokerage", got.fromAccount.Name, got.toAccount.Name)
		}
		if got.investmentTxnID != res.InvestmentTransaction.ID {
			t.Errorf("investmentTxnID = %s, want %s", got.investmentTxnID, res.InvestmentTransaction.ID)
		}
		if !got.amount.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("amount = %s, want 500.00", got.amount)
		}
	}
}

func TestResolveTransferPair_InvToReg(t *testing.T) {
	dbPath, checking, brokerage, _, _ := setupTransferDispatchAccounts(t)
	svc := openSvc(t, dbPath)

	res, err := svc.Investment.TransferCash(brokerage.ID, checking.ID, types.Today(), types.MustNewMoney("250.00"), "draw")
	if err != nil {
		t.Fatalf("TransferCash: %v", err)
	}

	for _, legID := range []types.ID{res.RegularTransaction.ID, res.InvestmentTransaction.ID} {
		got, err := resolveTransferPair(svc, legID)
		if err != nil {
			t.Fatalf("resolveTransferPair(%s): %v", legID, err)
		}
		if got.kind != transaction.DispatchInvToReg {
			t.Errorf("kind = %v, want DispatchInvToReg", got.kind)
		}
		if got.fromAccount.ID != brokerage.ID || got.toAccount.ID != checking.ID {
			t.Errorf("from/to = %s/%s, want Brokerage/Checking", got.fromAccount.Name, got.toAccount.Name)
		}
		if got.investmentTxnID != res.InvestmentTransaction.ID {
			t.Errorf("investmentTxnID = %s, want %s", got.investmentTxnID, res.InvestmentTransaction.ID)
		}
	}
}

func TestResolveTransferPair_InvToInv(t *testing.T) {
	dbPath, _, brokerage, ira, _ := setupTransferDispatchAccounts(t)
	svc := openSvc(t, dbPath)

	res, err := svc.Investment.TransferCashBetweenInvestments(brokerage.ID, ira.ID, types.Today(), types.MustNewMoney("1000.00"), "rollover")
	if err != nil {
		t.Fatalf("TransferCashBetweenInvestments: %v", err)
	}

	for _, legID := range []types.ID{res.SourceTransaction.ID, res.DestinationTransaction.ID} {
		got, err := resolveTransferPair(svc, legID)
		if err != nil {
			t.Fatalf("resolveTransferPair(%s): %v", legID, err)
		}
		if got.kind != transaction.DispatchInvToInv {
			t.Errorf("kind = %v, want DispatchInvToInv", got.kind)
		}
		if got.fromAccount.ID != brokerage.ID || got.toAccount.ID != ira.ID {
			t.Errorf("from/to = %s/%s, want Brokerage/Rollover IRA", got.fromAccount.Name, got.toAccount.Name)
		}
		if got.investmentTxnID.IsNil() {
			t.Errorf("investmentTxnID should be set for inv↔inv")
		}
	}
}

func TestResolveTransferPair_RefusesTransferLineSplit(t *testing.T) {
	dbPath, checking, brokerage, _, _ := setupTransferDispatchAccounts(t)
	svc := openSvc(t, dbPath)

	// Build a parent split transaction in Checking with one transfer-line
	// targeting the Brokerage investment account (paycheck → 401k shape).
	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-200.00"))
	line := transaction.NewSplit(parent.ID, types.NilID, types.MustNewMoney("-200.00"))
	line.TransferAccountID = types.NullableID{ID: brokerage.ID, Valid: true}
	if err := svc.Transaction.CreateWithSplits(parent, []*transaction.Split{line}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	// The investment-side counterpart row carries the split's transfer_id.
	invLeg := findInvestmentLegForTest(t, svc, brokerage.ID)

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

// findInvestmentLegForTest returns the ID of the single investment transaction
// in the given investment account.
func findInvestmentLegForTest(t *testing.T, svc *app.Services, invAcctID types.ID) types.ID {
	t.Helper()
	rows, err := svc.InvestmentRepo.ListByAccount(invAcctID, investment.TransactionFilter{})
	if err != nil {
		t.Fatalf("list investment txns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 investment row, got %d", len(rows))
	}
	return rows[0].ID
}
