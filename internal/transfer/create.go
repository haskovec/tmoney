package transfer

import (
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Spec is the complete input to Create. Amount is always a POSITIVE magnitude;
// the signs are applied per leg by planLegs.
//
// Note the argument shape: (From, To) in the user's own terms. The path this
// replaces for bank→investment, investment.DepositFromAccount, took
// (investmentAccountID, regularAccountID) — the reverse of the user's --from/--to
// — so both the CLI and the TUI flipped the arguments going in and flipped the
// result fields coming back, with no test asserting the correspondence.
type Spec struct {
	FromAccountID types.ID
	ToAccountID   types.ID
	Date          types.Date
	Amount        types.Money
	Memo          string
	CategoryID    types.NullableID // zero value = no category

	// Status defaults to StatusUncleared when left zero, so callers that do not
	// care about status can leave it out.
	Status transaction.Status
}

// withDefaults fills in the fields a caller may leave zero.
func (s Spec) withDefaults() Spec {
	if s.Status == "" {
		s.Status = transaction.StatusUncleared
	}
	return s
}

// LegRef identifies a written leg. Presentation uses it for post-save cursor
// restoration — it says both which row and which register the row is in.
type LegRef struct {
	Ledger    Ledger
	RowID     types.ID
	AccountID types.ID
}

// Result is the ONE result shape.
//
// It replaces three: transaction.TransferPair, investment.CashTransferResult and
// investment.InvestmentCashTransferResult, each destructured differently by
// every caller.
type Result struct {
	TransferID types.ID
	Kind       Kind
	From       LegRef
	To         LegRef

	// Before is the pre-edit state, set by Update / Reverse / SetStatus / Void
	// and nil for Create. Undo commands snapshot it instead of re-deriving the
	// old values from a dialog.
	Before *Transfer
}

// LegForAccount returns the written leg belonging to acctID, if either does.
func (r *Result) LegForAccount(acctID types.ID) (LegRef, bool) {
	if r.From.AccountID == acctID {
		return r.From, true
	}
	if r.To.AccountID == acctID {
		return r.To, true
	}
	return LegRef{}, false
}

// Create writes both legs — each to the ledger its own account belongs to —
// inside one transaction.
//
// This ONE method replaces transaction.Service.CreateTransfer,
// investment.Service.TransferCash, investment.Service.DepositFromAccount and
// investment.Service.TransferCashBetweenInvestments.
func (s *Service) Create(spec Spec) (*Result, error) {
	spec = spec.withDefaults()

	// Guards and account loads run OUTSIDE the transaction: keep transactions
	// short, per specs/design-withtx.md. Nothing here writes.
	from, to, err := s.guardSpec(spec)
	if err != nil {
		return nil, err
	}

	transferID := types.NewID()
	plans := planLegs(from, to, spec, transferID)

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		fromRef, err := b.insertLeg(transferID, plans[0])
		if err != nil {
			return err
		}
		toRef, err := b.insertLeg(transferID, plans[1])
		if err != nil {
			return err
		}
		if err := validatePair(transferID, fromRef, toRef, plans); err != nil {
			return err
		}
		res = &Result{
			TransferID: transferID,
			Kind:       ClassifyKind(from.Type, to.Type),
			From:       fromRef,
			To:         toRef,
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return res, nil
}
