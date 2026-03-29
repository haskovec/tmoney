package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// PositionRepository provides database operations for investment positions.
type PositionRepository struct {
	db *db.DB
}

// NewPositionRepository creates a new PositionRepository.
func NewPositionRepository(database *db.DB) *PositionRepository {
	return &PositionRepository{db: database}
}

// positionColumns is the standard column list for investment positions.
const positionColumns = `id, account_id, security_id, shares, average_cost_per_share, created_at, updated_at`

// scanPosition scans a row into a Position.
func scanPosition(row interface{ Scan(...any) error }) (*models.Position, error) {
	p := &models.Position{}
	err := row.Scan(
		&p.ID,
		&p.AccountID,
		&p.SecurityID,
		&p.Shares,
		&p.AverageCostPerShare,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	return p, err
}

// CreateOrUpdate inserts a new position or updates the existing one for the same account+security.
// Uses DELETE + INSERT pattern due to DuckDB limitations.
func (r *PositionRepository) CreateOrUpdate(position *models.Position) error {
	position.Touch()

	// Check if a position already exists for this account+security
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ? AND CAST(security_id AS VARCHAR) = ?`,
		position.AccountID.String(), position.SecurityID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing position: %w", err)
	}

	if count > 0 {
		// Delete the existing record
		_, err = r.db.Conn().Exec(
			`DELETE FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ? AND CAST(security_id AS VARCHAR) = ?`,
			position.AccountID.String(), position.SecurityID.String(),
		)
		if err != nil {
			return fmt.Errorf("failed to delete existing position for upsert: %w", err)
		}
	}

	// Insert the record
	query := `
		INSERT INTO investment_positions (
			id, account_id, security_id, shares, average_cost_per_share, created_at, updated_at
		) VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		position.ID,
		position.AccountID.String(),
		position.SecurityID.String(),
		position.Shares.String(),
		position.AverageCostPerShare.String(),
		position.CreatedAt,
		position.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create position: %w", err)
	}

	return nil
}

// GetByAccountAndSecurity retrieves a position for a specific account and security.
// If no position exists, returns a zero-value position (not an error).
func (r *PositionRepository) GetByAccountAndSecurity(accountID, securityID models.ID) (*models.Position, error) {
	query := `
		SELECT ` + positionColumns + `
		FROM investment_positions
		WHERE CAST(account_id AS VARCHAR) = ?
		  AND CAST(security_id AS VARCHAR) = ?
	`

	p, err := scanPosition(r.db.Conn().QueryRow(query, accountID.String(), securityID.String()))
	if err == sql.ErrNoRows {
		// Return zero-value position, not an error
		zeroPos := models.NewPosition(accountID, securityID)
		return &zeroPos, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get position: %w", err)
	}

	return p, nil
}

// ListByAccount retrieves all positions for a given account.
// Set excludeZeroShares to true to exclude positions with zero shares.
func (r *PositionRepository) ListByAccount(accountID models.ID, excludeZeroShares bool) ([]*models.Position, error) {
	query := `
		SELECT ` + positionColumns + `
		FROM investment_positions
		WHERE CAST(account_id AS VARCHAR) = ?
	`
	args := []any{accountID.String()}

	if excludeZeroShares {
		query += " AND shares > 0"
	}

	query += " ORDER BY created_at ASC"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list positions: %w", err)
	}
	defer rows.Close()

	positions := make([]*models.Position, 0)
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}
		positions = append(positions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating positions: %w", err)
	}

	return positions, nil
}

// Delete removes a position for a given account and security.
// Returns NotFoundError if no position exists.
func (r *PositionRepository) Delete(accountID, securityID models.ID) error {
	// Check if position exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ? AND CAST(security_id AS VARCHAR) = ?`,
		accountID.String(), securityID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check position exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "position", ID: accountID.String() + "+" + securityID.String()}
	}

	_, err = r.db.Conn().Exec(
		`DELETE FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ? AND CAST(security_id AS VARCHAR) = ?`,
		accountID.String(), securityID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete position: %w", err)
	}

	return nil
}
