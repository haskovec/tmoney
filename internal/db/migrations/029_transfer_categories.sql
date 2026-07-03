-- Migration 029: Transfer categories.
--
-- Relaxes the mutual-exclusion CHECK on transaction_splits and
-- scheduled_split_items so a transfer line may ALSO carry a category — a
-- "categorized transfer" (e.g. a loan payment's principal line labeled
-- Loan:Principal, or a credit-card-payment transfer labeled Bills:Credit
-- Card). The category is purely a label for tracking why money moved; it
-- never changes balance math or transfer linkage and is never required.
--
-- The old XOR CHECK,
--     CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))
-- forbade the both-set shape. The new at-least-one CHECK,
--     CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL)
-- permits three valid shapes — categorized, transfer, and categorized
-- transfer — while still rejecting the empty row. Both legacy shapes
-- satisfy the new CHECK by construction, so the data copy is loss-free.
--
-- DuckDB cannot drop or alter an anonymous CHECK constraint, so both tables
-- use the established backup-drop-recreate recipe (migrations 014, 015, 020,
-- 026, 028). Every FK / index / other-CHECK decision is copied verbatim from
-- each table's latest definition — transaction_splits from migration 026,
-- scheduled_split_items from migration 028: no inbound FK on the parent id,
-- outbound category_id -> categories(id) preserved, transfer_account_id /
-- transfer_id kept as plain UUIDs with no FK and no index on
-- transfer_account_id (the UPDATE-as-DELETE+INSERT rewrite trap documented in
-- migration 026), the paycheck_section / loan_section enum CHECKs, the
-- at-most-one-section CHECK, and the query indexes recreated.
--
-- Also recreates the category_spending view (migration 019) with an explicit
-- `t.transfer_id IS NULL` guard so a categorized whole-transaction transfer
-- row cannot silently enter the view's per-category totals. No production Go
-- code reads this view — the report service uses its own query-local CTE — but
-- the guard keeps a direct .tdb query consistent with the report's semantics.

-- transaction_splits: relax the XOR CHECK to at-least-one, preserving the
-- pairing CHECK and the paycheck_section enum CHECK (latest definition:
-- migration 026).
CREATE TEMPORARY TABLE transaction_splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL,
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    transfer_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    paycheck_section TEXT CHECK (paycheck_section IS NULL OR paycheck_section IN (
        'earnings', 'pre_tax', 'tax', 'post_tax', 'net_pay_destination'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL),
    CHECK ((transfer_account_id IS NULL) = (transfer_id IS NULL))
);

INSERT INTO transaction_splits (
    id, transaction_id, category_id, transfer_account_id, transfer_id,
    amount, memo, paycheck_section, created_at
)
SELECT
    id, transaction_id, category_id, transfer_account_id, transfer_id,
    amount, memo, paycheck_section, created_at
FROM transaction_splits_backup;

DROP TABLE transaction_splits_backup;

CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
CREATE INDEX idx_splits_transfer ON transaction_splits(transfer_id);

-- scheduled_split_items: same XOR -> at-least-one relaxation, preserving the
-- paycheck_section / loan_section enum CHECKs and the at-most-one-section
-- CHECK (latest definition: migration 028). This table has no transfer_id
-- column (pairs are minted at post time), so only the exclusivity CHECK
-- changes.
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
    CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL),
    CHECK (paycheck_section IS NULL OR loan_section IS NULL)
);

INSERT INTO scheduled_split_items (
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, paycheck_section, loan_section, created_at
)
SELECT
    id, scheduled_transaction_id, category_id, transfer_account_id,
    amount, memo, paycheck_section, loan_section, created_at
FROM scheduled_split_items_backup;

DROP TABLE scheduled_split_items_backup;

CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);

-- Recreate category_spending with an explicit transfer guard so categorized
-- transfer rows never enter the view's per-category totals (matches the
-- report service's new default guard).
DROP VIEW IF EXISTS category_spending;
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
    ON t.category_id = c.id AND t.status != 'void' AND t.transfer_id IS NULL
GROUP BY c.id, c.name, c.parent_id, c.type, DATE_TRUNC('month', t.date);
