package security

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Filter defines optional filters for listing securities.
type Filter struct {
	SecurityType *Type
	AssetClass   *AssetClass
	ExcludeHidden *bool
}

// Repository provides database operations for securities.
type Repository struct {
	db *db.DB
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// Create inserts a new security into the database.
func (r *Repository) Create(security *Security) error {
	// Check for duplicate ticker+currency
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM securities WHERE ticker = ? AND currency = ?)`,
		security.Ticker, security.Currency,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check security uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "security", Field: "ticker+currency", Value: security.Ticker + "+" + security.Currency}
	}

	query := `
		INSERT INTO securities (
			id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		security.ID,
		security.Ticker,
		security.Name,
		security.SecurityType,
		security.AssetClass,
		security.Currency,
		dbutil.NullString(security.Exchange),
		security.Hidden,
		security.CreatedAt,
		security.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create security: %w", err)
	}

	return nil
}

// GetByID retrieves a security by its ID.
func (r *Repository) GetByID(id types.ID) (*Security, error) {
	query := `
		SELECT id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		FROM securities
		WHERE CAST(id AS VARCHAR) = ?
	`

	sec := &Security{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&sec.ID,
		&sec.Ticker,
		&sec.Name,
		&sec.SecurityType,
		&sec.AssetClass,
		&sec.Currency,
		&sec.Exchange,
		&sec.Hidden,
		&sec.CreatedAt,
		&sec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security: %w", err)
	}

	return sec, nil
}

// GetByTicker retrieves a security by its ticker symbol.
// If currency is empty, returns the first match. If currency is specified,
// filters by both ticker and currency.
func (r *Repository) GetByTicker(ticker string, currency string) (*Security, error) {
	var query string
	var args []any

	if currency == "" {
		query = `
			SELECT id, ticker, name, security_type, asset_class, currency,
				exchange, hidden, created_at, updated_at
			FROM securities
			WHERE ticker = ?
		`
		args = []any{ticker}
	} else {
		query = `
			SELECT id, ticker, name, security_type, asset_class, currency,
				exchange, hidden, created_at, updated_at
			FROM securities
			WHERE ticker = ? AND currency = ?
		`
		args = []any{ticker, currency}
	}

	sec := &Security{}
	err := r.db.Conn().QueryRow(query, args...).Scan(
		&sec.ID,
		&sec.Ticker,
		&sec.Name,
		&sec.SecurityType,
		&sec.AssetClass,
		&sec.Currency,
		&sec.Exchange,
		&sec.Hidden,
		&sec.CreatedAt,
		&sec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security", ID: ticker}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security by ticker: %w", err)
	}

	return sec, nil
}

// List retrieves securities with optional filters.
func (r *Repository) List(filter Filter) ([]*Security, error) {
	query := `
		SELECT id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		FROM securities
		WHERE 1=1
	`
	var args []any

	if filter.SecurityType != nil {
		query += " AND security_type = ?"
		args = append(args, filter.SecurityType.String())
	}
	if filter.AssetClass != nil {
		query += " AND asset_class = ?"
		args = append(args, filter.AssetClass.String())
	}
	if filter.ExcludeHidden != nil && *filter.ExcludeHidden {
		query += " AND hidden = FALSE"
	}

	query += " ORDER BY ticker"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list securities: %w", err)
	}
	defer rows.Close()

	securities := make([]*Security, 0)
	for rows.Next() {
		sec := &Security{}
		err := rows.Scan(
			&sec.ID,
			&sec.Ticker,
			&sec.Name,
			&sec.SecurityType,
			&sec.AssetClass,
			&sec.Currency,
			&sec.Exchange,
			&sec.Hidden,
			&sec.CreatedAt,
			&sec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan security: %w", err)
		}
		securities = append(securities, sec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating securities: %w", err)
	}

	return securities, nil
}

// Update updates an existing security in the database.
// Uses DELETE + INSERT pattern due to DuckDB limitations with UPDATE on indexed tables.
func (r *Repository) Update(security *Security) error {
	security.Touch()

	// Check for duplicate ticker+currency (excluding current security)
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM securities WHERE ticker = ? AND currency = ? AND CAST(id AS VARCHAR) != ?)`,
		security.Ticker, security.Currency, security.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check security uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "security", Field: "ticker+currency", Value: security.Ticker + "+" + security.Currency}
	}

	// Check if security exists
	var count int
	err = r.db.Conn().QueryRow(`SELECT COUNT(*) FROM securities WHERE CAST(id AS VARCHAR) = ?`, security.ID.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check security exists: %w", err)
	}
	if count == 0 {
		return &dberrors.NotFoundError{Entity: "security", ID: security.ID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(`DELETE FROM securities WHERE CAST(id AS VARCHAR) = ?`, security.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO securities (
			id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		) VALUES (CAST(? AS UUID), ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		security.ID.String(),
		security.Ticker,
		security.Name,
		security.SecurityType.String(),
		security.AssetClass.String(),
		security.Currency,
		dbutil.NullString(security.Exchange),
		security.Hidden,
		security.CreatedAt.Time(),
		security.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a security from the database.
// Fails if the security has dependent prices or transactions.
func (r *Repository) Delete(id types.ID) error {
	// Check for price dependents
	var priceCount int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM security_prices WHERE CAST(security_id AS VARCHAR) = ?
	`, id.String()).Scan(&priceCount)
	if err != nil {
		return fmt.Errorf("failed to check security prices: %w", err)
	}
	if priceCount > 0 {
		return &dberrors.HasDependentsError{
			Entity:     "security",
			ID:         id.String(),
			Dependents: "prices",
			Count:      priceCount,
		}
	}

	result, err := r.db.Conn().Exec(`DELETE FROM securities WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete security: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "security", ID: id.String()}
	}

	return nil
}
