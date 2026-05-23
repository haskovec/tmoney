package account

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Repository provides database operations for accounts.
type Repository struct {
	db *db.DB
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// Create inserts a new account into the database.
func (r *Repository) Create(account *Account) error {
	// Check for duplicate name
	var exists bool
	err := r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ?)`, account.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check account name uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "account", Field: "name", Value: account.Name}
	}

	query := `
		INSERT INTO accounts (
			id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, track_lots, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		account.ID,
		account.Name,
		account.Type,
		account.Currency,
		dbutil.NullString(account.Institution),
		dbutil.NullString(account.AccountNumber),
		account.OpeningBalance,
		account.OpeningDate,
		dbutil.NullMoney(account.CreditLimit),
		dbutil.NullMoney(account.InterestRate),
		dbutil.NullString(account.Notes),
		account.Active,
		account.TrackLots,
		account.CreatedAt,
		account.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	return nil
}

// GetByID retrieves an account by its ID.
func (r *Repository) GetByID(id types.ID) (*Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, track_lots, created_at, updated_at
		FROM accounts
		WHERE CAST(id AS VARCHAR) = ?
	`

	account := &Account{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&account.ID,
		&account.Name,
		&account.Type,
		&account.Currency,
		&account.Institution,
		&account.AccountNumber,
		&account.OpeningBalance,
		&account.OpeningDate,
		&account.CreditLimit,
		&account.InterestRate,
		&account.Notes,
		&account.Active,
		&account.TrackLots,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "account", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// GetByName retrieves an account by its name.
func (r *Repository) GetByName(name string) (*Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, track_lots, created_at, updated_at
		FROM accounts
		WHERE name = ?
	`

	account := &Account{}
	err := r.db.Conn().QueryRow(query, name).Scan(
		&account.ID,
		&account.Name,
		&account.Type,
		&account.Currency,
		&account.Institution,
		&account.AccountNumber,
		&account.OpeningBalance,
		&account.OpeningDate,
		&account.CreditLimit,
		&account.InterestRate,
		&account.Notes,
		&account.Active,
		&account.TrackLots,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "account", ID: name}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account by name: %w", err)
	}

	return account, nil
}

// List retrieves all accounts, optionally filtered by active status.
func (r *Repository) List(activeOnly bool) ([]*Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, track_lots, created_at, updated_at
		FROM accounts
	`
	if activeOnly {
		query += " WHERE active = TRUE"
	}
	query += " ORDER BY name"

	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		account := &Account{}
		err := rows.Scan(
			&account.ID,
			&account.Name,
			&account.Type,
			&account.Currency,
			&account.Institution,
			&account.AccountNumber,
			&account.OpeningBalance,
			&account.OpeningDate,
			&account.CreditLimit,
			&account.InterestRate,
			&account.Notes,
			&account.Active,
			&account.TrackLots,
			&account.CreatedAt,
			&account.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating accounts: %w", err)
	}

	return accounts, nil
}

// Update updates an existing account in the database.
func (r *Repository) Update(account *Account) error {
	account.Touch()

	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ? AND CAST(id AS VARCHAR) != ?)`,
		account.Name, account.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check account name uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "account", Field: "name", Value: account.Name}
	}

	result, err := r.db.Conn().Exec(`
		UPDATE accounts SET
			name = ?, type = ?, currency = ?, institution = ?, account_number = ?,
			opening_balance = ?, opening_date = ?, credit_limit = ?, interest_rate = ?,
			notes = ?, active = ?, track_lots = ?, updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		account.Name,
		account.Type.String(),
		account.Currency,
		dbutil.NullString(account.Institution),
		dbutil.NullString(account.AccountNumber),
		account.OpeningBalance.String(),
		account.OpeningDate.Time(),
		dbutil.NullMoney(account.CreditLimit),
		dbutil.NullMoney(account.InterestRate),
		dbutil.NullString(account.Notes),
		account.Active,
		account.TrackLots,
		account.UpdatedAt.Time(),
		account.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "account", ID: account.ID.String()}
	}

	return nil
}

// Delete removes an account from the database.
// This will fail if the account has any transactions.
func (r *Repository) Delete(id types.ID) error {
	// Check for transactions first
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, id.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check transactions: %w", err)
	}
	if count > 0 {
		return &dberrors.HasDependentsError{
			Entity:     "account",
			ID:         id.String(),
			Dependents: "transactions",
			Count:      count,
		}
	}

	result, err := r.db.Conn().Exec(`DELETE FROM accounts WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "account", ID: id.String()}
	}

	return nil
}
