package price

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

const (
	yahooDefaultBaseURL = "https://query1.finance.yahoo.com"
	yahooDefaultTimeout = 10 * time.Second
	yahooUserAgent      = "tmoney/1.0 (https://github.com/haskovec/tmoney)"
)

// YahooProvider fetches end-of-day prices from Yahoo Finance's public
// chart endpoint (query1.finance.yahoo.com). The endpoint is unauthenticated
// and undocumented; the provider parses strictly so a schema drift fails
// loudly rather than silently producing bad prices.
//
// FetchQuote always returns the most recent CLOSED session: if the current
// regular session is still in progress, it falls back to the previous bar.
type YahooProvider struct {
	httpClient *http.Client
	baseURL    string
	now        func() time.Time
}

// YahooOption configures a YahooProvider.
type YahooOption func(*YahooProvider)

// WithHTTPClient sets a custom HTTP client (useful for tests).
func WithHTTPClient(c *http.Client) YahooOption {
	return func(p *YahooProvider) { p.httpClient = c }
}

// WithBaseURL overrides the upstream base URL (useful for tests).
func WithBaseURL(u string) YahooOption {
	return func(p *YahooProvider) { p.baseURL = u }
}

// WithClock overrides the clock used to determine if today's session is
// closed (useful for tests).
func WithClock(f func() time.Time) YahooOption {
	return func(p *YahooProvider) { p.now = f }
}

// NewYahooProvider constructs a YahooProvider with sensible defaults.
func NewYahooProvider(opts ...YahooOption) *YahooProvider {
	p := &YahooProvider{
		httpClient: &http.Client{Timeout: yahooDefaultTimeout},
		baseURL:    yahooDefaultBaseURL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns "yahoo".
func (p *YahooProvider) Name() string {
	return "yahoo"
}

// FetchQuote returns the most recent closed-session quote for ticker.
func (p *YahooProvider) FetchQuote(ticker string) (*Quote, error) {
	endpoint := fmt.Sprintf("%s/v8/finance/chart/%s", p.baseURL, ticker)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("yahoo: build request: %w", err)
	}
	q := url.Values{}
	q.Set("interval", "1d")
	q.Set("range", "5d")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", yahooUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo: request %s: %w", ticker, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("yahoo: read response: %w", err)
	}

	// 404 usually means unknown ticker; surface a typed error so the
	// orchestrator can label it cleanly. Other 5xx/4xx codes propagate
	// as generic errors so they can be retried in the future.
	if resp.StatusCode == http.StatusNotFound {
		return nil, &UnsupportedTickerError{Ticker: ticker, Detail: parseYahooErrorDetail(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("yahoo: %s: HTTP %d", ticker, resp.StatusCode)
	}

	var parsed yahooResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("yahoo: parse response for %s: %w", ticker, err)
	}

	if parsed.Chart.Error != nil {
		return nil, &UnsupportedTickerError{Ticker: ticker, Detail: parsed.Chart.Error.Description}
	}
	if len(parsed.Chart.Result) == 0 {
		return nil, &UnsupportedTickerError{Ticker: ticker, Detail: "no result"}
	}
	result := parsed.Chart.Result[0]

	loc, err := time.LoadLocation(result.Meta.ExchangeTimezoneName)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	bar, err := pickClosedBar(result, p.now().In(loc), loc)
	if err != nil {
		return nil, fmt.Errorf("yahoo: %s: %w", ticker, err)
	}

	priceStr := strconv.FormatFloat(bar.close, 'f', -1, 64)
	priceMoney, err := types.NewMoney(priceStr)
	if err != nil {
		return nil, fmt.Errorf("yahoo: %s: parse price %q: %w", ticker, priceStr, err)
	}

	return &Quote{
		Date:     bar.date,
		Price:    priceMoney,
		Currency: result.Meta.Currency,
	}, nil
}

// closedBar is an internal struct holding the chosen bar's date and close.
type closedBar struct {
	date  types.Date
	close float64
}

// pickClosedBar walks the timestamp/close arrays from the end and returns
// the most recent bar that is guaranteed to be closed.
//
// A bar is closed when either:
//   - its date in the exchange tz is strictly before today's date there, or
//   - its date is today and now >= currentTradingPeriod.regular.end.
//
// Bars whose close value is null/missing are skipped.
func pickClosedBar(r yahooResult, nowInLoc time.Time, loc *time.Location) (closedBar, error) {
	if len(r.Indicators.Quote) == 0 {
		return closedBar{}, fmt.Errorf("no quote indicators")
	}
	closes := r.Indicators.Quote[0].Close
	if len(r.Timestamp) == 0 {
		return closedBar{}, fmt.Errorf("no timestamps")
	}

	todayY, todayM, todayD := nowInLoc.Date()

	for i := len(r.Timestamp) - 1; i >= 0; i-- {
		if i >= len(closes) || closes[i] == nil {
			continue
		}
		ts := time.Unix(r.Timestamp[i], 0).In(loc)
		barY, barM, barD := ts.Date()
		barDay := time.Date(barY, barM, barD, 0, 0, 0, 0, loc)
		todayDay := time.Date(todayY, todayM, todayD, 0, 0, 0, 0, loc)

		if barDay.Before(todayDay) {
			return closedBar{
				date:  types.NewDate(barY, barM, barD),
				close: *closes[i],
			}, nil
		}
		if barDay.Equal(todayDay) && nowInLoc.Unix() >= r.Meta.CurrentTradingPeriod.Regular.End {
			return closedBar{
				date:  types.NewDate(barY, barM, barD),
				close: *closes[i],
			}, nil
		}
	}
	return closedBar{}, fmt.Errorf("no closed session found in response")
}

// parseYahooErrorDetail extracts the human-readable message from a Yahoo
// 404 body, falling back to the body when it isn't valid JSON.
func parseYahooErrorDetail(body []byte) string {
	var parsed yahooResponse
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Chart.Error != nil {
		return parsed.Chart.Error.Description
	}
	return ""
}

// =============================================================================
// Yahoo response schema (subset)
// =============================================================================

type yahooResponse struct {
	Chart struct {
		Result []yahooResult `json:"result"`
		Error  *yahooError   `json:"error"`
	} `json:"chart"`
}

type yahooResult struct {
	Meta       yahooMeta       `json:"meta"`
	Timestamp  []int64         `json:"timestamp"`
	Indicators yahooIndicators `json:"indicators"`
}

type yahooMeta struct {
	Currency             string                `json:"currency"`
	Symbol               string                `json:"symbol"`
	ExchangeTimezoneName string                `json:"exchangeTimezoneName"`
	GMTOffset            int                   `json:"gmtoffset"`
	RegularMarketTime    int64                 `json:"regularMarketTime"`
	RegularMarketPrice   float64               `json:"regularMarketPrice"`
	CurrentTradingPeriod yahooTradingPeriodSet `json:"currentTradingPeriod"`
}

type yahooTradingPeriodSet struct {
	Regular yahooTradingPeriod `json:"regular"`
}

type yahooTradingPeriod struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type yahooIndicators struct {
	Quote []yahooQuoteSeries `json:"quote"`
}

type yahooQuoteSeries struct {
	Close []*float64 `json:"close"`
}

type yahooError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}
