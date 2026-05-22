-- Migration 020: Add nullable paycheck_section column to split tables.
--
-- Tags each multi-line split row with the v2 paycheck wizard section it
-- belongs to ('earnings' | 'pre_tax' | 'tax' | 'post_tax' |
-- 'net_pay_destination'). The column is nullable: only the wizard sets
-- it, and pre-v2 multi-line schedules (plus any line added through the
-- generic split dialog) keep NULL. The Edit-as-paycheck affordance only
-- surfaces when every line on a schedule carries a non-NULL tag, so the
-- NULL state is the "treat as generic multi-line split" signal.
--
-- DuckDB does not support ALTER TABLE ADD COLUMN with a CHECK
-- constraint, so we use the backup-drop-recreate pattern that
-- migrations 014 and 015 established for these tables.

-- transaction_splits (latest definition from migration 019).
CREATE TEMPORARY TABLE transaction_splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    transfer_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    paycheck_section TEXT CHECK (paycheck_section IS NULL OR paycheck_section IN (
        'earnings', 'pre_tax', 'tax', 'post_tax', 'net_pay_destination'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL)),
    CHECK ((transfer_account_id IS NULL) = (transfer_id IS NULL))
);

INSERT INTO transaction_splits (
    id, transaction_id, category_id, transfer_account_id, transfer_id,
    amount, memo, paycheck_section, created_at
)
SELECT
    id, transaction_id, category_id, transfer_account_id, transfer_id,
    amount, memo, NULL, created_at
FROM transaction_splits_backup;

DROP TABLE transaction_splits_backup;

CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
CREATE INDEX idx_splits_transfer ON transaction_splits(transfer_id);

-- scheduled_split_items (latest definition from migration 016 — no FK on
-- scheduled_transaction_id since 016 dropped it).
CREATE TEMPORARY TABLE scheduled_split_items_backup AS SELECT * FROM scheduled_split_items;
DROP TABLE scheduled_split_items;

CREATE TABLE scheduled_split_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scheduled_transaction_id UUID NOT NULL,
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    paycheck_section TEXT CHECK (paycheck_section IS NULL OR paycheck_section IN (
        'earnings', 'pre_tax', 'tax', 'post_tax', 'net_pay_destination'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))
);

INSERT INTO scheduled_split_items (
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, paycheck_section, created_at
)
SELECT
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, NULL, created_at
FROM scheduled_split_items_backup;

DROP TABLE scheduled_split_items_backup;

CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);
