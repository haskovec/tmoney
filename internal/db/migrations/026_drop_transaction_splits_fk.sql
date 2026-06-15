-- Migration 026: Drop FK from transaction_splits.transaction_id to
-- transactions(id).
--
-- DuckDB enforces a parent row's inbound foreign keys when the row is
-- rewritten, and it rewrites an UPDATE as DELETE+INSERT whenever the update
-- touches an indexed or FK-backed column. The transactions table carries
-- outbound FKs (account_id -> accounts, payee_id -> payees, category_id ->
-- categories) whose enforcement indexes force that rewrite, so updating ANY
-- header field on a transaction that has transaction_splits children trips
-- "Violates foreign key constraint". This broke reconcile-finish and the
-- cleared-status toggle on a split (multi-category) transaction — e.g. a split
-- paycheck deposit — and any edit of such a transaction's header. Dropping the
-- secondary indexes (migration 021's approach for accounts) is insufficient
-- here because the outbound-FK enforcement indexes remain.
--
-- Application-level integrity is already maintained: transaction.Service
-- creates splits with their parent (CreateWithSplits), clears them on delete
-- (Repository.Delete / Service.Delete via DeleteByTransaction), and rewrites
-- them through ReplaceSplits. So we drop the inbound FK and keep the parent's
-- query indexes — same precedent as migration 010 (security FKs), 013 (lot
-- FK), and 016 (scheduled_split_items FK). The transaction_splits ->
-- categories FK, its CHECK constraints, and its indexes are preserved.

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
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL)),
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
