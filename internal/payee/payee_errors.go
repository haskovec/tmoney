package payee

import (
	"errors"
	"fmt"
)

// ErrMergeInTx is returned when MergePayees is called on a service bound to an
// existing transaction via InTx. DuckDB forbids deleting a parent row in the
// same transaction that dereferenced its foreign keys, so MergePayees must
// manage its own two-step transaction sequence.
var ErrMergeInTx = errors.New("cannot merge payees inside an existing transaction")

// MergeSameError is returned when trying to merge a payee into itself.
type MergeSameError struct {
	ID string
}

func (e *MergeSameError) Error() string {
	return fmt.Sprintf("cannot merge payee %s into itself", e.ID)
}
