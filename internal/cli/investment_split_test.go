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

// createInvestmentSplitTestDB creates a database with an investment
// account "Brokerage" holding 100 shares of AAPL at $150/share. If
// trackLots is true, the account uses lot tracking. Returns the dbPath.
// The database is closed after setup.
func createInvestmentSplitTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "split.tdb")

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
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	svc := app.NewServices(database)
	if _, err := svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit"); err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}
	totalAmount := types.MustNewMoney("15000")
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.Today(), types.MustNewQuantity("100"), &totalAmount, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	database.Close()
	return dbPath
}

func TestInvestmentSplit_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--ticker", "AAPL",
		"--ratio", "4:1",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment split) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentSplit_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", "/fake.tdb",
		"--ratio", "4:1",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment split) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestInvestmentSplit_MissingRatio(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(investment split) without --ratio should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ratio") {
		t.Errorf("expected Cobra required-flag error mentioning ratio, got: %v", err)
	}
}

func TestInvestmentSplit_InvalidRatio(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, false)
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "invalid",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --ratio") {
		t.Errorf("expected invalid ratio error, got: %v", err)
	}
}

func TestInvestmentSplit_SecurityNotFound(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, false)
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "ZZZZ",
		"--ratio", "4:1",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected security not found error, got: %v", err)
	}
}

func TestInvestmentSplit_ForwardSplit(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "4:1",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment split) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Stock split applied successfully") {
		t.Error("output should confirm split")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain ticker")
	}
	if !strings.Contains(output, "4:1") {
		t.Error("output should contain ratio")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestInvestmentSplit_ReverseSplit(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "1:10",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment split reverse) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Stock split applied successfully") {
		t.Error("output should confirm split")
	}
	if !strings.Contains(output, "1:10") {
		t.Error("output should contain ratio")
	}
}

func TestInvestmentSplit_WithDate(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "2:1",
		"--date", "2025-01-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment split with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-01-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentSplit_WithLotTracking(t *testing.T) {
	dbPath := createInvestmentSplitTestDB(t, true)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "4:1",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment split lot-tracking) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Stock split applied successfully") {
		t.Error("output should confirm split for lot-tracking account")
	}
}

func TestInvestmentSplit_InvalidDate(t *testing.T) {
	err := executeWith([]string{
		"investment", "split",
		"--file", "test.tdb",
		"--ticker", "AAPL",
		"--ratio", "4:1",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

func TestInvestmentSplit_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "split", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment split --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "split") {
		t.Errorf("expected `investment split --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsSplit(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "split") {
		t.Errorf("expected `investment --help` to list `split`; got:\n%s", stdout.String())
	}
}
