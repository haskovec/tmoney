# Security Master Specification

## Overview

The Security Master is a centralized registry of financial instruments (stocks, ETFs, mutual funds, and other assets) that can be held in investment accounts. It provides the foundation for portfolio tracking, pricing history, lot-level cost basis management, and corporate actions.

## Securities

### Security Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `ticker` | string | Yes | Trading symbol (e.g., "GDX", "AAPL") |
| `name` | string | Yes | Full security name (e.g., "Market Vectors Gold Miners ETF") |
| `security_type` | enum | Yes | Type of security (see below) |
| `asset_class` | enum | Yes | Asset classification (see below) |
| `currency` | string | Yes | ISO 4217 currency code (default: "USD") |
| `exchange` | string | No | Exchange where listed (e.g., "NYSE", "TSX") |
| `hidden` | boolean | Yes | Whether security is hidden from new transactions (default: false) |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Security Types

| Type | Description |
|------|-------------|
| `stock` | Individual company stock |
| `etf` | Exchange-traded fund |
| `mutual_fund` | Mutual fund |
| `other` | Other assets (physical gold, collectibles, etc.) |

### Asset Classes

| Asset Class | Description |
|-------------|-------------|
| `large_cap_stock` | Large capitalization equities |
| `small_cap_stock` | Small capitalization equities |
| `international_stock` | Foreign equities |
| `index` | Index funds/ETFs |
| `domestic_bond` | US bonds and bond funds |
| `foreign_bond` | International bonds and bond funds |
| `cash` | Money market, cash equivalents |
| `commodity` | Commodities (gold, oil, etc.) |
| `crypto` | Cryptocurrency |
| `asset_mixture` | Balanced/target-date/multi-asset funds |
| `unclassified` | Default for uncategorized securities |

### Security Validation Rules

1. `ticker` must be unique within a given `currency` (same company can be listed in different currencies with different tickers, e.g., "RY" in USD and "RY.TO" in CAD)
2. `ticker` cannot be empty, max 20 characters
3. `name` cannot be empty
4. `currency` must be a valid ISO 4217 code
5. A security can only be marked `hidden` if it has no open positions (zero shares held across all accounts)
6. Hidden securities retain all pricing history and historical transaction data
7. Hidden securities are excluded from new transaction security lookups
8. Hidden securities are excluded from price update operations (manual or API)

### Security Operations

#### Create Security

Required: ticker, name, security_type
Defaults: asset_class = "unclassified", currency = "USD", hidden = false

#### Edit Security

All properties except `id` and `created_at` can be modified. Ticker changes are supported to handle exchange migrations (e.g., company moves from one exchange to another).

#### Hide Security

Set `hidden = true`. Prerequisites:
- No open positions across any account (total shares held = 0)
- All lots for this security must be fully closed

Hidden securities:
- Remain in the security list (filterable)
- Retain all pricing history
- Cannot be used in new buy/reinvest transactions
- Are skipped during price update operations

#### Delete Security

Only allowed if the security has no transaction history and no pricing data. Otherwise, hide it instead.

#### Merge Securities

When one company acquires another or a ticker changes in a way that requires history consolidation:

1. Select source security and target security
2. Record merge date
3. Record exchange ratio (e.g., 2 shares of OLD = 1 share of NEW)
4. Record any cash consideration per share (optional)
5. System generates exchange transactions in all accounts holding the source security (see Corporate Actions)
6. Source security is marked hidden after merge
7. Pricing history for source security is retained separately (not moved to target)

## Prices

### Price Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `security_id` | UUID | Yes | Reference to security |
| `date` | date | Yes | Price date |
| `price` | decimal | Yes | Closing price per share |
| `source` | enum | Yes | How the price was entered (see below) |
| `created_at` | timestamp | Yes | When record was created |

### Price Sources

| Source | Description |
|--------|-------------|
| `manual` | User entered manually |
| `transaction` | Derived from a buy/sell transaction |
| `import` | Loaded via bulk import |
| `api` | Retrieved from an external price provider (e.g., Yahoo Finance) |

### Price Validation Rules

1. One price per security per date (unique constraint on `security_id` + `date`)
2. `price` must be positive (> 0)
3. `date` cannot be in the future
4. If a price already exists for a security+date and a new one is entered, the user is prompted to overwrite or keep existing

### Price Auto-Creation

Prices are automatically created from transactions in the following cases:

| Transaction Type | Price Created |
|------------------|---------------|
| Buy | price_per_share on transaction date |
| Sell | price_per_share on transaction date |
| Reinvest Dividend | reinvestment price on transaction date |

Auto-created prices use `source = 'transaction'`. If a price already exists for that security+date from a manual or import source, the transaction-derived price does **not** overwrite it (manual/import data is considered more authoritative).

### Price Operations

#### Add Price

Enter security (by ticker or name), date, and price. If a price already exists for that combination, prompt to overwrite.

#### Edit Price

Modify the price value for an existing security+date entry.

#### Delete Price

Remove a price entry. No cascade effects — positions and valuations will use the next most recent price.

#### Get Current Price

For a given security, return the most recent price on or before a given date (defaults to today).

#### Get Price History

Return all prices for a security within an optional date range, ordered by date.

#### Update Prices from a Provider

Bulk-fetch the latest closed-session price for every visible security with a non-empty ticker. Available from both the CLI (`--update-prices`) and the TUI (`u` on the securities view).

**Provider interface.** Providers implement `FetchQuote(ticker) (*Quote, error)` and a `Name()`. They are looked up on the price service's `ProviderRegistry`. The default and only built-in network provider is `yahoo` (Yahoo Finance's `query1.finance.yahoo.com/v8/finance/chart` endpoint, called with `interval=1d&range=5d`). The registry is open: additional providers can be registered without modifying core code.

**Last closed session only.** A provider must never return an in-progress (intraday) price. `YahooProvider` walks the response's daily-bar `timestamp[]` and `close[]` arrays from the end, picking the most recent bar that is guaranteed to be closed:

- If the bar's date in the exchange tz is strictly before today's date there → closed.
- If the bar's date is today and `now >= currentTradingPeriod.regular.end` → closed (today's session has already ended).
- Otherwise the bar is intraday; the provider falls back to the prior bar.

This makes runs idempotent: invoking the refresh twice on the same calendar day fetches the same quote, and the orchestrator notices the date is already on file and skips the second write.

**Skip rules.** The orchestrator (`price.Service.RefreshPrices`) iterates over the security master and applies these rules per security:

1. Hidden security → skipped silently.
2. Empty ticker → skipped silently.
3. Provider error (HTTP failure, unknown ticker, malformed body) → recorded as a per-ticker failure; the run continues with the next security.
4. Provider currency does not match the security's `currency` → skipped with a `provider X vs security Y` note. The price is **not** written.
5. The most recent price already on file has the same date the provider returned → recorded as `up-to-date`; nothing is written.
6. Otherwise → upsert via `(security_id, date)` with `source = api`.

**Polite delay.** A 200ms sleep is inserted between consecutive provider requests to stay below upstream rate-limit thresholds. Tests disable it via `Service.SetRefreshSleep(0)`.

**Result aggregation.** `RefreshPrices` returns a `RefreshResult` with one entry per security processed: `{Ticker, Outcome, Date, Price, Error, Note}`. Outcomes are `updated`, `up_to_date`, `skipped_hidden`, `skipped_no_ticker`, `skipped_currency_mismatch`, `failed`. The CLI prints a tabwriter table plus a one-line aggregate summary; the TUI posts the summary to the status bar and lists up to three failing tickers when present.

## Prices View

A dedicated view for managing pricing data.

### Layout

- **Security selector**: Dropdown or search field to select a security by ticker or name
- **Price table**: Date | Price | Source — sorted by date descending
- **Add price form**: Date field + Price field + Save button

### Operations

| Action | Description |
|--------|-------------|
| Select security | Filter price table to show prices for selected security |
| Add price | Enter date and price for the selected security |
| Edit price | Modify an existing price entry |
| Delete price | Remove a price entry |
| Bulk import | Open CSV import dialog (see Bulk Import) |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `n` | New price entry |
| `Enter` | Edit selected price |
| `d` | Delete selected price |
| `i` | Open bulk import dialog |
| `/` | Search/filter by security |

## Investment Account Enhancements

### New Account Properties

| Property | Type | Applies To | Description |
|----------|------|------------|-------------|
| `track_lots` | boolean | investment | Whether to track lot-level detail (default: false) |

When `track_lots = false`:
- Buy/sell transactions adjust a running share count per security
- No lot selection on sells
- Cost basis tracked as a weighted average across all purchases
- Simpler workflow for retirement and managed accounts

When `track_lots = true`:
- Each buy creates a distinct lot
- Sells require lot selection (full or partial)
- Cost basis tracked per lot
- Supports tax-loss harvesting strategies

### Cash Position

Investment accounts maintain a cash position alongside security holdings. Cash is tracked as a built-in position (not a security in the security master) and behaves as follows:

| Operation | Cash Effect |
|-----------|-------------|
| Deposit | Increases cash |
| Withdrawal | Decreases cash |
| Buy | Decreases cash by total cost (shares × price + commission) |
| Sell | Increases cash by total proceeds (shares × price - commission) |
| Dividend (cash) | Increases cash |
| Reinvest Dividend | No cash effect (shares received, no cash movement) |
| Fee | Decreases cash |
| Fee (via liquidation) | No cash effect (shares sold, proceeds offset fee) |
| Interest | Increases cash |
| Transfer in (cash) | Increases cash |
| Transfer out (cash) | Decreases cash |
| Transfer in (shares) | No cash effect |
| Transfer out (shares) | No cash effect |

### Account Valuation

Total account value = Cash balance + Market value of all holdings

Market value of holdings = Σ (shares held × most recent price) for each security

If no price exists for a security, use the cost basis as the estimated value and flag it as "no pricing data."

## Investment Transaction Types

Investment accounts use a distinct set of transaction types beyond the standard transaction model.

### Transaction Type Properties

All investment transactions share these base properties:

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Investment account |
| `date` | date | Yes | Transaction date |
| `transaction_type` | enum | Yes | See types below |
| `security_id` | UUID | Varies | Reference to security (not needed for cash-only transactions) |
| `shares` | decimal | Varies | Number of shares |
| `price_per_share` | decimal | Varies | Price per share |
| `total_amount` | decimal | Yes | Total cash amount |
| `commission` | decimal | No | Commission/fee for the trade (default: 0) |
| `memo` | string | No | User notes |
| `status` | enum | Yes | pending, cleared, reconciled |
| `lot_id` | UUID | No | Specific lot reference (for sells in lot-tracking accounts) |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Smart Field Computation

When entering a buy or sell transaction, the system auto-computes fields based on what the user enters:

| User Enters | System Computes |
|-------------|-----------------|
| total_amount + shares | price_per_share = (total_amount - commission) / shares |
| total_amount + shares + commission | price_per_share = (total_amount - commission) / shares |
| shares + price_per_share | total_amount = (shares × price_per_share) + commission |
| shares + price_per_share + commission | total_amount = (shares × price_per_share) + commission |

The user must provide at least shares plus one of (total_amount, price_per_share). Commission is always optional.

### Transaction Types

#### Buy

Purchase shares of a security.

- Creates a new lot (if account tracks lots)
- Adds to running share count (if account does not track lots)
- Deducts total_amount from cash position
- Auto-creates a price record for the security on the transaction date

#### Sell

Sell shares of a security.

- In lot-tracking accounts: user selects which lot(s) to sell from, can sell partial lots
- In non-lot-tracking accounts: reduces running share count
- Adds total_amount to cash position (proceeds after commission)
- Auto-creates a price record for the security on the transaction date

Lot selection flow (lot-tracking accounts):
1. User enters security, shares to sell, total amount received, commission
2. System shows available lots: Purchase Date | Shares Available | Cost Per Share | Cost Basis
3. User selects lot(s) and specifies shares from each lot
4. Total shares selected must equal shares to sell
5. System records which lots were reduced

#### Dividend (Cash)

Cash dividend received for a security.

- Increases cash position by dividend amount
- Security and share count are unchanged
- Records dividend per share if provided

#### Reinvest Dividend

Dividend automatically reinvested as new shares (common in mutual funds).

- No cash movement
- Creates a new lot (lot-tracking) or adds to share count (non-lot-tracking)
- Records shares received and price per share
- Auto-creates a price record for the security on the transaction date

#### Fee

Management fee or other account-level fee.

- Deducts from cash position
- No security or share impact
- Can have a memo describing the fee type

#### Fee via Liquidation

Fee paid by selling shares (common in mutual funds).

- Records shares sold and fee amount
- No net cash effect (proceeds from share sale exactly offset the fee)
- In lot-tracking accounts: requires lot selection (same as a sell)
- Reduces share count

#### Deposit

Cash deposited into the investment account.

- Increases cash position
- No security impact
- Can be linked as a transfer from another TMoney account (e.g., checking)

#### Withdrawal

Cash withdrawn from the investment account.

- Decreases cash position
- No security impact
- Can be linked as a transfer to another TMoney account

#### Interest

Interest earned on cash balance (sweep account interest).

- Increases cash position
- No security impact

#### Transfer (Shares)

Transfer shares of a security between investment accounts.

- Source account: reduces shares (removes lots in lot-tracking account)
- Destination account: adds shares (creates new lots with original cost basis if lot-tracking)
- No cash movement in either account
- Lots transfer with their original purchase date and cost basis intact
- If destination account does not track lots, shares are added to running count and cost basis is aggregated

#### Transfer (Cash)

Transfer cash between an investment account and any other account.

- Uses the existing transfer mechanism
- Creates paired transactions in both accounts

#### Exchange

Used for corporate actions where shares of one security are converted to shares of another.

- Removes shares of source security
- Adds shares of target security
- Cost basis transfers from source to target lots
- Optional cash component (recorded separately)
- No net cash effect for the share exchange portion
- See Corporate Actions for full detail

## Lots

### Lot Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Parent investment account |
| `security_id` | UUID | Yes | Reference to security |
| `shares` | decimal | Yes | Current number of shares in this lot |
| `original_shares` | decimal | Yes | Shares when lot was created (for history) |
| `cost_per_share` | decimal | Yes | Original cost per share |
| `purchase_date` | date | Yes | Date the lot was acquired |
| `source_transaction_id` | UUID | Yes | Transaction that created this lot |
| `closed` | boolean | Yes | Whether all shares have been sold (default: false) |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Lot Behavior

- A lot is created by: buy, reinvest dividend, exchange (target side), or share transfer (destination)
- A lot is reduced by: sell, fee via liquidation, exchange (source side), or share transfer (source)
- When `shares` reaches 0, the lot is marked `closed = true`
- Closed lots are retained for historical reporting
- Cost basis for a lot = `shares × cost_per_share`
- Total cost basis for a security = Σ (lot cost basis) across all open lots

### Non-Lot-Tracking Positions

For accounts where `track_lots = false`, positions are tracked at the security level:

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Parent investment account |
| `security_id` | UUID | Yes | Reference to security |
| `shares` | decimal | Yes | Total shares held |
| `average_cost_per_share` | decimal | Yes | Weighted average cost basis per share |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

Average cost is recalculated on each buy:
```
new_average = ((old_shares × old_average) + (new_shares × new_price)) / (old_shares + new_shares)
```

## Portfolio Display

### Account-Level Portfolio View

When viewing an investment account, the portfolio shows:

#### Summary Bar

| Field | Value |
|-------|-------|
| Cash Balance | Current cash in account |
| Market Value | Total market value of holdings |
| Total Value | Cash + Market Value |
| Total Cost Basis | Sum of cost basis for all holdings |
| Total Gain/Loss | Market Value - Total Cost Basis |
| Total Gain/Loss % | (Market Value - Total Cost Basis) / Total Cost Basis × 100 |

#### Holdings Table (Rolled Up by Security)

| Column | Description |
|--------|-------------|
| Ticker | Security ticker symbol |
| Name | Security name |
| Shares | Total shares held |
| Avg Cost | Average cost per share (weighted across lots) |
| Current Price | Most recent price |
| Price Date | Date of the most recent price |
| Market Value | Shares × Current Price |
| Cost Basis | Total cost basis |
| Gain/Loss ($) | Market Value - Cost Basis |
| Gain/Loss (%) | (Market Value - Cost Basis) / Cost Basis × 100 |

Securities with no pricing data display cost basis as estimated value with a visual indicator (e.g., "~" prefix or different color).

#### Lot Detail View (Lot-Tracking Accounts Only)

Accessible by selecting a security and drilling down:

| Column | Description |
|--------|-------------|
| Purchase Date | When the lot was acquired |
| Shares | Shares remaining in this lot |
| Cost/Share | Original purchase price per share |
| Cost Basis | Shares × Cost/Share |
| Current Value | Shares × Current Price |
| Gain/Loss ($) | Current Value - Cost Basis |
| Gain/Loss (%) | (Current Value - Cost Basis) / Cost Basis × 100 |

## Corporate Actions

### Stock Split / Reverse Split

Adjusts share count and cost basis across all lots (or positions) for a security.

**Input:**
- Security
- Split date
- Split ratio (e.g., 4:1 for a 4-for-1 split, 1:10 for a 1-for-10 reverse split)

**Effect:**
- For each lot (or position): `shares = shares × ratio`, `cost_per_share = cost_per_share / ratio`
- `original_shares` on lots is NOT modified (preserves history)
- A price adjustment is applied: all prices on or before the split date are divided by the ratio
- A record of the split is stored for audit purposes

### Merger / Acquisition (Exchange)

Converts shares of an acquired company into shares of the acquiring company.

**Input:**
- Source security (acquired company)
- Target security (acquiring company)
- Exchange date
- Share exchange ratio (e.g., 2 shares OLD = 1 share NEW)
- Cash per share (optional, for cash+stock deals)

**Effect (per account holding the source security):**
1. For each lot of source security:
   - Calculate new shares: `old_shares / exchange_ratio` (or `old_shares × target_per_source`)
   - Transfer cost basis: new lot cost_per_share = `(old_cost_per_share × old_shares) / new_shares`
   - Create exchange transaction removing source shares
   - Create exchange transaction adding target shares with new lots
2. If cash consideration:
   - Add cash per share × old shares to cash position
   - Record as part of the exchange transaction
3. Source security is eligible to be hidden after all positions are exchanged

### Spin-Off

A parent company spins off a division as a new publicly traded security. Cost basis is allocated between parent and spin-off based on relative market values.

**Input:**
- Parent security
- Spin-off security (must be pre-registered)
- Spin-off date
- Share ratio (e.g., 1 share of parent = 0.25 shares of spin-off)
- Cost basis allocation percentage to parent (e.g., 80%)
  - Remaining percentage (e.g., 20%) goes to spin-off
  - User must enter this based on IRS guidelines (relative market values on first trading day)

**Effect (per account holding the parent security):**
1. For each lot of parent security:
   - Reduce cost_per_share: `new_parent_cost = old_cost × (parent_allocation_pct / 100)`
   - Create new lot for spin-off:
     - shares = parent_lot_shares × spin_off_ratio
     - cost_per_share = `(old_cost × (1 - parent_allocation_pct / 100) × parent_lot_shares) / spin_off_shares`
     - purchase_date = original parent lot purchase_date (preserves holding period)
2. If fractional shares result:
   - Round down to nearest whole share (or supported fraction)
   - Record cash-in-lieu for fractional portion at spin-off price
   - Cash-in-lieu is a small taxable event
3. Create a price record for the spin-off security on the spin-off date

### Corporate Action Audit Log

All corporate actions are recorded in an audit table:

| Property | Type | Description |
|----------|------|-------------|
| `id` | UUID | Unique identifier |
| `action_type` | enum | split, reverse_split, merger, spin_off |
| `security_id` | UUID | Primary security affected |
| `target_security_id` | UUID | Target/spin-off security (if applicable) |
| `action_date` | date | Date of the corporate action |
| `parameters` | JSON | Action-specific parameters (ratio, percentages, etc.) |
| `created_at` | timestamp | When the action was recorded |

## Security Management View

A dedicated view for managing the security master.

### Layout

- **Security table**: Ticker | Name | Type | Asset Class | Currency | Status (Active/Hidden)
- **Filter/search bar**: Search by ticker or name, filter by type/asset_class/hidden status
- **Detail panel**: Shows full security details when selected

### Operations

| Action | Description |
|--------|-------------|
| Add | Create a new security |
| Edit | Modify security properties |
| Hide | Mark security as hidden (requires no open positions) |
| Unhide | Reactivate a hidden security |
| Delete | Remove security (only if no history) |
| Merge | Initiate a merger workflow |
| View prices | Navigate to prices view filtered to this security |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `n` | New security |
| `Enter` | Edit selected security |
| `h` | Toggle hidden status |
| `d` | Delete security (if eligible) |
| `m` | Merge security |
| `p` | View prices for selected security |
| `/` | Search securities |
| `f` | Toggle show/hide hidden securities |

### Add/Edit Security Dialog

Fields:
1. Ticker (text input)
2. Name (text input)
3. Type (dropdown: Stock, ETF, Mutual Fund, Other)
4. Asset Class (dropdown: see asset classes above)
5. Currency (dropdown, default USD)
6. Exchange (text input, optional)

## Bulk Price Import

### CSV Format

```csv
date,ticker,price
2024-01-15,AAPL,185.92
2024-01-15,GDX,28.45
2024-01-16,AAPL,187.44
2024-01-16,GDX,28.12
```

Requirements:
- Header row is required
- Column order: date, ticker, price
- Date format: YYYY-MM-DD
- Ticker must match an existing security (securities must be pre-registered)
- Price must be positive

### Import Behavior

1. Parse CSV file
2. Validate all rows before importing any:
   - All tickers must exist in the security master
   - All dates must be valid and not in the future
   - All prices must be positive numbers
3. Report validation errors with line numbers
4. For valid files, import with conflict resolution:
   - If a price already exists for a security+date: **skip** (existing data preserved)
   - Option to force overwrite existing prices
5. Report import summary: total rows, imported, skipped, errors
6. All imported prices use `source = 'import'`

### Import Dialog

1. File selector for CSV file
2. Preview of first 10 rows
3. Validation results (errors highlighted)
4. Conflict resolution option: Skip existing / Overwrite existing
5. Import button
6. Results summary after import

## CLI Commands

### Security Management

```bash
# List all securities
tmoney --list-securities
tmoney --list-securities --include-hidden
tmoney --list-securities --type etf
tmoney --list-securities --asset-class large_cap_stock

# Show security details
tmoney --security AAPL

# Add a security
tmoney --add-security --ticker AAPL --name "Apple Inc." --type stock --asset-class large_cap_stock
tmoney --add-security --ticker RY.TO --name "Royal Bank of Canada" --type stock --currency CAD --exchange TSX

# Edit a security
tmoney --edit-security AAPL --asset-class large_cap_stock
tmoney --edit-security OLD --ticker NEW  # Ticker change

# Hide/unhide a security
tmoney --hide-security DELISTED_CO
tmoney --unhide-security DELISTED_CO

# Delete a security (only if no history)
tmoney --delete-security UNUSED_TICKER
```

### Price Management

```bash
# List prices for a security
tmoney --prices AAPL
tmoney --prices AAPL --from 2024-01-01 --to 2024-03-31

# Add a price
tmoney --add-price --ticker AAPL --date 2024-03-15 --price 172.50

# Bulk import prices from CSV
tmoney --import-prices prices.csv
tmoney --import-prices prices.csv --overwrite  # Overwrite existing

# Get current price (most recent)
tmoney --current-price AAPL
```

### Investment Transactions

```bash
# Buy shares
tmoney -f portfolio.tdb --buy --account "Brokerage" \
  --ticker AAPL --shares 10 --total 1850.00
tmoney -f portfolio.tdb --buy --account "Brokerage" \
  --ticker AAPL --shares 10 --price-per-share 185.00 --commission 4.95

# Sell shares
tmoney -f portfolio.tdb --sell --account "Brokerage" \
  --ticker AAPL --shares 5 --total 950.00
tmoney -f portfolio.tdb --sell --account "Brokerage" \
  --ticker AAPL --shares 5 --total 950.00 --lot <lot-id>

# Dividend
tmoney -f portfolio.tdb --dividend --account "Brokerage" \
  --ticker AAPL --amount 47.50

# Reinvest dividend
tmoney -f portfolio.tdb --reinvest --account "IRA" \
  --ticker VTSAX --shares 0.234 --total 55.00

# Fee
tmoney -f portfolio.tdb --investment-fee --account "IRA" --amount 25.00 \
  --memo "Quarterly management fee"

# Deposit/withdraw cash
tmoney -f portfolio.tdb --deposit --account "Brokerage" --amount 5000.00
tmoney -f portfolio.tdb --withdraw --account "Brokerage" --amount 1000.00

# Transfer shares between accounts
tmoney -f portfolio.tdb --transfer-shares --from "Brokerage" --to "IRA" \
  --ticker AAPL --shares 10

# Portfolio view
tmoney -f portfolio.tdb --portfolio --account "Brokerage"
tmoney -f portfolio.tdb --portfolio --account "Brokerage" --show-lots
```

### Corporate Actions

```bash
# Stock split
tmoney --split --ticker AAPL --date 2024-08-01 --ratio 4:1

# Reverse split
tmoney --split --ticker XYZ --date 2024-08-01 --ratio 1:10

# Merger
tmoney --merge-security --source OLD_TICKER --target NEW_TICKER \
  --date 2024-06-15 --ratio 2:1
tmoney --merge-security --source OLD_TICKER --target NEW_TICKER \
  --date 2024-06-15 --ratio 2:1 --cash-per-share 5.00

# Spin-off
tmoney --spin-off --parent PARENT_TICKER --spinoff NEW_TICKER \
  --date 2024-06-15 --share-ratio 0.25 --parent-allocation 80
```

## Database Schema

### securities

```sql
CREATE TABLE securities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticker TEXT NOT NULL,
    name TEXT NOT NULL,
    security_type TEXT NOT NULL CHECK (security_type IN (
        'stock', 'etf', 'mutual_fund', 'other'
    )),
    asset_class TEXT NOT NULL DEFAULT 'unclassified' CHECK (asset_class IN (
        'large_cap_stock', 'small_cap_stock', 'international_stock',
        'index', 'domestic_bond', 'foreign_bond', 'cash',
        'commodity', 'crypto', 'asset_mixture', 'unclassified'
    )),
    currency TEXT NOT NULL DEFAULT 'USD',
    exchange TEXT,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (ticker, currency)
);

CREATE INDEX idx_securities_ticker ON securities(ticker);
CREATE INDEX idx_securities_type ON securities(security_type);
CREATE INDEX idx_securities_hidden ON securities(hidden);
```

### security_prices

```sql
CREATE TABLE security_prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    security_id UUID NOT NULL REFERENCES securities(id),
    date DATE NOT NULL,
    price DECIMAL(19, 4) NOT NULL CHECK (price > 0),
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN (
        'manual', 'transaction', 'import', 'api'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (security_id, date)
);

CREATE INDEX idx_prices_security ON security_prices(security_id);
CREATE INDEX idx_prices_date ON security_prices(date);
CREATE INDEX idx_prices_security_date ON security_prices(security_id, date);
```

### investment_lots (revised)

Replaces the existing `investment_lots` table:

```sql
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL REFERENCES securities(id),
    shares DECIMAL(19, 8) NOT NULL,
    original_shares DECIMAL(19, 8) NOT NULL,
    cost_per_share DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    source_transaction_id UUID NOT NULL,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_security ON investment_lots(security_id);
CREATE INDEX idx_lots_closed ON investment_lots(closed);
```

### investment_positions

For non-lot-tracking accounts:

```sql
CREATE TABLE investment_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL REFERENCES securities(id),
    shares DECIMAL(19, 8) NOT NULL DEFAULT 0,
    average_cost_per_share DECIMAL(19, 4) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, security_id)
);

CREATE INDEX idx_positions_account ON investment_positions(account_id);
CREATE INDEX idx_positions_security ON investment_positions(security_id);
```

### investment_transactions

```sql
CREATE TABLE investment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN (
        'buy', 'sell', 'dividend', 'reinvest_dividend',
        'fee', 'fee_liquidation', 'deposit', 'withdrawal',
        'interest', 'transfer_shares', 'transfer_cash', 'exchange'
    )),
    security_id UUID REFERENCES securities(id),
    shares DECIMAL(19, 8),
    price_per_share DECIMAL(19, 4),
    total_amount DECIMAL(19, 4) NOT NULL,
    commission DECIMAL(19, 4) DEFAULT 0,
    memo TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'cleared', 'reconciled'
    )),
    transfer_id UUID,
    transfer_account_id UUID REFERENCES accounts(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_inv_tx_account ON investment_transactions(account_id);
CREATE INDEX idx_inv_tx_date ON investment_transactions(date);
CREATE INDEX idx_inv_tx_type ON investment_transactions(transaction_type);
CREATE INDEX idx_inv_tx_security ON investment_transactions(security_id);
CREATE INDEX idx_inv_tx_transfer ON investment_transactions(transfer_id);
```

### investment_transaction_lots

Junction table linking sell/exchange transactions to specific lots:

```sql
CREATE TABLE investment_transaction_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES investment_transactions(id) ON DELETE CASCADE,
    lot_id UUID NOT NULL REFERENCES investment_lots(id),
    shares DECIMAL(19, 8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tx_lots_transaction ON investment_transaction_lots(transaction_id);
CREATE INDEX idx_tx_lots_lot ON investment_transaction_lots(lot_id);
```

### corporate_actions

```sql
CREATE TABLE corporate_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type TEXT NOT NULL CHECK (action_type IN (
        'split', 'reverse_split', 'merger', 'spin_off'
    )),
    security_id UUID NOT NULL REFERENCES securities(id),
    target_security_id UUID REFERENCES securities(id),
    action_date DATE NOT NULL,
    parameters TEXT NOT NULL,  -- JSON-encoded action parameters
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_corp_actions_security ON corporate_actions(security_id);
CREATE INDEX idx_corp_actions_date ON corporate_actions(action_date);
```

### accounts table modification

Add `track_lots` column:

```sql
ALTER TABLE accounts ADD COLUMN track_lots BOOLEAN DEFAULT FALSE;
```

This column is only meaningful for investment-type accounts.

## Database Views

### portfolio_holdings

```sql
CREATE VIEW portfolio_holdings AS
SELECT
    a.id AS account_id,
    a.name AS account_name,
    s.id AS security_id,
    s.ticker,
    s.name AS security_name,
    CASE
        WHEN a.track_lots THEN
            (SELECT COALESCE(SUM(l.shares), 0) FROM investment_lots l
             WHERE l.account_id = a.id AND l.security_id = s.id AND NOT l.closed)
        ELSE
            (SELECT COALESCE(p.shares, 0) FROM investment_positions p
             WHERE p.account_id = a.id AND p.security_id = s.id)
    END AS total_shares,
    CASE
        WHEN a.track_lots THEN
            (SELECT COALESCE(SUM(l.shares * l.cost_per_share), 0) FROM investment_lots l
             WHERE l.account_id = a.id AND l.security_id = s.id AND NOT l.closed)
        ELSE
            (SELECT COALESCE(p.shares * p.average_cost_per_share, 0) FROM investment_positions p
             WHERE p.account_id = a.id AND p.security_id = s.id)
    END AS total_cost_basis
FROM accounts a
CROSS JOIN securities s
WHERE a.type = 'investment' AND a.active = TRUE;
```

## Net Worth Integration

Investment accounts contribute to net worth as follows:

- **Assets**: Cash balance + Market value of all holdings
- The existing net worth report should include investment account total values alongside checking, savings, etc.
- If an investment account has holdings with no pricing data, the cost basis is used as a conservative estimate

## Future Considerations (Out of Scope)

- Automatic price fetching from Yahoo Finance / Google Finance APIs
- Options and derivatives tracking
- Tax lot reporting (Form 8949 generation)
- Dividend reinvestment plans (DRIP) with fractional share precision beyond 8 decimal places
- Multi-currency price conversion
- Real-time price streaming
- Performance metrics (IRR, time-weighted return)
