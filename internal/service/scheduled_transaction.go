package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// ScheduledTransactionService provides business logic for scheduled transaction operations.
type ScheduledTransactionService struct {
	repo    *repository.ScheduledTransactionRepository
	txnRepo *repository.TransactionRepository
	db      *db.DB
}

// NewScheduledTransactionService creates a new ScheduledTransactionService.
func NewScheduledTransactionService(
	repo *repository.ScheduledTransactionRepository,
	txnRepo *repository.TransactionRepository,
	database *db.DB,
) *ScheduledTransactionService {
	return &ScheduledTransactionService{
		repo:    repo,
		txnRepo: txnRepo,
		db:      database,
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create validates and creates a new scheduled transaction.
func (s *ScheduledTransactionService) Create(st *models.ScheduledTransaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	return s.repo.Create(st)
}

// GetByID retrieves a scheduled transaction by its ID.
func (s *ScheduledTransactionService) GetByID(id models.ID) (*models.ScheduledTransaction, error) {
	return s.repo.GetByID(id)
}

// Update validates and updates an existing scheduled transaction.
func (s *ScheduledTransactionService) Update(st *models.ScheduledTransaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	return s.repo.Update(st)
}

// Delete removes a scheduled transaction.
func (s *ScheduledTransactionService) Delete(id models.ID) error {
	return s.repo.Delete(id)
}

// List returns all scheduled transactions ordered by next_date ascending.
func (s *ScheduledTransactionService) List() ([]*models.ScheduledTransaction, error) {
	return s.repo.List()
}

// ListByAccount returns all scheduled transactions for an account.
func (s *ScheduledTransactionService) ListByAccount(accountID models.ID) ([]*models.ScheduledTransaction, error) {
	return s.repo.ListByAccount(accountID)
}

// ListDue returns all scheduled transactions that are due (next_date <= today).
func (s *ScheduledTransactionService) ListDue() ([]*models.ScheduledTransaction, error) {
	return s.repo.ListDue()
}

// ListUpcoming returns scheduled transactions with next_date within the specified number of days.
func (s *ScheduledTransactionService) ListUpcoming(days int) ([]*models.ScheduledTransaction, error) {
	return s.repo.ListUpcoming(days)
}

// =============================================================================
// Post and Skip Operations
// =============================================================================

// Post creates a real transaction from a scheduled transaction and advances the schedule.
// If the scheduled transaction has no fixed amount, the provided amount is used.
// If the scheduled transaction has a fixed amount and an amount is provided, it uses the provided amount.
// Returns the created transaction.
func (s *ScheduledTransactionService) Post(id models.ID, amount *models.Money) (*models.Transaction, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, &ScheduledTransactionCompletedError{ID: id.String()}
	}

	// Determine the amount to use
	var txnAmount models.Money
	if amount != nil {
		// Use provided amount
		txnAmount = *amount
	} else if st.HasAmount() {
		// Use scheduled amount
		txnAmount = st.Amount.Money
	} else {
		// Variable amount with no amount provided - try to estimate
		estimated, err := s.EstimateAmount(id)
		if err != nil {
			return nil, &ScheduledTransactionAmountRequiredError{ID: id.String()}
		}
		if estimated == nil {
			return nil, &ScheduledTransactionAmountRequiredError{ID: id.String()}
		}
		txnAmount = *estimated
	}

	// Create the transaction
	txn := models.NewTransaction(st.AccountID, st.NextDate, txnAmount)

	// Copy optional fields
	if st.HasPayee() {
		txn.SetPayee(st.PayeeID.ID)
	}
	if st.HasCategory() {
		txn.SetCategory(st.CategoryID.ID)
	}
	if st.Memo.Valid && st.Memo.String != "" {
		txn.SetMemo(st.Memo.String)
	}

	// Create the transaction
	if err := s.txnRepo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Advance the schedule
	if !st.AdvanceSchedule() {
		// Schedule is now completed, but transaction was created successfully
		// We still need to update the scheduled transaction to reflect completion
	}

	// Update the scheduled transaction
	if err := s.repo.Update(st); err != nil {
		// Transaction was created but schedule update failed
		// This is a partial success - log the error but return the transaction
		return txn, fmt.Errorf("transaction created but failed to update schedule: %w", err)
	}

	return txn, nil
}

// PostWithDate creates a transaction from a scheduled transaction with a specific date.
// Useful when posting a due transaction on a date other than the scheduled next_date.
func (s *ScheduledTransactionService) PostWithDate(id models.ID, date models.Date, amount *models.Money) (*models.Transaction, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, &ScheduledTransactionCompletedError{ID: id.String()}
	}

	// Determine the amount to use
	var txnAmount models.Money
	if amount != nil {
		txnAmount = *amount
	} else if st.HasAmount() {
		txnAmount = st.Amount.Money
	} else {
		estimated, err := s.EstimateAmount(id)
		if err != nil {
			return nil, &ScheduledTransactionAmountRequiredError{ID: id.String()}
		}
		if estimated == nil {
			return nil, &ScheduledTransactionAmountRequiredError{ID: id.String()}
		}
		txnAmount = *estimated
	}

	// Create the transaction with the specified date
	txn := models.NewTransaction(st.AccountID, date, txnAmount)

	// Copy optional fields
	if st.HasPayee() {
		txn.SetPayee(st.PayeeID.ID)
	}
	if st.HasCategory() {
		txn.SetCategory(st.CategoryID.ID)
	}
	if st.Memo.Valid && st.Memo.String != "" {
		txn.SetMemo(st.Memo.String)
	}

	// Create the transaction
	if err := s.txnRepo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Advance the schedule
	st.AdvanceSchedule()

	// Update the scheduled transaction
	if err := s.repo.Update(st); err != nil {
		return txn, fmt.Errorf("transaction created but failed to update schedule: %w", err)
	}

	return txn, nil
}

// Skip advances the schedule without creating a transaction.
// Useful when a scheduled payment is not made (e.g., skipping a bill payment).
func (s *ScheduledTransactionService) Skip(id models.ID) error {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return &ScheduledTransactionCompletedError{ID: id.String()}
	}

	// Advance the schedule without creating a transaction
	st.AdvanceSchedule()

	// Update the scheduled transaction
	return s.repo.Update(st)
}

// =============================================================================
// Amount Estimation
// =============================================================================

// EstimateAmount calculates an estimated amount based on recent transactions.
// Uses the average of the last N transactions to the same payee (where N is AmountEstimateCount).
// Returns nil if no estimate can be calculated.
func (s *ScheduledTransactionService) EstimateAmount(id models.ID) (*models.Money, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// If the scheduled transaction has a fixed amount, return that
	if st.HasAmount() {
		return &st.Amount.Money, nil
	}

	// If no estimate count is set, we can't estimate
	if !st.AmountEstimateCount.Valid || st.AmountEstimateCount.Int64 <= 0 {
		return nil, nil
	}

	// If no payee is set, we can't estimate
	if !st.HasPayee() {
		return nil, nil
	}

	// Get recent transactions to this payee
	count := int(st.AmountEstimateCount.Int64)
	transactions, err := s.getRecentTransactionsByPayee(st.AccountID, st.PayeeID.ID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, nil
	}

	// Calculate average
	var total models.Money
	for _, txn := range transactions {
		total = total.Add(txn.Amount)
	}

	average := total.Div(int64(len(transactions)))
	return &average, nil
}

// getRecentTransactionsByPayee retrieves the most recent transactions for a payee in an account.
func (s *ScheduledTransactionService) getRecentTransactionsByPayee(accountID, payeeID models.ID, limit int) ([]*models.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id, memo,
			   check_number, status, transfer_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		  AND CAST(payee_id AS VARCHAR) = ?
		ORDER BY date DESC, created_at DESC
		LIMIT ?
	`

	rows, err := s.db.Conn().Query(query, accountID.String(), payeeID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*models.Transaction
	for rows.Next() {
		txn := &models.Transaction{}
		err := rows.Scan(
			&txn.ID,
			&txn.AccountID,
			&txn.Date,
			&txn.Amount,
			&txn.PayeeID,
			&txn.CategoryID,
			&txn.Memo,
			&txn.CheckNumber,
			&txn.Status,
			&txn.TransferID,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, txn)
	}

	return transactions, rows.Err()
}

// =============================================================================
// Schedule Status Operations
// =============================================================================

// IsDue checks if a scheduled transaction is due (next_date <= today).
func (s *ScheduledTransactionService) IsDue(id models.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsDue(), nil
}

// IsCompleted checks if a scheduled transaction has finished all occurrences.
func (s *ScheduledTransactionService) IsCompleted(id models.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsCompleted(), nil
}

// GetNextDate returns the next occurrence date for a scheduled transaction.
func (s *ScheduledTransactionService) GetNextDate(id models.ID) (models.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return models.Date{}, err
	}
	return st.NextDate, nil
}

// CalculateNextDate calculates what the next occurrence would be after the current next_date.
// Does not modify the scheduled transaction.
func (s *ScheduledTransactionService) CalculateNextDate(id models.ID) (models.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return models.Date{}, err
	}
	return st.CalculateNextDate(), nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validateScheduledTransaction validates a scheduled transaction and returns any validation errors.
func (s *ScheduledTransactionService) validateScheduledTransaction(st *models.ScheduledTransaction) error {
	errors := st.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
