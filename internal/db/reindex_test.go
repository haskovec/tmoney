package db

import (
	"path/filepath"
	"testing"
)

// indexCount returns how many indexes named `name` exist.
func indexCount(t *testing.T, db *DB, name string) int {
	t.Helper()
	var count int
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM duckdb_indexes() WHERE index_name = ?`, name,
	).Scan(&count); err != nil {
		t.Fatalf("querying duckdb_indexes() for %s: %v", name, err)
	}
	return count
}

// TestMigration030DropsStatusIndexes verifies migration 030 removed the
// secondary indexes on transactions(status) and reconciliation_sessions(status),
// so a status-only UPDATE runs in place rather than as a DELETE+INSERT rewrite.
func TestMigration030DropsStatusIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.tdb")
	database, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer database.Close()

	for _, name := range []string{"idx_transactions_status", "idx_reconciliation_sessions_status"} {
		if got := indexCount(t, database, name); got != 0 {
			t.Errorf("%s should be dropped by migration 030, still present (count=%d)", name, got)
		}
	}
}

// TestMigration033DropsScheduledAccountIndex verifies migration 033 removed the
// secondary index on scheduled_transactions(account_id), which the planner could
// never use (both queries compare CAST(account_id AS VARCHAR)) and which was one
// of the two ARTs whose desync blocked a scheduled post. idx_scheduled_next_date
// is deliberately kept.
func TestMigration033DropsScheduledAccountIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.tdb")
	database, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer database.Close()

	if got := indexCount(t, database, "idx_scheduled_account"); got != 0 {
		t.Errorf("idx_scheduled_account should be dropped by migration 033, still present (count=%d)", got)
	}
	if got := indexCount(t, database, "idx_scheduled_next_date"); got != 1 {
		t.Errorf("idx_scheduled_next_date must be kept, got count=%d", got)
	}
}

// TestReindex rebuilds every secondary index and confirms the full set is
// present afterward, the deliberately-dropped status index stays dropped, and
// tables remain queryable. It guards the rebuild's mechanics and that the count
// matches the catalog; desync_test.go exercises the actual repair against a
// genuinely desynced index.
func TestReindex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.tdb")
	database, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer database.Close()

	// Count secondary indexes before, so we can assert Reindex touched them all.
	var want int
	if err := database.conn.QueryRow(
		`SELECT COUNT(*) FROM duckdb_indexes() WHERE NOT is_primary AND sql IS NOT NULL`,
	).Scan(&want); err != nil {
		t.Fatalf("counting indexes: %v", err)
	}
	if want == 0 {
		t.Fatal("expected some secondary indexes to rebuild")
	}

	// Idempotent: running twice must succeed and leave a consistent index set.
	var got int
	for i := range 2 {
		got, err = database.Reindex()
		if err != nil {
			t.Fatalf("Reindex() run %d error = %v", i+1, err)
		}
	}
	if got != want {
		t.Errorf("Reindex() rebuilt %d indexes, want %d", got, want)
	}

	// Representative indexes are present; the status index stays dropped.
	for _, name := range []string{"idx_transactions_transfer", "idx_reconciliation_sessions_account"} {
		if c := indexCount(t, database, name); c != 1 {
			t.Errorf("index %q should exist once after reindex, got count=%d", name, c)
		}
	}
	if c := indexCount(t, database, "idx_transactions_status"); c != 0 {
		t.Errorf("reindex must not resurrect idx_transactions_status (count=%d)", c)
	}

	var n int
	if err := database.conn.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&n); err != nil {
		t.Fatalf("transactions table not queryable after reindex: %v", err)
	}
}
