-- Migration 012: Add 'real_estate' to the securities.asset_class CHECK constraint
-- DuckDB does not support ALTER TABLE ... DROP CONSTRAINT, so we rebuild the
-- table with the widened CHECK list and copy data through.

CREATE TEMPORARY TABLE securities_backup AS SELECT * FROM securities;
DROP TABLE securities;

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
        'commodity', 'crypto', 'asset_mixture', 'real_estate', 'unclassified'
    )),
    currency TEXT NOT NULL DEFAULT 'USD',
    exchange TEXT,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO securities SELECT * FROM securities_backup;
DROP TABLE securities_backup;

CREATE INDEX idx_securities_ticker ON securities(ticker);
CREATE INDEX idx_securities_type ON securities(security_type);
CREATE INDEX idx_securities_asset_class ON securities(asset_class);
CREATE INDEX idx_securities_hidden ON securities(hidden);
CREATE INDEX idx_securities_ticker_currency ON securities(ticker, currency);
