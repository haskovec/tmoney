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

// InvalidCloseDateError is returned when a close date falls outside the allowed
// window [max(opening_date, latest transaction date), today].
type InvalidCloseDateError struct {
	ID       string
	Date     types.Date
	Earliest types.Date
	Today    types.Date
}

func (e *InvalidCloseDateError) Error() string {
	return fmt.Sprintf("invalid close date %s: must be between %s and %s",
		e.Date.String(), e.Earliest.String(), e.Today.String())
}

// AccountClosedError is returned when any mutation is attempted on a closed
// account. A closed account is frozen: no new transactions, edits, deletes,
// status toggles or reconciliation, and a transfer is blocked if either leg is
// closed.
//
// This is the ONE closed-account error. account.IsClosedError used to sit beside
// it, differing only in message ("cannot reconcile closed account"), so a caller
// that wanted to know "is this account frozen?" had to match two types and could
// not do it with a single errors.As. Reconciliation now returns this one; the
// scheduled package keeps its own ClosedAccountError, which reports a different
// fact (a SCHEDULE references a closed account — a template problem, not a
// rejected mutation).
type AccountClosedError struct {
	ID string
}

func (e *AccountClosedError) Error() string {
	return fmt.Sprintf("cannot modify transactions on closed account %s", e.ID)
}

// NotInvestmentError is returned when an investment operation is attempted on a non-investment account.
type NotInvestmentError struct {
	AccountID string
	Type      string
}

func (e *NotInvestmentError) Error() string {
	return fmt.Sprintf("account %s is not an investment account (type: %s)", e.AccountID, e.Type)
}
