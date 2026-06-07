package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// buyViaCLI records a buy of AAPL through the CLI.
func buyViaCLI(t *testing.T, dbPath, shares, pps, date string, extra ...string) {
	t.Helper()
	args := []string{
		"investment", "buy", "--file", dbPath, "--account", "Brokerage",
		"--ticker", "AAPL", "--shares", shares, "--price-per-share", pps, "--date", date,
	}
	args = append(args, extra...)
	if err := cli.ExecuteWith(args, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("buy via CLI (%s) failed: %v", date, err)
	}
}

// lotIDByDate returns the open AAPL lot whose purchase date matches dateStr.
func lotIDByDate(t *testing.T, dbPath, dateStr string) string {
	t.Helper()
	d, err := types.ParseDate(dateStr)
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()
	svc := app.NewServices(database)
	sec, _ := svc.Security.GetByTicker("AAPL", "")
	lots, _ := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	for _, l := range lots {
		if l.PurchaseDate.Time().Equal(d.Time()) {
			return l.ID.String()
		}
	}
	t.Fatalf("no open lot with purchase date %s", dateStr)
	return ""
}

func TestInvestmentSplitLot_Forward(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, true)

	// Original buy, then a security-wide 2:1 split (so split-lot is durable),
	// then a missed buy entered raw before the split.
	buyViaCLI(t, dbPath, "10", "100", "2020-01-10")
	if err := cli.ExecuteWith([]string{
		"investment", "split", "--file", dbPath, "--ticker", "AAPL",
		"--ratio", "2:1", "--date", "2020-08-31",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("split: %v", err)
	}
	buyViaCLI(t, dbPath, "10", "90", "2020-02-01")
	missedLotID := lotIDByDate(t, dbPath, "2020-02-01")

	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "split-lot",
		"--file", dbPath,
		"--lot", missedLotID,
		"--ratio", "2:1",
	}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("split-lot returned error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Lot split applied successfully") {
		t.Errorf("output should confirm split-lot; got:\n%s", out)
	}
	if !strings.Contains(out, "20") || !strings.Contains(out, "45") {
		t.Errorf("output should show 20 shares @ 45; got:\n%s", out)
	}

	// Verify the missed lot and the aggregate position, durable across a reopen.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()
	svc := app.NewServices(database)
	sec, _ := svc.Security.GetByTicker("AAPL", "")
	acct, _ := svc.Account.GetByName("Brokerage")
	lots, _ := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if len(lots) != 2 {
		t.Fatalf("expected 2 lots, got %d", len(lots))
	}
	for _, l := range lots {
		if !l.Shares.Equal(types.MustNewQuantity("20")) {
			t.Errorf("lot %s shares = %s, want 20", l.ID, l.Shares.String())
		}
		if l.Shares.Cmp(l.OriginalShares) != 0 {
			t.Errorf("lot %s shares != original_shares (%s vs %s)", l.ID, l.Shares.String(), l.OriginalShares.String())
		}
	}
	pos, err := svc.PositionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity: %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("position shares = %s, want 40 (20 + 20)", pos.Shares.String())
	}
}

func TestInvestmentSplitLot_MissingLot(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "split-lot",
		"--file", "/fake.tdb",
		"--ratio", "2:1",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "lot") {
		t.Errorf("expected required-flag error mentioning lot, got: %v", err)
	}
}

func TestInvestmentSplitLot_MissingRatio(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "split-lot",
		"--file", "/fake.tdb",
		"--lot", "019e9fea-463f-75bc-9044-cd6f10bb53f0",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ratio") {
		t.Errorf("expected required-flag error mentioning ratio, got: %v", err)
	}
}

func TestInvestmentSplitLot_InvalidLotID(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, true)
	err := cli.ExecuteWith([]string{
		"investment", "split-lot",
		"--file", dbPath,
		"--lot", "not-a-uuid",
		"--ratio", "2:1",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --lot") {
		t.Errorf("expected invalid --lot error, got: %v", err)
	}
}

func TestInvestmentCmd_HelpListsSplitLot(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "split-lot") {
		t.Errorf("expected `investment --help` to list `split-lot`; got:\n%s", stdout.String())
	}
}
