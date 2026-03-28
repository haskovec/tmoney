package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// BulkImportResult holds the results of a bulk price import.
type BulkImportResult struct {
	Total    int
	Imported int
	Skipped  int
}

// PriceRepository provides database operations for security prices.
type PriceRepository struct {
	db *db.DB
}

// NewPriceRepository creates a new PriceRepository.
func NewPriceRepository(database *db.DB) *PriceRepository {
	return &PriceRepository{db: database}
}

// Create inserts a new security price into the database.
func (r *PriceRepository) Create(price *models.SecurityPrice) error {
	// Check for duplicate security_id+date
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM security_prices WHERE CAST(security_id AS VARCHAR) = ? AND date = ?)`,
		price.SecurityID.String(), price.Date.Time(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check price uniqueness: %w", err)
	}
	if exists {
		return &DuplicateError{Entity: "security_price", Field: "security_id+date", Value: price.SecurityID.String() + "+" + price.Date.String()}
	}

	query := `
		INSERT INTO security_prices (id, security_id, date, price, source, created_at)
		VALUES (?, CAST(? AS UUID), ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		price.ID,
		price.SecurityID.String(),
		price.Date.Time(),
		price.Price.String(),
		price.Source.String(),
		price.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create security price: %w", err)
	}

	return nil
}

// CreateOrUpdate inserts a new price or updates an existing one for the same security+date.
func (r *PriceRepository) CreateOrUpdate(price *models.SecurityPrice) error {
	// Check if a price already exists for this security+date
	var existingID string
	err := r.db.Conn().QueryRow(
		`SELECT CAST(id AS VARCHAR) FROM security_prices WHERE CAST(security_id AS VARCHAR) = ? AND date = ?`,
		price.SecurityID.String(), price.Date.Time(),
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		// No existing price, just create
		return r.Create(price)
	}
	if err != nil {
		return fmt.Errorf("failed to check existing price: %w", err)
	}

	// Delete existing and insert new (DuckDB UPDATE limitation)
	_, err = r.db.Conn().Exec(
		`DELETE FROM security_prices WHERE CAST(id AS VARCHAR) = ?`, existingID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete existing price for upsert: %w", err)
	}

	query := `
		INSERT INTO security_prices (id, security_id, date, price, source, created_at)
		VALUES (?, CAST(? AS UUID), ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		price.ID,
		price.SecurityID.String(),
		price.Date.Time(),
		price.Price.String(),
		price.Source.String(),
		price.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert price for upsert: %w", err)
	}

	return nil
}

// GetBySecurityAndDate retrieves a price for a specific security on a specific date.
func (r *PriceRepository) GetBySecurityAndDate(securityID models.ID, date models.Date) (*models.SecurityPrice, error) {
	query := `
		SELECT id, security_id, date, price, source, created_at
		FROM security_prices
		WHERE CAST(security_id AS VARCHAR) = ? AND date = ?
	`

	p := &models.SecurityPrice{}
	err := r.db.Conn().QueryRow(query, securityID.String(), date.Time()).Scan(
		&p.ID,
		&p.SecurityID,
		&p.Date,
		&p.Price,
		&p.Source,
		&p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "security_price", ID: securityID.String() + "@" + date.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security price: %w", err)
	}

	return p, nil
}

// GetCurrentPrice retrieves the most recent price on or before the given date.
func (r *PriceRepository) GetCurrentPrice(securityID models.ID, asOf models.Date) (*models.SecurityPrice, error) {
	query := `
		SELECT id, security_id, date, price, source, created_at
		FROM security_prices
		WHERE CAST(security_id AS VARCHAR) = ? AND date <= ?
		ORDER BY date DESC
		LIMIT 1
	`

	p := &models.SecurityPrice{}
	err := r.db.Conn().QueryRow(query, securityID.String(), asOf.Time()).Scan(
		&p.ID,
		&p.SecurityID,
		&p.Date,
		&p.Price,
		&p.Source,
		&p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "security_price", ID: securityID.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get current price: %w", err)
	}

	return p, nil
}

// GetPriceHistory retrieves all prices for a security ordered by date descending.
// Optional from/to dates filter the range (inclusive).
func (r *PriceRepository) GetPriceHistory(securityID models.ID, from *models.Date, to *models.Date) ([]*models.SecurityPrice, error) {
	query := `
		SELECT id, security_id, date, price, source, created_at
		FROM security_prices
		WHERE CAST(security_id AS VARCHAR) = ?
	`
	args := []any{securityID.String()}

	if from != nil {
		query += " AND date >= ?"
		args = append(args, from.Time())
	}
	if to != nil {
		query += " AND date <= ?"
		args = append(args, to.Time())
	}

	query += " ORDER BY date DESC"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get price history: %w", err)
	}
	defer rows.Close()

	prices := make([]*models.SecurityPrice, 0)
	for rows.Next() {
		p := &models.SecurityPrice{}
		err := rows.Scan(
			&p.ID,
			&p.SecurityID,
			&p.Date,
			&p.Price,
			&p.Source,
			&p.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan security price: %w", err)
		}
		prices = append(prices, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating security prices: %w", err)
	}

	return prices, nil
}

// Delete removes a security price by its ID.
func (r *PriceRepository) Delete(id models.ID) error {
	result, err := r.db.Conn().Exec(
		`DELETE FROM security_prices WHERE CAST(id AS VARCHAR) = ?`, id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete security price: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &NotFoundError{Entity: "security_price", ID: id.String()}
	}

	return nil
}

// BulkCreate inserts multiple prices. When overwrite is false, existing prices
// (same security_id+date) are skipped. When overwrite is true, existing prices
// are replaced.
func (r *PriceRepository) BulkCreate(prices []*models.SecurityPrice, overwrite bool) (*BulkImportResult, error) {
	result := &BulkImportResult{
		Total: len(prices),
	}

	for _, price := range prices {
		if overwrite {
			err := r.CreateOrUpdate(price)
			if err != nil {
				return nil, fmt.Errorf("failed to upsert price: %w", err)
			}
			result.Imported++
		} else {
			err := r.Create(price)
			if err != nil {
				if _, ok := err.(*DuplicateError); ok {
					result.Skipped++
					continue
				}
				return nil, fmt.Errorf("failed to create price: %w", err)
			}
			result.Imported++
		}
	}

	return result, nil
}
