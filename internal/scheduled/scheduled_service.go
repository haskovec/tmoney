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
	txnSvc  *transaction.Service
	db      *db.DB
}

// NewService creates a new Service.
//
// txnSvc may be nil for legacy single-line use; posting a multi-line
// scheduled transaction requires a non-nil txnSvc so the multi-line create
// path (including paired transfer counterparts) can be delegated.
func NewService(
	repo *Repository,
	txnRepo *transaction.Repository,
	txnSvc *transaction.Service,
	database *db.DB,
) *Service {
	return &Service{
		repo:    repo,
		txnRepo: txnRepo,
		txnSvc:  txnSvc,
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
	// NextDate is the date of the next pending occurrence; it must never
	// precede StartDate. When the user shifts StartDate forward past the
	// current NextDate, advance NextDate with it so the schedule list and
	// due-detection see the new anchor. Backward StartDate shifts leave
	// NextDate alone — an in-progress schedule shouldn't roll back to its
	// origin just because the user corrected the recorded anchor.
	if st.NextDate.Before(st.StartDate) {
		st.NextDate = st.StartDate
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
			var txn *transaction.Transaction
			if len(st.Splits) > 0 {
				// Multi-line schedule: delegate to the multi-line create path
				// so transfer-line counterparts are minted and persisted.
				if s.txnSvc == nil {
					return nil, fmt.Errorf("multi-line auto-post requires a transaction service; scheduled.NewService was called with txnSvc=nil")
				}
				built, err := s.buildMultiLineTransaction(st, st.NextDate)
				if err != nil {
					return nil, fmt.Errorf("failed to build multi-line auto-post transaction: %w", err)
				}
				if err := s.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
					return nil, fmt.Errorf("failed to create multi-line auto-post transaction: %w", err)
				}
				txn = built.parent
			} else {
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
				txn = transaction.NewTransaction(st.AccountID, st.NextDate, txnAmount)

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

				if err := s.txnRepo.Create(txn); err != nil {
					return nil, fmt.Errorf("failed to create auto-post transaction: %w", err)
				}
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
// For multi-line scheduled transactions (with child Splits), the amount override is
// not supported — per-instance amount edits go through the post-time preview dialog.
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

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		return s.postMultiLine(st, st.NextDate)
	}

	return s.postSingleLine(st, st.NextDate, amount)
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

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		return s.postMultiLine(st, date)
	}

	return s.postSingleLine(st, date, amount)
}

// postSingleLine creates a single-line transaction from a legacy scheduled
// template and advances the schedule. The returned transaction is the
// newly-created real transaction.
func (s *Service) postSingleLine(st *Transaction, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	// Determine the amount to use
	var txnAmount types.Money
	switch {
	case amount != nil:
		txnAmount = *amount
	case st.HasAmount():
		txnAmount = st.Amount.Money
	default:
		// Variable amount with no amount provided - try to estimate
		estimated, err := s.EstimateAmount(st.ID)
		if err != nil {
			return nil, &AmountRequiredError{ID: st.ID.String()}
		}
		if estimated == nil {
			return nil, &AmountRequiredError{ID: st.ID.String()}
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

	if err := s.txnRepo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Advance the schedule. If AdvanceSchedule returns false the series is
	// complete; either way we persist the updated state below.
	st.AdvanceSchedule()

	if err := s.repo.Update(st); err != nil {
		// Transaction was created but schedule update failed
		// This is a partial success - log the error but return the transaction
		return txn, fmt.Errorf("transaction created but failed to update schedule: %w", err)
	}

	return txn, nil
}

// postMultiLine creates a multi-line real transaction from a multi-line
// scheduled template by delegating to transaction.Service.CreateWithSplits
// (which mints fresh TransferIDs and creates paired counterparts for any
// transfer-line splits). The template's children are left in place and the
// schedule advances by one cadence.
func (s *Service) postMultiLine(st *Transaction, date types.Date) (*transaction.Transaction, error) {
	if s.txnSvc == nil {
		return nil, fmt.Errorf("multi-line scheduled posting requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}

	built, err := s.buildMultiLineTransaction(st, date)
	if err != nil {
		return nil, err
	}

	if err := s.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
		return nil, fmt.Errorf("failed to create multi-line transaction: %w", err)
	}

	st.AdvanceSchedule()
	if err := s.repo.Update(st); err != nil {
		return built.parent, fmt.Errorf("transaction created but failed to update schedule: %w", err)
	}
	return built.parent, nil
}

// builtMultiLineTransaction holds the parent transaction and child splits
// assembled from a multi-line template, ready for CreateWithSplits.
type builtMultiLineTransaction struct {
	parent *transaction.Transaction
	splits []*transaction.Split
}

// buildMultiLineTransaction translates a multi-line scheduled template into a
// transaction + splits payload suitable for transaction.Service.CreateWithSplits.
// The parent transaction inherits the template's account, payee, and memo
// (but never a scalar category — multi-line parents must have no category).
// Transfer-line template entries become transfer-line splits with no
// TransferID; the transaction service mints one per call when persisting.
func (s *Service) buildMultiLineTransaction(st *Transaction, date types.Date) (*builtMultiLineTransaction, error) {
	parent := transaction.NewTransaction(st.AccountID, date, st.Amount.Money)
	if st.HasPayee() {
		parent.SetPayee(st.PayeeID.ID)
	}
	if st.Memo.Valid && st.Memo.String != "" {
		parent.SetMemo(st.Memo.String)
	}

	splits := make([]*transaction.Split, 0, len(st.Splits))
	for _, tmpl := range st.Splits {
		var ts *transaction.Split
		if tmpl.TransferAccountID.Valid {
			// Transfer-line: leave CategoryID zero; service mints TransferID.
			ts = &transaction.Split{
				BaseModel:         types.NewBaseModel(),
				TransactionID:     parent.ID,
				Amount:            tmpl.Amount,
				TransferAccountID: types.NullableID{ID: tmpl.TransferAccountID.ID, Valid: true},
			}
		} else {
			ts = transaction.NewSplit(parent.ID, tmpl.CategoryID.ID, tmpl.Amount)
		}
		if tmpl.Memo.Valid && tmpl.Memo.String != "" {
			ts.SetMemo(tmpl.Memo.String)
		}
		splits = append(splits, ts)
	}

	return &builtMultiLineTransaction{parent: parent, splits: splits}, nil
}

// PostWithEdits creates a real transaction using a caller-supplied
// parent (and, for multi-line schedules, splits) and advances the
// schedule by one cadence. Unlike Post, this entry point lets the
// caller carry per-instance edits — e.g. a date or memo change made
// in the post-time preview dialog — without those edits leaking into
// the stored template.
//
// The parent transaction and splits must already reflect the user's
// edits (date / payee / category / amount / memo / status for single-
// line; header + line edits for multi-line). For multi-line callers,
// transfer-line splits carry only TransferAccountID; the underlying
// transaction service mints fresh TransferIDs and creates paired
// counterparts in the target accounts.
//
// Schedule advancement uses the *template's* original next_date as
// the basis (st.AdvanceSchedule calls st.CalculateNextDate over
// st.NextDate before the user's edits are applied), so a one-off
// date edit in the preview never shifts the schedule's cadence.
//
// Returns the persisted parent transaction.
func (s *Service) PostWithEdits(id types.ID, txn *transaction.Transaction, splits []*transaction.Split) (*transaction.Transaction, error) {
	if s.txnSvc == nil {
		return nil, fmt.Errorf("PostWithEdits requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}
	if txn == nil {
		return nil, fmt.Errorf("PostWithEdits requires a non-nil parent transaction")
	}

	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if st.IsCompleted() {
		return nil, &CompletedError{ID: id.String()}
	}

	if len(splits) > 0 {
		// Stamp the parent transaction's ID onto every split. Callers
		// (e.g. the TUI preview dialog) typically build splits via
		// SplitDialog.buildSplits, which leaves TransactionID zero
		// because the parent isn't constructed at that point; the
		// transaction service's validation requires it.
		for _, sp := range splits {
			if sp != nil {
				sp.TransactionID = txn.ID
			}
		}
		if err := s.txnSvc.CreateWithSplits(txn, splits); err != nil {
			return nil, fmt.Errorf("failed to create transaction: %w", err)
		}
	} else {
		if err := s.txnSvc.Create(txn); err != nil {
			return nil, fmt.Errorf("failed to create transaction: %w", err)
		}
	}

	st.AdvanceSchedule()
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
