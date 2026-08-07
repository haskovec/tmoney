package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// The transaction status state machine: cleared, uncleared, void, and the restore
// path that undoes a void.
//
// Reconciled status is written here only as a guard to respect. It is SET by
// internal/reconciliation, which updates the status column directly through
// Repository.UpdateStatus for a documented DuckDB index reason.

// ClearTransaction marks a transaction as cleared.
// Void and reconciled transactions cannot be cleared directly.
func (s *Service) ClearTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Status-only change: use the narrow in-place UpdateStatus so DuckDB does
	// not rewrite the row (and cannot trip a desynced ART index on another
	// column). See transaction.Repository.UpdateStatus / migration 030.
	return s.txnRepo.UpdateStatus(id, StatusCleared)
}

// MarkTransactionUncleared marks a transaction as uncleared.
// Void and reconciled transactions cannot be marked uncleared directly.
// A reconciled transaction is unlocked by reconciliation.Service, which writes
// the prior status back through Repository.UpdateStatus, not by this package.
func (s *Service) MarkTransactionUncleared(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Status-only change: narrow in-place update (see ClearTransaction).
	return s.txnRepo.UpdateStatus(id, StatusUncleared)
}

// VoidTransaction voids a transaction by setting its amount to 0, memo to **VOID**,
// and status to void. For transfers, both sides are voided atomically.
// For split transactions, all splits are removed and any paired
// counter-transactions minted from transfer-line splits are cascade-
// deleted alongside the splits — matching the Delete cascade. Bank-side
// and investment-side counterparts are both handled; a reconciled
// counterpart blocks the void with IsReconciledError.
// Void and reconciled transactions cannot be voided.
func (s *Service) VoidTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Cannot void an already void transaction
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Cannot void a reconciled transaction
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// A closed account is frozen — block voids.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// A whole-transaction transfer is voided through transfer.Service, which
	// zeroes both legs wherever they live and refuses by name when a leg is on
	// the investment ledger (which has no void status). A split-line counterpart
	// falls through to the cascade below, as before.
	if txn.IsTransfer() {
		parentSplit, err := s.splitRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit == nil {
			return &IsTransferError{ID: id.String()}
		}
	}

	// Void the transaction
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	txn.Void()

	// Cascade to paired counter-transactions, drop the splits, and void the
	// parent in one transaction — otherwise a mid-cascade failure could orphan
	// a counterpart (dangling transfer_id) or leave the parent un-voided.
	return s.runInTx(func(b *Service) error {
		if err := b.deleteTransferLinePairs(id); err != nil {
			return err
		}
		if _, err := b.splitRepo.DeleteByTransaction(id); err != nil {
			return fmt.Errorf("failed to delete splits for void: %w", err)
		}
		return b.txnRepo.Update(txn)
	})
}

// RestoreVoidedTransaction restores a voided transaction to its original state.
// This is used by the undo system to reverse a void operation.
// Only void transactions can be restored.
func (s *Service) RestoreVoidedTransaction(id types.ID, amount types.Money, memo types.NullableString, status Status) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if !txn.IsVoid() {
		return fmt.Errorf("transaction %s is not void; cannot restore", id.String())
	}

	txn.Amount = amount
	txn.Memo = memo
	txn.SetStatus(status)

	// A single write today, but wrapped so it composes: phase 7's void-undo
	// command chains this with ReplaceSplits under one caller-supplied tx via
	// InTx, and a bound service joins that tx rather than opening its own.
	return s.runInTx(func(b *Service) error {
		return b.txnRepo.Update(txn)
	})
}

// RestoreVoidedTransactionWithSplits restores a voided transaction and, when the
// transaction had splits removed by the void, restores those splits too — all in
// one transaction. It is the composed method the void-undo command uses so the
// row restore and the split restore commit together (or not at all); the two
// bound calls join the single tx opened here. When splits is empty only the row
// is restored (matching a void of a plain, split-free transaction).
func (s *Service) RestoreVoidedTransactionWithSplits(id types.ID, amount types.Money, memo types.NullableString, status Status, splits []*Split) error {
	return s.runInTx(func(b *Service) error {
		if err := b.RestoreVoidedTransaction(id, amount, memo, status); err != nil {
			return err
		}
		if len(splits) > 0 {
			return b.ReplaceSplits(id, splits)
		}
		return nil
	})
}

// BalanceImpact represents the impact of a transaction on account balances.
type BalanceImpact struct {
	AccountID      types.ID
	Amount         types.Money
	IsTransferFrom bool
	IsTransferTo   bool
}
