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
