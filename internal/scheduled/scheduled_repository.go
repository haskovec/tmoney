package scheduled

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbutil"
	"github.com/haskovec/tmoney/internal/types"
)

// Repository provides database operations for scheduled transactions.
type Repository struct {
	db        *db.DB
	tx        db.Queryer // nil outside a transaction
	splitRepo *SplitRepository
}

// NewRepository creates a new Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{
		db:        database,
		splitRepo: NewSplitRepository(database),
	}
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
// unchanged and remains safe for non-transactional use. The child
// splitRepo is rebound to the same tx so both stay in the same transaction.
func (r *Repository) WithTx(tx db.Queryer) *Repository {
	return &Repository{db: r.db, tx: tx, splitRepo: r.splitRepo.WithTx(tx)}
}

// SplitRepo returns the underlying SplitRepository for multi-line template CRUD.
func (r *Repository) SplitRepo() *SplitRepository {
	return r.splitRepo
}

// Create inserts a new scheduled transaction into the database.
func (r *Repository) Create(st *Transaction) error {
	// Verify account exists
	var accountExists bool
	err := r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		st.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &dberrors.NotFoundError{Entity: "account", ID: st.AccountID.String()}
	}

	// Verify payee exists if specified
	if st.PayeeID.Valid {
		var payeeExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			st.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &dberrors.NotFoundError{Entity: "payee", ID: st.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if st.CategoryID.Valid {
		var categoryExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			st.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &dberrors.NotFoundError{Entity: "category", ID: st.CategoryID.ID.String()}
		}
	}

	// Verify transfer destination account exists if specified
	if st.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			st.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &dberrors.NotFoundError{Entity: "account", ID: st.TransferAccountID.ID.String()}
		}
	}

	query := `
		INSERT INTO scheduled_transactions (
			id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.q().Exec(query,
		st.ID,
		st.AccountID,
		dbutil.NullID(st.PayeeID),
		dbutil.NullID(st.CategoryID),
		dbutil.NullID(st.TransferAccountID),
		dbutil.NullMoney(st.Amount),
		dbutil.NullString(st.Memo),
		st.Frequency,
		st.Interval,
		st.StartDate,
		dbutil.NullDate(st.EndDate),
		dbutil.NullInt(st.Occurrences),
		dbutil.NullInt(st.DayOfMonth),
		dbutil.NullInt(st.SecondaryDayOfMonth),
		st.NextDate,
		dbutil.NullInt(st.OccurrencesRemaining),
		dbutil.NullInt(st.AmountEstimateCount),
		st.AutoPost,
		st.PostLeadDays,
		st.CreatedAt,
		st.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create scheduled transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a scheduled transaction by its ID.
func (r *Repository) GetByID(id types.ID) (*Transaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	st := &Transaction{}
	err := r.q().QueryRow(query, id.String()).Scan(
		&st.ID,
		&st.AccountID,
		&st.PayeeID,
		&st.CategoryID,
		&st.TransferAccountID,
		&st.Amount,
		&st.Memo,
		&st.Frequency,
		&st.Interval,
		&st.StartDate,
		&st.EndDate,
		&st.Occurrences,
		&st.DayOfMonth,
		&st.SecondaryDayOfMonth,
		&st.NextDate,
		&st.OccurrencesRemaining,
		&st.AmountEstimateCount,
		&st.AutoPost,
		&st.PostLeadDays,
		&st.CreatedAt,
		&st.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "scheduled_transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled transaction: %w", err)
	}

	if err := r.loadSplits(st); err != nil {
		return nil, err
	}

	return st, nil
}

// loadSplits populates st.Splits from scheduled_split_items.
func (r *Repository) loadSplits(st *Transaction) error {
	splits, err := r.splitRepo.ListByScheduledTransaction(st.ID)
	if err != nil {
		return fmt.Errorf("failed to load scheduled splits: %w", err)
	}
	st.Splits = SplitCollection(splits)
	return nil
}

// List retrieves all scheduled transactions ordered by next_date ascending.
func (r *Repository) List() ([]*Transaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryTransactions(query)
}

// ListByAccount retrieves all scheduled transactions for a specific account.
func (r *Repository) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryTransactionsWithArgs(query, accountID.String())
}

// ListDue retrieves all scheduled transactions that are due (next_date <=
// today) and not yet completed. The completion predicate mirrors
// Transaction.IsCompleted: a schedule is complete when occurrences_remaining
// has hit zero, or its next_date has passed its end_date. Excluding completed
// schedules here fixes a long-standing zombie bug — previously a completed
// schedule whose next_date was in the past surfaced as due forever (in the TUI
// due section, the due-count badge, and `scheduled list --due`) while Post and
// Skip refused it with CompletedError, leaving the user no way to clear it.
func (r *Repository) ListDue() ([]*Transaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE next_date <= CURRENT_DATE
		  AND (occurrences_remaining IS NULL OR occurrences_remaining > 0)
		  AND (end_date IS NULL OR next_date <= end_date)
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryTransactions(query)
}

// ListUpcoming retrieves scheduled transactions with next_date within the specified number of days.
func (r *Repository) ListUpcoming(days int) ([]*Transaction, error) {
	// Calculate the target date in Go since DuckDB doesn't support parameterized intervals
	targetDate := types.Today().AddDays(days)

	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE next_date <= ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryTransactionsWithArgs(query, targetDate.Time())
}

// ListAutoPostDue retrieves all auto-post scheduled transactions that should be posted.
// A transaction should be auto-posted when: next_date - post_lead_days <= today.
func (r *Repository) ListAutoPostDue() ([]*Transaction, error) {
	today := types.Today().Time()

	query := `
		SELECT id, account_id, payee_id, category_id, transfer_account_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, secondary_day_of_month, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE auto_post = TRUE
			AND next_date - INTERVAL (post_lead_days) DAY <= ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryTransactionsWithArgs(query, today)
}

// Update updates an existing scheduled transaction in the database.
func (r *Repository) Update(st *Transaction) error {
	st.Touch()

	var accountExists bool
	err := r.q().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		st.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &dberrors.NotFoundError{Entity: "account", ID: st.AccountID.String()}
	}

	if st.PayeeID.Valid {
		var payeeExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			st.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &dberrors.NotFoundError{Entity: "payee", ID: st.PayeeID.ID.String()}
		}
	}

	if st.CategoryID.Valid {
		var categoryExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			st.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &dberrors.NotFoundError{Entity: "category", ID: st.CategoryID.ID.String()}
		}
	}

	// Verify transfer destination account exists if specified
	if st.TransferAccountID.Valid {
		var transferAccountExists bool
		err := r.q().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
			st.TransferAccountID.ID.String(),
		).Scan(&transferAccountExists)
		if err != nil {
			return fmt.Errorf("failed to check transfer account exists: %w", err)
		}
		if !transferAccountExists {
			return &dberrors.NotFoundError{Entity: "account", ID: st.TransferAccountID.ID.String()}
		}
	}

	result, err := r.q().Exec(`
		UPDATE scheduled_transactions SET
			account_id = CAST(? AS UUID),
			payee_id = ?,
			category_id = ?,
			transfer_account_id = ?,
			amount = ?,
			memo = ?,
			frequency = ?,
			interval = ?,
			start_date = ?,
			end_date = ?,
			occurrences = ?,
			day_of_month = ?,
			secondary_day_of_month = ?,
			next_date = ?,
			occurrences_remaining = ?,
			amount_estimate_count = ?,
			auto_post = ?,
			post_lead_days = ?,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		st.AccountID.String(),
		dbutil.NullID(st.PayeeID),
		dbutil.NullID(st.CategoryID),
		dbutil.NullID(st.TransferAccountID),
		dbutil.NullMoney(st.Amount),
		dbutil.NullString(st.Memo),
		st.Frequency.String(),
		st.Interval,
		st.StartDate.Time(),
		dbutil.NullDate(st.EndDate),
		dbutil.NullInt(st.Occurrences),
		dbutil.NullInt(st.DayOfMonth),
		dbutil.NullInt(st.SecondaryDayOfMonth),
		st.NextDate.Time(),
		dbutil.NullInt(st.OccurrencesRemaining),
		dbutil.NullInt(st.AmountEstimateCount),
		st.AutoPost,
		st.PostLeadDays,
		st.UpdatedAt.Time(),
		st.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update scheduled transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "scheduled_transaction", ID: st.ID.String()}
	}

	return nil
}

// Delete removes a scheduled transaction from the database. Child
// scheduled_split_items rows are removed first to satisfy the FK constraint.
func (r *Repository) Delete(id types.ID) error {
	if _, err := r.splitRepo.DeleteByScheduledTransaction(id); err != nil {
		return fmt.Errorf("failed to delete scheduled splits: %w", err)
	}

	result, err := r.q().Exec(
		`DELETE FROM scheduled_transactions WHERE CAST(id AS VARCHAR) = ?`,
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled transaction: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "scheduled_transaction", ID: id.String()}
	}

	return nil
}

// HealNextDates corrects any rows where next_date precedes start_date —
// poisoned by an older binary that updated start_date without syncing
// next_date. Sets next_date := start_date in a single SQL UPDATE and
// returns the count of rows healed.
func (r *Repository) HealNextDates() (int, error) {
	result, err := r.q().Exec(`
		UPDATE scheduled_transactions
		SET next_date = start_date,
		    updated_at = CURRENT_TIMESTAMP
		WHERE next_date < start_date
	`)
	if err != nil {
		return 0, fmt.Errorf("HealNextDates: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("HealNextDates: rows affected: %w", err)
	}
	return int(rows), nil
}

// CountByAccount returns the number of scheduled transactions for an account.
func (r *Repository) CountByAccount(accountID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, accountID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// CountByCategory returns the number of scheduled transactions for a category.
func (r *Repository) CountByCategory(categoryID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(category_id AS VARCHAR) = ?
	`, categoryID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// CountByPayee returns the number of scheduled transactions for a payee.
func (r *Repository) CountByPayee(payeeID types.ID) (int, error) {
	var count int
	err := r.q().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(payee_id AS VARCHAR) = ?
	`, payeeID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// queryTransactions executes a query and returns a slice of scheduled transactions.
func (r *Repository) queryTransactions(query string) ([]*Transaction, error) {
	rows, err := r.q().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// queryTransactionsWithArgs executes a query with arguments and returns a slice of scheduled transactions.
func (r *Repository) queryTransactionsWithArgs(query string, args ...any) ([]*Transaction, error) {
	rows, err := r.q().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// scanTransactions scans rows into a slice of scheduled transactions.
func (r *Repository) scanTransactions(rows *sql.Rows) ([]*Transaction, error) {
	var transactions []*Transaction
	for rows.Next() {
		st := &Transaction{}
		err := rows.Scan(
			&st.ID,
			&st.AccountID,
			&st.PayeeID,
			&st.CategoryID,
			&st.TransferAccountID,
			&st.Amount,
			&st.Memo,
			&st.Frequency,
			&st.Interval,
			&st.StartDate,
			&st.EndDate,
			&st.Occurrences,
			&st.DayOfMonth,
			&st.SecondaryDayOfMonth,
			&st.NextDate,
			&st.OccurrencesRemaining,
			&st.AmountEstimateCount,
			&st.AutoPost,
			&st.PostLeadDays,
			&st.CreatedAt,
			&st.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scheduled transaction: %w", err)
		}
		transactions = append(transactions, st)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scheduled transactions: %w", err)
	}

	for _, st := range transactions {
		if err := r.loadSplits(st); err != nil {
			return nil, err
		}
	}

	return transactions, nil
}
