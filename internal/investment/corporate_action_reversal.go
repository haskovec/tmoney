package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Undoing a corporate action.
//
// A reversal is only safe when nothing downstream depends on the adjusted state,
// so every path here checks for later events first and refuses with a
// DownstreamEventsError rather than unwinding into a newer action. Merger reversal
// is not supported and says so.

// DeleteAction reverses a corporate action's effects (on lots, positions,
// and prices) and removes its audit row. Refuses to run if any investment
// transaction on the affected security(ies) has a date on or after the
// action date; the returned *DownstreamEventsError names the earliest
// blocking transaction so the caller can show a precise message.
//
// In v1, only stock-split and reverse-split actions are reversible.
// Merger and spin-off reversals create new transactions, lots, and
// (for mergers) hide the source security; safely undoing all of that
// requires careful per-account choreography and is deferred — those
// types return an *UnsupportedReversalError.
func (s *CorporateActionService) DeleteAction(actionID types.ID) error {
	ca, err := s.caRepo.GetByID(actionID)
	if err != nil {
		return err
	}

	switch ca.ActionType {
	case ActionTypeSplit, ActionTypeReverseSplit:
		if err := s.checkNoDownstreamEvents(ca); err != nil {
			return err
		}
		// The inverse lot/position/price adjustments plus the audit-row delete
		// commit atomically.
		return s.runInTx(func(b *CorporateActionService) error {
			return b.reverseSplit(ca)
		})
	case ActionTypeSpinOff:
		// The whole reversal — restored parent basis, deleted child
		// lots/positions/transactions/cash-in-lieu/price, and the audit-row
		// delete — commits atomically. The downstream-event guards live inside
		// reverseSpinOff and short-circuit before any write.
		return s.runInTx(func(b *CorporateActionService) error {
			return b.reverseSpinOff(ca)
		})
	case ActionTypeMerger:
		return &UnsupportedReversalError{ActionType: ca.ActionType}
	}
	return fmt.Errorf("unknown corporate action type: %s", ca.ActionType)
}

// reverseSplit undoes a Split or ReverseSplit by inverting the share
// and cost-basis multipliers applied at create time, then deleting the
// audit row.
func (s *CorporateActionService) reverseSplit(ca *CorporateAction) error {
	params, err := ParseSplitParams(ca.Parameters)
	if err != nil {
		return fmt.Errorf("failed to parse split params: %w", err)
	}

	// Original Split called adjust* with (ratio, inverseRatio). To undo,
	// swap them: shares *= inverseRatio, cost *= ratio, prices *= ratio.
	origRatio := alpacadecimal.NewFromFloat(params.Ratio())
	origInverse := alpacadecimal.NewFromFloat(1.0 / params.Ratio())

	if err := s.adjustLots(ca.SecurityID, ca.ActionDate, origInverse, origRatio); err != nil {
		return fmt.Errorf("failed to reverse lots: %w", err)
	}
	if err := s.adjustPositions(ca.SecurityID, ca.ActionDate, origRatio, true); err != nil {
		return fmt.Errorf("failed to reverse positions: %w", err)
	}
	if err := s.adjustPrices(ca.SecurityID, ca.ActionDate, origRatio); err != nil {
		return fmt.Errorf("failed to reverse prices: %w", err)
	}

	if err := s.caRepo.Delete(ca.ID); err != nil {
		return fmt.Errorf("failed to delete corporate action: %w", err)
	}
	return nil
}

// reverseSpinOff undoes a spin-off: it removes the spun-off child lots,
// positions, exchange/cash-in-lieu transactions, and the seeded child price,
// and restores the parent's cost basis. It refuses (with a *DownstreamEventsError
// naming the blocker) when the parent has any transaction on/after the spin date
// or when the child has any transaction other than the spin-off's own same-date
// exchange receipts — i.e. when the spun-off shares have been sold or otherwise
// used, since those consume the lots this reversal would delete.
func (s *CorporateActionService) reverseSpinOff(ca *CorporateAction) error {
	params, err := ParseSpinOffParams(ca.Parameters)
	if err != nil {
		return fmt.Errorf("failed to parse spin-off params: %w", err)
	}
	if !ca.TargetSecurityID.Valid {
		return fmt.Errorf("spin-off action %s has no target security", ca.ID)
	}
	parentID := ca.SecurityID
	childID := ca.TargetSecurityID.ID
	spinDate := ca.ActionDate

	// Guard A: the parent must have no transactions on/after the spin date
	// (the spin-off itself created none on the parent).
	earliest, err := s.invRepo.EarliestSinceDate(parentID, spinDate)
	if err != nil {
		return fmt.Errorf("failed to check parent downstream events: %w", err)
	}
	if earliest != nil {
		return s.downstreamError(parentID, spinDate, earliest.Date, string(earliest.Type))
	}

	// Guard B: the only child transactions may be the spin-off's own exchange
	// receipts dated the spin date. A sale, later buy, or transfer of the child
	// means the spun-off shares were used; refuse and name the blocker.
	childTxns, err := s.invRepo.ListBySecurity(childID)
	if err != nil {
		return fmt.Errorf("failed to list child transactions: %w", err)
	}
	for _, t := range childTxns {
		if t.Type != TransactionTypeExchange || !t.Date.Time().Equal(spinDate.Time()) {
			return s.downstreamError(childID, spinDate, t.Date, string(t.Type))
		}
	}

	// Restore parent cost basis (undo the × parentAllocPct scaling) and collect
	// the touched accounts for cash-in-lieu cleanup.
	parentAllocFrac := alpacadecimal.NewFromFloat(params.ParentAllocationPct).Div(alpacadecimal.NewFromInt(100))
	inverse := alpacadecimal.NewFromInt(1).Div(parentAllocFrac)
	touched := make(map[types.ID]bool)

	parentLots, err := s.lotRepo.GetOpenLotsBySecurity(parentID)
	if err != nil {
		return err
	}
	for _, lot := range parentLots {
		lot.CostPerShare = lot.CostPerShare.Mul(inverse)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to restore parent lot %s: %w", lot.ID, err)
		}
		touched[lot.AccountID] = true
	}
	parentPositions, err := s.positionRepo.GetPositionsBySecurity(parentID)
	if err != nil {
		return err
	}
	for _, pos := range parentPositions {
		pos.AverageCostPerShare = pos.AverageCostPerShare.Mul(inverse)
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to restore parent position: %w", err)
		}
		touched[pos.AccountID] = true
	}

	// Delete child lots and the exchange transactions that created them.
	for _, t := range childTxns {
		touched[t.AccountID] = true
		if lot, lerr := s.lotRepo.GetBySourceTransaction(t.ID); lerr == nil && lot != nil {
			if err := s.lotRepo.Delete(lot.ID); err != nil {
				return fmt.Errorf("failed to delete child lot %s: %w", lot.ID, err)
			}
		}
		if err := s.invRepo.Delete(t.ID); err != nil {
			return fmt.Errorf("failed to delete child exchange transaction %s: %w", t.ID, err)
		}
	}

	// Delete child positions (non-lot accounts).
	childPositions, err := s.positionRepo.GetPositionsBySecurity(childID)
	if err != nil {
		return err
	}
	for _, pos := range childPositions {
		if err := s.positionRepo.Delete(pos.AccountID, childID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return fmt.Errorf("failed to delete child position: %w", err)
			}
		}
	}

	// Delete cash-in-lieu deposits on the spin date in any touched account.
	for acctID := range touched {
		txns, err := s.invRepo.ListByAccount(acctID, TransactionFilter{})
		if err != nil {
			return fmt.Errorf("failed to list account transactions: %w", err)
		}
		for _, t := range txns {
			if t.Type == TransactionTypeDeposit && t.Date.Time().Equal(spinDate.Time()) &&
				t.Memo.Valid && t.Memo.String == "Spin-off cash-in-lieu for fractional shares" {
				if err := s.invRepo.Delete(t.ID); err != nil {
					return fmt.Errorf("failed to delete cash-in-lieu transaction %s: %w", t.ID, err)
				}
			}
		}
	}

	// Delete the seeded child price record on the spin date (best effort).
	if existing, perr := s.priceRepo.GetBySecurityAndDate(childID, spinDate); perr == nil && existing != nil {
		_ = s.priceRepo.Delete(existing.ID)
	}

	// Delete the audit row.
	if err := s.caRepo.Delete(ca.ID); err != nil {
		return fmt.Errorf("failed to delete corporate action: %w", err)
	}
	return nil
}

// checkNoDownstreamEvents returns a *DownstreamEventsError naming the
// earliest blocking transaction if any investment transaction on or
// after the action date exists for the action's security (or, for
// two-security actions like mergers and spin-offs, either security).
// Returns nil otherwise.
func (s *CorporateActionService) checkNoDownstreamEvents(ca *CorporateAction) error {
	secIDs := []types.ID{ca.SecurityID}
	if ca.TargetSecurityID.Valid {
		secIDs = append(secIDs, ca.TargetSecurityID.ID)
	}
	for _, secID := range secIDs {
		earliest, err := s.invRepo.EarliestSinceDate(secID, ca.ActionDate)
		if err != nil {
			return fmt.Errorf("failed to check downstream events: %w", err)
		}
		if earliest == nil {
			continue
		}
		ticker := ""
		if sec, err := s.secRepo.GetByID(secID); err == nil && sec != nil {
			ticker = sec.Ticker
		}
		return &DownstreamEventsError{
			ActionDate:     ca.ActionDate,
			BlockerTicker:  ticker,
			BlockerDate:    earliest.Date,
			BlockerTxnType: string(earliest.Type),
		}
	}
	return nil
}

// downstreamError builds a *DownstreamEventsError naming a blocking transaction.
func (s *CorporateActionService) downstreamError(secID types.ID, actionDate, blockerDate types.Date, blockerType string) *DownstreamEventsError {
	ticker := ""
	if sec, err := s.secRepo.GetByID(secID); err == nil && sec != nil {
		ticker = sec.Ticker
	}
	return &DownstreamEventsError{
		ActionDate:     actionDate,
		BlockerTicker:  ticker,
		BlockerDate:    blockerDate,
		BlockerTxnType: blockerType,
	}
}
