package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// Share splits and reverse splits.
//
// A split multiplies share counts and divides per-share costs and prices across
// lots, positions and price history. CatchUpSplitsFor* applies any split a lot or
// transaction missed because it was entered after the split date.

// Split applies a stock split (or reverse split) to a security.
// It adjusts all open lots, non-zero positions, and historical prices on or before the split date.
// A corporate action audit record is created.
func (s *CorporateActionService) Split(securityID types.ID, splitDate types.Date, params SplitParams) (*CorporateAction, error) {
	// Validate parameters
	if errs := params.Validate(); errs.HasErrors() {
		return nil, fmt.Errorf("invalid split parameters: %s", errs.Error())
	}

	ratio := alpacadecimal.NewFromFloat(params.Ratio())
	inverseRatio := alpacadecimal.NewFromFloat(1.0 / params.Ratio())

	// Create audit record
	actionType := ActionTypeSplit
	if params.Ratio() < 1 {
		actionType = ActionTypeReverseSplit
	}

	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize split params: %w", err)
	}

	ca := NewCorporateAction(actionType, securityID, splitDate, paramsJSON)

	// The lot/position/price adjustments plus the audit row commit atomically:
	// a split either fully lands or not at all.
	if err := s.runInTx(func(b *CorporateActionService) error {
		// Adjust open lots purchased on or before the split date. Shares acquired
		// after the split were already recorded at post-split quantities, so they
		// must NOT be re-split.
		if err := b.adjustLots(securityID, splitDate, ratio, inverseRatio); err != nil {
			return fmt.Errorf("failed to adjust lots: %w", err)
		}

		// Bring positions in line with the shares actually held as of the split date.
		if err := b.adjustPositions(securityID, splitDate, ratio, false); err != nil {
			return fmt.Errorf("failed to adjust positions: %w", err)
		}

		// Adjust price history on or before split date
		if err := b.adjustPrices(securityID, splitDate, inverseRatio); err != nil {
			return fmt.Errorf("failed to adjust prices: %w", err)
		}

		if err := b.caRepo.Create(ca); err != nil {
			return fmt.Errorf("failed to create corporate action record: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return ca, nil
}

// adjustLots adjusts open lots that existed as of the split date by the split
// multipliers. Shares and original_shares are multiplied by shareMul;
// cost_per_share by costMul. Lots whose purchase date is AFTER the split date
// are left untouched — they were acquired post-split and are already at
// split-adjusted quantities.
//
// original_shares is scaled in lock-step with shares so the lot's
// "remaining = original − consumed" invariant survives a split (consumed
// junction shares are recorded post-split). reverseSplit passes the inverse
// multipliers, so a forward-then-reverse round-trips original_shares exactly.
// Keeping original_shares aligned also means the buy/reinvest edit guard
// (shares != original_shares ⇒ "already sold against") no longer mis-fires on
// a freshly-split lot, and lot-tracked heal can recompute shares correctly.
func (s *CorporateActionService) adjustLots(securityID types.ID, splitDate types.Date, shareMul, costMul alpacadecimal.Decimal) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return err
	}

	for _, lot := range lots {
		if lot.PurchaseDate.Time().After(splitDate.Time()) {
			continue // acquired after the split — already split-adjusted
		}
		lot.Shares = lot.Shares.Mul(shareMul)
		lot.OriginalShares = lot.OriginalShares.Mul(shareMul)
		lot.CostPerShare = lot.CostPerShare.Mul(costMul)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot %s: %w", lot.ID.String(), err)
		}
	}
	return nil
}

// adjustPositions brings each account's stored position in line with the split,
// scoped to shares held as of the split date. For lot-tracking accounts the
// position is recomputed from the (already date-adjusted) open lots, so it
// stays consistent with them. For non-lot accounts the share count held as of
// the split date is split; shares acquired afterward are left unchanged, and
// total cost basis is preserved (a split never changes total invested). When
// reverse is true the adjustment is inverted (used to undo a split).
func (s *CorporateActionService) adjustPositions(securityID types.ID, splitDate types.Date, ratio alpacadecimal.Decimal, reverse bool) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return err
	}

	// Lot-tracking accounts: rebuild the aggregate position from the (already
	// date-adjusted) open lots so it stays consistent with them. This upserts
	// the row even when one didn't exist yet (a lot-tracking account holds its
	// shares in lots; the aggregate position is a derived cache).
	lotsByAccount := make(map[types.ID][]*Lot)
	for _, l := range lots {
		lotsByAccount[l.AccountID] = append(lotsByAccount[l.AccountID], l)
	}
	for accountID, accountLots := range lotsByAccount {
		rebuilt := NewPosition(accountID, securityID)
		for _, l := range accountLots {
			if err := rebuilt.AddShares(l.Shares, l.CostPerShare); err != nil {
				return fmt.Errorf("failed to rebuild position for account %s: %w", accountID.String(), err)
			}
		}
		if err := s.positionRepo.CreateOrUpdate(&rebuilt); err != nil {
			return fmt.Errorf("failed to update position for account %s: %w", accountID.String(), err)
		}
	}

	// Non-lot accounts: split only the shares held as of the split date; shares
	// acquired afterward stay put. Total cost basis is preserved.
	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return err
	}
	one := alpacadecimal.NewFromInt(1)
	for _, pos := range positions {
		if _, ok := lotsByAccount[pos.AccountID]; ok {
			continue // lot-tracking account, already rebuilt above
		}
		asOf, err := s.sharesHeldAsOf(pos.AccountID, securityID, splitDate)
		if err != nil {
			return err
		}
		if asOf.IsZero() || asOf.IsNegative() {
			continue // held nothing at the split date — nothing to adjust
		}
		delta := asOf.Mul(ratio.Sub(one)) // asOf × (ratio − 1)
		oldTotalCost := pos.AverageCostPerShare.Mul(pos.Shares.Decimal())
		if reverse {
			pos.Shares = pos.Shares.Sub(delta)
		} else {
			pos.Shares = pos.Shares.Add(delta)
		}
		if pos.Shares.IsZero() || pos.Shares.IsNegative() {
			continue
		}
		pos.AverageCostPerShare = oldTotalCost.Mul(one.Div(pos.Shares.Decimal()))
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to update position for account %s: %w", pos.AccountID.String(), err)
		}
	}
	return nil
}

// adjustPrices adjusts all prices on or before the split date by dividing by the ratio.
func (s *CorporateActionService) adjustPrices(securityID types.ID, splitDate types.Date, inverseRatio alpacadecimal.Decimal) error {
	prices, err := s.priceRepo.GetPriceHistory(securityID, nil, &splitDate)
	if err != nil {
		return err
	}

	for _, p := range prices {
		adjustedPrice := p.Price.Mul(inverseRatio)
		updated := price.NewPrice(p.SecurityID, p.Date, adjustedPrice, p.Source)
		if err := s.priceRepo.CreateOrUpdate(updated); err != nil {
			return fmt.Errorf("failed to update price for date %s: %w", p.Date.String(), err)
		}
	}
	return nil
}

// sharesHeldAsOf returns the net share count an account held in a security on
// or before the given date, replayed from the transaction ledger. Used to
// scope a split to the shares that existed when it occurred.
func (s *CorporateActionService) sharesHeldAsOf(accountID, securityID types.ID, asOfDate types.Date) (types.Quantity, error) {
	return netSharesHeldAsOf(s.invRepo, accountID, securityID, asOfDate)
}

// SplitLot applies a split ratio to a single open lot. It is a targeted repair
// for a lot entered AFTER a global split had already run (so the global action
// never scaled it): it brings just that lot into the post-split state the global
// action would have produced. Shares, original_shares, and the per-share cost
// are scaled by the ratio, then the (account, security) aggregate position is
// recomputed from the account's open lots.
//
// Unlike Split, SplitLot creates NO corporate-action audit record — the global
// action (if any) remains the record of the event; this only patches a lot that
// was added too late to be caught by it. It refuses on a lot that has already
// been sold against (shares != original_shares) or is closed, since scaling a
// consumed lot without also scaling its junction records would corrupt realized
// gain on the dependent sells.
func (s *CorporateActionService) SplitLot(lotID types.ID, params SplitParams) (*Lot, error) {
	if errs := params.Validate(); errs.HasErrors() {
		return nil, fmt.Errorf("invalid split parameters: %s", errs.Error())
	}
	lot, err := s.lotRepo.GetByID(lotID)
	if err != nil {
		return nil, err
	}
	if lot.Closed || lot.Shares.Cmp(lot.OriginalShares) != 0 {
		return nil, fmt.Errorf("cannot split lot %s: it has been sold against or is closed; split-lot only supports un-consumed lots", lotID)
	}

	// A per-lot scale is only durable when the security has a recorded split:
	// position heal recomputes a split security's lots/position by skipping it
	// (gated), so the manual scale survives. With no split action the heal would
	// replay the ledger and revert the lot's position. Refuse rather than leave a
	// scale that silently un-does itself on the next app launch.
	hasSplit, err := s.securityHasSplitAction(lot.SecurityID)
	if err != nil {
		return nil, err
	}
	if !hasSplit {
		return nil, fmt.Errorf("cannot split lot %s: security has no recorded split; a per-lot split is only durable alongside one — record it with `investment split` first, or enter the lot already split-adjusted", lotID)
	}

	ratio := alpacadecimal.NewFromFloat(params.Ratio())
	inverseRatio := alpacadecimal.NewFromFloat(1.0 / params.Ratio())
	lot.Shares = lot.Shares.Mul(ratio)
	lot.OriginalShares = lot.OriginalShares.Mul(ratio)
	lot.CostPerShare = lot.CostPerShare.Mul(inverseRatio)

	// The lot scale plus the derived-position recompute commit atomically.
	if err := s.runInTx(func(b *CorporateActionService) error {
		if err := b.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot %s: %w", lotID, err)
		}
		return b.rebuildPositionFromLots(lot.AccountID, lot.SecurityID)
	}); err != nil {
		return nil, err
	}
	return lot, nil
}

// CatchUpSplitsForLot applies every split / reverse-split corporate action for
// the lot's security whose effective date is on or after the lot's purchase
// date, in chronological order, to that single lot. It is the engine behind
// `investment buy --catch-up-splits`: a buy that is back-dated before an
// existing split creates a raw (un-scaled) lot, and this brings it into line
// with the splits that have already run — exactly as if the buy had been
// entered before those splits.
//
// It is a no-op when the lot's security has no split actions on or after the
// purchase date. Lots that have been sold against are rejected by SplitLot.
// Returns the number of splits applied.
func (s *CorporateActionService) CatchUpSplitsForLot(lotID types.ID) (int, error) {
	lot, err := s.lotRepo.GetByID(lotID)
	if err != nil {
		return 0, err
	}

	actions, err := s.caRepo.ListBySecurity(lot.SecurityID)
	if err != nil {
		return 0, fmt.Errorf("failed to list corporate actions: %w", err)
	}

	// Chronological order so successive splits compose correctly.
	sort.SliceStable(actions, func(i, j int) bool {
		return actions[i].ActionDate.Time().Before(actions[j].ActionDate.Time())
	})

	applied := 0
	for _, ca := range actions {
		if ca.ActionType != ActionTypeSplit && ca.ActionType != ActionTypeReverseSplit {
			continue
		}
		// adjustLots scales lots with purchase_date <= split_date, so a split
		// applies to this lot iff its date is on or after the purchase date.
		if ca.ActionDate.Time().Before(lot.PurchaseDate.Time()) {
			continue
		}
		params, err := ParseSplitParams(ca.Parameters)
		if err != nil {
			return applied, fmt.Errorf("failed to parse split params for action %s: %w", ca.ID, err)
		}
		if _, err := s.SplitLot(lotID, *params); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// CatchUpSplitsForTransaction runs CatchUpSplitsForLot on the lot opened by the
// given buy/reinvest transaction. It is the CLI entry point for
// `investment buy --catch-up-splits`, which has the transaction but not the lot
// ID. On a non-lot-tracking account (no lot exists for the transaction) it is a
// silent no-op returning 0 — those accounts already replay splits during heal.
func (s *CorporateActionService) CatchUpSplitsForTransaction(txnID types.ID) (int, error) {
	lot, err := s.lotRepo.GetBySourceTransaction(txnID)
	if err != nil {
		if _, ok := err.(*dberrors.NotFoundError); ok {
			return 0, nil // non-lot account, or no lot — nothing to catch up
		}
		return 0, err
	}
	return s.CatchUpSplitsForLot(lot.ID)
}

// securityHasSplitAction reports whether the security has any split or reverse
// split corporate action on record.
func (s *CorporateActionService) securityHasSplitAction(securityID types.ID) (bool, error) {
	return securityHasAction(s.caRepo, securityID, isSplitAction)
}

// rebuildPositionFromLots recomputes the aggregate (account, security) position
// from the account's open lots. Used by the per-lot repair paths, which mutate
// a lot directly while the normal position heal is gated off for securities
// that participate in a corporate action.
func (s *CorporateActionService) rebuildPositionFromLots(accountID, securityID types.ID) error {
	lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
	if err != nil {
		return err
	}
	pos := NewPosition(accountID, securityID)
	for _, l := range lots {
		if err := pos.AddShares(l.Shares, l.CostPerShare); err != nil {
			return fmt.Errorf("rebuild position for account %s: %w", accountID, err)
		}
	}
	return s.positionRepo.CreateOrUpdate(&pos)
}
