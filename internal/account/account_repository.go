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

// Create inserts a new account into the database.
func (r *Repository) Create(account *Account) error {
	// Check for duplicate name
	var exists bool
	err := r.q().QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ?)`, account.Name).Scan(&exists)
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

	_, err = r.q().Exec(query,
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
	err := r.q().QueryRow(query, id.String()).Scan(
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
	err := r.q().QueryRow(query, name).Scan(
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
	err := r.q().QueryRow(query, asOf.Time(), id.String()).Scan(&balance)
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
	err := r.q().QueryRow(query, id.String()).Scan(&balance)
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

	rows, err := r.q().Query(query)
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
	err := r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ? AND CAST(id AS VARCHAR) != ?)`,
		account.Name, account.ID.String(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check account name uniqueness: %w", err)
	}
	if exists {
		return &dberrors.DuplicateError{Entity: "account", Field: "name", Value: account.Name}
	}

	result, err := r.q().Exec(`
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
//
// It refuses with a HasDependentsError while any row still holds a foreign key
// to the account (see DeleteBlocker), so the caller gets an error it can act on
// instead of a driver error from inside the delete. Completed reconciliation
// sessions must already be gone: DeleteCompletedSessions removes them, in a
// transaction of its own (Service.Delete explains why).
func (r *Repository) Delete(id types.ID) error {
	blocker, err := r.DeleteBlocker(id)
	if err != nil {
		return err
	}
	if blocker != nil {
		return blocker
	}

	result, err := r.q().Exec(`DELETE FROM accounts WHERE CAST(id AS VARCHAR) = ?`, id.String())
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

// DeleteCompletedSessions removes the account's completed reconciliation
// sessions, the first step of deleting the account. It refuses first, and
// writes nothing, when DeleteBlocker names a reason the account cannot go:
// sessions of an account that stays would lose their history for nothing.
func (r *Repository) DeleteCompletedSessions(id types.ID) error {
	blocker, err := r.DeleteBlocker(id)
	if err != nil {
		return err
	}
	if blocker != nil {
		return blocker
	}
	if _, err := r.q().Exec(
		`DELETE FROM reconciliation_sessions
		 WHERE CAST(account_id AS VARCHAR) = ? AND status = 'completed'`, id.String(),
	); err != nil {
		return fmt.Errorf("failed to delete completed reconciliation sessions: %w", err)
	}
	return nil
}

// DeleteBlocker returns the first reason the account cannot be deleted, or nil
// when it can. It checks, in order, every row that holds a foreign key to the
// account, on both ledgers whatever the account's type:
//
//   - "transactions": its own register rows;
//   - "investment transactions": its own investment rows (lots and positions
//     are not counted here, so one buy reads as one transaction);
//   - "investment holdings": lots or positions left with no rows;
//   - "transfer references": rows in other accounts that name it as their
//     transfer partner, such as the surviving leg of a half-deleted transfer;
//   - "scheduled transactions": schedules that reference it in any role
//     (mirrors scheduled.Service.ListReferencing), which would be orphaned;
//   - "active reconciliation": an in-progress reconciliation session.
func (r *Repository) DeleteBlocker(id types.ID) (*dberrors.HasDependentsError, error) {
	sid := id.String()
	checks := []struct {
		dependents string
		query      string
		args       []any
	}{
		{"transactions",
			`SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?`,
			[]any{sid}},
		{"investment transactions",
			`SELECT COUNT(*) FROM investment_transactions WHERE CAST(account_id AS VARCHAR) = ?`,
			[]any{sid}},
		{"investment holdings",
			`SELECT (SELECT COUNT(*) FROM investment_lots WHERE CAST(account_id AS VARCHAR) = ?)
			      + (SELECT COUNT(*) FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ?)`,
			[]any{sid, sid}},
		{"transfer references",
			`SELECT (SELECT COUNT(*) FROM transactions
			         WHERE CAST(transfer_account_id AS VARCHAR) = ? AND CAST(account_id AS VARCHAR) <> ?)
			      + (SELECT COUNT(*) FROM investment_transactions
			         WHERE CAST(transfer_account_id AS VARCHAR) = ? AND CAST(account_id AS VARCHAR) <> ?)`,
			[]any{sid, sid, sid, sid}},
		{"scheduled transactions",
			`SELECT COUNT(*) FROM scheduled_transactions st
			 WHERE CAST(st.account_id AS VARCHAR) = ?
			    OR CAST(st.transfer_account_id AS VARCHAR) = ?
			    OR EXISTS (
					SELECT 1 FROM scheduled_split_items si
					WHERE si.scheduled_transaction_id = st.id
					  AND CAST(si.transfer_account_id AS VARCHAR) = ?
			    )`,
			[]any{sid, sid, sid}},
		{"active reconciliation",
			`SELECT COUNT(*) FROM reconciliation_sessions
			 WHERE CAST(account_id AS VARCHAR) = ? AND status = 'in_progress'`,
			[]any{sid}},
	}
	for _, c := range checks {
		var count int
		if err := r.q().QueryRow(c.query, c.args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("failed to check %s: %w", c.dependents, err)
		}
		if count > 0 {
			return &dberrors.HasDependentsError{
				Entity:     "account",
				ID:         sid,
				Dependents: c.dependents,
				Count:      count,
			}, nil
		}
	}
	return nil, nil
}

// CountLedgerRows returns how many rows the account owns in one ledger. With
// investment true it counts investment_transactions plus the account's
// investment_positions and investment_lots, because holdings left behind
// would vanish from portfolio_holdings once the type left the investment
// ledger. Otherwise it counts transactions plus scheduled_transactions,
// because a schedule posts into the regular ledger and would strand its next
// post if the account left it.
func (r *Repository) CountLedgerRows(accountID types.ID, investment bool) (int, error) {
	id := accountID.String()
	query := `
		SELECT (SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?)
		     + (SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(account_id AS VARCHAR) = ?)`
	args := []any{id, id}
	if investment {
		query = `
			SELECT (SELECT COUNT(*) FROM investment_transactions WHERE CAST(account_id AS VARCHAR) = ?)
			     + (SELECT COUNT(*) FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ?)
			     + (SELECT COUNT(*) FROM investment_lots WHERE CAST(account_id AS VARCHAR) = ?)`
		args = []any{id, id, id}
	}
	var n int
	if err := r.q().QueryRow(query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count ledger rows: %w", err)
	}
	return n, nil
}
