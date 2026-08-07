package investment

import (
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/types"
)

// Read-only share and cash queries. No writes, no transactions.

// GetCashBalance computes the cash balance for an investment account by summing
// all cash-affecting transactions.
func (s *Service) GetCashBalance(accountID types.ID) (types.Money, error) {
	return cashBalanceOf(s.repo, accountID)
}

// cashBalanceOf is the shared body: ValuationService needs the same figure to
// value an account, and neither type should own the other.
func cashBalanceOf(repo *Repository, accountID types.ID) (types.Money, error) {
	txns, err := repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions: %w", err)
	}

	balance := types.ZeroMoney
	for _, txn := range txns {
		if txn.Type.AffectsCash() {
			balance = balance.Add(txn.TotalAmount)
		}
	}

	return balance, nil
}

// TotalSharesForSecurity sums the current shares held for a security across all
// investment/HSA accounts — open lots for lot-tracked accounts, the aggregate
// position otherwise. Used to derive a spin-off's share ratio from a statement's
// resulting-share count (ratio = resulting_shares / total_shares).
func (s *Service) TotalSharesForSecurity(securityID types.ID) (types.Quantity, error) {
	accounts, err := s.accountRepo.List(false)
	if err != nil {
		return types.ZeroQuantity, fmt.Errorf("TotalSharesForSecurity: %w", err)
	}
	total := types.ZeroQuantity
	for _, acct := range accounts {
		if !acct.Type.IsInvestmentType() {
			continue
		}
		if acct.TrackLots {
			lots, err := s.lotRepo.ListByAccountAndSecurity(acct.ID, securityID, false)
			if err != nil {
				return types.ZeroQuantity, fmt.Errorf("TotalSharesForSecurity: %w", err)
			}
			for _, l := range lots {
				total = total.Add(l.Shares)
			}
			continue
		}
		pos, err := s.positionRepo.GetByAccountAndSecurity(acct.ID, securityID)
		if err != nil {
			continue
		}
		total = total.Add(pos.Shares)
	}
	return total, nil
}

// AccountShares is a per-account share total for a security, used by
// preview surfaces (e.g. the stock-split dialog) to show what a holding
// would look like before and after an action is applied.
type AccountShares struct {
	AccountID   types.ID
	AccountName string
	Shares      types.Quantity
}

// SharesBySecurity returns the share total each account currently holds
// for the given security. Lot-tracking accounts contribute the sum of
// their open lot shares; non-lot-tracking accounts contribute their
// stored position shares. Accounts with zero shares are omitted. Results
// are sorted by account name.
func (s *Service) SharesBySecurity(securityID types.ID) ([]AccountShares, error) {
	totals := make(map[types.ID]types.Quantity)

	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load lots: %w", err)
	}
	lotAccounts := make(map[types.ID]bool)
	for _, lot := range lots {
		totals[lot.AccountID] = totals[lot.AccountID].Add(lot.Shares)
		lotAccounts[lot.AccountID] = true
	}

	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load positions: %w", err)
	}
	for _, pos := range positions {
		// A lot-tracking account also carries an aggregate position row (a
		// derived cache); its shares already came from the lots above, so adding
		// the row too would double-count. Use the position only for non-lot
		// accounts.
		if lotAccounts[pos.AccountID] {
			continue
		}
		totals[pos.AccountID] = totals[pos.AccountID].Add(pos.Shares)
	}

	results := make([]AccountShares, 0, len(totals))
	for acctID, shares := range totals {
		if shares.IsZero() {
			continue
		}
		acct, err := s.accountRepo.GetByID(acctID)
		if err != nil {
			return nil, fmt.Errorf("failed to load account %s: %w", acctID.String(), err)
		}
		results = append(results, AccountShares{
			AccountID:   acctID,
			AccountName: acct.Name,
			Shares:      shares,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].AccountName < results[j].AccountName
	})
	return results, nil
}

// SharesBySecurityAsOf returns, per account, the shares of a security that were
// held on or before asOf — i.e. the shares a split dated asOf would actually
// adjust. Lot-tracking accounts contribute open lots purchased on/before asOf;
// non-lot accounts contribute their net ledger position as of asOf. Accounts
// holding nothing as of that date are omitted. Used by the stock-split preview
// so it reflects the date-scoped engine rather than naively scaling everything.
func (s *Service) SharesBySecurityAsOf(securityID types.ID, asOf types.Date) ([]AccountShares, error) {
	totals := make(map[types.ID]types.Quantity)
	lotAccounts := make(map[types.ID]bool)

	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load lots: %w", err)
	}
	for _, lot := range lots {
		lotAccounts[lot.AccountID] = true
		if lot.PurchaseDate.Time().After(asOf.Time()) {
			continue // acquired after the date — not affected by a split then
		}
		totals[lot.AccountID] = totals[lot.AccountID].Add(lot.Shares)
	}

	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load positions: %w", err)
	}
	for _, pos := range positions {
		if lotAccounts[pos.AccountID] {
			continue // lot-tracking account, handled via lots above
		}
		held, err := netSharesHeldAsOf(s.repo, pos.AccountID, securityID, asOf)
		if err != nil {
			return nil, err
		}
		if !held.IsZero() && !held.IsNegative() {
			totals[pos.AccountID] = held
		}
	}

	results := make([]AccountShares, 0, len(totals))
	for acctID, shares := range totals {
		if shares.IsZero() {
			continue
		}
		acct, err := s.accountRepo.GetByID(acctID)
		if err != nil {
			return nil, fmt.Errorf("failed to load account %s: %w", acctID.String(), err)
		}
		results = append(results, AccountShares{AccountID: acctID, AccountName: acct.Name, Shares: shares})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].AccountName < results[j].AccountName
	})
	return results, nil
}

// netSharesHeldAsOf replays an account's ledger for a security and returns the
// net share count held on or before asOf.
func netSharesHeldAsOf(repo *Repository, accountID, securityID types.ID, asOf types.Date) (types.Quantity, error) {
	filter := TransactionFilter{SecurityID: &securityID, ToDate: &asOf}
	txns, err := repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroQuantity, err
	}
	net := types.ZeroQuantity
	for _, t := range txns {
		if !t.Shares.Valid {
			continue
		}
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeReinvestDividend:
			net = net.Add(t.Shares.Quantity)
		case TransactionTypeSell, TransactionTypeFeeLiquidation:
			net = net.Sub(t.Shares.Quantity)
		case TransactionTypeTransferShares:
			if t.TotalAmount.IsNegative() {
				net = net.Sub(t.Shares.Quantity)
			} else {
				net = net.Add(t.Shares.Quantity)
			}
		}
	}
	return net, nil
}
