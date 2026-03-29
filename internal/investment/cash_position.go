package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// CashPosition represents the cash balance of an investment account.
// It is computed from the sum of all cash-affecting transactions.
type CashPosition struct {
	AccountID types.ID    `json:"account_id"`
	Balance   types.Money `json:"balance"`
}

// NewCashPosition creates a CashPosition with zero balance.
func NewCashPosition(accountID types.ID) CashPosition {
	return CashPosition{
		AccountID: accountID,
		Balance:   types.ZeroMoney,
	}
}

// Deposit increases the cash position by the given amount.
// Amount must be positive.
func (cp *CashPosition) Deposit(amount types.Money) error {
	if !amount.IsPositive() {
		return &InvalidTransferAmountError{Amount: amount}
	}
	cp.Balance = cp.Balance.Add(amount)
	return nil
}

// Withdraw decreases the cash position by the given amount.
// Amount must be positive and not exceed the current balance.
func (cp *CashPosition) Withdraw(amount types.Money) error {
	if !amount.IsPositive() {
		return &InvalidTransferAmountError{Amount: amount}
	}
	if cp.Balance.Cmp(amount) < 0 {
		return &InsufficientCashError{
			AccountID: cp.AccountID.String(),
			Available: cp.Balance,
			Requested: amount,
		}
	}
	cp.Balance = cp.Balance.Sub(amount)
	return nil
}

// InsufficientCashFormatted returns a descriptive error message about the cash shortfall.
func (cp *CashPosition) InsufficientCashFormatted(requested types.Money) string {
	return fmt.Sprintf("insufficient cash: available %s, requested %s",
		cp.Balance.String(), requested.String())
}
