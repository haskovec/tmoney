package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentDeposit_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "deposit",
		"--account", "Brokerage",
		"--amount", "1000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment deposit) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentDeposit_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "deposit",
		"--file", "/fake.tdb",
		"--amount", "1000",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment deposit) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentDeposit_MissingAmount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "deposit",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment deposit) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestInvestmentDeposit_Basic(t *testing.T) {
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
	err = executeWith([]string{
		"investment", "deposit",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "5000",
		"--memo", "Initial funding",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment deposit) returned error: %v", err)
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

func TestInvestmentDeposit_WithDate(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "deposit",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "250",
		"--date", "2025-04-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment deposit with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentDeposit_AccountNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "deposit",
		"--file", dbPath,
		"--account", "NonExistent",
		"--amount", "1000",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentDeposit_InvalidAmount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "deposit",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected invalid --amount error, got: %v", err)
	}
}

func TestInvestmentDeposit_InvalidDate(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "deposit",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "1000",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentDeposit_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "deposit", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment deposit --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "deposit") {
		t.Errorf("expected `investment deposit --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsDeposit(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "deposit") {
		t.Errorf("expected `investment --help` to list `deposit`; got:\n%s", stdout.String())
	}
}
