package scheduled

import (
	"errors"
	"fmt"
	"slices"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for scheduled transaction operations.
type Service struct {
	repo        *Repository
	txnRepo     *transaction.Repository
	txnSvc      *transaction.Service
	accountRepo *account.Repository
	db          *db.DB
}

// NewService creates a new Service.
//
// txnSvc may be nil for legacy single-line use; posting a multi-line
// scheduled transaction requires a non-nil txnSvc so the multi-line create
// path (including paired transfer counterparts) can be delegated.
//
// accountRepo backs the closed-account freeze gate: a schedule may not be
// created against, or posted into, a closed account.
func NewService(
	repo *Repository,
	txnRepo *transaction.Repository,
	txnSvc *transaction.Service,
	database *db.DB,
	accountRepo *account.Repository,
) *Service {
	return &Service{
		repo:        repo,
		txnRepo:     txnRepo,
		txnSvc:      txnSvc,
		accountRepo: accountRepo,
		db:          database,
	}
}

// referencedAccountIDs returns every account a schedule touches: its source
// account, its single-line transfer destination, and every transfer-line
// split target.
func referencedAccountIDs(st *Transaction) []types.ID {
	ids := []types.ID{st.AccountID}
	if st.IsTransfer() {
		ids = append(ids, st.TransferAccountID.ID)
	}
	for _, sp := range st.Splits {
		if sp.TransferAccountID.Valid {
			ids = append(ids, sp.TransferAccountID.ID)
		}
	}
	return ids
}

// ensureNoClosedAccounts rejects a schedule that references any closed account
// (source, transfer destination, or transfer-line split target). Nil-tolerant
// for fixtures constructed without an accountRepo; production always wires one.
func (s *Service) ensureNoClosedAccounts(st *Transaction) error {
	if s.accountRepo == nil {
		return nil
	}
	for _, id := range referencedAccountIDs(st) {
		acct, err := s.accountRepo.GetByID(id)
		if err != nil {
			return fmt.Errorf("failed to load account for closed check: %w", err)
		}
		if acct.IsClosed() {
			return &ClosedAccountError{ID: id.String()}
		}
	}
	return nil
}

// ListReferencing returns every schedule that references the given account as
// its source, single-line transfer destination, or a transfer-line split
// target. It backs the soft warning shown when closing an account.
func (s *Service) ListReferencing(accountID types.ID) ([]*Transaction, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	var out []*Transaction
	for _, st := range all {
		if slices.Contains(referencedAccountIDs(st), accountID) {
			out = append(out, st)
		}
	}
	return out, nil
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
	if err := s.ensureNoClosedAccounts(st); err != nil {
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

// HealNextDates corrects rows whose NextDate precedes StartDate. Older
// binaries updated StartDate without syncing NextDate; this normalizes
// any poisoned rows by setting NextDate := StartDate. Intended to run
// once on file open. Returns the count of rows healed.
func (s *Service) HealNextDates() (int, error) {
	return s.repo.HealNextDates()
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

		// Skip — don't error the batch — a schedule that references a closed
		// account; it is left due and unadvanced (decision 8).
		if cerr := s.ensureNoClosedAccounts(st); cerr != nil {
			var closedErr *ClosedAccountError
			if !errors.As(cerr, &closedErr) {
				return nil, cerr
			}
			result.Skipped = true
			result.SkipReason = "references a closed account"
		}

		// Post all overdue occurrences for this scheduled transaction
		for !result.Skipped && !st.IsCompleted() && s.isAutoPostDue(st, today) {
			var txn *transaction.Transaction
			if st.IsTransfer() {
				// Single-line transfer: post template values exactly as a clean
				// linked transfer pair.
				if s.txnSvc == nil {
					return nil, fmt.Errorf("transfer auto-post requires a transaction service; scheduled.NewService was called with txnSvc=nil")
				}
				magnitude := st.Amount.Money.Abs()
				pair, err := s.txnSvc.CreateTransfer(st.AccountID, st.TransferAccountID.ID, st.NextDate, magnitude)
				if err != nil {
					return nil, fmt.Errorf("failed to create transfer auto-post transaction: %w", err)
				}
				if st.Memo.Valid && st.Memo.String != "" {
					if err := s.txnSvc.UpdateTransfer(pair.FromTransaction.TransferID.ID, st.NextDate, magnitude, st.Memo.String, transaction.StatusUncleared); err != nil {
						return nil, fmt.Errorf("transfer auto-post created but failed to set memo: %w", err)
					}
				}
				txn = pair.FromTransaction
			} else if len(st.Splits) > 0 {
				// Multi-line schedule: delegate to the multi-line create path
				// so transfer-line counterparts are minted and persisted.
				if s.txnSvc == nil {
					return nil, fmt.Errorf("multi-line auto-post requires a transaction service; scheduled.NewService was called with txnSvc=nil")
				}
				built, err := s.buildMultiLineTransaction(st, st.NextDate)
				if err != nil {
					// A loan-recompute failure (paid off, negative-am, missing
					// APR, missing interest line) skips this schedule with a
					// reason — it must never abort the rest of the batch.
					if isLoanComputationError(err) {
						result.Skipped = true
						result.SkipReason = loanSkipReason(err)
						// A paid-off loan is terminal: mark it completed so it
						// stops surfacing as due.
						if errors.Is(err, ErrLoanPaidOff) {
							st.MarkCompleted()
						}
						break
					}
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
			// Payoff completion: a loan-shaped schedule whose balance reached
			// zero (e.g. a clamped final payment) is marked completed, which
			// also stops the loop.
			if err := s.finalizeLoanPayoff(st); err != nil {
				return nil, fmt.Errorf("auto-post created transaction but failed to finalize loan payoff: %w", err)
			}
		}

		// Persist schedule mutations. A posted occurrence advanced the schedule;
		// a paid-off skip marked it completed. Other skips (closed account,
		// no-estimate, non-terminal loan errors) leave the schedule untouched
		// and due, so they are not persisted.
		if len(result.Transactions) > 0 || (result.Skipped && st.IsCompleted()) {
			if err := s.repo.Update(st); err != nil {
				return nil, fmt.Errorf("failed to update schedule after auto-post: %w", err)
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
	txn, _, err := s.PostReturningSplits(id, amount)
	return txn, err
}

// PostReturningSplits behaves exactly like Post but also returns the child
// splits it persisted (nil for single-line schedules). The undo command for a
// plain post uses it to capture and replay the exact posted rows on redo:
// loan-shaped schedules recompute interest/principal from the loan's live
// balance, which can change between the original post and a later redo, so a
// redo that re-ran Post would produce different rows. Store-and-replay keeps
// redo deterministic.
func (s *Service) PostReturningSplits(id types.ID, amount *types.Money) (*transaction.Transaction, []*transaction.Split, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, nil, &CompletedError{ID: id.String()}
	}

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, nil, err
	}

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		return s.postMultiLine(st, st.NextDate)
	}

	txn, err := s.postSingleLine(st, st.NextDate, amount)
	return txn, nil, err
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

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, err
	}

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		txn, _, err := s.postMultiLine(st, date)
		return txn, err
	}

	return s.postSingleLine(st, date, amount)
}

// postSingleLine creates a single-line transaction from a legacy scheduled
// template and advances the schedule. The returned transaction is the
// newly-created real transaction.
func (s *Service) postSingleLine(st *Transaction, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	// Single-line transfer schedules create a clean linked transfer pair.
	if st.IsTransfer() {
		return s.postSingleLineTransfer(st, date, amount)
	}

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

// postSingleLineTransfer creates a clean linked transfer pair from a
// single-line transfer schedule (account_id = From, transfer_account_id = To)
// and advances the schedule. The schedule stores the amount as the signed
// effect on the source account (negative); CreateTransfer wants a positive
// magnitude, so the amount is taken as an absolute value. An optional override
// (the post-time preview's edited amount) replaces the stored estimate for
// this one occurrence without changing the template.
func (s *Service) postSingleLineTransfer(st *Transaction, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	if s.txnSvc == nil {
		return nil, fmt.Errorf("posting a transfer schedule requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}

	var magnitude types.Money
	switch {
	case amount != nil:
		magnitude = amount.Abs()
	case st.HasAmount():
		magnitude = st.Amount.Money.Abs()
	default:
		return nil, &AmountRequiredError{ID: st.ID.String()}
	}

	pair, err := s.txnSvc.CreateTransfer(st.AccountID, st.TransferAccountID.ID, date, magnitude)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduled transfer: %w", err)
	}

	// Carry the schedule's memo onto both legs (CreateTransfer takes no memo).
	if st.Memo.Valid && st.Memo.String != "" {
		transferID := pair.FromTransaction.TransferID.ID
		if err := s.txnSvc.UpdateTransfer(transferID, date, magnitude, st.Memo.String, transaction.StatusUncleared); err != nil {
			return pair.FromTransaction, fmt.Errorf("transfer created but failed to set memo: %w", err)
		}
		// Reflect the persisted memo on the returned (in-memory) From leg.
		pair.FromTransaction.SetMemo(st.Memo.String)
	}

	st.AdvanceSchedule()
	if err := s.repo.Update(st); err != nil {
		return pair.FromTransaction, fmt.Errorf("transfer created but failed to update schedule: %w", err)
	}

	return pair.FromTransaction, nil
}

// postMultiLine creates a multi-line real transaction from a multi-line
// scheduled template by delegating to transaction.Service.CreateWithSplits
// (which mints fresh TransferIDs and creates paired counterparts for any
// transfer-line splits). The template's children are left in place and the
// schedule advances by one cadence.
func (s *Service) postMultiLine(st *Transaction, date types.Date) (*transaction.Transaction, []*transaction.Split, error) {
	if s.txnSvc == nil {
		return nil, nil, fmt.Errorf("multi-line scheduled posting requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}

	built, err := s.buildMultiLineTransaction(st, date)
	if err != nil {
		// A loan already paid off at post time is a terminal state: refuse the
		// post and mark the schedule completed on the spot, so an ad-hoc payoff
		// transfer cannot strand a never-postable due schedule.
		if errors.Is(err, ErrLoanPaidOff) {
			st.MarkCompleted()
			_ = s.repo.Update(st)
		}
		return nil, nil, err
	}

	if err := s.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
		return nil, nil, fmt.Errorf("failed to create multi-line transaction: %w", err)
	}

	st.AdvanceSchedule()
	if err := s.finalizeLoanPayoff(st); err != nil {
		return built.parent, built.splits, fmt.Errorf("transaction created but failed to finalize loan payoff: %w", err)
	}
	if err := s.repo.Update(st); err != nil {
		return built.parent, built.splits, fmt.Errorf("transaction created but failed to update schedule: %w", err)
	}
	return built.parent, built.splits, nil
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
	// Loan-shaped schedules recompute their interest/principal split from the
	// loan's live balance at posting time; the stored template is never trusted.
	if s.isLoanShaped(st) {
		return s.buildLoanTransaction(st, date)
	}

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

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, err
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
	// Payoff completion applies on every posting path, including this flagship
	// manual-preview flow: a penny-tweaked edit that brings the loan to (or
	// past) zero marks the schedule completed.
	if err := s.finalizeLoanPayoff(st); err != nil {
		return txn, fmt.Errorf("transaction created but failed to finalize loan payoff: %w", err)
	}
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
