# Investment Total Return Specification

> **Status: v1 implemented.** The total-return model defined here is live in
> the valuation service, the `investment portfolio` CLI, and the TUI
> (investment register header, dashboard card, `View → Show closed
> positions` toggle). See
> [`implementation-plan-investment-total-return.md`](implementation-plan-investment-total-return.md)
> for the per-task history. The next batch — money-weighted return (XIRR),
> time-weighted return (TWR), and realized gain in non-lot accounts with
> corporate actions — remains in **Follow-up (not in this spec)** below.

## Overview

The current `investment portfolio` view reports only **unrealized gain on
currently-held positions**: `market_value − cost_basis` summed across rows
with shares > 0. This understates real performance because it ignores three
material cash flows:

1. **Realized gains** on positions that have been sold (in part or in full)
2. **Cash dividends** received from securities (still held or already sold)
3. **Interest** received on cash sweep and **fees / commissions** paid

This spec adds a *total-return* model that includes all four components,
exposes the breakdown on each per-security holding and at the account level,
and surfaces closed (fully-sold) positions when explicitly requested.

A time-weighted / money-weighted return (XIRR) is **not** in scope for this
spec — it is filed as a follow-up. The numbers introduced here are absolute
dollars plus a simple percent against deployed cost.

## Definitions

### Total return — account level

```
total_return = unrealized_gain
             + realized_gain
             + dividends_received
             + interest_received
             − fees_paid
```

Each component is a `types.Money` value summed across every security in the
account (plus the cash component for `interest_received` and account-level
`Fee` transactions). Cash dividends, interest, and fees are recorded directly
in the ledger; unrealized and realized gains are derived (see Computation).

### Total return — per security

For a single (account, security) pair:

```
security_total_return = unrealized_gain_on_open_shares
                      + realized_gain_on_closed_shares
                      + cash_dividends_received_for_security
                      − fees_attributed_to_security
```

`interest_received` is **not** per-security — it is account-level only.

### Total return percent

`total_return_pct` is computed against **total cost deployed** rather than
current cost basis, so that a fully-closed position still has a meaningful
denominator:

```
total_cost_deployed_for_security
    = Σ (buy.total_amount)                    -- includes commission paid on entry
    + Σ (reinvest_dividend.total_amount)      -- shares acquired via DRIP
    − Σ (return_of_capital.amount)            -- not yet a TMoney action; future-proofing

total_return_pct = total_return / total_cost_deployed × 100
```

Same formula at the account level, summed across securities. If
`total_cost_deployed` is zero (no buys ever — e.g., shares received only via
transfer-in), `total_return_pct` is `nil` and the UI displays `—`.

Reinvested dividends *are* included in the denominator. They represent
capital deployed inside the account even though no external cash was added.

### Sign conventions

All components are signed:

- `realized_gain` can be negative (sold at a loss)
- `dividends_received` is positive for cash dividends; reinvested dividends
  do **not** appear here (they show up only as new shares with their own
  cost basis)
- `fees_paid` is stored as a **positive** number representing the magnitude;
  it is subtracted in the total
- `interest_received` is positive

## Component computation

### 1. Unrealized gain

Unchanged from today (`internal/investment/valuation_service.go:151, 191,
268`). Holdings with `shares == 0` continue to be excluded from the open
holdings list, so they contribute zero to this component.

### 2. Realized gain

Computed once per `(account, security)` from the transaction ledger.

**Lot-tracking accounts** — authoritative via the `transaction_lots`
junction table. For every `sell` or `fee_liquidation` transaction:

```
realized_gain += Σ over junction rows of:
    (txn.price_per_share − lot.cost_per_share) × junction.shares
```

`txn.price_per_share` is already net of commission per
`ComputePricePerShare` (`internal/investment/computation.go:13`). Commission
itself is treated as a fee on the security, **not** netted into realized
gain a second time — see Component 5.

**Non-lot-tracking accounts** — replayed from the ledger using the existing
`replayPosition` machinery (`internal/investment/rebuild.go:77`). The
replay tracks running average cost per share. On each sell:

```
avg_cost_at_sell = position.average_cost_per_share (immediately before sell)
realized_gain   += (sell.price_per_share − avg_cost_at_sell) × sell.shares
```

A new helper `replayRealizedGain(accountID, secID, txns)` performs the
walk; it shares the same chronological ordering as `replayPosition` (date
asc, then `created_at` asc to break ties).

**Transfers, splits, mergers, spin-offs** do not produce realized gain in
TMoney. `transfer_shares` moves cost basis with the shares (no taxable
event modeled). Corporate actions adjust cost basis externally to the
ledger; see "Corporate actions" below.

### 3. Cash dividends received

Sum of `dividend` transactions, grouped by `security_id`:

```
dividends_received[secID] = Σ txn.total_amount
    where txn.type = 'dividend' and txn.security_id = secID
```

`reinvest_dividend` transactions are deliberately excluded — they have
already increased the position's shares and cost basis, so counting them
again here would double-count.

### 4. Interest received

Sum of `interest` transactions on the account. No `security_id`
attribution. Account-level only.

```
interest_received = Σ txn.total_amount
    where txn.type = 'interest' and txn.account_id = acctID
```

### 5. Fees paid

```
fees_paid[secID] = Σ (buy.commission)
                 + Σ (sell.commission)
                 + Σ (reinvest_dividend.commission)
                 + Σ (fee_liquidation.total_amount)   -- whole txn is a fee paid in shares
    where security_id = secID

fees_paid[account] = Σ fees_paid[secID]
                   + Σ (account-level fee.total_amount)   -- no security_id
```

Buy commission has been observed (`computation.go:13–17`) to be excluded
from the cost basis stored on the position/lot; it sits in the cash
outlay (`total_amount`) but not in `cost_per_share`. Subtracting it as a
fee here closes that gap so total return reflects every dollar that left
the trade.

`fee_liquidation` (a fee paid by selling shares) is treated as the entire
transaction being a fee, not as a realized event. The shares it consumes
do reduce the position; their cost basis becomes a fee.

## Data model

### `Holding` (extended)

```go
type Holding struct {
    // existing
    SecurityID   types.ID
    Shares       types.Quantity
    AvgCost      types.Money
    CurrentPrice types.Money
    PriceDate    types.Date
    MarketValue  types.Money
    CostBasis    types.Money
    GainLoss     types.Money     // unrealized only; kept for back-compat
    GainPct      float64         // unrealized only; kept for back-compat
    HasPricing   bool

    // new — total-return breakdown
    RealizedGain        types.Money
    DividendsReceived   types.Money
    FeesPaid            types.Money     // positive magnitude
    TotalCostDeployed   types.Money     // denominator for TotalReturnPct
    TotalReturn         types.Money     // sum of components above + unrealized
    TotalReturnPct      *float64        // nil if TotalCostDeployed == 0
    IsClosed            bool            // true when Shares == 0 and never re-opened
}
```

`IsClosed == true` rows are produced **only** when the caller passes
`include_closed = true` (CLI: `--include-closed`, TUI: a menu toggle).
Otherwise they remain filtered out of `Holdings` exactly as today.

### `AccountValuation` (extended)

```go
type AccountValuation struct {
    // existing
    AccountID      types.ID
    CashBalance    types.Money
    MarketValue    types.Money
    TotalValue     types.Money
    TotalCostBasis types.Money
    TotalGainLoss  types.Money     // unrealized only
    TotalGainPct   float64         // unrealized only
    Holdings       []Holding

    // new
    RealizedGain        types.Money
    DividendsReceived   types.Money
    InterestReceived    types.Money
    FeesPaid            types.Money     // positive magnitude
    TotalCostDeployed   types.Money
    TotalReturn         types.Money     // unrealized + realized + dividends + interest − fees
    TotalReturnPct      *float64
    HasClosedPositions  bool             // hint to UI to advertise --include-closed
}
```

The existing `TotalGainLoss` / `TotalGainPct` retain their unrealized-only
meaning. Total-return numbers are additive fields, not replacements. This
preserves all existing callers and tests.

### No new tables, no migration

All required data already exists in `investment_transactions`,
`investment_lots`, and `transaction_lots`. The new fields are computed on
demand by the valuation service.

## Service surface

```go
// New flag parameter on existing entry points.
func (s *Service) GetAccountValuation(
    accountID types.ID,
    asOf      types.Date,
    opts      ValuationOptions,
) (*AccountValuation, error)

func (s *Service) GetHoldings(
    accountID types.ID,
    asOf      types.Date,
    opts      ValuationOptions,
) ([]Holding, error)

type ValuationOptions struct {
    IncludeClosed bool   // when true, Holdings includes rows where Shares == 0
}
```

The current zero-arg form stays available by passing `ValuationOptions{}`
(behavior unchanged from today aside from the new total-return fields,
which are always populated).

### Helpers

- `computeRealizedGainAndFees(ctx, accountID, secID, txns) → (realized, fees Money)`
  — shared by lot-tracked and non-lot paths
- `replayRealizedGain(accountID, secID, txns) → Money` — used by the
  non-lot path; builds on `replayPosition`
- `sumDividendsForSecurity(accountID, secID) → Money`
- `sumInterestForAccount(accountID) → Money`
- `totalCostDeployed(accountID, secID) → Money`
- `listAllHeldSecurities(accountID) → []SecurityID` — open + ever-closed,
  used when `IncludeClosed == true` to surface fully-sold tickers

## Corporate actions

`RebuildPositions` and the non-lot realized-gain replay handle corporate
actions **per-security**, not all-or-nothing. A security touched only by
stock splits is replayed normally — each split's dated ratio is interleaved
into the position replay (`replayPosition`) and the realized-gain replay
(`replayRealizedGain`) — so a clean holding heals and reports realized gain
across splits even on a database that contains corporate-action history.
Only securities with a **merger or spin-off** (cross-security transforms /
cost-basis reallocation a per-security replay can't reconstruct) are skipped.

When a security contains corporate-action records:

- **Lot-tracked accounts** — realized-gain calculation works unchanged for
  every action type. Lots are mutated in-place by the corporate-action
  service (a split scales `shares` / `original_shares` / `cost_per_share`;
  mergers and spin-offs reallocate cost basis), and `transaction_lots` rows
  reference those lots. So the lot path is robust to splits, mergers, and
  spin-offs; numbers come out correct.
- **Non-lot accounts** — splits are reconstructed by the split-aware replay,
  so realized gain stays available across a split. For a security with a
  **merger or spin-off**, the replay can't reconstruct the cross-security
  transform, so the valuation service sets `RealizedGain = ZeroMoney` and
  emits a warning marker: `Holding.RealizedGainUnavailable = true`. The
  TUI/CLI render this as `(unavailable)` and a footnote explaining why; the
  other total-return components are still computed.

For exact non-lot realized gain across a merger or spin-off, enable lot
tracking on the account before running the action.

## CLI

### `investment portfolio` (extended)

New flag:

```
--include-closed     Include securities you no longer hold (fully sold).
                     They appear in a separate "Closed positions" section
                     after open holdings.
```

Default output adds total-return columns to the existing table:

```
TICKER  SHARES   PRICE     VALUE       BASIS       UNREAL       DIV       REAL        FEES     TOTAL RETURN    RET %
AAPL    100      150.00    15,000.00   12,000.00   +3,000.00    +450.00   —           −10.00   +3,440.00       +28.67%
VTI     50       240.00    12,000.00   10,500.00   +1,500.00    +120.00   +200.00     −5.00    +1,815.00       +16.50%

Account totals
  Market value          27,000.00
  Cash                   1,200.00
  Total value           28,200.00
  Cost basis (open)     22,500.00
  Unrealized gain       +4,500.00
  Realized gain           +200.00
  Dividends received      +570.00
  Interest received        +12.50
  Fees paid               −15.00
  Total return          +5,267.50
  Total return %         +22.51%
```

With `--include-closed`, a `Closed positions` block appears beneath open
holdings:

```
Closed positions (fully sold, total-return only)
TICKER  REALIZED    DIV        FEES      TOTAL RETURN    RET %
TSLA    +1,250.00   +30.00     −7.50     +1,272.50       +35.45%
```

### `investment portfolio --show-lots`

Unchanged. Lot detail rows are unrealized-only; total-return columns are
already aggregated at the holding level.

### CSV output

`tmoney investment portfolio --account X --csv` (existing flag, if
present) includes the new columns. Closed positions are appended after
open ones only when `--include-closed` is set. A `position_status` column
(`open` | `closed`) distinguishes them.

## TUI

The Securities and Prices views are unchanged. The **Account register
header** for investment accounts gains a one-line total-return summary
above today's "Cash / Market value / Total" panel:

```
Cash $1,200.00   Mkt $27,000.00   Tot $28,200.00
Unrealized +$4,500 · Realized +$200 · Div +$570 · Int +$13 · Fees −$15
Total return +$5,267.50  (+22.51%)
```

`Account → Show closed positions` menu item (toggle) flips
`IncludeClosed`. When on, the holdings list at the top of the register
gains a "Closed" section with closed tickers rendered in muted color.

The dashboard's per-account valuation card gains a `TR` row showing total
return $ and %. No new keystrokes.

## Validation and edge cases

| Case | Behavior |
|---|---|
| Position with no buys (received only via `transfer_shares`) | `TotalCostDeployed = 0`; `TotalReturnPct = nil`; displayed as `—`. Unrealized + realized + dividends still summed. |
| Closed position that paid dividends after sale (rare; e.g., last record date before sale clears) | Dividend included in `dividends_received`. Position still considered closed (`Shares == 0`). |
| `fee_liquidation` reduces shares to zero | The position closes. The fee_liquidation contributes to `fees_paid` but produces no realized gain. |
| Non-lot account with corporate actions | `RealizedGain` shown as `unavailable`; other components still computed. |
| Reinvest after position has been fully sold (data-entry error) | Treated as a re-open: new lot/position is created, fresh cost basis. Total cost deployed accumulates. |
| Account with `cash` type or `loan` type | Out of scope; total return is investment-account-only. |
| `asOf` date in the past | Realized gain, dividends, interest, fees are summed only for transactions on or before `asOf`. Unrealized uses the price as of that date. |

## Test plan

Unit tests in `internal/investment/valuation_service_test.go`:

- Holding with only buys → total return = unrealized
- Holding with buys + cash dividends → total return = unrealized + dividends
- Holding with buys + reinvested dividends → reinvest does NOT add to
  dividends_received; the reinvest cost basis flows through unrealized
- Lot-tracked sell at gain → realized gain = (sell_price − lot_cost) × shares
- Lot-tracked sell at loss → realized gain negative
- Non-lot sell at gain → replayRealizedGain produces same number as
  lot-tracked equivalent
- `fee_liquidation` → contributes to fees, not realized
- Buy commission → counted in fees, not in cost basis
- Closed position with `IncludeClosed = false` → not in holdings
- Closed position with `IncludeClosed = true` → present with
  `IsClosed = true`, zero shares, populated total-return fields
- Corporate action exists + non-lot account → `RealizedGainUnavailable`
  is true; other fields populated
- `TotalCostDeployed = 0` → `TotalReturnPct == nil`

CLI tests in `internal/cli/investment_test.go`:

- `investment portfolio` default output has new columns
- `investment portfolio --include-closed` adds Closed section
- Account totals row matches the sum of per-security values

## Follow-up (not in this spec)

- **Money-weighted return (XIRR)** at the account and per-security level
  over a user-selected date range. Adds an XIRR solver, per-cash-flow
  series construction, and date-range UI.
- **Time-weighted return (TWR)** which is comparable across portfolios
  with different deposit/withdraw timings.
- **Realized gain in non-lot accounts with corporate actions** — would
  require either an action-aware ledger replay or persisting realized
  gain on each sell at sell time.
- **Return-of-capital action type** — currently not modeled; would reduce
  cost basis without being a sale, and would subtract from
  `total_cost_deployed`.
- **1099-B-style realized gains report** — short vs. long term broken
  out, exportable as CSV for tax prep.

## Resolved design decisions

1. **Denominator for `total_return_pct`** — sum of `buy.total_amount` +
   `reinvest_dividend.total_amount`. Matches a brokerage's "total
   deployed" intuition; reinvested dividends count as deployed capital
   even though no external cash entered.
2. **`fee` transactions (no `security_id`)** — recorded at the account
   level only. They contribute to `AccountValuation.FeesPaid` but never
   to any per-security `Holding.FeesPaid`.
3. **Sign convention for `FeesPaid`** — stored as a positive magnitude
   on the struct and subtracted in the total formula. UI renders the
   column with a leading `−` so the subtraction is visually obvious.
