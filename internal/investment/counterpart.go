package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// The investment side of a bank-to-investment transfer.
//
// internal/transaction owns the bank side and calls in through the
// InvestmentCounterpartPort interface it declares itself; these four methods are
// what satisfy it. Each takes the caller's db.Queryer per call, so the counterpart
// row joins the caller's transaction without this package needing to open one.

// CreateTransferCashCounterpart mints a one-sided investment.Transaction
// of type TransferCash on invAcctID, linked by the caller-supplied
// transferID to otherAcctID. The signed amount controls direction
// (positive = cash arriving, negative = cash leaving), matching the sign
// of the destination leg of TransferCash / DepositFromAccount.
//
// Used by transaction.Service to mint the investment-side counterpart of a
// transfer-LINE split (e.g. a paycheck → 401k contribution line) whose target is
// an investment account. Satisfies transaction.InvestmentCounterpartPort.
//
// Every write runs on q, the caller's transaction, so the counterpart commits
// with the split row that owns it. Reads go through the bound service too: an
// unbound read would miss the caller's own uncommitted writes.
func (s *Service) CreateCounterpart(
	q db.Queryer,
	invAcctID, otherAcctID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	transferID types.ID,
) (types.ID, error) {
	b := s.InTx(q)
	if err := b.requireInvestmentAccount(invAcctID); err != nil {
		return types.ID{}, err
	}

	txn := NewTransaction(invAcctID, date, TransactionTypeTransferCash, amount)
	txn.SetTransfer(transferID, otherAcctID)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := b.validateTransaction(txn); err != nil {
		return types.ID{}, err
	}

	if err := b.repo.Create(txn); err != nil {
		return types.ID{}, fmt.Errorf("failed to create investment-side counterpart: %w", err)
	}

	return txn.ID, nil
}

// FindCounterpart returns the investment row linked to the given transferID,
// reading on q so it sees the caller's uncommitted writes. Returns found=false
// (no error) if no investment-side row exists. reconciled reports whether the row
// is fully reconciled, which callers use to block cascading deletes/edits.
func (s *Service) FindCounterpart(q db.Queryer, transferID types.ID) (rowID types.ID, reconciled bool, found bool, err error) {
	rows, err := s.InTx(q).repo.ListByTransferID(transferID)
	if err != nil {
		return types.ID{}, false, false, fmt.Errorf("failed to look up investment-side counterpart: %w", err)
	}
	if len(rows) == 0 {
		return types.ID{}, false, false, nil
	}
	row := rows[0]
	return row.ID, row.IsReconciled(), true, nil
}

// DeleteCounterpart removes the investment row identified by rowID, on q. The
// caller is responsible for the regular-side parent or counterpart cleanup; no
// cascade is performed here.
func (s *Service) DeleteCounterpart(q db.Queryer, rowID types.ID) error {
	if err := s.InTx(q).repo.Delete(rowID); err != nil {
		return fmt.Errorf("failed to delete investment-side counterpart: %w", err)
	}
	return nil
}

// UpdateTransferCashCounterpartAmount mirrors a transfer-line amount edit
// onto the investment-side counterpart row. The caller supplies the new
// signed amount in the destination's frame of reference (positive = cash
// arriving, negative = cash leaving) — i.e. the inverse of the parent
// split's amount.
//
// The caller is responsible for checking that the row is not reconciled before
// invoking this (use FindCounterpart's reconciled return). A no-op if the new
// amount already matches.
func (s *Service) UpdateCounterpartAmount(q db.Queryer, rowID types.ID, newAmount types.Money) error {
	b := s.InTx(q)
	row, err := b.repo.GetByID(rowID)
	if err != nil {
		return fmt.Errorf("failed to load investment-side counterpart: %w", err)
	}
	if row.TotalAmount.Equal(newAmount) {
		return nil
	}
	row.TotalAmount = newAmount
	if err := b.repo.Update(row); err != nil {
		return fmt.Errorf("failed to update investment-side counterpart amount: %w", err)
	}
	return nil
}
