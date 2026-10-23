package db

import (
	"os"
	"strings"
)

// The functions in this file work on a database file that is not open in this
// process: the backup package's own copy of a database. DuckDB caches one
// instance per path, so path must not be open here.

// ValidateFile checks, without changing the file, that path is a complete
// TMoney database this app can open: DuckDB attaches it read-only, the
// _metadata table names TMoney (validateTMoneyFile), and its schema version is
// not newer than this app. It runs no migrations and writes nothing.
//
// Restore calls it on a backup, and on its copy of the backup before that copy
// replaces the database. DuckDB refuses to open a file that is shorter than its
// header says, so a backup cut short by a crash during its copy fails here, not
// after the swap.
//
// The file is attached to an in-memory database by a SQL string, not opened by
// a DSN: duckdb-go parses a DSN as a URL, so a "#" in a folder name moved the
// read-only option into the URL fragment (a read-write open), and a "%" failed
// the parse.
func ValidateFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &FileNotFoundError{Path: path}
		}
		return &DatabaseError{Op: "validate", Err: err}
	}
	conn, err := openPoolAt(":memory:")
	if err != nil {
		return &DatabaseError{Op: "validate", Err: err}
	}
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(1) // USE below applies to one connection
	quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	if _, err := conn.Exec(`ATTACH ` + quoted + ` AS checked (READ_ONLY)`); err != nil {
		return &CorruptedFileError{Path: path, Err: err}
	}
	if _, err := conn.Exec(`USE checked`); err != nil {
		return &DatabaseError{Op: "validate", Err: err}
	}

	file := &DB{conn: conn, path: path}
	if err := file.validateTMoneyFile(); err != nil {
		return err
	}
	version, err := file.SchemaVersion()
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return &VersionMismatchError{FileVersion: version, AppVersion: CurrentSchemaVersion}
	}
	return nil
}
