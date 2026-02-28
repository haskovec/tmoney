package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// ScheduledTransactionRepository provides database operations for scheduled transactions.
type ScheduledTransactionRepository struct {
	db *db.DB
}

// NewScheduledTransactionRepository creates a new ScheduledTransactionRepository.
func NewScheduledTransactionRepository(database *db.DB) *ScheduledTransactionRepository {
	return &ScheduledTransactionRepository{db: database}
}

// Create inserts a new scheduled transaction into the database.
func (r *ScheduledTransactionRepository) Create(st *models.ScheduledTransaction) error {
	// Verify account exists
	var accountExists bool
	err := r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		st.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &NotFoundError{Entity: "account", ID: st.AccountID.String()}
	}

	// Verify payee exists if specified
	if st.PayeeID.Valid {
		var payeeExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			st.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &NotFoundError{Entity: "payee", ID: st.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if st.CategoryID.Valid {
		var categoryExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			st.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &NotFoundError{Entity: "category", ID: st.CategoryID.ID.String()}
		}
	}

	query := `
		INSERT INTO scheduled_transactions (
			id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Conn().Exec(query,
		st.ID,
		st.AccountID,
		nullID(st.PayeeID),
		nullID(st.CategoryID),
		nullMoney(st.Amount),
		nullString(st.Memo),
		st.Frequency,
		st.Interval,
		st.StartDate,
		nullDate(st.EndDate),
		nullInt(st.Occurrences),
		nullInt(st.DayOfMonth),
		nullInt(st.DayOfWeek),
		st.NextDate,
		nullInt(st.OccurrencesRemaining),
		nullInt(st.AmountEstimateCount),
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
func (r *ScheduledTransactionRepository) GetByID(id models.ID) (*models.ScheduledTransaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE CAST(id AS VARCHAR) = ?
	`

	st := &models.ScheduledTransaction{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&st.ID,
		&st.AccountID,
		&st.PayeeID,
		&st.CategoryID,
		&st.Amount,
		&st.Memo,
		&st.Frequency,
		&st.Interval,
		&st.StartDate,
		&st.EndDate,
		&st.Occurrences,
		&st.DayOfMonth,
		&st.DayOfWeek,
		&st.NextDate,
		&st.OccurrencesRemaining,
		&st.AmountEstimateCount,
		&st.AutoPost,
		&st.PostLeadDays,
		&st.CreatedAt,
		&st.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "scheduled_transaction", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled transaction: %w", err)
	}

	return st, nil
}

// List retrieves all scheduled transactions ordered by next_date ascending.
func (r *ScheduledTransactionRepository) List() ([]*models.ScheduledTransaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryScheduledTransactions(query)
}

// ListByAccount retrieves all scheduled transactions for a specific account.
func (r *ScheduledTransactionRepository) ListByAccount(accountID models.ID) ([]*models.ScheduledTransaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryScheduledTransactionsWithArgs(query, accountID.String())
}

// ListDue retrieves all scheduled transactions that are due (next_date <= today).
func (r *ScheduledTransactionRepository) ListDue() ([]*models.ScheduledTransaction, error) {
	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE next_date <= CURRENT_DATE
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryScheduledTransactions(query)
}

// ListUpcoming retrieves scheduled transactions with next_date within the specified number of days.
func (r *ScheduledTransactionRepository) ListUpcoming(days int) ([]*models.ScheduledTransaction, error) {
	// Calculate the target date in Go since DuckDB doesn't support parameterized intervals
	targetDate := models.Today().AddDays(days)

	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE next_date <= ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryScheduledTransactionsWithArgs(query, targetDate.Time())
}

// ListAutoPostDue retrieves all auto-post scheduled transactions that should be posted.
// A transaction should be auto-posted when: next_date - post_lead_days <= today.
func (r *ScheduledTransactionRepository) ListAutoPostDue() ([]*models.ScheduledTransaction, error) {
	today := models.Today().Time()

	query := `
		SELECT id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		FROM scheduled_transactions
		WHERE auto_post = TRUE
			AND next_date - INTERVAL (post_lead_days) DAY <= ?
		ORDER BY next_date ASC, created_at ASC
	`

	return r.queryScheduledTransactionsWithArgs(query, today)
}

// Update updates an existing scheduled transaction in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *ScheduledTransactionRepository) Update(st *models.ScheduledTransaction) error {
	st.Touch()

	// Check if scheduled transaction exists
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(id AS VARCHAR) = ?`,
		st.ID.String(),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check scheduled transaction exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "scheduled_transaction", ID: st.ID.String()}
	}

	// Verify account exists
	var accountExists bool
	err = r.db.Conn().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE CAST(id AS VARCHAR) = ?)`,
		st.AccountID.String(),
	).Scan(&accountExists)
	if err != nil {
		return fmt.Errorf("failed to check account exists: %w", err)
	}
	if !accountExists {
		return &NotFoundError{Entity: "account", ID: st.AccountID.String()}
	}

	// Verify payee exists if specified
	if st.PayeeID.Valid {
		var payeeExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM payees WHERE CAST(id AS VARCHAR) = ?)`,
			st.PayeeID.ID.String(),
		).Scan(&payeeExists)
		if err != nil {
			return fmt.Errorf("failed to check payee exists: %w", err)
		}
		if !payeeExists {
			return &NotFoundError{Entity: "payee", ID: st.PayeeID.ID.String()}
		}
	}

	// Verify category exists if specified
	if st.CategoryID.Valid {
		var categoryExists bool
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE CAST(id AS VARCHAR) = ?)`,
			st.CategoryID.ID.String(),
		).Scan(&categoryExists)
		if err != nil {
			return fmt.Errorf("failed to check category exists: %w", err)
		}
		if !categoryExists {
			return &NotFoundError{Entity: "category", ID: st.CategoryID.ID.String()}
		}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(
		`DELETE FROM scheduled_transactions WHERE CAST(id AS VARCHAR) = ?`,
		st.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO scheduled_transactions (
			id, account_id, payee_id, category_id, amount, memo,
			frequency, interval, start_date, end_date, occurrences,
			day_of_month, day_of_week, next_date, occurrences_remaining,
			amount_estimate_count, auto_post, post_lead_days,
			created_at, updated_at
		) VALUES (CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		st.ID.String(),
		st.AccountID.String(),
		nullID(st.PayeeID),
		nullID(st.CategoryID),
		nullMoney(st.Amount),
		nullString(st.Memo),
		st.Frequency.String(),
		st.Interval,
		st.StartDate.Time(),
		nullDate(st.EndDate),
		nullInt(st.Occurrences),
		nullInt(st.DayOfMonth),
		nullInt(st.DayOfWeek),
		st.NextDate.Time(),
		nullInt(st.OccurrencesRemaining),
		nullInt(st.AmountEstimateCount),
		st.AutoPost,
		st.PostLeadDays,
		st.CreatedAt.Time(),
		st.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a scheduled transaction from the database.
func (r *ScheduledTransactionRepository) Delete(id models.ID) error {
	result, err := r.db.Conn().Exec(
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
		return &NotFoundError{Entity: "scheduled_transaction", ID: id.String()}
	}

	return nil
}

// CountByAccount returns the number of scheduled transactions for an account.
func (r *ScheduledTransactionRepository) CountByAccount(accountID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(account_id AS VARCHAR) = ?
	`, accountID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// CountByCategory returns the number of scheduled transactions for a category.
func (r *ScheduledTransactionRepository) CountByCategory(categoryID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(category_id AS VARCHAR) = ?
	`, categoryID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// CountByPayee returns the number of scheduled transactions for a payee.
func (r *ScheduledTransactionRepository) CountByPayee(payeeID models.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM scheduled_transactions WHERE CAST(payee_id AS VARCHAR) = ?
	`, payeeID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scheduled transactions: %w", err)
	}
	return count, nil
}

// queryScheduledTransactions executes a query and returns a slice of scheduled transactions.
func (r *ScheduledTransactionRepository) queryScheduledTransactions(query string) ([]*models.ScheduledTransaction, error) {
	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled transactions: %w", err)
	}
	defer rows.Close()

	return r.scanScheduledTransactions(rows)
}

// queryScheduledTransactionsWithArgs executes a query with arguments and returns a slice of scheduled transactions.
func (r *ScheduledTransactionRepository) queryScheduledTransactionsWithArgs(query string, args ...any) ([]*models.ScheduledTransaction, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled transactions: %w", err)
	}
	defer rows.Close()

	return r.scanScheduledTransactions(rows)
}

// scanScheduledTransactions scans rows into a slice of scheduled transactions.
func (r *ScheduledTransactionRepository) scanScheduledTransactions(rows *sql.Rows) ([]*models.ScheduledTransaction, error) {
	var transactions []*models.ScheduledTransaction
	for rows.Next() {
		st := &models.ScheduledTransaction{}
		err := rows.Scan(
			&st.ID,
			&st.AccountID,
			&st.PayeeID,
			&st.CategoryID,
			&st.Amount,
			&st.Memo,
			&st.Frequency,
			&st.Interval,
			&st.StartDate,
			&st.EndDate,
			&st.Occurrences,
			&st.DayOfMonth,
			&st.DayOfWeek,
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

	return transactions, nil
}

// nullDate converts a NullableDate for database insertion.
func nullDate(nd models.NullableDate) any {
	if !nd.Valid {
		return nil
	}
	return nd.Date.Time()
}

// nullInt converts a NullableInt for database insertion.
func nullInt(ni models.NullableInt) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}
