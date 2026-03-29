package security

import "fmt"

// AlreadyHiddenError is returned when trying to hide an already hidden security.
type AlreadyHiddenError struct {
	ID string
}

func (e *AlreadyHiddenError) Error() string {
	return fmt.Sprintf("security %s is already hidden", e.ID)
}

// NotHiddenError is returned when trying to unhide a security that is not hidden.
type NotHiddenError struct {
	ID string
}

func (e *NotHiddenError) Error() string {
	return fmt.Sprintf("security %s is not hidden", e.ID)
}

// HasOpenPositionsError is returned when trying to hide a security with open positions.
type HasOpenPositionsError struct {
	ID string
}

func (e *HasOpenPositionsError) Error() string {
	return fmt.Sprintf("cannot hide security %s: has open positions", e.ID)
}

// HasDependentsError is returned when trying to delete a security with prices or transactions.
// Suggests hiding instead.
type HasDependentsError struct {
	ID         string
	Dependents string
	Count      int
}

func (e *HasDependentsError) Error() string {
	return fmt.Sprintf("cannot delete security %s: has %d %s; consider hiding instead", e.ID, e.Count, e.Dependents)
}
