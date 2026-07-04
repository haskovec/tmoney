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
// both transactions.transfer_id and reconciliation_sessions' indexes. The index
// set is read from duckdb_indexes() so it always matches the live schema:
// primary-key and FK-enforcement indexes have no CREATE statement (sql IS NULL)
// and are skipped, and migration 030's deliberately dropped transactions(status)
// index is absent from the catalog, so it stays dropped.
//
// Each statement runs in autocommit: DuckDB 1.5.4 aborts a CREATE INDEX issued
// inside an explicit transaction, so this cannot be a migration (they run inside
// one). It returns the number of indexes rebuilt. The connection is then reset
// so a later UPDATE does not run on the connection that just built the indexes —
// the same reconnect the open path performs after migrations.
func (db *DB) Reindex() (int, error) {
	type indexDef struct{ name, sql string }

	rows, err := db.conn.Query(`
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
		if _, err := db.conn.Exec("DROP INDEX IF EXISTS " + d.name); err != nil {
			return 0, &DatabaseError{Op: "drop index " + d.name, Err: err}
		}
		if _, err := db.conn.Exec(d.sql); err != nil {
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
