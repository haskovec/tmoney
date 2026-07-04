-- Migration 030: Drop the secondary indexes on transactions(status) and
-- reconciliation_sessions(status).
--
-- Reconcile-finish changes only a transaction's status and then only a session's
-- status/completed_at; the cleared/uncleared toggle and un-reconcile likewise
-- change only status. DuckDB turns an UPDATE that touches an indexed or FK-backed
-- column into an internal DELETE+INSERT, and it can leave a secondary ART index
-- desynced from the table on disk (a storage bug); the rewrite then aborts with
-- "Failed to delete all rows from index. Only deleted 0 out of 1 rows" -- exactly
-- what broke reconcile-finish, on both a transfer whose transfer_id index entry
-- could no longer be deleted and the reconciliation_sessions row.
--
-- status holds a handful of values on both tables, so its index provides
-- negligible query benefit (the planner scans regardless). Dropping it lets a
-- status-only UPDATE run as a true in-place update -- no DELETE+INSERT, no index
-- maintenance -- so status changes never rewrite the row and are immune to a
-- desynced index on any other column (e.g. transfer_id / account_id).
-- transaction.Repository.UpdateStatus and reconciliation.Repository.UpdateStatus
-- perform those narrow updates. This is migration 021's Path A (drop a low-value
-- secondary index to unblock UPDATE), applied to the two status columns.
--
-- Repairing an index that is ALREADY desynced (rebuilding the remaining
-- indexes -- transfer_id, account_id, and so on) is a separate concern handled by
-- `tmoney db reindex`, which rebuilds every secondary index in autocommit --
-- DuckDB aborts a CREATE INDEX issued inside a transaction, and migrations run
-- inside one, so the rebuild cannot live here.

DROP INDEX IF EXISTS idx_transactions_status;
DROP INDEX IF EXISTS idx_reconciliation_sessions_status;
