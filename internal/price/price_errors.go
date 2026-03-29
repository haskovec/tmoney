package price

import "fmt"

// AlreadyExistsError is returned when trying to add a price that already exists
// for the same security and date.
type AlreadyExistsError struct {
	SecurityID string
	Date       string
	Detail     string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("price for security %s on %s already exists", e.SecurityID, e.Date)
}
