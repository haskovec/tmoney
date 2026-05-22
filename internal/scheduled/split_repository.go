package scheduled

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// SplitRepository provides database operations for scheduled-split items.
type SplitRepository struct {
	db *db.DB
}

// NewSplitRepository creates a new SplitRepository.
func NewSplitRepository(database *db.DB) *SplitRepository {
	return &SplitRepository{db: database}
}

// splitColumns lists every column the repository reads/writes, in scan order.
const splitColumns = `id, scheduled_transaction_id, category_id, transfer_account_id,
	amount, memo, paycheck_section, created_at`

// verifyReferences confirms that the FK targets named on the split exist.
// Every split must point at an existing scheduled_transaction. Transfer-lines
// additionally must point at an existing account; categorized lines must
// point at an existing category. The exclusive shape is enforced by the
// CHECK constraint in migration 015.
func (r *SplitRepository) verifyReferences(split *Split) error {
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM scheduled_transactions WHERE CAST(id AS VARCHAR) = ?)`,
		split.ScheduledTransactionID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check scheduled transaction exists: %w", err)
	}
	if !exists {
		return &dberrors.NotFoundError{
			Entity: "scheduled_transaction",
			ID:     split.ScheduledTransactionID.String(),
		}
	}

	if split.TransferAccountID.Valid {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			split.TransferAccountID.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !exists {
			return &dberrors.NotFoundError{
				Entity: "account",
				ID:     split.TransferAccountID.ID.String(),
			}
		}
		return nil
	}

	if split.CategoryID.Valid {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			split.CategoryID.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !exists {
			return &dberrors.NotFoundError{
				Entity: "category",
				ID:     split.CategoryID.ID.String(),
			}
		}
	}
	return nil
}

// Create inserts a new scheduled split into the database.
func (r *SplitRepository) Create(split *Split) error {
	if err := r.verifyReferences(split); err != nil {
		return err
	}

	query := `
		INSERT INTO scheduled_split_items (
			id, scheduled_transaction_id, category_id, transfer_account_id,
			amount, memo, paycheck_section, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Conn().Exec(query,
		split.ID,
		split.ScheduledTransactionID,
		dbutil.NullID(split.CategoryID),
		dbutil.NullID(split.TransferAccountID),
		split.Amount,
		dbutil.NullString(split.Memo),
		dbutil.NullString(split.PaycheckSection),
		split.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create scheduled split: %w", err)
	}

	return nil
}

// GetByID retrieves a scheduled split by its ID.
func (r *SplitRepository) GetByID(id types.ID) (*Split, error) {
	query := `
		SELECT ` + splitColumns + `
		FROM scheduled_split_items
		WHERE CAST(id AS VARCHAR) = ?
	`

	rows, err := r.db.Conn().Query(query, id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled split: %w", err)
	}
	defer rows.Close()

	splits, err := r.scanSplits(rows)
	if err != nil {
		return nil, err
	}
	if len(splits) == 0 {
		return nil, &dberrors.NotFoundError{Entity: "scheduled_split", ID: id.String()}
	}

	return splits[0], nil
}

// ListByScheduledTransaction retrieves all splits for a scheduled transaction
// ordered by created_at ascending.
func (r *SplitRepository) ListByScheduledTransaction(scheduledTransactionID types.ID) ([]*Split, error) {
	query := `
		SELECT ` + splitColumns + `
		FROM scheduled_split_items
		WHERE CAST(scheduled_transaction_id AS VARCHAR) = ?
		ORDER BY created_at
	`

	return r.querySplitsWithArgs(query, scheduledTransactionID.String())
}

// Update updates an existing scheduled split in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *SplitRepository) Update(split *Split) error {
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM scheduled_split_items WHERE CAST(id AS VARCHAR) = ?`,
		split.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check scheduled split exists: %w", err)
	}
	if count == 0 {
		return &dberrors.NotFoundError{Entity: "scheduled_split", ID: split.ID.String()}
	}

	if err := r.verifyReferences(split); err != nil {
		return err
	}

	_, err = r.db.Conn().Exec(
		`DELETE FROM scheduled_split_items WHERE CAST(id AS VARCHAR) = ?`,
		split.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	catCast := dbutil.NullUUIDCast(split.CategoryID)
	xferAcctCast := dbutil.NullUUIDCast(split.TransferAccountID)

	insertQuery := fmt.Sprintf(`
		INSERT INTO scheduled_split_items (
			id, scheduled_transaction_id, category_id, transfer_account_id,
			amount, memo, paycheck_section, created_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), %s, %s, ?, ?, ?, ?)
	`, catCast, xferAcctCast)

	_, err = r.db.Conn().Exec(insertQuery,
		split.ID.String(),
		split.ScheduledTransactionID.String(),
		dbutil.NullID(split.CategoryID),
		dbutil.NullID(split.TransferAccountID),
		split.Amount.String(),
		dbutil.NullString(split.Memo),
		dbutil.NullString(split.PaycheckSection),
		split.CreatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a scheduled split from the database.
func (r *SplitRepository) Delete(id types.ID) error {
	result, err := r.db.Conn().Exec(
		`DELETE FROM scheduled_split_items WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled split: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "scheduled_split", ID: id.String()}
	}

	return nil
}

// DeleteByScheduledTransaction removes all splits for a scheduled transaction.
func (r *SplitRepository) DeleteByScheduledTransaction(scheduledTransactionID types.ID) (int, error) {
	result, err := r.db.Conn().Exec(
		`DELETE FROM scheduled_split_items WHERE CAST(scheduled_transaction_id AS VARCHAR) = ?`,
		scheduledTransactionID.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete scheduled splits: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to check rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// CountByScheduledTransaction returns the number of splits for a scheduled transaction.
func (r *SplitRepository) CountByScheduledTransaction(scheduledTransactionID types.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM scheduled_split_items WHERE CAST(scheduled_transaction_id AS VARCHAR) = ?
	`, scheduledTransactionID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled splits: %w", err)
	}
	return count, nil
}

// querySplitsWithArgs executes a query with arguments and returns a slice of splits.
func (r *SplitRepository) querySplitsWithArgs(query string, args ...any) ([]*Split, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled splits: %w", err)
	}
	defer rows.Close()

	return r.scanSplits(rows)
}

// scanSplits scans rows into a slice of scheduled splits.
func (r *SplitRepository) scanSplits(rows *sql.Rows) ([]*Split, error) {
	var splits []*Split
	for rows.Next() {
		split := &Split{}
		err := rows.Scan(
			&split.ID,
			&split.ScheduledTransactionID,
			&split.CategoryID,
			&split.TransferAccountID,
			&split.Amount,
			&split.Memo,
			&split.PaycheckSection,
			&split.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scheduled split: %w", err)
		}
		// scheduled_split_items has no updated_at; mirror created_at for symmetry.
		split.UpdatedAt = split.CreatedAt
		splits = append(splits, split)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scheduled splits: %w", err)
	}

	return splits, nil
}
