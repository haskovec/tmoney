-- Migration 031: Drop the unused day_of_week column from
-- scheduled_transactions.
--
-- The column has been dead weight since 001: no CLI flag, TUI field, or
-- import path can set it, and calculateNextDate never reads it. It is
-- also redundant by construction — weekly advance is next_date +
-- (interval * 7 days) and fortnightly is + 14 days, so the weekday of
-- next_date is preserved forever; the anchor date already encodes the
-- weekday selection a day_of_week column would express.
--
-- DuckDB can't ALTER a column out from under a CHECK constraint, so we
-- backup-drop-recreate the table (same pattern as migrations 018/019).
-- The recreated definition is migration 019's plus transfer_account_id
-- (added by 022), minus day_of_week.

CREATE TEMPORARY TABLE scheduled_transactions_backup AS
    SELECT * FROM scheduled_transactions;
DROP TABLE scheduled_transactions;

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
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    auto_post BOOLEAN NOT NULL DEFAULT FALSE,
    post_lead_days INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    transfer_account_id UUID
);

INSERT INTO scheduled_transactions (
    id, account_id, payee_id, category_id, amount, memo,
    frequency, interval, start_date, end_date, occurrences,
    day_of_month, secondary_day_of_month, next_date,
    occurrences_remaining, amount_estimate_count, auto_post,
    post_lead_days, created_at, updated_at, transfer_account_id
)
SELECT
    id, account_id, payee_id, category_id, amount, memo,
    frequency, interval, start_date, end_date, occurrences,
    day_of_month, secondary_day_of_month, next_date,
    occurrences_remaining, amount_estimate_count, auto_post,
    post_lead_days, created_at, updated_at, transfer_account_id
FROM scheduled_transactions_backup;

DROP TABLE scheduled_transactions_backup;

CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);
