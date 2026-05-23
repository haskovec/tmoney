-- Migration 021: Drop secondary indexes on accounts that block UPDATE.
--
-- DuckDB rewrites UPDATE on an indexed column as DELETE+INSERT internally.
-- When the row is referenced by a child FK (transactions.account_id, etc.),
-- that internal DELETE trips the FK constraint — the same failure as an
-- explicit DELETE+INSERT. Account rename surfaces this any time the
-- account already has transactions. The accounts table is tiny (typically
-- well under 100 rows for a personal-finance file), so the secondary
-- indexes on name/type/active provide negligible benefit; dropping them
-- unblocks UPDATE. The primary-key index on id is preserved.

DROP INDEX idx_accounts_name;
DROP INDEX idx_accounts_type;
DROP INDEX idx_accounts_active;
