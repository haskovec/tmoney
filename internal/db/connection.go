package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FileExtension is the standard extension for TMoney database files.
const FileExtension = ".tdb"

// AppIdentifier is used to verify that a database is a TMoney file.
const AppIdentifier = "tmoney"

// DB wraps a DuckDB connection with TMoney-specific functionality.
type DB struct {
	conn *sql.DB
	// connMu guards conn, which reconnect() swaps for a new pool. The swap used
	// to happen only at open time, before anything else could touch the field;
	// healAfterFatal (tx.go) now does it mid-session, concurrently with TUI read
	// goroutines, so the field needs a lock rather than a comment.
	connMu sync.RWMutex
	// healing single-flights the reconnect that healAfterFatal starts, so a
	// second failure cannot open a second pool while the first attempt is still
	// blocked on DuckDB's instance lock.
	healing atomic.Bool
	// closed records an intentional Close, guarded by connMu. reconnect refuses
	// to reopen while it is set, so healAfterFatal cannot resurrect a handle
	// that a backup or restore closed on purpose. WithFileClosed clears it.
	closed bool
	// closeGen counts intentional Closes, guarded by connMu. reconnect's open
	// can block for as long as a reader holds a cursor on the old instance, so
	// reconnect publishes its new pool only if no Close landed in the meantime.
	closeGen uint64
	// final records CloseAndSettle, guarded by connMu: the database is closed
	// for good, so WithFileClosed refuses to run and reopen.
	final bool
	// reopenMu serializes the opens of the file. reconnect holds it from the
	// close of the old pool to the publish of the new one, and WithFileClosed
	// holds it across its Close, fn and reopen, so a backup or restore waits
	// for a heal in flight. DuckDB attaches a file only once, so two opens
	// racing for it leave one failed; and a heal that opens under fn would
	// change the file while it is copied. Take it after txMu, never before.
	reopenMu sync.Mutex
	path     string
	txMu     sync.Mutex // serializes WithTx: single-writer discipline
}

// ErrClosed is returned by operations on a DB after Close.
var ErrClosed = errors.New("database is closed")

// live returns the current pool. Every read of db.conn goes through it.
//
// Take the pool fresh on each use and do not hold it across a statement that can
// fail: a reconnect closes the previous pool, so a retained handle can report
// "sql: database is closed" for one call afterwards. That is why Conn() is
// documented as per-call.
func (db *DB) live() *sql.DB {
	db.connMu.RLock()
	defer db.connMu.RUnlock()
	return db.conn
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
	conn, err := openPoolAt(path)
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
	conn, err := openPoolAt(path)
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
//
// Close is the only thing that makes a file copy of this database valid: DuckDB
// keeps committed writes in <path>.wal until close or a size-triggered
// checkpoint, and on Windows its open handle makes os.Open on the file fail.
// Backups therefore close first (see WithFileClosed, and CloseAndSettle for a
// copy after a Close for good).
//
// The closed pool stays published so a late caller gets "sql: database is
// closed" rather than a nil-pointer panic. Close is idempotent.
func (db *DB) Close() error {
	db.connMu.Lock()
	defer db.connMu.Unlock()

	db.closed = true
	db.closeGen++
	if db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

// reopenWaitTimeout bounds WithFileClosed's wait for a heal in flight. A var so
// tests can shorten it.
var reopenWaitTimeout = SettleTimeout

// SettleTimeout is how long CloseAndSettle waits before the last backup: on
// quit in the TUI, and after each CLI command that changes data.
const SettleTimeout = 5 * time.Second

// CloseAndSettle closes the database for good before a last copy of the file:
// the backup on quit, and the one after a CLI command. It first waits up to
// timeout for the work that can still touch the file: a write or a file
// operation in flight (WithFileClosed, which a backup or restore runs), and a
// reconnect that a heal left blocked in the background. Then it closes, and
// from then on WithFileClosed and reconnect refuse to reopen, so the copy sees
// a still file. It returns false when the wait ran out: the database is
// closed, but something may still touch the file, so do not copy it then.
func (db *DB) CloseAndSettle(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	// The same order as WithFileClosed: txMu, then reopenMu, which reconnect
	// holds for as long as a heal is in flight.
	settled := lockBy(&db.txMu, deadline)
	if settled {
		defer db.txMu.Unlock()
		settled = lockBy(&db.reopenMu, deadline)
		if settled {
			defer db.reopenMu.Unlock()
		}
	}

	db.connMu.Lock()
	db.final = true
	db.connMu.Unlock()
	if err := db.Close(); err != nil {
		return false, err
	}
	return settled, nil
}

// lockBy locks mu if it can before deadline, and reports whether it did.
func lockBy(mu *sync.Mutex, deadline time.Time) bool {
	for !mu.TryLock() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}

// isClosed reports whether Close was called and no reopen has followed.
func (db *DB) isClosed() bool {
	db.connMu.RLock()
	defer db.connMu.RUnlock()
	return db.closed
}

// WithFileClosed closes the database, runs fn against the file path, and
// reopens the same handle. It is the way to copy or replace the database file
// while the app keeps running: services, undo commands and views keep their
// *DB, because repositories take the pool fresh from Conn() on every call.
//
// It holds the writer mutex for the whole span, so no WithTx can run — and so
// trigger a reconnect through healAfterFatal — while the file is closed. A read
// that races the window gets "sql: database is closed" and can be retried.
//
// fn's error is returned after the reopen. A failed reopen is joined onto it;
// the handle then stays closed and callers should tell the user to restart.
func (db *DB) WithFileClosed(fn func(path string) error) error {
	db.txMu.Lock()
	defer db.txMu.Unlock()
	// Wait for a heal in flight: it publishes a healthy instance, so the Close
	// below can checkpoint it, and nothing else opens the file until the
	// reopen below. The wait is bounded: a heal blocked behind a reader that
	// never lets go must not hang the backup, with every write queued behind
	// txMu. errHealBlocked tells the caller to retry; nothing has changed yet.
	if !lockBy(&db.reopenMu, time.Now().Add(reopenWaitTimeout)) {
		return &DatabaseError{Op: "close for file operation", Err: errHealBlocked}
	}
	defer db.reopenMu.Unlock()

	db.connMu.RLock()
	final := db.final
	db.connMu.RUnlock()
	if final {
		return ErrClosed // CloseAndSettle closed it for good; never reopen
	}

	if err := db.Close(); err != nil {
		return &DatabaseError{Op: "close for file operation", Err: err}
	}
	fnErr := fn(db.path)
	if err := db.openPool(); err != nil {
		return errors.Join(fnErr, err)
	}
	return fnErr
}

// reconnect closes and reopens the database connection.
//
// Two callers need it. Reindex uses it because DuckDB mishandles an UPDATE on a
// table whose indexes were created in the same connection session. healAfterFatal
// (tx.go) uses it because a DuckDB fatal error invalidates the whole instance,
// and DuckDB caches instances by path — so the poisoned one has to be CLOSED
// before a reopen can hand back a fresh instance. Hence close-then-open, in that
// order.
//
// It does NOT hold connMu across the reopen, and that is load-bearing.
// duckdb.NewConnector (see openPoolAt) really opens the file, and DuckDB
// blocks there on its instance lock until every connection of the OLD instance
// has been released — which lasts as long as some reader holds an open
// *sql.Rows. Holding connMu across that blocked call would park every reader on
// the same lock and freeze the whole app. database/sql's Close, by contrast, is
// prompt: it marks the pool closed and closes idle connections, leaving a
// checked-out one to close when its caller returns it.
//
// On a failed reopen the previous pool stays published, closed. Callers then get
// "sql: database is closed" — an error they can surface — instead of the nil
// *sql.DB that every call site would dereference into a panic. It also means the
// next attempt can retry the reopen.
//
// It refuses after an intentional Close: a backup or restore that closed the
// handle must not have it reopened underneath the file copy by a concurrent
// failure's heal. reconnect holds reopenMu throughout, so WithFileClosed waits
// for it rather than racing it. A plain Close can still land while the open is
// blocked; the new pool is then dropped, not published, and CloseAndSettle
// waits for that open to finish before a copy. WithFileClosed reopens through
// openPool directly.
func (db *DB) reconnect() error {
	db.reopenMu.Lock()
	defer db.reopenMu.Unlock()

	db.connMu.Lock()
	old, closed, gen := db.conn, db.closed, db.closeGen
	db.connMu.Unlock()

	if closed {
		return ErrClosed
	}
	if old != nil {
		if err := old.Close(); err != nil {
			return &DatabaseError{Op: "close for reconnect", Err: err}
		}
	}

	conn, err := db.openConn()
	if err != nil {
		return err
	}
	db.connMu.Lock()
	defer db.connMu.Unlock()
	if db.closeGen != gen {
		_ = conn.Close() // a Close won the race; keep the handle closed
		return ErrClosed
	}
	db.conn = conn
	db.closed = false
	return nil
}

// openPool opens a fresh pool on db.path and publishes it, clearing closed.
// The previous pool must already be closed. See reconnect for why connMu is not
// held across the open. On a final database (CloseAndSettle gave up waiting
// while a WithFileClosed ran its fn) it drops the new pool instead: the close
// for good wins.
func (db *DB) openPool() error {
	conn, err := db.openConn()
	if err != nil {
		return err
	}
	db.connMu.Lock()
	defer db.connMu.Unlock()
	if db.final {
		_ = conn.Close()
		return ErrClosed
	}
	db.conn = conn
	db.closed = false
	return nil
}

// openConn opens and pings a fresh pool on db.path, without publishing it.
func (db *DB) openConn() (*sql.DB, error) {
	conn, err := openPoolAt(db.path)
	if err != nil {
		return nil, &DatabaseError{Op: "reconnect", Err: err}
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, &DatabaseError{Op: "reconnect ping", Err: err}
	}
	return conn, nil
}

// Conn returns the underlying sql.DB connection for direct queries.
//
// Call it per statement rather than caching the result: a reconnect (see
// reconnect) replaces the pool, and a cached handle from before the swap points
// at a closed one. Repositories already follow this — their q() calls Conn()
// on every statement.
func (db *DB) Conn() *sql.DB {
	return db.live()
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
	err := db.live().QueryRow(`
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
	err = db.live().QueryRow(`
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
	tx, err := db.live().Begin()
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
	err := db.live().QueryRow(`SELECT value FROM _metadata WHERE key = ?`, key).Scan(&value)
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
	result, err := db.live().Exec(`
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
		_, err = db.live().Exec(`INSERT INTO _metadata (key, value) VALUES (?, ?)`, key, value)
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
