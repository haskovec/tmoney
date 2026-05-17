-- Migration 016: Drop FK from scheduled_split_items.scheduled_transaction_id
-- to scheduled_transactions(id).
--
-- DuckDB implements UPDATE as DELETE+INSERT internally, which fails when child
-- rows reference the parent via an FK constraint. With the FK in place,
-- updating any column on a scheduled_transactions row (e.g. advancing
-- next_date after posting a multi-line schedule) trips a constraint error
-- once scheduled_split_items children exist. Application-level cascade in
-- scheduled.Repository.Delete and scheduled.Service.{Create,Update,Delete}
-- already enforces referential integrity — same precedent as migration 010
-- (security FKs) and migration 013 (lot FK).

CREATE TEMPORARY TABLE scheduled_split_items_backup AS SELECT * FROM scheduled_split_items;
DROP TABLE scheduled_split_items;

CREATE TABLE scheduled_split_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scheduled_transaction_id UUID NOT NULL,
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))
);

INSERT INTO scheduled_split_items SELECT * FROM scheduled_split_items_backup;
DROP TABLE scheduled_split_items_backup;

CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);
