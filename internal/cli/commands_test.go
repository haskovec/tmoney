package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// --- Reconciliation CLI Tests ---

func TestRun_FullReconciliationWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create account with opening balance 1000
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

	// Create transactions: -200 and +500 = 1300 total
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

	// Step 1: Start reconciliation
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

	// Step 2: Check status
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

	// Step 3: Finish reconciliation
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

	// Step 4: Verify status shows completed
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

// =============================================================================
// Security CLI Commands Tests (SM-097 through SM-102)
// =============================================================================

// createTestDBWithSecurity creates a test DB and inserts a security, returning the path.
func createTestDBWithSecurity(t *testing.T) (string, *security.Security) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	sec.SetExchange("NASDAQ")
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}

	database.Close()
	return dbPath, sec
}

// =============================================================================
// SM-103: --prices (list prices for a ticker)
// =============================================================================

func createTestDBWithSecurityAndPrices(t *testing.T) (string, *security.Security) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}

	priceRepo := price.NewRepository(database)
	p1 := price.NewPrice(sec.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("150.00"), price.SourceManual)
	if err := priceRepo.Create(p1); err != nil {
		t.Fatalf("failed to create price 1: %v", err)
	}
	p2 := price.NewPrice(sec.ID, types.MustParseDate("2024-02-15"), types.MustNewMoney("160.50"), price.SourceTransaction)
	if err := priceRepo.Create(p2); err != nil {
		t.Fatalf("failed to create price 2: %v", err)
	}
	p3 := price.NewPrice(sec.ID, types.MustParseDate("2024-03-15"), types.MustNewMoney("170.25"), price.SourceImport)
	if err := priceRepo.Create(p3); err != nil {
		t.Fatalf("failed to create price 3: %v", err)
	}

	database.Close()
	return dbPath, sec
}

// --- Investment transaction CLI tests (SM-106 through SM-112) ---

// createInvestmentTestDB creates a test database with an investment account, a deposit for cash,
// and a security. Returns the dbPath. The database is closed after setup.
func createInvestmentTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "invest.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create investment account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = trackLots
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	// Create a security
	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	// Deposit cash via the service so the cash balance is set
	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("50000"), "initial deposit")
	if err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	database.Close()
	return dbPath
}

// helper to create pointer to Money
func ptrMoney(s string) *types.Money {
	m := types.MustNewMoney(s)
	return &m
}

// --- End-to-end: buy then sell verifies cash flow ---

func TestRun_BuyThenSellUpdatesCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	// Buy 10 shares at $150 = $1500 deducted from $50000
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

	// Sell 5 shares at $160 = $800 received
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

// --- Helper for corporate action tests ---

// createCorporateActionTestDB creates a DB with an investment account holding shares.
// Returns the DB path. If withSecondSecurity is true, also creates a "GOOG" security.
func createCorporateActionTestDB(t *testing.T, trackLots bool, withSecondSecurity bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corp.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = trackLots
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	if withSecondSecurity {
		sec2 := security.NewSecurity("GOOG", "Alphabet Inc.", security.TypeStock)
		if err := secRepo.Create(sec2); err != nil {
			t.Fatalf("failed to create second security: %v", err)
		}
	}

	svc := app.NewServices(database)

	// Deposit cash
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit")
	if err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	// Buy 100 shares of AAPL at $150/share
	totalAmount := types.MustNewMoney("15000")
	_, err = svc.Investment.Buy(acct.ID, sec.ID, types.Today(), types.MustNewQuantity("100"), &totalAmount, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	database.Close()
	return dbPath
}
