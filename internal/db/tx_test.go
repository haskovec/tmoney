package db

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// newTxTestDB creates a fresh database with a plain tx_test table for the
// WithTx tests. It never touches domain tables.
func newTxTestDB(t *testing.T) *DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tx_test.tdb")
	db, err := Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Conn().Exec(`CREATE TABLE tx_test(id INTEGER, v TEXT)`); err != nil {
		t.Fatalf("create tx_test table: %v", err)
	}
	return db
}

// countRows returns how many rows in tx_test have the given id.
func countRows(t *testing.T, db *DB, id int) int {
	t.Helper()
	var n int
	if err := db.Conn().QueryRow(`SELECT COUNT(*) FROM tx_test WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count rows id=%d: %v", id, err)
	}
	return n
}

func TestWithTx_CommitPersists(t *testing.T) {
	db := newTxTestDB(t)

	err := db.WithTx(func(tx Queryer) error {
		_, err := tx.Exec(`INSERT INTO tx_test (id, v) VALUES (?, ?)`, 1, "hello")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	if got := countRows(t, db, 1); got != 1 {
		t.Errorf("row count after commit = %d, want 1", got)
	}
}

func TestWithTx_ErrorRollsBack(t *testing.T) {
	db := newTxTestDB(t)

	sentinel := errors.New("boom")
	err := db.WithTx(func(tx Queryer) error {
		if _, err := tx.Exec(`INSERT INTO tx_test (id, v) VALUES (?, ?)`, 2, "world"); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTx() error = %v, want wraps %v", err, sentinel)
	}

	if got := countRows(t, db, 2); got != 0 {
		t.Errorf("row count after rollback = %d, want 0", got)
	}
}

func TestWithTx_PanicRollsBackAndPropagates(t *testing.T) {
	db := newTxTestDB(t)

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic to propagate, got none")
			}
			if r != "kaboom" {
				t.Fatalf("recovered %v, want kaboom", r)
			}
		}()

		_ = db.WithTx(func(tx Queryer) error {
			if _, err := tx.Exec(`INSERT INTO tx_test (id, v) VALUES (?, ?)`, 3, "panic"); err != nil {
				t.Fatalf("insert before panic: %v", err)
			}
			panic("kaboom")
		})
	}()

	if got := countRows(t, db, 3); got != 0 {
		t.Errorf("row count after panic rollback = %d, want 0", got)
	}
}

func TestWithTx_ConcurrentCallsSerialize(t *testing.T) {
	db := newTxTestDB(t)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs[id] = db.WithTx(func(tx Queryer) error {
				_, err := tx.Exec(`INSERT INTO tx_test (id, v) VALUES (?, ?)`, id, "concurrent")
				return err
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d WithTx() error = %v", i, err)
		}
	}
	if got := countRows(t, db, 0); got != 1 {
		t.Errorf("row count id=0 = %d, want 1", got)
	}
	if got := countRows(t, db, 1); got != 1 {
		t.Errorf("row count id=1 = %d, want 1", got)
	}
}

func TestWithTx_SeesOwnWrites(t *testing.T) {
	db := newTxTestDB(t)

	err := db.WithTx(func(tx Queryer) error {
		if _, err := tx.Exec(`INSERT INTO tx_test (id, v) VALUES (?, ?)`, 4, "own"); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM tx_test WHERE id = ?`, 4).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("in-tx read of own write = %d, want 1", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}
