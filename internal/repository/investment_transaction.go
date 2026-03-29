package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// InvestmentTransactionFilter defines optional filters for listing investment transactions.
type InvestmentTransactionFilter struct {
	Type       *models.InvestmentTransactionType
	SecurityID *models.ID
	FromDate   *models.Date
	ToDate     *models.Date
}

// InvestmentTransactionRepository provides database operations for investment transactions.
type InvestmentTransactionRepository struct {
	db *db.DB
}

// NewInvestmentTransactionRepository creates a new InvestmentTransactionRepository.
func NewInvestmentTransactionRepository(database *db.DB) *InvestmentTransactionRepository {
	return &InvestmentTransactionRepository{db: database}
}

// investmentTransactionColumns is the standard column list for investment transactions.
const investmentTransactionColumns = `id, account_id, date, transaction_type, security_id, shares,
	price_per_share, total_amount, commission, memo, status, created_at, updated_at`

// scanInvestmentTransaction scans a row into an InvestmentTransaction.
func scanInvestmentTransaction(row interface{ Scan(...any) error }) (*models.InvestmentTransaction, error) {
	t := &models.InvestmentTransaction{}
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
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	return t, err
}

// Create inserts a new investment transaction into the database.
func (r *InvestmentTransactionRepository) Create(txn *models.InvestmentTransaction) error {
	query := `
		INSERT INTO investment_transactions (
			id, account_id, date, transaction_type, security_id, shares,
			price_per_share, total_amount, commission, memo, status,
			created_at, updated_at
		) VALUES (?, CAST(? AS UUID), ?, ?, ` + nullUUIDCast(txn.SecurityID) + `, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Conn().Exec(query,
		txn.ID,
		txn.AccountID.String(),
		txn.Date.Time(),
		txn.Type.String(),
		nullID(txn.SecurityID),
		nullQuantity(txn.Shares),
		nullMoney(txn.PricePerShare),
		txn.TotalAmount.String(),
		nullMoney(txn.Commission),
		nullString(txn.Memo),
		txn.Status.String(),
		txn.CreatedAt,
		txn.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create investment transaction: %w", err)
	}

	return nil
}

// GetByID retrieves an investment transaction by its ID.
func (r *InvestmentTransactionRepository) GetByID(id models.ID) (*models.InvestmentTransaction, error) {
	query := `
		SELECT ` + investmentTransactionColumns + `
		FROM investment_transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	t, err := scanInvestmentTransaction(r.db.Conn().QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "investment_transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get investment transaction: %w", err)
	}

	return t, nil
}

// ListByAccount retrieves investment transactions for an account with optional filters.
func (r *InvestmentTransactionRepository) ListByAccount(accountID models.ID, filter InvestmentTransactionFilter) ([]*models.InvestmentTransaction, error) {
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

	transactions := make([]*models.InvestmentTransaction, 0)
	for rows.Next() {
		t, err := scanInvestmentTransaction(rows)
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
// Uses DELETE + INSERT pattern due to DuckDB limitations with UPDATE on indexed tables.
func (r *InvestmentTransactionRepository) Update(txn *models.InvestmentTransaction) error {
	txn.Touch()

	// Check if transaction exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM investment_transactions WHERE CAST(id AS VARCHAR) = ?`,
		txn.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check investment transaction exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "investment_transaction", ID: txn.ID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM investment_transactions WHERE CAST(id AS VARCHAR) = ?`,
		txn.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	query := `
		INSERT INTO investment_transactions (
			id, account_id, date, transaction_type, security_id, shares,
			price_per_share, total_amount, commission, memo, status,
			created_at, updated_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, ?, ` + nullUUIDCast(txn.SecurityID) + `, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		txn.ID.String(),
		txn.AccountID.String(),
		txn.Date.Time(),
		txn.Type.String(),
		nullID(txn.SecurityID),
		nullQuantity(txn.Shares),
		nullMoney(txn.PricePerShare),
		txn.TotalAmount.String(),
		nullMoney(txn.Commission),
		nullString(txn.Memo),
		txn.Status.String(),
		txn.CreatedAt.Time(),
		txn.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes an investment transaction from the database.
// Must manually delete from investment_transaction_lots first (DuckDB does not support ON DELETE CASCADE).
func (r *InvestmentTransactionRepository) Delete(id models.ID) error {
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
		return &NotFoundError{Entity: "investment_transaction", ID: id.String()}
	}

	return nil
}

// nullQuantity returns the string value of a NullableQuantity for database insertion, or nil.
func nullQuantity(nq models.NullableQuantity) any {
	if nq.Valid {
		return nq.Quantity.String()
	}
	return nil
}

// nullUUIDCast returns "CAST(? AS UUID)" if the NullableID is valid, or "?" if null.
// This is needed because DuckDB requires UUID casting for non-null UUID values.
func nullUUIDCast(nid models.NullableID) string {
	if nid.Valid {
		return "CAST(? AS UUID)"
	}
	return "?"
}
