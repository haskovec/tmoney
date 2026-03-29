package price

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// BulkImportResult holds the results of a bulk price import.
type BulkImportResult struct {
	Total    int
	Imported int
	Skipped  int
}

// Repository provides database operations for security prices.
type Repository struct {
	db *db.DB
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// Create inserts a new security price into the database.
func (r *Repository) Create(price *Price) error {
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
		return &dberrors.DuplicateError{Entity: "security_price", Field: "security_id+date", Value: price.SecurityID.String() + "+" + price.Date.String()}
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
func (r *Repository) CreateOrUpdate(price *Price) error {
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
func (r *Repository) GetBySecurityAndDate(securityID types.ID, date types.Date) (*Price, error) {
	query := `
		SELECT id, security_id, date, price, source, created_at
		FROM security_prices
		WHERE CAST(security_id AS VARCHAR) = ? AND date = ?
	`

	p := &Price{}
	err := r.db.Conn().QueryRow(query, securityID.String(), date.Time()).Scan(
		&p.ID,
		&p.SecurityID,
		&p.Date,
		&p.Price,
		&p.Source,
		&p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security_price", ID: securityID.String() + "@" + date.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security price: %w", err)
	}

	return p, nil
}

// GetCurrentPrice retrieves the most recent price on or before the given date.
func (r *Repository) GetCurrentPrice(securityID types.ID, asOf types.Date) (*Price, error) {
	query := `
		SELECT id, security_id, date, price, source, created_at
		FROM security_prices
		WHERE CAST(security_id AS VARCHAR) = ? AND date <= ?
		ORDER BY date DESC
		LIMIT 1
	`

	p := &Price{}
	err := r.db.Conn().QueryRow(query, securityID.String(), asOf.Time()).Scan(
		&p.ID,
		&p.SecurityID,
		&p.Date,
		&p.Price,
		&p.Source,
		&p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security_price", ID: securityID.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get current price: %w", err)
	}

	return p, nil
}

// GetPriceHistory retrieves all prices for a security ordered by date descending.
// Optional from/to dates filter the range (inclusive).
func (r *Repository) GetPriceHistory(securityID types.ID, from *types.Date, to *types.Date) ([]*Price, error) {
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

	prices := make([]*Price, 0)
	for rows.Next() {
		p := &Price{}
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
func (r *Repository) Delete(id types.ID) error {
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
		return &dberrors.NotFoundError{Entity: "security_price", ID: id.String()}
	}

	return nil
}

// BulkCreate inserts multiple prices. When overwrite is false, existing prices
// (same security_id+date) are skipped. When overwrite is true, existing prices
// are replaced.
func (r *Repository) BulkCreate(prices []*Price, overwrite bool) (*BulkImportResult, error) {
	result := &BulkImportResult{
		Total: len(prices),
	}

	for _, p := range prices {
		if overwrite {
			err := r.CreateOrUpdate(p)
			if err != nil {
				return nil, fmt.Errorf("failed to upsert price: %w", err)
			}
			result.Imported++
		} else {
			err := r.Create(p)
			if err != nil {
				if _, ok := err.(*dberrors.DuplicateError); ok {
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
