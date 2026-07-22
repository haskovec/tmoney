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
			notes, active, closed_date, track_lots, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		dbutil.NullDate(account.ClosedDate),
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
			notes, active, closed_date, track_lots, created_at, updated_at
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
		&account.ClosedDate,
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
			notes, active, closed_date, track_lots, created_at, updated_at
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
		&account.ClosedDate,
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

// BalanceAsOf returns the account's signed balance as of the given date:
// opening_balance + Σ of the non-void transaction amounts dated on or before
// asOf. It is the single-account form of the net-worth report's as-of query
// (report_service.go netWorthAsOf); liability accounts (loan, credit card)
// return a negative balance when money is owed, matching the standardized
// sign convention. Only parent transaction amounts are summed — a
// transaction's split lines always net to its parent amount, so the parent
// already carries the full account impact (the split table is never summed
// into an account balance). Void rows keep their date/amount but contribute
// nothing. The loan recompute engine uses this to read a loan's outstanding
// balance as of the occurrence date being posted.
func (r *Repository) BalanceAsOf(id types.ID, asOf types.Date) (types.Money, error) {
	query := `
		SELECT a.opening_balance + COALESCE(
			(SELECT SUM(t.amount)
			 FROM transactions t
			 WHERE t.account_id = a.id
			   AND t.date <= ?
			   AND t.status != 'void'), 0)
		FROM accounts a
		WHERE CAST(a.id AS VARCHAR) = ?
	`
	var balance types.Money
	err := r.db.Conn().QueryRow(query, asOf.Time(), id.String()).Scan(&balance)
	if err == sql.ErrNoRows {
		return types.ZeroMoney, &dberrors.NotFoundError{Entity: "account", ID: id.String()}
	}
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to compute account balance as of date: %w", err)
	}
	return balance, nil
}

// Balance returns the account's full signed balance across all dates:
// opening_balance + Σ of every non-void transaction amount. It mirrors
// BalanceAsOf without the date bound and backs the loan payoff check (has the
// outstanding balance reached zero after a payment?), which must count a
// just-posted principal counterpart even when it is future-dated.
func (r *Repository) Balance(id types.ID) (types.Money, error) {
	query := `
		SELECT a.opening_balance + COALESCE(
			(SELECT SUM(t.amount)
			 FROM transactions t
			 WHERE t.account_id = a.id
			   AND t.status != 'void'), 0)
		FROM accounts a
		WHERE CAST(a.id AS VARCHAR) = ?
	`
	var balance types.Money
	err := r.db.Conn().QueryRow(query, id.String()).Scan(&balance)
	if err == sql.ErrNoRows {
		return types.ZeroMoney, &dberrors.NotFoundError{Entity: "account", ID: id.String()}
	}
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to compute account balance: %w", err)
	}
	return balance, nil
}

// List retrieves all accounts, optionally filtered by active status.
func (r *Repository) List(activeOnly bool) ([]*Account, error) {
	query := `
		SELECT id, name, type, currency, institution, account_number,
			opening_balance, opening_date, credit_limit, interest_rate,
			notes, active, closed_date, track_lots, created_at, updated_at
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
			&account.ClosedDate,
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
			notes = ?, active = ?, closed_date = ?, track_lots = ?, updated_at = ?
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
		dbutil.NullDate(account.ClosedDate),
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
// This will fail if the account has any transactions, or if any scheduled
// transaction references it (as its source account, its single-line transfer
// destination, or a transfer-line split target) — deleting would orphan those
// schedules.
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

	// Then for scheduled transactions referencing the account in any role
	// (mirrors scheduled.Service.ListReferencing).
	err = r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions st
		WHERE CAST(st.account_id AS VARCHAR) = ?
		   OR CAST(st.transfer_account_id AS VARCHAR) = ?
		   OR EXISTS (
				SELECT 1 FROM scheduled_split_items si
				WHERE si.scheduled_transaction_id = st.id
				  AND CAST(si.transfer_account_id AS VARCHAR) = ?
		   )
	`, id.String(), id.String(), id.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check scheduled transactions: %w", err)
	}
	if count > 0 {
		return &dberrors.HasDependentsError{
			Entity:     "account",
			ID:         id.String(),
			Dependents: "scheduled transactions",
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
