package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// sumDividendsForSecurity returns the total cash dividends received for
// the given (account, security) pair. Only `dividend` transactions are
// summed; `reinvest_dividend` transactions are excluded — the spec treats
// them as new share acquisitions with their own cost basis.
func (s *Service) sumDividendsForSecurity(accountID, securityID types.ID) (types.Money, error) {
	divType := TransactionTypeDividend
	filter := TransactionFilter{
		Type:       &divType,
		SecurityID: &securityID,
	}

	txns, err := s.repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list dividend transactions: %w", err)
	}

	total := types.ZeroMoney
	for _, txn := range txns {
		total = total.Add(txn.TotalAmount)
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
// per share immediately before each sell and accumulating
// (sell.price_per_share − avg_cost_at_sell) × sell.shares.
//
// Per the spec, fee_liquidation is not treated as a realized event (the
// whole transaction is a fee paid in shares); transfer_shares carries cost
// basis with the shares and is likewise not realized. Both still adjust
// the running share count so subsequent sells observe the correct basis.
//
// txns must be sorted by date ascending, then created_at ascending — the
// same canonical order replayPosition expects. The TR-008 service wrapper
// loads and sorts the slice before delegating here.
func (s *Service) replayRealizedGain(accountID, securityID types.ID, txns []*Transaction) (types.Money, error) {
	pos := NewPosition(accountID, securityID)
	total := types.ZeroMoney
	for _, t := range txns {
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
	return total, nil
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
	return s.replayRealizedGain(accountID, securityID, txns)
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
