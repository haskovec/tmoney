package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// IsTransferError is returned when trying to update a transfer as a regular transaction.
type IsTransferError struct {
	ID string
}

func (e *IsTransferError) Error() string {
	return fmt.Sprintf("transaction %s is a transfer; use UpdateTransfer instead", e.ID)
}

// HasSplitsError is returned when a transaction with splits has a category set.
type HasSplitsError struct {
	ID string
}

func (e *HasSplitsError) Error() string {
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
	TransactionAmount types.Money
	SplitTotal        types.Money
}

func (e *SplitTotalMismatchError) Error() string {
	return fmt.Sprintf("split total (%s) does not match transaction amount (%s) for transaction %s",
		e.SplitTotal.String(), e.TransactionAmount.String(), e.TransactionID)
}

// InvalidTransferAmountError is returned when a transfer amount is invalid (not positive).
type InvalidTransferAmountError struct {
	Amount types.Money
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

// CannotDuplicateSplitTransferError is returned when trying to duplicate a
// split transaction that contains a transfer line. The split-copy path can't
// reconstruct the paired counter-transaction, so duplication is refused rather
// than silently producing an orphaned (and, after migration 029, categorized)
// split with no counterpart.
type CannotDuplicateSplitTransferError struct {
	ID string
}

func (e *CannotDuplicateSplitTransferError) Error() string {
	return fmt.Sprintf("cannot duplicate transaction %s: it contains a transfer line", e.ID)
}

// IsVoidError is returned when trying to edit or void a void transaction.
type IsVoidError struct {
	ID string
}

func (e *IsVoidError) Error() string {
	return fmt.Sprintf("transaction %s is void and cannot be modified", e.ID)
}

// IsReconciledError is returned when trying to edit, delete, or void a reconciled transaction.
type IsReconciledError struct {
	ID string
}

func (e *IsReconciledError) Error() string {
	return fmt.Sprintf("transaction %s is reconciled; un-reconcile it first", e.ID)
}

// NotReconciledError is returned when trying to un-reconcile a non-reconciled transaction.
type NotReconciledError struct {
	ID string
}

func (e *NotReconciledError) Error() string {
	return fmt.Sprintf("transaction %s is not reconciled", e.ID)
}

// IsNotTransferError is returned when a transfer operation is attempted on a non-transfer transaction.
type IsNotTransferError struct {
	ID string
}

func (e *IsNotTransferError) Error() string {
	return fmt.Sprintf("transaction %s is not a transfer", e.ID)
}

// SelfTransferError is returned when a transfer-line's target account equals
// the parent transaction's account.
type SelfTransferError struct {
	AccountID string
}

func (e *SelfTransferError) Error() string {
	return fmt.Sprintf("transfer-line target account %s equals parent account; self-transfer is not allowed", e.AccountID)
}

// NotRegularAccountError is returned when CreateTransfer / UpdateTransfer is
// invoked with an investment-type account on either leg. Linked cash
// transfers that touch an investment account must go through
// investment.Service (TransferCash / DepositFromAccount /
// TransferCashBetweenInvestments) so the investment-side row is created as
// an investment.Transaction.
type NotRegularAccountError struct {
	AccountID string
	Type      string
}

func (e *NotRegularAccountError) Error() string {
	return fmt.Sprintf("account %s is an investment-type account (%s); use investment.Service for linked cash transfers", e.AccountID, e.Type)
}
