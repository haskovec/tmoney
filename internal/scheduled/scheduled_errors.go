package scheduled

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/loan"
)

// Loan-computation error sentinels returned by ComputeLoanSplits (and, through
// it, by the loan-shaped posting paths). Callers use errors.Is to branch:
// manual posts surface them to the user; auto-post skips the schedule with the
// error as its reason rather than aborting the batch (isLoanComputationError).
var (
	// ErrLoanPaidOff is returned when the loan's owed balance is already ≤ 0 at
	// post time. It is terminal — both the manual refusal and the auto-post
	// skip mark the schedule completed.
	ErrLoanPaidOff = errors.New("loan is paid off")

	// ErrLoanNoInterestRate is returned when the loan account's interest_rate is
	// NULL. A missing APR is a typed error, never a silent 0%.
	ErrLoanNoInterestRate = errors.New("loan account has no interest rate set")

	// ErrLoanMissingInterestLine is returned when computed interest is greater
	// than $0.00 but the schedule has no interest line to book it against.
	ErrLoanMissingInterestLine = errors.New("loan schedule has no interest line — open Edit as loan to add one")
)

// isLoanComputationError reports whether err is one of the loan-recompute
// failures that should skip an auto-post schedule (with a reason) rather than
// abort the whole batch: paid off, no interest rate, missing interest line, or
// negative amortization.
func isLoanComputationError(err error) bool {
	return errors.Is(err, ErrLoanPaidOff) ||
		errors.Is(err, ErrLoanNoInterestRate) ||
		errors.Is(err, ErrLoanMissingInterestLine) ||
		errors.Is(err, loan.ErrNegativeAmortization)
}

// loanSkipReason maps a loan-computation error to a concise auto-post skip
// reason (matching the style of the closed-account skip reason).
func loanSkipReason(err error) string {
	switch {
	case errors.Is(err, ErrLoanPaidOff):
		return "loan is paid off"
	case errors.Is(err, ErrLoanNoInterestRate):
		return "loan account has no interest rate set"
	case errors.Is(err, ErrLoanMissingInterestLine):
		return "loan schedule has no interest line"
	case errors.Is(err, loan.ErrNegativeAmortization):
		return "loan payment does not cover interest"
	default:
		return err.Error()
	}
}

// CompletedError is returned when trying to post/skip a completed schedule.
type CompletedError struct {
	ID string
}

func (e *CompletedError) Error() string {
	return fmt.Sprintf("scheduled transaction %s has completed all occurrences", e.ID)
}

// ClosedAccountError is returned when a schedule references a closed account —
// either at creation or at post time. A schedule may not target a closed
// account; reopen the account first.
type ClosedAccountError struct {
	ID string
}

func (e *ClosedAccountError) Error() string {
	return fmt.Sprintf("scheduled transaction references closed account %s", e.ID)
}

// AmountRequiredError is returned when posting a variable-amount schedule without an amount.
type AmountRequiredError struct {
	ID string
}

func (e *AmountRequiredError) Error() string {
	return fmt.Sprintf("scheduled transaction %s requires an amount (variable amount with no estimate available)", e.ID)
}
