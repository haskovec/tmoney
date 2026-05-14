# Implementation Plan: Investment Total Return

This document defines the order in which the total-return feature
(`specs/investment-total-return.md`) should be implemented. Each item is
one small session of work following a red-green (test-first) pattern.
Mark items as complete with `[x]` as they are finished.

Spec:

- `specs/investment-total-return.md` — total-return feature spec

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered to:

1. Extend the data model first (Phase 1) so subsequent phases have a
   place to put their numbers without changing public surfaces twice.
2. Build each return component as a pure helper before any of them are
   wired into the valuation service (Phases 2–5). Each helper is its
   own session with its own focused test fixture.
3. Wire the helpers into the existing valuation paths last among the
   compute work (Phase 6), so any breakage is localized to one
   integration step rather than smeared across the component phases.
4. Surface closed positions (Phase 7) only after total return works for
   open positions, since closed positions are the harder code path and
   benefit from the open-position machinery already being green.
5. CLI before TUI (Phases 8 and 9) — the CLI is a thinner shell and
   tests are easier to write against text output. TUI work then reuses
   the populated `AccountValuation` shape with no further service
   changes.
6. Documentation last (Phase 10).

Phases 4 (realized gain — lot-tracked) and 5 (realized gain — non-lot)
could be reordered. Lot-tracked is listed first because it uses an
already-authoritative table (`transaction_lots`) and is the simpler
case; the non-lot path replays the ledger and benefits from the
lot-tracked tests already passing.

The corporate-action gating for non-lot accounts is its own item
(TR-009) so the unavailable-marker UI path can be tested independently
of the happy-path replay.

---

## Phase 1: Data Model Extensions

Add the new fields with zero-valued defaults so existing call sites keep
working. No behavior change yet.

- [x] **TR-001 — Add `ValuationOptions` and extend `Holding` / `AccountValuation`**
  - RED: test `TestValuation_NewFieldsZeroValue_BackCompat` constructs a
    `Holding` and an `AccountValuation` via the existing service paths
    with `ValuationOptions{}` and asserts every new field is the zero
    value (e.g., `RealizedGain.IsZero()`, `TotalReturnPct == nil`).
    Existing valuation tests must continue to pass unchanged.
  - GREEN: in `internal/investment/valuation.go`, add fields per the
    spec's Data Model section to both `Holding` and `AccountValuation`.
    Add the `ValuationOptions` struct with `IncludeClosed bool`.
  - Signature change: `GetAccountValuation` and `GetHoldings` gain a
    trailing `opts ValuationOptions` parameter. Update every caller
    inside the repo to pass `ValuationOptions{}` (compile-driven). No
    field is populated yet — the new fields are zero in this phase.
  - Confirm: `go build ./... && go test ./internal/investment/...` green.

## Phase 2: Dividend & Interest Aggregation

Pure ledger sums. No dependency on positions or lots.

- [x] **TR-002 — `sumDividendsForSecurity(accountID, secID) Money`**
  - RED: test `TestSumDividendsForSecurity_HappyPath` — fixture with two
    `dividend` transactions on AAPL totalling $175 returns $175. Returns
    zero when no dividends exist.
  - Test `TestSumDividendsForSecurity_IgnoresReinvest` — `reinvest_dividend`
    transactions on the same security are *not* counted.
  - Test `TestSumDividendsForSecurity_IgnoresOtherSecurities` — dividends
    on a different `security_id` don't leak into the total.
  - GREEN: implement in a new file
    `internal/investment/total_return.go`. Reads via the existing
    transaction repo with a filter on type and security.

- [x] **TR-003 — `sumInterestForAccount(accountID) Money`**
  - RED: test `TestSumInterestForAccount_HappyPath` — fixture with three
    `interest` transactions on the cash sweep returns the total.
  - Test `TestSumInterestForAccount_OtherAccountIgnored` — interest on a
    sibling investment account doesn't leak.
  - GREEN: add to `total_return.go`. No `security_id` filter; account-
    level only.

## Phase 3: Fee Aggregation

- [x] **TR-004 — `sumFeesForSecurity(accountID, secID) Money`**
  - RED: test `TestSumFeesForSecurity_HappyPath` — fixture with one buy
    ($5 commission), one sell ($10 commission), one reinvest ($1
    commission), and one `fee_liquidation` ($25 `total_amount`) on the
    same security returns $41 (positive magnitude).
  - Test `TestSumFeesForSecurity_NoCommissionField` — buys/sells with
    NULL commission contribute $0, not an error.
  - Test `TestSumFeesForSecurity_AccountLevelFeeIgnored` — a `fee`
    transaction without `security_id` does NOT appear in any per-security
    sum.
  - GREEN: implement in `total_return.go`. Returns positive magnitude.

- [ ] **TR-005 — `sumFeesForAccount(accountID) Money`**
  - RED: test `TestSumFeesForAccount_AccumulatesPerSecurityAndAccount`
    — fixture with two securities, each with commissions, plus two
    account-level `fee` transactions returns the grand total.
  - Test `TestSumFeesForAccount_OnlyAccountLevelFees` — account with no
    trades but two `fee` transactions returns just those.
  - GREEN: implement in `total_return.go`. Sums per-security results
    plus account-level `fee` totals.

## Phase 4: Realized Gain — Lot-Tracked

- [ ] **TR-006 — `realizedGainLotTracked(accountID, secID) Money`**
  - RED: test `TestRealizedGainLotTracked_SingleSell_AtGain` — buy 10
    @ $100, sell 5 @ $150 → realized gain = $250.
  - Test `TestRealizedGainLotTracked_SingleSell_AtLoss` — buy 10 @ $100,
    sell 5 @ $80 → realized gain = −$100.
  - Test `TestRealizedGainLotTracked_MultipleSells_AcrossLots` — two
    buys at different prices, one sell allocated across both lots →
    realized gain = Σ over junction rows.
  - Test `TestRealizedGainLotTracked_FeeLiquidation` — fee_liquidation
    sale reducing 0.1 shares at price $200 from a lot bought at $100 →
    realized gain = ($200 − $100) × 0.1 = $10. (The fee component is
    separate; this only validates the realized side.)
  - Test `TestRealizedGainLotTracked_NoSells_ReturnsZero`.
  - GREEN: implement in `total_return.go`. Walks
    `transaction_lots` for txns of type `sell` and `fee_liquidation`.
    Uses `txn.price_per_share − lot.cost_per_share`.

## Phase 5: Realized Gain — Non-Lot

- [ ] **TR-007 — `replayRealizedGain(accountID, secID, txns) Money`**
  - RED: test `TestReplayRealizedGain_HappyPath` — buy 10 @ $100, buy
    10 @ $120 → avg cost $110. Sell 5 @ $150 → realized gain = (150 −
    110) × 5 = $200. Then sell 5 @ $130 → realized gain += (130 − 110)
    × 5 = $100. Total = $300.
  - Test `TestReplayRealizedGain_LossThenGain` — verify a loss sale
    contributes a negative number; subsequent buys do not "reset" the
    running avg cost (they accumulate as the existing
    `Position.AddShares` does).
  - Test `TestReplayRealizedGain_SameDateOrdering` — two same-date
    transactions resolve via `CreatedAt`, matching `replayPosition`'s
    sort.
  - Test `TestReplayRealizedGain_ReinvestRaisesAvgCost` — a
    reinvest_dividend before a sell raises the avg cost so realized
    gain on the subsequent sell reflects the new basis.
  - GREEN: implement in `total_return.go`. Mirrors `replayPosition`'s
    chronological walk but accumulates realized gain on each sell
    instead of just adjusting shares. Use `Position.AddShares` for
    buy/reinvest, capture `position.AverageCostPerShare` before each
    sell.

- [ ] **TR-008 — Service entry point `realizedGainNonLot(accountID, secID)`**
  - RED: test `TestRealizedGainNonLot_DelegatesToReplay` — fixture
    matching TR-007's happy path called via the service wrapper returns
    the same $300.
  - GREEN: thin service method that loads transactions for the
    `(account, security)` pair, sorts them, and delegates to
    `replayRealizedGain`.

## Phase 6: Corporate-Action Gate

- [ ] **TR-009 — Detect corporate actions and mark realized gain unavailable**
  - RED: test `TestRealizedGain_NonLot_WithCorporateActions_Unavailable`
    — fixture with a split on AAPL plus a sell after the split. The
    holding's `RealizedGainUnavailable == true`,
    `RealizedGain.IsZero()`, but `DividendsReceived` and `FeesPaid` are
    still populated normally.
  - Test `TestRealizedGain_LotTracked_WithCorporateActions_StillComputed`
    — lot-tracked account with the same corporate action still produces
    a real number (no `unavailable` flag), because the junction table
    references post-action lots.
  - GREEN: add `RealizedGainUnavailable bool` to `Holding`. In the
    realized-gain entry point, when the account is non-lot and
    `CorporateActionRepository.CountAll() > 0`, return
    `(ZeroMoney, true)`. Plumb the boolean through to the holding.

## Phase 7: Total Cost Deployed

- [ ] **TR-010 — `totalCostDeployedForSecurity(accountID, secID) Money`**
  - RED: test `TestTotalCostDeployed_BuyOnly` — two buys totalling
    $1,500 return $1,500.
  - Test `TestTotalCostDeployed_BuyPlusReinvest` — buy $1,000 plus
    reinvest $50 returns $1,050.
  - Test `TestTotalCostDeployed_TransferOnly_Zero` — security received
    only via `transfer_shares` returns $0 (the `transfer_shares` txn
    type is excluded from the denominator).
  - GREEN: implement in `total_return.go`. Sums `total_amount` for
    `buy` and `reinvest_dividend` transactions with the given
    `security_id`.

- [ ] **TR-011 — `totalCostDeployedForAccount(accountID) Money`**
  - RED: test `TestTotalCostDeployedForAccount_SumsAcrossSecurities` —
    account with two securities, each with buys totalling different
    amounts, returns the grand total.
  - GREEN: implement in `total_return.go`.

## Phase 8: Wire Components Into the Valuation Service

- [ ] **TR-012 — Enrich `Holding` with total-return components**
  - RED: test `TestGetHoldings_PopulatesTotalReturn` — fixture with one
    holding that has buys, a partial sell, dividends, and commissions.
    After `GetHoldings(accountID, asOf, ValuationOptions{})` the
    returned `Holding` has `RealizedGain`, `DividendsReceived`,
    `FeesPaid`, `TotalCostDeployed`, `TotalReturn`, and
    `TotalReturnPct` populated to the expected values per the spec
    formula.
  - Test `TestGetHoldings_TotalReturnPctNilWhenNoBuys` — holding
    received only via `transfer_shares` has `TotalReturnPct == nil`.
  - GREEN: add `enrichHoldingTotalReturn(h *Holding, accountID types.ID,
    asOf types.Date)` in `valuation_service.go`. Call it from each of
    `getHoldingsFromView`, `getHoldingsFromPositions`, and
    `getHoldingsFromLots` after the basic holding is built. The
    realized-gain entry point dispatches lot vs. non-lot based on
    `acct.TrackLots`.

- [ ] **TR-013 — Populate account-level totals in `AccountValuation`**
  - RED: test `TestGetAccountValuation_PopulatesAccountTotals` — fixture
    with two holdings produces an `AccountValuation` whose `RealizedGain`,
    `DividendsReceived`, `InterestReceived`, `FeesPaid`,
    `TotalCostDeployed`, `TotalReturn`, and `TotalReturnPct` equal the
    sum of the per-holding values (plus account-level interest and
    fees).
  - Test `TestGetAccountValuation_LegacyFieldsUnchanged` — the existing
    `TotalGainLoss` / `TotalGainPct` still mean *unrealized only* and
    match what they did before this feature.
  - GREEN: extend `GetAccountValuation` to call the account-level
    helpers (`sumInterestForAccount`, account-level `fee` sum) and
    aggregate the per-holding fields into account totals.

## Phase 9: Closed Positions

- [ ] **TR-014 — `listEverHeldSecurities(accountID) []ID`**
  - RED: test `TestListEverHeldSecurities` — fixture with one currently-
    held security and one fully-sold security returns both IDs. Account
    with no transactions returns empty.
  - GREEN: implement in `total_return.go`. Queries distinct
    `security_id` values from share-bearing transactions
    (`buy`, `sell`, `reinvest_dividend`, `fee_liquidation`,
    `transfer_shares`).

- [ ] **TR-015 — Synthesize closed-position holdings when `IncludeClosed = true`**
  - RED: test `TestGetHoldings_IncludeClosed_AddsClosedRows` — fixture
    with one open and one closed position returns one holding by default;
    returns two holdings with `IncludeClosed = true`. The closed row
    has `Shares.IsZero()`, `IsClosed == true`, `MarketValue.IsZero()`,
    `CostBasis.IsZero()`, but `RealizedGain`, `DividendsReceived`,
    `FeesPaid`, and `TotalCostDeployed` populated.
  - Test `TestGetHoldings_IncludeClosed_NotDoubleCountingOpen` —
    securities still open are not duplicated in the closed section.
  - GREEN: when `opts.IncludeClosed == true`, after building the open
    holdings, call `listEverHeldSecurities` and emit additional
    `Holding` rows for any security ID not already in the open list.
    These synthesized rows have zero shares/cost basis/market value
    but their total-return components are filled by the same enrichment
    helper from TR-012.

- [ ] **TR-016 — Set `HasClosedPositions` advisory flag**
  - RED: test `TestAccountValuation_HasClosedPositionsFlag` — fixture
    with at least one closed position sets the flag to `true` on the
    returned valuation regardless of `IncludeClosed`. Fixture with no
    closed positions sets it `false`.
  - GREEN: compare the size of `listEverHeldSecurities` to the count of
    currently-held securities; set the flag if they differ.

## Phase 10: CLI

- [ ] **TR-017 — Add `--include-closed` flag to `investment portfolio`**
  - RED: test in `internal/cli/investment_portfolio_test.go` (or wherever
    the portfolio command lives) — invoking the command without the flag
    omits closed positions, invoking with the flag prints the "Closed
    positions" section heading. Flag parsing matches Cobra conventions.
  - GREEN: add the bool flag to the portfolio subcommand; pass
    `ValuationOptions{IncludeClosed: flag}` through.

- [ ] **TR-018 — Extend the per-holding table with total-return columns**
  - RED: test asserts the table header contains `UNREAL`, `DIV`, `REAL`,
    `FEES`, `TOTAL RETURN`, `RET %`. A holding with all components
    populated renders the expected numeric strings (use the existing
    money/percent formatters). The legacy "Gain/Loss" column is
    relabeled to `UNREAL` to match the spec.
  - GREEN: update the portfolio rendering in `internal/cli/`. New columns
    use the existing `formatMoney` / `formatPercent` helpers; add an
    `unavailable` placeholder string for `RealizedGainUnavailable` rows.

- [ ] **TR-019 — Add account totals block with total-return rows**
  - RED: test asserts the totals block printed below the holdings table
    contains the rows: `Cost basis (open)`, `Unrealized gain`,
    `Realized gain`, `Dividends received`, `Interest received`,
    `Fees paid`, `Total return`, and `Total return %`. Order matches
    the spec.
  - GREEN: extend the totals printer in the portfolio command. Skip
    `Total return %` (render `—`) when `TotalReturnPct == nil`.

- [ ] **TR-020 — Render "Closed positions" section when `--include-closed`**
  - RED: test invocation with the flag prints a `Closed positions
    (fully sold, total-return only)` heading followed by a table with
    columns `TICKER`, `REALIZED`, `DIV`, `FEES`, `TOTAL RETURN`,
    `RET %`. Each row has zero `Shares` / market value but populated
    return fields.
  - GREEN: after the open holdings table, iterate holdings where
    `IsClosed == true` and render the closed table.

- [ ] **TR-021 — Footer hint when closed positions exist but flag is off**
  - RED: test asserts that a portfolio invocation without the flag, on
    an account with `HasClosedPositions == true`, prints a single-line
    footer: `Hint: --include-closed adds N closed-position rows.`
  - GREEN: when `cmd has no flag && val.HasClosedPositions`, print the
    hint after the totals block. Single line, no other output changes.

## Phase 11: TUI

- [ ] **TR-022 — Investment register header gains a total-return summary line**
  - RED: test in `internal/tui/investment_register_test.go` (or the
    equivalent) asserts the rendered header for an investment account
    includes a second line with `Unrealized`, `Realized`, `Div`, `Int`,
    `Fees`, and `Total return` values formatted as in the spec.
  - GREEN: extend the investment register header rendering to call
    `GetAccountValuation` and format the new fields. Re-use the
    existing money formatter.

- [ ] **TR-023 — Dashboard per-account card gains a `TR` row**
  - RED: test that the dashboard valuation card for an investment
    account renders an extra `TR` line below the existing `Total` row
    with formatted `TotalReturn` and `TotalReturnPct`.
  - GREEN: extend the dashboard card rendering. Non-investment accounts
    are unaffected.

- [ ] **TR-024 — `View → Show closed positions` toggle**
  - RED: test in `internal/tui/menubar_test.go` (or equivalent) that the
    `View` menu gains a "Show closed positions" item with a `✓` prefix
    when the toggle is on. Selecting the item flips the toggle and
    re-renders.
  - GREEN: add a `MenuActionToggleClosedPositions` action. Store the
    state on `App` (and persist to `cfg` so it survives a restart).
    Plumb the bool through to `ValuationOptions.IncludeClosed` when the
    investment register / dashboard requests its valuation.

- [ ] **TR-025 — Visual smoke check**
  - Manual: launch `tmoney`, open an investment account register,
    confirm the new header line appears. Toggle View → Show closed
    positions; confirm the holdings list grows to include fully-sold
    tickers in a muted style and the dashboard card updates. Verify the
    toggle persists across restart.

## Phase 12: Documentation

- [ ] **TR-026 — README updates**
  - GREEN: extend the **Investment** section in `README.md` with:
    - A short paragraph on total return composition (unrealized,
      realized, dividends, interest, fees).
    - `investment portfolio` output now includes total-return columns
      and an account totals block.
    - `--include-closed` flag and a one-line example.
    - The TUI `View → Show closed positions` toggle.
  - No new top-level section; this slots into the existing **Investment**
    block.

- [ ] **TR-027 — Mark spec as implemented**
  - GREEN: add a status note at the top of
    `specs/investment-total-return.md` indicating the v1 feature is
    implemented, pointing to the XIRR/TWR follow-up as the next batch.

---

## Out of Scope

The following are explicitly deferred — not in this implementation plan:

- **Money-weighted return (XIRR)** at the account and per-security
  level over a user-selected date range.
- **Time-weighted return (TWR)** comparable across portfolios with
  different deposit/withdraw timings.
- **Realized gain in non-lot accounts with corporate actions** — would
  require an action-aware ledger replay or persisting realized gain on
  each sell at sell time.
- **Return-of-capital action type** — currently not modeled; would
  reduce cost basis without being a sale and subtract from
  `total_cost_deployed`.
- **1099-B-style realized gains report** with short vs. long term
  breakouts, exportable as CSV for tax prep.
- **CSV export** of the extended portfolio output. The existing
  `--csv` flag (if present) will pick up the new columns automatically
  if it iterates struct fields; otherwise a separate session can wire
  it explicitly. Treated as follow-up, not blocking.
