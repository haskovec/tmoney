package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
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
		statusbar: widget.NewStatusBar(),
		styles:    widget.NewStyles(),
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

	uKey := tea.KeyPressMsg{Code: 'u', Text: "u"}
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

func TestHandlePriceListKeys_UTriggersRefresh(t *testing.T) {
	a, fp, _ := setupRefreshTUITest(t, "AAPL")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	a.priceView = &priceViewData{mode: pricesViewList}
	a.buildPriceListTable()

	uKey := tea.KeyPressMsg{Code: 'u', Text: "u"}
	_, cmd := a.handlePriceViewKeys(uKey)
	if cmd == nil {
		t.Fatal("u keypress on prices list returned nil cmd")
	}
	completed, ok := cmd().(priceRefreshCompleteMsg)
	if !ok {
		t.Fatalf("msg type = %T, want priceRefreshCompleteMsg", cmd())
	}
	if completed.err != nil {
		t.Errorf("unexpected err: %v", completed.err)
	}
	if completed.result.CountByOutcome()[price.OutcomeUpdated] != 1 {
		t.Errorf("updated count = %d, want 1", completed.result.CountByOutcome()[price.OutcomeUpdated])
	}
}

func TestHandlePriceDetailKeys_UTriggersRefresh(t *testing.T) {
	a, fp, secs := setupRefreshTUITest(t, "AAPL")
	fp.quotes["AAPL"] = quoteUSD("2026-04-22", "271.06")

	a.priceView = &priceViewData{
		mode:             pricesViewDetail,
		selectedSecurity: secs[0],
	}
	a.buildPriceTable()

	uKey := tea.KeyPressMsg{Code: 'u', Text: "u"}
	_, cmd := a.handlePriceViewKeys(uKey)
	if cmd == nil {
		t.Fatal("u keypress on prices detail returned nil cmd")
	}
	completed, ok := cmd().(priceRefreshCompleteMsg)
	if !ok {
		t.Fatalf("msg type = %T, want priceRefreshCompleteMsg", cmd())
	}
	if completed.err != nil {
		t.Errorf("unexpected err: %v", completed.err)
	}
	if completed.result.CountByOutcome()[price.OutcomeUpdated] != 1 {
		t.Errorf("updated count = %d, want 1", completed.result.CountByOutcome()[price.OutcomeUpdated])
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

// TestNewApp_RegistersYahooProvider verifies the production NewApp path
// wires the yahoo price provider into the registry so the securities-view
// "u" shortcut can find it.
func TestNewApp_RegistersYahooProvider(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newapp.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	a := NewApp(database, nil)
	if _, err := a.priceSvc.ProviderRegistry().Get(defaultRefreshProviderName); err != nil {
		t.Errorf("yahoo provider missing after NewApp: %v", err)
	}
}

// TestSwitchDatabase_RegistersYahooProvider catches the regression where
// opening a different file via File > Open / Open Recent rebuilt the
// price service but forgot to register yahoo, causing the "u" shortcut
// to fail with `price provider "yahoo" not found`.
func TestSwitchDatabase_RegistersYahooProvider(t *testing.T) {
	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "first.tdb")
	secondPath := filepath.Join(tmpDir, "second.tdb")

	firstDB, err := db.Create(firstPath)
	if err != nil {
		t.Fatalf("db.Create first: %v", err)
	}
	t.Cleanup(func() { firstDB.Close() })

	a := NewApp(firstDB, nil)

	secondDB, err := db.Create(secondPath)
	if err != nil {
		t.Fatalf("db.Create second: %v", err)
	}
	t.Cleanup(func() {
		if a.prevDB != nil {
			_ = a.prevDB.Close()
		}
		secondDB.Close()
	})

	a.switchDatabase(secondDB)

	if _, err := a.priceSvc.ProviderRegistry().Get(defaultRefreshProviderName); err != nil {
		t.Errorf("yahoo provider missing after switchDatabase: %v", err)
	}
}

// TestRefreshCompleteMsg_ClearsHistoryCache pins PC-016: a bulk refresh
// can silently update prices for any number of tickers, so the per-row
// chart cache must drop every entry on completion. A surgical evict
// won't do — the refresh result doesn't carry the list of securities
// whose prices actually changed; Clear() is the only correct response.
func TestRefreshCompleteMsg_ClearsHistoryCache(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t)
	a.currentView = ViewPrices

	secA := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	secB := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)
	cache := newHistoryCache()
	cache.Put(secA.ID, []*price.Price{
		price.NewPrice(secA.ID, types.NewDate(2026, time.April, 15), types.NewMoneyFromFloat(180.00), price.SourceManual),
	})
	cache.Put(secB.ID, []*price.Price{
		price.NewPrice(secB.ID, types.NewDate(2026, time.April, 15), types.NewMoneyFromFloat(420.00), price.SourceManual),
	})
	a.priceView = &priceViewData{
		mode:         pricesViewList,
		historyCache: cache,
	}

	result := &price.RefreshResult{Entries: []price.RefreshEntry{
		{Ticker: "AAPL", Outcome: price.OutcomeUpdated},
	}}
	a.Update(priceRefreshCompleteMsg{result: result})

	if _, ok := a.priceView.historyCache.Lookup(secA.ID); ok {
		t.Errorf("priceRefreshCompleteMsg should clear cache entry for %v (AAPL)", secA.ID)
	}
	if _, ok := a.priceView.historyCache.Lookup(secB.ID); ok {
		t.Errorf("priceRefreshCompleteMsg should clear cache entry for %v (MSFT)", secB.ID)
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

// TestStartPriceRefresh_SetsInProgressNotification asserts startPriceRefresh
// parks an "Updating prices…" notification in the status bar before
// dispatching the refresh, so a long-running run gives the user immediate
// feedback that the keystroke was received.
func TestStartPriceRefresh_SetsInProgressNotification(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t, "AAPL")

	cmd := a.startPriceRefresh()
	if cmd == nil {
		t.Fatal("startPriceRefresh returned nil cmd on first call")
	}

	if !a.refreshingPrices {
		t.Error("refreshingPrices flag not set after startPriceRefresh")
	}
	if a.refreshNotifID == 0 {
		t.Error("refreshNotifID not stored after startPriceRefresh")
	}

	notifs := a.statusbar.Notifications()
	found := false
	for _, n := range notifs {
		if n.Text == "Updating prices…" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("status bar missing 'Updating prices…' notification; got %+v", notifs)
	}
}

// TestStartPriceRefresh_SecondCallNoOp asserts a re-press of `u` while
// a refresh is in flight returns nil and doesn't stack notifications or
// dispatch a parallel refresh.
func TestStartPriceRefresh_SecondCallNoOp(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t, "AAPL")

	if cmd := a.startPriceRefresh(); cmd == nil {
		t.Fatal("first startPriceRefresh returned nil")
	}
	notifsBefore := len(a.statusbar.Notifications())

	if cmd := a.startPriceRefresh(); cmd != nil {
		t.Errorf("second startPriceRefresh while in flight returned non-nil cmd %v", cmd)
	}
	if got := len(a.statusbar.Notifications()); got != notifsBefore {
		t.Errorf("second startPriceRefresh changed notification count from %d to %d, want no-op",
			notifsBefore, got)
	}
}

// TestRefreshCompleteMsg_RemovesInProgressNotification asserts the
// completion handler retires the "Updating prices…" entry, replaces it
// with the summary, and resets refreshingPrices so a subsequent `u`
// works.
func TestRefreshCompleteMsg_RemovesInProgressNotification(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t)
	a.currentView = ViewSecurities

	a.startPriceRefresh()

	result := &price.RefreshResult{Entries: []price.RefreshEntry{
		{Ticker: "AAPL", Outcome: price.OutcomeUpdated},
	}}
	a.Update(priceRefreshCompleteMsg{result: result})

	for _, n := range a.statusbar.Notifications() {
		if n.Text == "Updating prices…" {
			t.Errorf("'Updating prices…' notification not removed after completion")
		}
	}
	if a.refreshingPrices {
		t.Error("refreshingPrices still true after completion")
	}
	if a.refreshNotifID != 0 {
		t.Errorf("refreshNotifID = %d after completion, want 0", a.refreshNotifID)
	}

	if cmd := a.startPriceRefresh(); cmd == nil {
		t.Error("startPriceRefresh after completion returned nil; guard should be cleared")
	}
}

// TestRefreshCompleteMsg_OnError_RemovesNotificationAndSetsErr asserts
// the in-progress notification is removed and the flag cleared even on
// the error branch (so the user can retry after dismissing the error
// dialog), and that no summary is added.
func TestRefreshCompleteMsg_OnError_RemovesNotificationAndSetsErr(t *testing.T) {
	a, _, _ := setupRefreshTUITest(t)
	a.currentView = ViewSecurities

	a.startPriceRefresh()

	a.Update(priceRefreshCompleteMsg{err: errors.New("provider blew up")})

	for _, n := range a.statusbar.Notifications() {
		if n.Text == "Updating prices…" {
			t.Errorf("'Updating prices…' notification not removed on error path")
		}
		if strings.Contains(n.Text, "updated") {
			t.Errorf("error path should not add the summary notification, got %q", n.Text)
		}
	}
	if a.refreshingPrices {
		t.Error("refreshingPrices still true on error path")
	}
	if a.err == nil {
		t.Error("a.err not set on error path")
	}
}
