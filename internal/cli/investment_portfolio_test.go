package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentPortfolio_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment portfolio) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentPortfolio_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", "test.tdb",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment portfolio) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentPortfolio_AccountNotFound(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false)
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "NonExistent",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentPortfolio_EmptyAccount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
	if !strings.Contains(output, "(No holdings)") {
		t.Error("output should indicate no holdings")
	}
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain summary section")
	}
	if !strings.Contains(output, "Cash Balance:") {
		t.Error("output should show cash balance")
	}
}

func TestInvestmentPortfolio_WithHoldings(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio) returned error: %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
	if !strings.Contains(output, "Ticker") {
		t.Error("output should contain Ticker column header")
	}
	if !strings.Contains(output, "Shares") {
		t.Error("output should contain Shares column header")
	}
	if !strings.Contains(output, "Market Value") {
		t.Error("output should contain Market Value column header")
	}
	if !strings.Contains(output, "Gain/Loss") {
		t.Error("output should contain Gain/Loss column header")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain AAPL ticker")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("output should contain Apple Inc. name")
	}
	if !strings.Contains(output, "MSFT") {
		t.Error("output should contain MSFT ticker")
	}
	if !strings.Contains(output, "Microsoft Corp.") {
		t.Error("output should contain Microsoft Corp. name")
	}
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain SUMMARY section")
	}
	if !strings.Contains(output, "Cash Balance:") {
		t.Error("output should show cash balance")
	}
	if !strings.Contains(output, "Market Value:") {
		t.Error("output should show market value")
	}
	if !strings.Contains(output, "Total Value:") {
		t.Error("output should show total value")
	}
	if !strings.Contains(output, "Total Cost Basis:") {
		t.Error("output should show total cost basis")
	}
	if !strings.Contains(output, "Total Gain/Loss:") {
		t.Error("output should show total gain/loss")
	}
}

func TestInvestmentPortfolio_WithAsOf(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--as-of", "2099-12-31",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio --as-of) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
}

func TestInvestmentPortfolio_InvalidAsOf(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false)

	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--as-of", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --as-of") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

func TestInvestmentPortfolio_ShowLotsWithLotTracking(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, true) // lot-tracking enabled

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--show-lots",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio --show-lots) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PORTFOLIO: Brokerage (with lots)") {
		t.Error("output should contain portfolio header with lots indicator")
	}
	if !strings.Contains(output, "Lot") {
		t.Error("output should contain Lot column header")
	}
	if !strings.Contains(output, "Purchase Date") {
		t.Error("output should contain Purchase Date column header")
	}
	if !strings.Contains(output, "Cost/Share") {
		t.Error("output should contain Cost/Share column header")
	}
	if !strings.Contains(output, "Cost Basis") {
		t.Error("output should contain Cost Basis column header")
	}
	if !strings.Contains(output, "Current Value") {
		t.Error("output should contain Current Value column header")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain AAPL ticker")
	}
	if !strings.Contains(output, "MSFT") {
		t.Error("output should contain MSFT ticker")
	}
	if !strings.Contains(output, "SUMMARY") {
		t.Error("output should contain SUMMARY section")
	}
}

func TestInvestmentPortfolio_ShowLotsNonLotTracking(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false) // lot-tracking disabled

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--show-lots",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio --show-lots non-lot) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PORTFOLIO: Brokerage") {
		t.Error("output should contain portfolio header")
	}
	if strings.Contains(output, "(with lots)") {
		t.Error("output should not indicate lots for non-lot-tracking account")
	}
}

func TestInvestmentPortfolio_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "portfolio", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment portfolio --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "portfolio") {
		t.Errorf("expected `investment portfolio --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsPortfolio(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "portfolio") {
		t.Errorf("expected `investment --help` to list `portfolio`; got:\n%s", stdout.String())
	}
}

// createPortfolioCmdTestDB creates a test DB with an investment account,
// securities, prices, and buy transactions for portfolio testing.
func createPortfolioCmdTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "portfolio.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = trackLots
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	aapl := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(aapl); err != nil {
		t.Fatalf("failed to create AAPL: %v", err)
	}
	msft := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)
	if err := secRepo.Create(msft); err != nil {
		t.Fatalf("failed to create MSFT: %v", err)
	}

	priceRepo := price.NewRepository(database)
	p1 := price.NewPrice(aapl.ID, types.Today(), types.MustNewMoney("175.00"), price.SourceManual)
	if err := priceRepo.Create(p1); err != nil {
		t.Fatalf("failed to create AAPL price: %v", err)
	}
	p2 := price.NewPrice(msft.ID, types.Today(), types.MustNewMoney("420.00"), price.SourceManual)
	if err := priceRepo.Create(p2); err != nil {
		t.Fatalf("failed to create MSFT price: %v", err)
	}

	svc := app.NewServices(database)
	if _, err := svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit"); err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	if _, err := svc.Investment.Buy(acct.ID, aapl.ID, types.Today(), types.MustNewQuantity("10"), nil, ptrMoney("150"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy AAPL: %v", err)
	}
	if _, err := svc.Investment.Buy(acct.ID, msft.ID, types.Today(), types.MustNewQuantity("5"), nil, ptrMoney("400"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy MSFT: %v", err)
	}

	database.Close()
	return dbPath
}
