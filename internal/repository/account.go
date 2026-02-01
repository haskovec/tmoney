package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// AccountRepository provides database operations for accounts.
type AccountRepository struct {
	db *db.DB
}

// NewAccountRepository creates a new AccountRepository.
func NewAccountRepository(database *db.DB) *AccountRepository {
	return &AccountRepository{db: database}
}

// Create inserts a new account into the database.
func (r *AccountRepository) Create(account *models.Account) error {
	// Check for duplicate name
	var exists bool
	err := r.db.Conn().QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ?)`, account.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check account name uniqueness: %w", err)
	}
	if exists {
		return &DuplicateError{Entity: "account", Field: "name", Value: account.Name}
	}

	query := `
		INSERT INTO accounts (
			id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		account.ID,
		account.Name,
		account.Type,
		account.Currency,
		nullString(account.Institution),
		nullString(account.AccountNumber),
		account.OpeningBalance,
		account.OpeningDate,
		nullMoney(account.CreditLimit),
		nullMoney(account.InterestRate),
		nullString(account.Notes),
		account.Active,
		account.CreatedAt,
		account.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	return nil
}

// GetByID retrieves an account by its ID.
func (r *AccountRepository) GetByID(id models.ID) (*models.Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, created_at, updated_at
		FROM accounts
		WHERE CAST(id AS VARCHAR) = ?
	`

	account := &models.Account{}
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
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "account", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// GetByName retrieves an account by its name.
func (r *AccountRepository) GetByName(name string) (*models.Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, created_at, updated_at
		FROM accounts
		WHERE name = ?
	`

	account := &models.Account{}
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
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "account", ID: name}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account by name: %w", err)
	}

	return account, nil
}

// List retrieves all accounts, optionally filtered by active status.
func (r *AccountRepository) List(activeOnly bool) ([]*models.Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, created_at, updated_at
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

	var accounts []*models.Account
	for rows.Next() {
		account := &models.Account{}
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
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations
// with UPDATE operations on tables that have indexes.
func (r *AccountRepository) Update(account *models.Account) error {
	account.Touch()

	// Check for duplicate name (excluding current account)
	var exists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ? AND CAST(id AS VARCHAR) != ?)`,
		account.Name, account.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check account name uniqueness: %w", err)
	}
	if exists {
		return &DuplicateError{Entity: "account", Field: "name", Value: account.Name}
	}

	// Check if account exists and get created_at
	var count int
	err = r.db.Conn().QueryRow(`SELECT COUNT(*) FROM accounts WHERE CAST(id AS VARCHAR) = ?`, account.ID.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "account", ID: account.ID.String()}
	}

	// Delete the existing record (non-transactional due to DuckDB index bug)
	_, err = r.db.Conn().Exec(`DELETE FROM accounts WHERE CAST(id AS VARCHAR) = ?`, account.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record with string ID to avoid type issues
	insertQuery := `
		INSERT INTO accounts (
			id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, created_at, updated_at
		) VALUES (CAST(? AS UUID), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		account.ID.String(),
		account.Name,
		account.Type.String(),
		account.Currency,
		nullString(account.Institution),
		nullString(account.AccountNumber),
		account.OpeningBalance.String(),
		account.OpeningDate.Time(),
		nullMoney(account.CreditLimit),
		nullMoney(account.InterestRate),
		nullString(account.Notes),
		account.Active,
		account.CreatedAt.Time(),
		account.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes an account from the database.
// This will fail if the account has any transactions.
func (r *AccountRepository) Delete(id models.ID) error {
	// Check for transactions first
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, id.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check transactions: %w", err)
	}
	if count > 0 {
		return &HasDependentsError{
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
		return &NotFoundError{Entity: "account", ID: id.String()}
	}

	return nil
}

// nullString converts NullableString to a value for database insertion.
func nullString(ns models.NullableString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

// nullMoney converts NullableMoney to a value for database insertion.
func nullMoney(nm models.NullableMoney) interface{} {
	if nm.Valid {
		return nm.Money.String()
	}
	return nil
}
