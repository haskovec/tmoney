package db

import "errors"

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
func (db *DB) WithTx(fn func(tx Queryer) error) error {
	db.txMu.Lock()
	defer db.txMu.Unlock()

	tx, err := db.conn.Begin() // reads the live conn — safe across reconnect()
	if err != nil {
		return &DatabaseError{Op: "begin transaction", Err: err}
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
			return &DatabaseError{Op: "rollback", Err: errors.Join(err, rbErr)}
		}
		return err
	}

	done = true
	if err := tx.Commit(); err != nil {
		return &DatabaseError{Op: "commit transaction", Err: err}
	}
	return nil
}
