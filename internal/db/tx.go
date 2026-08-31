package db

import (
	"errors"
	"time"
)

// WithTx runs fn inside a single SQL transaction. If fn returns an error
// or panics, the transaction is rolled back; otherwise it is committed.
// The transaction is passed to fn as a Queryer — hand it to repositories
// and services via their WithTx/InTx derivations.
//
// WithTx serializes: a mutex enforces single-writer discipline, so a
// concurrent call (e.g. from a TUI tea.Cmd goroutine) queues briefly
// instead of racing on a second pooled connection. WithTx must not be
// nested — DuckDB has no savepoints. A bug that nests a WithTx call
// deadlocks on the mutex, which any test exercising the path catches
// immediately; this is intentional.
//
// Wherever the DATABASE reports the failure — begin, rollback, commit — the
// error passes through healAfterFatal first, because a DuckDB fatal error
// invalidates the whole instance rather than just the statement that tripped it.
// The plain `return err` from fn needs no such handling: the rollback that
// precedes it succeeded, which is itself proof the handle still works.
func (db *DB) WithTx(fn func(tx Queryer) error) error {
	db.txMu.Lock()
	defer db.txMu.Unlock()

	tx, err := db.live().Begin() // fresh pool each call — safe across reconnect()
	if err != nil {
		return db.healAfterFatal(&DatabaseError{Op: "begin transaction", Err: err})
	}

	done := false
	defer func() {
		if !done {
			_ = tx.Rollback() // panic path; re-panic proceeds after rollback
		}
	}()

	if err := fn(tx); err != nil {
		done = true
		if rbErr := tx.Rollback(); rbErr != nil {
			return db.healAfterFatal(&DatabaseError{Op: "rollback", Err: errors.Join(err, rbErr)})
		}
		return err
	}

	done = true
	if err := tx.Commit(); err != nil {
		return db.healAfterFatal(&DatabaseError{Op: "commit transaction", Err: err})
	}
	return nil
}

// healAfterFatal returns cause, first restoring the connection if cause left the
// DuckDB instance invalidated.
//
// DuckDB raises a fatal error for a class of storage faults — a desynced ART
// index is the one that reached a user, see reindex.go — and then marks the
// ENTIRE database instance invalid, not just the failed statement. Every later
// query on every connection to that path fails with "database has been
// invalidated ... must be restarted". A TUI session therefore kept running with
// nothing able to read or write, and each follow-on action produced its own
// confusing error, until the user restarted the app.
//
// The repair is the close-then-open that reconnect performs. Order matters:
// DuckDB caches instances by path, so opening a second handle while the
// poisoned one is still open hands back the same invalid instance. Repositories
// reach the pool through db.Conn() on each call, so they pick up the replacement
// with no rebinding.
//
// The swap is why db.conn is now guarded by connMu (connection.go): before this,
// reconnect only ran at open time, when nothing else could touch the field.
// A read goroutine that is mid-statement on the OLD pool when the swap lands
// still sees that statement fail ("sql: database is closed"); it succeeds on
// retry, which is a far better failure than a session that can never write again.
//
// The probe has to be a real query. sql.DB.Ping does NOT report this
// invalidation — it returns nil against a poisoned handle — so a healthy Ping is
// no evidence the connection works.
//
// It deliberately does not retry cause's work. The caller still gets the
// original error and decides what to do; what this buys is that the NEXT
// attempt can succeed instead of inheriting a dead session.
//
// The reconnect runs on its own goroutine behind a bounded wait, because it can
// block for an unbounded time: DuckDB will not release an invalidated instance
// until every connection to it is returned, so a reader holding an open
// *sql.Rows stalls the reopen for as long as it keeps iterating. Waiting inline
// froze the caller — a worse failure than the one being repaired, since the app
// hangs instead of reporting an error. On timeout the attempt is left running:
// it publishes the new pool when the reader finally lets go, so the session can
// still recover a moment later without another write having to fail first.
func (db *DB) healAfterFatal(cause error) error {
	var probe int
	if err := db.live().QueryRow(`SELECT 1`).Scan(&probe); err == nil {
		return cause // the handle still works; nothing to heal
	}
	if !db.healing.CompareAndSwap(false, true) {
		return cause // an attempt from an earlier failure is still in flight
	}

	done := make(chan error, 1)
	go func() {
		defer db.healing.Store(false)
		done <- db.reconnect()
	}()

	select {
	case err := <-done:
		if err != nil {
			return errors.Join(cause, err)
		}
		return cause
	case <-time.After(healReconnectTimeout):
		return errors.Join(cause, errHealBlocked)
	}
}

// healReconnectTimeout bounds the inline wait in healAfterFatal. It only has to
// cover a reopen that is not blocked behind a reader, which is near-instant; the
// point of the bound is to never hang the caller.
const healReconnectTimeout = 5 * time.Second

// errHealBlocked reports that the connection could not be restored in time
// because something else still holds the old one open.
var errHealBlocked = errors.New(
	"could not restore the database connection: a read is still holding it open. " +
		"The reconnect continues in the background; if the next action still fails, restart tmoney")
