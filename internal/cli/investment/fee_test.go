package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentFee_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--account", "Brokerage",
		"--amount", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment fee) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentFee_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", "/fake.tdb",
		"--amount", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment fee) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentFee_MissingAmount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment fee) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestInvestmentFee_Basic(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "25.00",
		"--memo", "Annual fee",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment fee) returned error: %v", err)
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

func TestInvestmentFee_WithDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "10",
		"--date", "2025-04-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment fee with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentFee_AccountNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "NonExistent",
		"--amount", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentFee_InvalidAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected invalid --amount error, got: %v", err)
	}
}

func TestInvestmentFee_InvalidDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "25",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentFee_AllowsNegativeCash(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	database.Close()

	err := cli.ExecuteWith([]string{
		"investment", "fee",
		"--file", dbPath,
		"--account", "Brokerage",
		"--amount", "100",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	// Fees never block on the running cash balance — succeeds, balance goes negative.
	if err != nil {
		t.Errorf("expected fee to succeed even with insufficient cash, got: %v", err)
	}
}

func TestInvestmentFee_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "fee", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment fee --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "fee") {
		t.Errorf("expected `investment fee --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsFee(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "fee") {
		t.Errorf("expected `investment --help` to list `fee`; got:\n%s", stdout.String())
	}
}
