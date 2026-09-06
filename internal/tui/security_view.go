package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// securityViewData holds the loaded data for the security management view.
type securityViewData struct {
	securities  []*security.Security
	showHidden  bool
	searchQuery string
	searching   bool
}

// filteredSecurities returns securities filtered by hidden status and search query.
func (d *securityViewData) filteredSecurities() []*security.Security {
	var result []*security.Security
	query := strings.ToLower(strings.TrimSpace(d.searchQuery))

	for _, sec := range d.securities {
		// Filter by hidden status
		if !d.showHidden && sec.Hidden {
			continue
		}

		// Filter by search query
		if query != "" {
			tickerMatch := strings.Contains(strings.ToLower(sec.Ticker), query)
			nameMatch := strings.Contains(strings.ToLower(sec.Name), query)
			if !tickerMatch && !nameMatch {
				continue
			}
		}

		result = append(result, sec)
	}

	// Sort by ticker
	sort.Slice(result, func(i, j int) bool {
		return result[i].Ticker < result[j].Ticker
	})

	return result
}

// securityViewDataLoadedMsg is sent when security view data has been loaded.
type securityViewDataLoadedMsg struct {
	data *securityViewData
}

// securityAddedMsg is sent when a security has been added. id carries the new
// security's ID so the view can select it (and scroll it into view) after the
// reload.
type securityAddedMsg struct {
	id types.ID
}

// securityUpdatedMsg is sent when a security has been updated.
type securityUpdatedMsg struct{}

// securityDeletedMsg is sent when a security has been deleted.
type securityDeletedMsg struct{}

// securityHiddenMsg is sent when a security's hidden status changes.
type securityHiddenMsg struct {
	hidden bool
}

// loadSecurityViewData returns a command that loads security view data.
func (a *App) loadSecurityViewData() tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("security service not available")}
		}

		securities, err := a.securitySvc.List(security.Filter{})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		showHidden := false
		if a.securityView != nil {
			showHidden = a.securityView.showHidden
		}

		data := &securityViewData{
			securities: securities,
			showHidden: showHidden,
		}

		return securityViewDataLoadedMsg{data: data}
	}
}

// buildSecurityTable creates and populates the table for the security view.
func (a *App) buildSecurityTable() {
	if a.securityView == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Ticker", Width: 10, Align: widget.AlignLeft},
		{Header: "Name", MinWidth: 15, Align: widget.AlignLeft},
		{Header: "Type", Width: 12, Align: widget.AlignLeft},
		{Header: "Asset Class", Width: 20, Align: widget.AlignLeft},
		{Header: "Currency", Width: 8, Align: widget.AlignCenter},
		{Header: "Status", Width: 8, Align: widget.AlignCenter},
	}

	if a.securityTable == nil {
		a.securityTable = widget.NewTable(columns)
	} else {
		a.securityTable.SetColumns(columns)
	}

	filtered := a.securityView.filteredSecurities()
	rows := make([][]string, len(filtered))
	for i, sec := range filtered {
		rows[i] = a.formatSecurityRow(sec)
	}
	a.securityTable.SetRows(rows)
	a.securityTable.SetFocused(true)

	// After adding a security, move the cursor onto the just-added row by
	// matching its ID. Selecting by ID (not position) lands on the row even
	// though the list is sorted by ticker, and the next render scrolls it into
	// view. The pending ID is cleared only once a matching row is found: this
	// table is also rebuilt synchronously by the 'f' hidden-filter toggle and by
	// search keystrokes, which can fire against the still-stale list in the
	// brief window before the post-add reload lands. Clearing only on a match
	// lets such a stale rebuild pass without consuming the request, so the
	// reload that actually contains the new security still selects it.
	if !a.pendingSecuritySelectID.IsNil() {
		for i, sec := range filtered {
			if sec.ID == a.pendingSecuritySelectID {
				a.securityTable.SetCursor(i)
				a.pendingSecuritySelectID = types.NilID
				break
			}
		}
	}
}

// formatSecurityRow formats a security into a table row.
func (a *App) formatSecurityRow(sec *security.Security) []string {
	status := "Active"
	if sec.Hidden {
		status = "Hidden"
	}

	return []string{
		sec.Ticker,
		sec.Name,
		sec.SecurityType.DisplayName(),
		sec.AssetClass.DisplayName(),
		sec.Currency,
		status,
	}
}

// selectedSecurity returns the currently selected security based on table cursor.
func (a *App) selectedSecurity() *security.Security {
	if a.securityView == nil || a.securityTable == nil {
		return nil
	}

	filtered := a.securityView.filteredSecurities()
	cursor := a.securityTable.Cursor()
	if cursor < 0 || cursor >= len(filtered) {
		return nil
	}
	return filtered[cursor]
}

// renderSecurityView renders the security management view.
func (a *App) renderSecurityView() string {
	if a.securityView == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading securities...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Header: SECURITIES + filter status
	titleText := "SECURITIES"
	hiddenStatus := "Hidden: off"
	if a.securityView.showHidden {
		hiddenStatus = "Hidden: on"
	}
	if a.securityView.searchQuery != "" {
		hiddenStatus += "  Search: " + a.securityView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(hiddenStatus)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(hiddenStatus)
	sections = append(sections, headerRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2  // title + separator
	footerHeight := 1 // hint line
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	filtered := a.securityView.filteredSecurities()
	if a.securityTable != nil && len(filtered) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.securityTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.securityTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No securities found"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// handleSecurityViewKeys handles key presses in the securities view.
func (a *App) handleSecurityViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.securityView == nil {
		return a, nil
	}

	// Handle search mode
	if a.securityView.searching {
		return a.handleSecuritySearchKey(msg)
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		if a.securityTable != nil {
			a.securityTable.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.securityTable != nil {
			a.securityTable.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.securityTable != nil {
			a.securityTable.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.securityTable != nil {
			a.securityTable.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.securityTable != nil {
			tableHeight := max(a.height-10, 1)
			a.securityTable.PageUp(tableHeight)
		}
	case msg.String() == "pgdown":
		if a.securityTable != nil {
			tableHeight := max(a.height-10, 1)
			a.securityTable.PageDown(tableHeight)
		}
	case msg.String() == "f":
		// Toggle hidden filter
		a.securityView.showHidden = !a.securityView.showHidden
		a.buildSecurityTable()
	case key.Matches(msg, a.keys.Search):
		// Enter search mode
		a.securityView.searching = true
		a.securityView.searchQuery = ""
	case key.Matches(msg, a.keys.New):
		// Open add security dialog
		a.securityDialog = buildAddSecurityDialog()
		a.securityDialogMode = securityDialogModeAdd
		a.securityDialog.SetVisible(true)
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		// Open edit dialog for selected security
		sec := a.selectedSecurity()
		if sec != nil {
			a.securityDialog = buildEditSecurityDialog(sec)
			a.securityDialogMode = securityDialogModeEdit
			a.securityDialogEditID = sec.ID
			a.securityDialog.SetVisible(true)
		}
		return a, nil
	case msg.String() == "h":
		// Toggle hidden status of selected security
		sec := a.selectedSecurity()
		if sec != nil {
			return a, a.toggleSecurityHidden(sec)
		}
	case key.Matches(msg, a.keys.Delete):
		// Delete selected security (with confirmation)
		sec := a.selectedSecurity()
		if sec != nil {
			secID := sec.ID
			label := sec.Name
			if sec.Ticker != "" {
				label = fmt.Sprintf("%s (%s)", sec.Ticker, sec.Name)
			}
			a.showConfirmDialog(
				"Delete Security",
				fmt.Sprintf("Delete %s?", label),
				func() tea.Msg {
					if a.securitySvc == nil {
						return errMsg{err: fmt.Errorf("security service not available")}
					}
					if err := a.securitySvc.Delete(secID); err != nil {
						return errMsg{err: err}
					}
					return securityDeletedMsg{}
				},
			)
		}
	case msg.String() == "s":
		// Open stock split dialog for selected security
		sec := a.selectedSecurity()
		if sec != nil {
			secID := sec.ID
			a.stockSplitDialogPreSelectedID = &secID
		}
		return a, a.loadStockSplitDialogData()
	case msg.String() == "m":
		// Open merger dialog for selected security
		sec := a.selectedSecurity()
		if sec != nil {
			secID := sec.ID
			a.mergerDialogPreSelectedID = &secID
		}
		return a, a.loadMergerDialogData()
	case msg.String() == "o":
		// Open spin-off dialog for selected security
		sec := a.selectedSecurity()
		if sec != nil {
			secID := sec.ID
			a.spinOffDialogPreSelectedID = &secID
		}
		return a, a.loadSpinOffDialogData()
	case msg.String() == "p":
		// Navigate to prices view for selected security
		sec := a.selectedSecurity()
		if sec != nil {
			a.switchView(ViewPrices)
			return a, a.loadPriceViewDataForSecurity(sec)
		}
	case msg.String() == "u":
		// Refresh prices for all visible securities from the default provider.
		return a, a.startPriceRefresh()
	case msg.String() == "a":
		// Open the global corporate-action register, pre-filtered to the
		// highlighted ticker (so it acts as a per-security drill-in).
		if sec := a.selectedSecurity(); sec != nil {
			a.corporateActionViewFilter = sec.Ticker
		} else {
			a.corporateActionViewFilter = ""
		}
		a.switchView(ViewCorporateActions)
		return a, a.loadCorporateActionViewData()
	}

	return a, nil
}

// handleSecuritySearchKey handles key presses while in search mode.
func (a *App) handleSecuritySearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.securityView.searching = false
		a.securityView.searchQuery = ""
		a.buildSecurityTable()
	case key.Matches(msg, a.keys.Enter):
		a.securityView.searching = false
		// Keep the search query active
	case msg.String() == "backspace":
		if len(a.securityView.searchQuery) > 0 {
			a.securityView.searchQuery = a.securityView.searchQuery[:len(a.securityView.searchQuery)-1]
			a.buildSecurityTable()
		}
	case msg.Text != "":
		a.securityView.searchQuery += msg.Text
		a.buildSecurityTable()
	}
	return a, nil
}

// toggleSecurityHidden toggles the hidden status of a security.
func (a *App) toggleSecurityHidden(sec *security.Security) tea.Cmd {
	hidden := sec.Hidden
	secID := sec.ID
	return func() tea.Msg {
		if a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("security service not available")}
		}
		var err error
		if hidden {
			err = a.securitySvc.Unhide(secID)
		} else {
			err = a.securitySvc.Hide(secID)
		}
		if err != nil {
			return errMsg{err: err}
		}
		return securityHiddenMsg{hidden: !hidden}
	}
}

// Security dialog types and builders

type securityDialogMode int

const (
	securityDialogModeAdd securityDialogMode = iota
	securityDialogModeEdit
)

// buildAddSecurityDialog builds the dialog for adding a new security.
func buildAddSecurityDialog() *dialog.Dialog {
	d := dialog.NewDialog("Add Security")
	d.SetWidth(76)

	d.AddTextField("Ticker", "", "e.g. AAPL (optional)", 20)

	f := d.AddTextField("Name", "", "e.g. Apple Inc.", 50)
	f.Required = true

	d.AddTextField("ISIN", "", "e.g. US0378331005 (optional)", 12)

	typeOptions := make([]string, len(security.AllTypes()))
	for i, t := range security.AllTypes() {
		typeOptions[i] = t.DisplayName()
	}
	d.AddSelectField("Type", typeOptions, 0)

	acOptions := make([]string, len(security.AllAssetClasses()))
	for i, ac := range security.AllAssetClasses() {
		acOptions[i] = ac.DisplayName()
	}
	d.AddSelectField("Asset Class", acOptions, len(acOptions)-1) // Default: Unclassified (last)

	currencyOptions := []string{"USD", "EUR", "GBP", "CAD", "JPY", "CHF", "AUD"}
	d.AddSelectField("Currency", currencyOptions, 0) // Default: USD

	d.AddTextField("Exchange", "", "e.g. NASDAQ", 20)

	d.SetButtons([]dialog.DialogButton{
		{Label: "Save", Primary: true},
		{Label: "Cancel"},
	})

	return d
}

// buildEditSecurityDialog builds the dialog for editing an existing security.
func buildEditSecurityDialog(sec *security.Security) *dialog.Dialog {
	d := dialog.NewDialog("Edit Security")
	d.SetWidth(76)

	d.AddTextField("Ticker", sec.Ticker, "e.g. AAPL (optional)", 20)

	f := d.AddTextField("Name", sec.Name, "e.g. Apple Inc.", 50)
	f.Required = true

	d.AddTextField("ISIN", sec.ISIN, "e.g. US0378331005 (optional)", 12)

	// Type selection
	allTypes := security.AllTypes()
	typeOptions := make([]string, len(allTypes))
	selectedType := 0
	for i, t := range allTypes {
		typeOptions[i] = t.DisplayName()
		if t == sec.SecurityType {
			selectedType = i
		}
	}
	d.AddSelectField("Type", typeOptions, selectedType)

	// Asset class selection
	allClasses := security.AllAssetClasses()
	acOptions := make([]string, len(allClasses))
	selectedClass := 0
	for i, ac := range allClasses {
		acOptions[i] = ac.DisplayName()
		if ac == sec.AssetClass {
			selectedClass = i
		}
	}
	d.AddSelectField("Asset Class", acOptions, selectedClass)

	// Currency selection
	currencyOptions := []string{"USD", "EUR", "GBP", "CAD", "JPY", "CHF", "AUD"}
	selectedCurrency := 0
	for i, c := range currencyOptions {
		if c == sec.Currency {
			selectedCurrency = i
			break
		}
	}
	d.AddSelectField("Currency", currencyOptions, selectedCurrency)

	exchange := ""
	if sec.Exchange.Valid {
		exchange = sec.Exchange.String
	}
	d.AddTextField("Exchange", exchange, "e.g. NASDAQ", 20)

	d.SetButtons([]dialog.DialogButton{
		{Label: "Save", Primary: true},
		{Label: "Cancel"},
	})

	return d
}

// handleSecurityDialogKey handles key presses in the security add/edit dialog.
func (a *App) handleSecurityDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.securityDialogAction(a.securityDialog.HandleKey(msg))
}

// securityDialogAction dispatches a DialogAction for the security dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) securityDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.securityDialog.SetVisible(false)
		a.securityDialog = nil
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitSecurityDialog()
	}
	return a, nil
}

// submitSecurityDialog processes the security dialog submission.
func (a *App) submitSecurityDialog() (tea.Model, tea.Cmd) {
	fields := a.securityDialog.Fields()

	ticker := strings.TrimSpace(fields[0].Value)
	name := strings.TrimSpace(fields[1].Value)
	isin := strings.TrimSpace(fields[2].Value)

	if name == "" {
		a.securityDialog.SetErrorMsg("Name is required.")
		return a, nil
	}
	if isin != "" && !security.IsValidISIN(isin) {
		a.securityDialog.SetErrorMsg("ISIN is not a valid ISO 6166 identifier.")
		return a, nil
	}

	allTypes := security.AllTypes()
	secType := allTypes[fields[3].SelectedIndex]

	allClasses := security.AllAssetClasses()
	assetClass := allClasses[fields[4].SelectedIndex]

	currencyOptions := []string{"USD", "EUR", "GBP", "CAD", "JPY", "CHF", "AUD"}
	currency := currencyOptions[fields[5].SelectedIndex]

	exchange := strings.TrimSpace(fields[6].Value)

	a.securityDialog.SetVisible(false)
	a.securityDialog = nil

	if a.securityDialogMode == securityDialogModeAdd {
		return a, a.createSecurity(ticker, name, isin, secType, assetClass, currency, exchange)
	}
	return a, a.updateSecurity(a.securityDialogEditID, ticker, name, isin, secType, assetClass, currency, exchange)
}

// createSecurity creates a new security via the service.
func (a *App) createSecurity(ticker, name, isin string, secType security.Type, assetClass security.AssetClass, currency, exchange string) tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("security service not available")}
		}

		sec := security.NewSecurity(ticker, name, secType)
		sec.AssetClass = assetClass
		sec.Currency = currency
		sec.SetExchange(exchange)
		sec.SetISIN(isin)

		if err := a.securitySvc.Create(sec); err != nil {
			return errMsg{err: err}
		}
		return securityAddedMsg{id: sec.ID}
	}
}

// updateSecurity updates an existing security via the service.
func (a *App) updateSecurity(id types.ID, ticker, name, isin string, secType security.Type, assetClass security.AssetClass, currency, exchange string) tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("security service not available")}
		}

		sec, err := a.securitySvc.GetByID(id)
		if err != nil {
			return errMsg{err: err}
		}

		sec.Ticker = ticker
		sec.Name = name
		sec.SecurityType = secType
		sec.AssetClass = assetClass
		sec.Currency = currency
		sec.SetExchange(exchange)
		sec.SetISIN(isin)

		if err := a.securitySvc.Update(sec); err != nil {
			return errMsg{err: err}
		}
		return securityUpdatedMsg{}
	}
}
