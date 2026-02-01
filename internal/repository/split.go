package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// SplitRepository provides database operations for transaction splits.
type SplitRepository struct {
	db *db.DB
}

// NewSplitRepository creates a new SplitRepository.
func NewSplitRepository(database *db.DB) *SplitRepository {
	return &SplitRepository{db: database}
}

// Create inserts a new split into the database.
func (r *SplitRepository) Create(split *models.Split) error {
	// Verify transaction exists
	var txnExists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM transactions WHERE CAST(id AS VARCHAR) = ?)`,
		split.TransactionID.String(),
	).Scan(&txnExists)
	if err != nil {
		return fmt.Errorf("failed to check transaction exists: %w", err)
	}
	if !txnExists {
		return &NotFoundError{Entity: "transaction", ID: split.TransactionID.String()}
	}

	// Verify category exists
	var categoryExists bool
	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
		split.CategoryID.String(),
	).Scan(&categoryExists)
	if err != nil {
		return fmt.Errorf("failed to check category exists: %w", err)
	}
	if !categoryExists {
		return &NotFoundError{Entity: "category", ID: split.CategoryID.String()}
	}

	query := `
		INSERT INTO transaction_splits (
			id, transaction_id, category_id, amount, memo, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		split.ID,
		split.TransactionID,
		split.CategoryID,
		split.Amount,
		nullString(split.Memo),
		split.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create split: %w", err)
	}

	return nil
}

// GetByID retrieves a split by its ID.
func (r *SplitRepository) GetByID(id models.ID) (*models.Split, error) {
	query := `
		SELECT id, transaction_id, category_id, amount, memo, created_at
		FROM transaction_splits
		WHERE CAST(id AS VARCHAR) = ?
	`

	split := &models.Split{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&split.ID,
		&split.TransactionID,
		&split.CategoryID,
		&split.Amount,
		&split.Memo,
		&split.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "split", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get split: %w", err)
	}

	// Set UpdatedAt to CreatedAt since splits don't have updated_at in the schema
	split.UpdatedAt = split.CreatedAt

	return split, nil
}

// ListByTransaction retrieves all splits for a transaction.
func (r *SplitRepository) ListByTransaction(transactionID models.ID) ([]*models.Split, error) {
	query := `
		SELECT id, transaction_id, category_id, amount, memo, created_at
		FROM transaction_splits
		WHERE CAST(transaction_id AS VARCHAR) = ?
		ORDER BY created_at
	`

	return r.querySplitsWithArgs(query, transactionID.String())
}

// Update updates an existing split in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *SplitRepository) Update(split *models.Split) error {
	// Check if split exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM transaction_splits WHERE CAST(id AS VARCHAR) = ?`,
		split.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check split exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "split", ID: split.ID.String()}
	}

	// Verify transaction exists
	var txnExists bool
	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM transactions WHERE CAST(id AS VARCHAR) = ?)`,
		split.TransactionID.String(),
	).Scan(&txnExists)
	if err != nil {
		return fmt.Errorf("failed to check transaction exists: %w", err)
	}
	if !txnExists {
		return &NotFoundError{Entity: "transaction", ID: split.TransactionID.String()}
	}

	// Verify category exists
	var categoryExists bool
	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
		split.CategoryID.String(),
	).Scan(&categoryExists)
	if err != nil {
		return fmt.Errorf("failed to check category exists: %w", err)
	}
	if !categoryExists {
		return &NotFoundError{Entity: "category", ID: split.CategoryID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM transaction_splits WHERE CAST(id AS VARCHAR) = ?`,
		split.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO transaction_splits (
			id, transaction_id, category_id, amount, memo, created_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		split.ID.String(),
		split.TransactionID.String(),
		split.CategoryID.String(),
		split.Amount.String(),
		nullString(split.Memo),
		split.CreatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a split from the database.
func (r *SplitRepository) Delete(id models.ID) error {
	result, err := r.db.Conn().Exec(
		`DELETE FROM transaction_splits WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete split: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &NotFoundError{Entity: "split", ID: id.String()}
	}

	return nil
}

// DeleteByTransaction removes all splits for a transaction.
func (r *SplitRepository) DeleteByTransaction(transactionID models.ID) (int, error) {
	result, err := r.db.Conn().Exec(
		`DELETE FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?`,
		transactionID.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete splits: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to check rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// CountByTransaction returns the number of splits for a transaction.
func (r *SplitRepository) CountByTransaction(transactionID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?
	`, transactionID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count splits: %w", err)
	}
	return count, nil
}

// CountByCategory returns the number of splits for a category.
func (r *SplitRepository) CountByCategory(categoryID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transaction_splits WHERE CAST(category_id AS VARCHAR) = ?
	`, categoryID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count splits: %w", err)
	}
	return count, nil
}

// GetTotalByTransaction returns the sum of all split amounts for a transaction.
func (r *SplitRepository) GetTotalByTransaction(transactionID models.ID) (models.Money, error) {
	var totalStr sql.NullString
	err := r.db.Conn().QueryRow(`
		SELECT CAST(SUM(amount) AS VARCHAR) FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?
	`, transactionID.String()).Scan(&totalStr)
	if err != nil {
		return models.ZeroMoney, fmt.Errorf("failed to sum splits: %w", err)
	}

	if !totalStr.Valid || totalStr.String == "" {
		return models.ZeroMoney, nil
	}

	total, err := models.NewMoney(totalStr.String)
	if err != nil {
		return models.ZeroMoney, fmt.Errorf("failed to parse split total: %w", err)
	}

	return total, nil
}

// ValidateSplitsAgainstTransaction validates that the splits for a transaction
// sum to the transaction amount.
func (r *SplitRepository) ValidateSplitsAgainstTransaction(transactionID models.ID) (bool, error) {
	// Get the transaction amount
	var txnAmountStr string
	err := r.db.Conn().QueryRow(`
		SELECT CAST(amount AS VARCHAR) FROM transactions WHERE CAST(id AS VARCHAR) = ?
	`, transactionID.String()).Scan(&txnAmountStr)
	if err == sql.ErrNoRows {
		return false, &NotFoundError{Entity: "transaction", ID: transactionID.String()}
	}
	if err != nil {
		return false, fmt.Errorf("failed to get transaction amount: %w", err)
	}

	txnAmount, err := models.NewMoney(txnAmountStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse transaction amount: %w", err)
	}

	// Get the split total
	splitTotal, err := r.GetTotalByTransaction(transactionID)
	if err != nil {
		return false, err
	}

	// Compare
	return txnAmount.Equal(splitTotal), nil
}

// querySplitsWithArgs executes a query with arguments and returns a slice of splits.
func (r *SplitRepository) querySplitsWithArgs(query string, args ...interface{}) ([]*models.Split, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query splits: %w", err)
	}
	defer rows.Close()

	return r.scanSplits(rows)
}

// scanSplits scans rows into a slice of splits.
func (r *SplitRepository) scanSplits(rows *sql.Rows) ([]*models.Split, error) {
	var splits []*models.Split
	for rows.Next() {
		split := &models.Split{}
		err := rows.Scan(
			&split.ID,
			&split.TransactionID,
			&split.CategoryID,
			&split.Amount,
			&split.Memo,
			&split.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan split: %w", err)
		}
		// Set UpdatedAt to CreatedAt since splits don't have updated_at in the schema
		split.UpdatedAt = split.CreatedAt
		splits = append(splits, split)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating splits: %w", err)
	}

	return splits, nil
}
