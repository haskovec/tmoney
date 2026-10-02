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
	if a.prices.data == nil {
		return a, nil
	}
	if a.prices.data.searching {
		return a, a.prices.handleSearchKey(msg, a.keys, a.priceDeps())
	}
	if a.prices.data.mode == pricesViewDetail {
		return a.handlePriceDetailKeys(msg)
	}
	return a.handlePriceListKeys(msg)
}

// handlePriceListKeys handles keys on the prices landing page.
func (a *App) handlePriceListKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	tbl := a.prices.listTable
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
		a.prices.data.searching = true
		a.prices.data.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		return a, a.prices.drillIntoSelectedListRow(a.priceDeps())
	case msg.String() == "u":
		return a, a.startPriceRefresh()
	}
	if cursorMoved {
		return a, a.prices.scheduleChartFetch(a.prices.listCursorSecurityID())
	}
	return a, nil
}

// handlePriceDetailKeys handles keys on a single security's price history.
func (a *App) handlePriceDetailKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		if a.prices.table != nil {
			a.prices.table.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.prices.table != nil {
			a.prices.table.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.prices.table != nil {
			a.prices.table.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.prices.table != nil {
			a.prices.table.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.prices.table != nil {
			a.prices.table.PageUp(max(a.height-10, 1))
		}
	case msg.String() == "pgdown":
		if a.prices.table != nil {
			a.prices.table.PageDown(max(a.height-10, 1))
		}
	case key.Matches(msg, a.keys.Escape):
		// Flip back to list mode synchronously so the next render is the
		// landing page; load refreshes the data behind it.
		a.prices.data.mode = pricesViewList
		a.prices.data.selectedSecurity = nil
		a.prices.data.prices = nil
		return a, a.prices.load(a.priceDeps())
	case key.Matches(msg, a.keys.Search):
		a.prices.data.searching = true
		a.prices.data.searchQuery = ""
	case key.Matches(msg, a.keys.New):
		if a.prices.data.selectedSecurity != nil {
			d := buildAddPriceDialog(a.prices.data.selectedSecurity)
			d.SetVisible(true)
			a.price = priceSurface{modalSurface: modalSurface{dlg: d}, mode: priceDialogModeAdd}
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		p := a.prices.selectedPrice()
		if p != nil && a.prices.data.selectedSecurity != nil {
			d := buildEditPriceDialog(a.prices.data.selectedSecurity, p)
			d.SetVisible(true)
			a.price = priceSurface{
				modalSurface: modalSurface{dlg: d},
				mode:         priceDialogModeEdit,
				editID:       p.ID,
			}
		}
		return a, nil
	case key.Matches(msg, a.keys.Delete):
		p := a.prices.selectedPrice()
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
func (s *priceViewState) drillIntoSelectedListRow(d priceDeps) tea.Cmd {
	if s.data == nil || s.listTable == nil {
		return nil
	}
	cursor := s.listTable.Cursor()
	if cursor < 0 || cursor >= len(s.data.latestPrices) {
		return nil
	}
	targetID := s.data.latestPrices[cursor].SecurityID

	// Resolve to a *security.Security from the cached list so the loader
	// can populate ticker/name without an extra round-trip.
	var sec *security.Security
	for _, cand := range s.data.securities {
		if cand.ID == targetID {
			sec = cand
			break
		}
	}
	if sec == nil {
		// Fall back: synthesize from the LatestPrice row.
		sec = &security.Security{Ticker: s.data.latestPrices[cursor].Ticker, Name: s.data.latestPrices[cursor].Name}
		sec.ID = targetID
	}
	s.data.mode = pricesViewDetail
	s.data.selectedSecurity = sec
	return s.loadForSecurity(d, sec)
}

// handleSearchKey handles key presses while in search mode.
func (s *priceViewState) handleSearchKey(msg tea.KeyPressMsg, keys keyMap, d priceDeps) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Escape):
		s.data.searching = false
		s.data.searchQuery = ""
	case key.Matches(msg, keys.Enter):
		s.data.searching = false
		// Select the first filtered security
		filtered := s.data.filteredSecurities()
		if len(filtered) > 0 {
			s.data.selectedSecurity = filtered[0]
			s.data.searchQuery = ""
			return s.loadForSecurity(d, filtered[0])
		}
		s.data.searchQuery = ""
	case msg.String() == "backspace":
		if len(s.data.searchQuery) > 0 {
			s.data.searchQuery = s.data.searchQuery[:len(s.data.searchQuery)-1]
		}
	case msg.Text != "":
		s.data.searchQuery += msg.Text
	}
	return nil
}
