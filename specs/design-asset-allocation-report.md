# Design: asset allocation report

**Date:** 2026-10-28
**Status:** APPROVED on 2026-10-28, with the choices in §3.

Every name, ticker and amount in this document is fictional. Money shows as
`types.Money.Format` writes it, with no thousands separators.

## 1. Purpose

The holdings report (`specs/design-holdings-report.md`) shows each security
across all the investment accounts. It does not show how the money divides
across asset classes: how much is in large-cap stock, in bonds, in crypto.

The asset allocation report groups the rows of the holdings report by the
asset class of each security. A pie shows the share of each class, and a
legend table beside it gives the exact value and percentage. Enter on a class
shows its holdings, and `e` there edits a security, so the owner can fix the
classes of the securities from the report and see the pie change.

The report also adds a Mid Cap Stock class and removes the Index class (§4).

A pie is a good fit here, which it was not for the holdings report: there
are at most 12 classes, not a few dozen securities.

Most terminals cannot show an image. The pie uses the kitty graphics
protocol, and the report works as a table in every other terminal (§6.3).

**Owner guidance, not code:** a US total market fund is mostly large-cap
(about 70–75% by value), so its class is Large Cap Stock. Its mid-cap and
small-cap part (about a quarter of the fund) then counts as Large Cap. A
later version can split one security across classes (§9).

## 2. Decisions

These were settled with the owner on 2026-10-27 and 2026-10-28.

| # | Decision | Chosen |
|---|---|---|
| D1 | Class per security | One class per security, 100% of its value. The report gets the class of a row through one function, so a later version can split a security across classes by weight without other changes. |
| D2 | Class list | Add `mid_cap_stock` ("Mid Cap Stock"), between Large Cap Stock and Small Cap Stock in every picker. Remove `index`: the migration changes each `index` security to `unclassified`. Keep all the other classes, Asset Mixture too. |
| D3 | Accounts and total | The accounts, the valuation and the sections of the holdings report. One Cash class holds the uninvested cash and every security of class Cash (for example a money market fund). Bank accounts are not included. |
| D4 | Name and entry | "Asset Allocation". Key `a` in the Reports view, an "Asset Allocation" item in the Reports menu, and `tmoney report allocation`. |
| D5 | Layout | A full pie on the left, the legend table on the right. The slices are in the order of the table, clockwise from 12 o'clock. A class with no value has no slice and no row. There is no "Other" slice. The pie size follows the screen. The Holdings column counts securities. |
| D6 | When the pie shows | When the terminal replies "OK" to the kitty graphics query, the app is not in tmux, and the screen has room. Else the view is the table only, with a bar column in the class colors. A setting `"chart_images": "auto" \| "on" \| "off"` in `config.json` (no key = `auto`) and a "Chart images" item in the View menu change this. |
| D7 | Colors | Each class has one fixed color, whatever its size or position. Related classes use one color family: stocks blue, bonds green. Each color has a light-theme value and a dark-theme value (§5.4). A transparent gap separates the slices. The legend always shows the swatch, the name and the %. The colors are in the code, not in theme files. |
| D8 | Enter | Enter (or a double click) on a class opens the holdings in that class, full width, with no pie. Enter on a holding there opens its account split, the same table as in the holdings report. Esc goes back one level. |
| D9 | CLI | `tmoney report allocation` prints a text table with neutral bars. It prints the pie only when stdout is a terminal and the rules of D6 are true. `--asset-class <code>` prints the holdings in one class. For the account split of one security, `tmoney report holdings --ticker` is the command. There is no JSON output. |
| D10 | More than one currency | One section per currency, in currency-code order, each with its own pie, table and TOTAL. The TUI shows one currency at a time, and `←`/`→` change it. The CLI prints all the sections. No exchange rates. |
| D11 | Values a pie cannot show | Negative Cash, or a total of zero or less: no pie, the table only, with a note. A failed account: the pie and the percentages are of the accounts that were valued, with the notes of the holdings report. A holding with no price: left out of the pie, the class value, the count and the percentages, and a note names it. A slice smaller than 2° is drawn at 2°. |
| D12 | Unclassified | Always the last row and the last slice, in gray. When it has holdings, a note says how to set a class. In the class drill-down, `e` opens the Edit Security dialog for the holding under the cursor (any class). After a save the report loads again. The CLI prints the note only. |
| D13 | Migration 037 | Rebuild `securities` with the recipe of migration 034: drop the `portfolio_holdings` view, copy the CREATE from the live catalog, change only the CHECK list, map `index` to `unclassified`, do not create the 5 secondary indexes again, and create the view again. `TestMigration037` diffs the catalog. |
| D14 | Delivery | This one spec, and three PRs in order: Mid Cap and migration 037 (§4), the report as text (§5), the pie (§6). |

### 2.1 Rules

1. A class row is one asset class in one currency. Its value is the sum of
   the values of its priced holdings.
2. The Cash class is the uninvested cash row of the holdings report plus
   each security of class Cash.
3. The Holdings count of a class is the number of its priced securities. The
   uninvested cash row is not a security, so it does not count.
4. The rows sort by value, largest first, and rows with equal value sort by
   display name. Unclassified is always last.
5. A holding with no price is not in any value, count or percentage. It is
   not shown with `~`, as the holdings report does: this report leaves it
   out and names it in a note.
6. Because of rule 5, the allocation TOTAL is smaller than the holdings
   TOTAL when a holding has no price. In every other case the two totals
   are the same.
7. Money in different currencies is never added.
8. The report is for today only. There is no `--as-of`.
9. Text never uses a class color. The color is only on the bar, the swatch
   and the slice.
10. A click selects a row. A double click does the same as Enter.

## 3. Choices made while writing this spec

These points were added while the spec was written, and the owner approved
them. Each one follows a pattern that is in the code or in the decisions.

| # | Point | Choice | Why |
|---|---|---|---|
| C1 | Where the class comes from | `HoldingRow` gets `AssetClass`. `securityLabels` reads `asset_class` in the query it already runs. `Allocation()` is built from `Holdings()`. | One valuation path. The two reports cannot disagree, except by rule 6. |
| C2 | Colored cells | `widget.Table` gets one foreground color per cell (`SetCellColor`). Widths and truncation work on the plain text, before any style. | `AlignText` counts runes, so ANSI codes in a cell break the widths. The bar and the swatch need a color. |
| C3 | Light or dark colors | From the terminal background: `tea.RequestBackgroundColor` in the TUI, OSC 11 in the CLI. Dark when no reply comes. | The theme's `desktop.bg` is not painted in v1 (`theme.DesktopColors`), so the terminal background is the real background. |
| C4 | Pie size in the TUI | `R` rows, from the free rows of the view, at most 20. No pie below 8 rows. The columns follow from the cell size. The pie shows only when the table fits beside it (§6.5). | D5 says the size follows the screen. Below 8 rows the slices are too small to read. |
| C5 | Image ID | The low byte goes in a 256-color foreground (`38;5;N`, N from 1 to 255). The high byte goes in the third diacritic (1 to 255). A new random ID for each TUI run and each CLI run. | Bubble Tea reduces truecolor to 256 colors for a 256-color profile, and a 256-color index survives that. Random IDs keep old CLI pies in the scrollback from changing. |
| C6 | Colors needed for the pie | The TUI shows the pie only with a 256-color or truecolor profile. | With 16 colors the ID cannot be encoded. |
| C7 | TUI detection | The TUI sends the query at start and shows the pie after the kitty "OK" reply arrives. It does not wait for DA1. | Bubble Tea can send its own DA1 query, so a DA1 reply does not mark the end of our replies. Until "OK" arrives, the table shows, which is correct for a terminal with no support. |
| C8 | `"on"` | Skips the kitty reply check and the tmux check. All other conditions stay. | `on` is for a terminal that shows the pie but does not reply. |
| C9 | A click on the pie | Selects nothing. | The pie and the table share lines. A click on a slice must not select the row on the same line, because that row is a different class. |
| C10 | After `e` and Save | The drill-down stays on the same class. If the security is still in the class, the cursor stays on it. If it moved to another class, the cursor stays at the same position, which is now the next row. If the class is now empty, the view goes back to the allocation table. | D12 says the cursor goes to the next row after a reclassify. |
| C11 | The image in the TUI | Leaving the Reports view and quitting delete the image (`a=d,d=I`). The drill-down only stops drawing the placeholders, so the pie comes back with no new transmit. | The terminal keeps an image until it is deleted. |
| C12 | CLI `--asset-class` errors | An empty class gives `no holdings in Mid Cap Stock`. When an account failed, it gives `no holdings in Mid Cap Stock in the accounts that could be valued:` and the account errors. | The same pattern as `report holdings --ticker` (`notHeldError`). |
| C13 | Gap | 3 px for a 600 px pie, in proportion to the size, at least 2 px. | This is the gap in the preview that the owner approved. |

## 4. PR 1: Mid Cap Stock and migration 037

### 4.1 Go changes

`internal/security/security.go`:

- Add `AssetClassMidCapStock AssetClass = "mid_cap_stock"`, display name
  "Mid Cap Stock".
- Remove `AssetClassIndex` from the constants, `AllAssetClasses`, `IsValid`
  and `DisplayName`.
- `AllAssetClasses` order: Large Cap Stock, Mid Cap Stock, Small Cap Stock,
  International Stock, Domestic Bond, Foreign Bond, Cash, Commodity, Crypto,
  Asset Mixture, Real Estate, Unclassified. The TUI pickers and the filters
  use this order.
- `ParseAssetClass("index")` now returns the "invalid asset class" error.

The two tests that use `AssetClassIndex` (`security_test.go`,
`security_repository_test.go`) use `AssetClassMidCapStock`.

### 4.2 Migration 037

`internal/db/migrations/037_securities_mid_cap.sql`. The live catalog at
version 036 has these facts, which set the steps:

- No foreign key points to `securities`. Migration 010 removed them. So no
  other table is backed up, which is different from migration 034.
- The `portfolio_holdings` view reads `securities`.
- `securities` has `isin` (027), the `uuidv7()` default (032), and five
  secondary indexes from 012: `idx_securities_ticker`,
  `idx_securities_type`, `idx_securities_asset_class`,
  `idx_securities_hidden`, `idx_securities_ticker_currency`. None is UNIQUE.

Steps:

1. `DROP VIEW IF EXISTS portfolio_holdings;`
2. `CREATE TEMPORARY TABLE securities_backup AS SELECT * FROM securities;`
   then `DROP TABLE securities;`
3. Create `securities` again with the CREATE copied from
   `duckdb_tables().sql` at version 036, not from migration 012. The only
   change is the `asset_class` CHECK list: add `'mid_cap_stock'` after
   `'large_cap_stock'`, and remove `'index'`. The column order stays the
   same.
4. Copy the rows with an explicit column list. The `asset_class` column is
   `CASE WHEN asset_class = 'index' THEN 'unclassified' ELSE asset_class END`.
5. `DROP TABLE securities_backup;`
6. Do not create the five indexes again. Migrations 021, 023 and 027 give
   the reason: DuckDB changes an UPDATE of an indexed column into a
   DELETE+INSERT, and a stale index then fails the delete and closes the
   connection. The `e` workflow (D12) updates `asset_class` many times. The
   table is small, so a full scan costs almost nothing. The ticker+currency
   rule is in the service layer, not in an index.
7. Create `portfolio_holdings` again with the SQL copied from
   `duckdb_views().sql` at version 036.

The header comment of the file gives these reasons, as the header of 034
does.

### 4.3 Tests

`internal/db/migration_037_test.go`, with the helpers of
`migration_034_test.go` (`openAtVersion`, `applyVersion`, `catalogSQL`,
`tableCounts`):

- The catalog before and after differs only in the `asset_class` CHECK list
  and in the five removed indexes.
- Every row, ID and column value stays the same, except `index`, which
  becomes `unclassified`. A security with an ISIN keeps it.
- A new row gets a `uuidv7()` ID.
- After the migration, an UPDATE of `asset_class` on a security that has
  prices and positions works.
- An insert of `index` fails the CHECK, and an insert of `mid_cap_stock`
  works.
- `portfolio_holdings` gives the same rows before and after.

`internal/security`: `ParseAssetClass` accepts `mid_cap_stock` and rejects
`index`. `AllAssetClasses` has Mid Cap between Large Cap and Small Cap.

`internal/tui`: the Edit Security dialog offers Mid Cap Stock in the order
above.

### 4.4 Docs

- `specs/security-master.md`: the Asset Classes table and the schema
  snippet.
- `specs/database.md`: a bullet for migration 037 in "Avoiding the rewrite
  in the first place".
- `specs/cli.md`: nothing lists the class values, so no change. Verify.

## 5. PR 2: the report, as text

### 5.1 Data model

`internal/report/report.go`. `HoldingRow` gets one field:

```go
AssetClass security.AssetClass // AssetClassCash on the Cash row
```

`internal/report` can import `internal/security`: that package imports only
`db`, `dberrors`, `dbutil` and `types`, so there is no cycle.

```go
// Allocation is the asset allocation report: the holdings report grouped by
// the asset class of each security. There is one section per currency;
// money in different currencies is never added.
type Allocation struct {
	AsOfDate     types.Date
	AccountCount int // as in Holdings
	// Sections are sorted by currency code.
	Sections []AllocationSection
	Failed   []AccountFailure // as in Holdings
}

// AllocationSection is the allocation of one currency.
type AllocationSection struct {
	Currency string
	// Classes are largest Value first, with Unclassified always last.
	Classes []ClassRow
	Value   types.Money // the sum of the Classes' Value
	// Available is false when an account in this currency could not be
	// valued, as in HoldingsSection.
	Available bool
}

// ClassRow is one asset class in one currency.
type ClassRow struct {
	Class    security.AssetClass
	Value    types.Money // the sum of Holdings' Value
	Percent  float64     // of the section's Value
	Holdings []ClassHolding // largest Value first
	// NoPrice are the holdings of this class that have no price. They are
	// not in Value, Percent or Count. Sorted by Label.
	NoPrice []HoldingRow
}

// Count is the number of priced securities in the class: the uninvested
// cash row is not a security.
func (c ClassRow) Count() int

// ClassHolding is one holdings row in its class.
type ClassHolding struct {
	HoldingRow
	PercentOfClass float64 // of the class's Value
	PercentOfTotal float64 // of the section's Value
}

// Drawable reports whether a pie can show the section: its Value is more
// than zero and no class Value is negative. PR 3 adds it, with the pie that
// uses it.
func (s AllocationSection) Drawable() bool
```

The section also has `Count()`, `Shown()` (the classes with a priced
holding: the rows of the table), `Class(ac)`, and `NoPriceNames()`. The
package has `NoPriceNote` and `UnclassifiedNote`, so the TUI and the CLI
print the same note text.

`HoldingRow.Percent` is not used here. It is of the holdings total, which
includes holdings with no price at cost (rule 6).

### 5.2 Service

`internal/report/allocation.go`:

```go
// Allocation returns the asset allocation report as of today.
func (s *Service) Allocation() (*Allocation, error)
```

Steps:

1. `h, err := s.Holdings()`. Copy `AsOfDate`, `AccountCount` and `Failed`.
2. For each holdings section, for each row, call `classShares(row)`. In
   this version it returns one share: `{row.AssetClass, row.Value}`. This
   is the one function of D1: a split version returns a part of the value
   for each class.
   - A row with `Estimated` set goes to `NoPrice` of its class.
   - Every other row goes to `Holdings` of its class, and its Value adds to
     the class Value.
3. Remove each class that has no holding and no `NoPrice` row. A class that
   has only `NoPrice` rows stays in the data, but it has a Value of zero, so
   it has no row in the table and no slice (D5). The note names its holdings
   (§5.4).
4. The section Value is the sum of the class Values. `Percent`,
   `PercentOfClass` and `PercentOfTotal` use `percentOf`, which gives 0 when
   the whole is zero or less.
5. Sort the classes and the holdings (rule 4).
6. Keep a section when it has a class or it is not Available.

**Invariant.** For each currency where every account was valued and every
holding has a price, the section Value equals the holdings section Value. A
test pins this.

**Cost.** One `Holdings()` call: the same work as the holdings report.

### 5.3 CLI

`internal/cli/report/allocation.go` registers `tmoney report allocation`.
`report.NewCmd` adds it, and its `Long` and `Example` text name it.

```bash
tmoney report allocation
tmoney report allocation --asset-class mid_cap_stock
```

| Flag | Meaning |
|---|---|
| `--asset-class string` | Show the holdings in one class. The value is parsed by `security.ParseAssetClass`, the same as `security list --asset-class`. |

#### 5.3.1 Full report

The output uses `text/tabwriter`, as `report holdings` does. The bar is
`report.Bar` at 20 cells, with no color.

```
ASSET ALLOCATION
================
As of: January 15, 2024

Asset Class          Holdings  Value       % Total
Large Cap Stock      2         $110175.75  33.6%    ████████████████████
Asset Mixture        1         $58226.45   17.7%    ██████████▌
Mid Cap Stock        2         $45000.00   13.7%    ████████▏
International Stock  1         $40240.80   12.3%    ███████▎
Domestic Bond        1         $29581.50   9.0%     █████▎
Cash                 1         $17345.67   5.3%     ███▏
Real Estate          1         $12500.00   3.8%     ██▎
Small Cap Stock      1         $8000.00    2.4%     █▍
Crypto               1         $6000.00    1.8%     █
Unclassified         1         $1200.00    0.4%     ▏
TOTAL (USD)          12        $328270.17  100.0%

Not included (no price): STRK (Large Cap Stock). Add a price to include it.
1 holding has no asset class. Set one with: tmoney security edit <ticker> --asset-class <class>
```

- The bar is relative to the largest class of the section.
- The `Not included` note lists every holding of the section with no price,
  as `Label (Class)`, joined by `, `.
- The Unclassified note shows only when Unclassified has a priced holding.
  It says `holdings have` for more than one.
- With more than one currency, each currency has its own table and TOTAL
  line, in currency-code order, each with its own notes.
- With no investment account: `No investment accounts.` With accounts that
  hold nothing: `No holdings in the investment accounts.` Both exit 0, as in
  `report holdings`.
- Negative cash is a negative Cash row with an empty bar, as in
  `report holdings`. The TOTAL shows `100.0%` only when its Value is more
  than zero.
- A failed account: the TOTAL shows `not available`, the line
  `Percentages leave out accounts that could not be valued.` follows, and
  the command returns the account errors, so it exits non-zero
  (`holdingsErrors`).

When the pie can show (§6.6), the same report prints with the pie.

#### 5.3.2 One class

```
ASSET CLASS: Large Cap Stock
============================
As of: January 15, 2024

Security  Name                     Value       % of Class  % Total
ACME      Acme Total Market Index  $101103.75  91.8%       30.8%
INIT      Initech Corp             $9072.00    8.2%        2.8%
STRK      Stark Industries         no price
Total                              $110175.75  100.0%      33.6%

Not included (no price): STRK. Add a price to include it.
```

- The Cash class lists `Cash  Uninvested cash` as a row.
- An empty class returns the error of C12.
- A class in two currencies prints one block per currency.
- A failed account: the Total line is the class value, as in the class row
  of §5.3.1, and the line
  `Percentages leave out accounts that could not be valued.` follows.
- This output has no image.

### 5.4 Colors

`internal/chart/palette.go` (a new package with no TUI state, as
`internal/tui/pricechart`):

```go
// ClassColor is the fixed color of an asset class on a light or a dark
// background.
func ClassColor(ac security.AssetClass, dark bool) color.RGBA
```

| Class | Light | Dark |
|---|---|---|
| Large Cap Stock | `#184f95` | `#3664b2` |
| Mid Cap Stock | `#307cd7` | `#0587ff` |
| Small Cap Stock | `#71abee` | `#75adf0` |
| International Stock | `#653bd8` | `#9677cd` |
| Domestic Bond | `#248702` | `#299244` |
| Foreign Bond | `#94c521` | `#8dae00` |
| Cash | `#c88f00` | `#8b5c00` |
| Real Estate | `#8f3a0e` | `#df9575` |
| Commodity | `#e24848` | `#eb515c` |
| Crypto | `#b257a9` | `#b63d85` |
| Asset Mixture | `#1fba95` | `#37baa5` |
| Unclassified | `#898781` | `#898781` |

The palette validator of the dataviz method (`--pairs all`, because any two
classes can be adjacent) gives these results. No set of 12 colors passes all
checks, so the legend table and the gaps carry the identity of each class.

| Check | Light (surface `#fcfcfb`) | Dark (surface `#1a1a19`) |
|---|---|---|
| Worst pair, color vision deficiency | 6.9, Cash and Foreign Bond | 7.3, Asset Mixture and Real Estate |
| Worst pair, normal vision (floor 15) | 14.2, Asset Mixture and Foreign Bond | 13.8, Small Cap and Mid Cap (one family), and 13.8, Unclassified and International Stock |
| Contrast to the surface (3:1) | 4 colors below: Asset Mixture, Small Cap, Foreign Bond, Cash | All pass |
| Lightness band | Pass | 4 colors above the band, on purpose: light colors on a dark surface are easy to see, and the separation is much better |
| Chroma floor | Unclassified fails, on purpose: gray means "no class" | The same |

Six other grays were tested for dark Unclassified. Each one is worse than
`#898781`.

Cash and Real Estate change lightness between the modes: in light mode Cash
is gold and Real Estate is dark brown; in dark mode Cash is brown and Real
Estate is peach. The owner saw this in the preview and kept it.

The TUI selects the set from the terminal background (C3). The CLI text
output has no color.

### 5.5 TUI

#### 5.5.1 Entry and state

- `reportTypeAllocation` is a new `reportType`. `reportsViewData` gets
  `allocation *report.Allocation`, and `load` calls
  `reports.Allocation()` for it. Like holdings, it keeps the year, month and
  transfers fields.
- `a` in the Reports view loads it. `n`, `s` and `i` work from it. `y`, `m`
  and `t` do nothing. `←` and `→` change the currency (D10).
- The Reports menu gets `{Label: "Asset Allocation", Action:
  MenuActionAssetAllocation}` after "Holdings". `app_menu.go` handles it as
  it handles `MenuActionHoldings`.
- `reportsViewState` gets `allocation allocationViewState`: the currency
  index, the class table, the class drill-down table (nil when closed), the
  account split table (nil when closed), the refs behind the rows, the
  width, `pageRows`, and the `widget.ClickTracker`.
- The `ViewReports` entry in `views.go`: `table` returns the table on
  screen (split, then drill-down, then class table), and `hints` returns the
  hints of §5.5.6.
- `app.go` lets the view claim Esc while a drill-down or a split is open.
  `reportsViewState.claimsEsc()` replaces the `splitOpen()` call there.
- The app sends `tea.RequestBackgroundColor` at start. On
  `tea.BackgroundColorMsg` it stores `msg.IsDark()`. Until then it uses
  dark (C3).

#### 5.5.2 Layout, with no pie

The table width `W` is the content width minus the padding of 4 cells.

| Column | Width | Align |
|---|---|---|
| Asset Class | 19 | left |
| Holdings | 8 | right |
| Value | 13 | right |
| % Total | 7 | right |
| bar (no header) | spare, 10 to 40 | left |

`spare` is `W` minus the fixed widths and one separator between each two
columns. Below 61 cells (all columns with a bar of 10) the Holdings column
goes. Below 52 cells the table cuts the right side, as other tables do.

At `W` = 96:

```
ASSET ALLOCATION                                                             As of: Jan 15, 2024
════════════════════════════════════════════════════════════════════════════════════════════════
Asset Class         Holdings         Value % Total
────────────────────────────────────────────────────────────────────────────────────────────────
Large Cap Stock            2    $110175.75   33.6% ████████████████████████████████████████
Asset Mixture              1     $58226.45   17.7% █████████████████████▏
Mid Cap Stock              2     $45000.00   13.7% ████████████████▎
International Stock        1     $40240.80   12.3% ██████████████▌
Domestic Bond              1     $29581.50    9.0% ██████████▋
Cash                       1     $17345.67    5.3% ██████▎
Real Estate                1     $12500.00    3.8% ████▌
Small Cap Stock            1      $8000.00    2.4% ██▉
Crypto                     1      $6000.00    1.8% ██▏
Unclassified               1      $1200.00    0.4% ▍
────────────────────────────────────────────────────────────────────────────────────────────────
TOTAL                     12    $328270.17  100.0%
Not included (no price): STRK (Large Cap Stock). Add a price to include it.
1 holding has no asset class. Press enter on Unclassified, then e to set a class.

  enter holdings  n net worth  s spending  i holdings  a allocation  esc back
```

- Each bar is in the color of its class (C2). The text is in the normal
  text colors (rule 9).
- With more than one currency, the title is
  `ASSET ALLOCATION   USD (1 of 2)`, and the TOTAL reads `TOTAL`, because
  the view shows one currency.
- An account that failed: the error line and the note of the holdings
  report, under the TOTAL. Only the accounts of the currency on screen have
  an error line.
- The notes wrap to the table width, and each wrapped line counts in the
  height. A cut note would lose the names of holdings with no price.
- The footer does not scroll. When the names do not fit and leave the
  table its header, one row and the scroll line, the note counts them:
  `Not included (no price): 41 holdings. Press i to see them in the holdings report.`
- Negative cash is a negative Cash row with an empty bar. (PR 3 adds a
  note for it, §6.5.)
- The empty states are the ones of the holdings view, with "allocation" in
  place of "holdings".
- The title, the separator, the header, the TOTAL and the notes stay on the
  screen. Only the rows scroll.

#### 5.5.3 The class drill-down

Enter, or a double click on a class row, opens the holdings of the class:

```
■ Mid Cap Stock                                                        $45000.00  13.7% of total
════════════════════════════════════════════════════════════════════════════════════════════════
Security Name                                                           Value % of Class % Total
────────────────────────────────────────────────────────────────────────────────────────────────
MIDX     Maple Mid Cap Index                                        $30000.00      66.7%    9.1%
MIDV     Maple Mid Cap Value                                        $15000.00      33.3%    4.6%
────────────────────────────────────────────────────────────────────────────────────────────────
Total                                                               $45000.00     100.0%   13.7%

  enter accounts  e edit security  esc back to allocation
```

- The `■` is in the class color.
- Columns: Security (8), Name (flex), Value (13), % of Class (10),
  % Total (7).
- The rows are `ClassRow.Holdings`, then `ClassRow.NoPrice`, with
  `no price` in the Value column and no percentages. The `Not included`
  note of §5.3.2 is under the Total line. When the names do not fit, it
  counts them, as in §5.5.2, and ends `Add a price to include them.`: the
  holdings are rows of the table.
- The Cash class has the `Cash  Uninvested cash` row.
- Enter on a holding opens the account split of the holdings report
  (`splitColumns`, `splitCells`). Its title is the holding, as in the
  holdings report. Esc goes back to the drill-down.
- Esc in the drill-down goes back to the class table, with the cursor on
  the same class.

#### 5.5.4 Edit from the drill-down

- `e` (`keys.Edit`) on a security row loads the security by ID and opens
  the existing Edit Security dialog (`buildEditSecurityDialog`) on the
  `a.security` surface, in edit mode.
- On the Cash row, `e` does nothing.
- The security loads in a command, and the keys do not wait for it. The
  dialog opens only if the drill-down of that `e` is still open, with no
  reload since, and no other dialog is open. Else the reply is dropped.
- On save, `securityUpdatedMsg` runs `afterSecurityChange`. That function
  also reloads the allocation report when it is on screen. The cursor
  follows C10.

#### 5.5.5 Clicks

- A click on a row selects it. A second click on the same row opens it, as
  in the holdings report.
- In the allocation view, a click left of the table selects nothing (C9).
  In PR 2 the table starts at the left edge, so PR 3 adds this:
  `handleMouseTable` passes the message X.

#### 5.5.6 Hints and help

The status bar hints in `views.go`:

| Screen | Hint |
|---|---|
| Net worth, spending | `←→ period  n net worth  s spending  i holdings  a allocation  y year  m month  esc back` |
| Holdings | `↑↓ navigate  enter accounts  n net worth  s spending  a allocation  esc back` |
| Allocation | `↑↓ navigate  enter holdings  ←→ currency  n net worth  s spending  i holdings  esc back` (`←→ currency` only with more than one currency) |
| Class drill-down | `↑↓ navigate  enter accounts  e edit security  esc back to allocation` |
| Account split from a class | `↑↓ navigate  esc back to class` |

The hint line at the bottom of each report view lists every report key,
and gets `a allocation`. It uses `widget.FitParts`, as the holdings view
does.

`reportsShortcuts` in `help_overlay.go` adds `{"a", "Asset allocation
report"}`, `{"Enter", "Allocation: show the holdings of the class"}`, and
`{"e", "Allocation: edit the security"}`.

### 5.6 Tests

Every fixture uses fictional names and amounts.

`internal/report`:

- Two securities of one class make one class row. Value, Count and the
  percentages add up.
- The Cash class is the uninvested cash plus a money market security. The
  cash row does not add to Count.
- A security with no price goes to `NoPrice`, and it is not in Value,
  Count or a percentage. A class with only `NoPrice` holdings has Value 0.
- Unclassified sorts last, also when it is the largest class.
- Equal values sort by display name.
- The invariant: with every account valued and every holding priced, the
  section Value equals the holdings section Value.
- Two currencies give two sections, sorted by code.
- A failed account: the section is not Available, and `Failed` is copied.
- Negative cash: the Cash class is negative, and the section Value is still
  the holdings section Value. (The `Drawable` tests come with PR 3.)

`internal/chart`:

- `ClassColor` has a light and a dark color for every class in
  `AllAssetClasses`, and each color is the value of the §5.4 table.

`internal/cli/report`:

- The full output for a fixture, with both notes.
- `--asset-class mid_cap_stock` prints the class. `--asset-class index`
  returns the parse error. An empty class returns the C12 error.
- A failed account prints `not available` and exits non-zero.
- The Unclassified note uses the singular and the plural correctly.
- Output to a buffer has no escape codes.

`internal/tui`:

- `a` loads the report. The menu item opens it.
- Enter and a double click open the drill-down. Enter there opens the split.
  Esc goes back one level at a time, and the cursor stays on its row. One
  more Esc leaves the view.
- `e` opens the Edit Security dialog for the row under the cursor, and does
  nothing on the Cash row. A save reloads the report and follows C10.
- `←`/`→` change the currency with two sections, and do nothing with one.
- `SetCellColor`: the widths of a row with a colored cell are the widths of
  the same row with no color. The selected row keeps the cell color.
- The bar uses the light color after a light `BackgroundColorMsg`.
- The column rule at widths 51/52 and 60/61.
- A click on the first row selects it.
- The hints change with the screen, and the view guard tests still pass.

### 5.7 Docs

- `specs/reports.md`: an "Asset Allocation Report" section that points to
  this spec.
- `specs/cli.md`: a `report allocation` section.
- `specs/tui.md`: the Reports View, the Reports Menu, and the keys.
- `README.md`: the Reports features, the `3` Reports row, the Reports key
  table, and a CLI example.

## 6. PR 3: the pie

### 6.0 Verify first

Before the rest of PR 3, a spike must show these three facts in Ghostty and
in Rex. If one fails, the spec changes before the code is written.

1. A pie drawn with Unicode placeholders inside the Bubble Tea v2 renderer
   shows correctly, also after a partial redraw, a resize, and a scroll of
   the legend table. The renderer must count each placeholder cell (U+10EEEE
   and its diacritics) as one cell, and keep the diacritics.
2. A `38;5;N` foreground on the placeholder cells survives the renderer's
   color profile. (C5)
3. The third diacritic sets the high byte of the image ID. If it does not,
   the ID uses the low byte only, and the CLI accepts 1 chance in 255 that
   an old pie in the scrollback changes.

The scratch probe of 2026-10-27 showed that both placement methods work in
Ghostty and Rex, and that Rex replies "OK" before DA1. herdr is not tested.

### 6.1 Packages

- `internal/chart/pie.go`: draws the pie. Pure: no terminal, no TUI state.
- `internal/termimg`: the kitty graphics code that the TUI and the CLI
  share: the encoding, the placeholder lines, the decision rule, and the
  CLI terminal query.

### 6.2 Drawing

```go
// Slice is one slice of a pie.
type Slice struct {
	Value float64 // more than zero
	Color color.RGBA
}

// Pie draws the slices clockwise from 12 o'clock, in order, on a
// transparent square of size pixels. A slice smaller than minDeg is drawn
// at minDeg; the other slices get smaller in proportion to pay for it.
func Pie(slices []Slice, size int, minDeg float64) *image.NRGBA
```

- The gap between slices is transparent: `max(2, size/200)` px (C13).
- 4×4 samples per pixel give smooth edges.
- `minDeg` is 2 (D11). A slice is drawn at 2° only when its real angle is
  smaller. The preview of 2026-10-28 showed that a slice of 0.1% is a band
  about 7 px wide at the edge of a 600 px pie, with a clear color.
- The algorithm is the one in the preview: fix each slice under the
  minimum at the minimum, share the rest of the circle among the other
  slices by value, and repeat until no free slice is under the minimum.

### 6.3 When the pie shows

```go
// Mode is the chart_images setting.
type Mode int // ModeAuto, ModeOn, ModeOff

// Show reports whether to draw the pie.
func Show(mode Mode, kittyOK, inTmux, fits bool) bool
```

| Mode | Rule |
|---|---|
| `off` | Never. |
| `on` | When it fits (C8). |
| `auto` | When the kitty reply was "OK", `TMUX` is empty, and it fits. |

Other conditions, before `Show`:

- TUI: the color profile is 256 colors or truecolor (C6).
- CLI: stdout is a terminal (`term.IsTerminal(os.Stdout.Fd())`). If it is
  not, the CLI sends no query and prints only text. An AI agent, a pipe and
  a file get the text output.
- Both: the section is `Drawable` (D11).

### 6.4 Encoding

`internal/termimg`:

- Transmit: `kitty.EncodeGraphics` with `Action: kitty.Transmit`,
  `Transmission: kitty.Direct`, `Format: kitty.PNG`, `Quiet: 2`,
  `Chunk: true`, and the image ID. `Transmission` must be set: when it is
  zero, x/ansi v0.11.9 writes an empty payload (charmbracelet/x#1016).
- Place: `ESC_Ga=p,U=1,i=ID,c=C,r=R,q=2ESC\`: a virtual placement of C
  columns and R rows.
- Placeholder line `y`: the foreground `38;5;lo`, then for each column `x`
  the cell U+10EEEE + `kitty.Diacritic(y)` + `kitty.Diacritic(x)` +
  `kitty.Diacritic(hi)`, then `39m`.
- Delete: `ESC_Ga=d,d=I,i=ID,q=2ESC\`.
- The ID: `hi<<24 | lo`, both random from 1 to 255 (C5).

### 6.5 TUI

- At start, and when the setting changes from `off`, the app sends with
  `tea.Raw` the kitty query `ESC_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAAESC\` and
  the cell size query `CSI 16 t`. With `off`, it sends nothing. A `uv.KittyGraphicsEvent` with ID 31 and the
  payload "OK" sets `kittyOK` (C7). A `uv.CellSizeEvent` sets the cell
  size. With no cell size, the pie uses a cell of 1:2 and 600 px.
- `R` is the free rows of the view (the height minus the app chrome, the
  title, the separator, the TOTAL and the notes), at most 20. With `R` less
  than 8, there is no pie.
- `C` is `round(R × cellHeight / cellWidth)`, so the pie is round.
- The legend table is 52 cells: the swatch (1), Asset Class (19),
  Holdings (8), Value (13), % Total (7), and four separators. There is no
  bar column. It fits when `W ≥ C + 3 + 52`. If not, try a smaller `R`,
  down to 8. If it still does not fit, show the table of §5.5.2.
- The pie image is `C × cellWidth` by `R × cellHeight` pixels, so it is
  sharp. The app draws and transmits it again when the data, `R`, `C`, the
  currency or the light/dark choice changes. Each transmit replaces the
  image with the same ID.
- The pie and the legend start on the line under the separator. The TOTAL
  and the notes are under the legend. When the legend is shorter than the
  pie, the TOTAL goes under the pie.

```
ASSET ALLOCATION                                                                         As of: Jan 15, 2024
═════════════════════════════════════════════════════════════════════════════════════════════════════════════
 ┌──────────────────────────────┐     Asset Class         Holdings         Value % Total
 │                              │   ──────────────────────────────────────────────────────
 │                              │   ■ Large Cap Stock            2    $110175.75   33.6%
 │                              │   ■ Asset Mixture              1     $58226.45   17.7%
 │          pie image           │   ■ Mid Cap Stock              2     $45000.00   13.7%
 │   (R rows × C columns of     │   ■ International Stock        1     $40240.80   12.3%
 │    placeholder cells)        │   ■ Domestic Bond              1     $29581.50    9.0%
 │                              │   ■ Cash                       1     $17345.67    5.3%
 │                              │   ■ Real Estate                1     $12500.00    3.8%
 │                              │   ■ Small Cap Stock            1      $8000.00    2.4%
 │                              │   ■ Crypto                     1      $6000.00    1.8%
 │                              │   ■ Unclassified               1      $1200.00    0.4%
 │                              │   ──────────────────────────────────────────────────────
 │                              │     TOTAL                     12    $328270.17  100.0%
 └──────────────────────────────┘
 Not included (no price): STRK (Large Cap Stock). Add a price to include it.
 1 holding has no asset class. Press enter on Unclassified, then e to set a class.

  enter holdings  n net worth  s spending  i holdings  a allocation  esc back
```

(The box shows where the image is. The real view has no box.)

- When the pie would show but the section is not `Drawable`, the view
  shows the table of §5.5.2 with the note `No pie: Cash is negative.`

- The drill-down and the split do not draw the placeholders, so the pie is
  not on screen there (D8). Back in the class table, the pie shows again
  with no new transmit.
- Leaving the Reports view and quitting delete the image (C11).

### 6.6 CLI

- When `Show` is true, the CLI opens `/dev/tty` and sets raw mode with
  `VMIN=0` and `VTIME=1`, through `github.com/charmbracelet/x/termios`
  (already in the module graph). It restores the mode on every path.
- It writes the kitty query, OSC 11 (`ESC]11;?ESC\`), `CSI 16 t` and DA1
  (`ESC[c`), and reads until the DA1 reply or 300 ms. A kitty "OK" before
  DA1 means support. The OSC 11 reply picks the colors (C3).
- `R` is 16. `C` follows from the cell size. It fits when the terminal
  width (`term.GetSize` on stdout) is at least `C + 3 + 52`.
- The output is the text report of §5.3.1, with the pie in place of the
  bar column: each section transmits its own image with its own random ID,
  places it, and prints the R placeholder lines with the legend lines on
  the right. The notes follow, as in the text output. This is the layout of
  the preview that the owner approved.
- A section that is not `Drawable` prints as text, with the neutral bars
  and the note `No pie: Cash is negative.`
- The CLI does not delete its images, so the pies stay in the scrollback.
- A build for an OS with no `/dev/tty` (`termimg/probe_other.go`) returns
  no support.

### 6.7 The setting

`internal/config/config.go`:

```go
// ChartImages is the "View → Chart images" setting: "auto", "on" or "off".
// Empty or unknown means "auto", so old config files keep working.
ChartImages string `json:"chart_images,omitempty"`
```

- The View menu gets `Chart images: Auto` (or `On`, `Off`) under "Show
  closed positions". Each select moves Auto → On → Off → Auto, saves the
  config, and renders the view again.
- The CLI reads the same key with `config.Load`, as
  `cmdutil.OpenServices` does.

### 6.8 Tests

`internal/chart`:

- `Pie`: the pixel at the middle of each slice has the slice color. A pixel
  on a gap is transparent. A pixel outside the circle is transparent.
- The minimum angle: a 0.1% slice is 2°, the sum of the angles is 360°,
  and a slice larger than 2° does not change by more than its share.
- The order is clockwise from 12 o'clock.

`internal/termimg`:

- `Show` for each mode and each input.
- The transmit has a non-empty payload (pins charmbracelet/x#1016).
- A placeholder line has C cells, the correct diacritics for row, column
  and high byte, and the `38;5;lo` foreground.
- The CLI reply parser: "OK" before DA1, an error before DA1, DA1 only, no
  reply in time, and OSC 11 in its 4-digit and 2-digit forms.

`internal/tui`:

- With `kittyOK` set and room, the view has the placeholder lines and the
  legend with swatches. With no room, it has the table of §5.5.2.
- `R` and `C` for a few heights and cell sizes. No pie below 8 rows.
- No pie with a 16-color profile, in tmux (unless `on`), or with `off`.
- A section with negative cash shows the table and the note.
- A click on the pie selects nothing. A click on the legend selects the
  row.
- The View menu item moves Auto → On → Off → Auto and saves.

`internal/cli/report`:

- When stdout is not a terminal, the output has no escape codes, also with
  `chart_images` set to `on`.

### 6.9 Docs

- `specs/tui.md`: the View Menu gets "Chart images", and the Reports View
  describes the pie and when it shows.
- `specs/reports.md`: the pie.
- `README.md`: the pie, the terminals that show it (tested: Ghostty, Rex),
  and the `chart_images` setting.

## 7. Risks

- The renderer risk of §6.0. The spike comes first for this reason.
- herdr is not tested. If it shows broken cells, `"chart_images": "off"`
  is the fix for the owner.
- WezTerm and Konsole reply to the kitty query but do not support Unicode
  placeholders. In `auto` they would show broken cells. They are not in
  use by the owner. If needed later, a `TERM_PROGRAM` deny list can go in
  `Show`.
- The colors fail two checks of the validator (§5.4). The legend table,
  the gaps, and the fixed position of Unclassified are the relief.

## 8. Preview

The scratch program of 2026-10-28 (not in the repo) drew this layout with
the §5.4 colors in Ghostty and Rex, and the owner approved it. The minimum
angle was checked with the same program at 0°, 1°, 2° and 3°.

## 9. Not in this version

- Splitting one security across classes by weight (D1). The total market
  fund is the main reason to add it.
- Exchange rates and one pie for all currencies.
- JSON output.
- An edit from the CLI report.
- A pie for one class.
- A highlight of the selected slice.
- Sixel and the iTerm2 image protocol.
- tmux passthrough.
- Class colors in theme files.
- A donut.
