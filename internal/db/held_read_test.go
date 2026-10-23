package db

import (
	"database/sql"
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
			if err := database.WithFileClosed(func(string) error { return nil }); err == nil {
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
