// Account register: the search (the `/` key, or Transactions → Search…), its
// matching, its status line, and its key handler.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// searchActive reports whether the search shows on the register: the user is
// typing a query, or kept one with Enter.
func (s *registerViewState) searchActive() bool {
	return s.searching || strings.TrimSpace(s.query) != ""
}

// resetSearch ends the search and drops its query.
func (s *registerViewState) resetSearch() {
	s.searching = false
	s.query = ""
}

// rowMatches reports whether txn matches the lower-cased query: the query is
// in its date, payee, category or amount as the register shows them (cells),
// or in its memo or check number. An amount also matches as a plain number,
// and with the query's thousands commas dropped: "-1500" and "1,500" both
// find the row shown as -$1500.00.
func (s *registerViewState) rowMatches(txn *transaction.Transaction, cells []string, lowerQuery string) bool {
	texts := append(cells[:0:0], cells...)
	texts = append(texts, txn.Memo.String, txn.CheckNumber.String, txn.Amount.String())
	for _, t := range texts {
		if strings.Contains(strings.ToLower(t), lowerQuery) {
			return true
		}
	}
	digits := strings.ReplaceAll(lowerQuery, ",", "")
	if digits == lowerQuery {
		return false
	}
	for _, amount := range []string{cells[registerAmountCol], txn.Amount.String()} {
		if strings.Contains(amount, digits) {
			return true
		}
	}
	return false
}

// holds reports whether the loaded register has the transaction id.
func (s *registerViewState) holds(id types.ID) bool {
	if s.data == nil {
		return false
	}
	for _, txn := range s.data.transactions {
		if txn.ID == id {
			return true
		}
	}
	return false
}

// selectSavedRegisterRow puts the cursor on the row that a save asked for,
// matched by ID, so a back-dated row in the middle of the list is found too.
//
// It runs only when a reload lands. Other rebuilds (a resize, a search key,
// leaving the register) can run on the ledger from before the save, where the
// saved row is missing or at its old position. A reload that does not hold
// the row keeps the request for the next reload.
//
// When a kept search hides the saved row, the search ends, so the row shows.
// While the user types a new search, the query has priority and the request
// is dropped.
func (a *App) selectSavedRegisterRow() {
	id := a.pendingRegisterSelectID
	if id.IsNil() || !a.register.holds(id) {
		return
	}
	a.pendingRegisterSelectID = types.NilID
	if a.register.selectRow(id) || a.register.searching {
		return
	}
	a.register.resetSearch()
	a.buildRegisterTable()
	a.register.selectRow(id)
}

// selectRow puts the cursor on the row of the transaction id. It reports
// false when no row shows that transaction.
func (s *registerViewState) selectRow(id types.ID) bool {
	for i, txn := range s.shown {
		if txn.ID == id {
			s.table.SetCursor(i)
			return true
		}
	}
	return false
}

// searchLine returns the line under the register title while the search is
// active, cut to width cells. The query is cut first, so the count stays in
// view.
func (s *registerViewState) searchLine(width int) string {
	q := strings.TrimSpace(s.query)
	head, tail := "Search: (type to search — Enter keeps · Esc clears)", ""
	if q != "" {
		head = "Search: " + q
		if s.data != nil && len(s.shown) == 0 {
			tail = "  →  no matches"
		} else if s.data != nil {
			tail = fmt.Sprintf("  (%d of %d)", len(s.shown), len(s.data.transactions))
		}
	}
	if lipgloss.Width(head+tail) <= width {
		return head + tail
	}
	return ansi.Truncate(head, max(width-lipgloss.Width(tail), 0), "…") + tail
}

// startRegisterSearch starts a new search. It drops a kept query and shows
// every row with the cursor on the same transaction, as Esc does. The rows
// stay that way until the first character is typed.
func (a *App) startRegisterSearch() {
	a.clearRegisterSearch()
	a.register.searching = true
}

// clearRegisterSearch ends the search and shows every row again. The cursor
// stays on the transaction it was on, so a search can find a row to work on
// in the full register.
func (a *App) clearRegisterSearch() {
	sel := a.register.selectedTransaction()
	a.register.resetSearch()
	a.buildRegisterTable()
	if sel != nil {
		a.register.selectRow(sel.ID)
	}
}

// handleRegisterSearchKey edits the query while the user types it. Text and
// Backspace change the query and move the cursor to the first match. The
// arrow, page, Home and End keys move through the matches. Enter keeps the
// query and gives the keys back to the register; Esc clears the search.
func (a *App) handleRegisterSearchKey(msg tea.KeyPressMsg) {
	s := &a.register
	if s.table == nil {
		return
	}
	retype := func() {
		a.buildRegisterTable()
		s.table.SetCursor(0)
	}
	page := max(a.height-6, 1)

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.clearRegisterSearch()
	case key.Matches(msg, a.keys.Enter):
		s.searching = false
		if strings.TrimSpace(s.query) == "" {
			a.clearRegisterSearch()
		}
	// The arrows match on msg.Code, not on keys.Up/Down: their "k" and "j"
	// aliases are query text here.
	case msg.Code == tea.KeyUp:
		s.table.MoveUp()
	case msg.Code == tea.KeyDown:
		s.table.MoveDown()
	case msg.String() == "pgup":
		s.table.PageUp(page)
	case msg.String() == "pgdown":
		s.table.PageDown(page)
	case msg.String() == "home":
		s.table.MoveToTop()
	case msg.String() == "end":
		s.table.MoveToBottom()
	case msg.Code == tea.KeyBackspace:
		if s.query != "" {
			s.query = widget.DropLastGrapheme(s.query)
			retype()
		}
	case msg.Text != "":
		s.query += msg.Text
		retype()
	}
}
