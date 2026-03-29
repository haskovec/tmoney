package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for investment transaction operations.
type Service struct {
	repo        *Repository
	accountRepo *account.Repository
	db          *db.DB
}

// NewService creates a new Service.
func NewService(
	repo *Repository,
	accountRepo *account.Repository,
	database *db.DB,
) *Service {
	return &Service{
		repo:        repo,
		accountRepo: accountRepo,
		db:          database,
	}
}

// Deposit creates a deposit transaction that increases the cash position in an investment account.
func (s *Service) Deposit(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeDeposit, amount)
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
func (s *Service) Withdrawal(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
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

	txn := NewTransaction(accountID, date, TransactionTypeWithdrawal, negAmount)
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
func (s *Service) Interest(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeInterest, amount)
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
func (s *Service) Fee(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
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

	txn := NewTransaction(accountID, date, TransactionTypeFee, negAmount)
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
func (s *Service) GetCashBalance(accountID types.ID) (types.Money, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions: %w", err)
	}

	balance := types.ZeroMoney
	for _, txn := range txns {
		if txn.Type.AffectsCash() {
			balance = balance.Add(txn.TotalAmount)
		}
	}

	return balance, nil
}

// requireInvestmentAccount verifies that the given account exists and is an investment account.
func (s *Service) requireInvestmentAccount(accountID types.ID) error {
	acct, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	if acct.Type != account.TypeInvestment {
		return &account.NotInvestmentError{
			AccountID: accountID.String(),
			Type:      string(acct.Type),
		}
	}

	return nil
}

// validateTransaction validates an investment transaction and returns any validation errors.
func (s *Service) validateTransaction(txn *Transaction) error {
	errors := txn.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// InvalidTransferAmountError is returned when a transfer amount is invalid (not positive).
type InvalidTransferAmountError struct {
	Amount types.Money
}

func (e *InvalidTransferAmountError) Error() string {
	return fmt.Sprintf("transfer amount must be positive, got %s", e.Amount.String())
}
