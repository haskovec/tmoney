package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// InvestmentTransactionService provides business logic for investment transaction operations.
type InvestmentTransactionService struct {
	repo        *repository.InvestmentTransactionRepository
	accountRepo *repository.AccountRepository
	db          *db.DB
}

// NewInvestmentTransactionService creates a new InvestmentTransactionService.
func NewInvestmentTransactionService(
	repo *repository.InvestmentTransactionRepository,
	accountRepo *repository.AccountRepository,
	database *db.DB,
) *InvestmentTransactionService {
	return &InvestmentTransactionService{
		repo:        repo,
		accountRepo: accountRepo,
		db:          database,
	}
}

// Deposit creates a deposit transaction that increases the cash position in an investment account.
func (s *InvestmentTransactionService) Deposit(accountID models.ID, date models.Date, amount models.Money, memo string) (*models.InvestmentTransaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := models.NewInvestmentTransaction(accountID, date, models.InvestmentTransactionTypeDeposit, amount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}

	return txn, nil
}

// Withdrawal creates a withdrawal transaction that decreases the cash position in an investment account.
func (s *InvestmentTransactionService) Withdrawal(accountID models.ID, date models.Date, amount models.Money, memo string) (*models.InvestmentTransaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Check cash balance
	cashBalance, err := s.GetCashBalance(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash balance: %w", err)
	}

	if cashBalance.Cmp(amount) < 0 {
		return nil, &InsufficientCashError{
			AccountID: accountID.String(),
			Available: cashBalance,
			Requested: amount,
		}
	}

	// Store as negative amount for withdrawal
	negAmount := amount.Neg()

	txn := models.NewInvestmentTransaction(accountID, date, models.InvestmentTransactionTypeWithdrawal, negAmount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}

	return txn, nil
}

// Interest creates an interest transaction that increases the cash position in an investment account.
func (s *InvestmentTransactionService) Interest(accountID models.ID, date models.Date, amount models.Money, memo string) (*models.InvestmentTransaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := models.NewInvestmentTransaction(accountID, date, models.InvestmentTransactionTypeInterest, amount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create interest transaction: %w", err)
	}

	return txn, nil
}

// Fee creates a fee transaction that decreases the cash position in an investment account.
func (s *InvestmentTransactionService) Fee(accountID models.ID, date models.Date, amount models.Money, memo string) (*models.InvestmentTransaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Check cash balance
	cashBalance, err := s.GetCashBalance(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash balance: %w", err)
	}

	if cashBalance.Cmp(amount) < 0 {
		return nil, &InsufficientCashError{
			AccountID: accountID.String(),
			Available: cashBalance,
			Requested: amount,
		}
	}

	// Store as negative amount for fee
	negAmount := amount.Neg()

	txn := models.NewInvestmentTransaction(accountID, date, models.InvestmentTransactionTypeFee, negAmount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create fee transaction: %w", err)
	}

	return txn, nil
}

// GetCashBalance computes the cash balance for an investment account by summing
// all cash-affecting transactions.
func (s *InvestmentTransactionService) GetCashBalance(accountID models.ID) (models.Money, error) {
	txns, err := s.repo.ListByAccount(accountID, repository.InvestmentTransactionFilter{})
	if err != nil {
		return models.ZeroMoney, fmt.Errorf("failed to list transactions: %w", err)
	}

	balance := models.ZeroMoney
	for _, txn := range txns {
		if txn.Type.AffectsCash() {
			balance = balance.Add(txn.TotalAmount)
		}
	}

	return balance, nil
}

// requireInvestmentAccount verifies that the given account exists and is an investment account.
func (s *InvestmentTransactionService) requireInvestmentAccount(accountID models.ID) error {
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	if account.Type != models.AccountTypeInvestment {
		return &AccountNotInvestmentError{
			AccountID: accountID.String(),
			Type:      string(account.Type),
		}
	}

	return nil
}

// validateTransaction validates an investment transaction and returns any validation errors.
func (s *InvestmentTransactionService) validateTransaction(txn *models.InvestmentTransaction) error {
	errors := txn.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
