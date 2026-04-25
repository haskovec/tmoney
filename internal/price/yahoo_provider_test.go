package price

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =============================================================================
// YahooProvider
// =============================================================================

func TestYahooProvider_ImplementsInterface(t *testing.T) {
	var _ Provider = &YahooProvider{}
}

func TestYahooProvider_Name(t *testing.T) {
	p := NewYahooProvider()
	if p.Name() != "yahoo" {
		t.Errorf("Name() = %q, want %q", p.Name(), "yahoo")
	}
}

func TestYahooProvider_FetchQuote(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	// Five consecutive trading days ending Wed 2026-04-22.
	// Each daily bar is timestamped at the session's start (09:30 EDT).
	wed0930 := time.Date(2026, 4, 22, 9, 30, 0, 0, loc).Unix()
	wed1600 := time.Date(2026, 4, 22, 16, 0, 0, 0, loc).Unix()
	tue0930 := time.Date(2026, 4, 21, 9, 30, 0, 0, loc).Unix()
	mon0930 := time.Date(2026, 4, 20, 9, 30, 0, 0, loc).Unix()
	fri0930 := time.Date(2026, 4, 17, 9, 30, 0, 0, loc).Unix()
	thu0930 := time.Date(2026, 4, 16, 9, 30, 0, 0, loc).Unix()

	closedBody := fmt.Sprintf(`{
		"chart": {
			"result": [{
				"meta": {
					"currency": "USD",
					"symbol": "AAPL",
					"exchangeTimezoneName": "America/New_York",
					"gmtoffset": -14400,
					"regularMarketTime": %d,
					"regularMarketPrice": 271.06,
					"currentTradingPeriod": {"regular": {"start": %d, "end": %d}}
				},
				"timestamp": [%d, %d, %d, %d, %d],
				"indicators": {"quote": [{"close": [265.10, 267.45, 269.80, 273.43, 271.06]}]}
			}],
			"error": null
		}
	}`, wed1600, wed0930, wed1600, thu0930, fri0930, mon0930, tue0930, wed0930)

	t.Run("session closed: returns regular market price with that day's date", func(t *testing.T) {
		srv := newYahooTestServer(t, http.StatusOK, closedBody)
		defer srv.Close()

		// 5 minutes after close: today's bar is closed.
		now := time.Unix(wed1600+300, 0)
		p := NewYahooProvider(
			WithHTTPClient(srv.Client()),
			WithBaseURL(srv.URL),
			WithClock(func() time.Time { return now }),
		)

		quote, err := p.FetchQuote("AAPL")
		if err != nil {
			t.Fatalf("FetchQuote() error = %v", err)
		}
		if quote.Currency != "USD" {
			t.Errorf("Currency = %q, want %q", quote.Currency, "USD")
		}
		if got := quote.Date.String(); got != "2026-04-22" {
			t.Errorf("Date = %q, want 2026-04-22", got)
		}
		if got := quote.Price.String(); got != "271.06" {
			t.Errorf("Price = %q, want 271.06", got)
		}
	})

	t.Run("intraday: returns prior closed session", func(t *testing.T) {
		// Same body, but clock is mid-session on Wednesday.
		intradayBody := fmt.Sprintf(`{
			"chart": {
				"result": [{
					"meta": {
						"currency": "USD",
						"symbol": "AAPL",
						"exchangeTimezoneName": "America/New_York",
						"gmtoffset": -14400,
						"regularMarketTime": %d,
						"regularMarketPrice": 270.00,
						"currentTradingPeriod": {"regular": {"start": %d, "end": %d}}
					},
					"timestamp": [%d, %d, %d, %d, %d],
					"indicators": {"quote": [{"close": [265.10, 267.45, 269.80, 273.43, 270.00]}]}
				}],
				"error": null
			}
		}`, wed0930+90*60, wed0930, wed1600, thu0930, fri0930, mon0930, tue0930, wed0930)

		srv := newYahooTestServer(t, http.StatusOK, intradayBody)
		defer srv.Close()

		// Wed 11:00 EDT — session in progress.
		now := time.Unix(wed0930+90*60, 0)
		p := NewYahooProvider(
			WithHTTPClient(srv.Client()),
			WithBaseURL(srv.URL),
			WithClock(func() time.Time { return now }),
		)

		quote, err := p.FetchQuote("AAPL")
		if err != nil {
			t.Fatalf("FetchQuote() error = %v", err)
		}
		if got := quote.Date.String(); got != "2026-04-21" {
			t.Errorf("Date = %q, want 2026-04-21", got)
		}
		if got := quote.Price.String(); got != "273.43" {
			t.Errorf("Price = %q, want 273.43", got)
		}
	})

	t.Run("non-trading day: latest bar is already closed", func(t *testing.T) {
		// Sat 2026-04-25 10:00 EDT — Friday's close is the latest closed bar.
		// Body has Friday as the last bar (Wed thu fri).
		satBody := fmt.Sprintf(`{
			"chart": {
				"result": [{
					"meta": {
						"currency": "USD",
						"symbol": "AAPL",
						"exchangeTimezoneName": "America/New_York",
						"gmtoffset": -14400,
						"regularMarketTime": %d,
						"regularMarketPrice": 271.06,
						"currentTradingPeriod": {"regular": {"start": %d, "end": %d}}
					},
					"timestamp": [%d, %d, %d, %d, %d],
					"indicators": {"quote": [{"close": [265.10, 267.45, 269.80, 273.43, 271.06]}]}
				}],
				"error": null
			}
		}`,
			time.Date(2026, 4, 17, 16, 0, 0, 0, loc).Unix(),
			time.Date(2026, 4, 27, 9, 30, 0, 0, loc).Unix(),
			time.Date(2026, 4, 27, 16, 0, 0, 0, loc).Unix(),
			time.Date(2026, 4, 13, 9, 30, 0, 0, loc).Unix(),
			time.Date(2026, 4, 14, 9, 30, 0, 0, loc).Unix(),
			time.Date(2026, 4, 15, 9, 30, 0, 0, loc).Unix(),
			time.Date(2026, 4, 16, 9, 30, 0, 0, loc).Unix(),
			time.Date(2026, 4, 17, 9, 30, 0, 0, loc).Unix(),
		)

		srv := newYahooTestServer(t, http.StatusOK, satBody)
		defer srv.Close()

		now := time.Date(2026, 4, 25, 10, 0, 0, 0, loc)
		p := NewYahooProvider(
			WithHTTPClient(srv.Client()),
			WithBaseURL(srv.URL),
			WithClock(func() time.Time { return now }),
		)

		quote, err := p.FetchQuote("AAPL")
		if err != nil {
			t.Fatalf("FetchQuote() error = %v", err)
		}
		if got := quote.Date.String(); got != "2026-04-17" {
			t.Errorf("Date = %q, want 2026-04-17", got)
		}
		if got := quote.Price.String(); got != "271.06" {
			t.Errorf("Price = %q, want 271.06", got)
		}
	})

	t.Run("nil close at last bar: skips to prior bar", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"chart": {
				"result": [{
					"meta": {
						"currency": "USD",
						"symbol": "AAPL",
						"exchangeTimezoneName": "America/New_York",
						"gmtoffset": -14400,
						"regularMarketTime": %d,
						"regularMarketPrice": 0,
						"currentTradingPeriod": {"regular": {"start": %d, "end": %d}}
					},
					"timestamp": [%d, %d, %d, %d, %d],
					"indicators": {"quote": [{"close": [265.10, 267.45, 269.80, 273.43, null]}]}
				}],
				"error": null
			}
		}`, wed1600, wed0930, wed1600, thu0930, fri0930, mon0930, tue0930, wed0930)

		srv := newYahooTestServer(t, http.StatusOK, body)
		defer srv.Close()

		now := time.Unix(wed1600+300, 0)
		p := NewYahooProvider(
			WithHTTPClient(srv.Client()),
			WithBaseURL(srv.URL),
			WithClock(func() time.Time { return now }),
		)

		quote, err := p.FetchQuote("AAPL")
		if err != nil {
			t.Fatalf("FetchQuote() error = %v", err)
		}
		if got := quote.Date.String(); got != "2026-04-21" {
			t.Errorf("Date = %q, want 2026-04-21", got)
		}
		if got := quote.Price.String(); got != "273.43" {
			t.Errorf("Price = %q, want 273.43", got)
		}
	})

	t.Run("unknown ticker: chart.error set", func(t *testing.T) {
		body := `{"chart":{"result":null,"error":{"code":"Not Found","description":"No data found, symbol may be delisted"}}}`
		srv := newYahooTestServer(t, http.StatusNotFound, body)
		defer srv.Close()

		p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
		_, err := p.FetchQuote("NOPE")
		if err == nil {
			t.Fatal("FetchQuote() expected error")
		}
		var notFound *UnsupportedTickerError
		if !errors.As(err, &notFound) {
			t.Errorf("error %v, want UnsupportedTickerError", err)
		}
	})

	t.Run("empty result array: returns UnsupportedTickerError", func(t *testing.T) {
		body := `{"chart":{"result":[],"error":null}}`
		srv := newYahooTestServer(t, http.StatusOK, body)
		defer srv.Close()

		p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
		_, err := p.FetchQuote("EMPTY")
		var notFound *UnsupportedTickerError
		if !errors.As(err, &notFound) {
			t.Errorf("error %v, want UnsupportedTickerError", err)
		}
	})

	t.Run("server 500: returns generic error", func(t *testing.T) {
		srv := newYahooTestServer(t, http.StatusInternalServerError, `{"chart":{"result":null,"error":null}}`)
		defer srv.Close()

		p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
		_, err := p.FetchQuote("AAPL")
		if err == nil {
			t.Fatal("FetchQuote() expected error")
		}
		var notFound *UnsupportedTickerError
		if errors.As(err, &notFound) {
			t.Errorf("error %v, did not expect UnsupportedTickerError for 500", err)
		}
	})

	t.Run("malformed JSON: returns error", func(t *testing.T) {
		srv := newYahooTestServer(t, http.StatusOK, `not json`)
		defer srv.Close()

		p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
		_, err := p.FetchQuote("AAPL")
		if err == nil {
			t.Fatal("FetchQuote() expected error for malformed JSON")
		}
	})

	t.Run("no closed bars available: returns error", func(t *testing.T) {
		// Single intraday bar, session in progress, no prior bar to fall back to.
		body := fmt.Sprintf(`{
			"chart": {
				"result": [{
					"meta": {
						"currency": "USD",
						"symbol": "NEW",
						"exchangeTimezoneName": "America/New_York",
						"gmtoffset": -14400,
						"regularMarketTime": %d,
						"regularMarketPrice": 50.00,
						"currentTradingPeriod": {"regular": {"start": %d, "end": %d}}
					},
					"timestamp": [%d],
					"indicators": {"quote": [{"close": [50.00]}]}
				}],
				"error": null
			}
		}`, wed0930+90*60, wed0930, wed1600, wed0930)

		srv := newYahooTestServer(t, http.StatusOK, body)
		defer srv.Close()

		now := time.Unix(wed0930+90*60, 0)
		p := NewYahooProvider(
			WithHTTPClient(srv.Client()),
			WithBaseURL(srv.URL),
			WithClock(func() time.Time { return now }),
		)

		_, err := p.FetchQuote("NEW")
		if err == nil {
			t.Fatal("FetchQuote() expected error when no closed bar available")
		}
	})

	t.Run("request URL includes ticker and 5d range", func(t *testing.T) {
		var capturedPath, capturedQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"chart":{"result":[],"error":null}}`))
		}))
		defer srv.Close()

		p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
		_, _ = p.FetchQuote("AAPL")

		if capturedPath != "/v8/finance/chart/AAPL" {
			t.Errorf("path = %q, want /v8/finance/chart/AAPL", capturedPath)
		}
		if !contains(capturedQuery, "interval=1d") || !contains(capturedQuery, "range=5d") {
			t.Errorf("query = %q, want interval=1d and range=5d", capturedQuery)
		}
	})
}

func TestYahooProvider_FetchQuote_TickerEscaping(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chart":{"result":[],"error":null}}`))
	}))
	defer srv.Close()

	p := NewYahooProvider(WithHTTPClient(srv.Client()), WithBaseURL(srv.URL))
	_, _ = p.FetchQuote("BRK.B")

	if capturedPath != "/v8/finance/chart/BRK.B" {
		t.Errorf("path = %q, want /v8/finance/chart/BRK.B", capturedPath)
	}
}

// =============================================================================
// helpers
// =============================================================================

func newYahooTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
