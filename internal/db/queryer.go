package db

import "database/sql"

// Queryer is the query/exec surface shared by *sql.DB and *sql.Tx.
// Repositories run all SQL through a Queryer so the same method works
// inside and outside a transaction.
type Queryer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}
