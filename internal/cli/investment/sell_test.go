package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentSell_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--amount", "800",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment sell) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentSell_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--shares", "5",
		"--amount", "800",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment sell) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentSell_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--shares", "5",
		"--amount", "800",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment sell) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestInvestmentSell_MissingShares(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "800",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment sell) without --shares should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "shares") {
		t.Errorf("expected Cobra required-flag error mentioning shares, got: %v", err)
	}
}

func TestInvestmentSell_MissingAmountAndPrice(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment sell) without --amount or --price-per-share should return error")
	}
	if !strings.Contains(err.Error(), "--amount") {
		t.Errorf("expected error to mention --amount, got: %v", err)
	}
}

func TestInvestmentSell_Basic(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment sell) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Sell transaction created successfully") {
		t.Error("output should confirm sell creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
}

func TestInvestmentSell_WithTotalAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--amount", "800",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment sell with --amount) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Sell transaction created successfully") {
		t.Error("output should confirm sell creation with --amount")
	}
}

func TestInvestmentSell_WithCommission(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
		"--commission", "10",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment sell with commission) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Commission") {
		t.Error("output should show commission line")
	}
}

func TestInvestmentSell_InsufficientShares(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL",
		"--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "20",
		"--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected insufficient shares error")
	}
}

func TestInvestmentSell_AccountNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "NonExistent",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentSell_SecurityNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "FAKE",
		"--shares", "5",
		"--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentSell_InvalidShares(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "not-a-number",
		"--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --shares") {
		t.Errorf("expected invalid --shares error, got: %v", err)
	}
}

func TestInvestmentSell_InvalidDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentSell_WithLotAllocation(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

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
	if _, err := svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.Today(), types.MustNewQuantity("10"), nil, clitest.PtrMoney("150"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy: %v", err)
	}

	lots, _ := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if len(lots) == 0 {
		t.Fatal("no lots found after buy")
	}
	lotID := lots[0].ID.String()
	database.Close()

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "sell",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "160",
		"--lot", lotID,
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment sell with --lot) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Sell transaction created successfully") {
		t.Error("output should confirm sell with lot allocation")
	}
}

func TestInvestmentSell_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "sell", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment sell --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "sell") {
		t.Errorf("expected `investment sell --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsSell(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "sell") {
		t.Errorf("expected `investment --help` to list `sell`; got:\n%s", stdout.String())
	}
}
