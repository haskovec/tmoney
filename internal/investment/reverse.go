package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Undoing a transaction's effect on positions and lots.
//
// These stay on Service and are NOT part of the edit family, even though editing
// is their loudest caller. DeleteTransaction needs them too, so neither owner is
// exclusive — which is why update_edit.go was split by owner rather than by
// file. Callers MUST reverse before deleting the underlying row, because
// Repository.Delete cascades the junction rows the reversal reads.

// reverseTxnEffects undoes a transaction's effect on positions and lots.
// Callers MUST invoke this *before* deleting the underlying transaction,
// because Repository.Delete cascades the junction rows we rely on.
func (s *Service) reverseTxnEffects(txn *Transaction) error {
	switch txn.Type {
	case TransactionTypeBuy, TransactionTypeReinvestDividend:
		return s.reverseShareAddition(txn)
	case TransactionTypeSell, TransactionTypeFeeLiquidation:
		return s.reverseShareRemoval(txn)
	case TransactionTypeTransferShares:
		return s.reverseTransferShares(txn)
	case TransactionTypeDividend,
		TransactionTypeFee,
		TransactionTypeDeposit,
		TransactionTypeWithdrawal,
		TransactionTypeInterest,
		TransactionTypeTransferCash:
		return nil
	default:
		return fmt.Errorf("reverseTxnEffects: unsupported transaction type %s", txn.Type)
	}
}

// reverseShareAddition undoes a buy/reinvest's effect on the account's position or lot.
func (s *Service) reverseShareAddition(txn *Transaction) error {
	if !txn.SecurityID.Valid || !txn.Shares.Valid || !txn.PricePerShare.Valid {
		return fmt.Errorf("reverseShareAddition: missing security/shares/price on txn %s", txn.ID)
	}

	acct, err := s.getInvestmentAccount(txn.AccountID)
	if err != nil {
		return fmt.Errorf("reverseShareAddition: %w", err)
	}

	if acct.TrackLots {
		lot, err := s.lotRepo.GetBySourceTransaction(txn.ID)
		if err != nil {
			if _, ok := err.(*dberrors.NotFoundError); ok {
				// No lot to reverse (already gone) — nothing to do.
				return nil
			}
			return fmt.Errorf("reverseShareAddition: %w", err)
		}
		// Reject if the lot has been partially consumed by later sells —
		// otherwise we'd corrupt cost basis on the consuming junctions.
		if lot.Shares.Cmp(lot.OriginalShares) != 0 {
			return fmt.Errorf("cannot edit buy/reinvest: lot %s has already been sold against; revoke or edit the dependent sells first", lot.ID)
		}
		if err := s.lotRepo.Delete(lot.ID); err != nil {
			return fmt.Errorf("reverseShareAddition: %w", err)
		}
		return nil
	}

	pos, err := s.positionRepo.GetByAccountAndSecurity(txn.AccountID, txn.SecurityID.ID)
	if err != nil {
		return fmt.Errorf("reverseShareAddition: %w", err)
	}
	addedCost := txn.PricePerShare.Money.Mul(txn.Shares.Quantity.Decimal())
	currentCost := pos.AverageCostPerShare.Mul(pos.Shares.Decimal())
	newCost := currentCost.Sub(addedCost)
	newShares := pos.Shares.Sub(txn.Shares.Quantity)

	if newShares.IsZero() {
		if err := s.positionRepo.Delete(txn.AccountID, txn.SecurityID.ID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return fmt.Errorf("reverseShareAddition: %w", err)
			}
		}
		return nil
	}
	pos.Shares = newShares
	pos.AverageCostPerShare = newCost.Mul(alpacadecimal.NewFromInt(1).Div(newShares.Decimal()))
	if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
		return fmt.Errorf("reverseShareAddition: %w", err)
	}
	return nil
}

// reverseShareRemoval undoes a sell/fee-liquidation's effect on the account's position or lot.
func (s *Service) reverseShareRemoval(txn *Transaction) error {
	if !txn.SecurityID.Valid || !txn.Shares.Valid {
		return fmt.Errorf("reverseShareRemoval: missing security/shares on txn %s", txn.ID)
	}

	acct, err := s.getInvestmentAccount(txn.AccountID)
	if err != nil {
		return fmt.Errorf("reverseShareRemoval: %w", err)
	}

	if acct.TrackLots {
		junctions, err := s.transactionLotRepo.GetByTransaction(txn.ID)
		if err != nil {
			return fmt.Errorf("reverseShareRemoval: %w", err)
		}
		for _, j := range junctions {
			lot, err := s.lotRepo.GetByID(j.LotID)
			if err != nil {
				return fmt.Errorf("reverseShareRemoval: %w", err)
			}
			newShares := lot.Shares.Add(j.Shares)
			if err := s.lotRepo.UpdateSharesAndClosed(lot.ID, newShares, false); err != nil {
				return fmt.Errorf("reverseShareRemoval: %w", err)
			}
		}
		return nil
	}

	pos, err := s.positionRepo.GetByAccountAndSecurity(txn.AccountID, txn.SecurityID.ID)
	if err != nil {
		return fmt.Errorf("reverseShareRemoval: %w", err)
	}
	// Sells/fee-liquidations don't change avg cost, only share count.
	pos.Shares = pos.Shares.Add(txn.Shares.Quantity)
	if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
		return fmt.Errorf("reverseShareRemoval: %w", err)
	}
	return nil
}

// reverseTransferShares undoes one side of a share transfer.
// A negative total is the source side (shares left → restore them);
// a positive total is the destination side (shares arrived → remove them).
func (s *Service) reverseTransferShares(txn *Transaction) error {
	if !txn.SecurityID.Valid || !txn.Shares.Valid {
		return fmt.Errorf("reverseTransferShares: missing security/shares on txn %s", txn.ID)
	}

	if txn.TotalAmount.IsNegative() {
		// Source side: behaves like a sell for the purpose of share counts.
		return s.reverseShareRemoval(txn)
	}

	// Destination side: behaves like a buy/reinvest.
	if txn.PricePerShare.Valid {
		return s.reverseShareAddition(txn)
	}
	// Destination side lacks a per-share price field on lot-tracked accounts
	// when the destination is non-lot-tracked. Compute the implicit price.
	acct, err := s.getInvestmentAccount(txn.AccountID)
	if err != nil {
		return fmt.Errorf("reverseTransferShares: %w", err)
	}
	if acct.TrackLots {
		// Lot-tracked dest: delete the lot that this transfer created.
		lot, err := s.lotRepo.GetBySourceTransaction(txn.ID)
		if err != nil {
			if _, ok := err.(*dberrors.NotFoundError); ok {
				return nil
			}
			return fmt.Errorf("reverseTransferShares: %w", err)
		}
		if lot.Shares.Cmp(lot.OriginalShares) != 0 {
			return fmt.Errorf("cannot edit transfer-in: lot %s has already been sold against", lot.ID)
		}
		return s.lotRepo.Delete(lot.ID)
	}
	// Non-lot-tracked dest: compute implicit price-per-share from total/shares.
	if txn.Shares.Quantity.IsZero() {
		return fmt.Errorf("reverseTransferShares: zero shares on txn %s", txn.ID)
	}
	implicit := txn.TotalAmount.Mul(alpacadecimal.NewFromInt(1).Div(txn.Shares.Quantity.Decimal()))
	pos, err := s.positionRepo.GetByAccountAndSecurity(txn.AccountID, txn.SecurityID.ID)
	if err != nil {
		return fmt.Errorf("reverseTransferShares: %w", err)
	}
	addedCost := implicit.Mul(txn.Shares.Quantity.Decimal())
	currentCost := pos.AverageCostPerShare.Mul(pos.Shares.Decimal())
	newCost := currentCost.Sub(addedCost)
	newShares := pos.Shares.Sub(txn.Shares.Quantity)
	if newShares.IsZero() {
		if err := s.positionRepo.Delete(txn.AccountID, txn.SecurityID.ID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return fmt.Errorf("reverseTransferShares: %w", err)
			}
		}
		return nil
	}
	pos.Shares = newShares
	pos.AverageCostPerShare = newCost.Mul(alpacadecimal.NewFromInt(1).Div(newShares.Decimal()))
	if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
		return fmt.Errorf("reverseTransferShares: %w", err)
	}
	return nil
}

// loadAndReverseForEdit fetches the existing transaction, reverses its
// position/lot effects, and deletes the record. On any failure it leaves the
// original state intact and returns the error.
func (s *Service) loadAndReverseForEdit(oldID types.ID) (*Transaction, error) {
	old, err := s.repo.GetByID(oldID)
	if err != nil {
		return nil, fmt.Errorf("failed to load transaction for edit: %w", err)
	}
	// Refuse to edit a transaction on a closed account BEFORE the destructive
	// reverse/delete runs (a closed account is frozen).
	if err := s.ensureAccountOpen(old.AccountID); err != nil {
		return nil, err
	}
	if err := s.reverseTxnEffects(old); err != nil {
		return nil, err
	}
	if err := s.repo.Delete(oldID); err != nil {
		return nil, fmt.Errorf("failed to delete transaction for edit: %w", err)
	}
	return old, nil
}

// guardEditByOldID blocks an edit when the transaction being replaced lives on
// a closed account, before any destructive delete runs. Used by the cash-type
// edit methods that delete-then-recreate without going through
// loadAndReverseForEdit.
func (s *Service) guardEditByOldID(oldID types.ID) error {
	old, err := s.repo.GetByID(oldID)
	if err != nil {
		return fmt.Errorf("failed to load transaction for closed check: %w", err)
	}
	return s.ensureAccountOpen(old.AccountID)
}
