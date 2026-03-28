# Implementation Plan: Security Master

This document defines the order in which Security Master features should be implemented. Each item represents one small session of work following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Spec: `specs/security-master.md`

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

1. **Security & Price Models** — foundational types; everything else depends on these
2. **Security Repository & Service** — CRUD for the security master; prerequisite for prices, transactions, and all views
3. **Price Repository & Service** — pricing data is needed for valuations; price provider interface enables future API integration
4. **Account Enhancement (track_lots)** — small schema change needed before investment transactions
5. **Investment Transaction Models** — define the domain before persistence
6. **Lot & Position Models** — cost basis tracking models
7. **Investment Transaction Repository & Service** — core buy/sell/dividend logic
8. **Cash Position Tracking** — investment accounts need cash balance alongside holdings
9. **Smart Field Computation** — UX improvement for transaction entry
10. **Price Auto-Creation** — prices derived from transactions reduce manual entry
11. **CLI: Securities & Prices** — scriptable access before TUI work
12. **CLI: Investment Transactions & Portfolio** — complete CLI coverage
13. **Bulk Price Import** — CSV import for historical price data
14. **TUI: Security Management View** — visual security CRUD
15. **TUI: Prices View** — visual price management
16. **TUI: Investment Register & Portfolio** — the main investment account experience
17. **Corporate Actions Models & Migration** — schema for splits, mergers, spin-offs
18. **Corporate Actions Services** — complex business logic for each action type
19. **Corporate Actions CLI & TUI** — user-facing corporate action workflows
20. **Net Worth Integration** — investment accounts contribute to existing net worth report
21. **Portfolio Holdings View** — database view for efficient portfolio queries

---

## Phase 1: Security Model

- [x] **SM-001 - Security type enum and validation**
  - RED: Write tests for `SecurityType` enum — valid types (`stock`, `etf`, `mutual_fund`, `other`), invalid type rejected, `IsValid()`, `DisplayName()`
  - GREEN: Implement `SecurityType` in `internal/models/security.go`

- [x] **SM-002 - Asset class enum and validation**
  - RED: Write tests for `AssetClass` enum — all 11 valid values, invalid rejected, `IsValid()`, `DisplayName()`
  - GREEN: Implement `AssetClass` in `internal/models/security.go`

- [x] **SM-003 - Security model struct and validation**
  - RED: Write tests for `Security` struct validation — required ticker (max 20 chars), required name, required security_type, required asset_class, valid currency, default values (currency=USD, asset_class=unclassified, hidden=false)
  - GREEN: Implement `Security` struct with `Validate()` method using existing `Validator` pattern

- [x] **SM-004 - Security model helper methods**
  - RED: Write tests for `Security.CanHide()` (placeholder — returns true when no positions exist, tested more fully after positions are built), `Security.CanDelete()` (placeholder)
  - GREEN: Implement helper methods on `Security`

## Phase 2: Price Model

- [x] **SM-005 - Price source enum**
  - RED: Write tests for `PriceSource` enum — valid sources (`manual`, `transaction`, `import`, `api`), invalid rejected, `IsValid()`
  - GREEN: Implement `PriceSource` in `internal/models/price.go`

- [x] **SM-006 - Security price model and validation**
  - RED: Write tests for `SecurityPrice` struct — required security_id, required date (not future), required price (must be positive), required source, unique constraint concept (security_id + date)
  - GREEN: Implement `SecurityPrice` struct with `Validate()` method

## Phase 3: Securities Database Migration

- [ ] **SM-007 - Migration: securities table**
  - RED: Write integration test that runs migration, inserts a security, and reads it back verifying all columns
  - GREEN: Create migration `006_securities.sql` with `securities` table and indexes per spec schema

- [ ] **SM-008 - Migration: security_prices table**
  - RED: Write integration test that inserts a price for an existing security, verifies unique constraint on (security_id, date), verifies price > 0 check
  - GREEN: Add `security_prices` table and indexes to migration `006_securities.sql`

## Phase 4: Security Repository

- [ ] **SM-009 - SecurityRepository.Create**
  - RED: Write test — create security, verify returned ID, verify all fields persisted; test duplicate ticker+currency rejected
  - GREEN: Implement `Create` in `internal/repository/security.go`

- [ ] **SM-010 - SecurityRepository.GetByID**
  - RED: Write test — get existing security by ID returns all fields; get non-existent ID returns NotFoundError
  - GREEN: Implement `GetByID`

- [ ] **SM-011 - SecurityRepository.GetByTicker**
  - RED: Write test — get security by ticker returns match; get by ticker+currency when multiple currencies exist; not found returns error
  - GREEN: Implement `GetByTicker`

- [ ] **SM-012 - SecurityRepository.List with filters**
  - RED: Write tests — list all securities; list excluding hidden; filter by security_type; filter by asset_class; filter by hidden status; combined filters
  - GREEN: Implement `List` with `SecurityFilter` struct

- [ ] **SM-013 - SecurityRepository.Update**
  - RED: Write test — update all mutable fields (ticker, name, type, asset_class, currency, exchange, hidden); verify updated_at changes; verify non-existent ID returns error
  - GREEN: Implement `Update` using delete+insert pattern per project convention

- [ ] **SM-014 - SecurityRepository.Delete**
  - RED: Write test — delete security with no history succeeds; delete security with prices fails with HasDependentsError; delete security with transactions fails
  - GREEN: Implement `Delete` with dependency checks

## Phase 5: Security Service

- [ ] **SM-015 - SecurityService.Create with validation**
  - RED: Write test — valid security created; invalid security (empty ticker) returns validation error; duplicate ticker+currency returns error
  - GREEN: Implement `SecurityService.Create` in `internal/service/security.go`

- [ ] **SM-016 - SecurityService.Update**
  - RED: Write test — update fields; ticker change allowed; validation errors returned
  - GREEN: Implement `SecurityService.Update`

- [ ] **SM-017 - SecurityService.Hide and Unhide**
  - RED: Write test — hide security with no positions succeeds; hide security with open positions fails; unhide works unconditionally
  - GREEN: Implement `Hide` and `Unhide` methods (position check is a stub returning true until positions are built)

- [ ] **SM-018 - SecurityService.Delete**
  - RED: Write test — delete with no history succeeds; delete with prices/transactions suggests hiding instead
  - GREEN: Implement `SecurityService.Delete`

- [ ] **SM-019 - SecurityService.Search**
  - RED: Write test — search by partial ticker match; search by partial name match; case-insensitive
  - GREEN: Implement `SecurityService.Search`

- [ ] **SM-020 - Register SecurityService in service registry**
  - RED: Write test — `NewServices()` returns non-nil SecurityService
  - GREEN: Add SecurityService to `Services` struct and `NewServices()` in `registry.go`

## Phase 6: Price Repository

- [ ] **SM-021 - PriceRepository.Create**
  - RED: Write test — create price, verify fields; duplicate security_id+date returns DuplicateError; invalid security_id returns error
  - GREEN: Implement `Create` in `internal/repository/price.go`

- [ ] **SM-022 - PriceRepository.CreateOrUpdate (upsert)**
  - RED: Write test — insert new price; update existing price for same security+date; verify source is updated
  - GREEN: Implement `CreateOrUpdate` for overwrite scenarios

- [ ] **SM-023 - PriceRepository.GetBySecurityAndDate**
  - RED: Write test — exact match returns price; no match returns NotFoundError
  - GREEN: Implement `GetBySecurityAndDate`

- [ ] **SM-024 - PriceRepository.GetCurrentPrice**
  - RED: Write test — returns most recent price on or before given date; no price before date returns NotFoundError; respects security_id filter
  - GREEN: Implement `GetCurrentPrice`

- [ ] **SM-025 - PriceRepository.GetPriceHistory**
  - RED: Write test — returns all prices for security ordered by date desc; optional date range filter; empty result returns empty slice (not error)
  - GREEN: Implement `GetPriceHistory`

- [ ] **SM-026 - PriceRepository.Delete**
  - RED: Write test — delete existing price succeeds; delete non-existent returns NotFoundError
  - GREEN: Implement `Delete`

- [ ] **SM-027 - PriceRepository.BulkCreate**
  - RED: Write test — insert multiple prices in one call; skip duplicates when skipExisting=true; overwrite duplicates when overwrite=true; return import summary (total, imported, skipped)
  - GREEN: Implement `BulkCreate`

## Phase 7: Price Provider Interface

- [ ] **SM-028 - Define PriceProvider interface**
  - RED: Write test — verify manual provider implements interface; test `FetchPrice` returns a price for a known security+date
  - GREEN: Define `PriceProvider` interface in `internal/service/price_provider.go` with methods: `FetchPrice(ticker string, date Date) (*SecurityPrice, error)`, `FetchPriceHistory(ticker string, from, to Date) ([]SecurityPrice, error)`, `Name() string`
  - Implement `ManualPriceProvider` as a no-op/passthrough (returns error "manual entry required")

- [ ] **SM-029 - Price provider registry**
  - RED: Write test — register provider by name, retrieve by name, list available providers
  - GREEN: Implement `PriceProviderRegistry` that holds named providers and allows selection

## Phase 8: Price Service

- [ ] **SM-030 - PriceService.AddPrice**
  - RED: Write test — add price for valid security+date; reject future date; reject non-positive price; handle duplicate (return conflict info)
  - GREEN: Implement `PriceService.AddPrice` in `internal/service/price.go`

- [ ] **SM-031 - PriceService.UpdatePrice**
  - RED: Write test — update existing price; reject invalid values
  - GREEN: Implement `PriceService.UpdatePrice`

- [ ] **SM-032 - PriceService.GetCurrentPrice**
  - RED: Write test — returns most recent price on or before date; returns error when no price exists
  - GREEN: Implement `PriceService.GetCurrentPrice`

- [ ] **SM-033 - PriceService.GetPriceHistory**
  - RED: Write test — returns prices in date range; empty range returns all
  - GREEN: Implement `PriceService.GetPriceHistory`

- [ ] **SM-034 - PriceService.DeletePrice**
  - RED: Write test — delete price; verify no cascade effects
  - GREEN: Implement `PriceService.DeletePrice`

- [ ] **SM-035 - Register PriceService in service registry**
  - RED: Write test — `NewServices()` returns non-nil PriceService
  - GREEN: Add PriceService to `Services` struct and `NewServices()`

## Phase 9: Account Enhancement (track_lots)

- [ ] **SM-036 - Add TrackLots field to Account model**
  - RED: Write test — Account with TrackLots=true validates; TrackLots only meaningful for investment type; default is false
  - GREEN: Add `TrackLots` field to `Account` struct in `internal/models/account.go`

- [ ] **SM-037 - Migration: add track_lots column to accounts**
  - RED: Write integration test — existing accounts get track_lots=false; new investment account can set track_lots=true
  - GREEN: Create migration `007_account_track_lots.sql`

- [ ] **SM-038 - Update AccountRepository for track_lots**
  - RED: Write test — create account with track_lots=true; read back verifies field; update track_lots
  - GREEN: Update Create/Update/scan methods in account repository

## Phase 10: Investment Transaction Model

- [ ] **SM-039 - InvestmentTransactionType enum**
  - RED: Write tests for all 12 types (`buy`, `sell`, `dividend`, `reinvest_dividend`, `fee`, `fee_liquidation`, `deposit`, `withdrawal`, `interest`, `transfer_shares`, `transfer_cash`, `exchange`), invalid rejected, `IsValid()`, `RequiresSecurity()`, `RequiresShares()`, `AffectsCash()`
  - GREEN: Implement `InvestmentTransactionType` in `internal/models/investment_transaction.go`

- [ ] **SM-040 - InvestmentTransaction model and validation**
  - RED: Write tests — required fields (account_id, date, type, total_amount); security_id required for security-based types; shares required for share-based types; price_per_share optional; commission >= 0; status enum (pending, cleared, reconciled)
  - GREEN: Implement `InvestmentTransaction` struct with `Validate()`

- [ ] **SM-041 - InvestmentTransaction status methods**
  - RED: Write tests — `Clear()`, `Reconcile()`, status transitions
  - GREEN: Implement status methods mirroring existing transaction patterns

## Phase 11: Lot Model

- [ ] **SM-042 - Lot model and validation**
  - RED: Write tests — required fields (account_id, security_id, shares > 0, original_shares > 0, cost_per_share > 0, purchase_date, source_transaction_id); closed default false; `CostBasis()` returns shares × cost_per_share; `IsFullyClosed()` returns shares == 0
  - GREEN: Implement `Lot` struct in `internal/models/lot.go`

- [ ] **SM-043 - Lot reduce and close logic**
  - RED: Write tests — `Reduce(shares)` decreases shares; reducing to 0 sets closed=true; reducing more than available returns error; reducing negative amount returns error
  - GREEN: Implement `Reduce()` method on `Lot`

## Phase 12: Position Model

- [ ] **SM-044 - Position model and validation (non-lot-tracking)**
  - RED: Write tests — required fields (account_id, security_id); shares >= 0; average_cost_per_share >= 0; `CostBasis()` returns shares × average_cost_per_share; `MarketValue(currentPrice)` returns shares × currentPrice
  - GREEN: Implement `Position` struct in `internal/models/position.go`

- [ ] **SM-045 - Position average cost recalculation**
  - RED: Write tests — `AddShares(newShares, pricePerShare)` recalculates weighted average; `RemoveShares(shares)` reduces count without changing average; removing more than held returns error
  - GREEN: Implement `AddShares()` and `RemoveShares()` on `Position`

## Phase 13: Investment Tables Migration

- [ ] **SM-046 - Migration: investment_transactions table**
  - RED: Write integration test — insert investment transaction, read back, verify all columns and constraints
  - GREEN: Create migration `008_investment_tables.sql` with `investment_transactions` table and indexes

- [ ] **SM-047 - Migration: investment_lots table**
  - RED: Write integration test — insert lot, verify columns; verify foreign keys to accounts and securities
  - GREEN: Add `investment_lots` table to migration `008_investment_tables.sql`

- [ ] **SM-048 - Migration: investment_positions table**
  - RED: Write integration test — insert position, verify unique constraint on (account_id, security_id)
  - GREEN: Add `investment_positions` table to migration `008_investment_tables.sql`

- [ ] **SM-049 - Migration: investment_transaction_lots junction table**
  - RED: Write integration test — insert junction record linking transaction to lot; verify cascade delete
  - GREEN: Add `investment_transaction_lots` table to migration `008_investment_tables.sql`

## Phase 14: Investment Transaction Repository

- [ ] **SM-050 - InvestmentTransactionRepository.Create**
  - RED: Write test — create transaction, verify returned ID and all fields; verify account_id foreign key enforced
  - GREEN: Implement `Create` in `internal/repository/investment_transaction.go`

- [ ] **SM-051 - InvestmentTransactionRepository.GetByID**
  - RED: Write test — get existing by ID; not found returns error
  - GREEN: Implement `GetByID`

- [ ] **SM-052 - InvestmentTransactionRepository.ListByAccount**
  - RED: Write test — list all transactions for account ordered by date desc; filter by type; filter by date range; filter by security_id
  - GREEN: Implement `ListByAccount` with `InvestmentTransactionFilter` struct

- [ ] **SM-053 - InvestmentTransactionRepository.Update**
  - RED: Write test — update mutable fields; verify updated_at changes
  - GREEN: Implement `Update`

- [ ] **SM-054 - InvestmentTransactionRepository.Delete**
  - RED: Write test — delete transaction; verify cascade deletes junction records in investment_transaction_lots
  - GREEN: Implement `Delete`

## Phase 15: Lot Repository

- [ ] **SM-055 - LotRepository.Create**
  - RED: Write test — create lot, verify all fields; verify foreign keys
  - GREEN: Implement `Create` in `internal/repository/lot.go`

- [ ] **SM-056 - LotRepository.GetByID**
  - RED: Write test — get lot by ID; not found returns error
  - GREEN: Implement `GetByID`

- [ ] **SM-057 - LotRepository.ListByAccountAndSecurity**
  - RED: Write test — list open lots for account+security ordered by purchase_date; option to include closed lots
  - GREEN: Implement `ListByAccountAndSecurity`

- [ ] **SM-058 - LotRepository.Update (reduce shares)**
  - RED: Write test — update shares count; set closed=true when shares=0; verify updated_at changes
  - GREEN: Implement `Update`

- [ ] **SM-059 - LotRepository.GetOpenLotsBySecurity**
  - RED: Write test — returns all open lots across all accounts for a given security (needed for hide validation)
  - GREEN: Implement `GetOpenLotsBySecurity`

## Phase 16: Position Repository

- [ ] **SM-060 - PositionRepository.CreateOrUpdate**
  - RED: Write test — create new position; update existing position for same account+security (upsert)
  - GREEN: Implement `CreateOrUpdate` in `internal/repository/position.go`

- [ ] **SM-061 - PositionRepository.GetByAccountAndSecurity**
  - RED: Write test — get position; not found returns zero-value position (not error)
  - GREEN: Implement `GetByAccountAndSecurity`

- [ ] **SM-062 - PositionRepository.ListByAccount**
  - RED: Write test — list all positions for account; exclude zero-share positions optionally
  - GREEN: Implement `ListByAccount`

- [ ] **SM-063 - PositionRepository.Delete**
  - RED: Write test — delete position when shares reach zero
  - GREEN: Implement `Delete`

## Phase 17: Transaction-Lot Junction Repository

- [ ] **SM-064 - TransactionLotRepository.Create**
  - RED: Write test — link transaction to lot with share count; verify foreign keys
  - GREEN: Implement `Create` in `internal/repository/transaction_lot.go`

- [ ] **SM-065 - TransactionLotRepository.GetByTransaction**
  - RED: Write test — get all lot allocations for a transaction
  - GREEN: Implement `GetByTransaction`

## Phase 18: Investment Transaction Service — Cash Operations

- [ ] **SM-066 - InvestmentTransactionService.Deposit**
  - RED: Write test — deposit increases cash position; verify transaction created; verify account must be investment type
  - GREEN: Implement `Deposit` in `internal/service/investment_transaction.go`

- [ ] **SM-067 - InvestmentTransactionService.Withdrawal**
  - RED: Write test — withdrawal decreases cash position; insufficient cash returns error; verify transaction created
  - GREEN: Implement `Withdrawal`

- [ ] **SM-068 - InvestmentTransactionService.Interest**
  - RED: Write test — interest increases cash position; verify transaction created with correct type
  - GREEN: Implement `Interest`

- [ ] **SM-069 - InvestmentTransactionService.Fee**
  - RED: Write test — fee decreases cash position; insufficient cash returns error; verify memo stored
  - GREEN: Implement `Fee`

- [ ] **SM-070 - Register InvestmentTransactionService in service registry**
  - RED: Write test — `NewServices()` returns non-nil InvestmentTransactionService
  - GREEN: Add to `Services` struct and `NewServices()`

## Phase 19: Cash Position Tracking

- [ ] **SM-071 - CashPosition model**
  - RED: Write test — cash position has account_id and balance; `Deposit(amount)` increases; `Withdraw(amount)` decreases; withdraw more than balance returns error
  - GREEN: Implement cash position tracking (can be a column on the account or a computed value from investment transactions)

- [ ] **SM-072 - CashPosition computation from transactions**
  - RED: Write test — compute cash balance by summing all cash-affecting investment transactions for an account; verify each transaction type's cash effect matches spec table
  - GREEN: Implement `GetCashBalance(accountID)` in investment transaction service

## Phase 20: Smart Field Computation

- [ ] **SM-073 - Compute price_per_share from total and shares**
  - RED: Write test — given total_amount=1850 and shares=10, compute price_per_share=185; with commission=50, compute (1850-50)/10=180
  - GREEN: Implement `ComputePricePerShare` helper function

- [ ] **SM-074 - Compute total_amount from shares and price_per_share**
  - RED: Write test — given shares=10 and price_per_share=185, compute total=1850; with commission=4.95, compute (10×185)+4.95=1854.95
  - GREEN: Implement `ComputeTotalAmount` helper function

- [ ] **SM-075 - Smart computation integration in service**
  - RED: Write test — creating a buy transaction with only shares+total auto-fills price_per_share; creating with shares+price auto-fills total; at least shares + one of (total, price) required
  - GREEN: Integrate smart computation into buy/sell transaction creation

## Phase 21: Investment Transaction Service — Buy

- [ ] **SM-076 - Buy transaction (non-lot-tracking account)**
  - RED: Write test — buy adds shares to position; updates average cost; deducts total from cash; creates transaction record; requires sufficient cash
  - GREEN: Implement `Buy` for non-lot-tracking accounts

- [ ] **SM-077 - Buy transaction (lot-tracking account)**
  - RED: Write test — buy creates new lot with correct shares, cost_per_share, purchase_date; deducts total from cash; creates transaction record
  - GREEN: Implement `Buy` for lot-tracking accounts

## Phase 22: Investment Transaction Service — Sell

- [ ] **SM-078 - Sell transaction (non-lot-tracking account)**
  - RED: Write test — sell reduces position shares; adds proceeds to cash; selling more than held returns error; position removed when shares reach 0
  - GREEN: Implement `Sell` for non-lot-tracking accounts

- [ ] **SM-079 - Sell transaction (lot-tracking account)**
  - RED: Write test — sell requires lot selection; reduces specified lot shares; lot closed when shares=0; junction record created; total shares across lots must equal sell shares; proceeds added to cash
  - GREEN: Implement `Sell` with lot selection for lot-tracking accounts

- [ ] **SM-080 - Sell validation: lot selection**
  - RED: Write test — selling from non-existent lot returns error; selling more shares than lot holds returns error; selling from lot in different account returns error; partial lot sell works
  - GREEN: Implement lot selection validation

## Phase 23: Price Auto-Creation

- [ ] **SM-081 - Auto-create price on buy**
  - RED: Write test — buying shares creates a price record with source=`transaction` for security+date; if manual/import price already exists for that date, do NOT overwrite
  - GREEN: Add price auto-creation to `Buy` service method

- [ ] **SM-082 - Auto-create price on sell**
  - RED: Write test — selling shares creates a price record; existing manual/import price preserved
  - GREEN: Add price auto-creation to `Sell` service method

- [ ] **SM-083 - Auto-create price on reinvest dividend**
  - RED: Write test — reinvest creates price record from reinvestment price
  - GREEN: Add price auto-creation to `ReinvestDividend` service method

## Phase 24: Investment Transaction Service — Dividends

- [ ] **SM-084 - Cash dividend**
  - RED: Write test — dividend increases cash by amount; transaction created with security reference; share count unchanged
  - GREEN: Implement `Dividend`

- [ ] **SM-085 - Reinvest dividend (non-lot-tracking)**
  - RED: Write test — reinvest adds shares to position; recalculates average cost; no cash movement; transaction created
  - GREEN: Implement `ReinvestDividend` for non-lot-tracking

- [ ] **SM-086 - Reinvest dividend (lot-tracking)**
  - RED: Write test — reinvest creates new lot; no cash movement; lot has correct cost_per_share and purchase_date
  - GREEN: Implement `ReinvestDividend` for lot-tracking

## Phase 25: Investment Transaction Service — Fee via Liquidation

- [ ] **SM-087 - Fee via liquidation (non-lot-tracking)**
  - RED: Write test — fee reduces shares; no net cash effect; transaction records fee amount and shares sold
  - GREEN: Implement `FeeLiquidation` for non-lot-tracking

- [ ] **SM-088 - Fee via liquidation (lot-tracking)**
  - RED: Write test — requires lot selection (same as sell); reduces lot shares; no net cash effect; junction records created
  - GREEN: Implement `FeeLiquidation` for lot-tracking

## Phase 26: Investment Transaction Service — Transfers

- [ ] **SM-089 - Cash transfer between investment and regular account**
  - RED: Write test — deposit from checking creates investment_transaction (deposit) + regular transaction (withdrawal) linked by transfer_id; withdrawal to checking creates paired transactions
  - GREEN: Implement `TransferCash` that creates linked transactions across both tables

- [ ] **SM-090 - Share transfer between investment accounts (non-lot)**
  - RED: Write test — source position reduced; destination position increased with cost basis; no cash movement; shares must be available
  - GREEN: Implement `TransferShares` for non-lot-tracking

- [ ] **SM-091 - Share transfer between investment accounts (lot-tracking)**
  - RED: Write test — source lots reduced/closed; new lots created in destination with original purchase_date and cost_per_share; no cash movement
  - GREEN: Implement `TransferShares` for lot-tracking

- [ ] **SM-092 - Share transfer: lot-tracking to non-lot-tracking**
  - RED: Write test — source lots closed; destination position updated with aggregated cost basis
  - GREEN: Handle mixed account type transfers

## Phase 27: Account Valuation Service

- [ ] **SM-093 - GetAccountValuation**
  - RED: Write test — returns cash balance + market value of all holdings; market value = Σ(shares × current price); securities with no price use cost basis; total gain/loss calculated
  - GREEN: Implement `GetAccountValuation` in investment transaction service (or a new `PortfolioService`)

- [ ] **SM-094 - GetHoldings (rolled up by security)**
  - RED: Write test — for each security: total shares, average cost, current price, market value, cost basis, gain/loss ($), gain/loss (%); lot-tracking accounts aggregate across open lots
  - GREEN: Implement `GetHoldings`

- [ ] **SM-095 - GetLotDetail (lot-tracking accounts)**
  - RED: Write test — for a given account+security: list all open lots with purchase_date, shares, cost/share, cost basis, current value, gain/loss
  - GREEN: Implement `GetLotDetail`

## Phase 28: Security Hide Validation (wire up positions)

- [ ] **SM-096 - Wire SecurityService.Hide to real position check**
  - RED: Write test — hide fails when any account holds shares (via lots or positions); hide succeeds when all positions are zero/closed
  - GREEN: Update `SecurityService.Hide` to query lot and position repositories

## Phase 29: CLI — Security Management

- [ ] **SM-097 - CLI: --list-securities**
  - RED: Write test — lists securities with ticker, name, type, asset_class; respects --include-hidden flag; respects --type and --asset-class filters
  - GREEN: Implement in `cmd/tmoney/commands.go`; add flags to `cliOptions` in `args.go`

- [ ] **SM-098 - CLI: --security (show detail)**
  - RED: Write test — shows full detail for a security by ticker; not found returns error
  - GREEN: Implement `runSecurityDetail`

- [ ] **SM-099 - CLI: --add-security**
  - RED: Write test — creates security with --ticker, --name, --type; optional --asset-class, --currency, --exchange; validation errors displayed
  - GREEN: Implement `runAddSecurity`

- [ ] **SM-100 - CLI: --edit-security**
  - RED: Write test — edits security fields by ticker; supports ticker change
  - GREEN: Implement `runEditSecurity`

- [ ] **SM-101 - CLI: --hide-security / --unhide-security**
  - RED: Write test — hide sets hidden=true (with position check); unhide sets hidden=false
  - GREEN: Implement `runHideSecurity` and `runUnhideSecurity`

- [ ] **SM-102 - CLI: --delete-security**
  - RED: Write test — deletes security with no history; returns error and suggests hiding if history exists
  - GREEN: Implement `runDeleteSecurity`

## Phase 30: CLI — Price Management

- [ ] **SM-103 - CLI: --prices (list)**
  - RED: Write test — lists prices for a ticker ordered by date desc; respects --from and --to filters
  - GREEN: Implement `runListPrices`

- [ ] **SM-104 - CLI: --add-price**
  - RED: Write test — adds price for --ticker, --date, --price; conflict reported if duplicate
  - GREEN: Implement `runAddPrice`

- [ ] **SM-105 - CLI: --current-price**
  - RED: Write test — shows most recent price for ticker
  - GREEN: Implement `runCurrentPrice`

## Phase 31: CLI — Investment Transactions

- [ ] **SM-106 - CLI: --buy**
  - RED: Write test — buy with --account, --ticker, --shares, --total; optional --price-per-share, --commission; smart computation applies
  - GREEN: Implement `runBuy`

- [ ] **SM-107 - CLI: --sell**
  - RED: Write test — sell with --account, --ticker, --shares, --total; optional --lot for lot-tracking; validates sufficient shares
  - GREEN: Implement `runSell`

- [ ] **SM-108 - CLI: --dividend**
  - RED: Write test — cash dividend with --account, --ticker, --amount
  - GREEN: Implement `runDividend`

- [ ] **SM-109 - CLI: --reinvest**
  - RED: Write test — reinvest with --account, --ticker, --shares, --total
  - GREEN: Implement `runReinvest`

- [ ] **SM-110 - CLI: --investment-fee**
  - RED: Write test — fee with --account, --amount, --memo
  - GREEN: Implement `runInvestmentFee`

- [ ] **SM-111 - CLI: --deposit / --withdraw (investment)**
  - RED: Write test — deposit/withdraw cash in investment account
  - GREEN: Implement `runInvestmentDeposit` and `runInvestmentWithdraw`

- [ ] **SM-112 - CLI: --transfer-shares**
  - RED: Write test — transfer shares with --from, --to, --ticker, --shares
  - GREEN: Implement `runTransferShares`

## Phase 32: CLI — Portfolio Display

- [ ] **SM-113 - CLI: --portfolio**
  - RED: Write test — displays holdings table for account (ticker, name, shares, avg cost, current price, market value, cost basis, gain/loss); displays summary (cash, market value, total value)
  - GREEN: Implement `runPortfolio`

- [ ] **SM-114 - CLI: --portfolio --show-lots**
  - RED: Write test — displays lot detail under each security for lot-tracking accounts
  - GREEN: Add lot detail to `runPortfolio`

## Phase 33: Bulk Price Import

- [ ] **SM-115 - CSV parser for price import**
  - RED: Write test — parse valid CSV (date,ticker,price); reject missing header; reject invalid date format; reject invalid price; reject unknown ticker; report errors with line numbers
  - GREEN: Implement `ParsePriceCSV` in `internal/imexport/price_import.go`

- [ ] **SM-116 - Bulk import service**
  - RED: Write test — import valid CSV creates prices with source=`import`; skip existing by default; overwrite existing when flag set; return summary (total, imported, skipped, errors)
  - GREEN: Implement `PriceService.BulkImport`

- [ ] **SM-117 - CLI: --import-prices**
  - RED: Write test — import CSV file; --overwrite flag; display summary
  - GREEN: Implement `runImportPrices`

## Phase 34: TUI — Security Management View

- [ ] **SM-118 - Security list table component**
  - RED: Write test — table renders columns: Ticker, Name, Type, Asset Class, Currency, Status; sorts by ticker; hidden securities shown/hidden via filter toggle
  - GREEN: Implement security table in `internal/tui/security_view.go`

- [ ] **SM-119 - Security view navigation and keybindings**
  - RED: Write test — `n` opens add dialog; `Enter` opens edit; `h` toggles hidden; `d` deletes; `/` searches; `f` toggles hidden filter; `p` navigates to prices; `m` opens merge
  - GREEN: Implement key handlers and view routing

- [ ] **SM-120 - Add/Edit security dialog**
  - RED: Write test — dialog has fields: Ticker, Name, Type (dropdown), Asset Class (dropdown), Currency (dropdown), Exchange; Tab navigates fields; Enter submits; Esc cancels
  - GREEN: Implement security dialog

- [ ] **SM-121 - Security search/filter**
  - RED: Write test — typing in search bar filters by ticker and name; case-insensitive
  - GREEN: Implement search filter on security view

- [ ] **SM-122 - Wire security view to menu and navigation**
  - RED: Write test — security view accessible from menu; navigable via key shortcut
  - GREEN: Add ViewSecurities to app views; add menu entry; add keyboard shortcut

## Phase 35: TUI — Prices View

- [ ] **SM-123 - Price list table component**
  - RED: Write test — table renders columns: Date, Price, Source; sorted by date desc; security selector at top
  - GREEN: Implement price table in `internal/tui/price_view.go`

- [ ] **SM-124 - Price view navigation and keybindings**
  - RED: Write test — `n` opens add price form; `Enter` edits; `d` deletes; `i` opens import dialog; `/` searches by security
  - GREEN: Implement key handlers

- [ ] **SM-125 - Add/Edit price dialog**
  - RED: Write test — dialog has fields: Date, Price; pre-selects current security; validates date not future and price > 0
  - GREEN: Implement price dialog

- [ ] **SM-126 - Bulk import dialog**
  - RED: Write test — file selector for CSV; preview first 10 rows; validation results displayed; conflict resolution option; import button; results summary
  - GREEN: Implement import dialog

- [ ] **SM-127 - Wire prices view to menu and navigation**
  - RED: Write test — prices view accessible from menu and from security view via `p` key
  - GREEN: Add ViewPrices to app views; add menu entry

## Phase 36: TUI — Investment Account Register

- [ ] **SM-128 - Investment transaction list**
  - RED: Write test — register for investment account shows investment transactions (not regular transactions); columns: Date, Type, Security, Shares, Price, Total, Status
  - GREEN: Implement investment register rendering in tui

- [ ] **SM-129 - Investment transaction keybindings**
  - RED: Write test — `n` opens transaction type selector; `Enter` edits; `d` deletes; `c` toggles cleared
  - GREEN: Implement key handlers for investment register

- [ ] **SM-130 - Buy transaction dialog**
  - RED: Write test — dialog: Security (searchable dropdown), Shares, Price/Share, Total, Commission, Date, Memo; smart field computation updates fields as user types
  - GREEN: Implement buy dialog

- [ ] **SM-131 - Sell transaction dialog**
  - RED: Write test — same as buy but with lot selection panel for lot-tracking accounts; shows available lots with purchase date, shares, cost
  - GREEN: Implement sell dialog

- [ ] **SM-132 - Dividend dialog**
  - RED: Write test — dialog: Security (dropdown), Amount, Date, Memo; for reinvest: adds Shares and Price fields
  - GREEN: Implement dividend/reinvest dialog

- [ ] **SM-133 - Deposit/Withdrawal/Fee/Interest dialogs**
  - RED: Write test — simple dialogs for cash-only operations; Amount, Date, Memo fields
  - GREEN: Implement cash operation dialogs

- [ ] **SM-134 - Transfer dialogs (cash and shares)**
  - RED: Write test — cash transfer: select linked account, amount; share transfer: select destination account, security, shares
  - GREEN: Implement transfer dialogs

## Phase 37: TUI — Portfolio View

- [ ] **SM-135 - Portfolio summary bar**
  - RED: Write test — displays: Cash Balance, Market Value, Total Value, Total Cost Basis, Total Gain/Loss ($), Total Gain/Loss (%)
  - GREEN: Implement summary bar component

- [ ] **SM-136 - Holdings table**
  - RED: Write test — displays rolled-up holdings: Ticker, Name, Shares, Avg Cost, Current Price, Price Date, Market Value, Cost Basis, Gain/Loss ($), Gain/Loss (%); securities without pricing show "~" prefix
  - GREEN: Implement holdings table

- [ ] **SM-137 - Lot detail drill-down**
  - RED: Write test — selecting a security and pressing Enter shows lot detail: Purchase Date, Shares, Cost/Share, Cost Basis, Current Value, Gain/Loss ($), Gain/Loss (%); only for lot-tracking accounts
  - GREEN: Implement lot detail sub-view

- [ ] **SM-138 - Toggle between register and portfolio views**
  - RED: Write test — investment account has two sub-views: Register (transaction list) and Portfolio (holdings); Tab or keybinding switches between them
  - GREEN: Implement view toggle for investment accounts

## Phase 38: Corporate Actions Migration

- [ ] **SM-139 - Migration: corporate_actions table**
  - RED: Write integration test — insert corporate action record; verify action_type constraint; verify JSON parameters column
  - GREEN: Create migration `009_corporate_actions.sql`

## Phase 39: Corporate Action Models

- [ ] **SM-140 - CorporateActionType enum**
  - RED: Write tests — valid types (`split`, `reverse_split`, `merger`, `spin_off`); invalid rejected
  - GREEN: Implement `CorporateActionType` in `internal/models/corporate_action.go`

- [ ] **SM-141 - CorporateAction model**
  - RED: Write tests — required fields (action_type, security_id, action_date, parameters); parameters is JSON string; target_security_id required for merger and spin_off
  - GREEN: Implement `CorporateAction` struct with `Validate()`

- [ ] **SM-142 - Split parameters model**
  - RED: Write tests — `SplitParams` with numerator and denominator (e.g., 4:1); `Ratio()` returns decimal; validate both > 0
  - GREEN: Implement `SplitParams` struct with JSON serialization

- [ ] **SM-143 - Merger parameters model**
  - RED: Write tests — `MergerParams` with exchange_ratio and optional cash_per_share; validate ratio > 0
  - GREEN: Implement `MergerParams` struct

- [ ] **SM-144 - Spin-off parameters model**
  - RED: Write tests — `SpinOffParams` with share_ratio and parent_allocation_pct (0-100); validate allocation + remainder = 100
  - GREEN: Implement `SpinOffParams` struct

## Phase 40: Corporate Action Repository

- [ ] **SM-145 - CorporateActionRepository.Create**
  - RED: Write test — create action, verify fields and JSON parameters stored correctly
  - GREEN: Implement `Create` in `internal/repository/corporate_action.go`

- [ ] **SM-146 - CorporateActionRepository.ListBySecurity**
  - RED: Write test — list actions for a security ordered by date; includes actions where security is source or target
  - GREEN: Implement `ListBySecurity`

## Phase 41: Corporate Action Service — Stock Split

- [ ] **SM-147 - Stock split: adjust lots**
  - RED: Write test — 4:1 split on security with lot of 10 shares at $100/share → 40 shares at $25/share; original_shares unchanged; multiple lots all adjusted
  - GREEN: Implement lot adjustment in `CorporateActionService.Split`

- [ ] **SM-148 - Stock split: adjust positions (non-lot)**
  - RED: Write test — 4:1 split on position of 10 shares at $100 avg → 40 shares at $25 avg
  - GREEN: Implement position adjustment in `Split`

- [ ] **SM-149 - Stock split: adjust price history**
  - RED: Write test — all prices on or before split date divided by ratio; prices after split date unchanged
  - GREEN: Implement price history adjustment in `Split`

- [ ] **SM-150 - Stock split: audit log**
  - RED: Write test — split creates corporate_actions record with correct parameters
  - GREEN: Record audit entry in `Split`

- [ ] **SM-151 - Reverse split**
  - RED: Write test — 1:10 reverse split on 100 shares at $5 → 10 shares at $50; prices multiplied; verify same code path with inverted ratio
  - GREEN: Verify `Split` handles reverse splits (ratio < 1)

## Phase 42: Corporate Action Service — Merger

- [ ] **SM-152 - Merger: exchange shares across accounts**
  - RED: Write test — source security lots removed; target security lots created with transferred cost basis; new shares = old_shares / exchange_ratio; cost_per_share adjusted to preserve total cost basis
  - GREEN: Implement `CorporateActionService.Merger`

- [ ] **SM-153 - Merger: cash consideration**
  - RED: Write test — merger with cash_per_share adds cash to investment account; cash = cash_per_share × old_shares
  - GREEN: Add cash consideration handling to `Merger`

- [ ] **SM-154 - Merger: source security hidden after merge**
  - RED: Write test — after all positions exchanged, source security marked hidden
  - GREEN: Add auto-hide to `Merger`

- [ ] **SM-155 - Merger: non-lot-tracking accounts**
  - RED: Write test — position shares converted; average cost recalculated to preserve total cost basis
  - GREEN: Handle non-lot accounts in `Merger`

- [ ] **SM-156 - Merger: audit log**
  - RED: Write test — merger creates corporate_actions record
  - GREEN: Record audit entry

## Phase 43: Corporate Action Service — Spin-Off

- [ ] **SM-157 - Spin-off: cost basis allocation to parent**
  - RED: Write test — parent lot cost_per_share reduced by allocation percentage (e.g., 80% allocation means new cost = old cost × 0.80)
  - GREEN: Implement parent cost basis adjustment in `CorporateActionService.SpinOff`

- [ ] **SM-158 - Spin-off: create spin-off lots**
  - RED: Write test — new lots created for spin-off security; shares = parent_shares × share_ratio; cost = remaining allocation; purchase_date preserved from parent lot
  - GREEN: Implement spin-off lot creation

- [ ] **SM-159 - Spin-off: fractional shares handling**
  - RED: Write test — fractional shares rounded down; cash-in-lieu recorded for fractional portion at spin-off price
  - GREEN: Implement fractional share handling

- [ ] **SM-160 - Spin-off: non-lot-tracking accounts**
  - RED: Write test — parent position cost adjusted; new position created for spin-off security
  - GREEN: Handle non-lot accounts in `SpinOff`

- [ ] **SM-161 - Spin-off: price record for spin-off security**
  - RED: Write test — price record created for spin-off on spin-off date
  - GREEN: Add price creation to `SpinOff`

- [ ] **SM-162 - Spin-off: audit log**
  - RED: Write test — spin-off creates corporate_actions record
  - GREEN: Record audit entry

## Phase 44: CLI — Corporate Actions

- [ ] **SM-163 - CLI: --split**
  - RED: Write test — stock split with --ticker, --date, --ratio (format "4:1"); reverse split with "1:10"
  - GREEN: Implement `runSplit`

- [ ] **SM-164 - CLI: --merge-security**
  - RED: Write test — merger with --source, --target, --date, --ratio; optional --cash-per-share
  - GREEN: Implement `runMergeSecurity`

- [ ] **SM-165 - CLI: --spin-off**
  - RED: Write test — spin-off with --parent, --spinoff, --date, --share-ratio, --parent-allocation
  - GREEN: Implement `runSpinOff`

## Phase 45: TUI — Corporate Actions

- [ ] **SM-166 - Stock split dialog**
  - RED: Write test — dialog: Security (pre-selected or searchable), Date, Ratio (text input "4:1"); confirms affected accounts before executing
  - GREEN: Implement split dialog accessible from security view

- [ ] **SM-167 - Merger dialog**
  - RED: Write test — dialog: Source Security, Target Security, Date, Exchange Ratio, Cash Per Share (optional); shows preview of affected accounts
  - GREEN: Implement merger dialog

- [ ] **SM-168 - Spin-off dialog**
  - RED: Write test — dialog: Parent Security, Spin-off Security, Date, Share Ratio, Parent Allocation %; shows preview
  - GREEN: Implement spin-off dialog

- [ ] **SM-169 - Corporate action history view**
  - RED: Write test — accessible from security detail; lists all corporate actions for a security with type, date, parameters
  - GREEN: Implement corporate action history display

## Phase 46: Security Merge Workflow (TUI)

- [ ] **SM-170 - Merge security dialog**
  - RED: Write test — select source and target security; enter merge date and exchange ratio; optional cash per share; confirmation step showing affected accounts and lots
  - GREEN: Implement merge dialog accessible via `m` key in security view

## Phase 47: Net Worth Integration

- [ ] **SM-171 - Include investment accounts in net worth report**
  - RED: Write test — net worth includes investment account total value (cash + holdings market value); existing asset/liability classification works (investment = asset)
  - GREEN: Update `ReportService.NetWorth` to include investment account valuations

- [ ] **SM-172 - Net worth: handle missing prices**
  - RED: Write test — securities with no pricing data use cost basis as conservative estimate; flagged in report output
  - GREEN: Handle missing price fallback in net worth calculation

## Phase 48: Portfolio Holdings Database View

- [ ] **SM-173 - Migration: portfolio_holdings view**
  - RED: Write integration test — view returns correct total_shares and total_cost_basis for both lot-tracking and non-lot-tracking accounts; correctly joins securities
  - GREEN: Create migration `010_portfolio_holdings_view.sql` with the `portfolio_holdings` view per spec

- [ ] **SM-174 - Use portfolio_holdings view in service layer**
  - RED: Write test — `GetHoldings` can optionally use the database view for performance; results match manual computation
  - GREEN: Add repository method to query the view; wire into portfolio service

## Phase 49: Dashboard Integration

- [ ] **SM-175 - Investment accounts on dashboard**
  - RED: Write test — dashboard shows investment accounts with total value (cash + holdings); expandable to show top holdings
  - GREEN: Update dashboard data loading to include investment account valuations

- [ ] **SM-176 - Navigate from dashboard to investment account**
  - RED: Write test — selecting an investment account on dashboard opens portfolio view (not regular register)
  - GREEN: Route investment accounts to portfolio/investment register view

## Phase 50: Edge Cases and Polish

- [ ] **SM-177 - Hidden security enforcement**
  - RED: Write test — hidden securities excluded from security dropdowns in transaction dialogs; excluded from price update operations; still visible in portfolio if historically held
  - GREEN: Wire hidden flag filtering into all relevant UI components and service methods

- [ ] **SM-178 - Zero-position cleanup**
  - RED: Write test — positions with zero shares are excluded from portfolio display; lots with zero shares show as closed; no orphaned data
  - GREEN: Add cleanup/filter logic

- [ ] **SM-179 - Multi-currency securities**
  - RED: Write test — same company can have tickers in different currencies (e.g., "RY" USD and "RY.TO" CAD); unique constraint is on ticker+currency, not ticker alone
  - GREEN: Verify all queries respect the currency dimension

- [ ] **SM-180 - Price date validation**
  - RED: Write test — cannot add price with future date; cannot create transaction with future date; boundary: today is valid
  - GREEN: Verify date validation across all entry points

- [ ] **SM-181 - Commission handling edge cases**
  - RED: Write test — zero commission works; commission > total_amount rejected; commission precision (2 decimal places for USD)
  - GREEN: Verify commission validation and computation
