-- Migration 037: Add the 'mid_cap_stock' asset class and remove 'index'.
--
-- Design: specs/design-asset-allocation-report.md §4. 'index' told how a fund
-- is managed, not what it holds (an S&P 500 index fund is large-cap stock, a
-- bond index fund is a bond), so it could put a security in the wrong slice
-- of the allocation report. Every 'index' security becomes 'unclassified', so
-- no data is lost and the owner sets the real class.
--
-- DuckDB cannot drop or alter an anonymous CHECK, so securities is rebuilt
-- with the backup-drop-recreate recipe of migration 034. No table holds an FK
-- to securities (migration 010 removed them), so no other table is rebuilt;
-- only the portfolio_holdings view reads it and is dropped and recreated. The
-- CREATE below is copied from the live catalog (duckdb_tables().sql) at
-- version 036, not from migration 012, so no later change is silently
-- reverted: the uuidv7() default (032) and the isin column (027).
--
-- The five secondary indexes of migration 012 are deliberately NOT recreated,
-- for the reason of migrations 021, 023 and 027: DuckDB rewrites an UPDATE of
-- an indexed column as DELETE+INSERT, and a stale index then fails the delete
-- and invalidates the connection. Reclassifying securities from the
-- allocation report updates asset_class many times. The table is small, so a
-- full scan is cheap, and none of the indexes was UNIQUE: the ticker+currency
-- rule is enforced in the service layer. TestMigration037 pins all of this by
-- diffing the catalog before and after.

-- Step 1: Drop the dependent view.
DROP VIEW IF EXISTS portfolio_holdings;

-- Step 2: Back up and drop securities.
CREATE TEMPORARY TABLE securities_backup AS SELECT * FROM securities;
DROP TABLE securities;

-- Step 3: Recreate securities with the new asset_class list. Column order
-- matches the 036 catalog.
CREATE TABLE securities (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    ticker TEXT NOT NULL,
    name TEXT NOT NULL,
    security_type TEXT NOT NULL CHECK (security_type IN (
        'stock', 'etf', 'mutual_fund', 'other'
    )),
    asset_class TEXT NOT NULL DEFAULT 'unclassified' CHECK (asset_class IN (
        'large_cap_stock', 'mid_cap_stock', 'small_cap_stock', 'international_stock',
        'domestic_bond', 'foreign_bond', 'cash',
        'commodity', 'crypto', 'asset_mixture', 'real_estate', 'unclassified'
    )),
    currency TEXT NOT NULL DEFAULT 'USD',
    exchange TEXT,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    isin TEXT
);

-- Step 4: Copy the rows back, moving every 'index' security to 'unclassified'.
INSERT INTO securities (
    id, ticker, name, security_type, asset_class, currency, exchange,
    hidden, created_at, updated_at, isin
)
SELECT
    id, ticker, name, security_type,
    CASE WHEN asset_class = 'index' THEN 'unclassified' ELSE asset_class END,
    currency, exchange, hidden, created_at, updated_at, isin
FROM securities_backup;
DROP TABLE securities_backup;

-- Step 5: Recreate the view, as migration 034 left it.
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
WHERE a.type IN ('investment', 'hsa_investment') AND a.active = TRUE;
