-- Migration 005: Add bank_reference_id to transactions
-- Adds bank_reference_id (TEXT, nullable) column for OFX FITID tracking.
-- DuckDB does not support ALTER TABLE ADD COLUMN with constraints,
-- so we use backup-drop-recreate pattern.

-- Step 1: Drop views that depend on the transactions table
DROP VIEW IF EXISTS account_balances;
DROP VIEW IF EXISTS category_spending;

-- Step 2: Backup all data
CREATE TEMPORARY TABLE transactions_backup AS SELECT * FROM transactions;
CREATE TEMPORARY TABLE splits_backup AS SELECT * FROM transaction_splits;

-- Step 3: Drop tables with foreign key dependencies (in reverse order)
DROP TABLE transaction_splits;
DROP TABLE transactions;

-- Step 4: Recreate transactions table with new column
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

-- Step 5: Restore transactions data with NULL for new column
INSERT INTO transactions (
    id, account_id, date, amount, payee_id, category_id,
    memo, check_number, status, transfer_id, transfer_account_id,
    bank_reference_id, created_at, updated_at
)
SELECT
    id, account_id, date, amount, payee_id, category_id,
    memo, check_number, status, transfer_id, transfer_account_id,
    NULL, created_at, updated_at
FROM transactions_backup;

-- Step 6: Recreate transaction_splits table
CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    category_id UUID NOT NULL REFERENCES categories(id),
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Step 7: Restore splits data
INSERT INTO transaction_splits (id, transaction_id, category_id, amount, memo, created_at)
SELECT id, transaction_id, category_id, amount, memo, created_at
FROM splits_backup;

-- Step 8: Clean up temporary tables
DROP TABLE transactions_backup;
DROP TABLE splits_backup;

-- Step 9: Recreate indexes
CREATE INDEX idx_transactions_account ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_payee ON transactions(payee_id);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_transfer ON transactions(transfer_id);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_bank_ref ON transactions(bank_reference_id);
CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);

-- Step 10: Recreate views
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
