package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Deleting an investment transaction and unwinding everything it caused.
//
// The reverse helpers this depends on live in reverse.go and are shared with the
// edit family, which is why they do not belong to either one exclusively.

// DeleteTransaction deletes an investment transaction and cascades to its
// paired counterpart when the transaction is part of a transfer:
//   - transfer_cash: also deletes the paired regular-side row(s) in the
//     transactions table (linked by transfer_id).
//   - transfer_shares: also deletes the paired investment-side row in the
//     other investment account (also linked by transfer_id).
//
// Without this cascade the user is left with an orphaned counterpart that
// still has transfer_id set, which is what happened to the savings-side
// row when the wrong-direction cash transfer was deleted from the
// investment register.
//
// Non-transfer transactions are simply forwarded to repo.Delete.
func (s *Service) DeleteTransaction(id types.ID) error {
	txn, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("failed to load transaction for delete: %w", err)
	}

	// Exchange rows are created and unwound exclusively by the corporate-action
	// engine (mergers/spin-offs); deleting one directly would desync the
	// action's lots/positions from cost basis. Refuse and point at the proper
	// reversal path. (CA reversal deletes these via the raw repository, not here.)
	if txn.Type == TransactionTypeExchange {
		return fmt.Errorf("cannot delete a corporate-action exchange transaction directly; reverse the corporate action from the Corporate Action History view instead")
	}

	// A closed account is frozen — refuse the delete BEFORE any destructive
	// reversal runs. For transfers, block if either leg is a closed account.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}
	if txn.TransferAccountID.Valid {
		if err := s.ensureAccountOpen(txn.TransferAccountID.ID); err != nil {
			return err
		}
	}

	// A transfer_cash row is a LEG of a transfer, and its counterpart may be in
	// the other ledger. Refuse it here: transfer.Service owns the pair and
	// deletes both legs in one transaction, wherever they live.
	//
	// This replaces a cascade that reached into transaction.Repository to find
	// and delete the regular-side rows — the last thing in this package that
	// needed to know the other ledger exists, and the reason internal/investment
	// imported internal/transaction at all.
	//
	// Share transfers are NOT refused: they are owned here, and their cascade
	// below is unchanged.
	if txn.TransferID.Valid && txn.Type == TransactionTypeTransferCash {
		return &IsCashTransferLegError{ID: id.String(), TransferID: txn.TransferID.ID.String()}
	}

	// Reverse this transaction's effect on positions and lots BEFORE deleting
	// any rows: Repository.Delete cascades the junction rows that
	// reverseShareRemoval reads, so the reversal must run first (see
	// reverseTxnEffects' contract). This mirrors the reverse-then-apply the
	// Update* methods use — it restores a deleted sell's consumed lots, removes
	// the orphan lot a deleted buy/reinvest opened, and reverses non-lot
	// positions. Cash-only types reverse to a no-op. The reversal, the
	// counterpart cascade, and the row delete commit atomically.
	return s.runInTx(func(b *Service) error {
		if err := b.reverseTxnEffects(txn); err != nil {
			return fmt.Errorf("failed to reverse transaction effects before delete: %w", err)
		}

		if txn.TransferID.Valid {
			switch txn.Type {
			case TransactionTypeTransferShares:
				// The counterpart lives in the other investment account; find by transfer_id.
				others, lerr := b.repo.ListByAccount(txn.TransferAccountID.ID, TransactionFilter{})
				if lerr != nil {
					return fmt.Errorf("failed to list destination-account transfers: %w", lerr)
				}
				for _, o := range others {
					if o.TransferID.Valid && o.TransferID.ID == txn.TransferID.ID && o.ID != txn.ID {
						// Reverse the counterpart's share effect (restore source
						// lots / remove the dest lot) before its row + junctions are
						// cascaded away.
						if err := b.reverseTxnEffects(o); err != nil {
							return fmt.Errorf("failed to reverse paired share-transfer effects: %w", err)
						}
						if err := b.repo.Delete(o.ID); err != nil {
							return fmt.Errorf("failed to delete paired share-transfer row: %w", err)
						}
					}
				}
			}
		}

		if err := b.repo.Delete(id); err != nil {
			return fmt.Errorf("failed to delete investment transaction: %w", err)
		}
		// Reconcile the auto-price this transaction may have seeded: drop it if
		// the delete orphaned it, or re-point it to a surviving same-day
		// transaction.
		if txn.SecurityID.Valid && txn.Type.CreatesAutoPrice() {
			b.cleanupAutoPrice(txn.SecurityID.ID, txn.Date)
		}
		return nil
	})
}
