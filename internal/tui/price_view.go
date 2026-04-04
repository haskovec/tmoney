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

// priceViewData holds the loaded data for the price management view.
type priceViewData struct {
	securities       []*security.Security
	selectedSecurity *security.Security
	prices           []*price.Price
	searchQuery      string
	searching        bool
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

// loadPriceViewData returns a command that loads price view data.
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

		// Preserve selected security from previous state
		var selectedSec *security.Security
		if a.priceView != nil && a.priceView.selectedSecurity != nil {
			// Find the same security in the refreshed list
			for _, sec := range securities {
				if sec.ID == a.priceView.selectedSecurity.ID {
					selectedSec = sec
					break
				}
			}
		}

		// Default to first non-hidden security if none selected
		if selectedSec == nil {
			for _, sec := range securities {
				if !sec.Hidden {
					selectedSec = sec
					break
				}
			}
		}

		var prices []*price.Price
		if selectedSec != nil {
			prices, err = a.priceSvc.GetPriceHistory(selectedSec.ID, nil, nil)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to load prices: %w", err)}
			}
		}

		data := &priceViewData{
			securities:       securities,
			selectedSecurity: selectedSec,
			prices:           prices,
		}

		return priceViewDataLoadedMsg{data: data}
	}
}

// loadPriceViewDataForSecurity returns a command that loads price data for a specific security.
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
			securities:       securities,
			selectedSecurity: sec,
			prices:           prices,
		}

		return priceViewDataLoadedMsg{data: data}
	}
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

// renderPriceView renders the price management view.
func (a *App) renderPriceView() string {
	if a.priceView == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading prices...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Header: PRICES — TICKER (Name)
	titleText := "PRICES"
	var secInfo string
	if a.priceView.selectedSecurity != nil {
		secInfo = fmt.Sprintf("%s (%s)", a.priceView.selectedSecurity.Ticker, a.priceView.selectedSecurity.Name)
	} else {
		secInfo = "No security selected"
	}
	if a.priceView.searchQuery != "" {
		secInfo += "  Search: " + a.priceView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(secInfo)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(secInfo)
	sections = append(sections, headerRow)

	// Security nav hint
	navHint := "← → change security"
	sections = append(sections, a.styles.Muted.Render("  "+navHint))

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if a.priceView.selectedSecurity == nil {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No security selected"))

		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 3  // title + nav hint + separator
	footerHeight := 1 // hint line
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.priceTable != nil && len(a.priceView.prices) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.priceTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.priceTable.ScrollInfo(tableHeight - 1); info != "" {
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

// handlePriceViewKeys handles key presses in the prices view.
func (a *App) handlePriceViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.priceView == nil {
		return a, nil
	}

	// Handle search mode
	if a.priceView.searching {
		return a.handlePriceSearchKey(msg)
	}

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
			tableHeight := max(a.height-10, 1)
			a.priceTable.PageUp(tableHeight)
		}
	case msg.String() == "pgdown":
		if a.priceTable != nil {
			tableHeight := max(a.height-10, 1)
			a.priceTable.PageDown(tableHeight)
		}
	case key.Matches(msg, a.keys.Left):
		// Cycle to previous security
		return a, a.cyclePriceSecurity(-1)
	case key.Matches(msg, a.keys.Right):
		// Cycle to next security
		return a, a.cyclePriceSecurity(1)
	case key.Matches(msg, a.keys.Search):
		// Enter search mode for security selection
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.New):
		// Open add price dialog
		if a.priceView.selectedSecurity != nil {
			a.priceDialog = buildAddPriceDialog(a.priceView.selectedSecurity)
			a.priceDialogMode = priceDialogModeAdd
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		// Open edit dialog for selected price
		p := a.selectedPrice()
		if p != nil && a.priceView.selectedSecurity != nil {
			a.priceDialog = buildEditPriceDialog(a.priceView.selectedSecurity, p)
			a.priceDialogMode = priceDialogModeEdit
			a.priceDialogEditID = p.ID
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Delete):
		// Delete selected price with confirmation
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
		// Open import dialog
		a.priceImportDialog = buildImportPriceDialog()
		a.priceImportDialog.SetVisible(true)
		return a, nil
	}

	return a, nil
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

// cyclePriceSecurity cycles to the next/previous security and reloads prices.
func (a *App) cyclePriceSecurity(direction int) tea.Cmd {
	if a.priceView == nil || len(a.priceView.securities) == 0 {
		return nil
	}

	// Get non-hidden securities sorted by ticker
	var visible []*security.Security
	for _, sec := range a.priceView.securities {
		if !sec.Hidden {
			visible = append(visible, sec)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		return visible[i].Ticker < visible[j].Ticker
	})

	if len(visible) == 0 {
		return nil
	}

	// Find current index
	currentIdx := 0
	if a.priceView.selectedSecurity != nil {
		for i, sec := range visible {
			if sec.ID == a.priceView.selectedSecurity.ID {
				currentIdx = i
				break
			}
		}
	}

	// Cycle
	newIdx := (currentIdx + direction + len(visible)) % len(visible)
	a.priceView.selectedSecurity = visible[newIdx]

	return a.loadPriceViewDataForSecurity(visible[newIdx])
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
