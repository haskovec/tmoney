package reconciliation

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Repository provides database operations for reconciliation sessions.
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

// Create inserts a new reconciliation session into the database.
func (r *Repository) Create(session *Session) error {
	// Verify account exists
	var accountExists bool
	err := r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		session.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &dberrors.NotFoundError{Entity: "account", ID: session.AccountID.String()}
	}

	query := `
		INSERT INTO reconciliation_sessions (
			id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.q().Exec(query,
		session.ID,
		session.AccountID,
		session.StatementDate,
		session.StatementBalance,
		session.Status,
		dbutil.NullTimestamp(session.CompletedAt),
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create reconciliation session: %w", err)
	}

	return nil
}

// GetByID retrieves a reconciliation session by its ID.
func (r *Repository) GetByID(id types.ID) (*Session, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(id AS VARCHAR) = ?
	`

	session := &Session{}
	err := r.q().QueryRow(query, id.String()).Scan(
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
		return nil, &dberrors.NotFoundError{Entity: "reconciliation session", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reconciliation session: %w", err)
	}

	return session, nil
}

// GetActiveByAccountID retrieves the in-progress reconciliation session for an account.
// Returns nil, nil if no active session exists.
func (r *Repository) GetActiveByAccountID(accountID types.ID) (*Session, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ? AND status = 'in_progress'
	`

	session := &Session{}
	err := r.q().QueryRow(query, accountID.String()).Scan(
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
func (r *Repository) GetLastCompletedByAccountID(accountID types.ID) (*Session, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ? AND status = 'completed'
		ORDER BY completed_at DESC
		LIMIT 1
	`

	session := &Session{}
	err := r.q().QueryRow(query, accountID.String()).Scan(
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
func (r *Repository) ListByAccountID(accountID types.ID) ([]*Session, error) {
	query := `
		SELECT id, account_id, statement_date, statement_balance,
			status, completed_at, created_at, updated_at
		FROM reconciliation_sessions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY created_at DESC
	`

	rows, err := r.q().Query(query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list reconciliation sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
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

// Update rewrites an entire reconciliation-session row.
//
// For a status change (completing or reopening a session) call UpdateStatus
// instead: this full-row UPDATE rewrites indexed columns (account_id, status),
// so DuckDB runs it as an internal DELETE+INSERT that aborts if a secondary ART
// index for the row is desynced on disk — the storage bug that broke
// reconcile-finish. Update remains for a hypothetical full-field edit; no
// production path uses it today.
func (r *Repository) Update(session *Session) error {
	session.Touch()

	result, err := r.q().Exec(`
		UPDATE reconciliation_sessions SET
			account_id = CAST(? AS UUID),
			statement_date = ?,
			statement_balance = ?,
			status = ?,
			completed_at = ?,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		session.AccountID.String(),
		session.StatementDate.Time(),
		session.StatementBalance.String(),
		session.Status.String(),
		dbutil.NullTimestamp(session.CompletedAt),
		session.UpdatedAt.Time(),
		session.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update reconciliation session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "reconciliation session", ID: session.ID.String()}
	}

	return nil
}

// UpdateStatus updates only the status, completed_at, and updated_at of a
// session, in place. It is the safe path for completing and reopening a session.
//
// Like transaction.Repository.UpdateStatus, it avoids the full-row rewrite that
// DuckDB turns into an internal DELETE+INSERT (account_id and status are
// indexed), which aborts if a secondary ART index for the row is desynced on
// disk — the DuckDB storage bug that broke reconcile-finish. Migration 030
// dropped the status index and account_id is left out of the SET, so this
// narrow UPDATE touches no index: it is a genuine in-place update, immune to a
// desynced index on any column.
func (r *Repository) UpdateStatus(id types.ID, status SessionStatus, completedAt types.NullableTimestamp) error {
	result, err := r.q().Exec(`
		UPDATE reconciliation_sessions SET status = ?, completed_at = ?, updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		status.String(),
		dbutil.NullTimestamp(completedAt),
		types.Now().Time(),
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update reconciliation session status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "reconciliation session", ID: id.String()}
	}
	return nil
}

// Delete removes a reconciliation session from the database.
func (r *Repository) Delete(id types.ID) error {
	result, err := r.q().Exec(
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
		return &dberrors.NotFoundError{Entity: "reconciliation session", ID: id.String()}
	}

	return nil
}

// DeleteByAccountID removes all reconciliation sessions for an account.
func (r *Repository) DeleteByAccountID(accountID types.ID) (int64, error) {
	result, err := r.q().Exec(
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
