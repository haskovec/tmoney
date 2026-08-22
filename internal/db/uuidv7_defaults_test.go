package db

import (
	"path/filepath"
	"testing"
)

// TestUUIDColumnDefaultsAreV7 pins every UUID column DEFAULT to uuidv7().
//
// types.NewID generates v7 so that ids sort in insertion order and cluster in
// the index; migration 032 brought the schema DEFAULTs in line with that. The
// easy way to undo it is a later migration that rebuilds a table from an older
// CREATE TABLE body — 031 copied `DEFAULT gen_random_uuid()` forward exactly
// that way — so this asserts against the live catalog rather than the
// migration text, and catches the drift wherever it comes from.
func TestUUIDColumnDefaultsAreV7(t *testing.T) {
	database, err := Create(filepath.Join(t.TempDir(), "defaults.tdb"))
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	defer func() { _ = database.Close() }()

	rows, err := database.Conn().Query(
		`SELECT table_name, column_name, column_default
		   FROM duckdb_columns()
		  WHERE data_type = 'UUID' AND column_default IS NOT NULL
		  ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatalf("read column defaults: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var checked int
	for rows.Next() {
		var table, column, def string
		if err := rows.Scan(&table, &column, &def); err != nil {
			t.Fatalf("scan column default: %v", err)
		}
		checked++
		if def != "uuidv7()" {
			t.Errorf("%s.%s defaults to %s, want uuidv7()", table, column, def)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column defaults: %v", err)
	}

	// Guard against the query silently matching nothing and passing vacuously.
	if checked == 0 {
		t.Fatal("no UUID column defaults found; the catalog query is wrong")
	}
}
