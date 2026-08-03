// Package transfer owns whole-transaction cash transfers, whatever pair of
// account types they connect.
//
// The design it implements is specs/design-unified-transfer.md, which addresses
// item 2 of specs/code-quality-review.md. Its companion is
// specs/design-withtx.md (item 1): everything here is built on db.WithTx,
// Service.InTx(db.Queryer) and the join-if-bound runInTx contract.
//
// The package imports both ledger packages (transaction and investment) and is
// imported by neither, so it can write both legs of a transfer without an
// inverted-dependency port.
package transfer

import (
	"github.com/haskovec/tmoney/internal/account"
)

// Ledger names the physical table a leg's row lives in. It is derived from the
// leg account's type, never passed in by a caller.
type Ledger string

const (
	// LedgerRegular is the `transactions` table.
	LedgerRegular Ledger = "regular"
	// LedgerInvestment is the `investment_transactions` table.
	LedgerInvestment Ledger = "investment"
	// LedgerSplit is the `transaction_splits` table. It appears only on the
	// parent side of a transfer LINE inside a multi-line split, which has no
	// transaction row of its own — the line IS the row. No transfer this
	// package writes ever has a LedgerSplit leg; it exists so a split line can
	// be READ and explained rather than reported as malformed.
	LedgerSplit Ledger = "split"
)

// LedgerFor reports which table a leg belonging to an account of type t is
// written to and read from. HSA counts as an investment type (see
// account.Type.IsInvestmentType), so an HSA leg lives in
// investment_transactions.
func LedgerFor(t account.Type) Ledger {
	if t.IsInvestmentType() {
		return LedgerInvestment
	}
	return LedgerRegular
}

// Kind labels the (From, To) ledger combination of a transfer. It is a LABEL,
// not a routing decision: the write path derives each leg's table from its own
// account and never switches on Kind. Kind exists so errors can name the shape
// the user asked for, and so the category rule (which is kind-dependent until
// investment_transactions grows a category_id column) has something to test.
//
// This replaces transaction.TransferDispatchKind, which was consumed by five
// hand-written 4-arm switches in the TUI and CLI.
type Kind int

const (
	// KindRegToReg covers bank↔bank — both legs are non-investment.
	KindRegToReg Kind = iota
	// KindInvToReg covers cash leaving an investment account for a regular
	// account (e.g. brokerage → checking withdrawal).
	KindInvToReg
	// KindRegToInv covers cash flowing from a regular account into an
	// investment account (e.g. checking → 401k contribution).
	KindRegToInv
	// KindInvToInv covers cash moving between two investment accounts
	// (e.g. IRA → IRA rollover).
	KindInvToInv
)

// ClassifyKind picks the Kind for a transfer from its From/To account types.
// An unrecognized type is treated as non-investment and therefore falls through
// to KindRegToReg, matching the behavior of the transaction.ChooseTransferDispatch
// it replaces.
func ClassifyKind(from, to account.Type) Kind {
	fromInv := from.IsInvestmentType()
	toInv := to.IsInvestmentType()
	switch {
	case fromInv && toInv:
		return KindInvToInv
	case fromInv:
		return KindInvToReg
	case toInv:
		return KindRegToInv
	default:
		return KindRegToReg
	}
}

// String returns a stable identifier for the Kind, used in error messages.
func (k Kind) String() string {
	switch k {
	case KindRegToReg:
		return "bank→bank"
	case KindInvToReg:
		return "investment→bank"
	case KindRegToInv:
		return "bank→investment"
	case KindInvToInv:
		return "investment→investment"
	default:
		return "unknown"
	}
}

// StoresCategory reports whether a transfer of this Kind has anywhere to put a
// category. investment_transactions has no category_id column, so an inv↔inv
// transfer — whose two legs both live there — cannot carry one. Every other
// kind has at least one regular-table leg to label.
//
// This is the single domain home for a rule that is currently re-implemented in
// five presentation-layer sites (cli/transfer/add.go:112, add.go:194,
// cli/transfer/edit.go:107, tui/transfer_dialog.go:718 and :212-228) and
// enforced nowhere in the domain.
func (k Kind) StoresCategory() bool {
	return k != KindInvToInv
}

// Movement distinguishes what is actually moving. A transfer_shares pair is
// inv↔inv by account type but must never be touched by a cash verb, and today
// nothing enforces that: investment.Service.UpdateTransferCash never checks the
// row type, so handing it a buy's ID deletes the buy without reversing its lot
// effects. Classifying movement in the read model is what closes that hole.
type Movement string

const (
	// MovementCash is a transfer_cash pair, or a regular↔regular pair.
	MovementCash Movement = "cash"
	// MovementShares is a transfer_shares pair, owned by investment.Service.
	MovementShares Movement = "shares"
)

// Shape distinguishes a whole-transaction transfer from a transfer LINE inside
// a multi-line split (e.g. a paycheck's 401k contribution line). Split lines
// are owned by transaction.Service's split lifecycle, not by this package;
// reads resolve them so callers can explain the refusal, and every verb
// refuses them.
type Shape string

const (
	// ShapeWhole is a standalone two-leg transfer.
	ShapeWhole Shape = "whole"
	// ShapeSplitLine is a transfer_id owned by a multi-line split's line item.
	ShapeSplitLine Shape = "split-line"
)
