// Investment register: the security filter, its matching, its status line, and
// its search-key handler.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// investmentRegisterFilterActive reports whether the security filter is
// affecting the register — either the user is typing a query or a security is
// locked. While active, the running-balance column and total-return header are
// suppressed (they are account-wide and can't be meaningfully sliced).
func (a *App) investmentRegisterFilterActive() bool {
	return a.investmentRegister.filterSearching || !a.investmentRegister.filterLockedSec.IsNil()
}

// resetInvestmentRegisterFilter clears all filter state, returning the register
// to its full unfiltered view.
func (a *App) resetInvestmentRegisterFilter() {
	a.investmentRegister.filterSearching = false
	a.investmentRegister.filterQuery = ""
	a.investmentRegister.filterLockedSec = types.NilID
}

// visibleInvestmentTransactions returns the transactions the register should
// display given the current filter. With a security locked, only that
// security's rows show; while typing a non-empty query, rows whose security
// ticker or name contains the query show (rows with no security are excluded);
// otherwise the full ledger is returned.
func (a *App) visibleInvestmentTransactions() []*investment.Transaction {
	if a.investmentRegister.data == nil {
		return nil
	}
	all := a.investmentRegister.data.transactions

	if !a.investmentRegister.filterLockedSec.IsNil() {
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if txn.SecurityID.Valid && txn.SecurityID.ID == a.investmentRegister.filterLockedSec {
				out = append(out, txn)
			}
		}
		return out
	}

	if a.investmentRegister.filterSearching {
		q := strings.ToLower(strings.TrimSpace(a.investmentRegister.filterQuery))
		if q == "" {
			return all
		}
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if a.txnSecurityMatchesQuery(txn, q) {
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
func (a *App) txnSecurityMatchesQuery(txn *investment.Transaction, lowerQuery string) bool {
	if !txn.SecurityID.Valid {
		return false
	}
	id := txn.SecurityID.ID
	if strings.Contains(strings.ToLower(a.investmentRegister.data.securityNames[id]), lowerQuery) {
		return true
	}
	return strings.Contains(strings.ToLower(a.investmentRegister.data.securityFullNames[id]), lowerQuery)
}

// investmentFilterMatchedSecurities returns the distinct securities among the
// currently visible rows, in first-seen order. Used to decide whether a typed
// query resolves to exactly one security (Enter locks) and to render the
// status line.
func (a *App) investmentFilterMatchedSecurities() []types.ID {
	seen := make(map[types.ID]bool)
	var out []types.ID
	for _, txn := range a.visibleInvestmentTransactions() {
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
func (a *App) securityDisplayName(id types.ID) string {
	if a.investmentRegister.data == nil {
		return ""
	}
	label := a.investmentRegister.data.securityNames[id]
	full := a.investmentRegister.data.securityFullNames[id]
	switch {
	case label != "" && full != "" && label != full:
		return label + " — " + full
	case full != "":
		return full
	default:
		return label
	}
}

// investmentFilterStatusLine builds the one-line filter indicator shown under
// the register title while a filter is active.
func (a *App) investmentFilterStatusLine() string {
	if a.investmentRegister.data == nil {
		return ""
	}
	total := len(a.investmentRegister.data.transactions)
	n := len(a.visibleInvestmentTransactions())

	if !a.investmentRegister.filterLockedSec.IsNil() {
		return fmt.Sprintf("Filter: %s  (%d of %d)", a.securityDisplayName(a.investmentRegister.filterLockedSec), n, total)
	}

	q := strings.TrimSpace(a.investmentRegister.filterQuery)
	if q == "" {
		return "Filter: (type a ticker or name — Enter locks · Esc clears)"
	}
	matched := a.investmentFilterMatchedSecurities()
	switch len(matched) {
	case 0:
		return fmt.Sprintf("Filter: %s  →  no matches", q)
	case 1:
		return fmt.Sprintf("Filter: %s  →  %s (%d rows)", q, a.securityDisplayName(matched[0]), n)
	default:
		return fmt.Sprintf("Filter: %s  →  %d securities, %d rows", q, len(matched), n)
	}
}

// handleInvestmentRegisterSearchKey drives the security filter while the user
// is typing a query. Text/Backspace edit the query and re-narrow the list;
// the arrow / page / home / end keys still navigate so the user can
// preview-scroll matches; Enter locks the filter when the query resolves to
// exactly one security; Esc clears the filter entirely.
func (a *App) handleInvestmentRegisterSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.investmentRegister.table == nil {
		return a, nil
	}

	rebuildTop := func() {
		a.buildInvestmentRegisterTable()
		a.investmentRegister.table.SetCursor(0)
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.resetInvestmentRegisterFilter()
		a.buildInvestmentRegisterTable()
	case key.Matches(msg, a.keys.Enter):
		// Lock only when the query resolves to a single security; an ambiguous
		// or empty match keeps the user in typing mode.
		if matched := a.investmentFilterMatchedSecurities(); len(matched) == 1 {
			a.investmentRegister.filterLockedSec = matched[0]
			a.investmentRegister.filterSearching = false
			a.investmentRegister.filterQuery = ""
			rebuildTop()
		}
	// Navigation is matched on the physical arrow keys via msg.Code — NOT via
	// a.keys.Up/Down, whose vim aliases ("k"/"j") would otherwise swallow those
	// letters instead of appending them to the query. Page/Home/End match on
	// msg.String() (no letter alias, so they are safe for typing).
	case msg.Code == tea.KeyUp:
		a.investmentRegister.table.MoveUp()
	case msg.Code == tea.KeyDown:
		a.investmentRegister.table.MoveDown()
	case msg.String() == "pgup":
		a.investmentRegister.table.PageUp(max(a.height-6, 1))
	case msg.String() == "pgdown":
		a.investmentRegister.table.PageDown(max(a.height-6, 1))
	case msg.String() == "home":
		a.investmentRegister.table.MoveToTop()
	case msg.String() == "end":
		a.investmentRegister.table.MoveToBottom()
	case msg.Code == tea.KeyBackspace:
		if r := []rune(a.investmentRegister.filterQuery); len(r) > 0 {
			a.investmentRegister.filterQuery = string(r[:len(r)-1])
			rebuildTop()
		}
	case msg.Text != "":
		a.investmentRegister.filterQuery += msg.Text
		rebuildTop()
	}
	return a, nil
}
