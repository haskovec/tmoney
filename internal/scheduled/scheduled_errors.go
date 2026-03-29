package scheduled

import "fmt"

// CompletedError is returned when trying to post/skip a completed schedule.
type CompletedError struct {
	ID string
}

func (e *CompletedError) Error() string {
	return fmt.Sprintf("scheduled transaction %s has completed all occurrences", e.ID)
}

// AmountRequiredError is returned when posting a variable-amount schedule without an amount.
type AmountRequiredError struct {
	ID string
}

func (e *AmountRequiredError) Error() string {
	return fmt.Sprintf("scheduled transaction %s requires an amount (variable amount with no estimate available)", e.ID)
}
