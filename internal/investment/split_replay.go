package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// splitEvent is a split (or reverse split) applied to a security on a date.
// Ratio is new-shares-per-old-share: 2 for a 2:1 forward split, 0.5 for a 1:2
// reverse split.
type splitEvent struct {
	Date  types.Date
	Ratio alpacadecimal.Decimal
}

// splitEventsForSecurity returns the security's split / reverse-split events
// sorted oldest-first. Mergers and spin-offs are not splits and are excluded
// (they transform shares across securities or reallocate cost basis, which a
// per-security chronological replay cannot reconstruct).
func (s *Service) splitEventsForSecurity(securityID types.ID) ([]splitEvent, error) {
	// Bindable repo, not a fresh s.db one: syncPositionAndLots (which calls this)
	// runs inside a tx during heal-before-trade, where a pool read would deadlock.
	actions, err := s.corporateActionRepo.ListBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("splitEventsForSecurity: %w", err)
	}
	var events []splitEvent
	for _, ca := range actions {
		if ca.ActionType != ActionTypeSplit && ca.ActionType != ActionTypeReverseSplit {
			continue
		}
		params, err := ParseSplitParams(ca.Parameters)
		if err != nil {
			return nil, fmt.Errorf("splitEventsForSecurity: %w", err)
		}
		events = append(events, splitEvent{
			Date:  ca.ActionDate,
			Ratio: alpacadecimal.NewFromFloat(params.Ratio()),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Date.Time().Before(events[j].Date.Time())
	})
	return events, nil
}

// securityHasNonSplitAction reports whether the security participates (as the
// affected security, a merger target, or a spin-off parent/child) in any
// corporate action other than a split or reverse split. Such actions cannot be
// reconstructed by a per-security chronological replay, so callers must keep
// gating them.
func (s *Service) securityHasNonSplitAction(securityID types.ID) (bool, error) {
	// Bindable repo, not a fresh s.db one: syncPositionAndLots (which calls this)
	// runs inside a tx during heal-before-trade, where a pool read would deadlock.
	actions, err := s.corporateActionRepo.ListBySecurity(securityID)
	if err != nil {
		return false, fmt.Errorf("securityHasNonSplitAction: %w", err)
	}
	for _, ca := range actions {
		if ca.ActionType != ActionTypeSplit && ca.ActionType != ActionTypeReverseSplit {
			return true, nil
		}
	}
	return false, nil
}

// applySplitToPosition scales a running position by a split ratio: shares ×
// ratio, average cost ÷ ratio. Total cost basis is unchanged.
func applySplitToPosition(pos *Position, ratio alpacadecimal.Decimal) {
	pos.Shares = pos.Shares.Mul(ratio)
	inverse := alpacadecimal.NewFromInt(1).Div(ratio)
	pos.AverageCostPerShare = pos.AverageCostPerShare.Mul(inverse)
}

// applyDueSplits applies every split whose date falls strictly before the next
// transaction's date, advancing the split index. Transactions dated on the
// split date itself are treated as pre-split (processed before the split is
// applied), matching the engine's adjustLots rule (purchase_date <= splitDate
// is adjusted). Returns the advanced index.
func applyDueSplits(pos *Position, splits []splitEvent, si int, nextTxnDate types.Date) int {
	for si < len(splits) && splits[si].Date.Time().Before(nextTxnDate.Time()) {
		applySplitToPosition(pos, splits[si].Ratio)
		si++
	}
	return si
}
