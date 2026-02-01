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

// CategoryIsSystemError is returned when trying to modify or delete a system category.
type CategoryIsSystemError struct {
	ID   string
	Name string
}

func (e *CategoryIsSystemError) Error() string {
	return fmt.Sprintf("cannot modify system category %q (%s)", e.Name, e.ID)
}

// CategoryMergeTypeMismatchError is returned when trying to merge categories of different types.
type CategoryMergeTypeMismatchError struct {
	SourceID   string
	SourceType string
	TargetID   string
	TargetType string
}

func (e *CategoryMergeTypeMismatchError) Error() string {
	return fmt.Sprintf("cannot merge categories: source %s is %s, target %s is %s",
		e.SourceID, e.SourceType, e.TargetID, e.TargetType)
}

// CategoryMergeSameError is returned when trying to merge a category into itself.
type CategoryMergeSameError struct {
	ID string
}

func (e *CategoryMergeSameError) Error() string {
	return fmt.Sprintf("cannot merge category %s into itself", e.ID)
}

// PayeeMergeSameError is returned when trying to merge a payee into itself.
type PayeeMergeSameError struct {
	ID string
}

func (e *PayeeMergeSameError) Error() string {
	return fmt.Sprintf("cannot merge payee %s into itself", e.ID)
}
