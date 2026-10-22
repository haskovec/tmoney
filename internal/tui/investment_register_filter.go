// Investment register: the security filter, its matching, its status line, and
// its search-key handler.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// filterActive reports whether the security filter is
// affecting the register — either the user is typing a query or a security is
// locked. While active, the running-balance column and total-return header are
// suppressed (they are account-wide and can't be meaningfully sliced).
func (s *investmentRegisterViewState) filterActive() bool {
	return s.filterSearching || !s.filterLockedSec.IsNil()
}

// resetFilter clears all filter state, returning the register
// to its full unfiltered view.
func (s *investmentRegisterViewState) resetFilter() {
	s.filterSearching = false
	s.filterQuery = ""
	s.filterLockedSec = types.NilID
}

// visibleTransactions returns the transactions the register should
// display given the current filter. With a security locked, only that
// security's rows show; while typing a non-empty query, rows whose security
// ticker or name contains the query show (rows with no security are excluded);
// otherwise the full ledger is returned.
func (s *investmentRegisterViewState) visibleTransactions() []*investment.Transaction {
	if s.data == nil {
		return nil
	}
	all := s.data.transactions

	if !s.filterLockedSec.IsNil() {
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if txn.SecurityID.Valid && txn.SecurityID.ID == s.filterLockedSec {
				out = append(out, txn)
			}
		}
		return out
	}

	if s.filterSearching {
		q := strings.ToLower(strings.TrimSpace(s.filterQuery))
		if q == "" {
			return all
		}
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if s.txnSecurityMatchesQuery(txn, q) {
				out = append(out, txn)
			}
		}
		return out
	}

	return all
}

// txnSecurityMatchesQuery reports whether a transaction's security matches the
// (already lower-cased) query by ticker/label or full name. Cash rows (no
// security) never match.
func (s *investmentRegisterViewState) txnSecurityMatchesQuery(txn *investment.Transaction, lowerQuery string) bool {
	if !txn.SecurityID.Valid {
		return false
	}
	id := txn.SecurityID.ID
	if strings.Contains(strings.ToLower(s.data.securityNames[id]), lowerQuery) {
		return true
	}
	return strings.Contains(strings.ToLower(s.data.securityFullNames[id]), lowerQuery)
}

// filterMatchedSecurities returns the distinct securities among the
// currently visible rows, in first-seen order. Used to decide whether a typed
// query resolves to exactly one security (Enter locks) and to render the
// status line.
func (s *investmentRegisterViewState) filterMatchedSecurities() []types.ID {
	seen := make(map[types.ID]bool)
	var out []types.ID
	for _, txn := range s.visibleTransactions() {
		if txn.SecurityID.Valid && !seen[txn.SecurityID.ID] {
			seen[txn.SecurityID.ID] = true
			out = append(out, txn.SecurityID.ID)
		}
	}
	return out
}

// securityDisplayName renders a security as "TICKER — Full Name", degrading to
// just the name for a tickerless holding (where the label already equals the
// name).
func (s *investmentRegisterViewState) securityDisplayName(id types.ID) string {
	if s.data == nil {
		return ""
	}
	label := s.data.securityNames[id]
	full := s.data.securityFullNames[id]
	switch {
	case label != "" && full != "" && label != full:
		return label + " — " + full
	case full != "":
		return full
	default:
		return label
	}
}

// filterStatusLine builds the one-line filter indicator shown under
// the register title while a filter is active.
func (s *investmentRegisterViewState) filterStatusLine() string {
	head, tail := s.filterStatusParts()
	return head + tail
}

// fittedFilterStatusLine fits the filter line in width cells. The line sits
// above the table, so it must stay one row; the query and the security name
// are cut with "…", so the count stays in view.
func (s *investmentRegisterViewState) fittedFilterStatusLine(width int) string {
	head, tail := s.filterStatusParts()
	if lipgloss.Width(head+tail) <= width {
		return head + tail
	}
	return ansi.Truncate(head, max(width-lipgloss.Width(tail), 0), "…") + tail
}

// filterStatusParts returns the filter line as a head that may be cut (the
// query and the security name) and a tail that must stay (the count).
func (s *investmentRegisterViewState) filterStatusParts() (head, tail string) {
	if s.data == nil {
		return "", ""
	}
	total := len(s.data.transactions)
	n := len(s.visibleTransactions())

	if !s.filterLockedSec.IsNil() {
		return "Filter: " + s.securityDisplayName(s.filterLockedSec), fmt.Sprintf("  (%d of %d)", n, total)
	}

	q := strings.TrimSpace(s.filterQuery)
	if q == "" {
		return "Filter: (type a ticker or name — Enter locks · Esc clears)", ""
	}
	matched := s.filterMatchedSecurities()
	switch len(matched) {
	case 0:
		return "Filter: " + q, "  →  no matches"
	case 1:
		return "Filter: " + q + "  →  " + s.securityDisplayName(matched[0]), fmt.Sprintf(" (%d rows)", n)
	default:
		return "Filter: " + q, fmt.Sprintf("  →  %d securities, %d rows", len(matched), n)
	}
}

// handleSearchKey drives the security filter while the user
// is typing a query. Text/Backspace edit the query and re-narrow the list;
// the arrow / page / home / end keys still navigate so the user can
// preview-scroll matches; Enter locks the filter when the query resolves to
// exactly one security; Esc clears the filter entirely.
func (s *investmentRegisterViewState) handleSearchKey(msg tea.KeyPressMsg, keys keyMap, styles widget.Styles, height int) {
	if s.table == nil {
		return
	}

	rebuildTop := func() {
		s.buildTable(styles)
		s.table.SetCursor(0)
	}

	switch {
	case key.Matches(msg, keys.Escape):
		s.resetFilter()
		s.buildTable(styles)
	case key.Matches(msg, keys.Enter):
		// Lock only when the query resolves to a single security; an ambiguous
		// or empty match keeps the user in typing mode.
		if matched := s.filterMatchedSecurities(); len(matched) == 1 {
			s.filterLockedSec = matched[0]
			s.filterSearching = false
			s.filterQuery = ""
			rebuildTop()
		}
	// Navigation is matched on the physical arrow keys via msg.Code — NOT via
	// keys.Up/Down, whose vim aliases ("k"/"j") would otherwise swallow those
	// letters instead of appending them to the query. Page/Home/End match on
	// msg.String() (no letter alias, so they are safe for typing).
	case msg.Code == tea.KeyUp:
		s.table.MoveUp()
	case msg.Code == tea.KeyDown:
		s.table.MoveDown()
	case msg.String() == "pgup":
		s.table.PageUp(max(height-6, 1))
	case msg.String() == "pgdown":
		s.table.PageDown(max(height-6, 1))
	case msg.String() == "home":
		s.table.MoveToTop()
	case msg.String() == "end":
		s.table.MoveToBottom()
	case msg.Code == tea.KeyBackspace:
		if s.filterQuery != "" {
			s.filterQuery = widget.DropLastGrapheme(s.filterQuery)
			rebuildTop()
		}
	case msg.Text != "":
		s.filterQuery += msg.Text
		rebuildTop()
	}
}
