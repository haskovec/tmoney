-- Migration 028: Add nullable loan_section column to scheduled_split_items.
--
-- Tags each multi-line scheduled-split row with the loan-wizard section it
-- belongs to ('interest' | 'principal' | 'escrow'). The column is nullable:
-- only the loan wizard sets it, and every other multi-line schedule (plus
-- any line added through the generic split dialog) keeps NULL. A schedule
-- is "loan-shaped" — and gets recompute-at-post — only when every line
-- carries a non-NULL loan_section, so the NULL state is the "treat as
-- generic multi-line split" signal (mirroring paycheck_section, migration
-- 020).
--
-- Unlike paycheck_section, the tag lives on scheduled_split_items ONLY, not
-- on transaction_splits: posted transactions need no tag (the interest line
-- is identifiable by its category, the principal line by its transfer
-- target's account type), and transaction_splits.paycheck_section is in fact
-- never populated by any posting path — there is no copy-through precedent to
-- follow.
--
-- A split belongs to at most one wizard family: the
-- (paycheck_section IS NULL OR loan_section IS NULL) CHECK forbids a row from
-- being tagged both a paycheck line and a loan line.
--
-- DuckDB does not support ALTER TABLE ADD COLUMN with a CHECK constraint, so
-- we use the backup-drop-recreate pattern established by migrations 014, 015,
-- and 020 for this table.

-- scheduled_split_items (latest definition from migration 020, which added
-- paycheck_section; migration 016 had already dropped the
-- scheduled_transaction_id FK, so none is recreated here).
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
    loan_section TEXT CHECK (loan_section IS NULL OR loan_section IN (
        'interest', 'principal', 'escrow'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL)),
    CHECK (paycheck_section IS NULL OR loan_section IS NULL)
);

INSERT INTO scheduled_split_items (
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, paycheck_section, loan_section, created_at
)
SELECT
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, paycheck_section, NULL, created_at
FROM scheduled_split_items_backup;

DROP TABLE scheduled_split_items_backup;

CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);
