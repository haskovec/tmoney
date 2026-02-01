package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
)

// ServiceValidationError wraps model validation errors.
type ServiceValidationError struct {
	Errors models.ValidationErrors
}

func (e *ServiceValidationError) Error() string {
	return fmt.Sprintf("validation failed: %s", e.Errors.Error())
}

// AccountAlreadyClosedError is returned when trying to close an already closed account.
type AccountAlreadyClosedError struct {
	ID string
}

func (e *AccountAlreadyClosedError) Error() string {
	return fmt.Sprintf("account %s is already closed", e.ID)
}

// AccountNotClosedError is returned when trying to reopen an account that isn't closed.
type AccountNotClosedError struct {
	ID string
}

func (e *AccountNotClosedError) Error() string {
	return fmt.Sprintf("account %s is not closed", e.ID)
}

// AccountHasBalanceError is returned when trying to close an account with a non-zero balance.
type AccountHasBalanceError struct {
	ID      string
	Balance models.Money
}

func (e *AccountHasBalanceError) Error() string {
	return fmt.Sprintf("cannot close account %s: has balance of %s", e.ID, e.Balance.String())
}
