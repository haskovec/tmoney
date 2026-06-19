package tui

import (
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// balanceColWidth is the fixed width of the running-balance column in the
// register tables. It is wider than the Amount/Total column (12) because the
// balance is an accumulated running total that reaches larger magnitudes than
// any single amount: 15 holds -$9,999,999,999.99 (15 chars) without the
// alignText truncation that would silently corrupt the displayed figure. The
// money strings carry no thousands separators (see formatDashboardMoney).
const balanceColWidth = 15

// registerFlexMargin is the breathing room (in columns) required *above* each
// flex column's MinWidth before the running-balance column is allowed to claim
// space. Without it the Balance column could appear while Payee/Category are
// jammed at their absolute minimum width.
const registerFlexMargin = 5

// runningBalances returns the account balance *after* each transaction, in the
// same (display) order as txns — which the register loads newest-first (date
// DESC, created_at DESC). The balance after a given row is the opening balance
// plus the signed sum of every non-void transaction at that row and older
// (i.e. everything from this row through the end of the slice). Void
// transactions contribute nothing, so their row carries the prior balance
// forward unchanged.
//
// With the full account ledger loaded — the register applies no filter — the
// first element equals the account's current balance (the figure shown in the
// register title bar), which serves as a built-in reconciliation check.
func runningBalances(txns []*transaction.Transaction, opening types.Money) []types.Money {
	out := make([]types.Money, len(txns))
	bal := opening
	for i := len(txns) - 1; i >= 0; i-- {
		if !txns[i].IsVoid() {
			bal = bal.Add(txns[i].Amount)
		}
		out[i] = bal
	}
	return out
}

// runningCash mirrors runningBalances for an investment account's cash
// position. Investment accounts carry no opening cash, so accumulation starts
// at zero; only cash-affecting transaction types (Type.AffectsCash()) move the
// balance — share-only rows such as Reinvest Dividend, Transfer Shares, and
// Fee Liquidation carry the prior cash forward. The first element equals
// Service.GetCashBalance, the figure shown in the register title bar.
func runningCash(txns []*investment.Transaction) []types.Money {
	out := make([]types.Money, len(txns))
	bal := types.ZeroMoney
	for i := len(txns) - 1; i >= 0; i-- {
		if txns[i].Type.AffectsCash() {
			bal = bal.Add(txns[i].TotalAmount)
		}
		out[i] = bal
	}
	return out
}

// columnsFitWidth reports whether the given columns fit within tableWidth such
// that every flex column (Width == 0) still gets at least its MinWidth plus
// flexMargin columns of breathing room. Fixed columns are counted at their
// Width, plus one space separator between every adjacent pair. For an all-fixed
// column set the flexMargin is irrelevant and this reduces to "do the fixed
// widths plus separators fit". It is the gate that decides whether each
// register is wide enough to add the running-balance column.
func columnsFitWidth(cols []widget.Column, tableWidth, flexMargin int) bool {
	if len(cols) == 0 {
		return false
	}
	need := len(cols) - 1 // one-space separators between columns
	for _, c := range cols {
		if c.Width > 0 {
			need += c.Width
		} else {
			need += c.MinWidth + flexMargin
		}
	}
	return tableWidth >= need
}

// tableHasBalanceColumn reports whether the table's current column set ends with
// the running-balance column. Used on resize to rebuild a register only when
// the show/hide decision actually flips — a no-op resize then preserves the
// table's scroll/cursor state instead of resetting it via SetRows.
func tableHasBalanceColumn(t *widget.Table) bool {
	if t == nil {
		return false
	}
	cols := t.Columns()
	return len(cols) > 0 && cols[len(cols)-1].Header == "Balance"
}
