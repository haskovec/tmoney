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

// TransactionIsTransferError is returned when trying to update a transfer as a regular transaction.
type TransactionIsTransferError struct {
	ID string
}

func (e *TransactionIsTransferError) Error() string {
	return fmt.Sprintf("transaction %s is a transfer; use UpdateTransfer instead", e.ID)
}

// TransactionHasSplitsError is returned when a transaction with splits has a category set.
type TransactionHasSplitsError struct {
	ID string
}

func (e *TransactionHasSplitsError) Error() string {
	return fmt.Sprintf("transaction %s has splits and cannot have a category set", e.ID)
}

// TransferCannotHaveSplitsError is returned when trying to add splits to a transfer.
type TransferCannotHaveSplitsError struct {
	ID string
}

func (e *TransferCannotHaveSplitsError) Error() string {
	return fmt.Sprintf("transfer transaction %s cannot have splits", e.ID)
}

// SplitTotalMismatchError is returned when splits don't sum to the transaction amount.
type SplitTotalMismatchError struct {
	TransactionID     string
	TransactionAmount models.Money
	SplitTotal        models.Money
}

func (e *SplitTotalMismatchError) Error() string {
	return fmt.Sprintf("split total (%s) does not match transaction amount (%s) for transaction %s",
		e.SplitTotal.String(), e.TransactionAmount.String(), e.TransactionID)
}

// InvalidTransferAmountError is returned when a transfer amount is invalid (not positive).
type InvalidTransferAmountError struct {
	Amount models.Money
}

func (e *InvalidTransferAmountError) Error() string {
	return fmt.Sprintf("transfer amount must be positive, got %s", e.Amount.String())
}

// CannotDuplicateTransferError is returned when trying to duplicate a transfer.
type CannotDuplicateTransferError struct {
	ID string
}

func (e *CannotDuplicateTransferError) Error() string {
	return fmt.Sprintf("cannot duplicate transfer transaction %s; use CreateTransfer instead", e.ID)
}

// TransactionIsVoidError is returned when trying to edit or void a void transaction.
type TransactionIsVoidError struct {
	ID string
}

func (e *TransactionIsVoidError) Error() string {
	return fmt.Sprintf("transaction %s is void and cannot be modified", e.ID)
}

// TransactionIsReconciledError is returned when trying to edit, delete, or void a reconciled transaction.
type TransactionIsReconciledError struct {
	ID string
}

func (e *TransactionIsReconciledError) Error() string {
	return fmt.Sprintf("transaction %s is reconciled; un-reconcile it first", e.ID)
}

// TransactionNotReconciledError is returned when trying to un-reconcile a non-reconciled transaction.
type TransactionNotReconciledError struct {
	ID string
}

func (e *TransactionNotReconciledError) Error() string {
	return fmt.Sprintf("transaction %s is not reconciled", e.ID)
}

// TransactionIsNotTransferError is returned when a transfer operation is attempted on a non-transfer transaction.
type TransactionIsNotTransferError struct {
	ID string
}

func (e *TransactionIsNotTransferError) Error() string {
	return fmt.Sprintf("transaction %s is not a transfer", e.ID)
}

// ScheduledTransactionCompletedError is returned when trying to post/skip a completed schedule.
type ScheduledTransactionCompletedError struct {
	ID string
}

func (e *ScheduledTransactionCompletedError) Error() string {
	return fmt.Sprintf("scheduled transaction %s has completed all occurrences", e.ID)
}

// ScheduledTransactionAmountRequiredError is returned when posting a variable-amount schedule without an amount.
type ScheduledTransactionAmountRequiredError struct {
	ID string
}

func (e *ScheduledTransactionAmountRequiredError) Error() string {
	return fmt.Sprintf("scheduled transaction %s requires an amount (variable amount with no estimate available)", e.ID)
}
