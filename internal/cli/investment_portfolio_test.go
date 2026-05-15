package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
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
	if !strings.Contains(output, "Account totals") {
		t.Error("output should contain account totals section")
	}
	if !strings.Contains(output, "Cash") {
		t.Error("output should show cash row")
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
	if !strings.Contains(output, "UNREAL") {
		t.Error("output should contain UNREAL column header")
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
	if !strings.Contains(output, "Account totals") {
		t.Error("output should contain Account totals section")
	}
	if !strings.Contains(output, "Cash") {
		t.Error("output should show cash row")
	}
	if !strings.Contains(output, "Market value") {
		t.Error("output should show market value row")
	}
	if !strings.Contains(output, "Total value") {
		t.Error("output should show total value row")
	}
	if !strings.Contains(output, "Cost basis (open)") {
		t.Error("output should show cost basis (open) row")
	}
	if !strings.Contains(output, "Unrealized gain") {
		t.Error("output should show unrealized gain row")
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
	if !strings.Contains(output, "Account totals") {
		t.Error("output should contain Account totals section")
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

func TestInvestmentPortfolio_OmitsClosedByDefault(t *testing.T) {
	dbPath := createPortfolioCmdTestDBWithClosed(t)

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
	if strings.Contains(output, "Closed positions") {
		t.Errorf("default output should not contain 'Closed positions' heading; got:\n%s", output)
	}
	if strings.Contains(output, "MSFT") {
		t.Errorf("default output should not list fully-sold MSFT; got:\n%s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("default output should still list open AAPL; got:\n%s", output)
	}
}

func TestInvestmentPortfolio_IncludeClosed_PrintsHeading(t *testing.T) {
	dbPath := createPortfolioCmdTestDBWithClosed(t)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--include-closed",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio --include-closed) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Closed positions") {
		t.Errorf("--include-closed output should contain 'Closed positions' heading; got:\n%s", output)
	}
	if !strings.Contains(output, "MSFT") {
		t.Errorf("--include-closed output should list fully-sold MSFT; got:\n%s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("--include-closed output should still list open AAPL; got:\n%s", output)
	}
}

func TestInvestmentPortfolio_IncludeClosed_NoClosedPositions_NoHeading(t *testing.T) {
	dbPath := createPortfolioCmdTestDB(t, false)

	stdout := &bytes.Buffer{}
	err := executeWith([]string{
		"investment", "portfolio",
		"--file", dbPath,
		"--account", "Brokerage",
		"--include-closed",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executeWith(investment portfolio --include-closed) returned error: %v", err)
	}

	if strings.Contains(stdout.String(), "Closed positions") {
		t.Errorf("--include-closed on account without closed positions should not print 'Closed positions' heading; got:\n%s", stdout.String())
	}
}

// TR-018: the per-holding table gains total-return columns. Header must
// include the new column labels and rows must render the populated
// values for an account with dividends, a realized gain, and fees.
func TestInvestmentPortfolio_TotalReturnColumns(t *testing.T) {
	dbPath := createPortfolioCmdTestDBWithTotalReturn(t)

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

	for _, want := range []string{"UNREAL", "DIV", "REAL", "FEES", "TOTAL RETURN", "RET %"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected output to contain column header %q; got:\n%s", want, output)
		}
	}

	// Legacy "Gain/Loss" header on the holdings table is gone (renamed to
	// UNREAL). The summary block still says "Total Gain/Loss:" so the bare
	// substring still appears overall, but the holdings table header line
	// itself must not contain it.
	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		if strings.Contains(line, "Ticker") && strings.Contains(line, "Gain/Loss") {
			t.Errorf("holdings table header should not contain legacy 'Gain/Loss' column; got line:\n%s", line)
		}
	}

	// Dividends ($50) and a per-share realized gain should appear on the
	// AAPL row.
	if !strings.Contains(output, "50.00") {
		t.Errorf("expected output to contain the $50 dividend; got:\n%s", output)
	}
}

// TR-018: when a holding's realized gain is unavailable (non-lot account
// with corporate actions), the REAL column renders the "unavailable"
// placeholder.
func TestInvestmentPortfolio_RealizedGainUnavailable(t *testing.T) {
	dbPath := createPortfolioCmdTestDBWithCorporateAction(t)

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
	if !strings.Contains(output, "unavailable") {
		t.Errorf("expected output to render 'unavailable' for realized gain on the AAPL row; got:\n%s", output)
	}
	// Dividends should still render normally even when realized gain is
	// unavailable — the gate only suppresses the realized-gain column.
	if !strings.Contains(output, "50.00") {
		t.Errorf("expected the $50 dividend to render even with corporate-action gate; got:\n%s", output)
	}
}

// TR-019: the portfolio command prints an Account totals block below the
// holdings table with one row per total-return component, in the order
// specified by the spec.
func TestInvestmentPortfolio_AccountTotalsBlock(t *testing.T) {
	dbPath := createPortfolioCmdTestDBWithTotalReturn(t)

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

	wantInOrder := []string{
		"Cost basis (open)",
		"Unrealized gain",
		"Realized gain",
		"Dividends received",
		"Interest received",
		"Fees paid",
		"Total return",
		"Total return %",
	}
	lastIdx := -1
	var lastLabel string
	for _, want := range wantInOrder {
		idx := strings.Index(output, want)
		if idx < 0 {
			t.Errorf("expected output to contain totals row %q; got:\n%s", want, output)
			continue
		}
		if idx < lastIdx {
			t.Errorf("totals row %q (idx=%d) appeared before previous row %q (idx=%d); got:\n%s",
				want, idx, lastLabel, lastIdx, output)
		}
		lastIdx = idx
		lastLabel = want
	}
}

// TR-019: when TotalReturnPct is nil (no buys ever, denominator is zero),
// the "Total return %" row renders the "—" placeholder rather than 0%.
func TestInvestmentPortfolio_TotalReturnPctNilRendersDash(t *testing.T) {
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
	var pctLine string
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "Total return %") {
			pctLine = line
			break
		}
	}
	if pctLine == "" {
		t.Fatalf("expected output to contain 'Total return %%' line; got:\n%s", output)
	}
	if !strings.Contains(pctLine, "—") {
		t.Errorf("expected 'Total return %%' line to contain '—' when TotalReturnPct is nil; got line: %q", pctLine)
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

// createPortfolioCmdTestDBWithClosed creates a DB with one open position
// (AAPL) and one fully-sold (closed) position (MSFT) so that
// `investment portfolio --include-closed` has something to surface.
func createPortfolioCmdTestDBWithClosed(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "portfolio_closed.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
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
	if err := priceRepo.Create(price.NewPrice(aapl.ID, types.Today(), types.MustNewMoney("175.00"), price.SourceManual)); err != nil {
		t.Fatalf("failed to create AAPL price: %v", err)
	}
	if err := priceRepo.Create(price.NewPrice(msft.ID, types.Today(), types.MustNewMoney("420.00"), price.SourceManual)); err != nil {
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
	if _, err := svc.Investment.Sell(acct.ID, msft.ID, types.Today(), types.MustNewQuantity("5"), nil, ptrMoney("450"), types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("failed to sell MSFT: %v", err)
	}

	database.Close()
	return dbPath
}

// createPortfolioCmdTestDBWithTotalReturn builds a DB with one open
// position (AAPL) that exercises every total-return component: buy with
// commission, partial sell with commission (-> realized gain + fees),
// and a cash dividend.
func createPortfolioCmdTestDBWithTotalReturn(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "portfolio_tr.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	aapl := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(aapl); err != nil {
		t.Fatalf("failed to create AAPL: %v", err)
	}

	priceRepo := price.NewRepository(database)
	if err := priceRepo.Create(price.NewPrice(aapl.ID, types.Today(), types.MustNewMoney("175.00"), price.SourceManual)); err != nil {
		t.Fatalf("failed to create AAPL price: %v", err)
	}

	svc := app.NewServices(database)
	if _, err := svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit"); err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}
	if _, err := svc.Investment.Buy(acct.ID, aapl.ID, types.Today(), types.MustNewQuantity("10"), nil, ptrMoney("150"), types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("failed to buy AAPL: %v", err)
	}
	if _, err := svc.Investment.Sell(acct.ID, aapl.ID, types.Today(), types.MustNewQuantity("3"), nil, ptrMoney("160"), types.MustNewMoney("2"), "", nil); err != nil {
		t.Fatalf("failed to sell AAPL: %v", err)
	}
	if _, err := svc.Investment.Dividend(acct.ID, aapl.ID, types.Today(), types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("failed to record AAPL dividend: %v", err)
	}

	database.Close()
	return dbPath
}

// createPortfolioCmdTestDBWithCorporateAction builds a DB whose non-lot
// account has a corporate action (4:1 split) on a held security, so the
// valuation service flags realized gain as unavailable.
func createPortfolioCmdTestDBWithCorporateAction(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "portfolio_ca.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	aapl := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(aapl); err != nil {
		t.Fatalf("failed to create AAPL: %v", err)
	}

	priceRepo := price.NewRepository(database)
	if err := priceRepo.Create(price.NewPrice(aapl.ID, types.Today(), types.MustNewMoney("30.00"), price.SourceManual)); err != nil {
		t.Fatalf("failed to create AAPL price: %v", err)
	}

	svc := app.NewServices(database)

	d1 := types.NewDate(2024, time.March, 1)
	dSplit := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.July, 1)

	if _, err := svc.Investment.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}
	if _, err := svc.Investment.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"), nil, ptrMoney("100"), types.ZeroMoney, ""); err != nil {
		t.Fatalf("failed to buy AAPL: %v", err)
	}
	if _, err := svc.Investment.Dividend(acct.ID, aapl.ID, d1, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("failed to record AAPL dividend: %v", err)
	}
	if _, err := svc.CorporateAction.Split(aapl.ID, dSplit, investment.SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("failed to split AAPL: %v", err)
	}
	if _, err := svc.Investment.Sell(acct.ID, aapl.ID, d2, types.MustNewQuantity("5"), nil, ptrMoney("30"), types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("failed to sell AAPL: %v", err)
	}

	database.Close()
	return dbPath
}
