-- Migration 018: Rename the 'biweekly' frequency value to 'fortnightly'.
--
-- The display label has been "Fortnightly" since the paycheck wizard
-- landed (biweekly reads as "twice per week" too often), but the
-- stored value was still 'biweekly' for back-compat. There are no
-- production databases to preserve, so the storage value is renamed
-- in place to match the display label.
--
-- DuckDB doesn't support DROP/ADD CHECK or in-place enum widening,
-- so we backup-drop-recreate the table (same pattern as migrations
-- 007, 010, 016, 017). Any rows currently using 'biweekly' are
-- rewritten to 'fortnightly' during the re-insert.

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
    day_of_week INTEGER CHECK (day_of_week BETWEEN 0 AND 6),
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    auto_post BOOLEAN NOT NULL DEFAULT FALSE,
    post_lead_days INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO scheduled_transactions (
    id, account_id, payee_id, category_id, amount, memo,
    frequency, interval, start_date, end_date, occurrences,
    day_of_month, secondary_day_of_month, day_of_week, next_date,
    occurrences_remaining, amount_estimate_count, auto_post,
    post_lead_days, created_at, updated_at
)
SELECT
    id, account_id, payee_id, category_id, amount, memo,
    CASE WHEN frequency = 'biweekly' THEN 'fortnightly' ELSE frequency END,
    interval, start_date, end_date, occurrences,
    day_of_month, secondary_day_of_month, day_of_week, next_date,
    occurrences_remaining, amount_estimate_count, auto_post,
    post_lead_days, created_at, updated_at
FROM scheduled_transactions_backup;

DROP TABLE scheduled_transactions_backup;

CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);
