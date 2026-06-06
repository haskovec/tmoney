-- Migration 023: Drop secondary indexes on investment_positions and
-- investment_lots that block UPDATE/DELETE.
--
-- Same DuckDB limitation as migration 021 (accounts): DuckDB rewrites an
-- UPDATE on an indexed column — and an explicit DELETE (positionRepo's
-- DELETE+INSERT upsert) — as an index delete, and these ART indexes drift
-- out of sync with the table over a file's lifetime. The delete then fails
-- with "Failed to delete all rows from index. Only deleted N out of M rows",
-- which fatally invalidates the database connection.
--
-- This surfaced once per-security position/lot healing began running on
-- databases that contain corporate-action records (HealAllAccounts on open):
-- recomputing a position upserts it (DELETE+INSERT), and recomputing a lot
-- updates its indexed `closed` column — both trip the stale index.
--
-- These tables are small in a personal-finance file (positions: one row per
-- held security per account; lots: a few hundred to low thousands), so a full
-- scan is cheap and the secondary indexes provide negligible benefit. Dropping
-- them rebuilds nothing stale and unblocks the operations. The primary-key
-- index on each table's `id` and the unique constraint on
-- investment_positions(account_id, security_id) are preserved.

DROP INDEX idx_positions_account;
DROP INDEX idx_positions_security;
DROP INDEX idx_lots_account;
DROP INDEX idx_lots_security;
DROP INDEX idx_lots_closed;
