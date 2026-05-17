package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for scheduled transaction operations.
type Service struct {
	repo    *Repository
	txnRepo *transaction.Repository
	db      *db.DB
}

// NewService creates a new Service.
func NewService(
	repo *Repository,
	txnRepo *transaction.Repository,
	database *db.DB,
) *Service {
	return &Service{
		repo:    repo,
		txnRepo: txnRepo,
		db:      database,
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create validates and creates a new scheduled transaction. If st carries
// child Splits, the schedule is multi-line: child rows are persisted and
// validation enforces the signed-sum / mutually-exclusive-shape rules.
func (s *Service) Create(st *Transaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	if err := s.validateScheduledSplits(st); err != nil {
		return err
	}
	if err := s.repo.Create(st); err != nil {
		return err
	}
	if len(st.Splits) == 0 {
		return nil
	}
	for _, split := range st.Splits {
		split.ScheduledTransactionID = st.ID
		if err := s.repo.SplitRepo().Create(split); err != nil {
			// Best-effort rollback: remove the parent (cascades any
			// already-inserted children) so a failed multi-line create
			// leaves no partial state behind.
			_ = s.repo.Delete(st.ID)
			return fmt.Errorf("failed to create scheduled split: %w", err)
		}
	}
	return nil
}

// GetByID retrieves a scheduled transaction by its ID.
func (s *Service) GetByID(id types.ID) (*Transaction, error) {
	return s.repo.GetByID(id)
}

// Update validates and updates an existing scheduled transaction. If st
// carries Splits, the existing child rows are replaced (DELETE + INSERT) so
// the persisted template matches st.Splits exactly.
func (s *Service) Update(st *Transaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	if err := s.validateScheduledSplits(st); err != nil {
		return err
	}
	// Clear existing children first: the parent's DELETE+INSERT (under the
	// hood of repo.Update) would otherwise trip the FK from
	// scheduled_split_items.
	if _, err := s.repo.SplitRepo().DeleteByScheduledTransaction(st.ID); err != nil {
		return fmt.Errorf("failed to clear existing scheduled splits: %w", err)
	}
	if err := s.repo.Update(st); err != nil {
		return err
	}
	for _, split := range st.Splits {
		split.ScheduledTransactionID = st.ID
		if err := s.repo.SplitRepo().Create(split); err != nil {
			return fmt.Errorf("failed to insert updated scheduled split: %w", err)
		}
	}
	return nil
}

// Delete removes a scheduled transaction.
func (s *Service) Delete(id types.ID) error {
	return s.repo.Delete(id)
}

// List returns all scheduled transactions ordered by next_date ascending.
func (s *Service) List() ([]*Transaction, error) {
	return s.repo.List()
}

// ListByAccount returns all scheduled transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.repo.ListByAccount(accountID)
}

// ListDue returns all scheduled transactions that are due (next_date <= today).
func (s *Service) ListDue() ([]*Transaction, error) {
	return s.repo.ListDue()
}

// ListUpcoming returns scheduled transactions with next_date within the specified number of days.
func (s *Service) ListUpcoming(days int) ([]*Transaction, error) {
	return s.repo.ListUpcoming(days)
}

// =============================================================================
// Auto-post Operations
// =============================================================================

// AutoPostResult describes the outcome of an auto-post operation for a single scheduled transaction.
type AutoPostResult struct {
	ScheduledTransactionID types.ID
	Transactions           []*transaction.Transaction
	BeforeSchedule         *Transaction // Schedule state before auto-posting (for undo)
	Skipped                bool         // True if skipped due to variable amount with no estimate
	SkipReason             string       // Reason for skipping
}

// AutoPostSummary contains the results of running auto-post on file open.
type AutoPostSummary struct {
	Results      []AutoPostResult
	PostedCount  int // Total number of transactions created
	SkippedCount int // Number of scheduled transactions skipped
}

// AutoPost finds all due auto-post scheduled transactions and posts them.
// This should be called on file open (both TUI and CLI).
// Multiple overdue occurrences are posted, each with their correct scheduled date.
// Variable-amount transactions without an estimate are skipped.
func (s *Service) AutoPost() (*AutoPostSummary, error) {
	summary := &AutoPostSummary{}

	candidates, err := s.repo.ListAutoPostDue()
	if err != nil {
		return nil, fmt.Errorf("failed to list auto-post due transactions: %w", err)
	}

	today := types.Today()

	for _, st := range candidates {
		result := AutoPostResult{
			ScheduledTransactionID: st.ID,
		}

		// Capture schedule state before any modifications (deep copy for undo)
		beforeSchedule := *st
		result.BeforeSchedule = &beforeSchedule

		// Post all overdue occurrences for this scheduled transaction
		for !st.IsCompleted() && s.isAutoPostDue(st, today) {
			// Determine the amount to use
			var txnAmount types.Money
			if st.HasAmount() {
				txnAmount = st.Amount.Money
			} else {
				// Variable amount - try to estimate
				estimated, estErr := s.estimateAmountForSchedule(st)
				if estErr != nil || estimated == nil {
					result.Skipped = true
					result.SkipReason = "variable amount with no estimate available"
					break
				}
				txnAmount = *estimated
			}

			// Create the transaction with the scheduled date (not today)
			txn := transaction.NewTransaction(st.AccountID, st.NextDate, txnAmount)

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
				return nil, fmt.Errorf("failed to create auto-post transaction: %w", err)
			}

			result.Transactions = append(result.Transactions, txn)
			summary.PostedCount++

			// Advance the schedule
			st.AdvanceSchedule()
		}

		// Update the scheduled transaction to persist any schedule advancement
		if len(result.Transactions) > 0 || result.Skipped {
			if len(result.Transactions) > 0 {
				if err := s.repo.Update(st); err != nil {
					return nil, fmt.Errorf("failed to update schedule after auto-post: %w", err)
				}
			}
		}

		if result.Skipped {
			summary.SkippedCount++
		}

		if len(result.Transactions) > 0 || result.Skipped {
			summary.Results = append(summary.Results, result)
		}
	}

	return summary, nil
}

// isAutoPostDue checks if a scheduled transaction should be auto-posted based on lead days.
func (s *Service) isAutoPostDue(st *Transaction, today types.Date) bool {
	effectiveDate := st.NextDate.AddDays(-st.PostLeadDays)
	return !effectiveDate.After(today)
}

// estimateAmountForSchedule estimates the amount for a variable-amount scheduled transaction.
// This avoids re-fetching from the DB since we already have the scheduled transaction in memory.
func (s *Service) estimateAmountForSchedule(st *Transaction) (*types.Money, error) {
	if st.HasAmount() {
		return &st.Amount.Money, nil
	}

	if !st.AmountEstimateCount.Valid || st.AmountEstimateCount.Int64 <= 0 {
		return nil, nil
	}

	if !st.HasPayee() {
		return nil, nil
	}

	count := int(st.AmountEstimateCount.Int64)
	transactions, err := s.getRecentTransactionsByPayee(st.AccountID, st.PayeeID.ID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, nil
	}

	var total types.Money
	for _, txn := range transactions {
		total = total.Add(txn.Amount)
	}

	average := total.Div(int64(len(transactions)))
	return &average, nil
}

// =============================================================================
// Post and Skip Operations
// =============================================================================

// Post creates a real transaction from a scheduled transaction and advances the schedule.
// If the scheduled transaction has no fixed amount, the provided amount is used.
// If the scheduled transaction has a fixed amount and an amount is provided, it uses the provided amount.
// Returns the created transaction.
func (s *Service) Post(id types.ID, amount *types.Money) (*transaction.Transaction, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, &CompletedError{ID: id.String()}
	}

	// Determine the amount to use
	var txnAmount types.Money
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
			return nil, &AmountRequiredError{ID: id.String()}
		}
		if estimated == nil {
			return nil, &AmountRequiredError{ID: id.String()}
		}
		txnAmount = *estimated
	}

	// Create the transaction
	txn := transaction.NewTransaction(st.AccountID, st.NextDate, txnAmount)

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

	// Advance the schedule. If AdvanceSchedule returns false the series is
	// complete; either way we persist the updated state below.
	st.AdvanceSchedule()

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
func (s *Service) PostWithDate(id types.ID, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, &CompletedError{ID: id.String()}
	}

	// Determine the amount to use
	var txnAmount types.Money
	if amount != nil {
		txnAmount = *amount
	} else if st.HasAmount() {
		txnAmount = st.Amount.Money
	} else {
		estimated, err := s.EstimateAmount(id)
		if err != nil {
			return nil, &AmountRequiredError{ID: id.String()}
		}
		if estimated == nil {
			return nil, &AmountRequiredError{ID: id.String()}
		}
		txnAmount = *estimated
	}

	// Create the transaction with the specified date
	txn := transaction.NewTransaction(st.AccountID, date, txnAmount)

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
func (s *Service) Skip(id types.ID) error {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return &CompletedError{ID: id.String()}
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
func (s *Service) EstimateAmount(id types.ID) (*types.Money, error) {
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
	var total types.Money
	for _, txn := range transactions {
		total = total.Add(txn.Amount)
	}

	average := total.Div(int64(len(transactions)))
	return &average, nil
}

// getRecentTransactionsByPayee retrieves the most recent transactions for a payee in an account.
func (s *Service) getRecentTransactionsByPayee(accountID, payeeID types.ID, limit int) ([]*transaction.Transaction, error) {
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

	var transactions []*transaction.Transaction
	for rows.Next() {
		txn := &transaction.Transaction{}
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
func (s *Service) IsDue(id types.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsDue(), nil
}

// IsCompleted checks if a scheduled transaction has finished all occurrences.
func (s *Service) IsCompleted(id types.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsCompleted(), nil
}

// GetNextDate returns the next occurrence date for a scheduled transaction.
func (s *Service) GetNextDate(id types.ID) (types.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return types.Date{}, err
	}
	return st.NextDate, nil
}

// CalculateNextDate calculates what the next occurrence would be after the current next_date.
// Does not modify the scheduled transaction.
func (s *Service) CalculateNextDate(id types.ID) (types.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return types.Date{}, err
	}
	return st.CalculateNextDate(), nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validateScheduledTransaction validates a scheduled transaction and returns any validation errors.
func (s *Service) validateScheduledTransaction(st *Transaction) error {
	errors := st.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateScheduledSplits validates a scheduled transaction's child splits
// when present. Enforces:
//   - mutually exclusive shape: scalar category_id on the parent and child
//     splits cannot coexist (a multi-line schedule has no parent category);
//   - the multi-line parent must carry a fixed amount (variable amounts are
//     legacy single-line only — see specs/multiline-splits-and-paycheck.md);
//   - each split passes Split.Validate() (one of category_id /
//     transfer_account_id, non-zero amount, etc.);
//   - transfer-lines cannot target the parent's own account (self-transfer);
//   - the signed sum of split amounts equals the parent's amount.
//
// Returns nil for legacy single-line schedules (no child splits).
func (s *Service) validateScheduledSplits(st *Transaction) error {
	if len(st.Splits) == 0 {
		return nil
	}

	if st.HasCategory() {
		verrs := types.ValidationErrors{}
		verrs.Add("splits",
			"scheduled transaction cannot set both a scalar category_id and child splits")
		return &types.ServiceValidationError{Errors: verrs}
	}

	if !st.HasAmount() {
		verrs := types.ValidationErrors{}
		verrs.Add("amount",
			"multi-line scheduled transaction requires a fixed amount equal to the signed sum of its lines")
		return &types.ServiceValidationError{Errors: verrs}
	}

	for _, split := range st.Splits {
		// Ensure the parent linkage is set so split-level validation
		// doesn't trip on the required scheduled_transaction_id field.
		if split.ScheduledTransactionID.IsNil() {
			split.ScheduledTransactionID = st.ID
		}
		if errs := split.Validate(); errs.HasErrors() {
			return &types.ServiceValidationError{Errors: errs}
		}
		if split.TransferAccountID.Valid && split.TransferAccountID.ID == st.AccountID {
			verrs := types.ValidationErrors{}
			verrs.Add("transfer_account_id",
				"transfer-line cannot target the scheduled transaction's own account (self-transfer)")
			return &types.ServiceValidationError{Errors: verrs}
		}
	}

	if errs := st.Splits.ValidateAgainstTemplate(st.Amount.Money); errs.HasErrors() {
		return &types.ServiceValidationError{Errors: errs}
	}

	return nil
}
