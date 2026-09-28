package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// LedgerState reports what an investment account still holds: its cash, and
// whether it still holds shares. It implements account.InvestmentLedger, which
// the close rule uses.
//
// cash comes from cashBalanceOf, the same figure GetAccountValuation reports
// as CashBalance. Holdings follow the rule of the portfolio_holdings view that
// valuation reads: open lots for a lot-tracked account, positions otherwise.
// A lot-tracked account's aggregate position is not kept current by lot sells,
// so reading it there would refuse to close a sold-out account. Prices play no
// part: shares with no price, or a zero price, are still held.
func (s *ValuationService) LedgerState(accountID types.ID) (types.Money, bool, error) {
	acct, err := loadInvestmentAccount(s.accountRepo, accountID)
	if err != nil {
		return types.ZeroMoney, false, err
	}
	cash, err := cashBalanceOf(s.repo, acct)
	if err != nil {
		return types.ZeroMoney, false, fmt.Errorf("failed to get cash balance: %w", err)
	}

	if acct.TrackLots {
		lots, err := s.lotRepo.ListAllByAccount(accountID)
		if err != nil {
			return types.ZeroMoney, false, fmt.Errorf("failed to list lots: %w", err)
		}
		for _, lot := range lots {
			if !lot.Closed && !lot.Shares.IsZero() {
				return cash, true, nil
			}
		}
		return cash, false, nil
	}

	// Not excludeZeroShares: that filter keeps shares > 0, and a negative
	// count is not an empty account either.
	positions, err := s.positionRepo.ListByAccount(accountID, false)
	if err != nil {
		return types.ZeroMoney, false, fmt.Errorf("failed to list positions: %w", err)
	}
	for _, pos := range positions {
		if !pos.Shares.IsZero() {
			return cash, true, nil
		}
	}
	return cash, false, nil
}
