package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestInvestmentReinvest_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--amount", "300",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment reinvest) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentReinvest_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--shares", "2",
		"--amount", "300",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment reinvest) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentReinvest_NoSelector(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--shares", "2",
		"--amount", "300",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment reinvest) without a security selector should return error")
	}
	if !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected error to mention ticker selector, got: %v", err)
	}
}

func TestInvestmentReinvest_MissingShares(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "300",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment reinvest) without --shares should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "shares") {
		t.Errorf("expected Cobra required-flag error mentioning shares, got: %v", err)
	}
}

func TestInvestmentReinvest_MissingAmountAndPrice(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--amount") || !strings.Contains(err.Error(), "--price-per-share") {
		t.Errorf("expected error mentioning --amount and --price-per-share, got: %v", err)
	}
}

func TestInvestmentReinvest_BasicWithPricePerShare(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment reinvest) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Reinvest dividend transaction created successfully") {
		t.Error("output should confirm reinvest creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
}

func TestInvestmentReinvest_BasicWithAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--amount", "300",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment reinvest) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Reinvest dividend transaction created successfully") {
		t.Error("output should confirm reinvest creation")
	}
}

func TestInvestmentReinvest_WithDateAndMemo(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "150",
		"--date", "2025-04-15",
		"--memo", "DRIP",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment reinvest with date/memo) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentReinvest_AccountNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "NonExistent",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentReinvest_SecurityNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "FAKE",
		"--shares", "2",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentReinvest_InvalidShares(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "not-a-number",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --shares") {
		t.Errorf("expected invalid --shares error, got: %v", err)
	}
}

func TestInvestmentReinvest_InvalidAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--amount", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected invalid --amount error, got: %v", err)
	}
}

func TestInvestmentReinvest_InvalidPricePerShare(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --price-per-share") {
		t.Errorf("expected invalid --price-per-share error, got: %v", err)
	}
}

func TestInvestmentReinvest_InvalidDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "reinvest",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "2",
		"--price-per-share", "150",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentReinvest_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "reinvest", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment reinvest --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "reinvest") {
		t.Errorf("expected `investment reinvest --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsReinvest(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "reinvest") {
		t.Errorf("expected `investment --help` to list `reinvest`; got:\n%s", stdout.String())
	}
}
