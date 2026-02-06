package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func TestParseArgs_FileFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedFile string
	}{
		{"short flag with space", []string{"-f", "/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"long flag with space", []string{"--file", "/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"long flag with equals", []string{"--file=/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"short flag with equals", []string{"-f=/path/to/file.tdb"}, "/path/to/file.tdb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.file != tt.expectedFile {
				t.Errorf("parseArgs(%v) file = %q, want %q", tt.args, opts.file, tt.expectedFile)
			}
		})
	}
}

func TestParseArgs_FileFlagMissingPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"short flag missing path", []string{"-f"}},
		{"long flag missing path", []string{"--file"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error for missing path", tt.args)
			}
		})
	}
}

func TestParseArgs_ListAccountsFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--list-accounts"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.listAccounts {
		t.Error("parseArgs did not set listAccounts flag")
	}
}

func TestParseArgs_IncludeClosedFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--include-closed"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.includeClosed {
		t.Error("parseArgs did not set includeClosed flag")
	}
}

func TestParseArgs_CombinedFlags(t *testing.T) {
	opts, remaining, err := parseArgs([]string{"--file", "test.tdb", "--list-accounts", "--include-closed"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if opts.file != "test.tdb" {
		t.Errorf("file = %q, want %q", opts.file, "test.tdb")
	}
	if !opts.listAccounts {
		t.Error("listAccounts flag not set")
	}
	if !opts.includeClosed {
		t.Error("includeClosed flag not set")
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want empty", remaining)
	}
}

func TestParseArgs_RemainingArgs(t *testing.T) {
	opts, remaining, err := parseArgs([]string{"some-file.tdb", "extra-arg"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 elements", remaining)
	}
	if opts.file != "" {
		t.Errorf("file should be empty for positional args in parseArgs, got %q", opts.file)
	}
}

func TestRun_NoArgs(t *testing.T) {
	// Running with no args launches TUI mode, which requires a TTY.
	// In test environments (no TTY), this will fail with a TTY-related error.
	// This is expected behavior - the TUI cannot run without a terminal.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{}, stdout, stderr)
	// We expect an error when running TUI without a TTY
	if err == nil {
		t.Skip("TUI launched successfully (has TTY), skipping test")
	}
	// The error should be TTY-related
	if !strings.Contains(err.Error(), "TTY") && !strings.Contains(err.Error(), "tty") {
		t.Logf("run([]) returned expected non-TTY error: %v", err)
	}
}

func TestRun_UnknownArgs(t *testing.T) {
	// Running with a file argument launches TUI mode, which requires a TTY.
	// In test environments (no TTY), this will fail with a TTY-related error.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"some-file.tdb"}, stdout, stderr)
	// We expect an error when running TUI without a TTY
	if err == nil {
		t.Skip("TUI launched successfully (has TTY), skipping test")
	}
	// The error should be TTY-related or file-related
	// (acceptable since the file doesn't exist)
	t.Logf("run with file argument returned expected error: %v", err)
}

func TestRun_ListAccountsMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-accounts"}, stdout, stderr)
	if err == nil {
		t.Error("run(--list-accounts) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_ListAccountsFileNotFound(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--list-accounts", "--file", "/nonexistent/path.tdb"}, stdout, stderr)
	if err == nil {
		t.Error("run with nonexistent file should return error")
	}
}

func TestRun_ListAccountsWithValidFile(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Test Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the list-accounts command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "ACCOUNTS") {
		t.Error("output should contain ACCOUNTS header")
	}
	if !strings.Contains(output, "Test Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account type")
	}
	if !strings.Contains(output, "USD") {
		t.Error("output should contain currency")
	}
}

func TestRun_ListAccountsNoAccounts(t *testing.T) {
	// Create a temporary database with no accounts
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Run the list-accounts command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		name     string
		money    models.Money
		currency string
		want     string
	}{
		{"positive USD", models.MustNewMoney("100.50"), "USD", "$100.50"},
		{"negative USD", models.MustNewMoney("-50.25"), "USD", "-$50.25"},
		{"zero USD", models.MustNewMoney("0"), "USD", "$0.00"},
		{"positive EUR", models.MustNewMoney("100.50"), "EUR", "€100.50"},
		{"negative EUR", models.MustNewMoney("-50.25"), "EUR", "-€50.25"},
		{"positive GBP", models.MustNewMoney("100.50"), "GBP", "£100.50"},
		{"other currency", models.MustNewMoney("100.50"), "JPY", "JPY 100.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMoney(tt.money, tt.currency)
			if got != tt.want {
				t.Errorf("formatMoney(%v, %q) = %q, want %q", tt.money, tt.currency, got, tt.want)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	buf := &bytes.Buffer{}
	printVersion(buf)
	output := buf.String()

	if !strings.Contains(output, "tmoney version") {
		t.Error("version output should contain 'tmoney version'")
	}
	if !strings.Contains(output, "Build time") {
		t.Error("version output should contain 'Build time'")
	}
	if !strings.Contains(output, "Git commit") {
		t.Error("version output should contain 'Git commit'")
	}
}

func TestPrintHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "TMoney") {
		t.Error("help output should contain 'TMoney'")
	}
	if !strings.Contains(output, "--file") {
		t.Error("help output should document --file flag")
	}
	if !strings.Contains(output, "--list-accounts") {
		t.Error("help output should document --list-accounts flag")
	}
	if !strings.Contains(output, "--include-closed") {
		t.Error("help output should document --include-closed flag")
	}
}

func TestParseArgs_CreateFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedCreate string
	}{
		{"long flag with space", []string{"--create", "/path/to/new.tdb"}, "/path/to/new.tdb"},
		{"long flag with equals", []string{"--create=/path/to/new.tdb"}, "/path/to/new.tdb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.createDB != tt.expectedCreate {
				t.Errorf("parseArgs(%v) createDB = %q, want %q", tt.args, opts.createDB, tt.expectedCreate)
			}
		})
	}
}

func TestParseArgs_CreateFlagMissingPath(t *testing.T) {
	_, _, err := parseArgs([]string{"--create"})
	if err == nil {
		t.Error("parseArgs(--create) without path should return error")
	}
	if !strings.Contains(err.Error(), "requires a path") {
		t.Errorf("error should mention path requirement, got: %v", err)
	}
}

func TestRun_CreateDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb.tdb")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Created database:") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, dbPath) {
		t.Errorf("output should contain path, got: %s", output)
	}

	// Verify the file was created and can be opened
	database, err := db.Open(dbPath)
	if err != nil {
		t.Errorf("failed to open created database: %v", err)
		return
	}
	database.Close()
}

func TestRun_CreateDBWithEqualsFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb.tdb")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create=" + dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create=path) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Created database:") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
}

func TestRun_CreateDBAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "existing.tdb")

	// Create the database first
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create initial database: %v", err)
	}
	database.Close()

	// Try to create again
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--create", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--create) on existing file should return error")
	}
}

func TestRun_CreateDBAddsExtension(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newdb") // No .tdb extension

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--create) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, ".tdb") {
		t.Errorf("output should show .tdb extension was added, got: %s", output)
	}

	// Verify the file with .tdb extension was created
	database, err := db.Open(dbPath + ".tdb")
	if err != nil {
		t.Errorf("failed to open created database with .tdb extension: %v", err)
		return
	}
	database.Close()
}

func TestRun_CreateThenListAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	// Create the database
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--create", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--create) returned error: %v", err)
	}

	// List accounts (should be empty but work)
	stdout.Reset()
	stderr.Reset()
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) after create returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestPrintHelp_IncludesCreate(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--create") {
		t.Error("help output should document --create flag")
	}
	if !strings.Contains(output, "Create a new database file") {
		t.Error("help output should describe --create functionality")
	}
}

// TestRun_ListAccountsWithClosedAccount tests that --include-closed shows closed accounts
// Tests for --account flag parsing
func TestParseArgs_AccountFlag(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedAccount string
	}{
		{"long flag with space", []string{"--account", "My Checking"}, "My Checking"},
		{"long flag with equals", []string{"--account=My Checking"}, "My Checking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.accountName != tt.expectedAccount {
				t.Errorf("parseArgs(%v) accountName = %q, want %q", tt.args, opts.accountName, tt.expectedAccount)
			}
		})
	}
}

func TestParseArgs_AccountFlagMissingName(t *testing.T) {
	_, _, err := parseArgs([]string{"--account"})
	if err == nil {
		t.Error("parseArgs(--account) without name should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_BalanceFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--balance"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.showBalance {
		t.Error("parseArgs did not set showBalance flag")
	}
}

func TestRun_AccountMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--account", "Test Account"}, stdout, stderr)
	if err == nil {
		t.Error("run(--account) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AccountNotFound(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--account", "Nonexistent Account", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--account) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AccountWithValidAccount(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account with optional fields
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Test Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	account.SetInstitution("Chase Bank")
	account.SetAccountNumber("1234567890")
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the account command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--account", "Test Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "ACCOUNT: Test Checking") {
		t.Error("output should contain account header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account type")
	}
	if !strings.Contains(output, "USD") {
		t.Error("output should contain currency")
	}
	if !strings.Contains(output, "Chase Bank") {
		t.Error("output should contain institution")
	}
	if !strings.Contains(output, "****7890") {
		t.Error("output should contain masked account number")
	}
	if !strings.Contains(output, "Current Balance") {
		t.Error("output should contain current balance")
	}
	if !strings.Contains(output, "Active") {
		t.Error("output should show active status")
	}
}

func TestRun_BalanceMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--balance"}, stdout, stderr)
	if err == nil {
		t.Error("run(--balance) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_BalanceNoAccounts(t *testing.T) {
	// Create a temporary database with no accounts
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	// Run the balance command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--balance", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--balance) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No accounts found") {
		t.Errorf("output should say no accounts found, got: %s", output)
	}
}

func TestRun_BalanceWithAccounts(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create test accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("5000.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	creditCard := models.NewAccount(
		"Visa",
		models.AccountTypeCreditCard,
		"USD",
		models.MustNewMoney("-500.00"),
		models.Today(),
	)
	if err := repo.Create(creditCard); err != nil {
		t.Fatalf("failed to create credit card account: %v", err)
	}

	database.Close()

	// Run the balance command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--balance", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--balance) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "BALANCES") {
		t.Error("output should contain BALANCES header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
	if !strings.Contains(output, "Savings") {
		t.Error("output should contain Savings account")
	}
	if !strings.Contains(output, "Visa") {
		t.Error("output should contain Visa account")
	}
	if !strings.Contains(output, "Net Worth") {
		t.Error("output should contain Net Worth")
	}
}

func TestPrintHelp_IncludesAccountAndBalance(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--account") {
		t.Error("help output should document --account flag")
	}
	if !strings.Contains(output, "--balance") {
		t.Error("help output should document --balance flag")
	}
}

func TestRun_ListAccountsWithClosedAccount(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create an active and a closed account
	repo := repository.NewAccountRepository(database)

	activeAccount := models.NewAccount(
		"Active Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(activeAccount); err != nil {
		t.Fatalf("failed to create active account: %v", err)
	}

	closedAccount := models.NewAccount(
		"Closed Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("0"),
		models.Today(),
	)
	closedAccount.Close()
	if err := repo.Create(closedAccount); err != nil {
		t.Fatalf("failed to create closed account: %v", err)
	}

	database.Close()

	// Test without --include-closed (should only show active)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--list-accounts", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Active Checking") {
		t.Error("output should contain active account")
	}
	if strings.Contains(output, "Closed Savings") {
		t.Error("output should NOT contain closed account without --include-closed")
	}

	// Test with --include-closed (should show both)
	stdout.Reset()
	stderr.Reset()
	err = run([]string{"--list-accounts", "--file", dbPath, "--include-closed"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--list-accounts --include-closed) returned error: %v", err)
		return
	}

	output = stdout.String()
	if !strings.Contains(output, "Active Checking") {
		t.Error("output should contain active account")
	}
	if !strings.Contains(output, "Closed Savings") {
		t.Error("output should contain closed account with --include-closed")
	}
}

// Tests for --transactions flag parsing
func TestParseArgs_TransactionsFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--transactions"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.transactions {
		t.Error("parseArgs did not set transactions flag")
	}
}

func TestParseArgs_LimitFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLimit int
	}{
		{"long flag with space", []string{"--limit", "10"}, 10},
		{"long flag with equals", []string{"--limit=25"}, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.limit != tt.expectedLimit {
				t.Errorf("parseArgs(%v) limit = %d, want %d", tt.args, opts.limit, tt.expectedLimit)
			}
		})
	}
}

func TestParseArgs_LimitFlagInvalid(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing value", []string{"--limit"}},
		{"not a number", []string{"--limit", "abc"}},
		{"zero", []string{"--limit", "0"}},
		{"negative", []string{"--limit", "-5"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) should return error", tt.args)
			}
		})
	}
}

func TestParseArgs_FromToFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedFrom string
		expectedTo   string
	}{
		{"from with space", []string{"--from", "2024-01-01"}, "2024-01-01", ""},
		{"to with space", []string{"--to", "2024-12-31"}, "", "2024-12-31"},
		{"from with equals", []string{"--from=2024-01-01"}, "2024-01-01", ""},
		{"to with equals", []string{"--to=2024-12-31"}, "", "2024-12-31"},
		{"both flags", []string{"--from", "2024-01-01", "--to", "2024-12-31"}, "2024-01-01", "2024-12-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.fromDate != tt.expectedFrom {
				t.Errorf("parseArgs(%v) fromDate = %q, want %q", tt.args, opts.fromDate, tt.expectedFrom)
			}
			if opts.toDate != tt.expectedTo {
				t.Errorf("parseArgs(%v) toDate = %q, want %q", tt.args, opts.toDate, tt.expectedTo)
			}
		})
	}
}

func TestParseArgs_FromToMissingValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"from missing value", []string{"--from"}},
		{"to missing value", []string{"--to"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) should return error", tt.args)
			}
		})
	}
}

func TestRun_TransactionsMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--transactions", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_TransactionsMissingAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_TransactionsAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Nonexistent", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_TransactionsNoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	database.Close()

	// Run the transactions command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "TRANSACTIONS: Checking") {
		t.Error("output should contain TRANSACTIONS header")
	}
	if !strings.Contains(output, "No transactions found") {
		t.Errorf("output should say no transactions found, got: %s", output)
	}
}

func TestRun_TransactionsWithTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create transactions
	txnRepo := repository.NewTransactionRepository(database)

	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-5.50"))
	txn1.SetPayee(payee.ID)
	txn1.SetCategory(category.ID)
	txn1.Clear()
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-25.00"))
	txn2.SetPayee(payee.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	database.Close()

	// Run the transactions command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "TRANSACTIONS: Checking") {
		t.Error("output should contain TRANSACTIONS header")
	}
	if !strings.Contains(output, "Coffee Shop") {
		t.Error("output should contain payee name")
	}
	if !strings.Contains(output, "Food") {
		t.Error("output should contain category name")
	}
	if !strings.Contains(output, "-$5.50") {
		t.Error("output should contain transaction amount")
	}
	if !strings.Contains(output, "Cleared") {
		t.Error("output should contain transaction status")
	}
	if !strings.Contains(output, "Showing 2 transaction(s)") {
		t.Errorf("output should show transaction count, got: %s", output)
	}
}

func TestRun_TransactionsWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create multiple transactions
	txnRepo := repository.NewTransactionRepository(database)
	for i := 0; i < 5; i++ {
		txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-10.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction %d: %v", i, err)
		}
	}

	database.Close()

	// Run with --limit 2
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath, "--limit", "2"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions --limit) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 2 transaction(s)") {
		t.Errorf("output should show 2 transactions with limit, got: %s", output)
	}
}

func TestRun_TransactionsWithDateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transactions with different dates
	txnRepo := repository.NewTransactionRepository(database)

	jan15, _ := models.ParseDate("2024-01-15")
	txn1 := models.NewTransaction(account.ID, jan15, models.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	feb15, _ := models.ParseDate("2024-02-15")
	txn2 := models.NewTransaction(account.ID, feb15, models.MustNewMoney("-20.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	mar15, _ := models.ParseDate("2024-03-15")
	txn3 := models.NewTransaction(account.ID, mar15, models.MustNewMoney("-30.00"))
	if err := txnRepo.Create(txn3); err != nil {
		t.Fatalf("failed to create transaction 3: %v", err)
	}

	database.Close()

	// Run with date filter for February only
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transactions", "--account", "Checking", "--file", dbPath,
		"--from", "2024-02-01", "--to", "2024-02-28",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transactions with date filter) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 1 transaction(s)") {
		t.Errorf("output should show 1 transaction in date range, got: %s", output)
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Errorf("output should show February transaction, got: %s", output)
	}
}

func TestRun_TransactionsInvalidDateFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transactions", "--account", "Checking", "--file", dbPath, "--from", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transactions) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestPrintHelp_IncludesTransactions(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--transactions") {
		t.Error("help output should document --transactions flag")
	}
	if !strings.Contains(output, "--limit") {
		t.Error("help output should document --limit flag")
	}
	if !strings.Contains(output, "--from") {
		t.Error("help output should document --from flag")
	}
	if !strings.Contains(output, "--to") {
		t.Error("help output should document --to flag")
	}
}

// Tests for --add-transaction command
func TestParseArgs_AddTransactionFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--add-transaction"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.addTransaction {
		t.Error("parseArgs did not set addTransaction flag")
	}
}

func TestParseArgs_AmountFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedAmount string
	}{
		{"long flag with space", []string{"--amount", "-50.00"}, "-50.00"},
		{"long flag with equals", []string{"--amount=-50.00"}, "-50.00"},
		{"positive amount", []string{"--amount", "100.50"}, "100.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txAmount != tt.expectedAmount {
				t.Errorf("parseArgs(%v) txAmount = %q, want %q", tt.args, opts.txAmount, tt.expectedAmount)
			}
		})
	}
}

func TestParseArgs_AmountFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--amount"})
	if err == nil {
		t.Error("parseArgs(--amount) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_PayeeFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedPayee string
	}{
		{"long flag with space", []string{"--payee", "Coffee Shop"}, "Coffee Shop"},
		{"long flag with equals", []string{"--payee=Coffee Shop"}, "Coffee Shop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txPayee != tt.expectedPayee {
				t.Errorf("parseArgs(%v) txPayee = %q, want %q", tt.args, opts.txPayee, tt.expectedPayee)
			}
		})
	}
}

func TestParseArgs_PayeeFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--payee"})
	if err == nil {
		t.Error("parseArgs(--payee) without value should return error")
	}
}

func TestParseArgs_CategoryFlag(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedCategory string
	}{
		{"long flag with space", []string{"--category", "Food"}, "Food"},
		{"long flag with equals", []string{"--category=Food"}, "Food"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txCategory != tt.expectedCategory {
				t.Errorf("parseArgs(%v) txCategory = %q, want %q", tt.args, opts.txCategory, tt.expectedCategory)
			}
		})
	}
}

func TestParseArgs_CategoryFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--category"})
	if err == nil {
		t.Error("parseArgs(--category) without value should return error")
	}
}

func TestParseArgs_DateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedDate string
	}{
		{"long flag with space", []string{"--date", "2024-01-15"}, "2024-01-15"},
		{"long flag with equals", []string{"--date=2024-01-15"}, "2024-01-15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txDate != tt.expectedDate {
				t.Errorf("parseArgs(%v) txDate = %q, want %q", tt.args, opts.txDate, tt.expectedDate)
			}
		})
	}
}

func TestParseArgs_DateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--date"})
	if err == nil {
		t.Error("parseArgs(--date) without value should return error")
	}
}

func TestParseArgs_MemoFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedMemo string
	}{
		{"long flag with space", []string{"--memo", "Morning coffee"}, "Morning coffee"},
		{"long flag with equals", []string{"--memo=Morning coffee"}, "Morning coffee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txMemo != tt.expectedMemo {
				t.Errorf("parseArgs(%v) txMemo = %q, want %q", tt.args, opts.txMemo, tt.expectedMemo)
			}
		})
	}
}

func TestParseArgs_MemoFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--memo"})
	if err == nil {
		t.Error("parseArgs(--memo) without value should return error")
	}
}

func TestRun_AddTransactionMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-transaction", "--account", "Checking", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddTransactionMissingAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --account should return error")
	}
	if !strings.Contains(err.Error(), "requires --account") {
		t.Errorf("error should mention --account requirement, got: %v", err)
	}
}

func TestRun_AddTransactionMissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("error should mention --amount requirement, got: %v", err)
	}
}

func TestRun_AddTransactionInvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "invalid"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("error should mention invalid amount, got: %v", err)
	}
}

func TestRun_AddTransactionInvalidDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_AddTransactionAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Nonexistent", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AddTransactionCategoryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-transaction", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--category", "Nonexistent"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-transaction) with nonexistent category should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestRun_AddTransactionBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-50.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transaction created successfully") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Errorf("output should show account name, got: %s", output)
	}
	if !strings.Contains(output, "-$50.00") {
		t.Errorf("output should show amount, got: %s", output)
	}

	// Verify transaction was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Amount.String() != "-50" {
		t.Errorf("transaction amount = %s, want -50", transactions[0].Amount.String())
	}
}

func TestRun_AddTransactionWithPayeeAutoCreate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command with payee
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-5.50",
		"--payee", "Coffee Shop",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Coffee Shop") {
		t.Errorf("output should show payee name, got: %s", output)
	}
	if !strings.Contains(output, "(new)") {
		t.Errorf("output should indicate new payee was created, got: %s", output)
	}

	// Verify payee was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	payeeRepo := repository.NewPayeeRepository(database)
	payee, err := payeeRepo.GetByName("Coffee Shop")
	if err != nil {
		t.Errorf("payee should have been created: %v", err)
	}
	if payee.Name != "Coffee Shop" {
		t.Errorf("payee name = %q, want %q", payee.Name, "Coffee Shop")
	}
}

func TestRun_AddTransactionWithExistingPayee(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create existing payee
	payeeRepo := repository.NewPayeeRepository(database)
	existingPayee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(existingPayee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}
	database.Close()

	// Run the add-transaction command with existing payee
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-5.50",
		"--payee", "Coffee Shop",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Coffee Shop") {
		t.Errorf("output should show payee name, got: %s", output)
	}
	// Should NOT say (new) for existing payee
	if strings.Contains(output, "(new)") {
		t.Errorf("output should NOT indicate new payee for existing payee, got: %s", output)
	}
}

func TestRun_AddTransactionWithCategory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Food", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	database.Close()

	// Run the add-transaction command with category
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-25.00",
		"--category", "Food",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Food") {
		t.Errorf("output should show category name, got: %s", output)
	}

	// Verify transaction has category
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if !transactions[0].CategoryID.Valid {
		t.Error("transaction should have category set")
	}
}

func TestRun_AddTransactionWithDateAndMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	// Run the add-transaction command with date and memo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-15.00",
		"--date", "2024-01-15",
		"--memo", "Lunch with friend",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "2024-01-15") {
		t.Errorf("output should show date, got: %s", output)
	}
	if !strings.Contains(output, "Lunch with friend") {
		t.Errorf("output should show memo, got: %s", output)
	}

	// Verify transaction
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)
	transactions, err := txnRepo.ListByAccount(account.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Date.String() != "2024-01-15" {
		t.Errorf("transaction date = %s, want 2024-01-15", transactions[0].Date.String())
	}
	if !transactions[0].Memo.Valid || transactions[0].Memo.String != "Lunch with friend" {
		t.Errorf("transaction memo = %v, want 'Lunch with friend'", transactions[0].Memo)
	}
}

func TestRun_AddTransactionFullExample(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a category
	catRepo := repository.NewCategoryRepository(database)
	category := models.NewCategory("Dining", models.CategoryTypeExpense)
	if err := catRepo.Create(category); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	database.Close()

	// Run with all options
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--add-transaction",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-45.50",
		"--payee", "Olive Garden",
		"--category", "Dining",
		"--date", "2024-06-15",
		"--memo", "Birthday dinner",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-transaction) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transaction created successfully") {
		t.Error("output should confirm creation")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should show account")
	}
	if !strings.Contains(output, "-$45.50") {
		t.Error("output should show amount")
	}
	if !strings.Contains(output, "Olive Garden") {
		t.Error("output should show payee")
	}
	if !strings.Contains(output, "Dining") {
		t.Error("output should show category")
	}
	if !strings.Contains(output, "2024-06-15") {
		t.Error("output should show date")
	}
	if !strings.Contains(output, "Birthday dinner") {
		t.Error("output should show memo")
	}
}

func TestPrintHelp_IncludesAddTransaction(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--add-transaction") {
		t.Error("help output should document --add-transaction flag")
	}
	if !strings.Contains(output, "--amount") {
		t.Error("help output should document --amount flag")
	}
	if !strings.Contains(output, "--payee") {
		t.Error("help output should document --payee flag")
	}
	if !strings.Contains(output, "--category") {
		t.Error("help output should document --category flag")
	}
	if !strings.Contains(output, "--date") {
		t.Error("help output should document --date flag")
	}
	if !strings.Contains(output, "--memo") {
		t.Error("help output should document --memo flag")
	}
}

// Tests for --add-account command
func TestParseArgs_AddAccountFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--add-account"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.addAccount {
		t.Error("parseArgs did not set addAccount flag")
	}
}

func TestParseArgs_NameFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedName string
	}{
		{"long flag with space", []string{"--name", "My Checking"}, "My Checking"},
		{"long flag with equals", []string{"--name=My Checking"}, "My Checking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctName != tt.expectedName {
				t.Errorf("parseArgs(%v) acctName = %q, want %q", tt.args, opts.acctName, tt.expectedName)
			}
		})
	}
}

func TestParseArgs_NameFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--name"})
	if err == nil {
		t.Error("parseArgs(--name) without value should return error")
	}
}

func TestParseArgs_TypeFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedType string
	}{
		{"long flag with space", []string{"--type", "checking"}, "checking"},
		{"long flag with equals", []string{"--type=savings"}, "savings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctType != tt.expectedType {
				t.Errorf("parseArgs(%v) acctType = %q, want %q", tt.args, opts.acctType, tt.expectedType)
			}
		})
	}
}

func TestParseArgs_TypeFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--type"})
	if err == nil {
		t.Error("parseArgs(--type) without value should return error")
	}
}

func TestParseArgs_CurrencyFlag(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedCurrency string
	}{
		{"long flag with space", []string{"--currency", "EUR"}, "EUR"},
		{"long flag with equals", []string{"--currency=GBP"}, "GBP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctCurrency != tt.expectedCurrency {
				t.Errorf("parseArgs(%v) acctCurrency = %q, want %q", tt.args, opts.acctCurrency, tt.expectedCurrency)
			}
		})
	}
}

func TestParseArgs_CurrencyFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--currency"})
	if err == nil {
		t.Error("parseArgs(--currency) without value should return error")
	}
}

func TestParseArgs_OpeningBalanceFlag(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedBalance string
	}{
		{"long flag with space", []string{"--opening-balance", "1000.00"}, "1000.00"},
		{"long flag with equals", []string{"--opening-balance=500.50"}, "500.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctOpeningBal != tt.expectedBalance {
				t.Errorf("parseArgs(%v) acctOpeningBal = %q, want %q", tt.args, opts.acctOpeningBal, tt.expectedBalance)
			}
		})
	}
}

func TestParseArgs_OpeningBalanceFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--opening-balance"})
	if err == nil {
		t.Error("parseArgs(--opening-balance) without value should return error")
	}
}

func TestParseArgs_OpeningDateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedDate string
	}{
		{"long flag with space", []string{"--opening-date", "2024-01-15"}, "2024-01-15"},
		{"long flag with equals", []string{"--opening-date=2024-06-01"}, "2024-06-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctOpeningDate != tt.expectedDate {
				t.Errorf("parseArgs(%v) acctOpeningDate = %q, want %q", tt.args, opts.acctOpeningDate, tt.expectedDate)
			}
		})
	}
}

func TestParseArgs_OpeningDateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--opening-date"})
	if err == nil {
		t.Error("parseArgs(--opening-date) without value should return error")
	}
}

func TestParseArgs_InstitutionFlag(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		expectedInstitution string
	}{
		{"long flag with space", []string{"--institution", "Chase Bank"}, "Chase Bank"},
		{"long flag with equals", []string{"--institution=Wells Fargo"}, "Wells Fargo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctInstitution != tt.expectedInstitution {
				t.Errorf("parseArgs(%v) acctInstitution = %q, want %q", tt.args, opts.acctInstitution, tt.expectedInstitution)
			}
		})
	}
}

func TestParseArgs_InstitutionFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--institution"})
	if err == nil {
		t.Error("parseArgs(--institution) without value should return error")
	}
}

func TestParseArgs_AccountNumberFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedNumber string
	}{
		{"long flag with space", []string{"--account-number", "1234567890"}, "1234567890"},
		{"long flag with equals", []string{"--account-number=9876543210"}, "9876543210"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctNumber != tt.expectedNumber {
				t.Errorf("parseArgs(%v) acctNumber = %q, want %q", tt.args, opts.acctNumber, tt.expectedNumber)
			}
		})
	}
}

func TestParseArgs_AccountNumberFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--account-number"})
	if err == nil {
		t.Error("parseArgs(--account-number) without value should return error")
	}
}

func TestParseArgs_NotesFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedNotes string
	}{
		{"long flag with space", []string{"--notes", "Primary checking account"}, "Primary checking account"},
		{"long flag with equals", []string{"--notes=For emergencies only"}, "For emergencies only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctNotes != tt.expectedNotes {
				t.Errorf("parseArgs(%v) acctNotes = %q, want %q", tt.args, opts.acctNotes, tt.expectedNotes)
			}
		})
	}
}

func TestParseArgs_NotesFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--notes"})
	if err == nil {
		t.Error("parseArgs(--notes) without value should return error")
	}
}

func TestParseArgs_CreditLimitFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLimit string
	}{
		{"long flag with space", []string{"--credit-limit", "5000.00"}, "5000.00"},
		{"long flag with equals", []string{"--credit-limit=10000"}, "10000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctCreditLimit != tt.expectedLimit {
				t.Errorf("parseArgs(%v) acctCreditLimit = %q, want %q", tt.args, opts.acctCreditLimit, tt.expectedLimit)
			}
		})
	}
}

func TestParseArgs_CreditLimitFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--credit-limit"})
	if err == nil {
		t.Error("parseArgs(--credit-limit) without value should return error")
	}
}

func TestParseArgs_InterestRateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedRate string
	}{
		{"long flag with space", []string{"--interest-rate", "5.5"}, "5.5"},
		{"long flag with equals", []string{"--interest-rate=7.25"}, "7.25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctInterestRate != tt.expectedRate {
				t.Errorf("parseArgs(%v) acctInterestRate = %q, want %q", tt.args, opts.acctInterestRate, tt.expectedRate)
			}
		})
	}
}

func TestParseArgs_InterestRateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--interest-rate"})
	if err == nil {
		t.Error("parseArgs(--interest-rate) without value should return error")
	}
}

func TestRun_AddAccountMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--add-account", "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_AddAccountMissingName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --name should return error")
	}
	if !strings.Contains(err.Error(), "requires --name") {
		t.Errorf("error should mention --name requirement, got: %v", err)
	}
}

func TestRun_AddAccountMissingType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) without --type should return error")
	}
	if !strings.Contains(err.Error(), "requires --type") {
		t.Errorf("error should mention --type requirement, got: %v", err)
	}
}

func TestRun_AddAccountInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "invalid_type"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error should mention invalid type, got: %v", err)
	}
}

func TestRun_AddAccountInvalidOpeningBalance(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-balance", "invalid"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid opening-balance should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-balance") {
		t.Errorf("error should mention invalid opening-balance, got: %v", err)
	}
}

func TestRun_AddAccountInvalidOpeningDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with invalid opening-date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-date") {
		t.Errorf("error should mention invalid opening-date, got: %v", err)
	}
}

func TestRun_AddAccountDuplicateName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create existing account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("0"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with duplicate name should return error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists, got: %v", err)
	}
}

func TestRun_AddAccountCreditLimitOnNonCreditCard(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--credit-limit", "5000"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with credit-limit on non-credit_card should return error")
	}
	if !strings.Contains(err.Error(), "only valid for credit_card") {
		t.Errorf("error should mention credit_card requirement, got: %v", err)
	}
}

func TestRun_AddAccountInterestRateOnNonLoan(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--add-account", "--file", dbPath, "--name", "Checking", "--type", "checking", "--interest-rate", "5.5"}, stdout, stderr)
	if err == nil {
		t.Error("run(--add-account) with interest-rate on non-loan should return error")
	}
	if !strings.Contains(err.Error(), "only valid for loan") {
		t.Errorf("error should mention loan requirement, got: %v", err)
	}
}

func TestRun_AddAccountBasic(t *testing.T) {
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
		"--add-account",
		"--file", dbPath,
		"--name", "My Checking",
		"--type", "checking",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Account created successfully") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, "My Checking") {
		t.Errorf("output should show account name, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Errorf("output should show account type, got: %s", output)
	}
	if !strings.Contains(output, "USD") {
		t.Errorf("output should show default currency, got: %s", output)
	}
	if !strings.Contains(output, "$0.00") {
		t.Errorf("output should show default opening balance, got: %s", output)
	}

	// Verify account was created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("My Checking")
	if err != nil {
		t.Errorf("account should have been created: %v", err)
		return
	}
	if account.Name != "My Checking" {
		t.Errorf("account name = %q, want %q", account.Name, "My Checking")
	}
	if account.Type != models.AccountTypeChecking {
		t.Errorf("account type = %v, want %v", account.Type, models.AccountTypeChecking)
	}
}

func TestRun_AddAccountWithAllOptions(t *testing.T) {
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
		"--add-account",
		"--file", dbPath,
		"--name", "Primary Checking",
		"--type", "checking",
		"--currency", "EUR",
		"--opening-balance", "1000.50",
		"--opening-date", "2024-01-15",
		"--institution", "Chase Bank",
		"--account-number", "1234567890",
		"--notes", "Primary account",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Account created successfully") {
		t.Error("output should confirm creation")
	}
	if !strings.Contains(output, "Primary Checking") {
		t.Error("output should show account name")
	}
	if !strings.Contains(output, "EUR") {
		t.Error("output should show currency")
	}
	if !strings.Contains(output, "1000.50") || !strings.Contains(output, "€") {
		t.Errorf("output should show opening balance with EUR symbol, got: %s", output)
	}
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should show opening date")
	}
	if !strings.Contains(output, "Chase Bank") {
		t.Error("output should show institution")
	}
	if !strings.Contains(output, "1234567890") {
		t.Error("output should show account number")
	}
	if !strings.Contains(output, "Primary account") {
		t.Error("output should show notes")
	}

	// Verify account fields
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Primary Checking")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if account.Currency != "EUR" {
		t.Errorf("account currency = %q, want %q", account.Currency, "EUR")
	}
	if account.OpeningDate.String() != "2024-01-15" {
		t.Errorf("account opening date = %s, want 2024-01-15", account.OpeningDate.String())
	}
	if !account.Institution.Valid || account.Institution.String != "Chase Bank" {
		t.Errorf("account institution = %v, want 'Chase Bank'", account.Institution)
	}
	if !account.AccountNumber.Valid || account.AccountNumber.String != "1234567890" {
		t.Errorf("account number = %v, want '1234567890'", account.AccountNumber)
	}
	if !account.Notes.Valid || account.Notes.String != "Primary account" {
		t.Errorf("account notes = %v, want 'Primary account'", account.Notes)
	}
}

func TestRun_AddAccountCreditCard(t *testing.T) {
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
		"--add-account",
		"--file", dbPath,
		"--name", "Visa Card",
		"--type", "credit_card",
		"--credit-limit", "5000.00",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Credit Limit") {
		t.Errorf("output should show credit limit, got: %s", output)
	}
	if !strings.Contains(output, "$5000.00") {
		t.Errorf("output should show credit limit value, got: %s", output)
	}

	// Verify credit limit
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Visa Card")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if !account.CreditLimit.Valid {
		t.Error("credit limit should be set")
	}
	if account.CreditLimit.Money.String() != "5000" {
		t.Errorf("credit limit = %s, want 5000", account.CreditLimit.Money.String())
	}
}

func TestRun_AddAccountLoan(t *testing.T) {
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
		"--add-account",
		"--file", dbPath,
		"--name", "Car Loan",
		"--type", "loan",
		"--opening-balance", "-15000.00",
		"--interest-rate", "5.5",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--add-account) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Interest Rate") {
		t.Errorf("output should show interest rate, got: %s", output)
	}
	if !strings.Contains(output, "5.5%") {
		t.Errorf("output should show interest rate value, got: %s", output)
	}

	// Verify interest rate
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	acctRepo := repository.NewAccountRepository(database)
	account, err := acctRepo.GetByName("Car Loan")
	if err != nil {
		t.Fatalf("account should have been created: %v", err)
	}
	if !account.InterestRate.Valid {
		t.Error("interest rate should be set")
	}
	if account.InterestRate.Money.String() != "5.5" {
		t.Errorf("interest rate = %s, want 5.5", account.InterestRate.Money.String())
	}
}

func TestRun_AddAccountAllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	accountTypes := []string{"checking", "savings", "credit_card", "investment", "cash", "loan", "asset"}

	for _, acctType := range accountTypes {
		t.Run(acctType, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			err = run([]string{
				"--add-account",
				"--file", dbPath,
				"--name", "Test " + acctType,
				"--type", acctType,
			}, stdout, stderr)
			if err != nil {
				t.Errorf("run(--add-account --type %s) returned error: %v", acctType, err)
				return
			}

			output := stdout.String()
			if !strings.Contains(output, "Account created successfully") {
				t.Errorf("output should confirm creation for type %s, got: %s", acctType, output)
			}
		})
	}
}

func TestPrintHelp_IncludesAddAccount(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--add-account") {
		t.Error("help output should document --add-account flag")
	}
	if !strings.Contains(output, "--name") {
		t.Error("help output should document --name flag")
	}
	if !strings.Contains(output, "--type") {
		t.Error("help output should document --type flag")
	}
	if !strings.Contains(output, "--currency") {
		t.Error("help output should document --currency flag")
	}
	if !strings.Contains(output, "--opening-balance") {
		t.Error("help output should document --opening-balance flag")
	}
	if !strings.Contains(output, "--opening-date") {
		t.Error("help output should document --opening-date flag")
	}
	if !strings.Contains(output, "--institution") {
		t.Error("help output should document --institution flag")
	}
	if !strings.Contains(output, "--account-number") {
		t.Error("help output should document --account-number flag")
	}
	if !strings.Contains(output, "--notes") {
		t.Error("help output should document --notes flag")
	}
	if !strings.Contains(output, "--credit-limit") {
		t.Error("help output should document --credit-limit flag")
	}
	if !strings.Contains(output, "--interest-rate") {
		t.Error("help output should document --interest-rate flag")
	}
}

// =============================================================================
// Transfer CLI Tests
// =============================================================================

func TestParseArgs_TransferFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--transfer"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.transfer {
		t.Error("parseArgs did not set transfer flag")
	}
}

func TestParseArgs_TransferWithFromTo(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantFrom    string
		wantTo      string
		wantTransfer bool
	}{
		{
			name:        "transfer with from and to",
			args:        []string{"--transfer", "--from", "Checking", "--to", "Savings"},
			wantFrom:    "Checking",
			wantTo:      "Savings",
			wantTransfer: true,
		},
		{
			name:        "transfer with equals syntax",
			args:        []string{"--transfer", "--from=Checking", "--to=Savings"},
			wantFrom:    "Checking",
			wantTo:      "Savings",
			wantTransfer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.transfer != tt.wantTransfer {
				t.Errorf("transfer = %v, want %v", opts.transfer, tt.wantTransfer)
			}
			if opts.fromAccount != tt.wantFrom {
				t.Errorf("fromAccount = %q, want %q", opts.fromAccount, tt.wantFrom)
			}
			if opts.toAccount != tt.wantTo {
				t.Errorf("toAccount = %q, want %q", opts.toAccount, tt.wantTo)
			}
		})
	}
}

func TestRun_TransferMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--transfer", "--from", "Checking", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_TransferMissingFrom(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --from should return error")
	}
	if !strings.Contains(err.Error(), "requires --from") {
		t.Errorf("error should mention --from requirement, got: %v", err)
	}
}

func TestRun_TransferMissingTo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --to should return error")
	}
	if !strings.Contains(err.Error(), "requires --to") {
		t.Errorf("error should mention --to requirement, got: %v", err)
	}
}

func TestRun_TransferMissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "requires --amount") {
		t.Errorf("error should mention --amount requirement, got: %v", err)
	}
}

func TestRun_TransferInvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "not-a-number"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("error should mention invalid amount, got: %v", err)
	}
}

func TestRun_TransferNegativeAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "-100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with negative amount should return error")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("error should mention positive amount, got: %v", err)
	}
}

func TestRun_TransferSourceAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Nonexistent", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with nonexistent source account should return error")
	}
	if !strings.Contains(err.Error(), "source account") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention source account not found, got: %v", err)
	}
}

func TestRun_TransferDestAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source account
	repo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Nonexistent", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with nonexistent destination account should return error")
	}
	if !strings.Contains(err.Error(), "destination account") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention destination account not found, got: %v", err)
	}
}

func TestRun_TransferBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--transfer", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "100.00"}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transfer) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "From:") && !strings.Contains(output, "Checking") {
		t.Error("output should show source account")
	}
	if !strings.Contains(output, "To:") && !strings.Contains(output, "Savings") {
		t.Error("output should show destination account")
	}
	if !strings.Contains(output, "$100.00") {
		t.Error("output should show transfer amount")
	}
}

func TestRun_TransferWithDateAndMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "250.50",
		"--date", "2024-06-15",
		"--memo", "Monthly savings",
	}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--transfer) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "2024-06-15") {
		t.Error("output should show transfer date")
	}
	if !strings.Contains(output, "Monthly savings") {
		t.Error("output should show memo")
	}
	if !strings.Contains(output, "$250.50") {
		t.Error("output should show transfer amount")
	}
}

func TestRun_TransferInvalidDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	repo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := repo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := repo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100",
		"--date", "not-a-date",
	}, stdout, stderr)
	if err == nil {
		t.Error("run(--transfer) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestRun_TransferVerifyTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create source and destination accounts
	acctRepo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("500.00"),
		models.Today(),
	)
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}
	database.Close()

	// Run transfer
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{
		"--transfer",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("run(--transfer) returned error: %v", err)
	}

	// Verify transactions were created
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := repository.NewTransactionRepository(database)

	// Check checking account transactions
	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("failed to list checking transactions: %v", err)
	}
	if len(checkingTxns) != 1 {
		t.Fatalf("expected 1 checking transaction, got %d", len(checkingTxns))
	}
	if !checkingTxns[0].Amount.Equal(models.MustNewMoney("-100.00")) {
		t.Errorf("checking transaction amount = %s, want -$100.00", checkingTxns[0].Amount.String())
	}
	if !checkingTxns[0].IsTransfer() {
		t.Error("checking transaction should be a transfer")
	}

	// Check savings account transactions
	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("failed to list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Fatalf("expected 1 savings transaction, got %d", len(savingsTxns))
	}
	if !savingsTxns[0].Amount.Equal(models.MustNewMoney("100.00")) {
		t.Errorf("savings transaction amount = %s, want $100.00", savingsTxns[0].Amount.String())
	}
	if !savingsTxns[0].IsTransfer() {
		t.Error("savings transaction should be a transfer")
	}

	// Verify they have the same transfer ID
	if checkingTxns[0].TransferID.ID != savingsTxns[0].TransferID.ID {
		t.Error("both transactions should have the same transfer ID")
	}
}

func TestPrintHelp_IncludesTransfer(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--transfer") {
		t.Error("help output should document --transfer flag")
	}
	if !strings.Contains(output, "Source account") || !strings.Contains(output, "--from") {
		t.Error("help output should document --from flag for transfers")
	}
	if !strings.Contains(output, "Destination account") || !strings.Contains(output, "--to") {
		t.Error("help output should document --to flag for transfers")
	}
}

// Tests for --search command
func TestParseArgs_SearchFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedTerm string
	}{
		{"long flag with space", []string{"--search", "amazon"}, "amazon"},
		{"long flag with equals", []string{"--search=amazon"}, "amazon"},
		{"term with spaces", []string{"--search", "coffee shop"}, "coffee shop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.searchTerm != tt.expectedTerm {
				t.Errorf("parseArgs(%v) searchTerm = %q, want %q", tt.args, opts.searchTerm, tt.expectedTerm)
			}
		})
	}
}

func TestParseArgs_SearchFlagMissingTerm(t *testing.T) {
	_, _, err := parseArgs([]string{"--search"})
	if err == nil {
		t.Error("parseArgs(--search) without term should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_MinMaxFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedMin string
		expectedMax string
	}{
		{"min flag with space", []string{"--min", "10.00"}, "10.00", ""},
		{"min flag with equals", []string{"--min=10.00"}, "10.00", ""},
		{"max flag with space", []string{"--max", "100.00"}, "", "100.00"},
		{"max flag with equals", []string{"--max=100.00"}, "", "100.00"},
		{"both flags", []string{"--min", "10.00", "--max", "100.00"}, "10.00", "100.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.minAmount != tt.expectedMin {
				t.Errorf("parseArgs(%v) minAmount = %q, want %q", tt.args, opts.minAmount, tt.expectedMin)
			}
			if opts.maxAmount != tt.expectedMax {
				t.Errorf("parseArgs(%v) maxAmount = %q, want %q", tt.args, opts.maxAmount, tt.expectedMax)
			}
		})
	}
}

func TestParseArgs_MinFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--min"})
	if err == nil {
		t.Error("parseArgs(--min) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_MaxFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--max"})
	if err == nil {
		t.Error("parseArgs(--max) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestRun_SearchMissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{"--search", "amazon"}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) without --file should return error")
	}
	if !strings.Contains(err.Error(), "requires --file") {
		t.Errorf("error should mention --file requirement, got: %v", err)
	}
}

func TestRun_SearchByPayee(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Amazon")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-50.00"))
	txn1.SetPayee(payee.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction 1: %v", err)
	}

	// Create another transaction without the payee
	txn2 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-25.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction 2: %v", err)
	}

	database.Close()

	// Run search
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Amazon", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "SEARCH RESULTS") {
		t.Error("output should contain SEARCH RESULTS header")
	}
	if !strings.Contains(output, "Amazon") {
		t.Error("output should contain Amazon payee")
	}
	if !strings.Contains(output, "-$50.00") {
		t.Error("output should contain the amount")
	}
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result, got: %s", output)
	}
}

func TestRun_SearchByMemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create transaction with memo
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney("-75.00"))
	txn1.SetMemo("Office supplies from Staples")
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	database.Close()

	// Run search for memo content
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "office", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result for memo search, got: %s", output)
	}
}

func TestRun_SearchNoResults(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "nonexistent", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "No transactions found") {
		t.Errorf("output should say no transactions found, got: %s", output)
	}
}

func TestRun_SearchWithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create two accounts
	acctRepo := repository.NewAccountRepository(database)
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("5000.00"), models.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Target")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions in both accounts
	txnRepo := repository.NewTransactionRepository(database)
	txn1 := models.NewTransaction(checking.ID, models.Today(), models.MustNewMoney("-50.00"))
	txn1.SetPayee(payee.ID)
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create checking transaction: %v", err)
	}

	txn2 := models.NewTransaction(savings.ID, models.Today(), models.MustNewMoney("-30.00"))
	txn2.SetPayee(payee.ID)
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create savings transaction: %v", err)
	}

	database.Close()

	// Search with account filter
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Target", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with account filter, got: %s", output)
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain Checking account")
	}
}

func TestRun_SearchWithMinMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Store")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions with different amounts
	txnRepo := repository.NewTransactionRepository(database)
	amounts := []string{"-10.00", "-50.00", "-100.00", "-200.00"}
	for _, amt := range amounts {
		txn := models.NewTransaction(account.ID, models.Today(), models.MustNewMoney(amt))
		txn.SetPayee(payee.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	// Search with min/max filters (for negative amounts, min means more negative)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Store", "--min", "-100.00", "--max", "-50.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 2 transaction(s)") {
		t.Errorf("output should show 2 results with amount filter, got: %s", output)
	}
}

func TestRun_SearchInvalidMinAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "test", "--min", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) with invalid --min should return error")
	}
	if !strings.Contains(err.Error(), "invalid --min") {
		t.Errorf("error should mention invalid --min, got: %v", err)
	}
}

func TestRun_SearchInvalidMaxAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "test", "--max", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Error("run(--search) with invalid --max should return error")
	}
	if !strings.Contains(err.Error(), "invalid --max") {
		t.Errorf("error should mention invalid --max, got: %v", err)
	}
}

func TestRun_SearchWithDateFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create a test account
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create transactions with different dates
	txnRepo := repository.NewTransactionRepository(database)
	jan15, _ := models.ParseDate("2024-01-15")
	feb15, _ := models.ParseDate("2024-02-15")
	mar15, _ := models.ParseDate("2024-03-15")

	dates := []models.Date{jan15, feb15, mar15}
	for _, d := range dates {
		txn := models.NewTransaction(account.ID, d, models.MustNewMoney("-5.00"))
		txn.SetPayee(payee.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}

	database.Close()

	// Search with date filter
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = run([]string{"--search", "Coffee", "--from", "2024-02-01", "--to", "2024-02-28", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Errorf("run(--search) returned error: %v", err)
		return
	}

	output := stdout.String()
	if !strings.Contains(output, "Found 1 transaction(s)") {
		t.Errorf("output should show 1 result with date filter, got: %s", output)
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain February transaction date")
	}
}

func TestPrintHelp_IncludesSearch(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--search") {
		t.Error("help output should document --search flag")
	}
	if !strings.Contains(output, "--min") {
		t.Error("help output should document --min flag")
	}
	if !strings.Contains(output, "--max") {
		t.Error("help output should document --max flag")
	}
	if !strings.Contains(output, "Search transactions") {
		t.Error("help output should describe search functionality")
	}
}

// =============================================================================
// Scheduled Transaction CLI Tests
// =============================================================================

func TestParseArgs_ScheduledFlags(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedScheduled bool
		expectedDue       bool
	}{
		{"scheduled flag", []string{"--scheduled"}, true, false},
		{"scheduled with due", []string{"--scheduled", "--due"}, true, true},
		{"due without scheduled", []string{"--due"}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.scheduled != tt.expectedScheduled {
				t.Errorf("parseArgs(%v) scheduled = %v, want %v", tt.args, opts.scheduled, tt.expectedScheduled)
			}
			if opts.scheduledDue != tt.expectedDue {
				t.Errorf("parseArgs(%v) scheduledDue = %v, want %v", tt.args, opts.scheduledDue, tt.expectedDue)
			}
		})
	}
}

func TestParseArgs_PostScheduledFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"with space", []string{"--post-scheduled", "abc123"}, "abc123"},
		{"with equals", []string{"--post-scheduled=abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.postScheduled != tt.expected {
				t.Errorf("parseArgs(%v) postScheduled = %q, want %q", tt.args, opts.postScheduled, tt.expected)
			}
		})
	}
}

func TestParseArgs_SkipScheduledFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"with space", []string{"--skip-scheduled", "abc123"}, "abc123"},
		{"with equals", []string{"--skip-scheduled=abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.skipScheduled != tt.expected {
				t.Errorf("parseArgs(%v) skipScheduled = %q, want %q", tt.args, opts.skipScheduled, tt.expected)
			}
		})
	}
}

func TestParseArgs_PostScheduledMissingID(t *testing.T) {
	_, _, err := parseArgs([]string{"--post-scheduled"})
	if err == nil {
		t.Error("parseArgs(--post-scheduled) without ID should return error")
	}
	if !strings.Contains(err.Error(), "requires a scheduled transaction ID") {
		t.Errorf("error should mention ID requirement, got: %v", err)
	}
}

func TestParseArgs_SkipScheduledMissingID(t *testing.T) {
	_, _, err := parseArgs([]string{"--skip-scheduled"})
	if err == nil {
		t.Error("parseArgs(--skip-scheduled) without ID should return error")
	}
	if !strings.Contains(err.Error(), "requires a scheduled transaction ID") {
		t.Errorf("error should mention ID requirement, got: %v", err)
	}
}

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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(),
		models.MustNewMoney("-15.99"),
	)
	st.SetPayee(payee.ID)
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a due scheduled transaction (today)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(
		account.ID,
		models.FrequencyMonthly,
		models.Today(), // Due today
		models.MustNewMoney("-10.00"),
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
	acctRepo := repository.NewAccountRepository(database)
	checking := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("500.00"), models.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	// Create scheduled transactions for each account
	stRepo := repository.NewScheduledTransactionRepository(database)
	st1 := models.NewScheduledTransactionWithAmount(checking.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-10.00"))
	if err := stRepo.Create(st1); err != nil {
		t.Fatalf("failed to create scheduled transaction 1: %v", err)
	}
	st2 := models.NewScheduledTransactionWithAmount(savings.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-20.00"))
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-15.99"))
	st.SetPayee(payee.ID)
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

	txnRepo := repository.NewTransactionRepository(database)
	txns, err := txnRepo.ListByAccount(account.ID)
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction (variable amount)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a payee
	payeeRepo := repository.NewPayeeRepository(database)
	payee := models.NewPayee("Netflix")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	// Create a scheduled transaction
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-15.99"))
	st.SetPayee(payee.ID)
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

	txnRepo := repository.NewTransactionRepository(database)
	txns, err := txnRepo.ListByAccount(account.ID)
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with variable amount (no amount set)
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	// Create a scheduled transaction with limited occurrences
	stRepo := repository.NewScheduledTransactionRepository(database)
	st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), models.MustNewMoney("-50.00"))
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

func TestPrintHelp_IncludesScheduled(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--scheduled") {
		t.Error("help output should document --scheduled flag")
	}
	if !strings.Contains(output, "--due") {
		t.Error("help output should document --due flag")
	}
	if !strings.Contains(output, "--post-scheduled") {
		t.Error("help output should document --post-scheduled flag")
	}
	if !strings.Contains(output, "--skip-scheduled") {
		t.Error("help output should document --skip-scheduled flag")
	}
	if !strings.Contains(output, "Scheduled Transaction Commands") {
		t.Error("help output should have Scheduled Transaction Commands section")
	}
}

// Report CLI Tests

func TestParseArgs_ReportFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReport bool
		wantType   string
	}{
		{"report flag only", []string{"--report", "net-worth"}, true, "net-worth"},
		{"report spending", []string{"--report", "spending"}, true, "spending"},
		{"report with equals", []string{"--report=net-worth"}, true, "net-worth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.report != tt.wantReport {
				t.Errorf("report = %v, want %v", opts.report, tt.wantReport)
			}
			if opts.reportType != tt.wantType {
				t.Errorf("reportType = %q, want %q", opts.reportType, tt.wantType)
			}
		})
	}
}

func TestParseArgs_ReportMonthFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantMonth string
	}{
		{"month with space", []string{"--month", "2024-01"}, "2024-01"},
		{"month with equals", []string{"--month=2024-06"}, "2024-06"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportMonth != tt.wantMonth {
				t.Errorf("reportMonth = %q, want %q", opts.reportMonth, tt.wantMonth)
			}
		})
	}
}

func TestParseArgs_ReportYearFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantYear int
	}{
		{"year with space", []string{"--year", "2024"}, 2024},
		{"year with equals", []string{"--year=2023"}, 2023},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportYear != tt.wantYear {
				t.Errorf("reportYear = %d, want %d", opts.reportYear, tt.wantYear)
			}
		})
	}
}

func TestParseArgs_ReportAsOfFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantVal string
	}{
		{"as-of with space", []string{"--as-of", "2024-01-15"}, "2024-01-15"},
		{"as-of with equals", []string{"--as-of=2023-12-31"}, "2023-12-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportAsOf != tt.wantVal {
				t.Errorf("reportAsOf = %q, want %q", opts.reportAsOf, tt.wantVal)
			}
		})
	}
}

func TestParseArgs_ReportMonthMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--month"})
	if err == nil {
		t.Error("--month without value should return error")
	}
}

func TestParseArgs_ReportYearMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--year"})
	if err == nil {
		t.Error("--year without value should return error")
	}
}

func TestParseArgs_ReportYearInvalidValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--year", "abc"})
	if err == nil {
		t.Error("--year with non-numeric value should return error")
	}
}

func TestParseArgs_ReportAsOfMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--as-of"})
	if err == nil {
		t.Error("--as-of without value should return error")
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
	acctRepo := repository.NewAccountRepository(database)

	checking := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("5000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}

	savings := models.NewAccount(
		"Savings",
		models.AccountTypeSavings,
		"USD",
		models.MustNewMoney("10000.00"),
		models.Today(),
	)
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	creditCard := models.NewAccount(
		"Credit Card",
		models.AccountTypeCreditCard,
		"USD",
		models.MustNewMoney("0"),
		models.Today(),
	)
	if err := acctRepo.Create(creditCard); err != nil {
		t.Fatalf("failed to create credit card account: %v", err)
	}

	// Add a credit card transaction (liability)
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(creditCard.ID, models.Today(), models.MustNewMoney("-500.00"))
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
	acctRepo := repository.NewAccountRepository(database)
	account := models.NewAccount(
		"Checking",
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("5000.00"),
		models.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(account); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create expense category
	catRepo := repository.NewCategoryRepository(database)
	groceries := models.NewCategory("Groceries", models.CategoryTypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// Create expense transaction
	txnRepo := repository.NewTransactionRepository(database)
	txn := models.NewTransaction(account.ID, models.MustParseDate("2024-01-15"), models.MustNewMoney("-150.00"))
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

func TestParseYearMonth(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantYear  int
		wantMonth int
		wantErr   bool
	}{
		{"valid January", "2024-01", 2024, 1, false},
		{"valid December", "2024-12", 2024, 12, false},
		{"invalid format", "2024/01", 0, 0, true},
		{"missing month", "2024", 0, 0, true},
		{"invalid year", "abcd-01", 0, 0, true},
		{"invalid month", "2024-ab", 0, 0, true},
		{"month too low", "2024-00", 0, 0, true},
		{"month too high", "2024-13", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, err := parseYearMonth(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseYearMonth(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseYearMonth(%q) unexpected error: %v", tt.input, err)
				return
			}
			if year != tt.wantYear {
				t.Errorf("parseYearMonth(%q) year = %d, want %d", tt.input, year, tt.wantYear)
			}
			if month != tt.wantMonth {
				t.Errorf("parseYearMonth(%q) month = %d, want %d", tt.input, month, tt.wantMonth)
			}
		})
	}
}

func TestPrintHelp_IncludesReports(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--report net-worth") {
		t.Error("help output should document --report net-worth")
	}
	if !strings.Contains(output, "--report spending") {
		t.Error("help output should document --report spending")
	}
	if !strings.Contains(output, "--as-of") {
		t.Error("help output should document --as-of flag")
	}
	if !strings.Contains(output, "--month") {
		t.Error("help output should document --month flag")
	}
	if !strings.Contains(output, "--year") {
		t.Error("help output should document --year flag")
	}
	if !strings.Contains(output, "Report Commands") {
		t.Error("help output should have Report Commands section")
	}
}
