package transfer

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Leg is one side of a transfer, normalized across both ledgers.
//
// Amount is SIGNED as stored (negative = money leaving this account).
// Status is normalized to transaction.Status: an investment leg's "pending"
// reads back as StatusUncleared via StatusToRegular (which lives in this
// package — see status.go for why it cannot live in internal/investment).
type Leg struct {
	Ledger    Ledger
	RowID     types.ID
	AccountID types.ID

	// OtherAccountID is the leg's transfer_account_id: a back-reference to the
	// counterpart leg's account. Both legs carry it, pointing at each other.
	OtherAccountID types.ID

	Date   types.Date
	Amount types.Money
	Memo   string
	Status transaction.Status

	// CategoryID is always zero for an investment leg: investment_transactions
	// has no category_id column.
	CategoryID types.NullableID

	// InvType is the investment_transactions.transaction_type of an investment
	// leg, and empty for a regular leg. It is what Movement is derived from.
	InvType string
}

// IsFrom reports whether this leg is the sending side. Orientation is carried
// by the sign, so a zero-amount (voided) pair cannot be oriented this way —
// Transfer.From/To handle that case, see resolveOrientation.
func (l Leg) IsFrom() bool { return l.Amount.IsNegative() }

// Transfer is a whole transfer, read across both ledgers. It is the ONE view
// model, replacing cli.resolvedTransfer and tui.investmentTransferEdit.
type Transfer struct {
	TransferID types.ID
	Kind       Kind
	Movement   Movement
	Shape      Shape

	// From is the sending leg, To the receiving leg.
	From Leg
	To   Leg

	// FromAccount and ToAccount are the loaded account records for the two
	// legs. Reading a transfer already has to load them to classify Kind, so
	// exposing them saves every caller — and every guard — a second lookup.
	// Both are non-nil on a successfully read Transfer.
	FromAccount *account.Account
	ToAccount   *account.Account

	// Amount is the positive magnitude of the transfer.
	Amount types.Money

	Date   types.Date
	Memo   string
	Status transaction.Status

	// CategoryID is the transfer's category, read from whichever leg can carry
	// one. Zero when the transfer has no category or (for inv↔inv) cannot have
	// one. Threaded back through an edit so an edit that does not touch the
	// category preserves it.
	CategoryID types.NullableID

	// ParentTransactionID is set only when Shape == ShapeSplitLine: the
	// multi-line-split parent that owns the transfer_id.
	ParentTransactionID types.ID
}

// LegForAccount returns the leg belonging to acctID, if either does. The TUI
// uses it to put the cursor back on the row in the register the user is
// looking at.
func (t *Transfer) LegForAccount(acctID types.ID) (Leg, bool) {
	if t.From.AccountID == acctID {
		return t.From, true
	}
	if t.To.AccountID == acctID {
		return t.To, true
	}
	return Leg{}, false
}

// HasInvestmentLeg reports whether either leg lives in investment_transactions.
// A transfer with one cannot be voided (investment_transactions has no void
// status) and cannot be fully reconciled.
func (t *Transfer) HasInvestmentLeg() bool {
	return t.From.Ledger == LedgerInvestment || t.To.Ledger == LedgerInvestment
}

// Legs returns both legs in a stable order (From, To) for iteration.
func (t *Transfer) Legs() [2]Leg { return [2]Leg{t.From, t.To} }
