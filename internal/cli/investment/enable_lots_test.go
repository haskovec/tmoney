package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentEnableLots_MissingFile(t *testing.T) {
	err := cli.ExecuteWith([]string{"investment", "enable-lots", "--account", "X"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestInvestmentEnableLots_RequiresAccountOrAll(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	err := cli.ExecuteWith([]string{"investment", "enable-lots", "--file", dbPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--account") {
		t.Fatalf("expected error requiring --account or --all, got: %v", err)
	}
}

func TestInvestmentEnableLots_PreviewThenConfirm(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("VTI", "Vanguard Total Stock Market", security.TypeETF)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("create security: %v", err)
	}
	invRepo := investmentdom.NewRepository(database)
	buy := investmentdom.NewTransactionWithSecurity(acct.ID, types.NewDate(2024, 1, 2),
		investmentdom.TransactionTypeBuy, types.MustNewMoney("-1000.00"), sec.ID, types.MustNewQuantity("10"))
	buy.SetPricePerShare(types.MustNewMoney("100.00"))
	if err := invRepo.Create(buy); err != nil {
		t.Fatalf("create buy: %v", err)
	}
	database.Close()

	// Preview: prints the plan, writes nothing.
	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "enable-lots", "--file", dbPath, "--account", "Brokerage"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(stdout.String(), "preview") || !strings.Contains(stdout.String(), "VTI") {
		t.Errorf("preview output unexpected: %q", stdout.String())
	}
	assertEnableLotsState(t, dbPath, acct.ID, false, 0)

	// Confirm: applies.
	stdout = &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "enable-lots", "--file", dbPath, "--account", "Brokerage", "--confirm"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !strings.Contains(stdout.String(), "lot tracking enabled") {
		t.Errorf("confirm output unexpected: %q", stdout.String())
	}
	assertEnableLotsState(t, dbPath, acct.ID, true, 1)
}

// assertEnableLotsState reopens the db and checks the account's TrackLots flag
// and lot count.
func assertEnableLotsState(t *testing.T, dbPath string, acctID types.ID, wantTrackLots bool, wantLots int) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer database.Close()

	acct, err := account.NewRepository(database).GetByID(acctID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acct.TrackLots != wantTrackLots {
		t.Errorf("TrackLots = %v, want %v", acct.TrackLots, wantTrackLots)
	}
	lots, err := investmentdom.NewLotRepository(database).ListAllByAccount(acctID)
	if err != nil {
		t.Fatalf("list lots: %v", err)
	}
	if len(lots) != wantLots {
		t.Errorf("lot count = %d, want %d", len(lots), wantLots)
	}
}
