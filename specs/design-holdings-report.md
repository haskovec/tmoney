# Design: holdings report

**Date:** 2026-10-09
**Status:** APPROVED on 2026-10-09, with the choices in §3.

Every name, ticker and amount in this document is fictional. Money shows as
`types.Money.Format` writes it, with no thousands separators.

## 1. Purpose

The owner wants to see which holdings make up the largest part of all
investments. Each account view shows its own holdings, but no view adds the
same security up across accounts. A fund that is in a brokerage account, a
Roth IRA and an HSA shows as three small rows in three views, not as one
large position.

The holdings report adds every open position in every active investment
account into one row per security. Each row shows its percentage of the
total, and a bar shows its size. One Cash row holds the uninvested cash of
all the accounts, so the rows add up to 100% and the total is the
investment value in the net worth report.

A portfolio can hold a few dozen securities. That is too many slices for a
pie chart, and few enough to show every row in a table that scrolls.

## 2. Decisions

These were settled with the owner on 2026-10-09.

| # | Decision | Chosen |
|---|---|---|
| D1 | Display | A table with a bar column. No pie chart. (A kitty-graphics donut works in Ghostty, but it can be a separate feature later.) |
| D2 | Accounts | All active `investment` and `hsa_investment` accounts. This is the same set as the net worth report. Closed accounts are not included. |
| D3 | Cash | One Cash row: the sum of the cash in all the accounts. It is part of the 100%. The total is the investment value in net worth. |
| D4 | Rows | All rows. No top-N limit. The table scrolls. The column header and the TOTAL line stay on the screen. |
| D5 | Columns | Security, Name, Shares, Price, Value, % Total, Cost Basis, Gain, bar. There is no Accounts column. |
| D6 | Narrow screen | Remove Shares and Price first, then Cost Basis and Gain, then Name. Security, Value, % Total and the bar always stay. Name is at least 16 cells. The bar is at least 10 cells. |
| D7 | Gain | Unrealized gain in money only: Value − Cost Basis. No gain percentage. As in the Portfolio view, the cells in the table are plain text, and the TOTAL gain uses the positive and negative colors. |
| D8 | Enter | Enter (or a double click) on a row shows how the row splits across accounts. On the Cash row it shows the cash of each account. Esc goes back. |
| D9 | Bar | The largest row fills the full bar width. Partial cells use 1/8 blocks. The color is neutral, never red. |
| D10 | CLI | `tmoney report holdings` prints all columns. A security selector prints the split for one security (§6). The TUI and the CLI use one report struct. |
| D11 | TUI entry | Key `i` in the Reports view, and a "Holdings" item in the Reports menu. (`h` is vim Left in this view.) |

### 2.1 Rules

1. A row is one security ID. Its label is the ticker, or the name when the
   security has no ticker.
2. A hidden security that is still held is included.
3. A security with no price is valued at its cost basis, as in the rest of
   the app. Its label has a `~` prefix, and a note under the table says
   what `~` means.
4. The Cash row has no Shares, no Price and no Gain. Its Cost Basis is the
   cash.
5. There is one section per currency. Money in different currencies is
   never added.
6. The rows sort by Value, largest first. There is no other sort.
7. The report is for today only. There is no `--as-of`.
8. The bar color is neutral.
9. When an account cannot be valued, the report shows an error line for it,
   and the TOTAL of its currency shows "not available", as in net worth.
10. A click selects a row. A double click does the same as Enter.
11. The split shows Account, Shares, Value, and % of the holding, largest
    first.

## 3. Choices made while writing this spec

These points were added while the spec was written, and the owner approved
them. Each one follows a pattern that is already in the code.

| # | Point | Choice | Why |
|---|---|---|---|
| C1 | CLI selector | `--ticker`, `--isin` or `--name`, not `--security` | Every other command that picks a security uses these flags (`cmdutil.AddSecuritySelectorFlags`), and `security.Service.Resolve` names them in its errors. |
| C2 | Where `~` goes | On the Security label. Price and Gain show `N/A`. The TOTAL value gets `~` when a row has it. | The Portfolio view puts `~` on the label and shows `N/A` for the price. Net worth puts `~` on an estimated total. |
| C3 | Cash row | Label `Cash`, name `Uninvested cash`. It sorts with the other rows by value. It is not shown when the cash sum is zero. A negative sum shows as a negative value with an empty bar. | Rule 6 says one sort. A zero row adds nothing. |
| C4 | Name and bar width | Name and bar share the spare width equally. Name stops growing at the longest name. The bar is at most 40 cells. | The bar is the main point of the report, and a name longer than its text wastes cells. |
| C5 | Split display | The split replaces the table in the Reports view, like the lot detail in the Portfolio view. It is not a pop-up dialog. | One pattern for drill-down. The split table can scroll and takes mouse input like any table. |
| C6 | More than one currency | One table. The rows of each currency are together, in currency-code order. % Total and the bar are relative to the row's own currency. There is one TOTAL line per currency under the table. | No new key to change currency. A file with one currency (the normal case) looks as in §7.2. |
| C7 | % when an account fails | % Total is of the accounts that were valued. The error line says that the percentages do not include the account. | A partial total is still useful to read, but it must be marked. |
| C8 | Cash split in the CLI | Not available. The CLI selector picks a security, and Cash is not a security. | The TUI shows it. A `--cash` flag can come later. |

## 4. Data model

### 4.1 The valuer port

`report.InvestmentValuer` returns the totals of one account today. The
report needs each position too. `ValuationResult` gets one more field:

```go
type ValuationResult struct {
	TotalValue       types.Money
	CashBalance      types.Money // the investment ledger's cash, part of TotalValue
	HasMissingPrices bool        // true if any holdings used cost basis instead of market price
	// Holdings are the account's open positions. TotalValue is CashBalance
	// plus the sum of their MarketValue.
	Holdings []HoldingFigure
}

// HoldingFigure is one open position in one account, as valued.
type HoldingFigure struct {
	SecurityID  types.ID
	Shares      types.Quantity
	Price       types.Money // zero when HasPricing is false
	MarketValue types.Money // the cost basis when HasPricing is false
	CostBasis   types.Money
	HasPricing  bool
}
```

`investmentValuerAdapter` in `internal/app/registry.go` maps each
`investment.Holding` of `GetAccountValuation(..., ValuationOptions{})` into a
`HoldingFigure`. That call returns open positions only, and an unpriced
holding already has `MarketValue = CostBasis` (`investment/holdings.go`).
Net worth does not read the new field, so its output does not change.

### 4.2 The report struct

`internal/report/report.go`:

```go
// Holdings is the holdings report: what the active investment accounts hold
// today, added up across the accounts. There is one section per currency;
// money in different currencies is never added.
type Holdings struct {
	AsOfDate types.Date
	Sections []HoldingsSection // sorted by currency code
	// Failed lists the accounts that could not be valued. Their positions
	// and cash are in no row, and their currency's section is not Available.
	Failed []AccountFailure
}

// HoldingsSection is the holdings of one currency.
type HoldingsSection struct {
	Currency  string
	Rows      []HoldingRow // largest Value first; the Cash row sorts with the others
	Value     types.Money  // the sum of the rows' Value
	CostBasis types.Money  // the sum of the rows' CostBasis
	Gain      types.Money  // the sum of the priced rows' Gain
	// Available is false when an account in this currency could not be
	// valued. Value then leaves that account out and must not be shown as
	// the total. Each row's Percent is of the accounts that were valued.
	Available bool
	// Estimated is true when a row has no price and is valued at cost.
	Estimated bool
}

// HoldingRow is one security across all the accounts, or the Cash row.
type HoldingRow struct {
	SecurityID types.ID // NilID on the Cash row
	Cash       bool
	Label      string         // the ticker, or the name when there is no ticker; "Cash"
	Name       string         // "Uninvested cash" on the Cash row
	Shares     types.Quantity // zero on the Cash row
	Price      types.Money    // zero on the Cash row and when Estimated
	Value      types.Money
	CostBasis  types.Money // the cash itself on the Cash row
	Gain       types.Money // Value − CostBasis; zero on the Cash row and when Estimated
	Percent    float64     // of the section's Value
	Estimated  bool        // no price: Value is the cost basis
	Accounts   []HoldingAccount // largest Value first
}

// HoldingAccount is one account's part of a HoldingRow.
type HoldingAccount struct {
	AccountID types.ID
	Name      string
	Shares    types.Quantity // zero on the Cash row
	Value     types.Money
	Percent   float64 // of the row's Value
}

// AccountFailure is an account that the report could not value.
type AccountFailure struct {
	AccountID types.ID
	Name      string
	Currency  string
	Err       error
}
```

The split for Enter and for the CLI selector is `HoldingRow.Accounts`. No
second query is necessary.

## 5. Service

`internal/report/holdings.go`:

```go
// Holdings returns the holdings report as of today.
func (s *Service) Holdings() (*Holdings, error)
```

Steps:

1. Read the accounts with `accountRepo.List(true)`. Keep each account where
   `Type.IsInvestmentType()` is true and `OpeningDate` is on or before
   today. This is the set that `NetWorthReport` values for today.
2. Read the label of each security with one query:
   `SELECT id, ticker, name FROM securities`. Close the rows before step 3.
   (`netWorthAsOf` explains why: the valuer runs its own queries, and an
   open result set on a one-connection pool deadlocks.)
3. For each account, call `investmentValue.GetAccountValuation(id, today)`.
   - On an error, add an `AccountFailure`, mark the section of the
     account's currency not Available, and go to the next account.
   - Add `CashBalance` to the Cash row of the account's currency, with a
     `HoldingAccount` for the account when its cash is not zero.
   - Add each `HoldingFigure` to the row of its `SecurityID` in the
     account's currency: Shares, Value and CostBasis add up, and the
     account gets a `HoldingAccount`.
4. For each section:
   - Remove the Cash row when its Value is zero.
   - `Gain = Value − CostBasis` on each priced security row.
   - The section's Value, CostBasis and Gain are the sums of its rows.
   - `Percent = row.Value / section.Value × 100`. It is 0 when the
     section's Value is zero or less.
   - Sort the rows by Value, largest first. Rows with equal Value sort by
     Label.
   - Sort each row's Accounts by Value, largest first, then by account name.
     `Percent = account.Value / row.Value × 100`, or 0 when the row's Value
     is zero.
5. Sort the sections by currency code.

When `investmentValue` is nil, every account fails with
`ErrNoInvestmentValuer`, as in `AccountFigures`.

**Invariant.** For each currency where every account was valued, the
section's Value equals the sum of the investment rows in `NetWorthReport()`
for the same file. A test pins this (§8).

**Cost.** The report values each investment account once. This is the same
work as the net worth report.

## 6. CLI

`internal/cli/report/holdings.go` registers `tmoney report holdings`.
`report.NewCmd` adds it, and its `Long` and `Example` text name it.

```bash
tmoney report holdings
tmoney report holdings --ticker ACME
tmoney report holdings --name "Cedar 2045 Target Fund"
```

| Flag | Meaning |
|---|---|
| `--ticker string` | Show how one security splits across the accounts |
| `--isin string`, `--name string` | The same, for a security that has no ticker (`cmdutil.AddSecuritySelectorFlags`) |

Give at most one of the three. With none, the command prints the full
report.

### 6.1 Full report

The output uses `text/tabwriter`, as the other report commands do. The bar
is 20 cells wide.

```
HOLDINGS REPORT
===============
As of: January 15, 2024

Security                Name                        Shares    Price    Value        % Total  Cost Basis  Gain
ACME                    Acme Total Market Index     412.5     $245.10  $101103.75   40.0%    $78400.00   $22703.75  ████████████████████
Cedar 2045 Target Fund  Cedar 2045 Target Fund      1850.221  $31.47   $58226.45    23.0%    $49900.00   $8326.45   ███████████▌
GLBX                    Globex International Index  690       $58.32   $40240.80    15.9%    $38100.00   $2140.80   ███████▉
UMBR                    Umbrella Total Bond         410       $72.15   $29581.50    11.7%    $31200.00   -$1618.50  █████▊
Cash                    Uninvested cash                                $12345.67    4.9%     $12345.67              ██▍
INIT                    Initech Corp                60        $151.20  $9072.00     3.6%     $5400.00    $3672.00   █▊
~STRK                   Stark Industries            25        N/A      $2500.00     1.0%     $2500.00    N/A        ▍
TOTAL (USD)                                                            ~$253070.17  100.0%   $217845.67  $35224.50

~ No price on file: the value is the cost basis.
```

- With more than one currency, each currency has its own table and TOTAL
  line, in currency-code order.
- The `~` note shows only when a row has `~`.
- With no active investment account, the command prints
  `No investment accounts.` and exits 0.
- When an account fails, its currency's TOTAL shows `not available`. A line
  under the table says `Percentages leave out accounts that could not be
  valued.` After the report, the command returns an error that names each
  account and its reason, so it exits non-zero, as `report net-worth` does.

### 6.2 One security

```
HOLDING: ACME (Acme Total Market Index)
=======================================
As of: January 15, 2024

Account                 Shares  Value       % of Holding
Maple Invest Brokerage  250     $61275.00   60.6%
Maple Invest Roth IRA   100     $24510.00   24.2%
Cedar HSA Investment    62.5    $15318.75   15.2%
Total                   412.5   $101103.75  100.0%

ACME is 40.0% of all holdings (~$253070.17).
```

- The heading uses `cmdutil.SecurityDisplay`.
- `Resolve` errors (not found, more than one match) are returned as they
  are.
- A security that is in no row returns the error
  `ACME is not held in an active investment account` (with
  `cmdutil.SecurityRef`).
- A security held in two currencies prints one block per currency.

## 7. TUI

### 7.1 Entry and state

- `reportTypeHoldings` is a new `reportType`. `reportsViewData` gets
  `holdings *report.Holdings`, and `load` calls `reports.Holdings()` for
  it. Holdings does not use the year, month, or transfers fields.
- `i` in the Reports view loads it. `n` and `s` work from the holdings
  report as from the other reports. `←`, `→`, `y`, `m` and `t` do nothing
  in the holdings report.
- The Reports menu gets `{Label: "Holdings", Action: MenuActionHoldings}`
  after "Spending by Category". `app_menu.go` handles it as it handles
  `MenuActionSpendingByCategory`.
- `reportsViewState` gets the holdings table, the split table (nil when the
  split is closed), and a `widget.ClickTracker` for the double click, as
  the Prices list has.
- The `ViewReports` entry in `views.go` gets a `table` func. It returns the
  split table when it is open, the holdings table when the holdings report
  is on the screen, and nil for the other reports. The mouse wheel and
  clicks then work through the code that is already there.
- `render` needs the height to size the table. The view entry passes
  `a.height` with the styles.
- The title and the separator are the only lines above the holdings table
  and above the split table. That is the base offset in
  `tableContentRowOffset`, so the mouse needs no new case there. A test pins
  a click on the first row.

### 7.2 Layout

The table width `W` is the content width minus the view's padding of 4
cells. Fixed column widths:

| Column | Width | Align |
|---|---|---|
| Security | 8 | left |
| Name | spare (see below), at least 16 | left |
| Shares | 12 | right |
| Price | 10 | right |
| Value | 13 | right |
| % Total | 7 | right |
| Cost Basis | 13 | right |
| Gain | 12 | right |
| bar (no header) | spare (see below), 10 to 40 | left |

Which columns show:

| `W` | Columns |
|---|---|
| 109 or more | all nine |
| 85 to 108 | no Shares, no Price |
| 58 to 84 | also no Cost Basis, no Gain |
| less than 58 | also no Name |

`spare` is `W`, minus one separator cell between each two columns, minus the
fixed widths. `Name = max(16, min(longest name, spare / 2))`, and
`bar = min(spare − Name, 40)`. With no Name column, `bar = min(spare, 40)`.
Below 41 cells the bar gets narrower and the table cuts the right side, as
other tables do.

One pure function, `holdingsColumns(width, longestName int)`, returns the
columns. The view sets all widths as fixed widths, so it knows
the bar width when it builds the rows. The TOTAL line uses the same widths,
so it lines up with the table.

At `W` = 136:

```
HOLDINGS                                                                                                             As of: Jan 15, 2024
════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
Security Name                             Shares      Price         Value % Total    Cost Basis         Gain
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
ACME     Acme Total Market Index           412.5    $245.10    $101103.75   40.0%     $78400.00    $22703.75 ███████████████████████████
Cedar 2… Cedar 2045 Target Fund         1850.221     $31.47     $58226.45   23.0%     $49900.00     $8326.45 ███████████████▌
GLBX     Globex International Index          690     $58.32     $40240.80   15.9%     $38100.00     $2140.80 ██████████▋
UMBR     Umbrella Total Bond                 410     $72.15     $29581.50   11.7%     $31200.00    -$1618.50 ███████▉
Cash     Uninvested cash                                        $12345.67    4.9%     $12345.67              ███▎
INIT     Initech Corp                         60    $151.20      $9072.00    3.6%      $5400.00     $3672.00 ██▍
~STRK    Stark Industries                     25        N/A      $2500.00    1.0%      $2500.00          N/A ▋
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
TOTAL                                                         ~$253070.17  100.0%    $217845.67    $35224.50
~ No price on file: the value is the cost basis.

  enter accounts  n net worth  s spending  i holdings  esc back
```

At `W` = 64:

```
HOLDINGS                                     As of: Jan 15, 2024
════════════════════════════════════════════════════════════════
Security Name                     Value % Total
────────────────────────────────────────────────────────────────
ACME     Acme Total Mark…    $101103.75   40.0% ████████████████
Cedar 2… Cedar 2045 Targ…     $58226.45   23.0% █████████▏
GLBX     Globex Internat…     $40240.80   15.9% ██████▎
UMBR     Umbrella Total …     $29581.50   11.7% ████▋
Cash     Uninvested cash      $12345.67    4.9% █▉
INIT     Initech Corp          $9072.00    3.6% █▍
~STRK    Stark Industries      $2500.00    1.0% ▍
────────────────────────────────────────────────────────────────
TOTAL                       ~$253070.17  100.0%
~ No price on file: the value is the cost basis.

  enter accounts  n net worth  s spending  i holdings  esc back
```

- The title row, the separator, the column header, the TOTAL lines and the
  notes are always on the screen. Only the rows scroll.
- With more than one currency, the TOTAL line reads `TOTAL USD`, one line
  for each currency (C6). With one currency it reads `TOTAL`.
- An account that failed adds a line under the TOTAL in the error color:
  `Birch 401k: could not be valued (<reason>)`. Then one muted line:
  `Percentages leave out accounts that could not be valued.`
- With no active investment account, the view shows
  `No investment accounts. Add one to see holdings.`
- The bottom hint line of all three reports becomes
  `n net worth  s spending  i holdings  esc back`.

### 7.3 The bar

`report.Bar(value, largest types.Money, width int) string`, which the CLI
uses too:

- `eighths = floor(value / largest × width × 8)`.
- `eighths / 8` full blocks `█`, then one partial block from
  `▏▎▍▌▋▊▉` for `eighths % 8` when it is not zero, then spaces to `width`.
- An empty bar when `value` or `largest` is zero or less.
- No track characters (`░`). No color style.

`largest` is the largest row Value in the row's section. The spending
report keeps its own `renderSpendingBar`.

### 7.4 The split

Enter, or a double click on a row, opens the split of the selected row:

```
ACME  Acme Total Market Index                                                                                 $101103.75  40.0% of total
════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
Account                                                                                                Shares         Value % of Holding
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Maple Invest Brokerage                                                                                    250     $61275.00        60.6%
Maple Invest Roth IRA                                                                                     100     $24510.00        24.2%
Cedar HSA Investment                                                                                     62.5     $15318.75        15.2%
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Total                                                                                                   412.5    $101103.75       100.0%

  esc back to holdings
```

- Columns: Account (flex), Shares (12), Value (13), % of Holding (12).
- On the Cash row there is no Shares column. Each account with cash has a
  row.
- Esc closes the split. The cursor stays on the same holdings row. The
  global Esc handler in `app.go` lets the view claim Esc while the split is
  open, as it does for the Corporate Actions detail panel.
- Esc in the holdings table leaves the Reports view, as in the other
  reports.
- A reload (for example after a price refresh) closes the split and keeps
  the cursor on the same security, or on Cash.
- Leaving the Reports view closes the split.

### 7.5 Hints and help

Status bar hints in `views.go` depend on the report:

| Report | Hint |
|---|---|
| Net worth, spending | `←→ period  n net worth  s spending  i holdings  y year  m month  esc back` |
| Holdings | `↑↓ navigate  enter accounts  n net worth  s spending  esc back` |
| Holdings split | `↑↓ navigate  esc back to holdings` |

`reportsShortcuts` in `help_overlay.go` adds `{"i", "Holdings report"}`
and `{"Enter", "Holdings: show the accounts that hold the row"}`.

## 8. Tests

Every fixture uses fictional names and amounts.

`internal/report`:

- One security in two accounts makes one row. Shares, Value and Cost Basis
  add up. The split has two entries, largest first, with their percentages.
- The Cash row is the sum of the account cash. It is not shown when the sum
  is zero.
- The section Value equals the investment rows of `NetWorthReport()` on the
  same file (the invariant in §5).
- A closed account is not included. An account with an opening date after
  today is not included. A register account is not included.
- A hidden security that is held is included.
- An unpriced security: Value = Cost Basis, Estimated, Gain zero, and the
  section is Estimated.
- Two currencies give two sections, sorted by code, and are never added.
- A valuer that fails for one account: an `AccountFailure`, that section
  not Available, the other section still Available.
- Rows with equal Value sort by Label. The percentages of a section add up
  to 100 within rounding.

`internal/cli/report`:

- The full report output for a fixture, with `~` and its note.
- `--ticker` prints the split. `--name` finds a security with no ticker.
- A security that is not held, and an unknown ticker, return errors.
- Two selectors at once return the `Resolve` error.
- A failed account prints `not available` and the command exits non-zero.

`internal/tui`:

- `holdingsColumns` at widths 57/58, 84/85 and 108/109 shows the correct
  columns, and Name and bar follow the spare-width rule.
- `report.Bar`: the largest row is full, each partial eighth is correct,
  and zero or negative values give an empty bar.
- `i` loads the holdings report. The menu item opens it.
- Enter and a double click open the split. Esc closes it, and the cursor
  stays on the row. A second Esc leaves the view.
- A click on the first row selects it (pins `tableContentRowOffset`).
- The hints change with the report, and the view guard tests still pass.

## 9. Docs to update in the code PR

- `specs/reports.md`: a "Holdings Report" section that points to this spec.
- `specs/cli.md`: a `report holdings` section with its flags and output.
- `specs/tui.md`: the Reports View section gets the holdings report and `i`.
- `README.md`: the Reports features, the `3` Reports row, and the Reports
  key table.

## 10. Not in this version

- A pie or donut chart (kitty graphics).
- `--as-of` and closed accounts.
- A top-N limit, or a group for small rows.
- Group by asset class or by security type.
- Sort by a different column.
- The Cash split in the CLI (C8).
- JSON output.
