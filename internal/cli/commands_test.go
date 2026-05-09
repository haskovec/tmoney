package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestRun_ReportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--report", "net-worth"}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ReportMissingType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) without report type should return error")
	}
	if !strings.Contains(err.Error(), "requires a report type") {
		t.Errorf("error should mention report type requirement, got: %v", err)
	}
}

func TestRun_ReportInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "invalid-type", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report) with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "unknown report type") {
		t.Errorf("error should mention unknown report type, got: %v", err)
	}
}

func TestRun_ReportNetWorthEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "NET WORTH REPORT") {
		t.Error("output should contain NET WORTH REPORT header")
	}
	if !strings.Contains(output, "ASSETS") {
		t.Error("output should contain ASSETS section")
	}
	if !strings.Contains(output, "LIABILITIES") {
		t.Error("output should contain LIABILITIES section")
	}
	if !strings.Contains(output, "NET WORTH:") {
		t.Error("output should contain NET WORTH summary")
	}
}

func TestRun_ReportNetWorthWithAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create asset and liability accounts
	acctRepo := account.NewRepository(database)

	checking := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("5000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := account.NewAccount(
		"Savings",
		account.TypeSavings,
		"USD",
		types.MustNewMoney("10000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	creditCard := account.NewAccount(
		"Credit Card",
		account.TypeCreditCard,
		"USD",
		types.MustNewMoney("0"),
		types.Today(),
	)
	if err := acctRepo.Create(creditCard); err != nil {
		t.Fatalf("failed to create credit card account: %v", err)
	}

	// Add a credit card transaction (liability)
	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(creditCard.ID, types.Today(), types.MustNewMoney("-500.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
	if !strings.Contains(output, "Savings") {
		t.Error("output should contain Savings account")
	}
	if !strings.Contains(output, "Credit Card") {
		t.Error("output should contain Credit Card account")
	}
}

func TestRun_ReportNetWorthWithAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--as-of", "2024-01-15", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report net-worth --as-of) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "January 15, 2024") {
		t.Error("output should show the as-of date")
	}
}

func TestRun_ReportNetWorthInvalidAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "net-worth", "--as-of", "invalid-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report net-worth) with invalid as-of date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --as-of date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_ReportSpendingMissingPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) without period should return error")
	}
	if !strings.Contains(err.Error(), "requires --month") {
		t.Errorf("error should mention period requirement, got: %v", err)
	}
}

func TestRun_ReportSpendingByMonth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --month) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "January 2024") {
		t.Error("output should show the period")
	}
}

func TestRun_ReportSpendingByYear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--year", "2024", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --year) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "2024") {
		t.Error("output should show the year")
	}
}

func TestRun_ReportSpendingByDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--from", "2024-01-01", "--to", "2024-06-30", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending --from --to) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SPENDING BY CATEGORY") {
		t.Error("output should contain SPENDING BY CATEGORY header")
	}
	if !strings.Contains(output, "2024-01-01 to 2024-06-30") {
		t.Error("output should show the date range")
	}
}

func TestRun_ReportSpendingWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("5000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create expense category
	catRepo := category.NewRepository(database)
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create expense transaction
	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("-150.00"))
	txn.SetCategory(groceries.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--report spending) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Groceries") {
		t.Error("output should contain Groceries category")
	}
	if !strings.Contains(output, "$150.00") {
		t.Error("output should show spending amount")
	}
	if !strings.Contains(output, "100.0%") {
		t.Error("output should show percentage")
	}
}

func TestRun_ReportSpendingInvalidMonth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) with invalid month should return error")
	}
	if !strings.Contains(err.Error(), "invalid --month format") {
		t.Errorf("error should mention invalid month format, got: %v", err)
	}
}

func TestRun_ReportSpendingInvalidMonthValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--report", "spending", "--month", "2024-13", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--report spending) with month > 12 should return error")
	}
	if !strings.Contains(err.Error(), "month must be between 1 and 12") {
		t.Errorf("error should mention month range, got: %v", err)
	}
}

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
// Parse Args Tests for New Flags
// =============================================================================

// --- Export CLI tests ---

func TestParseArgs_ExportFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--export", "out.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--format", "csv",
		"--from", "2024-01-01",
		"--to", "2024-12-31",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.exportFile != "out.csv" {
		t.Errorf("exportFile should be out.csv, got %q", opts.exportFile)
	}
	if opts.accountName != "Checking" {
		t.Errorf("accountName should be Checking, got %q", opts.accountName)
	}
	if opts.formatOverride != "csv" {
		t.Errorf("formatOverride should be csv, got %q", opts.formatOverride)
	}
	if opts.fromDate != "2024-01-01" {
		t.Errorf("fromDate should be 2024-01-01, got %q", opts.fromDate)
	}
	if opts.toDate != "2024-12-31" {
		t.Errorf("toDate should be 2024-12-31, got %q", opts.toDate)
	}
}

func TestParseArgs_ExportFlagsEqualForm(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--export=out.qif",
		"--file", "test.tdb",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.exportFile != "out.qif" {
		t.Errorf("exportFile should be out.qif, got %q", opts.exportFile)
	}
}

func TestParseArgs_ExportMissingFile(t *testing.T) {
	_, _, err := parseArgs([]string{"--export"})
	if err == nil {
		t.Error("parseArgs should return error for --export without argument")
	}
}

func TestRun_ExportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--export", "out.csv"}, stdout, stderr)
	if err == nil {
		t.Error("run(--export) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ExportUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", filepath.Join(tmpDir, "out.csv"),
		"--file", dbPath,
		"--format", "ofx",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with OFX format should return error")
	}
	if !strings.Contains(err.Error(), "must be csv or qif") {
		t.Errorf("error should mention valid formats, got: %v", err)
	}
}

func TestRun_ExportCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	// Create database with account and transactions
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-15"), types.MustNewMoney("-120.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export to CSV
	exportPath := filepath.Join(tmpDir, "export.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "EXPORT COMPLETE") {
		t.Error("output should contain EXPORT COMPLETE header")
	}
	if !strings.Contains(output, "Transactions: 2") {
		t.Errorf("output should show 2 transactions, got: %s", output)
	}
	if !strings.Contains(output, "CSV") {
		t.Error("output should show CSV format")
	}

	// Verify the file was created and has content
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	csvStr := string(content)
	if !strings.Contains(csvStr, "Date,Account,Payee,Category,Amount") {
		t.Error("CSV should contain header row")
	}
	if !strings.Contains(csvStr, "2024-03-01") {
		t.Error("CSV should contain first transaction date")
	}
	if !strings.Contains(csvStr, "2024-03-15") {
		t.Error("CSV should contain second transaction date")
	}
}

func TestRun_ExportQIF(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-06-01"), types.MustNewMoney("-75.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	exportPath := filepath.Join(tmpDir, "export.qif")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export QIF) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "QIF") {
		t.Error("output should show QIF format")
	}

	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	qifStr := string(content)
	if !strings.Contains(qifStr, "!Type:") {
		t.Error("QIF should contain type header")
	}
	if !strings.Contains(qifStr, "T-75.00") {
		t.Error("QIF should contain transaction amount")
	}
}

func TestRun_ExportWithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("5000"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(checking.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := transaction.NewTransaction(savings.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-25.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export only Checking account
	exportPath := filepath.Join(tmpDir, "checking.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --account) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Accounts:     1") {
		t.Errorf("output should show 1 account, got: %s", output)
	}
	if !strings.Contains(output, "Transactions: 1") {
		t.Errorf("output should show 1 transaction, got: %s", output)
	}
}

func TestRun_ExportWithDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-15"), types.MustNewMoney("-75.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn3 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-06-15"), types.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export only Q1 2024
	exportPath := filepath.Join(tmpDir, "q1.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--from", "2024-01-01",
		"--to", "2024-03-31",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --from --to) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Transactions: 2") {
		t.Errorf("output should show 2 transactions for Q1, got: %s", output)
	}
}

func TestRun_ExportFormatOverrideCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Export with .txt extension but force CSV format
	exportPath := filepath.Join(tmpDir, "export.txt")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--export --format csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "CSV") {
		t.Error("output should show CSV format")
	}
}

func TestRun_ExportAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestRun_ExportNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Empty", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
		"--account", "Empty",
	}, stdout, stderr)
	if err == nil {
		t.Error("export with no transactions should return error")
	}
	if !strings.Contains(err.Error(), "no transactions") {
		t.Errorf("error should mention no transactions, got: %v", err)
	}
}

func TestRun_ExportUndetectableFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.xyz")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Error("export with undetectable format should return error")
	}
	if !strings.Contains(err.Error(), "cannot detect format") {
		t.Errorf("error should mention format detection failure, got: %v", err)
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

// Args parsing tests for security flags

func TestParseArgs_SecurityFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, opts *cliOptions)
	}{
		{
			"ticker flag",
			[]string{"--ticker", "MSFT"},
			func(t *testing.T, opts *cliOptions) {
				if opts.secTicker != "MSFT" {
					t.Errorf("secTicker = %q, want MSFT", opts.secTicker)
				}
			},
		},
		{
			"asset-class flag",
			[]string{"--asset-class", "large_cap_stock"},
			func(t *testing.T, opts *cliOptions) {
				if opts.secAssetClass != "large_cap_stock" {
					t.Errorf("secAssetClass = %q, want large_cap_stock", opts.secAssetClass)
				}
			},
		},
		{
			"exchange flag",
			[]string{"--exchange", "NYSE"},
			func(t *testing.T, opts *cliOptions) {
				if opts.secExchange != "NYSE" {
					t.Errorf("secExchange = %q, want NYSE", opts.secExchange)
				}
			},
		},
		{
			"combined security flags",
			[]string{"--ticker", "MSFT", "--name", "Apple", "--type", "stock", "--asset-class", "large_cap_stock", "--exchange", "NASDAQ"},
			func(t *testing.T, opts *cliOptions) {
				if opts.secTicker != "MSFT" {
					t.Errorf("secTicker = %q, want MSFT", opts.secTicker)
				}
				if opts.acctName != "Apple" {
					t.Errorf("acctName = %q, want Apple", opts.acctName)
				}
				if opts.acctType != "stock" {
					t.Errorf("acctType = %q, want stock", opts.acctType)
				}
				if opts.secAssetClass != "large_cap_stock" {
					t.Errorf("secAssetClass = %q, want large_cap_stock", opts.secAssetClass)
				}
				if opts.secExchange != "NASDAQ" {
					t.Errorf("secExchange = %q, want NASDAQ", opts.secExchange)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			tt.check(t, opts)
		})
	}
}

func TestParseArgs_SecurityFlagsMissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"ticker missing value", []string{"--ticker"}},
		{"asset-class missing value", []string{"--asset-class"}},
		{"exchange missing value", []string{"--exchange"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error for missing argument", tt.args)
			}
		})
	}
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

// Args parsing tests for price flags

func TestParseArgs_PriceFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, opts *cliOptions)
	}{
		{
			"price value flag",
			[]string{"--price", "150.00"},
			func(t *testing.T, opts *cliOptions) {
				if opts.priceValue != "150.00" {
					t.Errorf("priceValue = %q, want %q", opts.priceValue, "150.00")
				}
			},
		},
		{
			"price value equals format",
			[]string{"--price=99.99"},
			func(t *testing.T, opts *cliOptions) {
				if opts.priceValue != "99.99" {
					t.Errorf("priceValue = %q, want %q", opts.priceValue, "99.99")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			tt.check(t, opts)
		})
	}
}

func TestParseArgs_PriceFlagsMissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"price missing value", []string{"--price"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error for missing argument", tt.args)
			}
		})
	}
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

// --- parseArgs tests for investment flags ---

func TestParseArgs_InvestmentFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(*cliOptions) bool
	}{
		{"--shares value", []string{"--shares", "10"}, func(o *cliOptions) bool { return o.shares == "10" }},
		{"--shares=value", []string{"--shares=10"}, func(o *cliOptions) bool { return o.shares == "10" }},
		{"--commission value", []string{"--commission", "9.99"}, func(o *cliOptions) bool { return o.commission == "9.99" }},
		{"--commission=value", []string{"--commission=9.99"}, func(o *cliOptions) bool { return o.commission == "9.99" }},
		{"--price-per-share value", []string{"--price-per-share", "150"}, func(o *cliOptions) bool { return o.pricePerShare == "150" }},
		{"--price-per-share=value", []string{"--price-per-share=150"}, func(o *cliOptions) bool { return o.pricePerShare == "150" }},
		{"--lot value", []string{"--lot", "abc123"}, func(o *cliOptions) bool { return o.lot == "abc123" }},
		{"--lot=value", []string{"--lot=abc123"}, func(o *cliOptions) bool { return o.lot == "abc123" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs(%v) returned error: %v", tt.args, err)
			}
			if !tt.check(opts) {
				t.Errorf("parseArgs(%v) check failed", tt.args)
			}
		})
	}
}

func TestParseArgs_InvestmentFlagsMissingArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"shares missing value", []string{"--shares"}},
		{"commission missing value", []string{"--commission"}},
		{"price-per-share missing value", []string{"--price-per-share"}},
		{"lot missing value", []string{"--lot"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error for missing argument", tt.args)
			}
		})
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
