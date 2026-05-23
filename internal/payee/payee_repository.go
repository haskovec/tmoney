package payee

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Repository provides database operations for payees and aliases.
type Repository struct {
	db *db.DB
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// =============================================================================
// Payee CRUD Operations
// =============================================================================

// Create inserts a new payee into the database.
func (r *Repository) Create(payee *Payee) error {
	// Check for duplicate name
	var exists bool
	err := r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM payees WHERE name = ?)`, payee.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check payee name uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "payee", Field: "name", Value: payee.Name}
	}

	query := `
		INSERT INTO payees (
			id, name, default_category_id, notes,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		payee.ID,
		payee.Name,
		dbutil.NullID(payee.DefaultCategoryID),
		dbutil.NullString(payee.Notes),
		payee.CreatedAt,
		payee.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create payee: %w", err)
	}

	return nil
}

// GetByID retrieves a payee by its ID.
func (r *Repository) GetByID(id types.ID) (*Payee, error) {
	query := `
		SELECT id, name, default_category_id, notes,
			created_at, updated_at
		FROM payees
		WHERE CAST(id AS VARCHAR) = ?
	`

	payee := &Payee{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&payee.ID,
		&payee.Name,
		&payee.DefaultCategoryID,
		&payee.Notes,
		&payee.CreatedAt,
		&payee.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "payee", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payee: %w", err)
	}

	return payee, nil
}

// GetByName retrieves a payee by its name.
func (r *Repository) GetByName(name string) (*Payee, error) {
	query := `
		SELECT id, name, default_category_id, notes,
			created_at, updated_at
		FROM payees
		WHERE name = ?
	`

	payee := &Payee{}
	err := r.db.Conn().QueryRow(query, name).Scan(
		&payee.ID,
		&payee.Name,
		&payee.DefaultCategoryID,
		&payee.Notes,
		&payee.CreatedAt,
		&payee.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "payee", ID: name}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payee by name: %w", err)
	}

	return payee, nil
}

// List retrieves all payees ordered by name.
func (r *Repository) List() ([]*Payee, error) {
	query := `
		SELECT id, name, default_category_id, notes,
			created_at, updated_at
		FROM payees
		ORDER BY name
	`

	return r.queryPayees(query)
}

// Update updates an existing payee in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *Repository) Update(payee *Payee) error {
	payee.Touch()

	// Check for duplicate name (excluding current payee)
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM payees WHERE name = ? AND CAST(id AS VARCHAR) != ?)`,
		payee.Name, payee.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check payee name uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "payee", Field: "name", Value: payee.Name}
	}

	// Check if payee exists
	var count int
	err = r.db.Conn().QueryRow(`SELECT COUNT(*) FROM payees WHERE CAST(id AS VARCHAR) = ?`, payee.ID.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check payee exists: %w", err)
	}
	if count == 0 {
		return &dberrors.NotFoundError{Entity: "payee", ID: payee.ID.String()}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(`DELETE FROM payees WHERE CAST(id AS VARCHAR) = ?`, payee.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO payees (
			id, name, default_category_id, notes,
			created_at, updated_at
		) VALUES (CAST(? AS UUID), ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		payee.ID.String(),
		payee.Name,
		dbutil.NullID(payee.DefaultCategoryID),
		dbutil.NullString(payee.Notes),
		payee.CreatedAt.Time(),
		payee.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a payee from the database.
// This will fail if the payee has any transactions.
func (r *Repository) Delete(id types.ID) error {
	// Check for transactions
	var txnCount int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(payee_id AS VARCHAR) = ?
	`, id.String()).Scan(&txnCount)
	if err != nil {
		return fmt.Errorf("failed to check transactions: %w", err)
	}
	if txnCount > 0 {
		return &dberrors.HasDependentsError{
			Entity:     "payee",
			ID:         id.String(),
			Dependents: "transactions",
			Count:      txnCount,
		}
	}

	// Delete associated aliases first
	_, err = r.db.Conn().Exec(`DELETE FROM payee_aliases WHERE CAST(payee_id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete payee aliases: %w", err)
	}

	// Delete the payee
	result, err := r.db.Conn().Exec(`DELETE FROM payees WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete payee: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "payee", ID: id.String()}
	}

	return nil
}

// =============================================================================
// Alias CRUD Operations
// =============================================================================

// CreateAlias inserts a new alias into the database.
func (r *Repository) CreateAlias(alias *Alias) error {
	// Verify payee exists
	var exists bool
	err := r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`, alias.PayeeID.String()).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check payee exists: %w", err)
	}
	if !exists {
		return &dberrors.NotFoundError{Entity: "payee", ID: alias.PayeeID.String()}
	}

	// Check for duplicate pattern
	err = r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM payee_aliases WHERE pattern = ?)`, alias.Pattern).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check alias pattern uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "alias", Field: "pattern", Value: alias.Pattern}
	}

	query := `
		INSERT INTO payee_aliases (
			id, payee_id, pattern, match_type, created_at
		) VALUES (?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		alias.ID,
		alias.PayeeID,
		alias.Pattern,
		alias.MatchType,
		alias.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create alias: %w", err)
	}

	return nil
}

// GetAliasByID retrieves an alias by its ID.
func (r *Repository) GetAliasByID(id types.ID) (*Alias, error) {
	query := `
		SELECT id, payee_id, pattern, match_type, created_at
		FROM payee_aliases
		WHERE CAST(id AS VARCHAR) = ?
	`

	alias := &Alias{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&alias.ID,
		&alias.PayeeID,
		&alias.Pattern,
		&alias.MatchType,
		&alias.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "alias", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alias: %w", err)
	}

	return alias, nil
}

// GetAliasesByPayee retrieves all aliases for a payee.
func (r *Repository) GetAliasesByPayee(payeeID types.ID) ([]*Alias, error) {
	query := `
		SELECT id, payee_id, pattern, match_type, created_at
		FROM payee_aliases
		WHERE CAST(payee_id AS VARCHAR) = ?
		ORDER BY pattern
	`

	return r.queryAliasesWithArgs(query, payeeID.String())
}

// UpdateAlias updates an existing alias in the database.
func (r *Repository) UpdateAlias(alias *Alias) error {
	var exists bool
	err := r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`, alias.PayeeID.String()).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check payee exists: %w", err)
	}
	if !exists {
		return &dberrors.NotFoundError{Entity: "payee", ID: alias.PayeeID.String()}
	}

	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM payee_aliases WHERE pattern = ? AND CAST(id AS VARCHAR) != ?)`,
		alias.Pattern, alias.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check alias pattern uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "alias", Field: "pattern", Value: alias.Pattern}
	}

	result, err := r.db.Conn().Exec(`
		UPDATE payee_aliases SET
			payee_id = CAST(? AS UUID),
			pattern = ?,
			match_type = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		alias.PayeeID.String(),
		alias.Pattern,
		alias.MatchType.String(),
		alias.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update alias: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "alias", ID: alias.ID.String()}
	}

	return nil
}

// DeleteAlias removes an alias from the database.
func (r *Repository) DeleteAlias(id types.ID) error {
	result, err := r.db.Conn().Exec(`DELETE FROM payee_aliases WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "alias", ID: id.String()}
	}

	return nil
}

// =============================================================================
// Pattern Matching Operations
// =============================================================================

// FindPayeeByPattern searches all aliases for a match against the input string.
// Returns the matching payee, or nil if no match is found.
// Matching is case-insensitive.
func (r *Repository) FindPayeeByPattern(input string) (*Payee, error) {
	// Get all aliases
	aliases, err := r.listAllAliases()
	if err != nil {
		return nil, err
	}

	// Test each alias against the input
	for _, alias := range aliases {
		if alias.MatchesCaseInsensitive(input) {
			return r.GetByID(alias.PayeeID)
		}
	}

	return nil, nil
}

// FindAliasMatch searches all aliases for a match against the input string.
// Returns the matching alias, or nil if no match is found.
// Matching is case-insensitive.
func (r *Repository) FindAliasMatch(input string) (*Alias, error) {
	// Get all aliases
	aliases, err := r.listAllAliases()
	if err != nil {
		return nil, err
	}

	// Test each alias against the input
	for _, alias := range aliases {
		if alias.MatchesCaseInsensitive(input) {
			return alias, nil
		}
	}

	return nil, nil
}

// =============================================================================
// Helper Methods
// =============================================================================

// queryPayees executes a query and returns a slice of payees.
func (r *Repository) queryPayees(query string) ([]*Payee, error) {
	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query payees: %w", err)
	}
	defer rows.Close()

	return r.scanPayees(rows)
}

// scanPayees scans rows into a slice of payees.
func (r *Repository) scanPayees(rows *sql.Rows) ([]*Payee, error) {
	var payees []*Payee
	for rows.Next() {
		payee := &Payee{}
		err := rows.Scan(
			&payee.ID,
			&payee.Name,
			&payee.DefaultCategoryID,
			&payee.Notes,
			&payee.CreatedAt,
			&payee.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payee: %w", err)
		}
		payees = append(payees, payee)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payees: %w", err)
	}

	return payees, nil
}

// listAllAliases retrieves all aliases from the database.
func (r *Repository) listAllAliases() ([]*Alias, error) {
	query := `
		SELECT id, payee_id, pattern, match_type, created_at
		FROM payee_aliases
		ORDER BY pattern
	`

	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query aliases: %w", err)
	}
	defer rows.Close()

	return r.scanAliases(rows)
}

// queryAliasesWithArgs executes a query with arguments and returns a slice of aliases.
func (r *Repository) queryAliasesWithArgs(query string, args ...any) ([]*Alias, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query aliases: %w", err)
	}
	defer rows.Close()

	return r.scanAliases(rows)
}

// scanAliases scans rows into a slice of aliases.
func (r *Repository) scanAliases(rows *sql.Rows) ([]*Alias, error) {
	var aliases []*Alias
	for rows.Next() {
		alias := &Alias{}
		err := rows.Scan(
			&alias.ID,
			&alias.PayeeID,
			&alias.Pattern,
			&alias.MatchType,
			&alias.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, alias)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating aliases: %w", err)
	}

	return aliases, nil
}
