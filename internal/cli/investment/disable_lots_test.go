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

func TestInvestmentDisableLots_MissingFile(t *testing.T) {
	err := cli.ExecuteWith([]string{"investment", "disable-lots", "--account", "X"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestInvestmentDisableLots_RequiresAccountOrAll(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	err := cli.ExecuteWith([]string{"investment", "disable-lots", "--file", dbPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--account") {
		t.Fatalf("expected error requiring --account or --all, got: %v", err)
	}
}

func TestInvestmentDisableLots_RefusesNonLotAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	database.Close()

	err := cli.ExecuteWith([]string{"investment", "disable-lots", "--file", dbPath, "--account", "Brokerage"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not lot-tracked") {
		t.Fatalf("expected 'not lot-tracked' error, got: %v", err)
	}
}

func TestInvestmentDisableLots_PreviewThenConfirm(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = true
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("VTI", "Vanguard Total Stock Market", security.TypeETF)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("create security: %v", err)
	}
	invRepo := investmentdom.NewRepository(database)
	buyDate := types.NewDate(2024, 1, 2)
	buy := investmentdom.NewTransactionWithSecurity(acct.ID, buyDate,
		investmentdom.TransactionTypeBuy, types.MustNewMoney("-1000.00"), sec.ID, types.MustNewQuantity("10"))
	buy.SetPricePerShare(types.MustNewMoney("100.00"))
	if err := invRepo.Create(buy); err != nil {
		t.Fatalf("create buy: %v", err)
	}
	lot := investmentdom.NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("100.00"), buyDate, buy.ID)
	if err := investmentdom.NewLotRepository(database).Create(&lot); err != nil {
		t.Fatalf("create lot: %v", err)
	}
	database.Close()

	// Preview: prints the plan, writes nothing.
	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "disable-lots", "--file", dbPath, "--account", "Brokerage"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(stdout.String(), "preview") {
		t.Errorf("preview output unexpected: %q", stdout.String())
	}
	assertDisableLotsState(t, dbPath, acct.ID, true, 1)

	// Confirm: applies.
	stdout = &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "disable-lots", "--file", dbPath, "--account", "Brokerage", "--confirm"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !strings.Contains(stdout.String(), "lot tracking disabled") {
		t.Errorf("confirm output unexpected: %q", stdout.String())
	}
	assertDisableLotsState(t, dbPath, acct.ID, false, 0)
}

// assertDisableLotsState reopens the db and checks the account's TrackLots flag
// and lot count.
func assertDisableLotsState(t *testing.T, dbPath string, acctID types.ID, wantTrackLots bool, wantLots int) {
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

func TestInvestmentDisableLots_All(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	secRepo := security.NewRepository(database)
	invRepo := investmentdom.NewRepository(database)
	lotRepo := investmentdom.NewLotRepository(database)

	sec := security.NewSecurity("VTI", "Vanguard Total Stock Market", security.TypeETF)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("create security: %v", err)
	}

	buyDate := types.NewDate(2024, 1, 2)
	mkLotAccount := func(name string) *account.Account {
		acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
		acct.TrackLots = true
		if err := acctRepo.Create(acct); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		buy := investmentdom.NewTransactionWithSecurity(acct.ID, buyDate,
			investmentdom.TransactionTypeBuy, types.MustNewMoney("-1000.00"), sec.ID, types.MustNewQuantity("10"))
		buy.SetPricePerShare(types.MustNewMoney("100.00"))
		if err := invRepo.Create(buy); err != nil {
			t.Fatalf("create buy: %v", err)
		}
		lot := investmentdom.NewLot(acct.ID, sec.ID, types.MustNewQuantity("10"), types.MustNewMoney("100.00"), buyDate, buy.ID)
		if err := lotRepo.Create(&lot); err != nil {
			t.Fatalf("create lot: %v", err)
		}
		return acct
	}
	a := mkLotAccount("IRA A")
	b := mkLotAccount("IRA B")
	// A non-lot investment account must be skipped by --all.
	c := account.NewAccount("Brokerage C", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(c); err != nil {
		t.Fatalf("create C: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "disable-lots", "--file", dbPath, "--all", "--confirm"}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("disable-lots --all: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "IRA A") || !strings.Contains(out, "IRA B") {
		t.Errorf("--all output should name both lot-tracked accounts; got: %q", out)
	}
	if strings.Contains(out, "Brokerage C") {
		t.Errorf("non-lot account should not be processed by --all; got: %q", out)
	}
	assertDisableLotsState(t, dbPath, a.ID, false, 0)
	assertDisableLotsState(t, dbPath, b.ID, false, 0)
	assertDisableLotsState(t, dbPath, c.ID, false, 0)
}
