package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// InsufficientCashError lived here. Its last producer was cash_position.go's
// balance precheck, deleted in phase 5 — cash balances are deliberately allowed
// to go negative so historical data entry is not ordering-sensitive, so nothing
// ever raised it again.

// InsufficientSharesError is returned when a sell exceeds the available shares.
type InsufficientSharesError struct {
	SecurityID string
	Available  types.Quantity
	Requested  types.Quantity
}

func (e *InsufficientSharesError) Error() string {
	return fmt.Sprintf("insufficient shares for security %s: available %s, requested %s",
		e.SecurityID, e.Available.String(), e.Requested.String())
}

// LotAllocationMismatchError is returned when lot allocations don't sum to the sell quantity.
type LotAllocationMismatchError struct {
	Expected types.Quantity
	Actual   types.Quantity
}

func (e *LotAllocationMismatchError) Error() string {
	return fmt.Sprintf("lot allocations total %s shares but sell requires %s shares",
		e.Actual.String(), e.Expected.String())
}

// LotNotFoundError is returned when a specified lot does not exist.
type LotNotFoundError struct {
	LotID string
}

func (e *LotNotFoundError) Error() string {
	return fmt.Sprintf("lot %s not found", e.LotID)
}

// LotWrongAccountError is returned when a lot belongs to a different account.
type LotWrongAccountError struct {
	LotID     string
	AccountID string
}

func (e *LotWrongAccountError) Error() string {
	return fmt.Sprintf("lot %s does not belong to account %s", e.LotID, e.AccountID)
}

// LotInsufficientSharesError is returned when a lot has fewer shares than requested.
type LotInsufficientSharesError struct {
	LotID     string
	Available types.Quantity
	Requested types.Quantity
}

func (e *LotInsufficientSharesError) Error() string {
	return fmt.Sprintf("lot %s has %s shares available but %s requested",
		e.LotID, e.Available.String(), e.Requested.String())
}

// NonPositiveAmountError is returned when a cash operation's amount is not
// strictly positive — Deposit, Withdrawal, Interest, Dividend and Fee all take a
// magnitude and derive their own sign.
//
// It was called InvalidTransferAmountError, which named a rule it no longer
// guards: transfers moved to internal/transfer, which owns
// transfer.InvalidAmountError. The old name additionally collided with an
// identical transaction.InvalidTransferAmountError, so no caller could match the
// transfer case with one errors.As. Both of those are gone; this one is renamed
// to what it actually checks.
type NonPositiveAmountError struct {
	Amount types.Money
}

func (e *NonPositiveAmountError) Error() string {
	return fmt.Sprintf("transfer amount must be positive, got %s", e.Amount.String())
}

// IsCashTransferLegError is returned when a cash-transfer LEG is handed to a
// verb that acts on one investment row.
//
// A transfer_cash row is half of a pair whose counterpart may live in
// `transactions`, so acting on it here would silently orphan the other side.
// transfer.Service owns the pair.
//
// Share transfers are not covered: those are owned by this package.
type IsCashTransferLegError struct {
	ID         string
	TransferID string
}

func (e *IsCashTransferLegError) Error() string {
	return fmt.Sprintf(
		"investment transaction %s is a leg of cash transfer %s; edit or delete the transfer itself",
		e.ID, e.TransferID,
	)
}
