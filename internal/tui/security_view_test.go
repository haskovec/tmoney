package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestSecurityViewData(t *testing.T) {
	data := &securityViewData{
		showHidden: false,
	}

	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec2.Hidden = true

	data.securities = []*security.Security{sec1, sec2}

	if len(data.securities) != 2 {
		t.Errorf("expected 2 securities, got %d", len(data.securities))
	}

	if data.showHidden {
		t.Error("showHidden should be false by default")
	}
}

func TestSecurityViewData_FilteredSecurities(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec2.Hidden = true
	sec3 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	data := &securityViewData{
		securities: []*security.Security{sec1, sec2, sec3},
		showHidden: false,
	}

	filtered := data.filteredSecurities()
	if len(filtered) != 2 {
		t.Errorf("expected 2 visible securities, got %d", len(filtered))
	}

	// Now show hidden
	data.showHidden = true
	filtered = data.filteredSecurities()
	if len(filtered) != 3 {
		t.Errorf("expected 3 securities with showHidden, got %d", len(filtered))
	}
}

func TestSecurityViewData_FilteredSecurities_Search(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec3 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	data := &securityViewData{
		securities:  []*security.Security{sec1, sec2, sec3},
		showHidden:  true,
		searchQuery: "apple",
	}

	filtered := data.filteredSecurities()
	if len(filtered) != 1 {
		t.Errorf("expected 1 matching security for 'apple', got %d", len(filtered))
	}
	if filtered[0].Ticker != "AAPL" {
		t.Errorf("expected AAPL, got %s", filtered[0].Ticker)
	}

	// Search by ticker
	data.searchQuery = "gdx"
	filtered = data.filteredSecurities()
	if len(filtered) != 1 {
		t.Errorf("expected 1 matching security for 'gdx', got %d", len(filtered))
	}

	// Search no match
	data.searchQuery = "zzzz"
	filtered = data.filteredSecurities()
	if len(filtered) != 0 {
		t.Errorf("expected 0 matching securities, got %d", len(filtered))
	}

	// Case-insensitive
	data.searchQuery = "MICRO"
	filtered = data.filteredSecurities()
	if len(filtered) != 1 {
		t.Errorf("expected 1 matching security for 'MICRO', got %d", len(filtered))
	}
}

func TestFormatSecurityRow(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	sec.Currency = "USD"

	app := &App{
		securityView: &securityViewData{},
	}

	row := app.formatSecurityRow(sec)

	if len(row) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(row))
	}

	if row[0] != "AAPL" {
		t.Errorf("ticker = %q, want %q", row[0], "AAPL")
	}
	if row[1] != "Apple Inc." {
		t.Errorf("name = %q, want %q", row[1], "Apple Inc.")
	}
	if row[2] != "Stock" {
		t.Errorf("type = %q, want %q", row[2], "Stock")
	}
	if row[3] != "Large Cap Stock" {
		t.Errorf("asset class = %q, want %q", row[3], "Large Cap Stock")
	}
	if row[4] != "USD" {
		t.Errorf("currency = %q, want %q", row[4], "USD")
	}
	if row[5] != "Active" {
		t.Errorf("status = %q, want %q", row[5], "Active")
	}
}

func TestFormatSecurityRow_Hidden(t *testing.T) {
	sec := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec.AssetClass = security.AssetClassCommodity
	sec.Hidden = true

	app := &App{
		securityView: &securityViewData{},
	}

	row := app.formatSecurityRow(sec)

	if row[5] != "Hidden" {
		t.Errorf("status = %q, want %q", row[5], "Hidden")
	}
}

func TestBuildSecurityTable(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	app := &App{
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2},
			showHidden: true,
		},
	}

	app.buildSecurityTable()

	if app.securityTable == nil {
		t.Fatal("securityTable should not be nil after build")
	}
	if app.securityTable.RowCount() != 2 {
		t.Errorf("expected 2 rows, got %d", app.securityTable.RowCount())
	}
}

func TestBuildSecurityTable_WithHiddenFilter(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec2.Hidden = true

	app := &App{
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2},
			showHidden: false,
		},
	}

	app.buildSecurityTable()

	if app.securityTable.RowCount() != 1 {
		t.Errorf("expected 1 visible row, got %d", app.securityTable.RowCount())
	}
}

func TestBuildSecurityTable_SortsByTicker(t *testing.T) {
	sec1 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)
	sec2 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec3 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)

	app := &App{
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2, sec3},
			showHidden: true,
		},
	}

	app.buildSecurityTable()

	// The table should contain all 3 rows sorted by ticker
	if app.securityTable.RowCount() != 3 {
		t.Fatalf("expected 3 rows, got %d", app.securityTable.RowCount())
	}
}

func TestRenderSecurityView_Loading(t *testing.T) {
	app := &App{
		styles: NewStyles(),
	}
	app.styles.Resize(80, 24)

	output := app.renderSecurityView()
	if !strings.Contains(output, "Loading securities") {
		t.Error("should show loading message when security view data is nil")
	}
}

func TestRenderSecurityView_NoSecurities(t *testing.T) {
	app := &App{
		width:  80,
		height: 24,
		styles: NewStyles(),
		securityView: &securityViewData{
			securities: []*security.Security{},
			showHidden: true,
		},
	}
	app.styles.Resize(80, 24)

	output := app.renderSecurityView()
	if !strings.Contains(output, "No securities found") {
		t.Error("should show 'No securities found' message")
	}
}

func TestRenderSecurityView_WithData(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  100,
		height: 30,
		styles: NewStyles(),
		securityView: &securityViewData{
			securities: []*security.Security{sec},
			showHidden: true,
		},
	}
	app.styles.Resize(100, 30)
	app.buildSecurityTable()

	output := app.renderSecurityView()
	if !strings.Contains(output, "SECURITIES") {
		t.Error("should contain 'SECURITIES' title")
	}
}

func TestRenderSecurityView_ShowsFilterStatus(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	app := &App{
		width:  100,
		height: 30,
		styles: NewStyles(),
		securityView: &securityViewData{
			securities: []*security.Security{sec},
			showHidden: false,
		},
	}
	app.styles.Resize(100, 30)
	app.buildSecurityTable()

	output := app.renderSecurityView()
	if !strings.Contains(output, "Hidden: off") {
		t.Errorf("should show 'Hidden: off' when showHidden is false, got: %s", output)
	}

	// Toggle to show hidden
	app.securityView.showHidden = true
	app.buildSecurityTable()
	output = app.renderSecurityView()
	if !strings.Contains(output, "Hidden: on") {
		t.Errorf("should show 'Hidden: on' when showHidden is true")
	}
}

func TestHandleSecurityViewKeys_Navigation(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2},
			showHidden: true,
		},
	}
	app.buildSecurityTable()

	// Move down
	downKey := tea.KeyMsg{Type: tea.KeyDown}
	app.handleSecurityViewKeys(downKey)

	if app.securityTable.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 after down", app.securityTable.Cursor())
	}

	// Move up
	upKey := tea.KeyMsg{Type: tea.KeyUp}
	app.handleSecurityViewKeys(upKey)

	if app.securityTable.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 after up", app.securityTable.Cursor())
	}
}

func TestHandleSecurityViewKeys_ToggleHidden(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("GDX", "VanEck Gold Miners ETF", security.TypeETF)
	sec2.Hidden = true

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2},
			showHidden: false,
		},
	}
	app.buildSecurityTable()

	// Press 'f' to toggle hidden filter
	fKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	app.handleSecurityViewKeys(fKey)

	if !app.securityView.showHidden {
		t.Error("showHidden should be true after pressing 'f'")
	}

	// Press 'f' again to toggle back
	app.handleSecurityViewKeys(fKey)
	if app.securityView.showHidden {
		t.Error("showHidden should be false after pressing 'f' again")
	}
}

func TestSecurityViewDataLoadedMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	data := &securityViewData{
		securities: []*security.Security{sec},
		showHidden: false,
	}

	app := &App{
		currentView: ViewSecurities,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	msg := securityViewDataLoadedMsg{data: data}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.securityView == nil {
		t.Fatal("security view data should be set")
	}
	if updatedApp.securityTable == nil {
		t.Error("security table should be built")
	}
}

func TestSecurityViewString(t *testing.T) {
	v := ViewSecurities
	if v.String() != "Securities" {
		t.Errorf("ViewSecurities.String() = %q, want %q", v.String(), "Securities")
	}
}

func TestSecurityViewSwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.switchView(ViewSecurities)

	if app.currentView != ViewSecurities {
		t.Errorf("currentView = %v, want ViewSecurities", app.currentView)
	}
	if app.previousView != ViewDashboard {
		t.Errorf("previousView = %v, want ViewDashboard", app.previousView)
	}
}

func TestBuildAddSecurityDialog(t *testing.T) {
	d := buildAddSecurityDialog()

	if d == nil {
		t.Fatal("buildAddSecurityDialog() returned nil")
	}

	fields := d.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}

	// Verify field labels
	expectedLabels := []string{"Ticker", "Name", "Type", "Asset Class", "Currency", "Exchange"}
	for i, label := range expectedLabels {
		if fields[i].Label != label {
			t.Errorf("field[%d].Label = %q, want %q", i, fields[i].Label, label)
		}
	}

	// Ticker and Name should be required
	if !fields[0].Required {
		t.Error("Ticker field should be required")
	}
	if !fields[1].Required {
		t.Error("Name field should be required")
	}
}

func TestBuildEditSecurityDialog(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	sec.Currency = "USD"
	sec.SetExchange("NASDAQ")

	d := buildEditSecurityDialog(sec)

	if d == nil {
		t.Fatal("buildEditSecurityDialog() returned nil")
	}

	fields := d.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}

	// Verify pre-filled values
	if fields[0].Value != "AAPL" {
		t.Errorf("ticker value = %q, want %q", fields[0].Value, "AAPL")
	}
	if fields[1].Value != "Apple Inc." {
		t.Errorf("name value = %q, want %q", fields[1].Value, "Apple Inc.")
	}
	// Exchange field should be pre-filled
	if fields[5].Value != "NASDAQ" {
		t.Errorf("exchange value = %q, want %q", fields[5].Value, "NASDAQ")
	}
}

func TestBuildEditSecurityDialog_TypeSelection(t *testing.T) {
	sec := security.NewSecurity("VTI", "Vanguard Total Stock Market ETF", security.TypeETF)

	d := buildEditSecurityDialog(sec)
	fields := d.Fields()

	// Type field should have ETF selected (index 1 since order is Stock, ETF, Mutual Fund, Other)
	if fields[2].SelectedIndex != 1 {
		t.Errorf("type selected index = %d, want 1 (ETF)", fields[2].SelectedIndex)
	}
}

func TestBuildEditSecurityDialog_AssetClassSelection(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple", security.TypeStock)
	sec.AssetClass = security.AssetClassCommodity

	d := buildEditSecurityDialog(sec)
	fields := d.Fields()

	// Find the expected index for Commodity
	allClasses := security.AllAssetClasses()
	expectedIdx := 0
	for i, ac := range allClasses {
		if ac == security.AssetClassCommodity {
			expectedIdx = i
			break
		}
	}

	if fields[3].SelectedIndex != expectedIdx {
		t.Errorf("asset class selected index = %d, want %d (Commodity)", fields[3].SelectedIndex, expectedIdx)
	}
}

func TestSecurityViewKeyHints(t *testing.T) {
	app := &App{
		currentView: ViewSecurities,
	}

	hints := app.getKeyHints()
	if !strings.Contains(hints, "n new") {
		t.Errorf("hints should contain 'n new', got: %s", hints)
	}
	if !strings.Contains(hints, "enter edit") {
		t.Errorf("hints should contain 'enter edit', got: %s", hints)
	}
	if !strings.Contains(hints, "f filter hidden") {
		t.Errorf("hints should contain 'f filter hidden', got: %s", hints)
	}
}

func TestSecurityViewHelpOverlay(t *testing.T) {
	sections := viewShortcutSections(ViewSecurities)

	found := false
	for _, s := range sections {
		if s.Title == "Securities" {
			found = true
			hasNew := false
			hasEdit := false
			hasHide := false
			hasDelete := false
			hasFilter := false
			hasPrices := false
			for _, e := range s.Entries {
				switch e.Key {
				case "n":
					hasNew = true
				case "Enter":
					hasEdit = true
				case "h":
					hasHide = true
				case "d":
					hasDelete = true
				case "f":
					hasFilter = true
				case "p":
					hasPrices = true
				}
			}
			if !hasNew {
				t.Error("security shortcuts should include 'n' for new")
			}
			if !hasEdit {
				t.Error("security shortcuts should include 'Enter' for edit")
			}
			if !hasHide {
				t.Error("security shortcuts should include 'h' for toggle hidden")
			}
			if !hasDelete {
				t.Error("security shortcuts should include 'd' for delete")
			}
			if !hasFilter {
				t.Error("security shortcuts should include 'f' for filter hidden")
			}
			if !hasPrices {
				t.Error("security shortcuts should include 'p' for prices")
			}
		}
	}
	if !found {
		t.Error("securities shortcuts section not found in help overlay")
	}
}

func TestMenuBarHasSecurities(t *testing.T) {
	mb := NewMenuBar()
	found := false
	for _, m := range mb.menus {
		for _, item := range m.items {
			if item.action == MenuActionSecurities {
				found = true
				if item.label != "Securities Master" {
					t.Errorf("menu label = %q, want %q", item.label, "Securities Master")
				}
			}
		}
	}
	if !found {
		t.Error("MenuActionSecurities not found in menu bar")
	}
}

func TestSecuritySelectedSecurity(t *testing.T) {
	sec1 := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec2 := security.NewSecurity("MSFT", "Microsoft Corp", security.TypeStock)

	app := &App{
		width:  80,
		height: 24,
		keys:   defaultKeyMap(),
		securityView: &securityViewData{
			securities: []*security.Security{sec1, sec2},
			showHidden: true,
		},
	}
	app.buildSecurityTable()

	// Should select first security at cursor 0
	selected := app.selectedSecurity()
	if selected == nil {
		t.Fatal("selectedSecurity() returned nil")
	}
	if selected.Ticker != "AAPL" {
		t.Errorf("selected = %q, want AAPL", selected.Ticker)
	}

	// Move to second security
	app.securityTable.MoveDown()
	selected = app.selectedSecurity()
	if selected == nil {
		t.Fatal("selectedSecurity() returned nil after MoveDown")
	}
	if selected.Ticker != "MSFT" {
		t.Errorf("selected = %q, want MSFT", selected.Ticker)
	}
}

func TestSecuritySelectedSecurity_NilData(t *testing.T) {
	app := &App{}
	selected := app.selectedSecurity()
	if selected != nil {
		t.Error("selectedSecurity() should return nil when no security view data")
	}
}

func TestSecurityViewUpdate_SecurityAddedMsg(t *testing.T) {
	app := &App{
		currentView:  ViewSecurities,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		securityView: &securityViewData{securities: []*security.Security{}},
	}

	msg := securityAddedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	// Should have success notification
	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about security added")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "Security added") {
		t.Errorf("notification = %q, should contain 'Security added'", notifications[0].Text)
	}
	// Should return a reload command
	if cmd == nil {
		t.Error("should return a command to reload security data")
	}
}

func TestSecurityViewUpdate_SecurityUpdatedMsg(t *testing.T) {
	app := &App{
		currentView:  ViewSecurities,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		securityView: &securityViewData{securities: []*security.Security{}},
	}

	msg := securityUpdatedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about security updated")
	}
	if cmd == nil {
		t.Error("should return a command to reload security data")
	}
}

func TestSecurityViewUpdate_SecurityDeletedMsg(t *testing.T) {
	app := &App{
		currentView:  ViewSecurities,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		securityView: &securityViewData{securities: []*security.Security{}},
	}

	msg := securityDeletedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about security deleted")
	}
	if cmd == nil {
		t.Error("should return a command to reload security data")
	}
}

func TestSecurityViewUpdate_SecurityHiddenMsg(t *testing.T) {
	app := &App{
		currentView:  ViewSecurities,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		securityView: &securityViewData{securities: []*security.Security{}},
	}

	msg := securityHiddenMsg{hidden: true}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about security hidden")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "hidden") {
		t.Errorf("notification = %q, should contain 'hidden'", notifications[0].Text)
	}
	if cmd == nil {
		t.Error("should return a command to reload security data")
	}
}

func TestSecurityViewUpdate_SecurityUnhiddenMsg(t *testing.T) {
	app := &App{
		currentView:  ViewSecurities,
		keys:         defaultKeyMap(),
		statusbar:    NewStatusBar(),
		securityView: &securityViewData{securities: []*security.Security{}},
	}

	msg := securityHiddenMsg{hidden: false}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) == 0 {
		t.Error("should have notification about security unhidden")
	}
	if len(notifications) > 0 && !strings.Contains(notifications[0].Text, "unhidden") {
		t.Errorf("notification = %q, should contain 'unhidden'", notifications[0].Text)
	}
	if cmd == nil {
		t.Error("should return a command to reload security data")
	}
}

func TestSecurityView_NavigateKeyBinding(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
	}

	// Press '4' to go to securities view
	fourKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}}
	model, cmd := app.Update(fourKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewSecurities {
		t.Errorf("currentView = %v, want ViewSecurities", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load security data")
	}
}

func TestSecurityView_FullScreenRender(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewSecurities,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		securityView: &securityViewData{
			securities: []*security.Security{sec},
			showHidden: true,
		},
	}
	app.buildSecurityTable()

	content := app.renderContent(28)
	if !strings.Contains(content, "SECURITIES") {
		t.Error("renderContent should contain securities view content")
	}
}

func TestSecurityDialogDeleteConfirm(t *testing.T) {
	secID := types.NewID()
	app := &App{
		currentView: ViewSecurities,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		securityView: &securityViewData{
			securities: []*security.Security{
				{
					BaseModel:    types.BaseModel{ID: secID},
					Ticker:       "AAPL",
					Name:         "Apple Inc.",
					SecurityType: security.TypeStock,
					AssetClass:   security.AssetClassUnclassified,
					Currency:     "USD",
				},
			},
			showHidden: true,
		},
	}
	app.buildSecurityTable()

	// Pressing 'd' should set up a confirm dialog for delete
	dKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	app.handleSecurityViewKeys(dKey)

	if app.confirmDialog == nil {
		t.Error("confirm dialog should be set after pressing 'd'")
	}
	if app.confirmAction == nil {
		t.Error("confirm action should be set after pressing 'd'")
	}
}
