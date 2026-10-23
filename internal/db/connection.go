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
	// for a heal in flight. When WithFileClosed gives up on a held read, the
	// reopen it leaves in the background keeps it until that open ends.
	// DuckDB attaches a file only once, so two opens racing for it leave one
	// failed; and a heal that opens under fn would change the file while it
	// is copied. Take it after txMu, never before.
	reopenMu sync.Mutex
	// fileBusy is set while that background reopen waits for the read, so a
	// WithFileClosed that times out on reopenMu meanwhile gives the same "try
	// again" answer (errFileBusy), not the blocked heal's.
	fileBusy atomic.Bool
	// stale is set while the closed pool is still published after a failed
	// reopen (openPool, reconnect), although nothing closed the handle on
	// purpose. Each use of the handle then starts a reconnect (see healStale).
	stale atomic.Bool
	// lastHeal is the time of the last reconnect that healStale started, as
	// nanoseconds on the monotonic clock (see monoNow), so that a burst of
	// reads tries at most once a second.
	lastHeal atomic.Int64
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
	conn := db.conn
	db.connMu.RUnlock()
	if db.stale.Load() {
		db.healStale()
	}
	return conn
}

// healStale starts a reconnect in the background, at most once a second, while
// the handle is stale. The call that starts it still gets "sql: database is
// closed"; a later call gets the new pool. Before, only the heal of a failed
// write (healAfterFatal) tried the open again, so the reads of every view
// failed until the user wrote something, and nobody had seen the error of the
// failed reopen.
//
// The reconnect can wait for reopenMu behind a file operation (WithFileClosed),
// whose reopen clears stale. So it checks stale again once it holds reopenMu,
// or it would close the pool that the file operation just opened.
func (db *DB) healStale() {
	now, last := monoNow(), db.lastHeal.Load()
	if (last != 0 && now-last < int64(time.Second)) || !db.lastHeal.CompareAndSwap(last, now) {
		return
	}
	if !db.healing.CompareAndSwap(false, true) {
		return // a heal is in flight already
	}
	go func() {
		defer db.healing.Store(false)
		db.reopenMu.Lock()
		defer db.reopenMu.Unlock()
		if db.stale.Load() {
			_ = db.reconnectLocked()
		}
	}()
}

// monoStart anchors monoNow.
var monoStart = time.Now()

// monoNow returns nanoseconds since monoStart on the monotonic clock, which a
// change of the wall clock does not move. A wall-clock time would stop the
// heals for as long as the clock stepped back.
func monoNow() int64 {
	return int64(time.Since(monoStart))
}

// Open opens an existing TMoney database file.
// Returns an error if the file doesn't exist, isn't a valid DuckDB file,
// or isn't a TMoney database.
func Open(path string) (*DB, error) {
	path = ExpandHome(path)

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
	path = ExpandHome(path)

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

// ExpandHome replaces a leading "~" (alone, or before a path separator) with
// the home folder. DuckDB does that itself when it opens a path, but the os
// calls on the path do not: the checks of Open, Create and the reopen, and
// every backup. So the handle keeps the expanded path, and both see one file.
//
// DuckDB expands any leading "~", also in "~user/x", which the os reads as a
// folder "~user" in the working folder. Such a path gets "./" in front, so
// DuckDB opens that folder too.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return "." + string(filepath.Separator) + path
}

// Close closes the database connection.
//
// Close is the only thing that makes a file copy of this database valid: DuckDB
// keeps committed writes in <path>.wal until close or a size-triggered
// checkpoint, and on Windows its open handle makes os.Open on the file fail.
// Backups therefore close first (see WithFileClosed, and CloseAndSettle for a
// copy after a Close for good). Close alone is not enough: it closes only the
// idle connections, and the instance lives on until the last checked-out one
// is returned (see drained).
//
// The closed pool stays published so a late caller gets "sql: database is
// closed" rather than a nil-pointer panic. Close is idempotent.
func (db *DB) Close() error {
	_, _, err := db.close()
	return err
}

// close is Close. It also returns the pool it closed and closeGen after this
// close, read under the same lock, so a later Close always shows as a change
// of closeGen.
func (db *DB) close() (*sql.DB, uint64, error) {
	db.connMu.Lock()
	defer db.connMu.Unlock()

	db.closed = true
	db.closeGen++
	db.stale.Store(false) // closed on purpose: nothing to heal
	if db.conn == nil {
		return nil, db.closeGen, nil
	}
	return db.conn, db.closeGen, db.conn.Close()
}

// reopenWaitTimeout bounds WithFileClosed's waits: for a heal in flight, and
// then for the reads that still hold the closed pool. A var so tests can
// shorten it.
var reopenWaitTimeout = SettleTimeout

// errFileBusy reports that a read still held the closed pool, and with it the
// DuckDB instance, when WithFileClosed stopped waiting. fn did not run; the
// handle reopens by itself when the read lets go.
var errFileBusy = errors.New(
	"a read is still holding the database open, so the backup or restore did not run. Try again in a moment")

// SettleTimeout is how long CloseAndSettle waits before the last backup: on
// quit in the TUI, and after each CLI command that changes data.
const SettleTimeout = 5 * time.Second

// CloseAndSettle closes the database for good before a last copy of the file:
// the backup on quit, and the one after a CLI command. It first waits up to
// timeout for the work that can still touch the file: a write or a file
// operation in flight (WithFileClosed, which a backup or restore runs), and a
// reconnect that a heal left blocked in the background. Then it closes, and
// from then on WithFileClosed and reconnect refuse to reopen, so the copy sees
// a still file. Last, it waits for the reads that still hold the closed pool
// (see drained). It returns false when a wait ran out: the database is
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
	return settled && drained(db.live(), deadline), nil
}

// lockBy locks mu if it can before deadline, and reports whether it did.
func lockBy(mu *sync.Mutex, deadline time.Time) bool {
	return waitUntil(deadline, mu.TryLock)
}

// drained waits until pool, which is closed, has no open connection, or until
// deadline, and reports whether it got there.
//
// sql.DB.Close closes only the idle connections. A connection that a read
// holds (an open *sql.Rows, a sql.Conn, a Tx) stays open until it is returned,
// and keeps the DuckDB instance alive: the instance has not yet checkpointed
// <path>.wal into the file, it checkpoints into the file when the read lets go,
// and an open of the file blocks until then. database/sql counts a connection
// out only after the driver closed it, so at zero the instance is gone and the
// file is still.
//
// Two limits. The count is of this pool only: a second *DB on the same path
// shares the cached instance, which then lives on, so the app keeps one handle
// per file (see the TUI's submitOpenFile). And an instance that a fatal error
// invalidated cannot checkpoint, so its WAL stays beside the file.
func drained(pool *sql.DB, deadline time.Time) bool {
	return pool == nil || waitUntil(deadline, func() bool { return pool.Stats().OpenConnections == 0 })
}

// waitUntil polls ok until it reports true or deadline passes.
func waitUntil(deadline time.Time, ok func() bool) bool {
	for !ok() {
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
// After the Close it waits for the reads that still hold a connection (see
// drained), so fn sees a still file that holds every committed row. If a read
// holds on past the wait, fn does not run and the error wraps errFileBusy; the
// handle reopens in the background when the read lets go.
//
// fn's error is returned after the reopen. A failed reopen is joined onto it.
// The handle then stays on the closed pool, and its next use tries the open
// again (see healStale).
func (db *DB) WithFileClosed(fn func(path string) error) error {
	// The reopen of an earlier call still waits for a held read: say so at
	// once, rather than hold the writer lock for the whole wait below.
	if db.fileBusy.Load() {
		return &DatabaseError{Op: "close for file operation", Err: errFileBusy}
	}
	db.txMu.Lock()
	defer db.txMu.Unlock()
	// Wait for a heal in flight: it publishes a healthy instance, so the Close
	// below can checkpoint it, and nothing else opens the file until the
	// reopen below. The wait is bounded: a heal blocked behind a reader that
	// never lets go must not hang the backup, with every write queued behind
	// txMu. errHealBlocked tells the caller to retry; nothing has changed yet.
	deadline := time.Now().Add(reopenWaitTimeout)
	if !lockBy(&db.reopenMu, deadline) {
		if db.fileBusy.Load() {
			return &DatabaseError{Op: "close for file operation", Err: errFileBusy}
		}
		return &DatabaseError{Op: "close for file operation", Err: errHealBlocked}
	}
	unlock := true // false when a background reopen takes reopenMu over
	defer func() {
		if unlock {
			db.reopenMu.Unlock()
		}
	}()

	db.connMu.RLock()
	final := db.final
	db.connMu.RUnlock()
	if final {
		return ErrClosed // CloseAndSettle closed it for good; never reopen
	}

	pool, gen, err := db.close()
	if err != nil {
		return &DatabaseError{Op: "close for file operation", Err: err}
	}
	if !drained(pool, deadline) {
		// fn does not run. The reopen blocks until the read lets go, so it
		// runs in the background and keeps reopenMu, as a blocked heal does;
		// until it publishes, calls get "sql: database is closed". No one
		// waits for its error: a failure leaves the handle to the next heal
		// (see openPool).
		unlock = false
		db.fileBusy.Store(true)
		go func() {
			defer db.fileBusy.Store(false) // after the unlock: a call that timed out sees it
			defer db.reopenMu.Unlock()
			if db.awaitDrain(pool, gen) {
				_ = db.openPool(gen)
			}
		}()
		return &DatabaseError{Op: "close for file operation", Err: errFileBusy}
	}
	fnErr := fn(db.path)
	if err := db.openPool(gen); err != nil {
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
// It does NOT hold connMu across the reopen, and that is load-bearing. The
// open cannot start until every connection of the OLD instance has been
// released — which lasts as long as some reader holds an open *sql.Rows — so
// reconnect waits for that first (awaitDrain) and then opens. Holding connMu
// across that wait would park every reader on the same lock and freeze the
// whole app. database/sql's Close, by contrast, is prompt: it marks the pool
// closed and closes idle connections, leaving a checked-out one to close when
// its caller returns it.
//
// On a failed reopen the previous pool stays published, closed. Callers then get
// "sql: database is closed" — an error they can surface — instead of the nil
// *sql.DB that every call site would dereference into a panic. It also means the
// next attempt can retry the reopen.
//
// It refuses after an intentional Close: a backup or restore that closed the
// handle must not have it reopened underneath the file copy by a concurrent
// failure's heal. reconnect holds reopenMu throughout, so WithFileClosed waits
// for it rather than racing it. A plain Close can still land while reconnect
// waits; it then does not open at all, or drops the new pool rather than
// publish it, and CloseAndSettle waits for it to finish before a copy.
// WithFileClosed reopens through openPool directly.
func (db *DB) reconnect() error {
	db.reopenMu.Lock()
	defer db.reopenMu.Unlock()
	return db.reconnectLocked()
}

// reconnectLocked is reconnect for a caller that holds reopenMu.
func (db *DB) reconnectLocked() error {
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
	if !db.awaitDrain(old, gen) {
		return ErrClosed // a Close won the race; keep the handle closed
	}

	conn, err := db.openConn()
	if err != nil {
		db.stale.Store(true) // the closed pool stays published; see healStale
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
	db.stale.Store(false)
	return nil
}

// awaitDrain waits until pool, which is closed, has no open connection (see
// drained), with no time limit, and reports whether the handle is still to be
// reopened: false when a Close or CloseAndSettle came after the close that gen
// counts. An open of the file blocks until the old instance is gone, so the
// opens wait here, not inside DuckDB: an open that waited inside DuckDB would
// find the file as it was when the open began, and if the file was gone by
// then, DuckDB would create a new, empty database (see openConn).
func (db *DB) awaitDrain(pool *sql.DB, gen uint64) bool {
	for {
		db.connMu.RLock()
		current := !db.final && db.closeGen == gen
		db.connMu.RUnlock()
		if !current {
			return false
		}
		if pool == nil || pool.Stats().OpenConnections == 0 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// openPool opens a fresh pool on db.path and publishes it, clearing closed.
// The previous pool must already be closed. See reconnect for why connMu is not
// held across the open. gen is closeGen after the caller's own Close. On a
// final database (CloseAndSettle gave up waiting while a WithFileClosed ran its
// fn), or after a later Close, it drops the new pool instead: that close wins.
//
// If the open fails (the file is gone, or a network share dropped), the closed
// pool stays published, as after a failed reconnect, but the handle no longer
// counts as closed on purpose: the close was only for the file operation. The
// handle is stale, and its next use tries the open again (see healStale),
// instead of a session that stays closed for good.
func (db *DB) openPool(gen uint64) error {
	conn, err := db.openConn()
	db.connMu.Lock()
	defer db.connMu.Unlock()
	if db.final || db.closeGen != gen {
		if conn != nil {
			_ = conn.Close()
		}
		return ErrClosed
	}
	db.closed = false
	if err != nil {
		db.stale.Store(true)
		return err
	}
	db.conn = conn
	db.stale.Store(false)
	return nil
}

// openConn opens and pings a fresh pool on db.path, without publishing it.
//
// It fails when the file is gone. DuckDB creates a new, empty database at a
// path with no file, and a reopen must never do that: the file was moved or
// deleted, or a share dropped, and an empty database in its place would show
// no data, and the quit backup would copy it.
func (db *DB) openConn() (*sql.DB, error) {
	if _, err := os.Stat(db.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &FileNotFoundError{Path: db.path}
		}
		return nil, &DatabaseError{Op: "reconnect", Err: err}
	}
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
