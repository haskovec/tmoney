-- Migration 019: Widen accounts.type CHECK to include 'hsa'.
--
-- HSAs (Health Savings Accounts) are a new account type that behaves like
-- TypeInvestment but is reported separately. Adding a value to a CHECK
-- constraint in DuckDB requires rebuilding the table (no DROP CONSTRAINT
-- for anonymous CHECKs), and the accounts table is referenced by many
-- children — so we follow the same backup-drop-recreate dance migration
-- 007 used.
--
-- The portfolio_holdings view also gates on `a.type = 'investment'`; it
-- is widened to also include 'hsa' so HSAs surface in the same holdings
-- aggregation paths as investment accounts.

-- Step 1: Drop dependent views.
DROP VIEW IF EXISTS portfolio_holdings;
DROP VIEW IF EXISTS account_balances;
DROP VIEW IF EXISTS category_spending;

-- Step 2: Back up accounts and every table that holds an FK to accounts.
CREATE TEMPORARY TABLE accounts_backup AS SELECT * FROM accounts;

CREATE TEMPORARY TABLE transaction_splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

CREATE TEMPORARY TABLE transactions_backup AS SELECT * FROM transactions;
DROP TABLE transactions;

CREATE TEMPORARY TABLE scheduled_transactions_backup AS SELECT * FROM scheduled_transactions;
DROP TABLE scheduled_transactions;

CREATE TEMPORARY TABLE reconciliation_sessions_backup AS SELECT * FROM reconciliation_sessions;
DROP TABLE reconciliation_sessions;

CREATE TEMPORARY TABLE investment_transaction_lots_backup AS SELECT * FROM investment_transaction_lots;
DROP TABLE investment_transaction_lots;

CREATE TEMPORARY TABLE investment_positions_backup AS SELECT * FROM investment_positions;
DROP TABLE investment_positions;

CREATE TEMPORARY TABLE investment_lots_backup AS SELECT * FROM investment_lots;
DROP TABLE investment_lots;

CREATE TEMPORARY TABLE investment_transactions_backup AS SELECT * FROM investment_transactions;
DROP TABLE investment_transactions;

-- Step 3: Drop and recreate accounts with the widened CHECK.
DROP TABLE accounts;

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN (
        'checking', 'savings', 'credit_card',
        'investment', 'hsa', 'cash', 'loan', 'asset'
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
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO accounts SELECT * FROM accounts_backup;
DROP TABLE accounts_backup;

CREATE INDEX idx_accounts_name ON accounts(name);
CREATE INDEX idx_accounts_type ON accounts(type);
CREATE INDEX idx_accounts_active ON accounts(active);

-- Step 4: Recreate each child table with its FK to the new accounts
-- and restore data.

-- transactions (latest definition from migration 005)
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_payee ON transactions(payee_id);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_transfer ON transactions(transfer_id);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_bank_ref ON transactions(bank_reference_id);

-- transaction_splits (latest definition from migration 014)
CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    transfer_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL)),
    CHECK ((transfer_account_id IS NULL) = (transfer_id IS NULL))
);
INSERT INTO transaction_splits SELECT * FROM transaction_splits_backup;
DROP TABLE transaction_splits_backup;
CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
CREATE INDEX idx_splits_transfer ON transaction_splits(transfer_id);

-- scheduled_transactions (latest definition from migration 018)
CREATE TABLE scheduled_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
    day_of_week INTEGER CHECK (day_of_week BETWEEN 0 AND 6),
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    auto_post BOOLEAN NOT NULL DEFAULT FALSE,
    post_lead_days INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO scheduled_transactions SELECT * FROM scheduled_transactions_backup;
DROP TABLE scheduled_transactions_backup;
CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);

-- reconciliation_sessions (latest definition from migration 007)
CREATE TABLE reconciliation_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE INDEX idx_reconciliation_sessions_status ON reconciliation_sessions(status);

-- investment_transactions (latest definition from migration 010 — no FK on security_id)
CREATE TABLE investment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE INDEX idx_inv_tx_type ON investment_transactions(transaction_type);
CREATE INDEX idx_inv_tx_security ON investment_transactions(security_id);
CREATE INDEX idx_inv_tx_transfer ON investment_transactions(transfer_id);

-- investment_lots (latest definition from migration 010)
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_security ON investment_lots(security_id);
CREATE INDEX idx_lots_closed ON investment_lots(closed);

-- investment_transaction_lots (latest definition from migration 013 — no FK on lot_id)
CREATE TABLE investment_transaction_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL,
    lot_id UUID NOT NULL,
    shares DECIMAL(19, 8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO investment_transaction_lots SELECT * FROM investment_transaction_lots_backup;
DROP TABLE investment_transaction_lots_backup;
CREATE INDEX idx_tx_lots_transaction ON investment_transaction_lots(transaction_id);
CREATE INDEX idx_tx_lots_lot ON investment_transaction_lots(lot_id);

-- investment_positions (latest definition from migration 010)
CREATE TABLE investment_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
CREATE INDEX idx_positions_account ON investment_positions(account_id);
CREATE INDEX idx_positions_security ON investment_positions(security_id);

-- Step 5: Recreate views (account_balances and category_spending mirror
-- migration 007's definitions; portfolio_holdings is widened to also
-- include hsa accounts).
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
LEFT JOIN transactions t ON t.category_id = c.id AND t.status != 'void'
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
WHERE a.type IN ('investment', 'hsa') AND a.active = TRUE;
