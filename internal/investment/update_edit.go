package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// statusFromRegular maps a transaction.Status (the bank-side status that the
// unified Transfer dialog edits) to the corresponding investment-side
// TransactionStatus. Uncleared maps to Pending — both represent the default
// "unposted" state in their respective domains. Void on the bank side has no
// investment equivalent and is treated as Pending (the dialog never produces
// it; this fallback only matters if a future caller passes through a void
// status).
func statusFromRegular(s transaction.Status) TransactionStatus {
	switch s {
	case transaction.StatusCleared:
		return TransactionStatusCleared
	case transaction.StatusReconciled:
		return TransactionStatusReconciled
	default:
		return TransactionStatusPending
	}
}

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

// UpdateBuy edits an existing buy transaction by reversing its
// position/lot effect, deleting the old record, and creating a new one
// with the supplied parameters. The reverse, delete, and re-create run in one
// transaction, so the edit either fully lands or the original is left intact —
// there is no partial "reversed but not reapplied" state to compensate for.
func (s *Service) UpdateBuy(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
) (*Transaction, error) {
	// Heal stored position/lot state for the target (account, security) in its
	// own committed tx before the edit tx, mirroring what Buy does when called
	// standalone. The bound Buy inside the tx skips its own re-heal.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.Buy(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo)
		return err
	}); err != nil {
		return nil, err
	}
	// Reconcile the auto-price at the old (security, date): drop it if this edit
	// orphaned it, or re-point it to a surviving same-day transaction. Best-effort
	// cosmetic cleanup, deliberately outside the edit tx.
	if old.SecurityID.Valid {
		s.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateSell edits an existing sell transaction. The reverse, delete, and
// re-create run in one transaction — the edit fully lands or the original is
// left intact.
func (s *Service) UpdateSell(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.Sell(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo, lotAllocations)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateFeeLiquidation edits an existing fee-via-liquidation transaction by
// reversing its share/lot effect, deleting the old record, and re-creating it
// with the supplied parameters. fee_liquidation has no net cash effect (the
// whole total_amount is the fee), so only share counts/lots are reversed —
// reverseTxnEffects routes fee_liquidation through the same share-removal arm as
// sell, so this mirrors UpdateSell exactly. The reverse, delete, and re-create
// run in one transaction — the edit fully lands or the original is left intact.
//
// FeeLiquidation computes its FIFO lot allocation from the post-reverse lot
// state: called on the bound service below, its lookups see the uncommitted
// reverse, so growing the share count past the pre-reverse remaining works.
func (s *Service) UpdateFeeLiquidation(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.FeeLiquidation(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo, lotAllocations)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateReinvestDividend edits an existing reinvest-dividend transaction. The
// reverse, delete, and re-create run in one transaction — the edit fully lands
// or the original is left intact.
func (s *Service) UpdateReinvestDividend(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	memo string,
) (*Transaction, error) {
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.ReinvestDividend(accountID, securityID, date, shares, totalAmount, pricePerShare, memo)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateDividend edits an existing cash dividend transaction. Dividends have no
// position/lot effect, so the flow is delete-old + create-new; both writes run
// in one transaction so a create failure leaves the original row intact.
func (s *Service) UpdateDividend(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
) (*Transaction, error) {
	if err := s.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Dividend(accountID, securityID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateDeposit edits an existing deposit transaction. Delete-old + create-new
// commit in one transaction.
func (s *Service) UpdateDeposit(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Deposit(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateWithdrawal edits an existing withdrawal transaction. Delete-old +
// create-new commit in one transaction.
func (s *Service) UpdateWithdrawal(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Withdrawal(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateFee edits an existing fee transaction. Delete-old + create-new commit in
// one transaction.
func (s *Service) UpdateFee(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Fee(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateInterest edits an existing interest transaction. Delete-old + create-new
// commit in one transaction.
func (s *Service) UpdateInterest(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Interest(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateTransferCash edits an existing cash transfer. Both sides of the
// original transfer are deleted before re-creating the pair.
//
// Dispatch is polymorphic on the type of the second account argument:
//
//   - If regularAccountID points at a non-investment account, the new pair is
//     created via TransferCash (direction="out") or DepositFromAccount
//     (direction="in"). The counterpart lives in the regular-transaction repo.
//   - If regularAccountID points at another investment account, the new pair is
//     created via TransferCashBetweenInvestments. For direction="out" the
//     investmentAccountID is the source; for direction="in" it is the
//     destination (i.e. the orientation flips). The counterpart lives in the
//     investment repo and is exposed on
//     CashTransferResult.CounterpartInvestmentTransaction.
//
// The old-counterpart cleanup searches both repos so an inv↔inv original is
// fully reaped before the new pair lands.
//
// status is the user-selected cleared/uncleared/reconciled state from the
// unified Transfer dialog's Status radio. It is applied to both freshly-
// created legs after the new pair lands. The investment legs receive the
// statusFromRegular-mapped TransactionStatus, the regular leg (if any)
// receives status verbatim.
func (s *Service) UpdateTransferCash(
	oldInvestmentTxnID types.ID,
	investmentAccountID, regularAccountID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	categoryID types.NullableID, // optional label for the regular-side leg (inv↔reg only)
	direction string, // "in" = cash arrives at investmentAccountID, "out" = cash leaves it
	status transaction.Status,
) (*CashTransferResult, error) {
	old, err := s.repo.GetByID(oldInvestmentTxnID)
	if err != nil {
		return nil, fmt.Errorf("failed to load transfer for edit: %w", err)
	}
	// A closed account is frozen — refuse before any destructive delete. Guard
	// BOTH legs of the EXISTING transfer (old.AccountID plus the old
	// counterpart on old.TransferAccountID, e.g. an inv↔inv destination) and
	// both NEW target accounts.
	if err := s.ensureAccountOpen(old.AccountID); err != nil {
		return nil, err
	}
	if old.TransferAccountID.Valid {
		if err := s.ensureAccountOpen(old.TransferAccountID.ID); err != nil {
			return nil, err
		}
	}
	if err := s.ensureAccountOpen(investmentAccountID); err != nil {
		return nil, err
	}
	if err := s.ensureAccountOpen(regularAccountID); err != nil {
		return nil, err
	}

	// Validate direction up front — before any destructive write — so the inv↔inv
	// branch and the inv↔reg branch share one error site.
	if direction != "in" && direction != "out" {
		return nil, fmt.Errorf("UpdateTransferCash: invalid direction %q (want 'in' or 'out')", direction)
	}

	// Dispatch on the second account's type (a read, safe outside the tx).
	otherAcct, err := s.accountRepo.GetByID(regularAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load destination account: %w", err)
	}

	// The old-counterpart reaping, the old-row delete, the new pair, and the
	// status writes all commit in one transaction: the edit fully lands or the
	// original transfer is left intact — no orphaned counterpart to compensate.
	var result *CashTransferResult
	if err := s.runInTx(func(b *Service) error {
		if old.TransferID.Valid {
			// Regular-side counterpart (inv↔reg original).
			if b.txnRepo != nil {
				if regList, lerr := b.txnRepo.ListByTransferID(old.TransferID.ID); lerr == nil {
					for _, r := range regList {
						if derr := b.txnRepo.Delete(r.ID); derr != nil {
							return fmt.Errorf("failed to delete regular-side counterpart: %w", derr)
						}
					}
				}
			}
			// Investment-side counterpart (inv↔inv original): lives in the
			// other investment account, identified by transfer_account_id.
			if old.TransferAccountID.Valid {
				if others, lerr := b.repo.ListByAccount(old.TransferAccountID.ID, TransactionFilter{}); lerr == nil {
					for _, o := range others {
						if o.TransferID.Valid && o.TransferID.ID == old.TransferID.ID && o.ID != old.ID {
							if derr := b.repo.Delete(o.ID); derr != nil {
								return fmt.Errorf("failed to delete investment-side counterpart: %w", derr)
							}
						}
					}
				}
			}
		}
		if derr := b.repo.Delete(oldInvestmentTxnID); derr != nil {
			return fmt.Errorf("failed to delete transfer for edit: %w", derr)
		}

		if otherAcct.Type.IsInvestmentType() {
			// inv↔inv: route to TransferCashBetweenInvestments.
			var srcID, dstID types.ID
			switch direction {
			case "out":
				srcID, dstID = investmentAccountID, regularAccountID
			case "in":
				srcID, dstID = regularAccountID, investmentAccountID
			}
			invResult, ierr := b.TransferCashBetweenInvestments(srcID, dstID, date, amount, memo)
			if ierr != nil {
				return ierr
			}
			if serr := b.applyInvestmentStatus(invResult.SourceTransaction, status); serr != nil {
				return serr
			}
			if serr := b.applyInvestmentStatus(invResult.DestinationTransaction, status); serr != nil {
				return serr
			}
			result = &CashTransferResult{
				InvestmentTransaction:            invResult.SourceTransaction,
				CounterpartInvestmentTransaction: invResult.DestinationTransaction,
				TransferID:                       invResult.TransferID,
			}
			return nil
		}

		var rerr error
		switch direction {
		case "out":
			result, rerr = b.TransferCash(investmentAccountID, regularAccountID, date, amount, memo, categoryID)
		case "in":
			result, rerr = b.DepositFromAccount(investmentAccountID, regularAccountID, date, amount, memo, categoryID)
		}
		if rerr != nil {
			return rerr
		}
		if serr := b.applyInvestmentStatus(result.InvestmentTransaction, status); serr != nil {
			return serr
		}
		return b.applyRegularStatus(result.RegularTransaction, status)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// applyInvestmentStatus persists the status mapped from a transaction.Status
// onto an investment-side leg. No-op when the row already carries the target
// status (avoids a needless write on the default-Pending case).
func (s *Service) applyInvestmentStatus(txn *Transaction, status transaction.Status) error {
	if txn == nil {
		return nil
	}
	target := statusFromRegular(status)
	if txn.Status == target {
		return nil
	}
	txn.SetStatus(target)
	if err := s.repo.Update(txn); err != nil {
		return fmt.Errorf("failed to update investment leg status: %w", err)
	}
	return nil
}

// applyRegularStatus persists the status onto a regular-side leg. No-op when
// the row already carries the target status.
func (s *Service) applyRegularStatus(txn *transaction.Transaction, status transaction.Status) error {
	if txn == nil {
		return nil
	}
	if txn.Status == status {
		return nil
	}
	if s.txnRepo == nil {
		return fmt.Errorf("transaction repository not configured")
	}
	txn.SetStatus(status)
	if err := s.txnRepo.Update(txn); err != nil {
		return fmt.Errorf("failed to update regular leg status: %w", err)
	}
	return nil
}

// UpdateTransferShares edits an existing share transfer between two
// investment accounts. Both sides are reversed before creating the new pair.
func (s *Service) UpdateTransferShares(
	oldSourceTxnID types.ID,
	sourceAccountID, destAccountID types.ID,
	date types.Date,
	securityID types.ID,
	shares types.Quantity,
	memo string,
	lotAllocations []SellLotAllocation,
) (*ShareTransferResult, error) {
	srcOld, err := s.repo.GetByID(oldSourceTxnID)
	if err != nil {
		return nil, fmt.Errorf("failed to load source transfer for edit: %w", err)
	}
	if !srcOld.TransferID.Valid {
		return nil, fmt.Errorf("UpdateTransferShares: txn %s is not a share transfer", oldSourceTxnID)
	}
	// A closed account is frozen — refuse before any destructive reverse/delete.
	// Guard BOTH legs of the EXISTING transfer (old source + old destination,
	// which lives on srcOld.TransferAccountID) as well as both NEW target
	// accounts, mirroring the transaction package's checkTransferEditable. A
	// share-only account can be closed (the balance check is cash-only), so the
	// old destination must be checked or its leg would be silently
	// reversed/deleted below.
	if err := s.ensureAccountOpen(srcOld.AccountID); err != nil {
		return nil, err
	}
	if srcOld.TransferAccountID.Valid {
		if err := s.ensureAccountOpen(srcOld.TransferAccountID.ID); err != nil {
			return nil, err
		}
	}
	if err := s.ensureAccountOpen(sourceAccountID); err != nil {
		return nil, err
	}
	if err := s.ensureAccountOpen(destAccountID); err != nil {
		return nil, err
	}
	// Find the destination side by transfer_id.
	var dstOld *Transaction
	all, err := s.repo.ListByAccount(srcOld.TransferAccountID.ID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list destination transfers: %w", err)
	}
	for _, t := range all {
		if t.TransferID.Valid && t.TransferID.ID == srcOld.TransferID.ID && t.ID != srcOld.ID {
			dstOld = t
			break
		}
	}

	// Heal both legs' stored state in their own committed txs before the edit tx,
	// mirroring what TransferShares does when called standalone. The bound
	// TransferShares inside the tx skips its own re-heal.
	if err := s.healInOwnTx(sourceAccountID, securityID); err != nil {
		return nil, err
	}
	if err := s.healInOwnTx(destAccountID, securityID); err != nil {
		return nil, err
	}

	// Reverse both legs, delete both old rows, and create the new pair in one
	// transaction — the edit fully lands or the original pair is left intact.
	var result *ShareTransferResult
	if err := s.runInTx(func(b *Service) error {
		if err := b.reverseTxnEffects(srcOld); err != nil {
			return err
		}
		if dstOld != nil {
			if err := b.reverseTxnEffects(dstOld); err != nil {
				return err
			}
			if err := b.repo.Delete(dstOld.ID); err != nil {
				return fmt.Errorf("failed to delete destination transfer for edit: %w", err)
			}
		}
		if err := b.repo.Delete(oldSourceTxnID); err != nil {
			return fmt.Errorf("failed to delete source transfer for edit: %w", err)
		}
		var terr error
		result, terr = b.TransferShares(sourceAccountID, destAccountID, securityID, date, shares, memo, lotAllocations)
		return terr
	}); err != nil {
		return nil, err
	}
	return result, nil
}
