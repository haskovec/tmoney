package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Cash-only activity in an investment account: money in, money out, and the two
// income shapes that add cash without touching a position.
//
// None of these opens a transaction of its own — a single row is written per call.

// Deposit creates a deposit transaction that increases the cash position in an investment account.
func (s *Service) Deposit(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &NonPositiveAmountError{Amount: amount}
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
		return nil, &NonPositiveAmountError{Amount: amount}
	}

	// Cash balance is allowed to go negative — withdrawals never block on
	// the running balance so historical data entry isn't ordering-sensitive.

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
		return nil, &NonPositiveAmountError{Amount: amount}
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
		return nil, &NonPositiveAmountError{Amount: amount}
	}

	// Cash balance is allowed to go negative — see Withdrawal for rationale.

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

// Dividend creates a cash dividend transaction that increases the cash position.
// The security and share count are unchanged. The amount is stored as positive (adds cash).
func (s *Service) Dividend(accountID types.ID, securityID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &NonPositiveAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeDividend, amount)
	txn.SetSecurity(securityID)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create dividend transaction: %w", err)
	}

	return txn, nil
}
