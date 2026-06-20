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

// EnableLotsResult summarizes an enable-lots operation.
type EnableLotsResult struct {
	AccountID   types.ID
	AccountName string
	Method      LotMethod
	Plan        *BackfillPlan // the (computed) backfill; nil only when blocked
	Applied     bool          // true when lots were persisted and TrackLots set
}

// BackfillBlockedError is returned by EnableLots when the target account holds
// one or more securities with recorded corporate actions, which a naive lot
// backfill cannot reconstruct across.
type BackfillBlockedError struct {
	AccountName string
	Blockers    []BackfillBlocker
}

func (e *BackfillBlockedError) Error() string {
	return fmt.Sprintf("account %q holds %d security(ies) with corporate actions; a naive lot backfill cannot reconstruct lots across them",
		e.AccountName, len(e.Blockers))
}

// EnableLots enables lot tracking on an existing investment/HSA account and
// backfills its lots from transaction history. With confirm=false it returns
// the computed plan and writes nothing (a preview). With confirm=true and no
// blockers it persists the lots/junctions and flips the account's TrackLots
// flag on (so the portfolio_holdings view begins summing lots).
//
// It returns a *BackfillBlockedError when the account holds a security with a
// recorded corporate action, and a plain error when the account is not an
// investment/HSA account or already has lots.
func (s *Service) EnableLots(accountID types.ID, method LotMethod, confirm bool) (*EnableLotsResult, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	res := &EnableLotsResult{AccountID: accountID, AccountName: acct.Name, Method: method}

	existing, err := s.lotRepo.ListAllByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("EnableLots: %w", err)
	}
	if len(existing) > 0 {
		return nil, fmt.Errorf("account %q already has %d lot(s); enable-lots would double-create them", acct.Name, len(existing))
	}

	blockers, err := s.AccountBackfillBlockers(accountID)
	if err != nil {
		return nil, err
	}
	if len(blockers) > 0 {
		return res, &BackfillBlockedError{AccountName: acct.Name, Blockers: blockers}
	}

	plan, err := s.PlanLotBackfill(accountID, method)
	if err != nil {
		return nil, err
	}
	res.Plan = plan

	if !confirm {
		return res, nil
	}

	if err := s.ApplyLotBackfill(plan); err != nil {
		return nil, err
	}
	acct.TrackLots = true
	if err := s.accountRepo.Update(acct); err != nil {
		return nil, fmt.Errorf("EnableLots: set track_lots: %w", err)
	}
	res.Applied = true
	return res, nil
}

// DisableLotsResult summarizes a disable-lots operation. It is the structural
// inverse of EnableLotsResult.
type DisableLotsResult struct {
	AccountID           types.ID
	AccountName         string
	LotsDeleted         int               // lots removed (preview: lots that would be removed)
	JunctionsDeleted    int               // junction rows removed (preview: would be removed)
	Securities          int               // distinct securities that had lots
	PositionsRecomputed int               // positions rebuilt to average cost (apply only)
	SkippedSecurities   int               // securities the rebuild left untouched (corporate action)
	Blockers            []BackfillBlocker // held securities with corporate actions (warning, not refusal)
	Applied             bool              // true when lots were removed and TrackLots cleared
}

// HasCorporateActions reports whether any held security has a recorded **split**
// (the only corporate-action type disable-lots proceeds through). Splits replay
// cleanly into average cost, so disable-lots warns and proceeds; mergers and
// spin-offs are refused up front via DisableLotsBlockedError, so they never
// appear here.
func (r *DisableLotsResult) HasCorporateActions() bool { return len(r.Blockers) > 0 }

// DisableLotsBlockedError is returned by DisableLots when the account holds a
// security with a merger or spin-off. On a lot-tracked account those holdings
// live only in their lots (no average-cost position row), and the ledger's
// exchange rows cannot reconstruct cost basis — so deleting the lots would
// destroy the holding. The account is left untouched (still lot-tracked).
// Splits are exempt: they replay cleanly into average cost.
type DisableLotsBlockedError struct {
	AccountName string
	Blockers    []BackfillBlocker
}

func (e *DisableLotsBlockedError) Error() string {
	return fmt.Sprintf("account %q holds %d security(ies) with a merger or spin-off; their cost basis cannot be reconstructed as average cost, so disable-lots would destroy the holding — the account stays lot-tracked",
		e.AccountName, len(e.Blockers))
}

// DisableLots turns lot tracking OFF on an existing lot-tracked investment/HSA
// account and reverts it to average cost. With confirm=false it returns a
// preview (counts of what would be removed) and writes nothing. With
// confirm=true it clears the account's TrackLots flag, deletes its lots and
// junction rows, and recomputes investment_positions to average cost from the
// transaction ledger via RebuildPositions.
//
// Corporate-action handling is split by type. A held security with only
// **splits** is a warning (warn-and-proceed): RebuildPositions replays the split
// ratios into average cost. A held security with a **merger or spin-off** is a
// refusal (*DisableLotsBlockedError): on a lot-tracked account those holdings
// exist only as lots, and the ledger cannot reconstruct their average cost, so
// deleting the lots would destroy the holding. It also returns a plain error
// when the account is not an investment/HSA account or is not lot-tracked.
func (s *Service) DisableLots(accountID types.ID, confirm bool) (*DisableLotsResult, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	res := &DisableLotsResult{AccountID: accountID, AccountName: acct.Name}

	if !acct.TrackLots {
		return nil, fmt.Errorf("account %q is not lot-tracked; nothing to disable", acct.Name)
	}

	// Partition corporate-action holdings: splits replay into average cost
	// (warn-and-proceed); mergers/spin-offs cannot be reconstructed from the
	// ledger once their lots are deleted, so refuse rather than destroy data.
	blockers, err := s.AccountBackfillBlockers(accountID)
	if err != nil {
		return nil, err
	}
	var splitBlockers, unreconstructable []BackfillBlocker
	for _, b := range blockers {
		hasNonSplit, err := s.securityHasNonSplitAction(b.SecurityID)
		if err != nil {
			return nil, fmt.Errorf("DisableLots: %w", err)
		}
		if hasNonSplit {
			unreconstructable = append(unreconstructable, b)
		} else {
			splitBlockers = append(splitBlockers, b)
		}
	}
	if len(unreconstructable) > 0 {
		return res, &DisableLotsBlockedError{AccountName: acct.Name, Blockers: unreconstructable}
	}
	res.Blockers = splitBlockers

	lots, err := s.lotRepo.ListAllByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("DisableLots: %w", err)
	}
	secSet := make(map[types.ID]bool, len(lots))
	for _, l := range lots {
		secSet[l.SecurityID] = true
	}
	res.LotsDeleted = len(lots)
	res.Securities = len(secSet)

	jc, err := s.transactionLotRepo.CountByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("DisableLots: %w", err)
	}
	res.JunctionsDeleted = jc

	if !confirm {
		return res, nil
	}

	// Apply, flag-first. EnableLots proves a parent-row UPDATE works with child
	// lots still present (it sets TrackLots after creating lots), so flipping
	// first is safe — and it closes the window in which the flag would say
	// "lot-tracked" while the lots are already gone (which read/write paths key
	// off of). Then delete junctions (the subquery needs the lots to still
	// exist), delete lots, and recompute average-cost positions from the ledger.
	acct.TrackLots = false
	if err := s.accountRepo.Update(acct); err != nil {
		return nil, fmt.Errorf("DisableLots: clear track_lots: %w", err)
	}

	njDeleted, err := s.transactionLotRepo.DeleteByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("DisableLots: delete junctions: %w", err)
	}
	res.JunctionsDeleted = njDeleted

	nlDeleted, err := s.lotRepo.DeleteByAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("DisableLots: delete lots: %w", err)
	}
	res.LotsDeleted = nlDeleted

	rb, err := s.RebuildPositions(accountID)
	if err != nil {
		return nil, fmt.Errorf("DisableLots: rebuild positions: %w", err)
	}
	res.PositionsRecomputed = rb.PositionsRecomputed
	res.SkippedSecurities = rb.SkippedSecurities
	res.Applied = true
	return res, nil
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
