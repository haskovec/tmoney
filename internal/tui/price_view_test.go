package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// SM-123: Price list table component
// =============================================================================

func TestPriceViewData(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	data := &priceViewData{
		securities:       []*security.Security{sec},
		selectedSecurity: sec,
	}

	if data.selectedSecurity.Ticker != "AAPL" {
		t.Errorf("selected security ticker = %q, want AAPL", data.selectedSecurity.Ticker)
	}
	if len(data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(data.securities))
	}
}

func TestPriceViewData_FilteredSecurities(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)
	sec3 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec3.Hidden = true

	data := &priceViewData{
		securities: []*security.Security{sec1, sec2, sec3},
	}

	// Should return all non-hidden by default
	filtered := data.filteredSecurities()
	if len(filtered) != 2 {
		t.Errorf("expected 2 visible securities, got %d", len(filtered))
	}

	// Search by ticker
	data.searchQuery = "aapl"
	filtered = data.filteredSecurities()
	if len(filtered) != 1 {
		t.Errorf("expected 1 match for 'aapl', got %d", len(filtered))
	}
	if filtered[0].Ticker != "AAPL" {
		t.Errorf("expected AAPL, got %s", filtered[0].Ticker)
	}

	// Search by name (case-insensitive)
	data.searchQuery = "micro"
	filtered = data.filteredSecurities()
	if len(filtered) != 1 {
		t.Errorf("expected 1 match for 'micro', got %d", len(filtered))
	}

	// No match
	data.searchQuery = "zzz"
	filtered = data.filteredSecurities()
	if len(filtered) != 0 {
		t.Errorf("expected 0 matches, got %d", len(filtered))
	}
}

func TestFormatPriceRow(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")

	p := price.NewPrice(secID, d, m, price.SourceManual)

	app := &App{}
	row := app.formatPriceRow(p)

	if len(row) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(row))
	}

	if row[0] != "2025-03-15" {
		t.Errorf("date = %q, want %q", row[0], "2025-03-15")
	}
	if row[1] != "$185.50" {
		t.Errorf("price = %q, want %q", row[1], "$185.50")
	}
	if row[2] != "Manual" {
		t.Errorf("source = %q, want %q", row[2], "Manual")
	}
}

func TestFormatPriceRow_Sources(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.January, 1)
	m, _ := types.NewMoney("100.00")

	tests := []struct {
		source   price.Source
		expected string
	}{
		{price.SourceManual, "Manual"},
		{price.SourceTransaction, "Transaction"},
		{price.SourceImport, "Import"},
		{price.SourceAPI, "API"},
	}

	for _, tt := range tests {
		p := price.NewPrice(secID, d, m, tt.source)
		app := &App{}
		row := app.formatPriceRow(p)
		if row[2] != tt.expected {
			t.Errorf("source %q: got %q, want %q", tt.source, row[2], tt.expected)
		}
	}
}

func TestBuildPriceTable(t *testing.T) {
	secID := types.NewID()
	d1 := types.NewDate(2025, time.March, 15)
	d2 := types.NewDate(2025, time.March, 14)
	m1, _ := types.NewMoney("185.50")
	m2, _ := types.NewMoney("184.00")

	p1 := price.NewPrice(secID, d1, m1, price.SourceManual)
	p2 := price.NewPrice(secID, d2, m2, price.SourceImport)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		priceView: &priceViewData{
			selectedSecurity: sec,
			prices:           []*price.Price{p1, p2},
		},
	}

	app.buildPriceTable()

	if app.priceTable == nil {
		t.Fatal("priceTable should not be nil after build")
	}
	if app.priceTable.RowCount() != 2 {
		t.Errorf("expected 2 rows, got %d", app.priceTable.RowCount())
	}
}

func TestBuildPriceTable_SortedByDateDesc(t *testing.T) {
	secID := types.NewID()
	d1 := types.NewDate(2025, time.January, 1)
	d2 := types.NewDate(2025, time.March, 15)
	d3 := types.NewDate(2025, time.February, 10)
	m, _ := types.NewMoney("100.00")

	p1 := price.NewPrice(secID, d1, m, price.SourceManual)
	p2 := price.NewPrice(secID, d2, m, price.SourceManual)
	p3 := price.NewPrice(secID, d3, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		priceView: &priceViewData{
			selectedSecurity: sec,
			// Prices already sorted desc (as returned by service)
			prices: []*price.Price{p2, p3, p1},
		},
	}

	app.buildPriceTable()

	if app.priceTable == nil {
		t.Fatal("priceTable should not be nil")
	}

	// widget.Table should have rows sorted by date desc (newest first)
	if app.priceTable.RowCount() != 3 {
		t.Fatalf("expected 3 rows, got %d", app.priceTable.RowCount())
	}
}

// =============================================================================
// SM-124: Price view navigation and keybindings
// =============================================================================

func TestHandlePriceViewKeys_Navigation(t *testing.T) {
	secID := types.NewID()
	d1 := types.NewDate(2025, time.March, 15)
	d2 := types.NewDate(2025, time.March, 14)
	m, _ := types.NewMoney("100.00")

	p1 := price.NewPrice(secID, d1, m, price.SourceManual)
	p2 := price.NewPrice(secID, d2, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			prices:           []*price.Price{p1, p2},
		},
	}
	app.buildPriceTable()

	// Move down
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.handlePriceViewKeys(downKey)

	if app.priceTable.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 after down", app.priceTable.Cursor())
	}

	// Move up
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	app.handlePriceViewKeys(upKey)

	if app.priceTable.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 after up", app.priceTable.Cursor())
	}
}

func TestHandlePriceViewKeys_NewOpensDialog(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.buildPriceTable()

	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	app.handlePriceViewKeys(nKey)

	if app.priceDialog == nil {
		t.Error("priceDialog should be set after pressing 'n'")
	}
	if app.priceDialogMode != priceDialogModeAdd {
		t.Errorf("priceDialogMode = %d, want %d (add)", app.priceDialogMode, priceDialogModeAdd)
	}
}

func TestHandlePriceViewKeys_EnterOpensEditDialog(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{p},
		},
	}
	app.buildPriceTable()

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	app.handlePriceViewKeys(enterKey)

	if app.priceDialog == nil {
		t.Error("priceDialog should be set after pressing Enter")
	}
	if app.priceDialogMode != priceDialogModeEdit {
		t.Errorf("priceDialogMode = %d, want %d (edit)", app.priceDialogMode, priceDialogModeEdit)
	}
}

func TestHandlePriceViewKeys_DeleteShowsConfirm(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:     80,
		height:    24,
		keys:      defaultKeyMap(),
		statusbar: widget.NewStatusBar(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{p},
		},
	}
	app.buildPriceTable()

	dKey := tea.KeyPressMsg{Code: 'd', Text: "d"}
	app.handlePriceViewKeys(dKey)

	if app.confirmDialog == nil {
		t.Error("confirm dialog should be set after pressing 'd'")
	}
}

func TestHandlePriceViewKeys_ImportOpensDialog(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.buildPriceTable()

	iKey := tea.KeyPressMsg{Code: 'i', Text: "i"}
	app.handlePriceViewKeys(iKey)

	if app.priceImportDialog == nil {
		t.Error("priceImportDialog should be set after pressing 'i'")
	}
}

func TestHandlePriceViewKeys_SearchMode(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.buildPriceTable()

	// Enter search mode
	slashKey := tea.KeyPressMsg{Code: '/', Text: "/"}
	app.handlePriceViewKeys(slashKey)

	if !app.priceView.searching {
		t.Error("should be in search mode after pressing '/'")
	}

	// Type search query
	aKey := tea.KeyPressMsg{Code: 'a', Text: "a"}
	app.handlePriceSearchKey(aKey)

	if app.priceView.searchQuery != "a" {
		t.Errorf("searchQuery = %q, want %q", app.priceView.searchQuery, "a")
	}

	// Escape exits search
	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	app.handlePriceSearchKey(escKey)

	if app.priceView.searching {
		t.Error("should exit search mode after Escape")
	}
}

// =============================================================================
// SM-125: Add/Edit price dialog
// =============================================================================

func TestBuildAddPriceDialog(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	d := buildAddPriceDialog(sec)

	if d == nil {
		t.Fatal("buildAddPriceDialog() returned nil")
	}

	fields := d.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	// Verify field labels
	if fields[0].Label != "Date" {
		t.Errorf("field[0].Label = %q, want %q", fields[0].Label, "Date")
	}
	if fields[1].Label != "Price" {
		t.Errorf("field[1].Label = %q, want %q", fields[1].Label, "Price")
	}

	// Both should be required
	if !fields[0].Required {
		t.Error("Date field should be required")
	}
	if !fields[1].Required {
		t.Error("Price field should be required")
	}

	// Date should default to today
	today := time.Now().Format("2006-01-02")
	if fields[0].Value != today {
		t.Errorf("date default = %q, want %q", fields[0].Value, today)
	}
	// Date field uses the masked-input widget (TD-015).
	if fields[0].Type != dialog.FieldDate {
		t.Errorf("Date field type = %d, want dialog.FieldDate", fields[0].Type)
	}
}

func TestBuildEditPriceDialog(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	dlg := buildEditPriceDialog(sec, p)
	if dlg == nil {
		t.Fatal("buildEditPriceDialog() returned nil")
	}

	fields := dlg.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	// Check pre-filled values
	if fields[0].Value != "2025-03-15" {
		t.Errorf("date value = %q, want %q", fields[0].Value, "2025-03-15")
	}
	if fields[1].Value != "185.50" {
		t.Errorf("price value = %q, want %q", fields[1].Value, "185.50")
	}
	// Date field uses the masked-input widget (TD-015).
	if fields[0].Type != dialog.FieldDate {
		t.Errorf("Date field type = %d, want dialog.FieldDate", fields[0].Type)
	}
}

// =============================================================================
// SM-126: Bulk import dialog
// =============================================================================

func TestBuildImportPriceDialog(t *testing.T) {
	d := buildImportPriceDialog()
	if d == nil {
		t.Fatal("buildImportPriceDialog() returned nil")
	}

	fields := d.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}

	// File path field
	if fields[0].Label != "CSV File" {
		t.Errorf("field[0].Label = %q, want %q", fields[0].Label, "CSV File")
	}
	if !fields[0].Required {
		t.Error("CSV File field should be required")
	}

	// Overwrite checkbox
	if fields[1].Label != "Overwrite existing" {
		t.Errorf("field[1].Label = %q, want %q", fields[1].Label, "Overwrite existing")
	}
}

// =============================================================================
// SM-127: Wire prices view to menu and navigation
// =============================================================================

func TestPriceViewString(t *testing.T) {
	v := ViewPrices
	if v.String() != "Prices" {
		t.Errorf("ViewPrices.String() = %q, want %q", v.String(), "Prices")
	}
}

func TestRenderPriceView_Loading(t *testing.T) {
	app := &App{
		styles: widget.NewStyles(),
	}
	app.styles.Resize(80, 24)

	output := app.renderPriceView()
	if !strings.Contains(output, "Loading prices") {
		t.Error("should show loading message when price view data is nil")
	}
}

func TestRenderPriceView_NoPrices(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		styles: widget.NewStyles(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.styles.Resize(80, 24)

	output := app.renderPriceView()
	if !strings.Contains(output, "No prices found") {
		t.Error("should show 'No prices found' message")
	}
}

func TestRenderPriceView_WithData(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	secID := sec.ID
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	app := &App{
		width:  100,
		height: 30,
		styles: widget.NewStyles(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{p},
		},
	}
	app.styles.Resize(100, 30)
	app.buildPriceTable()

	output := app.renderPriceView()
	if !strings.Contains(output, "PRICES") {
		t.Error("should contain 'PRICES' title")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("should contain security ticker")
	}
}

func TestRenderPriceView_ShowsSecurityInfo(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  100,
		height: 30,
		styles: widget.NewStyles(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.styles.Resize(100, 30)

	output := app.renderPriceView()
	if !strings.Contains(output, "Apple Inc.") {
		t.Errorf("should show security name, got: %s", output)
	}
}

func TestPriceViewDataLoadedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	data := &priceViewData{
		mode:             pricesViewDetail,
		selectedSecurity: sec,
		securities:       []*security.Security{sec},
		prices:           []*price.Price{},
	}

	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	msg := priceViewDataLoadedMsg{data: data}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.priceView == nil {
		t.Fatal("price view data should be set")
	}
	if updatedApp.priceTable == nil {
		t.Error("detail-mode price table should be built")
	}
}

func TestPriceViewDataLoadedMsg_ListMode(t *testing.T) {
	data := &priceViewData{
		mode:         pricesViewList,
		latestPrices: []*price.LatestPrice{},
	}

	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	msg := priceViewDataLoadedMsg{data: data}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.priceListTable == nil {
		t.Error("list-mode price list table should be built")
	}
}

func TestPriceViewUpdate_PriceAddedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}

	msg := priceAddedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about price added")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "Price added") {
		t.Errorf("notification = %q, should contain 'Price added'", notifications[0].Text)
	}
	if cmd == nil {
		t.Error("should return a command to reload price data")
	}
}

func TestPriceViewUpdate_PriceUpdatedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}

	msg := priceUpdatedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about price updated")
	}
	if cmd == nil {
		t.Error("should return a command to reload price data")
	}
}

func TestPriceViewUpdate_PriceDeletedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}

	msg := priceDeletedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about price deleted")
	}
	if cmd == nil {
		t.Error("should return a command to reload price data")
	}
}

func TestPriceViewUpdate_PriceImportedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}

	msg := priceImportedMsg{total: 10, imported: 8, skipped: 2}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about price import")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "Imported") {
		t.Errorf("notification = %q, should contain 'Imported'", notifications[0].Text)
	}
	if cmd == nil {
		t.Error("should return a command to reload price data")
	}
}

// PC-015: each price-CRUD message must evict the affected security's
// chart-history cache entry while leaving other entries intact, so the
// chart re-fetches for the modified ticker on next render but unrelated
// tickers keep their memoized history.

// pc015TestApp returns an App + the IDs of the selected (target of CRUD)
// and other (must remain cached) securities, with the historyCache
// pre-populated for both.
func pc015TestApp(t *testing.T) (app *App, selectedID, otherID types.ID) {
	t.Helper()
	selectedSec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	otherSec := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	cache := newHistoryCache()
	cache.Put(selectedSec.ID, []*price.Price{
		price.NewPrice(selectedSec.ID, types.NewDate(2026, time.April, 15), types.NewMoneyFromFloat(180.00), price.SourceManual),
	})
	cache.Put(otherSec.ID, []*price.Price{
		price.NewPrice(otherSec.ID, types.NewDate(2026, time.April, 15), types.NewMoneyFromFloat(420.00), price.SourceManual),
	})

	app = &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: selectedSec,
			securities:       []*security.Security{selectedSec, otherSec},
			prices:           []*price.Price{},
			historyCache:     cache,
		},
	}
	return app, selectedSec.ID, otherSec.ID
}

func TestPriceViewUpdate_PriceAddedMsg_EvictsSelectedSecurityFromCache(t *testing.T) {
	app, selectedID, otherID := pc015TestApp(t)

	app.Update(priceAddedMsg{})

	if _, ok := app.priceView.historyCache.Lookup(selectedID); ok {
		t.Errorf("priceAddedMsg should evict cache entry for selected security %v", selectedID)
	}
	if _, ok := app.priceView.historyCache.Lookup(otherID); !ok {
		t.Errorf("priceAddedMsg must not evict unrelated cache entry for security %v", otherID)
	}
}

func TestPriceViewUpdate_PriceUpdatedMsg_EvictsSelectedSecurityFromCache(t *testing.T) {
	app, selectedID, otherID := pc015TestApp(t)

	app.Update(priceUpdatedMsg{})

	if _, ok := app.priceView.historyCache.Lookup(selectedID); ok {
		t.Errorf("priceUpdatedMsg should evict cache entry for selected security %v", selectedID)
	}
	if _, ok := app.priceView.historyCache.Lookup(otherID); !ok {
		t.Errorf("priceUpdatedMsg must not evict unrelated cache entry for security %v", otherID)
	}
}

func TestPriceViewUpdate_PriceDeletedMsg_EvictsSelectedSecurityFromCache(t *testing.T) {
	app, selectedID, otherID := pc015TestApp(t)

	app.Update(priceDeletedMsg{})

	if _, ok := app.priceView.historyCache.Lookup(selectedID); ok {
		t.Errorf("priceDeletedMsg should evict cache entry for selected security %v", selectedID)
	}
	if _, ok := app.priceView.historyCache.Lookup(otherID); !ok {
		t.Errorf("priceDeletedMsg must not evict unrelated cache entry for security %v", otherID)
	}
}

func TestPriceViewUpdate_PriceImportedMsg_EvictsSelectedSecurityFromCache(t *testing.T) {
	app, selectedID, otherID := pc015TestApp(t)

	app.Update(priceImportedMsg{total: 5, imported: 3, skipped: 2})

	if _, ok := app.priceView.historyCache.Lookup(selectedID); ok {
		t.Errorf("priceImportedMsg should evict cache entry for selected security %v", selectedID)
	}
	if _, ok := app.priceView.historyCache.Lookup(otherID); !ok {
		t.Errorf("priceImportedMsg must not evict unrelated cache entry for security %v", otherID)
	}
}

// PC-015 also requires that the priceViewDataLoadedMsg reload triggered
// by these CRUD handlers preserves the existing cache rather than
// replacing it with a fresh empty one — otherwise the surgical Evict is
// immediately overridden by a Clear-equivalent. This test pins that
// contract: when the priceView already has a cache, a subsequent
// priceViewDataLoadedMsg must keep its entries.
func TestPriceViewDataLoadedMsg_PreservesExistingHistoryCache(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	cache := newHistoryCache()
	cache.Put(sec.ID, []*price.Price{
		price.NewPrice(sec.ID, types.NewDate(2026, time.April, 15), types.NewMoneyFromFloat(180.00), price.SourceManual),
	})

	app := &App{
		currentView: ViewPrices,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			mode:         pricesViewList,
			latestPrices: []*price.LatestPrice{},
			historyCache: cache,
		},
	}

	// Reload yields fresh data with its own (empty) historyCache; the
	// handler must re-attach the existing cache to the new data.
	freshData := &priceViewData{
		mode:         pricesViewList,
		latestPrices: []*price.LatestPrice{},
		historyCache: newHistoryCache(),
	}
	app.Update(priceViewDataLoadedMsg{data: freshData})

	if app.priceView.historyCache == nil {
		t.Fatal("historyCache should not be nil after reload")
	}
	if _, ok := app.priceView.historyCache.Lookup(sec.ID); !ok {
		t.Errorf("reload dropped cached entry for %v; PC-015 requires preserving the cache across reload", sec.ID)
	}
}

func TestPriceViewSwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.switchView(ViewPrices)

	if app.currentView != ViewPrices {
		t.Errorf("currentView = %v, want ViewPrices", app.currentView)
	}
	if app.previousView != ViewDashboard {
		t.Errorf("previousView = %v, want ViewDashboard", app.previousView)
	}
}

func TestPriceViewNavigateKeyBinding(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
	}

	// Press '5' to go to prices view
	fiveKey := tea.KeyPressMsg{Code: '5', Text: "5"}
	model, cmd := app.Update(fiveKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewPrices {
		t.Errorf("currentView = %v, want ViewPrices", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load price data")
	}
}

func TestPriceViewHelpOverlay(t *testing.T) {
	sections := viewShortcutSections(ViewPrices)

	found := false
	for _, s := range sections {
		if s.Title == "Prices" {
			found = true
			hasEnter := false
			hasEsc := false
			hasNew := false
			hasDelete := false
			hasImport := false
			hasSearch := false
			hasLeftRight := false
			for _, e := range s.Entries {
				switch e.Key {
				case "Enter":
					hasEnter = true
				case "Esc":
					hasEsc = true
				case "n":
					hasNew = true
				case "d":
					hasDelete = true
				case "i":
					hasImport = true
				case "/":
					hasSearch = true
				case "Left/Right":
					hasLeftRight = true
				}
			}
			if !hasEnter {
				t.Error("price shortcuts should include 'Enter' (drill in / edit)")
			}
			if !hasEsc {
				t.Error("price shortcuts should include 'Esc' (back to list)")
			}
			if !hasNew {
				t.Error("price shortcuts should include 'n' for new")
			}
			if !hasDelete {
				t.Error("price shortcuts should include 'd' for delete")
			}
			if !hasImport {
				t.Error("price shortcuts should include 'i' for import")
			}
			if !hasSearch {
				t.Error("price shortcuts should include '/' for search")
			}
			if hasLeftRight {
				t.Error("Left/Right cycling was removed; help overlay should not list it")
			}
		}
	}
	if !found {
		t.Error("prices shortcuts section not found in help overlay")
	}
}

func TestMenuBarHasPrices(t *testing.T) {
	mb := widget.NewMenuBar()
	found := false
	for _, m := range mb.Menus() {
		for _, item := range m.Items {
			if item.Action == widget.MenuActionPrices {
				found = true
				if item.Label != "Prices" {
					t.Errorf("menu label = %q, want %q", item.Label, "Prices")
				}
			}
		}
	}
	if !found {
		t.Error("widget.MenuActionPrices not found in menu bar")
	}
}

func TestPriceViewKeyHints_DetailMode(t *testing.T) {
	app := &App{
		currentView: ViewPrices,
		priceView:   &priceViewData{mode: pricesViewDetail},
	}

	hints := app.getKeyHints()
	if !strings.Contains(hints, "n new") {
		t.Errorf("hints should contain 'n new', got: %s", hints)
	}
	if !strings.Contains(hints, "enter edit") {
		t.Errorf("hints should contain 'enter edit', got: %s", hints)
	}
	if !strings.Contains(hints, "i import") {
		t.Errorf("hints should contain 'i import', got: %s", hints)
	}
}

func TestPriceViewKeyHints_ListMode(t *testing.T) {
	app := &App{
		currentView: ViewPrices,
		priceView:   &priceViewData{mode: pricesViewList},
	}

	hints := app.getKeyHints()
	if !strings.Contains(hints, "view history") {
		t.Errorf("list-mode hints should mention 'view history', got: %s", hints)
	}
	if strings.Contains(hints, "n new") {
		t.Errorf("list-mode hints should not advertise 'n new', got: %s", hints)
	}
}

func TestSelectedPrice(t *testing.T) {
	secID := types.NewID()
	d1 := types.NewDate(2025, time.March, 15)
	d2 := types.NewDate(2025, time.March, 14)
	m, _ := types.NewMoney("100.00")

	p1 := price.NewPrice(secID, d1, m, price.SourceManual)
	p2 := price.NewPrice(secID, d2, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			prices:           []*price.Price{p1, p2},
		},
	}
	app.buildPriceTable()

	selected := app.selectedPrice()
	if selected == nil {
		t.Fatal("selectedPrice() returned nil")
	}

	// Move down
	app.priceTable.MoveDown()
	selected = app.selectedPrice()
	if selected == nil {
		t.Fatal("selectedPrice() returned nil after MoveDown")
	}
}

func TestSelectedPrice_NilData(t *testing.T) {
	app := &App{}
	selected := app.selectedPrice()
	if selected != nil {
		t.Error("selectedPrice() should return nil when no price view data")
	}
}

func TestPriceView_FullScreenRender(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	secID := sec.ID
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	styles := widget.NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewPrices,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		priceView: &priceViewData{
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{p},
		},
	}
	app.buildPriceTable()

	content := app.renderContent(28)
	if !strings.Contains(content, "PRICES") {
		t.Error("renderContent should contain prices view content")
	}
}

func TestSecurityView_PNavigatesToPrices(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		currentView: ViewSecurities,
		width:       80,
		height:      24,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		securityView: &securityViewData{
			securities: []*security.Security{sec},
			showHidden: true,
		},
	}
	app.buildSecurityTable()

	// Press 'p' to navigate to prices view
	pKey := tea.KeyPressMsg{Code: 'p', Text: "p"}
	model, cmd := app.handleSecurityViewKeys(pKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewPrices {
		t.Errorf("currentView = %v, want ViewPrices after pressing 'p'", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load price data")
	}
}

func TestPriceViewData_DefaultModeIsList(t *testing.T) {
	d := &priceViewData{}
	if d.mode != pricesViewList {
		t.Errorf("zero-value mode = %v, want pricesViewList", d.mode)
	}
}

func TestBuildPriceListTable(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")

	app := &App{
		priceView: &priceViewData{
			mode: pricesViewList,
			latestPrices: []*price.LatestPrice{
				{SecurityID: secID, Ticker: "AAPL", Name: "Apple Inc.", Date: d, Price: m},
			},
		},
	}
	app.buildPriceListTable()

	if app.priceListTable == nil {
		t.Fatal("priceListTable should be built")
	}
	if app.priceListTable.RowCount() != 1 {
		t.Errorf("row count = %d, want 1", app.priceListTable.RowCount())
	}
}

func TestRenderPriceView_ListMode_ShowsLatestPrices(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")

	app := &App{
		width:  100,
		height: 30,
		styles: widget.NewStyles(),
		priceView: &priceViewData{
			mode: pricesViewList,
			latestPrices: []*price.LatestPrice{
				{SecurityID: secID, Ticker: "AAPL", Name: "Apple Inc.", Date: d, Price: m},
			},
		},
	}
	app.styles.Resize(100, 30)
	app.buildPriceListTable()

	output := app.renderPriceView()
	if !strings.Contains(output, "AAPL") {
		t.Error("list-mode render should show ticker")
	}
	if !strings.Contains(output, "Apple Inc.") {
		t.Error("list-mode render should show name")
	}
	if !strings.Contains(output, "2025-03-15") {
		t.Error("list-mode render should show latest date")
	}
}

// TestRenderPriceView_ListMode_NarrowOmitsChartPanel asserts that on
// terminals where the prices content area is below the chart-panel
// threshold, the list view renders exactly as it did before the chart
// feature — no border-decorated chart title appears.
func TestRenderPriceView_ListMode_NarrowOmitsChartPanel(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 100
	a.height = 30
	a.styles.Resize(100, 30)

	if a.styles.ContentWidth() >= chartPanelMinContentWidth {
		t.Fatalf("test premise: width=100 should yield ContentWidth < %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	d := types.MustParseDate("2026-04-15")
	m1, _ := types.NewMoney("100.00")
	m2, _ := types.NewMoney("110.00")
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d, m1, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}
	d2 := types.MustParseDate("2026-04-22")
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d2, m2, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}

	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d2, Price: m2},
		},
	}
	a.buildPriceListTable()

	output := a.renderPriceView()
	// The chart-panel title format `─ TICKER — NAME ─` must not appear
	// when the panel is suppressed.
	if strings.Contains(output, "─ AAPL — AAPL Inc.") {
		t.Errorf("narrow render should not include chart-panel title; got:\n%s", output)
	}
}

// TestRenderPriceView_ListMode_WideShowsChartPanel asserts that on a
// terminal whose content area is at or above the chart threshold, the
// rendered list view includes the chart panel for the highlighted
// ticker — identifiable by the title decoration in the top border.
func TestRenderPriceView_ListMode_WideShowsChartPanel(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	d1 := types.MustParseDate("2026-04-15")
	d2 := types.MustParseDate("2026-04-22")
	m1, _ := types.NewMoney("100.00")
	m2, _ := types.NewMoney("110.00")
	hp1 := price.NewPrice(secs[0].ID, d1, m1, price.SourceManual)
	hp2 := price.NewPrice(secs[0].ID, d2, m2, price.SourceManual)

	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d2, Price: m2},
		},
		historyCache: newHistoryCache(),
	}
	// Under PC-013 the chart-render path no longer calls priceSvc; it
	// reads only from priceView.historyCache. Pre-populate the cache so
	// renderPriceView has data to draw.
	a.priceView.historyCache.Put(secs[0].ID, []*price.Price{hp2, hp1})
	a.buildPriceListTable()

	output := a.renderPriceView()
	// The chart-panel title decoration is unique to the panel — it
	// won't appear in the table row, which uses no em-dash separator.
	if !strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("wide render should include chart-panel title `AAPL — AAPL Inc.`; got:\n%s", output)
	}
}

// PC-007: at content width >= chartPanelMinContentWidth, when the
// highlighted security has zero prices on file, the chart panel renders
// the "No price history" placeholder inside a still-titled box rather
// than a chart. Today's loadPriceViewData filters 0-price securities
// out of latestPrices, so the test injects a synthetic LatestPrice row
// pointing at a security with no prices in the DB to drive the case.
func TestRenderPriceView_ListMode_ZeroPriceSecurityShowsPlaceholder(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	// Drive the 0-price branch by caching an empty history slice for
	// secs[0]. Under PC-013 the cache is the chart's source of truth.
	d := types.MustParseDate("2026-04-22")
	placeholder, _ := types.NewMoney("0.00")
	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: placeholder},
		},
		historyCache: newHistoryCache(),
	}
	a.priceView.historyCache.Put(secs[0].ID, nil)
	a.buildPriceListTable()

	output := a.renderPriceView()
	if !strings.Contains(output, "No price history") {
		t.Errorf("0-price chart panel should render `No price history` placeholder; got:\n%s", output)
	}
	// Title decoration must still be present — the panel renders, it
	// just shows a placeholder instead of a line chart.
	if !strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("0-price chart panel must still render with title; got:\n%s", output)
	}
}

// PC-008: at content width >= chartPanelMinContentWidth, when the
// highlighted security has exactly one price on file, the chart panel
// renders the "Only one price on file — chart needs ≥ 2 points"
// placeholder with the value and date inside a still-titled box rather
// than a chart. The 1-price routing branch shipped under PC-006; this
// test pins the end-to-end contract through renderPriceView.
func TestRenderPriceView_ListMode_OnePriceSecurityShowsPlaceholder(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("185.50")
	hp := price.NewPrice(secs[0].ID, d, m, price.SourceManual)

	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: m},
		},
		historyCache: newHistoryCache(),
	}
	a.priceView.historyCache.Put(secs[0].ID, []*price.Price{hp})
	a.buildPriceListTable()

	output := a.renderPriceView()
	if !strings.Contains(output, "Only one price on file") {
		t.Errorf("1-price chart panel should render `Only one price on file` placeholder; got:\n%s", output)
	}
	if !strings.Contains(output, "≥ 2 points") {
		t.Errorf("1-price chart panel should render the minimum-points note; got:\n%s", output)
	}
	if !strings.Contains(output, "$185.50") {
		t.Errorf("1-price chart panel should render the formatted value; got:\n%s", output)
	}
	if !strings.Contains(output, "2026-04-22") {
		t.Errorf("1-price chart panel should render the formatted date; got:\n%s", output)
	}
	// Title decoration must still be present — the panel renders, it
	// just shows a placeholder instead of a line chart.
	if !strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("1-price chart panel must still render with title; got:\n%s", output)
	}
}

// PC-009: at content width >= chartPanelMinContentWidth, when the
// highlighted security has a flat-line price history (all values equal),
// the chart panel renders a real chart — not a placeholder — without
// panicking. The clampYRange helper pads the all-equal values by ±0.5%
// so ntcharts has a non-zero Y spread; this test pins that wiring at the
// renderPriceView level. The unit-level guard lives in
// TestBuildChartPanel_FlatLineDoesNotPanic; this is the end-to-end pin.
func TestRenderPriceView_ListMode_FlatLinePriceHistoryRendersChart(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	flat, _ := types.NewMoney("100.00")
	dates := []string{"2026-04-01", "2026-04-08", "2026-04-15"}
	flatPrices := make([]*price.Price, 0, len(dates))
	for _, ds := range dates {
		d := types.MustParseDate(ds)
		flatPrices = append(flatPrices, price.NewPrice(secs[0].ID, d, flat, price.SourceManual))
	}

	latestDate := types.MustParseDate("2026-04-15")
	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: latestDate, Price: flat},
		},
		historyCache: newHistoryCache(),
	}
	a.priceView.historyCache.Put(secs[0].ID, flatPrices)
	a.buildPriceListTable()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderPriceView panicked on flat-line input: %v", r)
		}
	}()

	output := a.renderPriceView()

	if !strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("flat-line chart panel must render with title; got:\n%s", output)
	}
	if strings.Contains(output, "No price history") {
		t.Errorf("flat-line panel should not render the 0-price placeholder; got:\n%s", output)
	}
	if strings.Contains(output, "Only one price on file") {
		t.Errorf("flat-line panel should not render the 1-price placeholder; got:\n%s", output)
	}
	if !strings.ContainsRune(output, '│') {
		t.Errorf("flat-line panel must contain box-drawing border; got:\n%s", output)
	}
}

func TestRenderPriceView_ListMode_EmptyShowsHint(t *testing.T) {
	app := &App{
		width:  80,
		height: 24,
		styles: widget.NewStyles(),
		priceView: &priceViewData{
			mode:         pricesViewList,
			latestPrices: nil,
		},
	}
	app.styles.Resize(80, 24)

	output := app.renderPriceView()
	if !strings.Contains(strings.ToLower(output), "no prices") {
		t.Errorf("empty list-mode should hint at the absence of prices, got: %s", output)
	}
}

// PC-010: at content width >= chartPanelMinContentWidth, when latestPrices
// is empty, the chart panel must not render — the empty hint stands alone
// just as it does at narrow widths. The chart-panel title decoration `─ X
// — Y ─` is unique to the panel; its absence proves the early-return path
// fires before composePriceListBody is reached.
func TestRenderPriceView_ListMode_WideEmptyOmitsChartPanel(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	a.priceView = &priceViewData{
		mode:         pricesViewList,
		securities:   secs,
		latestPrices: nil,
	}
	a.buildPriceListTable()

	output := a.renderPriceView()
	if !strings.Contains(strings.ToLower(output), "no prices") {
		t.Errorf("wide empty list-mode should still show the empty hint; got:\n%s", output)
	}
	if strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("wide empty list-mode must not render a chart panel; got:\n%s", output)
	}
	if strings.ContainsRune(output, '│') {
		t.Errorf("wide empty list-mode must not render box-drawing borders; got:\n%s", output)
	}
}

// PC-010: when the priceListTable cursor is past the end of latestPrices
// (a transient inconsistency that can happen if the data slice shrinks
// between rebuilds), buildPriceListChartPanel must return "" so no chart
// panel renders. The render falls back to just the table.
func TestRenderPriceView_ListMode_OutOfRangeCursorOmitsChartPanel(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("100.00")
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d, m, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}

	// Build with two rows so the table holds two slots, then move the
	// cursor to slot 1, then shrink latestPrices to one row without
	// rebuilding the table. The table cursor (1) is now >= len(latestPrices) (1),
	// which is the out-of-range case PC-010 guards against.
	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: m},
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: m},
		},
	}
	a.buildPriceListTable()
	a.priceListTable.MoveDown()
	if a.priceListTable.Cursor() != 1 {
		t.Fatalf("test premise: expected cursor=1 after MoveDown, got %d", a.priceListTable.Cursor())
	}
	a.priceView.latestPrices = a.priceView.latestPrices[:1]

	output := a.renderPriceView()
	if strings.Contains(output, "AAPL — AAPL Inc.") {
		t.Errorf("out-of-range cursor must suppress the chart panel; got:\n%s", output)
	}
	if strings.Contains(output, "No price history") {
		t.Errorf("out-of-range cursor must not render placeholder either; got:\n%s", output)
	}
}

func TestHandlePriceViewKeys_ListMode_EnterDrillsIn(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")

	sec := &security.Security{Ticker: "AAPL", Name: "Apple Inc."}
	sec.ID = secID

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:       pricesViewList,
			securities: []*security.Security{sec},
			latestPrices: []*price.LatestPrice{
				{SecurityID: secID, Ticker: "AAPL", Name: "Apple Inc.", Date: d, Price: m},
			},
		},
	}
	app.buildPriceListTable()

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handlePriceViewKeys(enter)

	if cmd == nil {
		t.Fatal("Enter in list mode should return a command to load detail")
	}
	if app.priceView.mode != pricesViewDetail {
		t.Errorf("mode = %v, want pricesViewDetail after Enter", app.priceView.mode)
	}
	if app.priceView.selectedSecurity == nil || app.priceView.selectedSecurity.ID != secID {
		t.Error("selectedSecurity should be set to the row's security")
	}
}

func TestHandlePriceViewKeys_DetailMode_EscReturnsToList(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.buildPriceTable()

	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, cmd := app.handlePriceViewKeys(esc)

	if cmd == nil {
		t.Fatal("Esc in detail mode should return a command to reload list")
	}
	if app.priceView.mode != pricesViewList {
		t.Errorf("mode = %v, want pricesViewList after Esc", app.priceView.mode)
	}
}

// TestHandleKeyPress_PricesDetail_EscStaysInPricesView verifies that pressing
// Esc while drilled into a ticker on the Prices view returns to the Prices
// list (mode = pricesViewList) rather than switching back to the previously
// active view (e.g. ViewSecurities). The global Escape handler in
// handleKeyPress runs before view-specific handlers, so it must defer to the
// price view when in detail mode.
func TestHandleKeyPress_PricesDetail_EscStaysInPricesView(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		currentView:  ViewPrices,
		previousView: ViewSecurities,
		width:        80,
		height:       24,
		keys:         defaultKeyMap(),
		menubar:      widget.NewMenuBar(),
		statusbar:    widget.NewStatusBar(),
		priceView: &priceViewData{
			mode:             pricesViewDetail,
			selectedSecurity: sec,
			securities:       []*security.Security{sec},
			prices:           []*price.Price{},
		},
	}
	app.buildPriceTable()

	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, _ = app.handleKeyPress(esc)

	if app.currentView != ViewPrices {
		t.Errorf("currentView = %v, want ViewPrices (Esc in price detail must not switch views)", app.currentView)
	}
	if app.priceView.mode != pricesViewList {
		t.Errorf("mode = %v, want pricesViewList after Esc", app.priceView.mode)
	}
}

func TestApp_MousePricesList_DoubleClickDrillsIn(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")

	sec := &security.Security{Ticker: "AAPL", Name: "Apple Inc."}
	sec.ID = secID

	now := time.Unix(0, 0)

	app := &App{
		currentView: ViewPrices,
		width:       100,
		height:      30,
		keys:        defaultKeyMap(),
		styles:      widget.NewStyles(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		priceView: &priceViewData{
			mode:       pricesViewList,
			securities: []*security.Security{sec},
			latestPrices: []*price.LatestPrice{
				{SecurityID: secID, Ticker: "AAPL", Name: "Apple Inc.", Date: d, Price: m},
			},
		},
	}
	app.styles.Resize(100, 30)
	app.priceListClicks = widget.NewClickTracker(400 * time.Millisecond)
	app.priceListClicks.SetNowFn(func() time.Time { return now })
	app.buildPriceListTable()

	// Y layout: 0 menu bar, 1 top padding, 2 title, 3 title separator,
	// 4 table header, 5 header border, 6 data row 0.
	click := tea.MouseClickMsg{X: 5, Y: 6, Button: tea.MouseLeft}

	_, cmd := app.Update(click)
	if cmd != nil {
		t.Fatal("first click should not drill in")
	}

	now = now.Add(100 * time.Millisecond)
	_, cmd = app.Update(click)
	if cmd == nil {
		t.Fatal("double click should return a drill-in command")
	}
	if app.priceView.mode != pricesViewDetail {
		t.Errorf("mode = %v, want pricesViewDetail", app.priceView.mode)
	}
}

// TestRenderPriceView_ListMode_ChartUsesHistoryCache pins PC-012's
// contract under the PC-013 model: the chart-render path reads ONLY
// from priceView.historyCache and never queries priceSvc. The test
// proves this by mutating the price service between renders — if the
// chart bypassed the cache, the second render would reflect the
// mutation; with the cache as the source of truth, it does not.
//
// Setup: AAPL with two prices in priceSvc AND in the cache. After the
// first render, delete one price directly via priceSvc — the chart
// must continue to show the original 2-price slice on subsequent
// renders. Finally, historyCache.Clear() drops the entry so the next
// render shows no chart panel at all (the cache is the sole source).
func TestRenderPriceView_ListMode_ChartUsesHistoryCache(t *testing.T) {
	a, _, secs := setupRefreshTUITest(t, "AAPL")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	if a.styles.ContentWidth() < chartPanelMinContentWidth {
		t.Fatalf("test premise: width=200 should yield ContentWidth >= %d, got %d",
			chartPanelMinContentWidth, a.styles.ContentWidth())
	}

	d1 := types.MustParseDate("2026-04-15")
	d2 := types.MustParseDate("2026-04-22")
	m1, _ := types.NewMoney("100.00")
	m2, _ := types.NewMoney("110.00")
	older := price.NewPrice(secs[0].ID, d1, m1, price.SourceManual)
	newer := price.NewPrice(secs[0].ID, d2, m2, price.SourceManual)
	if err := a.priceSvc.AddPrice(older); err != nil {
		t.Fatalf("AddPrice older: %v", err)
	}
	if err := a.priceSvc.AddPrice(newer); err != nil {
		t.Fatalf("AddPrice newer: %v", err)
	}

	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d2, Price: m2},
		},
		historyCache: newHistoryCache(),
	}
	// Mirror what the async fetch path would do: cache the 2-price
	// slice (newest-first, matching priceSvc.GetPriceHistory contract).
	a.priceView.historyCache.Put(secs[0].ID, []*price.Price{newer, older})
	a.buildPriceListTable()

	// First render — the cache has the 2-price slice, full chart shows.
	out1 := a.renderPriceView()
	if !strings.Contains(out1, "AAPL — AAPL Inc.") {
		t.Fatalf("first render missing chart-panel title; got:\n%s", out1)
	}
	if strings.Contains(out1, "Only one price on file") {
		t.Fatalf("first render should show full chart, not 1-price placeholder; got:\n%s", out1)
	}

	// Mutate the underlying store: delete the older price. If the
	// chart-render path queried priceSvc, the next render would route
	// to the 1-price placeholder. With the cache as the sole source,
	// the chart is unaffected.
	if err := a.priceSvc.DeletePrice(older.ID); err != nil {
		t.Fatalf("DeletePrice: %v", err)
	}

	out2 := a.renderPriceView()
	if !strings.Contains(out2, "AAPL — AAPL Inc.") {
		t.Fatalf("second render missing chart-panel title; got:\n%s", out2)
	}
	if strings.Contains(out2, "Only one price on file") {
		t.Errorf("second render must read from cache, not priceSvc "+
			"(saw 1-price placeholder after deleting one price); got:\n%s", out2)
	}

	// Clear the cache. With no fallback (chartDisplayedID is also
	// dropped because Clear evicts every entry), the chart panel
	// disappears entirely — proving the cache is the only source.
	a.priceView.historyCache.Clear()

	out3 := a.renderPriceView()
	if strings.Contains(out3, "AAPL — AAPL Inc.") {
		t.Errorf("post-Clear render must omit the chart panel "+
			"(no cache entry, no fallback); got:\n%s", out3)
	}
	if strings.Contains(out3, "Only one price on file") {
		t.Errorf("post-Clear render must not re-fetch from priceSvc; got:\n%s", out3)
	}
}

// =============================================================================
// PC-013: Debounced fetch on cursor change
// =============================================================================

// withShortPriceChartDebounce swaps priceChartDebounceDelay for d for
// the duration of the test, then restores it. Tests use this to drive
// real tea.Tick commands without a 150 ms wait.
func withShortPriceChartDebounce(t *testing.T, d time.Duration) {
	t.Helper()
	prev := priceChartDebounceDelay
	priceChartDebounceDelay = d
	t.Cleanup(func() { priceChartDebounceDelay = prev })
}

// runDebounceTick blocks until cmd produces a priceChartDebounceTickMsg
// (or t.Fatals after 1 s). The caller must have shortened
// priceChartDebounceDelay first via withShortPriceChartDebounce.
func runDebounceTick(t *testing.T, cmd tea.Cmd) priceChartDebounceTickMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd from schedule, got nil")
	}
	type result struct{ msg tea.Msg }
	ch := make(chan result, 1)
	go func() { ch <- result{msg: cmd()} }()
	select {
	case r := <-ch:
		tick, ok := r.msg.(priceChartDebounceTickMsg)
		if !ok {
			t.Fatalf("debounce cmd produced %T, want priceChartDebounceTickMsg", r.msg)
		}
		return tick
	case <-time.After(time.Second):
		t.Fatal("debounce cmd did not fire within 1 s")
	}
	return priceChartDebounceTickMsg{}
}

// PC-013(a): a cursor-moving keypress on the prices list returns a
// non-nil tea.Cmd which, when invoked, produces a
// priceChartDebounceTickMsg targeting the row the cursor landed on.
func TestHandlePriceListKeys_DownSchedulesDebounceTick(t *testing.T) {
	withShortPriceChartDebounce(t, time.Millisecond)

	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()

	startCursor := a.priceListTable.Cursor()
	if startCursor != 0 {
		t.Fatalf("test premise: expected initial cursor 0, got %d", startCursor)
	}

	_, cmd := a.handlePriceListKeys(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd == nil {
		t.Fatal("Down keypress on price list must return a debounce-scheduling cmd, got nil")
	}
	if a.priceListTable.Cursor() != 1 {
		t.Fatalf("Down should advance cursor to 1, got %d", a.priceListTable.Cursor())
	}
	tick := runDebounceTick(t, cmd)
	if tick.secID != secs[1].ID {
		t.Errorf("tick.secID = %v, want %v (the row Down moved to)", tick.secID, secs[1].ID)
	}
	if tick.gen != a.priceView.chartDebounceGen {
		t.Errorf("tick.gen = %d, want current chartDebounceGen %d",
			tick.gen, a.priceView.chartDebounceGen)
	}
}

// PC-013(b): two consecutive cursor changes within the debounce window
// produce two scheduled ticks, but only the second one's gen matches
// chartDebounceGen — the first is silently dropped by the tick handler.
func TestPriceChartDebounceTick_StaleGenIsDropped(t *testing.T) {
	withShortPriceChartDebounce(t, time.Millisecond)

	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()

	// First schedule (cursor still on row 0).
	cmd1 := a.schedulePriceChartFetch(secs[0].ID)
	tick1 := runDebounceTick(t, cmd1)

	// Second schedule, before tick1 has been routed through Update.
	// chartDebounceGen bumps; tick1 is now stale.
	cmd2 := a.schedulePriceChartFetch(secs[1].ID)
	tick2 := runDebounceTick(t, cmd2)

	if tick1.gen >= tick2.gen {
		t.Fatalf("expected tick2.gen > tick1.gen, got %d vs %d", tick2.gen, tick1.gen)
	}
	if a.priceView.chartDebounceGen != tick2.gen {
		t.Fatalf("expected chartDebounceGen=%d, got %d", tick2.gen, a.priceView.chartDebounceGen)
	}

	// Move cursor onto secs[1] so tick2 (which targets secs[1]) won't
	// be dropped by the cursor-mismatch guard, isolating the gen check.
	a.priceListTable.MoveDown()

	// Stale tick1 must be ignored — Update returns no cmd, no state change.
	prevDisplayed := a.priceView.chartDisplayedID
	_, c1 := a.Update(tick1)
	if c1 != nil {
		t.Errorf("stale tick must produce no cmd, got %T", c1)
	}
	if a.priceView.chartDisplayedID != prevDisplayed {
		t.Errorf("stale tick must not mutate chartDisplayedID")
	}

	// Current-gen tick must produce a fetch cmd (cache miss path).
	_, c2 := a.Update(tick2)
	if c2 == nil {
		t.Errorf("current-gen tick on uncached row must return a fetch cmd, got nil")
	}
}

// PC-013(c): a tick whose gen matches chartDebounceGen and whose secID
// matches the cursor produces the actual fetch command. Running that
// command yields a priceChartHistoryLoadedMsg carrying the prices that
// priceSvc.GetPriceHistory returns.
func TestPriceChartDebounceTick_DispatchesFetchOnMatch(t *testing.T) {
	withShortPriceChartDebounce(t, time.Millisecond)

	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("180.00")
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d, m, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}

	cmd := a.schedulePriceChartFetch(secs[0].ID)
	tick := runDebounceTick(t, cmd)

	_, fetchCmd := a.Update(tick)
	if fetchCmd == nil {
		t.Fatal("matching tick on uncached row must return a fetch cmd")
	}
	msg := fetchCmd()
	loaded, ok := msg.(priceChartHistoryLoadedMsg)
	if !ok {
		t.Fatalf("fetch cmd produced %T, want priceChartHistoryLoadedMsg", msg)
	}
	if loaded.secID != secs[0].ID {
		t.Errorf("loaded.secID = %v, want %v", loaded.secID, secs[0].ID)
	}
	if len(loaded.prices) != 1 {
		t.Errorf("loaded.prices has %d entries, want 1", len(loaded.prices))
	}
}

// PC-013(d): the priceChartHistoryLoadedMsg handler stores the prices
// in the cache and sets chartDisplayedID. It returns no further cmd —
// the debounce/fetch chain terminates here.
func TestPriceChartHistoryLoadedMsg_UpdatesStateAndStops(t *testing.T) {
	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()
	a.priceView.historyCache = newHistoryCache()

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("180.00")
	hp := price.NewPrice(secs[0].ID, d, m, price.SourceManual)

	loaded := priceChartHistoryLoadedMsg{
		secID:  secs[0].ID,
		prices: []*price.Price{hp},
	}
	_, cmd := a.Update(loaded)

	if cmd != nil {
		t.Errorf("loaded handler must return no further cmd, got %T", cmd)
	}
	if a.priceView.chartDisplayedID != secs[0].ID {
		t.Errorf("chartDisplayedID = %v, want %v", a.priceView.chartDisplayedID, secs[0].ID)
	}
	cached, ok := a.priceView.historyCache.Lookup(secs[0].ID)
	if !ok {
		t.Fatalf("cache must contain entry for %v after loaded msg", secs[0].ID)
	}
	if len(cached) != 1 || cached[0] != hp {
		t.Errorf("cache stored %v, want [%v]", cached, hp)
	}
}

// PC-013: when the row under the cursor is already cached, the tick
// handler skips the fetch and just promotes that entry to displayed.
// This lets the user scroll back to a previously-viewed ticker without
// re-querying the price service.
func TestPriceChartDebounceTick_CacheHitSkipsFetch(t *testing.T) {
	withShortPriceChartDebounce(t, time.Millisecond)

	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("180.00")
	hp := price.NewPrice(secs[0].ID, d, m, price.SourceManual)
	a.priceView.historyCache = newHistoryCache()
	a.priceView.historyCache.Put(secs[0].ID, []*price.Price{hp})

	cmd := a.schedulePriceChartFetch(secs[0].ID)
	tick := runDebounceTick(t, cmd)

	_, fetchCmd := a.Update(tick)
	if fetchCmd != nil {
		t.Errorf("cache-hit tick must return no fetch cmd, got %T", fetchCmd)
	}
	if a.priceView.chartDisplayedID != secs[0].ID {
		t.Errorf("cache-hit tick must promote chartDisplayedID to %v, got %v",
			secs[0].ID, a.priceView.chartDisplayedID)
	}
}

// PC-013: a tick whose gen matches but whose secID no longer matches
// the cursor (the user moved off in the debounce window) is dropped —
// no fetch, no state change.
func TestPriceChartDebounceTick_CursorMismatchIsDropped(t *testing.T) {
	withShortPriceChartDebounce(t, time.Millisecond)

	a, _, secs := setupAppWithTwoSecurities(t)
	a.buildPriceListTable()

	cmd := a.schedulePriceChartFetch(secs[0].ID)
	tick := runDebounceTick(t, cmd)

	// Move cursor off secs[0] without scheduling a new tick (test
	// fixture; in production the move would itself schedule).
	a.priceListTable.MoveDown()
	if a.listCursorSecurityID() != secs[1].ID {
		t.Fatalf("test premise: cursor should be on secs[1] after MoveDown")
	}

	prevGen := a.priceView.chartDebounceGen
	prevDisplayed := a.priceView.chartDisplayedID
	_, c := a.Update(tick)
	if c != nil {
		t.Errorf("cursor-mismatch tick must produce no cmd, got %T", c)
	}
	if a.priceView.chartDebounceGen != prevGen {
		t.Errorf("cursor-mismatch tick must not bump gen")
	}
	if a.priceView.chartDisplayedID != prevDisplayed {
		t.Errorf("cursor-mismatch tick must not mutate chartDisplayedID")
	}
}

// PC-014: every cursor-moving key on the prices list returns a
// debounce-scheduling cmd. The PC-013(a) test pins Down specifically;
// this table covers the remaining five (Up, Home, g, End, G, PgUp,
// PgDown) so the wiring stays symmetric.
func TestHandlePriceListKeys_CursorMovingKeysScheduleDebounce(t *testing.T) {
	cases := []struct {
		name        string
		msg         tea.KeyPressMsg
		startCursor int
		wantSecIdx  int
	}{
		{"Up", tea.KeyPressMsg{Code: tea.KeyUp}, 1, 0},
		{"Home", tea.KeyPressMsg{Code: tea.KeyHome}, 1, 0},
		{"g", tea.KeyPressMsg{Code: 'g', Text: "g"}, 1, 0},
		{"End", tea.KeyPressMsg{Code: tea.KeyEnd}, 0, 1},
		{"G", tea.KeyPressMsg{Code: 'G', Text: "G"}, 0, 1},
		{"PgUp", tea.KeyPressMsg{Code: tea.KeyPgUp}, 1, 0},
		{"PgDown", tea.KeyPressMsg{Code: tea.KeyPgDown}, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withShortPriceChartDebounce(t, time.Millisecond)

			a, _, secs := setupAppWithTwoSecurities(t)
			a.buildPriceListTable()
			for a.priceListTable.Cursor() < tc.startCursor {
				a.priceListTable.MoveDown()
			}
			for a.priceListTable.Cursor() > tc.startCursor {
				a.priceListTable.MoveUp()
			}
			if a.priceListTable.Cursor() != tc.startCursor {
				t.Fatalf("test premise: failed to position cursor at %d, got %d",
					tc.startCursor, a.priceListTable.Cursor())
			}

			beforeGen := a.priceView.chartDebounceGen
			_, cmd := a.handlePriceListKeys(tc.msg)
			if cmd == nil {
				t.Fatalf("%s keypress on price list must return a debounce-scheduling cmd, got nil", tc.name)
			}
			if a.priceView.chartDebounceGen <= beforeGen {
				t.Errorf("%s must bump chartDebounceGen: before=%d after=%d",
					tc.name, beforeGen, a.priceView.chartDebounceGen)
			}

			wantID := secs[tc.wantSecIdx].ID
			if a.listCursorSecurityID() != wantID {
				t.Fatalf("cursor landed on %v, want %v (idx %d)",
					a.listCursorSecurityID(), wantID, tc.wantSecIdx)
			}
			tick := runDebounceTick(t, cmd)
			if tick.secID != wantID {
				t.Errorf("tick.secID = %v, want %v", tick.secID, wantID)
			}
			if tick.gen != a.priceView.chartDebounceGen {
				t.Errorf("tick.gen = %d, want current chartDebounceGen %d",
					tick.gen, a.priceView.chartDebounceGen)
			}
		})
	}
}

// PC-014: keys that are not cursor moves on the prices list must not
// schedule a debounce — chartDebounceGen must not bump. Covers `/`
// (search) and `u` (refresh).
func TestHandlePriceListKeys_NonCursorKeysDoNotScheduleDebounce(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"slash-search", tea.KeyPressMsg{Code: '/', Text: "/"}},
		{"u-refresh", tea.KeyPressMsg{Code: 'u', Text: "u"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withShortPriceChartDebounce(t, time.Millisecond)

			a, _, _ := setupAppWithTwoSecurities(t)
			a.buildPriceListTable()
			beforeCursor := a.priceListTable.Cursor()
			beforeGen := a.priceView.chartDebounceGen

			a.handlePriceListKeys(tc.msg)

			if a.priceView.chartDebounceGen != beforeGen {
				t.Errorf("%s must not bump chartDebounceGen: before=%d after=%d",
					tc.name, beforeGen, a.priceView.chartDebounceGen)
			}
			if a.priceListTable.Cursor() != beforeCursor {
				t.Errorf("%s must not move cursor: before=%d after=%d",
					tc.name, beforeCursor, a.priceListTable.Cursor())
			}
		})
	}
}

// setupAppWithTwoSecurities builds a wide TUI with two rows in the
// price list (AAPL, MSFT) but no prices loaded into the cache. The
// resulting App is ready to drive cursor-movement key handlers and
// dispatch debounce ticks through Update.
func setupAppWithTwoSecurities(t *testing.T) (*App, *fakeRefreshProvider, []*security.Security) {
	t.Helper()
	a, fp, secs := setupRefreshTUITest(t, "AAPL", "MSFT")
	a.width = 200
	a.height = 30
	a.styles.Resize(200, 30)

	d := types.MustParseDate("2026-04-22")
	m, _ := types.NewMoney("100.00")
	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: m},
			{SecurityID: secs[1].ID, Ticker: "MSFT", Name: "MSFT Inc.", Date: d, Price: m},
		},
		historyCache: newHistoryCache(),
	}
	return a, fp, secs
}
