package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// The split lifecycle: creating a transaction with splits, and adding, updating,
// removing or wholesale replacing the split lines of an existing one.
//
// Every method here that touches a transfer line delegates the counterpart work
// to service_transfer_line.go rather than writing the paired row itself.

// CreateWithSplits creates a transaction along with its splits. The signed
// sum of split amounts must equal the transaction amount. For each
// transfer-typed split-line (TransferAccountID set), the service mints a
// fresh TransferID, stores it on the split-item, and creates a paired
// single-line transaction in the target account with the inverted amount
// and matching transfer_id.
func (s *Service) CreateWithSplits(transaction *Transaction, splits []*Split) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	if err := s.validateSplits(transaction, splits); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
		return err
	}

	if transaction.HasCategory() && len(splits) > 0 {
		return &HasSplitsError{ID: transaction.ID.String()}
	}

	// Mint transfer_ids for transfer-lines so the split row and its paired
	// counter-transaction share the link from the first write.
	for _, split := range splits {
		if split.TransferAccountID.Valid && !split.TransferID.Valid {
			split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
		}
	}

	// Persist the parent, its split rows, and every transfer-line counterpart
	// in one transaction: a mid-flow failure rolls the whole thing back, so a
	// parent never lands without its splits (or with only some counterparts).
	return s.runInTx(func(b *Service) error {
		if err := b.txnRepo.Create(transaction); err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		for _, split := range splits {
			split.TransactionID = transaction.ID
			if err := b.splitRepo.Create(split); err != nil {
				return fmt.Errorf("failed to create split: %w", err)
			}

			if !split.TransferAccountID.Valid {
				continue
			}

			if err := b.createTransferLineCounterpart(transaction.AccountID, transaction.Date, split); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetSplits returns all splits for a transaction.
func (s *Service) GetSplits(transactionID types.ID) ([]*Split, error) {
	return s.splitRepo.ListByTransaction(transactionID)
}

// AddSplit adds a new split to an existing transaction.
// After adding, the splits must still sum to the transaction amount.
// Void and reconciled transactions cannot have splits added.
func (s *Service) AddSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	// Get the transaction
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	// Transfers cannot have splits
	if txn.IsTransfer() {
		return &TransferCannotHaveSplitsError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Create the split
	if err := s.splitRepo.Create(split); err != nil {
		return err
	}

	// Validate total (warning only - the user may be adding more splits)
	valid, err := s.splitRepo.ValidateSplitsAgainstTransaction(split.TransactionID)
	if err != nil {
		return fmt.Errorf("failed to validate splits: %w", err)
	}
	if !valid {
		// Return a specific error so caller knows splits don't match
		total, _ := s.splitRepo.GetTotalByTransaction(split.TransactionID)
		return &SplitTotalMismatchError{
			TransactionID:     split.TransactionID.String(),
			TransactionAmount: txn.Amount,
			SplitTotal:        total,
		}
	}

	return nil
}

// UpdateSplit updates an existing split.
// Splits on void or reconciled transactions cannot be updated.
//
// For transfer-line splits, the paired single-line counter-transaction in
// the target account is kept in sync:
//   - An amount edit mirrors the new (negated) amount onto the paired side.
//   - A target-account edit deletes the old paired side and creates a new
//     one in the new target with a fresh transfer_id.
//
// Self-transfers (transfer_account_id == parent.account_id) are rejected.
func (s *Service) UpdateSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	existing, err := s.splitRepo.GetByID(split.ID)
	if err != nil {
		return err
	}

	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	if split.TransferAccountID.Valid && split.TransferAccountID.ID == txn.AccountID {
		errors := types.ValidationErrors{}
		errors.Add("transfer_account_id",
			(&SelfTransferError{AccountID: txn.AccountID.String()}).Error())
		return &types.ServiceValidationError{Errors: errors}
	}

	wasTransfer := existing.TransferAccountID.Valid
	isTransfer := split.TransferAccountID.Valid

	if wasTransfer && isTransfer {
		targetMoved := existing.TransferAccountID.ID != split.TransferAccountID.ID
		if targetMoved {
			return s.moveTransferLine(txn, existing, split)
		}
		// Preserve the linkage so callers may omit transfer_id on edits.
		if !split.TransferID.Valid && existing.TransferID.Valid {
			split.TransferID = existing.TransferID
		}
		// Mirror an amount or category change onto the paired counterpart. A
		// category edit alone still cascades so the counterpart's label stays in
		// sync with the canonical split line. Pre-flight the counterpart before
		// persisting the split row so a reconciled counterpart fails cleanly with
		// no partial write.
		amountChanged := !existing.Amount.Equal(split.Amount)
		categoryChanged := existing.CategoryID != split.CategoryID
		cascade := existing.TransferID.Valid && (amountChanged || categoryChanged)
		if cascade {
			if err := s.ensureRetainedCounterpartMutable(existing.TransferID.ID, amountChanged); err != nil {
				return err
			}
		}
		// Persist the split row and mirror onto its counterpart atomically so a
		// failed mirror can't leave the split and counterpart out of sync.
		return s.runInTx(func(b *Service) error {
			if err := b.splitRepo.Update(split); err != nil {
				return err
			}
			if cascade {
				return b.mirrorToPairedCounterpart(existing.TransferID.ID, split.Amount.Neg(), splitCategoryNullable(split), amountChanged)
			}
			return nil
		})
	}

	return s.splitRepo.Update(split)
}

// DeleteSplit removes a split from a transaction.
// Splits on void or reconciled transactions cannot be deleted.
//
// For transfer-line splits, the paired single-line counter-transaction in
// the target account is also deleted. A reconciled paired side blocks the
// cascade with IsReconciledError. The parent transaction is left intact —
// its remaining splits may now leave the totals out of balance, which is
// the caller's responsibility to resolve on a subsequent save.
func (s *Service) DeleteSplit(splitID types.ID) error {
	// Get the split to find its parent transaction
	split, err := s.splitRepo.GetByID(splitID)
	if err != nil {
		return err
	}

	// Check the parent transaction status
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Delete the counterpart and the split row atomically so a mid-cascade
	// failure leaves both the split and its counterpart intact.
	return s.runInTx(func(b *Service) error {
		if split.TransferAccountID.Valid && split.TransferID.Valid {
			if err := b.deletePairedCounterTransaction(split.TransferID.ID); err != nil {
				return err
			}
		}
		return b.splitRepo.Delete(splitID)
	})
}

// ReplaceSplits replaces all splits for a transaction with new ones.
// The new splits must sum to the transaction amount.
// Void and reconciled transactions cannot have splits replaced.
//
// Transfer-line splits carry a paired single-line counter-transaction in
// their target account (regular or investment table). ReplaceSplits keeps
// those counterparts consistent by diffing the new transfer lines against
// the current ones rather than blindly dropping and recreating every row:
//
//   - a retained transfer line (matched by transfer_id, else by target
//     account) keeps its counterpart; an amount or category change mirrors
//     onto it;
//   - a removed transfer line's counterpart is deleted;
//   - an added transfer line mints a transfer_id and creates a counterpart
//     (carrying the line's category onto a regular-side counterpart).
//
// Callers may omit transfer_id on retained lines (the TUI split dialog does),
// so matching falls back to the target account. A reconciled counterpart that
// would be deleted or amount-changed blocks the whole operation before any
// mutation. Without this, a rewrite of a split set containing a transfer line
// trips the transfer_id/transfer_account_id pairing CHECK mid-flight and
// orphans the counterpart.
func (s *Service) ReplaceSplits(transactionID types.ID, splits []*Split) error {
	// Get the transaction
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Validate splits sum to transaction amount
	if err := s.validateSplits(txn, splits); err != nil {
		return err
	}

	// Diff the new transfer lines against the current split set so retained
	// counterparts survive the rewrite. Assigns transfer_ids onto the new
	// splits in place (retained → existing id, added → fresh id).
	oldSplits, err := s.splitRepo.ListByTransaction(transactionID)
	if err != nil {
		return fmt.Errorf("failed to list existing splits: %w", err)
	}
	plan := planSplitReplacement(oldSplits, splits)

	// Pre-flight every fallible counterpart operation before mutating anything,
	// so a reconciled or unroutable counterpart fails cleanly with no partial
	// write.
	if err := s.preflightSplitReplacement(plan); err != nil {
		return err
	}

	// Execute the whole plan — delete removed counterparts, re-sync retained
	// ones, rebuild the split rows, and mint counterparts for added lines — in
	// one transaction. A failure at any step rolls the entire rewrite back, so
	// the original splits and counterparts survive intact.
	return s.runInTx(func(b *Service) error {
		// Reconcile counterparts of removed and retained-changed transfer lines.
		for _, transferID := range plan.removedTransferIDs {
			if err := b.deletePairedCounterTransaction(transferID); err != nil {
				return err
			}
		}
		for _, change := range plan.retainedChanged {
			if err := b.mirrorToPairedCounterpart(change.transferID, change.newAmount.Neg(), change.newCategory, change.amountChanged); err != nil {
				return err
			}
		}

		// Rebuild the split rows. Retained transfer lines already carry their
		// original transfer_id (linking them to the still-live counterpart), so a
		// plain drop-and-recreate is safe.
		if _, err := b.splitRepo.DeleteByTransaction(transactionID); err != nil {
			return fmt.Errorf("failed to delete existing splits: %w", err)
		}
		for _, split := range splits {
			split.TransactionID = transactionID
			if err := b.splitRepo.Create(split); err != nil {
				return fmt.Errorf("failed to create split: %w", err)
			}
		}

		// Mint counterparts for the added transfer lines.
		for _, split := range plan.addedSplits {
			if err := b.createTransferLineCounterpart(txn.AccountID, txn.Date, split); err != nil {
				return err
			}
		}

		return nil
	})
}

// UpdateWithSplits updates a transaction's parent fields and replaces its splits
// as one atomic unit. It is the composed method the edit-with-splits undo command
// uses (for both Execute and Undo) so the parent update and the split rewrite
// commit together; the two bound calls join the single tx opened here. Update and
// ReplaceSplits are each individually transactional, but wrapping them here makes
// the pair atomic — a failure in the split rewrite rolls the parent update back.
func (s *Service) UpdateWithSplits(transaction *Transaction, splits []*Split) error {
	return s.runInTx(func(b *Service) error {
		if err := b.Update(transaction); err != nil {
			return err
		}
		return b.ReplaceSplits(transaction.ID, splits)
	})
}
