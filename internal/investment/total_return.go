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
