package investment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// LotMethod selects which open lots a historical disposition consumes when lots
// are synthesized from transaction history during a backfill.
type LotMethod int

const (
	// LotMethodFIFO consumes the oldest open lots first (the IRS default for
	// unspecified lots).
	LotMethodFIFO LotMethod = iota
	// LotMethodLIFO consumes the newest open lots first.
	LotMethodLIFO
	// LotMethodHIFO consumes the highest cost-per-share open lots first.
	LotMethodHIFO
)

// ParseLotMethod parses a lot-selection method name. An empty string defaults
// to FIFO.
func ParseLotMethod(s string) (LotMethod, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "fifo":
		return LotMethodFIFO, nil
	case "lifo":
		return LotMethodLIFO, nil
	case "hifo":
		return LotMethodHIFO, nil
	default:
		return LotMethodFIFO, fmt.Errorf("unknown lot method %q (want fifo, lifo, or hifo)", s)
	}
}

// String returns the lowercase method name.
func (m LotMethod) String() string {
	switch m {
	case LotMethodLIFO:
		return "lifo"
	case LotMethodHIFO:
		return "hifo"
	default:
		return "fifo"
	}
}

// BackfillShortfall records a disposition (sell, fee-liquidation, or outbound
// share transfer) whose shares could not be fully covered by open lots during
// the replay — e.g. out-of-order or imported data where a sale precedes its buy.
type BackfillShortfall struct {
	SecurityID    types.ID
	TransactionID types.ID
	Date          types.Date
	Requested     types.Quantity
	Covered       types.Quantity
}

// BackfillPlan is the set of lots and junction records a lot backfill would
// create for an account, plus any shortfalls. It is computed without writing
// anything; pass it to ApplyLotBackfill to persist.
type BackfillPlan struct {
	AccountID  types.ID
	Method     LotMethod
	Lots       []*Lot
	Junctions  []TransactionLot
	Shortfalls []BackfillShortfall
}

// LotsPerSecurity returns the number of lots the plan creates per security.
func (p *BackfillPlan) LotsPerSecurity() map[types.ID]int {
	out := make(map[types.ID]int, len(p.Lots))
	for _, l := range p.Lots {
		out[l.SecurityID]++
	}
	return out
}

// PlanLotBackfill replays an account's full transaction ledger and computes the
// lots and junction records needed to reconstruct lot-level cost basis, without
// writing anything. Buys, reinvested dividends, and inbound share transfers open
// lots; sells, fee liquidations, and outbound share transfers consume open lots
// by the chosen method. Cash-only transaction types have no lot effect.
//
// PlanLotBackfill does NOT apply corporate actions: callers must ensure the
// account holds no security with a recorded corporate action before relying on
// the result (a naive replay cannot reconstruct lots across a split/merger/
// spin-off). See the per-account gate in enable-lots.
func (s *Service) PlanLotBackfill(accountID types.ID, method LotMethod) (*BackfillPlan, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("PlanLotBackfill: %w", err)
	}

	bySecurity := make(map[types.ID][]*Transaction)
	for _, t := range txns {
		if !t.SecurityID.Valid {
			continue
		}
		bySecurity[t.SecurityID.ID] = append(bySecurity[t.SecurityID.ID], t)
	}

	// Deterministic security order for stable plan output.
	secIDs := make([]types.ID, 0, len(bySecurity))
	for id := range bySecurity {
		secIDs = append(secIDs, id)
	}
	sort.Slice(secIDs, func(i, j int) bool { return secIDs[i].String() < secIDs[j].String() })

	plan := &BackfillPlan{AccountID: accountID, Method: method}
	for _, secID := range secIDs {
		list := bySecurity[secID]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Date.Time().Equal(list[j].Date.Time()) {
				return list[i].CreatedAt.Time().Before(list[j].CreatedAt.Time())
			}
			return list[i].Date.Time().Before(list[j].Date.Time())
		})
		lots, junctions, shortfalls := replayLotsForSecurity(accountID, secID, list, method)
		plan.Lots = append(plan.Lots, lots...)
		plan.Junctions = append(plan.Junctions, junctions...)
		plan.Shortfalls = append(plan.Shortfalls, shortfalls...)
	}
	return plan, nil
}

// ApplyLotBackfill persists the lots and junction records of a plan. It does not
// flip the account's TrackLots flag — the caller (enable-lots) does that after a
// successful apply.
func (s *Service) ApplyLotBackfill(plan *BackfillPlan) error {
	for _, lot := range plan.Lots {
		if err := s.lotRepo.Create(lot); err != nil {
			return fmt.Errorf("ApplyLotBackfill: create lot %s: %w", lot.ID, err)
		}
	}
	for i := range plan.Junctions {
		if err := s.transactionLotRepo.Create(&plan.Junctions[i]); err != nil {
			return fmt.Errorf("ApplyLotBackfill: create junction: %w", err)
		}
	}
	return nil
}

// BackfillBlocker identifies a security held in an account that has a recorded
// corporate action, which prevents a naive lot backfill (the replay cannot
// reconstruct lots across a split/merger/spin-off).
type BackfillBlocker struct {
	SecurityID types.ID
	Actions    int // number of corporate actions on the security
}

// AccountBackfillBlockers returns the securities held in the account that have
// recorded corporate actions. enable-lots refuses when this is non-empty. The
// check is scoped per security held in the account (mirroring the realized-gain
// scoping in total_return.go), NOT the global CountAll gate that
// rebuild-positions uses — so a corporate action on a security the account never
// held (e.g. an SCHB split in a different account) does not block this account.
func (s *Service) AccountBackfillBlockers(accountID types.ID) ([]BackfillBlocker, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("AccountBackfillBlockers: %w", err)
	}

	seen := make(map[types.ID]bool)
	secIDs := make([]types.ID, 0)
	for _, t := range txns {
		if t.SecurityID.Valid && !seen[t.SecurityID.ID] {
			seen[t.SecurityID.ID] = true
			secIDs = append(secIDs, t.SecurityID.ID)
		}
	}
	sort.Slice(secIDs, func(i, j int) bool { return secIDs[i].String() < secIDs[j].String() })

	var blockers []BackfillBlocker
	for _, secID := range secIDs {
		actions, err := s.corporateActionRepo.ListBySecurity(secID)
		if err != nil {
			return nil, fmt.Errorf("AccountBackfillBlockers: list actions for %s: %w", secID, err)
		}
		if len(actions) > 0 {
			blockers = append(blockers, BackfillBlocker{SecurityID: secID, Actions: len(actions)})
		}
	}
	return blockers, nil
}

// backfillLot is a mutable lot tracked during the replay.
type backfillLot struct {
	lot       *Lot
	remaining types.Quantity
	seq       int // creation order within the security replay
}

// replayLotsForSecurity walks one security's transactions oldest-first,
// synthesizing lots for share-adding events and consuming open lots (by the
// chosen method) for share-removing events. Returns the lots (with their final
// remaining shares and closed flag), the junction records linking dispositions
// to the lots they consumed, and any uncovered-share shortfalls.
func replayLotsForSecurity(accountID, securityID types.ID, txns []*Transaction, method LotMethod) ([]*Lot, []TransactionLot, []BackfillShortfall) {
	var (
		open       []*backfillLot
		created    []*backfillLot
		junctions  []TransactionLot
		shortfalls []BackfillShortfall
		seq        int
	)

	addLot := func(t *Transaction, price types.Money) {
		l := NewLot(accountID, securityID, t.Shares.Quantity, price, t.Date, t.ID)
		bl := &backfillLot{lot: &l, remaining: t.Shares.Quantity, seq: seq}
		seq++
		open = append(open, bl)
		created = append(created, bl)
	}

	consume := func(t *Transaction) {
		need := t.Shares.Quantity
		for _, bl := range orderOpenLots(open, method) {
			if !need.IsPositive() {
				break
			}
			take := bl.remaining
			if take.Cmp(need) > 0 {
				take = need
			}
			junctions = append(junctions, NewTransactionLot(t.ID, bl.lot.ID, take))
			bl.remaining = bl.remaining.Sub(take)
			need = need.Sub(take)
		}
		open = keepOpen(open)
		if need.IsPositive() {
			shortfalls = append(shortfalls, BackfillShortfall{
				SecurityID:    securityID,
				TransactionID: t.ID,
				Date:          t.Date,
				Requested:     t.Shares.Quantity,
				Covered:       t.Shares.Quantity.Sub(need),
			})
		}
	}

	for _, t := range txns {
		if !t.Shares.Valid || t.Shares.Quantity.IsZero() {
			continue
		}
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeReinvestDividend:
			addLot(t, backfillBuyPrice(t))
		case TransactionTypeTransferShares:
			// Negative total_amount = outgoing transfer (shares leaving);
			// positive = incoming (carries its basis on total_amount).
			if t.TotalAmount.IsNegative() {
				consume(t)
			} else {
				addLot(t, backfillTransferInPrice(t))
			}
		case TransactionTypeSell, TransactionTypeFeeLiquidation:
			consume(t)
		default:
			// Dividend/Fee/Deposit/Withdrawal/Interest/TransferCash: no lot effect.
		}
	}

	lots := make([]*Lot, 0, len(created))
	for _, bl := range created {
		bl.lot.Shares = bl.remaining
		bl.lot.Closed = bl.remaining.IsZero()
		lots = append(lots, bl.lot)
	}
	return lots, junctions, shortfalls
}

// orderOpenLots returns the still-open lots (remaining > 0) in the order the
// chosen method consumes them.
func orderOpenLots(open []*backfillLot, method LotMethod) []*backfillLot {
	live := make([]*backfillLot, 0, len(open))
	for _, bl := range open {
		if bl.remaining.IsPositive() {
			live = append(live, bl)
		}
	}
	switch method {
	case LotMethodLIFO:
		sort.SliceStable(live, func(i, j int) bool { return live[i].seq > live[j].seq })
	case LotMethodHIFO:
		sort.SliceStable(live, func(i, j int) bool {
			if c := live[i].lot.CostPerShare.Cmp(live[j].lot.CostPerShare); c != 0 {
				return c > 0
			}
			return live[i].seq < live[j].seq
		})
	default: // FIFO
		sort.SliceStable(live, func(i, j int) bool { return live[i].seq < live[j].seq })
	}
	return live
}

// keepOpen filters out fully-consumed lots.
func keepOpen(open []*backfillLot) []*backfillLot {
	live := make([]*backfillLot, 0, len(open))
	for _, bl := range open {
		if bl.remaining.IsPositive() {
			live = append(live, bl)
		}
	}
	return live
}

// backfillBuyPrice returns the per-share cost basis for a buy/reinvest lot,
// preferring the stored price_per_share (already net of commission) and falling
// back to deriving it from the cash outlay.
func backfillBuyPrice(t *Transaction) types.Money {
	if t.PricePerShare.Valid {
		return t.PricePerShare.Money
	}
	gross := t.TotalAmount
	if gross.IsNegative() {
		gross = gross.Neg()
	}
	comm := types.ZeroMoney
	if t.Commission.Valid {
		comm = t.Commission.Money
	}
	price, err := ComputePricePerShare(gross, t.Shares.Quantity, comm)
	if err != nil {
		return types.ZeroMoney
	}
	return price
}

// backfillTransferInPrice returns the per-share basis carried by an inbound
// share transfer: the stored price_per_share if present, else total_amount
// divided across the shares.
func backfillTransferInPrice(t *Transaction) types.Money {
	if t.PricePerShare.Valid {
		return t.PricePerShare.Money
	}
	if t.Shares.Quantity.IsZero() {
		return types.ZeroMoney
	}
	return t.TotalAmount.Mul(alpacadecimal.NewFromInt(1).Div(t.Shares.Quantity.Decimal()))
}
