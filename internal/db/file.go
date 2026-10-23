package db

import "os"

// The functions in this file work on a database file that is not open in this
// process: the backup package's own copy of a database. DuckDB caches one
// instance per path, so path must not be open here.

// ValidateFile checks, without changing the file, that path is a complete
// TMoney database this app can open: DuckDB opens it read-only, the _metadata
// table names TMoney (validateTMoneyFile), and its schema version is not newer
// than this app. It runs no migrations and writes nothing.
//
// Restore calls it on its copy of a backup before that copy replaces the
// database. DuckDB refuses to open a file that is shorter than its header says,
// so a backup cut short by a crash during its copy fails here, not after the
// swap.
func ValidateFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &FileNotFoundError{Path: path}
		}
		return &DatabaseError{Op: "validate", Err: err}
	}
	conn, err := openPoolAt(path + "?access_mode=read_only")
	if err != nil {
		return &CorruptedFileError{Path: path, Err: err}
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Ping(); err != nil {
		return &CorruptedFileError{Path: path, Err: err}
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
