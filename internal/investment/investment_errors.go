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
