package transaction

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// SplitRepository provides database operations for transaction splits.
type SplitRepository struct {
	db *db.DB
}

// NewSplitRepository creates a new SplitRepository.
func NewSplitRepository(database *db.DB) *SplitRepository {
	return &SplitRepository{db: database}
}

// splitColumns lists every column the repository reads/writes, in scan order.
const splitColumns = `id, transaction_id, category_id, transfer_account_id,
	transfer_id, amount, memo, created_at`

// categoryArg returns the value to write for category_id: NULL for transfer-
// lines (CategoryID.IsNil()), otherwise the UUID string.
func categoryArg(split *Split) any {
	if split.CategoryID.IsNil() {
		return nil
	}
	return split.CategoryID
}

// verifyReferences confirms that the FK targets named on the split exist.
// Transfer-lines must point at an existing account; categorized lines must
// point at an existing category.
func (r *SplitRepository) verifyReferences(split *Split) error {
	if split.TransferAccountID.Valid {
		var exists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			split.TransferAccountID.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !exists {
			return &dberrors.NotFoundError{Entity: "account", ID: split.TransferAccountID.ID.String()}
		}
		return nil
	}

	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
		split.CategoryID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check category exists: %w", err)
	}
	if !exists {
		return &dberrors.NotFoundError{Entity: "category", ID: split.CategoryID.String()}
	}
	return nil
}

// Create inserts a new split into the database.
func (r *SplitRepository) Create(split *Split) error {
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
		return &dberrors.NotFoundError{Entity: "transaction", ID: split.TransactionID.String()}
	}

	if err := r.verifyReferences(split); err != nil {
		return err
	}

	query := `
		INSERT INTO transaction_splits (
			id, transaction_id, category_id, transfer_account_id, transfer_id,
			amount, memo, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		split.ID,
		split.TransactionID,
		categoryArg(split),
		dbutil.NullID(split.TransferAccountID),
		dbutil.NullID(split.TransferID),
		split.Amount,
		dbutil.NullString(split.Memo),
		split.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create split: %w", err)
	}

	return nil
}

// GetByID retrieves a split by its ID.
func (r *SplitRepository) GetByID(id types.ID) (*Split, error) {
	query := `
		SELECT ` + splitColumns + `
		FROM transaction_splits
		WHERE CAST(id AS VARCHAR) = ?
	`

	rows, err := r.db.Conn().Query(query, id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get split: %w", err)
	}
	defer rows.Close()

	splits, err := r.scanSplits(rows)
	if err != nil {
		return nil, err
	}
	if len(splits) == 0 {
		return nil, &dberrors.NotFoundError{Entity: "split", ID: id.String()}
	}

	return splits[0], nil
}

// GetByTransferID retrieves the split-item linked to a paired counter-
// transaction's transfer_id, or nil if none exists. Multi-line transfer-
// lines mint a fresh transfer_id per line, so at most one split has any
// given transfer_id.
func (r *SplitRepository) GetByTransferID(transferID types.ID) (*Split, error) {
	query := `
		SELECT ` + splitColumns + `
		FROM transaction_splits
		WHERE CAST(transfer_id AS VARCHAR) = ?
	`

	rows, err := r.db.Conn().Query(query, transferID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query split by transfer_id: %w", err)
	}
	defer rows.Close()

	splits, err := r.scanSplits(rows)
	if err != nil {
		return nil, err
	}
	if len(splits) == 0 {
		return nil, nil
	}
	return splits[0], nil
}

// ListByTransaction retrieves all splits for a transaction.
func (r *SplitRepository) ListByTransaction(transactionID types.ID) ([]*Split, error) {
	query := `
		SELECT ` + splitColumns + `
		FROM transaction_splits
		WHERE CAST(transaction_id AS VARCHAR) = ?
		ORDER BY created_at
	`

	return r.querySplitsWithArgs(query, transactionID.String())
}

// Update updates an existing split in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *SplitRepository) Update(split *Split) error {
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
		return &dberrors.NotFoundError{Entity: "split", ID: split.ID.String()}
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
		return &dberrors.NotFoundError{Entity: "transaction", ID: split.TransactionID.String()}
	}

	if err := r.verifyReferences(split); err != nil {
		return err
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM transaction_splits WHERE CAST(id AS VARCHAR) = ?`,
		split.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record. category_id, transfer_account_id, and
	// transfer_id are written as either CAST(? AS UUID) or plain NULL so
	// DuckDB accepts NULLs without a type assertion on the bind value.
	catCast := "CAST(? AS UUID)"
	if split.CategoryID.IsNil() {
		catCast = "?"
	}
	xferAcctCast := dbutil.NullUUIDCast(split.TransferAccountID)
	xferIDCast := dbutil.NullUUIDCast(split.TransferID)

	insertQuery := fmt.Sprintf(`
		INSERT INTO transaction_splits (
			id, transaction_id, category_id, transfer_account_id, transfer_id,
			amount, memo, created_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), %s, %s, %s, ?, ?, ?)
	`, catCast, xferAcctCast, xferIDCast)

	_, err = r.db.Conn().Exec(insertQuery,
		split.ID.String(),
		split.TransactionID.String(),
		categoryStringArg(split),
		dbutil.NullID(split.TransferAccountID),
		dbutil.NullID(split.TransferID),
		split.Amount.String(),
		dbutil.NullString(split.Memo),
		split.CreatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// categoryStringArg mirrors categoryArg but emits the UUID as a string,
// matching the CAST(? AS UUID) bind shape used by Update.
func categoryStringArg(split *Split) any {
	if split.CategoryID.IsNil() {
		return nil
	}
	return split.CategoryID.String()
}

// Delete removes a split from the database.
func (r *SplitRepository) Delete(id types.ID) error {
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
		return &dberrors.NotFoundError{Entity: "split", ID: id.String()}
	}

	return nil
}

// DeleteByTransaction removes all splits for a transaction.
func (r *SplitRepository) DeleteByTransaction(transactionID types.ID) (int, error) {
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
func (r *SplitRepository) CountByTransaction(transactionID types.ID) (int, error) {
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
func (r *SplitRepository) CountByCategory(categoryID types.ID) (int, error) {
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
func (r *SplitRepository) GetTotalByTransaction(transactionID types.ID) (types.Money, error) {
	var totalStr sql.NullString
	err := r.db.Conn().QueryRow(`
		SELECT CAST(SUM(amount) AS VARCHAR) FROM transaction_splits WHERE CAST(transaction_id AS VARCHAR) = ?
	`, transactionID.String()).Scan(&totalStr)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to sum splits: %w", err)
	}

	if !totalStr.Valid || totalStr.String == "" {
		return types.ZeroMoney, nil
	}

	total, err := types.NewMoney(totalStr.String)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to parse split total: %w", err)
	}

	return total, nil
}

// ValidateSplitsAgainstTransaction validates that the splits for a transaction
// sum to the transaction amount.
func (r *SplitRepository) ValidateSplitsAgainstTransaction(transactionID types.ID) (bool, error) {
	// Get the transaction amount
	var txnAmountStr string
	err := r.db.Conn().QueryRow(`
		SELECT CAST(amount AS VARCHAR) FROM transactions WHERE CAST(id AS VARCHAR) = ?
	`, transactionID.String()).Scan(&txnAmountStr)
	if err == sql.ErrNoRows {
		return false, &dberrors.NotFoundError{Entity: "transaction", ID: transactionID.String()}
	}
	if err != nil {
		return false, fmt.Errorf("failed to get transaction amount: %w", err)
	}

	txnAmount, err := types.NewMoney(txnAmountStr)
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
func (r *SplitRepository) querySplitsWithArgs(query string, args ...any) ([]*Split, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query splits: %w", err)
	}
	defer rows.Close()

	return r.scanSplits(rows)
}

// scanSplits scans rows into a slice of splits.
func (r *SplitRepository) scanSplits(rows *sql.Rows) ([]*Split, error) {
	var splits []*Split
	for rows.Next() {
		split := &Split{}
		err := rows.Scan(
			&split.ID,
			&split.TransactionID,
			&split.CategoryID,
			&split.TransferAccountID,
			&split.TransferID,
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
