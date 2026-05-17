package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for transaction operations.
type Service struct {
	txnRepo      *Repository
	splitRepo    *SplitRepository
	transferRepo *TransferRepository
	payeeRepo    *payee.Repository
	db           *db.DB
}

// NewService creates a new Service.
func NewService(
	txnRepo *Repository,
	splitRepo *SplitRepository,
	transferRepo *TransferRepository,
	payeeRepo *payee.Repository,
	database *db.DB,
) *Service {
	return &Service{
		txnRepo:      txnRepo,
		splitRepo:    splitRepo,
		transferRepo: transferRepo,
		payeeRepo:    payeeRepo,
		db:           database,
	}
}

// =============================================================================
// Transaction CRUD Operations
// =============================================================================

// Create validates and creates a new transaction.
// If the transaction has a payee with a default category and no category is set,
// the default category will be auto-populated.
func (s *Service) Create(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	// Auto-populate category from payee's default if not set
	if !transaction.HasCategory() && transaction.HasPayee() {
		if err := s.applyPayeeDefaultCategory(transaction); err != nil {
			return err
		}
	}

	return s.txnRepo.Create(transaction)
}

// GetByID retrieves a transaction by its ID.
func (s *Service) GetByID(id types.ID) (*Transaction, error) {
	return s.txnRepo.GetByID(id)
}

// Update validates and updates an existing transaction.
// For transfers, use UpdateTransfer to update both sides.
// Void and reconciled transactions cannot be edited.
func (s *Service) Update(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	// Check existing transaction state
	existing, err := s.txnRepo.GetByID(transaction.ID)
	if err != nil {
		return err
	}

	// Prevent editing void transactions
	if existing.IsVoid() {
		return &IsVoidError{ID: transaction.ID.String()}
	}

	// Prevent editing reconciled transactions
	if existing.IsReconciled() {
		return &IsReconciledError{ID: transaction.ID.String()}
	}

	// Check if this is a transfer - caller should use UpdateTransfer
	if existing.IsTransfer() && !transaction.IsTransfer() {
		return &IsTransferError{ID: transaction.ID.String()}
	}

	return s.txnRepo.Update(transaction)
}

// Delete removes a transaction.
// For transfers, this will delete both sides after confirmation.
// Void and reconciled transactions cannot be deleted.
func (s *Service) Delete(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Prevent deleting void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Prevent deleting reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// If this is a transfer, check both sides and delete
	if txn.IsTransfer() {
		pair, err := s.transferRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if pair.FromTransaction.IsReconciled() {
			return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
		}
		if pair.ToTransaction.IsReconciled() {
			return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
		}
		return s.transferRepo.Delete(txn.TransferID.ID)
	}

	// Delete any splits first
	if _, err := s.splitRepo.DeleteByTransaction(id); err != nil {
		return fmt.Errorf("failed to delete splits: %w", err)
	}

	return s.txnRepo.Delete(id)
}

// List returns all transactions ordered by date descending.
func (s *Service) List() ([]*Transaction, error) {
	return s.txnRepo.List()
}

// ListByAccount returns all transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.txnRepo.ListByAccount(accountID)
}

// ListByDateRange returns all transactions within a date range.
func (s *Service) ListByDateRange(startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByDateRange(startDate, endDate)
}

// ListByAccountAndDateRange returns transactions for an account within a date range.
func (s *Service) ListByAccountAndDateRange(accountID types.ID, startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByAccountAndDateRange(accountID, startDate, endDate)
}

// Search finds transactions matching the given criteria.
// Multiple criteria are combined with AND logic.
func (s *Service) Search(criteria SearchCriteria) ([]*Transaction, error) {
	return s.txnRepo.Search(criteria)
}

// SearchByPayee finds transactions by partial payee name match (case-insensitive).
func (s *Service) SearchByPayee(payeeName string) ([]*Transaction, error) {
	return s.txnRepo.SearchByPayee(payeeName)
}

// SearchByMemo finds transactions by partial memo match (case-insensitive).
func (s *Service) SearchByMemo(memo string) ([]*Transaction, error) {
	return s.txnRepo.SearchByMemo(memo)
}

// SearchByCategory finds transactions by partial category name match (case-insensitive).
func (s *Service) SearchByCategory(categoryName string) ([]*Transaction, error) {
	return s.txnRepo.SearchByCategory(categoryName)
}

// =============================================================================
// Split Transaction Operations
// =============================================================================

// CreateWithSplits creates a transaction along with its splits. The signed
// sum of split amounts must equal the transaction amount. For each
// transfer-typed split-line (TransferAccountID set), the service mints a
// fresh TransferID, stores it on the split-item, and creates a paired
// single-line transaction in the target account with the inverted amount
// and matching transfer_id.
func (s *Service) CreateWithSplits(transaction *Transaction, splits []*Split) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	if err := s.validateSplits(transaction, splits); err != nil {
		return err
	}

	if transaction.HasCategory() && len(splits) > 0 {
		return &HasSplitsError{ID: transaction.ID.String()}
	}

	// Mint transfer_ids for transfer-lines so the split row and its paired
	// counter-transaction share the link from the first write.
	for _, split := range splits {
		if split.TransferAccountID.Valid && !split.TransferID.Valid {
			split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
		}
	}

	if err := s.txnRepo.Create(transaction); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Track paired-counter-transactions so we can roll them back on a later
	// failure.
	createdPairs := make([]types.ID, 0, len(splits))

	for _, split := range splits {
		split.TransactionID = transaction.ID
		if err := s.splitRepo.Create(split); err != nil {
			s.rollbackCreateWithSplits(transaction.ID, createdPairs)
			return fmt.Errorf("failed to create split: %w", err)
		}

		if !split.TransferAccountID.Valid {
			continue
		}

		paired := NewTransaction(split.TransferAccountID.ID, transaction.Date, split.Amount.Neg())
		paired.SetTransfer(split.TransferID.ID, transaction.AccountID)
		if err := s.txnRepo.Create(paired); err != nil {
			s.rollbackCreateWithSplits(transaction.ID, createdPairs)
			return fmt.Errorf("failed to create paired transfer transaction: %w", err)
		}
		createdPairs = append(createdPairs, paired.ID)
	}

	return nil
}

// rollbackCreateWithSplits best-effort removes paired counter-transactions
// and the parent transaction (which cascades its splits) after a partial
// CreateWithSplits failure.
func (s *Service) rollbackCreateWithSplits(parentID types.ID, pairedIDs []types.ID) {
	for _, id := range pairedIDs {
		_ = s.txnRepo.Delete(id)
	}
	_, _ = s.splitRepo.DeleteByTransaction(parentID)
	_ = s.txnRepo.Delete(parentID)
}

// GetSplits returns all splits for a transaction.
func (s *Service) GetSplits(transactionID types.ID) ([]*Split, error) {
	return s.splitRepo.ListByTransaction(transactionID)
}

// AddSplit adds a new split to an existing transaction.
// After adding, the splits must still sum to the transaction amount.
// Void and reconciled transactions cannot have splits added.
func (s *Service) AddSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	// Get the transaction
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	// Transfers cannot have splits
	if txn.IsTransfer() {
		return &TransferCannotHaveSplitsError{ID: txn.ID.String()}
	}

	// Create the split
	if err := s.splitRepo.Create(split); err != nil {
		return err
	}

	// Validate total (warning only - the user may be adding more splits)
	valid, err := s.splitRepo.ValidateSplitsAgainstTransaction(split.TransactionID)
	if err != nil {
		return fmt.Errorf("failed to validate splits: %w", err)
	}
	if !valid {
		// Return a specific error so caller knows splits don't match
		total, _ := s.splitRepo.GetTotalByTransaction(split.TransactionID)
		return &SplitTotalMismatchError{
			TransactionID:     split.TransactionID.String(),
			TransactionAmount: txn.Amount,
			SplitTotal:        total,
		}
	}

	return nil
}

// UpdateSplit updates an existing split.
// Splits on void or reconciled transactions cannot be updated.
//
// For transfer-line splits, the paired single-line counter-transaction in
// the target account is kept in sync:
//   - An amount edit mirrors the new (negated) amount onto the paired side.
//   - A target-account edit deletes the old paired side and creates a new
//     one in the new target with a fresh transfer_id.
//
// Self-transfers (transfer_account_id == parent.account_id) are rejected.
func (s *Service) UpdateSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	existing, err := s.splitRepo.GetByID(split.ID)
	if err != nil {
		return err
	}

	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if split.TransferAccountID.Valid && split.TransferAccountID.ID == txn.AccountID {
		errors := types.ValidationErrors{}
		errors.Add("transfer_account_id",
			(&SelfTransferError{AccountID: txn.AccountID.String()}).Error())
		return &types.ServiceValidationError{Errors: errors}
	}

	wasTransfer := existing.TransferAccountID.Valid
	isTransfer := split.TransferAccountID.Valid

	if wasTransfer && isTransfer {
		targetMoved := existing.TransferAccountID.ID != split.TransferAccountID.ID
		if targetMoved {
			return s.moveTransferLine(txn, existing, split)
		}
		// Preserve the linkage so callers may omit transfer_id on edits.
		if !split.TransferID.Valid && existing.TransferID.Valid {
			split.TransferID = existing.TransferID
		}
		if err := s.splitRepo.Update(split); err != nil {
			return err
		}
		if existing.TransferID.Valid && !existing.Amount.Equal(split.Amount) {
			return s.updatePairedAmount(existing.TransferID.ID, split.Amount.Neg())
		}
		return nil
	}

	return s.splitRepo.Update(split)
}

// moveTransferLine handles the target-account-change cascade: delete the old
// paired counter-transaction, mint a fresh transfer_id on the split-line,
// persist the split, and create a new paired counterpart in the new target.
func (s *Service) moveTransferLine(parent *Transaction, existing, split *Split) error {
	if existing.TransferID.Valid {
		oldPaired, err := s.findPairedByTransferID(existing.TransferID.ID)
		if err != nil {
			return err
		}
		if oldPaired != nil {
			if oldPaired.IsReconciled() {
				return &IsReconciledError{ID: oldPaired.ID.String()}
			}
			if err := s.txnRepo.Delete(oldPaired.ID); err != nil {
				return fmt.Errorf("failed to delete old paired transaction: %w", err)
			}
		}
	}

	split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
	if err := s.splitRepo.Update(split); err != nil {
		return err
	}

	paired := NewTransaction(split.TransferAccountID.ID, parent.Date, split.Amount.Neg())
	paired.SetTransfer(split.TransferID.ID, parent.AccountID)
	if err := s.txnRepo.Create(paired); err != nil {
		return fmt.Errorf("failed to create new paired transaction: %w", err)
	}
	return nil
}

// findPairedByTransferID returns the paired single-line counter-transaction
// for a transfer-line's transfer_id, or nil if none exists. The parent
// (multi-line) transaction is not marked as a transfer, so only the paired
// counterpart lives on the transactions table with that transfer_id.
func (s *Service) findPairedByTransferID(transferID types.ID) (*Transaction, error) {
	matches, err := s.txnRepo.ListByTransferID(transferID)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return matches[0], nil
}

// updatePairedAmount sets the paired counter-transaction's amount to mirror a
// transfer-line amount edit. A reconciled paired side blocks the cascade.
func (s *Service) updatePairedAmount(transferID types.ID, newAmount types.Money) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired == nil {
		return nil
	}
	if paired.IsReconciled() {
		return &IsReconciledError{ID: paired.ID.String()}
	}
	paired.Amount = newAmount
	return s.txnRepo.Update(paired)
}

// DeleteSplit removes a split from a transaction.
// Splits on void or reconciled transactions cannot be deleted.
func (s *Service) DeleteSplit(splitID types.ID) error {
	// Get the split to find its parent transaction
	split, err := s.splitRepo.GetByID(splitID)
	if err != nil {
		return err
	}

	// Check the parent transaction status
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	return s.splitRepo.Delete(splitID)
}

// ReplaceSplits replaces all splits for a transaction with new ones.
// The new splits must sum to the transaction amount.
// Void and reconciled transactions cannot have splits replaced.
func (s *Service) ReplaceSplits(transactionID types.ID, splits []*Split) error {
	// Get the transaction
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	// Validate splits sum to transaction amount
	if err := s.validateSplits(txn, splits); err != nil {
		return err
	}

	// Delete existing splits
	if _, err := s.splitRepo.DeleteByTransaction(transactionID); err != nil {
		return fmt.Errorf("failed to delete existing splits: %w", err)
	}

	// Create new splits
	for _, split := range splits {
		split.TransactionID = transactionID
		if err := s.splitRepo.Create(split); err != nil {
			return fmt.Errorf("failed to create split: %w", err)
		}
	}

	return nil
}

// ValidateSplitTotals checks if the splits for a transaction sum to the transaction amount.
func (s *Service) ValidateSplitTotals(transactionID types.ID) (bool, error) {
	return s.splitRepo.ValidateSplitsAgainstTransaction(transactionID)
}

// =============================================================================
// Transfer Operations
// =============================================================================

// CreateTransfer creates a linked transfer between two accounts.
// This creates two transactions: one debit in the from account, one credit in the to account.
func (s *Service) CreateTransfer(fromAccountID, toAccountID types.ID, date types.Date, amount types.Money) (*TransferPair, error) {
	// Validate amount is positive
	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Create the transfer pair
	pair := NewTransferPair(fromAccountID, toAccountID, date, amount)

	// Validate the pair
	if errors := pair.Validate(); errors.HasErrors() {
		return nil, &types.ServiceValidationError{Errors: errors}
	}

	// Create both transactions
	if err := s.transferRepo.Create(pair); err != nil {
		return nil, err
	}

	return pair, nil
}

// GetTransferPair retrieves both sides of a transfer by the transfer ID.
func (s *Service) GetTransferPair(transferID types.ID) (*TransferPair, error) {
	return s.transferRepo.GetByTransferID(transferID)
}

// GetTransferCounterpart retrieves the other transaction in a transfer pair.
func (s *Service) GetTransferCounterpart(transactionID types.ID) (*Transaction, error) {
	return s.transferRepo.GetOtherSide(transactionID)
}

// UpdateTransfer updates both sides of a transfer.
// Only amount, date, memo, and status can be updated.
// Reconciled transfers cannot be edited.
func (s *Service) UpdateTransfer(transferID types.ID, date types.Date, amount types.Money, memo string, status Status) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Prevent editing reconciled transfers
	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}

	// Prevent editing void transfers
	if pair.FromTransaction.IsVoid() {
		return &IsVoidError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsVoid() {
		return &IsVoidError{ID: pair.ToTransaction.ID.String()}
	}

	// Update common fields on both sides
	pair.FromTransaction.Date = date
	pair.ToTransaction.Date = date

	pair.FromTransaction.Amount = amount.Neg()
	pair.ToTransaction.Amount = amount.Abs()

	pair.FromTransaction.SetMemo(memo)
	pair.ToTransaction.SetMemo(memo)

	pair.FromTransaction.SetStatus(status)
	pair.ToTransaction.SetStatus(status)

	return s.transferRepo.Update(pair)
}

// UpdateTransferAmount updates the amount on both sides of a transfer.
// Reconciled transfers cannot be edited.
func (s *Service) UpdateTransferAmount(transferID types.ID, newAmount types.Money) error {
	if !newAmount.IsPositive() {
		return &InvalidTransferAmountError{Amount: newAmount}
	}

	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	return s.transferRepo.UpdateAmount(transferID, newAmount)
}

// UpdateTransferDate updates the date on both sides of a transfer.
// Reconciled transfers cannot be edited.
func (s *Service) UpdateTransferDate(transferID types.ID, newDate types.Date) error {
	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	return s.transferRepo.UpdateDate(transferID, newDate)
}

// UpdateTransferStatus updates the status on both sides of a transfer.
func (s *Service) UpdateTransferStatus(transferID types.ID, status Status) error {
	return s.transferRepo.UpdateStatus(transferID, status)
}

// DeleteTransfer removes both sides of a transfer.
// Reconciled transfers cannot be deleted.
func (s *Service) DeleteTransfer(transferID types.ID) error {
	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	return s.transferRepo.Delete(transferID)
}

// checkTransferEditable checks if a transfer can be edited/deleted.
// Returns an error if either side is reconciled or void.
func (s *Service) checkTransferEditable(transferID types.ID) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}
	if pair.FromTransaction.IsVoid() {
		return &IsVoidError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsVoid() {
		return &IsVoidError{ID: pair.ToTransaction.ID.String()}
	}

	return nil
}

// IsTransfer checks if a transaction is part of a transfer.
func (s *Service) IsTransfer(transactionID types.ID) (bool, error) {
	return s.transferRepo.IsTransfer(transactionID)
}

// =============================================================================
// Transaction Status Operations
// =============================================================================

// ClearTransaction marks a transaction as cleared.
// Void and reconciled transactions cannot be cleared directly.
func (s *Service) ClearTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	txn.Clear()
	return s.txnRepo.Update(txn)
}

// ReconcileTransaction marks a transaction as reconciled.
// Void transactions cannot be reconciled.
func (s *Service) ReconcileTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	txn.Reconcile()
	return s.txnRepo.Update(txn)
}

// MarkTransactionUncleared marks a transaction as uncleared.
// Void and reconciled transactions cannot be marked uncleared directly.
// Use UnReconcileTransaction to move from reconciled to cleared.
func (s *Service) MarkTransactionUncleared(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	txn.MarkUncleared()
	return s.txnRepo.Update(txn)
}

// UnReconcileTransaction moves a reconciled transaction back to cleared status.
// Only reconciled transactions can be un-reconciled.
func (s *Service) UnReconcileTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if !txn.IsReconciled() {
		return &NotReconciledError{ID: id.String()}
	}

	txn.Clear()
	return s.txnRepo.Update(txn)
}

// VoidTransaction voids a transaction by setting its amount to 0, memo to **VOID**,
// and status to void. For transfers, both sides are voided atomically.
// For split transactions, all splits are removed.
// Void and reconciled transactions cannot be voided.
func (s *Service) VoidTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Cannot void an already void transaction
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Cannot void a reconciled transaction
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// If this is a transfer, void both sides
	if txn.IsTransfer() {
		return s.voidTransfer(txn.TransferID.ID)
	}

	// Delete splits if any
	if _, err := s.splitRepo.DeleteByTransaction(id); err != nil {
		return fmt.Errorf("failed to delete splits for void: %w", err)
	}

	// Void the transaction
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	txn.Void()

	return s.txnRepo.Update(txn)
}

// voidTransfer voids both sides of a transfer atomically.
func (s *Service) voidTransfer(transferID types.ID) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Check if either side is reconciled
	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}

	// Void both sides
	pair.FromTransaction.Amount = types.ZeroMoney
	pair.FromTransaction.SetMemo("**VOID**")
	pair.FromTransaction.Void()

	pair.ToTransaction.Amount = types.ZeroMoney
	pair.ToTransaction.SetMemo("**VOID**")
	pair.ToTransaction.Void()

	return s.transferRepo.Update(pair)
}

// RestoreVoidedTransaction restores a voided transaction to its original state.
// This is used by the undo system to reverse a void operation.
// Only void transactions can be restored.
func (s *Service) RestoreVoidedTransaction(id types.ID, amount types.Money, memo types.NullableString, status Status) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if !txn.IsVoid() {
		return fmt.Errorf("transaction %s is not void; cannot restore", id.String())
	}

	txn.Amount = amount
	txn.Memo = memo
	txn.SetStatus(status)

	return s.txnRepo.Update(txn)
}

// RestoreVoidedTransfer restores both sides of a voided transfer to their original state.
// This is used by the undo system to reverse a void transfer operation.
func (s *Service) RestoreVoidedTransfer(transferID types.ID, fromAmount types.Money, fromMemo types.NullableString, fromStatus Status, toAmount types.Money, toMemo types.NullableString, toStatus Status) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	if !pair.FromTransaction.IsVoid() {
		return fmt.Errorf("transfer from-side %s is not void; cannot restore", pair.FromTransaction.ID.String())
	}
	if !pair.ToTransaction.IsVoid() {
		return fmt.Errorf("transfer to-side %s is not void; cannot restore", pair.ToTransaction.ID.String())
	}

	pair.FromTransaction.Amount = fromAmount
	pair.FromTransaction.Memo = fromMemo
	pair.FromTransaction.SetStatus(fromStatus)

	pair.ToTransaction.Amount = toAmount
	pair.ToTransaction.Memo = toMemo
	pair.ToTransaction.SetStatus(toStatus)

	return s.transferRepo.Update(pair)
}

// =============================================================================
// Balance Impact Operations
// =============================================================================

// BalanceImpact represents the impact of a transaction on account balances.
type BalanceImpact struct {
	AccountID      types.ID
	Amount         types.Money
	IsTransferFrom bool
	IsTransferTo   bool
}

// GetBalanceImpact calculates the balance impact of a transaction.
// For transfers, returns impacts for both accounts.
func (s *Service) GetBalanceImpact(transactionID types.ID) ([]BalanceImpact, error) {
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	impacts := []BalanceImpact{
		{
			AccountID:      txn.AccountID,
			Amount:         txn.Amount,
			IsTransferFrom: txn.IsTransfer() && txn.Amount.IsNegative(),
			IsTransferTo:   txn.IsTransfer() && txn.Amount.IsPositive(),
		},
	}

	// For transfers, include the other side
	if txn.IsTransfer() {
		other, err := s.transferRepo.GetOtherSide(transactionID)
		if err != nil {
			return nil, err
		}
		impacts = append(impacts, BalanceImpact{
			AccountID:      other.AccountID,
			Amount:         other.Amount,
			IsTransferFrom: other.Amount.IsNegative(),
			IsTransferTo:   other.Amount.IsPositive(),
		})
	}

	return impacts, nil
}

// =============================================================================
// Duplicate Operations
// =============================================================================

// Duplicate creates a copy of a transaction with today's date and uncleared status.
func (s *Service) Duplicate(transactionID types.ID) (*Transaction, error) {
	original, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	// Cannot duplicate transfers (they need special handling)
	if original.IsTransfer() {
		return nil, &CannotDuplicateTransferError{ID: transactionID.String()}
	}

	// Create a new transaction with same properties
	duplicate := NewTransaction(original.AccountID, types.Today(), original.Amount)
	duplicate.PayeeID = original.PayeeID
	duplicate.CategoryID = original.CategoryID
	duplicate.Memo = original.Memo
	duplicate.CheckNumber = original.CheckNumber
	// Status is always uncleared for duplicates (set by NewTransaction)

	if err := s.txnRepo.Create(duplicate); err != nil {
		return nil, err
	}

	// Duplicate splits if any
	splits, err := s.splitRepo.ListByTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get splits: %w", err)
	}

	for _, split := range splits {
		newSplit := NewSplit(duplicate.ID, split.CategoryID, split.Amount)
		newSplit.Memo = split.Memo
		if err := s.splitRepo.Create(newSplit); err != nil {
			// Best effort cleanup
			_ = s.txnRepo.Delete(duplicate.ID)
			return nil, fmt.Errorf("failed to duplicate split: %w", err)
		}
	}

	return duplicate, nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validateTransaction validates a transaction and returns any validation errors.
func (s *Service) validateTransaction(transaction *Transaction) error {
	errors := transaction.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplit validates a split and returns any validation errors.
func (s *Service) validateSplit(split *Split) error {
	errors := split.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplits validates that splits' signed sum equals the transaction
// amount. Mixed-sign lines are allowed: each line carries its own sign
// independent of the parent. Transfer-typed lines additionally must not
// target the parent's own account.
// See SplitCollection.ValidateAgainstTransaction.
func (s *Service) validateSplits(transaction *Transaction, splits []*Split) error {
	if len(splits) == 0 {
		return nil
	}

	for _, split := range splits {
		if err := s.validateSplit(split); err != nil {
			return err
		}
		if split.TransferAccountID.Valid && split.TransferAccountID.ID == transaction.AccountID {
			errors := types.ValidationErrors{}
			errors.Add("transfer_account_id",
				(&SelfTransferError{AccountID: transaction.AccountID.String()}).Error())
			return &types.ServiceValidationError{Errors: errors}
		}
	}

	collection := SplitCollection(splits)
	errors := collection.ValidateAgainstTransaction(transaction.Amount)
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}

	return nil
}

// applyPayeeDefaultCategory sets the transaction category from the payee's default.
func (s *Service) applyPayeeDefaultCategory(transaction *Transaction) error {
	if s.payeeRepo == nil || !transaction.HasPayee() {
		return nil
	}

	p, err := s.payeeRepo.GetByID(transaction.PayeeID.ID)
	if err != nil {
		// Payee not found is not an error here - just skip
		if _, ok := err.(*dberrors.NotFoundError); ok {
			return nil
		}
		return fmt.Errorf("failed to get payee: %w", err)
	}

	if p.HasDefaultCategory() {
		transaction.SetCategory(p.DefaultCategoryID.ID)
	}

	return nil
}
