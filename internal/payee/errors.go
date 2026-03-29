package payee

import "fmt"

// MergeSameError is returned when trying to merge a payee into itself.
type MergeSameError struct {
	ID string
}

func (e *MergeSameError) Error() string {
	return fmt.Sprintf("cannot merge payee %s into itself", e.ID)
}
