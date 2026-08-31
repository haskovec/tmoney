package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

// errSentinelOrdinary stands for any failure that does not invalidate DuckDB.
var errSentinelOrdinary = errors.New("ordinary failure")

// The storage fault behind `tmoney db reindex`, pinned down.
//
// DuckDB can leave a secondary ART index out of sync with its table on disk: the
// row is in the table but its key is missing from the index. Because DuckDB
// rewrites an UPDATE that touches an indexed column as an internal
// DELETE+INSERT, the next such UPDATE cannot find the old key to erase and
// aborts with "Failed to delete all rows from index. Only deleted 0 out of 1
// rows". That is what blocked a scheduled-transaction post, and before it a
// reconcile-finish (see migration 030).
//
// The state IS reproducible, which reindex_test.go long assumed it was not:
// write an index-backed row, then shut down without the checkpoint that folds
// the WAL into the file. The next open replays the WAL, restoring the row to the
// table but not its key to the index. A tmoney process that dies without a clean
// Close — a crash, an OOM kill, a closed terminal — leaves exactly that behind.

const (
	desyncTable = "desync_probe" // carries the secondary index that goes stale
	healTable   = "heal_probe"   // no index; used to prove the session recovered
	desyncID    = "019ddc35-aba0-7c7b-a889-1c625f77d891"
	desyncOldND = "2026-09-07" // the key that goes missing from the index
	desyncNewND = "2026-10-07"
)

// desyncedIndexFixture returns a database whose index on desyncTable(nd) is
// missing the key for the single row in that table.
//
// It skips rather than fails when the recipe stops working. The setup leans on
// PRAGMA disable_checkpoint_on_shutdown and on WAL-replay behaviour, neither of
// which DuckDB promises to keep; a version bump that changes either would
// otherwise break the build over something these tests do not own. A skip here
// means the repair below is no longer covered — treat it as work to redo, not as
// a pass.
func desyncedIndexFixture(t *testing.T) *DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "desync.tdb")
	database, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	setup := []string{
		`CREATE TABLE ` + desyncTable + ` (id UUID PRIMARY KEY, nd DATE NOT NULL, memo TEXT)`,
		`CREATE INDEX idx_` + desyncTable + `_nd ON ` + desyncTable + `(nd)`,
		`CREATE TABLE ` + healTable + ` (id INTEGER, v TEXT)`,
	}
	for _, stmt := range setup {
		if _, err := database.Conn().Exec(stmt); err != nil {
			t.Fatalf("fixture setup %q: %v", stmt, err)
		}
	}

	// Leave the row in the WAL rather than in the file.
	if _, err := database.Conn().Exec(`PRAGMA disable_checkpoint_on_shutdown`); err != nil {
		_ = database.Close()
		t.Skipf("DuckDB no longer supports PRAGMA disable_checkpoint_on_shutdown (%v); "+
			"the desynced-index repair is NOT covered until this fixture is rebuilt", err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO `+desyncTable+` VALUES (CAST(? AS UUID), DATE '`+desyncOldND+`', 'x')`, desyncID,
	); err != nil {
		t.Fatalf("fixture insert: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("fixture close: %v", err)
	}

	// Reopening replays the WAL, which is what desyncs the index.
	database, err = Open(dbPath)
	if err != nil {
		t.Fatalf("fixture reopen: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// advanceND runs the shape of UPDATE that scheduled.Repository.Update issues:
// it writes the indexed column, so DuckDB rewrites the row.
func advanceND(db *DB) error {
	return db.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(
			`UPDATE `+desyncTable+` SET nd = DATE '`+desyncNewND+`' WHERE CAST(id AS VARCHAR) = ?`,
			desyncID,
		)
		return err
	})
}

// requireDesync skips when the fixture produced a healthy index, so the tests
// below never pass by accident on a DuckDB that stopped reproducing the fault.
func requireDesync(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Skip("this DuckDB build no longer desyncs the index on WAL replay; " +
			"the desynced-index repair is NOT covered until this fixture is rebuilt")
	}
	if !strings.Contains(err.Error(), "Failed to delete all rows from index") {
		t.Fatalf("fixture produced the wrong failure: %v", err)
	}
}

// TestDesyncedIndexAbortsAtCommitNotExec pins where the failure surfaces. The
// UPDATE itself reports success and even a correct RowsAffected; only the commit
// fails, because DuckDB defers the index erase of an already-stored row to commit
// time. That is why the user-visible text comes from WithTx's commit branch and
// reads "database error during commit transaction".
func TestDesyncedIndexAbortsAtCommitNotExec(t *testing.T) {
	database := desyncedIndexFixture(t)

	tx, err := database.Conn().Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	res, execErr := tx.Exec(
		`UPDATE `+desyncTable+` SET nd = DATE '`+desyncNewND+`' WHERE CAST(id AS VARCHAR) = ?`,
		desyncID,
	)
	if execErr != nil {
		_ = tx.Rollback()
		t.Skipf("this DuckDB build rejects the UPDATE at Exec (%v); the commit-time "+
			"characterisation no longer holds and needs revisiting", execErr)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Errorf("RowsAffected() = %d, want 1 — the statement itself should look fine", n)
	}

	commitErr := tx.Commit()
	requireDesync(t, commitErr)
}

// TestReindexRepairsDesyncedIndex is the regression test for `tmoney db reindex`:
// the advance UPDATE fails, Reindex runs, and the identical UPDATE then commits.
// Reindex is the only repair — the bad index is written into the file and
// survives a clean close and an explicit CHECKPOINT.
func TestReindexRepairsDesyncedIndex(t *testing.T) {
	database := desyncedIndexFixture(t)

	requireDesync(t, advanceND(database))

	// Reindex needs a live handle, and the fatal above invalidated the instance.
	// WithTx's healAfterFatal already restored it — that is what makes this call
	// possible at all, and TestWithTxHealsConnectionAfterFatal covers it directly.
	n, err := database.Reindex()
	if err != nil {
		t.Fatalf("Reindex() error = %v", err)
	}
	if n == 0 {
		t.Fatal("Reindex() rebuilt 0 indexes, want at least the fixture's")
	}

	if err := advanceND(database); err != nil {
		t.Fatalf("advance UPDATE still fails after Reindex(): %v", err)
	}

	var got string
	if err := database.Conn().QueryRow(
		`SELECT CAST(nd AS VARCHAR) FROM `+desyncTable+` WHERE CAST(id AS VARCHAR) = ?`, desyncID,
	).Scan(&got); err != nil {
		t.Fatalf("reading the repaired row: %v", err)
	}
	if got != desyncNewND {
		t.Errorf("nd = %s after repair, want %s", got, desyncNewND)
	}
}

// TestWithTxHealsConnectionAfterFatal covers healAfterFatal. A DuckDB fatal
// error invalidates the whole instance, so before the heal every later statement
// in the process failed with "database has been invalidated" and the app had to
// be restarted. The next write must now succeed. It targets healTable, which
// carries no index: the desynced index is still in the file, so a write to
// desyncTable would legitimately keep failing.
func TestWithTxHealsConnectionAfterFatal(t *testing.T) {
	database := desyncedIndexFixture(t)

	requireDesync(t, advanceND(database))

	if err := database.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(`INSERT INTO `+healTable+` (id, v) VALUES (?, ?)`, 1, "after-fatal")
		return err
	}); err != nil {
		t.Fatalf("WithTx() after a fatal commit error = %v, want the session to have recovered", err)
	}

	var n int
	if err := database.Conn().QueryRow(
		`SELECT COUNT(*) FROM ` + healTable + ` WHERE id = 1`,
	).Scan(&n); err != nil {
		t.Fatalf("reading after recovery: %v", err)
	}
	if n != 1 {
		t.Errorf("rows after recovery = %d, want 1", n)
	}
}

// TestHealAfterFatalIsRaceFreeWithConcurrentReaders covers the hazard the heal
// introduced: reconnect swaps db.conn, and it now does so mid-session rather than
// only at open time. Readers reach the pool through Conn() without holding txMu,
// so the field needs connMu. Run under -race, this fails if that lock is removed.
//
// A reader may legitimately see one "database is closed" or "invalidated" error
// as the swap lands, so the assertion is on the absence of a race and on reads
// working again once the heal is done — not on every read succeeding.
func TestHealAfterFatalIsRaceFreeWithConcurrentReaders(t *testing.T) {
	database := desyncedIndexFixture(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	// Deferred, not inline after the assertion: requireDesync can Skip or Fatal,
	// and an abandoned reader goroutine would keep hammering a closed pool into
	// the next test — where the race detector would blame the wrong code.
	var stopOnce sync.Once
	shutdown := func() {
		stopOnce.Do(func() { close(stop) })
		wg.Wait()
	}
	defer shutdown()

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				var n int
				// Errors are expected around the swap; the point is the race
				// detector, so the result is deliberately discarded.
				_ = database.Conn().QueryRow(`SELECT COUNT(*) FROM ` + healTable).Scan(&n)
			}
		}()
	}

	requireDesync(t, advanceND(database)) // trips the fatal, which heals

	shutdown()

	var n int
	if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM ` + healTable).Scan(&n); err != nil {
		t.Fatalf("reads still broken after the heal settled: %v", err)
	}
}

// TestHealAfterFatalDoesNotBlockOnOpenRows is the regression test for a hang the
// heal originally caused. DuckDB will not release an invalidated instance until
// every connection to it is returned, so the reopen blocks for as long as a
// reader holds an open *sql.Rows — and the first version waited for it inline
// while holding connMu, freezing the writer AND every reader. An app that hangs
// is a worse failure than the one being repaired, so the wait is now bounded.
//
// It must return promptly with both the original cause and errHealBlocked.
func TestHealAfterFatalDoesNotBlockOnOpenRows(t *testing.T) {
	database := desyncedIndexFixture(t)

	for i := range 200 {
		if _, err := database.Conn().Exec(
			`INSERT INTO `+healTable+` (id, v) VALUES (?, ?)`, i, "row"); err != nil {
			t.Fatalf("seeding %s: %v", healTable, err)
		}
	}

	// One row read, so the connection stays checked out behind this cursor.
	rows, err := database.Conn().Query(`SELECT id, v FROM ` + healTable)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected at least one row to hold the cursor open")
	}

	result := make(chan error, 1)
	go func() { result <- advanceND(database) }()

	select {
	case err := <-result:
		requireDesync(t, err) // the original cause must survive the join
		if !errors.Is(err, errHealBlocked) {
			t.Errorf("error = %v, want it to also report errHealBlocked", err)
		}
	case <-time.After(healReconnectTimeout + 15*time.Second):
		t.Fatal("healAfterFatal blocked on an open cursor instead of giving up")
	}
}

// TestReconnectKeepsPoolPublishedWhenReopenFails covers the other way the heal
// could be worse than the disease. reconnect closes the old pool before
// reopening, so a failed reopen must not publish a nil *sql.DB: every call site
// dereferences Conn() immediately, so nil turns an error into a panic — and
// healAfterFatal's own probe would panic too, disabling the heal for good.
// Leaving the closed pool published yields "sql: database is closed" instead,
// and lets the next attempt retry.
func TestReconnectKeepsPoolPublishedWhenReopenFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reopen.tdb")
	database, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(dbPath, 0o644)
		_ = database.Close()
	})

	// Make the reopen fail without corrupting anything.
	if err := os.Chmod(dbPath, 0o000); err != nil {
		t.Skipf("cannot revoke read permission on this filesystem: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("cannot revoke directory permission on this filesystem: %v", err)
	}

	if err := database.reconnect(); err == nil {
		t.Skip("the reopen succeeded despite revoked permissions; running as root?")
	}

	if got := database.Conn(); got == nil {
		t.Fatal("Conn() returned nil after a failed reopen; every call site would panic")
	}

	// The published pool is closed, so calls must ERROR rather than panic.
	var n int
	if err := database.Conn().QueryRow(`SELECT 1`).Scan(&n); err == nil {
		t.Error("expected an error from the closed pool, got nil")
	}
	if err := database.WithTx(func(tx Queryer) error { return nil }); err == nil {
		t.Error("expected WithTx to error after a failed reopen, got nil")
	}
}

// TestHealAfterFatalKeepsHandleOnOrdinaryError guards the probe from
// over-reacting. An ordinary failure leaves the instance perfectly usable, and
// swapping the connection then would throw away a working pool for nothing.
func TestHealAfterFatalKeepsHandleOnOrdinaryError(t *testing.T) {
	database := desyncedIndexFixture(t)

	before := database.Conn()
	if got := database.healAfterFatal(errSentinelOrdinary); got != errSentinelOrdinary {
		t.Errorf("healAfterFatal() returned %v, want the cause unchanged", got)
	}
	if database.Conn() != before {
		t.Error("healAfterFatal() reconnected on a healthy handle; it must not")
	}
}
