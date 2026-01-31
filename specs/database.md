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
        'daily', 'weekly', 'biweekly', 'monthly',
        'quarterly', 'yearly'
    )),
    interval INTEGER NOT NULL DEFAULT 1,
    start_date DATE NOT NULL,
    end_date DATE,
    occurrences INTEGER,
    day_of_month INTEGER CHECK (day_of_month BETWEEN -1 AND 31),
    day_of_week INTEGER CHECK (day_of_week BETWEEN 0 AND 6),
    next_date DATE NOT NULL,
    occurrences_remaining INTEGER,
    amount_estimate_count INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_scheduled_account ON scheduled_transactions(account_id);
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

## Backup and Recovery

### Backup Strategy

1. DuckDB file can be copied while closed
2. Export to SQL dump: `EXPORT DATABASE 'backup_folder'`
3. Future: Built-in backup command

### Recovery

1. Copy backup file over corrupted file
2. Import from SQL dump
3. DuckDB has built-in crash recovery

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
