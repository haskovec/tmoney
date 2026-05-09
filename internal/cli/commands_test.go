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

// --- Import CLI tests ---

func TestParseArgs_ImportFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--import", "bank.csv",
		"--account", "Checking",
		"--file", "test.tdb",
		"--confirm",
		"--skip-duplicates",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.importFile != "bank.csv" {
		t.Errorf("importFile should be bank.csv, got %q", opts.importFile)
	}
	if opts.accountName != "Checking" {
		t.Errorf("accountName should be Checking, got %q", opts.accountName)
	}
	if !opts.confirm {
		t.Error("confirm should be true")
	}
	if !opts.skipDuplicates {
		t.Error("skipDuplicates should be true")
	}
}

func TestParseArgs_ImportFlagsEqualFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--import=bank.txt",
		"--format=csv",
		"--account", "Checking",
		"--file", "test.tdb",
		"--update-duplicates",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.importFile != "bank.txt" {
		t.Errorf("importFile should be bank.txt, got %q", opts.importFile)
	}
	if opts.formatOverride != "csv" {
		t.Errorf("formatOverride should be csv, got %q", opts.formatOverride)
	}
	if !opts.updateDuplicates {
		t.Error("updateDuplicates should be true")
	}
}

func TestParseArgs_ImportMissingFile(t *testing.T) {
	_, _, err := parseArgs([]string{"--import"})
	if err == nil {
		t.Error("parseArgs should return error for --import without argument")
	}
}

func TestParseArgs_FormatMissing(t *testing.T) {
	_, _, err := parseArgs([]string{"--format"})
	if err == nil {
		t.Error("parseArgs should return error for --format without argument")
	}
}

func TestRun_ImportMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import", "bank.csv", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ImportMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import", "bank.csv", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_ImportMutuallyExclusiveDuplicateFlags(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--skip-duplicates",
		"--update-duplicates",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with both --skip-duplicates and --update-duplicates should return error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

func TestRun_ImportInvalidFormat(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--format", "xml",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with invalid --format should return error")
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestRun_ImportNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", "/nonexistent/bank.csv",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--import) with nonexistent import file should return error")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error should mention failed to open, got: %v", err)
	}
}

func TestRun_ImportCSVDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file for import
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Category,Memo\n2024-03-01,-50.00,Coffee Shop,Food:Coffee,Morning coffee\n2024-03-02,-120.00,Electric Co,Bills:Utilities,March electric\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import dry-run) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "2 transactions") {
		t.Error("output should show 2 parsed transactions")
	}
	if !strings.Contains(output, "Run with --confirm") {
		t.Error("output should prompt to run with --confirm")
	}
}

func TestRun_ImportCSVConfirm(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file for import
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Memo\n2024-03-01,-50.00,Coffee Shop,Morning coffee\n2024-03-02,-120.00,Electric Co,March electric\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--confirm",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --confirm) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE") {
		t.Error("output should contain IMPORT COMPLETE header")
	}
	if !strings.Contains(output, "Created:  2") {
		t.Errorf("output should show 2 created transactions, got: %s", output)
	}

	// Verify transactions were actually created
	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo := transaction.NewRepository(database2)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txns))
	}
}

func TestRun_ImportClosedAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := account.NewRepository(database)
	acct := account.NewAccount("Closed Account", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	acct.Active = false
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Closed Account",
	}, stdout, stderr)
	if err == nil {
		t.Error("import into closed account should return error")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should mention account is closed, got: %v", err)
	}
}

func TestRun_ImportFormatOverride(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Create a CSV file with a .txt extension
	csvPath := filepath.Join(tmpDir, "import.txt")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --format csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestRun_ImportSkipDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	accountRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create an existing transaction that should match an import row
	txnRepo := transaction.NewRepository(database)
	existingTxn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(existingTxn); err != nil {
		t.Fatalf("failed to create existing transaction: %v", err)
	}

	database.Close()

	// Create a CSV file with a matching transaction
	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n2024-03-02,-75.00,Gas Station\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--skip-duplicates",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import --skip-duplicates) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestRun_ImportAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--import", csvPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Error("import with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

// writeTestFile is a test helper that writes content to a file.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

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

// --- SM-113: CLI --portfolio ---

func TestParseArgs_PortfolioFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(*cliOptions) bool
	}{
		{"--portfolio", []string{"--portfolio"}, func(o *cliOptions) bool { return o.portfolio }},
		{"--show-lots", []string{"--show-lots"}, func(o *cliOptions) bool { return o.showLots }},
		{"--portfolio with --show-lots", []string{"--portfolio", "--show-lots"}, func(o *cliOptions) bool {
			return o.portfolio && o.showLots
		}},
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

func TestRun_PortfolioMissingFile(t *testing.T) {
	err := run([]string{"--portfolio", "--account", "Brokerage"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_PortfolioMissingAccount(t *testing.T) {
	err := run([]string{"--portfolio", "--file", "test.tdb"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_PortfolioAccountNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)
	err := run([]string{"--portfolio", "--file", dbPath, "--account", "NonExistent"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestRun_PortfolioEmptyAccount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{"--portfolio", "--file", dbPath, "--account", "Brokerage"}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--portfolio) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
	if !strings.Contains(output, "(No holdings)") {
		t.Error("output should indicate no holdings")
	}
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain summary section")
	}
	if !strings.Contains(output, "Cash Balance:") {
		t.Error("output should show cash balance")
	}
}

// createPortfolioTestDB creates a test DB with an investment account, securities,
// prices, and buy transactions for portfolio testing.
func createPortfolioTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "portfolio.tdb")

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

	// Create securities
	secRepo := security.NewRepository(database)
	aapl := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(aapl); err != nil {
		t.Fatalf("failed to create AAPL: %v", err)
	}
	msft := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)
	if err := secRepo.Create(msft); err != nil {
		t.Fatalf("failed to create MSFT: %v", err)
	}

	// Add prices
	priceRepo := price.NewRepository(database)
	p1 := price.NewPrice(aapl.ID, types.Today(), types.MustNewMoney("175.00"), price.SourceManual)
	if err := priceRepo.Create(p1); err != nil {
		t.Fatalf("failed to create AAPL price: %v", err)
	}
	p2 := price.NewPrice(msft.ID, types.Today(), types.MustNewMoney("420.00"), price.SourceManual)
	if err := priceRepo.Create(p2); err != nil {
		t.Fatalf("failed to create MSFT price: %v", err)
	}

	// Deposit cash and buy securities via services
	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit")
	if err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	// Buy 10 AAPL at $150/share
	_, err = svc.Investment.Buy(acct.ID, aapl.ID, types.Today(), types.MustNewQuantity("10"), nil, ptrMoney("150"), types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy AAPL: %v", err)
	}

	// Buy 5 MSFT at $400/share
	_, err = svc.Investment.Buy(acct.ID, msft.ID, types.Today(), types.MustNewQuantity("5"), nil, ptrMoney("400"), types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy MSFT: %v", err)
	}

	database.Close()
	return dbPath
}

func TestRun_PortfolioWithHoldings(t *testing.T) {
	dbPath := createPortfolioTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{"--portfolio", "--file", dbPath, "--account", "Brokerage"}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--portfolio) returned error: %v", err)
	}

	output := stdout.String()

	// Check header
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}

	// Check holdings table columns
	if !strings.Contains(output, "Ticker") {
		t.Error("output should contain Ticker column header")
	}
	if !strings.Contains(output, "Shares") {
		t.Error("output should contain Shares column header")
	}
	if !strings.Contains(output, "Market Value") {
		t.Error("output should contain Market Value column header")
	}
	if !strings.Contains(output, "Gain/Loss") {
		t.Error("output should contain Gain/Loss column header")
	}

	// Check securities appear
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain AAPL ticker")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain Apple Inc. name")
	}
	if !strings.Contains(output, "MSFT") {
		t.Error("output should contain MSFT ticker")
	}
	if !strings.Contains(output, "Microsoft Corp.") {
		t.Error("output should contain Microsoft Corp. name")
	}

	// Check summary section
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain SUMMARY section")
	}
	if !strings.Contains(output, "Cash Balance:") {
		t.Error("output should show cash balance")
	}
	if !strings.Contains(output, "Market Value:") {
		t.Error("output should show market value")
	}
	if !strings.Contains(output, "Total Value:") {
		t.Error("output should show total value")
	}
	if !strings.Contains(output, "Total Cost Basis:") {
		t.Error("output should show total cost basis")
	}
	if !strings.Contains(output, "Total Gain/Loss:") {
		t.Error("output should show total gain/loss")
	}
}

func TestRun_PortfolioWithAsOf(t *testing.T) {
	dbPath := createPortfolioTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--portfolio", "--file", dbPath,
		"--account", "Brokerage",
		"--as-of", "2099-12-31",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--portfolio --as-of) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
}

func TestRun_PortfolioInvalidAsOf(t *testing.T) {
	dbPath := createPortfolioTestDB(t, false)

	err := run([]string{
		"--portfolio", "--file", dbPath,
		"--account", "Brokerage",
		"--as-of", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --as-of") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

// --- SM-114: CLI --portfolio --show-lots ---

func TestRun_PortfolioShowLotsWithLotTracking(t *testing.T) {
	dbPath := createPortfolioTestDB(t, true) // lot-tracking enabled

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--portfolio", "--file", dbPath,
		"--account", "Brokerage",
		"--show-lots",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--portfolio --show-lots) returned error: %v", err)
	}

	output := stdout.String()

	// Check header indicates lots
	if !strings.Contains(output, "PORTFOLIO: Brokerage (with lots)") {
		t.Error("output should contain portfolio header with lots indicator")
	}

	// Check lot detail columns
	if !strings.Contains(output, "Lot") {
		t.Error("output should contain Lot column header")
	}
	if !strings.Contains(output, "Purchase Date") {
		t.Error("output should contain Purchase Date column header")
	}
	if !strings.Contains(output, "Cost/Share") {
		t.Error("output should contain Cost/Share column header")
	}
	if !strings.Contains(output, "Cost Basis") {
		t.Error("output should contain Cost Basis column header")
	}
	if !strings.Contains(output, "Current Value") {
		t.Error("output should contain Current Value column header")
	}

	// Check securities still appear
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain AAPL ticker")
	}
	if !strings.Contains(output, "MSFT") {
		t.Error("output should contain MSFT ticker")
	}

	// Check summary
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain SUMMARY section")
	}
}

func TestRun_PortfolioShowLotsNonLotTracking(t *testing.T) {
	// --show-lots on a non-lot-tracking account should fall back to normal display
	dbPath := createPortfolioTestDB(t, false) // lot-tracking disabled

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--portfolio", "--file", dbPath,
		"--account", "Brokerage",
		"--show-lots",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--portfolio --show-lots non-lot) returned error: %v", err)
	}

	output := stdout.String()
	// Should show normal portfolio (without lot detail header)
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
	// Should NOT contain "with lots" since account doesn't track lots
	if strings.Contains(output, "(with lots)") {
		t.Error("output should not indicate lots for non-lot-tracking account")
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

// setupMultiAccountImportFixture creates a test database with two
// accounts and a CSV file containing transactions for both. Returns the
// database and CSV paths.
func setupMultiAccountImportFixture(t *testing.T) (dbPath, csvPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath = filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	acctRepo := account.NewRepository(database)
	for _, name := range []string{"Checking", "Savings"} {
		acct := account.NewAccount(name, account.TypeChecking, "USD",
			types.MustNewMoney("0"), types.NewDate(2024, 1, 1))
		if err := acctRepo.Create(acct); err != nil {
			database.Close()
			t.Fatalf("failed to create account %s: %v", name, err)
		}
	}
	database.Close()

	csvPath = filepath.Join(tmpDir, "register.csv")
	csv := "Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account\n" +
		"2024-01-10,Checking,Coffee Shop,,-5.00,,,,\n" +
		"2024-01-11,Checking,Employer,,3000.00,,,,\n" +
		"2024-01-12,Savings,Interest,,5.00,,,,\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("failed to write CSV: %v", err)
	}
	return dbPath, csvPath
}

func TestRun_ImportMultiAccountCSV_RequiresSourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--file", dbPath,
		"--import", csvPath,
		"--account", "Checking",
	}, stdout, stderr)

	if err == nil {
		t.Fatal("expected error when CSV contains multiple accounts and no --source-account")
	}
	msg := err.Error()
	if !strings.Contains(msg, "transactions for 2 accounts") {
		t.Errorf("error should mention multiple accounts, got: %v", err)
	}
	if !strings.Contains(msg, "Checking") || !strings.Contains(msg, "Savings") {
		t.Errorf("error should list account names, got: %v", err)
	}
	if !strings.Contains(msg, "--source-account") {
		t.Errorf("error should suggest --source-account, got: %v", err)
	}
}

func TestRun_ImportMultiAccountCSV_FiltersBySourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--file", dbPath,
		"--import", csvPath,
		"--account", "Checking",
		"--source-account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "IMPORT PREVIEW") {
		t.Errorf("expected dry-run preview output, got: %s", out)
	}
	if !strings.Contains(out, "Parsed: 2 transactions") {
		t.Errorf("expected 2 parsed (Checking only), got: %s", out)
	}
}

func TestRun_ImportMultiAccountCSV_UnknownSourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--file", dbPath,
		"--import", csvPath,
		"--account", "Checking",
		"--source-account", "Brokerage",
	}, stdout, stderr)

	if err == nil {
		t.Fatal("expected error when --source-account is not in the CSV")
	}
	if !strings.Contains(err.Error(), "not found in import file") {
		t.Errorf("error should mention source-account not found, got: %v", err)
	}
}
