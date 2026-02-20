package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// TransactionService provides business logic for transaction operations.
type TransactionService struct {
	txnRepo      *repository.TransactionRepository
	splitRepo    *repository.SplitRepository
	transferRepo *repository.TransferRepository
	payeeRepo    *repository.PayeeRepository
	db           *db.DB
}

// NewTransactionService creates a new TransactionService.
func NewTransactionService(
	txnRepo *repository.TransactionRepository,
	splitRepo *repository.SplitRepository,
	transferRepo *repository.TransferRepository,
	payeeRepo *repository.PayeeRepository,
	database *db.DB,
) *TransactionService {
	return &TransactionService{
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
func (s *TransactionService) Create(transaction *models.Transaction) error {
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
func (s *TransactionService) GetByID(id models.ID) (*models.Transaction, error) {
	return s.txnRepo.GetByID(id)
}

// Update validates and updates an existing transaction.
// For transfers, use UpdateTransfer to update both sides.
// Void and reconciled transactions cannot be edited.
func (s *TransactionService) Update(transaction *models.Transaction) error {
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
		return &TransactionIsVoidError{ID: transaction.ID.String()}
	}

	// Prevent editing reconciled transactions
	if existing.IsReconciled() {
		return &TransactionIsReconciledError{ID: transaction.ID.String()}
	}

	// Check if this is a transfer - caller should use UpdateTransfer
	if existing.IsTransfer() && !transaction.IsTransfer() {
		return &TransactionIsTransferError{ID: transaction.ID.String()}
	}

	return s.txnRepo.Update(transaction)
}

// Delete removes a transaction.
// For transfers, this will delete both sides after confirmation.
func (s *TransactionService) Delete(id models.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// If this is a transfer, delete both sides
	if txn.IsTransfer() {
		return s.transferRepo.Delete(txn.TransferID.ID)
	}

	// Delete any splits first
	if _, err := s.splitRepo.DeleteByTransaction(id); err != nil {
		return fmt.Errorf("failed to delete splits: %w", err)
	}

	return s.txnRepo.Delete(id)
}

// List returns all transactions ordered by date descending.
func (s *TransactionService) List() ([]*models.Transaction, error) {
	return s.txnRepo.List()
}

// ListByAccount returns all transactions for an account.
func (s *TransactionService) ListByAccount(accountID models.ID) ([]*models.Transaction, error) {
	return s.txnRepo.ListByAccount(accountID)
}

// ListByDateRange returns all transactions within a date range.
func (s *TransactionService) ListByDateRange(startDate, endDate models.Date) ([]*models.Transaction, error) {
	return s.txnRepo.ListByDateRange(startDate, endDate)
}

// ListByAccountAndDateRange returns transactions for an account within a date range.
func (s *TransactionService) ListByAccountAndDateRange(accountID models.ID, startDate, endDate models.Date) ([]*models.Transaction, error) {
	return s.txnRepo.ListByAccountAndDateRange(accountID, startDate, endDate)
}

// Search finds transactions matching the given criteria.
// Multiple criteria are combined with AND logic.
func (s *TransactionService) Search(criteria repository.TransactionSearchCriteria) ([]*models.Transaction, error) {
	return s.txnRepo.Search(criteria)
}

// SearchByPayee finds transactions by partial payee name match (case-insensitive).
func (s *TransactionService) SearchByPayee(payeeName string) ([]*models.Transaction, error) {
	return s.txnRepo.SearchByPayee(payeeName)
}

// SearchByMemo finds transactions by partial memo match (case-insensitive).
func (s *TransactionService) SearchByMemo(memo string) ([]*models.Transaction, error) {
	return s.txnRepo.SearchByMemo(memo)
}

// SearchByCategory finds transactions by partial category name match (case-insensitive).
func (s *TransactionService) SearchByCategory(categoryName string) ([]*models.Transaction, error) {
	return s.txnRepo.SearchByCategory(categoryName)
}

// =============================================================================
// Split Transaction Operations
// =============================================================================

// CreateWithSplits creates a transaction along with its splits.
// The splits must sum to the transaction amount.
func (s *TransactionService) CreateWithSplits(transaction *models.Transaction, splits []*models.Split) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	// Validate splits sum to transaction amount
	if err := s.validateSplits(transaction, splits); err != nil {
		return err
	}

	// When a transaction has splits, it should not have a category set
	if transaction.HasCategory() && len(splits) > 0 {
		return &TransactionHasSplitsError{ID: transaction.ID.String()}
	}

	// Create the transaction
	if err := s.txnRepo.Create(transaction); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Create all splits
	for _, split := range splits {
		split.TransactionID = transaction.ID
		if err := s.splitRepo.Create(split); err != nil {
			// Best effort cleanup - delete the transaction
			_ = s.txnRepo.Delete(transaction.ID)
			return fmt.Errorf("failed to create split: %w", err)
		}
	}

	return nil
}

// GetSplits returns all splits for a transaction.
func (s *TransactionService) GetSplits(transactionID models.ID) ([]*models.Split, error) {
	return s.splitRepo.ListByTransaction(transactionID)
}

// AddSplit adds a new split to an existing transaction.
// After adding, the splits must still sum to the transaction amount.
func (s *TransactionService) AddSplit(split *models.Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	// Get the transaction
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
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
func (s *TransactionService) UpdateSplit(split *models.Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	return s.splitRepo.Update(split)
}

// DeleteSplit removes a split from a transaction.
func (s *TransactionService) DeleteSplit(splitID models.ID) error {
	return s.splitRepo.Delete(splitID)
}

// ReplaceSplits replaces all splits for a transaction with new ones.
// The new splits must sum to the transaction amount.
func (s *TransactionService) ReplaceSplits(transactionID models.ID, splits []*models.Split) error {
	// Get the transaction
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return err
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
func (s *TransactionService) ValidateSplitTotals(transactionID models.ID) (bool, error) {
	return s.splitRepo.ValidateSplitsAgainstTransaction(transactionID)
}

// =============================================================================
// Transfer Operations
// =============================================================================

// CreateTransfer creates a linked transfer between two accounts.
// This creates two transactions: one debit in the from account, one credit in the to account.
func (s *TransactionService) CreateTransfer(fromAccountID, toAccountID models.ID, date models.Date, amount models.Money) (*models.TransferPair, error) {
	// Validate amount is positive
	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Create the transfer pair
	pair := models.NewTransferPair(fromAccountID, toAccountID, date, amount)

	// Validate the pair
	if errors := pair.Validate(); errors.HasErrors() {
		return nil, &ServiceValidationError{Errors: errors}
	}

	// Create both transactions
	if err := s.transferRepo.Create(pair); err != nil {
		return nil, err
	}

	return pair, nil
}

// GetTransferPair retrieves both sides of a transfer by the transfer ID.
func (s *TransactionService) GetTransferPair(transferID models.ID) (*models.TransferPair, error) {
	return s.transferRepo.GetByTransferID(transferID)
}

// GetTransferCounterpart retrieves the other transaction in a transfer pair.
func (s *TransactionService) GetTransferCounterpart(transactionID models.ID) (*models.Transaction, error) {
	return s.transferRepo.GetOtherSide(transactionID)
}

// UpdateTransfer updates both sides of a transfer.
// Only amount, date, memo, and status can be updated.
func (s *TransactionService) UpdateTransfer(transferID models.ID, date models.Date, amount models.Money, memo string, status models.TransactionStatus) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
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
func (s *TransactionService) UpdateTransferAmount(transferID models.ID, newAmount models.Money) error {
	if !newAmount.IsPositive() {
		return &InvalidTransferAmountError{Amount: newAmount}
	}
	return s.transferRepo.UpdateAmount(transferID, newAmount)
}

// UpdateTransferDate updates the date on both sides of a transfer.
func (s *TransactionService) UpdateTransferDate(transferID models.ID, newDate models.Date) error {
	return s.transferRepo.UpdateDate(transferID, newDate)
}

// UpdateTransferStatus updates the status on both sides of a transfer.
func (s *TransactionService) UpdateTransferStatus(transferID models.ID, status models.TransactionStatus) error {
	return s.transferRepo.UpdateStatus(transferID, status)
}

// DeleteTransfer removes both sides of a transfer.
func (s *TransactionService) DeleteTransfer(transferID models.ID) error {
	return s.transferRepo.Delete(transferID)
}

// IsTransfer checks if a transaction is part of a transfer.
func (s *TransactionService) IsTransfer(transactionID models.ID) (bool, error) {
	return s.transferRepo.IsTransfer(transactionID)
}

// =============================================================================
// Transaction Status Operations
// =============================================================================

// ClearTransaction marks a transaction as cleared.
func (s *TransactionService) ClearTransaction(id models.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	txn.Clear()
	return s.txnRepo.Update(txn)
}

// ReconcileTransaction marks a transaction as reconciled.
func (s *TransactionService) ReconcileTransaction(id models.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	txn.Reconcile()
	return s.txnRepo.Update(txn)
}

// MarkTransactionUncleared marks a transaction as uncleared.
func (s *TransactionService) MarkTransactionUncleared(id models.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	txn.MarkUncleared()
	return s.txnRepo.Update(txn)
}

// VoidTransaction voids a transaction by setting its amount to 0, memo to **VOID**,
// and status to void. For transfers, both sides are voided atomically.
// For split transactions, all splits are removed.
// Void and reconciled transactions cannot be voided.
func (s *TransactionService) VoidTransaction(id models.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Cannot void an already void transaction
	if txn.IsVoid() {
		return &TransactionIsVoidError{ID: id.String()}
	}

	// Cannot void a reconciled transaction
	if txn.IsReconciled() {
		return &TransactionIsReconciledError{ID: id.String()}
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
	txn.Amount = models.ZeroMoney
	txn.SetMemo("**VOID**")
	txn.Void()

	return s.txnRepo.Update(txn)
}

// voidTransfer voids both sides of a transfer atomically.
func (s *TransactionService) voidTransfer(transferID models.ID) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Check if either side is reconciled
	if pair.FromTransaction.IsReconciled() {
		return &TransactionIsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &TransactionIsReconciledError{ID: pair.ToTransaction.ID.String()}
	}

	// Void both sides
	pair.FromTransaction.Amount = models.ZeroMoney
	pair.FromTransaction.SetMemo("**VOID**")
	pair.FromTransaction.Void()

	pair.ToTransaction.Amount = models.ZeroMoney
	pair.ToTransaction.SetMemo("**VOID**")
	pair.ToTransaction.Void()

	return s.transferRepo.Update(pair)
}

// =============================================================================
// Balance Impact Operations
// =============================================================================

// BalanceImpact represents the impact of a transaction on account balances.
type BalanceImpact struct {
	AccountID      models.ID
	Amount         models.Money
	IsTransferFrom bool
	IsTransferTo   bool
}

// GetBalanceImpact calculates the balance impact of a transaction.
// For transfers, returns impacts for both accounts.
func (s *TransactionService) GetBalanceImpact(transactionID models.ID) ([]BalanceImpact, error) {
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
func (s *TransactionService) Duplicate(transactionID models.ID) (*models.Transaction, error) {
	original, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	// Cannot duplicate transfers (they need special handling)
	if original.IsTransfer() {
		return nil, &CannotDuplicateTransferError{ID: transactionID.String()}
	}

	// Create a new transaction with same properties
	duplicate := models.NewTransaction(original.AccountID, models.Today(), original.Amount)
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
		newSplit := models.NewSplit(duplicate.ID, split.CategoryID, split.Amount)
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
func (s *TransactionService) validateTransaction(transaction *models.Transaction) error {
	errors := transaction.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplit validates a split and returns any validation errors.
func (s *TransactionService) validateSplit(split *models.Split) error {
	errors := split.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplits validates that splits sum to the transaction amount.
func (s *TransactionService) validateSplits(transaction *models.Transaction, splits []*models.Split) error {
	if len(splits) == 0 {
		return nil
	}

	// Validate each split
	for _, split := range splits {
		if err := s.validateSplit(split); err != nil {
			return err
		}
	}

	// Validate sum
	collection := models.SplitCollection(splits)
	errors := collection.ValidateAgainstTransaction(transaction.Amount)
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}

	return nil
}

// applyPayeeDefaultCategory sets the transaction category from the payee's default.
func (s *TransactionService) applyPayeeDefaultCategory(transaction *models.Transaction) error {
	if s.payeeRepo == nil || !transaction.HasPayee() {
		return nil
	}

	payee, err := s.payeeRepo.GetByID(transaction.PayeeID.ID)
	if err != nil {
		// Payee not found is not an error here - just skip
		if _, ok := err.(*repository.NotFoundError); ok {
			return nil
		}
		return fmt.Errorf("failed to get payee: %w", err)
	}

	if payee.HasDefaultCategory() {
		transaction.SetCategory(payee.DefaultCategoryID.ID)
	}

	return nil
}
