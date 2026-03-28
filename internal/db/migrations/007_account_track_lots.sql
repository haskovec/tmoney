-- Migration 007: Add track_lots column to accounts
-- Adds track_lots BOOLEAN column for investment accounts with lot-based cost tracking.
-- DuckDB does not support ALTER TABLE ADD COLUMN with NOT NULL constraint,
-- so we add it as nullable with a default, then use backup-drop-recreate to make it NOT NULL.

-- Step 1: Drop views that depend on the accounts table
DROP VIEW IF EXISTS account_balances;
DROP VIEW IF EXISTS category_spending;

-- Step 2: Backup existing accounts data
CREATE TEMPORARY TABLE accounts_backup AS SELECT *, FALSE AS track_lots_new FROM accounts;

-- Step 3: Drop dependent tables in reverse dependency order, backing up each

-- transaction_splits -> transactions
CREATE TEMPORARY TABLE splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

CREATE TEMPORARY TABLE transactions_backup AS SELECT * FROM transactions;
DROP TABLE transactions;

CREATE TEMPORARY TABLE scheduled_backup AS SELECT * FROM scheduled_transactions;
DROP TABLE scheduled_transactions;

CREATE TEMPORARY TABLE reconciliation_backup AS SELECT * FROM reconciliation_sessions;
DROP TABLE reconciliation_sessions;

CREATE TEMPORARY TABLE investment_lots_backup AS SELECT * FROM investment_lots;
DROP TABLE investment_lots;

-- Step 4: Drop accounts table
DROP TABLE accounts;

-- Step 5: Recreate accounts table with track_lots column
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN (
        'checking', 'savings', 'credit_card',
        'investment', 'cash', 'loan', 'asset'
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

-- Step 6: Restore accounts data
INSERT INTO accounts (
    id, name, type, currency, institution, account_number,
    opening_balance, opening_date, credit_limit, interest_rate,
    notes, active, track_lots, created_at, updated_at
)
SELECT
    id, name, type, currency, institution, account_number,
    opening_balance, opening_date, credit_limit, interest_rate,
    notes, active, track_lots_new, created_at, updated_at
FROM accounts_backup;
DROP TABLE accounts_backup;

-- Step 7: Recreate accounts indexes
CREATE INDEX idx_accounts_name ON accounts(name);
CREATE INDEX idx_accounts_type ON accounts(type);
CREATE INDEX idx_accounts_active ON accounts(active);

-- Step 8: Restore dependent tables

-- Investment lots
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    quantity DECIMAL(19, 8) NOT NULL,
    purchase_price DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO investment_lots SELECT * FROM investment_lots_backup;
DROP TABLE investment_lots_backup;
CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_symbol ON investment_lots(symbol);

-- Reconciliation sessions
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
INSERT INTO reconciliation_sessions SELECT * FROM reconciliation_backup;
DROP TABLE reconciliation_backup;
CREATE INDEX idx_reconciliation_sessions_account ON reconciliation_sessions(account_id);
CREATE INDEX idx_reconciliation_sessions_status ON reconciliation_sessions(status);

-- Scheduled transactions
CREATE TABLE scheduled_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    payee_id UUID REFERENCES payees(id),
    category_id UUID REFERENCES categories(id),
    amount DECIMAL(19, 4),
    memo TEXT,
    frequency TEXT NOT NULL CHECK (frequency IN (
        'daily', 'weekly', 'biweekly', 'monthly',
        'quarterly', 'yearly'
    )),
    interval INTEGER NOT NULL DEFAULT 1,
    start_date DATE NOT NULL,
    end_date DATE,
    occurrences INTEGER,
    day_of_month INTEGER CHECK (day_of_month BETWEEN -1 AND 31),
    day_of_week INTEGER CHECK (day_of_week BETWEEN 0 AND 6),
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    auto_post BOOLEAN NOT NULL DEFAULT FALSE,
    post_lead_days INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO scheduled_transactions SELECT * FROM scheduled_backup;
DROP TABLE scheduled_backup;
CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);

-- Transactions
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

-- Transaction splits
CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    category_id UUID NOT NULL REFERENCES categories(id),
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO transaction_splits SELECT * FROM splits_backup;
DROP TABLE splits_backup;
CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);

-- Step 9: Recreate views
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
