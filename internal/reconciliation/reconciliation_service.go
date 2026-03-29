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

	rows, err := s.db.Conn().Query(query, accountID.String(), statementDate.Time())
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
	err := s.db.Conn().QueryRow(
		`SELECT opening_balance FROM accounts WHERE CAST(id AS VARCHAR) = ?`,
		accountID.String(),
	).Scan(&openingBalance)
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to get opening balance: %w", err)
	}

	// Get sum of reconciled transactions
	var reconciledSum types.Money
	err = s.db.Conn().QueryRow(
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

		err = s.db.Conn().QueryRow(query, args...).Scan(&checkedSum)
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

	// Mark all checked transactions as reconciled
	for _, txnID := range transactionIDs {
		txn, err := s.txnRepo.GetByID(txnID)
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

		txn.Reconcile()
		if err := s.txnRepo.Update(txn); err != nil {
			return fmt.Errorf("failed to reconcile transaction %s: %w", txnID.String(), err)
		}
	}

	// Complete the session
	session.Complete()
	if err := s.reconRepo.Update(session); err != nil {
		return fmt.Errorf("failed to complete reconciliation session: %w", err)
	}

	return nil
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

	session.Status = SessionStatusInProgress
	session.CompletedAt = types.NullableTimestamp{}
	session.Touch()

	return s.reconRepo.Update(session)
}

// RestoreTransactionStatuses restores transactions to their previous statuses.
// Used by the undo system to reverse transaction status changes from reconciliation.
// Only reconciled transactions can be restored.
func (s *Service) RestoreTransactionStatuses(statuses map[types.ID]transaction.Status) error {
	for txnID, prevStatus := range statuses {
		txn, err := s.txnRepo.GetByID(txnID)
		if err != nil {
			return fmt.Errorf("failed to get transaction %s: %w", txnID.String(), err)
		}

		if !txn.IsReconciled() {
			continue
		}

		txn.SetStatus(prevStatus)
		if err := s.txnRepo.Update(txn); err != nil {
			return fmt.Errorf("failed to restore transaction %s status: %w", txnID.String(), err)
		}
	}

	return nil
}
