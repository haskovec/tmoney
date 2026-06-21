package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// sumDividendsForSecurity returns the total dividends received for the given
// (account, security) pair. Both cash `dividend` and `reinvest_dividend`
// transactions count: a reinvested dividend is income the fund paid you (you
// chose to plow it back into shares), so it belongs in total return. The shares
// it bought carry their own cost basis, so those shares' appreciation is
// captured separately in unrealized gain; counting the dividend here adds only
// its principal once — with no double-count, since a later sale realizes gain
// against that same basis (proceeds = realized + the dividend already booked).
// Reinvested dividends are NOT counted as capital deployed (see
// totalCostDeployedForSecurity), so the total-return percent measures earnings
// against your own contributions.
func (s *Service) sumDividendsForSecurity(accountID, securityID types.ID) (types.Money, error) {
	filter := TransactionFilter{
		SecurityID: &securityID,
	}

	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list dividend transactions: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		switch txn.Type {
		case TransactionTypeDividend, TransactionTypeReinvestDividend:
			total = total.Add(txn.TotalAmount)
		}
	}
	return total, nil
}

// sumInterestForAccount returns the total interest received on the cash
// sweep of an investment account. Interest is not tied to a specific
// security, so there is no security filter.
func (s *Service) sumInterestForAccount(accountID types.ID) (types.Money, error) {
	intType := TransactionTypeInterest
	filter := TransactionFilter{
		Type: &intType,
	}

	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list interest transactions: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		total = total.Add(txn.TotalAmount)
	}
	return total, nil
}

// sumFeesForSecurity returns the total fees paid for the given
// (account, security) pair, as a positive magnitude. Fees include
// commissions on buy, sell, and reinvest_dividend transactions, plus
// the full total_amount of any fee_liquidation transactions (the
// whole transaction is the fee paid in shares). Account-level `fee`
// transactions (no security_id) are summed separately by
// sumFeesForAccount.
func (s *Service) sumFeesForSecurity(accountID, securityID types.ID) (types.Money, error) {
	filter := TransactionFilter{
		SecurityID: &securityID,
	}

	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for fees: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		switch txn.Type {
		case TransactionTypeBuy, TransactionTypeSell, TransactionTypeReinvestDividend:
			if txn.Commission.Valid {
				total = total.Add(txn.Commission.Money)
			}
		case TransactionTypeFeeLiquidation:
			total = total.Add(txn.TotalAmount)
		}
	}
	return total, nil
}

// realizedGainLotTracked returns the realized gain for a (account, security)
// pair in a lot-tracking account. It walks the `transaction_lots` junction
// table for every `sell` and `fee_liquidation` transaction on the security
// and sums (txn.price_per_share − lot.cost_per_share) × junction.shares.
// `txn.price_per_share` is already net of commission per ComputePricePerShare;
// commission is counted separately as a fee, not subtracted twice here.
func (s *Service) realizedGainLotTracked(accountID, securityID types.ID) (types.Money, error) {
	filter := TransactionFilter{
		SecurityID: &securityID,
	}
	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for realized gain: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		if txn.Type != TransactionTypeSell && txn.Type != TransactionTypeFeeLiquidation {
			continue
		}
		if !txn.PricePerShare.Valid {
			continue
		}
		junctions, err := s.transactionLotRepo.GetByTransaction(txn.ID)
		if err != nil {
			return types.ZeroMoney, fmt.Errorf("failed to get transaction lot junctions: %w", err)
		}
		for _, j := range junctions {
			lot, err := s.lotRepo.GetByID(j.LotID)
			if err != nil {
				return types.ZeroMoney, fmt.Errorf("failed to get lot %s: %w", j.LotID, err)
			}
			perShareGain := txn.PricePerShare.Money.Sub(lot.CostPerShare)
			total = total.Add(perShareGain.Mul(j.Shares.Decimal()))
		}
	}
	return total, nil
}

// replayRealizedGain reconstructs the realized gain for a non-lot-tracking
// (account, security) pair by replaying its transaction ledger. It mirrors
// replayPosition's chronological walk, capturing the running average cost
// per share immediately before each disposition and accumulating
// (price_per_share − avg_cost_at_disposition) × shares.
//
// Both sell and fee_liquidation are dispositions: shares leave the position
// at their market price, so both realize gain against the running average
// cost — matching realizedGainLotTracked, which walks both types. (A
// fee_liquidation's dollar amount is also booked as a fee by
// sumFeesForSecurity; realized gain and the fee are independent total-return
// terms, so there is no double-count.) transfer_shares is NOT a disposition —
// it carries cost basis with the shares — so it adjusts only the running
// share count, never realized gain.
//
// txns must be sorted by date ascending, then created_at ascending — the
// same canonical order replayPosition expects. The TR-008 service wrapper
// loads and sorts the slice before delegating here.
func (s *Service) replayRealizedGain(accountID, securityID types.ID, txns []*Transaction, splits []splitEvent) (types.Money, error) {
	pos := NewPosition(accountID, securityID)
	total := types.ZeroMoney
	si := 0
	for _, t := range txns {
		// A split before this transaction rescales the running basis (shares ×
		// ratio, avg cost ÷ ratio), so a later sell measures gain against the
		// post-split per-share cost.
		si = applyDueSplits(&pos, splits, si, t.Date)
		if !t.Shares.Valid || t.Shares.Quantity.IsZero() {
			continue
		}
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeReinvestDividend:
			price := types.ZeroMoney
			if t.PricePerShare.Valid {
				price = t.PricePerShare.Money
			}
			if err := pos.AddShares(t.Shares.Quantity, price); err != nil {
				return types.ZeroMoney, fmt.Errorf("replayRealizedGain Buy/Reinvest %s: %w", t.ID, err)
			}
		case TransactionTypeSell:
			avgCost := pos.AverageCostPerShare
			price := types.ZeroMoney
			if t.PricePerShare.Valid {
				price = t.PricePerShare.Money
			}
			perShareGain := price.Sub(avgCost)
			total = total.Add(perShareGain.Mul(t.Shares.Quantity.Decimal()))
			if pos.Shares.Cmp(t.Shares.Quantity) < 0 {
				return types.ZeroMoney, fmt.Errorf("replayRealizedGain Sell %s: have %s shares, sold %s",
					t.ID, pos.Shares.String(), t.Shares.Quantity.String())
			}
			if err := pos.RemoveShares(t.Shares.Quantity); err != nil {
				return types.ZeroMoney, fmt.Errorf("replayRealizedGain Sell %s: %w", t.ID, err)
			}
		case TransactionTypeFeeLiquidation:
			// A fee_liquidation disposes of shares at their market price to pay
			// a fee, so it realizes gain against the running average cost
			// exactly like a sell (and like realizedGainLotTracked, which walks
			// both types). The dollar amount is booked separately as a fee by
			// sumFeesForSecurity, so the two are independent terms — no
			// double-count.
			avgCost := pos.AverageCostPerShare
			price := types.ZeroMoney
			if t.PricePerShare.Valid {
				price = t.PricePerShare.Money
			}
			perShareGain := price.Sub(avgCost)
			total = total.Add(perShareGain.Mul(t.Shares.Quantity.Decimal()))
			if pos.Shares.Cmp(t.Shares.Quantity) < 0 {
				return types.ZeroMoney, fmt.Errorf("replayRealizedGain FeeLiquidation %s: have %s shares, fee %s",
					t.ID, pos.Shares.String(), t.Shares.Quantity.String())
			}
			if err := pos.RemoveShares(t.Shares.Quantity); err != nil {
				return types.ZeroMoney, fmt.Errorf("replayRealizedGain FeeLiquidation %s: %w", t.ID, err)
			}
		case TransactionTypeTransferShares:
			if t.TotalAmount.IsNegative() {
				if pos.Shares.Cmp(t.Shares.Quantity) < 0 {
					return types.ZeroMoney, fmt.Errorf("replayRealizedGain TransferShares %s: have %s shares, sent %s",
						t.ID, pos.Shares.String(), t.Shares.Quantity.String())
				}
				if err := pos.RemoveShares(t.Shares.Quantity); err != nil {
					return types.ZeroMoney, fmt.Errorf("replayRealizedGain TransferShares %s: %w", t.ID, err)
				}
			} else {
				price := types.ZeroMoney
				if t.PricePerShare.Valid {
					price = t.PricePerShare.Money
				} else if !t.Shares.Quantity.IsZero() {
					price = t.TotalAmount.Mul(alpacadecimal.NewFromInt(1).Div(t.Shares.Quantity.Decimal()))
				}
				if err := pos.AddShares(t.Shares.Quantity, price); err != nil {
					return types.ZeroMoney, fmt.Errorf("replayRealizedGain TransferShares-in %s: %w", t.ID, err)
				}
			}
		}
	}
	// Splits dated after the last transaction rescale the (now closed-out or
	// remaining) basis; they add no realized gain themselves.
	for ; si < len(splits); si++ {
		applySplitToPosition(&pos, splits[si].Ratio)
	}
	return total, nil
}

// realizedGain is the dispatcher entry point that valuation code calls
// to obtain realized gain for an (account, security) pair. The bool
// return ("unavailable") is true when a real number cannot be produced
// — currently only the non-lot path when *this* security has any
// corporate action on file, since the ledger reflects post-action share
// counts that the chronological replay is unaware of. A corporate
// action on an unrelated security does not affect this security's
// replay, so the check is scoped to ListBySecurity rather than CountAll.
// Callers surface this as Holding.RealizedGainUnavailable.
//
// The lot-tracked path is robust to corporate actions because the
// corporate-action service mutates lots in place and transaction_lots
// junction rows reference post-action lots, so junction-based math
// remains correct.
func (s *Service) realizedGain(accountID, securityID types.ID, trackLots bool) (types.Money, bool, error) {
	if trackLots {
		gain, err := s.realizedGainLotTracked(accountID, securityID)
		return gain, false, err
	}
	// Splits are a dated ratio transform the non-lot replay can reconstruct;
	// mergers and spin-offs (cross-security, cost-basis reallocation) still
	// cannot, so they remain "unavailable".
	hasNonSplit, err := s.securityHasNonSplitAction(securityID)
	if err != nil {
		return types.ZeroMoney, false, fmt.Errorf("failed to inspect corporate actions for security: %w", err)
	}
	if hasNonSplit {
		return types.ZeroMoney, true, nil
	}
	gain, err := s.realizedGainNonLot(accountID, securityID)
	return gain, false, err
}

// realizedGainNonLot is the service entry point for realized gain on a
// non-lot-tracking (account, security) pair. It loads every transaction
// for the pair, sorts them by (date asc, created_at asc) — the canonical
// order replayPosition expects — and delegates to replayRealizedGain.
func (s *Service) realizedGainNonLot(accountID, securityID types.ID) (types.Money, error) {
	secFilter := securityID
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{SecurityID: &secFilter})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for realized gain: %w", err)
	}
	sort.SliceStable(txns, func(i, j int) bool {
		if txns[i].Date.Time().Equal(txns[j].Date.Time()) {
			return txns[i].CreatedAt.Time().Before(txns[j].CreatedAt.Time())
		}
		return txns[i].Date.Time().Before(txns[j].Date.Time())
	})
	splits, err := s.splitEventsForSecurity(securityID)
	if err != nil {
		return types.ZeroMoney, err
	}
	return s.replayRealizedGain(accountID, securityID, txns, splits)
}

// totalCostDeployedForSecurity returns the capital YOU put into a
// (account, security) pair — the denominator for total-return percent.
// Only `buy` transactions count: that is your own money. Shares received via
// `transfer_shares` carry their basis with them (not new capital here), and
// `reinvest_dividend` is income the fund paid you (counted in the numerator as
// a dividend), so neither contributes to deployed capital. The result is a
// positive magnitude; buy transactions store `total_amount` as a negative cash
// debit, so the magnitude is taken via Abs(). A position built without any buy
// (e.g. transfer-in only, or reinvested-dividends only) returns zero, which the
// caller renders as "—" for the percent.
func (s *Service) totalCostDeployedForSecurity(accountID, securityID types.ID) (types.Money, error) {
	filter := TransactionFilter{
		SecurityID: &securityID,
	}
	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for total cost deployed: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		switch txn.Type {
		case TransactionTypeBuy:
			total = total.Add(txn.TotalAmount.Abs())
		}
	}
	return total, nil
}

// totalCostDeployedForAccount returns the capital YOU put into the account
// across every security — the account-level denominator for total-return
// percent. Only `buy` transactions count (your own money); `transfer_shares`
// carry their basis with them and `reinvest_dividend` is income (counted in the
// numerator as a dividend), so both are excluded. The result is a positive
// magnitude.
func (s *Service) totalCostDeployedForAccount(accountID types.ID) (types.Money, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for account total cost deployed: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		switch txn.Type {
		case TransactionTypeBuy:
			total = total.Add(txn.TotalAmount.Abs())
		}
	}
	return total, nil
}

// listEverHeldSecurities returns the distinct security IDs that the account
// has ever held shares of — both currently-open positions and fully-sold
// (closed) positions. It scans the share-bearing transaction types — `buy`,
// `sell`, `reinvest_dividend`, `fee_liquidation`, and `transfer_shares` —
// so a security received only via a transfer-in is included and a security
// that has been fully sold is still surfaced. Non-share-bearing types
// (`dividend`, `interest`, `fee` at account level, `deposit`, `withdrawal`,
// `transfer_cash`) do not contribute, which prevents a stray dividend on a
// never-held ticker from leaking into the closed-position list.
//
// The returned slice is in stable order (security ID ascending as a string)
// so callers can iterate it deterministically.
func (s *Service) listEverHeldSecurities(accountID types.ID) ([]types.ID, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions for ever-held securities: %w", err)
	}

	seen := make(map[types.ID]struct{})
	for _, t := range txns {
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeSell,
			TransactionTypeReinvestDividend,
			TransactionTypeFeeLiquidation,
			TransactionTypeTransferShares:
		default:
			continue
		}
		if !t.SecurityID.Valid {
			continue
		}
		seen[t.SecurityID.ID] = struct{}{}
	}

	ids := make([]types.ID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids, nil
}

// sumFeesForAccount returns the total fees paid across every security in
// the account plus any account-level `fee` transactions (which carry no
// security_id). The result is a positive magnitude — the spec's
// fees_paid[account] from the total-return formula.
func (s *Service) sumFeesForAccount(accountID types.ID) (types.Money, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions for account fees: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		switch txn.Type {
		case TransactionTypeBuy, TransactionTypeSell, TransactionTypeReinvestDividend:
			if txn.Commission.Valid {
				total = total.Add(txn.Commission.Money)
			}
		case TransactionTypeFeeLiquidation:
			total = total.Add(txn.TotalAmount)
		case TransactionTypeFee:
			// `Fee` transactions store `total_amount` as a negative cash
			// debit; take the magnitude so it adds to the fee total.
			total = total.Add(txn.TotalAmount.Abs())
		}
	}
	return total, nil
}
