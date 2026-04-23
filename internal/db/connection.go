package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/duckdb/duckdb-go/v2"
)

// FileExtension is the standard extension for TMoney database files.
const FileExtension = ".tdb"

// AppIdentifier is used to verify that a database is a TMoney file.
const AppIdentifier = "tmoney"

// DB wraps a DuckDB connection with TMoney-specific functionality.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens an existing TMoney database file.
// Returns an error if the file doesn't exist, isn't a valid DuckDB file,
// or isn't a TMoney database.
func Open(path string) (*DB, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, &FileNotFoundError{Path: path}
	}

	// Open DuckDB connection
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, &DatabaseError{Op: "open", Err: err}
	}

	// Verify connection works
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, &CorruptedFileError{Path: path, Err: err}
	}

	db := &DB{
		conn: conn,
		path: path,
	}

	// Validate this is a TMoney file
	if err := db.validateTMoneyFile(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Run any pending migrations
	if err := db.Migrate(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Reconnect after migrations to work around DuckDB index issues
	if err := db.reconnect(); err != nil {
		return nil, err
	}

	return db, nil
}

// Create creates a new TMoney database file at the specified path.
// Returns an error if the file already exists.
func Create(path string) (*DB, error) {
	// Ensure path has correct extension
	if !strings.HasSuffix(path, FileExtension) {
		path = path + FileExtension
	}

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		return nil, &FileExistsError{Path: path}
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, &DatabaseError{Op: "create directory", Err: err}
	}

	// Create DuckDB connection (creates file)
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, &DatabaseError{Op: "create", Err: err}
	}

	// Verify connection works
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		_ = os.Remove(path)
		return nil, &DatabaseError{Op: "initialize", Err: err}
	}

	db := &DB{
		conn: conn,
		path: path,
	}

	// Initialize metadata table
	if err := db.initializeMetadata(); err != nil {
		_ = conn.Close()
		_ = os.Remove(path)
		return nil, err
	}

	// Run migrations to create schema
	if err := db.Migrate(); err != nil {
		_ = conn.Close()
		_ = os.Remove(path)
		return nil, err
	}

	// Reconnect after migrations to work around DuckDB index issues
	// DuckDB has issues with UPDATE operations on tables with indexes
	// when using the same connection that created them
	if err := db.reconnect(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	if db.conn == nil {
		return nil
	}
	err := db.conn.Close()
	db.conn = nil
	return err
}

// reconnect closes and reopens the database connection.
// This is used as a workaround for DuckDB issues with UPDATE operations
// on tables that have indexes created in the same connection session.
func (db *DB) reconnect() error {
	if err := db.conn.Close(); err != nil {
		return &DatabaseError{Op: "close for reconnect", Err: err}
	}

	conn, err := sql.Open("duckdb", db.path)
	if err != nil {
		return &DatabaseError{Op: "reconnect", Err: err}
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return &DatabaseError{Op: "reconnect ping", Err: err}
	}

	db.conn = conn
	return nil
}

// Conn returns the underlying sql.DB connection for direct queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Path returns the file path of the database.
func (db *DB) Path() string {
	return db.path
}

// DefaultDirectory returns the default TMoney directory for the current OS.
func DefaultDirectory() string {
	var home string
	if runtime.GOOS == "windows" {
		home = os.Getenv("USERPROFILE")
	} else {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, "Documents", "TMoney")
}

// validateTMoneyFile checks that the database is a valid TMoney file.
func (db *DB) validateTMoneyFile() error {
	// Check if _metadata table exists
	var tableName string
	err := db.conn.QueryRow(`
		SELECT table_name FROM information_schema.tables
		WHERE table_name = '_metadata'
	`).Scan(&tableName)
	if err == sql.ErrNoRows {
		return &InvalidFileError{Path: db.path, Reason: "missing _metadata table"}
	}
	if err != nil {
		return &CorruptedFileError{Path: db.path, Err: err}
	}

	// Check app_identifier
	var identifier string
	err = db.conn.QueryRow(`
		SELECT value FROM _metadata WHERE key = 'app_identifier'
	`).Scan(&identifier)
	if err == sql.ErrNoRows {
		return &InvalidFileError{Path: db.path, Reason: "missing app_identifier"}
	}
	if err != nil {
		return &CorruptedFileError{Path: db.path, Err: err}
	}
	if identifier != AppIdentifier {
		return &InvalidFileError{Path: db.path, Reason: fmt.Sprintf("wrong app_identifier: %s", identifier)}
	}

	return nil
}

// initializeMetadata creates the _metadata table and populates it with required entries.
func (db *DB) initializeMetadata() error {
	tx, err := db.conn.Begin()
	if err != nil {
		return &DatabaseError{Op: "begin transaction", Err: err}
	}
	defer tx.Rollback()

	// Create _metadata table
	_, err = tx.Exec(`
		CREATE TABLE _metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		return &DatabaseError{Op: "create _metadata table", Err: err}
	}

	// Insert required metadata entries
	// Note: schema_version starts at 0, migrations will update it
	_, err = tx.Exec(`
		INSERT INTO _metadata (key, value) VALUES
			('app_identifier', ?),
			('schema_version', '0'),
			('created_at', CURRENT_TIMESTAMP),
			('default_currency', 'USD')
	`, AppIdentifier)
	if err != nil {
		return &DatabaseError{Op: "insert metadata", Err: err}
	}

	if err := tx.Commit(); err != nil {
		return &DatabaseError{Op: "commit transaction", Err: err}
	}

	return nil
}

// GetMetadata retrieves a metadata value by key.
func (db *DB) GetMetadata(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(`SELECT value FROM _metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", &MetadataNotFoundError{Key: key}
	}
	if err != nil {
		return "", &DatabaseError{Op: "get metadata", Err: err}
	}
	return value, nil
}

// SetMetadata sets a metadata value.
func (db *DB) SetMetadata(key, value string) error {
	result, err := db.conn.Exec(`
		UPDATE _metadata SET value = ? WHERE key = ?
	`, value, key)
	if err != nil {
		return &DatabaseError{Op: "set metadata", Err: err}
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return &DatabaseError{Op: "check rows affected", Err: err}
	}

	if rowsAffected == 0 {
		// Key doesn't exist, insert it
		_, err = db.conn.Exec(`INSERT INTO _metadata (key, value) VALUES (?, ?)`, key, value)
		if err != nil {
			return &DatabaseError{Op: "insert metadata", Err: err}
		}
	}

	return nil
}

// SchemaVersion returns the current schema version of the database.
func (db *DB) SchemaVersion() (int, error) {
	versionStr, err := db.GetMetadata("schema_version")
	if err != nil {
		return 0, err
	}

	var version int
	_, err = fmt.Sscanf(versionStr, "%d", &version)
	if err != nil {
		return 0, &CorruptedFileError{Path: db.path, Err: fmt.Errorf("invalid schema_version: %s", versionStr)}
	}

	return version, nil
}
