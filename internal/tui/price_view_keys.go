// Prices view: key handling for the list, the history, and the search box.

package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/security"
)

// handlePriceViewKeys dispatches key presses to the list- or detail-mode
// handler.
func (a *App) handlePriceViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
func (a *App) handlePriceListKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	tbl := a.priceListTable
	cursorMoved := false
	switch {
	case key.Matches(msg, a.keys.Up):
		if tbl != nil {
			tbl.MoveUp()
			cursorMoved = true
		}
	case key.Matches(msg, a.keys.Down):
		if tbl != nil {
			tbl.MoveDown()
			cursorMoved = true
		}
	case msg.String() == "home" || msg.String() == "g":
		if tbl != nil {
			tbl.MoveToTop()
			cursorMoved = true
		}
	case msg.String() == "end" || msg.String() == "G":
		if tbl != nil {
			tbl.MoveToBottom()
			cursorMoved = true
		}
	case msg.String() == "pgup":
		if tbl != nil {
			tbl.PageUp(max(a.height-10, 1))
			cursorMoved = true
		}
	case msg.String() == "pgdown":
		if tbl != nil {
			tbl.PageDown(max(a.height-10, 1))
			cursorMoved = true
		}
	case key.Matches(msg, a.keys.Search):
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		return a, a.drillIntoSelectedListRow()
	case msg.String() == "u":
		return a, a.startPriceRefresh()
	}
	if cursorMoved {
		return a, a.schedulePriceChartFetch(a.listCursorSecurityID())
	}
	return a, nil
}

// handlePriceDetailKeys handles keys on a single security's price history.
func (a *App) handlePriceDetailKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			d := buildAddPriceDialog(a.priceView.selectedSecurity)
			d.SetVisible(true)
			a.price = priceSurface{modalSurface: modalSurface{dlg: d}, mode: priceDialogModeAdd}
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		p := a.selectedPrice()
		if p != nil && a.priceView.selectedSecurity != nil {
			d := buildEditPriceDialog(a.priceView.selectedSecurity, p)
			d.SetVisible(true)
			a.price = priceSurface{
				modalSurface: modalSurface{dlg: d},
				mode:         priceDialogModeEdit,
				editID:       p.ID,
			}
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
					if a.services.Price == nil {
						return errMsg{err: fmt.Errorf("price service not available")}
					}
					if err := a.services.Price.DeletePrice(priceID); err != nil {
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
		return a, a.startPriceRefresh()
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
func (a *App) handlePriceSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case msg.String() == "backspace":
		if len(a.priceView.searchQuery) > 0 {
			a.priceView.searchQuery = a.priceView.searchQuery[:len(a.priceView.searchQuery)-1]
		}
	case msg.Text != "":
		a.priceView.searchQuery += msg.Text
	}
	return a, nil
}
