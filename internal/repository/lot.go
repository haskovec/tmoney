package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// LotRepository provides database operations for investment lots.
type LotRepository struct {
	db *db.DB
}

// NewLotRepository creates a new LotRepository.
func NewLotRepository(database *db.DB) *LotRepository {
	return &LotRepository{db: database}
}

// lotColumns is the standard column list for investment lots.
const lotColumns = `id, account_id, security_id, shares, original_shares, cost_per_share,
	purchase_date, source_transaction_id, closed, created_at, updated_at`

// scanLot scans a row into a Lot.
func scanLot(row interface{ Scan(...any) error }) (*models.Lot, error) {
	l := &models.Lot{}
	err := row.Scan(
		&l.ID,
		&l.AccountID,
		&l.SecurityID,
		&l.Shares,
		&l.OriginalShares,
		&l.CostPerShare,
		&l.PurchaseDate,
		&l.SourceTransactionID,
		&l.Closed,
		&l.CreatedAt,
		&l.UpdatedAt,
	)
	return l, err
}

// Create inserts a new lot into the database.
func (r *LotRepository) Create(lot *models.Lot) error {
	query := `
		INSERT INTO investment_lots (
			id, account_id, security_id, shares, original_shares, cost_per_share,
			purchase_date, source_transaction_id, closed, created_at, updated_at
		) VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, CAST(? AS UUID), ?, ?, ?)
	`

	_, err := r.db.Conn().Exec(query,
		lot.ID,
		lot.AccountID.String(),
		lot.SecurityID.String(),
		lot.Shares.String(),
		lot.OriginalShares.String(),
		lot.CostPerShare.String(),
		lot.PurchaseDate.Time(),
		lot.SourceTransactionID.String(),
		lot.Closed,
		lot.CreatedAt,
		lot.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create lot: %w", err)
	}

	return nil
}

// GetByID retrieves a lot by its ID.
func (r *LotRepository) GetByID(id models.ID) (*models.Lot, error) {
	query := `
		SELECT ` + lotColumns + `
		FROM investment_lots
		WHERE CAST(id AS VARCHAR) = ?
	`

	l, err := scanLot(r.db.Conn().QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "lot", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lot: %w", err)
	}

	return l, nil
}

// ListByAccountAndSecurity retrieves lots for a specific account and security.
// By default only open (non-closed) lots are returned, ordered by purchase_date ASC.
// Set includeClosed to true to include closed lots.
func (r *LotRepository) ListByAccountAndSecurity(accountID, securityID models.ID, includeClosed bool) ([]*models.Lot, error) {
	query := `
		SELECT ` + lotColumns + `
		FROM investment_lots
		WHERE CAST(account_id AS VARCHAR) = ?
		  AND CAST(security_id AS VARCHAR) = ?
	`
	args := []any{accountID.String(), securityID.String()}

	if !includeClosed {
		query += " AND closed = false"
	}

	query += " ORDER BY purchase_date ASC"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list lots: %w", err)
	}
	defer rows.Close()

	lots := make([]*models.Lot, 0)
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lot: %w", err)
		}
		lots = append(lots, l)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lots: %w", err)
	}

	return lots, nil
}

// Update updates an existing lot in the database.
// Uses DELETE + INSERT pattern due to DuckDB limitations with UPDATE on indexed tables.
func (r *LotRepository) Update(lot *models.Lot) error {
	lot.Touch()

	// Check if lot exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM investment_lots WHERE CAST(id AS VARCHAR) = ?`,
		lot.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check lot exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "lot", ID: lot.ID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM investment_lots WHERE CAST(id AS VARCHAR) = ?`,
		lot.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	query := `
		INSERT INTO investment_lots (
			id, account_id, security_id, shares, original_shares, cost_per_share,
			purchase_date, source_transaction_id, closed, created_at, updated_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, CAST(? AS UUID), ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		lot.ID.String(),
		lot.AccountID.String(),
		lot.SecurityID.String(),
		lot.Shares.String(),
		lot.OriginalShares.String(),
		lot.CostPerShare.String(),
		lot.PurchaseDate.Time(),
		lot.SourceTransactionID.String(),
		lot.Closed,
		lot.CreatedAt.Time(),
		lot.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// GetOpenLotsBySecurity returns all open (non-closed) lots across all accounts for a given security.
// This is needed for security hide validation — a security cannot be hidden if open lots exist.
func (r *LotRepository) GetOpenLotsBySecurity(securityID models.ID) ([]*models.Lot, error) {
	query := `
		SELECT ` + lotColumns + `
		FROM investment_lots
		WHERE CAST(security_id AS VARCHAR) = ?
		  AND closed = false
		ORDER BY purchase_date ASC
	`

	rows, err := r.db.Conn().Query(query, securityID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get open lots by security: %w", err)
	}
	defer rows.Close()

	lots := make([]*models.Lot, 0)
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lot: %w", err)
		}
		lots = append(lots, l)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lots: %w", err)
	}

	return lots, nil
}
