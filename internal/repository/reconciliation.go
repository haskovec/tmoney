package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// ReconciliationRepository provides database operations for reconciliation sessions.
type ReconciliationRepository struct {
	db *db.DB
}

// NewReconciliationRepository creates a new ReconciliationRepository.
func NewReconciliationRepository(database *db.DB) *ReconciliationRepository {
	return &ReconciliationRepository{db: database}
}

// Create inserts a new reconciliation session into the database.
func (r *ReconciliationRepository) Create(session *models.ReconciliationSession) error {
	// Verify account exists
	var accountExists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		session.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &NotFoundError{Entity: "account", ID: session.AccountID.String()}
	}

	query := `
		INSERT INTO reconciliation_sessions (
			id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		session.ID,
		session.AccountID,
		session.StatementDate,
		session.StatementBalance,
		session.Status,
		nullTimestamp(session.CompletedAt),
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create reconciliation session: %w", err)
	}

	return nil
}

// GetByID retrieves a reconciliation session by its ID.
func (r *ReconciliationRepository) GetByID(id models.ID) (*models.ReconciliationSession, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(id AS VARCHAR) = ?
	`

	session := &models.ReconciliationSession{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&session.ID,
		&session.AccountID,
		&session.StatementDate,
		&session.StatementBalance,
		&session.Status,
		&session.CompletedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "reconciliation session", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reconciliation session: %w", err)
	}

	return session, nil
}

// GetActiveByAccountID retrieves the in-progress reconciliation session for an account.
// Returns nil, nil if no active session exists.
func (r *ReconciliationRepository) GetActiveByAccountID(accountID models.ID) (*models.ReconciliationSession, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ? AND status = 'in_progress'
	`

	session := &models.ReconciliationSession{}
	err := r.db.Conn().QueryRow(query, accountID.String()).Scan(
		&session.ID,
		&session.AccountID,
		&session.StatementDate,
		&session.StatementBalance,
		&session.Status,
		&session.CompletedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active reconciliation session: %w", err)
	}

	return session, nil
}

// GetLastCompletedByAccountID retrieves the most recently completed reconciliation session for an account.
// Returns nil, nil if no completed session exists.
func (r *ReconciliationRepository) GetLastCompletedByAccountID(accountID models.ID) (*models.ReconciliationSession, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ? AND status = 'completed'
		ORDER BY completed_at DESC
		LIMIT 1
	`

	session := &models.ReconciliationSession{}
	err := r.db.Conn().QueryRow(query, accountID.String()).Scan(
		&session.ID,
		&session.AccountID,
		&session.StatementDate,
		&session.StatementBalance,
		&session.Status,
		&session.CompletedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get last completed reconciliation session: %w", err)
	}

	return session, nil
}

// ListByAccountID retrieves all reconciliation sessions for an account, ordered by creation date descending.
func (r *ReconciliationRepository) ListByAccountID(accountID models.ID) ([]*models.ReconciliationSession, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Conn().Query(query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list reconciliation sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*models.ReconciliationSession
	for rows.Next() {
		session := &models.ReconciliationSession{}
		err := rows.Scan(
			&session.ID,
			&session.AccountID,
			&session.StatementDate,
			&session.StatementBalance,
			&session.Status,
			&session.CompletedAt,
			&session.CreatedAt,
			&session.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reconciliation session: %w", err)
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reconciliation sessions: %w", err)
	}

	return sessions, nil
}

// Update updates an existing reconciliation session in the database.
// Note: Uses DELETE + INSERT due to DuckDB limitations with UPDATE operations on tables that have indexes.
func (r *ReconciliationRepository) Update(session *models.ReconciliationSession) error {
	session.Touch()

	// Check if session exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM reconciliation_sessions WHERE CAST(id AS VARCHAR) = ?`,
		session.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check reconciliation session exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "reconciliation session", ID: session.ID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM reconciliation_sessions WHERE CAST(id AS VARCHAR) = ?`,
		session.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO reconciliation_sessions (
			id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		) VALUES (CAST(? AS UUID), ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		session.ID.String(),
		session.AccountID.String(),
		session.StatementDate.Time(),
		session.StatementBalance.String(),
		session.Status.String(),
		nullTimestamp(session.CompletedAt),
		session.CreatedAt.Time(),
		session.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a reconciliation session from the database.
func (r *ReconciliationRepository) Delete(id models.ID) error {
	result, err := r.db.Conn().Exec(
		`DELETE FROM reconciliation_sessions WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete reconciliation session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &NotFoundError{Entity: "reconciliation session", ID: id.String()}
	}

	return nil
}

// DeleteByAccountID removes all reconciliation sessions for an account.
func (r *ReconciliationRepository) DeleteByAccountID(accountID models.ID) (int64, error) {
	result, err := r.db.Conn().Exec(
		`DELETE FROM reconciliation_sessions WHERE CAST(account_id AS VARCHAR) = ?`,
		accountID.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete reconciliation sessions by account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to check rows affected: %w", err)
	}

	return rowsAffected, nil
}

// nullTimestamp converts NullableTimestamp to a value for database insertion.
func nullTimestamp(nt models.NullableTimestamp) any {
	if nt.Valid {
		return nt.Timestamp.Time()
	}
	return nil
}
