package investment

import (
	"fmt"

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
