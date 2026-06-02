package price

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// fake Provider for orchestrator tests
// =============================================================================

type fakeProvider struct {
	name      string
	quotes    map[string]*Quote
	errors    map[string]error
	calls     []string
	callTimes []time.Time
}

func (f *fakeProvider) FetchQuote(ticker string) (*Quote, error) {
	f.calls = append(f.calls, ticker)
	f.callTimes = append(f.callTimes, time.Now())
	if err, ok := f.errors[ticker]; ok {
		return nil, err
	}
	if q, ok := f.quotes[ticker]; ok {
		return q, nil
	}
	return nil, &UnsupportedTickerError{Ticker: ticker}
}

func (f *fakeProvider) FetchQuoteOn(ticker string, _ types.Date) (*Quote, error) {
	return f.FetchQuote(ticker)
}

func (f *fakeProvider) Name() string { return f.name }

func newFakeProvider(name string) *fakeProvider {
	return &fakeProvider{
		name:   name,
		quotes: make(map[string]*Quote),
		errors: make(map[string]error),
	}
}

// setupRefreshTest builds a service with a registered fake provider and
// disables the polite delay so tests run fast.
func setupRefreshTest(t *testing.T) (*Service, *fakeProvider, *security.Repository) {
	t.Helper()
	database := createTestDB(t)
	secRepo := security.NewRepository(database)
	priceRepo := NewRepository(database)
	svc := NewService(priceRepo, secRepo, database)
	svc.SetRefreshSleep(0)
	fp := newFakeProvider("fake")
	svc.ProviderRegistry().Register(fp)
	return svc, fp, secRepo
}

func mustCreateSecurity(t *testing.T, repo *security.Repository, ticker, currency string) *security.Security {
	t.Helper()
	sec := security.NewSecurity(ticker, ticker+" Inc.", security.TypeStock)
	sec.Currency = currency
	if err := repo.Create(sec); err != nil {
		t.Fatalf("create security %s: %v", ticker, err)
	}
	return sec
}

func quoteUSD(date string, price string) *Quote {
	return &Quote{
		Date:     types.MustParseDate(date),
		Price:    types.MustNewMoney(price),
		Currency: "USD",
	}
}

// =============================================================================
// Service.RefreshPrices
// =============================================================================

func TestService_RefreshPrices_UpdatesPriceForSecurity(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	sec := mustCreateSecurity(t, secRepo, "AAPL", "USD")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	entry := result.Entries[0]
	if entry.Outcome != OutcomeUpdated {
		t.Errorf("Outcome = %q, want %q", entry.Outcome, OutcomeUpdated)
	}
	if entry.Ticker != "AAPL" {
		t.Errorf("Ticker = %q, want AAPL", entry.Ticker)
	}
	if entry.Date.String() != "2026-04-22" {
		t.Errorf("Date = %q, want 2026-04-22", entry.Date.String())
	}
	if entry.Price.String() != "271.06" {
		t.Errorf("Price = %q, want 271.06", entry.Price.String())
	}

	// Verify the price was actually written.
	stored, err := svc.GetCurrentPrice(sec.ID, types.MustParseDate("2026-04-22"))
	if err != nil {
		t.Fatalf("GetCurrentPrice error = %v", err)
	}
	if stored.Source != SourceAPI {
		t.Errorf("Source = %q, want %q", stored.Source, SourceAPI)
	}
	if stored.Price.String() != "271.06" {
		t.Errorf("stored price = %q, want 271.06", stored.Price.String())
	}
}

func TestService_RefreshPrices_SkipsHiddenSecurity(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	sec := mustCreateSecurity(t, secRepo, "OLD", "USD")
	sec.Hide()
	if err := secRepo.Update(sec); err != nil {
		t.Fatalf("hide security: %v", err)
	}
	fp.quotes["OLD"] = quoteUSD("2026-04-22", "1.00")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Outcome != OutcomeSkippedHidden {
		t.Errorf("Outcome = %q, want %q", result.Entries[0].Outcome, OutcomeSkippedHidden)
	}
	if len(fp.calls) != 0 {
		t.Errorf("provider was called %d times for hidden security; want 0", len(fp.calls))
	}
}

func TestService_RefreshPrices_SkipsNoTicker(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)

	// Create a security and then null out the ticker via a direct DB edit
	// since NewSecurity validates non-empty. Easier: mark it hidden so we
	// don't touch the schema; but we need a *no-ticker* case. We'll
	// simulate it by clearing ticker in memory then upserting via repo.
	// In practice the security model rejects empty tickers via Validate(),
	// but the repository Create check is simpler: it just stores what's
	// passed. We bypass NewSecurity validation by constructing the struct
	// directly.
	sec := &security.Security{
		BaseModel:    types.NewBaseModel(),
		Ticker:       "",
		Name:         "Cash",
		SecurityType: security.TypeOther,
		AssetClass:   security.AssetClassCash,
		Currency:     "USD",
	}
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("create no-ticker security: %v", err)
	}

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Outcome != OutcomeSkippedNoTicker {
		t.Errorf("Outcome = %q, want %q", result.Entries[0].Outcome, OutcomeSkippedNoTicker)
	}
	if len(fp.calls) != 0 {
		t.Errorf("provider was called %d times; want 0", len(fp.calls))
	}
}

func TestService_RefreshPrices_SkipsCurrencyMismatch(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	mustCreateSecurity(t, secRepo, "FOO", "EUR")
	// Provider returns USD but security is EUR.
	fp.quotes["FOO"] = quoteUSD("2026-04-22", "100.00")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	entry := result.Entries[0]
	if entry.Outcome != OutcomeSkippedCurrency {
		t.Errorf("Outcome = %q, want %q", entry.Outcome, OutcomeSkippedCurrency)
	}
	if entry.Note == "" {
		t.Errorf("Note expected to describe currency mismatch")
	}
}

func TestService_RefreshPrices_UpToDateWhenSameDateExists(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	sec := mustCreateSecurity(t, secRepo, "AAPL", "USD")
	// Prior price already stored for the same date the provider will return.
	existing := NewPrice(sec.ID, types.MustParseDate("2026-04-22"), types.MustNewMoney("271.06"), SourceAPI)
	if err := svc.repo.Create(existing); err != nil {
		t.Fatalf("seed existing price: %v", err)
	}
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Outcome != OutcomeUpToDate {
		t.Errorf("Outcome = %q, want %q", result.Entries[0].Outcome, OutcomeUpToDate)
	}
}

func TestService_RefreshPrices_OverwritesOlderPriceWhenProviderHasNewer(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	sec := mustCreateSecurity(t, secRepo, "AAPL", "USD")
	older := NewPrice(sec.ID, types.MustParseDate("2026-04-15"), types.MustNewMoney("260.00"), SourceManual)
	if err := svc.repo.Create(older); err != nil {
		t.Fatalf("seed older price: %v", err)
	}
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if result.Entries[0].Outcome != OutcomeUpdated {
		t.Errorf("Outcome = %q, want updated", result.Entries[0].Outcome)
	}

	stored, err := svc.GetCurrentPrice(sec.ID, types.MustParseDate("2026-04-22"))
	if err != nil {
		t.Fatalf("GetCurrentPrice: %v", err)
	}
	if stored.Date.String() != "2026-04-22" {
		t.Errorf("latest stored date = %q, want 2026-04-22", stored.Date.String())
	}
}

func TestService_RefreshPrices_ContinuesAfterPerTickerError(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	mustCreateSecurity(t, secRepo, "AAPL", "USD")
	mustCreateSecurity(t, secRepo, "BADTICK", "USD")
	mustCreateSecurity(t, secRepo, "MSFT", "USD")

	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")
	fp.errors["BADTICK"] = &UnsupportedTickerError{Ticker: "BADTICK", Detail: "not found"}
	fp.quotes["MSFT"] = quoteUSD("2026-04-22", "415.00")

	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(result.Entries))
	}

	byTicker := make(map[string]RefreshEntry)
	for _, e := range result.Entries {
		byTicker[e.Ticker] = e
	}
	if byTicker["AAPL"].Outcome != OutcomeUpdated {
		t.Errorf("AAPL outcome = %q, want updated", byTicker["AAPL"].Outcome)
	}
	if byTicker["BADTICK"].Outcome != OutcomeFailed {
		t.Errorf("BADTICK outcome = %q, want failed", byTicker["BADTICK"].Outcome)
	}
	if byTicker["BADTICK"].Error == nil {
		t.Errorf("BADTICK error not set")
	}
	if byTicker["MSFT"].Outcome != OutcomeUpdated {
		t.Errorf("MSFT outcome = %q, want updated", byTicker["MSFT"].Outcome)
	}
}

func TestService_RefreshPrices_FiltersByExplicitTickers(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	mustCreateSecurity(t, secRepo, "AAPL", "USD")
	mustCreateSecurity(t, secRepo, "MSFT", "USD")
	mustCreateSecurity(t, secRepo, "GOOG", "USD")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")
	fp.quotes["MSFT"] = quoteUSD("2026-04-22", "415.00")
	fp.quotes["GOOG"] = quoteUSD("2026-04-22", "180.00")

	result, err := svc.RefreshPrices("fake", []string{"aapl", "MSFT"})
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(result.Entries))
	}
	tickers := []string{result.Entries[0].Ticker, result.Entries[1].Ticker}
	want := map[string]bool{"AAPL": true, "MSFT": true}
	for _, tk := range tickers {
		if !want[tk] {
			t.Errorf("unexpected ticker %q in result", tk)
		}
	}
	if len(fp.calls) != 2 {
		t.Errorf("provider calls = %d, want 2", len(fp.calls))
	}
}

func TestService_RefreshPrices_ExplicitTickerHonorsHiddenSkip(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	sec := mustCreateSecurity(t, secRepo, "OLD", "USD")
	sec.Hide()
	if err := secRepo.Update(sec); err != nil {
		t.Fatalf("hide: %v", err)
	}
	fp.quotes["OLD"] = quoteUSD("2026-04-22", "1.00")

	result, err := svc.RefreshPrices("fake", []string{"OLD"})
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(result.Entries))
	}
	if result.Entries[0].Outcome != OutcomeSkippedHidden {
		t.Errorf("Outcome = %q, want skipped_hidden", result.Entries[0].Outcome)
	}
}

func TestService_RefreshPrices_ErrorsForUnknownProvider(t *testing.T) {
	svc, _, _ := setupRefreshTest(t)
	_, err := svc.RefreshPrices("nonexistent", nil)
	if err == nil {
		t.Fatal("RefreshPrices expected error for unknown provider")
	}
}

func TestService_RefreshPrices_NoSecurities(t *testing.T) {
	svc, _, _ := setupRefreshTest(t)
	result, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(result.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(result.Entries))
	}
}

func TestService_RefreshPrices_DelaysBetweenFetches(t *testing.T) {
	svc, fp, secRepo := setupRefreshTest(t)
	svc.SetRefreshSleep(15 * time.Millisecond)

	mustCreateSecurity(t, secRepo, "A", "USD")
	mustCreateSecurity(t, secRepo, "B", "USD")
	mustCreateSecurity(t, secRepo, "C", "USD")
	fp.quotes["A"] = quoteUSD("2026-04-22", "1.00")
	fp.quotes["B"] = quoteUSD("2026-04-22", "2.00")
	fp.quotes["C"] = quoteUSD("2026-04-22", "3.00")

	_, err := svc.RefreshPrices("fake", nil)
	if err != nil {
		t.Fatalf("RefreshPrices error = %v", err)
	}
	if len(fp.callTimes) < 2 {
		t.Fatal("expected at least 2 fetches")
	}
	gap := fp.callTimes[1].Sub(fp.callTimes[0])
	if gap < 10*time.Millisecond {
		t.Errorf("gap between fetches = %v, expected >= 10ms", gap)
	}
}

func TestRefreshResult_Helpers(t *testing.T) {
	r := &RefreshResult{Entries: []RefreshEntry{
		{Outcome: OutcomeUpdated},
		{Outcome: OutcomeUpdated},
		{Outcome: OutcomeUpToDate},
		{Outcome: OutcomeSkippedHidden},
		{Outcome: OutcomeFailed, Error: errors.New("boom")},
	}}

	counts := r.CountByOutcome()
	if counts[OutcomeUpdated] != 2 {
		t.Errorf("updated count = %d, want 2", counts[OutcomeUpdated])
	}
	if counts[OutcomeFailed] != 1 {
		t.Errorf("failed count = %d, want 1", counts[OutcomeFailed])
	}
	if !r.HasFailures() {
		t.Errorf("HasFailures() = false, want true")
	}

	r2 := &RefreshResult{Entries: []RefreshEntry{{Outcome: OutcomeUpdated}}}
	if r2.HasFailures() {
		t.Errorf("HasFailures() = true, want false for clean run")
	}
}

// Sanity: verify our fakeProvider satisfies Provider.
func TestFakeProvider_ImplementsInterface(t *testing.T) {
	var _ Provider = newFakeProvider("x")
	// also exercise fmt path so unused import warnings don't appear.
	_ = fmt.Sprintf("%T", &fakeProvider{})
}
