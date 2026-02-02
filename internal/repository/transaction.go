package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// TransactionRepository provides database operations for transactions.
type TransactionRepository struct {
	db *db.DB
}

// NewTransactionRepository creates a new TransactionRepository.
func NewTransactionRepository(database *db.DB) *TransactionRepository {
	return &TransactionRepository{db: database}
}

// Create inserts a new transaction into the database.
func (r *TransactionRepository) Create(transaction *models.Transaction) error {
	// Verify account exists
	var accountExists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		transaction.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &NotFoundError{Entity: "account", ID: transaction.AccountID.String()}
	}

	// Verify payee exists if specified
	if transaction.PayeeID.Valid {
		var payeeExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &NotFoundError{Entity: "payee", ID: transaction.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if transaction.CategoryID.Valid {
		var categoryExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &NotFoundError{Entity: "category", ID: transaction.CategoryID.ID.String()}
		}
	}

	// Verify transfer account exists if specified
	if transaction.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &NotFoundError{Entity: "account", ID: transaction.TransferAccountID.ID.String()}
		}
	}

	query := `
		INSERT INTO transactions (
			id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		transaction.ID,
		transaction.AccountID,
		transaction.Date,
		transaction.Amount,
		nullID(transaction.PayeeID),
		nullID(transaction.CategoryID),
		nullString(transaction.Memo),
		nullString(transaction.CheckNumber),
		transaction.Status,
		nullID(transaction.TransferID),
		nullID(transaction.TransferAccountID),
		transaction.CreatedAt,
		transaction.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a transaction by its ID.
func (r *TransactionRepository) GetByID(id models.ID) (*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	transaction := &models.Transaction{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
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
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return transaction, nil
}

// List retrieves all transactions ordered by date descending.
func (r *TransactionRepository) List() ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactions(query)
}

// ListByAccount retrieves all transactions for a specific account.
func (r *TransactionRepository) ListByAccount(accountID models.ID) ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactionsWithArgs(query, accountID.String())
}

// ListByDateRange retrieves all transactions within a date range.
func (r *TransactionRepository) ListByDateRange(startDate, endDate models.Date) ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		WHERE date >= ? AND date <= ?
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactionsWithArgs(query, startDate.Time(), endDate.Time())
}

// ListByAccountAndDateRange retrieves transactions for an account within a date range.
func (r *TransactionRepository) ListByAccountAndDateRange(accountID models.ID, startDate, endDate models.Date) ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ? AND date >= ? AND date <= ?
		ORDER BY date DESC, created_at DESC
	`

	return r.queryTransactionsWithArgs(query, accountID.String(), startDate.Time(), endDate.Time())
}

// Update updates an existing transaction in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *TransactionRepository) Update(transaction *models.Transaction) error {
	transaction.Touch()

	// Check if transaction exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		transaction.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check transaction exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "transaction", ID: transaction.ID.String()}
	}

	// Verify account exists
	var accountExists bool
	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		transaction.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &NotFoundError{Entity: "account", ID: transaction.AccountID.String()}
	}

	// Verify payee exists if specified
	if transaction.PayeeID.Valid {
		var payeeExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &NotFoundError{Entity: "payee", ID: transaction.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if transaction.CategoryID.Valid {
		var categoryExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &NotFoundError{Entity: "category", ID: transaction.CategoryID.ID.String()}
		}
	}

	// Verify transfer account exists if specified
	if transaction.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			transaction.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &NotFoundError{Entity: "account", ID: transaction.TransferAccountID.ID.String()}
		}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM transactions WHERE CAST(id AS VARCHAR) = ?`,
		transaction.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO transactions (
			id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		transaction.ID.String(),
		transaction.AccountID.String(),
		transaction.Date.Time(),
		transaction.Amount.String(),
		nullID(transaction.PayeeID),
		nullID(transaction.CategoryID),
		nullString(transaction.Memo),
		nullString(transaction.CheckNumber),
		transaction.Status.String(),
		nullID(transaction.TransferID),
		nullID(transaction.TransferAccountID),
		transaction.CreatedAt.Time(),
		transaction.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a transaction from the database.
// This will fail if the transaction has any splits.
func (r *TransactionRepository) Delete(id models.ID) error {
	// Check for splits
	var splitCount int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?
	`, id.String()).Scan(&splitCount)
	if err != nil {
		return fmt.Errorf("failed to check splits: %w", err)
	}
	if splitCount > 0 {
		return &HasDependentsError{
			Entity:     "transaction",
			ID:         id.String(),
			Dependents: "splits",
			Count:      splitCount,
		}
	}

	result, err := r.db.Conn().Exec(
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
		return &NotFoundError{Entity: "transaction", ID: id.String()}
	}

	return nil
}

// CountByAccount returns the number of transactions for an account.
func (r *TransactionRepository) CountByAccount(accountID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, accountID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// CountByCategory returns the number of transactions for a category.
func (r *TransactionRepository) CountByCategory(categoryID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(category_id AS VARCHAR) = ?
	`, categoryID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// CountByPayee returns the number of transactions for a payee.
func (r *TransactionRepository) CountByPayee(payeeID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(payee_id AS VARCHAR) = ?
	`, payeeID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}
	return count, nil
}

// queryTransactions executes a query and returns a slice of transactions.
func (r *TransactionRepository) queryTransactions(query string) ([]*models.Transaction, error) {
	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// queryTransactionsWithArgs executes a query with arguments and returns a slice of transactions.
func (r *TransactionRepository) queryTransactionsWithArgs(query string, args ...interface{}) ([]*models.Transaction, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// scanTransactions scans rows into a slice of transactions.
func (r *TransactionRepository) scanTransactions(rows *sql.Rows) ([]*models.Transaction, error) {
	var transactions []*models.Transaction
	for rows.Next() {
		transaction := &models.Transaction{}
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

// TransactionSearchCriteria defines filters for searching transactions.
type TransactionSearchCriteria struct {
	// PayeeName filters by partial payee name match (case-insensitive).
	PayeeName string
	// Memo filters by partial memo match (case-insensitive).
	Memo string
	// CategoryName filters by partial category name match (case-insensitive).
	CategoryName string
	// StartDate filters transactions on or after this date.
	StartDate *models.Date
	// EndDate filters transactions on or before this date.
	EndDate *models.Date
	// AccountID filters by specific account.
	AccountID *models.ID
}

// HasFilters returns true if any search criteria is set.
func (c *TransactionSearchCriteria) HasFilters() bool {
	return c.PayeeName != "" ||
		c.Memo != "" ||
		c.CategoryName != "" ||
		c.StartDate != nil ||
		c.EndDate != nil ||
		c.AccountID != nil
}

// Search finds transactions matching the given criteria.
// Multiple criteria are combined with AND logic.
// Results are ordered by date descending.
func (r *TransactionRepository) Search(criteria TransactionSearchCriteria) ([]*models.Transaction, error) {
	// Build the query dynamically based on criteria
	query := `
		SELECT DISTINCT t.id, t.account_id, t.date, t.amount, t.payee_id, t.category_id,
			t.memo, t.check_number, t.status, t.transfer_id, t.transfer_account_id,
			t.created_at, t.updated_at
		FROM transactions t
	`

	var joins []string
	var conditions []string
	var args []interface{}

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

	// Build final query
	for _, join := range joins {
		query += " " + join
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, cond := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}

	query += " ORDER BY t.date DESC, t.created_at DESC"

	return r.queryTransactionsWithArgs(query, args...)
}

// SearchByPayee finds transactions by partial payee name match (case-insensitive).
func (r *TransactionRepository) SearchByPayee(payeeName string) ([]*models.Transaction, error) {
	return r.Search(TransactionSearchCriteria{PayeeName: payeeName})
}

// SearchByMemo finds transactions by partial memo match (case-insensitive).
func (r *TransactionRepository) SearchByMemo(memo string) ([]*models.Transaction, error) {
	return r.Search(TransactionSearchCriteria{Memo: memo})
}

// SearchByCategory finds transactions by partial category name match (case-insensitive).
func (r *TransactionRepository) SearchByCategory(categoryName string) ([]*models.Transaction, error) {
	return r.Search(TransactionSearchCriteria{CategoryName: categoryName})
}
