package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// AccountBalance holds the calculated balance information for an account.
type AccountBalance struct {
	AccountID      models.ID
	CurrentBalance models.Money
	ClearedBalance models.Money
}

// AccountService provides business logic for account operations.
type AccountService struct {
	repo *repository.AccountRepository
	db   *db.DB
}

// NewAccountService creates a new AccountService.
func NewAccountService(repo *repository.AccountRepository, database *db.DB) *AccountService {
	return &AccountService{
		repo: repo,
		db:   database,
	}
}

// Create validates and creates a new account.
func (s *AccountService) Create(account *models.Account) error {
	if err := s.validateAccount(account); err != nil {
		return err
	}
	return s.repo.Create(account)
}

// GetByID retrieves an account by its ID.
func (s *AccountService) GetByID(id models.ID) (*models.Account, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves an account by its name.
func (s *AccountService) GetByName(name string) (*models.Account, error) {
	return s.repo.GetByName(name)
}

// Update validates and updates an existing account.
func (s *AccountService) Update(account *models.Account) error {
	if err := s.validateAccount(account); err != nil {
		return err
	}
	return s.repo.Update(account)
}

// Delete removes an account. The account must have no transactions.
func (s *AccountService) Delete(id models.ID) error {
	return s.repo.Delete(id)
}

// List returns all accounts, optionally filtered to active accounts only.
func (s *AccountService) List(activeOnly bool) ([]*models.Account, error) {
	return s.repo.List(activeOnly)
}

// GetBalance calculates and returns the balance information for an account.
func (s *AccountService) GetBalance(id models.ID) (*AccountBalance, error) {
	// Use the account_balances view to get calculated balances
	query := `
		SELECT current_balance, cleared_balance
		FROM account_balances
		WHERE CAST(id AS VARCHAR) = ?
	`

	var currentBalance, clearedBalance models.Money
	err := s.db.Conn().QueryRow(query, id.String()).Scan(&currentBalance, &clearedBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balance: %w", err)
	}

	return &AccountBalance{
		AccountID:      id,
		CurrentBalance: currentBalance,
		ClearedBalance: clearedBalance,
	}, nil
}

// GetAllBalances returns balance information for all accounts.
func (s *AccountService) GetAllBalances() (map[models.ID]*AccountBalance, error) {
	query := `
		SELECT id, current_balance, cleared_balance
		FROM account_balances
	`

	rows, err := s.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[models.ID]*AccountBalance)
	for rows.Next() {
		var id models.ID
		var currentBalance, clearedBalance models.Money
		if err := rows.Scan(&id, &currentBalance, &clearedBalance); err != nil {
			return nil, fmt.Errorf("failed to scan account balance: %w", err)
		}
		balances[id] = &AccountBalance{
			AccountID:      id,
			CurrentBalance: currentBalance,
			ClearedBalance: clearedBalance,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account balances: %w", err)
	}

	return balances, nil
}

// Close closes an account after validating it can be closed.
// An account can only be closed if it has a zero balance.
func (s *AccountService) Close(id models.ID) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if !account.Active {
		return &AccountAlreadyClosedError{ID: id.String()}
	}

	balance, err := s.GetBalance(id)
	if err != nil {
		return err
	}

	if !balance.CurrentBalance.IsZero() {
		return &AccountHasBalanceError{
			ID:      id.String(),
			Balance: balance.CurrentBalance,
		}
	}

	account.Close()
	return s.repo.Update(account)
}

// Reopen reopens a closed account.
func (s *AccountService) Reopen(id models.ID) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if account.Active {
		return &AccountNotClosedError{ID: id.String()}
	}

	account.Reopen()
	return s.repo.Update(account)
}

// validateAccount validates an account and returns any validation errors.
func (s *AccountService) validateAccount(account *models.Account) error {
	errors := account.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
