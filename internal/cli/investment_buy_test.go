package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
)

func TestInvestmentBuy_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment buy) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentBuy_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment buy) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentBuy_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--shares", "10",
		"--amount", "1500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment buy) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestInvestmentBuy_MissingShares(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", "/fake.tdb",
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "1500",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment buy) without --shares should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "shares") {
		t.Errorf("expected Cobra required-flag error mentioning shares, got: %v", err)
	}
}

func TestInvestmentBuy_MissingAmountAndPrice(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment buy) without --amount or --price-per-share should return error")
	}
	if !strings.Contains(err.Error(), "--amount") {
		t.Errorf("expected error to mention --amount, got: %v", err)
	}
}

func TestInvestmentBuy_WithTotalAmount(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment buy) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "Brokerage") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "10") {
		t.Error("output should contain shares")
	}
}

func TestInvestmentBuy_WithPricePerShare(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment buy with price-per-share) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation")
	}
	if !strings.Contains(output, "$150.00") {
		t.Error("output should contain price per share")
	}
}

func TestInvestmentBuy_WithCommission(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1510",
		"--commission", "10",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment buy with commission) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Commission") {
		t.Error("output should show commission")
	}
}

func TestInvestmentBuy_WithDateAndMemo(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "5",
		"--price-per-share", "150",
		"--date", "2025-06-15",
		"--memo", "Buying AAPL dip",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment buy with date/memo) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "2025-06-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentBuy_AccountNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "NonExistent",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected account not found error, got: %v", err)
	}
}

func TestInvestmentBuy_SecurityNotFound(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "FAKE",
		"--shares", "10",
		"--amount", "1500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentBuy_InsufficientCash(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "1000",
		"--price-per-share", "150",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	// 1000 * 150 = 150,000 > 50,000 cash
	if err == nil {
		t.Error("expected insufficient cash error")
	}
}

func TestInvestmentBuy_WithLotTracking(t *testing.T) {
	dbPath := createInvestmentTestDB(t, true)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--price-per-share", "150",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment buy with lot tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Buy transaction created successfully") {
		t.Error("output should confirm buy creation with lot tracking")
	}

	// Verify a lot was created
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	svc := app.NewServices(database)
	sec, _ := svc.Security.GetByTicker("AAPL", "")
	lots, err := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if err != nil {
		t.Fatalf("failed to list lots: %v", err)
	}
	if len(lots) != 1 {
		t.Errorf("expected 1 lot, got %d", len(lots))
	}
}

func TestInvestmentBuy_InvalidShares(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "not-a-number",
		"--amount", "1500",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --shares") {
		t.Errorf("expected invalid --shares error, got: %v", err)
	}
}

func TestInvestmentBuy_InvalidDate(t *testing.T) {
	dbPath := createInvestmentTestDB(t, false)

	err := executeWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentBuy_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "buy", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment buy --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "buy") {
		t.Errorf("expected `investment buy --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsBuy(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "buy") {
		t.Errorf("expected `investment --help` to list `buy`; got:\n%s", stdout.String())
	}
}
