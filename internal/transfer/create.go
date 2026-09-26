package transfer

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
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

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		res, err = b.writePair(from, to, spec)
		return err
	}); err != nil {
		return nil, err
	}

	return res, nil
}

// writePair inserts both legs of a new transfer on the bound service b. The
// caller owns the transaction and has already run guardSpec.
func (b *Service) writePair(from, to *account.Account, spec Spec) (*Result, error) {
	transferID := types.NewID()
	plans := planLegs(from, to, spec, transferID)

	fromRef, err := b.insertLeg(transferID, plans[0])
	if err != nil {
		return nil, err
	}
	toRef, err := b.insertLeg(transferID, plans[1])
	if err != nil {
		return nil, err
	}
	if err := validatePair(transferID, fromRef, toRef, plans); err != nil {
		return nil, err
	}
	return &Result{
		TransferID: transferID,
		Kind:       ClassifyKind(from.Type, to.Type),
		From:       fromRef,
		To:         toRef,
	}, nil
}

// ReplaceWithTransfer turns a plain investment cash row (a Deposit or a
// Withdrawal) into a transfer. The row is deleted and both legs are written in
// one transaction, so the row is either replaced or left as it was. The row's
// account must be one side of the transfer.
//
// Only Deposit and Withdrawal qualify: they are the rows that move cash in or
// out of the account with no other effect. A row that is already a leg is
// edited through Update instead.
func (s *Service) ReplaceWithTransfer(rowID types.ID, spec Spec) (*Result, error) {
	spec = spec.withDefaults()

	row, err := s.invRepo.GetByID(rowID)
	if err != nil {
		return nil, err
	}
	switch {
	case row.TransferID.Valid:
		return nil, &NotReplaceableError{RowID: rowID, Reason: "it is already part of a transfer"}
	case row.Type != investment.TransactionTypeDeposit && row.Type != investment.TransactionTypeWithdrawal:
		return nil, &NotReplaceableError{RowID: rowID, Reason: "only a deposit or a withdrawal can become a transfer"}
	case row.AccountID != spec.FromAccountID && row.AccountID != spec.ToAccountID:
		return nil, &NotReplaceableError{RowID: rowID, Reason: "the transfer must include the account the row is in"}
	}

	from, to, err := s.guardSpec(spec)
	if err != nil {
		return nil, err
	}

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		if err := b.invRepo.Delete(rowID); err != nil {
			return err
		}
		res, err = b.writePair(from, to, spec)
		return err
	}); err != nil {
		return nil, err
	}

	return res, nil
}
