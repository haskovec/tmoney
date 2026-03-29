package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// InsufficientCashError is returned when an investment account has insufficient cash for an operation.
type InsufficientCashError struct {
	AccountID string
	Available types.Money
	Requested types.Money
}

func (e *InsufficientCashError) Error() string {
	return fmt.Sprintf("insufficient cash in account %s: available %s, requested %s",
		e.AccountID, e.Available.String(), e.Requested.String())
}

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
