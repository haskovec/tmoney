package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// connector wraps the DuckDB connector so that Close waits for a Connect in
// flight.
//
// database/sql calls Connector.Connect with no lock of its own, and
// sql.DB.Close does not wait for it before it calls Connector.Close. duckdb-go
// (v2.10506.0, and its main branch as of 2026-10) does not guard the handle
// either: Close runs duckdb_close, which frees the handle that duckdb_connect
// is reading. A view load that opened a connection while a backup, restore,
// heal or quit closed the pool therefore crashed the process with SIGSEGV in
// duckdb_connect. Delete this type when duckdb-go guards Connect against Close.
type connector struct {
	mu     sync.RWMutex
	closed bool
	inner  *duckdb.Connector
}

var (
	_ driver.Connector = (*connector)(nil)
	_ io.Closer        = (*connector)(nil)
)

// openPoolAt opens a pool on the DuckDB file at path through connector. Like
// sql.Open("duckdb", path), it opens the file now.
func openPoolAt(path string) (*sql.DB, error) {
	return openPoolInit(path, nil)
}

// openPoolInit is openPoolAt with a DSN, and with init, when not nil, run on
// each new connection before its first use.
func openPoolInit(dsn string, init func(driver.ExecerContext) error) (*sql.DB, error) {
	inner, err := duckdb.NewConnector(dsn, init)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(&connector{inner: inner}), nil
}

// Connect opens a connection unless the connector is closed. After Close it
// returns driver.ErrBadConn; database/sql then sees its pool closed and
// reports "sql: database is closed".
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, driver.ErrBadConn
	}
	return c.inner.Connect(ctx)
}

// Driver returns the DuckDB driver.
func (c *connector) Driver() driver.Driver { return c.inner.Driver() }

// Close closes the DuckDB connector once, after any Connect in flight.
func (c *connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.inner.Close()
}
