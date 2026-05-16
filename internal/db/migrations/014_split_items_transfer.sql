-- Migration 014: Add transfer_account_id and transfer_id to transaction_splits.
--
-- Adds support for split-line transfers per multiline-splits-and-paycheck.md:
-- a split row may be either categorized (category_id set, transfer columns
-- NULL) or transferred to another account (transfer_account_id + transfer_id
-- set, category_id NULL). category_id is relaxed to NULLABLE; two CHECK
-- constraints enforce the exclusive shapes:
--   * exactly one of category_id / transfer_account_id is set
--   * transfer_id is set iff transfer_account_id is set
--
-- DuckDB does not support ALTER TABLE ADD COLUMN with check constraints or
-- ALTER COLUMN to relax NOT NULL, so we use the backup-drop-recreate pattern.
-- No FK on transfer_account_id: lot-FK precedent (migration 013) shows that
-- DuckDB enforces FKs on parent UPDATE too aggressively for our workflows;
-- the service layer maintains referential integrity.

CREATE TEMPORARY TABLE splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

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

INSERT INTO transaction_splits (
    id, transaction_id, category_id, transfer_account_id, transfer_id,
    amount, memo, created_at
)
SELECT
    id, transaction_id, category_id, NULL, NULL,
    amount, memo, created_at
FROM splits_backup;

DROP TABLE splits_backup;

CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
CREATE INDEX idx_splits_transfer ON transaction_splits(transfer_id);
