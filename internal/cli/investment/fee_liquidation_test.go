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

func TestInvestmentFeeLiquidation_MissingFile(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation",
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--amount", "80",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_MissingAccount(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation",
		"--file", "/fake.tdb", "--ticker", "AAPL", "--shares", "0.5", "--amount", "80",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_MissingTicker(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation",
		"--file", "/fake.tdb", "--account", "Brokerage", "--shares", "0.5", "--amount", "80",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected required-flag error mentioning ticker, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_MissingShares(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation",
		"--file", "/fake.tdb", "--account", "Brokerage", "--ticker", "AAPL", "--amount", "80",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "shares") {
		t.Errorf("expected required-flag error mentioning shares, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_MissingAmountAndPrice(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation",
		"--file", dbPath, "--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--amount") {
		t.Errorf("expected error to mention --amount, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_Basic(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--price-per-share", "160",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("fee-liquidation returned error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Fee-liquidation transaction created successfully") {
		t.Errorf("output should confirm fee-liquidation creation; got: %q", out)
	}
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "Fee:") {
		t.Errorf("output should contain ticker and Fee line; got: %q", out)
	}
}

func TestInvestmentFeeLiquidation_WithTotalAmount(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--amount", "80",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("fee-liquidation with --amount returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Fee-liquidation transaction created successfully") {
		t.Error("output should confirm fee-liquidation with --amount")
	}
}

func TestInvestmentFeeLiquidation_WithCommission(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--price-per-share", "160", "--commission", "1",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("fee-liquidation with commission returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Commission") {
		t.Error("output should show commission line")
	}
}

func TestInvestmentFeeLiquidation_InsufficientShares(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "10", "--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "20", "--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Error("expected insufficient shares error")
	}
}

func TestInvestmentFeeLiquidation_AccountNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "NonExistent", "--ticker", "AAPL", "--shares", "0.5", "--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_SecurityNotFound(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "FAKE", "--shares", "0.5", "--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_InvalidShares(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "not-a-number", "--price-per-share", "160",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --shares") {
		t.Errorf("expected invalid --shares error, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_InvalidDate(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)
	err := cli.ExecuteWith([]string{
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--price-per-share", "160", "--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentFeeLiquidation_WithLotAllocation(t *testing.T) {
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
		"investment", "fee-liquidation", "--file", dbPath,
		"--account", "Brokerage", "--ticker", "AAPL", "--shares", "0.5", "--price-per-share", "160", "--lot", lotID,
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("fee-liquidation with --lot returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Fee-liquidation transaction created successfully") {
		t.Error("output should confirm fee-liquidation with lot allocation")
	}
}

func TestInvestmentFeeLiquidation_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "fee-liquidation", "--help"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("fee-liquidation --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "fee-liquidation") {
		t.Errorf("expected --help to describe fee-liquidation; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsFeeLiquidation(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("investment --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "fee-liquidation") {
		t.Errorf("expected `investment --help` to list fee-liquidation; got:\n%s", stdout.String())
	}
}
