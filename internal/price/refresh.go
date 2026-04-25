package price

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// RefreshOutcome labels the per-security result of a RefreshPrices run.
type RefreshOutcome string

const (
	OutcomeUpdated         RefreshOutcome = "updated"
	OutcomeUpToDate        RefreshOutcome = "up_to_date"
	OutcomeSkippedHidden   RefreshOutcome = "skipped_hidden"
	OutcomeSkippedNoTicker RefreshOutcome = "skipped_no_ticker"
	OutcomeSkippedCurrency RefreshOutcome = "skipped_currency_mismatch"
	OutcomeFailed          RefreshOutcome = "failed"
)

// RefreshEntry is the per-security outcome of a RefreshPrices call.
// Date and Price are populated for OutcomeUpdated and OutcomeUpToDate.
// Error is populated for OutcomeFailed.
// Note carries human-readable detail (e.g., "USD vs EUR" for currency
// mismatches, security name for no-ticker skips).
type RefreshEntry struct {
	Ticker  string
	Outcome RefreshOutcome
	Date    types.Date
	Price   types.Money
	Error   error
	Note    string
}

// RefreshResult aggregates the outcomes of a RefreshPrices call.
type RefreshResult struct {
	Entries []RefreshEntry
}

// CountByOutcome returns counts grouped by outcome.
func (r *RefreshResult) CountByOutcome() map[RefreshOutcome]int {
	counts := make(map[RefreshOutcome]int)
	for _, e := range r.Entries {
		counts[e.Outcome]++
	}
	return counts
}

// HasFailures reports whether any entry failed.
func (r *RefreshResult) HasFailures() bool {
	for _, e := range r.Entries {
		if e.Outcome == OutcomeFailed {
			return true
		}
	}
	return false
}

// SetRefreshSleep sets the polite delay between consecutive provider
// requests during RefreshPrices. Defaults to 200ms; pass 0 to disable
// (used in tests).
func (s *Service) SetRefreshSleep(d time.Duration) {
	s.refreshSleep = d
}

// RefreshPrices fetches the latest closed-session price from the named
// provider for each visible security with a ticker, persisting any new
// quote that isn't already on file.
//
// If tickers is empty, all visible securities are refreshed. If tickers
// is non-empty, only matching securities are refreshed (case-insensitive).
//
// Per-security failures are recorded in the result and do not abort the
// run. The only fatal errors are unknown providers and database listing
// failures.
func (s *Service) RefreshPrices(providerName string, tickers []string) (*RefreshResult, error) {
	provider, err := s.registry.Get(providerName)
	if err != nil {
		return nil, err
	}

	securities, err := s.secRepo.List(security.Filter{})
	if err != nil {
		return nil, fmt.Errorf("list securities: %w", err)
	}

	tickerFilter := normalizeTickerFilter(tickers)
	today := types.Today()
	result := &RefreshResult{}
	fetchCount := 0

	for _, sec := range securities {
		if tickerFilter != nil && !tickerFilter[strings.ToUpper(sec.Ticker)] {
			continue
		}
		if sec.Ticker == "" {
			result.Entries = append(result.Entries, RefreshEntry{
				Outcome: OutcomeSkippedNoTicker,
				Note:    sec.Name,
			})
			continue
		}
		if sec.Hidden {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeSkippedHidden,
			})
			continue
		}

		// Polite delay between provider requests.
		if fetchCount > 0 && s.refreshSleep > 0 {
			time.Sleep(s.refreshSleep)
		}
		fetchCount++

		quote, err := provider.FetchQuote(sec.Ticker)
		if err != nil {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeFailed,
				Error:   err,
			})
			continue
		}
		if quote.Currency != "" && !strings.EqualFold(quote.Currency, sec.Currency) {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeSkippedCurrency,
				Note:    fmt.Sprintf("provider %s vs security %s", quote.Currency, sec.Currency),
			})
			continue
		}

		// Skip if we already stored a price for the same date.
		if existing, err := s.repo.GetCurrentPrice(sec.ID, today); err == nil && existing.Date.Equal(quote.Date) {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeUpToDate,
				Date:    quote.Date,
				Price:   existing.Price,
			})
			continue
		} else if err != nil {
			var notFound *dberrors.NotFoundError
			if !errors.As(err, &notFound) {
				result.Entries = append(result.Entries, RefreshEntry{
					Ticker:  sec.Ticker,
					Outcome: OutcomeFailed,
					Error:   fmt.Errorf("check existing price: %w", err),
				})
				continue
			}
		}

		priceRecord := NewPrice(sec.ID, quote.Date, quote.Price, SourceAPI)
		if errs := priceRecord.Validate(); errs.HasErrors() {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeFailed,
				Error:   fmt.Errorf("invalid quote: %s", errs.Error()),
			})
			continue
		}
		if err := s.repo.CreateOrUpdate(priceRecord); err != nil {
			result.Entries = append(result.Entries, RefreshEntry{
				Ticker:  sec.Ticker,
				Outcome: OutcomeFailed,
				Error:   fmt.Errorf("save price: %w", err),
			})
			continue
		}
		result.Entries = append(result.Entries, RefreshEntry{
			Ticker:  sec.Ticker,
			Outcome: OutcomeUpdated,
			Date:    quote.Date,
			Price:   quote.Price,
		})
	}
	return result, nil
}

// normalizeTickerFilter returns a case-insensitive set of tickers, or nil
// if the input is empty (meaning "no filter — all securities").
func normalizeTickerFilter(tickers []string) map[string]bool {
	if len(tickers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(tickers))
	for _, t := range tickers {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t != "" {
			set[t] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
