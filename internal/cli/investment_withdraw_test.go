package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestInvestmentWithdraw_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "withdraw",
		"--account", "Brokerage",
		"--amount", "500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment withdraw) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentWithdraw_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "withdraw",
		"--file", "/fake.tdb",
		"--amount", "500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment withdraw) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentWithdraw_MissingAmount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "withdraw",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment withdraw) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestInvestmentWithdraw_Basic(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "500",
		"--memo", "Quarterly draw",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment withdraw) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Investment withdrawal created successfully") {
		t.Error("output should confirm withdrawal creation")
	}
	if !strings.Contains(output, "$500.00") {
		t.Error("output should contain withdrawal amount")
	}
	if !strings.Contains(output, "Quarterly draw") {
		t.Error("output should contain memo")
	}
}

func TestInvestmentWithdraw_WithDate(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "250",
		"--date", "2025-04-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment withdraw with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentWithdraw_AccountNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "NonExistent",
		"--amount", "500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentWithdraw_InvalidAmount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected invalid --amount error, got: %v", err)
	}
}

func TestInvestmentWithdraw_InvalidDate(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "500",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentWithdraw_AllowsNegativeCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "withdraw",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "999999",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	// Withdrawals never block on the running cash balance — succeeds, balance goes negative.
	if err != nil {
		t.Errorf("expected withdrawal to succeed even with insufficient cash, got: %v", err)
	}
}

func TestInvestmentWithdraw_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "withdraw", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment withdraw --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "withdraw") {
		t.Errorf("expected `investment withdraw --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsWithdraw(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "withdraw") {
		t.Errorf("expected `investment --help` to list `withdraw`; got:\n%s", stdout.String())
	}
}
