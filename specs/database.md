# Database Specification

## Overview

TMoney uses DuckDB as its embedded database. Each "money file" is a self-contained DuckDB database file with a `.tdb` extension.

## File Format

| Property | Value |
|----------|-------|
| Extension | `.tdb` |
| Format | DuckDB database file |
| Default Location | `~/Documents/TMoney/` |
| Encoding | UTF-8 for text |

### Default Location (OS-Agnostic)

| OS | Path |
|----|------|
| macOS | `~/Documents/TMoney/` |
| Linux | `~/Documents/TMoney/` |
| Windows | `%USERPROFILE%\Documents\TMoney\` |

The application creates this directory if it doesn't exist.

## Metadata Table

Every TMoney database has a `_metadata` table to identify and version the file.

```sql
CREATE TABLE _metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Required entries:
INSERT INTO _metadata VALUES ('app_identifier', 'tmoney');
INSERT INTO _metadata VALUES ('schema_version', '1');
INSERT INTO _metadata VALUES ('created_at', '2024-01-15T10:30:00Z');
INSERT INTO _metadata VALUES ('default_currency', 'USD');
```

### Metadata Keys

| Key | Description | Example |
|-----|-------------|---------|
| `app_identifier` | Always "tmoney" | `tmoney` |
| `schema_version` | Database schema version | `1` |
| `created_at` | When file was created | ISO 8601 timestamp |
| `default_currency` | Default currency for new accounts | `USD` |
| `last_opened` | Last time file was opened | ISO 8601 timestamp |

## Schema Version Management

### Version Check on Open

```
1. Open database file
2. Check _metadata table exists
3. Verify app_identifier = 'tmoney'
4. Read schema_version
5. If version < current_app_version:
   - Run migrations
   - Update schema_version
6. If version > current_app_version:
   - Error: "File was created with newer version"
```

### Migration Strategy

Migrations are sequential SQL scripts:
- `001_initial.sql`
- `002_add_feature_x.sql`
- `003_fix_column_type.sql`

Each migration:
1. Runs in a transaction
2. Updates schema_version on success
3. Rolls back on failure

## Schema Definition

### accounts

```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN (
        'checking', 'savings', 'credit_card',
        'investment', 'cash', 'loan', 'asset'
    )),
    currency TEXT NOT NULL DEFAULT 'USD',
    institution TEXT,
    account_number TEXT,
    opening_balance DECIMAL(19, 4) NOT NULL DEFAULT 0,
    opening_date DATE NOT NULL,
    credit_limit DECIMAL(19, 4),
    interest_rate DECIMAL(5, 4),
    notes TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_accounts_type ON accounts(type);
CREATE INDEX idx_accounts_active ON accounts(active);
```

### categories

```sql
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    parent_id UUID REFERENCES categories(id),
    type TEXT NOT NULL CHECK (type IN ('income', 'expense')),
    system_category BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (name, parent_id)
);

CREATE INDEX idx_categories_parent ON categories(parent_id);
CREATE INDEX idx_categories_type ON categories(type);
```

### payees

```sql
CREATE TABLE payees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    default_category_id UUID REFERENCES categories(id),
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_payees_name ON payees(name);
```

### payee_aliases

```sql
CREATE TABLE payee_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payee_id UUID NOT NULL REFERENCES payees(id) ON DELETE CASCADE,
    pattern TEXT NOT NULL UNIQUE,
    match_type TEXT NOT NULL CHECK (match_type IN (
        'exact', 'contains', 'starts_with', 'regex'
    )),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_payee_aliases_payee ON payee_aliases(payee_id);
CREATE INDEX idx_payee_aliases_pattern ON payee_aliases(pattern);
```

### transactions

```sql
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    date DATE NOT NULL,
    amount DECIMAL(19, 4) NOT NULL,
    payee_id UUID REFERENCES payees(id),
    category_id UUID REFERENCES categories(id),
    memo TEXT,
    check_number TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'cleared', 'reconciled'
    )),
    transfer_id UUID,
    transfer_account_id UUID REFERENCES accounts(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transactions_account ON transactions(account_id);
CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_payee ON transactions(payee_id);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_transfer ON transactions(transfer_id);
CREATE INDEX idx_transactions_status ON transactions(status);
```

### transaction_splits

```sql
CREATE TABLE transaction_splits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id),
    amount DECIMAL(19, 4) NOT NULL,
    memo TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_splits_transaction ON transaction_splits(transaction_id);
```

> **Note — stale DDL (pre-`014`).** The definition above predates the
> transfer-line and category changes; the live schema has since added
> `transfer_account_id` / `transfer_id` columns and evolved the constraints.
> As of migration `029` the table carries a *relaxed* check —
> `CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL)` plus the
> pairing check `CHECK ((transfer_account_id IS NULL) = (transfer_id IS NULL))`
> — which permits a "categorized transfer" split row with both `category_id`
> and `transfer_account_id` set. See the numbered migration files (through
> `029_transfer_categories.sql`) for the authoritative current shape, and
> [`specs/transfer-categories.md`](transfer-categories.md) for the rationale.

### investment_lots

```sql
CREATE TABLE investment_lots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    quantity DECIMAL(19, 8) NOT NULL,
    purchase_price DECIMAL(19, 4) NOT NULL,
    purchase_date DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lots_account ON investment_lots(account_id);
CREATE INDEX idx_lots_symbol ON investment_lots(symbol);
```

### scheduled_transactions

```sql
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
    transfer_account_id UUID  -- no FK/index by design (see migration 022)
);

-- idx_scheduled_account was dropped by migration 033: both queries on that
-- column compare CAST(account_id AS VARCHAR), so the index was unreachable.
CREATE INDEX idx_scheduled_next_date ON scheduled_transactions(next_date);
```

## Decimal Precision

All monetary values use `DECIMAL(19, 4)`:
- 19 total digits
- 4 decimal places
- Supports values up to 999,999,999,999,999.9999

Investment quantities use `DECIMAL(19, 8)` for fractional shares.

## UUID Generation

DuckDB supports UUID generation via `gen_random_uuid()`.

## Timestamps

All timestamps stored in UTC as `TIMESTAMP` type.
- `created_at`: Set on insert, never modified
- `updated_at`: Updated on every modification

## Views

### account_balances

```sql
CREATE VIEW account_balances AS
SELECT
    a.id,
    a.name,
    a.type,
    a.opening_balance,
    a.opening_balance + COALESCE(SUM(t.amount), 0) AS current_balance,
    a.opening_balance + COALESCE(
        SUM(CASE WHEN t.status IN ('cleared', 'reconciled')
            THEN t.amount ELSE 0 END), 0
    ) AS cleared_balance
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id
GROUP BY a.id, a.name, a.type, a.opening_balance;
```

### category_spending

```sql
CREATE VIEW category_spending AS
SELECT
    c.id,
    c.name,
    c.parent_id,
    c.type,
    DATE_TRUNC('month', t.date) AS month,
    SUM(t.amount) AS total
FROM categories c
LEFT JOIN transactions t ON t.category_id = c.id
GROUP BY c.id, c.name, c.parent_id, c.type, DATE_TRUNC('month', t.date);
```

> **Note — migration `029` recreates this view.** The join above is the
> original, unguarded shape. Migration `029_transfer_categories.sql`
> recreates `category_spending` with `AND t.status != 'void' AND
> t.transfer_id IS NULL` in the `LEFT JOIN`, so voided rows and
> categorized transfers stay out of its totals — matching the explicit
> transfer guard the spending report gained. See
> [`specs/transfer-categories.md`](transfer-categories.md).

## Backup and Recovery

### Backup Strategy

1. DuckDB file can be copied while closed
2. Export to SQL dump: `EXPORT DATABASE 'backup_folder'`
3. Future: Built-in backup command

### Recovery

1. Copy backup file over corrupted file
2. Import from SQL dump
3. DuckDB has built-in crash recovery

### Index Repair (`db reindex`)

DuckDB can leave a secondary ART index desynced from its table on disk — a row
is present in the table but its key is missing from the index. Because DuckDB
turns an UPDATE that touches an indexed or FK-backed column into an internal
DELETE+INSERT, the next UPDATE that rewrites the affected row aborts with:

```
FATAL Error: Invalid Input Error: Failed to delete all rows from index.
Only deleted 0 out of 1 rows.
```

The state is reproducible, and `internal/db/desync_test.go` pins it: write an
index-backed row, shut down without the checkpoint that folds the WAL into the
file, then reopen. The WAL replay restores the row to the table but not its key
to the index. A tmoney process that dies without a clean `Close()` — a crash, an
OOM kill, a closed terminal — leaves the same state behind.

The abort lands on **commit**, not on the statement: DuckDB defers the index
erase of an already-stored row to commit time, so the `UPDATE` reports success
and a correct `RowsAffected`, and only `tx.Commit()` fails. That is why the
user-visible text starts `database error during commit transaction`, from
`db.WithTx`.

`tmoney db reindex` repairs it. It reads every secondary index from
`duckdb_indexes()` and, for each, runs `DROP INDEX` followed by the stored
`CREATE INDEX`, rebuilding the ART from the table data. It changes no financial
data. Each statement runs in **autocommit**, and `db.Reindex()` reconnects
afterward so a later `UPDATE` does not run on the connection that just built the
indexes. Close the TUI first: the repair opens the file itself, and DuckDB's
per-process file lock refuses it while tmoney holds the file.

**Scope limit.** `duckdb_indexes()` lists only indexes created by `CREATE INDEX`.
On DuckDB 1.5.5 the ARTs backing a `PRIMARY KEY`, `FOREIGN KEY` or `UNIQUE`
constraint are absent from it entirely, and no SQL reaches them. A desync in one
of those fails the same way but is beyond `db reindex`; that file needs a
backup/drop/recreate table rebuild. If a reindex does not clear the error, that
is the case you are in.

**A fatal error invalidates the whole instance,** not just the failed statement.
Every later query then reports `database has been invalidated ... must be
restarted`. `db.WithTx` handles that itself: on a failure reported by begin,
rollback or commit it calls `healAfterFatal`, which probes the handle with a real
query and reconnects when the probe fails. The probe must be a real query —
`sql.DB.Ping()` returns nil against a poisoned handle and cannot be used to
detect this.

The coverage is exactly the writes that go through `WithTx`. An autocommit
statement issued straight through `q()` / `Conn()` — `scheduled.Repository.
HealNextDates` is the one such write on a desync-prone table — bypasses the heal,
so a fatal there still leaves the session needing a restart.

Two properties of the heal matter more than they look:

- It **does not hold `connMu` across the reopen.** duckdb-go opens the file
  inside `sql.Open`, and DuckDB blocks there on its instance lock until every
  connection to the old instance is released — which lasts as long as a reader
  holds an open `*sql.Rows`. Holding the lock across that parked every reader
  too, hanging the app; an app that hangs is a worse failure than the one being
  repaired.
- The inline wait is **bounded** (`healReconnectTimeout`), and a timed-out
  attempt keeps running in the background so it can still publish the new pool
  once the reader lets go. On a failed reopen the previous pool stays published,
  closed, so callers get `sql: database is closed` rather than a nil `*sql.DB`
  they would dereference into a panic.

### Avoiding the rewrite in the first place

A low-value secondary index is worth dropping, because each one is another ART
that can desync and block a write:

- Migration 021 drops the `accounts` name/type/active indexes, so renaming an
  account with transactions no longer trips FK enforcement.
- Migration 030 drops `transactions(status)` and
  `reconciliation_sessions(status)`, so reconcile-finish, the cleared/uncleared
  toggle and un-reconcile update `status` (and `updated_at`/`completed_at`)
  **in place** — no rewrite, no index maintenance — via
  `transaction.Repository.UpdateStatus` and `reconciliation.Repository.UpdateStatus`.
- Migration 033 drops `scheduled_transactions(account_id)`. Both queries on that
  column compare `CAST(account_id AS VARCHAR)`, which hides the column from the
  index, so it was never reachable and cost only risk.

Narrowing an `UPDATE` only helps when the narrowed set excludes every indexed
column. It does not help `scheduled_transactions`, because the advance writes
`next_date` and `idx_scheduled_next_date` covers exactly that column. That index
is kept anyway: `ListDue` (`next_date <= CURRENT_DATE`) and `ListUpcoming`
(`next_date <= ?`) filter the bare column, so unlike `account_id` it is at least
reachable. (`ListAutoPostDue` wraps it in arithmetic —
`next_date - INTERVAL (post_lead_days) DAY <= ?` — so that one scans regardless.)
Header/amount/transfer edits and voids still rewrite the row, so those are what
need `db reindex` after a desync.

## Performance Considerations

1. Indexes on frequently queried columns
2. Views for common calculations
3. DuckDB is columnar - good for analytics
4. Consider VACUUM periodically for large files

## File Validation

On open, validate:
1. File exists and is readable
2. Is a valid DuckDB file
3. Contains `_metadata` table
4. `app_identifier` = "tmoney"
5. `schema_version` is compatible

Error messages:
- "File not found"
- "Not a valid TMoney file"
- "File was created with a newer version of TMoney"
- "File appears to be corrupted"
