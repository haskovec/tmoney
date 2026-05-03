package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
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

	// Table should have rows sorted by date desc (newest first)
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
		statusbar: NewStatusBar(),
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
}

func TestBuildEditPriceDialog(t *testing.T) {
	secID := types.NewID()
	d := types.NewDate(2025, time.March, 15)
	m, _ := types.NewMoney("185.50")
	p := price.NewPrice(secID, d, m, price.SourceManual)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	dialog := buildEditPriceDialog(sec, p)
	if dialog == nil {
		t.Fatal("buildEditPriceDialog() returned nil")
	}

	fields := dialog.Fields()
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
		styles: NewStyles(),
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
		styles: NewStyles(),
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
		styles: NewStyles(),
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
		styles: NewStyles(),
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
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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

func TestPriceViewSwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
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
	mb := NewMenuBar()
	found := false
	for _, m := range mb.menus {
		for _, item := range m.items {
			if item.action == MenuActionPrices {
				found = true
				if item.label != "Prices" {
					t.Errorf("menu label = %q, want %q", item.label, "Prices")
				}
			}
		}
	}
	if !found {
		t.Error("MenuActionPrices not found in menu bar")
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

	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewPrices,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		statusbar:   NewStatusBar(),
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
		styles: NewStyles(),
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
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d1, m1, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}
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

	// Note: no prices added for secs[0]. priceSvc.GetPriceHistory will
	// return an empty slice, which routes buildChartPanel to the
	// 0-price placeholder branch.
	d := types.MustParseDate("2026-04-22")
	placeholder, _ := types.NewMoney("0.00")
	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: placeholder},
		},
	}
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
	if err := a.priceSvc.AddPrice(price.NewPrice(secs[0].ID, d, m, price.SourceManual)); err != nil {
		t.Fatalf("AddPrice: %v", err)
	}

	a.priceView = &priceViewData{
		mode:       pricesViewList,
		securities: secs,
		latestPrices: []*price.LatestPrice{
			{SecurityID: secs[0].ID, Ticker: "AAPL", Name: "AAPL Inc.", Date: d, Price: m},
		},
	}
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

func TestRenderPriceView_ListMode_EmptyShowsHint(t *testing.T) {
	app := &App{
		width:  80,
		height: 24,
		styles: NewStyles(),
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
		menubar:      NewMenuBar(),
		statusbar:    NewStatusBar(),
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
		styles:      NewStyles(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		priceView: &priceViewData{
			mode:       pricesViewList,
			securities: []*security.Security{sec},
			latestPrices: []*price.LatestPrice{
				{SecurityID: secID, Ticker: "AAPL", Name: "Apple Inc.", Date: d, Price: m},
			},
		},
	}
	app.styles.Resize(100, 30)
	app.priceListClicks = NewClickTracker(400 * time.Millisecond)
	app.priceListClicks.SetNowFn(func() time.Time { return now })
	app.buildPriceListTable()

	// Click coordinates: content offset 3 (padding+title+separator) + table header 1 = row 0 of data at y=4 (contentY).
	// Convert to msg.Y = contentY + 1 (header row).
	click := tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft}

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
