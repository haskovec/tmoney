package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Transfer-line counterparts.
//
// A split line pointing at another account mints a paired counter-transaction in
// that account. This file owns the whole contract: creating the counterpart,
// routing it to the bank or investment ledger, mirroring later edits onto it,
// refusing edits it must not accept, and cascade-deleting it.
//
// It is also the only cluster with a private dependency of its own —
// investmentCounterpart, used by these methods and nothing else in the package.

// createTransferLineCounterpart mints the paired counter-transaction for
// a transfer-line split. If the target account is investment-type, the
// row is created on the investment_transactions table via the configured
// InvestmentCounterpartPort; otherwise a regular transaction is
// created on the transactions table.
//
// A category on the split (a "categorized transfer", e.g. a loan payment's
// principal line labeled Loan:Principal) is mirrored onto a regular-side
// counterpart. The investment adapter row has no category column, so an
// investment-side counterpart carries none — the split line holds it alone.
//
// The split must already carry a valid TransferID (CreateWithSplits and
// moveTransferLine mint it before calling here).
func (s *Service) createTransferLineCounterpart(
	parentAcctID types.ID,
	parentDate types.Date,
	split *Split,
) error {
	targetAcctID := split.TransferAccountID.ID
	counterAmount := split.Amount.Neg()
	transferID := split.TransferID.ID

	// Guard the target account: it must be open, and an investment target
	// requires the counterpart adapter. Guards CreateWithSplits,
	// moveTransferLine (which re-targets a split), and ReplaceSplits.
	isInv, err := s.ensureTransferTargetRoutable(targetAcctID)
	if err != nil {
		return err
	}

	if isInv {
		if _, err := s.investmentCounterpart.CreateCounterpart(
			s.q(),
			targetAcctID, parentAcctID, parentDate, counterAmount, "", transferID,
		); err != nil {
			return fmt.Errorf("failed to create investment-side paired transfer transaction: %w", err)
		}
		return nil
	}

	paired := NewTransaction(targetAcctID, parentDate, counterAmount)
	paired.SetTransfer(transferID, parentAcctID)
	if !split.CategoryID.IsNil() {
		paired.SetCategory(split.CategoryID)
	}
	if err := s.txnRepo.Create(paired); err != nil {
		return fmt.Errorf("failed to create paired transfer transaction: %w", err)
	}
	return nil
}

// targetIsInvestment reports whether the given account is an investment-
// type account (TypeInvestment or TypeHSA). Returns false (no error) if
// accountRepo is not wired — only test fixtures hit that path.
func (s *Service) targetIsInvestment(acctID types.ID) (bool, error) {
	if s.accountRepo == nil {
		return false, nil
	}
	acct, err := s.accountRepo.GetByID(acctID)
	if err != nil {
		return false, fmt.Errorf("failed to load target account: %w", err)
	}
	return acct.Type.IsInvestmentType(), nil
}

// ensureTransferTargetRoutable verifies a transfer-line's target account can
// receive a paired counter-transaction: it must be open, and an investment
// target requires the investment-counterpart adapter to be wired. It returns
// whether the target is an investment account so the caller can route the
// counterpart to the right table without re-loading it. Reused by
// createTransferLineCounterpart and ReplaceSplits' pre-flight so a rewrite of
// the split rows can't strand a would-be counterpart.
func (s *Service) ensureTransferTargetRoutable(targetAcctID types.ID) (bool, error) {
	if err := s.ensureAccountOpen(targetAcctID); err != nil {
		return false, err
	}
	isInv, err := s.targetIsInvestment(targetAcctID)
	if err != nil {
		return false, err
	}
	if isInv && s.investmentCounterpart == nil {
		return false, fmt.Errorf(
			"transfer-line split targets investment account %s but no investment-counterpart adapter is wired on transaction.Service",
			targetAcctID.String(),
		)
	}
	return isInv, nil
}

// findPairedByTransferID returns the paired single-line counter-transaction
// for a transfer-line's transfer_id, or nil if none exists. The parent
// (multi-line) transaction is not marked as a transfer, so only the paired
// counterpart lives on the transactions table with that transfer_id.
func (s *Service) findPairedByTransferID(transferID types.ID) (*Transaction, error) {
	matches, err := s.txnRepo.ListByTransferID(transferID)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return matches[0], nil
}

// moveTransferLine handles the target-account-change cascade: delete the old
// paired counter-transaction, mint a fresh transfer_id on the split-line,
// persist the split, and create a new paired counterpart in the new target.
// The new counterpart is routed to the investment table when the new
// target is an investment account, and carries the split's category onto a
// regular-side counterpart (via createTransferLineCounterpart).
func (s *Service) moveTransferLine(parent *Transaction, existing, split *Split) error {
	split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}

	// Delete the old counterpart, rewrite the split row, and mint the new
	// counterpart in one transaction: a failure at any step leaves the original
	// split and counterpart untouched.
	return s.runInTx(func(b *Service) error {
		if existing.TransferID.Valid {
			if err := b.deletePairedCounterTransaction(existing.TransferID.ID); err != nil {
				return err
			}
		}
		if err := b.splitRepo.Update(split); err != nil {
			return err
		}
		return b.createTransferLineCounterpart(parent.AccountID, parent.Date, split)
	})
}

// mirrorToPairedCounterpart syncs the paired counter-transaction of a retained
// transfer line to the line's current amount and category. A regular-side
// counterpart mirrors both amount and category. An investment-side counterpart
// has no category column, so only its amount is mirrored — and only when the
// amount actually changed: a category-only change is a no-op there and must not
// touch (or be blocked by) the investment row. A reconciled counterpart that
// would be written blocks the sync with IsReconciledError.
//
// newAmount is the counterpart's amount (already negated relative to the split
// line); categoryID is the split line's category (Valid:false clears it);
// amountChanged reports whether the amount actually changed.
func (s *Service) mirrorToPairedCounterpart(transferID types.ID, newAmount types.Money, categoryID types.NullableID, amountChanged bool) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		paired.Amount = newAmount
		paired.CategoryID = categoryID
		return s.txnRepo.Update(paired)
	}
	// Investment-side counterpart: no category column, so a category-only change
	// leaves it untouched.
	if !amountChanged || s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return s.investmentCounterpart.UpdateCounterpartAmount(s.q(), rowID, newAmount)
}

// splitCategoryNullable converts a split's plain CategoryID into a NullableID
// for mirroring onto a counter-transaction (whose category column is nullable).
// A nil category becomes an unset NullableID (which clears the counterpart's
// category).
func splitCategoryNullable(split *Split) types.NullableID {
	if split.CategoryID.IsNil() {
		return types.NullableID{Valid: false}
	}
	return types.NullableID{ID: split.CategoryID, Valid: true}
}

// ensureRetainedCounterpartMutable verifies a retained transfer line's
// counterpart can be re-synced, mirroring mirrorToPairedCounterpart's own
// blocking rule so ReplaceSplits/UpdateSplit fail cleanly before any write. A
// regular-side counterpart mirrors both fields, so a reconciled one always
// blocks; an investment-side counterpart is written only when the amount
// changed, so a reconciled one blocks only then (a category-only change never
// touches it).
func (s *Service) ensureRetainedCounterpartMutable(transferID types.ID, amountChanged bool) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		return nil
	}
	if !amountChanged || s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if found && reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return nil
}

// ensureCounterpartNotReconciled returns IsReconciledError if the counter-
// transaction linked to transferID (regular or investment table) is
// reconciled, and nil if it is unreconciled or absent.
func (s *Service) ensureCounterpartNotReconciled(transferID types.ID) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		return nil
	}
	if s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if found && reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return nil
}

// deletePairedSideOfMultiLine reverse-cascades a paired-side delete back to
// the parent multi-line transaction: the parent's transfer-line split-item
// is removed first, then the paired single-line counter-transaction itself.
// A reconciled parent blocks the cascade with IsReconciledError. The
// parent's other split-items are left intact even if the totals are now
// out of balance — the caller will reconcile on a later save.
func (s *Service) deletePairedSideOfMultiLine(paired *Transaction, parentSplit *Split) error {
	parent, err := s.txnRepo.GetByID(parentSplit.TransactionID)
	if err != nil {
		return err
	}
	if parent.IsReconciled() {
		return &IsReconciledError{ID: parent.ID.String()}
	}
	if err := s.splitRepo.Delete(parentSplit.ID); err != nil {
		return fmt.Errorf("failed to delete parent split-item: %w", err)
	}
	if err := s.txnRepo.Delete(paired.ID); err != nil {
		return fmt.Errorf("failed to delete paired transaction: %w", err)
	}
	return nil
}

// deleteTransferLinePairs deletes the paired counter-transaction of every
// transfer-typed split-line attached to the given parent transaction. A
// reconciled paired side blocks the cascade with IsReconciledError so the
// parent and its other splits remain intact.
func (s *Service) deleteTransferLinePairs(parentID types.ID) error {
	splits, err := s.splitRepo.ListByTransaction(parentID)
	if err != nil {
		return fmt.Errorf("failed to list splits for delete cascade: %w", err)
	}
	for _, split := range splits {
		if !split.TransferAccountID.Valid || !split.TransferID.Valid {
			continue
		}
		if err := s.deletePairedCounterTransaction(split.TransferID.ID); err != nil {
			return err
		}
	}
	return nil
}

// deletePairedCounterTransaction removes the single-line counter-transaction
// linked to a transfer-line's transfer_id. The counterpart may live on
// the regular transactions table (bank target) or on the
// investment_transactions table (investment target) — both are checked.
// Returns nil if no paired side exists; returns IsReconciledError if the
// paired side is reconciled.
func (s *Service) deletePairedCounterTransaction(transferID types.ID) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		if err := s.txnRepo.Delete(paired.ID); err != nil {
			return fmt.Errorf("failed to delete paired transfer transaction: %w", err)
		}
		return nil
	}
	if s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	if err := s.investmentCounterpart.DeleteCounterpart(s.q(), rowID); err != nil {
		return fmt.Errorf("failed to delete investment-side paired transfer transaction: %w", err)
	}
	return nil
}
