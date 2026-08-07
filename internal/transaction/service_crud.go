package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Plain create, read, update and delete for a transaction row.
//
// CRUD is not a layer above splits here — it reaches into both. Update probes
// splitRepo for transfer linkage and cascades an amount change into a parent
// split; Delete cascades into the paired counterpart rows. That entanglement is
// measured and accepted, not accidental.

// Create validates and creates a new transaction.
// If the transaction has a payee with a default category and no category is set,
// the default category will be auto-populated.
func (s *Service) Create(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
		return err
	}

	// Auto-populate category from payee's default if not set
	if !transaction.HasCategory() && transaction.HasPayee() {
		if err := s.applyPayeeDefaultCategory(transaction); err != nil {
			return err
		}
	}

	return s.txnRepo.Create(transaction)
}

// GetByID retrieves a transaction by its ID.
func (s *Service) GetByID(id types.ID) (*Transaction, error) {
	return s.txnRepo.GetByID(id)
}

// Update validates and updates an existing transaction.
// For transfers, use UpdateTransfer to update both sides.
// Void and reconciled transactions cannot be edited.
//
// If the transaction is the paired single-line counter-transaction of a
// multi-line split (i.e. its transfer_id matches a split-item's transfer_id),
// an amount change mirrors the new (negated) amount onto the parent's
// transfer-line split-item. A reconciled parent transaction blocks the
// reverse cascade with IsReconciledError.
func (s *Service) Update(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	// Check existing transaction state
	existing, err := s.txnRepo.GetByID(transaction.ID)
	if err != nil {
		return err
	}

	// Prevent editing void transactions
	if existing.IsVoid() {
		return &IsVoidError{ID: transaction.ID.String()}
	}

	// Prevent editing reconciled transactions
	if existing.IsReconciled() {
		return &IsReconciledError{ID: transaction.ID.String()}
	}

	// A whole-transaction transfer leg is not editable through here: this method
	// writes ONE row, and a transfer's counterpart may live in
	// investment_transactions. transfer.Service owns the pair — including
	// status-only changes, which go through transfer.SetLegStatus (that is what
	// the register's cleared toggle uses).
	//
	// The probe distinguishes the two kinds of transfer-linked row: a split
	// line's counterpart is owned by the split lifecycle and still edits here,
	// via the reverse cascade below.
	if existing.IsTransfer() && existing.TransferID.Valid {
		parentSplit, err := s.splitRepo.GetByTransferID(existing.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit == nil {
			return &IsTransferError{ID: transaction.ID.String()}
		}
	}

	// A closed account is frozen — block edits (incl. the cleared toggle,
	// which routes through here). Guard both the current account and, if the
	// edit moves the transaction, the destination account.
	if err := s.ensureAccountOpen(existing.AccountID); err != nil {
		return err
	}
	if transaction.AccountID != existing.AccountID {
		if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
			return err
		}
	}

	// Reverse cascade: if this transaction is the paired side of a multi-
	// line split, mirror an amount change back onto the parent's split-item.
	if existing.IsTransfer() && existing.TransferID.Valid {
		parentSplit, err := s.splitRepo.GetByTransferID(existing.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit != nil && !existing.Amount.Equal(transaction.Amount) {
			if err := s.cascadeAmountToParentSplit(parentSplit, transaction.Amount.Neg()); err != nil {
				return err
			}
		}
	}

	return s.txnRepo.Update(transaction)
}

// cascadeAmountToParentSplit mirrors an amount edit on the paired single-
// line counter-transaction back onto the parent multi-line split-item. The
// parent's split row is rewritten with the negated paired amount. A
// reconciled parent transaction blocks the cascade with IsReconciledError.
func (s *Service) cascadeAmountToParentSplit(parentSplit *Split, newSplitAmount types.Money) error {
	parent, err := s.txnRepo.GetByID(parentSplit.TransactionID)
	if err != nil {
		return err
	}
	if parent.IsReconciled() {
		return &IsReconciledError{ID: parent.ID.String()}
	}
	parentSplit.Amount = newSplitAmount
	if err := s.splitRepo.Update(parentSplit); err != nil {
		return fmt.Errorf("failed to mirror amount to parent split-item: %w", err)
	}
	return nil
}

// Delete removes a transaction.
// For transfers, this will delete both sides after confirmation.
// Void and reconciled transactions cannot be deleted.
func (s *Service) Delete(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Prevent deleting void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Prevent deleting reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// A closed account is frozen — block deletes.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// A transfer-linked row is one of two things, and only one of them belongs
	// to this service.
	if txn.IsTransfer() {
		// (a) The counterpart of a transfer LINE inside a multi-line split
		// (e.g. a paycheck's 401k contribution line). The split lifecycle owns
		// those, so the reverse cascade stays here: remove the parent's
		// transfer-line split-item, then the paired row.
		parentSplit, err := s.splitRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit != nil {
			return s.runInTx(func(b *Service) error {
				return b.deletePairedSideOfMultiLine(txn, parentSplit)
			})
		}

		// (b) A leg of a whole-transaction transfer, owned by
		// transfer.Service. Deleting one leg here would leave the counterpart
		// orphaned whenever it lives in investment_transactions — which the old
		// two-leg branch could not even see, since it read the pair through a
		// repository that assumed both legs were on `transactions`.
		return &IsTransferError{ID: id.String()}
	}

	// Cascade to paired counter-transactions of any transfer-typed split-
	// lines before removing the parent, then drop the splits and the parent —
	// all in one transaction so a mid-cascade failure leaves the whole
	// transaction (parent, splits, counterparts) intact. The parent itself is
	// not marked as a transfer (only the split-item carries the linkage), so
	// the legacy transfer branch above does not run.
	return s.runInTx(func(b *Service) error {
		if err := b.deleteTransferLinePairs(id); err != nil {
			return err
		}
		if _, err := b.splitRepo.DeleteByTransaction(id); err != nil {
			return fmt.Errorf("failed to delete splits: %w", err)
		}
		return b.txnRepo.Delete(id)
	})
}

// ListByAccount returns all transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.txnRepo.ListByAccount(accountID)
}

// ListByAccountAndDateRange returns transactions for an account within a date range.
func (s *Service) ListByAccountAndDateRange(accountID types.ID, startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByAccountAndDateRange(accountID, startDate, endDate)
}
