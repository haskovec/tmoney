package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// fakeRefreshProvider is a Provider that returns canned quotes for tests.
type fakeRefreshProvider struct {
	quotes map[string]*price.Quote
	errors map[string]error
}

func (f *fakeRefreshProvider) FetchQuote(ticker string) (*price.Quote, error) {
	if err, ok := f.errors[ticker]; ok {
		return nil, err
	}
	if q, ok := f.quotes[ticker]; ok {
		return q, nil
	}
	return nil, &price.UnsupportedTickerError{Ticker: ticker}
}

func (f *fakeRefreshProvider) Name() string { return defaultRefreshProviderName }

// setupRefreshTUITest creates a temp DB seeded with the given tickers,
// builds the price/security services from it, and registers a fake
// provider under the "yahoo" name so refreshPricesCmd uses it.
// Returns the App, the fake provider for further configuration, and a
// list of created securities.
func setupRefreshTUITest(t *testing.T, tickers ...string) (*App, *fakeRefreshProvider, []*security.Security) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tui-refresh.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	secRepo := security.NewRepository(database)
	priceRepo := price.NewRepository(database)
	priceSvc := price.NewService(priceRepo, secRepo, database)
	priceSvc.SetRefreshSleep(0)

	fp := &fakeRefreshProvider{
		quotes: make(map[string]*price.Quote),
		errors: make(map[string]error),
	}
	priceSvc.ProviderRegistry().Register(fp)

	var seeded []*security.Security
	for _, tk := range tickers {
		sec := security.NewSecurity(tk, tk+" Inc.", security.TypeStock)
		if err := secRepo.Create(sec); err != nil {
			t.Fatalf("create security %s: %v", tk, err)
		}
		seeded = append(seeded, sec)
	}

	a := &App{
		width:     100,
		height:    30,
		keys:      defaultKeyMap(),
		statusbar: NewStatusBar(),
		styles:    NewStyles(),
		priceSvc:  priceSvc,
	}
	return a, fp, seeded
}

func quoteUSD(date, p string) *price.Quote {
	return &price.Quote{
		Date:     types.MustParseDate(date),
		Price:    types.MustNewMoney(p),
		Currency: "USD",
	}
}

func TestRefreshPricesCmd_DispatchesAndPersists(t *testing.T) {
	a, fp, secs := setupRefreshTUITest(t, "AAPL", "MSFT")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")
	fp.quotes["MSFT"] = quoteUSD("2026-04-22", "415.00")

	cmd := a.refreshPricesCmd()
	if cmd == nil {
		t.Fatal("refreshPricesCmd returned nil")
	}
	msg := cmd()
	completed, ok := msg.(priceRefreshCompleteMsg)
	if !ok {
		t.Fatalf("msg type = %T, want priceRefreshCompleteMsg", msg)
	}
	if completed.err != nil {
		t.Fatalf("unexpected err: %v", completed.err)
	}
	counts := completed.result.CountByOutcome()
	if counts[price.OutcomeUpdated] != 2 {
		t.Errorf("updated = %d, want 2", counts[price.OutcomeUpdated])
	}

	// Verify both prices are now in the DB.
	for _, sec := range secs {
		stored, err := a.priceSvc.GetCurrentPrice(sec.ID, types.Today())
		if err != nil {
			t.Errorf("GetCurrentPrice(%s): %v", sec.Ticker, err)
			continue
		}
		if stored.Source != price.SourceAPI {
			t.Errorf("%s source = %q, want %q", sec.Ticker, stored.Source, price.SourceAPI)
		}
	}
}

func TestRefreshPricesCmd_NilService(t *testing.T) {
	a := &App{}
	msg := a.refreshPricesCmd()()
	completed, ok := msg.(priceRefreshCompleteMsg)
	if !ok {
		t.Fatalf("msg type = %T, want priceRefreshCompleteMsg", msg)
	}
	if completed.err == nil {
		t.Errorf("expected err for nil price service")
	}
}

func TestHandleSecurityViewKeys_UTriggersRefresh(t *testing.T) {
	a, fp, _ := setupRefreshTUITest(t, "AAPL")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	// Wire up the security view so the handler can run.
	a.securityView = &securityViewData{
		securities: []*security.Security{},
		showHidden: false,
	}
	a.buildSecurityTable()

	uKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}
	_, cmd := a.handleSecurityViewKeys(uKey)
	if cmd == nil {
		t.Fatal("u keypress returned nil cmd")
	}
	msg := cmd()
	completed, ok := msg.(priceRefreshCompleteMsg)
	if !ok {
		t.Fatalf("msg type = %T, want priceRefreshCompleteMsg", msg)
	}
	if completed.err != nil {
		t.Errorf("unexpected err: %v", completed.err)
	}
	counts := completed.result.CountByOutcome()
	if counts[price.OutcomeUpdated] != 1 {
		t.Errorf("updated = %d, want 1", counts[price.OutcomeUpdated])
	}
}

func TestSummarizeRefreshResult_AllUpdated(t *testing.T) {
	r := &price.RefreshResult{Entries: []price.RefreshEntry{
		{Ticker: "AAPL", Outcome: price.OutcomeUpdated},
		{Ticker: "MSFT", Outcome: price.OutcomeUpdated},
	}}
	got := summarizeRefreshResult(r)
	if !strings.Contains(got, "2 updated") {
		t.Errorf("summary = %q, want '2 updated'", got)
	}
	if !strings.Contains(got, "0 failed") {
		t.Errorf("summary = %q, want '0 failed'", got)
	}
}

func TestSummarizeRefreshResult_ListsFailures(t *testing.T) {
	r := &price.RefreshResult{Entries: []price.RefreshEntry{
		{Ticker: "AAPL", Outcome: price.OutcomeUpdated},
		{Ticker: "BAD1", Outcome: price.OutcomeFailed, Error: errors.New("nope")},
		{Ticker: "BAD2", Outcome: price.OutcomeFailed, Error: errors.New("nope")},
		{Ticker: "BAD3", Outcome: price.OutcomeFailed, Error: errors.New("nope")},
		{Ticker: "BAD4", Outcome: price.OutcomeFailed, Error: errors.New("nope")},
	}}
	got := summarizeRefreshResult(r)
	if !strings.Contains(got, "BAD1") || !strings.Contains(got, "BAD2") || !strings.Contains(got, "BAD3") {
		t.Errorf("summary should include first three failures: %s", got)
	}
	if !strings.Contains(got, "...") {
		t.Errorf("summary should indicate truncation when >3 failures: %s", got)
	}
}

func TestRefreshCompleteMsg_AddsStatusBarNotification(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t)
	a.currentView = ViewSecurities

	result := &price.RefreshResult{Entries: []price.RefreshEntry{
		{Ticker: "AAPL", Outcome: price.OutcomeUpdated},
	}}
	msg := priceRefreshCompleteMsg{result: result}

	a.Update(msg)
	notifs := a.statusbar.Notifications()
	if len(notifs) == 0 {
		t.Fatal("expected a status bar notification, got none")
	}
	if !strings.Contains(notifs[0].Text, "1 updated") {
		t.Errorf("notification = %q, want to contain '1 updated'", notifs[0].Text)
	}
}
