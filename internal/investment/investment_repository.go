package investment

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// TransactionFilter defines optional filters for listing investment transactions.
type TransactionFilter struct {
	Type       *TransactionType
	SecurityID *types.ID
	FromDate   *types.Date
	ToDate     *types.Date
}

// Repository provides database operations for investment transactions.
type Repository struct {
	db *db.DB
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// investmentTransactionColumns is the standard column list for investment transactions.
const investmentTransactionColumns = `id, account_id, date, transaction_type, security_id, shares,
	price_per_share, total_amount, commission, memo, status, transfer_id, transfer_account_id,
	created_at, updated_at`

// scanTransaction scans a row into a Transaction.
func scanTransaction(row interface{ Scan(...any) error }) (*Transaction, error) {
	t := &Transaction{}
	err := row.Scan(
		&t.ID,
		&t.AccountID,
		&t.Date,
		&t.Type,
		&t.SecurityID,
		&t.Shares,
		&t.PricePerShare,
		&t.TotalAmount,
		&t.Commission,
		&t.Memo,
		&t.Status,
		&t.TransferID,
		&t.TransferAccountID,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	return t, err
}

// Create inserts a new investment transaction into the database.
func (r *Repository) Create(txn *Transaction) error {
	query := `
		INSERT INTO investment_transactions (
			id, account_id, date, transaction_type, security_id, shares,
			price_per_share, total_amount, commission, memo, status,
			transfer_id, transfer_account_id,
			created_at, updated_at
		) VALUES (?, CAST(? AS UUID), ?, ?, ` + dbutil.NullUUIDCast(txn.SecurityID) + `, ?, ?, ?, ?, ?, ?,
			` + dbutil.NullUUIDCast(txn.TransferID) + `, ` + dbutil.NullUUIDCast(txn.TransferAccountID) + `,
			?, ?)
	`

	_, err := r.db.Conn().Exec(query,
		txn.ID,
		txn.AccountID.String(),
		txn.Date.Time(),
		txn.Type.String(),
		dbutil.NullID(txn.SecurityID),
		dbutil.NullQuantity(txn.Shares),
		dbutil.NullMoney(txn.PricePerShare),
		txn.TotalAmount.String(),
		dbutil.NullMoney(txn.Commission),
		dbutil.NullString(txn.Memo),
		txn.Status.String(),
		dbutil.NullID(txn.TransferID),
		dbutil.NullID(txn.TransferAccountID),
		txn.CreatedAt,
		txn.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create investment transaction: %w", err)
	}

	return nil
}

// GetByID retrieves an investment transaction by its ID.
func (r *Repository) GetByID(id types.ID) (*Transaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	t, err := scanTransaction(r.db.Conn().QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "investment_transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get investment transaction: %w", err)
	}

	return t, nil
}

// ListByAccount retrieves investment transactions for an account with optional filters.
func (r *Repository) ListByAccount(accountID types.ID, filter TransactionFilter) ([]*Transaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(account_id AS VARCHAR) = ?
	`
	args := []any{accountID.String()}

	if filter.Type != nil {
		query += " AND transaction_type = ?"
		args = append(args, filter.Type.String())
	}
	if filter.SecurityID != nil {
		query += " AND CAST(security_id AS VARCHAR) = ?"
		args = append(args, filter.SecurityID.String())
	}
	if filter.FromDate != nil {
		query += " AND date >= ?"
		args = append(args, filter.FromDate.Time())
	}
	if filter.ToDate != nil {
		query += " AND date <= ?"
		args = append(args, filter.ToDate.Time())
	}

	query += " ORDER BY date DESC, created_at DESC"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list investment transactions: %w", err)
	}
	defer rows.Close()

	transactions := make([]*Transaction, 0)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan investment transaction: %w", err)
		}
		transactions = append(transactions, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating investment transactions: %w", err)
	}

	return transactions, nil
}

// ListByTransferID retrieves all investment transactions linked by a
// shared transfer_id. Used to find the investment-side counterpart of
// a transfer-line split (where the parent transaction lives on the
// regular transactions table) and the destination-side row of an
// inv↔inv cash transfer.
func (r *Repository) ListByTransferID(transferID types.ID) ([]*Transaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(transfer_id AS VARCHAR) = ?
		ORDER BY created_at ASC
	`

	rows, err := r.db.Conn().Query(query, transferID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list investment transactions by transfer_id: %w", err)
	}
	defer rows.Close()

	transactions := make([]*Transaction, 0)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan investment transaction: %w", err)
		}
		transactions = append(transactions, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating investment transactions: %w", err)
	}

	return transactions, nil
}

// EarliestSinceDate returns the earliest investment transaction (by
// date, then by created_at) on or after a given date for a security,
// across all accounts. Returns sql.ErrNoRows wrapped as a NotFound when
// nothing matches. Used to enforce the "no downstream events" guard
// before reversing a corporate action.
func (r *Repository) EarliestSinceDate(securityID types.ID, since types.Date) (*Transaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(security_id AS VARCHAR) = ? AND date >= ?
		ORDER BY date ASC, created_at ASC
		LIMIT 1
	`
	row := r.db.Conn().QueryRow(query, securityID.String(), since.Time())
	t, err := scanTransaction(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find earliest transaction since date: %w", err)
	}
	return t, nil
}

// ListBySecurity returns every investment transaction for a security across all
// accounts, oldest first. Used by corporate-action reversal to find and remove
// the transactions an action created.
func (r *Repository) ListBySecurity(securityID types.ID) ([]*Transaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(security_id AS VARCHAR) = ?
		ORDER BY date ASC, created_at ASC
	`
	rows, err := r.db.Conn().Query(query, securityID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list investment transactions by security: %w", err)
	}
	defer rows.Close()

	transactions := make([]*Transaction, 0)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan investment transaction: %w", err)
		}
		transactions = append(transactions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating investment transactions: %w", err)
	}
	return transactions, nil
}

// Update updates an existing investment transaction in the database.
func (r *Repository) Update(txn *Transaction) error {
	txn.Touch()

	query := `
		UPDATE investment_transactions SET
			account_id = CAST(? AS UUID),
			date = ?,
			transaction_type = ?,
			security_id = ` + dbutil.NullUUIDCast(txn.SecurityID) + `,
			shares = ?,
			price_per_share = ?,
			total_amount = ?,
			commission = ?,
			memo = ?,
			status = ?,
			transfer_id = ` + dbutil.NullUUIDCast(txn.TransferID) + `,
			transfer_account_id = ` + dbutil.NullUUIDCast(txn.TransferAccountID) + `,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`

	result, err := r.db.Conn().Exec(query,
		txn.AccountID.String(),
		txn.Date.Time(),
		txn.Type.String(),
		dbutil.NullID(txn.SecurityID),
		dbutil.NullQuantity(txn.Shares),
		dbutil.NullMoney(txn.PricePerShare),
		txn.TotalAmount.String(),
		dbutil.NullMoney(txn.Commission),
		dbutil.NullString(txn.Memo),
		txn.Status.String(),
		dbutil.NullID(txn.TransferID),
		dbutil.NullID(txn.TransferAccountID),
		txn.UpdatedAt.Time(),
		txn.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update investment transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "investment_transaction", ID: txn.ID.String()}
	}

	return nil
}

// Delete removes an investment transaction from the database.
// Must manually delete from investment_transaction_lots first (DuckDB does not support ON DELETE CASCADE).
func (r *Repository) Delete(id types.ID) error {
	// Delete junction records first
	_, err := r.db.Conn().Exec(
		`DELETE FROM investment_transaction_lots WHERE CAST(transaction_id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete transaction lot records: %w", err)
	}

	// Delete the transaction
	result, err := r.db.Conn().Exec(
		`DELETE FROM investment_transactions WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete investment transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "investment_transaction", ID: id.String()}
	}

	return nil
}
