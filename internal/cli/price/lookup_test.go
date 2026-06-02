package price

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serveYahoo returns an httptest server that always responds with the given
// chart fixture body, regardless of query params.
func serveYahoo(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPriceLookup_MissingFile(t *testing.T) {
	err := execPrice([]string{"price", "lookup", "--ticker", "AAPL", "--date", "2024-01-15"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got %v", err)
	}
}

func TestPriceLookup_FetchesAndPrints(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "GBTC")
	srv := serveYahoo(t, yahooFixture(t, "GBTC", "2024-07-31", 52.07))
	defer withYahooAt(t, srv.URL, time.Now())()

	stdout := &bytes.Buffer{}
	if err := execPrice([]string{"price", "lookup", "--ticker", "GBTC", "--date", "2024-07-31", "--file", dbPath}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "52.07") || !strings.Contains(out, "2024-07-31") {
		t.Errorf("lookup output unexpected: %q", out)
	}
}

func TestPriceAdd_Fetch(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "GBTC")
	srv := serveYahoo(t, yahooFixture(t, "GBTC", "2024-07-31", 52.07))
	defer withYahooAt(t, srv.URL, time.Now())()

	stdout := &bytes.Buffer{}
	if err := execPrice([]string{"price", "add", "--ticker", "GBTC", "--date", "2024-07-31", "--fetch", "--file", dbPath}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("price add --fetch: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "52.07") || !strings.Contains(out, "api") {
		t.Errorf("add --fetch output unexpected: %q", out)
	}

	// The fetched price should be persisted (visible via price current).
	stdout.Reset()
	if err := execPrice([]string{"price", "current", "GBTC", "--file", dbPath}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("price current: %v", err)
	}
	if !strings.Contains(stdout.String(), "52.07") {
		t.Errorf("fetched price not persisted; price current = %q", stdout.String())
	}
}

func TestPriceAdd_FetchAndPriceConflict(t *testing.T) {
	dbPath := setupUpdatePricesDB(t, "GBTC")
	err := execPrice([]string{"price", "add", "--ticker", "GBTC", "--date", "2024-07-31", "--price", "50", "--fetch", "--file", dbPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
