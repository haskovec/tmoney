package db

// Reindex drops and recreates every secondary index in the database, rebuilding
// each ART index from the table data. It repairs a desynced index — DuckDB can
// leave a secondary index out of sync with the table on disk (a row present in
// the table whose key is missing from the index). The next UPDATE that rewrites
// such a row (DuckDB turns any UPDATE touching an indexed/FK-backed column into
// an internal DELETE+INSERT) then aborts with "Failed to delete all rows from
// index. Only deleted 0 out of 1 rows", blocking reconcile, edits, and voids of
// that row.
//
// It rebuilds ALL secondary indexes rather than one table's, because the desync
// can affect any of them — the failure that surfaced during reconcile-finish hit
// both transactions.transfer_id and reconciliation_sessions' indexes, and the one
// that blocked a scheduled post hit both of scheduled_transactions' indexes at
// once. The index set is read from duckdb_indexes() so it always matches the live
// schema, which also means a deliberately dropped index stays dropped:
// transactions(status) and reconciliation_sessions(status) from migration 030,
// and scheduled_transactions(account_id) from migration 033, are absent from the
// catalog and so are never recreated here.
//
// SCOPE LIMIT: duckdb_indexes() lists only indexes made by CREATE INDEX. On
// DuckDB 1.5.5 the ARTs backing a PRIMARY KEY, FOREIGN KEY or UNIQUE constraint
// do not appear in it at all — every row it returns has is_primary = false and a
// non-NULL sql, so the filter below excludes nothing — and no SQL reaches them
// (DROP INDEX gives a Catalog Error, ALTER TABLE ... DROP CONSTRAINT is not
// implemented). They fail identically when desynced, because the abort comes from
// generic per-index code. So a desync in a constraint-backed ART is beyond this
// repair, and the file needs a table rebuild instead — the backup/drop/recreate
// shape the migrations use. If a reindex does not clear the error, that is the
// case you are in.
//
// Each statement runs in autocommit. It returns the number of indexes rebuilt.
// The connection is then reset so a later UPDATE does not run on the connection
// that just built the indexes — the same reconnect the open path performs after
// migrations.
func (db *DB) Reindex() (int, error) {
	type indexDef struct{ name, sql string }

	rows, err := db.live().Query(`
		SELECT index_name, sql FROM duckdb_indexes()
		WHERE NOT is_primary AND sql IS NOT NULL
		ORDER BY table_name, index_name
	`)
	if err != nil {
		return 0, &DatabaseError{Op: "list indexes", Err: err}
	}
	var indexes []indexDef
	for rows.Next() {
		var d indexDef
		if err := rows.Scan(&d.name, &d.sql); err != nil {
			_ = rows.Close()
			return 0, &DatabaseError{Op: "scan index", Err: err}
		}
		indexes = append(indexes, d)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, &DatabaseError{Op: "iterate indexes", Err: err}
	}
	_ = rows.Close()

	for _, d := range indexes {
		if _, err := db.live().Exec("DROP INDEX IF EXISTS " + d.name); err != nil {
			return 0, &DatabaseError{Op: "drop index " + d.name, Err: err}
		}
		if _, err := db.live().Exec(d.sql); err != nil {
			return 0, &DatabaseError{Op: "create index " + d.name, Err: err}
		}
	}

	// Reset the connection so subsequent UPDATEs don't hit DuckDB's
	// "UPDATE on the connection that created the index" issue.
	if err := db.reconnect(); err != nil {
		return 0, err
	}
	return len(indexes), nil
}
