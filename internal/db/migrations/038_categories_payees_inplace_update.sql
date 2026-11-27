-- Migration 038: Unblock in-place UPDATE on categories and payees.
--
-- DuckDB (including v2.0.0) rewrites an UPDATE that touches any secondary-
-- indexed or outbound-FK column as an internal DELETE+INSERT. When the row is
-- referenced by an inbound foreign key from a child table (transactions,
-- transaction_splits, payee_aliases, scheduled_transactions,
-- scheduled_split_items), that internal DELETE trips the FK constraint.
--
-- Previously, category.Repository.Update and payee.Repository.Update worked
-- around this with an explicit non-transactional DELETE+INSERT, which both
-- failed on any referenced category/payee and risked permanently deleting the
-- row if the INSERT failed.
--
-- Following the precedent of migrations 021 (dropping low-value secondary
-- indexes on accounts) and 010/013/016/026 (dropping blocking FKs in favor of
-- in-tx repository checks), this migration:
--   1. Drops the four low-value secondary indexes on these small tables:
--      idx_categories_parent, idx_categories_type, idx_categories_name_parent,
--      and idx_payees_name.
--   2. Rebuilds categories and payees without their two outbound FKs
--      (categories.parent_id REFERENCES categories(id) and
--      payees.default_category_id REFERENCES categories(id)), which are
--      enforced in-tx at the repository/service layer.
--   3. Preserves EVERY inbound FK from child tables (transactions,
--      transaction_splits, payee_aliases, scheduled_transactions,
--      scheduled_split_items) intact — in-place UPDATE on categories and payees
--      now succeeds even when referenced.
--
-- Because DuckDB requires dropping child tables holding an FK to a parent
-- before dropping the parent, the five referencing child tables are backed up,
-- dropped, and recreated verbatim from their version-037 catalog definitions.

-- Step 1: Drop dependent views.
DROP VIEW IF EXISTS account_balances;
DROP VIEW IF EXISTS category_spending;

-- Step 2: Back up and drop tables holding FKs to categories or payees.
CREATE TEMPORARY TABLE scheduled_split_items_backup AS SELECT * FROM scheduled_split_items;
DROP TABLE scheduled_split_items;

CREATE TEMPORARY TABLE scheduled_transactions_backup AS SELECT * FROM scheduled_transactions;
DROP TABLE scheduled_transactions;

CREATE TEMPORARY TABLE transaction_splits_backup AS SELECT * FROM transaction_splits;
DROP TABLE transaction_splits;

CREATE TEMPORARY TABLE transactions_backup AS SELECT * FROM transactions;
DROP TABLE transactions;

CREATE TEMPORARY TABLE payee_aliases_backup AS SELECT * FROM payee_aliases;
DROP TABLE payee_aliases;

CREATE TEMPORARY TABLE payees_backup AS SELECT * FROM payees;
DROP TABLE payees;

CREATE TEMPORARY TABLE categories_backup AS SELECT * FROM categories;
DROP TABLE categories;

-- Step 3: Recreate categories without secondary indexes or self-FK on parent_id.
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    parent_id UUID,
    type TEXT NOT NULL CHECK (type IN ('income', 'expense')),
    system_category BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO categories SELECT * FROM categories_backup;
DROP TABLE categories_backup;

-- Step 4: Recreate payees without idx_payees_name or outbound FK on default_category_id.
CREATE TABLE payees (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name TEXT NOT NULL,
    default_category_id UUID,
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO payees SELECT * FROM payees_backup;
DROP TABLE payees_backup;

-- Step 5: Recreate child tables verbatim from the version-037 catalog,
-- preserving all inbound FKs to categories(id) and payees(id).

-- payee_aliases
CREATE TABLE payee_aliases (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES payees(id),
    pattern TEXT NOT NULL,
    match_type TEXT NOT NULL CHECK (match_type IN (
        'exact', 'contains', 'starts_with', 'regex'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO payee_aliases SELECT * FROM payee_aliases_backup;
DROP TABLE payee_aliases_backup;
CREATE INDEX idx_payee_aliases_payee ON payee_aliases(payee_id);
CREATE INDEX idx_payee_aliases_pattern ON payee_aliases(pattern);

-- transactions
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    amount DECIMAL(19, 4) NOT NULL,
    payee_id UUID REFERENCES payees(id),
    category_id UUID REFERENCES categories(id),
    memo TEXT,
    check_number TEXT,
    status TEXT NOT NULL DEFAULT 'uncleared' CHECK (status IN (
        'uncleared', 'cleared', 'reconciled', 'void'
    )),
    transfer_id UUID,
    transfer_account_id UUID REFERENCES accounts(id),
    bank_reference_id TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO transactions SELECT * FROM transactions_backup;
DROP TABLE transactions_backup;
CREATE INDEX idx_transactions_account ON transactions(account_id);
CREATE INDEX idx_transactions_bank_ref ON transactions(bank_reference_id);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_payee ON transactions(payee_id);
CREATE INDEX idx_transactions_transfer ON transactions(transfer_id);

-- transaction_splits
CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
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
INSERT INTO transaction_splits SELECT * FROM transaction_splits_backup;
DROP TABLE transaction_splits_backup;
CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
CREATE INDEX idx_splits_transfer ON transaction_splits(transfer_id);

-- scheduled_transactions
CREATE TABLE scheduled_transactions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
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
INSERT INTO scheduled_transactions SELECT * FROM scheduled_transactions_backup;
DROP TABLE scheduled_transactions_backup;
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);

-- scheduled_split_items
CREATE TABLE scheduled_split_items (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
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
INSERT INTO scheduled_split_items SELECT * FROM scheduled_split_items_backup;
DROP TABLE scheduled_split_items_backup;
CREATE INDEX idx_scheduled_split_items_parent
    ON scheduled_split_items(scheduled_transaction_id);

-- Step 6: Recreate the views verbatim from migration 034.
CREATE VIEW account_balances AS
SELECT
    a.id,
    a.name,
    a.type,
    a.opening_balance,
    a.opening_balance + COALESCE(
        SUM(CASE WHEN t.status != 'void' THEN t.amount ELSE 0 END), 0
    ) AS current_balance,
    a.opening_balance + COALESCE(
        SUM(CASE WHEN t.status IN ('cleared', 'reconciled')
            THEN t.amount ELSE 0 END), 0
    ) AS cleared_balance
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id
GROUP BY a.id, a.name, a.type, a.opening_balance;

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
    ON t.category_id = c.id
   AND t.status != 'void'
   AND t.transfer_id IS NULL
GROUP BY c.id, c.name, c.parent_id, c.type, DATE_TRUNC('month', t.date);
