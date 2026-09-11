package investment

import (
	"fmt"
	"math"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// Money-weighted (IRR) and time-weighted (TWR) returns for an investment
// account.
//
// Both figures measure the account against the money that crossed its
// boundary — deposits, withdrawals, cash transfers and share transfers —
// rather than against the sum of buys that TotalReturnPct uses. Moving idle
// cash into shares is therefore invisible to both: the account is worth the
// same the moment after the buy as the moment before.
//
//   - IRR is the annual rate that discounts every external flow plus the
//     closing value to zero (an XIRR). It rewards putting money in before a
//     rise and taking it out before a fall, so it is the investor's own return.
//   - TWR chains the growth of each sub-period between external flows, so
//     the size and timing of contributions do not affect it. It is the return
//     the holdings produced, and is comparable to a fund's published return.
//
// Both need the account's value on every flow date. That value is rebuilt
// from the ledger in a single ascending pass (ledgerWalker): cash from the
// cash-affecting types, shares from a per-security replay that honours splits
// and mergers, and prices from the stored history on or before that date
// (falling back to the running cost basis when no price exists yet).

// accountPerformance is the result of computeAccountPerformance. A nil
// pointer means the figure is undefined for this ledger (no external flows,
// no period with a positive opening value, or no solver root).
type accountPerformance struct {
	MoneyWeightedReturnPct          *float64
	TimeWeightedReturnPct           *float64
	TimeWeightedReturnAnnualizedPct *float64
}

// daysPerYear is the day-count basis for annualising both figures.
const daysPerYear = 365.25

// externalFlow reports the signed amount that crossed the account boundary in
// this transaction (positive into the account) and whether the transaction is
// an external flow at all. Deposits, withdrawals and cash transfers carry the
// cash moved; a share transfer carries the cost basis that moved with the
// shares (the ledger stores no market value for it). Buys, sells, dividends,
// interest, fees and corporate-action exchanges are internal and return false.
func externalFlow(t *Transaction) (types.Money, bool) {
	switch t.Type {
	case TransactionTypeDeposit, TransactionTypeWithdrawal,
		TransactionTypeTransferCash, TransactionTypeTransferShares:
		return t.TotalAmount, true
	}
	return types.ZeroMoney, false
}

// computeAccountPerformance derives IRR and TWR for the account as of asOf.
// Transactions dated after asOf are ignored, so the figures describe the
// account as it stood on that date.
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
	// date (so the chain starts as soon as the account has a value) and
	// every date with a net external flow.
	// Both maps are keyed by the date's string form: types.Date wraps a
	// time.Time, and two equal dates scanned from different rows need not
	// compare equal as struct values.
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
			// The flow lands on this date; the sub-period's closing value
			// is what the account was worth just before the flow arrived.
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

	if chained {
		twr := factor - 1
		perf.TimeWeightedReturnPct = ptrPct(twr)
		days := daysBetween(checkpoints[0], asOf)
		if days >= 365 && factor > 0 {
			ann := math.Pow(factor, daysPerYear/days) - 1
			perf.TimeWeightedReturnAnnualizedPct = ptrPct(ann)
		}
	}

	flows = append(flows, cashFlow{date: asOf, amount: endValue})
	if r, ok := solveXIRR(flows); ok {
		perf.MoneyWeightedReturnPct = ptrPct(r)
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

// solveXIRR returns the annual rate r at which Σ amount_i / (1+r)^(years_i)
// is zero. It needs at least one positive and one negative flow spanning
// more than one day, and a sign change of the NPV on (−99.99 %, 1e6]. The
// root is bracketed by expanding the upper bound and then bisected, which is
// slower than Newton but never diverges.
func solveXIRR(flows []cashFlow) (float64, bool) {
	if len(flows) < 2 {
		return 0, false
	}
	first := flows[0].date
	lastDate := first
	hasPos, hasNeg := false, false
	for _, f := range flows {
		if f.date.Before(first) {
			first = f.date
		}
		if f.date.After(lastDate) {
			lastDate = f.date
		}
		if f.amount > 0 {
			hasPos = true
		}
		if f.amount < 0 {
			hasNeg = true
		}
	}
	if !hasPos || !hasNeg || daysBetween(first, lastDate) < 1 {
		return 0, false
	}

	npv := func(r float64) float64 {
		sum := 0.0
		for _, f := range flows {
			years := daysBetween(first, f.date) / daysPerYear
			sum += f.amount / math.Pow(1+r, years)
		}
		return sum
	}

	lo, hi := -0.9999, 1.0
	fLo, fHi := npv(lo), npv(hi)
	for fLo*fHi > 0 && hi < 1e6 {
		hi *= 10
		fHi = npv(hi)
	}
	if fLo*fHi > 0 || math.IsNaN(fLo) || math.IsNaN(fHi) {
		return 0, false
	}
	// Bisect: keep the sign of npv(lo) so each step halves the bracket.
	for range 200 {
		mid := (lo + hi) / 2
		fMid := npv(mid)
		if fMid == 0 || hi-lo < 1e-10 {
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
// from one ascending ledger. Per-security splits, merger dates and price
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
	mergedOn  map[types.ID]types.Date // source security → earliest merger date
	merged    map[types.ID]bool
	prices    map[types.ID][]*price.Price // ascending by date
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
		mergedOn:  make(map[types.ID]types.Date),
		merged:    make(map[types.ID]bool),
		prices:    make(map[types.ID][]*price.Price),
	}
}

// advanceTo applies every transaction dated on or before date, then any
// split or merger dated on or before it. Dates must be non-decreasing across
// calls.
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
// merger date and price history on first use.
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
		actions, err := w.svc.corporateActionRepo.ListBySecurity(secID)
		if err != nil {
			return nil, fmt.Errorf("failed to list corporate actions for performance: %w", err)
		}
		for _, ca := range actions {
			if ca.ActionType != ActionTypeMerger || ca.SecurityID != secID {
				continue
			}
			if md, ok := w.mergedOn[secID]; !ok || ca.ActionDate.Before(md) {
				w.mergedOn[secID] = ca.ActionDate
			}
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
// before date.
func (w *ledgerWalker) priceOnOrBefore(secID types.ID, date types.Date) (types.Money, bool) {
	hist := w.prices[secID]
	// First index whose date is after `date`; the answer is the one before it.
	i := sort.Search(len(hist), func(i int) bool { return hist[i].Date.After(date) })
	if i == 0 {
		return types.ZeroMoney, false
	}
	return hist[i-1].Price, true
}
