package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests cover a read that holds a connection (an open *sql.Rows) while a
// backup, a restore or the quit backup closes the database. sql.DB.Close
// closes only the idle connections, so the held one keeps the DuckDB instance
// alive: the last committed rows are still only in <path>.wal, and the instance
// writes them to the file when the read lets go.

const heldMarker = "held_read_marker"

// walOnlyRow returns a database whose last committed row, heldMarker in
// _metadata, is only in <path>.wal, and a read that holds a connection.
func walOnlyRow(t *testing.T) (*DB, *sql.Rows) {
	t.Helper()
	database, err := Create(filepath.Join(t.TempDir(), "held.tdb"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Conn().Exec(`CREATE TABLE filler AS SELECT range AS id FROM range(1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().Exec(`CHECKPOINT`); err != nil {
		t.Fatal(err)
	}
	insertProbe(t, database, heldMarker)
	if _, err := os.Stat(database.Path() + ".wal"); err != nil {
		t.Fatalf("the marker row is not in a WAL: %v", err)
	}
	rows, err := database.Conn().Query(`SELECT id FROM filler`)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("no row")
	}
	t.Cleanup(func() { _ = rows.Close() })
	return database, rows
}

// waitForReopen waits until no open of the file is in flight.
func waitForReopen(t *testing.T, db *DB) {
	t.Helper()
	if !lockBy(&db.reopenMu, time.Now().Add(30*time.Second)) {
		t.Fatal("the reopen did not finish")
	}
	db.reopenMu.Unlock()
}

// TestWithFileClosedWaitsForAHeldRead: fn must not run until the read lets
// go, or its copy of the file misses the rows in the WAL.
func TestWithFileClosedWaitsForAHeldRead(t *testing.T) {
	database, rows := walOnlyRow(t)
	go func() { time.Sleep(300 * time.Millisecond); _ = rows.Close() }()

	copyPath := filepath.Join(t.TempDir(), "copy.tdb")
	walInFn := false
	err := database.WithFileClosed(func(p string) error {
		_, err := os.Stat(p + ".wal")
		walInFn = err == nil
		copyFileTo(t, p, copyPath)
		return nil
	})
	if err != nil {
		t.Fatalf("WithFileClosed() error = %v", err)
	}
	if walInFn {
		t.Error("fn ran while <path>.wal still held committed rows")
	}
	assertProbe(t, copyPath, heldMarker)
	insertProbe(t, database, "after") // the same handle works again
}

// TestWithFileClosedGivesUpOnAHeldRead: a read that does not let go must not
// hang the backup with txMu held. fn does not run, the call fails, and the
// handle reopens by itself when the read lets go.
func TestWithFileClosedGivesUpOnAHeldRead(t *testing.T) {
	shortReopenWait(t)
	database, rows := walOnlyRow(t)

	ran := false
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- database.WithFileClosed(func(string) error { ran = true; return nil }) }()
	select {
	case err := <-done:
		if err == nil || ran {
			t.Errorf("WithFileClosed() = %v, fn ran %v; want an error and no fn", err, ran)
		}
		t.Logf("WithFileClosed returned after %v: %v", time.Since(start).Round(time.Millisecond), err)
	case <-time.After(5 * time.Second):
		t.Error("WithFileClosed hung on a read that holds rows")
		_ = rows.Close()
		<-done
		return
	}

	_ = rows.Close()
	waitForReopen(t, database)
	insertProbe(t, database, "after") // reopened in the background
}

// withFileClosedBy runs WithFileClosed with fn and fails the test if it does
// not return within 5 seconds, so a call that waits for the read the test
// itself holds fails instead of hanging the package run. On a hang it lets the
// read go, so the call can return.
func withFileClosedBy(t *testing.T, database *DB, rows *sql.Rows, fn func(string) error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- database.WithFileClosed(fn) }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		_ = rows.Close()
		<-done
		t.Fatal("WithFileClosed hung on a read that holds rows")
		return nil
	}
}

// TestBackgroundReopenYieldsToALaterClose: after a WithFileClosed that gave up,
// a Close (the CLI's deferred Close) or CloseAndSettle (quit) must win over the
// reopen that waits for the read in the background.
func TestBackgroundReopenYieldsToALaterClose(t *testing.T) {
	for name, closeIt := range map[string]func(*DB){
		"Close":          func(d *DB) { _ = d.Close() },
		"CloseAndSettle": func(d *DB) { _, _ = d.CloseAndSettle(100 * time.Millisecond) },
	} {
		t.Run(name, func(t *testing.T) {
			shortReopenWait(t)
			database, rows := walOnlyRow(t)
			if err := withFileClosedBy(t, database, rows, func(string) error { return nil }); err == nil {
				t.Fatal("WithFileClosed() = nil while a read holds the instance")
			}
			closeIt(database)
			_ = rows.Close()
			waitForReopen(t, database)
			if !database.isClosed() {
				t.Error("the background reopen undid the later close")
			}
		})
	}
}

// TestWithFileClosedRetryWhileTheReadHolds: the user follows "try again" while
// the read still holds and the first call's reopen still waits for it. The
// retry must give the same "try again" answer, not the heal's "restart tmoney".
func TestWithFileClosedRetryWhileTheReadHolds(t *testing.T) {
	shortReopenWait(t)
	database, rows := walOnlyRow(t)
	for i := range 2 {
		err := withFileClosedBy(t, database, rows, func(string) error { return nil })
		if !errors.Is(err, errFileBusy) {
			t.Fatalf("call %d: WithFileClosed() = %v, want errFileBusy", i+1, err)
		}
	}
	_ = rows.Close()
	waitForReopen(t, database)
	insertProbe(t, database, "after")
}

// failedBackgroundReopen makes the reopen that waits for a held read in the
// background fail (the file is gone when the read lets go; in use, a network
// share that dropped), and then brings the file back. Nobody sees the error
// of that reopen, so the handle must not stay closed for good.
func failedBackgroundReopen(t *testing.T) *DB {
	t.Helper()
	shortReopenWait(t)
	database, rows := walOnlyRow(t)
	path := database.Path()
	if err := withFileClosedBy(t, database, rows, func(string) error { return nil }); !errors.Is(err, errFileBusy) {
		t.Fatalf("WithFileClosed() = %v, want errFileBusy", err)
	}

	// A folder in place of the file makes the reopen fail.
	if err := os.Rename(path, path+".away"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	waitForReopen(t, database)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".away", path); err != nil {
		t.Fatal(err)
	}
	return database
}

// healsWithin calls use until it succeeds, for up to 5 seconds. The heal runs
// in the background, so the calls that find the closed pool still fail.
func healsWithin(t *testing.T, use func() error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := use()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the handle did not heal after the file came back: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestFailedBackgroundReopenHealsOnAWrite: a write after a failed reopen
// heals the handle.
func TestFailedBackgroundReopenHealsOnAWrite(t *testing.T) {
	database := failedBackgroundReopen(t)
	healsWithin(t, func() error {
		return database.WithTx(func(tx Queryer) error {
			_, err := tx.Exec(`INSERT INTO _metadata (key, value) VALUES ('healed', 'present') ON CONFLICT DO NOTHING`)
			return err
		})
	})
}

// TestFailedBackgroundReopenHealsOnARead: the views only read. If only a
// failed write healed the handle, every view kept failing with "sql: database
// is closed" until the user wrote something.
func TestFailedBackgroundReopenHealsOnARead(t *testing.T) {
	database := failedBackgroundReopen(t)
	healsWithin(t, func() error {
		_, err := database.GetMetadata("app_identifier")
		return err
	})
}

// TestRetryWhileTheReopenWaitsReturnsAtOnce: while the background reopen still
// waits for the read, a retry must not hold the writer lock for the whole
// wait (5 s) before it says "try again": every write would wait with it.
func TestRetryWhileTheReopenWaitsReturnsAtOnce(t *testing.T) {
	shortReopenWait(t)
	database, rows := walOnlyRow(t)
	if err := withFileClosedBy(t, database, rows, func(string) error { return nil }); !errors.Is(err, errFileBusy) {
		t.Fatalf("WithFileClosed() = %v, want errFileBusy", err)
	}
	reopenWaitTimeout = SettleTimeout // the wait of a real retry
	start := time.Now()
	err := withFileClosedBy(t, database, rows, func(string) error { return nil })
	if !errors.Is(err, errFileBusy) {
		t.Errorf("the retry = %v, want errFileBusy", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("the retry took %v before it said try again", took.Round(time.Millisecond))
	}
	_ = rows.Close()
	waitForReopen(t, database)
}

// TestQuitAfterABlockedHealKeepsTheWALRows: a fatal error invalidated the
// instance, and its heal waits for a read in the background. An invalidated
// instance cannot checkpoint, so the rows committed before the fatal error are
// only in <path>.wal, and the heal's reopen is what writes them into the file.
// Quit must let that reopen run, or the quit backup (the file alone) misses
// them.
func TestQuitAfterABlockedHealKeepsTheWALRows(t *testing.T) {
	shortHealTimeout(t)
	database := desyncedIndexFixture(t)
	rows := seedAndHoldCursor(t, database) // 200 committed rows, only in the WAL
	t.Cleanup(func() { _ = rows.Close() })
	if _, err := os.Stat(database.Path() + ".wal"); err != nil {
		t.Fatalf("the seeded rows are not in a WAL: %v", err)
	}
	tripBlockedHeal(t, database)

	go func() { time.Sleep(100 * time.Millisecond); _ = rows.Close() }()
	if settled, err := database.CloseAndSettle(10 * time.Second); err != nil || !settled {
		t.Fatalf("CloseAndSettle() = %v, %v; want true, nil", settled, err)
	}
	copyPath := filepath.Join(t.TempDir(), "quit-copy.tdb")
	copyFileTo(t, database.Path(), copyPath) // what the quit backup copies
	copied, err := Open(copyPath)
	if err != nil {
		t.Fatalf("Open(copy) = %v", err)
	}
	defer func() { _ = copied.Close() }()
	var n int
	if err := copied.Conn().QueryRow(`SELECT COUNT(*) FROM ` + healTable).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 200 {
		t.Errorf("the quit backup holds %d of the 200 rows committed before the fatal error", n)
	}
}

// TestStaleHealQueuedBehindAFileOperation: a read starts the heal of a stale
// handle while a backup holds reopenMu. The backup's reopen gives the handle a
// healthy pool, so the heal must not close that pool and open the file again.
func TestStaleHealQueuedBehindAFileOperation(t *testing.T) {
	database, err := Create(filepath.Join(t.TempDir(), "stale.tdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	database.stale.Store(true)

	database.reopenMu.Lock() // the backup (WithFileClosed) holds it
	_ = database.Conn()      // a view read starts the heal, which queues
	if !waitUntil(time.Now().Add(5*time.Second), database.healing.Load) {
		database.reopenMu.Unlock()
		t.Fatal("the read did not start a heal")
	}
	_, gen, _ := database.close() // the backup's close, fn, and reopen
	if err := database.openPool(gen); err != nil {
		database.reopenMu.Unlock()
		t.Fatal(err)
	}
	healthy := database.live()
	database.reopenMu.Unlock()

	if !waitUntil(time.Now().Add(5*time.Second), func() bool { return !database.healing.Load() }) {
		t.Fatal("the heal did not end")
	}
	if database.live() != healthy {
		t.Error("the queued heal closed the pool that the backup opened, and opened the file again")
	}
}

// TestReopenNeverCreatesAMissingFile: DuckDB creates a new, empty database at a
// path with no file. When the file is gone at the reopen (moved, deleted, a
// share that dropped), the handle must not open such an empty database: it
// would show no data, and the quit backup would copy it.
func TestReopenNeverCreatesAMissingFile(t *testing.T) {
	away := func(t *testing.T, path string) {
		t.Helper()
		for _, suffix := range []string{"", ".wal"} {
			if err := os.Rename(path+suffix, path+".away"+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		}
	}

	t.Run("reopen after fn", func(t *testing.T) {
		database, err := Create(filepath.Join(t.TempDir(), "gone.tdb"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Close() })
		err = database.WithFileClosed(func(p string) error { away(t, p); return nil })
		var notFound *FileNotFoundError
		if !errors.As(err, &notFound) {
			t.Errorf("WithFileClosed() = %v, want a FileNotFoundError from the reopen", err)
		}
		if _, err := os.Stat(database.Path()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the reopen created a file at the path of the moved database: %v", err)
		}
	})

	t.Run("background reopen", func(t *testing.T) {
		shortReopenWait(t)
		database, rows := walOnlyRow(t)
		if err := withFileClosedBy(t, database, rows, func(string) error { return nil }); !errors.Is(err, errFileBusy) {
			t.Fatalf("WithFileClosed() = %v, want errFileBusy", err)
		}
		away(t, database.Path())
		_ = rows.Close()
		waitForReopen(t, database)
		if _, err := os.Stat(database.Path()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the background reopen created a file at the path of the moved database: %v", err)
		}
	})
}

// TestCloseAndSettleWaitsForAHeldRead covers the backup on quit and after a
// CLI command: it copies the file right after CloseAndSettle reports true.
func TestCloseAndSettleWaitsForAHeldRead(t *testing.T) {
	t.Run("read lets go", func(t *testing.T) {
		database, rows := walOnlyRow(t)
		go func() { time.Sleep(300 * time.Millisecond); _ = rows.Close() }()
		settled, err := database.CloseAndSettle(10 * time.Second)
		if err != nil || !settled {
			t.Fatalf("CloseAndSettle() = %v, %v; want true, nil", settled, err)
		}
		before := fileSnapshot(database.Path())
		copyPath := filepath.Join(t.TempDir(), "copy.tdb")
		copyFileTo(t, database.Path(), copyPath)
		assertProbe(t, copyPath, heldMarker)
		time.Sleep(300 * time.Millisecond)
		if after := fileSnapshot(database.Path()); before != after {
			t.Errorf("the file changed after CloseAndSettle: before %s, after %s", before, after)
		}
	})
	t.Run("read holds on", func(t *testing.T) {
		database, _ := walOnlyRow(t)
		if settled, err := database.CloseAndSettle(200 * time.Millisecond); err != nil || settled {
			t.Errorf("CloseAndSettle() = %v, %v; want false, nil while a read holds the instance", settled, err)
		}
	})
}
