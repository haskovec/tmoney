package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createInvestmentTransferTestDB creates a test database with two
// investment accounts ("Source IRA" and "Dest 401k"), an AAPL security,
// and 10 shares purchased in the source account at $150. Returns the
// dbPath. The database is closed after setup.
func createInvestmentTransferTestDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "transfer.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	src := account.NewAccount("Source IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(src); err != nil {
		t.Fatalf("failed to create source account: %v", err)
	}
	dst := account.NewAccount("Dest 401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(dst); err != nil {
		t.Fatalf("failed to create dest account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	svc := app.NewServices(database)
	if _, err := svc.Investment.Deposit(src.ID, types.Today(), types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("failed to deposit: %v", err)
	}
	pps := types.MustNewMoney("150")
	if _, err := svc.Investment.Buy(src.ID, sec.ID, types.Today(), types.MustNewQuantity("10"), nil, &pps, types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy: %v", err)
	}

	database.Close()
	return dbPath
}

func TestInvestmentTransfer_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment transfer) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentTransfer_MissingFrom(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", "/fake.tdb",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment transfer) without --from should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "from") {
		t.Errorf("expected Cobra required-flag error mentioning from, got: %v", err)
	}
}

func TestInvestmentTransfer_MissingTo(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", "/fake.tdb",
		"--from", "Source IRA",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment transfer) without --to should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "to") {
		t.Errorf("expected Cobra required-flag error mentioning to, got: %v", err)
	}
}

func TestInvestmentTransfer_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", "/fake.tdb",
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--shares", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment transfer) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestInvestmentTransfer_MissingShares(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", "/fake.tdb",
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment transfer) without --shares should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "shares") {
		t.Errorf("expected Cobra required-flag error mentioning shares, got: %v", err)
	}
}

func TestInvestmentTransfer_Basic(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment transfer) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Share transfer created successfully") {
		t.Error("output should confirm transfer creation")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "Source IRA") {
		t.Error("output should contain source account")
	}
	if !strings.Contains(output, "Dest 401k") {
		t.Error("output should contain dest account")
	}
}

func TestInvestmentTransfer_WithDate(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "3",
		"--date", "2025-04-15",
		"--memo", "rollover",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment transfer with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-04-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentTransfer_SourceAccountNotFound(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "NonExistent",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "source account") {
		t.Errorf("expected source account not found error, got: %v", err)
	}
}

func TestInvestmentTransfer_DestAccountNotFound(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "NonExistent",
		"--ticker", "AAPL",
		"--shares", "5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "destination account") {
		t.Errorf("expected destination account not found error, got: %v", err)
	}
}

func TestInvestmentTransfer_SecurityNotFound(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "ZZZZ",
		"--shares", "5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "security") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentTransfer_InvalidShares(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --shares") {
		t.Errorf("expected invalid --shares error, got: %v", err)
	}
}

func TestInvestmentTransfer_InvalidDate(t *testing.T) {
	dbPath := createInvestmentTransferTestDB(t)

	err := executeWith([]string{
		"investment", "transfer",
		"--file", dbPath,
		"--from", "Source IRA",
		"--to", "Dest 401k",
		"--ticker", "AAPL",
		"--shares", "5",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid --date error, got: %v", err)
	}
}

func TestInvestmentTransfer_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "transfer", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment transfer --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "transfer") {
		t.Errorf("expected `investment transfer --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsTransfer(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "transfer") {
		t.Errorf("expected `investment --help` to list `transfer`; got:\n%s", stdout.String())
	}
}
