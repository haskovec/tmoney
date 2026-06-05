package price

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	pricedom "github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// execPrice runs args against a throwaway root that exposes only the price
// command plus the inherited persistent --file flag. The white-box update
// tests use it instead of cli.ExecuteWith, which would form an import cycle
// (price -> cli -> price). It mirrors how the real root dispatches `price`.
func execPrice(args []string, stdout, stderr io.Writer) error {
	root := &cobra.Command{
		Use:           "tmoney",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringP("file", "f", "", "Database file path")
	root.AddCommand(NewCmd())
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.Execute()
}

// =============================================================================
// `tmoney price update` CLI tests
// =============================================================================

// withYahooAt overrides the price-provider registration hook so that
// `price update` points the Yahoo provider at the given httptest server
// and uses a fixed clock. Returns a cleanup func.
func withYahooAt(t *testing.T, baseURL string, now time.Time) func() {
	t.Helper()
	prev := registerPriceProviders
	registerPriceProviders = func(svc *app.Services) {
		svc.Price.ProviderRegistry().Register(pricedom.NewYahooProvider(
			pricedom.WithBaseURL(baseURL),
			pricedom.WithClock(func() time.Time { return now }),
		))
	}
	return func() { registerPriceProviders = prev }
}

func setupUpdatePricesDB(t *testing.T, tickers ...string) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "prices.tdb")
	t.Cleanup(func() { database.Close() })

	secRepo := security.NewRepository(database)
	for _, tk := range tickers {
		sec := security.NewSecurity(tk, tk+" Inc.", security.TypeStock)
		if err := secRepo.Create(sec); err != nil {
			t.Fatalf("create %s: %v", tk, err)
		}
	}
	return dbPath
}

func yahooFixture(t *testing.T, ticker string, date string, price float64) string {
	t.Helper()
	loc, _ := time.LoadLocation("America/New_York")
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		t.Fatalf("parse date %q: %v", date, err)
	}
	bar := time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, loc).Unix()
	end := time.Date(d.Year(), d.Month(), d.Day(), 16, 0, 0, 0, loc).Unix()
	return fmt.Sprintf(`{"chart":{"result":[{"meta":{"currency":"USD","symbol":"%s","exchangeTimezoneName":"America/New_York","gmtoffset":-14400,"regularMarketTime":%d,"regularMarketPrice":%g,"currentTradingPeriod":{"regular":{"start":%d,"end":%d}}},"timestamp":[%d],"indicators":{"quote":[{"close":[%g]}]}}],"error":null}}`,
		ticker, end, price, bar, end, bar, price)
}

func TestPriceUpdate_MissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := execPrice([]string{"price", "update"}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error for missing --file")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("error %q, expected mention of --file/file", err)
	}
}

func TestPriceUpdate_HappyPath(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "AAPL")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(yahooFixture(t, "AAPL", "2026-04-22", 271.06)))
	}))
	defer srv.Close()

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 4, 22, 17, 0, 0, 0, loc) // 1 hour after close
	cleanup := withYahooAt(t, srv.URL, now)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := execPrice([]string{"price", "update", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("execPrice error = %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "AAPL") {
		t.Errorf("output missing AAPL: %s", out)
	}
	if !strings.Contains(out, "271.06") {
		t.Errorf("output missing 271.06: %s", out)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("output missing 'updated' status: %s", out)
	}
	if !strings.Contains(out, "1 updated") {
		t.Errorf("output missing summary count: %s", out)
	}

	// Verify the price was actually persisted.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer database.Close()
	svc := app.NewServices(database)
	sec, err := svc.Security.GetByTicker("AAPL", "")
	if err != nil {
		t.Fatalf("get security: %v", err)
	}
	stored, err := svc.Price.GetCurrentPrice(sec.ID, types.Today())
	if err != nil {
		t.Fatalf("get current price: %v", err)
	}
	if stored.Price.String() != "271.06" {
		t.Errorf("stored price = %q, want 271.06", stored.Price.String())
	}
	if stored.Source != pricedom.SourceAPI {
		t.Errorf("stored source = %q, want %q", stored.Source, pricedom.SourceAPI)
	}
}

func TestPriceUpdate_FilterByPositionalTickers(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "AAPL", "MSFT", "GOOG")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := strings.TrimPrefix(r.URL.Path, "/v8/finance/chart/")
		w.Header().Set("Content-Type", "application/json")
		var px float64
		switch ticker {
		case "AAPL":
			px = 271.06
		case "MSFT":
			px = 415.00
		case "GOOG":
			px = 180.00
		}
		_, _ = w.Write([]byte(yahooFixture(t, ticker, "2026-04-22", px)))
	}))
	defer srv.Close()

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 4, 22, 17, 0, 0, 0, loc)
	cleanup := withYahooAt(t, srv.URL, now)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := execPrice([]string{"price", "update", "AAPL", "MSFT", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("execPrice error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "AAPL") {
		t.Errorf("output missing AAPL")
	}
	if !strings.Contains(out, "MSFT") {
		t.Errorf("output missing MSFT")
	}
	if strings.Contains(out, "GOOG") {
		t.Errorf("output should not contain GOOG when filtered: %s", out)
	}
	if !strings.Contains(out, "2 updated") {
		t.Errorf("output missing summary '2 updated': %s", out)
	}
}

func TestPriceUpdate_NonZeroExitOnFailure(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "AAPL", "BADTICK")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticker := strings.TrimPrefix(r.URL.Path, "/v8/finance/chart/")
		w.Header().Set("Content-Type", "application/json")
		if ticker == "BADTICK" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"chart":{"result":null,"error":{"code":"Not Found","description":"unknown"}}}`))
			return
		}
		_, _ = w.Write([]byte(yahooFixture(t, ticker, "2026-04-22", 271.06)))
	}))
	defer srv.Close()

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 4, 22, 17, 0, 0, 0, loc)
	cleanup := withYahooAt(t, srv.URL, now)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := execPrice([]string{"price", "update", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("expected non-nil error when a ticker fails")
	}

	out := stdout.String()
	// AAPL still updated, BADTICK reported as failed.
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "updated") {
		t.Errorf("output missing AAPL/updated: %s", out)
	}
	if !strings.Contains(out, "BADTICK") || !strings.Contains(out, "failed") {
		t.Errorf("output missing BADTICK/failed: %s", out)
	}
}

func TestPriceUpdate_UnknownProvider(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "AAPL")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(yahooFixture(t, "AAPL", "2026-04-22", 271.06)))
	}))
	defer srv.Close()
	loc, _ := time.LoadLocation("America/New_York")
	cleanup := withYahooAt(t, srv.URL, time.Date(2026, 4, 22, 17, 0, 0, 0, loc))
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := execPrice([]string{"price", "update", "--provider", "doesnotexist", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "doesnotexist") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention unknown provider", err)
	}
}

func TestPriceUpdate_UpToDateSecondRun(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "AAPL")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(yahooFixture(t, "AAPL", "2026-04-22", 271.06)))
	}))
	defer srv.Close()

	loc, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 4, 22, 17, 0, 0, 0, loc)
	cleanup := withYahooAt(t, srv.URL, now)
	defer cleanup()

	// First run: updates.
	if err := execPrice([]string{"price", "update", "--file", dbPath}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run: should report up-to-date.
	stdout := &bytes.Buffer{}
	if err := execPrice([]string{"price", "update", "--file", dbPath}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "up-to-date") {
		t.Errorf("second-run output should report 'up-to-date': %s", out)
	}
}
