# Implementation Plan: Spin-Off Recording, Lot-Tracking Backfill, and Historical Price Lookup

> **Status: planned (not implemented).** This plan was developed by walking the
> full design tree for a real-world need: recording the July 2024 Grayscale
> "Mini Trust" spin-offs (GBTC→BTC, ETHE→ETH) in a Wealthfront IRA, and tracking
> their impact on returns. The investigation surfaced that the *real* blocker is
> not a spin-off entry point but **the inability to retroactively lot-track an
> account with existing history** — without lots, realized gain through a
> corporate action plus the subsequent sale of the spun-off shares cannot be
> computed. This plan therefore covers three interlocking features.

## 1. Motivation and the real-world scenario

A Wealthfront IRA received two Grayscale Mini Trust spin-offs in July 2024
(from the statement's "Other Transactions / Spin-off" section):

| Date | Type | Resulting Symbol | Resulting Shares | Resulting Cash |
|------|------|------------------|------------------|----------------|
| 2024-07-23 | Spin-off | ETH | 138 | — |
| 2024-07-31 | Spin-off | BTC | 227 | — |

The parent positions (GBTC, ETHE) were **not** reduced in share count
("Shares Affected: —"), and **no cash** changed hands. The economic event:
Grayscale contributed ~10% of each parent trust's crypto to a new Mini Trust
and distributed Mini Trust shares ~1-for-1 to existing holders. The user wants
to (a) record these in the investment register and (b) see the correct impact
on returns, including the later sale of the spun-off shares.

### What's confirmed from the actual ledger (`~/Documents/TMoney/haskovec.tdb`, read-only)

- **Account "Wealthfront IRA"** — `type=investment`, **`track_lots=false`**.
- **Holdings at the spin-off dates (net of all buys/sells), confirmed by replay:**
  - **GBTC: 227 shares** (1572 bought − 1345 sold) → 227 BTC ⇒ **ratio 1.0**.
  - **ETHE: 138 shares** (1983 bought − 1845 sold) → 138 ETH ⇒ **ratio 1.0**.
  - Last GBTC/ETHE activity (2024-03 / 2024-07-05) predates both spin dates.
- **GBTC and ETHE are held only in the IRA** (62 / 88 txns; no other account) ⇒
  the spin-off's "applies to all holding accounts" behavior affects only the IRA.
- **BTC and ETH securities already exist** in the security master
  (`Grayscale Bitcoin Mini Trust ETF`, `Grayscale Ethereum Mini Trust ETF`),
  type `etf`, not hidden. No BTC/ETH transactions exist yet.
- **IRA transaction-type mix:** buy (394), dividend (236), sell (191), fee (54),
  interest (54), `transfer_cash` (3), **`transfer_shares` (3)**, deposit (2).
  No `reinvest_dividend`, no `fee_liquidation`.
- **The 3 `transfer_shares` are inbound from the E\*Trade Rollover IRA**
  (NEM 200 @ 2020-01-02, GLD 200 / AEM 100 @ 2021-03-22), each carrying a
  `total_amount` cost basis (price_per_share NULL). All three were later fully
  sold (closed positions).
- **One corporate action already exists in the file: an SCHB 2-for-1 split
  (2022-03-11)** — but **SCHB is held in the *Wealthfront Brokerage*, not the
  IRA.** None of the IRA's 14 securities (AEM, EMB, ETHE, GBTC, GLD, IBIT, IEMG,
  LQD, NEM, SCHF, VEA, VNQ, VTI, VWO) has any corporate action.

The SCHB fact is decisive: the lot-backfill **must not** be gated on a *global*
"any corporate action exists" check, because that pre-existing split — in an
unrelated account — would otherwise block the IRA from ever being lot-tracked.

## 2. Design decisions (with rationale)

These were resolved one-by-one; this section is the record of *why*.

1. **Cost-basis model: allocate (carve-out), not zero-basis.** The user wants
   taxable-account-correct behavior for the future. The existing engine already
   does this: parent keeps X% of basis, child gets (100−X)%, the two sum to the
   original (running total preserved), and the child lot **inherits the parent
   lot's purchase date** for holding-period correctness. *In an IRA basis is
   tax-irrelevant and the aggregate total return is identical to zero-basis;
   allocation only refines the per-security split.*
2. **Allocation % source: Grayscale's published factors** (IRS Form 8937),
   entered as a plain "parent keeps X%" field. No price-derivation UI needed.
3. **Engine: reuse the existing global corporate-action spin-off** rather than a
   new per-account action — it is already lot-correct and holding-period-correct.
4. **Share input: resulting shares**, taken straight from the statement
   (227 / 138). The dialog derives the ratio from the launching account's parent
   holding and previews the effect on any other holding accounts.
5. **Child security pre-exists** (BTC/ETH already added). Inline-create is a
   future nicety, explicitly out of scope.
6. **Realized gain requires lots.** The user's spun-off shares were *sold* by
   Wealthfront, so realized gain on BTC/ETH is a real (non-zero) number that the
   non-lot replay cannot reconstruct once a corporate action exists
   (`total_return.go:222`). Lot tracking — which is automatic once enabled,
   mirroring how Wealthfront tracks internally — is the answer.
7. **Lot tracking rollout:** build `investment enable-lots` (+ `--all`) and
   **default new investment/HSA accounts to lot-tracked**; keep the non-lot
   average-cost path intact (a safety valve for lenient/out-of-order entry).
8. **Backfill sell-allocation: FIFO default, with `--method fifo|lifo|hifo`.**
   Will not reproduce Wealthfront's actual lot selection (that data isn't on any
   statement); individual sells can be re-allocated later via the sell dialog's
   `--lot`.
9. **Reversal: build it.** Add real spin-off unwind exposed as Delete in the
   Corporate Action History view; when the child shares were already sold,
   **refuse and list the blocking transactions** (never cascade-delete).
10. **Historical price lookup: build it broadly.** New `FetchQuoteOn(ticker,date)`
    provider method; a Lookup button on the spin-off dialog *and* the manual
    Price add/edit dialog, plus a `price lookup` / `price add --fetch` CLI.

## 3. Current implementation (baseline) — file references

- **Spin-off engine (global, security-master):** `internal/investment/corporate_action_service.go:312`
  (`SpinOff`), `:364` (`spinOffProcessLots`), `:426` (`spinOffProcessPositions`).
  Params + validation: `internal/investment/corporate_action.go:271`
  (`SpinOffParams`), `:283` (rejects `parent_allocation_pct <= 0 || >= 100`),
  `:291` (`SpinOffAllocationPct`). Audit row + `exchange`/`deposit` txns + child
  price seed are written per the spin-off. **Spin-off is not reversible today**
  (`DeleteAction` → `UnsupportedReversalError`).
- **TUI entry points:** Securities view `o` key — `internal/tui/security_view.go:319`;
  menu — `internal/tui/app_menu.go:131`; dialog — `internal/tui/corporate_action_spinoff_dialog.go`
  (fields: Parent, Spin-Off, Date, Share Ratio, Parent Allocation %, Spin-Off Price).
  Register type-selector — `internal/tui/investment_register_view.go:449`
  (`investmentTransactionTypeOptions`), `:465` (`...FromIndex`), `:508`
  (`openInvestmentTypeSelector`), `:539` (`handleInvestmentTypeSelectorKey`).
  Corporate Action History view — `internal/tui/corporate_action_history.go`.
- **Rebuild / heal:** `internal/investment/rebuild.go:29` (`RebuildPositions`),
  `:40` (**global `CountAll` refusal**), `:95-102` (lot rebuild recomputes
  *existing* lots only; **returns early when `len(lots)==0` — never synthesizes**),
  `:243` (`replayPosition`), `:139` (`HealAllAccounts`, runs on every launch/CLI).
- **Realized gain / valuation:** `internal/investment/total_return.go:217`
  (`realizedGain`; **per-security** corporate-action scope via `ListBySecurity`
  at `:222`), `:237` (`realizedGainNonLot`/`replayRealizedGain`).
  `internal/investment/valuation_service.go:38` (`GetAccountValuation`; total
  return is still computed with unavailable-realized counted as $0, `:66`,`:119`).
- **Lots:** `internal/investment/lot.go:11` (struct), `:72` (`Validate` — **requires
  positive `cost_per_share`**, relevant to future zero-basis work), `investment_service.go:226`
  (Buy creates a lot iff `TrackLots`), `:1284` (TransferShares creates dest lots).
- **Account lot flag:** `internal/account/account.go:167` (`TrackLots`), `:170`
  (`NewAccount` defaults false). **No CLI flag** (`internal/cli/account/add.go`)
  and **no TUI field** (`internal/tui/account_dialog.go`) set it; the repository
  `Update` does persist it (`internal/account/account_repository.go:215`).
- **Price provider:** `internal/price/provider.go:20` (`FetchQuote(ticker)` only —
  "latest closed-session"); `internal/price/yahoo_provider.go:72`
  (`range=5d&interval=1d`), `:156` (`pickClosedBar` keeps only the newest bar —
  the rest of the returned series is discarded); `internal/price/refresh.go:122`
  (`RefreshPrices`, as-of `today`); `internal/cli/price/update.go`.

## 4. Feature A — Lot-tracking enablement + historical backfill *(prerequisite)*

### A1. Enabling the flag

- **New accounts default to lot-tracked.** Set `TrackLots=true` in `NewAccount`
  for `investment`/`hsa` types (or at the dialog/CLI default). Add a
  `--track-lots` flag to `account add` and a checkbox to the New Account dialog
  (default on), so it can be turned off deliberately.
- **Existing accounts** are enabled via the backfill command below — *not* by a
  bare edit flag (a heavy, near-irreversible backfill must not hang off an
  innocuous edit toggle). `account edit` may expose a read-only indicator but
  should not flip the flag without running the backfill.

### A2. `investment enable-lots`

```
tmoney -f file.tdb investment enable-lots --account "Wealthfront IRA"
tmoney -f file.tdb investment enable-lots --all
tmoney -f file.tdb investment enable-lots --account X --method fifo|lifo|hifo   # default fifo
```

Behavior:
1. **Pre-flight.** Refuse if the target account is non-investment, or if it
   **already has lots** (don't double-create). Print the planned summary and
   require confirmation (or `--confirm`). Recommend `db backup` first.
2. **Per-account corporate-action gate (the key change).** Replace the global
   `CountAll` guard with a **per-account/per-security** check: for each security
   *held in the target account*, if `corporateActionRepo.ListBySecurity(secID)`
   is non-empty, that account cannot be cleanly backfilled by the naive replay
   (it would need the action-aware replay — see Follow-ups). Mirror the existing
   realized-gain scoping (`total_return.go:222`). **For the Wealthfront IRA this
   set is empty**, so the backfill proceeds; the unrelated SCHB split in the
   Brokerage no longer blocks it.
3. **Set `TrackLots=true`** on the account.
4. **Backfill via chronological per-security replay** (extends `replayPosition`
   machinery; lives beside it in `rebuild.go`, reused by `enable-lots` but **not**
   wired into `HealAllAccounts`, which stays recompute-only):
   - Group the account's `investment_transactions` by `security_id`, sort by
     `(date asc, created_at asc)`.
   - `buy` → open a lot: `shares`, `cost_per_share` consistent with live `Buy`
     (net of commission, per `computation.go`), `purchase_date = txn.date`,
     `source_transaction_id = txn.id`.
   - `reinvest_dividend` → open a lot (same as buy). *(none in this account)*
   - `transfer_shares` **in** → open a lot from the row's carried
     `total_amount` (`cost_per_share = total_amount / shares`),
     `purchase_date = txn.date`. *(NEM/GLD/AEM in this account.)*
   - `sell`, `fee_liquidation`, `transfer_shares` **out** → consume open lots by
     the chosen `--method` (default FIFO) and write `investment_transaction_lots`
     junction rows (the rows realized-gain math reads). *(no fee_liquidation /
     no outbound transfer_shares in this account.)*
   - `dividend`, `interest`, `fee`, `deposit`, `withdrawal`, `transfer_cash` →
     no share effect; skipped for lot purposes.
   - **Insufficient-shares handling:** if a sell can't be fully covered by open
     lots (out-of-order/imported data), do not crash the whole run — record the
     shortfall, skip that consumption, and include it in the summary so the user
     can fix it. (This account replays cleanly; the guard is for robustness.)
5. **Summary output:** lots created per security, sells matched, any shortfalls,
   and a reminder that realized gain on past sells is now lot-exact (FIFO).

### A3. Consequences / notes

- Once `TrackLots=true`, **all** of the account's securities route through the
  lot path for realized gain (`total_return.go:218`). The backfill therefore
  must create lots for **every** security (it does), or those securities' lot
  realized gain would read empty.
- The **Wealthfront Brokerage** (SCHB split) is the harder case: a per-security
  gate would skip SCHB, but the account-level flag would then route SCHB through
  the empty lot path. Lot-tracking that account correctly needs the **action-aware
  replay** (Follow-ups). Out of scope here; the IRA does not need it.
- **Transferred-in lots use the transfer date as purchase date** (the original
  acquisition date isn't available across a non-lot transfer). Holding-period is
  irrelevant in an IRA; flag this as a known limitation for taxable accounts.
- **No dependency on the source (E\*Trade Rollover IRA) account.** The IRA's
  inbound `transfer_shares` rows carry their own basis in `total_amount`
  (NEM $9,623 / GLD $14,939.44 / AEM $2,399), so the backfill seeds those lots
  directly without consulting the source account's lots. Lot-tracking E\*Trade is
  **not required** for the Wealthfront IRA to be correct (and wouldn't change its
  transferred-in lots — a transfer carries only an aggregate basis, not per-lot
  detail). Enabling lots on E\*Trade is optional and only affects *its own*
  returns; it holds no corporate-action securities, so it would backfill cleanly
  via `enable-lots` whenever desired.

## 5. Feature B — Spin-off recording from the register

- **New register affordance.** Add **"Spin-Off…"** to the investment register
  type-selector (`investmentTransactionTypeOptions` / `...FromIndex` /
  `handleInvestmentTypeSelectorKey`, `investment_register_view.go:449/465/539`).
  Launching it pre-fills **parent = the security on the selected register row**
  and the **current account**. Keep the existing Securities-view (`o`) and menu
  entry points — same engine, multiple doors.
- **Dialog (adapt `corporate_action_spinoff_dialog.go`):** Parent (pre-filled),
  Spin-Off security (existing-securities picker), Date, **Resulting shares**
  (statement value), **Parent keeps %** (Grayscale factor), **Child price**
  (with a Lookup button — Feature D).
  - **Resulting shares → ratio:** derive `ratio = resulting_shares ÷
    parent_shares_held_in_launching_account`; show the implied ratio and a
    **preview of resulting shares for any other holding accounts** before
    confirm. (For this user the preview is empty — GBTC/ETHE are IRA-only.)
  - Child price does **not** affect basis (allocation does); it seeds the child
    price-history row and would drive fractional cash-in-lieu (none here, whole
    shares).
- **Engine unchanged** (`SpinOff`): carves parent basis by the %, creates child
  lots with the carved basis and inherited purchase date. Because the IRA will
  be lot-tracked (Feature A) *before* this runs, the spin-off operates on real
  lots and realized gain stays exact through the later BTC/ETH sale.

## 6. Feature C — Spin-off reversal

- Implement a real unwind in the corporate-action service (replace the
  `UnsupportedReversalError` for `ActionTypeSpinOff`): delete the created child
  lots/positions and the `exchange`/`deposit` txns, remove the seeded child
  price row, and restore the parent lots' `cost_per_share`.
- Expose as **Delete** in the Corporate Action History view
  (`corporate_action_history.go`).
- **Downstream-dependency guard:** if any child-security lot has been consumed
  (e.g., the BTC/ETH sale), **refuse and enumerate the blocking transactions**
  ("BTC sale on YYYY-MM-DD consumes 227 spun-off shares — delete it first").
  Never cascade-delete user-entered transactions.

## 7. Feature D — Historical price lookup

- **Provider:** add `FetchQuoteOn(ticker string, date types.Date) (*Quote, error)`
  to the `Provider` interface (`price/provider.go`). Yahoo implementation
  (`yahoo_provider.go`): request the chart endpoint with `period1`/`period2`
  bracketing the date instead of `range=5d`, and pick the close **on or before**
  the requested date (handles weekends/holidays). The endpoint already returns
  the series; this stops discarding it.
- **UI/CLI surfaces:**
  - Lookup button on the spin-off dialog's Child price field.
  - Lookup button on the manual Price add/edit dialog.
  - CLI `tmoney price lookup --ticker X --date YYYY-MM-DD` (and/or
    `price add --ticker X --date D --fetch`).
- **Ticker-ambiguity guard:** `BTC`/`ETH` resolve to the Grayscale ETFs on Yahoo,
  but spot crypto is `BTC-USD`/`ETH-USD`. Always surface the fetched **value +
  date** in the UI so the user can sanity-check (~$5–6 ETF close, not a ~$60k+
  crypto print). On miss/error, leave the field editable with a toast; manual
  entry is always the fallback.

## 8. Order of operations for this user's backfill

1. `tmoney -f haskovec.tdb db backup`
2. `tmoney -f haskovec.tdb investment enable-lots --account "Wealthfront IRA" --confirm`
   → builds lots for all 14 securities from history (FIFO); the SCHB split (in
   the Brokerage) no longer blocks it once the gate is per-account.
3. Record **ETHE→ETH** (2024-07-23) then **GBTC→BTC** (2024-07-31): **parent-keep
   = 90%** (§9), resulting shares 138 / 227, child price via Lookup.
4. Record the **BTC/ETH sale(s)** Wealthfront made → realized gain computed
   against the child lots (FIFO). _(The 2024-11-19 1-for-5 reverse split does
   **not** apply — the shares were sold before then, so record at the
   as-distributed **227 BTC / 138 ETH** counts.)_
5. Verify in `investment portfolio --account "Wealthfront IRA" --include-closed`.

## 9. Cost-basis factsheet (Grayscale Mini Trust spin-offs)

**Sources:** Grayscale SEC filings (8-K Exhibit 99.1, Schedule 14C) + price
vendors (Yahoo Finance, StockAnalysis.com). This is an IRA, so basis allocation
is **informational only** (no gain/loss recognition, no 1099-B). The 1-for-1
ratios are independently confirmed by the ledger (227 GBTC→227 BTC; 138 ETHE→138
ETH).

### GBTC → BTC (Grayscale Bitcoin Mini Trust)

| Item | Value | Confidence |
|---|---|---|
| Distribution date | **2024-07-31** (record 2024-07-30) | High |
| Share ratio | **1-for-1** | High |
| Cost-basis split | **parent keeps 90% / child 10%** (quantity-based — 10% of the bitcoin was contributed — not FMV) | High |
| GBTC close 2024-07-31 | **$52.07** | High |
| BTC first-day close 2024-07-31 (as-traded) | **~$5.84** (Yahoo's split-adjusted figure shows $28.95) | High |

### ETHE → ETH (Grayscale Ethereum Mini Trust)

| Item | Value | Confidence |
|---|---|---|
| Distribution date | **2024-07-23** (record 2024-07-18) | High |
| Share ratio | **1-for-1** | High |
| Cost-basis split | **parent keeps 90% / child 10%** (quantity-based) | High |
| ETHE close 2024-07-23 | **$29.35** | High |
| ETH first-day close 2024-07-23 | **~$32** (Yahoo $32.70 vs StockAnalysis $32.21) | **Medium** |
| ETH close 2024-07-24 | **$31.70** | High |

**For both spin-offs, enter parent-keep = 90%.**

> ⚠️ **Reverse split — affects your share counts.** BTC and ETH both underwent a
> **1-for-5 reverse split effective 2024-11-19**. The statement's 227 BTC / 138
> ETH are the *as-distributed* (pre-reverse-split) counts. If the shares were
> **held past 2024-11-19**, the position became ~45.4 BTC / ~27.6 ETH and the
> reverse split must be recorded (`investment split --ratio 1:5`) before the
> sale. If Wealthfront **sold them before 2024-11-19**, the reverse split is
> moot. **Resolved: the shares were sold before 2024-11-19, so the reverse split
> does not apply — record at the as-distributed 227 BTC / 138 ETH counts.**

**Verify against your Wealthfront documents:** the exact per-lot basis applied
(including the small Sponsor's-Fee adjustment to the 90% figure), the ETH 7/23
first-day close (vendors disagree — use the 7/24 $31.70 as a cross-check), and
whether 227 / 138 are pre- or post-reverse-split.

## 10. Test plan

- **Backfill replay** (`rebuild_test.go` / new): buy→lot; sell→FIFO consume +
  junctions; `transfer_shares`-in→lot from `total_amount`; FIFO vs LIFO vs HIFO;
  insufficient-shares shortfall reported not fatal; per-account gate refuses an
  account holding a corporate-action security but allows one that doesn't (the
  SCHB-in-Brokerage scenario); `HealAllAccounts` still never synthesizes lots.
- **enable-lots CLI:** refuses on non-investment / already-lotted; `--all`;
  `--method`; summary correctness.
- **Spin-off register entry:** resulting-shares→ratio derivation; other-account
  preview; reuse of the engine produces carved basis + inherited purchase date;
  realized gain exact after a subsequent child sale in a lot-tracked account.
- **Reversal:** clean unwind restores parent basis and removes child rows;
  refuses with an itemized list when child shares were sold.
- **Historical lookup:** `FetchQuoteOn` returns the on-or-before close;
  weekend/holiday fallback; ambiguous-ticker value surfaced; CLI + both dialogs.

## 11. Risks, open items, and follow-ups

- **Action-aware backfill replay (follow-up).** Needed to lot-track accounts that
  *already* contain corporate actions (e.g., the Wealthfront Brokerage / SCHB
  split) — the replay would interleave splits/mergers/spin-offs chronologically
  and apply each transform. Also the proper fix for non-lot realized gain through
  corporate actions (already filed in `specs/investment-total-return.md`
  Follow-up). Out of scope; the IRA does not require it.
- **Zero-basis spin-off (follow-up).** Not needed here (allocation chosen), but if
  ever supported it requires relaxing both `SpinOffParams.Validate`
  (`corporate_action.go:283`, allow 100%) **and** `Lot.Validate`
  (`lot.go:72`, allow `cost_per_share == 0`).
- **Transferred-in lot purchase dates** use the transfer date (acquisition date
  unavailable cross-account) — cosmetic in an IRA, a holding-period caveat in a
  taxable account.
- **Inline child-security creation** in the spin-off dialog — deferred.
- **FIFO ≠ broker lot selection.** Backfilled realized gains won't match
  Wealthfront's actual per-sale lot picks; re-allocate individual sells with the
  sell dialog's `--lot` if precision is ever needed (irrelevant in the IRA).
- **README / `specs/cli.md` updates** for `enable-lots`, `price lookup`,
  `account add --track-lots`, the register Spin-Off entry, and spin-off Delete —
  drafted in §12 below.

## 12. Documentation updates (draft — apply on implementation)

> Staged here, **not** applied to the live docs (the features don't exist yet).
> Drop each block into the named location when the corresponding feature lands
> (task #13). Each draft was written to match the surrounding doc's existing
> style.

### README.md

**Investment (CLI Reference) — new `investment enable-lots` subsection.**
*Placement: immediately after the `investment rebuild-positions` block, before `investment portfolio`.*

````md
```bash
# Preview enabling lot tracking on an existing account (no changes made)
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA"

# Execute the backfill once the previewed plan looks right
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" --confirm

# Enable lots on every investment/HSA account at once
tmoney -f personal.tdb investment enable-lots --all --confirm

# Choose which lots historical sells consume (default fifo)
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" \
  --method hifo --confirm
```

`investment enable-lots` turns on lot tracking for an **existing** investment or
HSA account and backfills its lots by replaying the full transaction ledger in
chronological order. Each `buy` and `reinvest_dividend` opens a lot; an inbound
`transfer_shares` opens a lot from the cost basis carried on that row's
`total_amount`; and each `sell`, `fee_liquidation`, and outbound
`transfer_shares` consumes open lots. Cash-only types (`dividend`, `interest`,
`fee`, `deposit`, `withdrawal`, `transfer_cash`) have no share effect and are
skipped. `--method fifo|lifo|hifo` (default `fifo`) controls which open lots the
historical sells draw down. Supply either `--account NAME` or `--all` (every
investment/HSA account). By default the command prints the plan — the lots to
create per security, the sells matched against them, and any insufficient-share
shortfalls — and makes no changes; pass `--confirm` to execute. **Run `tmoney db
backup` first**, since the backfill writes lots and lot junction records across
the account's entire history.

`investment enable-lots` refuses to run when the target account is not an
`investment` or `hsa` account, when the account **already has lots** (re-running
would double-create them), or when the account holds a security that already has
a corporate action (split, merger, or spin-off). A naive ledger replay cannot
reconstruct lots across a corporate action — those accounts need the future
action-aware replay — so the command stops and names the blocking security
rather than producing an incorrect cost basis.
````

**Prices (CLI Reference) — new examples** *(inside the `### Prices` bash block, after `price import … --overwrite`):*

````md
# Look up the provider's closing price on or before a date (prints, does not store)
tmoney -f personal.tdb price lookup --ticker AAPL --date 2024-01-15
tmoney -f personal.tdb price lookup --ticker AAPL --date 2024-01-15 --provider yahoo

# Record a price by fetching it from the provider instead of passing --price
tmoney -f personal.tdb price add --ticker AAPL --date 2024-01-15 --fetch
````

**Prices (CLI Reference) — new prose** *(after the `price add` paragraph, before `price list`):*

````md
`price add --fetch` fetches the closing price for `--date` from the provider
(Yahoo by default; override with `--provider`) instead of requiring `--price`,
and stores it with `source = api`. Exactly one of `--price` or `--fetch` is
given. The provider returns the close *on or before* `--date`, so a weekend or
holiday date resolves to the prior trading day.

`price lookup --ticker X --date YYYY-MM-DD` fetches and prints the provider's
closing price on or before that date — it does **not** store anything. It prints
the resolved value and date so you can copy it into `price add` or sanity-check
it before recording. `--provider` defaults to `yahoo`.

> **Sanity-check the ticker.** Some symbols are ambiguous on the provider. On
> Yahoo, `BTC` and `ETH` are the Grayscale Bitcoin/Ethereum Mini Trust ETFs
> (closing prices around $5–$6), while spot crypto trades under `BTC-USD` /
> `ETH-USD` (tens of thousands of dollars). Because `price lookup` prints the
> fetched value and date, verify both before using the number — a ~$60k print
> for a `BTC` ETF row means you fetched the wrong instrument.
````

**Features → Prices bullet** *(replace the "Bulk refresh…" bullet):*

````md
- Bulk refresh from an online provider (Yahoo Finance by default) stores the latest closed-session price, so reruns on the same day are idempotent; historical closes can also be fetched for a specific date via `price lookup` or `price add --fetch`
````

**Accounts (CLI Reference) — new examples** *(inside the `### Accounts` bash block, after the `account add …` lines):*

````md
# Create a lot-tracked investment account (the default for investment/HSA)
tmoney account add --name "Brokerage" --type investment

# Opt a new investment/HSA account out of lot tracking
tmoney account add --name "401k" --type investment --track-lots=false
````

**Accounts (CLI Reference) — new prose** *(after the bash block, before the "Account types:" line):*

````md
New `investment` and `hsa` accounts are **lot-tracked by default** — each buy
opens a lot and sells are allocated against open lots for exact cost-basis and
realized-gain tracking. Pass `--track-lots=false` to `account add` to opt a new
account out (it then uses the average-cost path); `--track-lots` (or
`--track-lots=true`) is the explicit way to force it on. The flag is ignored for
non-investment account types. To enable lot tracking on an **existing**
investment/HSA account — which backfills lots from its transaction history — use
[`investment enable-lots`](#investment) rather than `account edit`; the edit
command does not flip this flag.
````

**Features → Accounts — new bullet** *(after the "Interest rate (APR)…" bullet)* **and edit the HSA bullet** (`with optional lot tracking` → `with lot tracking on by default`):

````md
- Investment and HSA accounts are lot-tracked by default (override with
  `account add --track-lots=false`, or a "Track lots" checkbox on the New
  Account dialog); existing accounts gain lot tracking via
  `investment enable-lots`, which backfills lots from history
````

**TUI → Register table** *(replace the `| `n` | New transaction |` row):*

````md
| `n` | New transaction (investment accounts open a type selector — see below) |
````

**TUI → Register — new prose** *(after the Register table):*

````md
In an investment account, `n` opens a transaction-type selector (Buy, Sell,
Dividend, …) that now includes **Spin-Off…**. Choosing it launches the spin-off
dialog with the **parent** pre-filled to the security on the selected register
row and the **current account** as the launching account — a convenience door
onto the same engine as the Securities-view `o` shortcut, which remains
available.
````

**TUI → new "Corporate Action History" subsection** *(between `#### Securities` and `#### Prices`):*

````md
#### Corporate Action History

Reached via the Securities-view `a` shortcut or **Securities → Corporate Action History…** (`Alt+S`).

| Key | Action |
|-----|--------|
| `Enter` | View details for the selected corporate action |
| `d` | Delete / reverse the selected corporate action |

Spin-offs are reversible: `d` unwinds the action — removing the spun-off child
lots, positions, and generated transactions, and restoring the parent lots'
cost basis. If the child shares have already been sold, the reversal **refuses**
and lists the blocking transactions (for example, `BTC sale on 2024-08-15
consumes 227 spun-off shares — delete it first`); nothing is cascade-deleted.
````

### specs/cli.md

**New `investment enable-lots`** *(in the `investment` noun, alphabetically between `dividend` and `fee`):*

````md
### `investment enable-lots`

`Use: investment enable-lots` · `Args: NoArgs`

Enable lot tracking on an existing investment (or HSA) account and backfill its lots from the transaction ledger. Buys, reinvested dividends, and inbound share transfers open lots; sells, liquidating fees, and outbound transfers consume open lots by the chosen method, writing the junction rows that realized-gain math reads. Once enabled, every security in the account routes through the lot path. This is a heavy, near-irreversible operation — run `db backup` first.

By default the command prints the planned summary and makes no changes; pass `--confirm` to execute the backfill.

**Optional flags:**
- `--account string` — Limit to a single investment account by name
- `--all` — Enable lots on every investment/HSA account that isn't already lot-tracked
- `--method string` — Sell-allocation method: `fifo`, `lifo`, or `hifo` (default `fifo`)
- `--confirm` — Execute the backfill (default is a planned-summary preview)

Exactly one of `--account` or `--all` is required.

The command refuses to run when the target account is not an investment/HSA account, or when it already has lots (lots are never double-created). It also refuses, per account, any account that holds a security with recorded corporate actions (splits, mergers, spin-offs), since the naive replay cannot reproduce those — a corporate action on a security held in an *unrelated* account does not block accounts that don't hold it. A sell that open lots can't fully cover is reported as a shortfall in the summary rather than aborting the run.

```bash
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA"
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" --confirm
tmoney -f personal.tdb investment enable-lots --account Brokerage --method hifo --confirm
tmoney -f personal.tdb investment enable-lots --all --confirm
```

The summary reports lots created per security, sells matched, any uncovered-share shortfalls, and a reminder that realized gain on past sells is now lot-exact for the chosen method.
````

**Changed `price add` + new `price lookup`** *(in the `price` noun):*

````md
### `price add`

`Use: price add` · `Args: NoArgs`

Record a price for a security on a specific date. The source is set to `manual`. Pass `--fetch` to look the price up from a provider for the given date instead of supplying it by hand.

**Required flags:** `--ticker`, `--date`, and `--price` (omit `--price` when using `--fetch`)

**Optional flags:**
- `--fetch` — Fetch the closing price for `--date` from a provider instead of passing `--price`
- `--provider string` — Price provider name (default `yahoo`; used with `--fetch`)

```bash
tmoney price add --ticker AAPL --date 2024-01-15 --price 150.00
tmoney price add --ticker AAPL --date 2024-01-15 --fetch
```

### `price lookup`

`Use: price lookup` · `Args: NoArgs`

Fetch the closing price for a security on a specific date from a provider and print it, without recording anything. The provider returns the close on or before the requested date, so weekends and holidays resolve to the prior trading day. Use this to sanity-check a value before recording it with `price add --fetch`.

**Required flags:** `--ticker`, `--date`

**Optional flags:**
- `--provider string` — Price provider name (default `yahoo`)

```bash
tmoney price lookup --ticker AAPL --date 2024-01-15
tmoney price lookup --ticker GBTC --date 2024-07-31 --provider yahoo
```
````

**Changed `account add`** *(add the `--track-lots` flag + default-on note and one example):*

````md
- `--track-lots` — Track individual tax lots (`investment`/`hsa` accounts only; default on for those types). Pass `--track-lots=false` to opt out and use the average-cost path instead.

# example line:
tmoney account add --name "Wealthfront IRA" --type investment --track-lots=false
````
