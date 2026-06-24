package price

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
	pricedom "github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// stubProvider is a fixed-quote price provider for the --refetch tests. It
// registers under "yahoo" so the command's default provider name resolves.
type stubProvider struct{ price types.Money }

func (p stubProvider) FetchQuote(string) (*pricedom.Quote, error) {
	return &pricedom.Quote{Price: p.price, Currency: "USD"}, nil
}
func (p stubProvider) FetchQuoteOn(_ string, d types.Date) (*pricedom.Quote, error) {
	return &pricedom.Quote{Date: d, Price: p.price, Currency: "USD"}, nil
}
func (p stubProvider) Name() string { return "yahoo" }

// withStubProvider overrides the provider-registration hook so --refetch pulls
// from a fixed-quote stub instead of the network. Returns a cleanup func.
func withStubProvider(t *testing.T, quote types.Money) func() {
	t.Helper()
	prev := registerPriceProviders
	registerPriceProviders = func(svc *app.Services) {
		svc.Price.ProviderRegistry().Register(stubProvider{price: quote})
	}
	return func() { registerPriceProviders = prev }
}

// seedCleanupDB builds a DB whose VSIAX security has one income-only legacy
// price: a buy on 2023-06-15 (justified $71 transaction price), a reinvested
// dividend on 2023-06-22 (no auto-price under current policy), and a hand-seeded
// $60 source=transaction price on the reinvest date standing in for legacy data.
func seedCleanupDB(t *testing.T) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "cleanup.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("VSIAX", "Vanguard Small Cap Value", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("create security: %v", err)
	}

	svc := app.NewServices(database)
	buyDate := types.NewDate(2023, time.June, 15)
	reinvestDate := types.NewDate(2023, time.June, 22)

	buyTotal := types.MustNewMoney("710.00") // 10 sh @ 71.00 → justified transaction price
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, buyDate, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("buy: %v", err)
	}
	rTotal := types.MustNewMoney("0.24")
	if _, err := svc.Investment.ReinvestDividend(acct.ID, sec.ID, reinvestDate, types.MustNewQuantity("0.004"), &rTotal, nil, ""); err != nil {
		t.Fatalf("reinvest: %v", err)
	}

	// Legacy income-only price: 0.24 / 0.004 = $60, but the real NAV was ~$71.
	priceRepo := pricedom.NewRepository(database)
	if err := priceRepo.Create(pricedom.NewPrice(sec.ID, reinvestDate, types.MustNewMoney("60.00"), pricedom.SourceTransaction)); err != nil {
		t.Fatalf("seed legacy price: %v", err)
	}

	if err := database.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return dbPath
}

func TestPriceCleanup_HelpListsCleanup(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "cleanup") {
		t.Errorf("expected `price --help` to list `cleanup`; got:\n%s", stdout.String())
	}
}

func TestPriceCleanup_NoCandidates(t *testing.T) {
	// AAPL has a source=transaction price but no transactions backing it, so it
	// is not income-only and must be left alone.
	dbPath, _ := clitest.CreateTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "cleanup", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price cleanup): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "No income-only") {
		t.Errorf("expected a 'nothing to clean up' message, got:\n%s", stdout.String())
	}
}

func TestPriceCleanup_DryRunListsCandidateWithoutDeleting(t *testing.T) {
	dbPath := seedCleanupDB(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "cleanup", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price cleanup): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"VSIAX", "2023-06-22", "60.00", "1 candidate", "--confirm"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run output, got:\n%s", want, out)
		}
	}

	// Dry-run must not delete: the legacy price is still listed.
	stdout.Reset()
	if err := execPrice([]string{"price", "list", "VSIAX", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price list): %v", err)
	}
	if !strings.Contains(stdout.String(), "60.00") {
		t.Errorf("dry-run should not delete the price; expected 60.00 still listed, got:\n%s", stdout.String())
	}
}

func TestPriceCleanup_DeleteConfirm(t *testing.T) {
	dbPath := seedCleanupDB(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "cleanup", "--file", dbPath, "--confirm"}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price cleanup --confirm): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"deleted", "1 deleted"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in confirm output, got:\n%s", want, out)
		}
	}

	// The legacy $60 price is gone; the buy's $71 price survives.
	stdout.Reset()
	if err := execPrice([]string{"price", "list", "VSIAX", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price list): %v", err)
	}
	listing := stdout.String()
	if strings.Contains(listing, "60.00") {
		t.Errorf("deleted price 60.00 should be gone, got:\n%s", listing)
	}
	if !strings.Contains(listing, "71.00") {
		t.Errorf("the buy's 71.00 price should remain, got:\n%s", listing)
	}
}

func TestPriceCleanup_RefetchConfirm(t *testing.T) {
	restore := withStubProvider(t, types.MustNewMoney("70.50"))
	defer restore()

	dbPath := seedCleanupDB(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "cleanup", "--file", dbPath, "--refetch", "--confirm"}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price cleanup --refetch --confirm): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"refetched", "70.50", "1 refetched"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in refetch output, got:\n%s", want, out)
		}
	}

	// The legacy $60 price is replaced by the provider's $70.50.
	stdout.Reset()
	if err := execPrice([]string{"price", "list", "VSIAX", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price list): %v", err)
	}
	listing := stdout.String()
	if strings.Contains(listing, "60.00") {
		t.Errorf("legacy price 60.00 should be replaced, got:\n%s", listing)
	}
	if !strings.Contains(listing, "70.50") {
		t.Errorf("expected refetched 70.50 in listing, got:\n%s", listing)
	}
}

func TestPriceCleanup_TickerScoped(t *testing.T) {
	dbPath := seedCleanupDB(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := execPrice([]string{"price", "cleanup", "--file", dbPath, "--ticker", "VSIAX"}, stdout, stderr); err != nil {
		t.Fatalf("execPrice(price cleanup --ticker VSIAX): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "VSIAX") || !strings.Contains(stdout.String(), "1 candidate") {
		t.Errorf("expected the VSIAX candidate when scoped by --ticker, got:\n%s", stdout.String())
	}
}
