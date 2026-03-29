package reconciliation

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// NoActiveError is returned when trying to finish or cancel with no active session.
type NoActiveError struct {
	AccountID string
}

func (e *NoActiveError) Error() string {
	return fmt.Sprintf("no active reconciliation session for account %s", e.AccountID)
}

// DifferenceError is returned when trying to finish with a non-zero difference.
type DifferenceError struct {
	Difference types.Money
}

func (e *DifferenceError) Error() string {
	return fmt.Sprintf("cannot complete reconciliation: difference is %s (must be $0.00; use force to override)", e.Difference.String())
}

// StatementDateFutureError is returned when the statement date is in the future.
type StatementDateFutureError struct{}

func (e *StatementDateFutureError) Error() string {
	return "statement date must not be in the future"
}
