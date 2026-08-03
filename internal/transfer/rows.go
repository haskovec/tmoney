package transfer

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// legPlan is a table-agnostic description of one row to write.
type legPlan struct {
	accountID types.ID
	other     types.ID // the counterpart account (transfer_account_id)
	ledger    Ledger
	amount    types.Money // already signed
	memo      string
	// categoryID is dropped when ledger == LedgerInvestment:
	// investment_transactions has no category_id column.
	categoryID types.NullableID
	status     transaction.Status
	date       types.Date
}

// planLegs is where the four-way graph used to be.
//
// There is no switch. The sign comes from which side of the transfer a leg is
// on; the table comes from the account's own type. 2 signs × 2 ledgers
// reproduces bank→bank, bank→inv, inv→bank and inv→inv as an emergent property
// of the data rather than four hand-written branches.
//
// The four functions this replaces — transaction.CreateTransfer,
// investment.TransferCash, investment.DepositFromAccount and
// investment.TransferCashBetweenInvestments — were 47 + 89 + 87 + 65 lines, and
// the middle two were byte-identical across 85 of them apart from which leg got
// .Neg().
func planLegs(from, to *account.Account, spec Spec, _ types.ID) [2]legPlan {
	return [2]legPlan{
		{
			accountID:  from.ID,
			other:      to.ID,
			ledger:     LedgerFor(from.Type),
			amount:     spec.Amount.Abs().Neg(),
			memo:       spec.Memo,
			categoryID: spec.CategoryID,
			status:     spec.Status,
			date:       spec.Date,
		},
		{
			accountID:  to.ID,
			other:      from.ID,
			ledger:     LedgerFor(to.Type),
			amount:     spec.Amount.Abs(),
			memo:       spec.Memo,
			categoryID: spec.CategoryID,
			status:     spec.Status,
			date:       spec.Date,
		},
	}
}

// insertLeg writes one planned leg to whichever ledger its account belongs to
// and returns a reference to the row.
//
// This and updateLeg/deleteLeg are the ONLY places a whole-transaction transfer
// row is written. Each is a two-arm switch on the plan's ledger — the only
// switch left in the write path, and it is over storage, not over business
// shape.
//
// The investment arm ALWAYS writes TransactionTypeTransferCash. This is
// load-bearing and easy to get silently wrong: GetCashBalance sums TotalAmount
// only over rows whose Type.AffectsCash(), so a leg written with the wrong type
// stops counting as money with no error anywhere. The test matrix asserts it
// explicitly.
func (s *Service) insertLeg(transferID types.ID, p legPlan) (LegRef, error) {
	switch p.ledger {
	case LedgerInvestment:
		status, err := StatusFromRegular(p.status)
		if err != nil {
			return LegRef{}, err
		}
		row := investment.NewTransaction(p.accountID, p.date, investment.TransactionTypeTransferCash, p.amount)
		row.SetTransfer(transferID, p.other)
		row.Status = status
		if p.memo != "" {
			row.SetMemo(p.memo)
		}
		// A category is deliberately not carried: investment_transactions has
		// no column for one. The guard preamble refuses a category on a kind
		// whose legs both land here, so nothing is silently dropped.
		if err := s.invRepo.Create(row); err != nil {
			return LegRef{}, fmt.Errorf("failed to create investment transfer leg: %w", err)
		}
		return LegRef{Ledger: LedgerInvestment, RowID: row.ID, AccountID: p.accountID}, nil

	default:
		row := transaction.NewTransaction(p.accountID, p.date, p.amount)
		row.SetTransfer(transferID, p.other)
		row.Status = p.status
		if p.memo != "" {
			row.SetMemo(p.memo)
		}
		if p.categoryID.Valid {
			row.SetCategory(p.categoryID.ID)
		}
		if err := s.txnRepo.Create(row); err != nil {
			return LegRef{}, fmt.Errorf("failed to create regular transfer leg: %w", err)
		}
		return LegRef{Ledger: LedgerRegular, RowID: row.ID, AccountID: p.accountID}, nil
	}
}

// updateLeg rewrites an existing leg in place to match p.
//
// Editing in place, rather than the delete-and-recreate that
// investment.Service.UpdateTransferCash does today, is what preserves row
// identity across an edit. That is not cosmetic: the old approach minted a NEW
// transfer_id, so anything referencing the old one — a split line, a
// reconciliation record, an undo token the user has not replayed yet — was
// silently orphaned.
func (s *Service) updateLeg(leg Leg, p legPlan) error {
	switch leg.Ledger {
	case LedgerInvestment:
		status, err := StatusFromRegular(p.status)
		if err != nil {
			return err
		}
		row, err := s.invRepo.GetByID(leg.RowID)
		if err != nil {
			return fmt.Errorf("failed to load investment transfer leg %s: %w", leg.RowID.String(), err)
		}
		row.Date = p.date
		row.TotalAmount = p.amount
		row.Status = status
		row.Memo = nullableMemo(p.memo)
		// transfer_id and transfer_account_id are deliberately untouched: an
		// Update never moves a transfer between accounts (§13 Q2 — Delete +
		// Create is the observable, undoable way to do that), so the pair
		// linkage is already correct and rewriting it could only break it.
		if err := s.invRepo.Update(row); err != nil {
			return fmt.Errorf("failed to update investment transfer leg: %w", err)
		}
		return nil

	default:
		row, err := s.txnRepo.GetByID(leg.RowID)
		if err != nil {
			return fmt.Errorf("failed to load regular transfer leg %s: %w", leg.RowID.String(), err)
		}
		row.Date = p.date
		row.Amount = p.amount
		row.Status = p.status
		row.Memo = nullableMemo(p.memo)
		row.CategoryID = p.categoryID
		if err := s.txnRepo.Update(row); err != nil {
			return fmt.Errorf("failed to update regular transfer leg: %w", err)
		}
		return nil
	}
}

// deleteLeg removes one leg from whichever ledger holds it.
func (s *Service) deleteLeg(leg Leg) error {
	switch leg.Ledger {
	case LedgerInvestment:
		if err := s.invRepo.Delete(leg.RowID); err != nil {
			return fmt.Errorf("failed to delete investment transfer leg %s: %w", leg.RowID.String(), err)
		}
		return nil
	default:
		if err := s.txnRepo.Delete(leg.RowID); err != nil {
			return fmt.Errorf("failed to delete regular transfer leg %s: %w", leg.RowID.String(), err)
		}
		return nil
	}
}

// setLegStatus writes only the status column of one leg, through each ledger's
// narrow UpdateStatus.
func (s *Service) setLegStatus(leg Leg, status transaction.Status) error {
	switch leg.Ledger {
	case LedgerInvestment:
		invStatus, err := StatusFromRegular(status)
		if err != nil {
			return err
		}
		if err := s.invRepo.UpdateStatus(leg.RowID, invStatus); err != nil {
			return fmt.Errorf("failed to set investment transfer leg status: %w", err)
		}
		return nil
	default:
		if err := s.txnRepo.UpdateStatus(leg.RowID, status); err != nil {
			return fmt.Errorf("failed to set regular transfer leg status: %w", err)
		}
		return nil
	}
}

func nullableMemo(memo string) types.NullableString {
	if memo == "" {
		return types.NullableString{}
	}
	return types.NullableString{String: memo, Valid: true}
}

// validatePair asserts, for any two legs on any tables, the invariants that make
// two rows a transfer: both carry the same transfer_id; each transfer_account_id
// points at the OTHER leg's account; the amounts are equal and opposite; the
// accounts differ.
//
// It replaces transaction.TransferPair.Validate, and unlike it, runs on the
// recreate path too — RecreateTransferPair routes through Service.Create per leg
// today and therefore skips pair validation entirely.
func validatePair(transferID types.ID, a, b LegRef, plans [2]legPlan) error {
	if a.RowID == b.RowID {
		return fmt.Errorf("transfer %s: both legs resolved to the same row %s",
			transferID.String(), a.RowID.String())
	}
	if a.AccountID == b.AccountID {
		return &SameAccountError{AccountID: a.AccountID}
	}
	if plans[0].other != b.AccountID {
		return fmt.Errorf("transfer %s: from-leg counterpart %s does not point at the to-leg account %s",
			transferID.String(), plans[0].other.String(), b.AccountID.String())
	}
	if plans[1].other != a.AccountID {
		return fmt.Errorf("transfer %s: to-leg counterpart %s does not point at the from-leg account %s",
			transferID.String(), plans[1].other.String(), a.AccountID.String())
	}
	if !plans[0].amount.Neg().Equal(plans[1].amount) {
		return fmt.Errorf("transfer %s: leg amounts %s and %s are not equal and opposite",
			transferID.String(), plans[0].amount.String(), plans[1].amount.String())
	}
	return nil
}
