package reconciliation

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Status holds the reconciliation state for an account.
type Status struct {
	ActiveSession        *Session
	LastCompletedSession *Session
	CandidateCount       int
}

// Service provides business logic for reconciliation operations.
type Service struct {
	reconRepo   *Repository
	txnRepo     *transaction.Repository
	accountRepo *account.Repository
	db          *db.DB
	tx          db.Queryer // nil outside a transaction
}

// InTx returns a copy of the service bound to tx, with every repository field
// rebound to the same transaction so all writes join one atomic unit. The
// original service is unchanged and remains safe for non-transactional use.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.reconRepo = s.reconRepo.WithTx(tx)
	c.txnRepo = s.txnRepo.WithTx(tx)
	if s.accountRepo != nil {
		c.accountRepo = s.accountRepo.WithTx(tx)
	}
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not
// query the pool directly — the transaction pins the single connection
// (SetMaxOpenConns(1)), so a pool read inside a tx deadlocks.
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. This is what makes service methods composable
// without savepoints: an outer flow binds once, inner calls join. A bound
// service must never reach db.WithTx (nesting would deadlock the mutex).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}

// NewService creates a new Service.
func NewService(
	reconRepo *Repository,
	txnRepo *transaction.Repository,
	accountRepo *account.Repository,
	database *db.DB,
) *Service {
	return &Service{
		reconRepo:   reconRepo,
		txnRepo:     txnRepo,
		accountRepo: accountRepo,
		db:          database,
	}
}

// StartReconciliation creates a new reconciliation session for an account.
// If an in-progress session already exists, it is replaced.
// Validates that the account exists, is active, and the statement date is not in the future.
func (s *Service) StartReconciliation(accountID types.ID, statementDate types.Date, statementBalance types.Money) (*Session, error) {
	// Validate account exists
	acct, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, err
	}

	// Cannot reconcile a closed account
	if !acct.Active {
		return nil, &account.IsClosedError{ID: accountID.String()}
	}

	// Statement date must not be in the future
	if statementDate.After(types.Today()) {
		return nil, &StatementDateFutureError{}
	}

	// If an in-progress session exists, delete it (replace)
	existing, err := s.reconRepo.GetActiveByAccountID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing session: %w", err)
	}
	if existing != nil {
		if err := s.reconRepo.Delete(existing.ID); err != nil {
			return nil, fmt.Errorf("failed to replace existing session: %w", err)
		}
	}

	// Create new session
	session := NewSession(accountID, statementDate, statementBalance)
	if errors := session.Validate(); errors.HasErrors() {
		return nil, &types.ServiceValidationError{Errors: errors}
	}

	if err := s.reconRepo.Create(session); err != nil {
		return nil, fmt.Errorf("failed to create reconciliation session: %w", err)
	}

	return session, nil
}

// GetCandidateTransactions returns all unreconciled transactions for an account
// dated on or before the statement date. These are transactions with status
// "uncleared" or "cleared" (not reconciled or void).
func (s *Service) GetCandidateTransactions(accountID types.ID, statementDate types.Date) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ?
			AND date <= ?
			AND status IN ('uncleared', 'cleared')
		ORDER BY date ASC, created_at ASC
	`

	rows, err := s.q().Query(query, accountID.String(), statementDate.Time())
	if err != nil {
		return nil, fmt.Errorf("failed to query candidate transactions: %w", err)
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
			&txn.TransferAccountID,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan candidate transaction: %w", err)
		}
		transactions = append(transactions, txn)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating candidate transactions: %w", err)
	}

	return transactions, nil
}

// CalculateClearedTotal computes the cleared total for reconciliation:
// opening_balance + sum(reconciled transactions) + sum(checked transactions).
// The checkedIDs are transaction IDs currently checked by the user.
func (s *Service) CalculateClearedTotal(accountID types.ID, checkedIDs []types.ID) (types.Money, error) {
	// Get opening balance
	var openingBalance types.Money
	err := s.q().QueryRow(
		`SELECT opening_balance FROM accounts WHERE CAST(id AS VARCHAR) = ?`,
		accountID.String(),
	).Scan(&openingBalance)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to get opening balance: %w", err)
	}

	// Get sum of reconciled transactions
	var reconciledSum types.Money
	err = s.q().QueryRow(
		`SELECT COALESCE(SUM(amount), 0) FROM transactions
		 WHERE CAST(account_id AS VARCHAR) = ? AND status = 'reconciled'`,
		accountID.String(),
	).Scan(&reconciledSum)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to sum reconciled transactions: %w", err)
	}

	// Get sum of checked transactions (exclude already-reconciled to avoid double-counting)
	checkedSum := types.ZeroMoney
	if len(checkedIDs) > 0 {
		placeholders := make([]string, len(checkedIDs))
		args := make([]any, len(checkedIDs))
		for i, id := range checkedIDs {
			placeholders[i] = "?"
			args[i] = id.String()
		}

		query := fmt.Sprintf(
			`SELECT COALESCE(SUM(amount), 0) FROM transactions
			 WHERE CAST(id AS VARCHAR) IN (%s) AND status != 'reconciled'`,
			strings.Join(placeholders, ", "),
		)

		err = s.q().QueryRow(query, args...).Scan(&checkedSum)
		if err != nil {
			return types.ZeroMoney, fmt.Errorf("failed to sum checked transactions: %w", err)
		}
	}

	// cleared total = opening_balance + reconciled + checked
	total := openingBalance.Add(reconciledSum).Add(checkedSum)
	return total, nil
}

// FinishReconciliation completes the active reconciliation session for an account.
// All specified transaction IDs are marked as reconciled.
// The difference must be $0.00 unless force is true.
func (s *Service) FinishReconciliation(accountID types.ID, transactionIDs []types.ID, force bool) error {
	// Get active session
	session, err := s.reconRepo.GetActiveByAccountID(accountID)
	if err != nil {
		return fmt.Errorf("failed to get active session: %w", err)
	}
	if session == nil {
		return &NoActiveError{AccountID: accountID.String()}
	}

	// Calculate cleared total with the checked transactions
	clearedTotal, err := s.CalculateClearedTotal(accountID, transactionIDs)
	if err != nil {
		return err
	}

	// Check difference
	difference := session.StatementBalance.Sub(clearedTotal)
	if !difference.IsZero() && !force {
		return &DifferenceError{Difference: difference}
	}

	// Mark all checked transactions reconciled and complete the session in one
	// transaction — a mid-loop failure cannot leave some transactions
	// reconciled with the session still open.
	return s.runInTx(func(b *Service) error {
		for _, txnID := range transactionIDs {
			txn, err := b.txnRepo.GetByID(txnID)
			if err != nil {
				return fmt.Errorf("failed to get transaction %s: %w", txnID.String(), err)
			}

			// Skip if already reconciled
			if txn.IsReconciled() {
				continue
			}

			// Cannot reconcile void transactions
			if txn.IsVoid() {
				return &transaction.IsVoidError{ID: txnID.String()}
			}

			// Status-only change: use the narrow in-place UpdateStatus so DuckDB
			// does not rewrite the row. The full-row Update turns into an internal
			// DELETE+INSERT (an indexed/FK column is in the SET) which aborts if any
			// secondary ART index for the row is desynced on disk — the DuckDB
			// storage bug that broke reconcile-finish on a transfer whose transfer_id
			// index entry could not be deleted. See transaction.Repository.UpdateStatus
			// and migration 030.
			if err := b.txnRepo.UpdateStatus(txnID, transaction.StatusReconciled); err != nil {
				return fmt.Errorf("failed to reconcile transaction %s: %w", txnID.String(), err)
			}
		}

		// Complete the session with a narrow in-place status update (see the
		// UpdateStatus calls above) so the session row is not rewritten either —
		// reconciliation_sessions carries the same desync-prone indexes.
		completedAt := types.NullableTimestamp{Timestamp: types.Now(), Valid: true}
		if err := b.reconRepo.UpdateStatus(session.ID, SessionStatusCompleted, completedAt); err != nil {
			return fmt.Errorf("failed to complete reconciliation session: %w", err)
		}

		return nil
	})
}

// CancelReconciliation cancels the active reconciliation session for an account.
// No transactions are modified.
func (s *Service) CancelReconciliation(accountID types.ID) error {
	session, err := s.reconRepo.GetActiveByAccountID(accountID)
	if err != nil {
		return fmt.Errorf("failed to get active session: %w", err)
	}
	if session == nil {
		return &NoActiveError{AccountID: accountID.String()}
	}

	if err := s.reconRepo.Delete(session.ID); err != nil {
		return fmt.Errorf("failed to cancel reconciliation session: %w", err)
	}

	return nil
}

// GetReconciliationStatus returns the reconciliation state for an account,
// including any active session and the last completed session.
func (s *Service) GetReconciliationStatus(accountID types.ID) (*Status, error) {
	activeSession, err := s.reconRepo.GetActiveByAccountID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	lastCompleted, err := s.reconRepo.GetLastCompletedByAccountID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get last completed session: %w", err)
	}

	candidateCount := 0
	if activeSession != nil {
		candidates, err := s.GetCandidateTransactions(accountID, activeSession.StatementDate)
		if err != nil {
			return nil, fmt.Errorf("failed to count candidate transactions: %w", err)
		}
		candidateCount = len(candidates)
	}

	return &Status{
		ActiveSession:        activeSession,
		LastCompletedSession: lastCompleted,
		CandidateCount:       candidateCount,
	}, nil
}

// GetActiveSession returns the active reconciliation session for an account, or nil if none.
func (s *Service) GetActiveSession(accountID types.ID) (*Session, error) {
	return s.reconRepo.GetActiveByAccountID(accountID)
}

// GetLastCompletedSession returns the most recently completed reconciliation session for an account.
func (s *Service) GetLastCompletedSession(accountID types.ID) (*Session, error) {
	return s.reconRepo.GetLastCompletedByAccountID(accountID)
}

// ReopenSession reverts a completed reconciliation session back to in_progress.
// Used by the undo system to reverse a FinishReconciliation operation.
func (s *Service) ReopenSession(sessionID types.ID) error {
	session, err := s.reconRepo.GetByID(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}

	if !session.IsCompleted() {
		return fmt.Errorf("session %s is not completed; cannot reopen", sessionID.String())
	}

	// Narrow in-place status update (see FinishReconciliation): clears the
	// completed_at and moves the session back to in_progress without rewriting
	// the row.
	return s.reconRepo.UpdateStatus(sessionID, SessionStatusInProgress, types.NullableTimestamp{})
}

// RestoreTransactionStatuses restores transactions to their previous statuses.
// Used by the undo system to reverse transaction status changes from reconciliation.
// Only reconciled transactions can be restored. The whole restore commits in
// one transaction (joining the caller's when bound, as in UndoFinish).
func (s *Service) RestoreTransactionStatuses(statuses map[types.ID]transaction.Status) error {
	return s.runInTx(func(b *Service) error {
		for txnID, prevStatus := range statuses {
			txn, err := b.txnRepo.GetByID(txnID)
			if err != nil {
				return fmt.Errorf("failed to get transaction %s: %w", txnID.String(), err)
			}

			if !txn.IsReconciled() {
				continue
			}

			// Status-only change: narrow in-place update (see FinishReconciliation).
			if err := b.txnRepo.UpdateStatus(txnID, prevStatus); err != nil {
				return fmt.Errorf("failed to restore transaction %s status: %w", txnID.String(), err)
			}
		}

		return nil
	})
}

// UndoFinish reverses a completed FinishReconciliation atomically: the
// transactions' previous statuses and the session reopen commit in one
// transaction, so a mid-failure cannot leave statuses restored while the
// session stays completed (or vice versa). Counterpart of the atomic
// FinishReconciliation; used by the undo system.
func (s *Service) UndoFinish(sessionID types.ID, statuses map[types.ID]transaction.Status) error {
	return s.runInTx(func(b *Service) error {
		if err := b.RestoreTransactionStatuses(statuses); err != nil {
			return err
		}
		return b.ReopenSession(sessionID)
	})
}
