package security

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Filter defines optional filters for listing securities.
type Filter struct {
	SecurityType  *Type
	AssetClass    *AssetClass
	ExcludeHidden *bool
}

// Repository provides database operations for securities.
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

// securityColumns is the shared SELECT column list (and its Scan order) for a
// full security row. Keep it in lock-step with scanSecurity. isin is read via
// COALESCE because migration 027 adds it as a plain nullable column (DuckDB
// cannot ADD a NOT NULL column), so legacy rows hold NULL — the model always
// sees a plain string.
const securityColumns = `id, ticker, name, COALESCE(isin, '') AS isin, security_type, asset_class, currency,
	exchange, hidden, created_at, updated_at`

// scanSecurity scans a full security row in securityColumns order.
func scanSecurity(scan func(dest ...any) error) (*Security, error) {
	sec := &Security{}
	err := scan(
		&sec.ID,
		&sec.Ticker,
		&sec.Name,
		&sec.ISIN,
		&sec.SecurityType,
		&sec.AssetClass,
		&sec.Currency,
		&sec.Exchange,
		&sec.Hidden,
		&sec.CreatedAt,
		&sec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return sec, nil
}

// Create inserts a new security into the database.
func (r *Repository) Create(security *Security) error {
	if err := r.ensureUnique(security); err != nil {
		return err
	}

	query := `
		INSERT INTO securities (
			id, ticker, name, isin, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.q().Exec(query,
		security.ID,
		security.Ticker,
		security.Name,
		security.ISIN,
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

// ensureUnique enforces the security uniqueness rules, excluding the row with
// the candidate's own ID (so it is safe for both Create and Update):
//
//   - ticker+currency must be unique, but ONLY when a ticker is present. An
//     empty ticker means "no ticker", and many tickerless securities may
//     coexist.
//   - among tickerless securities, name+currency must be unique (a guard rail
//     against entering the same un-tickered fund twice).
//   - isin must be globally unique (case-insensitive) when present.
func (r *Repository) ensureUnique(s *Security) error {
	conn := r.q()
	id := s.ID.String()

	if s.Ticker != "" {
		var exists bool
		if err := conn.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM securities WHERE ticker = ? AND currency = ? AND CAST(id AS VARCHAR) != ?)`,
			s.Ticker, s.Currency, id,
		).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check ticker uniqueness: %w", err)
		}
		if exists {
			return &dberrors.DuplicateError{Entity: "security", Field: "ticker+currency", Value: s.Ticker + "+" + s.Currency}
		}
	} else {
		var exists bool
		if err := conn.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM securities WHERE ticker = '' AND LOWER(name) = LOWER(?) AND currency = ? AND CAST(id AS VARCHAR) != ?)`,
			s.Name, s.Currency, id,
		).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check name uniqueness: %w", err)
		}
		if exists {
			return &dberrors.DuplicateError{Entity: "security", Field: "name+currency", Value: s.Name + "+" + s.Currency}
		}
	}

	if s.ISIN != "" {
		var exists bool
		if err := conn.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM securities WHERE COALESCE(isin, '') != '' AND UPPER(isin) = UPPER(?) AND CAST(id AS VARCHAR) != ?)`,
			s.ISIN, id,
		).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check ISIN uniqueness: %w", err)
		}
		if exists {
			return &dberrors.DuplicateError{Entity: "security", Field: "isin", Value: s.ISIN}
		}
	}

	return nil
}

// GetByID retrieves a security by its ID.
func (r *Repository) GetByID(id types.ID) (*Security, error) {
	query := `SELECT ` + securityColumns + ` FROM securities WHERE CAST(id AS VARCHAR) = ?`

	sec, err := scanSecurity(r.q().QueryRow(query, id.String()).Scan)
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
	query := `SELECT ` + securityColumns + ` FROM securities WHERE ticker = ?`
	args := []any{ticker}
	if currency != "" {
		query += ` AND currency = ?`
		args = append(args, currency)
	}

	sec, err := scanSecurity(r.q().QueryRow(query, args...).Scan)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security", ID: ticker}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security by ticker: %w", err)
	}

	return sec, nil
}

// GetByISIN retrieves a security by its ISIN (case-insensitive). An empty ISIN
// never matches. Returns a NotFoundError when no security has the given ISIN.
func (r *Repository) GetByISIN(isin string) (*Security, error) {
	norm := NormalizeISIN(isin)
	if norm == "" {
		return nil, &dberrors.NotFoundError{Entity: "security", ID: isin}
	}

	query := `SELECT ` + securityColumns + ` FROM securities WHERE COALESCE(isin, '') != '' AND UPPER(isin) = ?`

	sec, err := scanSecurity(r.q().QueryRow(query, norm).Scan)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "security", ID: isin}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get security by ISIN: %w", err)
	}

	return sec, nil
}

// FindByName retrieves every security whose name matches exactly
// (case-insensitive, trimmed). Names are not unique among tickered securities,
// so this returns a slice; callers resolve ambiguity.
func (r *Repository) FindByName(name string) ([]*Security, error) {
	trimmed := strings.TrimSpace(name)
	query := `SELECT ` + securityColumns + ` FROM securities WHERE LOWER(name) = LOWER(?) ORDER BY ticker, isin`

	rows, err := r.q().Query(query, trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to find securities by name: %w", err)
	}
	defer rows.Close()

	return scanSecurityRows(rows)
}

// List retrieves securities with optional filters.
func (r *Repository) List(filter Filter) ([]*Security, error) {
	query := `SELECT ` + securityColumns + ` FROM securities WHERE 1=1`
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

	rows, err := r.q().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list securities: %w", err)
	}
	defer rows.Close()

	return scanSecurityRows(rows)
}

// scanSecurityRows scans every row of a securities query in securityColumns order.
func scanSecurityRows(rows *sql.Rows) ([]*Security, error) {
	securities := make([]*Security, 0)
	for rows.Next() {
		sec, err := scanSecurity(rows.Scan)
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
func (r *Repository) Update(security *Security) error {
	security.Touch()

	if err := r.ensureUnique(security); err != nil {
		return err
	}

	result, err := r.q().Exec(`
		UPDATE securities SET
			ticker = ?,
			name = ?,
			isin = ?,
			security_type = ?,
			asset_class = ?,
			currency = ?,
			exchange = ?,
			hidden = ?,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		security.Ticker,
		security.Name,
		security.ISIN,
		security.SecurityType.String(),
		security.AssetClass.String(),
		security.Currency,
		dbutil.NullString(security.Exchange),
		security.Hidden,
		security.UpdatedAt.Time(),
		security.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update security: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "security", ID: security.ID.String()}
	}

	return nil
}

// Delete removes a security from the database.
// Fails if the security has dependent prices or transactions.
func (r *Repository) Delete(id types.ID) error {
	// Check for price dependents
	var priceCount int
	err := r.q().QueryRow(`
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

	result, err := r.q().Exec(`DELETE FROM securities WHERE CAST(id AS VARCHAR) = ?`, id.String())
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
