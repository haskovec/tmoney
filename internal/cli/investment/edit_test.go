package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// seedEditFixture creates the standard investment test DB and records a
// buy (10 AAPL for $1500 on 2024-01-15) and a dividend ($25.50 on
// 2024-02-20). Returns the DB path.
func seedEditFixture(t *testing.T) string {
	t.Helper()
	return seedListFixture(t)
}

// findTxn returns the single transaction of the given type on Brokerage.
func findTxn(t *testing.T, dbPath string, txnType investmentdom.TransactionType) *investmentdom.Transaction {
	t.Helper()
	svc := clitest.OpenSvc(t, dbPath)
	acct, err := svc.Account.GetByName("Brokerage")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	rows, err := svc.InvestmentRepo.ListByAccount(acct.ID, investmentdom.TransactionFilter{Type: &txnType})
	if err != nil {
		t.Fatalf("list %s txns: %v", txnType, err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 %s transaction, got %d", txnType, len(rows))
	}
	return rows[0]
}

func TestInvestmentEdit_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--txn-id", "0195b8f0-0000-7000-8000-000000000000",
		"--shares", "11",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentEdit_MissingTxnID(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", "/fake.tdb",
		"--shares", "11",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) without --txn-id should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "txn-id") {
		t.Errorf("expected Cobra required-flag error mentioning txn-id, got: %v", err)
	}
}

func TestInvestmentEdit_NoEditableFlags(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) without editable flags should return error")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("expected error to require at least one editable flag, got: %v", err)
	}
}

func TestInvestmentEdit_UnknownTxnID(t *testing.T) {
	dbPath := seedEditFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", "0195b8f0-0000-7000-8000-000000000000",
		"--shares", "11",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) with unknown --txn-id should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestInvestmentEdit_BuyShares_KeepsAmount(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
		"--shares", "11",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit --shares) failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "updated") {
		t.Errorf("expected success message, got:\n%s", stdout.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)
	if !after.Shares.Quantity.Equal(types.MustNewQuantity("11")) {
		t.Errorf("expected shares 11, got %s", after.Shares.Quantity)
	}
	// Total is preserved when only --shares is edited (price re-derives).
	if !after.TotalAmount.Equal(types.MustNewMoney("-1500")) {
		t.Errorf("expected total -1500, got %s", after.TotalAmount)
	}

	// Position reflects the new share count.
	svc := clitest.OpenSvc(t, dbPath)
	acct, _ := svc.Account.GetByName("Brokerage")
	pos, err := svc.PositionRepo.GetByAccountAndSecurity(acct.ID, after.SecurityID.ID)
	if err != nil {
		t.Fatalf("get position: %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("11")) {
		t.Errorf("expected position 11 shares, got %s", pos.Shares)
	}
}

func TestInvestmentEdit_BuyPrice_RecomputesTotal(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
		"--price-per-share", "160",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit --price-per-share) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)
	if !after.Shares.Quantity.Equal(types.MustNewQuantity("10")) {
		t.Errorf("expected shares unchanged at 10, got %s", after.Shares.Quantity)
	}
	if !after.TotalAmount.Equal(types.MustNewMoney("-1600")) {
		t.Errorf("expected total -1600 from 10 × $160, got %s", after.TotalAmount)
	}
}

func TestInvestmentEdit_BuyDateAndMemo(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
		"--date", "2024-01-16",
		"--memo", "fixed date",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit --date --memo) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)
	if got := after.Date.String(); got != "2024-01-16" {
		t.Errorf("expected date 2024-01-16, got %s", got)
	}
	if !after.Memo.Valid || after.Memo.String != "fixed date" {
		t.Errorf("expected memo %q, got %+v", "fixed date", after.Memo)
	}
	if !after.Shares.Quantity.Equal(types.MustNewQuantity("10")) {
		t.Errorf("expected shares unchanged at 10, got %s", after.Shares.Quantity)
	}
	if !after.TotalAmount.Equal(types.MustNewMoney("-1500")) {
		t.Errorf("expected total unchanged at -1500, got %s", after.TotalAmount)
	}
}

func TestInvestmentEdit_DividendAmount(t *testing.T) {
	dbPath := seedEditFixture(t)
	div := findTxn(t, dbPath, investmentdom.TransactionTypeDividend)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", div.ID.String(),
		"--amount", "30.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit dividend --amount) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeDividend)
	if !after.TotalAmount.Equal(types.MustNewMoney("30")) {
		t.Errorf("expected dividend amount 30, got %s", after.TotalAmount)
	}
}

func TestInvestmentEdit_DepositAmount(t *testing.T) {
	dbPath := seedEditFixture(t)
	dep := findTxn(t, dbPath, investmentdom.TransactionTypeDeposit)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", dep.ID.String(),
		"--amount", "40000",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit deposit --amount) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeDeposit)
	if !after.TotalAmount.Equal(types.MustNewMoney("40000")) {
		t.Errorf("expected deposit amount 40000, got %s", after.TotalAmount)
	}
}

func TestInvestmentEdit_RejectSharesOnDividend(t *testing.T) {
	dbPath := seedEditFixture(t)
	div := findTxn(t, dbPath, investmentdom.TransactionTypeDividend)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", div.ID.String(),
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit dividend --shares) should return error")
	}
	if !strings.Contains(err.Error(), "--shares") || !strings.Contains(err.Error(), "dividend") {
		t.Errorf("expected error rejecting --shares for dividend, got: %v", err)
	}
}

func TestInvestmentEdit_RejectPriceOnDeposit(t *testing.T) {
	dbPath := seedEditFixture(t)
	dep := findTxn(t, dbPath, investmentdom.TransactionTypeDeposit)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", dep.ID.String(),
		"--price-per-share", "160",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit deposit --price-per-share) should return error")
	}
	if !strings.Contains(err.Error(), "--price-per-share") || !strings.Contains(err.Error(), "deposit") {
		t.Errorf("expected error rejecting --price-per-share for deposit, got: %v", err)
	}
}

func TestInvestmentEdit_RefuseTransferLeg(t *testing.T) {
	dbPath, _, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Brokerage",
		"--amount", "500",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed transfer failed: %v\nstderr: %s", err, stderr.String())
	}

	svc := clitest.OpenSvc(t, dbPath)
	legID := clitest.FindInvestmentLegForTest(t, svc, brokerage.ID)

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", legID.String(),
		"--amount", "600",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) on a transfer leg should return error")
	}
	if !strings.Contains(err.Error(), "transfer edit") {
		t.Errorf("expected error to point at `tmoney transfer edit`, got: %v", err)
	}
}

func TestInvestmentEdit_RefuseReconciled(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	svc := clitest.OpenSvc(t, dbPath)
	buy.Status = investmentdom.TransactionStatusReconciled
	if err := svc.InvestmentRepo.Update(buy); err != nil {
		t.Fatalf("mark reconciled: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
		"--shares", "11",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment edit) on a reconciled transaction should return error")
	}
	if !strings.Contains(err.Error(), "reconciled") {
		t.Errorf("expected error to mention reconciled, got: %v", err)
	}
}

func TestInvestmentEdit_PreservesClearedStatus(t *testing.T) {
	dbPath := seedEditFixture(t)
	buy := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)

	svc := clitest.OpenSvc(t, dbPath)
	if err := svc.Investment.SetClearedStatus(buy.ID, true); err != nil {
		t.Fatalf("mark cleared: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", buy.ID.String(),
		"--shares", "11",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeBuy)
	if after.Status != investmentdom.TransactionStatusCleared {
		t.Errorf("expected cleared status preserved across edit, got %s", after.Status)
	}
}

func TestInvestmentEdit_LotTrackedSellRepointsLots(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, true)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
		"--date", "2024-01-15",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed buy failed: %v", err)
	}
	// A lot-tracked sell needs an explicit lot allocation, both at entry
	// and when edited.
	seedSvc := clitest.OpenSvc(t, dbPath)
	seedAcct, err := seedSvc.Account.GetByName("Brokerage")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	sec, err := seedSvc.Security.Resolve("AAPL", "", "")
	if err != nil {
		t.Fatalf("resolve security: %v", err)
	}
	lots, err := seedSvc.LotRepo.ListByAccountAndSecurity(seedAcct.ID, sec.ID, false)
	if err != nil || len(lots) != 1 {
		t.Fatalf("expected 1 open lot, got %d (err %v)", len(lots), err)
	}
	lotID := lots[0].ID.String()

	if err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "4",
		"--amount", "700",
		"--date", "2024-02-01",
		"--lot", lotID,
	}, stdout, stderr); err != nil {
		t.Fatalf("seed sell failed: %v", err)
	}

	sell := findTxn(t, dbPath, investmentdom.TransactionTypeSell)

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"investment", "edit",
		"--file", dbPath,
		"--txn-id", sell.ID.String(),
		"--shares", "5",
		"--lot", lotID,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment edit sell, lot-tracked) failed: %v\nstderr: %s", err, stderr.String())
	}

	after := findTxn(t, dbPath, investmentdom.TransactionTypeSell)
	if !after.Shares.Quantity.Equal(types.MustNewQuantity("5")) {
		t.Errorf("expected sell shares 5, got %s", after.Shares.Quantity)
	}

	svc := clitest.OpenSvc(t, dbPath)
	acct, _ := svc.Account.GetByName("Brokerage")
	pos, err := svc.PositionRepo.GetByAccountAndSecurity(acct.ID, after.SecurityID.ID)
	if err != nil {
		t.Fatalf("get position: %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("5")) {
		t.Errorf("expected position 5 shares after 10 buy − 5 sell, got %s", pos.Shares)
	}
}
