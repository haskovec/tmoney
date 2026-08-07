package transaction

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Repository provides database operations for transactions.
type Repository struct {
	db *db.DB
	tx db.Queryer // nil outside a transaction
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// q returns the active Queryer: the bound transaction if any, else the
// live connection. All SQL in this repo goes through q().
func (r *Repository) q() db.Queryer {
	if r.tx != nil {
		return r.tx
	}
	return r.db.Conn()
}

// WithTx returns a copy of the repository bound to tx. The original is
// unchanged and remains safe for non-transactional use.
func (r *Repository) WithTx(tx db.Queryer) *Repository {
	c := *r
	c.tx = tx
	return &c
}

// Create inserts a new transaction into the database.
func (r *Repository) Create(transaction *Transaction) error {
	// Verify account exists
	var accountExists bool
	err := r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		transaction.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &dberrors.NotFoundError{Entity: "account", ID: transaction.AccountID.String()}
	}

	// Verify payee exists if specified
	if transaction.PayeeID.Valid {
		var payeeExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &dberrors.NotFoundError{Entity: "payee", ID: transaction.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if transaction.CategoryID.Valid {
		var categoryExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &dberrors.NotFoundError{Entity: "category", ID: transaction.CategoryID.ID.String()}
		}
	}

	// Verify transfer account exists if specified
	if transaction.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &dberrors.NotFoundError{Entity: "account", ID: transaction.TransferAccountID.ID.String()}
		}
	}

	query := `
		INSERT INTO transactions (
			id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.q().Exec(query,
		transaction.ID,
		transaction.AccountID,
		transaction.Date,
		transaction.Amount,
		dbutil.NullID(transaction.PayeeID),
		dbutil.NullID(transaction.CategoryID),
		dbutil.NullString(transaction.Memo),
		dbutil.NullString(transaction.CheckNumber),
		transaction.Status,
		dbutil.NullID(transaction.TransferID),
		dbutil.NullID(transaction.TransferAccountID),
		dbutil.NullString(transaction.BankReferenceID),
		transaction.CreatedAt,
		transaction.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a transaction by its ID.
func (r *Repository) GetByID(id types.ID) (*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	transaction := &Transaction{}
	err := r.q().QueryRow(query, id.String()).Scan(
		&transaction.ID,
		&transaction.AccountID,
		&transaction.Date,
		&transaction.Amount,
		&transaction.PayeeID,
		&transaction.CategoryID,
		&transaction.Memo,
		&transaction.CheckNumber,
		&transaction.Status,
		&transaction.TransferID,
		&transaction.TransferAccountID,
		&transaction.BankReferenceID,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return transaction, nil
}

// List retrieves all transactions ordered by date descending.
func (r *Repository) List() ([]*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactions(query)
}

// ListByAccount retrieves all transactions for a specific account.
func (r *Repository) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactionsWithArgs(query, accountID.String())
}

// ListByTransferID retrieves all transactions sharing a transfer_id.
// Used to find both sides of a transfer pair.
func (r *Repository) ListByTransferID(transferID types.ID) ([]*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(transfer_id AS VARCHAR) = ?
		ORDER BY created_at ASC
	`

	return r.queryTransactionsWithArgs(query, transferID.String())
}

// ListByAccountAndDateRange retrieves transactions for an account within a date range.
func (r *Repository) ListByAccountAndDateRange(accountID types.ID, startDate, endDate types.Date) ([]*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ? AND date >= ? AND date <= ?
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactionsWithArgs(query, accountID.String(), startDate.Time(), endDate.Time())
}

// Update updates an existing transaction in the database.
// Uses an in-place UPDATE (not DELETE+INSERT) so it is safe for transactions
// that have child transaction_splits rows; see migration 026.
func (r *Repository) Update(transaction *Transaction) error {
	transaction.Touch()

	// Check if transaction exists
	var count int
	err := r.q().QueryRow(
		`SELECT COUNT(*) FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		transaction.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check transaction exists: %w", err)
	}
	if count == 0 {
		return &dberrors.NotFoundError{Entity: "transaction", ID: transaction.ID.String()}
	}

	// Verify account exists
	var accountExists bool
	err = r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		transaction.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &dberrors.NotFoundError{Entity: "account", ID: transaction.AccountID.String()}
	}

	// Verify payee exists if specified
	if transaction.PayeeID.Valid {
		var payeeExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &dberrors.NotFoundError{Entity: "payee", ID: transaction.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if transaction.CategoryID.Valid {
		var categoryExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &dberrors.NotFoundError{Entity: "category", ID: transaction.CategoryID.ID.String()}
		}
	}

	// Verify transfer account exists if specified
	if transaction.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &dberrors.NotFoundError{Entity: "account", ID: transaction.TransferAccountID.ID.String()}
		}
	}

	// Persist with an in-place UPDATE rather than DELETE+INSERT. DuckDB
	// rewrites an UPDATE that touches an indexed/FK-backed column as an
	// internal delete+insert; combined with the explicit DELETE the old code
	// used, that tripped the inbound transaction_splits.transaction_id FK on
	// any transaction with splits (a multi-category transaction, e.g. a split
	// paycheck deposit) — breaking reconcile-finish, the cleared-status toggle,
	// and split-transaction header edits. Migration 026 drops that inbound FK
	// (app-level code maintains split integrity), so the row can be rewritten
	// safely; the in-place UPDATE also avoids needlessly rewriting created_at.
	result, err := r.q().Exec(`
		UPDATE transactions SET
			account_id = CAST(? AS UUID), date = ?, amount = ?, payee_id = ?,
			category_id = ?, memo = ?, check_number = ?, status = ?,
			transfer_id = ?, transfer_account_id = ?, bank_reference_id = ?,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		transaction.AccountID.String(),
		transaction.Date.Time(),
		transaction.Amount.String(),
		dbutil.NullID(transaction.PayeeID),
		dbutil.NullID(transaction.CategoryID),
		dbutil.NullString(transaction.Memo),
		dbutil.NullString(transaction.CheckNumber),
		transaction.Status.String(),
		dbutil.NullID(transaction.TransferID),
		dbutil.NullID(transaction.TransferAccountID),
		dbutil.NullString(transaction.BankReferenceID),
		transaction.UpdatedAt.Time(),
		transaction.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "transaction", ID: transaction.ID.String()}
	}

	return nil
}

// UpdateStatus updates only the status (and updated_at) of a transaction, in
// place. It is the safe path for status-only changes — reconcile, clear,
// unclear, un-reconcile — and does NOT rewrite the rest of the row.
//
// DuckDB rewrites an UPDATE as an internal DELETE+INSERT whenever the SET clause
// touches an indexed or FK-backed column, and it can leave a secondary ART index
// desynced from the table on disk (a storage bug). When that happens the rewrite
// aborts with "Failed to delete all rows from index. Only deleted 0 out of 1
// rows" — the failure that broke reconcile-finish on a transfer whose
// transfer_id index entry could no longer be deleted. Migration 030 dropped the
// index on status, and updated_at is neither indexed nor an FK, so this narrow
// UPDATE touches no index at all: DuckDB performs a genuine in-place update, so
// it never rewrites the row and is immune to a desynced index on any other
// column. (The full-row Update stays for header/amount/transfer edits.)
func (r *Repository) UpdateStatus(id types.ID, status Status) error {
	result, err := r.q().Exec(`
		UPDATE transactions SET status = ?, updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`, status.String(), types.Now().Time(), id.String())
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "transaction", ID: id.String()}
	}
	return nil
}

// Delete removes a transaction from the database.
// This will fail if the transaction has any splits.
func (r *Repository) Delete(id types.ID) error {
	// Check for splits
	var splitCount int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?
	`, id.String()).Scan(&splitCount)
	if err != nil {
		return fmt.Errorf("failed to check splits: %w", err)
	}
	if splitCount > 0 {
		return &dberrors.HasDependentsError{
			Entity:     "transaction",
			ID:         id.String(),
			Dependents: "splits",
			Count:      splitCount,
		}
	}

	result, err := r.q().Exec(
		`DELETE FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "transaction", ID: id.String()}
	}

	return nil
}

// CountByAccount returns the number of transactions for an account.
func (r *Repository) CountByAccount(accountID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, accountID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// CountByCategory returns the number of transactions for a category.
func (r *Repository) CountByCategory(categoryID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(category_id AS VARCHAR) = ?
	`, categoryID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// CountByPayee returns the number of transactions for a payee.
func (r *Repository) CountByPayee(payeeID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(payee_id AS VARCHAR) = ?
	`, payeeID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// GetByBankReferenceID retrieves a transaction by its bank reference ID within an account.
func (r *Repository) GetByBankReferenceID(accountID types.ID, bankRefID string) (*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ? AND bank_reference_id = ?
	`

	transaction := &Transaction{}
	err := r.q().QueryRow(query, accountID.String(), bankRefID).Scan(
		&transaction.ID,
		&transaction.AccountID,
		&transaction.Date,
		&transaction.Amount,
		&transaction.PayeeID,
		&transaction.CategoryID,
		&transaction.Memo,
		&transaction.CheckNumber,
		&transaction.Status,
		&transaction.TransferID,
		&transaction.TransferAccountID,
		&transaction.BankReferenceID,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		return nil, nil // Not found returns nil, nil (not an error)
	}

	return transaction, nil
}

// queryTransactions executes a query and returns a slice of transactions.
func (r *Repository) queryTransactions(query string) ([]*Transaction, error) {
	rows, err := r.q().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// queryTransactionsWithArgs executes a query with arguments and returns a slice of transactions.
func (r *Repository) queryTransactionsWithArgs(query string, args ...any) ([]*Transaction, error) {
	rows, err := r.q().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// scanTransactions scans rows into a slice of transactions.
func (r *Repository) scanTransactions(rows *sql.Rows) ([]*Transaction, error) {
	var transactions []*Transaction
	for rows.Next() {
		transaction := &Transaction{}
		err := rows.Scan(
			&transaction.ID,
			&transaction.AccountID,
			&transaction.Date,
			&transaction.Amount,
			&transaction.PayeeID,
			&transaction.CategoryID,
			&transaction.Memo,
			&transaction.CheckNumber,
			&transaction.Status,
			&transaction.TransferID,
			&transaction.TransferAccountID,
			&transaction.BankReferenceID,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	return transactions, nil
}

// Search finds transactions matching the given criteria.
// Multiple criteria are combined with AND logic.
// Results are ordered by date descending.
func (r *Repository) Search(criteria SearchCriteria) ([]*Transaction, error) {
	// Build the query dynamically based on criteria
	var query strings.Builder
	query.WriteString(`
		SELECT DISTINCT t.id, t.account_id, t.date, t.amount, t.payee_id, t.category_id,
			t.memo, t.check_number, t.status, t.transfer_id, t.transfer_account_id,
			t.bank_reference_id, t.created_at, t.updated_at
		FROM transactions t
	`)

	var joins []string
	var conditions []string
	var args []any

	// Join payees table if filtering by payee name
	if criteria.PayeeName != "" {
		joins = append(joins, "LEFT JOIN payees p ON CAST(t.payee_id AS VARCHAR) = CAST(p.id AS VARCHAR)")
		conditions = append(conditions, "LOWER(p.name) LIKE LOWER(?)")
		args = append(args, "%"+criteria.PayeeName+"%")
	}

	// Join categories table if filtering by category name
	if criteria.CategoryName != "" {
		joins = append(joins, "LEFT JOIN categories c ON CAST(t.category_id AS VARCHAR) = CAST(c.id AS VARCHAR)")
		conditions = append(conditions, "LOWER(c.name) LIKE LOWER(?)")
		args = append(args, "%"+criteria.CategoryName+"%")
	}

	// Filter by memo
	if criteria.Memo != "" {
		conditions = append(conditions, "LOWER(t.memo) LIKE LOWER(?)")
		args = append(args, "%"+criteria.Memo+"%")
	}

	// Filter by date range
	if criteria.StartDate != nil {
		conditions = append(conditions, "t.date >= ?")
		args = append(args, criteria.StartDate.Time())
	}
	if criteria.EndDate != nil {
		conditions = append(conditions, "t.date <= ?")
		args = append(args, criteria.EndDate.Time())
	}

	// Filter by account
	if criteria.AccountID != nil {
		conditions = append(conditions, "CAST(t.account_id AS VARCHAR) = ?")
		args = append(args, criteria.AccountID.String())
	}

	// Filter by amount range
	if criteria.MinAmount != nil {
		conditions = append(conditions, "t.amount >= CAST(? AS DECIMAL)")
		args = append(args, criteria.MinAmount.String())
	}
	if criteria.MaxAmount != nil {
		conditions = append(conditions, "t.amount <= CAST(? AS DECIMAL)")
		args = append(args, criteria.MaxAmount.String())
	}

	// Build final query
	for _, join := range joins {
		query.WriteString(" ")
		query.WriteString(join)
	}

	if len(conditions) > 0 {
		query.WriteString(" WHERE ")
		for i, cond := range conditions {
			if i > 0 {
				query.WriteString(" AND ")
			}
			query.WriteString(cond)
		}
	}

	query.WriteString(" ORDER BY t.date DESC, t.created_at DESC")

	return r.queryTransactionsWithArgs(query.String(), args...)
}
