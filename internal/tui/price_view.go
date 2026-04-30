package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// pricesViewMode is the prices view's top-level mode: a summary list of
// the latest price per security, or the price history of a single
// drilled-in security.
type pricesViewMode int

const (
	pricesViewList   pricesViewMode = iota // landing page: latest price per ticker
	pricesViewDetail                       // price history for one security
)

// priceViewData holds the loaded data for the price management view.
// Both list and detail modes share this struct; mode tells which slice
// is the source of truth.
type priceViewData struct {
	mode pricesViewMode

	// List mode: the most recent price per non-hidden security with
	// any prices, sorted by ticker. selectedSecurity is nil here.
	latestPrices []*price.LatestPrice

	// Detail mode: full history for selectedSecurity (newest first).
	selectedSecurity *security.Security
	prices           []*price.Price

	// Shared
	securities  []*security.Security
	searchQuery string
	searching   bool
}

// filteredSecurities returns securities filtered by search query (hidden excluded).
func (d *priceViewData) filteredSecurities() []*security.Security {
	var result []*security.Security
	query := strings.ToLower(strings.TrimSpace(d.searchQuery))

	for _, sec := range d.securities {
		if sec.Hidden {
			continue
		}
		if query != "" {
			tickerMatch := strings.Contains(strings.ToLower(sec.Ticker), query)
			nameMatch := strings.Contains(strings.ToLower(sec.Name), query)
			if !tickerMatch && !nameMatch {
				continue
			}
		}
		result = append(result, sec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Ticker < result[j].Ticker
	})

	return result
}

// Message types

type priceViewDataLoadedMsg struct {
	data *priceViewData
}

type priceAddedMsg struct{}
type priceUpdatedMsg struct{}
type priceDeletedMsg struct{}

type priceImportedMsg struct {
	total    int
	imported int
	skipped  int
}

// loadPriceViewData returns a command that loads the prices landing page:
// the list of latest prices per non-hidden security with any prices.
func (a *App) loadPriceViewData() tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil || a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		latest, err := a.priceSvc.GetLatestPrices()
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load latest prices: %w", err)}
		}

		data := &priceViewData{
			mode:         pricesViewList,
			securities:   securities,
			latestPrices: latest,
		}

		return priceViewDataLoadedMsg{data: data}
	}
}

// reloadPriceViewKeepingMode refreshes the prices view in whichever mode
// it is currently showing. Used after a CRUD operation so the user stays
// in detail mode (instead of being kicked back to the landing list) when
// they add/edit/delete a price for a specific ticker.
func (a *App) reloadPriceViewKeepingMode() tea.Cmd {
	if a.priceView != nil && a.priceView.mode == pricesViewDetail && a.priceView.selectedSecurity != nil {
		return a.loadPriceViewDataForSecurity(a.priceView.selectedSecurity)
	}
	return a.loadPriceViewData()
}

// loadPriceViewDataForSecurity returns a command that drills into a single
// security's price history (detail mode).
func (a *App) loadPriceViewDataForSecurity(sec *security.Security) tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil || a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		var prices []*price.Price
		if sec != nil {
			prices, err = a.priceSvc.GetPriceHistory(sec.ID, nil, nil)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to load prices: %w", err)}
			}
		}

		data := &priceViewData{
			mode:             pricesViewDetail,
			securities:       securities,
			selectedSecurity: sec,
			prices:           prices,
		}

		return priceViewDataLoadedMsg{data: data}
	}
}

// buildPriceListTable creates and populates the list-mode summary table
// (one row per security with its latest price).
func (a *App) buildPriceListTable() {
	if a.priceView == nil {
		return
	}

	columns := []Column{
		{Header: "Ticker", Width: 10, Align: AlignLeft},
		{Header: "Name", Width: 32, Align: AlignLeft},
		{Header: "Latest Price", Width: 15, Align: AlignRight},
		{Header: "Date", Width: 12, Align: AlignLeft},
	}

	if a.priceListTable == nil {
		a.priceListTable = NewTable(columns)
	} else {
		a.priceListTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.priceView.latestPrices))
	for i, lp := range a.priceView.latestPrices {
		rows[i] = []string{
			lp.Ticker,
			lp.Name,
			fmt.Sprintf("$%.2f", lp.Price.Float64()),
			lp.Date.Time().Format("2006-01-02"),
		}
	}
	a.priceListTable.SetRows(rows)
	a.priceListTable.SetFocused(true)
}

// buildPriceTable creates and populates the table for the price view.
func (a *App) buildPriceTable() {
	if a.priceView == nil {
		return
	}

	columns := []Column{
		{Header: "Date", Width: 12, Align: AlignLeft},
		{Header: "Price", Width: 15, Align: AlignRight},
		{Header: "Source", Width: 12, Align: AlignLeft},
	}

	if a.priceTable == nil {
		a.priceTable = NewTable(columns)
	} else {
		a.priceTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.priceView.prices))
	for i, p := range a.priceView.prices {
		rows[i] = a.formatPriceRow(p)
	}
	a.priceTable.SetRows(rows)
	a.priceTable.SetFocused(true)
}

// formatPriceRow formats a price into a table row.
func (a *App) formatPriceRow(p *price.Price) []string {
	return []string{
		p.Date.Time().Format("2006-01-02"),
		fmt.Sprintf("$%.2f", p.Price.Float64()),
		p.Source.DisplayName(),
	}
}

// selectedPrice returns the currently selected price based on table cursor.
func (a *App) selectedPrice() *price.Price {
	if a.priceView == nil || a.priceTable == nil {
		return nil
	}

	cursor := a.priceTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.prices) {
		return nil
	}
	return a.priceView.prices[cursor]
}

// renderPriceView renders the prices view in either list or detail mode.
func (a *App) renderPriceView() string {
	if a.priceView == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading prices...")
	}

	if a.priceView.mode == pricesViewDetail {
		return a.renderPriceDetail()
	}
	return a.renderPriceList()
}

// renderPriceList renders the landing-page summary table.
func (a *App) renderPriceList() string {
	contentWidth := a.styles.ContentWidth()

	var sections []string

	titleText := "PRICES"
	hint := "Enter: view history  ·  u: update prices  ·  /: search"
	if a.priceView.searchQuery != "" {
		hint += "  Search: " + a.priceView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(hint)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(hint)
	sections = append(sections, headerRow)

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if len(a.priceView.latestPrices) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No prices on file. Press 'p' on a security to start."))
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 // title + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.priceListTable != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.priceListTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.priceListTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderPriceDetail renders the per-security price history (drill-in).
func (a *App) renderPriceDetail() string {
	contentWidth := a.styles.ContentWidth()

	var sections []string

	titleText := "PRICES"
	var secInfo string
	if a.priceView.selectedSecurity != nil {
		secInfo = fmt.Sprintf("%s (%s)", a.priceView.selectedSecurity.Ticker, a.priceView.selectedSecurity.Name)
	}
	if a.priceView.searchQuery != "" {
		secInfo += "  Search: " + a.priceView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(secInfo)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(secInfo)
	sections = append(sections, headerRow)

	sections = append(sections, a.styles.Muted.Render("  Esc: back to list"))

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 3 // title + back hint + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.priceTable != nil && len(a.priceView.prices) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.priceTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.priceTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No prices found"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// handlePriceViewKeys dispatches key presses to the list- or detail-mode
// handler.
func (a *App) handlePriceViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.priceView == nil {
		return a, nil
	}
	if a.priceView.searching {
		return a.handlePriceSearchKey(msg)
	}
	if a.priceView.mode == pricesViewDetail {
		return a.handlePriceDetailKeys(msg)
	}
	return a.handlePriceListKeys(msg)
}

// handlePriceListKeys handles keys on the prices landing page.
func (a *App) handlePriceListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tbl := a.priceListTable
	switch {
	case key.Matches(msg, a.keys.Up):
		if tbl != nil {
			tbl.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if tbl != nil {
			tbl.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if tbl != nil {
			tbl.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if tbl != nil {
			tbl.MoveToBottom()
		}
	case msg.String() == "pgup":
		if tbl != nil {
			tbl.PageUp(max(a.height-10, 1))
		}
	case msg.String() == "pgdown":
		if tbl != nil {
			tbl.PageDown(max(a.height-10, 1))
		}
	case key.Matches(msg, a.keys.Search):
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		return a, a.drillIntoSelectedListRow()
	case msg.String() == "u":
		return a, a.refreshPricesCmd()
	}
	return a, nil
}

// handlePriceDetailKeys handles keys on a single security's price history.
func (a *App) handlePriceDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		if a.priceTable != nil {
			a.priceTable.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.priceTable != nil {
			a.priceTable.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.priceTable != nil {
			a.priceTable.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.priceTable != nil {
			a.priceTable.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.priceTable != nil {
			a.priceTable.PageUp(max(a.height-10, 1))
		}
	case msg.String() == "pgdown":
		if a.priceTable != nil {
			a.priceTable.PageDown(max(a.height-10, 1))
		}
	case key.Matches(msg, a.keys.Escape):
		// Flip back to list mode synchronously so the next render is the
		// landing page; loadPriceViewData refreshes the data behind it.
		a.priceView.mode = pricesViewList
		a.priceView.selectedSecurity = nil
		a.priceView.prices = nil
		return a, a.loadPriceViewData()
	case key.Matches(msg, a.keys.Search):
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.New):
		if a.priceView.selectedSecurity != nil {
			a.priceDialog = buildAddPriceDialog(a.priceView.selectedSecurity)
			a.priceDialogMode = priceDialogModeAdd
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		p := a.selectedPrice()
		if p != nil && a.priceView.selectedSecurity != nil {
			a.priceDialog = buildEditPriceDialog(a.priceView.selectedSecurity, p)
			a.priceDialogMode = priceDialogModeEdit
			a.priceDialogEditID = p.ID
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Delete):
		p := a.selectedPrice()
		if p != nil {
			priceID := p.ID
			dateStr := p.Date.Time().Format("2006-01-02")
			a.showConfirmDialog(
				"Delete Price",
				fmt.Sprintf("Delete price for %s?", dateStr),
				func() tea.Msg {
					if a.priceSvc == nil {
						return errMsg{err: fmt.Errorf("price service not available")}
					}
					if err := a.priceSvc.DeletePrice(priceID); err != nil {
						return errMsg{err: err}
					}
					return priceDeletedMsg{}
				},
			)
		}
	case msg.String() == "i":
		a.priceImportDialog = buildImportPriceDialog()
		a.priceImportDialog.SetVisible(true)
		return a, nil
	case msg.String() == "u":
		return a, a.refreshPricesCmd()
	}
	return a, nil
}

// drillIntoSelectedListRow loads detail mode for the security at the
// list-table cursor.
func (a *App) drillIntoSelectedListRow() tea.Cmd {
	if a.priceView == nil || a.priceListTable == nil {
		return nil
	}
	cursor := a.priceListTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.latestPrices) {
		return nil
	}
	targetID := a.priceView.latestPrices[cursor].SecurityID

	// Resolve to a *security.Security from the cached list so the loader
	// can populate ticker/name without an extra round-trip.
	var sec *security.Security
	for _, s := range a.priceView.securities {
		if s.ID == targetID {
			sec = s
			break
		}
	}
	if sec == nil {
		// Fall back: synthesize from the LatestPrice row.
		sec = &security.Security{Ticker: a.priceView.latestPrices[cursor].Ticker, Name: a.priceView.latestPrices[cursor].Name}
		sec.ID = targetID
	}
	a.priceView.mode = pricesViewDetail
	a.priceView.selectedSecurity = sec
	return a.loadPriceViewDataForSecurity(sec)
}

// handlePriceSearchKey handles key presses while in search mode.
func (a *App) handlePriceSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.priceView.searching = false
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		a.priceView.searching = false
		// Select the first filtered security
		filtered := a.priceView.filteredSecurities()
		if len(filtered) > 0 {
			a.priceView.selectedSecurity = filtered[0]
			a.priceView.searchQuery = ""
			return a, a.loadPriceViewDataForSecurity(filtered[0])
		}
		a.priceView.searchQuery = ""
	case msg.Type == tea.KeyBackspace:
		if len(a.priceView.searchQuery) > 0 {
			a.priceView.searchQuery = a.priceView.searchQuery[:len(a.priceView.searchQuery)-1]
		}
	case msg.Type == tea.KeyRunes:
		a.priceView.searchQuery += string(msg.Runes)
	}
	return a, nil
}

// Price dialog types and builders

type priceDialogMode int

const (
	priceDialogModeAdd priceDialogMode = iota
	priceDialogModeEdit
)

// buildAddPriceDialog builds the dialog for adding a new price.
func buildAddPriceDialog(sec *security.Security) *Dialog {
	d := NewDialog(fmt.Sprintf("Add Price — %s", sec.Ticker))

	today := time.Now().Format("2006-01-02")
	f := d.AddTextField("Date", today, "YYYY-MM-DD", 12)
	f.Required = true

	f = d.AddTextField("Price", "", "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Save", Primary: true},
	})

	return d
}

// buildEditPriceDialog builds the dialog for editing an existing price.
func buildEditPriceDialog(sec *security.Security, p *price.Price) *Dialog {
	d := NewDialog(fmt.Sprintf("Edit Price — %s", sec.Ticker))

	dateStr := p.Date.Time().Format("2006-01-02")
	f := d.AddTextField("Date", dateStr, "YYYY-MM-DD", 12)
	f.Required = true

	priceStr := fmt.Sprintf("%.2f", p.Price.Float64())
	f = d.AddTextField("Price", priceStr, "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Save", Primary: true},
	})

	return d
}

// handlePriceDialogKey handles key presses in the price add/edit dialog.
func (a *App) handlePriceDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := a.priceDialog.HandleKey(msg)
	switch action {
	case DialogActionCancel:
		a.priceDialog.SetVisible(false)
		a.priceDialog = nil
		return a, nil
	case DialogActionSubmit:
		return a.submitPriceDialog()
	}
	return a, nil
}

// submitPriceDialog processes the price dialog submission.
func (a *App) submitPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.priceDialog.Fields()

	dateStr := strings.TrimSpace(fields[0].Value)
	priceStr := strings.TrimSpace(fields[1].Value)

	if dateStr == "" {
		a.priceDialog.SetErrorMsg("Date is required.")
		return a, nil
	}
	if priceStr == "" {
		a.priceDialog.SetErrorMsg("Price is required.")
		return a, nil
	}

	date, err := types.ParseDate(dateStr)
	if err != nil {
		a.priceDialog.SetErrorMsg("Invalid date format. Use YYYY-MM-DD.")
		return a, nil
	}

	amount, err := types.NewMoney(priceStr)
	if err != nil {
		a.priceDialog.SetErrorMsg("Invalid price value.")
		return a, nil
	}

	if amount.IsZero() || !amount.IsPositive() {
		a.priceDialog.SetErrorMsg("Price must be positive.")
		return a, nil
	}

	secID := a.priceView.selectedSecurity.ID
	mode := a.priceDialogMode
	editID := a.priceDialogEditID

	a.priceDialog.SetVisible(false)
	a.priceDialog = nil

	if mode == priceDialogModeAdd {
		return a, a.createPrice(secID, date, amount)
	}
	return a, a.updatePrice(editID, secID, date, amount)
}

// createPrice creates a new price via the service.
func (a *App) createPrice(securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		if err := a.priceSvc.AddPrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceAddedMsg{}
	}
}

// updatePrice updates an existing price via the service.
func (a *App) updatePrice(id, securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		p.ID = id
		if err := a.priceSvc.UpdatePrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceUpdatedMsg{}
	}
}

// Bulk import dialog

// buildImportPriceDialog builds the dialog for importing prices from CSV.
func buildImportPriceDialog() *Dialog {
	d := NewDialog("Import Prices")

	f := d.AddTextField("CSV File", "", "Path to CSV file", 0)
	f.Required = true

	d.AddCheckboxField("Overwrite existing", false)

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Import", Primary: true},
	})

	return d
}

// handlePriceImportDialogKey handles key presses in the import dialog.
func (a *App) handlePriceImportDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := a.priceImportDialog.HandleKey(msg)
	switch action {
	case DialogActionCancel:
		a.priceImportDialog.SetVisible(false)
		a.priceImportDialog = nil
		return a, nil
	case DialogActionSubmit:
		return a.submitImportPriceDialog()
	}
	return a, nil
}

// submitImportPriceDialog processes the import dialog submission.
func (a *App) submitImportPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.priceImportDialog.Fields()

	filePath := strings.TrimSpace(fields[0].Value)
	if filePath == "" {
		a.priceImportDialog.SetErrorMsg("CSV file path is required.")
		return a, nil
	}

	overwrite := fields[1].Checked

	a.priceImportDialog.SetVisible(false)
	a.priceImportDialog = nil

	return a, a.importPrices(filePath, overwrite)
}

// importPrices imports prices from a CSV file.
func (a *App) importPrices(filePath string, overwrite bool) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil || a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		f, err := os.Open(filePath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to open file: %w", err)}
		}
		defer f.Close()

		result, err := imexport.ParsePriceCSV(f)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to parse CSV: %w", err)}
		}

		if result.HasErrors() {
			var msgs []string
			for _, e := range result.Errors {
				msgs = append(msgs, fmt.Sprintf("line %d: %s", e.Line, e.Message))
			}
			return errMsg{err: fmt.Errorf("CSV errors:\n%s", strings.Join(msgs, "\n"))}
		}

		// Resolve tickers to security IDs
		var prices []*price.Price
		for _, rec := range result.Records {
			sec, lookupErr := a.securitySvc.GetByTicker(rec.Ticker, "USD")
			if lookupErr != nil {
				return errMsg{err: fmt.Errorf("unknown ticker %q (line %d): %w", rec.Ticker, rec.SourceLine, lookupErr)}
			}
			prices = append(prices, price.NewPrice(sec.ID, rec.Date, rec.Price, price.SourceImport))
		}

		importResult, err := a.priceSvc.BulkImport(prices, overwrite)
		if err != nil {
			return errMsg{err: fmt.Errorf("import failed: %w", err)}
		}

		return priceImportedMsg{
			total:    importResult.Total,
			imported: importResult.Imported,
			skipped:  importResult.Skipped,
		}
	}
}
