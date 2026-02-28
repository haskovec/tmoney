-- Migration 004: Auto-post scheduled transactions
-- Adds auto_post and post_lead_days columns to scheduled_transactions table.
-- DuckDB does not support ALTER TABLE ADD COLUMN with constraints,
-- so we use backup-drop-recreate pattern.

-- Step 1: Backup existing data
CREATE TEMPORARY TABLE scheduled_transactions_backup AS SELECT * FROM scheduled_transactions;

-- Step 2: Drop existing table and indexes
DROP TABLE scheduled_transactions;

-- Step 3: Recreate table with new columns
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

-- Step 4: Restore data with defaults for new columns
INSERT INTO scheduled_transactions (
    id, account_id, payee_id, category_id, amount, memo,
    frequency, interval, start_date, end_date, occurrences,
    day_of_month, day_of_week, next_date, occurrences_remaining,
    amount_estimate_count, auto_post, post_lead_days,
    created_at, updated_at
)
SELECT
    id, account_id, payee_id, category_id, amount, memo,
    frequency, interval, start_date, end_date, occurrences,
    day_of_month, day_of_week, next_date, occurrences_remaining,
    amount_estimate_count, FALSE, 0,
    created_at, updated_at
FROM scheduled_transactions_backup;

-- Step 5: Clean up temporary table
DROP TABLE scheduled_transactions_backup;

-- Step 6: Recreate indexes
CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);
