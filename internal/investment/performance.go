package investment

import (
	"fmt"
	"math"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// Money-weighted (IRR) and time-weighted (TWR) return for an account, both
// measured against external flows (see specs/investment-total-return.md,
// "Money- and time-weighted return").

// accountPerformance is the result of computeAccountPerformance. A nil
// pointer means the ledger cannot define the figure.
type accountPerformance struct {
	MoneyWeightedReturnPct           *float64
	MoneyWeightedReturnAnnualizedPct *float64
	TimeWeightedReturnPct            *float64
	TimeWeightedReturnAnnualizedPct  *float64
}

// daysPerYear is the day-count basis for annualising both figures.
const daysPerYear = 365.25

// externalFlow reports the signed amount that crossed the account boundary in
// this transaction (positive into the account) and whether the transaction is
// an external flow at all. A share transfer carries the cost basis that moved
// with the shares: the ledger stores no market value for it. Merger cash
// consideration and spin-off cash-in-lieu are posted as `deposit` rows but
// are proceeds of a holding, so they are internal like a dividend.
func externalFlow(t *Transaction) (types.Money, bool) {
	switch t.Type {
	case TransactionTypeDeposit:
		if isCorporateActionCash(t) {
			return types.ZeroMoney, false
		}
		return t.TotalAmount, true
	case TransactionTypeWithdrawal, TransactionTypeTransferCash, TransactionTypeTransferShares:
		return t.TotalAmount, true
	}
	return types.ZeroMoney, false
}

// computeAccountPerformance derives IRR and TWR for the account as of asOf.
// Transactions dated after asOf are ignored.
func (s *ValuationService) computeAccountPerformance(accountID types.ID, asOf types.Date) (accountPerformance, error) {
	var perf accountPerformance

	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{ToDate: &asOf})
	if err != nil {
		return perf, fmt.Errorf("failed to list transactions for performance: %w", err)
	}
	if len(txns) == 0 {
		return perf, nil
	}
	sortTransactionsAscending(txns)

	// Checkpoints are the dates the chain is valued on: the first ledger
	// date and every date with a net external flow. Both maps are keyed by
	// the date's string form: types.Date wraps a time.Time, and two equal
	// dates scanned from different rows need not compare equal as values.
	flowByDate := make(map[string]types.Money)
	checkpointSet := map[string]types.Date{txns[0].Date.String(): txns[0].Date}
	for _, t := range txns {
		f, ok := externalFlow(t)
		if !ok {
			continue
		}
		key := t.Date.String()
		prev, seen := flowByDate[key]
		if !seen {
			prev = types.ZeroMoney
		}
		flowByDate[key] = prev.Add(f)
		checkpointSet[key] = t.Date
	}
	checkpoints := make([]types.Date, 0, len(checkpointSet))
	for _, d := range checkpointSet {
		checkpoints = append(checkpoints, d)
	}
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].Before(checkpoints[j]) })

	w := newLedgerWalker(s, txns, asOf)

	var (
		factor    = 1.0
		chained   bool
		prevValue float64
		prevSet   bool
		flows     []cashFlow
	)
	for _, d := range checkpoints {
		if err := w.advanceTo(d); err != nil {
			return perf, err
		}
		v := w.value(d).Float64()
		f := flowByDate[d.String()].Float64()
		if prevSet && prevValue > 0 {
			// The flow lands on this date; the sub-period closes at what
			// the account was worth just before it arrived.
			factor *= math.Max(v-f, 0) / prevValue
			chained = true
		}
		prevValue, prevSet = v, true
		if f != 0 {
			flows = append(flows, cashFlow{date: d, amount: -f})
		}
	}

	if err := w.advanceTo(asOf); err != nil {
		return perf, err
	}
	endValue := w.value(asOf).Float64()
	last := checkpoints[len(checkpoints)-1]
	if prevSet && prevValue > 0 && last.Before(asOf) {
		factor *= math.Max(endValue, 0) / prevValue
		chained = true
	}

	days := daysBetween(checkpoints[0], asOf)
	if chained {
		perf.TimeWeightedReturnPct = ptrPct(factor - 1)
		if days >= 365 && factor > 0 {
			perf.TimeWeightedReturnAnnualizedPct = ptrPct(math.Pow(factor, daysPerYear/days) - 1)
		}
	}

	flows = append(flows, cashFlow{date: asOf, amount: endValue})
	if x, ok := solveXIRR(flows); ok {
		// x = ln(1+r) per year. The holding-period figure compounds it over
		// the flows' own span; the annual figure is reported only once that
		// span reaches a year, matching TWR.
		span := daysBetween(flows[0].date, asOf) / daysPerYear
		perf.MoneyWeightedReturnPct = ptrPct(math.Exp(x*span) - 1)
		if days >= 365 {
			perf.MoneyWeightedReturnAnnualizedPct = ptrPct(math.Exp(x) - 1)
		}
	}
	return perf, nil
}

// ptrPct converts a rate (0.1 = 10 %) to a heap-allocated percent.
func ptrPct(rate float64) *float64 {
	p := rate * 100
	return &p
}

// daysBetween returns the whole days from a to b (negative when b is earlier).
func daysBetween(a, b types.Date) float64 {
	return b.Time().Sub(a.Time()).Hours() / 24
}

// sortTransactionsAscending orders a ledger oldest-first (date, then
// created_at) — the canonical replay order used by replayPosition.
func sortTransactionsAscending(txns []*Transaction) {
	sort.SliceStable(txns, func(i, j int) bool {
		if txns[i].Date.Equal(txns[j].Date) {
			return txns[i].CreatedAt.Time().Before(txns[j].CreatedAt.Time())
		}
		return txns[i].Date.Before(txns[j].Date)
	})
}

// cashFlow is one dated amount from the investor's point of view: money paid
// into the account is negative, money taken out (or the closing value) is
// positive.
type cashFlow struct {
	date   types.Date
	amount float64
}

// solveXIRR returns x = ln(1+r), where r is the annual rate at which
// Σ amount_i · e^(−x · years_i) is zero. Solving for x rather than r lets the
// root sit arbitrarily close to r = −1 (a large one-day loss) or far above
// r = 1e6 (a large one-day gain) without the bracket missing it. It needs a
// negative flow and a later flow at least one day apart. Flows that never
// return anything positive are a total loss and report x = −Inf (r = −100 %).
func solveXIRR(flows []cashFlow) (float64, bool) {
	if len(flows) < 2 {
		return 0, false
	}
	first, lastDate := flows[0].date, flows[0].date
	hasPos, hasNeg := false, false
	for _, f := range flows {
		if f.date.Before(first) {
			first = f.date
		}
		if f.date.After(lastDate) {
			lastDate = f.date
		}
		hasPos = hasPos || f.amount > 0
		hasNeg = hasNeg || f.amount < 0
	}
	spanDays := daysBetween(first, lastDate)
	if !hasNeg || spanDays < 1 {
		return 0, false
	}
	if !hasPos {
		return math.Inf(-1), true
	}

	npv := func(x float64) float64 {
		sum := 0.0
		for _, f := range flows {
			t := daysBetween(first, f.date) / daysPerYear
			sum += f.amount * math.Exp(-x*t)
		}
		return sum
	}

	// Expand the bracket outward from 0 until the sign changes, then
	// bisect. |x·span| ≤ 700 keeps every exponent finite.
	limit := 700 / (spanDays / daysPerYear)
	lo, hi := -1.0, 1.0
	fLo, fHi := npv(lo), npv(hi)
	for fLo*fHi > 0 && hi < limit {
		lo, hi = lo*2, hi*2
		fLo, fHi = npv(lo), npv(hi)
	}
	if fLo*fHi > 0 || math.IsNaN(fLo) || math.IsNaN(fHi) {
		return 0, false
	}
	for range 200 {
		mid := (lo + hi) / 2
		fMid := npv(mid)
		if fMid == 0 || hi-lo < 1e-12 {
			return mid, true
		}
		if fLo*fMid < 0 {
			hi = mid
		} else {
			lo, fLo = mid, fMid
		}
	}
	return (lo + hi) / 2, true
}

// ledgerWalker rebuilds an account's cash and positions at successive dates
// from one ascending ledger. Per-security splits, corporate actions and price
// history are loaded once, on first sight of the security.
type ledgerWalker struct {
	svc  *ValuationService
	asOf types.Date
	txns []*Transaction
	next int

	cash      types.Money
	positions map[types.ID]*Position
	splits    map[types.ID][]splitEvent
	splitIdx  map[types.ID]int
	basisCuts map[types.ID][]basisEvent // spin-off parent allocations, ascending
	cutIdx    map[types.ID]int
	mergedOn  map[types.ID]types.Date // source security → earliest merger date
	merged    map[types.ID]bool
	prices    map[types.ID][]*price.Price // ascending by date
}

// basisEvent scales a security's running average cost on a date: a spin-off
// leaves the parent with parentAllocPct of its former basis.
type basisEvent struct {
	Date   types.Date
	Factor alpacadecimal.Decimal
}

func newLedgerWalker(svc *ValuationService, txns []*Transaction, asOf types.Date) *ledgerWalker {
	return &ledgerWalker{
		svc:       svc,
		asOf:      asOf,
		txns:      txns,
		cash:      types.ZeroMoney,
		positions: make(map[types.ID]*Position),
		splits:    make(map[types.ID][]splitEvent),
		splitIdx:  make(map[types.ID]int),
		basisCuts: make(map[types.ID][]basisEvent),
		cutIdx:    make(map[types.ID]int),
		mergedOn:  make(map[types.ID]types.Date),
		merged:    make(map[types.ID]bool),
		prices:    make(map[types.ID][]*price.Price),
	}
}

// advanceTo applies every transaction dated on or before date, then any
// split, spin-off basis cut or merger dated on or before it. Dates must be
// non-decreasing across calls.
func (w *ledgerWalker) advanceTo(date types.Date) error {
	for w.next < len(w.txns) && !w.txns[w.next].Date.After(date) {
		t := w.txns[w.next]
		w.next++
		if t.Type.AffectsCash() {
			w.cash = w.cash.Add(t.TotalAmount)
		}
		if !t.SecurityID.Valid || !t.Shares.Valid || t.Shares.Quantity.IsZero() {
			continue
		}
		pos, err := w.position(t.SecurityID.ID)
		if err != nil {
			return err
		}
		w.applySplitsBefore(t.SecurityID.ID, pos, t.Date)
		applyShareTransaction(pos, t)
	}
	for secID, pos := range w.positions {
		w.applySplitsThrough(secID, pos, date)
		w.applyBasisCutsThrough(secID, pos, date)
		if md, ok := w.mergedOn[secID]; ok && !w.merged[secID] && !md.After(date) {
			pos.Shares = types.ZeroQuantity
			w.merged[secID] = true
		}
	}
	return nil
}

// applyShareTransaction mutates pos for one share-bearing transaction. A
// removal larger than the running position clamps to zero instead of
// failing: this is a display figure, and an inconsistent ledger is repaired
// by RebuildPositions, not here.
func applyShareTransaction(pos *Position, t *Transaction) {
	shares := t.Shares.Quantity
	add := func() {
		p := types.ZeroMoney
		switch {
		case t.PricePerShare.Valid:
			p = t.PricePerShare.Money
		case !shares.IsZero():
			p = t.TotalAmount.Abs().Mul(alpacadecimal.NewFromInt(1).Div(shares.Decimal()))
		}
		_ = pos.AddShares(shares, p)
	}
	remove := func() {
		if pos.Shares.Cmp(shares) <= 0 {
			pos.Shares = types.ZeroQuantity
			return
		}
		_ = pos.RemoveShares(shares)
	}
	switch t.Type {
	case TransactionTypeBuy, TransactionTypeReinvestDividend, TransactionTypeExchange:
		add()
	case TransactionTypeSell, TransactionTypeFeeLiquidation:
		remove()
	case TransactionTypeTransferShares:
		if t.TotalAmount.IsNegative() {
			remove()
		} else {
			add()
		}
	}
}

// position returns the running position for a security, loading its splits,
// corporate actions and price history on first use.
func (w *ledgerWalker) position(secID types.ID) (*Position, error) {
	if pos, ok := w.positions[secID]; ok {
		return pos, nil
	}
	pos := NewPosition(types.ID{}, secID)
	w.positions[secID] = &pos

	if w.svc.corporateActionRepo != nil {
		splits, err := splitEventsFor(w.svc.corporateActionRepo, secID)
		if err != nil {
			return nil, err
		}
		w.splits[secID] = splits
		if err := w.loadActions(secID); err != nil {
			return nil, err
		}
	}
	if w.svc.priceRepo != nil {
		hist, err := w.svc.priceRepo.GetPriceHistory(secID, nil, &w.asOf)
		if err != nil {
			return nil, fmt.Errorf("failed to load price history for performance: %w", err)
		}
		// GetPriceHistory returns newest-first; the lookup wants ascending.
		for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
			hist[i], hist[j] = hist[j], hist[i]
		}
		w.prices[secID] = hist
	}
	return &pos, nil
}

// loadActions records the security's merger date (its shares leave without a
// ledger row) and spin-off basis cuts (its cost is reallocated without one).
func (w *ledgerWalker) loadActions(secID types.ID) error {
	actions, err := w.svc.corporateActionRepo.ListBySecurity(secID)
	if err != nil {
		return fmt.Errorf("failed to list corporate actions for performance: %w", err)
	}
	for _, ca := range actions {
		if ca.SecurityID != secID {
			continue
		}
		switch ca.ActionType {
		case ActionTypeMerger:
			if md, ok := w.mergedOn[secID]; !ok || ca.ActionDate.Before(md) {
				w.mergedOn[secID] = ca.ActionDate
			}
		case ActionTypeSpinOff:
			params, err := ParseSpinOffParams(ca.Parameters)
			if err != nil {
				return fmt.Errorf("failed to parse spin-off params for performance: %w", err)
			}
			factor := alpacadecimal.NewFromFloat(params.ParentAllocationPct).Div(alpacadecimal.NewFromInt(100))
			w.basisCuts[secID] = append(w.basisCuts[secID], basisEvent{Date: ca.ActionDate, Factor: factor})
		}
	}
	cuts := w.basisCuts[secID]
	sort.SliceStable(cuts, func(i, j int) bool { return cuts[i].Date.Before(cuts[j].Date) })
	return nil
}

// applySplitsBefore applies splits dated strictly before nextTxnDate — the
// same pre-split rule replayPosition uses for a transaction on the split date.
func (w *ledgerWalker) applySplitsBefore(secID types.ID, pos *Position, nextTxnDate types.Date) {
	w.splitIdx[secID] = applyDueSplits(pos, w.splits[secID], w.splitIdx[secID], nextTxnDate)
}

// applySplitsThrough applies splits dated on or before date, so a checkpoint
// valued at the end of the split day sees post-split shares.
func (w *ledgerWalker) applySplitsThrough(secID types.ID, pos *Position, date types.Date) {
	splits := w.splits[secID]
	si := w.splitIdx[secID]
	for si < len(splits) && !splits[si].Date.After(date) {
		applySplitToPosition(pos, splits[si].Ratio)
		si++
	}
	w.splitIdx[secID] = si
}

// applyBasisCutsThrough scales the running average cost for every spin-off
// dated on or before date, so the cost-basis fallback matches the live
// position after the action.
func (w *ledgerWalker) applyBasisCutsThrough(secID types.ID, pos *Position, date types.Date) {
	cuts := w.basisCuts[secID]
	ci := w.cutIdx[secID]
	for ci < len(cuts) && !cuts[ci].Date.After(date) {
		pos.AverageCostPerShare = pos.AverageCostPerShare.Mul(cuts[ci].Factor)
		ci++
	}
	w.cutIdx[secID] = ci
}

// value is cash plus every open position priced on or before date. A
// position with no price yet is carried at its running cost basis, matching
// the live valuation's fallback.
func (w *ledgerWalker) value(date types.Date) types.Money {
	total := w.cash
	for secID, pos := range w.positions {
		if pos.Shares.IsZero() {
			continue
		}
		if p, ok := w.priceOnOrBefore(secID, date); ok {
			total = total.Add(p.Mul(pos.Shares.Decimal()))
		} else {
			total = total.Add(pos.CostBasis())
		}
	}
	return total
}

// priceOnOrBefore finds the latest stored price for the security dated on or
// before date, in the share units the walker holds on that date. Split
// processing rewrites every stored price on or before the split day into
// post-split units (adjustPrices), while the walker's share count stays
// pre-split until the split is applied — so each split not yet applied
// scales the stored price back up by its ratio.
func (w *ledgerWalker) priceOnOrBefore(secID types.ID, date types.Date) (types.Money, bool) {
	hist := w.prices[secID]
	// First index whose date is after `date`; the answer is the one before it.
	i := sort.Search(len(hist), func(i int) bool { return hist[i].Date.After(date) })
	if i == 0 {
		return types.ZeroMoney, false
	}
	p := hist[i-1].Price
	for _, sp := range w.splits[secID][w.splitIdx[secID]:] {
		p = p.Mul(sp.Ratio)
	}
	return p, true
}
