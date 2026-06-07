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

// `buy --catch-up-splits` brings a back-dated buy's raw lot into line with a
// split that already ran, on a lot-tracked account.
func TestInvestmentBuy_CatchUpSplits_LotTracked(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, true)

	// Original pre-split buy, then a security-wide 2:1 split.
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath, "--account", "Brokerage",
		"--ticker", "AAPL", "--shares", "10", "--price-per-share", "100", "--date", "2020-01-10",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first buy: %v", err)
	}
	if err := cli.ExecuteWith([]string{
		"investment", "split", "--file", dbPath, "--ticker", "AAPL",
		"--ratio", "2:1", "--date", "2020-08-31",
	}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("split: %v", err)
	}

	// The missed buy, back-dated before the split, with --catch-up-splits.
	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath, "--account", "Brokerage",
		"--ticker", "AAPL", "--shares", "10", "--price-per-share", "90", "--date", "2020-02-01",
		"--catch-up-splits",
	}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("catch-up buy: %v", err)
	}
	if !strings.Contains(stdout.String(), "1 split applied") {
		t.Errorf("output should report 1 catch-up split; got:\n%s", stdout.String())
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()
	svc := app.NewServices(database)
	sec, _ := svc.Security.GetByTicker("AAPL", "")
	acct, _ := svc.Account.GetByName("Brokerage")

	// Both lots are now 20 shares (10 → 20 each); position is 40.
	lots, _ := svc.LotRepo.GetOpenLotsBySecurity(sec.ID)
	if len(lots) != 2 {
		t.Fatalf("expected 2 lots, got %d", len(lots))
	}
	for _, l := range lots {
		if !l.Shares.Equal(types.MustNewQuantity("20")) {
			t.Errorf("lot %s shares = %s, want 20", l.ID, l.Shares.String())
		}
		if l.Shares.Cmp(l.OriginalShares) != 0 {
			t.Errorf("lot %s shares (%s) != original_shares (%s)", l.ID, l.Shares.String(), l.OriginalShares.String())
		}
	}
	pos, err := svc.PositionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity: %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("position shares = %s, want 40", pos.Shares.String())
	}
}

// On a non-lot account there's no lot to catch up; the flag is a no-op.
func TestInvestmentBuy_CatchUpSplits_NonLotNoop(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "buy", "--file", dbPath, "--account", "Brokerage",
		"--ticker", "AAPL", "--shares", "10", "--price-per-share", "100",
		"--catch-up-splits",
	}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if !strings.Contains(stdout.String(), "none applied") {
		t.Errorf("output should report no catch-up on a non-lot account; got:\n%s", stdout.String())
	}
}
