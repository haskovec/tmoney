-- Migration 006: Add securities and security_prices tables
-- Creates the Security Master tables for investment tracking.

-- Securities table
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
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_securities_ticker ON securities(ticker);
CREATE INDEX idx_securities_type ON securities(security_type);
CREATE INDEX idx_securities_asset_class ON securities(asset_class);
CREATE INDEX idx_securities_hidden ON securities(hidden);
CREATE INDEX idx_securities_ticker_currency ON securities(ticker, currency);

-- Security prices table
CREATE TABLE security_prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    security_id UUID NOT NULL REFERENCES securities(id),
    date DATE NOT NULL,
    price DECIMAL(19, 4) NOT NULL CHECK (price > 0),
    source TEXT NOT NULL CHECK (source IN (
        'manual', 'transaction', 'import', 'api'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_security_prices_security_date ON security_prices(security_id, date);
CREATE INDEX idx_security_prices_security ON security_prices(security_id);
CREATE INDEX idx_security_prices_date ON security_prices(date);
