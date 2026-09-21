-- Migration 034: Split the HSA account type into 'hsa' and 'hsa_investment'.
--
-- Design: specs/design-hsa-split.md. Before this migration 'hsa' meant the
-- INVESTED side of a Health Savings Account: IsInvestmentType() was true and
-- every row lived in investment_transactions. After it, 'hsa' is the CASH
-- side (a register account like checking) and 'hsa_investment' is the
-- invested side. Every existing 'hsa' account therefore becomes
-- 'hsa_investment' — that is what its rows already mean — and the owner
-- converts a cash HSA by hand through the guarded type change, which moves
-- its cash-only rows into the register.
--
-- DuckDB cannot drop or alter an anonymous CHECK, so accounts is rebuilt
-- with the backup-drop-recreate recipe of migration 019. Every table with an
-- FK to accounts is backed up, dropped and recreated too. Each CREATE below
-- is copied from the live catalog (duckdb_tables().sql) at version 033, not
-- from an older migration, so no default, CHECK or FK that a later migration
-- changed is silently reverted: uuidv7() defaults (032), closed_date (020),
-- no status indexes (030), no day_of_week (031), no idx_scheduled_account
-- (033). TestMigration034 pins this by diffing the catalog before and after.
--
-- transaction_splits, scheduled_split_items and investment_transaction_lots
-- carry no FK to any rebuilt table (026, 028, 013) and are left alone.

-- Step 1: Drop dependent views.
DROP VIEW IF EXISTS portfolio_holdings;
DROP VIEW IF EXISTS account_balances;
DROP VIEW IF EXISTS category_spending;

-- Step 2: Back up accounts and every table that holds an FK to accounts.
CREATE TEMPORARY TABLE accounts_backup AS SELECT * FROM accounts;

CREATE TEMPORARY TABLE transactions_backup AS SELECT * FROM transactions;
DROP TABLE transactions;

CREATE TEMPORARY TABLE scheduled_transactions_backup AS SELECT * FROM scheduled_transactions;
DROP TABLE scheduled_transactions;

CREATE TEMPORARY TABLE reconciliation_sessions_backup AS SELECT * FROM reconciliation_sessions;
DROP TABLE reconciliation_sessions;

CREATE TEMPORARY TABLE investment_positions_backup AS SELECT * FROM investment_positions;
DROP TABLE investment_positions;

CREATE TEMPORARY TABLE investment_lots_backup AS SELECT * FROM investment_lots;
DROP TABLE investment_lots;

CREATE TEMPORARY TABLE investment_transactions_backup AS SELECT * FROM investment_transactions;
DROP TABLE investment_transactions;

-- Step 3: Drop and recreate accounts with the widened CHECK, rewriting the
-- type of every existing HSA. Column order matches the 033 catalog so
-- SELECT * lines up.
DROP TABLE accounts;

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN (
        'checking', 'savings', 'credit_card',
        'investment', 'hsa', 'hsa_investment', 'cash', 'loan', 'asset'
    )),
    currency TEXT NOT NULL DEFAULT 'USD',
    institution TEXT,
    account_number TEXT,
    opening_balance DECIMAL(19, 4) NOT NULL DEFAULT 0,
    opening_date DATE NOT NULL,
    credit_limit DECIMAL(19, 4),
    interest_rate DECIMAL(5, 4),
    notes TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    track_lots BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_date DATE
);

INSERT INTO accounts
SELECT * REPLACE (CASE WHEN type = 'hsa' THEN 'hsa_investment' ELSE type END AS type)
FROM accounts_backup;
DROP TABLE accounts_backup;

-- Step 4: Recreate each child table from its 033 catalog definition and
-- restore its rows unchanged.

-- transactions
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    amount DECIMAL(19, 4) NOT NULL,
    payee_id UUID REFERENCES payees(id),
    category_id UUID REFERENCES categories(id),
    memo TEXT,
    check_number TEXT,
    status TEXT NOT NULL DEFAULT 'uncleared' CHECK (status IN (
        'uncleared', 'cleared', 'reconciled', 'void'
    )),
    transfer_id UUID,
    transfer_account_id UUID REFERENCES accounts(id),
    bank_reference_id TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO transactions SELECT * FROM transactions_backup;
DROP TABLE transactions_backup;
CREATE INDEX idx_transactions_account ON transactions(account_id);
CREATE INDEX idx_transactions_bank_ref ON transactions(bank_reference_id);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_payee ON transactions(payee_id);
CREATE INDEX idx_transactions_transfer ON transactions(transfer_id);

-- scheduled_transactions
CREATE TABLE scheduled_transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    payee_id UUID REFERENCES payees(id),
    category_id UUID REFERENCES categories(id),
    amount DECIMAL(19, 4),
    memo TEXT,
    frequency TEXT NOT NULL CHECK (frequency IN (
        'daily', 'weekly', 'fortnightly', 'semimonthly',
        'monthly', 'quarterly', 'yearly'
    )),
    interval INTEGER NOT NULL DEFAULT 1,
    start_date DATE NOT NULL,
    end_date DATE,
    occurrences INTEGER,
    day_of_month INTEGER CHECK (day_of_month BETWEEN -1 AND 31),
    secondary_day_of_month INTEGER CHECK (secondary_day_of_month BETWEEN -1 AND 31),
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    auto_post BOOLEAN NOT NULL DEFAULT FALSE,
    post_lead_days INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    transfer_account_id UUID
);
INSERT INTO scheduled_transactions SELECT * FROM scheduled_transactions_backup;
DROP TABLE scheduled_transactions_backup;
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);

-- reconciliation_sessions
CREATE TABLE reconciliation_sessions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    statement_date DATE NOT NULL,
    statement_balance DECIMAL(19, 4) NOT NULL,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN (
        'in_progress', 'completed'
    )),
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO reconciliation_sessions SELECT * FROM reconciliation_sessions_backup;
DROP TABLE reconciliation_sessions_backup;
CREATE INDEX idx_reconciliation_sessions_account ON reconciliation_sessions(account_id);

-- investment_transactions
CREATE TABLE investment_transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN (
        'buy', 'sell', 'dividend', 'reinvest_dividend',
        'fee', 'fee_liquidation', 'deposit', 'withdrawal',
        'interest', 'transfer_shares', 'transfer_cash', 'exchange'
    )),
    security_id UUID,
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
INSERT INTO investment_transactions SELECT * FROM investment_transactions_backup;
DROP TABLE investment_transactions_backup;
CREATE INDEX idx_inv_tx_account ON investment_transactions(account_id);
CREATE INDEX idx_inv_tx_date ON investment_transactions(date);
CREATE INDEX idx_inv_tx_security ON investment_transactions(security_id);
CREATE INDEX idx_inv_tx_transfer ON investment_transactions(transfer_id);
CREATE INDEX idx_inv_tx_type ON investment_transactions(transaction_type);

-- investment_lots
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL,
    original_shares DECIMAL(19, 8) NOT NULL,
    cost_per_share DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    source_transaction_id UUID NOT NULL,
    closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO investment_lots SELECT * FROM investment_lots_backup;
DROP TABLE investment_lots_backup;

-- investment_positions
CREATE TABLE investment_positions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    security_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL DEFAULT 0,
    average_cost_per_share DECIMAL(19, 4) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, security_id)
);
INSERT INTO investment_positions SELECT * FROM investment_positions_backup;
DROP TABLE investment_positions_backup;

-- Step 5: Recreate the views. account_balances and category_spending are
-- verbatim from 029; portfolio_holdings gates on the renamed type.
CREATE VIEW account_balances AS
SELECT
    a.id,
    a.name,
    a.type,
    a.opening_balance,
    a.opening_balance + COALESCE(
        SUM(CASE WHEN t.status != 'void' THEN t.amount ELSE 0 END), 0
    ) AS current_balance,
    a.opening_balance + COALESCE(
        SUM(CASE WHEN t.status IN ('cleared', 'reconciled')
            THEN t.amount ELSE 0 END), 0
    ) AS cleared_balance
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id
GROUP BY a.id, a.name, a.type, a.opening_balance;

CREATE VIEW category_spending AS
SELECT
    c.id,
    c.name,
    c.parent_id,
    c.type,
    DATE_TRUNC('month', t.date) AS month,
    SUM(t.amount) AS total
FROM categories c
LEFT JOIN transactions t
    ON t.category_id = c.id
   AND t.status != 'void'
   AND t.transfer_id IS NULL
GROUP BY c.id, c.name, c.parent_id, c.type, DATE_TRUNC('month', t.date);

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
