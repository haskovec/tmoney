-- Migration 015: Add scheduled_split_items table.
--
-- Stores multi-line templates for scheduled transactions per
-- multiline-splits-and-paycheck.md. Mirrors transaction_splits but without
-- a transfer_id column — templates don't link to real counter-transactions;
-- the paired transaction (and its transfer_id) is minted at post time.
--
-- A scheduled transaction is multi-line when it has one or more rows here;
-- otherwise it stays single-line using the scalar amount / category_id on
-- scheduled_transactions. The two shapes cannot coexist on the same record
-- (enforced at the service layer in a later task — MS-015).
--
-- No FK on transfer_account_id: same lot-FK precedent (migration 013) — the
-- service layer maintains referential integrity. categories and
-- scheduled_transactions FKs are kept since those tables aren't rewritten
-- by subsequent migrations.

CREATE TABLE scheduled_split_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scheduled_transaction_id UUID NOT NULL REFERENCES scheduled_transactions(id),
    category_id UUID REFERENCES categories(id),
    transfer_account_id UUID,
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))
);

CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);
