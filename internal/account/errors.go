package account

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// AlreadyClosedError is returned when trying to close an already closed account.
type AlreadyClosedError struct {
	ID string
}

func (e *AlreadyClosedError) Error() string {
	return fmt.Sprintf("account %s is already closed", e.ID)
}

// NotClosedError is returned when trying to reopen an account that isn't closed.
type NotClosedError struct {
	ID string
}

func (e *NotClosedError) Error() string {
	return fmt.Sprintf("account %s is not closed", e.ID)
}

// HasBalanceError is returned when trying to close an account with a non-zero balance.
type HasBalanceError struct {
	ID      string
	Balance types.Money
}

func (e *HasBalanceError) Error() string {
	return fmt.Sprintf("cannot close account %s: has balance of %s", e.ID, e.Balance.String())
}

// IsClosedError is returned when trying to reconcile a closed account.
type IsClosedError struct {
	ID string
}

func (e *IsClosedError) Error() string {
	return fmt.Sprintf("cannot reconcile closed account %s", e.ID)
}

// NotInvestmentError is returned when an investment operation is attempted on a non-investment account.
type NotInvestmentError struct {
	AccountID string
	Type      string
}

func (e *NotInvestmentError) Error() string {
	return fmt.Sprintf("account %s is not an investment account (type: %s)", e.AccountID, e.Type)
}
