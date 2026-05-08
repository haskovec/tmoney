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
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestRun_ScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--scheduled"}, stdout, stderr)
	if err == nil {
		t.Error("run(--scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_PostScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--post-scheduled", "abc123"}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_SkipScheduledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--skip-scheduled", "abc123"}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ScheduledNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SCHEDULED TRANSACTIONS") {
		t.Error("output should contain SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "No scheduled transactions found") {
		t.Error("output should indicate no scheduled transactions found")
	}
}

func TestRun_ScheduledWithTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Netflix")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(
		acct.ID,
		scheduled.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-15.99"),
	)
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SCHEDULED TRANSACTIONS") {
		t.Error("output should contain SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "-$15.99") {
		t.Error("output should contain amount")
	}
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Error("output should show count of scheduled transactions")
	}
}

func TestRun_ScheduledDueOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a due scheduled transaction (today)
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(
		acct.ID,
		scheduled.FrequencyMonthly,
		types.Today(), // Due today
		types.MustNewMoney("-10.00"),
	)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled --due command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--due", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled --due) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "DUE SCHEDULED TRANSACTIONS") {
		t.Error("output should contain DUE SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Error("output should show count of due transactions")
	}
}

func TestRun_ScheduledFilterByAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create two test accounts
	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	// Create scheduled transactions for each account
	stRepo := scheduled.NewRepository(database)
	st1 := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-10.00"))
	if err := stRepo.Create(st1); err != nil {
		t.Fatalf("failed to create scheduled transaction 1: %v", err)
	}
	st2 := scheduled.NewTransactionWithAmount(savings.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-20.00"))
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction 2: %v", err)
	}

	database.Close()

	// Run the scheduled command filtered by account
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled --account Checking) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Errorf("output should show 1 scheduled transaction, got: %s", output)
	}
	if !strings.Contains(output, "-$10.00") {
		t.Error("output should contain the checking account scheduled transaction")
	}
}

func TestRun_PostScheduledInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestRun_PostScheduledNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Use a valid UUID format that doesn't exist
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--post-scheduled) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_PostScheduledSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Netflix")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	// Post the scheduled transaction
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--post-scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "posted successfully") {
		t.Error("output should confirm posting")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "-$15.99") {
		t.Error("output should contain amount")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "Next:") {
		t.Error("output should show next date")
	}

	// Verify the transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := transaction.NewRepository(database)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(txns))
	}
}

func TestRun_PostScheduledWithCustomAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction (variable amount)
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransaction(acct.ID, scheduled.FrequencyMonthly, types.Today())
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	// Post with custom amount
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--post-scheduled", stID, "--amount", "-25.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--post-scheduled) with --amount returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "-$25.00") {
		t.Error("output should contain custom amount")
	}
}

func TestRun_SkipScheduledInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestRun_SkipScheduledNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--skip-scheduled) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_SkipScheduledSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Netflix")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()
	originalNextDate := st.NextDate.String()

	database.Close()

	// Skip the scheduled transaction
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--skip-scheduled", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--skip-scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "skipped") {
		t.Error("output should confirm skipping")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Netflix") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Monthly") {
		t.Error("output should contain frequency")
	}
	if !strings.Contains(output, "Skipped:") {
		t.Error("output should show skipped date")
	}
	if !strings.Contains(output, originalNextDate) {
		t.Error("output should show original date in Skipped field")
	}
	if !strings.Contains(output, "Next:") {
		t.Error("output should show next date")
	}

	// Verify no transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := transaction.NewRepository(database)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("expected 0 transactions after skip, got %d", len(txns))
	}
}

func TestRun_ScheduledVariableAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with variable amount (no amount set)
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransaction(acct.ID, scheduled.FrequencyMonthly, types.Today())
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	// Variable amount should show as "~"
	if !strings.Contains(output, "~") {
		t.Error("output should show ~ for variable amount")
	}
}

func TestRun_ScheduledWithOccurrences(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with limited occurrences
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	st.SetOccurrences(5)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	// Run the scheduled command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--scheduled) returned error: %v", err)
		return
	}

	output := stdout.String()
	// Should show occurrences remaining
	if !strings.Contains(output, "(5 left)") {
		t.Error("output should show occurrences remaining")
	}
}

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

func TestRun_StartReconcileMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--account", "Checking", "--statement-date", "2024-01-31", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_StartReconcileMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--statement-date", "2024-01-31", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_StartReconcileMissingDate(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--account", "Checking", "--statement-balance", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --statement-date")
	}
	if !strings.Contains(err.Error(), "requires --statement-date") {
		t.Errorf("error should mention --statement-date, got: %v", err)
	}
}

func TestRun_StartReconcileMissingBalance(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--start-reconcile", "--file", "test.tdb", "--account", "Checking", "--statement-date", "2024-01-31"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --statement-balance")
	}
	if !strings.Contains(err.Error(), "requires --statement-balance") {
		t.Errorf("error should mention --statement-balance, got: %v", err)
	}
}

func TestRun_StartReconcileSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
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

	// Create some transactions
	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-10"), types.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--start-reconcile",
		"--file", dbPath,
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "850.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--start-reconcile) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation started for Checking") {
		t.Error("output should confirm reconciliation started")
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$850.00") {
		t.Error("output should contain statement balance")
	}
	if !strings.Contains(output, "Unreconciled transactions: 2") {
		t.Errorf("output should show 2 unreconciled transactions, got:\n%s", output)
	}
}

func TestRun_StartReconcileAccountNotFound(t *testing.T) {
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
		"--start-reconcile",
		"--file", dbPath,
		"--account", "NonExistent",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000",
	}, stdout, stderr)
	if err == nil {
		t.Error("should fail with nonexistent account")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestRun_MarkReconciledMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--mark-reconciled", "some-id", "--file", ""}, stdout, stderr)
	// --mark-reconciled without --file should fail
	if err == nil {
		t.Error("should fail without proper --file")
	}
}

func TestRun_MarkReconciledNoIDs(t *testing.T) {
	_, _, err := parseArgs([]string{"--mark-reconciled"})
	if err == nil {
		t.Error("--mark-reconciled without IDs should return parse error")
	}
	if !strings.Contains(err.Error(), "requires at least one") {
		t.Errorf("error should mention requiring IDs, got: %v", err)
	}
}

func TestRun_FinishReconcileMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--finish-reconcile", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_FinishReconcileMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--finish-reconcile", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_FinishReconcileNoActiveSession(t *testing.T) {
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
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail with no active session")
	}
	if !strings.Contains(err.Error(), "no active reconciliation") {
		t.Errorf("error should mention no active session, got: %v", err)
	}
}

func TestRun_FinishReconcileSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account with opening balance 1000
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

	// Create transactions: -50 and -100 = 850 total balance
	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-10"), types.MustNewMoney("-100.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Start a reconciliation session
	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("850.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish reconciliation
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--finish-reconcile) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation completed for Checking") {
		t.Errorf("output should confirm completion, got:\n%s", output)
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$850.00") {
		t.Error("output should contain balance")
	}

	// Verify transactions are now reconciled
	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo2 := transaction.NewRepository(database2)
	reconciledTxn1, err := txnRepo2.GetByID(txn1.ID)
	if err != nil {
		t.Fatalf("failed to get transaction: %v", err)
	}
	if reconciledTxn1.Status != transaction.StatusReconciled {
		t.Errorf("transaction 1 should be reconciled, got %q", reconciledTxn1.Status)
	}

	reconciledTxn2, err := txnRepo2.GetByID(txn2.ID)
	if err != nil {
		t.Fatalf("failed to get transaction: %v", err)
	}
	if reconciledTxn2.Status != transaction.StatusReconciled {
		t.Errorf("transaction 2 should be reconciled, got %q", reconciledTxn2.Status)
	}
}

func TestRun_FinishReconcileWithDifference(t *testing.T) {
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

	// Create a transaction: balance is 1000 - 50 = 950
	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Create session with wrong balance (should cause difference)
	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"), // Wrong balance
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish should fail due to difference
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail with non-zero difference")
	}
	if !strings.Contains(err.Error(), "Difference") {
		t.Errorf("error should mention difference, got: %v", err)
	}
}

func TestRun_FinishReconcileWithForce(t *testing.T) {
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
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"), // Wrong balance
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	// Finish with --force should succeed
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--finish-reconcile", "--file", dbPath, "--account", "Checking", "--force"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--finish-reconcile --force) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Reconciliation completed for Checking") {
		t.Errorf("output should confirm completion, got:\n%s", output)
	}
}

func TestRun_ReconcileStatusMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--reconcile-status", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --file")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file, got: %v", err)
	}
}

func TestRun_ReconcileStatusMissingAccount(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--reconcile-status", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("should fail without --account")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account, got: %v", err)
	}
}

func TestRun_ReconcileStatusNoSession(t *testing.T) {
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
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--reconcile-status", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--reconcile-status) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Error("output should contain status header")
	}
	if !strings.Contains(output, "Last reconciled:  Never") {
		t.Errorf("output should show 'Never' for last reconciled, got:\n%s", output)
	}
	if !strings.Contains(output, "Current session:  None") {
		t.Errorf("output should show 'None' for current session, got:\n%s", output)
	}
}

func TestRun_ReconcileStatusWithActiveSession(t *testing.T) {
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

	// Create an active reconciliation session
	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--reconcile-status", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--reconcile-status) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Error("output should contain status header")
	}
	if !strings.Contains(output, "In progress") {
		t.Errorf("output should show 'In progress', got:\n%s", output)
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Error("output should contain statement date")
	}
	if !strings.Contains(output, "$5000.00") {
		t.Errorf("output should contain statement balance, got:\n%s", output)
	}
}

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
	err = run([]string{
		"--start-reconcile",
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
	err = run([]string{
		"--reconcile-status",
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
	err = run([]string{
		"--finish-reconcile",
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
	err = run([]string{
		"--reconcile-status",
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

func TestParseArgs_ReconciliationFlags(t *testing.T) {
	// Test --start-reconcile flag parsing
	opts, _, err := parseArgs([]string{
		"--start-reconcile",
		"--file", "test.tdb",
		"--account", "Checking",
		"--statement-date", "2024-01-31",
		"--statement-balance", "5000.00",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.startReconcile {
		t.Error("startReconcile should be true")
	}
	if opts.statementDate != "2024-01-31" {
		t.Errorf("statementDate should be 2024-01-31, got %q", opts.statementDate)
	}
	if opts.statementBalance != "5000.00" {
		t.Errorf("statementBalance should be 5000.00, got %q", opts.statementBalance)
	}

	// Test --finish-reconcile with --force
	opts, _, err = parseArgs([]string{
		"--finish-reconcile",
		"--force",
		"--file", "test.tdb",
		"--account", "Checking",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.finishReconcile {
		t.Error("finishReconcile should be true")
	}
	if !opts.reconcileForce {
		t.Error("reconcileForce should be true")
	}

	// Test --mark-reconciled with multiple IDs
	opts, _, err = parseArgs([]string{
		"--mark-reconciled", "id1", "id2", "id3",
		"--file", "test.tdb",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if len(opts.markReconciled) != 3 {
		t.Errorf("markReconciled should have 3 IDs, got %d", len(opts.markReconciled))
	}

	// Test --reconcile-status
	opts, _, err = parseArgs([]string{
		"--reconcile-status",
		"--file", "test.tdb",
		"--account", "Checking",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if !opts.reconcileStatus {
		t.Error("reconcileStatus should be true")
	}

	// Test = form for statement-date and statement-balance
	opts, _, err = parseArgs([]string{
		"--start-reconcile",
		"--statement-date=2024-02-28",
		"--statement-balance=1234.56",
	})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if opts.statementDate != "2024-02-28" {
		t.Errorf("statementDate should be 2024-02-28, got %q", opts.statementDate)
	}
	if opts.statementBalance != "1234.56" {
		t.Errorf("statementBalance should be 1234.56, got %q", opts.statementBalance)
	}
}

// =============================================================================
// Scheduled Transactions Auto-Post Indicator Tests
// =============================================================================

func TestRun_ScheduledShowsAutoPostIndicator(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with auto-post
	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(
		acct.ID,
		scheduled.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-1500.00"),
	)
	st.SetAutoPost(true)
	st.SetPostLeadDays(3)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	// Create another without auto-post
	st2 := scheduled.NewTransactionWithAmount(
		acct.ID,
		scheduled.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-50.00"),
	)
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--scheduled) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto 3d]") {
		t.Errorf("output should contain [Auto 3d] indicator, got: %s", output)
	}
	if !strings.Contains(output, "Auto") {
		t.Error("output should contain Auto header column")
	}
}

func TestRun_ScheduledAutoPostZeroLeadDays(t *testing.T) {
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
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(
		acct.ID,
		scheduled.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-100.00"),
	)
	st.SetAutoPost(true)
	// PostLeadDays defaults to 0
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--scheduled", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--scheduled) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto]") {
		t.Errorf("output should contain [Auto] indicator, got: %s", output)
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

// SM-097: --list-securities

func TestRun_ListSecuritiesMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-securities"}, stdout, stderr)
	if err == nil {
		t.Error("run(--list-securities) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ListSecuritiesEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-securities) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No securities found") {
		t.Errorf("output should say no securities found, got: %s", output)
	}
}

func TestRun_ListSecuritiesWithData(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-securities) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SECURITIES") {
		t.Error("output should contain SECURITIES header")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker AAPL")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain security name")
	}
	if !strings.Contains(output, "Stock") {
		t.Error("output should contain security type")
	}
	if !strings.Contains(output, "Large Cap Stock") {
		t.Error("output should contain asset class")
	}
}

func TestRun_ListSecuritiesExcludesHiddenByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}
	database.Close()

	// Without --include-hidden, hidden securities should not appear
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run returned error: %v", err)
		return
	}
	if strings.Contains(stdout.String(), "MSFT") {
		t.Error("hidden security should not appear without --include-hidden")
	}

	// With --include-hidden, it should appear
	stdout.Reset()
	err = run([]string{"--list-securities", "--file", dbPath, "--include-hidden"}, stdout, stderr)
	if err != nil {
		t.Errorf("run returned error: %v", err)
		return
	}
	if !strings.Contains(stdout.String(), "MSFT") {
		t.Error("hidden security should appear with --include-hidden")
	}
	if !strings.Contains(stdout.String(), "[hidden]") {
		t.Error("output should indicate hidden status")
	}
}

func TestRun_ListSecuritiesFilterByType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	stock := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := repo.Create(stock); err != nil {
		t.Fatalf("failed to create stock: %v", err)
	}
	etf := security.NewSecurity("SPY", "SPDR S&P 500 ETF", security.TypeETF)
	if err := repo.Create(etf); err != nil {
		t.Fatalf("failed to create etf: %v", err)
	}
	database.Close()

	// Filter by etf only
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-securities", "--file", dbPath, "--type", "etf"}, stdout, stderr)
	if err != nil {
		t.Errorf("run returned error: %v", err)
		return
	}
	output := stdout.String()
	if !strings.Contains(output, "SPY") {
		t.Error("output should contain ETF")
	}
	if strings.Contains(output, "AAPL") {
		t.Error("output should not contain stock when filtering by etf")
	}
}

func TestRun_ListSecuritiesFilterByAssetClass(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	stock := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	stock.AssetClass = security.AssetClassLargeCapStock
	if err := repo.Create(stock); err != nil {
		t.Fatalf("failed to create stock: %v", err)
	}
	bond := security.NewSecurity("BND", "Vanguard Bond ETF", security.TypeETF)
	bond.AssetClass = security.AssetClassDomesticBond
	if err := repo.Create(bond); err != nil {
		t.Fatalf("failed to create bond: %v", err)
	}
	database.Close()

	// Filter by domestic_bond
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-securities", "--file", dbPath, "--asset-class", "domestic_bond"}, stdout, stderr)
	if err != nil {
		t.Errorf("run returned error: %v", err)
		return
	}
	output := stdout.String()
	if !strings.Contains(output, "BND") {
		t.Error("output should contain bond ETF")
	}
	if strings.Contains(output, "AAPL") {
		t.Error("output should not contain stock when filtering by domestic_bond")
	}
}

// SM-098: --security (show detail)

func TestRun_SecurityDetailMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--security", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--security) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_SecurityDetailNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--security", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--security FAKE) should return error for non-existent security")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_SecurityDetailShowsFull(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--security AAPL) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SECURITY: AAPL") {
		t.Error("output should contain security header")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain name")
	}
	if !strings.Contains(output, "Stock") {
		t.Error("output should contain type")
	}
	if !strings.Contains(output, "Large Cap Stock") {
		t.Error("output should contain asset class")
	}
	if !strings.Contains(output, "USD") {
		t.Error("output should contain currency")
	}
	if !strings.Contains(output, "NASDAQ") {
		t.Error("output should contain exchange")
	}
	if !strings.Contains(output, "Active") {
		t.Error("output should show active status")
	}
}

// SM-099: --add-security

func TestRun_AddSecurityMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-security", "--ticker", "AAPL", "--name", "Apple", "--type", "stock"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-security) without --file should return error")
	}
}

func TestRun_AddSecurityMissingTicker(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-security", "--file", "/fake.tdb", "--name", "Apple", "--type", "stock"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-security) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "--ticker") {
		t.Errorf("error should mention --ticker, got: %v", err)
	}
}

func TestRun_AddSecurityMissingName(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-security", "--file", "/fake.tdb", "--ticker", "AAPL", "--type", "stock"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-security) without --name should return error")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("error should mention --name, got: %v", err)
	}
}

func TestRun_AddSecurityMissingType(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-security", "--file", "/fake.tdb", "--ticker", "AAPL", "--name", "Apple"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-security) without --type should return error")
	}
	if !strings.Contains(err.Error(), "--type") {
		t.Errorf("error should mention --type, got: %v", err)
	}
}

func TestRun_AddSecurityInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-security", "--file", dbPath, "--ticker", "AAPL", "--name", "Apple", "--type", "invalid_type"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-security) with invalid --type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error should mention invalid type, got: %v", err)
	}
}

func TestRun_AddSecuritySuccess(t *testing.T) {
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
		"--add-security",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple Inc.",
		"--type", "stock",
		"--asset-class", "large_cap_stock",
		"--currency", "USD",
		"--exchange", "NASDAQ",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Security created successfully") {
		t.Error("output should confirm creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain name")
	}
	if !strings.Contains(output, "NASDAQ") {
		t.Error("output should contain exchange")
	}

	// Verify security is persisted by listing
	stdout.Reset()
	err = run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-securities) returned error: %v", err)
		return
	}
	if !strings.Contains(stdout.String(), "AAPL") {
		t.Error("security should be persisted and visible in list")
	}
}

func TestRun_AddSecurityDefaultValues(t *testing.T) {
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
		"--add-security",
		"--file", dbPath,
		"--ticker", "GOOG",
		"--name", "Alphabet Inc.",
		"--type", "stock",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "USD") {
		t.Error("default currency should be USD")
	}
	if !strings.Contains(output, "Unclassified") {
		t.Error("default asset class should be Unclassified")
	}
}

func TestRun_AddSecurityDuplicate(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--add-security",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple Again",
		"--type", "stock",
	}, stdout, stderr)
	if err == nil {
		t.Error("adding duplicate ticker should return error")
	}
}

// SM-100: --edit-security

func TestRun_EditSecurityMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--edit-security", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--edit-security) without --file should return error")
	}
}

func TestRun_EditSecurityNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--edit-security", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("editing non-existent security should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_EditSecurityChangeName(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--edit-security", "AAPL",
		"--file", dbPath,
		"--name", "Apple Corporation",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--edit-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Security updated successfully") {
		t.Error("output should confirm update")
	}
	if !strings.Contains(output, "Apple Corporation") {
		t.Error("output should show updated name")
	}

	// Verify the change was persisted
	stdout.Reset()
	err = run([]string{"--security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--security) returned error: %v", err)
		return
	}
	if !strings.Contains(stdout.String(), "Apple Corporation") {
		t.Error("name change should be persisted")
	}
}

func TestRun_EditSecurityChangeTicker(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--edit-security", "AAPL",
		"--file", dbPath,
		"--ticker", "AAPL2",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--edit-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "AAPL2") {
		t.Error("output should show new ticker")
	}

	// Old ticker should not be found
	stdout.Reset()
	err = run([]string{"--security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("old ticker should not be found after rename")
	}

	// New ticker should be found
	stdout.Reset()
	err = run([]string{"--security", "AAPL2", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("new ticker should be found, got error: %v", err)
	}
}

func TestRun_EditSecurityChangeType(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--edit-security", "AAPL",
		"--file", dbPath,
		"--type", "etf",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--edit-security) returned error: %v", err)
		return
	}

	if !strings.Contains(stdout.String(), "ETF") {
		t.Error("output should show updated type")
	}
}

func TestRun_EditSecurityInvalidType(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{
		"--edit-security", "AAPL",
		"--file", dbPath,
		"--type", "invalid",
	}, stdout, stderr)
	if err == nil {
		t.Error("editing with invalid type should return error")
	}
}

// SM-101: --hide-security / --unhide-security

func TestRun_HideSecurityMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--hide-security", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--hide-security) without --file should return error")
	}
}

func TestRun_HideSecurityNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--hide-security", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("hiding non-existent security should return error")
	}
}

func TestRun_HideSecuritySuccess(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--hide-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--hide-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "hidden successfully") {
		t.Error("output should confirm hiding")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}

	// Security should no longer appear in default list
	stdout.Reset()
	err = run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-securities) returned error: %v", err)
		return
	}
	if strings.Contains(stdout.String(), "AAPL") {
		t.Error("hidden security should not appear in default listing")
	}
}

func TestRun_HideSecurityAlreadyHidden(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--hide-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("hiding already hidden security should return error")
	}
}

func TestRun_UnhideSecurityMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--unhide-security", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--unhide-security) without --file should return error")
	}
}

func TestRun_UnhideSecuritySuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--unhide-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--unhide-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "unhidden successfully") {
		t.Error("output should confirm unhiding")
	}

	// Security should now appear in default list
	stdout.Reset()
	err = run([]string{"--list-securities", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-securities) returned error: %v", err)
		return
	}
	if !strings.Contains(stdout.String(), "AAPL") {
		t.Error("unhidden security should appear in default listing")
	}
}

func TestRun_UnhideSecurityNotHidden(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--unhide-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("unhiding visible security should return error")
	}
}

// SM-102: --delete-security

func TestRun_DeleteSecurityMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--delete-security", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--delete-security) without --file should return error")
	}
}

func TestRun_DeleteSecurityNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--delete-security", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("deleting non-existent security should return error")
	}
}

func TestRun_DeleteSecuritySuccess(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--delete-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--delete-security) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "deleted successfully") {
		t.Error("output should confirm deletion")
	}

	// Security should no longer exist
	stdout.Reset()
	err = run([]string{"--security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("deleted security should not be found")
	}
}

func TestRun_DeleteSecurityWithPricesSuggestsHide(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	// Add a price to create a dependency
	_, err = database.Conn().Exec(
		`INSERT INTO security_prices (id, security_id, date, price, source, created_at)
		 VALUES (?, ?, '2024-01-01', 150.00, 'manual', CURRENT_TIMESTAMP)`,
		types.NewID().String(), sec.ID.String(),
	)
	if err != nil {
		t.Fatalf("failed to create price: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--delete-security", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("deleting security with prices should return error")
	}
	if !strings.Contains(err.Error(), "--hide-security") {
		t.Errorf("error should suggest using --hide-security, got: %v", err)
	}
}

// Args parsing tests for security flags

func TestParseArgs_SecurityFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, opts *cliOptions)
	}{
		{
			"list-securities flag",
			[]string{"--list-securities"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.listSecurities {
					t.Error("listSecurities should be true")
				}
			},
		},
		{
			"security detail flag",
			[]string{"--security", "AAPL"},
			func(t *testing.T, opts *cliOptions) {
				if opts.securityTicker != "AAPL" {
					t.Errorf("securityTicker = %q, want AAPL", opts.securityTicker)
				}
			},
		},
		{
			"security detail equals format",
			[]string{"--security=GOOG"},
			func(t *testing.T, opts *cliOptions) {
				if opts.securityTicker != "GOOG" {
					t.Errorf("securityTicker = %q, want GOOG", opts.securityTicker)
				}
			},
		},
		{
			"add-security flag",
			[]string{"--add-security"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.addSecurity {
					t.Error("addSecurity should be true")
				}
			},
		},
		{
			"edit-security flag",
			[]string{"--edit-security", "AAPL"},
			func(t *testing.T, opts *cliOptions) {
				if opts.editSecurity != "AAPL" {
					t.Errorf("editSecurity = %q, want AAPL", opts.editSecurity)
				}
			},
		},
		{
			"hide-security flag",
			[]string{"--hide-security", "AAPL"},
			func(t *testing.T, opts *cliOptions) {
				if opts.hideSecurity != "AAPL" {
					t.Errorf("hideSecurity = %q, want AAPL", opts.hideSecurity)
				}
			},
		},
		{
			"unhide-security flag",
			[]string{"--unhide-security", "AAPL"},
			func(t *testing.T, opts *cliOptions) {
				if opts.unhideSecurity != "AAPL" {
					t.Errorf("unhideSecurity = %q, want AAPL", opts.unhideSecurity)
				}
			},
		},
		{
			"delete-security flag",
			[]string{"--delete-security", "AAPL"},
			func(t *testing.T, opts *cliOptions) {
				if opts.deleteSecurity != "AAPL" {
					t.Errorf("deleteSecurity = %q, want AAPL", opts.deleteSecurity)
				}
			},
		},
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
			"include-hidden flag",
			[]string{"--include-hidden"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.includeHidden {
					t.Error("includeHidden should be true")
				}
			},
		},
		{
			"combined security flags",
			[]string{"--add-security", "--ticker", "AAPL", "--name", "Apple", "--type", "stock", "--asset-class", "large_cap_stock", "--exchange", "NASDAQ"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.addSecurity {
					t.Error("addSecurity should be true")
				}
				if opts.secTicker != "AAPL" {
					t.Errorf("secTicker = %q, want AAPL", opts.secTicker)
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
		{"security missing ticker", []string{"--security"}},
		{"edit-security missing ticker", []string{"--edit-security"}},
		{"hide-security missing ticker", []string{"--hide-security"}},
		{"unhide-security missing ticker", []string{"--unhide-security"}},
		{"delete-security missing ticker", []string{"--delete-security"}},
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

func TestRun_ListPricesMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--prices) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ListPricesMissingTicker(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--prices) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "--ticker") {
		t.Errorf("error should mention --ticker, got: %v", err)
	}
}

func TestRun_ListPricesSecurityNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "ZZZZ", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--prices) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_ListPricesShowsAll(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--prices) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "PRICES: AAPL") {
		t.Error("output should contain prices header")
	}
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should contain first price date")
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain second price date")
	}
	if !strings.Contains(output, "2024-03-15") {
		t.Error("output should contain third price date")
	}
	if !strings.Contains(output, "150.00") {
		t.Error("output should contain first price value")
	}
	if !strings.Contains(output, "160.50") {
		t.Error("output should contain second price value")
	}
	if !strings.Contains(output, "170.25") {
		t.Error("output should contain third price value")
	}
	if !strings.Contains(output, "Manual") {
		t.Error("output should contain manual source")
	}
	if !strings.Contains(output, "Transaction") {
		t.Error("output should contain transaction source")
	}
	if !strings.Contains(output, "Import") {
		t.Error("output should contain import source")
	}
	if !strings.Contains(output, "Total: 3 price(s)") {
		t.Error("output should contain total count")
	}
}

func TestRun_ListPricesWithFromFilter(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath, "--from", "2024-02-01"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--prices --from) returned error: %v", err)
		return
	}

	output := stdout.String()
	if strings.Contains(output, "2024-01-15") {
		t.Error("output should not contain price before --from date")
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain price on/after --from date")
	}
	if !strings.Contains(output, "2024-03-15") {
		t.Error("output should contain price on/after --from date")
	}
	if !strings.Contains(output, "Total: 2 price(s)") {
		t.Error("output should contain total count of 2")
	}
}

func TestRun_ListPricesWithToFilter(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath, "--to", "2024-02-28"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--prices --to) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should contain price before --to date")
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain price on/before --to date")
	}
	if strings.Contains(output, "2024-03-15") {
		t.Error("output should not contain price after --to date")
	}
	if !strings.Contains(output, "Total: 2 price(s)") {
		t.Error("output should contain total count of 2")
	}
}

func TestRun_ListPricesNoPrices(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--prices) with no prices returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No prices found") {
		t.Error("output should indicate no prices found")
	}
}

// =============================================================================
// SM-104: --add-price
// =============================================================================

func TestRun_AddPriceMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "150.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddPriceMissingTicker(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--file", "/fake.tdb", "--date", "2024-01-15", "--price", "150.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "--ticker") {
		t.Errorf("error should mention --ticker, got: %v", err)
	}
}

func TestRun_AddPriceMissingDate(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--file", "/fake.tdb", "--ticker", "AAPL", "--price", "150.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) without --date should return error")
	}
	if !strings.Contains(err.Error(), "--date") {
		t.Errorf("error should mention --date, got: %v", err)
	}
}

func TestRun_AddPriceMissingPrice(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--file", "/fake.tdb", "--ticker", "AAPL", "--date", "2024-01-15"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) without --price should return error")
	}
	if !strings.Contains(err.Error(), "--price") {
		t.Errorf("error should mention --price, got: %v", err)
	}
}

func TestRun_AddPriceSuccess(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "150.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-price) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Price added") {
		t.Error("output should confirm price was added")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should contain date")
	}
	if !strings.Contains(output, "150.00") {
		t.Error("output should contain price value")
	}

	// Verify price was actually stored by listing prices
	stdout.Reset()
	err = run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--prices) returned error: %v", err)
		return
	}
	if !strings.Contains(stdout.String(), "150.00") {
		t.Error("price should be visible in --prices listing")
	}
}

func TestRun_AddPriceSecurityNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "ZZZZ", "--date", "2024-01-15", "--price", "150.00", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AddPriceDuplicateConflict(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	// Try to add a price for a date that already has one (2024-01-15)
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "155.00", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) with duplicate date should return error")
	}
}

func TestRun_AddPriceInvalidDate(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "not-a-date", "--price", "150.00", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_AddPriceInvalidPrice(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "not-a-number", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-price) with invalid price should return error")
	}
	if !strings.Contains(err.Error(), "invalid --price") {
		t.Errorf("error should mention invalid price, got: %v", err)
	}
}

// =============================================================================
// SM-105: --current-price
// =============================================================================

func TestRun_CurrentPriceMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--current-price", "--ticker", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Error("run(--current-price) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_CurrentPriceMissingTicker(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--current-price", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run(--current-price) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "--ticker") {
		t.Errorf("error should mention --ticker, got: %v", err)
	}
}

func TestRun_CurrentPriceSecurityNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--current-price", "--ticker", "ZZZZ", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--current-price) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_CurrentPriceShowsMostRecent(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--current-price", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--current-price) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "CURRENT PRICE: AAPL") {
		t.Error("output should contain current price header")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain security name")
	}
	// The most recent price is 2024-03-15 at 170.25
	if !strings.Contains(output, "2024-03-15") {
		t.Error("output should contain most recent price date")
	}
	if !strings.Contains(output, "170.25") {
		t.Error("output should contain most recent price value")
	}
	if !strings.Contains(output, "Import") {
		t.Error("output should contain price source")
	}
}

func TestRun_CurrentPriceNoPriceExists(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--current-price", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--current-price) with no prices should return error")
	}
	if !strings.Contains(err.Error(), "no price found") {
		t.Errorf("error should mention no price found, got: %v", err)
	}
}

// Args parsing tests for price flags

func TestParseArgs_PriceFlags(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, opts *cliOptions)
	}{
		{
			"prices flag",
			[]string{"--prices"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.listPrices {
					t.Error("listPrices should be true")
				}
			},
		},
		{
			"add-price flag",
			[]string{"--add-price"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.addPrice {
					t.Error("addPrice should be true")
				}
			},
		},
		{
			"current-price flag",
			[]string{"--current-price"},
			func(t *testing.T, opts *cliOptions) {
				if !opts.currentPrice {
					t.Error("currentPrice should be true")
				}
			},
		},
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

// helper to run and return stdout
func run2(args []string) (string, error) {
	stdout := &bytes.Buffer{}
	err := run(args, stdout, &bytes.Buffer{})
	return stdout.String(), err
}

// helper to create pointer to Money
func ptrMoney(s string) *types.Money {
	m := types.MustNewMoney(s)
	return &m
}

// --- SM-106: CLI --buy ---

func TestRun_BuyMissingFile(t *testing.T) {
	err := run([]string{"--buy", "--account", "Brokerage", "--ticker", "AAPL", "--shares", "10", "--amount", "1500"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_BuyMissingAccount(t *testing.T) {
	err := run([]string{"--buy", "--file", "test.tdb", "--ticker", "AAPL", "--shares", "10", "--amount", "1500"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_BuyMissingTicker(t *testing.T) {
	err := run([]string{"--buy", "--file", "test.tdb", "--account", "Brokerage", "--shares", "10", "--amount", "1500"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ticker") {
		t.Errorf("expected --ticker required error, got: %v", err)
	}
}

func TestRun_BuyMissingShares(t *testing.T) {
	err := run([]string{"--buy", "--file", "test.tdb", "--account", "Brokerage", "--ticker", "AAPL", "--amount", "1500"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --shares") {
		t.Errorf("expected --shares required error, got: %v", err)
	}
}

func TestRun_BuyMissingAmountAndPrice(t *testing.T) {
	err := run([]string{"--buy", "--file", "test.tdb", "--account", "Brokerage", "--ticker", "AAPL", "--shares", "10"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("expected --amount/--price-per-share required error, got: %v", err)
	}
}

func TestRun_BuyWithTotalAmount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--buy) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "Brokerage") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "10") {
		t.Error("output should contain shares")
	}
}

func TestRun_BuyWithPricePerShare(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--buy with price-per-share) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation")
	}
	if !strings.Contains(output, "$150.00") {
		t.Error("output should contain price per share")
	}
}

func TestRun_BuyWithCommission(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1510",
		"--commission", "10",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--buy with commission) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Commission") {
		t.Error("output should show commission")
	}
}

func TestRun_BuyWithDateAndMemo(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "150",
		"--date", "2025-06-15",
		"--memo", "Buying AAPL dip",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--buy with date/memo) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-06-15") {
		t.Error("output should contain the specified date")
	}
}

func TestRun_BuyAccountNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "NonExistent",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestRun_BuySecurityNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "FAKE",
		"--shares", "10",
		"--amount", "1500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestRun_BuyInsufficientCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "1000",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	// 1000 * 150 = 150,000 > 50,000 cash
	if err == nil {
		t.Error("expected insufficient cash error")
	}
}

func TestRun_BuyWithLotTracking(t *testing.T) {
	dbPath := createInvestmentTestDB(t, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--buy with lot tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation with lot tracking")
	}

	// Verify a lot was created
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	svc := app.NewServices(database)
	sec, _ := svc.Security.GetByTicker("AAPL", "")
	lots, err := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if err != nil {
		t.Fatalf("failed to list lots: %v", err)
	}
	if len(lots) != 1 {
		t.Errorf("expected 1 lot, got %d", len(lots))
	}
}

// --- SM-107: CLI --sell ---

func TestRun_SellMissingFile(t *testing.T) {
	err := run([]string{"--sell", "--account", "Brokerage", "--ticker", "AAPL", "--shares", "5", "--amount", "800"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_SellMissingAccount(t *testing.T) {
	err := run([]string{"--sell", "--file", "test.tdb", "--ticker", "AAPL", "--shares", "5", "--amount", "800"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_SellMissingTicker(t *testing.T) {
	err := run([]string{"--sell", "--file", "test.tdb", "--account", "Brokerage", "--shares", "5", "--amount", "800"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ticker") {
		t.Errorf("expected --ticker required error, got: %v", err)
	}
}

func TestRun_SellMissingShares(t *testing.T) {
	err := run([]string{"--sell", "--file", "test.tdb", "--account", "Brokerage", "--ticker", "AAPL", "--amount", "800"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --shares") {
		t.Errorf("expected --shares required error, got: %v", err)
	}
}

func TestRun_SellBasic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	// First buy some shares
	_, err := run2([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	})
	if err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	// Now sell some
	stdout := &bytes.Buffer{}
	err = run([]string{
		"--sell", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--sell) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Sell transaction created successfully") {
		t.Error("output should confirm sell creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
}

func TestRun_SellInsufficientShares(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	// Buy 10 shares
	_, err := run2([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	})
	if err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	// Try to sell 20
	err = run([]string{
		"--sell", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "20",
		"--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected insufficient shares error")
	}
}

func TestRun_SellWithLotAllocation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = true
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("50000"), "")
	if err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}
	_, err = svc.Investment.Buy(acct.ID, sec.ID, types.Today(), types.MustNewQuantity("10"), nil, ptrMoney("150"), types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy: %v", err)
	}

	// Get the lot ID
	lots, _ := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if len(lots) == 0 {
		t.Fatal("no lots found after buy")
	}
	lotID := lots[0].ID.String()
	database.Close()

	// Sell with --lot
	stdout := &bytes.Buffer{}
	err = run([]string{
		"--sell", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
		"--lot", lotID,
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--sell with lot) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Sell transaction created successfully") {
		t.Error("output should confirm sell with lot allocation")
	}
}

// --- SM-108: CLI --dividend ---

func TestRun_DividendMissingFile(t *testing.T) {
	err := run([]string{"--dividend", "--account", "Brokerage", "--ticker", "AAPL", "--amount", "50"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_DividendMissingAccount(t *testing.T) {
	err := run([]string{"--dividend", "--file", "test.tdb", "--ticker", "AAPL", "--amount", "50"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_DividendMissingTicker(t *testing.T) {
	err := run([]string{"--dividend", "--file", "test.tdb", "--account", "Brokerage", "--amount", "50"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ticker") {
		t.Errorf("expected --ticker required error, got: %v", err)
	}
}

func TestRun_DividendMissingAmount(t *testing.T) {
	err := run([]string{"--dividend", "--file", "test.tdb", "--account", "Brokerage", "--ticker", "AAPL"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("expected --amount required error, got: %v", err)
	}
}

func TestRun_DividendBasic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--dividend", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "125.50",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--dividend) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Dividend transaction created successfully") {
		t.Error("output should confirm dividend creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "$125.50") {
		t.Error("output should contain amount")
	}
}

// --- SM-109: CLI --reinvest ---

func TestRun_ReinvestMissingFile(t *testing.T) {
	err := run([]string{"--reinvest", "--account", "Brokerage", "--ticker", "AAPL", "--shares", "2", "--amount", "300"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_ReinvestMissingShares(t *testing.T) {
	err := run([]string{"--reinvest", "--file", "test.tdb", "--account", "Brokerage", "--ticker", "AAPL", "--amount", "300"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --shares") {
		t.Errorf("expected --shares required error, got: %v", err)
	}
}

func TestRun_ReinvestBasic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--reinvest", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--reinvest) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Reinvest dividend transaction created successfully") {
		t.Error("output should confirm reinvest creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
}

// --- SM-110: CLI --investment-fee ---

func TestRun_InvestmentFeeMissingFile(t *testing.T) {
	err := run([]string{"--investment-fee", "--account", "Brokerage", "--amount", "25"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_InvestmentFeeMissingAccount(t *testing.T) {
	err := run([]string{"--investment-fee", "--file", "test.tdb", "--amount", "25"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_InvestmentFeeMissingAmount(t *testing.T) {
	err := run([]string{"--investment-fee", "--file", "test.tdb", "--account", "Brokerage"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("expected --amount required error, got: %v", err)
	}
}

func TestRun_InvestmentFeeBasic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--investment-fee", "--file", dbPath,
		"--account", "Brokerage",
		"--amount", "25.00",
		"--memo", "Annual fee",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--investment-fee) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Investment fee transaction created successfully") {
		t.Error("output should confirm fee creation")
	}
	if !strings.Contains(output, "$25.00") {
		t.Error("output should contain fee amount")
	}
	if !strings.Contains(output, "Annual fee") {
		t.Error("output should contain memo")
	}
}

func TestRun_InvestmentFeeInsufficientCash(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	database.Close()

	err = run([]string{
		"--investment-fee", "--file", dbPath,
		"--account", "Brokerage",
		"--amount", "100",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected insufficient cash error for fee")
	}
}

// --- SM-111: CLI --invest-deposit / --invest-withdraw ---

func TestRun_InvestDepositMissingFile(t *testing.T) {
	err := run([]string{"--invest-deposit", "--account", "Brokerage", "--amount", "1000"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_InvestDepositMissingAccount(t *testing.T) {
	err := run([]string{"--invest-deposit", "--file", "test.tdb", "--amount", "1000"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("expected --account required error, got: %v", err)
	}
}

func TestRun_InvestDepositMissingAmount(t *testing.T) {
	err := run([]string{"--invest-deposit", "--file", "test.tdb", "--account", "Brokerage"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("expected --amount required error, got: %v", err)
	}
}

func TestRun_InvestDepositBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	err = run([]string{
		"--invest-deposit", "--file", dbPath,
		"--account", "Brokerage",
		"--amount", "5000",
		"--memo", "Initial funding",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--invest-deposit) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Investment deposit created successfully") {
		t.Error("output should confirm deposit creation")
	}
	if !strings.Contains(output, "$5000.00") {
		t.Error("output should contain deposit amount")
	}
	if !strings.Contains(output, "Initial funding") {
		t.Error("output should contain memo")
	}
}

func TestRun_InvestWithdrawMissingFile(t *testing.T) {
	err := run([]string{"--invest-withdraw", "--account", "Brokerage", "--amount", "500"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_InvestWithdrawBasic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--invest-withdraw", "--file", dbPath,
		"--account", "Brokerage",
		"--amount", "500",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--invest-withdraw) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Investment withdrawal created successfully") {
		t.Error("output should confirm withdrawal creation")
	}
	if !strings.Contains(output, "$500.00") {
		t.Error("output should contain withdrawal amount")
	}
}

func TestRun_InvestWithdrawInsufficientCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := run([]string{
		"--invest-withdraw", "--file", dbPath,
		"--account", "Brokerage",
		"--amount", "999999",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected insufficient cash error for withdrawal")
	}
}

// --- SM-112: CLI --transfer-shares ---

func TestRun_TransferSharesMissingFile(t *testing.T) {
	err := run([]string{"--transfer-shares", "--from", "A", "--to", "B", "--ticker", "AAPL", "--shares", "10"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_TransferSharesMissingFrom(t *testing.T) {
	err := run([]string{"--transfer-shares", "--file", "test.tdb", "--to", "B", "--ticker", "AAPL", "--shares", "10"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --from") {
		t.Errorf("expected --from required error, got: %v", err)
	}
}

func TestRun_TransferSharesMissingTo(t *testing.T) {
	err := run([]string{"--transfer-shares", "--file", "test.tdb", "--from", "A", "--ticker", "AAPL", "--shares", "10"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --to") {
		t.Errorf("expected --to required error, got: %v", err)
	}
}

func TestRun_TransferSharesMissingTicker(t *testing.T) {
	err := run([]string{"--transfer-shares", "--file", "test.tdb", "--from", "A", "--to", "B", "--shares", "10"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ticker") {
		t.Errorf("expected --ticker required error, got: %v", err)
	}
}

func TestRun_TransferSharesMissingShares(t *testing.T) {
	err := run([]string{"--transfer-shares", "--file", "test.tdb", "--from", "A", "--to", "B", "--ticker", "AAPL"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --shares") {
		t.Errorf("expected --shares required error, got: %v", err)
	}
}

func TestRun_TransferSharesBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	src := account.NewAccount("Source IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(src); err != nil {
		t.Fatalf("failed to create source account: %v", err)
	}
	dst := account.NewAccount("Dest 401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(dst); err != nil {
		t.Fatalf("failed to create dest account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(src.ID, types.Today(), types.MustNewMoney("50000"), "")
	if err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}
	_, err = svc.Investment.Buy(src.ID, sec.ID, types.Today(), types.MustNewQuantity("10"), nil, ptrMoney("150"), types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	err = run([]string{
		"--transfer-shares", "--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--transfer-shares) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Share transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "Source IRA") {
		t.Error("output should contain source account")
	}
	if !strings.Contains(output, "Dest 401k") {
		t.Error("output should contain dest account")
	}
}

// --- End-to-end: buy then sell verifies cash flow ---

func TestRun_BuyThenSellUpdatesCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	// Buy 10 shares at $150 = $1500 deducted from $50000
	_, err := run2([]string{
		"--buy", "--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	})
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	// Sell 5 shares at $160 = $800 received
	stdout := &bytes.Buffer{}
	err = run([]string{
		"--sell", "--file", dbPath,
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
		{"--buy flag", []string{"--buy"}, func(o *cliOptions) bool { return o.buy }},
		{"--sell flag", []string{"--sell"}, func(o *cliOptions) bool { return o.sell }},
		{"--dividend flag", []string{"--dividend"}, func(o *cliOptions) bool { return o.dividend }},
		{"--reinvest flag", []string{"--reinvest"}, func(o *cliOptions) bool { return o.reinvest }},
		{"--investment-fee flag", []string{"--investment-fee"}, func(o *cliOptions) bool { return o.investmentFee }},
		{"--invest-deposit flag", []string{"--invest-deposit"}, func(o *cliOptions) bool { return o.investDeposit }},
		{"--invest-withdraw flag", []string{"--invest-withdraw"}, func(o *cliOptions) bool { return o.investWithdraw }},
		{"--transfer-shares flag", []string{"--transfer-shares"}, func(o *cliOptions) bool { return o.transferShares }},
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

// =============================================================================
// SM-117: --import-prices
// =============================================================================

func TestRun_ImportPricesMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import-prices", "prices.csv"}, stdout, stderr)
	if err == nil {
		t.Error("run(--import-prices) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ImportPricesFileNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import-prices", "/nonexistent/prices.csv", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--import-prices) with nonexistent CSV should return error")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error should mention file open failure, got: %v", err)
	}
}

func TestRun_ImportPricesSuccess(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	// Create a temp CSV file
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-16,AAPL,152.50\n2024-01-17,AAPL,148.75\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import-prices", csvPath, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import-prices) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE") {
		t.Error("output should contain IMPORT COMPLETE")
	}
	if !strings.Contains(output, "Imported:       3") {
		t.Errorf("output should show 3 imported, got: %s", output)
	}

	// Verify prices were actually stored
	stdout.Reset()
	err = run([]string{"--prices", "--ticker", "AAPL", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--prices) returned error: %v", err)
	}
	priceOutput := stdout.String()
	if !strings.Contains(priceOutput, "150.00") {
		t.Error("price 150.00 should be in prices listing")
	}
	if !strings.Contains(priceOutput, "152.50") {
		t.Error("price 152.50 should be in prices listing")
	}
	if !strings.Contains(priceOutput, "148.75") {
		t.Error("price 148.75 should be in prices listing")
	}
}

func TestRun_ImportPricesWithOverwrite(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	// First, add a price manually
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-price", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "150.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--add-price) returned error: %v", err)
	}

	// Create CSV with same date but different price
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,155.00\n2024-01-16,AAPL,160.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	// Import WITHOUT overwrite — should skip the duplicate
	stdout.Reset()
	err = run([]string{"--import-prices", csvPath, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import-prices) returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Skipped:        1") {
		t.Errorf("expected 1 skipped, got: %s", output)
	}
	if !strings.Contains(output, "Imported:       1") {
		t.Errorf("expected 1 imported, got: %s", output)
	}

	// Import WITH --overwrite — should overwrite the existing
	stdout.Reset()
	err = run([]string{"--import-prices", csvPath, "--overwrite", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import-prices --overwrite) returned error: %v", err)
	}
	output = stdout.String()
	if !strings.Contains(output, "Imported:       2") {
		t.Errorf("expected 2 imported with overwrite, got: %s", output)
	}
	if !strings.Contains(output, "Skipped:        0") {
		t.Errorf("expected 0 skipped with overwrite, got: %s", output)
	}
}

func TestRun_ImportPricesUnknownTicker(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-15,ZZZZ,99.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import-prices", csvPath, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import-prices) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "unknown ticker") {
		t.Error("output should warn about unknown ticker")
	}
	if !strings.Contains(output, "ZZZZ") {
		t.Error("output should mention the unknown ticker ZZZZ")
	}
	if !strings.Contains(output, "Imported:       1") {
		t.Errorf("expected 1 imported (AAPL only), got: %s", output)
	}
	if !strings.Contains(output, "Unknown ticker: 1") {
		t.Errorf("expected 1 unknown ticker in summary, got: %s", output)
	}
}

func TestRun_ImportPricesDisplaysSummary(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-16,AAPL,152.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--import-prices", csvPath, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--import-prices) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE: prices.csv") {
		t.Error("output should show import complete with filename")
	}
	if !strings.Contains(output, "Total rows:") {
		t.Error("output should show total rows")
	}
	if !strings.Contains(output, "Imported:") {
		t.Error("output should show imported count")
	}
	if !strings.Contains(output, "Skipped:") {
		t.Error("output should show skipped count")
	}
}

func TestParseArgs_ImportPrices(t *testing.T) {
	opts, _, err := parseArgs([]string{"--import-prices", "prices.csv", "--file", "test.tdb"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.importPrices != "prices.csv" {
		t.Errorf("importPrices should be prices.csv, got %q", opts.importPrices)
	}
	if opts.file != "test.tdb" {
		t.Errorf("file should be test.tdb, got %q", opts.file)
	}
}

func TestParseArgs_ImportPricesWithOverwrite(t *testing.T) {
	opts, _, err := parseArgs([]string{"--import-prices", "prices.csv", "--overwrite", "--file", "test.tdb"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.importPrices != "prices.csv" {
		t.Errorf("importPrices should be prices.csv, got %q", opts.importPrices)
	}
	if !opts.overwrite {
		t.Error("overwrite should be true")
	}
}

func TestParseArgs_ImportPricesEqualsFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{"--import-prices=prices.csv", "--file", "test.tdb"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.importPrices != "prices.csv" {
		t.Errorf("importPrices should be prices.csv, got %q", opts.importPrices)
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

// --- SM-163: CLI --split ---

func TestRun_SplitMissingFile(t *testing.T) {
	err := run([]string{"--split", "--ticker", "AAPL", "--ratio", "4:1"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_SplitMissingTicker(t *testing.T) {
	err := run([]string{"--split", "--file", "test.tdb", "--ratio", "4:1"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ticker") {
		t.Errorf("expected --ticker required error, got: %v", err)
	}
}

func TestRun_SplitMissingRatio(t *testing.T) {
	err := run([]string{"--split", "--file", "test.tdb", "--ticker", "AAPL"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --ratio") {
		t.Errorf("expected --ratio required error, got: %v", err)
	}
}

func TestRun_SplitInvalidRatio(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, false)
	err := run([]string{"--split", "--file", dbPath, "--ticker", "AAPL", "--ratio", "invalid"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --ratio") {
		t.Errorf("expected invalid ratio error, got: %v", err)
	}
}

func TestRun_SplitSecurityNotFound(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, false)
	err := run([]string{"--split", "--file", dbPath, "--ticker", "ZZZZ", "--ratio", "4:1"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestRun_SplitForwardSplit(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--split", "--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "4:1",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--split) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Stock split applied successfully") {
		t.Error("output should confirm split")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "4:1") {
		t.Error("output should contain ratio")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestRun_SplitReverseSplit(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--split", "--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "1:10",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--split reverse) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Stock split applied successfully") {
		t.Error("output should confirm split")
	}
	if !strings.Contains(output, "1:10") {
		t.Error("output should contain ratio")
	}
}

func TestRun_SplitWithDate(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--split", "--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "2:1",
		"--date", "2025-01-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--split with date) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-01-15") {
		t.Error("output should contain the specified date")
	}
}

func TestRun_SplitWithLotTracking(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, true, false)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--split", "--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "4:1",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--split lot-tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Stock split applied successfully") {
		t.Error("output should confirm split for lot-tracking account")
	}
}

func TestRun_SplitInvalidDate(t *testing.T) {
	err := run([]string{
		"--split", "--file", "test.tdb",
		"--ticker", "AAPL",
		"--ratio", "4:1",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

// --- SM-164: CLI --merge-security ---

func TestRun_MergeSecurityMissingFile(t *testing.T) {
	err := run([]string{"--merge-security", "--source", "AAPL", "--target", "GOOG", "--exchange-ratio", "0.5"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_MergeSecurityMissingSource(t *testing.T) {
	err := run([]string{"--merge-security", "--file", "test.tdb", "--target", "GOOG", "--exchange-ratio", "0.5"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --source") {
		t.Errorf("expected --source required error, got: %v", err)
	}
}

func TestRun_MergeSecurityMissingTarget(t *testing.T) {
	err := run([]string{"--merge-security", "--file", "test.tdb", "--source", "AAPL", "--exchange-ratio", "0.5"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --target") {
		t.Errorf("expected --target required error, got: %v", err)
	}
}

func TestRun_MergeSecurityMissingRatio(t *testing.T) {
	err := run([]string{"--merge-security", "--file", "test.tdb", "--source", "AAPL", "--target", "GOOG"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --exchange-ratio") {
		t.Errorf("expected --exchange-ratio required error, got: %v", err)
	}
}

func TestRun_MergeSecurityInvalidRatio(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --exchange-ratio") {
		t.Errorf("expected invalid exchange ratio error, got: %v", err)
	}
}

func TestRun_MergeSecuritySourceNotFound(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "ZZZZ", "--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected source not found error, got: %v", err)
	}
}

func TestRun_MergeSecurityTargetNotFound(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "ZZZZ",
		"--exchange-ratio", "0.5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected target not found error, got: %v", err)
	}
}

func TestRun_MergeSecurityBasic(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--merge-security) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain source ticker")
	}
	if !strings.Contains(output, "GOOG") {
		t.Error("output should contain target ticker")
	}
	if !strings.Contains(output, "0.5") {
		t.Error("output should contain exchange ratio")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestRun_MergeSecurityWithCashPerShare(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--cash-per-share", "10.50",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--merge-security with cash) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger")
	}
	if !strings.Contains(output, "Cash/Share") {
		t.Error("output should show cash per share")
	}
}

func TestRun_MergeSecurityWithDate(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--date", "2025-06-01",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--merge-security with date) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-06-01") {
		t.Error("output should contain the specified date")
	}
}

func TestRun_MergeSecurityWithLotTracking(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, true, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--merge-security", "--file", dbPath,
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--merge-security lot-tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger for lot-tracking account")
	}
}

func TestRun_MergeSecurityInvalidCashPerShare(t *testing.T) {
	err := run([]string{
		"--merge-security", "--file", "test.tdb",
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--cash-per-share", "abc",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --cash-per-share") {
		t.Errorf("expected invalid cash-per-share error, got: %v", err)
	}
}

// --- SM-165: CLI --spin-off ---

func TestRun_SpinOffMissingFile(t *testing.T) {
	err := run([]string{
		"--spin-off", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("expected --file required error, got: %v", err)
	}
}

func TestRun_SpinOffMissingParent(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --parent") {
		t.Errorf("expected --parent required error, got: %v", err)
	}
}

func TestRun_SpinOffMissingSpinoff(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --spinoff") {
		t.Errorf("expected --spinoff required error, got: %v", err)
	}
}

func TestRun_SpinOffMissingShareRatio(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--parent-allocation", "80", "--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --share-ratio") {
		t.Errorf("expected --share-ratio required error, got: %v", err)
	}
}

func TestRun_SpinOffMissingParentAllocation(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --parent-allocation") {
		t.Errorf("expected --parent-allocation required error, got: %v", err)
	}
}

func TestRun_SpinOffMissingPrice(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --spin-off-price") {
		t.Errorf("expected --spin-off-price required error, got: %v", err)
	}
}

func TestRun_SpinOffInvalidShareRatio(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "abc", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --share-ratio") {
		t.Errorf("expected invalid share-ratio error, got: %v", err)
	}
}

func TestRun_SpinOffInvalidParentAllocation(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "xyz",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --parent-allocation") {
		t.Errorf("expected invalid parent-allocation error, got: %v", err)
	}
}

func TestRun_SpinOffInvalidPrice(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb", "--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "abc",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --spin-off-price") {
		t.Errorf("expected invalid spin-off-price error, got: %v", err)
	}
}

func TestRun_SpinOffParentNotFound(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)
	err := run([]string{
		"--spin-off", "--file", dbPath, "--parent", "ZZZZ", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected parent not found error, got: %v", err)
	}
}

func TestRun_SpinOffChildNotFound(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)
	err := run([]string{
		"--spin-off", "--file", dbPath, "--parent", "AAPL", "--spinoff", "ZZZZ",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected spin-off security not found error, got: %v", err)
	}
}

func TestRun_SpinOffBasic(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--spin-off", "--file", dbPath,
		"--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--spin-off) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Spin-off applied successfully") {
		t.Error("output should confirm spin-off")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain parent ticker")
	}
	if !strings.Contains(output, "GOOG") {
		t.Error("output should contain spin-off ticker")
	}
	if !strings.Contains(output, "0.5") {
		t.Error("output should contain share ratio")
	}
	if !strings.Contains(output, "80%") {
		t.Error("output should contain parent allocation")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestRun_SpinOffWithDate(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--spin-off", "--file", dbPath,
		"--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
		"--date", "2025-03-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--spin-off with date) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-03-15") {
		t.Error("output should contain the specified date")
	}
}

func TestRun_SpinOffWithLotTracking(t *testing.T) {
	dbPath := createCorporateActionTestDB(t, true, true)

	stdout := &bytes.Buffer{}
	err := run([]string{
		"--spin-off", "--file", dbPath,
		"--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(--spin-off lot-tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Spin-off applied successfully") {
		t.Error("output should confirm spin-off for lot-tracking account")
	}
}

func TestRun_SpinOffInvalidDate(t *testing.T) {
	err := run([]string{
		"--spin-off", "--file", "test.tdb",
		"--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25", "--date", "bad-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

// --- Parse args tests for corporate actions ---

func TestParseArgs_SplitFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{"--split", "--file", "test.tdb", "--ticker", "AAPL", "--ratio", "4:1", "--date", "2025-01-15"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if !opts.split {
		t.Error("split should be true")
	}
	if opts.secTicker != "AAPL" {
		t.Errorf("ticker should be AAPL, got %q", opts.secTicker)
	}
	if opts.splitRatio != "4:1" {
		t.Errorf("splitRatio should be 4:1, got %q", opts.splitRatio)
	}
	if opts.txDate != "2025-01-15" {
		t.Errorf("date should be 2025-01-15, got %q", opts.txDate)
	}
}

func TestParseArgs_SplitEqualsFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{"--split", "--file", "test.tdb", "--ticker=AAPL", "--ratio=4:1"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.splitRatio != "4:1" {
		t.Errorf("splitRatio should be 4:1, got %q", opts.splitRatio)
	}
}

func TestParseArgs_MergeSecurityFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--merge-security", "--file", "test.tdb",
		"--source", "AAPL", "--target", "GOOG",
		"--exchange-ratio", "0.5", "--cash-per-share", "10.50",
		"--date", "2025-06-01",
	})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if !opts.mergeSecurity {
		t.Error("mergeSecurity should be true")
	}
	if opts.mergeSource != "AAPL" {
		t.Errorf("mergeSource should be AAPL, got %q", opts.mergeSource)
	}
	if opts.mergeTarget != "GOOG" {
		t.Errorf("mergeTarget should be GOOG, got %q", opts.mergeTarget)
	}
	if opts.exchangeRatio != "0.5" {
		t.Errorf("exchangeRatio should be 0.5, got %q", opts.exchangeRatio)
	}
	if opts.cashPerShare != "10.50" {
		t.Errorf("cashPerShare should be 10.50, got %q", opts.cashPerShare)
	}
}

func TestParseArgs_MergeSecurityEqualsFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--merge-security", "--file", "test.tdb",
		"--source=AAPL", "--target=GOOG",
		"--exchange-ratio=0.5", "--cash-per-share=10.50",
	})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.mergeSource != "AAPL" {
		t.Errorf("mergeSource should be AAPL, got %q", opts.mergeSource)
	}
	if opts.mergeTarget != "GOOG" {
		t.Errorf("mergeTarget should be GOOG, got %q", opts.mergeTarget)
	}
	if opts.exchangeRatio != "0.5" {
		t.Errorf("exchangeRatio should be 0.5, got %q", opts.exchangeRatio)
	}
	if opts.cashPerShare != "10.50" {
		t.Errorf("cashPerShare should be 10.50, got %q", opts.cashPerShare)
	}
}

func TestParseArgs_SpinOffFlags(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--spin-off", "--file", "test.tdb",
		"--parent", "AAPL", "--spinoff", "GOOG",
		"--share-ratio", "0.5", "--parent-allocation", "80",
		"--spin-off-price", "25",
	})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if !opts.spinOff {
		t.Error("spinOff should be true")
	}
	if opts.spinOffParent != "AAPL" {
		t.Errorf("spinOffParent should be AAPL, got %q", opts.spinOffParent)
	}
	if opts.spinOffChild != "GOOG" {
		t.Errorf("spinOffChild should be GOOG, got %q", opts.spinOffChild)
	}
	if opts.shareRatio != "0.5" {
		t.Errorf("shareRatio should be 0.5, got %q", opts.shareRatio)
	}
	if opts.parentAllocation != "80" {
		t.Errorf("parentAllocation should be 80, got %q", opts.parentAllocation)
	}
	if opts.spinOffPrice != "25" {
		t.Errorf("spinOffPrice should be 25, got %q", opts.spinOffPrice)
	}
}

func TestParseArgs_SpinOffEqualsFormat(t *testing.T) {
	opts, _, err := parseArgs([]string{
		"--spin-off", "--file", "test.tdb",
		"--parent=AAPL", "--spinoff=GOOG",
		"--share-ratio=0.5", "--parent-allocation=80",
		"--spin-off-price=25",
	})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if opts.spinOffParent != "AAPL" {
		t.Errorf("spinOffParent should be AAPL, got %q", opts.spinOffParent)
	}
	if opts.spinOffChild != "GOOG" {
		t.Errorf("spinOffChild should be GOOG, got %q", opts.spinOffChild)
	}
	if opts.shareRatio != "0.5" {
		t.Errorf("shareRatio should be 0.5, got %q", opts.shareRatio)
	}
	if opts.parentAllocation != "80" {
		t.Errorf("parentAllocation should be 80, got %q", opts.parentAllocation)
	}
	if opts.spinOffPrice != "25" {
		t.Errorf("spinOffPrice should be 25, got %q", opts.spinOffPrice)
	}
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
