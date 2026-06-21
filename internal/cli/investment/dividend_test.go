package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestInvestmentDividend_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "50",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment dividend) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentDividend_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--amount", "50",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment dividend) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentDividend_NoSelector(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "100",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment dividend) without a security selector should return error")
	}
	if !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected error to mention ticker selector, got: %v", err)
	}
}

func TestInvestmentDividend_MissingAmount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment dividend) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestInvestmentDividend_Basic(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "125.50",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment dividend) returned error: %v", err)
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

func TestInvestmentDividend_WithDateAndMemo(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "75",
		"--date", "2025-04-15",
		"--memo", "Q1 dividend",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment dividend with date/memo) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentDividend_AccountNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "NonExistent",
		"--ticker", "AAPL",
		"--amount", "50",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentDividend_SecurityNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "FAKE",
		"--amount", "50",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentDividend_InvalidAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected invalid --amount error, got: %v", err)
	}
}

func TestInvestmentDividend_InvalidDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "50",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentDividend_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "dividend", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment dividend --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "dividend") {
		t.Errorf("expected `investment dividend --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsDividend(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "dividend") {
		t.Errorf("expected `investment --help` to list `dividend`; got:\n%s", stdout.String())
	}
}
