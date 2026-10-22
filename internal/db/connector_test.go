package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// TestReadersDuringWithFileClosed is the crash this connector exists for: a
// reader whose query opens a new connection while a backup or restore closes
// the pool. database/sql calls Connect with no lock, and duckdb-go's Close
// frees the handle that Connect reads, so the process died with SIGSEGV in
// duckdb_connect. Every pool entry point that can connect runs here while
// WithFileClosed closes and reopens the pool; the only error a reader may see
// is "sql: database is closed".
func TestReadersDuringWithFileClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}
	database, err := Create(filepath.Join(t.TempDir(), "readers.tdb"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Conn().Exec(`CREATE TABLE probe (id INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	ops := map[string]func() error{
		"Query": func() error {
			rows, err := database.Conn().Query(`SELECT id FROM probe`)
			if err != nil {
				return err
			}
			return rows.Close()
		},
		"QueryRow": func() error {
			var n int
			return database.Conn().QueryRow(`SELECT count(*) FROM probe`).Scan(&n)
		},
		"Exec": func() error {
			_, err := database.Conn().Exec(`SELECT 1`)
			return err
		},
		"Begin": func() error {
			tx, err := database.Conn().Begin()
			if err != nil {
				return err
			}
			return tx.Rollback()
		},
		"Conn": func() error {
			conn, err := database.Conn().Conn(context.Background())
			if err != nil {
				return err
			}
			return conn.Close()
		},
		"Ping": func() error { return database.Conn().Ping() },
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	unexpected := map[string]int{}
	for name, op := range ops {
		for range 2 {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if err := op(); err != nil && err.Error() != "sql: database is closed" {
						mu.Lock()
						unexpected[name+": "+err.Error()]++
						mu.Unlock()
					}
				}
			})
		}
	}
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := database.WithFileClosed(func(string) error { return nil }); err != nil {
				t.Errorf("WithFileClosed() error = %v", err)
				return
			}
			time.Sleep(time.Duration(i%5) * time.Millisecond)
		}
	})
	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	for msg, n := range unexpected {
		t.Errorf("a reader got %q (%d times), want only %q", msg, n, "sql: database is closed")
	}
}

// TestConnectorCloseWaitsForConnect pins the contract that removes the crash:
// Close waits for a Connect in flight, a Connect after Close fails cleanly with
// ErrBadConn (database/sql then reports "sql: database is closed"), and a
// second Close does nothing.
func TestConnectorCloseWaitsForConnect(t *testing.T) {
	inner, err := duckdb.NewConnector(filepath.Join(t.TempDir(), "connector.tdb"), nil)
	if err != nil {
		t.Fatalf("NewConnector() error = %v", err)
	}
	c := &connector{inner: inner}
	conn, err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	_ = conn.Close()

	c.mu.RLock() // stands for a Connect in flight
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close() returned (%v) while a Connect was in flight", err)
	case <-time.After(100 * time.Millisecond):
	}
	c.mu.RUnlock()
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := c.Connect(context.Background()); !errors.Is(err, driver.ErrBadConn) {
		t.Errorf("Connect() after Close = %v, want ErrBadConn", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

// TestPoolAfterCloseReturnsClosedError keeps the error a caller sees after a
// Close: the connector must not change it.
func TestPoolAfterCloseReturnsClosedError(t *testing.T) {
	pool, err := openPoolAt(filepath.Join(t.TempDir(), "closed.tdb"))
	if err != nil {
		t.Fatalf("openPoolAt() error = %v", err)
	}
	if err := pool.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := pool.Query(`SELECT 1`); err == nil || err.Error() != "sql: database is closed" {
		t.Errorf("Query() after Close = %v, want %q", err, "sql: database is closed")
	}
}

// TestCheckedOutConnSurvivesPoolClose documents what Close leaves alone: a
// connection checked out of the pool keeps working, reads and writes, until it
// is returned, and then the file can be opened again.
func TestCheckedOutConnSurvivesPoolClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pinned.tdb")
	pool, err := openPoolAt(path)
	if err != nil {
		t.Fatalf("openPoolAt() error = %v", err)
	}
	if _, err := pool.Exec(`CREATE TABLE probe AS SELECT range AS id FROM range(1000)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx := context.Background()
	pinned, err := pool.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}
	rows, err := pinned.QueryContext(ctx, `SELECT id FROM probe ORDER BY id`)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !rows.Next() {
		t.Fatal("expected rows")
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("pool Close() error = %v", err)
	}

	n := 1
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil || n != 1000 {
		t.Fatalf("rows after pool Close: %d rows, err %v; want 1000, nil", n, err)
	}
	_ = rows.Close()
	if _, err := pinned.ExecContext(ctx, `INSERT INTO probe VALUES (-1)`); err != nil {
		t.Fatalf("write on the checked-out connection after pool Close: %v", err)
	}
	if err := pinned.Close(); err != nil {
		t.Fatalf("checked-out Close() error = %v", err)
	}

	again, err := openPoolAt(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	var count int
	if err := again.QueryRow(`SELECT count(*) FROM probe`).Scan(&count); err != nil || count != 1001 {
		t.Errorf("after reopen: %d rows, err %v; want 1001, nil", count, err)
	}
}
