package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// TestRun_FullReconciliationWorkflow exercises start → status → finish → status
// across multiple verbs to confirm reconcile state survives between invocations.
func TestRun_FullReconciliationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-200.00"))
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("500.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = executeWith([]string{
		"reconcile", "start",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "1300.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("start reconcile failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Reconciliation started") {
		t.Error("should confirm reconciliation started")
	}

	stdout.Reset()
	err = executeWith([]string{
		"reconcile", "status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("reconcile status failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "In progress") {
		t.Error("status should show in progress")
	}

	stdout.Reset()
	err = executeWith([]string{
		"reconcile", "finish",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("finish reconcile failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Reconciliation completed") {
		t.Error("should confirm reconciliation completed")
	}

	stdout.Reset()
	err = executeWith([]string{
		"reconcile", "status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("reconcile status after completion failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Last reconciled:  2024-01-31") {
		t.Errorf("status should show last reconciled date, got:\n%s", output)
	}
	if !strings.Contains(output, "Current session:  None") {
		t.Errorf("status should show no current session, got:\n%s", output)
	}
}

// TestRun_BuyThenSellUpdatesCash exercises investment buy then sell to confirm
// cash flow is correctly applied across the two verbs.
func TestRun_BuyThenSellUpdatesCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	if err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Sell transaction created successfully") {
		t.Error("output should confirm sell")
	}
	if !strings.Contains(output, "$800.00") {
		t.Error("output should show sell total of $800.00")
	}
}
