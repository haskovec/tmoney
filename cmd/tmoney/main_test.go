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
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run([]string{}, stdout, stderr)
	if err != nil {
		t.Errorf("run([]) returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "TMoney") {
		t.Error("output should contain TMoney")
	}
}

func TestRun_UnknownArgs(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	// Currently unknown args are silently ignored and we fall through to TUI mode
	err := run([]string{"some-file.tdb"}, stdout, stderr)
	if err != nil {
		t.Errorf("run with file argument returned error: %v", err)
	}
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
