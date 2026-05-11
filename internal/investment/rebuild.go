package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// RebuildResult summarises the outcome of a positions+lots rebuild for one account.
type RebuildResult struct {
	AccountID           types.ID
	AccountName         string
	PositionsRecomputed int
	LotsRecomputed      int
	HasCorporateActions bool // true → rebuild skipped because corporate actions exist
}

// RebuildPositions recomputes investment_positions and the shares/closed
// fields of investment_lots for the given account from transaction history
// and junction records.
//
// The function is a no-op (returns HasCorporateActions=true) when corporate
// actions exist in the database, since splits/mergers/spin-offs mutate
// positions/lots outside the transaction ledger and a naive replay would
// produce incorrect cost bases.
func (s *Service) RebuildPositions(accountID types.ID) (*RebuildResult, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	caRepo := NewCorporateActionRepository(s.db)
	caCount, err := caRepo.CountAll()
	if err != nil {
		return nil, fmt.Errorf("RebuildPositions: %w", err)
	}
	if caCount > 0 {
		return &RebuildResult{
			AccountID:           accountID,
			AccountName:         acct.Name,
			HasCorporateActions: true,
		}, nil
	}

	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("RebuildPositions: %w", err)
	}

	// Group txns by security_id, oldest first.
	bySecurity := make(map[types.ID][]*Transaction)
	for _, t := range txns {
		if !t.SecurityID.Valid {
			continue
		}
		bySecurity[t.SecurityID.ID] = append(bySecurity[t.SecurityID.ID], t)
	}
	for _, list := range bySecurity {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Date.Time().Equal(list[j].Date.Time()) {
				return list[i].CreatedAt.Time().Before(list[j].CreatedAt.Time())
			}
			return list[i].Date.Time().Before(list[j].Date.Time())
		})
	}

	// Recompute positions for non-lot-tracking accounts. Lot-tracking accounts
	// also get aggregate positions (used by some reporting paths) — but those
	// are derived from lots, not transactions; the lot rebuild below handles
	// them. To keep things consistent we replay for both account types.
	result := &RebuildResult{AccountID: accountID, AccountName: acct.Name}
	for secID, list := range bySecurity {
		pos, err := s.replayPosition(accountID, secID, list)
		if err != nil {
			return nil, fmt.Errorf("RebuildPositions: %w", err)
		}
		if err := s.persistRebuiltPosition(accountID, secID, pos); err != nil {
			return nil, fmt.Errorf("RebuildPositions: %w", err)
		}
		result.PositionsRecomputed++
	}

	// Delete any stale position rows that no longer have any transactions.
	if err := s.deleteOrphanPositions(accountID, bySecurity); err != nil {
		return nil, fmt.Errorf("RebuildPositions: %w", err)
	}

	if !acct.TrackLots {
		return result, nil
	}

	// Rebuild lot shares/closed from junctions.
	lots, err := s.lotRepo.ListAllByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("RebuildPositions: %w", err)
	}
	if len(lots) == 0 {
		return result, nil
	}
	lotIDs := make([]types.ID, 0, len(lots))
	for _, l := range lots {
		lotIDs = append(lotIDs, l.ID)
	}
	consumed, err := s.transactionLotRepo.SumSharesByLot(lotIDs)
	if err != nil {
		return nil, fmt.Errorf("RebuildPositions: %w", err)
	}
	for _, lot := range lots {
		used := consumed[lot.ID]
		newShares := lot.OriginalShares.Sub(used)
		if newShares.IsNegative() {
			return nil, fmt.Errorf("RebuildPositions: lot %s has more consumed shares (%s) than original (%s)",
				lot.ID, used.String(), lot.OriginalShares.String())
		}
		closed := newShares.IsZero()
		if lot.Shares.Cmp(newShares) == 0 && lot.Closed == closed {
			continue
		}
		if err := s.lotRepo.UpdateSharesAndClosed(lot.ID, newShares, closed); err != nil {
			return nil, fmt.Errorf("RebuildPositions: %w", err)
		}
		result.LotsRecomputed++
	}
	return result, nil
}

// HealAllAccounts runs RebuildPositions for every investment account in the
// database. It is intended to be invoked once when the app opens a database
// so that desynced positions/lots — caused by older binaries or aborted
// edits — are silently corrected before the user sees them. The function
// is a no-op on databases that contain corporate-action records.
//
// Errors from individual accounts are not fatal: HealAllAccounts logs nothing
// and continues so a single bad account can't keep the app from launching.
// The total count of accounts healed is returned for telemetry/testing.
func (s *Service) HealAllAccounts() (int, error) {
	accounts, err := s.accountRepo.List(false)
	if err != nil {
		return 0, fmt.Errorf("HealAllAccounts: %w", err)
	}
	healed := 0
	for _, acct := range accounts {
		if acct.Type != account.TypeInvestment {
			continue
		}
		res, err := s.RebuildPositions(acct.ID)
		if err != nil {
			// Skip the account; don't break startup.
			continue
		}
		if res.HasCorporateActions {
			continue
		}
		healed++
	}
	return healed, nil
}

// syncPositionAndLots recomputes the position for (accountID, securityID)
// and any lots in the account whose shares disagree with their junction
// totals. The function is a silent no-op when the database contains any
// corporate-action records (splits/mergers/spin-offs mutate positions and
// lots outside the ledger, so a naive replay would corrupt cost basis).
//
// Called at the top of share-bearing service operations so that desynced
// state (e.g. from an older binary or an aborted edit) auto-heals on the
// next user action.
func (s *Service) syncPositionAndLots(accountID, securityID types.ID) error {
	caRepo := NewCorporateActionRepository(s.db)
	caCount, err := caRepo.CountAll()
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	if caCount > 0 {
		return nil
	}

	// Recompute the aggregate position for this (account, security).
	secFilter := securityID
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{SecurityID: &secFilter})
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	sort.SliceStable(txns, func(i, j int) bool {
		if txns[i].Date.Time().Equal(txns[j].Date.Time()) {
			return txns[i].CreatedAt.Time().Before(txns[j].CreatedAt.Time())
		}
		return txns[i].Date.Time().Before(txns[j].Date.Time())
	})
	pos, err := s.replayPosition(accountID, securityID, txns)
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	if err := s.persistRebuiltPosition(accountID, securityID, pos); err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}

	// For lot-tracking accounts, also bring this security's lots in line
	// with the junction records.
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	if !acct.TrackLots {
		return nil
	}
	lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, true)
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	if len(lots) == 0 {
		return nil
	}
	lotIDs := make([]types.ID, 0, len(lots))
	for _, l := range lots {
		lotIDs = append(lotIDs, l.ID)
	}
	consumed, err := s.transactionLotRepo.SumSharesByLot(lotIDs)
	if err != nil {
		return fmt.Errorf("syncPositionAndLots: %w", err)
	}
	for _, lot := range lots {
		newShares := lot.OriginalShares.Sub(consumed[lot.ID])
		if newShares.IsNegative() {
			return fmt.Errorf("syncPositionAndLots: lot %s has more consumed shares than original", lot.ID)
		}
		closed := newShares.IsZero()
		if lot.Shares.Cmp(newShares) == 0 && lot.Closed == closed {
			continue
		}
		if err := s.lotRepo.UpdateSharesAndClosed(lot.ID, newShares, closed); err != nil {
			return fmt.Errorf("syncPositionAndLots: %w", err)
		}
	}
	return nil
}

// replayPosition reconstructs an aggregate (account, security) position from
// transactions, ordered oldest-first. Used by RebuildPositions.
func (s *Service) replayPosition(accountID, securityID types.ID, txns []*Transaction) (*Position, error) {
	pos := NewPosition(accountID, securityID)
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
				return nil, fmt.Errorf("replayPosition Buy/Reinvest %s: %w", t.ID, err)
			}
		case TransactionTypeSell, TransactionTypeFeeLiquidation:
			if pos.Shares.Cmp(t.Shares.Quantity) < 0 {
				return nil, fmt.Errorf("replayPosition Sell/FeeLiquidation %s: have %s shares, sold %s",
					t.ID, pos.Shares.String(), t.Shares.Quantity.String())
			}
			if err := pos.RemoveShares(t.Shares.Quantity); err != nil {
				return nil, fmt.Errorf("replayPosition Sell/FeeLiquidation %s: %w", t.ID, err)
			}
		case TransactionTypeTransferShares:
			if t.TotalAmount.IsNegative() {
				// outgoing transfer (shares leaving)
				if pos.Shares.Cmp(t.Shares.Quantity) < 0 {
					return nil, fmt.Errorf("replayPosition TransferShares %s: have %s shares, sent %s",
						t.ID, pos.Shares.String(), t.Shares.Quantity.String())
				}
				if err := pos.RemoveShares(t.Shares.Quantity); err != nil {
					return nil, fmt.Errorf("replayPosition TransferShares %s: %w", t.ID, err)
				}
			} else {
				price := types.ZeroMoney
				if t.PricePerShare.Valid {
					price = t.PricePerShare.Money
				} else if !t.Shares.Quantity.IsZero() {
					price = t.TotalAmount.Mul(alpacadecimal.NewFromInt(1).Div(t.Shares.Quantity.Decimal()))
				}
				if err := pos.AddShares(t.Shares.Quantity, price); err != nil {
					return nil, fmt.Errorf("replayPosition TransferShares-in %s: %w", t.ID, err)
				}
			}
		default:
			// Dividend/Fee/Deposit/Withdrawal/Interest/TransferCash: no position effect.
		}
	}
	return &pos, nil
}

// persistRebuiltPosition writes the recomputed position to the position
// repository, deleting any existing zero-share entry.
func (s *Service) persistRebuiltPosition(accountID, securityID types.ID, pos *Position) error {
	if pos.Shares.IsZero() {
		if err := s.positionRepo.Delete(accountID, securityID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return err
			}
		}
		return nil
	}
	return s.positionRepo.CreateOrUpdate(pos)
}

// deleteOrphanPositions removes stored positions that no longer have any
// matching transactions. This cleans up the corruption case where positions
// got desynced by an aborted edit.
func (s *Service) deleteOrphanPositions(accountID types.ID, hasTxns map[types.ID][]*Transaction) error {
	existing, err := s.positionRepo.ListByAccount(accountID, false)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if _, ok := hasTxns[p.SecurityID]; ok {
			continue
		}
		if err := s.positionRepo.Delete(accountID, p.SecurityID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return err
			}
		}
	}
	return nil
}
